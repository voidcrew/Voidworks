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
			index, err := p.CopyRoomCrewJob("cargo_basic", "other", "standard", 0)
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
			_, err = p.CopyRoomCrewJob("cargo_basic", "other", "standard", 0)
			check(err)
			_, err = p.CopyRoomCrewJob("cargo_basic", "third", "standard", 0)
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
