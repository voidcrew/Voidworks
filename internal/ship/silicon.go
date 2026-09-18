package ship

import (
	"fmt"
	"strings"

	"sdmm/internal/util"
)

const AICore = "/obj/structure/ai_core/latejoin_inactive"

func (j CrewJob) IsSilicon() bool { return j.Role == "cyborg" || j.Role == "ai" }

func (p *Project) SupportsSiliconCrew() bool {
	o := p.Dme.Objects["/datum/job"]
	return o != nil && o.Vars.ValueV("ship_role", "") != ""
}

func (p *Project) validateCrewRole(j CrewJob) error {
	if j.Role != "" && j.Role != "crew" && !j.IsSilicon() {
		return fmt.Errorf("%s: unknown starting role %q", j.Name, j.Role)
	}
	if !j.IsSilicon() {
		if j.BorgModel != "" {
			return fmt.Errorf("%s: only cyborgs can have a borg model", j.Name)
		}
		return nil
	}
	if !p.SupportsSiliconCrew() {
		return fmt.Errorf("update the game project to use silicon crew roles")
	}
	if j.Officer {
		return fmt.Errorf("%s: silicon roles cannot be ship officers", j.Name)
	}
	if (j.Outfit != "" && j.Outfit != "null") || j.BaseOutfit != "" || len(j.Equipment) > 0 || j.Backpack != nil || j.Belt != nil {
		return fmt.Errorf("%s: silicon roles use their built-in equipment", j.Name)
	}
	if j.Role == "cyborg" && (!strings.HasPrefix(j.BorgModel, "/obj/item/robot_model/") || p.Dme.Objects[j.BorgModel] == nil) {
		return fmt.Errorf("%s: choose a borg model", j.Name)
	}
	if j.Role == "ai" && j.BorgModel != "" {
		return fmt.Errorf("%s: AI roles cannot have a borg model", j.Name)
	}
	return nil
}

func aiSlots(jobs []CrewJob) int {
	n := 0
	for _, j := range jobs {
		if j.Role == "ai" {
			n += j.Slots
		}
	}
	return n
}

// Count only live grid instances, including mapped overrides. A bare construction
// frame, disabled transmitter or unavailable core cannot seat a player.
func (p *Project) coreLocations(a *Assembly, source int) map[util.Point]int {
	cores := map[util.Point]int{}
	for coord, atoms := range a.Cells {
		floor, shipArea := false, false
		for _, atom := range atoms {
			floor = floor || strings.HasPrefix(atom.Prefab.Path(), "/turf/open/floor/") || atom.Prefab.Path() == "/turf/open/floor"
			shipArea = shipArea || atom.Prefab.Path() == "/area/shuttle/voidcrew" || strings.HasPrefix(atom.Prefab.Path(), "/area/shuttle/voidcrew/")
		}
		if !floor || !shipArea {
			continue
		}
		for _, atom := range atoms {
			path := atom.Prefab.Path()
			if atom.Source != source || (path != AICore && !strings.HasPrefix(path, AICore+"/")) {
				continue
			}
			usable := true
			for _, field := range []string{"available", "active"} {
				value := "TRUE"
				if o := p.Dme.Objects[path]; o != nil {
					value = o.Vars.ValueV(field, value)
				}
				value = atom.Prefab.Vars().ValueV(field, value)
				if value == "FALSE" || value == "0" || value == "null" {
					usable = false
				}
			}
			if usable {
				cores[coord]++
			}
		}
	}
	return cores
}

// Validate every theme and optional module, including an uncommitted roster.
// Hull cores must survive every option. Each slot reserves its largest deficit,
// so two independently optional rooms cannot promise the same hull core.
func (p *Project) ValidateSiliconCrew(scope string, replacement []CrewJob) error {
	read := func(id string) ([]CrewJob, error) {
		if scope != "" && id == scope {
			return replacement, nil
		}
		if module, theme := RoomCrewIDs(id); theme != "" && scope == "module/"+module && !p.hasRoomCrewVariant(module, theme) {
			return replacement, nil
		}
		return p.CrewJobs(id)
	}
	hasAI := false
	for _, s := range p.CrewScopes() {
		jobs, err := read(s.ID)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if err := p.validateCrewRole(j); err != nil {
				return err
			}
			if j.IsSilicon() {
				if err := p.ValidateCrew([]CrewJob{j}); err != nil {
					return err
				}
			}
			hasAI = hasAI || j.Role == "ai"
		}
	}
	// A first per-theme edit may not yet appear in CrewScopes.
	for _, j := range replacement {
		hasAI = hasAI || j.Role == "ai"
	}
	if !hasAI {
		return nil
	}
	themes := p.Hull.Themes
	if len(themes) == 0 {
		themes = []Theme{{}}
	}
	for _, theme := range themes {
		base, err := read("ship")
		if err != nil {
			return err
		}
		if theme.ID != "" {
			jobs, err := read("theme/" + theme.ID)
			if err != nil {
				return err
			}
			if len(jobs) > 0 {
				base = jobs
			}
		}
		a, err := p.Assemble(theme, map[string]string{})
		if err != nil {
			return err
		}
		permanent := p.coreLocations(a, 0)
		deficits := map[string]int{}
		for _, m := range p.Hull.Modules {
			if !m.Available(theme.ID) || !Contains(p.Hull.SlotsFor(theme), m.Slot) {
				continue
			}
			jobs, err := read(p.RoomCrewScope(m.ID, theme.ID))
			if err != nil {
				return err
			}
			// Read a new per-theme roster before its override has been installed.
			if module, variant := RoomCrewIDs(scope); module == m.ID && variant == theme.ID && variant != "" {
				jobs = replacement
			}
			pair, err := p.Assemble(theme, map[string]string{m.Slot: m.ID})
			if err != nil {
				return err
			}
			surviving := p.coreLocations(pair, 0)
			for coord, n := range permanent {
				permanent[coord] = min(n, surviving[coord])
			}
			own := 0
			for _, n := range p.coreLocations(pair, 1) {
				own += n
			}
			deficits[m.Slot] = max(deficits[m.Slot], aiSlots(jobs)-own)
		}
		available, required := 0, aiSlots(base)
		for _, n := range permanent {
			available += n
		}
		for _, n := range deficits {
			required += n
		}
		if required > available {
			return fmt.Errorf("%s: Add %d networked AI core(s) to the permanent hull or the module adding AI crew", theme.Name, required-available)
		}
	}
	return nil
}
