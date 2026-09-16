package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/ship"
)

func exerciseCrew(t *testing.T, ws *WsShip, render func(), legacy bool) {
	t.Helper()
	ws.setStage(stepBuild)
	ws.beginCrew()
	ws.loadCrewScope("ship")
	// The temporary DME includes the real game; copy the GAGS recipe used by
	// this test so its worn-color layers resolve from the fixture root too.
	if original := os.Getenv("SHIP_RENDER_TEST_DME"); original != "" {
		rel := "code/datums/greyscale/json_configs/jumpsuit_worn.json"
		data, e := os.ReadFile(filepath.Join(filepath.Dir(original), rel))
		if e != nil {
			t.Fatal(e)
		}
		dst := filepath.Join(ws.project.Dme.RootDir, rel)
		if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(dst, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if ws.Map() != nil {
		t.Fatal("crew editor exposes map tools")
	}
	if ws.crew.error != "" {
		t.Fatal(ws.crew.error)
	}
	if len(ws.crew.jobs) < 2 {
		t.Fatal("default crew missing")
	}
	for _, path := range []string{"/obj/item/clothing/suit/armor", "/obj/item/clothing/suit/armor/abductor", "/obj/item/clothing/under"} {
		if ws.project.Dme.Objects[path] == nil {
			t.Fatalf("missing item fixture: %s", path)
		}
		if ws.crewItemSprite(path) != nil {
			t.Fatalf("iconless parent appears in the crew item library: %s", path)
		}
	}
	for _, path := range []string{"/obj/item/clothing/suit/armor/abductor/vest", "/obj/item/clothing/under/color", "/obj/item/clothing/under/color/blue", "/obj/item/crowbar"} {
		if ws.crewItemSprite(path) == nil {
			t.Fatalf("usable item is missing from the crew item library: %s", path)
		}
	}
	ws.crew.selected = 1
	ws.crew.jobs[1].Name = "Salvage Engineer"
	ws.crew.jobs[1].Outfit = "/datum/outfit/job/engineer"
	ws.crew.jobs[1].BaseOutfit = ""
	ws.crew.jobs[1].Equipment = nil
	ws.crew.dirty = true
	if !ws.commitCrew() {
		t.Fatal(ws.crew.error)
	}
	ws.crew.slot = 0
	ws.chooseCrewItem("/obj/item/clothing/under/color/orange")
	for i := 0; i < 3; i++ {
		render()
	}
	first := ws.crewVisual.key
	// Select from the same global prefab channel used by Environment search.
	ws.app.DoSelectPrefab(dmmap.PrefabStorage.Initial("/obj/item/clothing/under/color/blue"))
	render()
	if first == ws.crewVisual.key || ws.project.CrewEquipment(ws.crew.jobs[1])["uniform"] != "/obj/item/clothing/under/color/blue" {
		t.Fatal("Environment selection did not update outfit and preview")
	}
	if len(ws.crewVisual.layers) < 10 {
		t.Fatalf("missing worn/body sprites: %d %+v", len(ws.crewVisual.layers), ws.crewVisual.warnings)
	}
	if !legacy {
		path := "/obj/item/clothing/under/color/blue/audit_tint"
		if e := ws.project.Dme.AddDraftType(path, map[string]string{"alpha": "128", "color": `"#ff8040"`}); e != nil {
			t.Fatal(e)
		}
		job := ship.CloneCrewJobs([]ship.CrewJob{ws.crew.jobs[1]})[0]
		job.Equipment["uniform"] = path
		for _, dir := range []int{1, 2, 4, 8} {
			plain := buildCrewVisual(ws.project, ws.crew.jobs[1], dir)
			tinted := buildCrewVisual(ws.project, job, dir)
			if len(plain.layers) != len(tinted.layers) {
				t.Fatal("tint lost worn layers")
			}
			changed := false
			for i, l := range tinted.layers {
				if l.color != plain.layers[i].color && l.color>>24 == 128 {
					changed = true
				}
			}
			if !changed {
				t.Fatalf("missing clothing tint/alpha in direction %d", dir)
			}
		}
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		name := "crew-editor.png"
		if legacy {
			name = "loaded-crew-editor.png"
		}
		captureFrame(t, filepath.Join(dst, name), 1400, 960)
	}
	ws.crew.contents = 1
	ws.chooseCrewItem("/obj/item/crowbar")
	if ws.crew.error != "" {
		t.Fatal(ws.crew.error)
	}
	ws.crew.contents = 0
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	fresh, e := dmenv.New(ws.project.Dme.RootFile)
	if e != nil {
		t.Fatal(e)
	}
	reopened, e := ship.OpenProject(ws.catalog, fresh, ws.project.Hull)
	if e != nil {
		t.Fatal(e)
	}
	jobs, e := reopened.CrewJobs("ship")
	if e != nil || len(jobs) < 2 || jobs[1].Name != "Salvage Engineer" || reopened.CrewEquipment(jobs[1])["uniform"] != "/obj/item/clothing/under/color/blue" {
		t.Fatalf("saved roster did not reopen: %+v %v", jobs, e)
	}
	custom := fresh.Objects[jobs[1].Outfit]
	if custom == nil || custom.Vars.ValueV("jobtype", "") != "/datum/job/station_engineer" {
		t.Fatal("custom outfit lost its underlying job")
	}
	if legacy {
		// Theme and existing module edits must merge safely with room edits even
		// when definitions share a DM file and DME include list.
		for _, scope := range reopened.CrewScopes() {
			if scope.ID == "ship" {
				continue
			}
			j := ship.CrewJob{Name: "Guest", Slots: 1, Category: "Assistant", Outfit: "/datum/outfit/job/assistant"}
			if e = reopened.SetCrewJobs(scope.ID, []ship.CrewJob{j}); e != nil {
				t.Fatal(e)
			}
			break
		}
		if e = reopened.Save(); e != nil {
			t.Fatal(e)
		}
		b, _ := os.ReadFile(filepath.Join(ws.catalog.Root, "voidcrew/mapping/shuttles/workshop_fixture.dm"))
		if !bytes.Contains(b, []byte("Handwritten ship with custom jobs and costs")) {
			t.Fatal("crew edits erased handwritten fields")
		}
	}
	ws.setStage(stepBuild)
}
