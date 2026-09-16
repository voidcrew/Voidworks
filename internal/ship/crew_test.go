package ship

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func TestCrewScopesFollowVariantRooms(t *testing.T) {
	p := &Project{Dme: &dmenv.Dme{Objects: map[string]*dmenv.Object{}}, Hull: Hull{
		Type: HullType + "/crew_fixture", Slots: []string{"bay", "bridge"},
		Themes: []Theme{{ID: "standard"}, {ID: "medical", Slots: []string{"bay"}}, {ID: "empty", Slots: []string{}}},
		Modules: []Module{
			{ID: "shared", Slot: "bay", Themes: []string{"standard", "medical", "empty"}},
			{ID: "cargo", Slot: "bay", Themes: []string{"standard"}},
			{ID: "surgery", Slot: "bay", Themes: []string{"medical"}},
			{ID: "bridge", Slot: "bridge", Themes: []string{"standard", "medical"}},
			{ID: "unthemed", Slot: "bay"},
		},
	}}
	for _, test := range []struct {
		theme Theme
		want  []string
	}{
		{p.Hull.Themes[0], []string{"ship", "theme/standard", "module/shared", "module/cargo", "module/bridge"}},
		{p.Hull.Themes[1], []string{"ship", "theme/medical", "module/shared", "module/surgery"}},
		{p.Hull.Themes[2], []string{"ship", "theme/empty"}},
		{Theme{}, []string{"ship", "module/unthemed"}},
	} {
		var got []string
		for _, scope := range p.CrewScopesForTheme(test.theme) {
			got = append(got, scope.ID)
		}
		if test.theme.ID != "" {
			for i, scope := range test.want {
				if module, _ := RoomCrewIDs(scope); module != "" {
					test.want[i] = p.RoomCrewScope(module, test.theme.ID)
				}
			}
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("variant %q: rosters %v, want %v", test.theme.ID, got, test.want)
		}
	}
	if len(p.CrewScopes()) != 9 {
		t.Fatal("filtering the view removed rosters from the complete project")
	}
}

func TestCrewLiteralPreservesSurroundingSource(t *testing.T) {
	source := []byte("// job_slots = list(bad)\r\n/datum/example\r\n\tname = \"Example\"\r\n\tjob_slots = list(\r\n\t\tlist(name = \"A, B\", outfit = /datum/outfit/job/assistant, category = JOB_CAT_ASSISTANT, slots = 2, extra = list(\"a=b\", 3)),\r\n\t) // preserve this\r\n\tcost = 123\r\n\n/datum/example/proc/run()\r\n\treturn list(1, 2)\r\n")
	out, e := rewriteCrewList(source, "/datum/example", "job_slots", "list()")
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"// job_slots = list(bad)", "\tname = \"Example\"", "// preserve this\r\n\tcost = 123", "/datum/example/proc/run()\r\n\treturn list(1, 2)"} {
		if !bytes.Contains(out, []byte(s)) {
			t.Fatal("lost unrelated source", s)
		}
	}
	jobs, e := parseCrew(`list(list(name = "A, B", outfit = /datum/outfit/job/assistant, category = JOB_CAT_ASSISTANT, slots = 2, extra = list("a=b", 3)))`)
	if e != nil || len(jobs) != 1 || jobs[0].Name != "A, B" || jobs[0].Extra["extra"] != `list("a=b", 3)` {
		t.Fatalf("nested roster: %+v %v", jobs, e)
	}
	for _, s := range []string{"\tjob_slots = make_jobs()", "\tjob_slots = list() + other_jobs", "#ifdef CUSTOM\n\tjob_slots = list()\n#endif", "\tjob_slots = list()\n\tjob_slots = list()"} {
		if _, e := rewriteCrewList([]byte("/datum/example\n"+s+"\n"), "/datum/example", "job_slots", "list()"); e == nil {
			t.Fatal("accepted computed/ambiguous roster", s)
		}
	}
}
func crewTestTypes(p *Project) {
	for _, path := range []string{"/datum/job/assistant", "/datum/outfit/job/assistant", "/datum/outfit/job/captain"} {
		v := dmvars.MutableVariables{}
		v.Put("jobtype", "/datum/job/assistant")
		p.Dme.Objects[path] = &dmenv.Object{Path: path, Vars: v.ToImmutable()}
	}
}
func TestCrewSaveReopenUndoAndExternalEdit(t *testing.T) {
	c, env := authorEnvironment(t)
	p, e := NewProject(c, env, "crew_fixture", "Crew Fixture", 24, 24)
	if e != nil {
		t.Fatal(e)
	}
	crewTestTypes(p)
	before := p.Capture()
	jobs, _ := p.CrewJobs("ship")
	jobs[1].Name = "Builder"
	jobs[1].Slots = 3
	jobs[1].Equipment = map[string]string{"l_hand": "/obj/item/test", "head": ""}
	jobs[1].Backpack = map[string]int{"/obj/item/test": 2}
	if e = p.SetCrewJobs("ship", jobs); e != nil {
		t.Fatal(e)
	}
	after := p.Capture()
	if e = p.Save(); e != nil {
		t.Fatal(e)
	}
	if p.Modified() {
		t.Fatal("saved crew remains dirty")
	}
	reopened, e := OpenProject(c, env, p.Hull)
	if e != nil {
		t.Fatal(e)
	}
	got, e := reopened.CrewJobs("ship")
	if e != nil || got[1].Name != "Builder" || got[1].Backpack["/obj/item/test"] != 2 {
		t.Fatalf("reopen lost crew: %+v %v", got, e)
	}
	p.Restore(before)
	if e = p.Save(); e != nil {
		t.Fatal(e)
	}
	hull, _ := os.ReadFile(p.outputPaths()[1])
	if bytes.Contains(hull, []byte("Builder")) {
		t.Fatal("undo after save kept edited jobs")
	}
	p.Restore(after)
	if e = p.Save(); e != nil {
		t.Fatal(e)
	}
	got, _ = p.CrewJobs("ship")
	got[1].Name = " CAPTAIN "
	if p.SetCrewJobs("ship", got) == nil {
		t.Fatal("accepted duplicate job name")
	}
	got[1].Name = "Builder"
	got[1].Slots = 0
	if p.SetCrewJobs("ship", got) == nil {
		t.Fatal("accepted zero slots (game treats it as one)")
	}
	got[1].Slots = 4
	if e = p.SetCrewJobs("ship", got); e != nil {
		t.Fatal(e)
	}
	_, code := p.crewPaths()
	b, _ := os.ReadFile(code)
	_ = os.WriteFile(code, append(b, []byte("// external edit\n")...), 0600)
	if p.Save() == nil {
		t.Fatal("overwrote external outfit edits")
	}
}
func TestFleetCrewSources(t *testing.T) {
	file := os.Getenv("SHIP_RENDER_TEST_DME")
	if file == "" {
		t.Skip("set SHIP_RENDER_TEST_DME")
	}
	env, e := dmenv.New(file)
	if e != nil {
		t.Fatal(e)
	}
	catalog, e := Discover(env)
	if e != nil {
		t.Fatal(e)
	}
	checked := 0
	for _, h := range catalog.Hulls {
		p, e := OpenProject(catalog, env, h)
		if e != nil {
			t.Fatal(e)
		}
		for _, scope := range p.CrewScopes() {
			jobs, e := p.CrewJobs(scope.ID)
			if e != nil {
				t.Fatalf("%s %s: %v", h.Name, scope.Name, e)
			}
			path, e := p.roomTypeFile(scope.Type)
			if e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = rewriteCrewList(b, scope.Type, scope.Field, renderCrew(jobs, p)); e != nil {
				t.Fatalf("%s %s: %v", h.Name, scope.Name, e)
			}
			actual, explicit, e := sourceCrew(b, scope.Type, scope.Field)
			if e != nil || (explicit && renderCrew(actual, p) != renderCrew(jobs, p)) {
				t.Fatalf("source roster differs: %s %s: %v", h.Name, scope.Name, e)
			}
			roundtrip, e := parseCrew(renderCrew(jobs, p))
			if e != nil || len(roundtrip) != len(jobs) {
				t.Fatal("roundtrip", e)
			}
			checked++
		}
		if p.Modified() {
			t.Fatal("reading crew marked ship dirty")
		}
	}
	if checked == 0 {
		t.Fatal("no rosters checked")
	}
	t.Logf("checked %d ship, variant and module crew definitions", checked)
}

func TestLoadedCrewSurvivesLaterRoomEdits(t *testing.T) {
	p, source := loadedRoomProject(t, true)
	crewTestTypes(p)
	jobs := []CrewJob{{Name: "Technician", Slots: 2, Category: "Engineering", Outfit: "/datum/outfit/job/assistant"}}
	if e := p.SetCrewJobs("theme/other", jobs); e != nil {
		t.Fatal(e)
	}
	if e := p.Save(); e != nil {
		t.Fatal(e)
	}
	if e := p.addRect(1, "crew_bay", "Crew Bay", util.Point{X: 10, Y: 10, Z: 1}, util.Point{X: 12, Y: 12, Z: 1}); e != nil {
		t.Fatal(e)
	}
	jobs[0].Name = "Mechanic"
	if e := p.SetCrewJobs("module/crew_bay_basic", jobs); e != nil {
		t.Fatal(e)
	}
	after := p.Capture()
	if e := p.Save(); e != nil {
		t.Fatal(e)
	}
	data, _ := os.ReadFile(source)
	if !bytes.Contains(data, []byte(`"Technician"`)) || !bytes.Contains(data, []byte(`"crew_bay"`)) || !bytes.Contains(data, []byte("custom_proc()")) {
		t.Fatal("room save discarded crew or handwritten source")
	}
	code, _ := os.ReadFile(p.rooms.code)
	if !bytes.Contains(code, []byte(`job_slots_add = list(list(name = "Mechanic"`)) {
		t.Fatal("new room crew missing")
	}
	jobs[0].Name = "Replacement"
	if e := p.SetCrewJobs("module/crew_bay_basic", jobs); e != nil {
		t.Fatal(e)
	}
	if e := p.Save(); e != nil {
		t.Fatal(e)
	}
	p.Restore(after)
	if e := p.Save(); e != nil {
		t.Fatal(e)
	}
	code, _ = os.ReadFile(p.rooms.code)
	if bytes.Contains(code, []byte("Replacement")) {
		t.Fatal("undo after save kept changed module job")
	}
}
