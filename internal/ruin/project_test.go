package ruin

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap/dmmdata"
)

const fixture = `#define TRUE 1
#define FALSE 0
/area/space
/area/ruin
	var/requires_power = TRUE
	var/always_unpowered = FALSE
	var/default_gravity = 1
/area/ruin/powered
	requires_power = FALSE
/area/ruin/unpowered
	always_unpowered = TRUE
/area/ruin/space
	default_gravity = 0
/area/ruin/space/unpowered
	always_unpowered = TRUE
/area/ruin/space/has_grav
	default_gravity = 1
/area/ruin/space/has_grav/powered
	requires_power = FALSE
/area/template_noop
/area/ice_outdoors
/turf/open/space
/turf/open/floor/plating
/turf/template_noop
/datum/map_template/shuttle/voidcrew
/datum/map_template/ruin
	var/name = null
	var/id = null
	var/description = "Inherited description"
	var/prefix = null
	var/suffix = null
	var/cost = 0
	var/mineral_cost = 0
	var/placement_weight = 1
	var/allow_duplicates = TRUE
	var/always_place = FALSE
	var/unpickable = FALSE
	var/always_spawn_with = null
	var/default_area = /area/space
	var/ruin_type = null
/datum/overmap/planet
	var/ruin_type = null
	var/surface_area = null
/datum/overmap/planet/space
	ruin_type = "space"
/datum/overmap/planet/ice
	ruin_type = "ice"
	surface_area = /area/ice_outdoors
/datum/map_template/ruin/space
	ruin_type = "space"
	prefix = "_maps/voidcrew/RandomRuins/SpaceRuins/"
	cost = 3
	allow_duplicates = FALSE
/datum/map_template/ruin/icemoon
	ruin_type = "ice"
	prefix = "_maps/RandomRuins/IceRuins/"
/datum/map_template/ruin/unused_planet
	ruin_type = "unused"
	prefix = "_maps/RandomRuins/UnusedRuins/"
/datum/map_template/ruin/icemoon/underground
	cost = 6
/datum/map_template/ruin/space/old
	id = "old"
	name = "Old wreck"
	suffix = "old.dmm"
	always_spawn_with = list(/datum/map_template/ruin/space/old/linked = 4)
/datum/map_template/ruin/space/old/proc/custom_behavior()
	return "preserve this"
/datum/map_template/ruin/space/old/linked
	id = "linked"
	name = "Linked wreck"
`

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func environment(t *testing.T) *Catalog {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "fixture.dm"), []byte(fixture))
	dmeFile := filepath.Join(root, "test.dme")
	write(t, dmeFile, []byte("// BEGIN_INCLUDE\r\n#include \"fixture.dm\"\r\n// END_INCLUDE\r\n"))
	dme, err := dmenv.New(dmeFile)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Discover(dme)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func setup(c *Catalog) Setup {
	var loc Location
	for _, l := range c.Locations {
		if l.Type == Type+"/space" {
			loc = l
		}
	}
	return Setup{Location: loc, ID: "new_wreck", Properties: Properties{Name: "New [wreck]", Description: "Quotes \" and a [bracket]\nSecond line", Cost: 3, Weight: 1}, Width: 9, Height: 7, Turf: "/turf/open/floor/plating", Area: "/area/ruin/space/unpowered"}
}
func find(t *testing.T, c *Catalog, path string) Template {
	t.Helper()
	for _, item := range c.Templates {
		if item.Type == path {
			return item
		}
	}
	t.Fatal("missing template", path)
	return Template{}
}
func reload(t *testing.T, c *Catalog) *Catalog {
	t.Helper()
	dme, err := dmenv.New(c.Dme.RootFile)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Discover(dme)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDiscoveryAndDefaults(t *testing.T) {
	c := environment(t)
	if len(c.Locations) != 2 || len(c.Templates) != 2 {
		t.Fatalf("wrong catalog: %+v", c)
	}
	linked := find(t, c, Type+"/space/old/linked")
	if linked.Location != "Space" || !strings.HasSuffix(filepath.ToSlash(linked.File), "SpaceRuins/old.dmm") {
		t.Fatalf("inheritance lost: %+v", linked)
	}
	p, err := ReadProperties(c.Dme.Objects[linked.Type])
	if err != nil {
		t.Fatal(err)
	}
	if p.Cost != 3 || p.AllowDuplicates || p.Weight != 1 {
		t.Fatalf("defaults lost: %+v", p)
	}
	if c.NameError("  OLD   WRECK ", "") == nil {
		t.Fatal("duplicate display name accepted")
	}
	s := setup(c)
	if c.SuggestID("123 wreck", s.Location) != "ruin_123_wreck" {
		t.Fatal("invalid automatic ID")
	}
	if c.SuggestID("Old", s.Location) != "old_2" {
		t.Fatal("ID collision not avoided")
	}
}

func TestCreateAndReloadRuin(t *testing.T) {
	c := environment(t)
	s := setup(c)
	p, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := p.Changes()
	if err != nil || len(changes) != 3 {
		t.Fatalf("review: %v %+v", err, changes)
	}
	if exists(p.Template.File) || c.Dme.Objects[p.Template.Type] != nil {
		t.Fatal("preview changed files or environment")
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if p.Modified() {
		t.Fatal("save left a dirty project")
	}
	if c.NameError(s.Properties.Name, "") == nil {
		t.Fatal("newly saved name was not reserved before refreshing the catalog")
	}
	data, err := dmmdata.New(p.Template.File)
	if err != nil {
		t.Fatal(err)
	}
	if !data.IsTgm || data.MaxX != 9 || data.MaxY != 7 || data.MaxZ != 1 || len(data.Grid) != 63 {
		t.Fatal("bad map dimensions or format")
	}
	for _, prefabs := range data.Dictionary {
		if len(prefabs) != 2 || prefabs[0].Path() != s.Turf || prefabs[1].Path() != s.Area {
			t.Fatal("bad map palette")
		}
	}
	projectBytes := read(t, c.Dme.RootFile)
	if strings.Count(string(projectBytes), "new_wreck.dm") != 1 || bytes.Count(projectBytes, []byte("\n")) != bytes.Count(projectBytes, []byte("\r\n")) {
		t.Fatal("include or line endings wrong")
	}
	c = reload(t, c)
	loaded := find(t, c, p.Template.Type)
	q, err := Open(c, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if q.Properties != s.Properties {
		t.Fatalf("properties did not round-trip: %+v != %+v", q.Properties, s.Properties)
	}
	if c.Dme.Objects["/area/ruin/"+s.ID] != nil || bytes.Contains(read(t, p.source.Path), []byte("/area/")) {
		t.Fatal("created a custom area instead of using the existing area")
	}
	beforeMap := read(t, p.Template.File)
	q.Properties.Cost = 7.5
	if err = q.Save(); err != nil {
		t.Fatal(err)
	}
	q.Properties.Weight = 2
	if err = q.Save(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeMap, read(t, p.Template.File)) {
		t.Fatal("property editing rewrote the map")
	}
	if strings.Count(string(read(t, c.Dme.RootFile)), "new_wreck.dm") != 1 {
		t.Fatal("duplicate include")
	}
	c = reload(t, c)
	q, err = Open(c, find(t, c, p.Template.Type))
	if err != nil {
		t.Fatal(err)
	}
	if q.Properties.Cost != 7.5 || q.Properties.Weight != 2 || q.Properties.Name != s.Properties.Name {
		t.Fatal("repeated save lost fields")
	}
}

func TestHandwrittenPropertiesPreserveCustomCode(t *testing.T) {
	c := environment(t)
	original := read(t, filepath.Join(c.Dme.RootDir, "fixture.dm"))
	template := find(t, c, Type+"/space/old")
	p, err := Open(c, template)
	if err != nil {
		t.Fatal(err)
	}
	p.Properties.Weight = 2.5
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	source := string(read(t, p.source.Path))
	if strings.Contains(source, "always_spawn_with") || strings.Contains(source, "cost =") || strings.Contains(source, "name =") {
		t.Fatal("untouched values were overridden")
	}
	if !bytes.Equal(original, read(t, filepath.Join(c.Dme.RootDir, "fixture.dm"))) {
		t.Fatal("handwritten source changed")
	}
	c = reload(t, c)
	if c.Dme.Objects[template.Type].Vars.ValueV("always_spawn_with", "null") == "null" {
		t.Fatal("linked ruin behavior lost")
	}
	p, err = Open(c, find(t, c, template.Type))
	if err != nil {
		t.Fatal(err)
	}
	p.Properties.Name = "Renamed wreck"
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	c = reload(t, c)
	p, err = Open(c, find(t, c, template.Type))
	if err != nil {
		t.Fatal(err)
	}
	if p.Properties.Weight != 2.5 || p.Properties.Name != "Renamed wreck" {
		t.Fatal("override did not survive reopen")
	}
}

func TestValidationAndConflictingSaves(t *testing.T) {
	c := environment(t)
	s := setup(c)
	for _, mutate := range []func(*Setup){func(s *Setup) { s.ID = "../escape" }, func(s *Setup) { s.ID = "old" }, func(s *Setup) { s.Width = 0 }, func(s *Setup) { s.Height = 257 }, func(s *Setup) { s.Turf = "/turf/missing" }, func(s *Setup) { s.Properties.Weight = math.NaN() }, func(s *Setup) { s.Properties.Name = " OLD WRECK " }, func(s *Setup) { s.Location.Prefix = "../../outside/" }} {
		bad := s
		mutate(&bad)
		if _, err := New(c, bad); err == nil {
			t.Fatalf("accepted invalid setup: %+v", bad)
		}
	}
	p, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	// A .dme changed after loading (such as a game update) keeps its changes;
	// only this ruin's include is applied on top.
	write(t, c.Dme.RootFile, append(read(t, c.Dme.RootFile), []byte("// outside edit\r\n")...))
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if dme := string(read(t, c.Dme.RootFile)); !strings.Contains(dme, "// outside edit") || !exists(p.source.Path) {
		t.Fatalf("save lost the outside .dme edit or the ruin source:\n%s", dme)
	}
	s.ID += "_second"
	s.Properties.Name += " Second"
	p, err = New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	write(t, p.Template.File, []byte("another mapper's map"))
	if err = p.Save(); err == nil {
		t.Fatal("overwrote existing map")
	}
	if string(read(t, p.Template.File)) != "another mapper's map" {
		t.Fatal("collision destroyed map")
	}
}

func TestOriginalSourceConflict(t *testing.T) {
	c := environment(t)
	p, err := Open(c, find(t, c, Type+"/space/old"))
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(c.Dme.RootDir, "fixture.dm")
	write(t, original, append(read(t, original), []byte("\n// external change\n")...))
	p.Properties.Cost = 9
	if err = p.Save(); err == nil {
		t.Fatal("stale properties saved over changed source")
	}
	if exists(p.source.Path) {
		t.Fatal("conflict left an override")
	}
}
