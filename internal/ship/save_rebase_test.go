package ship

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const loadedDme = "#include \"code\\a.dm\"\r\n#include \"voidcrew\\nanites\\hub.dm\"\r\n#include \"voidcrew\\nanites\\programmer.dm\"\r\n#include \"voidcrew\\ships\\old.dm\"\r\n"

// The game update removed the nanite sources and added a new file.
const updatedDme = "#include \"code\\a.dm\"\r\n#include \"code\\b.dm\"\r\n#include \"voidcrew\\ships\\old.dm\"\r\n"

func TestRebaseIncludesKeepsGameUpdate(t *testing.T) {
	after := []byte(AddInclude([]byte(loadedDme), "/root", "/root/voidcrew/ships/mine.dm"))
	after = []byte(strings.Replace(string(after), "#include \"voidcrew\\ships\\old.dm\"\r\n", "", 1))
	merged, ok := rebaseIncludes([]byte(loadedDme), after, []byte(updatedDme))
	if !ok {
		t.Fatal("include-only edit was not rebased")
	}
	got := string(merged)
	for _, want := range []string{"code\\b.dm", "voidcrew\\ships\\mine.dm"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	for _, gone := range []string{"nanites", "old.dm"} {
		if strings.Contains(got, gone) {
			t.Fatalf("kept %s:\n%s", gone, got)
		}
	}
	if strings.Count(got, "\r\n") != strings.Count(got, "\n") {
		t.Fatalf("line endings changed:\n%q", got)
	}
}

func TestRebaseIncludesRefusesOtherEdits(t *testing.T) {
	after := loadedDme + "#define SOMETHING 1\r\n"
	if _, ok := rebaseIncludes([]byte(loadedDme), []byte(after), []byte(updatedDme)); ok {
		t.Fatal("a non-include edit was rebased")
	}
}

func TestSavingAfterGameUpdateKeepsUpdatedEnvironment(t *testing.T) {
	for _, withWarnings := range []bool{false, true} {
		root := t.TempDir()
		dme := filepath.Join(root, "tgstation.dme")
		if err := os.WriteFile(dme, []byte(updatedDme), 0600); err != nil {
			t.Fatal(err)
		}
		// The workshop loaded the project before the update was pulled.
		changes := []FileChange{{Path: dme, Before: []byte(loadedDme), Existed: true,
			After: AddInclude([]byte(loadedDme), root, filepath.Join(root, "voidcrew/ships/mine.dm"))}}
		var warnings []SaveWarning
		var err error
		if withWarnings {
			err = writeChangesWithWarnings(root, changes, os.Rename, &warnings, map[string]bool{})
		} else {
			err = WriteChanges(root, changes)
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(dme)
		if strings.Contains(string(data), "nanites") || !strings.Contains(string(data), "code\\b.dm") ||
			!strings.Contains(string(data), "voidcrew\\ships\\mine.dm") {
			t.Fatalf("saved environment lost the update or the new ship:\n%s", data)
		}
		if len(warnings) != 0 {
			t.Fatalf("rebased environment was reported as replaced: %+v", warnings)
		}
		if string(changes[0].After) != string(data) || string(changes[0].Before) != updatedDme {
			t.Fatal("caller was not told what was written")
		}
	}
}

func TestEnvironmentWithOtherEditsStillFollowsSaveRules(t *testing.T) {
	root := t.TempDir()
	dme := filepath.Join(root, "tgstation.dme")
	if err := os.WriteFile(dme, []byte(updatedDme), 0600); err != nil {
		t.Fatal(err)
	}
	changes := []FileChange{{Path: dme, Before: []byte(loadedDme), Existed: true, After: []byte(loadedDme + "#define X 1\r\n")}}
	if WriteChanges(root, changes) == nil {
		t.Fatal("strict save replaced an externally changed environment")
	}
}
