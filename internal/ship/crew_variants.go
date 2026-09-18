package ship

import (
	"fmt"
	"sort"
	"strings"
)

const roomCrewField = "job_slots_add_by_theme"

func RoomCrewIDs(scope string) (module, theme string) {
	parts := strings.Split(scope, "/")
	if len(parts) >= 2 && parts[0] == "module" {
		module = parts[1]
		if len(parts) == 4 && parts[2] == "theme" {
			theme = parts[3]
		}
	}
	return
}

func roomCrewScope(module, theme string) string { return "module/" + module + "/theme/" + theme }

func (p *Project) SupportsRoomCrewVariants() bool {
	o := p.Dme.Objects["/datum/ship_upgrade_module"]
	return o != nil && o.Vars.ValueV(roomCrewField, "") != ""
}

func parseRoomCrew(raw string) (map[string][]CrewJob, error) {
	parts, err := dmListParts(raw)
	if err != nil {
		return nil, err
	}
	result := map[string][]CrewJob{}
	for _, part := range parts {
		pair := dmSplit(part, '=')
		if len(pair) != 2 {
			return nil, fmt.Errorf("module crew needs a roster for each theme")
		}
		id, err := dmUnquote(pair[0])
		if err != nil || ValidID(id) != nil {
			return nil, fmt.Errorf("invalid module crew theme %s", pair[0])
		}
		if _, exists := result[id]; exists {
			return nil, fmt.Errorf("duplicate module crew theme %s", id)
		}
		jobs, err := parseCrew(pair[1])
		if err != nil {
			return nil, err
		}
		result[id] = jobs
	}
	return result, nil
}

func renderRoomCrew(rosters map[string][]CrewJob, p *Project) string {
	keys := make([]string, 0, len(rosters))
	for key := range rosters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, dmQuote(key)+" = "+renderCrew(rosters[key], p))
	}
	return "list(" + strings.Join(parts, ", ") + ")"
}

func (p *Project) readRoomCrew(module string) (map[string][]CrewJob, error) {
	s, err := p.crewScope("module/" + module)
	if err != nil {
		return nil, err
	}
	if o := p.Dme.Objects[s.Type]; o != nil {
		return parseRoomCrew(o.Vars.ValueV(roomCrewField, "null"))
	}
	return map[string][]CrewJob{}, nil
}

func (p *Project) roomCrewVariants(module string) (map[string][]CrewJob, error) {
	if p.Crew != nil {
		if rosters, ok := p.Crew.ModuleThemes[module]; ok {
			return rosters, nil
		}
	}
	return p.readRoomCrew(module)
}

func (p *Project) hasRoomCrewVariant(module, theme string) bool {
	rosters, _ := p.roomCrewVariants(module)
	_, own := rosters[theme]
	return own
}

// Every room variant has its own editing scope, even before its first edit.
func (p *Project) RoomCrewScope(module, theme string) string {
	if theme != "" {
		return roomCrewScope(module, theme)
	}
	return "module/" + module
}

func (p *Project) clearRoomCrewVariant(module, theme string) error {
	rosters, err := p.roomCrewVariants(module)
	if err != nil {
		return err
	}
	if _, own := rosters[theme]; !own {
		return nil
	}
	if err = p.SetCrewJobs(roomCrewScope(module, theme), nil); err != nil {
		return err
	}
	delete(p.Crew.ModuleThemes[module], theme)
	return nil
}

type RoomCrewSource struct {
	Module Module
	Theme  Theme
	Jobs   []CrewJob
}

// RoomCrewSources offers configured rosters in other modules or themes.
func (p *Project) RoomCrewSources(module, target string) ([]RoomCrewSource, error) {
	i := p.moduleIndex(module)
	if i < 0 {
		return nil, fmt.Errorf("module option no longer exists")
	}
	themes := p.Hull.Themes
	if len(themes) == 0 {
		themes = []Theme{{Name: "Shared"}}
	}
	var sources []RoomCrewSource
	// Keep the current module's other themes first, preserving the usual copy
	// choice while also offering every other module option on this ship.
	modules := append([]Module{p.Hull.Modules[i]}, p.Hull.Modules[:i]...)
	modules = append(modules, p.Hull.Modules[i+1:]...)
	for _, m := range modules {
		for _, theme := range themes {
			if m.ID == module && theme.ID == target || !m.Available(theme.ID) || !Contains(p.Hull.SlotsFor(theme), m.Slot) {
				continue
			}
			jobs, err := p.CrewJobs(p.RoomCrewScope(m.ID, theme.ID))
			if err != nil {
				return nil, fmt.Errorf("%s / %s: %w", m.Name, theme.Name, err)
			}
			if len(jobs) > 0 {
				sources = append(sources, RoomCrewSource{Module: m, Theme: theme, Jobs: jobs})
			}
		}
	}
	return sources, nil
}

// CopyRoomCrewJob appends a detached copy, including its slots and equipment.
func (p *Project) CopyRoomCrewJob(sourceModule, sourceTheme, module, target string, index int) (int, error) {
	i, t := p.moduleIndex(module), p.themeIndex(target)
	var theme Theme
	if t >= 0 {
		theme = p.Hull.Themes[t]
	} else if target != "" || len(p.Hull.Themes) != 0 {
		return -1, fmt.Errorf("choose a module available in this theme")
	}
	if i < 0 || !p.Hull.Modules[i].Available(target) || !Contains(p.Hull.SlotsFor(theme), p.Hull.Modules[i].Slot) {
		return -1, fmt.Errorf("choose a module available in this theme")
	}
	sources, err := p.RoomCrewSources(module, target)
	if err != nil {
		return -1, err
	}
	for _, candidate := range sources {
		if candidate.Module.ID != sourceModule || candidate.Theme.ID != sourceTheme {
			continue
		}
		if index < 0 || index >= len(candidate.Jobs) {
			break
		}
		return p.PasteCrewJob(p.RoomCrewScope(module, target), candidate.Jobs[index])
	}
	return -1, fmt.Errorf("choose a job from another module or theme")
}

// PasteCrewJob appends an independent job to a ship, theme or module roster.
func (p *Project) PasteCrewJob(scope string, copied CrewJob) (int, error) {
	jobs, err := p.CrewJobs(scope)
	if err != nil {
		return -1, err
	}
	job := CloneCrewJobs([]CrewJob{copied})[0]
	job.ID, job.Outfit = "", p.CrewOutfit(job)
	used := map[string]bool{}
	for _, existing := range jobs {
		used[strings.ToLower(strings.Join(strings.Fields(existing.Name), " "))] = true
	}
	original := job.Name
	for n := 1; used[strings.ToLower(strings.Join(strings.Fields(job.Name), " "))]; n++ {
		job.Name = original + " (copy)"
		if n > 1 {
			job.Name = fmt.Sprintf("%s (copy %d)", original, n)
		}
	}
	selected := len(jobs)
	if err = p.SetCrewJobs(scope, append(jobs, job)); err != nil {
		return -1, err
	}
	return selected, nil
}

// editedCrewRosters includes independent rosters for outfit generation and IDs.
func (p *Project) editedCrewRosters() map[string][]CrewJob {
	result := map[string][]CrewJob{}
	if p.Crew != nil {
		for scope, jobs := range p.Crew.Rosters {
			result[scope] = jobs
		}
		for module, variants := range p.Crew.ModuleThemes {
			for theme, jobs := range variants {
				result[roomCrewScope(module, theme)] = jobs
			}
		}
	}
	return result
}

func (p *Project) editedRoomCrewModules() []string {
	touched := map[string]bool{}
	for scope := range p.crewOriginal {
		if module, theme := RoomCrewIDs(scope); theme != "" {
			touched[module] = true
		}
	}
	if p.Crew != nil {
		for module := range p.Crew.ModuleThemes {
			touched[module] = true
		}
	}
	var modules []string
	for _, m := range p.Hull.Modules {
		if touched[m.ID] {
			modules = append(modules, m.ID)
		}
	}
	return modules
}
