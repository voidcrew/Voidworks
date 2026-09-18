package wsplanet

import (
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/mappreview"
	"sdmm/internal/util"
)

func TestPlanetSpriteTargetsUseRenderedOrigin(t *testing.T) {
	makePrefab := func(path, state string) *dmmprefab.Prefab {
		vars := dmvars.Set(dmvars.Set(&dmvars.Variables{}, "icon", "'planet.dmi'"), "icon_state", `"`+state+`"`)
		return dmmprefab.New(dmmprefab.IdStage, path, vars)
	}
	ground := makePrefab("/turf/ground", "ground")
	plant := makePrefab("/obj/tree", "canopy")
	area := makePrefab("/area/planet", "area")
	hidden := makePrefab("/obj/hidden", "marker")
	hidden = dmmprefab.New(dmmprefab.IdStage, hidden.Path(), dmvars.Set(hidden.Vars(), "invisibility", "100"))
	tile := &dmmap.Tile{Coord: util.Point{X: 1, Y: 1, Z: 1}}
	tile.InstancesSet(dmmdata.Prefabs{ground, plant, area, hidden})
	w := &Workspace{scene: &mappreview.Scene{Map: &dmmap.Dmm{MaxX: 1, MaxY: 1, MaxZ: 1, Tiles: []*dmmap.Tile{tile}}}, mouseWorld: imgui.Vec2{X: 55, Y: 55}}
	for _, instance := range tile.Instances() {
		if instance.Prefab() == plant {
			w.hovered = instance
		}
	}
	got := w.previewSprites()
	if len(got) != 2 || got[0] != plant || got[1] != ground {
		t.Fatal("menu lost staged appearances, included hidden helpers or used the overhanging pixel's tile", got)
	}
	w.hovered = nil
	if len(w.previewSprites()) != 0 {
		t.Fatal("out-of-bounds pointer offered a sprite")
	}
}
