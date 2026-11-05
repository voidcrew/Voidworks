package ruin

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmmap/dmmdata"
)

func TestDestinationAreas(t *testing.T) {
	c := environment(t)
	for _, loc := range c.Locations {
		areas := c.Areas(loc, false)
		if len(areas) < 3 {
			t.Fatalf("missing standard areas: %+v", areas)
		}
		outdoors := "/area/space"
		if loc.Type == Type+"/icemoon" {
			outdoors = "/area/ice_outdoors"
		}
		if areas[0].Path != outdoors {
			t.Fatalf("wrong outdoor area for %s: %+v", loc.Name, areas)
		}
		if c.Areas(loc, true)[0].Path != "/area/template_noop" {
			t.Fatal("shared outdoors would override its host environment")
		}
	}
	s := setup(c)
	s.Area = "/area/ruin/custom"
	if _, err := New(c, s); err == nil {
		t.Fatal("custom area accepted")
	}
}

func TestAnywhereCreateReloadAndEdit(t *testing.T) {
	c := environment(t)
	if c.HasAnywhere() {
		t.Fatal("a project without shared ruins reported some")
	}
	s := setup(c)
	s.Anywhere, s.Area = true, "/area/template_noop"
	p, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := p.Changes()
	if err != nil || len(changes) != len(c.Locations)+2 {
		t.Fatalf("shared review: %v %+v", err, changes)
	}
	if exists(p.Template.File) {
		t.Fatal("review created the map")
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if c.NameError(s.Properties.Name, "") == nil {
		t.Fatal("new shared display name was not reserved")
	}
	beforeMap := read(t, p.Template.File)
	data, err := dmmdata.New(p.Template.File)
	if err != nil {
		t.Fatal(err)
	}
	if data.Dictionary["aaa"][1].Path() != "/area/template_noop" {
		t.Fatal("shared map overrides host area")
	}
	c = reload(t, c)
	shared := find(t, c, p.Template.Type)
	if shared.Name != s.Properties.Name || shared.Location != "Anywhere" || len(shared.Variants) != 2 || len(c.Templates) != 3 {
		t.Fatalf("shared map was not grouped: %+v", shared)
	}
	if !c.HasAnywhere() {
		t.Fatal("shared ruin was not reported")
	}
	for _, loc := range c.Locations {
		if !shared.PlacedIn(loc.Name) {
			t.Fatal("location filter hides shared map")
		}
		obj := c.Dme.Objects[loc.Type+"/"+s.ID]
		if obj == nil || obj.Parent().Path != loc.Type || obj.Vars.ValueV("ruin_type", "") != c.Dme.Objects[loc.Type].Vars.ValueV("ruin_type", "") {
			t.Fatal("shared ruin lost native destination inheritance")
		}
		if text(obj.Vars, "name") != s.Properties.Name+" ("+loc.Name+")" {
			t.Fatal("runtime template names collide")
		}
		if text(obj.Vars, "suffix") != s.ID+".dmm" || text(obj.Vars, "prefix") != c.anywherePrefix() {
			t.Fatal("registrations do not share one map")
		}
	}
	if c.NameError(s.Properties.Name, "") == nil || c.SuggestAnywhereID(s.Properties.Name) == s.ID {
		t.Fatal("shared name or identifier not reserved")
	}
	p, err = Open(c, shared)
	if err != nil {
		t.Fatal(err)
	}
	p.Properties.Name = "Shared survey camp"
	p.Properties.Cost = 4
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	p.Properties.Weight = 2
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeMap, read(t, shared.File)) {
		t.Fatal("editing shared properties rewrote the map")
	}
	c = reload(t, c)
	shared = find(t, c, shared.Type)
	for _, variant := range shared.Variants {
		props, err := ReadProperties(c.Dme.Objects[variant.Type])
		if err != nil || props.Cost != 4 || props.Weight != 2 || props.Name != "Shared survey camp ("+variant.Location+")" {
			t.Fatalf("variant edits lost: %+v %v", props, err)
		}
		rel := strings.ReplaceAll(c.sourceRelative(variant.Type), "/", "\\")
		if strings.Count(string(read(t, c.Dme.RootFile)), rel) != 1 {
			t.Fatal("duplicated include")
		}
	}
}

func TestAnywherePreservesVariantSettingsAndConflicts(t *testing.T) {
	c := environment(t)
	s := setup(c)
	s.Anywhere, s.Area = true, "/area/template_noop"
	p, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	spaceSource := filepath.Join(c.Dme.RootDir, c.sourceRelative(Type+"/space/"+s.ID))
	custom := strings.Replace(string(read(t, spaceSource)), "\tcost = 3", "\tcost = 9", 1) + "\n// mapper's custom code\n"
	write(t, spaceSource, []byte(custom))
	c = reload(t, c)
	p, err = Open(c, find(t, c, p.Template.Type))
	if err != nil {
		t.Fatal(err)
	}
	p.Properties.Weight = 2.5
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if got := string(read(t, spaceSource)); !strings.Contains(got, "\tcost = 9") || !strings.Contains(got, "// mapper's custom code") {
		t.Fatal("per-destination settings or custom code overwritten")
	}
	before := read(t, p.source.Path)
	write(t, spaceSource, append(read(t, spaceSource), []byte("// external edit\n")...))
	p.Properties.Cost = 5
	if err = p.Save(); err == nil {
		t.Fatal("shared save overwrote an external edit")
	}
	if !bytes.Equal(before, read(t, p.source.Path)) {
		t.Fatal("conflict partially saved another destination")
	}
}

func TestAnywhereCreationConflict(t *testing.T) {
	c := environment(t)
	s := setup(c)
	s.Anywhere, s.Area = true, "/area/template_noop"
	p, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	conflict := p.companions[0].source.Path
	write(t, conflict, []byte("// another mapper's file\n"))
	if err = p.Save(); err == nil {
		t.Fatal("shared creation overwrote an existing file")
	}
	if exists(p.Template.File) || exists(p.source.Path) {
		t.Fatal("failed creation left partial output")
	}
}
