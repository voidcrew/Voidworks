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
		if module, _ := ship.RoomCrewIDs(scope); module != "" {
			scope = p.RoomCrewScope(module, ws.currentTheme().ID)
		}
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
	if ws.theme != 1 || ws.crew.scope != p.RoomCrewScope("variant_engineering", second.ID) || len(ws.crew.jobs) != 0 {
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
	setJob("Second quartermaster")
	ws.switchCrewTheme(0)
	if ws.crew.scope != p.RoomCrewScope("cargo_basic", first.ID) || len(ws.crew.jobs) != 0 {
		t.Fatal("a different variant inherited this room's new job")
	}
	if len(ws.crew.copySources) < 3 || ws.crew.copySources[0].Module.ID != "cargo_basic" || ws.crew.copySources[0].Theme.ID != second.ID {
		t.Fatal("copy control did not offer the configured variant")
	}
	ws.crew.copyOpen = true
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		name := "copy-room-job.png"
		if p.Settings == nil {
			name = "copy-room-job-loaded.png"
		}
		captureFrame(t, filepath.Join(dst, name), 1400, 960)
	}
	ws.crew.copyOpen = false
	render()
	if !ws.copyRoomCrewJob("cargo_basic", second.ID, 0) || len(ws.crew.jobs) != 1 || ws.crew.jobs[0].Name != "Second quartermaster" {
		t.Fatal("copying the selected job failed", ws.crew.error)
	}
	ws.app.CommandStorage().Undo()
	if len(ws.crew.jobs) != 0 || ws.theme != 0 {
		t.Fatal("undo did not remove the copied job from only its target variant")
	}
	ws.app.CommandStorage().Redo()
	if len(ws.crew.jobs) != 1 {
		t.Fatal("redo did not restore the copied job")
	}
	for _, source := range []struct{ module, theme, name string }{
		{"medical", first.ID, "Medical technician"},
		{"variant_engineering", second.ID, "Variant engineer"},
	} {
		before := len(ws.crew.jobs)
		if !ws.copyRoomCrewJob(source.module, source.theme, 0) || len(ws.crew.jobs) != before+1 || ws.crew.jobs[before].Name != source.name || ws.theme != 0 {
			t.Fatal("copying a job from another module failed", ws.crew.error)
		}
		ws.app.CommandStorage().Undo()
		if len(ws.crew.jobs) != before || ws.crew.scope != p.RoomCrewScope("cargo_basic", first.ID) {
			t.Fatal("cross-module copy undo changed the wrong roster")
		}
		ws.app.CommandStorage().Redo()
		if len(ws.crew.jobs) != before+1 || ws.crew.jobs[before].Name != source.name {
			t.Fatal("cross-module copy redo lost the copied job")
		}
	}
	ws.crew.jobs[0].Name, ws.crew.dirty = "First quartermaster", true
	if !ws.commitCrew() {
		t.Fatal(ws.crew.error)
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
	if err = json.Unmarshal(data, &recovery); err != nil || recovery.Theme != 0 || recovery.Form.CrewScope != p.RoomCrewScope("cargo_basic", first.ID) || !recovery.Form.CrewDirty {
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
	for scope, names := range map[string][]string{
		"theme/" + first.ID: {"First captain"}, "theme/" + second.ID: {"Second captain"},
		p.RoomCrewScope("medical", first.ID): {"Medical technician"}, p.RoomCrewScope("variant_engineering", second.ID): {"Variant engineer"},
		p.RoomCrewScope("cargo_basic", first.ID): {"First quartermaster", "Medical technician", "Variant engineer"}, p.RoomCrewScope("cargo_basic", second.ID): {"Second quartermaster"},
	} {
		jobs, err := reopened.CrewJobs(scope)
		if err != nil || len(jobs) != len(names) {
			t.Fatalf("saved roster %s did not reopen: %+v, %v", scope, jobs, err)
		}
		for i, name := range names {
			if jobs[i].Name != name {
				t.Fatalf("saved module copy did not reopen in %s: %+v", scope, jobs)
			}
		}
	}
	if jobs, err := reopened.CrewJobs("ship"); err != nil || !reflect.DeepEqual(jobs, baseJobs) {
		t.Fatal("variant edits changed the shared ship crew", err)
	}
}
