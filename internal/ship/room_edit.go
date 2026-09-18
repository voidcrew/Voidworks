package ship

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/util"
)

func isNoop(path string) bool {
	return path == "/turf/template_noop" || path == "/area/template_noop"
}

func (p *Project) moduleIndex(id string) int {
	for i, m := range p.Hull.Modules {
		if m.ID == id {
			return i
		}
	}
	return -1
}

// defaultModule is the option a slot shows when nothing else is chosen.
func (p *Project) defaultModule(slot, theme string) *Module {
	var first *Module
	for i := range p.Hull.Modules {
		m := &p.Hull.Modules[i]
		if m.Slot != slot || !m.Available(theme) {
			continue
		}
		if m.Default {
			return m
		}
		if first == nil {
			first = m
		}
	}
	return first
}

// componentType finds the handwritten datum registered for a theme or module ID.
func (p *Project) componentType(prefix, id string) (string, error) {
	typePath := ""
	for path, obj := range p.Dme.Objects {
		if strings.HasPrefix(path, prefix) && text(obj.Vars, "id") == id && obj.Vars.ValueV("for_ship", "") == p.Hull.Type {
			if typePath != "" {
				return "", fmt.Errorf("multiple definitions use the ID %s", id)
			}
			typePath = path
		}
	}
	if typePath == "" {
		return "", fmt.Errorf("cannot locate the definition registered as %s", id)
	}
	return typePath, nil
}

// existingDocument opens a map that is on disk or already open; absent maps return nil.
func (p *Project) existingDocument(file string) (*Document, error) {
	if d := p.Documents[file]; d != nil && d.Active {
		return d, nil
	}
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return p.document(file)
}

func connectorAt(m *dmmap.Dmm) util.Point {
	var found []util.Point
	for _, tile := range m.Tiles {
		for _, i := range tile.Instances() {
			if path := i.Prefab().Path(); path == Connector || strings.HasPrefix(path, Connector+"/") {
				found = append(found, tile.Coord)
			}
		}
	}
	if len(found) == 1 {
		return found[0]
	}
	return util.Point{X: 1, Y: 1, Z: 1}
}

func slotMarker(hull *dmmap.Dmm, slot string) (util.Point, string, *dmminstance.Instance) {
	for _, tile := range hull.Tiles {
		for _, i := range tile.Instances() {
			path := i.Prefab().Path()
			if (path == SlotMarker || strings.HasPrefix(path, SlotMarker+"/")) && text(i.Prefab().Vars(), "key") == slot {
				return tile.Coord, text(i.Prefab().Vars(), footprintVar), i
			}
		}
	}
	return util.Point{}, "", nil
}

// roomShapes reads every hull marker with the module box behind it. A slot
// without a readable default option covers only its marker tile.
func (p *Project) roomShapes(hull *dmmap.Dmm, theme Theme) (map[string]RoomShape, error) {
	rooms := map[string]RoomShape{}
	for _, tile := range hull.Tiles {
		for _, i := range tile.Instances() {
			path := i.Prefab().Path()
			if path != SlotMarker && !strings.HasPrefix(path, SlotMarker+"/") {
				continue
			}
			slot := text(i.Prefab().Vars(), "key")
			if slot == "" {
				continue
			}
			mask := text(i.Prefab().Vars(), footprintVar)
			room := RoomShape{Slot: slot, Origin: tile.Coord, Marker: tile.Coord, Footprint: FullFootprint(1, 1)}
			var err error
			if m := p.defaultModule(slot, theme.ID); m != nil {
				file, err := p.moduleFile(*m, theme.ID)
				if err != nil {
					return nil, err
				}
				d, err := p.document(file)
				if err != nil {
					return nil, err
				}
				conn := connectorAt(d.Map)
				room.Origin = util.Point{X: tile.Coord.X - conn.X + 1, Y: tile.Coord.Y - conn.Y + 1, Z: 1}
				room.Footprint = FullFootprint(d.Map.MaxX, d.Map.MaxY)
				if mask != "" {
					room.Footprint, err = ParseFootprint(mask, d.Map.MaxX, d.Map.MaxY)
				}
			} else if mask != "" {
				room.Footprint, err = parseMask(mask)
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", SlotDisplayName(slot), err)
			}
			rooms[slot] = room
		}
	}
	return rooms, nil
}

// slotShape reads a slot's shape from the theme's hull against a module's box.
func (p *Project) slotShape(theme Theme, slot string, module *dmmap.Dmm) (Footprint, error) {
	full := FullFootprint(module.MaxX, module.MaxY)
	file, err := p.Catalog.HullFile(p.Hull, theme)
	if err != nil {
		return full, nil
	}
	d, err := p.existingDocument(file)
	if err != nil || d == nil {
		return full, err
	}
	_, mask, i := slotMarker(d.Map, slot)
	if i == nil || mask == "" {
		return full, nil
	}
	f, err := ParseFootprint(mask, module.MaxX, module.MaxY)
	if err != nil {
		return full, fmt.Errorf("%s: %w", SlotDisplayName(slot), err)
	}
	return f, nil
}

type hullMap struct {
	theme Theme
	doc   *Document
}

// hullMaps opens each distinct hull map, paired with a theme that selects the
// option variants loaded onto it.
func (p *Project) hullMaps() ([]hullMap, error) {
	var maps []hullMap
	seen := map[string]bool{}
	themes := append([]Theme{}, p.Hull.Themes...)
	if p.Hull.Suffix != "" {
		themes = append(themes, Theme{})
	}
	for _, theme := range themes {
		file, err := p.Catalog.HullFile(p.Hull, theme)
		if err != nil {
			return nil, err
		}
		if seen[file] {
			continue
		}
		d, err := p.existingDocument(file)
		if err != nil {
			return nil, err
		}
		if d == nil {
			continue
		}
		seen[file] = true
		maps = append(maps, hullMap{theme, d})
	}
	return maps, nil
}

// moduleFiles lists an option's base map and every existing theme variant.
func (p *Project) moduleFiles(m Module) ([]string, error) {
	var files []string
	for _, theme := range append([]string{""}, m.Themes...) {
		name := m.File
		if theme != "" {
			name = strings.TrimSuffix(name, ".dmm") + "_" + theme + ".dmm"
		}
		file, err := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, name))
		if err != nil {
			return nil, err
		}
		if d := p.Documents[file]; d == nil || !d.Active {
			if _, err := os.Stat(file); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return nil, err
			}
		}
		files = append(files, file)
	}
	return files, nil
}

// retireMap takes a map out of the project. Save deletes it unless a later
// undo makes the hull reference it again.
func (p *Project) retireMap(file string) error {
	d, err := p.existingDocument(file)
	if err != nil || d == nil {
		return err
	}
	d.Active = false
	if p.renamedMaps == nil {
		p.renamedMaps = map[string]bool{}
	}
	p.renamedMaps[file] = true
	return nil
}

// RemoveModule deletes one room option with its maps, crew and prices.
func (p *Project) RemoveModule(id string) error {
	i := p.moduleIndex(id)
	if i < 0 {
		return fmt.Errorf("module option no longer exists")
	}
	m := p.Hull.Modules[i]
	others := 0
	for _, other := range p.Hull.Modules {
		if other.Slot == m.Slot && other.ID != id {
			others++
		}
	}
	if others == 0 {
		return fmt.Errorf("%s is the only option for the %s module; remove the module instead", m.Name, SlotDisplayName(m.Slot))
	}
	if m.Default {
		return fmt.Errorf("%s is the default option; choose another default first", m.Name)
	}
	return p.dropModule(i)
}

func (p *Project) dropModule(i int) error {
	m := p.Hull.Modules[i]
	if p.Settings == nil {
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		for _, base := range p.rooms.base.Modules {
			if base.ID != m.ID {
				continue
			}
			// A handwritten definition is cut out of its source on save.
			typePath, err := p.componentType("/datum/ship_upgrade_module/", m.ID)
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
			_, removed, err := removeDefinitions(p.rooms.sources[file].Before, []string{typePath})
			if err != nil {
				return err
			}
			if !removed[typePath] {
				return fmt.Errorf("cannot locate the definition of %s; reload the environment first", m.Name)
			}
			if p.rooms.removals == nil {
				p.rooms.removals = map[string]nameTarget{}
			}
			p.rooms.removals[m.ID] = nameTarget{file, typePath}
		}
	}
	files, err := p.moduleFiles(m)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err = p.retireMap(file); err != nil {
			return err
		}
	}
	if p.Crew != nil {
		delete(p.Crew.Rosters, "module/"+m.ID)
		delete(p.Crew.ModuleThemes, m.ID)
	}
	delete(p.partCosts, "module/"+m.ID)
	p.Hull.Modules = append(p.Hull.Modules[:i], p.Hull.Modules[i+1:]...)
	return nil
}

// SetDefaultModule makes one option the slot's default and clears the others.
func (p *Project) SetDefaultModule(slot, id string) error {
	i := p.moduleIndex(id)
	if i < 0 || p.Hull.Modules[i].Slot != slot {
		return fmt.Errorf("choose an option of the %s module", SlotDisplayName(slot))
	}
	if p.Hull.Modules[i].Default {
		return nil
	}
	if p.Settings == nil {
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		for _, base := range p.rooms.base.Modules {
			if base.Slot != slot {
				continue
			}
			if _, ok := p.rooms.defaults[base.ID]; ok {
				continue
			}
			typePath, err := p.componentType("/datum/ship_upgrade_module/", base.ID)
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
			if _, err = rewriteFlag(p.rooms.sources[file].Before, typePath, "is_default", base.ID == id); err != nil {
				return err
			}
			if p.rooms.defaults == nil {
				p.rooms.defaults = map[string]nameTarget{}
			}
			p.rooms.defaults[base.ID] = nameTarget{file, typePath}
		}
	}
	for j := range p.Hull.Modules {
		if p.Hull.Modules[j].Slot == slot {
			p.Hull.Modules[j].Default = j == i
		}
	}
	return nil
}

func remove(values []string, value string) []string {
	kept := make([]string, 0, len(values))
	for _, v := range values {
		if v != value {
			kept = append(kept, v)
		}
	}
	return kept
}

// RemoveSlot puts the default option's content back into the hull and deletes
// the room with every option. The chosen variant must place the room.
func (p *Project) RemoveSlot(themeIndex int, slot string) error {
	theme, err := p.roomTheme(themeIndex)
	if err != nil {
		return err
	}
	if !p.slotIDUsed(slot) {
		return fmt.Errorf("module no longer exists")
	}
	file, err := p.Catalog.HullFile(p.Hull, theme)
	if err != nil {
		return err
	}
	hull, err := p.document(file)
	if err != nil {
		return err
	}
	if _, _, marker := slotMarker(hull.Map, slot); marker == nil {
		return fmt.Errorf("%s has no hull marker in this theme", SlotDisplayName(slot))
	}
	if p.Settings == nil {
		if Contains(p.Hull.Slots, slot) {
			if err = p.prepareRooms(&Theme{}); err != nil {
				return err
			}
		}
		for _, t := range p.Hull.Themes {
			if Contains(t.Slots, slot) {
				if err = p.prepareRooms(&t); err != nil {
					return err
				}
			}
		}
	}
	hulls, err := p.hullMaps()
	if err != nil {
		return err
	}
	type restore struct {
		hull   *Document
		module *Document
		marker *dmminstance.Instance
		coord  util.Point
	}
	var restores []restore
	for _, h := range hulls {
		coord, _, marker := slotMarker(h.doc.Map, slot)
		if marker == nil {
			continue
		}
		r := restore{hull: h.doc, marker: marker, coord: coord}
		if m := p.defaultModule(slot, h.theme.ID); m != nil {
			file, err := p.moduleFile(*m, h.theme.ID)
			if err != nil {
				return err
			}
			if r.module, err = p.document(file); err != nil {
				return err
			}
			if err = restoreModule(h.doc.Map, r.module.Map, coord, true); err != nil {
				return err
			}
		}
		restores = append(restores, r)
	}
	for _, r := range restores {
		if r.module != nil {
			restoreModule(r.hull.Map, r.module.Map, r.coord, false)
		}
		r.hull.Map.GetTile(r.coord).InstancesRemoveByInstance(r.marker)

		p.protect(r.hull.Map)
	}
	for i := len(p.Hull.Modules) - 1; i >= 0; i-- {
		if p.Hull.Modules[i].Slot == slot {
			if err = p.dropModule(i); err != nil {
				return err
			}
		}
	}
	p.Hull.Slots = remove(p.Hull.Slots, slot)
	for i, t := range p.Hull.Themes {
		if t.Slots != nil {
			p.Hull.Themes[i].Slots = remove(t.Slots, slot)
		}
	}
	return nil
}

// restoreModule lays an option's content onto the hull at its marker. With
// check set it only verifies that every tile lands inside the hull.
func restoreModule(hull, module *dmmap.Dmm, marker util.Point, check bool) error {
	conn := connectorAt(module)
	for _, tile := range module.Tiles {
		target := util.Point{X: marker.X - conn.X + tile.Coord.X, Y: marker.Y - conn.Y + tile.Coord.Y, Z: 1}
		for _, i := range tile.Instances() {
			path := i.Prefab().Path()
			if mappingMarker(path) || isNoop(path) {
				continue
			}
			if !hull.HasTile(target) {
				return fmt.Errorf("option content at %d,%d lies outside the hull", tile.Coord.X, tile.Coord.Y)
			}
			if check {
				continue
			}
			dst := hull.GetTile(target)
			for _, group := range []string{"/turf/", "/area/"} {
				if strings.HasPrefix(path, group) {
					dst.InstancesRemoveByPath(strings.TrimSuffix(group, "/"))
				}
			}
			dst.InstancesAdd(i.Prefab())
		}
	}
	return nil
}

// ReshapeSlot changes a room's tiles and box. Content keeps its hull position;
// tiles leaving the room must be empty in every option.
func (p *Project) ReshapeSlot(themeIndex int, slot string, origin util.Point, shape Footprint) error {
	theme, err := p.roomTheme(themeIndex)
	if err != nil {
		return err
	}
	if err = p.footprintError(shape); err != nil {
		return err
	}
	file, err := p.Catalog.HullFile(p.Hull, theme)
	if err != nil {
		return err
	}
	hull, err := p.document(file)
	if err != nil {
		return err
	}
	rooms, err := p.roomShapes(hull.Map, theme)
	if err != nil {
		return err
	}
	old, ok := rooms[slot]
	if !ok {
		return fmt.Errorf("%s has no hull marker in this theme", SlotDisplayName(slot))
	}
	max := util.Point{X: origin.X + shape.W - 1, Y: origin.Y + shape.H - 1, Z: 1}
	if origin.Z != 1 || !hull.Map.HasTile(origin) || !hull.Map.HasTile(max) {
		return fmt.Errorf("select tiles inside the hull")
	}
	room := RoomShape{Slot: slot, Origin: origin, Marker: origin, Footprint: shape}
	if err = roomOverlap(room, rooms); err != nil {
		return err
	}
	type option struct {
		m    Module
		file string
		doc  *Document
		from util.Point // hull tile under module tile 1,1
	}
	var options []option
	blocked := map[string]bool{}
	for _, m := range p.Hull.Modules {
		if m.Slot != slot {
			continue
		}
		files, err := p.moduleFiles(m)
		if err != nil {
			return err
		}
		for _, file := range files {
			d, err := p.document(file)
			if err != nil {
				return err
			}
			conn := connectorAt(d.Map)
			o := option{m, file, d, util.Point{X: old.Marker.X - conn.X + 1, Y: old.Marker.Y - conn.Y + 1, Z: 1}}
			for _, tile := range d.Map.Tiles {
				at := util.Point{X: o.from.X + tile.Coord.X - 1, Y: o.from.Y + tile.Coord.Y - 1, Z: 1}
				for _, i := range tile.Instances() {
					if path := i.Prefab().Path(); !mappingMarker(path) && !isNoop(path) && !room.Contains(at) {
						blocked[m.Name] = true
					}
				}
			}
			options = append(options, o)
		}
	}
	if len(blocked) > 0 {
		names := make([]string, 0, len(blocked))
		for name := range blocked {
			names = append(names, name)
		}
		sort.Strings(names)
		verb := "have"
		if len(names) == 1 {
			verb = "has"
		}
		return fmt.Errorf("%s still %s items on the removed tiles", strings.Join(names, ", "), verb)
	}
	hulls, err := p.hullMaps()
	if err != nil {
		return err
	}
	type placed struct {
		hullMap
		marker *dmminstance.Instance
	}
	var markers []placed
	for _, h := range hulls {
		coord, _, marker := slotMarker(h.doc.Map, slot)
		if marker == nil {
			continue
		}
		if coord != old.Marker {
			return fmt.Errorf("the %s theme places this module elsewhere; reshape it there first", h.theme.Name)
		}
		markers = append(markers, placed{h, marker})
	}
	for _, o := range options {
		reshapeModule(o.doc.Map, util.Point{X: o.from.X - origin.X, Y: o.from.Y - origin.Y}, shape.W, shape.H)

		p.protect(o.doc.Map)
	}
	added := map[util.Point]bool{}
	for cell, in := range shape.Cells {
		if at := (util.Point{X: origin.X + cell.X - 1, Y: origin.Y + cell.Y - 1, Z: 1}); in && !old.Contains(at) {
			added[cell] = true
		}
	}
	for _, mk := range markers {
		if m := p.defaultModule(slot, mk.theme.ID); m != nil {
			for _, o := range options {
				if o.m.ID == m.ID && o.file == p.moduleSource(*m, mk.theme.ID) {
					moveFurniture(mk.doc.Map, o.doc.Map, room, added)
				}
			}
		}
		mk.doc.Map.GetTile(old.Marker).InstancesRemoveByInstance(mk.marker)
		mk.doc.Map.GetTile(origin).InstancesAdd(p.markerPrefab(slot, shape))

		p.protect(mk.doc.Map)
	}
	return nil
}

func (p *Project) moduleSource(m Module, theme string) string {
	file, _ := p.moduleFile(m, theme)
	return file
}

// reshapeModule moves an option's content by shift into a w by h box and puts
// the connector back on tile 1,1. Callers verify the content fits first.
func reshapeModule(m *dmmap.Dmm, shift util.Point, w, h int) {
	moved := map[util.Point]dmmdata.Prefabs{}
	for _, tile := range m.Tiles {
		var kept dmmdata.Prefabs
		for _, i := range tile.Instances() {
			if path := i.Prefab().Path(); !mappingMarker(path) && !isNoop(path) {
				kept = append(kept, i.Prefab())
			}
		}
		if len(kept) > 0 {
			moved[util.Point{X: tile.Coord.X + shift.X, Y: tile.Coord.Y + shift.Y, Z: 1}] = kept
		}
	}
	m.SetMapSize(w, h, 1)
	for _, tile := range m.Tiles {
		prefabs := moved[tile.Coord]
		turf, area := false, false
		for _, f := range prefabs {
			turf = turf || strings.HasPrefix(f.Path(), "/turf/")
			area = area || strings.HasPrefix(f.Path(), "/area/")
		}
		if !turf {
			prefabs = append(prefabs, dmmap.PrefabStorage.Initial("/turf/template_noop"))
		}
		if !area {
			prefabs = append(prefabs, dmmap.PrefabStorage.Initial("/area/template_noop"))
		}
		tile.InstancesSet(prefabs)
	}
	m.GetTile(util.Point{X: 1, Y: 1, Z: 1}).InstancesAdd(dmmap.PrefabStorage.Initial(Connector))
}
