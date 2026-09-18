package ship

import (
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/util"
)

func TestDisplayPreservesAtomsWithoutPersistingPreviewPrefabs(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)
	known := prefab("/obj/display_test", "pixel_w", "-7")
	unknown := prefab("/obj/display_test_removed", "name", `"keep me"`)
	defaults := prefab(known.Path(), "icon", `'test.dmi'`, "pixel_z", "4")
	dme := &dmenv.Dme{RootDir: t.TempDir(), Objects: map[string]*dmenv.Object{known.Path(): {Vars: defaults.Vars()}}}
	local, displayed := util.Point{X: 1, Y: 1, Z: 1}, util.Point{X: 2, Y: 2, Z: 1}
	live := dmminstance.New(local, known)
	for _, instance := range []*dmminstance.Instance{nil, live} {
		a := &Assembly{MaxX: 3, MaxY: 3, MaxZ: 1, Cells: map[util.Point][]Atom{displayed: {{Prefab: known, Instance: instance}, {Prefab: unknown}}}}
		view, err := a.Display(dme)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Tiles) != 9 || len(view.GetTile(local).Instances()) != 0 {
			t.Fatal("empty display tiles were lost")
		}
		atoms := view.GetTile(displayed).Instances()
		if len(atoms) != 2 || atoms[0].Prefab() != known || atoms[1].Prefab() != unknown {
			t.Fatal("display dropped or reordered known/unknown atoms")
		}
		if instance != nil && (atoms[0] == live || atoms[0].Id() != live.Id() || live.Coord() != local) {
			t.Fatal("display changed source identity or coordinates")
		}
		if atoms[0].Coord() != displayed || atoms[0].Prefab().Vars().IntV("pixel_w", 0) != -7 || atoms[0].Prefab().Vars().IntV("pixel_z", 0) != 4 {
			t.Fatal("display lost position, overrides or inherited variables")
		}
		for _, p := range []string{known.Path(), unknown.Path()} {
			if len(dmmap.PrefabStorage.GetAllByPath(p)) != 0 {
				t.Fatal("display persisted a temporary prefab", p)
			}
		}
	}
}
