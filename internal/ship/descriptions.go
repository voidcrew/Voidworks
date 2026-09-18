package ship

import (
	"fmt"
	"strings"
)

func componentDescription(h Hull, scope string) (*string, bool) {
	for _, theme := range h.Themes {
		if scope == "theme/"+theme.ID {
			return theme.Description, true
		}
	}
	for _, module := range h.Modules {
		if scope == "module/"+module.ID {
			return module.Description, true
		}
	}
	return nil, false
}

func descriptionText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Description returns the text shown to players for a variant or room option.
func (p *Project) Description(scope string) string {
	value, _ := componentDescription(p.Hull, scope)
	return descriptionText(value)
}

// SetDescription preserves identifiers and handwritten source around the desc.
func (p *Project) SetDescription(scope, value string) error {
	current, ok := componentDescription(p.Hull, scope)
	if !ok {
		return fmt.Errorf("choose a ship theme or module option")
	}
	value = strings.ReplaceAll(value, "\r", "")
	if descriptionText(current) == value {
		return nil
	}
	if p.Settings == nil {
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		if before, exists := componentDescription(p.rooms.base, scope); exists {
			if p.rooms.descriptions == nil {
				p.rooms.descriptions = map[string]nameTarget{}
			}
			if _, prepared := p.rooms.descriptions[scope]; !prepared {
				prefix, id, _ := strings.Cut(scope, "/")
				typePath, err := p.componentType(map[string]string{"theme": "/datum/ship_theme/", "module": "/datum/ship_upgrade_module/"}[prefix], id)
				if err != nil {
					return err
				}
				file, err := p.roomTypeFile(typePath)
				if err != nil {
					return err
				}
				if err = p.roomSource(file); err != nil {
					return err
				}
				if _, err = rewriteTextField(p.rooms.sources[file].Before, typePath, "desc", descriptionText(before), value); err != nil {
					return err
				}
				p.rooms.descriptions[scope] = nameTarget{file: file, typePath: typePath}
			}
		}
	}
	for i, theme := range p.Hull.Themes {
		if scope == "theme/"+theme.ID {
			p.Hull.Themes[i].Description = &value
		}
	}
	for i, module := range p.Hull.Modules {
		if scope == "module/"+module.ID {
			p.Hull.Modules[i].Description = &value
		}
	}
	return nil
}
