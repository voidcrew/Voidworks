package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/SpaiR/imgui-go"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

// Click the actual hull resize entry point with either hull or room selected.
// Runs on both a loaded Delta and the disposable workshop-authored fixture.
func exerciseResizeControl(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	io := imgui.CurrentIO()
	originalSource := ws.source
	defer func() {
		io.SetMouseButtonDown(0, false)
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		ws.finishTask()
		ws.source = originalSource
		ws.rebuild()
	}()
	for source := 0; source < 2 && source < len(ws.assembly.Sources); source++ {
		ws.source = source
		ws.rebuild()
		ws.finishTask()
		var button imgui.Vec2
		frame := func() {
			imgui.NewFrame()
			imgui.SetNextWindowPos(imgui.Vec2{X: 20, Y: 20})
			imgui.SetNextWindowSize(imgui.Vec2{X: 1100, Y: 160})
			imgui.BeginV("Ship canvas controls regression", nil, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsNoResize|imgui.WindowFlagsNoMove)
			button = imgui.CursorScreenPos().Plus(imgui.CurrentStyle().FramePadding()).Plus(imgui.CalcTextSize("Change canvas size...", false, -1).Times(.5))
			ws.canvasHeader()
			imgui.End()
			imgui.Render()
		}
		for i := 0; i < 3; i++ {
			frame()
		}
		io.SetMousePosition(button)
		frame()
		io.SetMouseButtonDown(0, true)
		frame()
		io.SetMouseButtonDown(0, false)
		frame()
		if ws.task != taskResize {
			t.Fatalf("hull resize control unavailable for source %d (workshop metadata: %t)", source, ws.project.Settings != nil)
		}
		hull := ws.assembly.Sources[0].Data
		if ws.width != int32(hull.MaxX) || ws.height != int32(hull.MaxY) {
			t.Fatal("resize form uses selected room dimensions instead of hull dimensions")
		}
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		for i := 0; i < 3; i++ {
			render()
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" && source == 0 && ws.project.Settings == nil {
			captureFrame(t, filepath.Join(dst, "loaded-ship-resize-form.png"), 1400, 960)
		}
		ws.finishTask()
	}
}

// Runs on the disposable authored ship in the native workflow. The real map
// editor, workspace refresh, command history and rendered resize form are used.
func exerciseHelperMovesAndResize(t *testing.T, ws *WsShip, capture func(string)) {
	t.Helper()
	ws.source, ws.isolated = 0, true
	ws.rebuild()
	ws.OnFocusChange(true)
	original := ship.RawData(ws.pane.Dmm()).EncodeTGM()
	checkAreaBorders := func() {
		t.Helper()
		m := ws.pane.Dmm()
		for _, zone := range ws.pane.Editor().AreasZones() {
			for _, border := range zone.Borders {
				found := false
				if m.HasTile(border.Coord) {
					for _, instance := range m.GetTile(border.Coord).Instances() {
						found = found || instance.Prefab().Path() == zone.Name
					}
				}
				if !found {
					t.Fatalf("area outline for %s stayed at old coordinates %v", zone.Name, border.Coord)
				}
			}
		}
	}
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
	checkAreaBorders()
	for range 2 {
		ws.app.CommandStorage().Undo()
		checkAreaBorders()
		if !bytes.Equal(original, ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
			t.Fatal("resize undo changed the original hull")
		}
		ws.app.CommandStorage().Redo()
		checkAreaBorders()
		if ws.assembly.Markers["cargo"] != start.Plus(util.Point{X: 3, Y: 5}) {
			t.Fatal("resize redo shifted the room incorrectly")
		}
	}
	ws.app.CommandStorage().Undo()
	ws.rebuild()
}
