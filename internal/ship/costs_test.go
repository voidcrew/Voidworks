package ship

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
	"sdmm/third_party/sdmmparser"
)

func TestPartCostsParsingAndValidation(t *testing.T) {
	for _, raw := range []string{`list(PART_CLASS_COMBAT = 2, PART_CLASS_SCIENCE = 3, PART_CLASS_TRADE = 5, PART_CLASS_MISC = 7)`, `list("combat" = 2, "science" = 3, "trade" = 5, "misc" = 7)`} {
		cost, err := parsePartCosts(raw)
		if err != nil || !cost.Equal(PartCosts{"combat": 2, "science": 3, "trade": 5, "misc": 7}) {
			t.Fatal(cost, err)
		}
		again, err := parsePartCosts(renderPartCosts(cost))
		if err != nil || !again.Equal(cost) {
			t.Fatal("cost round trip failed", err)
		}
	}
	for _, raw := range []string{`list("combat" = -1)`, `list("combat" = 1.5)`, `list("combat" = 1, PART_CLASS_COMBAT = 2)`, `list("missing" = 3)`, `list("misc" = INFINITY)`, `some_proc()`} {
		if _, err := parsePartCosts(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	for _, raw := range []string{"null", "list()"} {
		if c, err := parsePartCosts(raw); err != nil || len(c) != 0 || renderPartCosts(c) != "list()" {
			t.Fatal("free cost is not an empty list", c, err)
		}
	}
}

func TestDisplayCostsDoNotReadProjectSources(t *testing.T) {
	p, _ := loadedRoomProject(t, true)
	vars := dmvars.MutableVariables{}
	vars.Put("part_requirements", `list("combat" = 2, "trade" = 7)`)
	p.Dme.Objects[p.Hull.Type].Vars = vars.ToImmutable()
	// Labels must still render from the loaded environment when source files
	// are unavailable. The price editor must continue to report that problem.
	p.Dme.RootFile = filepath.Join(t.TempDir(), "unavailable.dme")
	cost, err := p.DisplayPartCosts("ship")
	if err != nil || !cost.Equal(PartCosts{"combat": 2, "trade": 7}) {
		t.Fatal("display price unavailable", cost, err)
	}
	if _, err = p.PartCosts("ship"); err == nil {
		t.Fatal("price editor skipped its source checks")
	}
	p.partCosts = map[string]PartCosts{"ship": {"science": 13}}
	cost, err = p.DisplayPartCosts("ship")
	if err != nil || !cost.Equal(PartCosts{"science": 13}) {
		t.Fatal("display did not reflect unsaved price", cost, err)
	}
	cost["science"] = 99
	if p.partCosts["ship"]["science"] != 13 {
		t.Fatal("display price aliases editable settings")
	}
}

func TestAuthoredCostsLegacyMigrationRoundTripAndUndo(t *testing.T) {
	c, dme := authorEnvironment(t)
	p, err := NewProject(c, dme, "priced", "Priced", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	p.Settings.Cost = 9
	if err := p.addRect(0, "cargo", "Cargo", util.Point{X: 3, Y: 3, Z: 1}, util.Point{X: 5, Y: 5, Z: 1}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTheme(0, "variant", "Variant", true); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	p, err = OpenProject(c, dme, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	if cost, err := p.PartCosts("ship"); err != nil || !cost.Equal(PartCosts{"misc": 9}) {
		t.Fatal("legacy cost changed", cost, err)
	}
	before := p.Capture()
	want := map[string]PartCosts{"ship": {"combat": 2, "science": 3, "trade": 4, "misc": 5}, "theme/variant": {"science": 7, "trade": 9}, "module/cargo_basic": {"combat": 6, "misc": 8}, "theme/standard": {}}
	for scope, cost := range want {
		if err := p.SetPartCosts(scope, cost); err != nil {
			t.Fatal(err)
		}
	}
	if !p.Modified() {
		t.Fatal("cost edits not marked unsaved")
	}
	after := p.Capture()
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenProject(c, dme, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Modified() {
		t.Fatal("reopened cost metadata is dirty")
	}
	for scope, cost := range want {
		actual, err := reopened.PartCosts(scope)
		if err != nil || !actual.Equal(cost) {
			t.Fatal("cost lost on reload", scope, actual, err)
		}
	}
	// Regenerating registrations for an unrelated edit must retain every price.
	reopened.Settings.Description = "Keep all prices"
	if err := reopened.Save(); err != nil {
		t.Fatal(err)
	}
	for _, scope := range reopened.CostScopes() {
		file := reopened.outputPaths()[2]
		if scope.ID == "ship" {
			file = reopened.outputPaths()[1]
		}
		data, _ := os.ReadFile(file)
		raw, found, explicit, err := sourceCostList(data, scope.Type, scope.Field)
		if err != nil || !found || !explicit {
			t.Fatal("missing generated price", scope, err)
		}
		actual, err := parsePartCosts(raw)
		if err != nil || !actual.Equal(want[scope.ID]) {
			t.Fatal("regeneration changed price", scope, actual, err)
		}
	}
	// Use a fresh fixture state for undo across Save; no unrelated external write.
	p = reopened
	baseline := p.Capture()
	if err := p.SetPartCosts("ship", PartCosts{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	p.Restore(baseline)
	if err := p.Save(); err != nil {
		t.Fatal("undo after save", err)
	}
	if cost, _ := p.PartCosts("ship"); !cost.Equal(want["ship"]) {
		t.Fatal("undo lost costs")
	}
	if len(before.PartCosts) != 0 || len(after.PartCosts) != 4 {
		t.Fatal("history shares cost maps")
	}
}

func TestHandwrittenCostsPreserveSourceAndMergeRoomEdits(t *testing.T) {
	p, file := loadedRoomProject(t, true)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	scope := "theme/other"
	path := "/datum/ship_theme/loaded_rooms_other"
	modified, err := rewriteLiteralList(before, path, "part_cost", "list(PART_CLASS_TRADE = 17)")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, modified, 0600); err != nil {
		t.Fatal(err)
	}
	price, err := p.PartCosts(scope)
	if err != nil || price["trade"] != 17 {
		t.Fatal("source price not loaded", price, err)
	}
	if p.Modified() {
		t.Fatal("viewing prices changed the project")
	}
	initial := p.Capture()
	want := PartCosts{"combat": 2, "science": 4, "trade": 6, "misc": 8}
	if err := p.SetPartCosts(scope, want); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	p.Restore(initial)
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(file)
	if !bytes.Equal(actual, modified) {
		t.Fatal("undo did not restore the original literal and neighboring code")
	}
	if err := p.SetPartCosts(scope, want); err != nil {
		t.Fatal(err)
	}
	if err := p.addRect(1, "new_bay", "New Bay", util.Point{X: 8, Y: 8, Z: 1}, util.Point{X: 10, Y: 10, Z: 1}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPartCosts("module/new_bay_basic", PartCosts{"trade": 11}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal("combined price and room save", err)
	}
	actual, _ = os.ReadFile(file)
	if !bytes.Contains(actual, []byte("return 17")) || !bytes.Contains(actual, []byte("new_bay")) || !bytes.Contains(actual, []byte(renderPartCosts(want))) {
		t.Fatal("combined save lost source content")
	}
	roomCode, _ := os.ReadFile(p.rooms.code)
	if !bytes.Contains(roomCode, []byte("part_cost = list(PART_CLASS_TRADE = 11)")) {
		t.Fatal("new room price not registered")
	}
}

func TestInheritedCostUndoAndExternalConflict(t *testing.T) {
	for _, external := range []bool{false, true} {
		p, file := loadedRoomProject(t, true)
		before, _ := os.ReadFile(file)
		initial := p.Capture()
		if err := p.SetPartCosts("theme/other", PartCosts{"science": 3}); err != nil {
			t.Fatal(err)
		}
		if external {
			if err := os.WriteFile(file, append(before, []byte("\n// external edit\n")...), 0600); err != nil {
				t.Fatal(err)
			}
			if err := p.Save(); err == nil {
				t.Fatal("overwrote external code")
			}
			continue
		}
		if err := p.Save(); err != nil {
			t.Fatal(err)
		}
		p.Restore(initial)
		if err := p.Save(); err != nil {
			t.Fatal(err)
		}
		after, _ := os.ReadFile(file)
		if !bytes.Equal(before, after) {
			t.Fatal("undo left an explicit price in an inherited field")
		}
	}
}

func TestPriceInReopenedTypeDefinition(t *testing.T) {
	p, first := loadedRoomProject(t, true)
	other := filepath.Join(p.Catalog.Root, "prices.dm")
	path := "/datum/ship_theme/loaded_rooms_other"
	if err := os.WriteFile(other, []byte(path+"\n\tpart_cost = list(PART_CLASS_COMBAT = 12)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dme, _ := os.ReadFile(p.Dme.RootFile)
	if err := os.WriteFile(p.Dme.RootFile, append(dme, []byte("\n#include \"prices.dm\"\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(first)
	if cost, err := p.PartCosts("theme/other"); err != nil || cost["combat"] != 12 {
		t.Fatal(cost, err)
	}
	if err := p.SetPartCosts("theme/other", PartCosts{"trade": 5}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(first)
	if !bytes.Equal(before, after) {
		t.Fatal("edited the first declaration instead of the cost owner")
	}
	data, _ := os.ReadFile(other)
	if !strings.Contains(string(data), "PART_CLASS_TRADE = 5") {
		t.Fatal("price override was not updated")
	}
}

func TestCostScopeUsesModuleRegistration(t *testing.T) {
	p, file := loadedRoomProject(t, true)
	m := p.Hull.Modules[0]
	path := "/datum/ship_upgrade_module/loaded_rooms_" + m.ID
	vars := dmvars.MutableVariables{}
	vars.Put("id", dmQuote(m.ID))
	vars.Put("for_ship", p.Hull.Type)
	p.Dme.Objects[path] = &dmenv.Object{Path: path, Vars: vars.ToImmutable(), Location: sdmmparser.Location{File: file}}
	if err := p.SetPartCosts("module/"+m.ID, PartCosts{"misc": 13}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	raw, _, _, err := sourceCostList(data, path, "part_cost")
	if err != nil || !strings.Contains(raw, "13") {
		t.Fatal("module price missing", raw, err)
	}
}
