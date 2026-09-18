package ship

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/third_party/sdmmparser"
)

const variantHullSource = `/datum/map_template/shuttle/voidcrew/vship
	name = "Variant Ship"
	suffix = "vship_a"
	has_upgrade_slots = TRUE
	upgrade_slot_ids = list("cargo")
	available_themes = list("standard", "other")
	part_requirements = list(PART_CLASS_MISC = 10)

/obj/docking_port/mobile/voidcrew/vship
	name = "Variant Ship"
	area_type = /area/shuttle/voidcrew/vship

/area/shuttle/voidcrew/vship
	name = "Variant Ship"
`

const variantCrew = `list(list(name = "Captain", officer = TRUE, outfit = /datum/outfit/job/captain, category = JOB_CAT_COMMAND, slots = 1))`

const variantRoomSource = `// Variant Ship rooms. One shared list covers every option.
/datum/ship_upgrade_module/vship
	for_ship = /datum/map_template/shuttle/voidcrew/vship
	for_theme = list("standard", "other")

/datum/ship_upgrade_module/vship/cargo_basic
	id = "cargo_basic"
	name = "Cargo Bay"
	slot = "cargo"
	map_file = "vship/cargo_basic.dmm"
	is_default = TRUE

/datum/ship_upgrade_module/vship/cargo_lab
	id = "cargo_lab"
	name = "Cargo Lab"
	slot = "cargo"
	map_file = "vship/cargo_lab.dmm"
	part_cost = list(PART_CLASS_SCIENCE = 4)

/datum/ship_theme/vship
	for_ship = /datum/map_template/shuttle/voidcrew/vship

/datum/ship_theme/vship/standard
	id = "standard"
	name = "Standard"
	template_suffix = "vship_a"
	is_default = TRUE
	upgrade_slot_ids = list("cargo")
	job_slots = ` + variantCrew + `

/datum/ship_theme/vship/other
	id = "other"
	name = "Other Variant"
	template_suffix = "vship_b"
	upgrade_slot_ids = list("cargo")
	part_cost = list(PART_CLASS_COMBAT = 6)
	job_slots = ` + variantCrew + `

/datum/ship_theme/vship/other/custom_proc()
	return 17
`

func variantObject(env *dmenv.Dme, path, file string, values map[string]string) {
	v := dmvars.MutableVariables{}
	for key, value := range values {
		v.Put(key, value)
	}
	env.Objects[path] = &dmenv.Object{Path: path, Vars: v.ToImmutable(), Location: sdmmparser.Location{File: file}}
}

// variantShipProject mirrors a handwritten fleet ship: lettered hull maps,
// shared room maps and one inherited for_theme list on the module family.
func variantShipProject(t *testing.T) (*Project, string, string) {
	t.Helper()
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "vship", "Variant Ship", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	crewTestTypes(p)
	if err = p.Deck(p.Hull.Themes[0], pt(3, 3), pt(16, 16)); err != nil {
		t.Fatal(err)
	}
	if err = p.addRect(0, "cargo", "Cargo Bay", pt(5, 5), pt(7, 7)); err != nil {
		t.Fatal(err)
	}
	if err = p.AddModule(0, p.Hull.Modules[0], "cargo_lab", "Cargo Lab", true); err != nil {
		t.Fatal(err)
	}
	paths := p.outputPaths()
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(paths[0]); err != nil {
		t.Fatal(err)
	}
	rooms := filepath.Join(c.Root, filepath.FromSlash(c.ModuleDir), "vship")
	for _, id := range []string{"cargo_basic", "cargo_lab"} {
		if err = os.Rename(filepath.Join(rooms, id+"_standard.dmm"), filepath.Join(rooms, id+".dmm")); err != nil {
			t.Fatal(err)
		}
	}
	ships := filepath.Join(c.Root, "_maps", "voidcrew", "ships")
	if err = os.Rename(filepath.Join(ships, "ship_vship.dmm"), filepath.Join(ships, "ship_vship_a.dmm")); err != nil {
		t.Fatal(err)
	}
	hullMap, err := os.ReadFile(filepath.Join(ships, "ship_vship_a.dmm"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(ships, "ship_vship_b.dmm"), hullMap, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(paths[1], []byte(variantHullSource), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(paths[2], []byte(variantRoomSource), 0600); err != nil {
		t.Fatal(err)
	}
	shipType := HullType + "/vship"
	variantObject(env, shipType, paths[1], map[string]string{"name": `"Variant Ship"`, "suffix": `"vship_a"`, "has_upgrade_slots": "TRUE", "upgrade_slot_ids": `list("cargo")`, "available_themes": `list("standard", "other")`, "part_requirements": "list(PART_CLASS_MISC = 10)"})
	variantObject(env, "/datum/ship_upgrade_module/vship", paths[2], map[string]string{"for_ship": shipType, "for_theme": `list("standard", "other")`})
	variantObject(env, "/datum/ship_theme/vship", paths[2], map[string]string{"for_ship": shipType})
	modules := []Module{
		{ID: "cargo_basic", Name: "Cargo Bay", Slot: "cargo", File: "vship/cargo_basic.dmm", Themes: []string{"standard", "other"}, Default: true},
		{ID: "cargo_lab", Name: "Cargo Lab", Slot: "cargo", File: "vship/cargo_lab.dmm", Themes: []string{"standard", "other"}},
	}
	for _, m := range modules {
		values := map[string]string{"id": dmQuote(m.ID), "name": dmQuote(m.Name), "slot": dmQuote(m.Slot), "map_file": dmQuote(m.File), "for_ship": shipType, "for_theme": `list("standard", "other")`}
		if m.Default {
			values["is_default"] = "TRUE"
		}
		if m.ID == "cargo_lab" {
			values["part_cost"] = "list(PART_CLASS_SCIENCE = 4)"
		}
		variantObject(env, "/datum/ship_upgrade_module/vship/"+m.ID, paths[2], values)
	}
	themes := []Theme{
		{ID: "standard", Name: "Standard", Suffix: "vship_a", Slots: []string{"cargo"}, Default: true},
		{ID: "other", Name: "Other Variant", Suffix: "vship_b", Slots: []string{"cargo"}},
	}
	for _, theme := range themes {
		values := map[string]string{"id": dmQuote(theme.ID), "name": dmQuote(theme.Name), "template_suffix": dmQuote(theme.Suffix), "for_ship": shipType, "upgrade_slot_ids": `list("cargo")`, "job_slots": variantCrew}
		if theme.Default {
			values["is_default"] = "TRUE"
		}
		if theme.ID == "other" {
			values["part_cost"] = "list(PART_CLASS_COMBAT = 6)"
		}
		variantObject(env, "/datum/ship_theme/vship/"+theme.ID, paths[2], values)
	}
	h := Hull{Type: shipType, Name: "Variant Ship", Prefix: "_maps/voidcrew/ships/", Port: "ship", Suffix: "vship_a", Slots: []string{"cargo"}, Themes: themes, Modules: modules}
	c.Hulls = []Hull{h}
	p, err = OpenProject(c, env, h)
	if err != nil {
		t.Fatal(err)
	}
	crewTestTypes(p)
	if p.Settings != nil || p.Modified() {
		t.Fatal("handwritten variant ship was converted or marked dirty")
	}
	return p, paths[2], paths[1]
}

func mapPath(t *testing.T, p *Project, relative string) string {
	t.Helper()
	file, err := Inside(p.Catalog.Root, relative)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestAddVariantSharesRoomsOnHandwrittenShip(t *testing.T) {
	p, roomSource, hullSource := variantShipProject(t)
	rooms, _ := os.ReadFile(roomSource)
	hull, _ := os.ReadFile(hullSource)
	before := p.Capture()
	if err := p.AddTheme(1, "arctic", "Arctic", false); err != nil {
		t.Fatal(err)
	}
	theme := p.Hull.Themes[len(p.Hull.Themes)-1]
	if theme.Suffix != "vship_c" {
		t.Fatalf("new variant did not follow the ship's hull naming: %s", theme.Suffix)
	}
	if theme.Default {
		t.Fatal("new variant took over as the default")
	}
	for _, m := range p.Hull.Modules {
		if !m.Available("arctic") {
			t.Fatalf("%s is not offered in the new variant", m.Name)
		}
		if forked, _, err := p.ModuleThemeStatus(m.ID, "arctic"); err != nil || forked {
			t.Fatalf("%s was copied instead of shared: %v", m.Name, err)
		}
	}
	after := p.Capture()
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_vship_c.dmm")) {
		t.Fatal("the variant's hull map was not written")
	}
	if fileExists(t, mapPath(t, p, "_maps/voidcrew/ship_modules/vship/cargo_basic_arctic.dmm")) {
		t.Fatal("a shared room was copied for the new variant")
	}
	saved, _ := os.ReadFile(roomSource)
	text := string(saved)
	expected := "\n/datum/ship_theme/vship/arctic\n\tid = \"arctic\"\n\tname = \"Arctic\"\n\tfor_ship = /datum/map_template/shuttle/voidcrew/vship\n\ttemplate_suffix = \"vship_c\"\n\tis_default = FALSE\n\tupgrade_slot_ids = list(\"cargo\")\n\tjob_slots = " + variantCrew + "\n"
	if !strings.Contains(text, expected) {
		t.Fatalf("variant registration is not as expected:\n%s", text)
	}
	if strings.Count(text, "for_theme") != 1 || !strings.Contains(text, "\tfor_theme = list(\"standard\", \"other\", \"arctic\")\n") {
		t.Fatalf("the shared availability list was not extended once:\n%s", text)
	}
	if !strings.Contains(text, "custom_proc()") || !strings.Contains(text, "part_cost = list(PART_CLASS_COMBAT = 6)") {
		t.Fatal("handwritten code around the variants was lost")
	}
	savedHull, _ := os.ReadFile(hullSource)
	if !bytes.Contains(savedHull, []byte(`available_themes = list("standard", "other", "arctic")`)) {
		t.Fatalf("the hull does not offer the new variant:\n%s", savedHull)
	}
	if p.Modified() {
		t.Fatal("saved ship remains dirty")
	}
	p.Restore(before)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if restored, _ := os.ReadFile(roomSource); !bytes.Equal(restored, rooms) {
		t.Fatalf("undo did not restore the room source:\n%s", restored)
	}
	if restored, _ := os.ReadFile(hullSource); !bytes.Equal(restored, hull) {
		t.Fatal("undo did not restore the hull source")
	}
	if fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_vship_c.dmm")) {
		t.Fatal("undo left the variant's hull map behind")
	}
	p.Restore(after)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_vship_c.dmm")) {
		t.Fatal("redo did not restore the variant's hull map")
	}
}

func TestAddVariantCopiesRoomsWhenAsked(t *testing.T) {
	p, _, _ := variantShipProject(t)
	if err := p.AddTheme(0, "arctic", "Arctic", true); err != nil {
		t.Fatal(err)
	}
	for _, m := range p.Hull.Modules {
		forked, same, err := p.ModuleThemeStatus(m.ID, "arctic")
		if err != nil || !forked || !same {
			t.Fatalf("%s was not copied from the shared room: %v %v %v", m.Name, forked, same, err)
		}
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cargo_basic", "cargo_lab"} {
		if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ship_modules/vship/"+id+"_arctic.dmm")) {
			t.Fatalf("%s has no copy for the new variant", id)
		}
	}
}

func TestRemoveVariantOnHandwrittenShip(t *testing.T) {
	p, roomSource, hullSource := variantShipProject(t)
	rooms, _ := os.ReadFile(roomSource)
	if err := p.RemoveTheme("standard"); err == nil || !strings.Contains(err.Error(), "default theme") {
		t.Fatalf("removed the default variant: %v", err)
	}
	if err := p.ForkModuleForTheme("cargo_lab", "other"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPartCosts("theme/other", PartCosts{"combat": 9}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	copied := mapPath(t, p, "_maps/voidcrew/ship_modules/vship/cargo_lab_other.dmm")
	if !fileExists(t, copied) {
		t.Fatal("the variant's own room was not written")
	}
	rooms = mustRead(t, roomSource)
	before := p.Capture()
	if err := p.RemoveTheme("other"); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.partCosts["theme/other"]; ok {
		t.Fatal("the removed variant kept its price")
	}
	if err := p.RemoveTheme("standard"); err == nil || !strings.Contains(err.Error(), "only theme") {
		t.Fatalf("removed the last variant: %v", err)
	}
	after := p.Capture()
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, copied) {
		t.Fatal("the variant's own room survived")
	}
	if fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_vship_b.dmm")) {
		t.Fatal("the variant's hull map survived")
	}
	if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ship_modules/vship/cargo_lab.dmm")) {
		t.Fatal("the shared room was deleted with the variant")
	}
	text, _ := os.ReadFile(roomSource)
	if bytes.Contains(text, []byte("/datum/ship_theme/vship/other\n")) || bytes.Contains(text, []byte("custom_proc")) {
		t.Fatalf("the variant's definition survived:\n%s", text)
	}
	if !bytes.Contains(text, []byte("\tfor_theme = list(\"standard\")\n")) || bytes.Count(text, []byte("for_theme")) != 1 {
		t.Fatalf("availability was not narrowed once:\n%s", text)
	}
	if !bytes.Contains(text, []byte("/datum/ship_theme/vship/standard\n")) {
		t.Fatal("the surviving variant was removed too")
	}
	hull, _ := os.ReadFile(hullSource)
	if !bytes.Contains(hull, []byte(`available_themes = list("standard")`)) {
		t.Fatalf("the hull still offers the removed variant:\n%s", hull)
	}
	p.Restore(before)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if restored, _ := os.ReadFile(roomSource); !bytes.Equal(restored, rooms) {
		t.Fatalf("undo did not restore the variant definition:\n%s", restored)
	}
	if !fileExists(t, copied) || !fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_vship_b.dmm")) {
		t.Fatal("undo did not restore the variant's maps")
	}
	p.Restore(after)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, copied) {
		t.Fatal("redo did not remove the variant again")
	}
}

func TestSetDefaultVariantOnHandwrittenShip(t *testing.T) {
	p, roomSource, _ := variantShipProject(t)
	if err := p.SetDefaultTheme("missing"); err == nil {
		t.Fatal("accepted an unknown variant")
	}
	if err := p.SetDefaultTheme("other"); err != nil {
		t.Fatal(err)
	}
	defaults := 0
	for _, theme := range p.Hull.Themes {
		if theme.Default {
			defaults++
		}
	}
	if defaults != 1 || !p.Hull.Themes[1].Default {
		t.Fatal("the ship does not have exactly one default variant")
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(roomSource)
	if !bytes.Contains(text, []byte("\ttemplate_suffix = \"vship_a\"\n\tis_default = FALSE\n")) {
		t.Fatalf("the old default was not cleared:\n%s", text)
	}
	if !bytes.Contains(text, []byte("/datum/ship_theme/vship/other\n\tis_default = TRUE\n")) {
		t.Fatalf("the new default was not written:\n%s", text)
	}
}

func TestSetVariantRoomsOnHandwrittenShip(t *testing.T) {
	p, roomSource, _ := variantShipProject(t)
	if err := p.SetThemeSlots("other", []string{"galley"}); err == nil {
		t.Fatal("accepted a room the ship does not have")
	}
	if err := p.SetThemeSlots("other", []string{"cargo", "cargo"}); err == nil {
		t.Fatal("accepted a duplicated room")
	}
	if err := p.SetThemeSlots("other", nil); err != nil {
		t.Fatal(err)
	}
	if p.Hull.Themes[1].Slots != nil || len(p.Hull.SlotsFor(p.Hull.Themes[1])) != 1 {
		t.Fatal("the variant did not fall back to the hull's rooms")
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(roomSource)
	if bytes.Count(text, []byte("upgrade_slot_ids")) != 1 {
		t.Fatalf("the variant's own room list was not dropped:\n%s", text)
	}
	if err := p.SetThemeSlots("other", []string{"cargo"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(roomSource)
	if bytes.Count(text, []byte("upgrade_slot_ids")) != 2 {
		t.Fatalf("the variant's room list did not come back:\n%s", text)
	}
}

func TestSetOptionVariantsOnHandwrittenShip(t *testing.T) {
	p, roomSource, _ := variantShipProject(t)
	if err := p.SetModuleThemes("cargo_lab", []string{"nope"}); err == nil {
		t.Fatal("accepted an unknown variant")
	}
	if err := p.SetModuleThemes("cargo_basic", []string{"standard"}); err == nil || !strings.Contains(err.Error(), "default option") {
		t.Fatalf("dropped the default option from a variant: %v", err)
	}
	if err := p.SetModuleThemes("cargo_lab", []string{"standard"}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetModuleThemes("cargo_basic", []string{"standard"}); err == nil || !strings.Contains(err.Error(), "no option") {
		t.Fatalf("left a variant with no option for a room: %v", err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, roomSource))
	if !strings.Contains(text, "/datum/ship_upgrade_module/vship\n\tfor_ship = /datum/map_template/shuttle/voidcrew/vship\n\tfor_theme = list(\"standard\", \"other\")\n") {
		t.Fatalf("the shared availability list changed:\n%s", text)
	}
	if !strings.Contains(text, "/datum/ship_upgrade_module/vship/cargo_lab\n\tfor_theme = list(\"standard\")\n") {
		t.Fatalf("the option did not override its availability:\n%s", text)
	}
	if !strings.Contains(text, "part_cost = list(PART_CLASS_SCIENCE = 4)") {
		t.Fatal("the option lost its price")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestForkAndUnforkRoomOption(t *testing.T) {
	p, _, _ := variantShipProject(t)
	copied := mapPath(t, p, "_maps/voidcrew/ship_modules/vship/cargo_basic_other.dmm")
	if _, _, err := p.ModuleThemeStatus("cargo_basic", "missing"); err == nil {
		t.Fatal("accepted an unknown variant")
	}
	if err := p.UnforkModuleForTheme("cargo_basic", "other"); err == nil {
		t.Fatal("unforked a room that was already shared")
	}
	if err := p.ForkModuleForTheme("cargo_basic", "other"); err != nil {
		t.Fatal(err)
	}
	forked, same, err := p.ModuleThemeStatus("cargo_basic", "other")
	if err != nil || !forked || !same {
		t.Fatalf("a fresh copy does not match the shared room: %v %v %v", forked, same, err)
	}
	if err = p.ForkModuleForTheme("cargo_basic", "other"); err == nil {
		t.Fatal("forked twice")
	}
	file, err := p.ModuleSource(p.Hull.Modules[0], "other")
	if err != nil || file != copied {
		t.Fatalf("the variant does not use its own copy: %s %v", file, err)
	}
	d, err := p.document(copied)
	if err != nil {
		t.Fatal(err)
	}
	d.Map.GetTile(pt(2, 2)).InstancesAdd(dmmap.PrefabStorage.Initial("/obj/item/test"))
	if _, same, err = p.ModuleThemeStatus("cargo_basic", "other"); err != nil || same {
		t.Fatalf("an edited copy still reads as identical: %v", err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, copied) {
		t.Fatal("the copy was not written")
	}
	before := p.Capture()
	if err = p.UnforkModuleForTheme("cargo_basic", "other"); err != nil {
		t.Fatal(err)
	}
	if forked, _, err = p.ModuleThemeStatus("cargo_basic", "other"); err != nil || forked {
		t.Fatalf("the variant still keeps its own copy: %v", err)
	}
	if file, err = p.ModuleSource(p.Hull.Modules[0], "other"); err != nil || file != mapPath(t, p, "_maps/voidcrew/ship_modules/vship/cargo_basic.dmm") {
		t.Fatalf("the variant did not fall back to the shared room: %s %v", file, err)
	}
	if !p.Modified() {
		t.Fatal("dropping a copy is not marked dirty")
	}
	after := p.Capture()
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, copied) {
		t.Fatal("the copy was not deleted")
	}
	p.Restore(before)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, copied) {
		t.Fatal("undo did not restore the copy")
	}
	p.Restore(after)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if fileExists(t, copied) {
		t.Fatal("redo did not remove the copy")
	}
}

// Scarab-style options exist only as per-variant maps, with nothing to share.
func TestUnforkNeedsASharedRoom(t *testing.T) {
	p, _, _ := variantShipProject(t)
	data := mustRead(t, mapPath(t, p, "_maps/voidcrew/ship_modules/vship/cargo_lab.dmm"))
	if err := os.WriteFile(mapPath(t, p, "_maps/voidcrew/ship_modules/vship/cargo_themed_other.dmm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	p.Hull.Modules = append(p.Hull.Modules, Module{ID: "cargo_themed", Name: "Themed Cargo", Slot: "cargo", File: "vship/cargo_themed.dmm", Themes: []string{"standard", "other"}})
	if forked, same, err := p.ModuleThemeStatus("cargo_themed", "other"); err != nil || !forked || same {
		t.Fatalf("a variant-only room does not read as its own copy: %v %v %v", forked, same, err)
	}
	if err := p.UnforkModuleForTheme("cargo_themed", "other"); err == nil || !strings.Contains(err.Error(), "shared module") {
		t.Fatalf("dropped the only copy of a room: %v", err)
	}
}

func TestVariantSummary(t *testing.T) {
	p, _, _ := variantShipProject(t)
	if err := p.ForkModuleForTheme("cargo_lab", "other"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetThemeSlots("other", nil); err != nil {
		t.Fatal(err)
	}
	info, err := p.ThemeSummary("other")
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "other" || info.Name != "Other Variant" || info.Default {
		t.Fatalf("wrong variant: %+v", info)
	}
	if info.HullFile != "_maps/voidcrew/ships/ship_vship_b.dmm" {
		t.Fatalf("wrong hull map: %s", info.HullFile)
	}
	if !info.Inherited || len(info.Slots) != 1 || info.Slots[0] != "cargo" {
		t.Fatalf("wrong rooms: %+v", info)
	}
	if info.Price != "6 Combat" || info.Crew != 1 {
		t.Fatalf("wrong price or crew: %q %d", info.Price, info.Crew)
	}
	if len(info.Options) != 2 {
		t.Fatalf("wrong options: %+v", info.Options)
	}
	if !info.Options[0].Available || info.Options[0].Forked || info.Options[0].Slot != "cargo" {
		t.Fatalf("the shared option is wrong: %+v", info.Options[0])
	}
	if !info.Options[1].Forked || info.Options[1].Differs {
		t.Fatalf("the copied option is wrong: %+v", info.Options[1])
	}
}

func TestVariantDetailsOnHandwrittenShip(t *testing.T) {
	p, roomSource, _ := variantShipProject(t)
	if err := p.Rename("theme/other", "Arctic Variant"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetDescription("theme/other", "Cold weather fit-out."); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPartCosts("theme/other", PartCosts{"science": 7}); err != nil {
		t.Fatal(err)
	}
	jobs, err := p.CrewJobs("theme/other")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("the variant's crew was not read: %+v", jobs)
	}
	jobs = append(jobs, CrewJob{Name: "Deckhand", Outfit: "/datum/outfit/job/assistant", Category: "Assistant", Slots: 2})
	if err = p.SetCrewJobs("theme/other", jobs); err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, roomSource))
	for _, want := range []string{`name = "Arctic Variant"`, `desc = "Cold weather fit-out."`, "part_cost = list(PART_CLASS_SCIENCE = 7)", `template_suffix = "vship_arctic_variant"`, `name = "Deckhand"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in:\n%s", want, text)
		}
	}
	if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_vship_arctic_variant.dmm")) {
		t.Fatal("the variant's hull map did not follow its name")
	}
	if fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_vship_b.dmm")) {
		t.Fatal("the old hull map survived the rename")
	}
	if p.Modified() {
		t.Fatal("saved ship remains dirty")
	}
}

func TestVariantOpsOnAuthoredShip(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "authored", "Authored Ship", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Deck(p.Hull.Themes[0], pt(3, 3), pt(16, 16)); err != nil {
		t.Fatal(err)
	}
	if err = p.addRect(0, "cargo", "Cargo Bay", pt(5, 5), pt(7, 7)); err != nil {
		t.Fatal(err)
	}
	if err = p.AddModule(0, p.Hull.Modules[0], "cargo_lab", "Cargo Lab", true); err != nil {
		t.Fatal(err)
	}
	before := p.Capture()
	if err = p.AddTheme(0, "arctic", "Arctic", false); err != nil {
		t.Fatal(err)
	}
	if err = p.SetDefaultTheme("arctic"); err != nil {
		t.Fatal(err)
	}
	if err = p.SetThemeSlots("arctic", nil); err != nil {
		t.Fatal(err)
	}
	if err = p.SetModuleThemes("cargo_lab", []string{"standard"}); err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, p.outputPaths()[2]))
	if !strings.Contains(text, `/datum/ship_theme/authored_arctic`) || !strings.Contains(text, "\tis_default = TRUE\n\tupgrade_slot_ids = list(\"cargo\")") {
		t.Fatalf("the new variant was not registered:\n%s", text)
	}
	if !strings.Contains(text, `for_theme = list("standard")`) {
		t.Fatalf("the option's availability was not written:\n%s", text)
	}
	hull := string(mustRead(t, p.outputPaths()[1]))
	if !strings.Contains(hull, `available_themes = list("standard", "arctic")`) {
		t.Fatalf("the hull does not offer the new variant:\n%s", hull)
	}
	if err = p.RemoveTheme("standard"); err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	text = string(mustRead(t, p.outputPaths()[2]))
	if strings.Contains(text, "/datum/ship_theme/authored_standard\n") {
		t.Fatalf("the removed variant is still registered:\n%s", text)
	}
	// That variant shared the hull's own map, which the hull still names.
	if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_authored.dmm")) {
		t.Fatal("the hull's own map was deleted with the variant")
	}
	if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_authored_arctic.dmm")) {
		t.Fatal("the surviving variant lost its hull map")
	}
	p.Restore(before)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if len(p.Hull.Themes) != 1 || p.Hull.Themes[0].ID != "standard" {
		t.Fatalf("undo did not restore the ship's variants: %+v", p.Hull.Themes)
	}
	if !fileExists(t, mapPath(t, p, "_maps/voidcrew/ships/ship_authored.dmm")) {
		t.Fatal("undo did not restore the original hull map")
	}
}

func TestFirstVariantAdoptsTheShipsOwnHull(t *testing.T) {
	p, roomSource, hullSource := variantShipProject(t)
	source := bytes.Replace(mustRead(t, hullSource), []byte("\tavailable_themes = list(\"standard\", \"other\")\n"), nil, 1)
	if err := os.WriteFile(hullSource, source, 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.track(hullSource); err != nil {
		t.Fatal(err)
	}
	p.Hull.Themes = nil
	p.Hull.Modules[0].Themes, p.Hull.Modules[1].Themes = nil, nil
	if err := p.AddTheme(0, "base", "Base Fitting", false); err != nil {
		t.Fatal(err)
	}
	theme := p.Hull.Themes[0]
	if theme.Suffix != p.Hull.Suffix || !theme.Default {
		t.Fatalf("the first variant did not adopt the ship: %+v", theme)
	}
	for _, m := range p.Hull.Modules {
		if !m.Available("base") {
			t.Fatalf("%s is not offered in the first variant", m.Name)
		}
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(mustRead(t, roomSource), []byte("\ttemplate_suffix = \"vship_a\"\n\tis_default = TRUE\n")) {
		t.Fatal("the first variant is not the default")
	}
	if !bytes.Contains(mustRead(t, hullSource), []byte(`available_themes = list("base")`)) {
		t.Fatal("the hull does not offer the first variant")
	}
}

func TestNextVariantSuffix(t *testing.T) {
	p, _, _ := variantShipProject(t)
	suffix, err := p.nextThemeSuffix("arctic")
	if err != nil || suffix != "vship_c" {
		t.Fatalf("lettered fleet naming: %s %v", suffix, err)
	}
	p.Hull.Themes[1].Suffix = "vship_other"
	if suffix, err = p.nextThemeSuffix("arctic"); err != nil || suffix != "vship_arctic" {
		t.Fatalf("mixed naming: %s %v", suffix, err)
	}
	p.Hull.Themes[1].Suffix = "vship_arctic"
	if suffix, err = p.nextThemeSuffix("arctic"); err != nil || suffix != "vship_arctic_2" {
		t.Fatalf("collision: %s %v", suffix, err)
	}
}
