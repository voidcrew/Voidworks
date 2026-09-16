package wsship

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dminclude"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

// A handwritten ship without upgrade slots, registered beside the authored
// fixture, is converted through the same selection, room and save path.
func exerciseFixedShipConversion(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	root := ws.catalog.Root
	hull, err := os.ReadFile(filepath.Join(root, "_maps/voidcrew/ships/ship_workshop_fixture.dmm"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "_maps/voidcrew/ships/ship_fixed_fixture.dmm"), hull, 0600); err != nil {
		t.Fatal(err)
	}
	registration := filepath.Join(root, "voidcrew/mapping/shuttles/fixed_fixture.dm")
	definition := "/datum/map_template/shuttle/voidcrew/fixed_fixture\n\tname = \"Fixed Fixture\"\n\tsuffix = \"fixed_fixture\"\n\tshort_name = \"Fixed\"\n\tpart_requirements = list(PART_CLASS_SCIENCE = 2)\n\tjob_slots = list(\n\t\tlist(name = \"Skipper\", officer = TRUE, outfit = /datum/outfit/job/captain, category = JOB_CAT_COMMAND, slots = 1),\n\t)\n"
	if err = os.WriteFile(registration, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	dmeFile := ws.app.LoadedEnvironment().RootFile
	includes, err := os.ReadFile(dmeFile)
	if err != nil {
		t.Fatal(err)
	}
	// Later exercises continue with the authored fixture, so put the workspace
	// back once the fixed ship has been converted.
	savedCatalog, savedProjects, savedDme := ws.catalog, ws.projects, ws.app.(*previewApp).dme
	savedHull, savedTheme := ws.hull, ws.theme
	defer func() {
		ws.flush()
		ws.OnFocusChange(false)
		// The fixture's projects tracked the environment file before this
		// registration was included; give them back the file they know.
		if err := os.WriteFile(dmeFile, includes, 0600); err != nil {
			t.Fatal(err)
		}
		ws.catalog, ws.projects = savedCatalog, savedProjects
		ws.app.(*previewApp).dme = savedDme
		ws.hull, ws.theme = savedHull, savedTheme
		ws.defaults()
		ws.rebuild()
		ws.setStage(stepBuild)
		ws.OnFocusChange(true)
		if ws.message != "" {
			t.Fatal(ws.message)
		}
	}()
	if err = os.WriteFile(dmeFile, dminclude.Add(includes, "voidcrew/mapping/shuttles/fixed_fixture.dm"), 0600); err != nil {
		t.Fatal(err)
	}
	environment, err := dmenv.New(dmeFile)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ship.Discover(environment)
	if err != nil {
		t.Fatal(err)
	}
	ws.app.(*previewApp).dme = environment
	ws.catalog = catalog
	ws.projects = map[string]*ship.Project{}
	ws.setStage(stepChoose)
	ws.shipKind = 2
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "fleet-fixed-layouts.png"), 1400, 960)
	}
	ws.shipKind = 0
	ws.hull = -1
	for i, h := range catalog.Hulls {
		if h.Type == ship.HullType+"/fixed_fixture" {
			ws.hull = i
			if !h.Fixed || len(h.Slots) != 0 || len(h.Modules) != 0 {
				t.Fatalf("fixed ship was not discovered as fixed: %+v", h)
			}
		}
	}
	if ws.hull < 0 {
		t.Fatal("fixed ship missing from the fleet library")
	}
	ws.theme = 0
	ws.isolated = false
	ws.defaults()
	ws.rebuild()
	ws.setStage(stepBuild)
	ws.OnFocusChange(true)
	if ws.message != "" || ws.assembly == nil || len(ws.assembly.Sources) != 1 {
		t.Fatalf("fixed ship did not open as a bare hull: %s", ws.message)
	}
	lo, hi := util.Point{X: 5, Y: 5, Z: 1}, util.Point{X: 7, Y: 7, Z: 1}
	if !tools.SetGrabSelection(lo, hi) {
		t.Fatal("could not select a room on the fixed ship")
	}
	ws.beginTask(taskRoom)
	ws.itemName = "Fixed Bay"
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "fixed-ship-confirm.png"), 1400, 960)
	}
	// The one-time modular confirmation hides the form until Continue.
	if ws.itemID != "" || ws.fixedConfirmed[ws.project.Hull.Type] {
		t.Fatal("fixed ship skipped the modular confirmation")
	}
	ws.fixedConfirmed = map[string]bool{ws.project.Hull.Type: true}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "fixed-ship-first-room.png"), 1400, 960)
	}
	ws.applyRegion()
	project := ws.project
	if ws.message != "" || ws.source == 0 || project.Hull.Fixed {
		t.Fatalf("fixed ship could not create its first room: %s", ws.message)
	}
	ws.app.CommandStorage().Undo()
	if !project.Hull.Fixed || len(project.Hull.Slots) != 0 {
		t.Fatal("undo did not restore the fixed layout")
	}
	ws.app.CommandStorage().Redo()
	if project.Hull.Fixed || !ship.Contains(project.Hull.Slots, "fixed_bay") {
		t.Fatal("redo did not restore the conversion")
	}
	for _, variant := range []string{"standard", "cargo"} {
		ws.change("Add variant", func() error { return project.AddTheme(0, variant, variant, false) })
		if ws.message != "" {
			t.Fatal(ws.message)
		}
	}
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	saved, _ := os.ReadFile(registration)
	text := string(saved)
	if !strings.Contains(text, "\thas_upgrade_slots = TRUE\n") || !strings.Contains(text, "\tupgrade_slot_ids = list(\"fixed_bay\")\n") || !strings.Contains(text, "PART_CLASS_SCIENCE = 2") || !strings.Contains(text, "name = \"Skipper\"") {
		t.Fatalf("conversion did not keep the definition and enable slots:\n%s", text)
	}
	// Ctrl+W disposes the workshop, but leaves the application's DME loaded.
	// Reopen through the real constructor, without manually reparsing that DME.
	app := ws.app
	ws.Dispose()
	*ws = *New(app)
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	if app.LoadedEnvironment() != environment {
		t.Fatal("reopening replaced the application's environment")
	}
	for i, h := range ws.catalog.Hulls {
		if h.Type != ship.HullType+"/fixed_fixture" {
			continue
		}
		if h.Fixed || !ship.Contains(h.Slots, "fixed_bay") || len(h.Modules) != 1 || h.Modules[0].Slot != "fixed_bay" || len(h.Themes) != 2 {
			t.Fatalf("converted ship was not rediscovered as modular: %+v", h)
		}
		ws.hull, ws.theme = i, 0
		ws.defaults()
		ws.rebuild()
		ws.setStage(stepBuild)
		if ws.message != "" || ws.assembly == nil || len(ws.assembly.Sources) != 2 || ws.IsModified() {
			t.Fatalf("saved room did not reopen cleanly: %s", ws.message)
		}
		render()
		// Source locations and registration values must also be current: a
		// refreshed label alone would still fail on the next edit and save.
		ws.beginRename(taskRenameModule, h.Modules[0].ID, h.Modules[0].Name)
		ws.itemName = "Reopened bay"
		ws.applyRename()
		if ws.message != "" || !ws.Save() {
			t.Fatalf("could not edit and save the reopened room: %s", ws.message)
		}
		return
	}
	t.Fatal("converted ship disappeared after rediscovery")
}
