package app

import (
	"bytes"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/dmi"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

// Run through the production app entry point and picker, then save the actual
// source map. The fixture owns every file it writes.
func exerciseSpritePickerCommands(t *testing.T, a *commandTestApp, frame func(), capture func(string), click func(imgui.Vec2)) {
	t.Helper()
	root := a.LoadedEnvironment().RootDir
	doc, _ := dmi.Create(32, 32)
	doc.Icon.States = []*dmi.State{dmi.NewState("initial", 32, 32), dmi.NewState("replacement", 32, 32)}
	for n, state := range doc.Icon.States {
		for y := 6; y < 26; y++ {
			for x := 6; x < 26; x++ {
				state.Cels[0].SetNRGBA(x, y, color.NRGBA{R: uint8(60 + n*150), G: 170, B: 100, A: 255})
			}
		}
	}
	if err := doc.Save(filepath.Join(root, "picker-test.dmi"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	dmicon.Cache.SetRootDirPath(root)
	e := a.CurrentEditor()
	m := e.Dmm()
	coord := util.Point{X: 2, Y: 2, Z: 1}
	initial := dmmap.PrefabStorage.Initial("/obj/item/crowbar")
	vars := dmvars.Set(dmvars.Set(initial.Vars(), "icon", "'picker-test.dmi'"), "icon_state", `"initial"`)
	prefab := dmmap.PrefabStorage.Get(initial.Path(), vars)
	m.GetTile(coord).InstancesAdd(prefab)
	e.CommitContextNow("Place sprite fixture")
	var id uint64
	for _, i := range m.GetTile(coord).Instances() {
		if i.Prefab() == prefab {
			id = i.Id()
		}
	}
	if id == 0 {
		t.Fatal("missing fixture instance")
	}
	before := ship.RawData(m).EncodeTGM()
	a.DoReplaceSprite(e.SpriteTarget(id), nil)
	for range 4 {
		frame()
	}
	capture("replace-sprite-open")
	click(imgui.Vec2{X: 650, Y: 407}) // second sprite row at 1400 x 960
	click(imgui.Vec2{X: 350, Y: 821}) // Apply sprite
	e.CommitContextNow("Replace sprite")
	frame()
	if e.SpriteTarget(id).Prefab().Vars().TextV("icon_state", "") != "replacement" {
		t.Fatal("production picker did not replace the map instance")
	}
	after := ship.RawData(m).EncodeTGM()
	a.DoUndo()
	frame()
	if !bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("application sprite undo failed")
	}
	a.DoRedo()
	frame()
	if !bytes.Equal(after, ship.RawData(m).EncodeTGM()) {
		t.Fatal("application sprite redo failed")
	}
	a.DoSave()
	frame()
	saved, err := dmmdata.New(m.Path.Absolute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tile := range m.Tiles {
		if !tile.Instances().Prefabs().Sorted().Equals(saved.Dictionary[saved.Grid[tile.Coord]].Sorted()) {
			t.Fatal("saved sprite replacement differs from source map")
		}
	}
	capture("replace-sprite-applied")
}
