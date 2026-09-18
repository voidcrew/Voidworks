package spritepicker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
)

func TestSavedUnreferencedDMIsAreDiscovered(t *testing.T) {
	root := t.TempDir()
	write := func(path string) {
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"icons/zebra.dmi", "icons/Alpha.DMI", "icons/alpha.dmi", "new art/fresh.dmi", "icons/image.png", ".git/hidden.dmi", "data/checkout/.git", "data/checkout/icons/other.dmi"} {
		write(path)
	}
	files, err := scanDMIs(root)
	want := []string{"icons/Alpha.DMI", "icons/alpha.dmi", "new art/fresh.dmi", "icons/zebra.dmi"}
	// Windows has case-insensitive filenames.
	if _, err := os.Stat(filepath.Join(root, "icons/ALPHA.DMI")); err == nil {
		want = []string{"icons/Alpha.DMI", "new art/fresh.dmi", "icons/zebra.dmi"}
	}
	if err != nil || !reflect.DeepEqual(files, want) {
		t.Fatal(files, err)
	}
	write("icons/newly-saved.dmi")
	files, err = scanDMIs(root)
	if err != nil || !reflect.DeepEqual(filter(files, "NEWLY"), []string{"icons/newly-saved.dmi"}) {
		t.Fatal(files, err)
	}
	if path, err := resourcePath(root, filepath.Join(root, "new art/fresh.dmi")); err != nil || path != "new art/fresh.dmi" {
		t.Fatal(path, err)
	}
	for _, path := range []string{"../other.dmi", "icons/no.png", "", filepath.Join(filepath.Dir(root), "outside.dmi")} {
		if _, err := resourcePath(root, path); err == nil {
			t.Fatal("accepted nonportable resource", path)
		}
	}
}

func TestReplacementPreservesTypeAndOtherVariables(t *testing.T) {
	parent := dmvars.Set(&dmvars.Variables{}, "dir", "8")
	vars := dmvars.FromParent(parent)
	vars = dmvars.Set(vars, "icon", "'old.dmi'")
	vars = dmvars.Set(vars, "icon_state", `"old"`)
	vars = dmvars.Set(vars, "pixel_x", "12")
	vars = dmvars.Set(vars, "color", `"#ff0000"`)
	for _, path := range []string{"/obj/chair", "/turf/floor", "/mob/test", "/area/test"} {
		original := dmmprefab.New(0, path, vars)
		changed := replacement(original, "icons/new.dmi", "")
		if changed.Path() != path || changed.Vars().TextV("icon", "") != "icons/new.dmi" || changed.Vars().ValueV("icon_state", "bad") != `""` || changed.Vars().Parent() != parent || changed.Vars().IntV("dir", 0) != 8 || changed.Vars().IntV("pixel_x", 0) != 12 || changed.Vars().TextV("color", "") != "#ff0000" {
			t.Fatal("replacement lost mapped or inherited appearance")
		}
		if original.Vars().TextV("icon", "") != "old.dmi" || original.Vars().TextV("icon_state", "") != "old" {
			t.Fatal("mutated original")
		}
		quoted := replacement(original, "icons/artist's.dmi", "a\"[b]\\c")
		if quoted.Vars().ValueV("icon", "") != `'icons/artist\'s.dmi'` || quoted.Vars().ValueV("icon_state", "") != `"a\"\[b\]\\c"` {
			t.Fatal("unescaped DM literal")
		}
	}
}
