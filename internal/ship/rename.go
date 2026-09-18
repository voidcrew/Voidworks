package ship

import (
	"fmt"
	"strings"
)

type nameTarget struct {
	file, typePath string
}

func componentName(h Hull, scope string) (string, bool) {
	for _, theme := range h.Themes {
		if scope == "theme/"+theme.ID {
			return theme.Name, true
		}
	}
	for _, module := range h.Modules {
		if scope == "module/"+module.ID {
			return module.Name, true
		}
	}
	return "", false
}

// RenameNameError excludes the selected component while validating its new name.
func (p *Project) RenameNameError(scope, name string) error {
	if _, ok := componentName(p.Hull, scope); !ok {
		return fmt.Errorf("choose a ship theme or module option")
	}
	if strings.HasPrefix(scope, "theme/") {
		return p.themeNameError(name, strings.TrimPrefix(scope, "theme/"))
	}
	return p.moduleNameError(name, strings.TrimPrefix(scope, "module/"))
}

// Rename updates the label and map filenames, preserving IDs and room slots.
func (p *Project) Rename(scope, name string) error {
	if err := p.RenameNameError(scope, name); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if current, _ := componentName(p.Hull, scope); current == name {
		return nil
	}
	if p.Settings == nil {
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		if before, exists := componentName(p.rooms.base, scope); exists {
			if p.rooms.names == nil {
				p.rooms.names = map[string]nameTarget{}
			}
			if _, prepared := p.rooms.names[scope]; !prepared {
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
				if _, err = rewriteName(p.rooms.sources[file].Before, typePath, before, name); err != nil {
					return err
				}
				p.rooms.names[scope] = nameTarget{file: file, typePath: typePath}
				if err = p.prepareMapField(scope, typePath); err != nil {
					return err
				}
			}
		}
	}
	if err := p.renameComponentMaps(scope, name); err != nil {
		return err
	}
	for i, theme := range p.Hull.Themes {
		if scope == "theme/"+theme.ID {
			p.Hull.Themes[i].Name = name
			return nil
		}
	}
	for i, module := range p.Hull.Modules {
		if scope == "module/"+module.ID {
			p.Hull.Modules[i].Name = name
			return nil
		}
	}
	return nil
}
