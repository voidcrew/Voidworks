package ship

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"sdmm/internal/dmapi/dmmap/dmmdata"
)

// Handwritten variants are registered as subtypes of the ship's own theme
// family, matching the fleet's own /datum/ship_theme/<ship>/<variant> layout.
func roomThemeType(shipID, id string) string { return "/datum/ship_theme/" + shipID + "/" + id }

// moduleThemeTarget records where a room option's availability list is written.
// Fleet ships declare one list on a shared parent, so an edit that applies to
// every option of the ship rewrites that line instead of overriding each block.
type moduleThemeTarget struct{ own, shared nameTarget }

type OptionStatus struct {
	ModuleID, Slot             string
	Available, Forked, Differs bool
}

type ThemeInfo struct {
	ID, Name  string
	Default   bool
	HullFile  string // project-relative
	Slots     []string
	Inherited bool // the room list comes from the hull
	Options   []OptionStatus
	Price     string
	Crew      int // seats across the variant's own roster
}

func themeIDs(h Hull) []string {
	ids := make([]string, 0, len(h.Themes))
	for _, t := range h.Themes {
		ids = append(ids, t.ID)
	}
	return ids
}

func moduleThemesOf(h Hull, id string) ([]string, bool) {
	for _, m := range h.Modules {
		if m.ID == id {
			return m.Themes, true
		}
	}
	return nil, false
}

func (p *Project) themeIndex(id string) int {
	for i, t := range p.Hull.Themes {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (p *Project) theme(id string) (Theme, error) {
	if i := p.themeIndex(id); i >= 0 {
		return p.Hull.Themes[i], nil
	}
	return Theme{}, fmt.Errorf("choose a ship theme")
}

func (p *Project) inBaseThemes(id string) bool {
	if p.rooms == nil {
		return false
	}
	for _, t := range p.rooms.base.Themes {
		if t.ID == id {
			return true
		}
	}
	return false
}

// liveDocument reports the map a variant currently uses, treating a map this
// session retired as absent rather than reopening it from disk.
func (p *Project) liveDocument(file string) (*Document, error) {
	if d := p.Documents[file]; d != nil && !d.Active {
		return nil, nil
	}
	return p.existingDocument(file)
}

func (p *Project) sharedModuleFile(m Module) (string, error) {
	return Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, m.File))
}

func (p *Project) themedModuleFile(m Module, themeID string) (string, error) {
	return Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, strings.TrimSuffix(m.File, ".dmm")+"_"+themeID+".dmm"))
}

func (p *Project) hasSharedModuleMap(m Module) (bool, error) {
	file, err := p.sharedModuleFile(m)
	if err != nil {
		return false, err
	}
	d, err := p.liveDocument(file)
	return d != nil, err
}

// nextThemeSuffix names a variant's hull map. The fleet numbers them by letter
// (scarab_a, scarab_b, ...); anything else falls back to the variant ID.
func (p *Project) nextThemeSuffix(id string) (string, error) {
	used := map[string]bool{}
	if p.Hull.Suffix != "" {
		used[p.Hull.Suffix] = true
	}
	for _, t := range p.Hull.Themes {
		if t.Suffix != "" {
			used[t.Suffix] = true
		}
	}
	stems, letters := map[string]byte{}, 0
	for suffix := range used {
		cut := strings.LastIndex(suffix, "_")
		if cut <= 0 || len(suffix)-cut != 2 || suffix[cut+1] < 'a' || suffix[cut+1] > 'z' {
			continue
		}
		letters++
		if suffix[cut+1] > stems[suffix[:cut]] {
			stems[suffix[:cut]] = suffix[cut+1]
		}
	}
	candidate := p.mapFileID() + "_" + id
	if letters > 1 && len(stems) == 1 {
		for stem, last := range stems {
			for c := last + 1; c <= 'z'; c++ {
				if next := stem + "_" + string(c); !used[next] {
					candidate = next
					break
				}
			}
		}
	}
	stem := candidate
	for n := 2; ; n++ {
		taken := used[candidate]
		if !taken {
			file, err := p.Catalog.HullFile(p.Hull, Theme{ID: id, Suffix: candidate})
			if err != nil {
				return "", err
			}
			if d := p.Documents[file]; d != nil && d.Active {
				taken = true
			} else if _, err := os.Stat(file); err == nil {
				taken = true
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		if !taken {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s_%d", stem, n)
	}
}

// optionGaps lists the rooms a variant cannot fill. Edits are checked against
// the gaps a ship already had, so an unrelated change is never blocked by one.
func optionGaps(h Hull) map[string]string {
	gaps := map[string]string{}
	themes := h.Themes
	if len(themes) == 0 {
		themes = []Theme{{Name: h.Name}}
	}
	for _, t := range themes {
		label := t.Name
		if label == "" {
			label = h.Name
		}
		for _, slot := range h.SlotsFor(t) {
			offered, defaulted := 0, true
			for _, m := range h.Modules {
				if m.Slot != slot {
					continue
				}
				if m.Available(t.ID) {
					offered++
				} else if m.Default {
					defaulted = false
				}
			}
			if offered == 0 {
				gaps[t.ID+"\x00"+slot] = fmt.Sprintf("%s would have no option for the %s module", label, SlotDisplayName(slot))
			} else if !defaulted {
				gaps[t.ID+"\x00"+slot+"\x00default"] = fmt.Sprintf("%s would lose the default option of the %s module", label, SlotDisplayName(slot))
			}
		}
	}
	return gaps
}

func optionGapError(before, after Hull) error {
	had := optionGaps(before)
	gaps := optionGaps(after)
	keys := make([]string, 0, len(gaps))
	for key := range gaps {
		if _, ok := had[key]; !ok {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return fmt.Errorf("%s", gaps[keys[0]])
}

// themeRegistrationFile picks where new variant definitions are written: beside
// the ship's existing ones, else beside its room options, else the workshop file.
func (p *Project) themeRegistrationFile() (string, error) {
	if p.rooms.themeFile != "" {
		return p.rooms.themeFile, nil
	}
	file := ""
	for _, t := range p.Hull.Themes {
		if typePath, err := p.componentType("/datum/ship_theme/", t.ID); err == nil {
			if found, err := p.roomTypeFile(typePath); err == nil {
				file = found
				break
			}
		}
	}
	for _, m := range p.Hull.Modules {
		if file != "" {
			break
		}
		if typePath, err := p.componentType("/datum/ship_upgrade_module/", m.ID); err == nil {
			if found, err := p.roomTypeFile(typePath); err == nil {
				file = found
			}
		}
	}
	if file == "" {
		file = p.rooms.code
	}
	if err := p.roomSource(file); err != nil {
		return "", err
	}
	p.rooms.themeFile = file
	return file, nil
}

// generatedSourceFile is where a component created this session is registered.
func (p *Project) generatedSourceFile(scope string) string {
	if strings.HasPrefix(scope, "theme/") && p.rooms.themeFile != "" {
		return p.rooms.themeFile
	}
	return p.rooms.code
}

// prepareAvailableThemes tracks the hull's variant list so save can rewrite it.
func (p *Project) prepareAvailableThemes() error {
	if p.rooms.themeList.typePath != "" {
		return nil
	}
	file, err := p.roomTypeFile(p.Hull.Type)
	if err != nil {
		return err
	}
	if err = p.roomSource(file); err != nil {
		return err
	}
	ids := themeIDs(p.rooms.base)
	if _, err = rewriteLiteralList(p.rooms.sources[file].Before, p.Hull.Type, "available_themes", dmList(ids)); err != nil {
		return err
	}
	raw, _, explicit, err := sourceCostList(p.rooms.sources[file].Before, p.Hull.Type, "available_themes")
	if err != nil {
		return err
	}
	// The hull lists its variants in its own order, which the catalog sorts for
	// display. Keep the source's order and only check that it holds the same set.
	p.rooms.themeOrder = ids
	if explicit {
		current, err := stringList(string(dmSourceMask([]byte(raw), false)))
		if err != nil {
			return err
		}
		if !sameIDs(current, ids) {
			return fmt.Errorf("%s theme list differs from the loaded environment; reload the environment first", p.Hull.Name)
		}
		p.rooms.themeOrder = current
	}
	p.rooms.themeList = nameTarget{file, p.Hull.Type}
	return nil
}

func sameIDs(a, b []string) bool {
	first, second := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(first)
	sort.Strings(second)
	return dmList(first) == dmList(second)
}

// sharedThemeDeclaration finds the type that actually assigns for_theme, so an
// inherited list is edited once rather than overridden on every room option.
func (p *Project) sharedThemeDeclaration(own nameTarget) (nameTarget, error) {
	if _, _, explicit, err := sourceCostList(p.rooms.sources[own.file].Before, own.typePath, "for_theme"); err != nil || explicit {
		return own, nil
	}
	path := own.typePath
	for {
		cut := strings.LastIndex(path, "/")
		if cut <= 0 {
			return own, nil
		}
		path = path[:cut]
		if len(path) <= len("/datum/ship_upgrade_module") {
			return own, nil
		}
		obj := p.Dme.Objects[path]
		if obj == nil || obj.Vars.ValueV("for_ship", "") != p.Hull.Type {
			continue
		}
		file, err := p.roomTypeFile(path)
		if err != nil {
			return own, nil
		}
		if err = p.roomSource(file); err != nil {
			return nameTarget{}, err
		}
		if _, _, explicit, err := sourceCostList(p.rooms.sources[file].Before, path, "for_theme"); err == nil && explicit {
			return nameTarget{file, path}, nil
		}
	}
}

func (p *Project) prepareModuleThemes() error {
	if p.rooms.moduleThemes == nil {
		p.rooms.moduleThemes = map[string]moduleThemeTarget{}
	}
	for _, m := range p.rooms.base.Modules {
		if _, ok := p.rooms.moduleThemes[m.ID]; ok {
			continue
		}
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
		own := nameTarget{file, typePath}
		shared, err := p.sharedThemeDeclaration(own)
		if err != nil {
			return err
		}
		p.rooms.moduleThemes[m.ID] = moduleThemeTarget{own, shared}
	}
	return nil
}

// prepareThemeDefinition tracks one handwritten variant block for rewriting.
func (p *Project) prepareThemeDefinition(id string) error {
	if _, ok := p.rooms.themeTargets[id]; ok || !p.inBaseThemes(id) {
		return nil
	}
	typePath, err := p.componentType("/datum/ship_theme/", id)
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
	if p.rooms.themeTargets == nil {
		p.rooms.themeTargets = map[string]nameTarget{}
	}
	p.rooms.themeTargets[id] = nameTarget{file, typePath}
	return nil
}

// captureThemeJobs copies the crew a new variant starts with. A themed hull
// keeps its jobs on the variant, so an empty block would leave it crewless.
func (p *Project) captureThemeJobs(base Theme, id string) error {
	if base.ID == "" {
		return nil
	}
	if p.rooms.themeJobs == nil {
		p.rooms.themeJobs = map[string]string{}
	}
	if jobs, ok := p.rooms.themeJobs[base.ID]; ok {
		p.rooms.themeJobs[id] = jobs
		return nil
	}
	typePath, err := p.componentType("/datum/ship_theme/", base.ID)
	if err != nil {
		return nil
	}
	file, err := p.roomTypeFile(typePath)
	if err != nil {
		return nil
	}
	if err = p.roomSource(file); err != nil {
		return err
	}
	raw, found, explicit, err := sourceCostList(p.rooms.sources[file].Before, typePath, "job_slots")
	if err != nil || !found || !explicit {
		return nil
	}
	p.rooms.themeJobs[id] = raw
	return nil
}

// AddTheme creates a ship variant from an existing one. The variant always gets
// its own hull map. Room options stay shared unless copyRooms is set, so both
// variants show the same rooms until one is deliberately given its own copy.
func (p *Project) AddTheme(baseIndex int, id, name string, copyRooms bool) error {
	if err := p.ThemeNameError(name); err != nil {
		return err
	}
	if err := ValidID(id); err != nil {
		return err
	}
	if p.themeIndex(id) >= 0 {
		return fmt.Errorf("theme ID already exists")
	}
	base, err := p.roomTheme(baseIndex)
	if err != nil {
		return err
	}
	shipID, err := p.roomID()
	if err != nil {
		return err
	}
	// The first variant of an unthemed ship is that ship: it keeps the hull's
	// own map and becomes the default, as the fleet's themed hulls do.
	first := len(p.Hull.Themes) == 0
	theme := Theme{ID: id, Name: name, Suffix: p.Hull.Suffix, Slots: append([]string{}, p.Hull.SlotsFor(base)...), Default: first}
	if !first {
		if theme.Suffix, err = p.nextThemeSuffix(id); err != nil {
			return err
		}
	}
	if p.Settings == nil {
		if err = p.prepareRooms(nil); err != nil {
			return err
		}
		if p.Dme.Objects[roomThemeType(shipID, id)] != nil {
			return fmt.Errorf("theme definition already exists for this identifier")
		}
		if _, err = p.themeRegistrationFile(); err != nil {
			return err
		}
		if err = p.prepareAvailableThemes(); err != nil {
			return err
		}
		if err = p.prepareModuleThemes(); err != nil {
			return err
		}
		if err = p.captureThemeJobs(base, id); err != nil {
			return err
		}
	}
	type clone struct {
		file string
		data *dmmdata.DmmData
	}
	var clones []clone
	if !first {
		file, err := p.Catalog.HullFile(p.Hull, base)
		if err != nil {
			return err
		}
		hull, err := p.document(file)
		if err != nil {
			return err
		}
		target, err := p.Catalog.HullFile(p.Hull, theme)
		if err != nil {
			return err
		}
		clones = append(clones, clone{target, RawData(hull.Map)})
	}
	for _, m := range p.Hull.Modules {
		if !m.Available(base.ID) {
			continue
		}
		if !copyRooms {
			// Options with no shared map (the Scarab family) must be copied
			// anyway, or the new variant would load nothing into the room.
			shared, err := p.hasSharedModuleMap(m)
			if err != nil {
				return err
			}
			if shared {
				continue
			}
		}
		file, err := p.moduleFile(m, base.ID)
		if err != nil {
			return err
		}
		d, err := p.document(file)
		if err != nil {
			return err
		}
		target, err := p.themedModuleFile(m, id)
		if err != nil {
			return err
		}
		clones = append(clones, clone{target, RawData(d.Map)})
	}
	added := []string{}
	for _, c := range clones {
		if err = p.addMap(c.file, c.data); err != nil {
			for _, file := range added {
				delete(p.Documents, file)
			}
			return err
		}
		added = append(added, c.file)
	}
	for i, m := range p.Hull.Modules {
		if m.Available(base.ID) {
			p.Hull.Modules[i].Themes = append(append([]string{}, m.Themes...), id)
		}
	}
	p.Hull.Themes = append(p.Hull.Themes, theme)
	for _, module := range p.Hull.Modules {
		if !module.Available(base.ID) || !p.SupportsRoomCrewVariants() {
			continue
		}
		jobs, e := p.CrewJobs(p.RoomCrewScope(module.ID, base.ID))
		if e != nil {
			return e
		}
		if len(jobs) == 0 && !p.hasRoomCrewVariant(module.ID, base.ID) {
			continue
		}
		for i := range jobs {
			jobs[i].ID = ""
		}
		if e = p.SetCrewJobs(roomCrewScope(module.ID, id), jobs); e != nil {
			return e
		}
	}
	if p.Crew != nil {
		if jobs, ok := p.Crew.Rosters["theme/"+base.ID]; ok {
			jobs = CloneCrewJobs(jobs)
			for i := range jobs {
				jobs[i].ID = ""
			}
			if err = p.SetCrewJobs("theme/"+id, jobs); err != nil {
				return err
			}
		}
	}
	return nil
}

// RemoveTheme deletes a ship variant with its hull map and its private copies
// of any room option. Options shared with the rest of the ship are kept.
func (p *Project) RemoveTheme(id string) error {
	i := p.themeIndex(id)
	if i < 0 {
		return fmt.Errorf("ship theme no longer exists")
	}
	theme := p.Hull.Themes[i]
	if len(p.Hull.Themes) < 2 {
		return fmt.Errorf("%s is the only theme; a ship needs one", theme.Name)
	}
	if theme.Default {
		return fmt.Errorf("%s is the default theme; choose another default theme first", theme.Name)
	}
	if p.Settings == nil {
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		if err := p.prepareThemeDefinition(id); err != nil {
			return err
		}
		if target, ok := p.rooms.themeTargets[id]; ok {
			_, removed, err := removeDefinitions(p.rooms.sources[target.file].Before, []string{target.typePath})
			if err != nil {
				return err
			}
			if !removed[target.typePath] {
				return fmt.Errorf("cannot locate the definition of %s; reload the environment first", theme.Name)
			}
		}
		if err := p.prepareAvailableThemes(); err != nil {
			return err
		}
		if err := p.prepareModuleThemes(); err != nil {
			return err
		}
	}
	retire := []string{}
	file, err := p.Catalog.HullFile(p.Hull, theme)
	if err != nil {
		return err
	}
	retire = append(retire, file)
	for _, m := range p.Hull.Modules {
		themed, err := p.themedModuleFile(m, id)
		if err != nil {
			return err
		}
		retire = append(retire, themed)
	}
	for _, module := range p.Hull.Modules {
		if p.hasRoomCrewVariant(module.ID, id) {
			if err = p.clearRoomCrewVariant(module.ID, id); err != nil {
				return err
			}
		}
	}
	p.Hull.Themes = append(p.Hull.Themes[:i], p.Hull.Themes[i+1:]...)
	for j, m := range p.Hull.Modules {
		if Contains(m.Themes, id) {
			p.Hull.Modules[j].Themes = remove(m.Themes, id)
		}
	}
	// Maps another variant still uses, including a shared base hull, stay put.
	referenced := p.referencedMaps()
	for _, file := range retire {
		if referenced[file] {
			continue
		}
		if err = p.retireMap(file); err != nil {
			return err
		}
	}
	if p.Crew != nil {
		delete(p.Crew.Rosters, "theme/"+id)
		for _, variants := range p.Crew.ModuleThemes {
			delete(variants, id)
		}
	}
	delete(p.partCosts, "theme/"+id)
	return nil
}

// SetDefaultTheme picks the variant new owners get for free.
func (p *Project) SetDefaultTheme(id string) error {
	i := p.themeIndex(id)
	if i < 0 {
		return fmt.Errorf("choose a ship theme")
	}
	if p.Hull.Themes[i].Default {
		return nil
	}
	if p.Settings == nil {
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		for _, t := range p.Hull.Themes {
			if err := p.prepareThemeDefinition(t.ID); err != nil {
				return err
			}
		}
	}
	for j := range p.Hull.Themes {
		p.Hull.Themes[j].Default = j == i
	}
	return nil
}

// SetThemeSlots overrides which rooms a variant offers. A nil list puts the
// variant back on the hull's own room list.
func (p *Project) SetThemeSlots(themeID string, slots []string) error {
	i := p.themeIndex(themeID)
	if i < 0 {
		return fmt.Errorf("choose a ship theme")
	}
	seen := map[string]bool{}
	for _, slot := range slots {
		if !p.slotIDUsed(slot) {
			return fmt.Errorf("this ship has no %s module", SlotDisplayName(slot))
		}
		if seen[slot] {
			return fmt.Errorf("%s is listed twice", SlotDisplayName(slot))
		}
		seen[slot] = true
	}
	var next []string
	if slots != nil {
		next = append([]string{}, slots...)
	}
	if reflect.DeepEqual(p.Hull.Themes[i].Slots, next) {
		return nil
	}
	if p.Settings == nil {
		theme := p.Hull.Themes[i]
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		if p.inBaseThemes(themeID) {
			if err := p.prepareRooms(&theme); err != nil {
				return err
			}
		}
	}
	before := cloneHull(p.Hull)
	p.Hull.Themes[i].Slots = next
	if err := optionGapError(before, p.Hull); err != nil {
		p.Hull = before
		return err
	}
	return nil
}

// SetModuleThemes chooses which variants offer one room option.
func (p *Project) SetModuleThemes(moduleID string, themes []string) error {
	i := p.moduleIndex(moduleID)
	if i < 0 {
		return fmt.Errorf("module option no longer exists")
	}
	seen := map[string]bool{}
	for _, id := range themes {
		if p.themeIndex(id) < 0 {
			return fmt.Errorf("this ship has no %q theme", id)
		}
		if seen[id] {
			return fmt.Errorf("the %s theme is listed twice", p.Hull.Themes[p.themeIndex(id)].Name)
		}
		seen[id] = true
	}
	var next []string
	if themes != nil {
		next = append([]string{}, themes...)
	}
	if reflect.DeepEqual(p.Hull.Modules[i].Themes, next) {
		return nil
	}
	if p.Settings == nil {
		if err := p.prepareRooms(nil); err != nil {
			return err
		}
		if err := p.prepareModuleThemes(); err != nil {
			return err
		}
	}
	before := cloneHull(p.Hull)
	p.Hull.Modules[i].Themes = next
	if err := optionGapError(before, p.Hull); err != nil {
		p.Hull = before
		return err
	}
	return nil
}

// ForkModuleForTheme gives one variant its own copy of a room option, taken
// from the map that variant shows today, so the two can be edited apart.
func (p *Project) ForkModuleForTheme(moduleID, themeID string) error {
	i := p.moduleIndex(moduleID)
	if i < 0 {
		return fmt.Errorf("module option no longer exists")
	}
	theme, err := p.theme(themeID)
	if err != nil {
		return err
	}
	m := p.Hull.Modules[i]
	if !m.Available(themeID) {
		return fmt.Errorf("%s is not offered in the %s theme", m.Name, theme.Name)
	}
	target, err := p.themedModuleFile(m, themeID)
	if err != nil {
		return err
	}
	if d := p.Documents[target]; d != nil && !d.Active && p.deletedMaps[target] {
		delete(p.deletedMaps, target)
		d.Active = true
		return nil
	}
	source, err := p.moduleFile(m, themeID)
	if err != nil {
		return err
	}
	if source == target {
		return fmt.Errorf("%s already has its own module for the %s theme", m.Name, theme.Name)
	}
	d, err := p.document(source)
	if err != nil {
		return err
	}
	return p.addMap(target, RawData(d.Map))
}

// UnforkModuleForTheme drops a variant's private copy of a room option so it
// shows the shared room again.
func (p *Project) UnforkModuleForTheme(moduleID, themeID string) error {
	i := p.moduleIndex(moduleID)
	if i < 0 {
		return fmt.Errorf("module option no longer exists")
	}
	theme, err := p.theme(themeID)
	if err != nil {
		return err
	}
	m := p.Hull.Modules[i]
	target, err := p.themedModuleFile(m, themeID)
	if err != nil {
		return err
	}
	d, err := p.liveDocument(target)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("%s already uses the shared module in the %s theme", m.Name, theme.Name)
	}
	shared, err := p.hasSharedModuleMap(m)
	if err != nil {
		return err
	}
	if !shared {
		return fmt.Errorf("%s has no shared module to fall back on; every theme keeps its own copy", m.Name)
	}
	d.Active = false
	if p.deletedMaps == nil {
		p.deletedMaps = map[string]bool{}
	}
	p.deletedMaps[target] = true
	return nil
}

// ModuleThemeStatus reports whether a variant keeps its own copy of a room
// option and whether that copy still matches the shared room.
func (p *Project) ModuleThemeStatus(moduleID, themeID string) (bool, bool, error) {
	i := p.moduleIndex(moduleID)
	if i < 0 {
		return false, false, fmt.Errorf("module option no longer exists")
	}
	if _, err := p.theme(themeID); err != nil {
		return false, false, err
	}
	m := p.Hull.Modules[i]
	target, err := p.themedModuleFile(m, themeID)
	if err != nil {
		return false, false, err
	}
	themed, err := p.liveDocument(target)
	if err != nil || themed == nil {
		return false, false, err
	}
	file, err := p.sharedModuleFile(m)
	if err != nil {
		return true, false, err
	}
	shared, err := p.liveDocument(file)
	if err != nil || shared == nil {
		return true, false, err
	}
	return true, fingerprint(themed.Map) == fingerprint(shared.Map), nil
}

// ThemeSummary describes one variant for the variant editor.
func (p *Project) ThemeSummary(themeID string) (ThemeInfo, error) {
	theme, err := p.theme(themeID)
	if err != nil {
		return ThemeInfo{}, err
	}
	file, err := p.Catalog.HullFile(p.Hull, theme)
	if err != nil {
		return ThemeInfo{}, err
	}
	relative, err := filepath.Rel(p.Catalog.Root, file)
	if err != nil {
		return ThemeInfo{}, err
	}
	info := ThemeInfo{
		ID: theme.ID, Name: theme.Name, Default: theme.Default,
		HullFile:  filepath.ToSlash(relative),
		Slots:     append([]string{}, p.Hull.SlotsFor(theme)...),
		Inherited: theme.Slots == nil,
	}
	for _, m := range p.Hull.Modules {
		option := OptionStatus{ModuleID: m.ID, Slot: m.Slot, Available: m.Available(theme.ID)}
		if option.Available {
			forked, same, err := p.ModuleThemeStatus(m.ID, theme.ID)
			if err != nil {
				return ThemeInfo{}, err
			}
			option.Forked, option.Differs = forked, forked && !same
		}
		info.Options = append(info.Options, option)
	}
	cost, err := p.DisplayPartCosts("theme/" + theme.ID)
	if err != nil {
		return ThemeInfo{}, err
	}
	info.Price = cost.Summary()
	jobs, err := p.CrewJobs("theme/" + theme.ID)
	if err != nil {
		return ThemeInfo{}, err
	}
	for _, job := range jobs {
		info.Crew += job.Slots
	}
	return info, nil
}

// themeDefinition renders a handwritten registration block for a new variant.
func (p *Project) themeDefinition(t Theme) string {
	id, _ := p.roomID()
	def := "FALSE"
	if t.Default {
		def = "TRUE"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n\tid = %s\n\tname = %s\n\tfor_ship = %s\n\ttemplate_suffix = %s\n\tis_default = %s\n", roomThemeType(id, t.ID), dmQuote(t.ID), dmQuote(t.Name), p.Hull.Type, dmQuote(t.Suffix), def)
	if t.Description != nil {
		fmt.Fprintf(&b, "\tdesc = %s\n", dmQuote(*t.Description))
	}
	if t.Slots != nil {
		fmt.Fprintf(&b, "\tupgrade_slot_ids = %s\n", dmList(t.Slots))
	}
	if jobs := p.rooms.themeJobs[t.ID]; jobs != "" {
		fmt.Fprintf(&b, "\tjob_slots = %s\n", jobs)
	}
	return b.String()
}

// themeChanges applies every variant edit to the handwritten sources gathered
// in contents, which start as the session's original file snapshots.
func (p *Project) themeChanges(contents map[string][]byte) error {
	for id, target := range p.rooms.themeTargets {
		i := p.themeIndex(id)
		if i < 0 {
			var err error
			if contents[target.file], _, err = removeDefinitions(contents[target.file], []string{target.typePath}); err != nil {
				return err
			}
			continue
		}
		before := false
		for _, t := range p.rooms.base.Themes {
			if t.ID == id {
				before = t.Default
			}
		}
		if before == p.Hull.Themes[i].Default {
			continue
		}
		var err error
		if contents[target.file], err = rewriteFlag(contents[target.file], target.typePath, "is_default", p.Hull.Themes[i].Default); err != nil {
			return err
		}
	}
	if target := p.rooms.themeList; target.typePath != "" {
		kept, seen := []string{}, map[string]bool{}
		for _, id := range p.rooms.themeOrder {
			if p.themeIndex(id) >= 0 {
				kept, seen[id] = append(kept, id), true
			}
		}
		for _, id := range themeIDs(p.Hull) {
			if !seen[id] {
				kept = append(kept, id)
			}
		}
		if dmList(kept) != dmList(p.rooms.themeOrder) {
			var err error
			if contents[target.file], err = rewriteLiteralList(contents[target.file], target.typePath, "available_themes", dmList(kept)); err != nil {
				return err
			}
		}
	}
	// One inherited for_theme line covers every option of a fleet ship; only
	// an edit that leaves them disagreeing overrides the individual blocks.
	uniform := map[nameTarget]bool{}
	values := map[nameTarget][]string{}
	for _, m := range p.Hull.Modules {
		target, ok := p.rooms.moduleThemes[m.ID]
		if !ok {
			continue
		}
		if value, seen := values[target.shared]; !seen {
			values[target.shared], uniform[target.shared] = m.Themes, true
		} else if !reflect.DeepEqual(value, m.Themes) {
			uniform[target.shared] = false
		}
	}
	for _, m := range p.Hull.Modules {
		target, ok := p.rooms.moduleThemes[m.ID]
		if !ok {
			continue
		}
		before, _ := moduleThemesOf(p.rooms.base, m.ID)
		if reflect.DeepEqual(before, m.Themes) {
			continue
		}
		write := target.own
		if target.shared != target.own && uniform[target.shared] {
			write = target.shared
		}
		value := "null"
		if m.Themes != nil {
			value = dmList(m.Themes)
		}
		var err error
		if contents[write.file], err = rewriteLiteralList(contents[write.file], write.typePath, "for_theme", value); err != nil {
			return err
		}
	}
	var definitions strings.Builder
	for _, t := range p.Hull.Themes {
		if !p.inBaseThemes(t.ID) {
			definitions.WriteString(p.themeDefinition(t))
		}
	}
	if definitions.Len() > 0 {
		file, err := p.themeRegistrationFile()
		if err != nil {
			return err
		}
		contents[file] = append(contents[file], definitions.String()...)
	}
	return nil
}
