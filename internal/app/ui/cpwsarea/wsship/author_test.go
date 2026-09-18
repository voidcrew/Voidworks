package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/ship"
	"sdmm/internal/shippreview"
	"sdmm/internal/util"
	"strings"
	"testing"
)

// Called inside the native GL test so the same pane, snapshots and command
// storage used by the application execute the whole authoring workflow.
func exerciseAuthoring(t *testing.T, ws *WsShip, dme *dmenv.Dme, render func()) {
	t.Helper()
	root := os.Getenv("SHIP_AUTHOR_FIXTURE_OUTPUT")
	if root == "" {
		root = t.TempDir()
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	draftEnv := *dme
	draftEnv.RootDir = root
	draftEnv.RootFile = filepath.Join(root, "workshop.dme")
	if err := os.WriteFile(draftEnv.RootFile, []byte("#include \""+filepath.ToSlash(dme.RootFile)+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	catalog := &ship.Catalog{Root: root, ModuleDir: "_maps/voidcrew/ship_modules/"}
	ws.catalog = catalog
	ws.projects = map[string]*ship.Project{}
	ws.app.(*previewApp).dme = &draftEnv
	ws.BeginNewShip()
	ws.newName = "Workshop Fixture"
	if ws.Map() != nil {
		t.Fatal("new ship setup exposes the old map")
	}
	capture := func(name string) {
		for i := 0; i < 3; i++ {
			render()
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
			captureFrame(t, filepath.Join(dst, name+".png"), 1400, 960)
		}
	}
	capture("new-ship-name")
	if ws.newID != "workshop_fixture" {
		t.Fatal("ship name did not generate a file identifier")
	}
	ws.wizardStep = 1
	ws.sizePreset, ws.width, ws.height = 0, 24, 24
	capture("new-ship-canvas")
	ws.createShip()
	project := ws.project
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	if ws.Map() == nil || ws.wizard || ws.stage != stepBuild {
		t.Fatal("creating a ship did not enter the editor")
	}
	if project.Hull.Name != "Workshop Fixture" || project.Settings.ID != "workshop_fixture" {
		t.Fatal("creation lost ship identity")
	}
	foundFloor := false
	for _, i := range ws.pane.Dmm().GetTile(util.Point{X: 3, Y: 3, Z: 1}).Instances() {
		if i.Prefab().Path() == "/turf/open/floor/plating" {
			foundFloor = true
		}
	}
	if foundFloor {
		t.Fatal("new ship painted floors automatically")
	}
	capture("new-ship-canvas-empty")
	ws.beginTask(taskRoom)
	ws.itemName = "Cargo bay"
	capture("guided-room")
	ws.finishTask()
	ws.beginTask(taskSettings)
	capture("ship-details")
	ws.finishTask()
	lo, hi := util.Point{X: 4, Y: 5, Z: 1}, util.Point{X: 12, Y: 13, Z: 1}
	ws.change("Lay permanent deck", func() error { return project.Deck(ws.currentTheme(), lo, hi) })
	if !tools.SetGrabSelection(lo, hi) {
		t.Fatal("could not select hull")
	}
	beforeArea := ship.RawData(ws.pane.Dmm()).EncodeTGM()
	ws.beginTask(taskArea)
	if gotLo, gotHi, ready := tools.SelectionBounds(); !ready || gotLo != lo || gotHi != hi {
		t.Fatal("opening areas lost Grab selection")
	}
	ws.itemName, ws.areaIcon = "Bridge", "bridge"
	capture("ship-areas-create")
	ws.createArea()
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	bridgeArea := ws.areaPath
	for _, point := range []util.Point{lo, hi} {
		found := false
		for _, inst := range ws.pane.Dmm().GetTile(point).Instances() {
			if inst.Prefab().Path() == bridgeArea {
				found = true
			}
		}
		if !found {
			t.Fatal("create area did not assign selected tiles")
		}
	}
	ws.app.DoSelectPrefab(dmmap.PrefabStorage.Initial(bridgeArea))
	ws.app.CommandStorage().Undo()
	if !bytes.Equal(beforeArea, ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
		t.Fatal("one undo did not restore selected tiles")
	}
	if project.AreaActive(bridgeArea) {
		t.Fatal("undo kept created area")
	}
	if selected, ok := ws.app.SelectedPrefab(); ok && selected.Path() == bridgeArea {
		t.Fatal("undo kept an invalid area brush")
	}
	ws.app.CommandStorage().Redo()
	if !tools.SetGrabSelection(util.Point{X: 8, Y: 5, Z: 1}, hi) {
		t.Fatal("could not select cargo area")
	}
	ws.beginTask(taskArea)
	ws.itemName, ws.areaIcon = "Cargo bay", "quart"
	capture("ship-areas-second")
	ws.createArea()
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	cargoArea := ws.areaPath
	ws.change("Assign bridge area", func() error {
		return project.AssignArea(ws.currentTheme(), ws.assembly.Sources[0].File, bridgeArea, lo, hi)
	})
	ws.change("Assign cargo area", func() error {
		return project.AssignArea(ws.currentTheme(), ws.assembly.Sources[0].File, cargoArea, util.Point{X: 8, Y: 5, Z: 1}, hi)
	})
	capture("ship-areas-assigned")
	ws.finishTask()
	// Place the existing mobile port on a real entrance, with one undo entry.
	entrance := util.Point{X: 7, Y: lo.Y, Z: 1}
	ws.change("Map entrance", func() error {
		ws.assembly.Sources[0].Live.GetTile(entrance).InstancesAdd(dmmap.PrefabStorage.Initial("/obj/machinery/door/airlock"))
		return nil
	})
	beforeDock := ship.RawData(ws.pane.Dmm()).EncodeTGM()
	if !tools.SetGrabSelection(entrance, entrance) {
		t.Fatal("could not select airlock")
	}
	ws.beginTask(taskDocking)
	capture("docking-port-setup")
	ws.applyDocking()
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	afterDock := ship.RawData(ws.pane.Dmm()).EncodeTGM()
	if bytes.Equal(beforeDock, afterDock) {
		t.Fatal("port placement did not change map")
	}
	ws.app.CommandStorage().Undo()
	if !bytes.Equal(beforeDock, ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
		t.Fatal("port undo did not restore hull")
	}
	ws.app.CommandStorage().Redo()
	if !bytes.Equal(afterDock, ship.RawData(ws.pane.Dmm()).EncodeTGM()) {
		t.Fatal("port redo did not restore placement")
	}
	if !tools.SetGrabSelection(lo, hi) {
		t.Fatal("could not select room")
	}
	ws.beginTask(taskRoom)
	if !tools.IsSelected(tools.TNRoomShape) {
		t.Fatal("room action did not arm the room-shape tool")
	}
	if shape, origin, err := ship.FootprintFromTiles(tools.RoomShapeTiles()); err != nil || origin != lo || shape.W != hi.X-lo.X+1 || shape.H != hi.Y-lo.Y+1 || !shape.IsFull() {
		t.Fatalf("opening room action lost Grab selection: %v %v %v", shape, origin, err)
	}
	if ok, _ := ws.acceptShapeTile(util.Point{X: 200, Y: 200, Z: 1}); ok {
		t.Fatal("room shape accepted a tile outside the hull")
	}
	ws.itemID, ws.itemName = "cargo", "Cargo"
	capture("grab-create-room")
	ws.applyRegion()
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	ws.defaults()
	ws.rebuild()
	exerciseResizeControl(t, ws, render)
	exerciseHelperMovesAndResize(t, ws, capture)
	ws.source = 1
	ws.rebuild()
	ws.OnFocusChange(true)
	moduleEntrance := util.Point{X: entrance.X - lo.X + 1, Y: 1, Z: 1}
	if !tools.SetGrabSelection(moduleEntrance, moduleEntrance) {
		t.Fatal("could not select entrance through module")
	}
	ws.beginTask(taskDocking)
	if a, b, ok := tools.SelectionBounds(); !ok || ws.source != 0 || a != entrance || b != entrance {
		t.Fatal("docking action lost module-to-hull selection")
	}
	ws.finishTask()
	ws.source = 1
	ws.rebuild()
	ws.OnFocusChange(true)
	moduleLo, moduleHi := util.Point{X: 2, Y: 2, Z: 1}, util.Point{X: 3, Y: 3, Z: 1}
	if !tools.SetGrabSelection(moduleLo, moduleHi) {
		t.Fatal("could not select placed module")
	}
	ws.beginTask(taskArea)
	if ws.source != 1 {
		t.Fatal("area action switched away from selected module")
	}
	if gotLo, gotHi, ready := tools.SelectionBounds(); !ready || gotLo != moduleLo || gotHi != moduleHi {
		t.Fatal("area action lost module-local selection")
	}
	ws.areaPath = cargoArea
	ws.change("Assign module area", func() error {
		return project.AssignArea(ws.currentTheme(), ws.assembly.Sources[1].File, cargoArea, moduleLo, moduleHi)
	})
	ws.finishTask()
	tools.SetSelected(tools.TNAdd)
	native := ws.pane.Dmm()
	ws.selectRoomOption("cargo", "")
	if ws.source != 0 || len(ws.assembly.Sources) != 1 {
		t.Fatal("empty room did not select hull")
	}
	ws.selectRoomOption("cargo", "cargo_basic")
	if ws.source != 1 || ws.pane.Dmm() != native {
		t.Fatal("choosing a room option did not select it for editing")
	}
	ws.change("Create alternate room", func() error { return project.AddModule(0, project.Hull.Modules[0], "medical", "Medical", true) })
	ws.selectRoomOption("cargo", "medical")
	if ws.source != 1 || ws.assembly.Sources[ws.source].Name != "Medical" || ws.pane.Dmm() == native {
		t.Fatal("choosing a different option did not activate its map")
	}
	ws.selectRoomOption("cargo", "cargo_basic")
	hull := ws.assembly.Sources[0].Live
	beforeHull := ship.RawData(hull).EncodeTGM()
	coord := util.Point{X: 2, Y: 3, Z: 1}
	prefab := dmmap.PrefabStorage.Initial("/obj/structure/table")
	ws.pane.Editor().TileReplace(coord, dmmdata.Prefabs{prefab, dmmap.PrefabStorage.Initial("/turf/template_noop"), dmmap.PrefabStorage.Initial("/area/template_noop")})
	ws.pane.Editor().CommitContextNow("Paint table")
	if !bytes.Equal(beforeHull, ship.RawData(hull).EncodeTGM()) {
		t.Fatal("module edit changed hull")
	}
	global := util.Point{X: lo.X + coord.X - 1, Y: lo.Y + coord.Y - 1, Z: 1}
	found := false
	for _, atom := range ws.assembly.Cells[global] {
		if atom.Prefab.Path() == prefab.Path() {
			found = true
			if atom.Local != coord || atom.Source != 1 {
				t.Fatal("incorrect owner or local coordinate")
			}
		}
	}
	if !found {
		t.Fatal("source edit did not appear in live assembly")
	}
	// Switching away before undo must focus the owner and update the scene.
	ws.source = 0
	ws.rebuild()
	ws.app.CommandStorage().Undo()
	if ws.pane.Dmm() != native {
		t.Fatal("undo did not activate its source")
	}
	for _, atom := range ws.assembly.Cells[global] {
		if atom.Prefab.Path() == prefab.Path() {
			t.Fatal("undo kept painted object")
		}
	}
	ws.app.CommandStorage().Redo()
	ws.change("Clone theme", func() error { return project.AddTheme(0, "pirate", "Pirate", true) })
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	ws.theme = 1
	ws.defaults()
	ws.rebuild()
	if ws.assembly.Sources[1].Live == native {
		t.Fatal("theme shares source")
	}
	ws.app.CommandStorage().Undo() // theme creation
	ws.app.CommandStorage().Undo() // the earlier source edit still has valid history
	ws.app.CommandStorage().Redo()
	ws.app.CommandStorage().Redo()
	ws.theme = 0
	ws.defaults()
	ws.rebuild()
	ws.source = 1
	ws.rebuild()
	// Save just the isolated fixture, never production maps.
	ws.flush()
	ws.setStage(stepReview)
	capture("review-unsaved")
	if len(ws.reviewed) != 1 || len(ws.reviewed[0].files) == 0 {
		t.Fatal("review did not list the new ship's files")
	}
	ws.setStage(stepBuild)
	if err := project.Save(); err != nil {
		t.Fatal(err)
	}
	parsed, err := dmenv.New(draftEnv.RootFile)
	if err != nil {
		t.Fatal("generated area definitions did not parse:", err)
	}
	for _, path := range []string{bridgeArea, cargoArea} {
		if parsed.Objects[path] == nil || parsed.Objects[path].Parent().Path != "/area/shuttle/voidcrew/workshop_fixture" {
			t.Fatal("saved room area lost ship inheritance:", path)
		}
	}
	reopened, err := ship.OpenProject(catalog, &draftEnv, project.Hull)
	if err != nil {
		t.Fatal(err)
	}
	assembled, err := reopened.Assemble(reopened.Hull.Themes[0], map[string]string{"cargo": "cargo_basic"})
	if err != nil {
		t.Fatal(err)
	}
	if len(assembled.Sources) != 2 {
		t.Fatal("reopen lost module")
	}
	ws.pane.FitView()
	render()
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "new-ship-workshop.png"), 1400, 960)
	}
	ws.setStage(stepReview)
	if ws.Map() != nil {
		t.Fatal("review screen exposes map tools")
	}
	capture("review-save")
	exerciseCrew(t, ws, render, false)
	exerciseSiliconCrew(t, ws, render)
	exerciseVariantCrew(t, ws, render)
	exerciseCosts(t, ws, render, false)
	exerciseConfigurationRows(t, ws, render)
	exerciseShipDetails(t, ws, render)
	exerciseRenaming(t, ws, render)
	exerciseRoomRenaming(t, ws, render)
	exerciseTransitions(t, ws, render)
	exerciseRoomsPanel(t, ws, render)
	exerciseVariantsPanel(t, ws, render)
	exerciseLoadedShipRooms(t, ws, render)
	exerciseVariantCrew(t, ws, render)
	exerciseFixedShipConversion(t, ws, render)
	exerciseCrew(t, ws, render, true)
	exerciseCosts(t, ws, render, true)
	exerciseRenaming(t, ws, render)
	exerciseRoomRenaming(t, ws, render)
	exercisePreviewSaveHook(t, ws, render)
	exerciseRecovery(t, ws, render)
	exerciseLoadedShipDetailsAndRename(t, ws, render)
	exerciseRemoval(t, ws, render)
}

func exercisePreviewSaveHook(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	app := ws.app.(*previewApp)
	count := app.previewRequests
	if !ws.Save() || app.previewRequests != count+1 {
		t.Fatal("successful save did not request previews")
	}
	before := ws.project.Capture()
	if err := ws.project.SetPartCosts("ship", ship.PartCosts{"trade": 23}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ws.catalog.Root, "voidcrew/mapping/shuttles/workshop_fixture.dm")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(source, []byte("\n// external change\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if !ws.Save() || app.previewRequests != count+2 || !strings.Contains(ws.message, "Warning:") {
		t.Fatal("external edit prevented save or warning/preview was missing: ", ws.message)
	}
	render()
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "save-external-warning.png"), 1400, 960)
	}
	if err = os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	ws.project.Restore(before)
	ws.message = "Saved. Your ship files are up to date."
	ws.setStage(stepReview)
	for _, phase := range []string{"running", "stopping", "stopped", "failed"} {
		app.previewStatus = shippreview.Status{Phase: phase, Message: "Generating purchase previews... Another refresh is queued.\nhull workshop_fixture: 24x24, slots [cargo loaded_bay]"}
		if phase == "failed" {
			app.previewStatus.Message = "Ship saved. Preview generation failed. Check the log, then retry."
		} else if phase == "stopping" {
			app.previewStatus.Message = "Stopping preview generation..."
		} else if phase == "stopped" {
			app.StopShipPreviews()
		}
		for i := 0; i < 3; i++ {
			render()
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
			captureFrame(t, filepath.Join(dst, "preview-status-"+phase+".png"), 1400, 960)
		}
	}
	app.previewStatus = shippreview.Status{}
}
