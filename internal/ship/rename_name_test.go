package ship

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sdmm/internal/util"
)

func nameChangeProject(t *testing.T, authored bool) *Project {
	t.Helper()
	if !authored {
		return loadedRenameProject(t)
	}
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "original", "Original", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.addRect(0, "cargo", "Cargo", util.Point{X: 3, Y: 3, Z: 1}, util.Point{X: 5, Y: 5, Z: 1}); err != nil {
		t.Fatal(err)
	}
	if err = p.AddTheme(0, "medical", "Medical", true); err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestShipDisplayNamePreservesFilesAndReferences(t *testing.T) {
	for _, authored := range []bool{false, true} {
		label := "handwritten"
		if authored {
			label = "authored"
		}
		t.Run(label, func(t *testing.T) {
			p := nameChangeProject(t, authored)
			for _, theme := range p.Hull.Themes {
				if _, err := p.Assemble(theme, nil); err != nil {
					t.Fatal(err)
				}
			}
			originalHull := cloneHull(p.Hull)
			fileID := p.ShipFileName()
			includes, err := removalSources(p.Catalog.Root, p.Dme.RootFile)
			if err != nil {
				t.Fatal(err)
			}
			dme, _ := os.ReadFile(p.Dme.RootFile)
			maps := map[string][]byte{}
			for file, d := range p.Documents {
				if d.Active {
					maps[file], err = os.ReadFile(file)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			before := p.Capture()
			// A same-named destination must not block a display-only rename.
			collision := filepath.Join(p.Catalog.Root, "_maps/voidcrew/ships/ship_explorer.dmm")
			if err := os.WriteFile(collision, []byte("unrelated map"), 0600); err != nil {
				t.Fatal(err)
			}
			p.BeforeOpen = func(string) error { t.Fatal("display rename tried to move/open a map"); return nil }
			if err := p.RenameShip("Explorer"); err != nil {
				t.Fatal(err)
			}
			after := p.Capture()
			check := func(name string) {
				t.Helper()
				want := cloneHull(originalHull)
				want.Name = name
				if !reflect.DeepEqual(p.Hull, want) || p.ShipFileName() != fileID {
					t.Fatal("display name changed file references or identity")
				}
				for file, data := range maps {
					got, err := os.ReadFile(file)
					if err != nil || !bytes.Equal(got, data) {
						t.Fatalf("map changed: %s: %v", file, err)
					}
				}
				got, _ := os.ReadFile(p.Dme.RootFile)
				if !bytes.Equal(got, dme) {
					t.Fatal("display rename changed includes")
				}
				for file := range includes {
					if _, err := os.Stat(file); err != nil {
						t.Fatal("source moved:", file, err)
					}
				}
				got, _ = os.ReadFile(collision)
				if string(got) != "unrelated map" {
					t.Fatal("display rename overwrote another map")
				}
				reopened, err := OpenProject(p.Catalog, p.Dme, p.Hull)
				if err != nil || reopened.Hull.Name != name || reopened.Modified() {
					t.Fatal("saved display name did not reopen cleanly", err)
				}
			}
			if err := p.Save(); err != nil {
				t.Fatal(err)
			}
			check("Explorer")
			p.Restore(before)
			if err := p.Save(); err != nil {
				t.Fatal(err)
			}
			check(originalHull.Name)
			p.Restore(after)
			if err := p.Save(); err != nil {
				t.Fatal(err)
			}
			check("Explorer")
			p.BeforeOpen = nil
			if err := p.RenameShipFiles("separate_files"); err != nil {
				t.Fatal(err)
			}
			if err := p.Save(); err != nil {
				t.Fatal(err)
			}
			if p.Hull.Name != "Explorer" || p.ShipFileName() != "separate_files" {
				t.Fatal("file rename changed display name")
			}
			for file := range maps {
				if _, err := os.Stat(file); !os.IsNotExist(err) {
					t.Fatal("file rename left old dedicated map", file, err)
				}
			}
			if err := p.RenameShip("Second display name"); err != nil {
				t.Fatal(err)
			}
			if err := p.Save(); err != nil {
				t.Fatal(err)
			}
			if p.ShipFileName() != "separate_files" {
				t.Fatal("later display rename moved files")
			}
			reopened, err := OpenProject(p.Catalog, p.Dme, p.Hull)
			if err != nil || reopened.Hull.Name != "Second display name" || reopened.Modified() {
				t.Fatal("separate edits did not reopen", err)
			}
		})
	}
}
