package ship

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/util"
)

// DM quoted strings use bracket interpolation, which must be escaped too.
func dmQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return `"` + s + `"`
}
func dmList(ids []string) string {
	parts := []string{}
	for _, id := range ids {
		parts = append(parts, dmQuote(id))
	}
	return "list(" + strings.Join(parts, ", ") + ")"
}

func (p *Project) registration() ([]byte, []byte, error) {
	s := p.Settings
	h := p.Hull
	if err := ShipNameError(p.Catalog, p.Dme, h.Name, h.Type); err != nil {
		return nil, nil, err
	}
	if err := ValidID(s.ID); err != nil {
		return nil, nil, err
	}
	if s.Crew < 1 || s.Crew > 32 || s.Cost < 0 {
		return nil, nil, fmt.Errorf("crew must be 1–32 and cost cannot be negative")
	}
	if s.PortDirection != 1 && s.PortDirection != 2 && s.PortDirection != 4 && s.PortDirection != 8 {
		return nil, nil, fmt.Errorf("choose a cardinal docking direction")
	}
	hidden := "FALSE"
	if s.Hidden {
		hidden = "TRUE"
	}
	hull := fmt.Sprintf("%s\n\tname = %s\n\tcatalog_desc = %s\n\tsuffix = %s\n\tshort_name = %s\n\tpart_requirements = list(PART_CLASS_MISC = %d)\n\thas_upgrade_slots = TRUE\n\tupgrade_slot_ids = %s\n\tplayer_hidden = %s\n\tjob_slots = list(\n\t\tlist(name = \"Captain\", officer = TRUE, outfit = /datum/outfit/job/captain, category = JOB_CAT_COMMAND, slots = 1),\n", h.Type, dmQuote(h.Name), dmQuote(s.Description), dmQuote(h.Suffix), dmQuote(h.Name), s.Cost, dmList(h.Slots), hidden)
	if s.Crew > 1 {
		hull += fmt.Sprintf("\t\tlist(name = \"Crew\", outfit = /datum/outfit/job/assistant, category = JOB_CAT_ASSISTANT, slots = %d),\n", s.Crew-1)
	}
	hull += "\t)\n" + p.availableThemesLine()
	hull += fmt.Sprintf("\n%s\n\tname = %s\n\tarea_type = %s\n\tport_direction = %d\n\tpreferred_direction = NORTH\n\n%s\n\tname = %s\n\ticon_state = \"station\"\n", p.portType(), dmQuote(h.Name), p.areaType(), s.PortDirection, p.areaType(), dmQuote(h.Name))
	var modules strings.Builder
	for _, t := range h.Themes {
		def := "FALSE"
		if t.Default {
			def = "TRUE"
		}
		fmt.Fprintf(&modules, "\n/datum/ship_theme/%s_%s\n\tid = %s\n\tname = %s\n\tfor_ship = %s\n\ttemplate_suffix = %s\n\tis_default = %s\n\tupgrade_slot_ids = %s\n", s.ID, t.ID, dmQuote(t.ID), dmQuote(t.Name), h.Type, dmQuote(t.Suffix), def, dmList(h.SlotsFor(t)))
		if t.Description != nil {
			fmt.Fprintf(&modules, "\tdesc = %s\n", dmQuote(*t.Description))
		}
	}
	for _, m := range h.Modules {
		def := "FALSE"
		if m.Default {
			def = "TRUE"
		}
		fmt.Fprintf(&modules, "\n/datum/ship_upgrade_module/%s_%s\n\tid = %s\n\tname = %s\n\tslot = %s\n\tfor_ship = %s\n\tfor_theme = %s\n\tmap_file = %s\n\tis_default = %s\n", s.ID, m.ID, dmQuote(m.ID), dmQuote(m.Name), dmQuote(m.Slot), h.Type, dmList(m.Themes), dmQuote(m.File), def)
		if m.Description != nil {
			fmt.Fprintf(&modules, "\tdesc = %s\n", dmQuote(*m.Description))
		}
	}
	hullBytes, moduleBytes := []byte(hull), []byte(modules.String())
	if p.Crew != nil {
		for key, jobs := range p.Crew.Rosters {
			scope, err := p.crewScope(key)
			if err != nil {
				return nil, nil, err
			}
			if key == "ship" {
				hullBytes, err = rewriteCrewList(hullBytes, scope.Type, scope.Field, renderCrew(jobs, p))
			} else {
				moduleBytes, err = rewriteCrewList(moduleBytes, scope.Type, scope.Field, renderCrew(jobs, p))
			}
			if err != nil {
				return nil, nil, err
			}
		}
	}
	for _, module := range p.editedRoomCrewModules() {
		s, err := p.crewScope("module/" + module)
		if err != nil {
			return nil, nil, err
		}
		variants, err := p.roomCrewVariants(module)
		if err != nil {
			return nil, nil, err
		}
		moduleBytes, err = rewriteCrewList(moduleBytes, s.Type, roomCrewField, renderRoomCrew(variants, p))
		if err != nil {
			return nil, nil, err
		}
	}
	return p.applyGeneratedCosts(hullBytes, moduleBytes)
}

func (p *Project) availableThemesLine() string {
	ids := make([]string, 0, len(p.Hull.Themes))
	for _, theme := range p.Hull.Themes {
		ids = append(ids, theme.ID)
	}
	return "\tavailable_themes = " + dmList(ids) + "\n"
}

func permanent(path string) bool {
	for _, prefix := range []string{"/obj/docking_port", "/obj/machinery/door", "/obj/machinery/atmospherics", "/obj/structure/cable", "/obj/machinery/power", "/obj/machinery/light", "/obj/machinery/airalarm", "/obj/machinery/firealarm", "/obj/machinery/cryopod"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return mappingMarker(path)
}

// AddSlot turns the hull tiles under shape, placed with module tile 1,1 at
// origin, into an upgrade room. Tiles outside the shape keep their hull
// furniture and stay transparent in every option.
func (p *Project) AddSlot(themeIndex int, id, name string, origin util.Point, shape Footprint) error {
	if err := p.ModuleNameError(name); err != nil {
		return err
	}
	if err := ValidID(id); err != nil {
		return err
	}
	if p.slotIDUsed(id) {
		return fmt.Errorf("slot ID already exists")
	}
	if err := p.moduleIDError(id + "_basic"); err != nil {
		return err
	}
	if err := p.footprintError(shape); err != nil {
		return err
	}
	theme, err := p.roomTheme(themeIndex)
	if err != nil {
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
	max := util.Point{X: origin.X + shape.W - 1, Y: origin.Y + shape.H - 1, Z: 1}
	if origin.Z != 1 || !hull.Map.HasTile(origin) || !hull.Map.HasTile(max) {
		return fmt.Errorf("select tiles inside the hull")
	}
	rooms, err := p.roomShapes(hull.Map, theme)
	if err != nil {
		return err
	}
	room := RoomShape{Slot: id, Origin: origin, Marker: origin, Footprint: shape}
	if err = roomOverlap(room, rooms); err != nil {
		return err
	}
	if err = p.prepareRooms(&theme); err != nil {
		return err
	}
	module, target, err := p.newRoomModule(theme, id+"_basic", name, id)
	if err != nil {
		return err
	}
	module.Default = true
	if err = p.addMap(target, blankData(shape.W, shape.H)); err != nil {
		return err
	}
	dst := p.Documents[target].Map
	moveFurniture(hull.Map, dst, room, room.Footprint.Cells)
	dst.GetTile(util.Point{X: 1, Y: 1, Z: 1}).InstancesAdd(dmmap.PrefabStorage.Initial(Connector))
	hull.Map.GetTile(origin).InstancesAdd(p.markerPrefab(id, shape))
	if p.Settings == nil && theme.ID != "" {
		// Existing ships can inherit their slot list. Override only the chosen
		// theme, leaving the base and every other variant's registration intact.
		p.Hull.Themes[themeIndex].Slots = append(append([]string{}, p.Hull.SlotsFor(theme)...), id)
	} else {
		for i, t := range p.Hull.Themes {
			p.Hull.Themes[i].Slots = append([]string{}, p.Hull.SlotsFor(t)...)
		}
		p.Hull.Slots = append(p.Hull.Slots, id)
		if theme.ID != "" {
			p.Hull.Themes[themeIndex].Slots = append(p.Hull.Themes[themeIndex].Slots, id)
		}
	}
	p.Hull.Modules = append(p.Hull.Modules, module)
	// The first upgrade room turns a fixed ship into a modular one.
	p.Hull.Fixed = false
	p.reserveAnchors(hull.Map)
	p.reserveAnchors(dst)
	p.protect(hull.Map)
	p.protect(dst)
	return nil
}

// moveFurniture extracts the hull's movable content under the given module
// cells into the module map. Turfs, areas and permanent equipment stay.
func moveFurniture(hull, module *dmmap.Dmm, room RoomShape, cells map[util.Point]bool) {
	for cell, in := range cells {
		if !in {
			continue
		}
		tile := module.GetTile(util.Point{X: cell.X, Y: cell.Y, Z: 1})
		source := hull.GetTile(util.Point{X: room.Origin.X + cell.X - 1, Y: room.Origin.Y + cell.Y - 1, Z: 1})
		moved := dmmap.Instances{}
		for _, i := range source.Instances() {
			path := i.Prefab().Path()
			if !strings.HasPrefix(path, "/area/") && !strings.HasPrefix(path, "/turf/") && !permanent(path) {
				tile.InstancesAdd(i.Prefab())
				moved = append(moved, i)
			}
		}
		for _, i := range moved {
			source.InstancesRemoveByInstance(i)
		}
	}
}

func (p *Project) markerPrefab(id string, shape Footprint) *dmmprefab.Prefab {
	values := map[string]string{"key": strconv.Quote(id)}
	if !shape.IsFull() {
		values[footprintVar] = strconv.Quote(shape.String())
	}
	return p.prefab(SlotMarker, values)
}

func roomOverlap(room RoomShape, rooms map[string]RoomShape) error {
	tiles := room.Tiles()
	for slot, other := range rooms {
		if slot == room.Slot {
			continue
		}
		for tile := range other.Tiles() {
			if tiles[tile] {
				return fmt.Errorf("selection overlaps the %s room", SlotDisplayName(slot))
			}
		}
	}
	return nil
}

func (p *Project) AddModule(themeIndex int, base Module, id, name string, empty bool) error {
	if err := p.ModuleNameError(name); err != nil {
		return err
	}
	if err := ValidID(id); err != nil {
		return err
	}
	if err := p.moduleIDError(id); err != nil {
		return err
	}
	var jobs []CrewJob
	if !empty {
		var err error
		theme, themeErr := p.roomTheme(themeIndex)
		if themeErr != nil {
			return themeErr
		}
		jobs, err = p.CrewJobs(p.RoomCrewScope(base.ID, theme.ID))
		if err != nil {
			return err
		}
		for i := range jobs {
			jobs[i].ID = ""
		}
	}
	theme, err := p.roomTheme(themeIndex)
	if err != nil {
		return err
	}
	file, err := p.moduleFile(base, theme.ID)
	if err != nil {
		return err
	}
	d, err := p.document(file)
	if err != nil {
		return err
	}
	if err = p.prepareRooms(nil); err != nil {
		return err
	}
	m, target, err := p.newRoomModule(theme, id, name, base.Slot)
	if err != nil {
		return err
	}
	shape, err := p.slotShape(theme, base.Slot, d.Map)
	if err != nil {
		return err
	}
	data := RawData(d.Map)
	// Every tile has its own key, so tiles can be blanked one at a time. A copy
	// never carries content outside the room's shape.
	for coord, k := range data.Grid {
		if !empty && shape.Contains(coord) {
			continue
		}
		next := dmmdata.Prefabs{dmmap.PrefabStorage.Initial("/turf/template_noop"), dmmap.PrefabStorage.Initial("/area/template_noop")}
		for _, f := range data.Dictionary[k] {
			if mappingMarker(f.Path()) {
				next = append(next, f)
			}
		}
		data.Dictionary[k] = next
	}
	if err = p.addMap(target, data); err != nil {
		return err
	}
	p.Hull.Modules = append(p.Hull.Modules, m)
	if len(jobs) > 0 {
		return p.SetCrewJobs("module/"+id, jobs)
	}
	return nil
}

// State stores authoring changes as a single history entry across every file.
type State struct {
	PartCosts map[string]PartCosts
	Crew      *CrewConfig
	Hull      Hull
	RoomAreas []RoomArea
	Settings  *Settings
	Maps      map[string]dmmap.Dmm
}

func (p *Project) Capture() State {
	state := State{Maps: map[string]dmmap.Dmm{}}
	state.Crew = cloneCrew(p.Crew)
	state.PartCosts = cloneCostScopes(p.partCosts)
	// JSON round-trip deep-copies nested registration slices.
	state.Hull = cloneHull(p.Hull)
	state.RoomAreas = append([]RoomArea(nil), p.RoomAreas...)
	if p.Settings != nil {
		s := *p.Settings
		state.Settings = &s
	}
	for file, d := range p.Documents {
		if d.Active {
			state.Maps[file] = d.Map.Copy()
		}
	}
	return state
}
func (p *Project) Restore(state State) {
	p.partCosts = cloneCostScopes(state.PartCosts)
	p.Crew = cloneCrew(state.Crew)
	p.Hull = cloneHull(state.Hull)
	p.RoomAreas = append([]RoomArea(nil), state.RoomAreas...)
	if state.Settings != nil {
		s := *state.Settings
		p.Settings = &s
	}
	for file, d := range p.Documents {
		m, ok := state.Maps[file]
		d.Active = ok
		if ok {
			copy := m.Copy()
			*d.Map = copy
		}
	}
	for file, m := range state.Maps {
		if p.Documents[file] == nil {
			copy := m.Copy()
			p.Documents[file] = &Document{Map: &copy, Active: true}
		}
	}
	referenced := p.referencedMaps()
	for file := range p.renamedMaps {
		// A copy dropped back to a shared room is still referenced by its
		// option, so only the captured state decides whether it is live.
		if p.deletedMaps[file] {
			continue
		}
		if d := p.Documents[file]; d != nil {
			d.Active = referenced[file]
		}
	}
}
func cloneHull(h Hull) Hull {
	b, _ := json.Marshal(h)
	var copy Hull
	_ = json.Unmarshal(b, &copy)
	return copy
}

func (p *Project) Resize(theme Theme, w, h int) error {
	if w < 5 || h < 5 || w > 128 || h > 128 {
		return fmt.Errorf("canvas dimensions must be between 5 and 128")
	}
	file, err := p.Catalog.HullFile(p.Hull, theme)
	if err != nil {
		return err
	}
	d, err := p.document(file)
	if err != nil {
		return err
	}
	for _, tile := range d.Map.Tiles {
		if tile.Coord.X <= w && tile.Coord.Y <= h {
			continue
		}
		for _, i := range tile.Instances() {
			if i.Prefab().Path() != "/turf/template_noop" && i.Prefab().Path() != "/area/template_noop" {
				return fmt.Errorf("resize would remove content at %d,%d", tile.Coord.X, tile.Coord.Y)
			}
		}
	}
	oldW, oldH := d.Map.MaxX, d.Map.MaxY
	d.Map.SetMapSize(w, h, 1)
	for _, tile := range d.Map.Tiles {
		if tile.Coord.X > oldW || tile.Coord.Y > oldH {
			tile.InstancesSet(dmmdata.Prefabs{dmmap.PrefabStorage.Initial("/turf/template_noop"), dmmap.PrefabStorage.Initial("/area/template_noop")})
		}
	}
	p.protect(d.Map)
	return nil
}
