package ship

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Crew stores only rosters explicitly edited in the workshop. Unedited variants
// keep the game's inheritance rules. Item overrides are local outfit subtypes.
type CrewConfig struct {
	Rosters map[string][]CrewJob
	// A present module entry replaces its complete per-variant override list.
	// An empty entry means every variant uses the shared roster again.
	ModuleThemes map[string]map[string][]CrewJob `json:",omitempty"`
}
type CrewJob struct {
	ID, Name, Outfit, BaseOutfit, Category string
	Slots                                  int
	Officer                                bool
	Equipment                              map[string]string
	Backpack, Belt                         map[string]int
	Extra                                  map[string]string
}
type CrewScope struct{ ID, Name, Type, Field string }

var CrewCategories = []string{"Command", "Security", "Engineering", "Medical", "Science", "Cargo", "Service", "Assistant"}

type EquipmentSlot struct {
	ID, Name string
	Flag     int
}

var EquipmentSlots = []EquipmentSlot{
	{"uniform", "Uniform", 2}, {"suit", "Outer suit", 1}, {"head", "Head", 64},
	{"mask", "Mask", 32}, {"glasses", "Glasses", 8}, {"ears", "Ears", 16},
	{"neck", "Neck", 4096}, {"gloves", "Gloves", 4}, {"shoes", "Shoes", 128},
	{"back", "Back", 1024}, {"belt", "Belt", 512}, {"id", "ID card", 256},
	{"suit_store", "Suit storage", 0}, {"l_pocket", "Left pocket", 0}, {"r_pocket", "Right pocket", 0},
	{"l_hand", "Left hand", 0}, {"r_hand", "Right hand", 0}, {"accessory", "Accessory", 0}, {"box", "Survival box", 0},
}

func (p *Project) baseCrewScopes() []CrewScope {
	scopes := []CrewScope{{"ship", "Ship crew", p.Hull.Type, "job_slots"}}
	id, _ := p.roomID()
	themes, modules := map[string]string{}, map[string]string{}
	for path, object := range p.Dme.Objects {
		if strings.HasPrefix(path, "/datum/ship_theme/") && object.Vars.ValueV("for_ship", "") == p.Hull.Type {
			themes[text(object.Vars, "id")] = path
		} else if strings.HasPrefix(path, "/datum/ship_upgrade_module/") && object.Vars.ValueV("for_ship", "") == p.Hull.Type {
			modules[text(object.Vars, "id")] = path
		}
	}
	for _, t := range p.Hull.Themes {
		path := "/datum/ship_theme/" + id + "_" + t.ID
		if p.Settings == nil {
			path = roomThemeType(id, t.ID)
		}
		if existing := themes[t.ID]; existing != "" {
			path = existing
		}
		scopes = append(scopes, CrewScope{"theme/" + t.ID, "Theme: " + t.Name, path, "job_slots"})
	}
	for _, m := range p.Hull.Modules {
		path := "/datum/ship_upgrade_module/" + id + "_" + m.ID
		if p.Settings == nil {
			path = roomModuleType(id, m.ID)
		}
		if existing := modules[m.ID]; existing != "" {
			path = existing
		}
		scopes = append(scopes, CrewScope{"module/" + m.ID, "Module option: " + m.Name, path, "job_slots_add"})
	}
	return scopes
}

func (p *Project) CrewScopes() []CrewScope {
	scopes := p.baseCrewScopes()
	for _, m := range p.Hull.Modules {
		for _, t := range p.Hull.Themes {
			if p.hasRoomCrewVariant(m.ID, t.ID) {
				s, _ := p.crewScope(roomCrewScope(m.ID, t.ID))
				scopes = append(scopes, s)
			}
		}
	}
	return scopes
}

// CrewScopesForTheme lists the rosters that can contribute crew to this variant.
// Keep CrewScopes unfiltered for saving and validating the complete project.
func (p *Project) CrewScopesForTheme(theme Theme) []CrewScope {
	available := map[string]bool{"ship": true}
	if theme.ID != "" {
		available["theme/"+theme.ID] = true
	}
	for _, m := range p.Hull.Modules {
		if m.Available(theme.ID) && Contains(p.Hull.SlotsFor(theme), m.Slot) {
			available["module/"+m.ID] = true
		}
	}
	var scopes []CrewScope
	for _, scope := range p.baseCrewScopes() {
		if available[scope.ID] {
			if module, _ := RoomCrewIDs(scope.ID); module != "" {
				scope.ID = p.RoomCrewScope(module, theme.ID)
				if _, variant := RoomCrewIDs(scope.ID); variant != "" {
					scope.Field = roomCrewField
				}
			}
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

func (p *Project) crewScope(id string) (CrewScope, error) {
	if module, theme := RoomCrewIDs(id); theme != "" {
		if p.themeIndex(theme) < 0 {
			return CrewScope{}, fmt.Errorf("crew theme no longer exists")
		}
		s, err := p.crewScope("module/" + module)
		s.ID, s.Field = id, roomCrewField
		return s, err
	}
	for _, s := range p.baseCrewScopes() {
		if s.ID == id {
			return s, nil
		}
	}
	return CrewScope{}, fmt.Errorf("crew scope no longer exists")
}
func (p *Project) crewPaths() (string, string) {
	id := p.fileID()
	meta, _ := Inside(p.Catalog.Root, "voidcrew/mapping/ship_projects/"+id+".crew.json")
	code, _ := Inside(p.Catalog.Root, "voidcrew/mapping/ship_crew/"+id+".dm")
	return meta, code
}
func (p *Project) crewBytes() []byte {
	if p.Crew == nil {
		return nil
	}
	b, _ := json.MarshalIndent(p.Crew, "", "  ")
	return append(b, '\n')
}
func cloneCrew(c *CrewConfig) *CrewConfig {
	if c == nil {
		return nil
	}
	b, _ := json.Marshal(c)
	var v CrewConfig
	_ = json.Unmarshal(b, &v)
	return &v
}
func CloneCrewJobs(j []CrewJob) []CrewJob {
	b, _ := json.Marshal(j)
	var v []CrewJob
	_ = json.Unmarshal(b, &v)
	return v
}

func (p *Project) openCrew() error {
	meta, code := p.crewPaths()
	b, e := os.ReadFile(meta)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var config CrewConfig
	if e = json.Unmarshal(b, &config); e != nil {
		return e
	}
	if config.Rosters == nil {
		return fmt.Errorf("invalid crew project: %s", meta)
	}
	p.Crew = &config
	// Generated outfits belong to this project, but never overwrite manual edits.
	actual, e := os.ReadFile(code)
	if e != nil {
		return e
	}
	if !sameDMSource(actual, p.crewOutfits()) {
		return fmt.Errorf("crew outfits were edited outside the workshop: %s; reload the matching crew project before editing", code)
	}
	if p.Settings == nil {
		for scope, old := range p.editedCrewRosters() {
			s, e := p.crewScope(scope)
			if e != nil {
				return e
			}
			jobs, e := p.readCrew(s)
			if e != nil {
				return e
			}
			for i := range jobs {
				for _, j := range old {
					if jobs[i].Outfit == p.crewOutfitPath(j) && j.ID != "" {
						jobs[i].ID, jobs[i].BaseOutfit, jobs[i].Equipment, jobs[i].Backpack, jobs[i].Belt = j.ID, j.BaseOutfit, j.Equipment, j.Backpack, j.Belt
					}
				}
			}
			if module, theme := RoomCrewIDs(scope); theme != "" {
				config.ModuleThemes[module][theme] = jobs
			} else {
				config.Rosters[scope] = jobs
			}
		}
	}
	p.savedCrew = p.crewBytes()
	for _, path := range []string{meta, code, p.Dme.RootFile} {
		if _, ok := p.files[path]; !ok {
			if e = p.track(path); e != nil {
				return e
			}
		}
	}
	return nil
}
func (p *Project) readCrew(s CrewScope) ([]CrewJob, error) {
	if module, theme := RoomCrewIDs(s.ID); theme != "" {
		rosters, err := p.readRoomCrew(module)
		if jobs, own := rosters[theme]; own || err != nil {
			return CloneCrewJobs(jobs), err
		}
		base, err := p.crewScope("module/" + module)
		if err != nil {
			return nil, err
		}
		return p.readCrew(base)
	}
	if p.Settings != nil && s.ID == "ship" {
		j := []CrewJob{{Name: "Captain", Outfit: "/datum/outfit/job/captain", Category: "Command", Slots: 1, Officer: true}}
		if p.Settings.Crew > 1 {
			j = append(j, CrewJob{Name: "Crew", Outfit: "/datum/outfit/job/assistant", Category: "Assistant", Slots: p.Settings.Crew - 1})
		}
		return j, nil
	}
	if o := p.Dme.Objects[s.Type]; o != nil {
		return parseCrew(o.Vars.ValueV(s.Field, "null"))
	}
	return nil, nil
}
func (p *Project) CrewJobs(scope string) ([]CrewJob, error) {
	if module, theme := RoomCrewIDs(scope); theme != "" {
		if _, err := p.crewScope(scope); err != nil {
			return nil, err
		}
		rosters, err := p.roomCrewVariants(module)
		if jobs, own := rosters[theme]; own || err != nil {
			return CloneCrewJobs(jobs), err
		}
		return p.CrewJobs("module/" + module)
	}
	if p.Crew != nil {
		if j, ok := p.Crew.Rosters[scope]; ok {
			return CloneCrewJobs(j), nil
		}
	}
	s, e := p.crewScope(scope)
	if e != nil {
		return nil, e
	}
	return p.readCrew(s)
}
func (p *Project) CrewOutfit(j CrewJob) string {
	if j.BaseOutfit != "" {
		return j.BaseOutfit
	}
	return j.Outfit
}
func (p *Project) CrewEquipment(j CrewJob) map[string]string {
	result := map[string]string{}
	if o := p.Dme.Objects[p.CrewOutfit(j)]; o != nil {
		for _, s := range EquipmentSlots {
			v := o.Vars.ValueV(s.ID, "null")
			if strings.HasPrefix(v, "/obj/item") {
				result[s.ID] = v
			}
		}
		if result["back"] == "/obj/item/storage/backpack" {
			if v := o.Vars.ValueV("backpack", ""); strings.HasPrefix(v, "/obj/item") {
				result["back"] = v
			}
		}
	}
	for k, v := range j.Equipment {
		result[k] = v
	}
	return result
}
func (p *Project) CrewContents(j CrewJob, belt bool) (map[string]int, error) {
	v := j.Backpack
	field := "backpack_contents"
	if belt {
		v = j.Belt
		field = "belt_contents"
	}
	if v != nil {
		return v, nil
	}
	out := map[string]int{}
	o := p.Dme.Objects[p.CrewOutfit(j)]
	if o == nil {
		return out, nil
	}
	parts, e := dmListParts(o.Vars.ValueV(field, "null"))
	if e != nil {
		return nil, e
	}
	for _, part := range parts {
		pair := dmSplit(part, '=')
		n := 1
		if len(pair) > 1 {
			n, e = strconv.Atoi(strings.TrimSpace(pair[1]))
			if e != nil {
				return nil, e
			}
		}
		out[strings.TrimSpace(pair[0])] += n
	}
	return out, nil
}
func (p *Project) ValidateCrew(jobs []CrewJob) error {
	names := map[string]bool{}
	for _, j := range jobs {
		key := strings.ToLower(strings.Join(strings.Fields(j.Name), " "))
		if key == "" {
			return fmt.Errorf("give each job a name")
		}
		if names[key] {
			return fmt.Errorf("a job named %q already exists in this roster", j.Name)
		}
		names[key] = true
		if j.Slots < 1 || j.Slots > 128 {
			return fmt.Errorf("%s: choose 1–128 slots", j.Name)
		}
		if !Contains(CrewCategories, j.Category) {
			return fmt.Errorf("%s: choose a job category", j.Name)
		}
		o := p.Dme.Objects[p.CrewOutfit(j)]
		if o == nil || !strings.HasPrefix(p.CrewOutfit(j), "/datum/outfit/") || p.Dme.Objects[o.Vars.ValueV("jobtype", "")] == nil {
			return fmt.Errorf("%s: choose a job outfit with a valid job type", j.Name)
		}
		for k, v := range j.Equipment {
			found := false
			for _, s := range EquipmentSlots {
				if s.ID == k {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown equipment slot %s", k)
			}
			if v != "" && (!strings.HasPrefix(v, "/obj/item/") || p.Dme.Objects[v] == nil) {
				return fmt.Errorf("unknown item %s", v)
			}
		}
		for _, bag := range []map[string]int{j.Backpack, j.Belt} {
			for path, n := range bag {
				if n < 1 || n > 100 || !strings.HasPrefix(path, "/obj/item/") || p.Dme.Objects[path] == nil {
					return fmt.Errorf("invalid carried item or quantity: %s", path)
				}
			}
		}
	}
	return nil
}
func (p *Project) SetCrewJobs(scope string, jobs []CrewJob) error {
	module, theme := RoomCrewIDs(scope)
	if theme != "" && !p.SupportsRoomCrewVariants() {
		return fmt.Errorf("this game project needs per-theme module crew support")
	}
	if theme != "" && !p.hasRoomCrewVariant(module, theme) {
		// Legacy room crew supplies the initial values, never shared outfit IDs.
		jobs = CloneCrewJobs(jobs)
		for i := range jobs {
			jobs[i].ID = ""
		}
	}
	if e := p.ValidateCrew(jobs); e != nil {
		return e
	}
	ids := map[string]bool{}
	if p.Crew != nil {
		for key, roster := range p.editedCrewRosters() {
			if key != scope {
				for _, j := range roster {
					if j.ID != "" {
						ids[j.ID] = true
					}
				}
			}
		}
	}
	for _, j := range jobs {
		if j.ID == "" {
			continue
		}
		if e := ValidID(j.ID); e != nil {
			return e
		}
		if ids[j.ID] {
			return fmt.Errorf("duplicate crew identifier %s; copy jobs with new identifiers", j.ID)
		}
		ids[j.ID] = true
	}
	s, e := p.crewScope(scope)
	if e != nil {
		return e
	}
	meta, code := p.crewPaths()
	if p.Crew == nil {
		if _, tracked := p.files[code]; !tracked {
			if _, e := os.Stat(code); e == nil {
				return fmt.Errorf("crew outfit file already exists without matching workshop metadata: %s", code)
			}
		}
	}
	for _, path := range []string{meta, code, p.Dme.RootFile} {
		if _, ok := p.files[path]; !ok {
			if e = p.track(path); e != nil {
				return e
			}
		}
	}
	if p.Settings == nil && p.Dme.Objects[s.Type] != nil {
		file, e := p.roomTypeFile(s.Type)
		if e != nil {
			return e
		}
		if _, ok := p.files[file]; !ok {
			if e = p.track(file); e != nil {
				return e
			}
		}
		value := renderCrew(jobs, p)
		if theme != "" {
			rosters, err := p.roomCrewVariants(module)
			if err != nil {
				return err
			}
			value = renderRoomCrew(rosters, p)
		}
		if _, e = rewriteCrewList(p.files[file].Before, s.Type, s.Field, value); e != nil {
			return e
		}
		if theme != "" {
			// The first edit must agree with the parsed game, just like base crew.
			alreadyEdited := false
			for original := range p.crewOriginal {
				id, variant := RoomCrewIDs(original)
				alreadyEdited = alreadyEdited || id == module && variant != ""
			}
			if !alreadyEdited {
				raw, _, explicit, err := sourceCostList(p.files[file].Before, s.Type, roomCrewField)
				if err != nil {
					return err
				}
				if explicit {
					source, err := parseRoomCrew(raw)
					if err != nil {
						return err
					}
					loaded, err := p.readRoomCrew(module)
					if err != nil {
						return err
					}
					if renderRoomCrew(source, p) != renderRoomCrew(loaded, p) {
						return fmt.Errorf("module crew differs from the loaded environment; reload before editing")
					}
				}
			}
		}
		if _, edited := p.crewOriginal[scope]; !edited && theme == "" {
			source, explicit, e := sourceCrew(p.files[file].Before, s.Type, s.Field)
			if e != nil {
				return e
			}
			loaded, e := p.readCrew(s)
			if e != nil {
				return e
			}
			if explicit && renderCrew(source, p) != renderCrew(loaded, p) {
				return fmt.Errorf("%s crew differs from the loaded environment; reload before editing", s.Name)
			}
		}
	}
	if p.crewOriginal == nil {
		p.crewOriginal = map[string][]CrewJob{}
	}
	if _, ok := p.crewOriginal[scope]; !ok {
		original, e := p.CrewJobs(scope)
		if e != nil {
			return e
		}
		p.crewOriginal[scope] = original
	}
	jobs = CloneCrewJobs(jobs)
	used := map[string]bool{}
	for id := range ids {
		used[id] = true
	}
	if p.Crew != nil {
		for _, roster := range p.editedCrewRosters() {
			for _, j := range roster {
				used[j.ID] = true
			}
		}
	}
	for i := range jobs {
		j := &jobs[i]
		if j.ID == "" {
			for n := 1; ; n++ {
				id := "job_" + strconv.Itoa(n)
				if !used[id] {
					j.ID = id
					used[id] = true
					break
				}
			}
		}
		if len(j.Equipment) > 0 || j.Backpack != nil || j.Belt != nil {
			if j.BaseOutfit == "" {
				j.BaseOutfit = j.Outfit
			}
			j.Outfit = p.crewOutfitPath(*j)
		}
	}
	if p.Crew == nil {
		p.Crew = &CrewConfig{Rosters: map[string][]CrewJob{}}
	}
	if theme != "" {
		if p.Crew.ModuleThemes == nil {
			p.Crew.ModuleThemes = map[string]map[string][]CrewJob{}
		}
		if _, ok := p.Crew.ModuleThemes[module]; !ok {
			rosters, err := p.readRoomCrew(module)
			if err != nil {
				return err
			}
			p.Crew.ModuleThemes[module] = rosters
		}
		p.Crew.ModuleThemes[module][theme] = jobs
	} else {
		p.Crew.Rosters[scope] = jobs
	}
	return nil
}
func (p *Project) crewOutfitPath(j CrewJob) string {
	id, _ := p.roomID()
	return "/datum/outfit/job/workshop_" + id + "_" + j.ID
}
func renderCrew(jobs []CrewJob, p *Project) string {
	parts := []string{}
	for _, j := range jobs {
		outfit := j.Outfit
		if j.BaseOutfit != "" {
			outfit = p.crewOutfitPath(j)
		}
		officer := "FALSE"
		if j.Officer {
			officer = "TRUE"
		}
		fields := []string{"name = " + dmQuote(j.Name), "officer = " + officer, "outfit = " + outfit, "category = " + dmQuote(j.Category), "slots = " + strconv.Itoa(j.Slots)}
		keys := []string{}
		for k := range j.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fields = append(fields, dmQuote(k)+" = "+j.Extra[k])
		}
		parts = append(parts, "list("+strings.Join(fields, ", ")+")")
	}
	return "list(" + strings.Join(parts, ", ") + ")"
}
func renderContents(m map[string]int) string {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, k+" = "+strconv.Itoa(m[k]))
	}
	return "list(" + strings.Join(parts, ", ") + ")"
}
func (p *Project) crewOutfits() []byte {
	var b strings.Builder
	b.WriteString("// Ship Workshop crew outfits. Edit these through the crew editor.\n")
	if p.Crew == nil {
		return []byte(b.String())
	}
	keys := []string{}
	rosters := p.editedCrewRosters()
	for k := range rosters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, j := range rosters[k] {
			if j.BaseOutfit == "" {
				continue
			}
			path := p.crewOutfitPath(j)
			fmt.Fprintf(&b, "\n%s\n\tparent_type = %s\n\tname = %s\n", path, j.BaseOutfit, dmQuote(p.Hull.Name+" — "+j.Name))
			fields := []string{}
			for _, s := range EquipmentSlots {
				if v, ok := j.Equipment[s.ID]; ok {
					if v == "" {
						v = "null"
					}
					fields = append(fields, s.ID+" = "+v)
				}
			}
			if j.Backpack != nil {
				fields = append(fields, "backpack_contents = "+renderContents(j.Backpack))
			}
			if j.Belt != nil {
				fields = append(fields, "belt_contents = "+renderContents(j.Belt))
			}
			for _, f := range fields {
				fmt.Fprintf(&b, "\t%s\n", f)
			}
			// Keep the base job hooks, then apply explicit selections after preference
			// substitutions (backpack type, skirt and reward cloak) have run.
			fmt.Fprintf(&b, "\n%s/pre_equip(mob/living/carbon/human/H, visuals_only = FALSE)\n\t. = ..()\n", path)
			for _, f := range fields {
				fmt.Fprintf(&b, "\t%s\n", f)
			}
		}
	}
	return []byte(b.String())
}
func (p *Project) crewChanges(changes []FileChange) ([]FileChange, error) {
	if bytes.Equal(p.crewBytes(), p.savedCrew) && (p.Crew == nil || len(changes) == 0) {
		return changes, nil
	}
	meta, code := p.crewPaths()
	contents := map[string][]byte{}
	for _, c := range changes {
		contents[c.Path] = c.After
	}
	get := func(path string) []byte {
		if b, ok := contents[path]; ok {
			return b
		}
		return p.files[path].Before
	}
	rosters := map[string][]CrewJob{}
	for k, j := range p.crewOriginal {
		rosters[k] = j
	}
	if p.Crew != nil {
		for k, j := range p.Crew.Rosters {
			rosters[k] = j
		}
	}
	if p.Settings == nil {
		for key, jobs := range rosters {
			if _, theme := RoomCrewIDs(key); theme != "" {
				continue
			}
			s, e := p.crewScope(key)
			if e != nil {
				if p.Crew == nil {
					continue
				}
				if _, active := p.Crew.Rosters[key]; !active {
					continue
				}
				return nil, e
			}
			file := ""
			if p.Dme.Objects[s.Type] != nil {
				file, e = p.roomTypeFile(s.Type)
			} else if p.rooms != nil {
				file = p.generatedSourceFile(key)
			} else {
				e = fmt.Errorf("cannot locate crew source")
			}
			if e != nil {
				return nil, e
			}
			if _, ok := p.files[file]; !ok {
				if e = p.track(file); e != nil {
					return nil, e
				}
			}
			b, e := rewriteCrewList(get(file), s.Type, s.Field, renderCrew(jobs, p))
			if e != nil {
				return nil, e
			}
			contents[file] = b
		}
		for _, module := range p.editedRoomCrewModules() {
			s, e := p.crewScope("module/" + module)
			if e != nil {
				return nil, e
			}
			file := ""
			if p.Dme.Objects[s.Type] != nil {
				file, e = p.roomTypeFile(s.Type)
			} else if p.rooms != nil {
				file = p.generatedSourceFile(s.ID)
			} else {
				e = fmt.Errorf("cannot locate module crew source")
			}
			if e != nil {
				return nil, e
			}
			variants, e := p.roomCrewVariants(module)
			if e != nil {
				return nil, e
			}
			contents[file], e = rewriteCrewList(get(file), s.Type, roomCrewField, renderRoomCrew(variants, p))
			if e != nil {
				return nil, e
			}
		}
	}
	content := p.crewBytes()
	if content == nil {
		content = []byte("{\"Rosters\":{}}\n")
	}
	contents[meta], contents[code] = content, p.crewOutfits()
	contents[p.Dme.RootFile] = addInclude(get(p.Dme.RootFile), p.Catalog.Root, code)
	for path, b := range contents {
		c, ok := p.files[path]
		if !ok {
			continue
		}
		c.After = b
		found := false
		for i := range changes {
			if changes[i].Path == path {
				changes[i].After = b
				found = true
				break
			}
		}
		if !found {
			changes = append(changes, c)
		}
	}
	return changes, nil
}

func dmSplit(s string, delimiter byte) []string {
	mask := dmSourceMask([]byte(s), true)
	depth, start := 0, 0
	var parts []string
	for i, ch := range mask {
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
		if ch == delimiter && depth == 0 {
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}
func dmListParts(raw string) ([]string, error) {
	s := strings.TrimSpace(string(dmSourceMask([]byte(raw), false)))
	if s == "" || s == "null" {
		return nil, nil
	}
	if !strings.HasPrefix(s, "list(") || !strings.HasSuffix(s, ")") {
		return nil, fmt.Errorf("crew needs a literal list, found %s", s)
	}
	parts := dmSplit(s[5:len(s)-1], ',')
	out := []string{}
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
func dmCrewText(s string) string {
	if v, e := dmUnquote(strings.TrimSpace(s)); e == nil {
		return v
	}
	return strings.TrimSpace(s)
}
func parseCrew(raw string) ([]CrewJob, error) {
	parts, e := dmListParts(raw)
	if e != nil {
		return nil, e
	}
	jobs := []CrewJob{}
	for _, part := range parts {
		fields, e := dmListParts(part)
		if e != nil {
			return nil, e
		}
		j := CrewJob{Slots: 1, Category: "Assistant", Extra: map[string]string{}}
		for _, f := range fields {
			pair := dmSplit(f, '=')
			if len(pair) != 2 {
				return nil, fmt.Errorf("unsupported job definition %s", f)
			}
			k, v := dmCrewText(pair[0]), pair[1]
			switch k {
			case "name":
				j.Name = dmCrewText(v)
			case "outfit":
				j.Outfit = v
			case "category":
				j.Category = dmCrewText(v)
				for i, macro := range []string{"COMMAND", "SECURITY", "ENGINEERING", "MEDICAL", "SCIENCE", "CARGO", "SERVICE", "ASSISTANT"} {
					if v == "JOB_CAT_"+macro {
						j.Category = CrewCategories[i]
					}
				}
			case "officer":
				if v != "TRUE" && v != "FALSE" && v != "1" && v != "0" {
					return nil, fmt.Errorf("unsupported officer value")
				}
				j.Officer = v == "TRUE" || v == "1"
			case "slots":
				j.Slots, e = strconv.Atoi(v)
				if e != nil {
					return nil, e
				}
			default:
				j.Extra[k] = v
			}
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// Preserve every byte outside the selected type's job list, including procs,
// prices, other variants and comments. Reject computed/conditional lists.
func rewriteCrewList(data []byte, typePath, field, value string) ([]byte, error) {
	return rewriteLiteralList(data, typePath, field, value)
}
