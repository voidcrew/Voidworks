package editor

import (
	"bytes"
	"testing"

	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/canvas"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

type spriteMap struct {
	resizeTestMap
	state canvas.State
}

func (*spriteMap) InContext() bool              { return true }
func (*spriteMap) Refresh([]util.Point)         {}
func (*spriteMap) BeforeHistory()               {}
func (m *spriteMap) CanvasState() *canvas.State { return &m.state }

type spriteApp struct{ resizeTestApp }

func (*spriteApp) DoSelectPrefab(*dmmprefab.Prefab)     {}
func (*spriteApp) DoEditInstance(*dmminstance.Instance) {}
func (*spriteApp) SyncPrefabs()                         {}

func TestReplaceSpriteIsScopedUndoableAndSaved(t *testing.T) {
	dmmap.PrefabStorage.Free()
	defer dmmap.PrefabStorage.Free()
	for _, path := range []string{"/obj/chair", "/turf/floor", "/mob/test", "/area/test"} {
		vars := dmvars.Set(&dmvars.Variables{}, "icon", "'old.dmi'")
		vars = dmvars.Set(vars, "icon_state", `"idle"`)
		vars = dmvars.Set(vars, "pixel_x", "7")
		original := dmmprefab.New(0, path, vars)
		coord := util.Point{X: 1, Y: 1, Z: 1}
		chosen, other := dmminstance.New(coord, original), dmminstance.New(coord, original)
		tile := &dmmap.Tile{Coord: coord}
		tile.Set(dmmap.Instances{chosen, other})
		m := &dmmap.Dmm{Name: "sprite fixture", MaxX: 1, MaxY: 1, MaxZ: 1, Tiles: []*dmmap.Tile{tile}}
		pane := &spriteMap{resizeTestMap: resizeTestMap{snap: dmmsnap.New(m), level: 1}}
		a := &spriteApp{resizeTestApp: resizeTestApp{commands: command.NewStorage()}}
		a.commands.SetStack(pane.CommandStackId())
		e := &Editor{app: a, pMap: pane, dmm: m}
		before := ship.RawData(m).EncodeTGM()
		replacement := dmmprefab.New(0, path, dmvars.Set(dmvars.Set(vars, "icon", "'icons/new.dmi'"), "icon_state", `"new"`))
		if err := e.ReplaceSprite(chosen.Id(), original, replacement); err != nil {
			t.Fatal(err)
		}
		e.CommitContextNow("Replace sprite")
		if other.Prefab() != original || chosen.Prefab().Path() != path || chosen.Prefab().Vars().IntV("pixel_x", 0) != 7 {
			t.Fatal("replacement changed another instance or variable")
		}
		after := ship.RawData(m).EncodeTGM()
		if bytes.Equal(before, after) {
			t.Fatal("no map edit")
		}
		reopened, err := dmmdata.Read("sprite.dmm", bytes.NewReader(after))
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, prefabs := range reopened.Dictionary {
			for _, prefab := range prefabs {
				if prefab.Vars().TextV("icon", "") == "icons/new.dmi" && prefab.Vars().TextV("icon_state", "") == "new" {
					found++
				}
			}
		}
		if found != 1 {
			t.Fatalf("saved %d replacements", found)
		}
		a.commands.Undo()
		if !bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
			t.Fatal("undo failed")
		}
		a.commands.Redo()
		if !bytes.Equal(after, ship.RawData(m).EncodeTGM()) {
			t.Fatal("redo failed")
		}
		if err := e.ReplaceSprite(chosen.Id(), original, replacement); err == nil {
			t.Fatal("accepted stale picker")
		}
		if err := e.ReplaceSprite(0, original, replacement); err == nil {
			t.Fatal("accepted removed instance")
		}
		if !bytes.Equal(after, ship.RawData(m).EncodeTGM()) {
			t.Fatal("rejected replacement mutated map")
		}
	}
}
