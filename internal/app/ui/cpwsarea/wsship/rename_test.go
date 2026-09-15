package wsship

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/ship"
)

func exerciseShipDetails(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	ws.setStage(stepBuild)
	oldName := ws.project.Hull.Name
	ws.beginTask(taskSettings)
	ws.settings.name = "Renamed Workshop Ship"
	ws.settings.costs = ship.PartCosts{"combat": 2, "science": 3, "trade": 4, "misc": 5}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "ship-details-price.png"), 1400, 960)
	}
	if !ws.IsModified() || !ws.Save() {
		t.Fatalf("details draft did not save: %s", ws.message)
	}
	fresh, err := dmenv.New(ws.project.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ship.OpenProject(ws.catalog, fresh, ws.project.Hull)
	if err != nil {
		t.Fatal(err)
	}
	costs, err := p.PartCosts("ship")
	if err != nil || !costs.Equal(ws.settings.costs) || p.Hull.Name != "Renamed Workshop Ship" {
		t.Fatal("saved ship details were lost on reload")
	}
	if !strings.Contains(fresh.Objects[p.Hull.Type].Vars.ValueV("part_requirements", ""), "science") {
		t.Fatal("base ship price did not reach the game registration")
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() || ws.project.Hull.Name != oldName {
		t.Fatalf("could not undo ship details: %s", ws.message)
	}
	ws.app.CommandStorage().Redo()
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	ws.beginTask(taskSettings)
	ws.settings.costs["combat"] = -1
	if ws.Save() || ws.task != taskSettings {
		t.Fatal("Save accepted an invalid base ship price")
	}
	ws.settings.costs, err = ws.project.PartCosts("ship")
	if err != nil {
		t.Fatal(err)
	}
	ws.finishTask()
}

// Exercise form validation, editing labels, persistence and the actual undo stack
// in the native rendering fixture, including a freshly parsed handwritten ship.
func exerciseRenaming(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	ws.setStage(stepBuild)
	ws.selectRoomOption("cargo", "cargo_basic")
	theme, module := ws.currentTheme(), ws.project.Hull.Modules[0]
	for _, target := range []struct {
		task                    buildTask
		id, old, name, typePath string
	}{
		{taskRenameTheme, theme.ID, theme.Name, "Renamed Variant", "/datum/ship_theme/workshop_fixture_" + theme.ID},
		{taskRenameModule, module.ID, module.Name, "Renamed Cargo Bay", "/datum/ship_upgrade_module/workshop_fixture_" + module.ID},
	} {
		ws.beginRename(target.task, target.id, target.old)
		if ws.itemName != target.old || ws.project.RenameNameError(ws.renameScope(), ws.itemName) != nil {
			t.Fatal("rename form did not accept its existing name")
		}
		ws.itemName = "   "
		render()
		ws.applyRename()
		if ws.message == "" || ws.task != target.task {
			t.Fatal("rename form accepted an empty name")
		}
		ws.itemName = target.name
		ws.itemDescription = "A medical \"bay\" with supplies.\nReady for longer trips."
		ws.message = ""
		for i := 0; i < 3; i++ {
			render()
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
			captureFrame(t, filepath.Join(dst, "rename-"+target.id+".png"), 1400, 960)
		}
		ws.applyRename()
		if ws.message != "" || ws.task != taskPaint {
			t.Fatalf("rename did not return to ship editing: %s", ws.message)
		}
		if target.task == taskRenameModule && ws.assembly.Sources[1].Name != target.name {
			t.Fatal("room editing label was not refreshed")
		}
		if !ws.Save() {
			t.Fatal(ws.message)
		}
		fresh, err := dmenv.New(ws.project.Dme.RootFile)
		if err != nil {
			t.Fatal(err)
		}
		if obj := fresh.Objects[target.typePath]; obj == nil || obj.Vars.ValueV("name", "") != strconv.Quote(target.name) {
			t.Fatal("renamed component was not present in the reloaded environment")
		}
		if got := fresh.Objects[target.typePath].Vars.ValueV("desc", ""); got != strconv.Quote(ws.itemDescription) {
			t.Fatalf("description did not survive Save and environment reload: %s", got)
		}
		ws.app.CommandStorage().Undo()
		if !ws.Save() {
			t.Fatal(ws.message)
		}
		ws.app.CommandStorage().Redo()
		if !ws.Save() {
			t.Fatal(ws.message)
		}
		// Restore original names for the remaining fixture exercises.
		ws.app.CommandStorage().Undo()
		if !ws.Save() {
			t.Fatal(ws.message)
		}
	}
	// Save also has to include text still in the form, without an Apply click.
	theme = ws.currentTheme()
	ws.beginRename(taskRenameTheme, theme.ID, theme.Name)
	ws.itemName = "Variant saved directly from form"
	ws.itemDescription = "Description saved directly from form"
	if !ws.IsModified() || !strings.HasPrefix(ws.Name(), "* ") {
		t.Fatal("variant name draft has no unsaved indicator")
	}
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	fresh, err := dmenv.New(ws.project.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	path := "/datum/ship_theme/workshop_fixture_" + theme.ID
	if obj := fresh.Objects[path]; obj == nil || obj.Vars.ValueV("name", "") != strconv.Quote("Variant saved directly from form") {
		t.Fatal("Save discarded the variant name still being edited in its form")
	}
	if fresh.Objects[path].Vars.ValueV("desc", "") != strconv.Quote(ws.itemDescription) {
		t.Fatal("Save discarded the description draft")
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	ws.beginRename(taskRenameTheme, theme.ID, theme.Name)
	ws.itemName = ""
	if ws.Save() || ws.task != taskRenameTheme {
		t.Fatal("Save accepted or discarded an invalid variant name")
	}
	ws.itemName = "Cancelled variant name"
	ws.itemDescription = "Cancelled description"
	ws.cancelRename()
	if ws.IsModified() || ws.currentTheme().Name != theme.Name || ws.project.Description("theme/"+theme.ID) == "Cancelled description" {
		t.Fatal("Cancel applied the variant name draft")
	}
	ws.beginRename(taskRenameTheme, theme.ID, theme.Name)
	ws.itemDescription = "Only the description changes"
	if !ws.IsModified() || !ws.Save() || ws.currentTheme().Name != theme.Name {
		t.Fatalf("description-only draft did not save: %s", ws.message)
	}
	ws.beginRename(taskRenameTheme, theme.ID, theme.Name)
	ws.itemDescription = ""
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	fresh, err = dmenv.New(ws.project.Dme.RootFile)
	if err != nil || fresh.Objects[path].Vars.ValueV("desc", "missing") != `""` {
		t.Fatalf("clearing the description did not persist: %v", err)
	}
	ws.app.CommandStorage().Undo()
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal(ws.message)
	}
}

func exerciseRoomRenaming(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	ws.setStage(stepBuild)
	ws.selectRoomOption("cargo", "cargo_basic")
	originalSelection := ws.selected["cargo"]
	ws.beginRename(taskRenameRoom, "cargo", "Cargo")
	ws.itemName = ""
	if ws.Save() || ws.task != taskRenameRoom {
		t.Fatal("Save discarded an invalid room name")
	}
	ws.itemName = "Forward Cargo"
	ws.message = ""
	if !ws.IsModified() {
		t.Fatal("room name draft has no unsaved indicator")
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "rename-room.png"), 1400, 960)
	}
	data, _, err := ws.captureRecovery()
	var recovered recoveryWorkspace
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &recovered); err != nil || recovered.Form.Task != taskRenameRoom || recovered.Form.ItemName != "Forward Cargo" {
		t.Fatalf("recovery lost the room name form: %v", err)
	}
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	if ws.selected["Forward_Cargo"] != originalSelection || ws.task != taskPaint {
		t.Fatal("renaming lost the selected option")
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "renamed-room.png"), 1400, 960)
	}
	fresh, err := dmenv.New(ws.project.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	path := "/datum/ship_upgrade_module/workshop_fixture_cargo_basic"
	if fresh.Objects[path].Vars.ValueV("slot", "") != `"Forward_Cargo"` {
		t.Fatal("room rename did not reach the game registration")
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() || ws.selected["cargo"] != originalSelection {
		t.Fatalf("room rename undo failed: %s", ws.message)
	}
	ws.app.CommandStorage().Redo()
	if !ws.Save() || ws.selected["Forward_Cargo"] != originalSelection {
		t.Fatalf("room rename redo failed: %s", ws.message)
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	ws.beginRename(taskRenameRoom, "cargo", "Cargo")
	ws.itemName = "Cancelled room"
	ws.cancelRename()
	if ws.IsModified() {
		t.Fatal("Cancel kept the room-name draft")
	}
}

func exerciseFleetRoomRename(t *testing.T, ws *WsShip) {
	t.Helper()
	slots := ws.project.Hull.SlotsFor(ws.currentTheme())
	if len(slots) == 0 {
		t.Fatal("fleet fixture has no rooms")
	}
	slot := slots[0]
	if err := ws.project.PrepareSlotRename(slot, "Fleet Rename Check"); err != nil {
		t.Fatal(err)
	}
	before := ws.project.Capture()
	key, err := ws.project.RenameSlot(slot, "Fleet Rename Check")
	if err != nil {
		t.Fatal(err)
	}
	changes, err := ws.project.Changes()
	if err != nil || len(changes) == 0 {
		t.Fatalf("fleet rename has no valid save: %v", err)
	}
	for _, theme := range ws.project.Hull.Themes {
		if !ship.Contains(ws.project.Hull.SlotsFor(theme), key) {
			continue
		}
		selected := map[string]string{}
		for _, m := range ws.project.Hull.Modules {
			if m.Slot == key && m.Available(theme.ID) {
				selected[key] = m.ID
				break
			}
		}
		a, err := ws.project.Assemble(theme, selected)
		if err != nil || len(a.Sources) != 2 {
			t.Fatalf("renamed fleet variant failed to assemble: %v", err)
		}
	}
	ws.project.Restore(before)
	ws.rebuild()
	if ws.project.Modified() {
		t.Fatal("fleet rename inspection left unsaved changes")
	}
}
