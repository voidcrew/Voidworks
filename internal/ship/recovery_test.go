package ship

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func assertRecoveryChanges(t *testing.T, p *Project) *Project {
	t.Helper()
	before, err := p.Changes()
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.CaptureRecovery()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RecoverProject(p.Catalog, p.Dme, data)
	if err != nil {
		t.Fatal(err)
	}
	after, err := restored.Changes()
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("recovery changed the set of pending files")
	}
	for i, c := range before {
		v := after[i]
		if c.Path != v.Path || c.Existed != v.Existed || c.Delete != v.Delete || !bytes.Equal(c.Before, v.Before) {
			t.Fatalf("recovery changed the baseline for %s", c.Path)
		}
		if filepath.Ext(c.Path) == ".dmm" && !c.Delete {
			a, err := dmmdata.Read(c.Path, bytes.NewReader(c.After))
			if err != nil {
				t.Fatal(err)
			}
			b, err := dmmdata.Read(v.Path, bytes.NewReader(v.After))
			if err != nil {
				t.Fatal(err)
			}
			if a.MaxX != b.MaxX || a.MaxY != b.MaxY || a.MaxZ != b.MaxZ || len(a.Grid) != len(b.Grid) {
				t.Fatal("recovery changed map dimensions")
			}
			for at, key := range a.Grid {
				if !a.Dictionary[key].Equals(b.Dictionary[b.Grid[at]]) {
					t.Fatalf("recovery changed map content at %v in %s", at, c.Path)
				}
			}
		} else if !bytes.Equal(c.After, v.After) {
			t.Fatalf("recovery changed generated definitions in %s", c.Path)
		}
	}

	if !restored.Modified() {
		t.Fatal("recovered draft was marked saved")
	}
	return restored
}
func TestRecoverUnsavedShipWithRoomsVariantsAndUnknownTypes(t *testing.T) {
	c, d := authorEnvironment(t)
	p, err := NewProject(c, d, "recovery_ship", "Recovery Ship", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Deck(p.Hull.Themes[0], util.Point{X: 4, Y: 5, Z: 1}, util.Point{X: 7, Y: 8, Z: 1}); err != nil {
		t.Fatal(err)
	}
	if err = p.addRect(0, "cargo", "Cargo", util.Point{X: 4, Y: 5, Z: 1}, util.Point{X: 7, Y: 8, Z: 1}); err != nil {
		t.Fatal(err)
	}
	if err = p.AddTheme(0, "rescue", "Rescue", true); err != nil {
		t.Fatal(err)
	}
	file, _ := c.HullFile(p.Hull, p.Hull.Themes[0])
	vars := &dmvars.MutableVariables{}
	vars.Put("name", `"Still here"`)
	p.Documents[file].Map.GetTile(util.Point{X: 5, Y: 5, Z: 1}).InstancesAdd(dmmap.PrefabStorage.Put(dmmprefab.New(0, "/obj/removed", vars.ToImmutable())))
	restored := assertRecoveryChanges(t, p)
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("autosave wrote a game map")
	}
	if err = restored.Save(); err != nil {
		t.Fatal(err)
	}
	if restored.Modified() {
		t.Fatal("normal Save did not settle recovered edits")
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("/obj/removed")) || !bytes.Contains(content, []byte("Still here")) {
		t.Fatal("recovery lost unknown mapped overrides")
	}
}
func TestRecoverHandwrittenSourceEditsAndOutsideConflict(t *testing.T) {
	p, _, _ := variantShipProject(t)
	if err := p.AddTheme(0, "rescue", "Rescue", false); err != nil {
		t.Fatal(err)
	}
	if err := p.Rename("module/cargo_basic", "Supplies"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPartCosts("theme/rescue", PartCosts{"science": 8}); err != nil {
		t.Fatal(err)
	}
	restored := assertRecoveryChanges(t, p)
	changes, err := restored.Changes()
	if err != nil {
		t.Fatal(err)
	}
	var changed FileChange
	for _, c := range changes {
		if c.Existed {
			changed = c
			break
		}
	}
	if changed.Path == "" {
		t.Fatal("missing saved source fixture")
	}
	external := append(append([]byte(nil), changed.Before...), []byte(`
// outside edit
`)...)
	if err = os.WriteFile(changed.Path, external, 0600); err != nil {
		t.Fatal(err)
	}
	if err = restored.Save(); err == nil {
		t.Fatal("recovery overwrote an external edit")
	}
	got, _ := os.ReadFile(changed.Path)
	if !bytes.Equal(got, external) {
		t.Fatal("failed Save changed outside content")
	}
}
func TestRecoveryRejectsDifferentEnvironmentAndEscapingPaths(t *testing.T) {
	c, d := authorEnvironment(t)
	p, err := NewProject(c, d, "guarded", "Guarded", 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.CaptureRecovery()
	if err != nil {
		t.Fatal(err)
	}
	var r recoveryProject
	if err = json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	r.Environment = filepath.Join(c.Root, "other.dme")
	bad, _ := json.Marshal(r)
	if _, err = RecoverProject(c, d, bad); err == nil {
		t.Fatal("accepted another environment")
	}
	r.Environment = d.RootFile
	r.Files[filepath.Join(c.Root, "..", "outside.dm")] = FileChange{Path: filepath.Join(c.Root, "..", "outside.dm")}
	bad, _ = json.Marshal(r)
	if _, err = RecoverProject(c, d, bad); err == nil {
		t.Fatal("accepted a path outside the project")
	}
}

func TestRecoveryReadsPreviousVersionAndRejectsUnknownVersion(t *testing.T) {
	c, d := authorEnvironment(t)
	p, err := NewProject(c, d, "compatible", "Compatible", 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.CaptureRecovery()
	if err != nil {
		t.Fatal(err)
	}
	var snapshot recoveryProject
	if err = json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 2 {
		t.Fatal("new recovery is not protected from older readers")
	}
	snapshot.Version = 1
	legacy, _ := json.Marshal(snapshot)
	if _, err = RecoverProject(c, d, legacy); err != nil {
		t.Fatalf("cannot restore an earlier beta's draft: %v", err)
	}
	snapshot.Version = 999
	unknown, _ := json.Marshal(snapshot)
	if _, err = RecoverProject(c, d, unknown); err == nil {
		t.Fatal("accepted an unsupported recovery format")
	}
}
