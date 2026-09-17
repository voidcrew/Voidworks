package wsship

import (
	"bytes"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

// Runs on the disposable authored ship in the native workflow. The real map
// editor, workspace refresh, command history and rendered resize form are used.
func exerciseHelperMovesAndResize(t *testing.T, ws *WsShip, capture func(string)) {
	t.Helper()
	ws.source, ws.isolated = 0, true
	ws.rebuild()
	ws.OnFocusChange(true)
	original := ship.RawData(ws.pane.Dmm()).EncodeTGM()
	find := func() *dmminstance.Instance {
		for _, tile := range ws.pane.Dmm().Tiles {
			for _, instance := range tile.Instances() {
				if instance.Prefab().Path() == ship.SlotMarker {
					return instance
				}
			}
		}
		return nil
	}
	marker := find()
	if marker == nil {
		t.Fatal("authored room has no slot marker")
	}
	start, dest := marker.Coord(), marker.Coord().Plus(util.Point{X: 1})
	before := ws.pane.Dmm().GetTile(start).Instances().Prefabs().Copy()
	editor := ws.pane.Editor()
	editor.TileDelete(start)
	editor.TileReplace(dest, before)
	editor.CommitContextNow("Move helper tile")
	count := 0
	for _, tile := range ws.pane.Dmm().Tiles {
		for _, instance := range tile.Instances() {
			if instance.Prefab().Path() == ship.SlotMarker {
				count++
				if instance.Coord() != dest || instance.Prefab().Id() != marker.Prefab().Id() {
					t.Fatal("move left a helper behind or changed its slot data")
				}
			}
		}
	}
	if count != 1 || ws.invalid {
		t.Fatal("helper move duplicated the slot or broke assembly", ws.message)
	}
	capture("helper-moved")
	ws.app.CommandStorage().Undo()
	if !bytes.Equal(original, ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
		t.Fatal("helper move undo did not restore the original hull")
	}
	ws.app.CommandStorage().Redo()
	if find().Coord() != dest {
		t.Fatal("helper move redo lost the destination")
	}
	ws.app.CommandStorage().Undo()
	editor.InstanceDelete(find())
	editor.CommitContextNow("Delete helper")
	if find() != nil {
		t.Fatal("deleted helper regenerated")
	}
	ws.app.CommandStorage().Undo()
	if find() == nil || !bytes.Equal(original, ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
		t.Fatal("undo did not restore the deleted helper")
	}
	ws.isolated = false
	ws.rebuild()
	ws.beginTask(taskResize)
	ws.width += 3
	ws.height += 5
	ws.resizeDirection = dmmap.ResizeSouthWest
	capture("resize-direction")
	ws.change("Resize hull", func() error {
		return ws.project.ResizeToward(ws.currentTheme(), int(ws.width), int(ws.height), ws.resizeDirection)
	})
	ws.finishTask()
	if ws.invalid || ws.assembly.Markers["cargo"] != start.Plus(util.Point{X: 3, Y: 5}) {
		t.Fatal("south/west resize did not move the room with its hull", ws.message)
	}
	capture("resized-south-west")
	for range 2 {
		ws.app.CommandStorage().Undo()
		if !bytes.Equal(original, ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
			t.Fatal("resize undo changed the original hull")
		}
		ws.app.CommandStorage().Redo()
		if ws.assembly.Markers["cargo"] != start.Plus(util.Point{X: 3, Y: 5}) {
			t.Fatal("resize redo shifted the room incorrectly")
		}
	}
	ws.app.CommandStorage().Undo()
	ws.rebuild()
}
