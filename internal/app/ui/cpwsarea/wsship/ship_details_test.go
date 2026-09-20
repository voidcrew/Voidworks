package wsship

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/ship"
)

// Exercise the actual configuration rows on both ordinary and authored ships.
func exerciseConfigurationRows(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	io := imgui.CurrentIO()
	defer func() {
		io.SetMouseButtonDown(0, false)
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		ws.showSources = false
		ws.finishTask()
	}()
	for _, row := range []int{0, 1, 3} {
		ws.finishTask()
		var at imgui.Vec2
		frame := func() {
			imgui.NewFrame()
			imgui.SetNextWindowPos(imgui.Vec2{X: 20, Y: 20})
			imgui.SetNextWindowSize(imgui.Vec2{X: 420, Y: 620})
			imgui.BeginV("Ship configuration regression", nil, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsNoResize|imgui.WindowFlagsNoMove)
			workshop.PushStyle()
			height := imgui.TextLineHeight() + 22*window.PointSize()
			at = imgui.CursorScreenPos().Plus(imgui.Vec2{X: 100, Y: height/2 + float32(row)*(height+10*window.PointSize())})
			if row == 3 {
				at.Y += imgui.TextLineHeight()
			}
			ws.configurationActions()
			workshop.PopStyle()
			imgui.End()
			imgui.Render()
		}
		for range 3 {
			frame()
		}
		io.SetMousePosition(at)
		frame()
		io.SetMouseButtonDown(0, true)
		frame()
		io.SetMouseButtonDown(0, false)
		frame()
		if row == 0 && ws.task != taskSettings || row == 1 && ws.task != taskRenameShip || row == 3 && !ws.showSources {
			t.Fatalf("configuration row %d is inaccessible (authored: %t, task: %d)", row, ws.project.Settings != nil, ws.task)
		}
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		for range 3 {
			render()
		}
	}
}

func exerciseLoadedShipDetailsAndRename(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	if ws.project.Settings != nil {
		t.Fatal("expected handwritten fixture")
	}
	ws.setStage(stepBuild)
	exerciseConfigurationRows(t, ws, render)
	old := ws.project.Hull
	originalFileName := ws.project.ShipFileName()
	ws.beginTask(taskSettings)
	ws.settings.description = "An existing ship with edited details"
	ws.settings.hidden = true
	if !ws.Save() {
		t.Fatal("handwritten details save:", ws.message)
	}
	check := func(name, description string, hidden bool) {
		t.Helper()
		want := old
		want.Name, want.Description, want.Hidden = name, description, hidden
		if !reflect.DeepEqual(ws.project.Hull, want) || ws.project.ShipFileName() != originalFileName {
			t.Fatal("display-name form changed map references or filenames")
		}
		cached, err := ship.OpenProject(ws.catalog, ws.project.Dme, ws.project.Hull)
		if err != nil {
			t.Fatal(err)
		}
		probe := &WsShip{project: cached}
		probe.beginTask(taskSettings)
		if probe.settings.hidden != hidden {
			t.Fatalf("reopened details checkbox is %v for saved hidden=%v", probe.settings.hidden, hidden)
		}
		env, err := dmenv.New(ws.project.Dme.RootFile)
		if err != nil {
			t.Fatal(err)
		}
		catalog, err := ship.Discover(env)
		if err != nil {
			t.Fatal(err)
		}
		for _, hull := range catalog.Hulls {
			if hull.Type != old.Type {
				continue
			}
			if hull.Name != name || hull.Description != description || hull.Hidden != hidden {
				t.Fatalf("reparsed ship differs: %+v", hull)
			}
			p, err := ship.OpenProject(catalog, env, hull)
			if err != nil {
				t.Fatal(err)
			}
			for _, theme := range p.Hull.Themes {
				if _, err := p.Assemble(theme, nil); err != nil {
					t.Fatal(err)
				}
			}
			return
		}
		t.Fatal("ship disappeared from library")
	}
	check(old.Name, ws.settings.description, true)
	ws.beginRename(taskRenameShip, "ship", old.Name)
	ws.itemName = "Renamed Existing Ship"
	for range 3 {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "rename-existing-ship.png"), 1400, 960)
	}
	if !ws.Save() {
		t.Fatal("handwritten rename save:", ws.message)
	}
	check("Renamed Existing Ship", ws.settings.description, true)
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal("rename undo:", ws.message)
	}
	check(old.Name, ws.settings.description, true)
	ws.app.CommandStorage().Redo()
	if !ws.Save() {
		t.Fatal("rename redo:", ws.message)
	}
	check("Renamed Existing Ship", ws.settings.description, true)
	ws.app.CommandStorage().Undo()
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal("details undo:", ws.message)
	}
	check(old.Name, old.Description, old.Hidden)
	exerciseSeparateFileRename(t, ws, render)
	ws.setStage(stepBuild)
	ws.showSources = true
	io := imgui.CurrentIO()
	io.SetMousePosition(imgui.Vec2{X: 150, Y: 800})
	for range 3 {
		render()
	}
	io.AddMouseWheelDelta(0, -100)
	for range 3 {
		render()
	}
	io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "ship-configuration.png"), 1400, 960)
	}
	ws.showSources = false
}

func exerciseSeparateFileRename(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	oldName, oldFiles := ws.project.Hull.Name, ws.project.ShipFileName()
	oldMap, err := ws.catalog.HullFile(ws.project.Hull, ws.project.Hull.Themes[0])
	if err != nil {
		t.Fatal(err)
	}
	ws.setStage(stepBuild)
	ws.beginRename(taskRenameShipFiles, "files", oldFiles)
	ws.itemName = "separate_file_test"
	for range 3 {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "rename-files.png"), 1400, 960)
	}
	// Ctrl+S must stage the rename and show the review, not move files immediately.
	if ws.Save() || ws.stage != stepReview || ws.project.Hull.Name != oldName {
		t.Fatal("file rename skipped review or changed player-facing name:", ws.message)
	}
	if _, err := os.Stat(oldMap); err != nil {
		t.Fatal("file was moved before review", err)
	}
	ws.prepareReview()
	removed, created := false, false
	for _, project := range ws.reviewed {
		for _, file := range project.files {
			removed = removed || file.deleted
			created = created || !file.existed && !file.deleted
		}
	}
	if !removed || !created {
		t.Fatal("review does not expose both old and new paths")
	}
	for range 3 {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "rename-files-review.png"), 1400, 960)
	}
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	if ws.project.Hull.Name != oldName || ws.project.ShipFileName() != "separate_file_test" {
		t.Fatal("file rename did not preserve display name")
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() || ws.project.ShipFileName() != oldFiles {
		t.Fatal("file rename undo failed:", ws.message)
	}
	ws.app.CommandStorage().Redo()
	if !ws.Save() || ws.project.Hull.Name != oldName {
		t.Fatal("file rename redo failed:", ws.message)
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	if _, err := os.Stat(oldMap); err != nil {
		t.Fatal("file undo did not restore old map", err)
	}
	ws.setStage(stepBuild)
}
