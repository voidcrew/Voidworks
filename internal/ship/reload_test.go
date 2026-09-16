package ship

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmenv"
)

func TestReloadCatalogFindsSavedRegistrationsInNewIncludes(t *testing.T) {
	root := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("modules.toml", "directory = \"modules/\"\n")
	const base = `/datum
	var/name
/datum/map_template/shuttle/voidcrew
	var/prefix = "ships/"
	var/port_id = "ship"
	var/suffix
	var/has_upgrade_slots = 0
	var/upgrade_slot_ids
/obj/modular_map_root/ship_upgrade
	var/config_file = "modules.toml"
/datum/ship_upgrade_module
	var/id
	var/for_ship
	var/slot
	var/map_file
/datum/ship_theme
	var/id
	var/for_ship
	var/template_suffix
`
	write("test.dme", base+"#include \"hull.dm\"\n")
	write("hull.dm", HullType+"/box\n\tname = \"Box\"\n\tsuffix = \"box\"\n")
	dme, err := dmenv.New(filepath.Join(root, "test.dme"))
	if err != nil {
		t.Fatal(err)
	}
	old, err := Discover(dme)
	if err != nil || len(old.Hulls) != 1 || !old.Hulls[0].Fixed {
		t.Fatalf("fixture did not start with a fixed ship: %v", err)
	}
	write("hull.dm", HullType+"/box\n\tname = \"Box\"\n\tsuffix = \"box\"\n\thas_upgrade_slots = 1\n\tupgrade_slot_ids = list(\"bay\")\n")
	write("test.dme", base+"#include \"hull.dm\"\n#include \"rooms.dm\"\n")
	write("rooms.dm", `/datum/ship_upgrade_module/bay
	name = "Bay"
	id = "bay"
	for_ship = /datum/map_template/shuttle/voidcrew/box
	slot = "bay"
	map_file = "bay.dmm"
/datum/ship_theme/cargo
	name = "Cargo"
	id = "cargo"
	for_ship = /datum/map_template/shuttle/voidcrew/box
	template_suffix = "box"
`)
	current, err := ReloadCatalog(dme)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Hulls) != 1 || current.Hulls[0].Fixed || len(current.Hulls[0].Modules) != 1 || len(current.Hulls[0].Themes) != 1 || !Contains(current.Hulls[0].Slots, "bay") {
		t.Fatalf("saved rooms and variants were lost on reopening: %+v", current.Hulls)
	}
	again, err := Discover(dme)
	if err != nil || !reflect.DeepEqual(current, again) {
		t.Fatal("loaded definitions disagree with the refreshed catalog", err)
	}
	if got := dme.Objects["/datum/ship_upgrade_module/bay"].Location.File; filepath.Base(got) != "rooms.dm" {
		t.Fatal("new registration lost its source location", got)
	}
	// A failed refresh must leave the last good definitions intact.
	write("modules.toml", "invalid configuration\n")
	before := dme.Objects[HullType+"/box"]
	if _, err = ReloadCatalog(dme); err == nil {
		t.Fatal("invalid catalog was accepted")
	}
	if dme.Objects[HullType+"/box"] != before {
		t.Fatal("failed refresh replaced live definitions")
	}
}
