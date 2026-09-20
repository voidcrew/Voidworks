package ship

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readShipProject(c *Catalog, h Hull) (string, []byte, error) {
	path, err := Inside(c.Root, "voidcrew/mapping/ship_projects/"+strings.TrimPrefix(h.Type, HullType+"/")+".ship.json")
	if err != nil {
		return "", nil, err
	}
	data, err := os.ReadFile(path)
	if !os.IsNotExist(err) {
		return path, data, err
	}
	matches, e := filepath.Glob(filepath.Join(c.Root, "voidcrew/mapping/ship_projects/*.ship.json"))
	if e != nil {
		return "", nil, e
	}
	info, e := filepath.Glob(filepath.Join(c.Root, "voidcrew/mapping/ship_projects/*.shipinfo.json"))
	if e != nil {
		return "", nil, e
	}
	matches = append(matches, info...)
	for _, candidate := range matches {
		b, e := os.ReadFile(candidate)
		if e != nil {
			return "", nil, e
		}
		var s Settings
		if json.Unmarshal(b, &s) == nil && s.Hull.Type == h.Type {
			if err == nil {
				return "", nil, fmt.Errorf("multiple projects register %s", h.Type)
			}
			path, data, err = candidate, b, nil
		}
	}
	return path, data, err
}

func (p *Project) fileID() string {
	if p.Settings != nil && p.Settings.FileID != "" {
		return p.Settings.FileID
	}
	if p.loadedFileID != "" {
		return p.loadedFileID
	}
	id, _ := p.roomID()
	return id
}

// Map creation follows the current display name; source editors retain logical paths.
func (p *Project) mapFileID() string {
	if p.sourceMoves != nil {
		if target := p.sourceNames[p.sourceMoves.Manifest]; target != "" {
			return strings.TrimSuffix(filepath.Base(target), ".shipinfo.json")
		}
	}
	return p.fileID()
}

func (p *Project) authoredPaths() []string {
	paths := append([]string{}, p.outputPaths()...)
	meta, code := p.crewPaths()
	if p.Crew != nil {
		paths = append(paths, meta, code)
	}
	if len(p.RoomAreas) > 0 {
		meta, code, _ = p.areaPaths()
		paths = append(paths, meta, code)
	}
	return paths
}

// RenameShip changes player-facing labels without moving maps or source files.
func (p *Project) RenameShip(name string) (err error) {
	before := p.Capture()
	defer func() {
		if err != nil {
			p.Restore(before)
		}
	}()
	if err = ShipNameError(p.Catalog, p.Dme, name, p.Hull.Type); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if p.Hull.Name == name {
		return nil
	}
	if p.Settings == nil {
		if err = p.prepareShipDetails(); err != nil {
			return err
		}
	}
	p.Hull.Name = name
	_, err = p.Changes()
	return err
}

// ShipFileName is the current stem used for this ship's dedicated files.
func (p *Project) ShipFileName() string { return p.mapFileID() }

// RenameShipFiles moves dedicated files and updates references, preserving labels and game IDs.
func (p *Project) RenameShipFiles(name string) (err error) {
	before := p.Capture()
	defer func() {
		if err != nil {
			p.Restore(before)
		}
	}()
	if err := ValidID(strings.TrimSpace(name)); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if p.ShipFileName() == name {
		return nil
	}
	var oldPaths, newPaths []string
	if p.Settings == nil {
		if err := p.prepareLoadedShipRename(name); err != nil {
			return err
		}
	} else {
		oldPaths = p.authoredPaths()
		oldID := p.Settings.FileID
		p.Settings.FileID = name
		newPaths = p.authoredPaths()
		p.Settings.FileID = oldID
	}
	for i, to := range newPaths {
		from := oldPaths[i]
		if from == to {
			continue
		}
		if _, err := os.Stat(to); err == nil && !p.renamedSources[to] {
			return fmt.Errorf("file already exists: %s", to)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := p.checkGenerated(from); err != nil {
			return err
		}
		if _, ok := p.files[from]; !ok {
			if err := p.track(from); err != nil {
				return err
			}
		}
		if _, ok := p.files[to]; !ok {
			if err := p.track(to); err != nil {
				return err
			}
		}
	}
	h := cloneHull(p.Hull)
	nextHull := cloneHull(h)
	renamedSuffixes := map[string]string{h.Suffix: name}
	moves := map[string]string{}
	for i, t := range h.Themes {
		from, err := p.Catalog.HullFile(h, t)
		if err != nil {
			return err
		}
		stem := renamedSuffixes[t.Suffix]
		if stem == "" {
			stem = name + "_" + fileName(t.Name)
			renamedSuffixes[t.Suffix] = stem
		}
		nextHull.Themes[i].Suffix = stem
		to, err := p.Catalog.HullFile(h, nextHull.Themes[i])
		if err != nil {
			return err
		}
		moves[from] = to
	}
	oldBase, err := p.Catalog.HullFile(h, Theme{})
	if err != nil {
		return err
	}
	nextHull.Suffix = name
	newBase, err := p.Catalog.HullFile(nextHull, Theme{})
	if err != nil {
		return err
	}
	moves[oldBase] = newBase
	for i, m := range h.Modules {
		nextHull.Modules[i].File = renamedModuleFile(m.File, name)
		for _, theme := range append([]string{""}, m.Themes...) {
			old, next := m.File, nextHull.Modules[i].File
			if theme != "" {
				old = strings.TrimSuffix(old, ".dmm") + "_" + theme + ".dmm"
				next = strings.TrimSuffix(next, ".dmm") + "_" + theme + ".dmm"
			}
			from, e := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, old))
			if e != nil {
				return e
			}
			if d := p.Documents[from]; d == nil || !d.Active {
				if _, e = os.Stat(from); os.IsNotExist(e) {
					continue
				} else if e != nil {
					return e
				}
			}
			to, e := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, next))
			if e != nil {
				return e
			}
			moves[from] = to
		}
	}
	if err := p.moveMaps(moves); err != nil {
		return err
	}
	if p.renamedSources == nil {
		p.renamedSources = map[string]bool{}
	}
	for i, from := range oldPaths {
		to := newPaths[i]
		if from == to {
			continue
		}
		p.renamedSources[from], p.renamedSources[to] = true, true
		if expected, ok := p.generatedBefore[from]; ok {
			p.expectGenerated(to, expected)
		}
	}
	if p.Settings != nil {
		p.Settings.FileID = name
	}
	p.Hull = nextHull
	if _, err := p.Changes(); err != nil {
		return err
	}
	return nil
}

func (p *Project) sourceRenameChanges(changes []FileChange) ([]FileChange, error) {
	if len(p.renamedSources) == 0 {
		return changes, nil
	}
	current := map[string]bool{}
	for _, path := range p.authoredPaths() {
		current[path] = true
	}
	kept := changes[:0]
	for _, c := range changes {
		if !p.renamedSources[c.Path] || current[c.Path] {
			kept = append(kept, c)
		}
	}
	changes = kept
	dme := p.files[p.Dme.RootFile]
	dme.After = append([]byte{}, dme.Before...)
	for i := 0; i < len(changes); i++ {
		if changes[i].Path == dme.Path {
			dme.After = changes[i].After
			changes = append(changes[:i], changes[i+1:]...)
			break
		}
	}
	deleted := map[string]bool{}
	for path := range p.renamedSources {
		if current[path] {
			continue
		}
		if err := p.checkGenerated(path); err != nil {
			return nil, err
		}
		c := p.files[path]
		if c.Existed {
			c.After, c.Delete = nil, true
			changes = append(changes, c)
		}
		deleted[strings.ToLower(path)] = true
	}
	dme.After = removeIncludes(p.Catalog.Root, dme.Path, dme.After, deleted)
	if !bytes.Equal(dme.Before, dme.After) {
		changes = append(changes, dme)
	}
	return changes, nil
}

// File names follow labels, while game typepaths, slot keys and component IDs
// remain stable so existing ship selections and custom code keep working.
func fileName(name string) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if separator && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	stem := b.String()
	if stem == "" {
		stem = "ship"
	}
	if stem[0] < 'a' || stem[0] > 'z' {
		stem = "ship_" + stem
	}
	if len(stem) > 40 {
		stem = strings.TrimRight(stem[:40], "_")
	}
	return stem
}

func (p *Project) referencedMaps() map[string]bool {
	paths := map[string]bool{}
	if p.sourceMoves != nil {
		for path := range p.sourceMoves.RetainedMaps {
			paths[path] = true
		}
	}
	addHull := func(h Hull) {
		add := func(relative string) { paths[filepath.Join(p.Catalog.Root, filepath.FromSlash(relative))] = true }
		if h.Suffix != "" {
			add(h.Prefix + h.Port + "_" + h.Suffix + ".dmm")
		}
		for _, t := range h.Themes {
			add(h.Prefix + h.Port + "_" + t.Suffix + ".dmm")
		}
		for _, m := range h.Modules {
			add(filepath.Join(p.Catalog.ModuleDir, m.File))
			for _, id := range m.Themes {
				add(filepath.Join(p.Catalog.ModuleDir, strings.TrimSuffix(m.File, ".dmm")+"_"+id+".dmm"))
			}
		}
	}
	addHull(p.Hull)
	// Maps shared by another hull must remain available after this ship moves.
	for _, other := range p.Catalog.Hulls {
		if other.Type == p.Hull.Type {
			continue
		}
		addHull(other)
	}
	return paths
}

func (p *Project) hasMapRenames() bool {
	for file := range p.deletedMaps {
		if d := p.Documents[file]; d != nil && d.Existed && !d.Active {
			return true
		}
	}
	if len(p.renamedMaps) == 0 {
		return false
	}
	referenced := p.referencedMaps()
	for file := range p.renamedMaps {
		if d := p.Documents[file]; d != nil && d.Existed && !referenced[file] {
			return true
		}
	}
	return false
}

// Preflight the whole move before changing any documents. Save applies the
// new files and removals in the same rollback-protected batch as registrations.
func (p *Project) moveMaps(moves map[string]string) error {
	seen := map[string]bool{}
	for from, to := range moves {
		if from == to {
			continue
		}
		if seen[strings.ToLower(to)] {
			return fmt.Errorf("two maps would use %s", to)
		}
		seen[strings.ToLower(to)] = true
		if _, err := sourcePath(p.Catalog.Root, to); err != nil {
			return err
		}
		if p.BeforeOpen != nil {
			if err := p.BeforeOpen(from); err != nil {
				return err
			}
			if err := p.BeforeOpen(to); err != nil {
				return err
			}
		}
		if d := p.Documents[to]; d != nil && d.Active {
			return fmt.Errorf("map already exists: %s", to)
		}
		if _, err := os.Stat(to); err == nil && !p.renamedMaps[to] {
			return fmt.Errorf("file already exists: %s", to)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if _, err := p.document(from); err != nil {
			return err
		}
	}
	if p.renamedMaps == nil {
		p.renamedMaps = map[string]bool{}
	}
	for from, to := range moves {
		if from == to {
			continue
		}
		old := p.Documents[from]
		m := old.Map.Copy()
		m.Path.Absolute, m.Name = to, filepath.Base(to)
		m.Path.Readable, _ = filepath.Rel(p.Catalog.Root, to)
		if prior := p.Documents[to]; prior != nil {
			*prior.Map, prior.Active = m, true
		} else {
			p.Documents[to] = &Document{Map: &m, Active: true, Initial: old.Initial, Unknown: append([]string{}, old.Unknown...)}
		}
		old.Active = false
		p.renamedMaps[from], p.renamedMaps[to] = true, true
	}
	return nil
}

func (p *Project) renameComponentMaps(scope, name string) error {
	moves := map[string]string{}
	if strings.HasPrefix(scope, "theme/") {
		for i, t := range p.Hull.Themes {
			if scope != "theme/"+t.ID {
				continue
			}
			from, err := p.Catalog.HullFile(p.Hull, t)
			if err != nil {
				return err
			}
			updated := t
			id := p.mapFileID()
			updated.Suffix = id + "_" + fileName(name)
			to, err := p.Catalog.HullFile(p.Hull, updated)
			if err != nil {
				return err
			}
			moves[from] = to
			// The base and default variant can share a hull map.
			if p.Hull.Suffix == t.Suffix && p.Settings == nil {
				if err := p.prepareMapField("ship", p.Hull.Type); err != nil {
					return err
				}
			}
			if p.Settings == nil {
				for _, shared := range p.Hull.Themes {
					if shared.Suffix != t.Suffix {
						continue
					}
					s, err := p.crewScope("theme/" + shared.ID)
					if err != nil {
						return err
					}
					if err = p.prepareMapField(s.ID, s.Type); err != nil {
						return err
					}
				}
			}
			if err := p.moveMaps(moves); err != nil {
				return err
			}
			for j := range p.Hull.Themes {
				if p.Hull.Themes[j].Suffix == t.Suffix {
					p.Hull.Themes[j].Suffix = updated.Suffix
				}
			}
			if p.Hull.Suffix == t.Suffix {
				p.Hull.Suffix = updated.Suffix
			}
			p.Hull.Themes[i].Suffix = updated.Suffix
			return nil
		}
	}
	for i, m := range p.Hull.Modules {
		if scope != "module/"+m.ID {
			continue
		}
		newFile := filepath.ToSlash(filepath.Join(filepath.Dir(m.File), fileName(name)+".dmm"))
		for _, theme := range append([]string{""}, m.Themes...) {
			old, next := m.File, newFile
			if theme != "" {
				old = strings.TrimSuffix(old, ".dmm") + "_" + theme + ".dmm"
				next = strings.TrimSuffix(next, ".dmm") + "_" + theme + ".dmm"
			}
			from, err := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, old))
			if err != nil {
				return err
			}
			if d := p.Documents[from]; d == nil || !d.Active {
				if _, err := os.Stat(from); os.IsNotExist(err) {
					continue
				} else if err != nil {
					return err
				}
			}
			to, err := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, next))
			if err != nil {
				return err
			}
			moves[from] = to
		}
		if err := p.moveMaps(moves); err != nil {
			return err
		}
		p.Hull.Modules[i].File = newFile
		return nil
	}
	return nil
}

func mapField(h Hull, scope string) (field, value string) {
	if scope == "ship" {
		return "suffix", h.Suffix
	}
	for _, t := range h.Themes {
		if scope == "theme/"+t.ID {
			return "template_suffix", t.Suffix
		}
	}
	for _, m := range h.Modules {
		if scope == "module/"+m.ID {
			return "map_file", m.File
		}
	}
	return "", ""
}

func (p *Project) prepareMapField(scope, typePath string) error {
	file, err := p.roomTypeFile(typePath)
	if err != nil {
		return err
	}
	if err = p.roomSource(file); err != nil {
		return err
	}
	if p.rooms.mapFields == nil {
		p.rooms.mapFields = map[string]nameTarget{}
	}
	p.rooms.mapFields[scope] = nameTarget{file, typePath}
	return nil
}

func renamedModuleFile(file, id string) string {
	file = filepath.ToSlash(file)
	_, rest, found := strings.Cut(file, "/")
	if !found {
		rest = file
	}
	return id + "/" + rest
}
