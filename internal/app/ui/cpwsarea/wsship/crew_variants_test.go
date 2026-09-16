package wsship

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/ship"
)

func exerciseVariantCrew(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	ws.setStage(stepBuild)
	p := ws.project
	before, theme := p.Capture(), ws.theme
	defer func() {
		ws.task = taskPaint
		p.Restore(before)
		ws.catalog.Hulls[ws.hull] = p.Hull
		ws.theme = theme
		ws.defaults()
		ws.rebuild()
		if !ws.Save() {
			t.Fatal(ws.message)
		}
	}()
	first, second := p.Hull.Themes[0], p.Hull.Themes[1]
	baseJobs, err := p.CrewJobs("ship")
	if err != nil {
		t.Fatal(err)
	}
	ws.change("Configure variant crew rooms", func() error {
		if err := p.SetModuleThemes("medical", []string{first.ID}); err != nil {
			return err
		}
		return p.AddModule(1, p.Hull.Modules[0], "variant_engineering", "Variant engineering", false)
	})
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	ws.switchTheme(0)
	ws.beginCrew()
	checkScope := func(scope string, present bool) {
		t.Helper()
		found := false
		for _, s := range ws.crew.scopes {
			found = found || s.ID == scope
		}
		if found != present {
			t.Fatalf("variant %s: roster %s visible=%v, want %v", ws.currentTheme().ID, scope, found, present)
		}
	}
	if ws.crew.scope != "theme/"+first.ID {
		t.Fatal("crew editor did not open the active variant")
	}
	checkScope("theme/"+second.ID, false)
	checkScope("module/medical", true)
	checkScope("module/variant_engineering", false)
	setJob := func(name string) {
		ws.crew.jobs = []ship.CrewJob{{Name: name, Category: "Assistant", Slots: 1, Outfit: "/datum/outfit/job/assistant"}}
		ws.crew.selected, ws.crew.dirty = 0, true
	}
	ws.loadCrewScope("module/medical")
	setJob("Medical technician")
	ws.switchCrewTheme(1)
	if ws.crew.scope != "theme/"+second.ID || ws.crew.dirty {
		t.Fatal("variant switch did not commit the room and select the new variant roster")
	}
	checkScope("module/medical", false)
	checkScope("module/variant_engineering", true)
	ws.loadCrewScope("module/variant_engineering")
	setJob("Variant engineer")
	if !ws.commitCrew() {
		t.Fatal(ws.crew.error)
	}
	ws.app.CommandStorage().Undo()
	if ws.theme != 1 || ws.crew.scope != "module/variant_engineering" || len(ws.crew.jobs) != 0 {
		t.Fatal("undo restored the wrong variant or room roster")
	}
	ws.app.CommandStorage().Redo()
	if len(ws.crew.jobs) != 1 || ws.crew.jobs[0].Name != "Variant engineer" {
		t.Fatal("redo did not restore the room's crew")
	}
	ws.switchCrewTheme(0)
	ws.loadCrewScope("module/medical")
	if len(ws.crew.jobs) != 1 || ws.crew.jobs[0].Name != "Medical technician" {
		t.Fatal("editing another variant changed the first room's crew")
	}
	ws.loadCrewScope("theme/" + first.ID)
	setJob("First captain")
	ws.switchCrewTheme(1)
	setJob("Second captain")
	if !ws.commitCrew() {
		t.Fatal(ws.crew.error)
	}
	ws.loadCrewScope("module/cargo_basic")
	setJob("Shared quartermaster")
	ws.switchCrewTheme(0)
	if ws.crew.scope != "module/cargo_basic" || ws.crew.jobs[0].Name != "Shared quartermaster" {
		t.Fatal("switching variants lost the shared room roster")
	}
	ws.crew.jobs[0].Name, ws.crew.dirty = "", true
	ws.switchCrewTheme(1)
	if ws.theme != 0 || !ws.crew.dirty || ws.crew.error == "" {
		t.Fatal("variant switch discarded an unfinished job")
	}
	data, _, err := ws.captureRecovery()
	if err != nil {
		t.Fatal(err)
	}
	var recovery recoveryWorkspace
	if err = json.Unmarshal(data, &recovery); err != nil || recovery.Theme != 0 || recovery.Form.CrewScope != "module/cargo_basic" || !recovery.Form.CrewDirty {
		t.Fatal("recovery lost the variant or unfinished room roster", err)
	}
	ws.loadCrewScope("module/cargo_basic") // Discard the unfinished form.
	// Older recovery copies could select a room from another variant because
	// the previous roster picker listed every room at once.
	ws.loadCrewScope("module/variant_engineering")
	if ws.theme != 1 || ws.crew.jobs[0].Name != "Variant engineer" {
		t.Fatal("loading a variant-specific room did not synchronize the variant selector")
	}
	checkScope("module/medical", false)
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "variant-crew.png"), 1400, 960)
	}
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	fresh, err := dmenv.New(p.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := ship.OpenProject(ws.catalog, fresh, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	for scope, name := range map[string]string{
		"theme/" + first.ID: "First captain", "theme/" + second.ID: "Second captain",
		"module/medical": "Medical technician", "module/variant_engineering": "Variant engineer",
		"module/cargo_basic": "Shared quartermaster",
	} {
		jobs, err := reopened.CrewJobs(scope)
		if err != nil || len(jobs) != 1 || jobs[0].Name != name {
			t.Fatalf("saved roster %s did not reopen: %+v, %v", scope, jobs, err)
		}
	}
	if jobs, err := reopened.CrewJobs("ship"); err != nil || !reflect.DeepEqual(jobs, baseJobs) {
		t.Fatal("variant edits changed the shared ship crew", err)
	}
}
