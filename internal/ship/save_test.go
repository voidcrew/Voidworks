package ship

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConflictTouchesNoDestination(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	_ = os.WriteFile(a, []byte("original a"), 0600)
	_ = os.WriteFile(b, []byte("external b"), 0600)
	changes := []FileChange{{Path: a, Before: []byte("original a"), After: []byte("edited a"), Existed: true}, {Path: b, Before: []byte("original b"), After: []byte("edited b"), Existed: true}}
	if WriteChanges(root, changes) == nil {
		t.Fatal("external edit accepted")
	}
	got, _ := os.ReadFile(a)
	if string(got) != "original a" {
		t.Fatal("first destination changed")
	}
	got, _ = os.ReadFile(b)
	if string(got) != "external b" {
		t.Fatal("external edit overwritten")
	}
}
func TestSaveFailureRollsBackEveryReplacement(t *testing.T) {
	root := t.TempDir()
	var changes []FileChange
	for _, name := range []string{"a", "b", "c"} {
		path := filepath.Join(root, name)
		_ = os.WriteFile(path, []byte(name), 0600)
		changes = append(changes, FileChange{Path: path, Before: []byte(name), After: []byte("new " + name), Existed: true})
	}
	count := 0
	err := writeChanges(root, changes, func(from, to string) error {
		count++
		if count == 4 {
			return errors.New("injected rename failure")
		}
		return os.Rename(from, to)
	})
	if err == nil {
		t.Fatal("failure ignored")
	}
	for _, c := range changes {
		got, _ := os.ReadFile(c.Path)
		if !bytes.Equal(got, c.Before) {
			t.Fatalf("not restored: %s", c.Path)
		}
	}
	files, _ := os.ReadDir(root)
	if len(files) != 3 {
		t.Fatalf("temporary files left: %v", files)
	}
}
func TestSaveNewFileCollisionAndEscape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing")
	_ = os.WriteFile(path, []byte("keep"), 0600)
	if WriteChanges(root, []FileChange{{Path: path, After: []byte("replace")}}) == nil {
		t.Fatal("new file overwrote existing")
	}
	if WriteChanges(root, []FileChange{{Path: filepath.Join(root, "..", "outside"), After: []byte("bad")}}) == nil {
		t.Fatal("path escaped")
	}
}

func TestSaveWarningsPreserveDiskVersions(t *testing.T) {
	for _, mode := range []string{"modified", "created", "deleted", "removed"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "ship.dm")
			c := FileChange{Path: path, Before: []byte("original"), After: []byte("editor"), Existed: mode != "created"}
			if mode != "deleted" {
				if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "removed" {
				c.Delete, c.After = true, nil
			}
			var warnings []SaveWarning
			if err := writeChangesWithWarnings(root, []FileChange{c}, os.Rename, &warnings, nil); err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 1 || warnings[0].Path != path {
				t.Fatalf("missing warning: %+v", warnings)
			}
			got, err := os.ReadFile(path)
			if mode == "removed" {
				if !os.IsNotExist(err) {
					t.Fatal("file not removed", err)
				}
			} else if err != nil || string(got) != "editor" {
				t.Fatal("editor version not saved", err)
			}
			if mode != "deleted" {
				backup, err := os.ReadFile(warnings[0].Backup)
				if err != nil || string(backup) != "external" {
					t.Fatal("external version not retained", err)
				}
			}
		})
	}
}

func TestSaveWarningsRollbackRestoresExternalVersion(t *testing.T) {
	root := t.TempDir()
	var changes []FileChange
	for _, name := range []string{"a", "b"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("external "+name), 0600); err != nil {
			t.Fatal(err)
		}
		changes = append(changes, FileChange{Path: path, Before: []byte("original"), After: []byte("editor"), Existed: true})
	}
	var warnings []SaveWarning
	count := 0
	err := writeChangesWithWarnings(root, changes, func(from, to string) error {
		count++
		if count == 4 {
			return errors.New("injected failure")
		}
		return os.Rename(from, to)
	}, &warnings, nil)
	if err == nil || len(warnings) != 0 {
		t.Fatal("failed save reported success")
	}
	for _, c := range changes {
		got, err := os.ReadFile(c.Path)
		if err != nil || string(got) != "external "+filepath.Base(c.Path) {
			t.Fatal("external version not restored", err)
		}
	}
}
