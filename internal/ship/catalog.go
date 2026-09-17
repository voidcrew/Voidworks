// Package ship models Voidcrew's modular ship templates without writing their sources.
package ship

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmvars"
)

const HullType = "/datum/map_template/shuttle/voidcrew"
const SlotMarker = "/obj/modular_map_root/ship_upgrade"
const Connector = "/obj/modular_map_connector"

type Theme struct {
	ID, Name, Suffix string
	Description      *string  `json:",omitempty"` // nil keeps the inherited description.
	Slots            []string // nil inherits the hull's slots; an empty list disables them.
	Default          bool
}

type Module struct {
	ID, Name, Slot, File string
	Description          *string `json:",omitempty"`
	Themes               []string
	Default              bool
}

type Hull struct {
	Type, Name, Prefix, Port, Suffix string
	Description                      string `json:",omitempty"`
	Hidden                           bool   `json:",omitempty"`
	Slots                            []string
	Themes                           []Theme
	Modules                          []Module
	// Fixed ships have no upgrade slots yet. Their first upgrade room makes
	// them modular, which the game requires before selling them or rolling
	// them into the roundstart fleet.
	Fixed bool `json:",omitempty"`
}

type Catalog struct {
	Root, ModuleDir string
	Hulls           []Hull
}

func text(v *dmvars.Variables, name string) string {
	s, _ := dmUnquote(v.ValueV(name, `""`))
	return s
}

func description(v *dmvars.Variables) *string {
	s, err := dmUnquote(v.ValueV("desc", "null"))
	if err != nil {
		return nil
	}
	return &s
}

// stringList deliberately rejects expressions and associative lists: these fields
// are sequences of string IDs, already constant-folded by the DM parser.
func stringList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return nil, nil
	}
	if strings.HasPrefix(s, `"`) {
		v, err := dmUnquote(s)
		return []string{v}, err
	}
	if !strings.HasPrefix(s, "list(") || !strings.HasSuffix(s, ")") {
		return nil, fmt.Errorf("expected a string list, got %s", s)
	}
	s = strings.TrimSpace(s[5 : len(s)-1])
	result := []string{}
	for s != "" {
		if s[0] != '"' {
			return nil, fmt.Errorf("expected a quoted ID in %s", s)
		}
		end := 1
		for ; end < len(s); end++ {
			if s[end] == '\\' {
				end++
				continue
			}
			if s[end] == '"' {
				break
			}
		}
		if end >= len(s) {
			return nil, fmt.Errorf("unterminated ID")
		}
		v, err := dmUnquote(s[:end+1])
		if err != nil {
			return nil, err
		}
		result = append(result, v)
		s = strings.TrimSpace(s[end+1:])
		if s == "" {
			break
		}
		if s[0] != ',' {
			return nil, fmt.Errorf("expected comma after ID")
		}
		s = strings.TrimSpace(s[1:])
	}
	return result, nil
}

func Contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (m Module) Available(theme string) bool {
	if theme == "" {
		return m.Themes == nil
	}
	return Contains(m.Themes, theme)
}

func (h Hull) SlotsFor(t Theme) []string {
	if t.Slots != nil {
		return t.Slots
	}
	return h.Slots
}

// Inside confines all profile paths (including existing symlinks) to the project.
func Inside(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", fmt.Errorf("expected a project-relative path: %s", relative)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project: %s", relative)
	}
	ancestor := path
	resolved, err := filepath.EvalSymlinks(ancestor)
	for os.IsNotExist(err) && ancestor != root && filepath.Dir(ancestor) != ancestor {
		ancestor = filepath.Dir(ancestor)
		resolved, err = filepath.EvalSymlinks(ancestor)
	}
	if err == nil {
		realRoot, e := filepath.EvalSymlinks(root)
		if e != nil {
			return "", e
		}
		rel, e = filepath.Rel(realRoot, resolved)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("symlink escapes project: %s", relative)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func (c Catalog) HullFile(h Hull, t Theme) (string, error) {
	suffix := h.Suffix
	if t.ID != "" {
		suffix = t.Suffix
	}
	if suffix == "" {
		return "", fmt.Errorf("%s has no hull suffix", h.Name)
	}
	return Inside(c.Root, h.Prefix+h.Port+"_"+suffix+".dmm")
}

func (c Catalog) ModuleFile(m Module, theme string) (string, error) {
	base, err := Inside(c.Root, filepath.Join(c.ModuleDir, m.File))
	if err != nil {
		return "", err
	}
	if theme != "" {
		themed, err := Inside(c.Root, filepath.Join(c.ModuleDir, strings.TrimSuffix(m.File, ".dmm")+"_"+theme+".dmm"))
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(themed); err == nil && !info.IsDir() {
			return themed, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	if _, err := os.Stat(base); err != nil {
		return "", fmt.Errorf("%s: neither a themed map nor its base is available: %w", m.Name, err)
	}
	return base, nil
}

func Discover(dme *dmenv.Dme) (*Catalog, error) {
	if dme == nil || dme.Objects[HullType] == nil {
		return nil, fmt.Errorf("load a Voidcrew environment to use the ship workspace")
	}
	c := &Catalog{Root: dme.RootDir}
	marker := dme.Objects[SlotMarker]
	if marker == nil {
		return nil, fmt.Errorf("ship upgrade marker type is missing")
	}
	config, err := Inside(c.Root, text(marker.Vars, "config_file"))
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(config)
	if err != nil {
		return nil, err
	}
	// Only the root directory value is needed; rooms are resolved from DM datums.
	match := regexp.MustCompile(`(?m)^\s*directory\s*=\s*("(?:[^"\\]|\\.)*")\s*(?:#.*)?$`).FindSubmatch(data)
	if len(match) != 2 {
		return nil, fmt.Errorf("%s needs a quoted directory value", config)
	}
	c.ModuleDir, err = strconv.Unquote(string(match[1]))
	if err != nil {
		return nil, err
	}
	modules := map[string][]Module{}
	themes := map[string][]Theme{}
	paths := make([]string, 0, len(dme.Objects))
	for path := range dme.Objects {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		v := dme.Objects[path].Vars
		id := text(v, "id")
		forShip := v.ValueV("for_ship", "")
		if id == "" || !strings.HasPrefix(forShip, HullType+"/") {
			continue
		}
		if strings.HasPrefix(path, "/datum/ship_upgrade_module/") {
			allowed, err := stringList(v.ValueV("for_theme", "null"))
			if err != nil {
				return nil, fmt.Errorf("%s for_theme: %w", path, err)
			}
			m := Module{ID: id, Name: text(v, "name"), Description: description(v), Slot: text(v, "slot"), File: text(v, "map_file"), Themes: allowed, Default: v.IntV("is_default", 0) != 0}
			for _, prior := range modules[forShip] {
				if prior.ID == id {
					return nil, fmt.Errorf("duplicate module ID %s", id)
				}
			}
			modules[forShip] = append(modules[forShip], m)
		} else if strings.HasPrefix(path, "/datum/ship_theme/") {
			slots, err := stringList(v.ValueV("upgrade_slot_ids", "null"))
			if err != nil {
				return nil, fmt.Errorf("%s slots: %w", path, err)
			}
			for _, prior := range themes[forShip] {
				if prior.ID == id {
					return nil, fmt.Errorf("duplicate theme ID %s", id)
				}
			}
			themes[forShip] = append(themes[forShip], Theme{ID: id, Name: text(v, "name"), Description: description(v), Suffix: text(v, "template_suffix"), Slots: slots, Default: v.IntV("is_default", 0) != 0})
		}
	}
	allHulls := map[string]Hull{}
	for _, path := range paths {
		if !strings.HasPrefix(path, HullType+"/") {
			continue
		}
		obj := dme.Objects[path]
		v := obj.Vars
		if v.ValueV("abstract", "") == path {
			continue
		}
		// List registered base hulls once, rather than every inherited theme
		// subtype. Ships without registered rooms, including fixed layouts that
		// are not modular yet, are listed from their own top-level definition.
		registered := modules[path] != nil || themes[path] != nil
		if !registered && (strings.Contains(strings.TrimPrefix(path, HullType+"/"), "/") || text(v, "suffix") == "") {
			continue
		}
		// Rooms registered for a hull that never enabled slots are not loaded
		// by the game; keep such a definition out of the library as before.
		fixed := v.IntV("has_upgrade_slots", 0) == 0
		if fixed && registered {
			continue
		}
		slots, err := stringList(v.ValueV("upgrade_slot_ids", "null"))
		if err != nil {
			return nil, fmt.Errorf("%s slots: %w", path, err)
		}
		h := Hull{Type: path, Name: text(v, "name"), Description: text(v, "catalog_desc"), Hidden: v.IntV("player_hidden", 0) != 0, Prefix: text(v, "prefix"), Port: text(v, "port_id"), Suffix: text(v, "suffix"), Slots: slots, Themes: themes[path], Modules: modules[path], Fixed: fixed}
		sort.SliceStable(h.Themes, func(i, j int) bool {
			if h.Themes[i].Default != h.Themes[j].Default {
				return h.Themes[i].Default
			}
			return h.Themes[i].Name < h.Themes[j].Name
		})
		allHulls[h.Type] = h
		if !h.Hidden {
			c.Hulls = append(c.Hulls, h)
		}
	}
	// Draft projects remain available even while hidden from the purchase screen.
	projects, err := filepath.Glob(filepath.Join(c.Root, "voidcrew", "mapping", "ship_projects", "*.ship.json"))
	if err != nil {
		return nil, err
	}
	info, err := filepath.Glob(filepath.Join(c.Root, "voidcrew", "mapping", "ship_projects", "*.shipinfo.json"))
	if err != nil {
		return nil, err
	}
	projects = append(projects, info...)
	for _, path := range projects {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var s Settings
		if err = json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if err = ValidID(s.ID); err != nil {
			return nil, err
		}
		if (s.Version != 1 && s.Version != 2) || s.Version == 1 && s.Hull.Type != HullType+"/"+s.ID {
			return nil, fmt.Errorf("unsupported project %s", path)
		}
		if s.Version == 2 {
			loaded, ok := allHulls[s.Hull.Type]
			if !ok {
				return nil, fmt.Errorf("ship definition missing for %s", s.Hull.Type)
			}
			s.Hull = loaded
		}
		found := false
		for i, h := range c.Hulls {
			if h.Type == s.Hull.Type {
				c.Hulls[i] = s.Hull
				found = true
				break
			}
		}
		if !found {
			c.Hulls = append(c.Hulls, s.Hull)
		}
	}
	sort.Slice(c.Hulls, func(i, j int) bool { return c.Hulls[i].Name < c.Hulls[j].Name })
	return c, nil
}
