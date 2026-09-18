package wsship

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/ship"
	"sdmm/internal/util"
	"strings"
	"testing"

	"github.com/SpaiR/imgui-go"
)

func TestRoomHeadingTogglesOptionsWithoutChangingSelection(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 400, Y: 600})
	io.SetDeltaTime(1.0 / 60)
	io.Fonts().TextureDataRGBA32()
	ws := &WsShip{
		project: &ship.Project{Hull: ship.Hull{Type: "/datum/map_template/shuttle/test", Slots: []string{"engineering"},
			Modules: []ship.Module{{ID: "engine", Slot: "engineering", Name: "Engine"}}}},
		assembly: &ship.Assembly{Sources: []ship.Source{{Name: "Hull"}, {Slot: "engineering"}}},
		source:   1, selected: map[string]string{"engineering": "engine"},
		optionInfos: map[string]optionInfo{"engine": {price: "Free"}},
	}
	before, err := json.Marshal(ws.project.Hull)
	if err != nil {
		t.Fatal(err)
	}
	var row imgui.Vec2
	frame := func() float32 {
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: 400, Y: 600})
		imgui.BeginV("Room headings", nil, imgui.WindowFlagsNoSavedSettings)
		pos := imgui.CursorScreenPos()
		// The first upgrade heading follows the section label and Hull row.
		row = pos.Plus(imgui.Vec2{X: 100, Y: 110})
		ws.roomsControls()
		height := imgui.CursorScreenPos().Y - pos.Y
		imgui.End()
		imgui.Render()
		return height
	}
	click := func() float32 {
		io.SetMousePosition(row)
		frame()
		io.SetMouseButtonDown(0, true)
		frame()
		io.SetMouseButtonDown(0, false)
		frame()
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		return frame()
	}
	frame()
	expanded := frame()
	collapsed := click()
	if collapsed >= expanded || ws.roomExpanded("engineering") {
		t.Fatal("clicking the active heading did not hide its options")
	}
	ws.source = 0
	frame()
	ws.source = 1
	if frame() != collapsed {
		t.Fatal("changing the edit target reset the disclosure")
	}
	if click() != expanded {
		t.Fatal("clicking the heading again did not reopen its options")
	}
	after, err := json.Marshal(ws.project.Hull)
	if err != nil || string(before) != string(after) || ws.source != 1 || ws.selected["engineering"] != "engine" {
		t.Fatal("toggling options changed the ship or edit target")
	}
}

// The canvas outline is derived from a room's hull tiles: an L-shaped room
// draws its inner corner, and the marker tile is not part of the outline.
func TestRoomTilesOutline(t *testing.T) {
	shape, origin, err := ship.FootprintFromTiles([]util.Point{
		{X: 5, Y: 5, Z: 1}, {X: 6, Y: 5, Z: 1}, {X: 7, Y: 5, Z: 1}, {X: 5, Y: 6, Z: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	room := ship.RoomShape{Slot: "cargo", Origin: origin, Marker: util.Point{X: 4, Y: 5, Z: 1}, Footprint: shape}
	tiles := map[util.Point]bool{}
	for _, tile := range roomTiles(room) {
		tiles[tile] = true
	}
	if len(tiles) != 4 || tiles[room.Marker] || !tiles[util.Point{X: 7, Y: 5, Z: 1}] || !tiles[util.Point{X: 5, Y: 6, Z: 1}] {
		t.Fatalf("room tiles: %v", tiles)
	}
	sides := util.OuterSides(tiles)
	if sides[util.Point{X: 5, Y: 5, Z: 1}] != (util.Sides{South: true, West: true}) {
		t.Fatalf("inner corner: %+v", sides[util.Point{X: 5, Y: 5, Z: 1}])
	}
	if sides[util.Point{X: 6, Y: 5, Z: 1}] != (util.Sides{North: true, South: true}) {
		t.Fatalf("arm tile: %+v", sides[util.Point{X: 6, Y: 5, Z: 1}])
	}
	if sides[util.Point{X: 5, Y: 6, Z: 1}] != (util.Sides{North: true, East: true, West: true}) {
		t.Fatalf("tip tile: %+v", sides[util.Point{X: 5, Y: 6, Z: 1}])
	}
}

func TestEditingLabelUsesSlotDisplayName(t *testing.T) {
	ws := &WsShip{assembly: &ship.Assembly{Sources: []ship.Source{{Name: "Hull"}, {Name: "Medical bay", Slot: "cargo_bay"}}}}
	if got := ws.editingLabel(); got != "Hull" {
		t.Fatalf("hull label: %q", got)
	}
	ws.source = 1
	if got := ws.editingLabel(); got != "Cargo bay - Medical bay option" {
		t.Fatalf("room label: %q", got)
	}
	if slot := ws.editingSlot(); slot != "cargo_bay" {
		t.Fatalf("editing slot: %q", slot)
	}
}

// Native: the Rooms panel replaces the part combo. Rows switch the edit
// target, options are managed in place, rooms reshape and delete with undo.
func exerciseRoomsPanel(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	capture := func(name string) {
		for i := 0; i < 3; i++ {
			render()
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
			captureFrame(t, filepath.Join(dst, name+".png"), 1400, 960)
		}
	}
	ws.setStage(stepBuild)
	ws.finishTask()
	ws.flush()
	ws.source = 0
	ws.rebuild()
	ws.OnFocusChange(true)
	project := ws.project
	if ws.Map() == nil || ws.message != "" {
		t.Fatalf("rooms panel fixture: %s", ws.message)
	}
	capture("rooms-panel-hull")
	ws.editRoom("cargo")
	if ws.editingSlot() != "cargo" || ws.selected["cargo"] == "" {
		t.Fatal("room row did not activate its displayed option")
	}
	if got := ws.editingLabel(); !strings.HasPrefix(got, "Cargo - ") || !strings.HasSuffix(got, " option") {
		t.Fatalf("editing label: %q", got)
	}
	capture("rooms-panel-expanded")
	ws.clickRoomHeading("cargo")
	if ws.roomExpanded("cargo") || ws.editingSlot() != "cargo" {
		t.Fatal("collapsing the edited room changed the editing target")
	}
	ws.rebuild()
	ws.beginTask(taskSettings)
	render()
	ws.finishTask()
	capture("rooms-panel-collapsed")
	if ws.roomExpanded("cargo") {
		t.Fatal("rebuild or returning from ship details reopened a collapsed room")
	}
	ws.clickRoomHeading("cargo")
	if !ws.roomExpanded("cargo") {
		t.Fatal("clicking the collapsed heading did not expand it")
	}
	if room, ok := ws.assembly.Rooms["cargo"]; !ok || !room.Footprint.IsFull() {
		t.Fatal("cargo room has no footprint on the assembly")
	}
	// Hidden option: hull only, editing returns to the hull, the row keeps its options.
	ws.selectRoomOption("cargo", "")
	if ws.source != 0 || ws.displayedOption("cargo") == "" {
		t.Fatal("show none did not fall back to the hull")
	}
	ws.editRoom("cargo")
	if ws.editingSlot() != "cargo" {
		t.Fatal("editing a hidden room did not display its default option")
	}
	// Add an option from the panel, then manage it: default, delete, undo.
	ws.beginAddOption("cargo")
	if ws.task != taskModule || ws.editingSlot() != "cargo" {
		t.Fatal("add option did not open the option form for the room")
	}
	ws.itemID, ws.itemName, ws.emptyModule = "panel_option", "Panel Option", true
	ws.change("Create room option", func() error {
		for _, m := range project.Hull.Modules {
			if m.ID == ws.selected["cargo"] {
				return project.AddModule(ws.theme, m, ws.itemID, ws.itemName, true)
			}
		}
		return nil
	})
	ws.finishTask()
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	ws.selectRoomOption("cargo", "panel_option")
	if ws.editingSlot() != "cargo" || ws.assembly.Sources[ws.source].Name != "Panel Option" {
		t.Fatal("new option is not the edit target")
	}
	if ws.option("panel_option").price != "Free" {
		t.Fatalf("new option is not free: %q", ws.option("panel_option").price)
	}
	ws.change("Set default option", func() error { return project.SetDefaultModule("cargo", "panel_option") })
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	ws.deleteOption("cargo", "panel_option")
	if ws.message == "" {
		t.Fatal("deleting the default option was allowed")
	}
	ws.message = ""
	ws.change("Set default option", func() error { return project.SetDefaultModule("cargo", "cargo_basic") })
	ws.deleteOption("cargo", "panel_option")
	if ws.message != "" || ws.invalid {
		t.Fatalf("delete option: %s", ws.message)
	}
	for _, m := range project.Hull.Modules {
		if m.ID == "panel_option" {
			t.Fatal("option was not removed")
		}
	}
	if ws.selected["cargo"] != "cargo_basic" {
		t.Fatalf("deleted option still displayed: %q", ws.selected["cargo"])
	}
	ws.app.CommandStorage().Undo()
	if ws.invalid || ws.message != "" || len(ws.slotOptions("cargo")) != 3 {
		t.Fatalf("undo of option removal: invalid=%v %s", ws.invalid, ws.message)
	}
	ws.app.CommandStorage().Redo()
	if ws.invalid || ws.message != "" || len(ws.slotOptions("cargo")) != 2 {
		t.Fatalf("redo of option removal left the workshop invalid: %s", ws.message)
	}
	ws.app.CommandStorage().Undo() // removal
	ws.app.CommandStorage().Undo() // default back to cargo_basic
	ws.app.CommandStorage().Undo() // default to panel_option
	ws.app.CommandStorage().Undo() // creation
	if len(ws.slotOptions("cargo")) != 2 || ws.invalid {
		t.Fatal("undo did not remove the panel option")
	}
	// Reshape: drop the top row. The sandbox game only knows rectangles, so an
	// L-shape must be refused with the game-support message.
	ws.reshapeSlot = "cargo"
	ws.beginTask(taskReshape)
	room := ws.assembly.Rooms["cargo"]
	tiles := tools.RoomShapeTiles()
	if ws.source != 0 || !tools.IsSelected(tools.TNRoomShape) || len(tiles) != room.Footprint.Count() {
		t.Fatalf("reshape did not start from the room's %d tiles: %d", room.Footprint.Count(), len(tiles))
	}
	capture("rooms-reshape")
	top := room.Origin.Y + room.Footprint.H - 1
	var shorter []util.Point
	for _, tile := range tiles {
		if tile.Y != top {
			shorter = append(shorter, tile)
		}
	}
	if !project.SupportsFootprints() {
		corner := shorter[:0:0]
		for _, tile := range shorter {
			if tile.X != room.Origin.X || tile.Y != room.Origin.Y {
				corner = append(corner, tile)
			}
		}
		tools.SetRoomShapeTiles(corner)
		ws.applyRegion()
		if !strings.Contains(ws.message, "custom module shapes") {
			t.Fatalf("L-shape on rectangle-only game: %q", ws.message)
		}
		ws.message = ""
	}
	tools.SetRoomShapeTiles(shorter)
	ws.applyRegion()
	if ws.message != "" {
		t.Fatalf("reshape: %s", ws.message)
	}
	if got := ws.assembly.Rooms["cargo"].Footprint; got.H != room.Footprint.H-1 || got.W != room.Footprint.W {
		t.Fatalf("reshape did not shrink the room: %dx%d", got.W, got.H)
	}
	if ws.task != taskPaint || ws.editingSlot() != "cargo" || tools.IsSelected(tools.TNRoomShape) {
		t.Fatal("reshape did not return to editing the room")
	}
	ws.app.CommandStorage().Undo()
	if got := ws.assembly.Rooms["cargo"].Footprint; got.H != room.Footprint.H {
		t.Fatal("undo did not restore the room shape")
	}
	// Delete the room and bring it back.
	ws.deleteRoom("cargo")
	if ws.message != "" || ws.invalid || ship.Contains(project.Hull.SlotsFor(ws.currentTheme()), "cargo") || len(ws.assembly.Sources) != 1 {
		t.Fatalf("delete room: %s", ws.message)
	}
	capture("rooms-panel-deleted")
	ws.app.CommandStorage().Undo()
	if ws.invalid || !ship.Contains(project.Hull.SlotsFor(ws.currentTheme()), "cargo") {
		t.Fatal("undo did not restore the room")
	}
	ws.editRoom("cargo")
	if ws.editingSlot() != "cargo" {
		t.Fatal("restored room cannot be edited")
	}
	// Leaving the workshop drops the workshop-only tool.
	ws.beginTask(taskRoom)
	ws.OnFocusChange(false)
	if tools.IsSelected(tools.TNRoomShape) {
		t.Fatal("room-shape tool stayed selected after losing focus")
	}
	ws.OnFocusChange(true)
	ws.finishTask()
}
