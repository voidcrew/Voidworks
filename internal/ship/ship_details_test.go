package ship

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/third_party/sdmmparser"
)

func loadedRenameProject(t *testing.T) *Project {
	t.Helper()
	p, source := loadedRoomProject(t, true)
	p.Dme.Objects[p.Hull.Type].Vars = dmvars.Set(p.Dme.Objects[p.Hull.Type].Vars, "short_name", dmQuote(p.Hull.Name))
	for _, module := range p.Hull.Modules {
		vars := dmvars.MutableVariables{}
		vars.Put("id", dmQuote(module.ID))
		vars.Put("for_ship", p.Hull.Type)
		typePath := "/datum/ship_upgrade_module/loaded_rooms_" + module.ID
		p.Dme.Objects[typePath] = &dmenv.Object{Path: typePath, Vars: vars.ToImmutable(), Location: sdmmparser.Location{File: source}}
	}
	return p
}

func TestLoadedShipDetailsPreserveSourceAndHistory(t *testing.T) {
	p := loadedRenameProject(t)
	file, _ := p.roomTypeFile(p.Hull.Type)
	original, _ := os.ReadFile(file)
	before := p.Capture()
	want := ShipDetails{Description: "A \"medical\" ship [special].\nSupplies \\ equipment.", Hidden: true}
	if err := p.SetShipDetails(want); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPartCosts("ship", PartCosts{"science": 7}); err != nil {
		t.Fatal(err)
	}
	after := p.Capture()
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(file)
	for _, text := range []string{"catalog_desc = " + dmQuote(want.Description), "player_hidden = TRUE", "job_slots = list(", "PART_CLASS_SCIENCE = 7"} {
		if !bytes.Contains(saved, []byte(text)) {
			t.Fatalf("missing %s", text)
		}
	}
	if p.Settings != nil || p.Modified() {
		t.Fatal("details converted or dirtied the saved project")
	}
	p.Restore(before)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(file); !bytes.Equal(data, original) {
		t.Fatal("undo lost original source")
	}
	p.Restore(after)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(file); !bytes.Equal(data, saved) {
		t.Fatal("redo differs")
	}
}

func TestShipVisibilityAfterReopeningWithLoadedEnvironment(t *testing.T) {
	p := loadedRenameProject(t)
	for _, hidden := range []bool{true, false, true} {
		if err := p.SetShipDetails(ShipDetails{Hidden: hidden}); err != nil {
			t.Fatal(err)
		}
		if err := p.Save(); err != nil {
			t.Fatal(err)
		}
		// Closing and reopening Ship Workshop retains the parsed environment.
		reopened, err := OpenProject(p.Catalog, p.Dme, p.Hull)
		if err != nil {
			t.Fatal(err)
		}
		if reopened.ShipDetails().Hidden != hidden {
			t.Fatalf("saved player_hidden=%v, but the reopened checkbox is %v", hidden, reopened.ShipDetails().Hidden)
		}
		if reopened.Modified() {
			t.Fatal("reading the saved visibility dirtied the ship")
		}
		p = reopened
	}
	if err := p.RenameShip("Renamed hidden ship"); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenProject(p.Catalog, p.Dme, p.Hull)
	if err != nil || !reopened.ShipDetails().Hidden {
		t.Fatal("renaming left visibility dependent on the old source location", err)
	}
}

func TestLoadedShipRenameFilesRecoveryAndHistory(t *testing.T) {
	p := loadedRenameProject(t)
	for _, theme := range p.Hull.Themes {
		if _, err := p.Assemble(theme, nil); err != nil {
			t.Fatal(err)
		}
	}
	original := map[string][]byte{}
	files, err := removalSources(p.Catalog.Root, p.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	for file, data := range files {
		original[file] = data
	}
	for file := range p.Documents {
		original[file], _ = os.ReadFile(file)
	}
	before := p.Capture()
	if err := p.RenameShip("New Explorer"); err != nil {
		t.Fatal(err)
	}
	if p.Hull.Type != before.Hull.Type || p.Settings != nil {
		t.Fatal("rename changed game identity or converted handwritten code")
	}
	after := p.Capture()
	draft, err := p.CaptureRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = RecoverProject(p.Catalog, p.Dme, draft); err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if p.Modified() {
		t.Fatal("saved rename is dirty")
	}
	for file := range original {
		if file == p.Dme.RootFile {
			continue
		}
		if _, err = os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("old owned file remains: %s: %v", file, err)
		}
	}
	newCode := filepath.Join(p.Catalog.Root, "voidcrew/modules/ship_upgrades/ships/new_explorer.dm")
	data, _ := os.ReadFile(newCode)
	if !bytes.Contains(data, []byte("custom_proc()")) {
		t.Fatal("rename lost handwritten code")
	}
	for _, module := range p.Hull.Modules {
		if !bytes.Contains(data, []byte(dmQuote(module.File))) {
			t.Fatalf("module map reference not renamed: %s", module.File)
		}
	}
	for _, theme := range p.Hull.Themes {
		if _, err := p.Assemble(theme, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Editing another field after saving must continue using the moved source.
	if err = p.SetShipDetails(ShipDetails{Description: "After rename"}); err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	p.Restore(before)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	for path, expected := range original {
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("undo did not restore %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(p.Catalog.Root, "voidcrew/mapping/ship_projects/new_explorer.shipinfo.json")); !os.IsNotExist(err) {
		t.Fatal("undo retained rename metadata")
	}
	p.Restore(after)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	// A recovered saved session still supports another rename and save.
	draft, err = p.CaptureRecovery()
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := RecoverProject(p.Catalog, p.Dme, draft)
	if err != nil {
		t.Fatal(err)
	}
	if err = recovered.RenameShip("Final Ship"); err != nil {
		t.Fatal(err)
	}
	if err = recovered.Save(); err != nil {
		t.Fatal(err)
	}
	includes, err := removalSources(p.Catalog.Root, p.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	for path := range includes {
		if strings.Contains(path, "new_explorer.dm") {
			t.Fatalf("intermediate include remains: %s", path)
		}
	}
}

func TestLoadedShipRenameCollisionAndExternalEdit(t *testing.T) {
	p := loadedRenameProject(t)
	target := filepath.Join(p.Catalog.Root, "voidcrew/mapping/shuttles/existing.dm")
	if err := os.WriteFile(target, []byte("another ship"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.RenameShip("Existing"); err == nil {
		t.Fatal("accepted source collision")
	}
	if p.Modified() {
		t.Fatal("failed rename changed project")
	}
	if err := p.RenameShip("Explorer"); err != nil {
		t.Fatal(err)
	}
	file, _ := p.roomTypeFile(p.Hull.Type)
	data, _ := os.ReadFile(file)
	data = append(data, []byte("\n// External edit\n")...)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err == nil {
		t.Fatal("overwrote external source edit")
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got, data) {
		t.Fatal("failed save damaged source")
	}
	if _, err := os.Stat(filepath.Join(p.Catalog.Root, "voidcrew/mapping/shuttles/explorer.dm")); !os.IsNotExist(err) {
		t.Fatal("failed preflight wrote partial rename")
	}
}

func TestLoadedRenameReparseAndHiddenDiscovery(t *testing.T) {
	p := loadedRenameProject(t)
	base := `#define TRUE 1
#define FALSE 0
#define PART_CLASS_MISC "misc"
#define JOB_CAT_COMMAND "Command"
#define JOB_CAT_ASSISTANT "Assistant"
/datum
	var/name
/datum/map_template/shuttle/voidcrew
	var/prefix = "_maps/voidcrew/ships/"
	var/port_id = "ship"
	var/suffix
	var/catalog_desc
	var/short_name
	var/player_hidden = 0
	var/has_upgrade_slots = 0
	var/upgrade_slot_ids
	var/available_themes
	var/job_slots
	var/part_requirements
/obj/docking_port/mobile/voidcrew
	var/area_type
	var/port_direction
	var/preferred_direction
/obj/modular_map_root/ship_upgrade
	var/config_file = "modules.toml"
/datum/ship_theme
	var/id
	var/for_ship
	var/template_suffix
	var/is_default = 0
	var/upgrade_slot_ids
	var/part_cost
/datum/ship_upgrade_module
	var/id
	var/for_ship
	var/for_theme
	var/slot
	var/map_file
	var/is_default = 0
	var/part_cost
`
	data, _ := os.ReadFile(p.Dme.RootFile)
	if err := os.WriteFile(p.Dme.RootFile, append([]byte(base), data...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Catalog.Root, "modules.toml"), []byte("directory = \"_maps/voidcrew/ship_modules/\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.RenameShip("Reparsed Ship"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetShipDetails(ShipDetails{Description: "Hidden but editable", Hidden: true}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	env, err := dmenv.New(p.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := p.CaptureRecovery()
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := RecoverProject(p.Catalog, env, draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.RenameShip("Recovered Name"); err != nil {
		t.Fatal("recovery with refreshed environment:", err)
	}
	catalog, err := Discover(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Hulls) != 1 {
		t.Fatalf("hidden ship disappeared: %+v", catalog.Hulls)
	}
	hull := catalog.Hulls[0]
	if hull.Name != "Reparsed Ship" || hull.Description != "Hidden but editable" || !hull.Hidden || hull.Type != p.Hull.Type {
		t.Fatalf("reparsed details differ: %+v", hull)
	}
	reopened, err := OpenProject(catalog, env, hull)
	if err != nil || reopened.Settings != nil || reopened.Modified() {
		t.Fatalf("reopen: %v", err)
	}
	for _, theme := range hull.Themes {
		if _, err := reopened.Assemble(theme, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := reopened.RenameShip("Second Name"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := dmenv.New(p.Dme.RootFile); err != nil {
		t.Fatal(err)
	}
}

func TestRenamePreservesNestedJobNames(t *testing.T) {
	before := []byte("/datum/ship\n\tname = \"Ship\" // label\n\tjob_slots = list(\n\t\tlist(\n\t\t\tname = \"Captain\",\n\t\t),\n\t\tlist(\n\t\t\tname = \"Engineer\",\n\t\t),\n\t)\n")
	after, err := rewriteName(before, "/datum/ship", "Ship", "Explorer")
	if err != nil || !bytes.Equal(after, bytes.Replace(before, []byte(`"Ship"`), []byte(`"Explorer"`), 1)) {
		t.Fatalf("changed nested job names: %v", err)
	}
}

func TestDetailsUndoWithCrewAndPricesRemovesMetadata(t *testing.T) {
	p := loadedRenameProject(t)
	p.Crew = &CrewConfig{Rosters: map[string][]CrewJob{}}
	for _, file := range func() []string { meta, code := p.crewPaths(); return []string{meta, code, p.Dme.RootFile} }() {
		if err := p.track(file); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.SetPartCosts("ship", PartCosts{"science": 9}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	before := p.Capture()
	if err := p.SetShipDetails(ShipDetails{Description: "Temporary", Hidden: true}); err != nil {
		t.Fatal(err)
	}
	if err := p.RenameShip("Temporary Name"); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	p.Restore(before)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	for _, stem := range []string{"loaded_rooms", "temporary_name"} {
		if _, err := os.Stat(filepath.Join(p.Catalog.Root, "voidcrew/mapping/ship_projects", stem+".shipinfo.json")); !os.IsNotExist(err) {
			t.Fatal("undo left an empty/obsolete project manifest", stem, err)
		}
	}
}

func TestLoadedRenameKeepsSharedSourcesAndMaps(t *testing.T) {
	p := loadedRenameProject(t)
	source, _ := p.roomTypeFile(p.Hull.Type)
	data, _ := os.ReadFile(source)
	neighbor := []byte("\n/datum/unrelated\n\tvar/message = \"Keep this\"\n")
	if err := os.WriteFile(source, append(data, neighbor...), 0600); err != nil {
		t.Fatal(err)
	}
	shared, _ := p.Catalog.HullFile(p.Hull, p.Hull.Themes[0])
	mapBefore, _ := os.ReadFile(shared)
	vars := dmvars.MutableVariables{}
	vars.Put("prefix", dmQuote(p.Hull.Prefix))
	vars.Put("port_id", dmQuote(p.Hull.Port))
	vars.Put("suffix", dmQuote(p.Hull.Themes[0].Suffix))
	other := HullType + "/hidden_neighbor"
	p.Dme.Objects[other] = &dmenv.Object{Path: other, Vars: vars.ToImmutable()}
	if err := p.RenameShip("Shared Explorer"); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(shared)
	if err != nil || !bytes.Equal(got, mapBefore) {
		t.Fatal("renaming deleted a hidden ship's shared map", err)
	}
	got, err = os.ReadFile(source)
	if err != nil || !bytes.Contains(got, neighbor) || !bytes.Contains(got, []byte(`name = "Shared Explorer"`)) {
		t.Fatal("shared definition file was moved or damaged", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(source), "shared_explorer.dm")); !os.IsNotExist(err) {
		t.Fatal("shared source file was renamed")
	}
}

func TestLoadedRenameRespectsOpenMapAndNestedIncludes(t *testing.T) {
	p := loadedRenameProject(t)
	file, _ := p.Catalog.HullFile(p.Hull, p.Hull.Themes[0])
	if _, err := p.document(file); err != nil {
		t.Fatal(err)
	}
	p.BeforeOpen = func(path string) error {
		if path == file {
			return fmt.Errorf("map is open elsewhere")
		}
		return nil
	}
	if err := p.RenameShip("Busy Ship"); err == nil || p.Modified() {
		t.Fatal("renamed an open map or changed state on failure", err)
	}
	p.BeforeOpen = nil
	// Move all includes into a shared nested include file. Keep slash style and
	// comments, and verify that includes still resolve after save and undo.
	data, _ := os.ReadFile(p.Dme.RootFile)
	nested := filepath.Join(p.Catalog.Root, "nested.dme")
	if err := os.WriteFile(nested, data, 0600); err != nil {
		t.Fatal(err)
	}
	root := []byte("#include \"nested.dme\" // Keep this comment\n")
	if err := os.WriteFile(p.Dme.RootFile, root, 0600); err != nil {
		t.Fatal(err)
	}
	// Begin a fresh session so these deliberate source changes are its baseline.
	q, err := OpenProject(p.Catalog, p.Dme, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	before := q.Capture()
	if err := q.RenameShip("Nested Ship"); err != nil {
		t.Fatal(err)
	}
	if err := q.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := removalSources(p.Catalog.Root, p.Dme.RootFile); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p.Dme.RootFile); !bytes.Equal(got, root) {
		t.Fatal("unrelated include/comment changed")
	}
	q.Restore(before)
	if err := q.Save(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(nested); !bytes.Equal(got, data) {
		t.Fatal("nested include undo differed")
	}
}

func TestLoadedRenameWithNewModulesAndFleetSave(t *testing.T) {
	p := loadedRenameProject(t)
	q, err := NewProject(p.Catalog, p.Dme, "neighbor", "Neighbor", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.RenameShip("Explorer"); err != nil {
		t.Fatal(err)
	}
	if err := p.AddModule(0, p.Hull.Modules[0], "extra", "Extra option", false); err != nil {
		t.Fatal(err)
	}
	if got := p.Hull.Modules[len(p.Hull.Modules)-1].File; !strings.HasPrefix(got, "explorer/") {
		t.Fatalf("new module used the old ship directory: %s", got)
	}
	if err := SaveProjects([]*Project{q, p}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetDescription("module/extra", "Added after rename"); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPartCosts("ship", PartCosts{"misc": 5}); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjects([]*Project{p, q}); err != nil {
		t.Fatal(err)
	}
	if p.Modified() || q.Modified() {
		t.Fatal("fleet save left a ship dirty")
	}
	includes, err := removalSources(p.Catalog.Root, p.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	for file := range includes {
		if strings.HasSuffix(file, "loaded_rooms.dm") {
			t.Fatal("fleet save restored an old include", file)
		}
	}
	code := filepath.Join(p.Catalog.Root, "voidcrew/modules/ship_upgrades/workshop/explorer.dm")
	if got, _ := os.ReadFile(code); !bytes.Contains(got, []byte(`desc = "Added after rename"`)) {
		t.Fatal("new module source was not moved or updated")
	}
}

// Read-only audit against the real fleet: no maps or game sources are written.
func TestFleetShipDetailsAndRename(t *testing.T) {
	file := os.Getenv("SHIP_RENDER_TEST_DME")
	if file == "" {
		t.Skip("set SHIP_RENDER_TEST_DME for fleet checks")
	}
	env, err := dmenv.New(file)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := Discover(env)
	if err != nil {
		t.Fatal(err)
	}
	dmmap.PrefabStorage.Free()
	dmmap.Init(env)
	sources, err := removalSources(catalog.Root, env.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, hull := range catalog.Hulls {
		if !strings.HasSuffix(hull.Type, "/delta") && !strings.HasSuffix(hull.Type, "/bogatyr") {
			continue
		}
		p, err := OpenProject(catalog, env, hull)
		if err != nil {
			t.Fatal(err)
		}
		p.costSources = sources
		if err = p.SetShipDetails(ShipDetails{Description: "Updated description"}); err != nil {
			t.Fatalf("%s details: %v", hull.Name, err)
		}
		if err = p.RenameShip("Rename Audit " + hull.Name); err != nil {
			t.Fatalf("%s rename: %v", hull.Name, err)
		}
		changes, err := p.Changes()
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) == 0 {
			t.Fatal("empty rename")
		}
		t.Logf("%s: %d planned file changes", hull.Name, len(changes))
		checked++
	}
	if checked != 2 {
		t.Fatalf("checked %d ships", checked)
	}
}
