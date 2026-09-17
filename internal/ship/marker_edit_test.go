package ship

import (
	"bytes"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/util"
)

func TestShipMappingHelpersCanMoveAndDelete(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "movable_markers", "Movable markers", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddSlot(0, "cargo", "Cargo", pt(4, 4), FullFootprint(3, 3)); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	// Reloading exercises the protection applied to existing ships as well as
	// newly authored rooms. Normal map tabs must retain the same edit semantics.
	p, err = OpenProject(c, env, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	a, err := p.Assemble(p.Hull.Themes[0], map[string]string{"cargo": p.Hull.Modules[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range a.Sources {
		for _, ordinary := range []bool{false, true} {
			name := src.Name + "/workshop"
			if ordinary {
				name = src.Name + "/map-tab"
			}
			t.Run(name, func(t *testing.T) {
				m := src.Live
				beforeProject := p.Capture()
				defer p.Restore(beforeProject)
				if ordinary {
					m, _ = dmmap.New(env, RawData(m), "")
				}
				var start util.Point
				var markerPrefab dmmdata.Prefabs
				for _, tile := range m.Tiles {
					for _, instance := range tile.Instances() {
						if mappingMarker(instance.Prefab().Path()) {
							start = tile.Coord
							markerPrefab = dmmdata.Prefabs{instance.Prefab()}
						}
					}
				}
				if len(markerPrefab) != 1 {
					t.Fatal("fixture needs a mapping helper")
				}
				at := start
				check := func(want util.Point, count int) {
					t.Helper()
					got := 0
					for _, tile := range m.Tiles {
						tile.InstancesRegenerate()
						for _, inst := range tile.Instances() {
							if !mappingMarker(inst.Prefab().Path()) {
								continue
							}
							got++
							if inst.Coord() != want || inst.Prefab().Id() != markerPrefab[0].Id() {
								t.Fatalf("helper regenerated or changed: got %v, want %v", inst.Coord(), want)
							}
						}
					}
					if got != count {
						t.Fatalf("got %d helpers, want %d", got, count)
					}
				}
				// The move tool removes the source instance, regenerates the tile,
				// then inserts the original prefab at the destination.
				for _, to := range []util.Point{start.Plus(util.Point{X: 1}), start} {
					m.GetTile(at).InstancesRemoveByPath(markerPrefab[0].Path())
					m.GetTile(at).InstancesRegenerate()
					m.GetTile(to).InstancesAdd(markerPrefab[0])
					check(to, 1)
					at = to
				}
				original := RawData(m).EncodeTGM()
				snap := dmmsnap.New(m)
				m.GetTile(at).InstancesRemoveByPath(markerPrefab[0].Path())
				m.GetTile(at).InstancesRegenerate()
				deleted, _ := snap.Commit()
				check(util.Point{}, 0)
				snap.GoTo(deleted - 1)
				check(start, 1)
				if !bytes.Equal(original, RawData(m).EncodeTGM()) {
					t.Fatal("undo did not restore the exact map")
				}
				snap.GoTo(deleted)
				check(util.Point{}, 0)
			})
		}
	}
}
