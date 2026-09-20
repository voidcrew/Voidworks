package ship

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// Source editors use stable logical paths for the lifetime of a project. The
// save boundary translates these to physical filenames, including after Undo.
// Physical baselines retain the usual external-edit and rollback protection.
type sourceMoves struct {
	Logical        map[string]bool
	Physical       map[string]FileChange
	Saved          map[string]string
	Manifest       string
	ManifestBefore FileChange
	RetainedMaps   map[string]bool
}

func clonePaths(v map[string]string) map[string]string {
	if v == nil {
		return nil
	}
	r := make(map[string]string, len(v))
	for k, value := range v {
		r[k] = value
	}
	return r
}

func pathAt(names map[string]string, path string) string {
	if next := names[path]; next != "" {
		return next
	}
	return path
}

func (p *Project) sourceMovesPending() bool {
	return p.sourceMoves != nil && !reflect.DeepEqual(p.sourceNames, p.sourceMoves.Saved)
}

func (p *Project) watchSource(path string) error {
	if _, err := sourcePath(p.Catalog.Root, path); err != nil {
		return err
	}
	if _, ok := p.files[path]; !ok {
		if err := p.track(path); err != nil {
			return err
		}
	}
	p.sourceMoves.Logical[path] = true
	return p.watchPhysical(path)
}

func (p *Project) watchPhysical(path string) error {
	if _, ok := p.sourceMoves.Physical[path]; ok {
		return nil
	}
	if _, err := sourcePath(p.Catalog.Root, path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	p.sourceMoves.Physical[path] = FileChange{Path: path, Before: data, Existed: err == nil}
	return nil
}

func (p *Project) logicalSources() (map[string][]byte, error) {
	sources, err := removalSources(p.Catalog.Root, p.Dme.RootFile)
	if err != nil || p.sourceMoves == nil {
		return sources, err
	}
	reverse := map[string]string{}
	for logical, physical := range p.sourceMoves.Saved {
		reverse[physical] = logical
	}
	result := map[string][]byte{}
	for physical, data := range sources {
		logical := pathAt(reverse, physical)
		result[logical] = rewriteMovedIncludes(logical, data, reverse)
	}
	for logical := range p.sourceMoves.Logical {
		if c := p.files[logical]; c.Existed {
			result[logical] = c.Before
		}
	}
	return result, nil
}

func rewriteMovedIncludes(file string, data []byte, names map[string]string) []byte {
	mask := dmSourceMask(data, false)
	matches := removalInclude.FindAllSubmatchIndex(mask, -1)
	result := append([]byte{}, data...)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		old := string(data[m[2]:m[3]])
		path := includePath(file, strings.ReplaceAll(old, "\\", "/"))
		next := pathAt(names, path)
		if next == path {
			continue
		}
		rel, err := filepath.Rel(filepath.Dir(file), next)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(old, "\\") {
			rel = strings.ReplaceAll(rel, "/", "\\")
		}
		result = append(append(append([]byte{}, result[:m[2]]...), rel...), result[m[3]:]...)
	}
	return result
}

// Move only dedicated definition files. Shared files retain their filenames;
// their ship-specific blocks are still updated by the normal source editors.
func (p *Project) prepareLoadedShipRename(name string) error {
	if err := p.prepareShipDetails(); err != nil {
		return err
	}
	if p.sourceMoves == nil {
		p.sourceMoves = &sourceMoves{Logical: map[string]bool{}, Physical: map[string]FileChange{}}
		p.sourceMoves.Manifest = filepath.Join(p.Catalog.Root, "voidcrew/mapping/ship_projects", p.fileID()+".shipinfo.json")
		if err := p.watchSource(p.sourceMoves.Manifest); err != nil {
			return err
		}
		p.sourceMoves.ManifestBefore = p.files[p.sourceMoves.Manifest]
	}
	// Keep the source scan cached: the parser and every source editor still use
	// these logical locations even after files have been saved under new names.
	if p.costSources == nil {
		var err error
		p.costSources, err = p.logicalSources()
		if err != nil {
			return err
		}
	}
	roots := []string{p.Hull.Type}
	shipID := strings.TrimPrefix(p.Hull.Type, HullType+"/")
	roots = append(roots, "/area/shuttle/voidcrew/"+shipID, "/obj/docking_port/mobile/voidcrew/"+shipID)
	for _, scope := range p.baseCrewScopes() {
		roots = append(roots, scope.Type)
	}
	for _, jobs := range p.editedCrewRosters() {
		for _, job := range jobs {
			if job.BaseOutfit != "" {
				roots = append(roots, p.crewOutfitPath(job))
			}
		}
	}
	for _, data := range p.costSources {
		roots = append(roots, ownedRegistrationTypes(data, []string{p.Hull.Type})...)
	}
	// Hidden registrations and literal map references can be absent from the
	// library. Copy their shared maps for this ship, retaining the originals.
	p.sourceMoves.RetainedMaps = map[string]bool{}
	for path, obj := range p.Dme.Objects {
		if withinTypes(path, roots) || obj.Vars.ValueV("for_ship", "") == p.Hull.Type {
			continue
		}
		for _, file := range registeredMapPaths(p.Catalog.Root, path, obj) {
			p.sourceMoves.RetainedMaps[file] = true
		}
		if strings.HasPrefix(path, "/datum/ship_upgrade_module/") {
			file := text(obj.Vars, "map_file")
			if file == "" {
				continue
			}
			if full, err := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, file)); err == nil {
				p.sourceMoves.RetainedMaps[full] = true
			}
			themes, err := stringList(obj.Vars.ValueV("for_theme", "null"))
			if err != nil {
				return err
			}
			for _, theme := range themes {
				if full, err := Inside(p.Catalog.Root, filepath.Join(p.Catalog.ModuleDir, strings.TrimSuffix(file, ".dmm")+"_"+theme+".dmm")); err == nil {
					p.sourceMoves.RetainedMaps[full] = true
				}
			}
		}
	}
	literalMaps := regexp.MustCompile(`["']([^"'\r\n]+\.dmm)["']`)
	for _, data := range p.costSources {
		for _, literal := range literalMaps.FindAllSubmatch(dmSourceMask(data, false), -1) {
			name := strings.ReplaceAll(string(literal[1]), "\\", "/")
			if full, err := Inside(p.Catalog.Root, name); err == nil {
				p.sourceMoves.RetainedMaps[full] = true
			}
		}
	}
	owned := map[string]bool{}
	for file, data := range p.costSources {
		if strings.EqualFold(filepath.Ext(file), ".dm") && containsRemovalRoot(data, roots) {
			rest, _, err := removeDefinitions(data, roots)
			if err != nil {
				return fmt.Errorf("cannot rename %s: %w", file, err)
			}
			if len(bytes.TrimSpace(dmSourceMask(rest, true))) == 0 {
				owned[file] = true
			}
		}
	}
	// Include generated companions, including those first created later in this
	// session. Their game identifiers remain independent of their filenames.
	meta, code := p.crewPaths()
	areaMeta, areaCode, err := p.areaPaths()
	if err != nil {
		return err
	}
	for _, file := range []string{meta, code, areaMeta, areaCode, p.rooms.code, p.rooms.themeFile, p.sourceMoves.Manifest} {
		if file == "" {
			continue
		}
		_, compiled := p.costSources[file]
		if !compiled || strings.HasSuffix(file, ".json") {
			owned[file] = true
		}
	}
	names := map[string]string{}
	used := map[string]string{}
	files := make([]string, 0, len(owned))
	for file := range owned {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		if err := p.watchSource(file); err != nil {
			return err
		}
		ext := filepath.Ext(file)
		if strings.HasSuffix(file, ".shipinfo.json") {
			ext = ".shipinfo.json"
		}
		if strings.HasSuffix(file, ".crew.json") {
			ext = ".crew.json"
		}
		if strings.HasSuffix(file, ".areas.json") {
			ext = ".areas.json"
		}
		to := filepath.Join(filepath.Dir(file), name+ext)
		// Several dedicated files can live in one directory; preserve each role.
		if prior := used[strings.ToLower(to)]; prior != "" && prior != file {
			to = filepath.Join(filepath.Dir(file), name+"_"+strings.TrimSuffix(filepath.Base(file), ext)+ext)
		}
		if prior := used[strings.ToLower(to)]; prior != "" && prior != file {
			return fmt.Errorf("two source files would use %s", to)
		}
		used[strings.ToLower(to)] = file
		if err := p.watchPhysical(to); err != nil {
			return err
		}
		if p.sourceMoves.Physical[to].Existed && to != pathAt(p.sourceMoves.Saved, file) {
			return fmt.Errorf("file already exists: %s", to)
		}
		names[file] = to
	}
	// Every including file is watched too, so nested includes are updated and a
	// concurrent edit anywhere in the include chain cannot be overwritten.
	for file, data := range p.costSources {
		if !bytes.Equal(data, rewriteMovedIncludes(file, data, names)) {
			if err := p.watchSource(file); err != nil {
				return err
			}
		}
	}
	if err := p.watchSource(p.Dme.RootFile); err != nil {
		return err
	}
	for _, scope := range p.baseCrewScopes() {
		if _, exists := p.Dme.Objects[scope.Type]; exists {
			if err := p.prepareMapField(scope.ID, scope.Type); err != nil {
				return err
			}
		}
	}
	p.sourceNames = names
	return nil
}

func (p *Project) relocatedChanges(changes []FileChange) ([]FileChange, error) {
	moves := p.sourceMoves
	if moves == nil {
		return changes, nil
	}
	virtual := map[string]FileChange{}
	var result []FileChange
	for _, c := range changes {
		if moves.Logical[c.Path] {
			virtual[c.Path] = c
		} else {
			result = append(result, c)
		}
	}
	manifest, provided := virtual[moves.Manifest]
	if !provided {
		manifest = p.files[moves.Manifest]
	}
	if len(p.sourceNames) > 0 {
		id, _ := p.roomID()
		fileID := strings.TrimSuffix(filepath.Base(pathAt(p.sourceNames, moves.Manifest)), ".shipinfo.json")
		manifest.After, _ = json.MarshalIndent(Settings{Version: 2, ID: id, FileID: fileID, Hull: p.Hull}, "", "  ")
		manifest.After = append(manifest.After, '\n')
	} else if provided {
		// The details editor already supplied the metadata or its undo.
	} else if !moves.ManifestBefore.Existed {
		manifest.Delete = true
	} else {
		manifest.After = moves.ManifestBefore.Before
	}
	virtual[moves.Manifest] = manifest
	active := map[string]bool{}
	for logical := range moves.Logical {
		active[pathAt(p.sourceNames, logical)] = true
	}
	for logical := range moves.Logical {
		dest := pathAt(p.sourceNames, logical)
		c, changed := virtual[logical]
		if !changed {
			c = p.files[logical]
			c.After, c.Delete = c.Before, !c.Existed
		}
		physical, watched := moves.Physical[dest]
		if !watched {
			return nil, fmt.Errorf("untracked rename destination: %s", dest)
		}
		physical.After = rewriteMovedIncludes(logical, c.After, p.sourceNames)
		physical.Delete = c.Delete
		if physical.Existed && physical.Delete || !physical.Delete && (!physical.Existed || !bytes.Equal(physical.Before, physical.After)) {
			result = append(result, physical)
		}
		old := pathAt(moves.Saved, logical)
		if old != dest && !active[old] {
			prior := moves.Physical[old]
			if prior.Existed {
				prior.Delete, prior.After = true, nil
				result = append(result, prior)
			}
		}
	}
	return result, nil
}

func (p *Project) acceptRelocated(changes []FileChange) []FileChange {
	moves := p.sourceMoves
	if moves == nil {
		return changes
	}
	reverse := map[string]string{}
	for logical := range moves.Logical {
		reverse[pathAt(p.sourceNames, logical)] = logical
	}
	result := []FileChange{}
	for _, c := range changes {
		if _, ok := moves.Physical[c.Path]; ok {
			moves.Physical[c.Path] = FileChange{Path: c.Path, Before: c.After, Existed: !c.Delete}
		}
		if logical, ok := reverse[c.Path]; ok {
			c.Path = logical
			c.After = rewriteMovedIncludes(logical, c.After, reverse)
			result = append(result, c)
		} else if _, moved := moves.Physical[c.Path]; !moved {
			result = append(result, c)
		}
	}
	moves.Saved = clonePaths(p.sourceNames)
	return result
}
