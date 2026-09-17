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
	Theme Theme
	Jobs  []CrewJob
}

// RoomCrewSources offers only other variants with configured crew for this option.
func (p *Project) RoomCrewSources(module, target string) ([]RoomCrewSource, error) {
	i := p.moduleIndex(module)
	if i < 0 {
		return nil, fmt.Errorf("module option no longer exists")
	}
	m := p.Hull.Modules[i]
	var sources []RoomCrewSource
	for _, theme := range p.Hull.Themes {
		if theme.ID == target || !m.Available(theme.ID) || !Contains(p.Hull.SlotsFor(theme), m.Slot) {
			continue
		}
		jobs, err := p.CrewJobs(p.RoomCrewScope(module, theme.ID))
		if err != nil {
			return nil, err
		}
		if len(jobs) > 0 {
			sources = append(sources, RoomCrewSource{Theme: theme, Jobs: jobs})
		}
	}
	return sources, nil
}

// CopyRoomCrewJob appends a detached copy, including its slots and equipment.
func (p *Project) CopyRoomCrewJob(module, source, target string, index int) (int, error) {
	i, t := p.moduleIndex(module), p.themeIndex(target)
	if i < 0 || t < 0 || !p.Hull.Modules[i].Available(target) || !Contains(p.Hull.SlotsFor(p.Hull.Themes[t]), p.Hull.Modules[i].Slot) {
		return -1, fmt.Errorf("choose a module available in this theme")
	}
	sources, err := p.RoomCrewSources(module, target)
	if err != nil {
		return -1, err
	}
	for _, candidate := range sources {
		if candidate.Theme.ID != source {
			continue
		}
		if index < 0 || index >= len(candidate.Jobs) {
			break
		}
		jobs, err := p.CrewJobs(p.RoomCrewScope(module, target))
		if err != nil {
			return -1, err
		}
		job := candidate.Jobs[index]
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
		if err = p.SetCrewJobs(p.RoomCrewScope(module, target), append(jobs, job)); err != nil {
			return -1, err
		}
		return selected, nil
	}
	return -1, fmt.Errorf("choose a job from another theme")
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
