package ship

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

// Handwritten ships keep their existing registrations. Source snapshots remain
// fixed for the session so undo after Save can restore the original definitions.
type roomEditing struct {
	base         Hull
	sources      map[string]FileChange
	targets      map[string]string     // theme ID -> DM type; empty ID edits the base hull
	names        map[string]nameTarget // component scope -> original name definition
	descriptions map[string]nameTarget
	mapFields    map[string]nameTarget
	removals     map[string]nameTarget // module ID -> definition cut out on save
	defaults     map[string]nameTarget // module ID -> is_default flag to rewrite
	themeTargets map[string]nameTarget // theme ID -> definition to flag or cut out
	themeList    nameTarget            // the hull's available_themes list
	themeOrder   []string              // that list as the source spells it
	themeJobs    map[string]string     // theme ID -> crew copied for a new variant
	themeFile    string                // where new variant registrations are written
	moduleSlots  map[string]nameTarget // option ID -> original slot assignment
	moduleThemes map[string]moduleThemeTarget
	code         string
}

func (p *Project) roomBytes() []byte {
	if p.Settings != nil || p.rooms == nil {
		return nil
	}
	b, _ := json.Marshal(p.Hull)
	return b
}

func (p *Project) roomTheme(index int) (Theme, error) {
	if index == 0 && len(p.Hull.Themes) == 0 {
		return Theme{}, nil
	}
	if index < 0 || index >= len(p.Hull.Themes) {
		return Theme{}, fmt.Errorf("choose a ship variant")
	}
	return p.Hull.Themes[index], nil
}

func (p *Project) roomID() (string, error) {
	if p.Settings != nil {
		return p.Settings.ID, nil
	}
	id := strings.ReplaceAll(strings.TrimPrefix(p.Hull.Type, HullType+"/"), "/", "__")
	return id, ValidID(id)
}

func (p *Project) slotIDUsed(id string) bool {
	if Contains(p.Hull.Slots, id) {
		return true
	}
	for _, theme := range p.Hull.Themes {
		if Contains(theme.Slots, id) {
			return true
		}
	}
	for _, module := range p.Hull.Modules {
		if module.Slot == id {
			return true
		}
	}
	return false
}

func (p *Project) moduleIDError(id string) error {
	for _, module := range p.Hull.Modules {
		if module.ID == id {
			return fmt.Errorf("module ID already exists")
		}
	}
	shipID, err := p.roomID()
	if err != nil {
		return err
	}
	if p.Settings == nil && p.Dme.Objects[roomModuleType(shipID, id)] != nil {
		return fmt.Errorf("module definition already exists for this identifier")
	}
	return nil
}

func roomModuleType(shipID, id string) string {
	return "/datum/ship_upgrade_module/workshop_" + shipID + "_" + id
}

func (p *Project) newRoomModule(theme Theme, id, name, slot string) (Module, string, error) {
	_, err := p.roomID()
	if err != nil {
		return Module{}, "", err
	}
	dir := p.fileID() + "/"
	if p.Settings == nil {
		dir += "workshop/"
	}
	m := Module{ID: id, Name: name, Slot: slot, File: dir + id + ".dmm"}
	file := m.File
	if theme.ID != "" {
		m.Themes = []string{theme.ID}
		file = strings.TrimSuffix(file, ".dmm") + "_" + theme.ID + ".dmm"
	}
	target, err := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, file))
	return m, target, err
}

func (p *Project) roomSource(path string) error {
	if _, ok := p.rooms.sources[path]; ok {
		return nil
	}
	if _, ok := p.files[path]; !ok {
		if err := p.track(path); err != nil {
			return err
		}
	}
	p.rooms.sources[path] = p.files[path]
	return nil
}

func (p *Project) roomTypeFile(typePath string) (string, error) {
	obj := p.Dme.Objects[typePath]
	if obj == nil || obj.Location.File == "" {
		return "", fmt.Errorf("cannot locate the ship definition for %s", typePath)
	}
	file := obj.Location.File
	if filepath.IsAbs(file) {
		var err error
		file, err = filepath.Rel(p.Catalog.Root, file)
		if err != nil {
			return "", err
		}
	}
	return Inside(p.Catalog.Root, file)
}

func (p *Project) prepareRooms(theme *Theme) error {
	if p.Settings != nil {
		return nil
	}
	if p.rooms == nil {
		id, err := p.roomID()
		if err != nil {
			return err
		}
		code, err := Inside(p.Catalog.Root, "voidcrew/modules/ship_upgrades/workshop/"+id+".dm")
		if err != nil {
			return err
		}
		p.rooms = &roomEditing{base: cloneHull(p.Hull), sources: map[string]FileChange{}, targets: map[string]string{}, code: code}
		p.savedRooms = p.roomBytes()
	}
	if err := p.roomSource(p.rooms.code); err != nil {
		return err
	}
	if _, ok := p.files[p.Dme.RootFile]; !ok {
		if err := p.track(p.Dme.RootFile); err != nil {
			return err
		}
	}
	if theme == nil {
		return nil
	}
	typePath := p.Hull.Type
	if theme.ID != "" {
		typePath = ""
		for path, obj := range p.Dme.Objects {
			if strings.HasPrefix(path, "/datum/ship_theme/") && text(obj.Vars, "id") == theme.ID && obj.Vars.ValueV("for_ship", "") == p.Hull.Type {
				if typePath != "" {
					return fmt.Errorf("multiple theme definitions use the ID %s", theme.ID)
				}
				typePath = path
			}
		}
	}
	file, err := p.roomTypeFile(typePath)
	if err != nil {
		return err
	}
	if err = p.roomSource(file); err != nil {
		return err
	}
	before := roomSlots(p.rooms.base, theme.ID)
	if _, err = rewriteRoomSlots(p.rooms.sources[file].Before, typePath, before, before); err != nil {
		return err
	}
	// A fixed ship also needs has_upgrade_slots enabled on its own definition.
	if p.rooms.base.Fixed && theme.ID == "" {
		if _, err = rewriteModularFlag(p.rooms.sources[file].Before, typePath); err != nil {
			return err
		}
	}
	p.rooms.targets[theme.ID] = typePath
	return nil
}

func roomSlots(h Hull, themeID string) []string {
	for _, theme := range h.Themes {
		if theme.ID == themeID {
			return theme.Slots
		}
	}
	return h.Slots
}

func (p *Project) roomChanges(changes []FileChange) ([]FileChange, error) {
	if p.rooms == nil || bytes.Equal(p.roomBytes(), p.savedRooms) {
		return changes, nil
	}
	contents := map[string][]byte{}
	for path, source := range p.rooms.sources {
		contents[path] = append([]byte{}, source.Before...)
	}
	for themeID, typePath := range p.rooms.targets {
		// A removed variant's whole block goes; nothing is left to rewrite in it.
		if themeID != "" && p.themeIndex(themeID) < 0 {
			continue
		}
		before, after := roomSlots(p.rooms.base, themeID), roomSlots(p.Hull, themeID)
		if reflect.DeepEqual(before, after) {
			continue
		}
		file, err := p.roomTypeFile(typePath)
		if err != nil {
			return nil, err
		}
		// A variant back on the hull's rooms drops its own list entirely.
		if after == nil {
			contents[file], err = removeCostAssignment(contents[file], typePath, "upgrade_slot_ids")
		} else {
			contents[file], err = rewriteRoomSlots(contents[file], typePath, before, after)
		}
		if err != nil {
			return nil, err
		}
		if themeID == "" && p.rooms.base.Fixed && !p.Hull.Fixed {
			contents[file], err = rewriteModularFlag(contents[file], typePath)
			if err != nil {
				return nil, err
			}
		}
	}
	for id, target := range p.rooms.removals {
		if p.moduleIndex(id) >= 0 {
			continue
		}
		var err error
		if contents[target.file], _, err = removeDefinitions(contents[target.file], []string{target.typePath}); err != nil {
			return nil, err
		}
	}
	for id, target := range p.rooms.defaults {
		i := p.moduleIndex(id)
		if i < 0 {
			continue
		}
		before := false
		for _, base := range p.rooms.base.Modules {
			if base.ID == id {
				before = base.Default
			}
		}
		if before == p.Hull.Modules[i].Default {
			continue
		}
		var err error
		if contents[target.file], err = rewriteFlag(contents[target.file], target.typePath, "is_default", p.Hull.Modules[i].Default); err != nil {
			return nil, err
		}
	}
	for scope, target := range p.rooms.names {
		name, ok := componentName(p.Hull, scope)
		before, _ := componentName(p.rooms.base, scope)
		if !ok || name == before {
			continue
		}
		var err error
		contents[target.file], err = rewriteName(contents[target.file], target.typePath, before, name)
		if err != nil {
			return nil, err
		}
	}
	for scope, target := range p.rooms.mapFields {
		field, before := mapField(p.rooms.base, scope)
		_, after := mapField(p.Hull, scope)
		// A removed option's block is gone; nothing is left to rewrite in it.
		if _, exists := componentName(p.Hull, scope); before == after || scope != "ship" && !exists {
			continue
		}
		var err error
		contents[target.file], err = rewriteTextField(contents[target.file], target.typePath, field, before, after)
		if err != nil {
			return nil, err
		}
	}
	for scope, target := range p.rooms.descriptions {
		value, ok := componentDescription(p.Hull, scope)
		before, _ := componentDescription(p.rooms.base, scope)
		if !ok || reflect.DeepEqual(value, before) {
			continue
		}
		var err error
		contents[target.file], err = rewriteTextField(contents[target.file], target.typePath, "desc", descriptionText(before), descriptionText(value))
		if err != nil {
			return nil, err
		}
	}
	for id, target := range p.rooms.moduleSlots {
		i := p.moduleIndex(id)
		if i < 0 {
			continue
		}
		before := ""
		for _, m := range p.rooms.base.Modules {
			if m.ID == id {
				before = m.Slot
			}
		}
		after := p.Hull.Modules[i].Slot
		if before == after {
			continue
		}
		var err error
		contents[target.file], err = rewriteTextField(contents[target.file], target.typePath, "slot", before, after)
		if err != nil {
			return nil, err
		}
	}
	baseIDs := map[string]bool{}
	for _, module := range p.rooms.base.Modules {
		baseIDs[module.ID] = true
	}
	id, _ := p.roomID()
	var definitions strings.Builder
	for _, module := range p.Hull.Modules {
		if baseIDs[module.ID] {
			continue
		}
		def, themes := "FALSE", "null"
		if module.Default {
			def = "TRUE"
		}
		if module.Themes != nil {
			themes = dmList(module.Themes)
		}
		fmt.Fprintf(&definitions, "\n%s\n\tid = %s\n\tname = %s\n\tslot = %s\n\tfor_ship = %s\n\tfor_theme = %s\n\tmap_file = %s\n\tis_default = %s\n", roomModuleType(id, module.ID), dmQuote(module.ID), dmQuote(module.Name), dmQuote(module.Slot), p.Hull.Type, themes, dmQuote(module.File), def)
		if module.Description != nil {
			fmt.Fprintf(&definitions, "\tdesc = %s\n", dmQuote(*module.Description))
		}
	}
	generated := len(contents[p.rooms.code])
	contents[p.rooms.code] = append(contents[p.rooms.code], definitions.String()...)
	if err := p.themeChanges(contents); err != nil {
		return nil, err
	}
	for path, content := range contents {
		c := p.files[path]
		if (!c.Existed && len(content) == 0) || bytes.Equal(c.Before, content) {
			continue
		}
		c.After = content
		changes = append(changes, c)
	}
	if len(contents[p.rooms.code]) > generated {
		c := p.files[p.Dme.RootFile]
		c.After = addInclude(c.Before, p.Catalog.Root, p.rooms.code)
		if !bytes.Equal(c.Before, c.After) {
			changes = append(changes, c)
		}
	}
	return changes, nil
}
