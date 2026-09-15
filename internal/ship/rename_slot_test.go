package ship

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
)

func TestRenameSlotSaveRecoveryAndUndo(t *testing.T) {
	for _, handwritten := range []bool{false, true} {
		t.Run(map[bool]string{false: "authored", true: "handwritten"}[handwritten], func(t *testing.T) {
			p, source := loadedRoomProject(t, true)
			registerFixtureModules(p, source)
			if !handwritten {
				p.Settings = &Settings{Version: 1, ID: "loaded_rooms", Crew: 1, PortDirection: 1}
				for _, path := range p.outputPaths() {
					if err := p.track(path); err != nil {
						t.Fatal(err)
					}
				}
				if err := p.Save(); err != nil {
					t.Fatal(err)
				}
			}
			if key, err := p.RenameSlot("original", "Original"); err != nil || key != "original" || p.Modified() {
				t.Fatalf("keeping the same room name changed it: %s %v", key, err)
			}
			if err := p.PrepareSlotRename("original", `Forward "Cargo" [A]`); err != nil {
				t.Fatal(err)
			}
			hulls, err := p.hullMaps()
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hulls {
				_, _, marker := slotMarker(h.doc.Map, "original")
				if marker == nil {
					t.Fatal("fixture has no room marker")
				}
				f := marker.Prefab()
				marker.SetPrefab(dmmap.PrefabStorage.Put(dmmprefab.New(0, f.Path(), dmvars.Set(f.Vars(), "name", dmQuote("keep marker name")))))
			}
			if err := p.Save(); err != nil {
				t.Fatal(err)
			}
			before := p.Capture()
			beforeSource, _ := os.ReadFile(source)
			key, err := p.RenameSlot("original", `Forward "Cargo" [A]`)
			if err != nil {
				t.Fatal(err)
			}
			if SlotDisplayName(key) != `Forward "Cargo" [A]` {
				t.Fatal("display name lost case or punctuation")
			}
			expected := cloneHull(before.Hull)
			for i := range expected.Slots {
				if expected.Slots[i] == "original" {
					expected.Slots[i] = key
				}
			}
			for i := range expected.Themes {
				for j := range expected.Themes[i].Slots {
					if expected.Themes[i].Slots[j] == "original" {
						expected.Themes[i].Slots[j] = key
					}
				}
			}
			for i := range expected.Modules {
				expected.Modules[i].Slot = key
			}
			if !reflect.DeepEqual(expected, p.Hull) {
				t.Fatal("renaming changed option metadata or missed a registration")
			}
			for _, h := range hulls {
				for _, tile := range h.doc.Map.Tiles {
					tile.InstancesRegenerate()
				}
				_, _, marker := slotMarker(h.doc.Map, key)
				if marker == nil || text(marker.Prefab().Vars(), "name") != "keep marker name" {
					t.Fatal("rename lost a hull marker or its overrides")
				}
				if _, _, old := slotMarker(h.doc.Map, "original"); old != nil {
					t.Fatal("old hull marker key remains")
				}
			}
			assertRecoveryChanges(t, p)
			after := p.Capture()
			if err = p.Save(); err != nil || p.Modified() {
				t.Fatalf("rename did not save cleanly: %v", err)
			}
			savedSource, _ := os.ReadFile(source)
			if !bytes.Contains(savedSource, []byte("slot = "+dmQuote(key))) || !bytes.Contains(savedSource, []byte("custom_proc()")) && handwritten {
				t.Fatal("saved source lost the new slot or custom code")
			}
			reopened, err := OpenProject(p.Catalog, p.Dme, p.Hull)
			if err != nil || reopened.Modified() {
				t.Fatalf("failed to reopen renamed ship: %v", err)
			}
			for _, theme := range reopened.Hull.Themes {
				m := reopened.defaultModule(key, theme.ID)
				if m == nil {
					t.Fatal("renamed room lost its default option")
				}
				a, err := reopened.Assemble(theme, map[string]string{key: m.ID})
				if err != nil || len(a.Sources) != 2 {
					t.Fatalf("renamed variant cannot assemble: %v", err)
				}
			}
			p.Restore(before)
			if err = p.Save(); err != nil {
				t.Fatal(err)
			}
			restored, _ := os.ReadFile(source)
			if !bytes.Equal(beforeSource, restored) {
				t.Fatal("undo after Save changed the original definitions")
			}
			for file, m := range before.Maps {
				d, err := p.document(file)
				if err != nil || !bytes.Equal(RawData(&m).EncodeTGM(), RawData(d.Map).EncodeTGM()) {
					t.Fatalf("undo lost a variant map: %v", err)
				}
			}
			p.Restore(after)
			if err = p.Save(); err != nil {
				t.Fatal(err)
			}
			redone, _ := os.ReadFile(source)
			if !bytes.Equal(savedSource, redone) {
				t.Fatal("redo did not restore the rename")
			}
			key, err = p.RenameSlot(key, "Second Name")
			if err != nil {
				t.Fatal(err)
			}
			if err = p.Save(); err != nil {
				t.Fatal(err)
			}
			if _, err = p.RenameSlot(key, "Original"); err != nil {
				t.Fatal(err)
			}
			if err = p.Save(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRenameSlotRejectsCollisionsAndPreservesExternalEdits(t *testing.T) {
	p, source := loadedRoomProject(t, true)
	registerFixtureModules(p, source)
	p.Hull.Modules = append(p.Hull.Modules, Module{ID: "hidden", Slot: "crew_quarters", Name: "Hidden", Themes: []string{"disabled"}})
	for _, name := range []string{"", "   ", " HULL ", "CREW   QUARTERS", "crew_quarters"} {
		if _, err := p.RenameSlot("original", name); err == nil {
			t.Fatalf("accepted collision %q", name)
		}
	}
	p.Hull.Modules = p.Hull.Modules[:len(p.Hull.Modules)-1]
	if _, err := p.RenameSlot("missing", "New room"); err == nil {
		t.Fatal("renamed missing room")
	}
	if _, err := p.RenameSlot("original", "New Room"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(source)
	changed := append(data, []byte("\n// edit outside the editor\n")...)
	if err := os.WriteFile(source, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err == nil || !strings.Contains(err.Error(), "changed outside") {
		t.Fatalf("external edit was overwritten: %v", err)
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(changed, after) {
		t.Fatal("failed Save changed external source")
	}
}

func TestRenameSlotWithNewVariantAndInheritedLists(t *testing.T) {
	p, source := loadedRoomProject(t, true)
	registerFixtureModules(p, source)
	if err := p.AddTheme(0, "new_variant", "New Variant", false); err != nil {
		t.Fatal(err)
	}
	if err := p.SetThemeSlots("other", nil); err != nil {
		t.Fatal(err)
	}
	key, err := p.RenameSlot("original", "New Bay")
	if err != nil {
		t.Fatal(err)
	}
	if p.Hull.Themes[p.themeIndex("other")].Slots != nil {
		t.Fatal("rename replaced an inherited list")
	}
	if !Contains(p.Hull.Themes[p.themeIndex("new_variant")].Slots, key) {
		t.Fatal("new variant kept its old room")
	}
	assertRecoveryChanges(t, p)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
}
