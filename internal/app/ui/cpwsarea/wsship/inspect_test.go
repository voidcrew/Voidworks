package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SpaiR/imgui-go"

	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/tilemenu"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

// Runs against the real Delta in the opt-in native test. No game files are saved.
func exerciseShipContextContents(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	original := copySelection(ws.selected)
	defer func() { ws.selected = original; ws.source = 0; ws.rebuild() }()
	for _, module := range ws.project.Hull.Modules {
		if strings.Contains(module.ID, "cabins") && module.Available(ws.currentTheme().ID) {
			ws.selected[module.Slot] = module.ID
			break
		}
	}
	ws.rebuild()
	if ws.invalid {
		t.Fatal(ws.message)
	}
	find := func(path string) (util.Point, tilemenu.Entry) {
		for coord, atoms := range ws.assembly.Cells {
			for _, atom := range atoms {
				if atom.Source == 0 || atom.Prefab.Path() != path {
					continue
				}
				offset := ws.assembly.Sources[ws.source].Offset
				contents := ws.pane.TileMenuContents(util.Point{X: coord.X - offset.X, Y: coord.Y - offset.Y, Z: coord.Z})
				if len(contents.Entries) != len(atoms) {
					t.Fatalf("menu omits assembled objects at %v", coord)
				}
				for _, entry := range contents.Entries {
					if entry.Instance.Id() == atom.Instance.Id() {
						return coord, entry
					}
				}
				t.Fatalf("missing %s at %v", path, coord)
			}
		}
		t.Fatalf("Delta fixture lacks %s", path)
		return util.Point{}, tilemenu.Entry{}
	}
	for _, path := range []string{"/obj/structure/closet/wardrobe/mixed", "/obj/item/toy/plush/moth"} {
		ws.source = 0
		ws.rebuild()
		coord, entry := find(path)
		if entry.Editable || entry.EditSource == nil {
			t.Fatal("room object incorrectly belongs to hull")
		}
		t.Logf("Assembled menu includes %s at %v from %s", path, coord, entry.Source)
		// Open the real popup with a right-click on the rendered object's tile.
		ws.OnFocusChange(true)
		for i := 0; i < 3; i++ {
			render()
		}
		camera, control := ws.pane.Canvas().Render().Camera, ws.pane.CanvasControl()
		mouse := imgui.Vec2{
			X: control.PosMin().X + (float32(coord.X)*float32(dmmap.WorldIconSize)-float32(dmmap.WorldIconSize)/2+camera.ShiftX)*camera.Scale,
			Y: control.PosMax().Y - (float32(coord.Y)*float32(dmmap.WorldIconSize)-float32(dmmap.WorldIconSize)/2+camera.ShiftY)*camera.Scale,
		}
		io := imgui.CurrentIO()
		io.SetMousePosition(mouse)
		render()
		io.SetMouseButtonDown(1, true)
		render()
		io.SetMouseButtonDown(1, false)
		for i := 0; i < 3; i++ {
			render()
		}
		if !imgui.IsPopupOpenV("", imgui.PopupFlagsAnyPopup) {
			t.Fatal("right-click did not open the tile popup")
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
			captureFrame(t, filepath.Join(dst, "context-"+filepath.Base(path)+".png"), 1400, 960)
		}
		io.SetMousePosition(imgui.Vec2{X: 10, Y: 10})
		render()
		io.SetMouseButtonDown(0, true)
		render()
		io.SetMouseButtonDown(0, false)
		render()
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		entry.EditSource()
		selected, ok := ws.app.SelectedInstance()
		if !ok || selected.Id() != entry.Instance.Id() || selected == entry.Instance || !ws.pane.Dmm().IsInstanceExist(selected.Id()) || ws.source == 0 {
			t.Fatal("Edit in room did not select the live room instance")
		}
		_, active := find(path)
		// Other copies of this type may come first; check the selected ID directly.
		for _, current := range ws.pane.TileMenuContents(selected.Coord()).Entries {
			if current.Instance.Id() == selected.Id() {
				active = current
				break
			}
		}
		if !active.Editable || active.Instance != selected {
			t.Fatal("active room menu would edit a preview copy")
		}
		before := map[string][]byte{}
		for file, doc := range ws.project.Documents {
			before[file] = ship.RawData(doc.Map).EncodeTGM()
		}
		file := ws.pane.Dmm().Path.Absolute
		ws.pane.Editor().InstanceDelete(selected)
		ws.pane.Editor().CommitContextNow("Delete context test object")
		if ws.project.Documents[file].Map.IsInstanceExist(selected.Id()) {
			t.Fatal("room delete failed")
		}
		for name, doc := range ws.project.Documents {
			if name != file && !bytes.Equal(before[name], ship.RawData(doc.Map).EncodeTGM()) {
				t.Fatal("room edit changed another source")
			}
		}
		ws.app.CommandStorage().Undo()
		if !bytes.Equal(before[file], ship.RawData(ws.project.Documents[file].Map).EncodeTGM()) {
			t.Fatal("room undo failed")
		}
		ws.app.CommandStorage().Redo()
		if ws.project.Documents[file].Map.IsInstanceExist(selected.Id()) {
			t.Fatal("room redo failed")
		}
		ws.app.CommandStorage().Undo()
		render()
	}
	// Inspect the entire ship from each room, including negative local coordinates.
	for index := range ws.assembly.Sources {
		ws.source = index
		ws.rebuild()
		offset := ws.assembly.Sources[index].Offset
		owned := map[uint64]bool{}
		for _, tile := range ws.pane.Dmm().Tiles {
			for _, instance := range tile.Instances() {
				owned[instance.Id()] = true
			}
		}
		for coord, atoms := range ws.assembly.Cells {
			contents := ws.pane.TileMenuContents(util.Point{X: coord.X - offset.X, Y: coord.Y - offset.Y, Z: coord.Z})
			if len(contents.Entries) != len(atoms) {
				t.Fatalf("source %d omitted context at %v", index, coord)
			}
			for _, entry := range contents.Entries {
				if entry.Editable != owned[entry.Instance.Id()] {
					t.Fatal("wrong edit ownership")
				}
				if !entry.Editable && entry.EditSource == nil {
					t.Fatal("missing owner navigation")
				}
			}
		}
	}
}
