package ruin

import (
	"path/filepath"
	"sort"
	"strings"

	"sdmm/internal/ship"
)

const groupHeader = "// RUIN WORKSHOP GROUP: "

func memberOf(t Template, path string) bool {
	if t.Type == path {
		return true
	}
	for _, v := range t.Variants {
		if v.Type == path {
			return true
		}
	}
	return false
}

func (t Template) PlacedIn(location string) bool {
	if t.Location == location {
		return true
	}
	for _, v := range t.Variants {
		if v.Location == location {
			return true
		}
	}
	return false
}

// A shared map gets one library entry. Each destination still has a real subtype
// of its own ruin family, as required by Voidcrew's planet and space loaders.
func (c *Catalog) groupTemplates() {
	groups := map[string][]Template{}
	var result []Template
	for _, t := range c.Templates {
		if t.Group == "" {
			result = append(result, t)
			continue
		}
		key := t.Group + "\x00" + strings.ToLower(filepath.Clean(t.File))
		groups[key] = append(groups[key], t)
	}
	for _, variants := range groups {
		sort.Slice(variants, func(i, j int) bool { return variants[i].Type < variants[j].Type })
		t := variants[0]
		t.Name = strings.TrimSuffix(t.Name, " ("+t.Location+")")
		t.Location = "Anywhere"
		t.Variants = variants
		result = append(result, t)
	}
	c.Templates = result
}

func (c *Catalog) anywherePrefix() string {
	if c.Dme.Objects[ship.HullType] != nil {
		return "_maps/voidcrew/RandomRuins/AnywhereRuins/"
	}
	return "_maps/RandomRuins/AnywhereRuins/"
}

func (c *Catalog) SuggestAnywhereID(name string) string {
	if len(c.Locations) == 0 {
		return "ruin"
	}
	return suggestID(name, func(id string) bool {
		for _, loc := range c.Locations {
			if c.idUsed(id, loc) {
				return true
			}
		}
		path, err := ship.Inside(c.Dme.RootDir, c.anywherePrefix()+id+".dmm")
		return err != nil || exists(path)
	})
}

type AreaChoice struct{ Name, Path string }

// Offer existing shared area types. Outdoors inherits the host environment for
// shared maps, so one DMM can keep a planet's atmosphere or a space encounter.
func (c *Catalog) Areas(location Location, anywhere bool) []AreaChoice {
	outdoor := location.Outdoor
	if anywhere {
		outdoor = "/area/template_noop"
	}
	options := []AreaChoice{{"Outdoors", outdoor}, {"Interior (powered)", "/area/ruin/powered"}, {"Interior (unpowered)", "/area/ruin/unpowered"}, {"Interior (needs power)", "/area/ruin"}}
	if location.Type == Type+"/space" && !anywhere {
		options = []AreaChoice{{"Space outside the ruin", "/area/space"}, {"Interior (powered, gravity)", "/area/ruin/space/has_grav/powered"}, {"Interior (unpowered, no gravity)", "/area/ruin/space/unpowered"}, {"Interior (needs power, no gravity)", "/area/ruin/space"}, {"Interior (needs power, gravity)", "/area/ruin/space/has_grav"}}
	}
	var result []AreaChoice
	for _, a := range options {
		if c.Dme.Objects[a.Path] != nil {
			result = append(result, a)
		}
	}
	return result
}
