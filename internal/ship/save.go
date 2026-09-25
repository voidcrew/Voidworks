package ship

import (
	"bytes"
	"fmt"
	"github.com/rs/zerolog/log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sdmm/internal/dmapi/dminclude"
)

type FileChange struct {
	Path          string
	Before, After []byte
	Existed       bool
	Delete        bool
}

func (c FileChange) unchanged() error {
	data, err := os.ReadFile(c.Path)
	if !c.Existed && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot check %s: %w", c.Path, err)
	}
	if !c.Existed || !bytes.Equal(data, c.Before) {
		return fmt.Errorf("%s changed outside this project; save stopped", c.Path)
	}
	return nil
}

// WriteChanges stages every file before replacing any destination, checks for
// external edits, and rolls back replacements if a filesystem operation fails.
func WriteChanges(root string, changes []FileChange) error {
	return writeChanges(root, changes, os.Rename)
}

func writeChanges(root string, changes []FileChange, rename func(string, string) error) error {
	return writeChangesWithWarnings(root, changes, rename, nil, nil)
}

var includeDirective = regexp.MustCompile(`^[\t ]*#include[\t ]+"([^"\r\n]+)"`)

// rebaseEnvironment applies a workshop's .dme edit to the file's current disk
// contents when the .dme changed after the project loaded. Workshops only add
// and remove #include lines; replacing the whole file instead would undo a
// game update pulled while the editor was open and re-add deleted sources.
func rebaseEnvironment(c FileChange) (FileChange, bool) {
	if !strings.EqualFold(filepath.Ext(c.Path), ".dme") || c.Delete || !c.Existed {
		return c, false
	}
	disk, err := os.ReadFile(c.Path)
	if err != nil || bytes.Equal(disk, c.Before) {
		return c, false
	}
	merged, ok := rebaseIncludes(c.Before, c.After, disk)
	if !ok {
		return c, false
	}
	log.Info().Str("file", c.Path).Msg("Applied workshop includes to the changed environment file")
	c.Before, c.After = disk, merged
	return c, true
}

// rebaseIncludes applies the #include lines added and removed between before
// and after to disk. It refuses edits that change anything else.
func rebaseIncludes(before, after, disk []byte) ([]byte, bool) {
	count := map[string]int{}
	for _, line := range strings.Split(string(before), "\n") {
		count[strings.TrimRight(line, "\r")]--
	}
	for _, line := range strings.Split(string(after), "\n") {
		count[strings.TrimRight(line, "\r")]++
	}
	added, removed := []string{}, map[string]bool{}
	for line, n := range count {
		if n == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		match := includeDirective.FindStringSubmatch(line)
		if match == nil {
			return nil, false
		}
		if n > 0 {
			added = append(added, match[1])
		} else {
			removed[includeKey(match[1])] = true
		}
	}
	var result strings.Builder
	for _, line := range strings.SplitAfter(string(disk), "\n") {
		if match := includeDirective.FindStringSubmatch(line); match != nil && removed[includeKey(match[1])] {
			continue
		}
		result.WriteString(line)
	}
	merged := []byte(result.String())
	sort.Strings(added)
	for _, name := range added {
		merged = dminclude.Add(merged, name)
	}
	return merged, true
}

func includeKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
}

// SaveWarning identifies a replaced file and the retained copy of its disk contents.
type SaveWarning struct {
	Path, Backup string
}

func writeChangesWithWarnings(root string, changes []FileChange, rename func(string, string) error, warnings *[]SaveWarning, preserve map[string]bool) error {
	type staged struct {
		change         FileChange
		temp, backup   string
		moved, written bool
		conflict       bool
	}
	files := make([]staged, 0, len(changes))
	seen := map[string]bool{}
	defer func() {
		for _, f := range files {
			if f.temp != "" {
				_ = os.Remove(f.temp)
			}
		}
	}()
	for i, c := range changes {
		rel, err := filepath.Rel(root, c.Path)
		if err != nil {
			return err
		}
		path, err := Inside(root, rel)
		if err != nil {
			return err
		}
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			return fmt.Errorf("duplicate save destination %s", path)
		}
		seen[key] = true
		c.Path = path
		conflict := preserve[path]
		if rebased, ok := rebaseEnvironment(c); ok {
			// Keep the disk version's other edits, such as a game update, and
			// report what was actually written back to the caller.
			c = rebased
			changes[i].Before, changes[i].After = c.Before, c.After
		} else if warnings != nil {
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			exists := err == nil
			conflict = conflict || exists != c.Existed || !bytes.Equal(data, c.Before)
			// Stage against the current disk version so rollback restores it.
			c.Before, c.Existed = data, exists
		} else if err := c.unchanged(); err != nil {
			return err
		}
		if c.Delete && (len(c.After) != 0 || warnings == nil && !c.Existed) {
			return fmt.Errorf("invalid deletion: %s", path)
		}
		if c.Delete && !c.Existed {
			continue
		}
		if !c.Delete && c.Existed && bytes.Equal(c.Before, c.After) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		temp, err := os.CreateTemp(filepath.Dir(path), ".ship-save-*")
		if err != nil {
			return err
		}
		files = append(files, staged{change: c, temp: temp.Name(), backup: temp.Name() + ".previous", conflict: conflict})
		if _, err = temp.Write(c.After); err == nil {
			err = temp.Sync()
		}
		closeErr := temp.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		mode := os.FileMode(0644)
		if c.Existed {
			if info, e := os.Stat(path); e == nil {
				mode = info.Mode().Perm()
			}
		}
		if err := os.Chmod(temp.Name(), mode); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := f.change.unchanged(); err != nil {
			return err
		}
	}
	rollback := func(cause error) error {
		var failures []string
		for i := len(files) - 1; i >= 0; i-- {
			f := &files[i]
			if f.written {
				data, err := os.ReadFile(f.change.Path)
				if err != nil || !bytes.Equal(data, f.change.After) {
					failures = append(failures, "keep recovery copy "+f.backup)
					continue
				}
				if err := os.Remove(f.change.Path); err != nil {
					failures = append(failures, err.Error())
					continue
				}
			}
			if f.moved {
				if err := os.Rename(f.backup, f.change.Path); err != nil {
					failures = append(failures, "recovery copy: "+f.backup+": "+err.Error())
				}
			}
		}
		if len(failures) > 0 {
			return fmt.Errorf("%w; rollback needs attention: %s", cause, strings.Join(failures, "; "))
		}
		return cause
	}
	for i := range files {
		f := &files[i]
		if err := f.change.unchanged(); err != nil {
			return rollback(err)
		}
		if f.change.Existed {
			if err := rename(f.change.Path, f.backup); err != nil {
				return rollback(err)
			}
			f.moved = true
		}
		if f.change.Delete {
			continue
		}
		if err := rename(f.temp, f.change.Path); err != nil {
			return rollback(err)
		}
		f.written = true
	}
	for _, f := range files {
		if f.conflict && warnings != nil {
			warning := SaveWarning{Path: f.change.Path}
			if f.moved {
				warning.Backup = f.backup
			}
			*warnings = append(*warnings, warning)
			continue
		}
		if f.moved {
			if err := os.Remove(f.backup); err != nil {
				log.Warn().Err(err).Msg("Files saved; recovery copy retained at " + f.backup)
			}
		}
	}
	return nil
}
