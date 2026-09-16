package shippreview

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type CleanupFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type CleanupPlan struct {
	Root      string        `json:"root"`
	Directory string        `json:"directory"`
	Files     []CleanupFile `json:"files"`
}

// ScanCleanup only reads the project. Call off the UI thread; hashes allow the
// worker to skip any file changed after review, even while another editor saves.
func (c *Client) ScanCleanup(root string) (*CleanupPlan, error) {
	helper, err := workerPath(c.folder(root))
	if err != nil {
		return nil, err
	}
	executable, args, environmentVars, err := c.previewCommand()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, append(args, "-B", helper, "--scan", root)...)
	cmd.Env = environmentVars
	configureProcess(cmd)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("could not review unused previews: %s (%w)", data, err)
	}
	var plan CleanupPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// RequestCleanup rebuilds the current manifest before considering the exact
// filenames and hashes the user approved. It uses the normal serialized queue.
func (c *Client) RequestCleanup(root, environment string, plan CleanupPlan) error {
	if len(plan.Files) == 0 {
		return nil
	}
	folder := c.folder(root)
	if err := os.MkdirAll(folder, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(folder, "approved-cleanup-*.json")
	if err != nil {
		return err
	}
	err = json.NewEncoder(file).Encode(plan)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(file.Name())
		return fmt.Errorf("could not save cleanup review: %v %v", err, closeErr)
	}
	c.request(root, environment, false, file.Name())
	return nil
}

func (c *Client) CleanupBackups(root string) string {
	return filepath.Join(c.folder(root), "cleanup-backups")
}
