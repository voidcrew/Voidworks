package wsship

import (
	"path/filepath"
	"sdmm/internal/ship"
	"strings"
	"testing"
)

func exerciseTransitions(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	ws.setStage(stepBuild)
	ws.beginCrew()
	ws.loadCrewScope("ship")
	original := ws.crew.jobs[0].Name
	ws.crew.jobs[0].Name = ""
	ws.crew.dirty = true
	ws.BeginNewShip()
	if ws.wizard || ws.task != taskCrew || !ws.crew.dirty || ws.crew.error == "" {
		t.Fatal("new ship discarded an invalid crew draft")
	}
	if !strings.HasPrefix(ws.Name(), "* ") {
		t.Fatal("draft has no unsaved indicator")
	}
	ws.crew.jobs[0].Name = original
	if !ws.commitCrew() {
		t.Fatal(ws.crew.error)
	}
	ws.finishTask()
	ws.OnFocusChange(true)
	ws.rebuild()
	current := ws.hull
	previous := ws.project
	ws.catalog.Hulls = append(ws.catalog.Hulls, ship.Hull{Type: "/datum/map_template/shuttle/voidcrew/missing_audit", Name: "Missing", Suffix: "missing_audit"})
	ws.hull = len(ws.catalog.Hulls) - 1
	ws.defaults()
	ws.rebuild()
	render()
	if !ws.invalid || ws.Map() != nil || ws.assembly != nil {
		t.Fatal("failed load retained previous editing target")
	}
	ws.catalog.Hulls = ws.catalog.Hulls[:len(ws.catalog.Hulls)-1]
	ws.hull = current
	ws.defaults()
	ws.rebuild()
	if ws.Map() == nil || ws.project != previous {
		t.Fatal("could not recover after failed load")
	}
	busy := ws.assembly.Sources[0].File
	ws.SourceBusy = func(file string) bool { return filepath.Clean(file) == filepath.Clean(busy) }
	ws.rebuild()
	if !ws.invalid || ws.Map() != nil || ws.assembly != nil {
		t.Fatal("ordinary tab conflict retained editing target")
	}
	ws.SourceBusy = nil
	ws.rebuild()
	if ws.Map() == nil {
		t.Fatal("could not recover after source conflict")
	}
	base := ws.project.Hull.Modules[0]
	ws.change("Clone audit room", func() error { return ws.project.AddModule(0, base, "audit_option", "Audit Option", false) })
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	option := ws.project.Hull.Modules[len(ws.project.Hull.Modules)-1]
	ws.selectRoomOption(option.Slot, option.ID)
	file := ws.pane.Dmm().Path.Absolute
	ws.beginCrew()
	ws.FocusSource(file)
	if ws.task != taskPaint || ws.Map() == nil || ws.pane.Dmm().Path.Absolute != file || ws.selected[option.Slot] != option.ID {
		t.Fatal("opening nondefault source did not activate its room")
	}
	ws.app.CommandStorage().Undo()
	if !ws.Save() {
		t.Fatal(ws.message)
	}
}
