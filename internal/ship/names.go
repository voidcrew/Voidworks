package ship

import (
	"fmt"
	"sdmm/internal/dmapi/dmenv"
	"strings"
)

func nameKey(name string) string { return strings.ToLower(strings.Join(strings.Fields(name), " ")) }

func uniqueName(name string, existing []string) error {
	key := nameKey(name)
	if key == "" {
		return fmt.Errorf("enter a name")
	}
	for _, other := range existing {
		if key == nameKey(other) {
			return fmt.Errorf("the name %q is already in use", strings.TrimSpace(name))
		}
	}
	return nil
}

// Check the whole fleet, including ships outside the modular catalog and drafts
// created in this session. Exclude the current ship only when renaming it.
func ShipNameError(c *Catalog, dme *dmenv.Dme, name, except string) error {
	var existing []string
	for _, h := range c.Hulls {
		if h.Type != except {
			existing = append(existing, h.Name)
		}
	}
	for path, obj := range dme.Objects {
		if path != except && strings.HasPrefix(path, HullType+"/") {
			existing = append(existing, text(obj.Vars, "name"))
		}
	}
	return uniqueName(name, existing)
}

func (p *Project) ModuleNameError(name string) error {
	return p.moduleNameError(name, "")
}

func (p *Project) moduleNameError(name, except string) error {
	existing := []string{"Hull", "Empty module", "Empty room"}
	for _, module := range p.Hull.Modules {
		if module.ID != except {
			existing = append(existing, module.Name)
		}
	}
	return uniqueName(name, existing)
}

func (p *Project) ThemeNameError(name string) error {
	return p.themeNameError(name, "")
}

func (p *Project) themeNameError(name, except string) error {
	var existing []string
	for _, theme := range p.Hull.Themes {
		if theme.ID != except {
			existing = append(existing, theme.Name)
		}
	}
	return uniqueName(name, existing)
}

func (p *Project) AreaNameError(theme Theme, name string) error {
	areas, err := p.Areas(theme)
	if err != nil {
		return err
	}
	var existing []string
	for _, area := range areas {
		existing = append(existing, area.Name)
	}
	// Keep literal names from our definitions too: DM escaping can differ from
	// the parser's display-string representation (for example literal brackets).
	for _, area := range p.RoomAreas {
		existing = append(existing, area.Name)
	}
	return uniqueName(name, existing)
}
