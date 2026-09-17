package ship

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type PartCosts map[string]int

type PartClass struct{ ID, Name, Define string }

var PartClasses = []PartClass{
	{"combat", "Combat", "PART_CLASS_COMBAT"},
	{"science", "Science", "PART_CLASS_SCIENCE"},
	{"trade", "Trade", "PART_CLASS_TRADE"},
	{"misc", "Misc", "PART_CLASS_MISC"},
}

type CostScope struct {
	ID, Name, Kind, Type, Field string
	Default                     bool
}

type costSource struct {
	file, raw string
	explicit  bool
	value     PartCosts
}

func (c PartCosts) Validate() error {
	for id, value := range c {
		known := false
		for _, class := range PartClasses {
			if class.ID == id {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown part type %q", id)
		}
		if value < 0 || value > 1000000 {
			return fmt.Errorf("part costs must be whole numbers from 0 to 1,000,000")
		}
	}
	return nil
}

func (c PartCosts) Total() int {
	total := 0
	for _, n := range c {
		total += n
	}
	return total
}

func (c PartCosts) Equal(other PartCosts) bool {
	for _, class := range PartClasses {
		if c[class.ID] != other[class.ID] {
			return false
		}
	}
	return true
}

func (c PartCosts) Summary() string {
	var parts []string
	for _, class := range PartClasses {
		if n := c[class.ID]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, class.Name))
		}
	}
	if len(parts) == 0 {
		return "Free"
	}
	return strings.Join(parts, "  ·  ")
}

func clonePartCosts(c PartCosts) PartCosts {
	copy := PartCosts{}
	for k, v := range c {
		if v != 0 {
			copy[k] = v
		}
	}
	return copy
}

func cloneCostScopes(scopes map[string]PartCosts) map[string]PartCosts {
	if len(scopes) == 0 {
		return nil
	}
	copy := map[string]PartCosts{}
	for id, cost := range scopes {
		copy[id] = clonePartCosts(cost)
	}
	return copy
}

func renderPartCosts(c PartCosts) string {
	var pairs []string
	for _, class := range PartClasses {
		if n := c[class.ID]; n > 0 {
			pairs = append(pairs, fmt.Sprintf("%s = %d", class.Define, n))
		}
	}
	return "list(" + strings.Join(pairs, ", ") + ")"
}

func parsePartCosts(raw string) (PartCosts, error) {
	parts, err := dmListParts(strings.TrimSpace(string(dmSourceMask([]byte(raw), false))))
	if err != nil {
		return nil, fmt.Errorf("part costs need a literal list: %w", err)
	}
	cost := PartCosts{}
	seen := map[string]bool{}
	for _, part := range parts {
		pair := dmSplit(part, '=')
		if len(pair) != 2 {
			return nil, fmt.Errorf("each part type needs a quantity")
		}
		key := strings.TrimSpace(pair[0])
		for _, class := range PartClasses {
			if key == class.Define {
				key = dmQuote(class.ID)
				break
			}
		}
		id, err := dmUnquote(key)
		if err != nil {
			return nil, fmt.Errorf("unknown part type %s", key)
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate part type %s", id)
		}
		seen[id] = true
		n, err := strconv.Atoi(strings.TrimSpace(pair[1]))
		if err != nil {
			return nil, fmt.Errorf("%s needs a whole-number cost", id)
		}
		cost[id] = n
	}
	if err := cost.Validate(); err != nil {
		return nil, err
	}
	return clonePartCosts(cost), nil
}

func (p *Project) CostScopes() []CostScope {
	crew := p.baseCrewScopes()
	scopes := []CostScope{{ID: "ship", Name: p.Hull.Name, Kind: "Hull", Type: p.Hull.Type, Field: "part_requirements", Default: true}}
	for _, s := range crew[1:] {
		one := CostScope{ID: s.ID, Name: s.Name, Type: s.Type, Field: "part_cost"}
		if strings.HasPrefix(s.ID, "theme/") {
			one.Kind = "Ship theme"
			for _, theme := range p.Hull.Themes {
				if s.ID == "theme/"+theme.ID {
					one.Name, one.Default = theme.Name, theme.Default
					break
				}
			}
		} else {
			one.Kind = "Module option"
			for _, module := range p.Hull.Modules {
				if s.ID == "module/"+module.ID {
					one.Name, one.Default = module.Name, module.Default
					break
				}
			}
		}
		scopes = append(scopes, one)
	}
	return scopes
}

func (p *Project) costScope(id string) (CostScope, error) {
	for _, s := range p.CostScopes() {
		if s.ID == id {
			return s, nil
		}
	}
	return CostScope{}, fmt.Errorf("this ship component no longer exists")
}

func (p *Project) costBytes() []byte {
	if len(p.partCosts) == 0 {
		return nil
	}
	data, _ := json.Marshal(p.partCosts)
	return data
}

// DisplayPartCosts uses the loaded definitions for informational labels. Reading
// a label must not walk the entire source tree; PartCosts and SetPartCosts still
// inspect the current source before a price is edited.
func (p *Project) DisplayPartCosts(id string) (PartCosts, error) {
	s, err := p.costScope(id)
	if err != nil {
		return nil, err
	}
	if cost, ok := p.partCosts[id]; ok {
		return clonePartCosts(cost), cost.Validate()
	}
	if p.Settings != nil {
		if id == "ship" {
			return PartCosts{"misc": p.Settings.Cost}, nil
		}
		return PartCosts{}, nil
	}
	if original, ok := p.costOriginal[id]; ok {
		return clonePartCosts(original.value), nil
	}
	if object := p.Dme.Objects[s.Type]; object != nil {
		return parsePartCosts(object.Vars.ValueV(s.Field, "null"))
	}
	return PartCosts{}, nil
}

func (p *Project) PartCosts(id string) (PartCosts, error) {
	s, err := p.costScope(id)
	if err != nil {
		return nil, err
	}
	if cost, ok := p.partCosts[id]; ok {
		return clonePartCosts(cost), cost.Validate()
	}
	if p.Settings != nil {
		if id == "ship" {
			return PartCosts{"misc": p.Settings.Cost}, nil
		}
		return PartCosts{}, nil
	}
	if p.Dme.Objects[s.Type] == nil {
		return PartCosts{}, nil
	}
	original, err := p.loadCostSource(s)
	if err != nil {
		return nil, err
	}
	return clonePartCosts(original.value), nil
}

func (p *Project) SetPartCosts(id string, cost PartCosts) error {
	if err := cost.Validate(); err != nil {
		return err
	}
	s, err := p.costScope(id)
	if err != nil {
		return err
	}
	if p.Settings == nil && p.Dme.Objects[s.Type] != nil {
		original, err := p.loadCostSource(s)
		if err != nil {
			return err
		}
		if _, ok := p.files[original.file]; !ok {
			p.files[original.file] = FileChange{Path: original.file, Before: append([]byte{}, p.costSources[original.file]...), Existed: true}
		}
	}
	if p.partCosts == nil {
		p.partCosts = map[string]PartCosts{}
	}
	if p.costEdited == nil {
		p.costEdited = map[string]bool{}
	}
	p.costEdited[id] = true
	p.partCosts[id] = clonePartCosts(cost)
	return nil
}

func (p *Project) loadCostSource(s CostScope) (costSource, error) {
	if value, ok := p.costOriginal[s.ID]; ok {
		return value, nil
	}
	if p.costSources == nil {
		var err error
		p.costSources, err = p.logicalSources()
		if err != nil {
			return costSource{}, err
		}
	}
	var candidates []costSource
	for file, data := range p.costSources {
		if !bytes.Contains(data, []byte(s.Type)) {
			continue
		}
		raw, found, explicit, err := sourceCostList(data, s.Type, s.Field)
		if err != nil {
			return costSource{}, err
		}
		if !found {
			continue
		}
		candidates = append(candidates, costSource{file: file, raw: raw, explicit: explicit})
	}
	var chosen *costSource
	for i := range candidates {
		if candidates[i].explicit {
			if chosen != nil {
				return costSource{}, fmt.Errorf("%s has multiple cost definitions; edit them in source", s.Name)
			}
			chosen = &candidates[i]
		}
	}
	if chosen == nil {
		file, err := p.roomTypeFile(s.Type)
		if err != nil {
			return costSource{}, err
		}
		for i := range candidates {
			if candidates[i].file == file {
				chosen = &candidates[i]
				break
			}
		}
	}
	if chosen == nil {
		return costSource{}, fmt.Errorf("cannot locate costs for %s", s.Name)
	}
	if _, err := sourcePath(p.Catalog.Root, chosen.file); err != nil {
		return costSource{}, err
	}
	raw := chosen.raw
	if !chosen.explicit {
		raw = p.Dme.Objects[s.Type].Vars.ValueV(s.Field, "null")
	}
	cost, err := parsePartCosts(raw)
	if err != nil {
		return costSource{}, fmt.Errorf("%s: %w", s.Name, err)
	}
	chosen.value = cost
	if p.costOriginal == nil {
		p.costOriginal = map[string]costSource{}
	}
	p.costOriginal[s.ID] = *chosen
	return *chosen, nil
}

func (p *Project) applyGeneratedCosts(hull, modules []byte) ([]byte, []byte, error) {
	for _, s := range p.CostScopes() {
		cost, ok := p.partCosts[s.ID]
		if !ok {
			continue
		}
		if err := cost.Validate(); err != nil {
			return nil, nil, err
		}
		var err error
		if s.ID == "ship" {
			hull, err = rewriteLiteralList(hull, s.Type, s.Field, renderPartCosts(cost))
		} else {
			modules, err = rewriteLiteralList(modules, s.Type, s.Field, renderPartCosts(cost))
		}
		if err != nil {
			return nil, nil, err
		}
	}
	return hull, modules, nil
}

func (p *Project) costChanges(changes []FileChange) ([]FileChange, error) {
	if p.Settings != nil || bytes.Equal(p.costBytes(), p.savedPartCosts) && len(changes) == 0 {
		return changes, nil
	}
	contents := map[string][]byte{}
	for _, c := range changes {
		contents[c.Path] = c.After
	}
	get := func(file string) []byte {
		if b, ok := contents[file]; ok {
			return b
		}
		return p.files[file].Before
	}
	for _, s := range p.CostScopes() {
		cost, edited := p.partCosts[s.ID]
		original, seen := p.costOriginal[s.ID]
		if !edited && (!seen || !p.costEdited[s.ID]) {
			continue
		}
		file := original.file
		if file == "" && p.rooms != nil {
			file = p.generatedSourceFile(s.ID)
		}
		if file == "" {
			return nil, fmt.Errorf("cannot locate costs for %s", s.Name)
		}
		if _, ok := p.files[file]; !ok {
			if err := p.track(file); err != nil {
				return nil, err
			}
		}
		value := original.raw
		if edited {
			if err := cost.Validate(); err != nil {
				return nil, err
			}
			value = renderPartCosts(cost)
		}
		data := get(file)
		var err error
		if !edited && !original.explicit {
			data, err = removeCostAssignment(data, s.Type, s.Field)
		} else {
			data, err = rewriteLiteralList(data, s.Type, s.Field, value)
		}
		if err != nil {
			return nil, err
		}
		contents[file] = data
	}
	for file, data := range contents {
		c, ok := p.files[file]
		if !ok {
			continue
		}
		c.After = data
		found := false
		for i := range changes {
			if changes[i].Path == file {
				changes[i].After = data
				found = true
				break
			}
		}
		if !found && !bytes.Equal(c.Before, c.After) {
			changes = append(changes, c)
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}
