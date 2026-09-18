package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/render/bucket/level/chunk/unit"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/ship"
)

// Exercise rendered, alpha-tested picking on the real Scarab, including objects
// with pixel offsets. The fixture's files are never written.
func exerciseShipPicking(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	previousTool := tools.Selected().Name()
	defer func() {
		tools.SetSelected(previousTool)
		imgui.CurrentIO().SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		ws.source = 0
		ws.rebuild()
	}()
	ws.source = 0
	ws.rebuild()
	ws.OnFocusChange(true)
	tools.SetSelected(tools.TNPick)
	for range 3 {
		render()
	}
	before := map[string][]byte{}
	for file, doc := range ws.project.Documents {
		before[file] = ship.RawData(doc.Map).EncodeTGM()
	}
	pick := func(match func(ship.Source, *dmminstance.Instance) bool) (*dmminstance.Instance, int) {
		t.Helper()
		start := ws.source
		for index, source := range ws.assembly.Sources {
			for _, tile := range source.Live.Tiles {
				for _, live := range tile.Instances() {
					if !match(source, live) {
						continue
					}
					coord := live.Coord().Plus(source.Offset)
					u := unit.Make(coord.X, coord.Y, live, dmmap.WorldIconSize)
					bounds := u.ViewBounds()
					for _, point := range []imgui.Vec2{{X: 16, Y: 16}, {X: 8, Y: 8}, {X: 24, Y: 8}, {X: 8, Y: 24}, {X: 24, Y: 24}} {
						camera, control := ws.pane.Canvas().Render().Camera, ws.pane.CanvasControl()
						mouse := imgui.Vec2{X: control.PosMin().X + (bounds.X1+point.X+camera.ShiftX)*camera.Scale, Y: control.PosMax().Y - (bounds.Y1+point.Y+camera.ShiftY)*camera.Scale}
						imgui.CurrentIO().SetMousePosition(mouse)
						render()
						hovered := ws.pane.Editor().HoveredInstance()
						if hovered == nil || hovered.Id() != live.Id() {
							continue
						}
						if ws.source != start {
							t.Fatal("hover alone changed the editable source")
						}
						cameraBefore := *camera
						// This is the same selection call made by ToolPick.onStart.
						ws.pane.Editor().InstanceSelect(hovered)
						for range 3 {
							render()
						}
						selected, ok := ws.app.SelectedInstance()
						if !ok || selected != live || ws.source != index || ws.pane.Dmm() != source.Live {
							t.Fatal("Pick did not switch to the live item's source")
						}
						if *ws.pane.Canvas().Render().Camera != cameraBefore {
							t.Fatal("picking another module moved the camera")
						}
						t.Logf("Picked %s in %s from source %d", live.Prefab().Path(), source.Name, start)
						return live, index
					}
				}
			}
		}
		t.Fatal("no matching visible item could be picked")
		return nil, 0
	}
	medkit, medical := pick(func(source ship.Source, i *dmminstance.Instance) bool {
		return strings.Contains(strings.ToLower(source.Name), "med") && strings.HasPrefix(i.Prefab().Path(), "/obj/item/storage/medkit")
	})
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "scarab-medkit-pick.png"), 1400, 960)
	}
	file := ws.pane.Dmm().Path.Absolute
	ws.pane.Editor().InstanceDelete(medkit)
	ws.pane.Editor().CommitContextNow("Delete picked medkit")
	if ws.pane.Dmm().IsInstanceExist(medkit.Id()) {
		t.Fatal("selected module item was not deleted")
	}
	for name, doc := range ws.project.Documents {
		if name != file && !bytes.Equal(before[name], ship.RawData(doc.Map).EncodeTGM()) {
			t.Fatal("editing a picked item changed another source")
		}
	}
	ws.app.CommandStorage().Undo()
	if !bytes.Equal(before[file], ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
		t.Fatal("picked item undo used the wrong source")
	}
	ws.app.CommandStorage().Redo()
	if ws.pane.Dmm().IsInstanceExist(medkit.Id()) {
		t.Fatal("picked item redo failed")
	}
	ws.app.CommandStorage().Undo()
	pick(func(source ship.Source, i *dmminstance.Instance) bool {
		return source.Slot != "" && source.File != ws.assembly.Sources[medical].File && strings.HasPrefix(i.Prefab().Path(), "/obj/")
	})
	pick(func(source ship.Source, i *dmminstance.Instance) bool {
		return source.Slot == "" && strings.HasPrefix(i.Prefab().Path(), "/obj/")
	})
	for file, doc := range ws.project.Documents {
		if !bytes.Equal(before[file], ship.RawData(doc.Map).EncodeTGM()) {
			t.Fatal("picking changed map contents")
		}
	}
}
