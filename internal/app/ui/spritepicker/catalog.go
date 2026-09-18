package spritepicker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
)

// ResourcePath keeps saved map references portable, including newly created
// icons which have never been referenced by a DM type.
func resourcePath(root, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("Choose a DMI file.")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("Save or copy this DMI into the project folder first.")
	}
	if !strings.EqualFold(filepath.Ext(rel), ".dmi") {
		return "", fmt.Errorf("Choose a .dmi file.")
	}
	return filepath.ToSlash(rel), nil
}

func alphabetical(items []string) {
	sort.Slice(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i]), strings.ToLower(items[j])
		if a == b {
			return items[i] < items[j]
		}
		return a < b
	})
}

func scanDMIs(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root {
				if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || entry.Name() == "vendor" {
					return filepath.SkipDir
				}
				// Do not browse another checkout nested inside this project.
				if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink == 0 && strings.EqualFold(filepath.Ext(path), ".dmi") {
			rel, err := resourcePath(root, path)
			if err == nil {
				files = append(files, rel)
			}
		}
		return nil
	})
	alphabetical(files)
	return files, err
}

func filter(items []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return items
	}
	var found []string
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), query) || item == "" && strings.Contains("(default)", query) {
			found = append(found, item)
		}
	}
	return found
}

// Only appearance overrides change; type, direction, tint and all other mapped
// variables remain intact. Nothing enters prefab storage until Apply.
func replacement(original *dmmprefab.Prefab, path, state string) *dmmprefab.Prefab {
	quote := func(value, delimiter string) string {
		value = strings.NewReplacer("\\", "\\\\", delimiter, "\\"+delimiter, "[", "\\[", "]", "\\]", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(value)
		return delimiter + value + delimiter
	}
	vars := dmvars.Set(original.Vars(), "icon", quote(path, "'"))
	vars = dmvars.Set(vars, "icon_state", quote(state, `"`))
	return dmmprefab.New(dmmprefab.IdNone, original.Path(), vars)
}
