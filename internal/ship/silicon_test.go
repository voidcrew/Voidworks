package ship

import (
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmvars"
)

func siliconProject(t *testing.T) *Project {
	t.Helper()
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "silicon", "Silicon", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	crewTestTypes(p)
	enableRoomCrewVariants(p)
	for _, path := range []string{"/datum/job", "/obj/item/robot_model/engineering", AICore} {
		v := dmvars.MutableVariables{}
		v.Put("ship_role", `"crew"`)
		v.Put("active", "TRUE")
		v.Put("available", "TRUE")
		env.Objects[path] = &dmenv.Object{Path: path, Vars: v.ToImmutable()}
	}
	if err := p.Deck(p.Hull.Themes[0], pt(3, 3), pt(16, 16)); err != nil {
		t.Fatal(err)
	}
	return p
}

func aiJob() []CrewJob {
	return []CrewJob{{Name: "Ship AI", Role: "ai", Category: "Silicon", Slots: 1}}
}

func TestSiliconCrewRoundTripAndCoreRemoval(t *testing.T) {
	p := siliconProject(t)
	jobs := append(aiJob(), CrewJob{Name: "Repair borg", Role: "cyborg", BorgModel: "/obj/item/robot_model/engineering", Category: "Silicon", Slots: 2})
	if err := p.SetCrewJobs("ship", jobs); err == nil {
		t.Fatal("AI without a mapped core accepted")
	}
	file, _ := p.Catalog.HullFile(p.Hull, p.Hull.Themes[0])
	tile := p.Documents[file].Map.GetTile(pt(4, 4))
	tile.InstancesAdd(dmmap.PrefabStorage.Initial(AICore))
	if err := p.SetCrewJobs("ship", jobs); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	encoded := renderCrew(jobs, p)
	if strings.Contains(encoded, "outfit") {
		t.Fatal("silicon saved a human outfit", encoded)
	}
	parsed, err := parseCrew(encoded)
	if err != nil || parsed[1].BorgModel != jobs[1].BorgModel || parsed[0].Role != "ai" {
		t.Fatalf("lost silicon fields: %+v %v", parsed, err)
	}
	reopened, err := OpenProject(p.Catalog, p.Dme, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.CrewJobs("ship")
	if err != nil || got[0].Role != "ai" || got[1].BorgModel != jobs[1].BorgModel {
		t.Fatalf("reopen lost roles: %+v %v", got, err)
	}
	before := p.Capture()
	instances := tile.Instances()
	for _, inst := range instances {
		if inst.Prefab().Path() == AICore {
			tile.InstancesRemoveByInstance(inst)
		}
	}
	if _, err := p.Changes(); err == nil {
		t.Fatal("save accepted removal of required core")
	}
	p.Restore(before)
	if _, err := p.Changes(); err != nil {
		t.Fatal("undo did not restore valid core", err)
	}
}

func TestSiliconModuleCoreAndThemeValidation(t *testing.T) {
	p := siliconProject(t)
	if err := p.addRect(0, "robotics", "Robotics", pt(5, 5), pt(7, 7)); err != nil {
		t.Fatal(err)
	}
	module := p.Hull.Modules[0]
	scope := p.RoomCrewScope(module.ID, p.Hull.Themes[0].ID)
	if err := p.SetCrewJobs(scope, aiJob()); err == nil {
		t.Fatal("module AI without core accepted")
	}
	file, err := p.moduleFile(module, p.Hull.Themes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	p.Documents[file].Map.GetTile(pt(1, 1)).InstancesAdd(dmmap.PrefabStorage.Initial(AICore))
	if err := p.SetCrewJobs(scope, aiJob()); err != nil {
		t.Fatal(err)
	}
	if err := p.SetCrewJobs("ship", aiJob()); err == nil {
		t.Fatal("optional module core was used for hull AI")
	}
	if err := p.AddTheme(0, "other", "Other", false); err != nil {
		t.Fatal(err)
	}
	jobs := aiJob()
	jobs[0].Slots = 2
	if err := p.SetCrewJobs(p.RoomCrewScope(module.ID, "other"), jobs); err == nil {
		t.Fatal("two AIs sharing one core accepted")
	}
	if err := p.SetCrewJobs(scope, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateSiliconCrew("", nil); err != nil {
		t.Fatal(err)
	}
}

func TestSiliconCrewRejectsBadRolesAndOldProjects(t *testing.T) {
	p := siliconProject(t)
	job := CrewJob{Name: "Borg", Role: "cyborg", Category: "Silicon", Slots: 1, BorgModel: "/obj/item/robot_model/engineering"}
	if err := p.ValidateCrew([]CrewJob{job}); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*CrewJob){
		func(j *CrewJob) { j.BorgModel = "/obj/item/test" },
		func(j *CrewJob) { j.Role = "typo" },
		func(j *CrewJob) { j.Officer = true },
		func(j *CrewJob) { j.Outfit = "/datum/outfit/job/assistant" },
	} {
		bad := job
		edit(&bad)
		if p.ValidateCrew([]CrewJob{bad}) == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	delete(p.Dme.Objects, "/datum/job")
	if p.ValidateCrew([]CrewJob{job}) == nil {
		t.Fatal("old game accepted unsupported roles")
	}
}

func TestOptionalModulesCannotShareOneHullCore(t *testing.T) {
	p := siliconProject(t)
	for i, id := range []string{"robotics", "computer"} {
		x := 5 + i*5
		if err := p.addRect(0, id, id, pt(x, 5), pt(x+2, 7)); err != nil {
			t.Fatal(err)
		}
	}
	file, _ := p.Catalog.HullFile(p.Hull, p.Hull.Themes[0])
	p.Documents[file].Map.GetTile(pt(4, 4)).InstancesAdd(dmmap.PrefabStorage.Initial(AICore))
	if err := p.SetCrewJobs(p.RoomCrewScope(p.Hull.Modules[0].ID, "standard"), aiJob()); err != nil {
		t.Fatal(err)
	}
	if err := p.SetCrewJobs(p.RoomCrewScope(p.Hull.Modules[1].ID, "standard"), aiJob()); err == nil {
		t.Fatal("two independent modules promised the same hull core")
	}
	p.Documents[file].Map.GetTile(pt(4, 5)).InstancesAdd(dmmap.PrefabStorage.Initial(AICore))
	if err := p.SetCrewJobs(p.RoomCrewScope(p.Hull.Modules[1].ID, "standard"), aiJob()); err != nil {
		t.Fatal(err)
	}
}
