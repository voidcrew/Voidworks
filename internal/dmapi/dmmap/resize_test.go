package dmmap

import (
	"testing"

	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func TestResizeDirectionsPreserveTilesAndInstances(t *testing.T) {
	oldTurf, oldArea := BaseTurf, BaseArea
	defer func() { BaseTurf, BaseArea = oldTurf, oldArea }()
	vars := dmvars.MutableVariables{}
	vars.Put("key", `"cargo"`)
	vars.Put("dir", "4")
	vars.Put("footprint", `"##/#."`)
	helper := dmmprefab.New(1, "/obj/modular_map_root/ship_upgrade", vars.ToImmutable())
	BaseTurf = dmmprefab.New(2, "/turf/space", nil)
	BaseArea = dmmprefab.New(3, "/area/space", nil)
	before := &Dmm{}
	before.SetMapSize(4, 4, 2)
	for _, tile := range before.Tiles {
		tile.InstancesAdd(helper)
	}
	offsets := []util.Point{{}, {X: 1}, {X: 3}, {Y: 2}, {X: 1, Y: 2}, {X: 3, Y: 2}, {Y: 5}, {X: 1, Y: 5}, {X: 3, Y: 5}}
	for direction := ResizeNorthEast; direction <= ResizeSouthWest; direction++ {
		t.Run(direction.String(), func(t *testing.T) {
			m := before.Copy()
			m.Resize(7, 9, 3, direction)
			offset := offsets[direction]
			for _, tile := range before.Tiles {
				want := tile.Coord.Plus(offset)
				got := m.GetTile(want)
				if got.Coord != want || len(got.Instances()) != len(tile.Instances()) {
					t.Fatalf("tile at %v was not shifted to %v", tile.Coord, want)
				}
				for i, instance := range got.Instances() {
					old := tile.Instances()[i]
					if instance.Id() != old.Id() || instance.Prefab() != old.Prefab() || instance.Coord() != want || old.Coord() != tile.Coord {
						t.Fatal("resize lost identity/overrides or mutated the old state")
					}
				}
			}
			m.Resize(4, 4, 2, direction)
			for _, tile := range before.Tiles {
				got := m.GetTile(tile.Coord)
				if !got.Instances().PrefabsEquals(tile.Instances()) || got.Coord != tile.Coord {
					t.Fatal("inverse shrink did not restore the original tile contents")
				}
			}
		})
	}
}
