package ship

import (
	"bytes"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmvars"
)

func enableRoomCrewVariants(p *Project) {
	v := dmvars.MutableVariables{}
	v.Put(roomCrewField, "null")
	p.Dme.Objects["/datum/ship_upgrade_module"] = &dmenv.Object{Path: "/datum/ship_upgrade_module", Vars: v.ToImmutable()}
}

func TestIndependentRoomCrewAndJobCopies(t *testing.T) {
	for _, authored := range []bool{false, true} {
		t.Run(map[bool]string{false: "handwritten", true: "generated"}[authored], func(t *testing.T) {
			check := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			p, source, _ := variantShipProject(t)
			if authored {
				c, env := authorEnvironment(t)
				var err error
				p, err = NewProject(c, env, "crew_variants", "Crew Variants", 20, 20)
				check(err)
				crewTestTypes(p)
				check(p.Deck(p.Hull.Themes[0], pt(3, 3), pt(16, 16)))
				check(p.addRect(0, "cargo", "Cargo", pt(5, 5), pt(7, 7)))
				check(p.AddTheme(0, "other", "Other", false))
				source = p.outputPaths()[2]
			}
			enableRoomCrewVariants(p)
			legacy := []CrewJob{{Name: "Engineer", Slots: 2, Category: "Engineering", Outfit: "/datum/outfit/job/assistant", Equipment: map[string]string{"head": ""}, Backpack: map[string]int{"/obj/item/test": 2}}}
			check(p.SetCrewJobs("module/cargo_basic", legacy))
			check(p.Save())
			before := p.Capture()
			first, second := p.RoomCrewScope("cargo_basic", "standard"), p.RoomCrewScope("cargo_basic", "other")
			jobs, err := p.CrewJobs(first)
			check(err)
			jobs[0].Slots, jobs[0].Backpack["/obj/item/test"] = 3, 4
			check(p.SetCrewJobs(first, jobs))
			other, err := p.CrewJobs(second)
			check(err)
			if other[0].Slots != 2 || other[0].Backpack["/obj/item/test"] != 2 {
				t.Fatal("editing a room changed another variant's crew")
			}
			other[0].Name, other[0].Slots = "Medic", 5
			check(p.SetCrewJobs(second, other))
			check(p.SetCrewJobs(first, nil))
			if jobs, err = p.CrewJobs(first); err != nil || len(jobs) != 0 {
				t.Fatal("empty room crew fell back to legacy jobs", jobs, err)
			}
			index, err := p.CopyRoomCrewJob("cargo_basic", "other", "cargo_basic", "standard", 0)
			check(err)
			if index != 0 {
				t.Fatal("copy did not select the added job")
			}
			check(p.AddTheme(0, "third", "Third", false))
			third := p.RoomCrewScope("cargo_basic", "third")
			jobs, err = p.CrewJobs(third)
			check(err)
			jobs[0].Name, jobs[0].Slots = "Scientist", 1
			check(p.SetCrewJobs(third, jobs))
			sources, err := p.RoomCrewSources("cargo_basic", "standard")
			check(err)
			if len(sources) != 2 || sources[0].Theme.ID != "other" || sources[1].Theme.ID != "third" {
				t.Fatal("copy picker did not offer every configured variant", sources)
			}
			_, err = p.CopyRoomCrewJob("cargo_basic", "other", "cargo_basic", "standard", 0)
			check(err)
			_, err = p.CopyRoomCrewJob("cargo_basic", "third", "cargo_basic", "standard", 0)
			check(err)
			copied, err := p.CrewJobs(first)
			check(err)
			if len(copied) != 3 || copied[0].Name != "Medic" || copied[1].Name != "Medic (copy)" || copied[2].Name != "Scientist" || copied[0].Backpack["/obj/item/test"] != 2 {
				t.Fatal("copy replaced existing jobs or lost equipment", copied)
			}
			other, err = p.CrewJobs(second)
			check(err)
			if copied[0].ID == other[0].ID || copied[0].Outfit == other[0].Outfit {
				t.Fatal("copy retained shared outfit identifiers")
			}
			other[0].Slots, other[0].Backpack["/obj/item/test"] = 9, 9
			check(p.SetCrewJobs(second, other))
			got, err := p.CrewJobs(first)
			check(err)
			if !reflect.DeepEqual(copied, got) {
				t.Fatal("copied jobs stayed linked to their source")
			}
			check(p.RemoveTheme("third"))
			if _, exists := p.Crew.ModuleThemes["cargo_basic"]["third"]; exists {
				t.Fatal("deleted variant kept its roster")
			}
			after := p.Capture()
			p.Restore(before)
			jobs, err = p.CrewJobs(first)
			check(err)
			if len(jobs) != 1 || jobs[0].Slots != 2 {
				t.Fatal("undo did not restore the original crew")
			}
			p.Restore(after)
			data, err := p.CaptureRecovery()
			check(err)
			recovered, err := RecoverProject(p.Catalog, p.Dme, data)
			check(err)
			jobs, err = recovered.CrewJobs(first)
			check(err)
			if !reflect.DeepEqual(jobs, copied) {
				t.Fatal("recovery lost independent copied jobs")
			}
			check(p.Save())
			if !bytes.Contains(mustRead(t, source), []byte("job_slots_add_by_theme = list(")) {
				t.Fatal("save omitted per-variant game rosters")
			}
			if authored {
				reopened, err := OpenProject(p.Catalog, p.Dme, p.Hull)
				check(err)
				jobs, err = reopened.CrewJobs(first)
				check(err)
				if !reflect.DeepEqual(jobs, copied) {
					t.Fatal("save/reopen lost copied jobs")
				}
			}
		})
	}
}

func TestIndependentRoomCrewRequiresGameSupport(t *testing.T) {
	p, _, _ := variantShipProject(t)
	before := p.Capture()
	if p.SetCrewJobs(p.RoomCrewScope("cargo_basic", "other"), nil) == nil {
		t.Fatal("accepted a roster the game cannot load")
	}
	if !reflect.DeepEqual(before.Crew, p.Crew) {
		t.Fatal("unsupported edit changed crew")
	}
}

func TestCopyCrewAcrossModules(t *testing.T) {
	p, _, _ := variantShipProject(t)
	enableRoomCrewVariants(p)
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	source := p.RoomCrewScope("cargo_lab", "standard")
	other := p.RoomCrewScope("cargo_lab", "other")
	target := p.RoomCrewScope("cargo_basic", "standard")
	job := CrewJob{Name: "Technician", Slots: 3, Category: "Engineering", Officer: true, Outfit: "/datum/outfit/job/assistant",
		Equipment: map[string]string{"head": "", "l_hand": "/obj/item/test"},
		Backpack:  map[string]int{"/obj/item/test": 4}, Belt: map[string]int{"/obj/item/test": 2}, Extra: map[string]string{"custom": "TRUE"}}
	check(p.SetCrewJobs(source, []CrewJob{job}))
	job.Slots = 5
	check(p.SetCrewJobs(other, []CrewJob{job}))
	job.Slots = 1
	check(p.SetCrewJobs(target, []CrewJob{job}))
	before := p.Capture()
	for _, theme := range []string{"standard", "other"} {
		selected, err := p.CopyRoomCrewJob("cargo_lab", theme, "cargo_basic", "standard", 0)
		check(err)
		if selected < 1 {
			t.Fatal("copy replaced the destination roster")
		}
	}
	copied, err := p.CrewJobs(target)
	check(err)
	original, err := p.CrewJobs(source)
	check(err)
	if len(copied) != 3 || copied[0].Slots != 1 || copied[1].Slots != 3 || copied[2].Slots != 5 || copied[1].Name != "Technician (copy)" || copied[2].Name != "Technician (copy 2)" {
		t.Fatalf("copy lost slots, replaced jobs, or duplicated names: %+v", copied)
	}
	for _, copy := range copied[1:] {
		if copy.ID == original[0].ID || copy.Outfit == original[0].Outfit || !copy.Officer || copy.Category != "Engineering" ||
			!reflect.DeepEqual(copy.Equipment, original[0].Equipment) || !reflect.DeepEqual(copy.Backpack, original[0].Backpack) ||
			!reflect.DeepEqual(copy.Belt, original[0].Belt) || !reflect.DeepEqual(copy.Extra, original[0].Extra) {
			t.Fatal("module copy lost equipment or retained a shared identity")
		}
	}
	copied[1].Equipment["head"], copied[1].Backpack["/obj/item/test"], copied[1].Belt["/obj/item/test"], copied[1].Extra["custom"] = "/obj/item/test", 9, 8, "FALSE"
	check(p.SetCrewJobs(target, copied))
	if unchanged, err := p.CrewJobs(source); err != nil || !reflect.DeepEqual(unchanged, original) {
		t.Fatal("editing the copy changed its source module", err)
	}
	after := p.Capture()
	p.Restore(before)
	if jobs, err := p.CrewJobs(target); err != nil || len(jobs) != 1 {
		t.Fatal("undo failed to remove only the copied module jobs", err)
	}
	p.Restore(after)
	data, err := p.CaptureRecovery()
	check(err)
	recovered, err := RecoverProject(p.Catalog, p.Dme, data)
	check(err)
	if got, err := recovered.CrewJobs(target); err != nil || !reflect.DeepEqual(got, copied) {
		t.Fatal("recovery lost module copies", err)
	}
	check(p.Save())
	// Filter both disabled slots and options unavailable in a source theme.
	p.Hull.Modules[1].Themes = []string{"other"}
	p.Hull.Themes[1].Slots = []string{}
	for _, args := range []struct {
		module, theme, target, targetTheme string
		index                              int
	}{
		{"cargo_lab", "standard", "cargo_basic", "standard", 0},
		{"cargo_lab", "other", "cargo_basic", "standard", 0},
		{"missing", "standard", "cargo_basic", "standard", 0},
		{"cargo_basic", "standard", "cargo_basic", "standard", 0},
		{"cargo_basic", "standard", "missing", "standard", 0},
		{"cargo_basic", "standard", "cargo_lab", "missing", 0},
		{"cargo_basic", "standard", "cargo_lab", "other", 0},
	} {
		unchanged := cloneCrew(p.Crew)
		if _, err := p.CopyRoomCrewJob(args.module, args.theme, args.target, args.targetTheme, args.index); err == nil || !reflect.DeepEqual(unchanged, p.Crew) {
			t.Fatalf("invalid source or destination changed crew: %+v, %v", args, err)
		}
	}
	p.Hull.Modules[1].Themes = []string{"standard", "other"}
	p.Hull.Themes[1].Slots = []string{"cargo"}
	for _, index := range []int{-1, 1} {
		unchanged := cloneCrew(p.Crew)
		if _, err := p.CopyRoomCrewJob("cargo_lab", "standard", "cargo_basic", "standard", index); err == nil || !reflect.DeepEqual(unchanged, p.Crew) {
			t.Fatal("invalid source job changed crew")
		}
	}
}

func TestCopyModuleCrewWithoutThemes(t *testing.T) {
	p, _, _ := variantShipProject(t)
	p.Hull.Themes = nil
	for i := range p.Hull.Modules {
		p.Hull.Modules[i].Themes = nil
	}
	job := CrewJob{Name: "Lab technician", Slots: 2, Category: "Science", Outfit: "/datum/outfit/job/assistant"}
	if err := p.SetCrewJobs("module/cargo_lab", []CrewJob{job}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CopyRoomCrewJob("cargo_lab", "", "cargo_basic", "", 0); err != nil {
		t.Fatal(err)
	}
	jobs, err := p.CrewJobs("module/cargo_basic")
	if err != nil || len(jobs) != 1 || jobs[0].Slots != 2 || jobs[0].Name != job.Name {
		t.Fatal("copying modules without themes failed", jobs, err)
	}
}
