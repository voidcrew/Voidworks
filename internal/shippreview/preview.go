// Package shippreview schedules the project's purchase-preview script after saves.
package shippreview

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const Script = "tools/ship_previews/generate_ship_previews.py"

//go:embed worker.py
var worker []byte

type Status struct {
	Phase        string  `json:"phase"`
	Queued       bool    `json:"queued"`
	PID          int     `json:"pid"`
	Updated      float64 `json:"updated"`
	Message, Log string
}

type Client struct {
	mu        sync.Mutex
	profile   string
	status    map[string]Status
	checked   map[string]time.Time
	requested map[string]time.Time
}

func New(profile string) *Client {
	return &Client{profile: profile, status: map[string]Status{}, checked: map[string]time.Time{}, requested: map[string]time.Time{}}
}

func Available(root string) bool {
	info, err := os.Stat(filepath.Join(root, Script))
	return err == nil && !info.IsDir()
}

func IsShipMap(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = strings.ToLower(filepath.ToSlash(rel))
	return strings.HasSuffix(rel, ".dmm") && (strings.HasPrefix(rel, "_maps/voidcrew/ships/") || strings.HasPrefix(rel, "_maps/voidcrew/ship_modules/"))
}

func (c *Client) folder(root string) string {
	key, _ := filepath.Abs(root)
	if canonical, err := filepath.EvalSymlinks(key); err == nil {
		key = canonical
	}
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return filepath.Join(c.profile, "ship-previews", fmt.Sprintf("%x", sha256.Sum256([]byte(key))))
}

func python() (string, []string, error) {
	configured := os.Getenv("VOIDWORKS_PYTHON")
	if configured == "" {
		configured = os.Getenv("STRONGDMM_PYTHON") // Existing local preview overrides.
	}
	if configured != "" {
		path, err := exec.LookPath(configured)
		return path, nil, err
	}
	for _, name := range []string{"python", "python3", "py"} {
		if path, err := exec.LookPath(name); err == nil {
			if name == "py" {
				return path, []string{"-3"}, nil
			}
			return path, nil, nil
		}
	}
	return "", nil, fmt.Errorf("install Python 3 with Pillow, then restart the editor")
}

// Request starts the helper before returning, without waiting for generation.
// This also covers Save when closing the editor: no pending launch goroutine
// can be lost during shutdown. The helper owns one queue per project.
func (c *Client) Request(root, environment string) {
	c.request(root, environment, false, "")
}

// RequestFull also refreshes unchanged maps after icon or rendering-code edits.
func (c *Client) RequestFull(root, environment string) {
	c.request(root, environment, true, "")
}

func (c *Client) Resume(root, environment string) {
	c.request(root, environment, false, "", "--resume")
}

func (c *Client) Stop(root, environment string) {
	c.request(root, environment, false, "", "--stop")
}

func (c *Client) request(root, environment string, force bool, cleanup string, options ...string) {
	folder := c.folder(root)
	if !force && cleanup == "" && len(options) == 0 {
		var queue struct{ Paused bool }
		data, _ := os.ReadFile(filepath.Join(folder, "request.json"))
		if json.Unmarshal(data, &queue) == nil && queue.Paused {
			return
		}
	}
	c.mu.Lock()
	c.status[folder] = Status{Phase: "starting", Message: "Starting preview generation...", Log: filepath.Join(folder, "generation.log")}
	if len(options) > 0 && options[0] == "--stop" {
		c.status[folder] = Status{Phase: "stopping", Message: "Stopping preview generation...", Log: filepath.Join(folder, "generation.log")}
	}
	c.checked[folder] = time.Now()
	c.requested[folder] = time.Now()
	c.mu.Unlock()
	if err := c.launch(folder, root, environment, force, cleanup, options...); err != nil {
		c.mu.Lock()
		c.status[folder] = Status{Phase: "error", Message: "Ship saved. Previews could not start: " + err.Error(), Log: filepath.Join(folder, "generation.log")}
		c.mu.Unlock()
	}
}

func workerPath(folder string) (string, error) {
	if err := os.MkdirAll(folder, 0700); err != nil {
		return "", err
	}
	// Versioned helpers let an older generation finish during an editor update.
	helper := filepath.Join(folder, fmt.Sprintf("worker-%x.py", sha256.Sum256(worker)))
	if _, err := os.Stat(helper); os.IsNotExist(err) {
		temp, err := os.CreateTemp(folder, "worker-*.tmp")
		if err != nil {
			return "", err
		}
		defer os.Remove(temp.Name())
		_, writeErr := temp.Write(worker)
		closeErr := temp.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if err = os.Rename(temp.Name(), helper); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return helper, nil
}

func (c *Client) launch(folder, root, environment string, force bool, cleanup string, options ...string) error {
	if !Available(root) && !(len(options) > 0 && options[0] == "--stop") {
		return fmt.Errorf("%s is missing from this project", Script)
	}
	executable, args, environmentVars, err := c.previewCommand()
	if err != nil {
		return err
	}
	helper, err := workerPath(folder)
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(folder, "launcher.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	args = append(args, "-B", "-u", helper, folder, root, environment)
	args = append(args, options...)
	if force {
		args = append(args, "--force")
	}
	if cleanup != "" {
		args = append(args, "--cleanup", cleanup)
	}
	cmd := exec.Command(executable, args...)
	cmd.Env = environmentVars
	cmd.Dir, cmd.Stdout, cmd.Stderr = root, log, log
	configureProcess(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.status[folder] = Status{Phase: "error", Message: "Preview helper failed: " + lastLine(filepath.Join(folder, "launcher.log")), Log: filepath.Join(folder, "launcher.log")}
		}
	}()
	return nil
}

func (c *Client) Status(root string) Status {
	folder := c.folder(root)
	c.mu.Lock()
	defer c.mu.Unlock()
	status := c.status[folder]
	if time.Since(c.checked[folder]) < time.Second {
		return status
	}
	c.checked[folder] = time.Now()
	if data, err := os.ReadFile(filepath.Join(folder, "status.json")); err == nil {
		var current Status
		if json.Unmarshal(data, &current) == nil && current.Updated >= float64(c.requested[folder].UnixMilli())/1000 {
			status = current
		}
	}
	status.Log = filepath.Join(folder, "generation.log")
	switch status.Phase {
	case "running":
		if status.PID > 0 && !processAlive(status.PID) {
			status.Phase, status.Message = "failed", "Preview generation was interrupted. Retry to refresh the saved ship."
			break
		}
		status.Message = "Refreshing purchase previews..."
		if status.Queued {
			status.Message += " Another refresh is queued."
		}
		if line := previewProgress(status.Log); line != "" {
			status.Message += "\n" + line
		}
	case "complete":
		status.Message = "Purchase previews are up to date."
		if line := lastLine(status.Log); strings.HasPrefix(line, "Preview images: ") {
			status.Message += "\n" + line
		}
	case "failed":
		status.Message = "Ship saved. Preview generation failed. Check the log, then retry."
	case "stopping":
		status.Message = "Stopping preview generation..."
		if status.PID > 0 && !processAlive(status.PID) {
			status.Phase = "stopped"
		}
		fallthrough
	case "stopped":
		if status.Phase == "stopped" {
			status.Message = "Preview generation is paused. Your ship saves are safe. Resume when ready; completed renders will be reused."
		}
	case "starting":
		status.Message = "Starting preview generation..."
		if time.Since(c.requested[folder]) > 20*time.Second {
			status.Phase, status.Message = "failed", "The preview helper did not start. Open the preview log, then retry."
			status.Log = filepath.Join(folder, "launcher.log")
		}
	}
	c.status[folder] = status
	return status
}

func lastLine(path string) string {
	lines := logTail(path)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(string(lines[len(lines)-1]))
}

func previewProgress(path string) string {
	lines := logTail(path)
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(string(lines[i]))
		if strings.HasPrefix(line, "Preview progress: ") || strings.HasPrefix(line, "Rendering changed or missing preview: ") {
			return line
		}
	}
	if len(lines) > 0 {
		return strings.TrimSpace(string(lines[len(lines)-1]))
	}
	return ""
}

func logTail(path string) [][]byte {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > 2048 {
		_, _ = file.Seek(-2048, io.SeekEnd)
	}
	data, _ := io.ReadAll(io.LimitReader(file, 2048))
	return bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
}
