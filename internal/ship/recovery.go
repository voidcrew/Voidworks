package ship

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"strings"
)

type recoveryDocument struct {
	Active, Existed          bool
	Current, Initial, Before []byte
	Fingerprint              [32]byte
}

type recoveryProject struct {
	SourceMoves         *sourceMoves
	SourceNames         map[string]string
	LoadedFileID        string
	Version             int
	Environment         string
	Documents           map[string]recoveryDocument
	Hull                Hull
	Settings            *Settings
	Files               map[string]FileChange
	SavedSettings       []byte
	RoomAreas           []RoomArea
	SavedAreas          []byte
	DraftAreaPaths      map[string]bool
	Rooms               *roomEditing
	SavedRooms          []byte
	Crew                *CrewConfig
	SavedCrew           []byte
	CrewOriginal        map[string][]CrewJob
	GeneratedBefore     map[string][]byte
	RegistrationUpgrade bool
	PartCosts           map[string]PartCosts
	SavedPartCosts      []byte
	CostOriginal        map[string]costSource
	CostSources         map[string][]byte
	CostEdited          map[string]bool
	RenamedMaps         map[string]bool
	DeletedMaps         map[string]bool
	RenamedSources      map[string]bool
}

// CaptureRecovery preserves unfinished edits without validating or writing game files.
// Call on the editor thread; the returned bytes can be written in the background.
func (p *Project) CaptureRecovery() ([]byte, error) {
	// Version 2 preserves slot-assignment rewrites; older editors must not
	// restore them without the corresponding source rewrite support.
	r := recoveryProject{Version: 2, Environment: p.Dme.RootFile, Documents: map[string]recoveryDocument{}}
	if p.Crew != nil && len(p.Crew.ModuleThemes) > 0 {
		r.Version = 3 // Older editors do not preserve independent room rosters.
	}
	if p.rooms != nil && p.rooms.details.file != "" {
		r.Version = 4
	}
	if p.sourceMoves != nil {
		r.Version = 4
	}
	r.SourceMoves, r.SourceNames, r.LoadedFileID = p.sourceMoves, p.sourceNames, p.loadedFileID
	r.Hull = p.Hull
	r.Settings = p.Settings
	r.Files = p.files
	r.SavedSettings = p.savedSettings
	r.RoomAreas = p.RoomAreas
	r.SavedAreas = p.savedAreas
	r.DraftAreaPaths = p.draftAreaPaths
	r.Rooms = p.rooms
	r.SavedRooms = p.savedRooms
	r.Crew = p.Crew
	r.SavedCrew = p.savedCrew
	r.CrewOriginal = p.crewOriginal
	r.GeneratedBefore = p.generatedBefore
	r.RegistrationUpgrade = p.registrationUpgrade
	r.PartCosts = p.partCosts
	r.SavedPartCosts = p.savedPartCosts
	r.CostOriginal = p.costOriginal
	// The cost browser caches the entire source tree. Keep only the original
	// files for inspected scopes; new scopes can rebuild that read-only cache.
	r.CostSources = nil
	r.Files = make(map[string]FileChange, len(p.files)+len(p.costOriginal))
	for path, f := range p.files {
		r.Files[path] = f
	}
	for _, original := range p.costOriginal {
		if original.file == "" {
			continue
		}
		if _, ok := r.Files[original.file]; !ok {
			data, ok := p.costSources[original.file]
			if !ok {
				return nil, fmt.Errorf("missing cost baseline for %s", original.file)
			}
			r.Files[original.file] = FileChange{Path: original.file, Before: data, Existed: true}
		}
	}
	r.CostEdited = p.costEdited
	r.RenamedMaps = p.renamedMaps
	r.DeletedMaps = p.deletedMaps
	r.RenamedSources = p.renamedSources
	for path, d := range p.Documents {
		v := recoveryDocument{Active: d.Active, Existed: d.Existed, Before: d.Before, Fingerprint: d.fingerprint, Current: RawData(d.Map).EncodeTGM()}
		if d.Initial != nil {
			v.Initial = d.Initial.EncodeTGM()
		}
		r.Documents[path] = v
	}
	return json.Marshal(r)
}

// RecoverProject restores an unsaved draft in memory. Original file baselines are
// retained so normal Save still refuses to overwrite files changed elsewhere.
func RecoverProject(c *Catalog, dme *dmenv.Dme, data []byte) (*Project, error) {
	var r recoveryProject
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if (r.Version < 1 || r.Version > 4) || !sameRecoveryPath(r.Environment, dme.RootFile) {
		return nil, fmt.Errorf("recovery belongs to a different project or version")
	}
	if !strings.HasPrefix(r.Hull.Type, HullType+"/") {
		return nil, fmt.Errorf("invalid recovered ship")
	}
	if r.Settings != nil {
		if err := ValidID(r.Settings.ID); err != nil {
			return nil, err
		}
		if r.Hull.Type != HullType+"/"+r.Settings.ID {
			return nil, fmt.Errorf("recovered ship ID does not match its template")
		}
	}
	paths := []string{r.Environment}
	if r.LoadedFileID != "" {
		if err := ValidID(r.LoadedFileID); err != nil {
			return nil, err
		}
	}
	if r.SourceMoves != nil {
		paths = append(paths, r.SourceMoves.Manifest, r.SourceMoves.ManifestBefore.Path)
		for path := range r.SourceMoves.RetainedMaps {
			paths = append(paths, path)
		}
		for path := range r.SourceMoves.Logical {
			paths = append(paths, path)
		}
		for path, c := range r.SourceMoves.Physical {
			paths = append(paths, path, c.Path)
		}
		for path, to := range r.SourceMoves.Saved {
			paths = append(paths, path, to)
		}
	}
	for path, to := range r.SourceNames {
		paths = append(paths, path, to)
	}
	for path := range r.Documents {
		paths = append(paths, path)
	}
	for path, v := range r.Files {
		paths = append(paths, path, v.Path)
	}
	for path := range r.GeneratedBefore {
		paths = append(paths, path)
	}
	for path := range r.CostSources {
		paths = append(paths, path)
	}
	for _, v := range r.CostOriginal {
		paths = append(paths, v.file)
	}
	for _, m := range []map[string]bool{r.RenamedMaps, r.DeletedMaps, r.RenamedSources} {
		for path := range m {
			paths = append(paths, path)
		}
	}
	if v := r.Rooms; v != nil {
		paths = append(paths, v.code, v.themeFile, v.themeList.file, v.details.file, v.manifest, v.portLabel.file)
		for path, f := range v.sources {
			paths = append(paths, path, f.Path)
		}
		for _, m := range []map[string]nameTarget{v.names, v.descriptions, v.mapFields, v.removals, v.defaults, v.themeTargets, v.moduleSlots} {
			for _, n := range m {
				paths = append(paths, n.file)
			}
		}
		for _, n := range v.moduleThemes {
			paths = append(paths, n.own.file, n.shared.file)
		}
	}
	for _, path := range paths {
		if path != "" {
			rel, err := filepath.Rel(c.Root, path)
			if err != nil {
				return nil, err
			}
			if _, err = Inside(c.Root, rel); err != nil {
				return nil, err
			}
		}
	}
	raw, initial := map[string]*dmmdata.DmmData{}, map[string]*dmmdata.DmmData{}
	for path, v := range r.Documents {
		if filepath.Ext(path) != ".dmm" {
			return nil, fmt.Errorf("invalid recovered map path")
		}
		var err error
		raw[path], err = dmmdata.Read(path, bytes.NewReader(v.Current))
		if err != nil {
			return nil, fmt.Errorf("recover %s: %w", filepath.Base(path), err)
		}
		if len(v.Initial) > 0 {
			initial[path], err = dmmdata.Read(path, bytes.NewReader(v.Initial))
			if err != nil {
				return nil, err
			}
		}
	}
	p := &Project{Catalog: c, Dme: dme, Documents: map[string]*Document{}}
	p.Hull = r.Hull
	p.Settings = r.Settings
	p.files = r.Files
	p.savedSettings = r.SavedSettings
	p.RoomAreas = r.RoomAreas
	p.savedAreas = r.SavedAreas
	p.draftAreaPaths = r.DraftAreaPaths
	p.rooms = r.Rooms
	p.savedRooms = r.SavedRooms
	p.Crew = r.Crew
	p.savedCrew = r.SavedCrew
	p.crewOriginal = r.CrewOriginal
	p.generatedBefore = r.GeneratedBefore
	p.registrationUpgrade = r.RegistrationUpgrade
	p.partCosts = r.PartCosts
	p.savedPartCosts = r.SavedPartCosts
	p.costOriginal = r.CostOriginal
	p.costSources = r.CostSources
	p.costEdited = r.CostEdited
	p.sourceMoves, p.sourceNames, p.loadedFileID = r.SourceMoves, r.SourceNames, r.LoadedFileID
	p.renamedMaps = r.RenamedMaps
	p.deletedMaps = r.DeletedMaps
	p.renamedSources = r.RenamedSources
	if p.files == nil {
		p.files = map[string]FileChange{}
	}
	if p.draftAreaPaths == nil {
		p.draftAreaPaths = map[string]bool{}
	}
	if p.Settings != nil {
		if err := p.installTypes(); err != nil {
			return nil, err
		}
	}
	for _, area := range p.RoomAreas {
		if err := validateRoomArea(area); err != nil {
			return nil, err
		}
		if err := dme.AddDraftType(area.Path, areaValues(area)); err != nil {
			return nil, err
		}
		p.draftAreaPaths[area.Path] = true
	}
	for path, v := range r.Documents {
		m, unknown := dmmap.New(dme, raw[path], "")
		keepUnknownPrefabs(m, raw[path], unknown)

		p.protect(m)
		p.Documents[path] = &Document{Map: m, Initial: initial[path], Active: v.Active, Existed: v.Existed, Before: v.Before, fingerprint: v.Fingerprint, Unknown: sortedPaths(unknown)}
	}
	return p, nil
}
func sameRecoveryPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

type recoveryNameTarget struct {
	File     string
	TypePath string
}

func (v nameTarget) MarshalJSON() ([]byte, error) {
	return json.Marshal(recoveryNameTarget{File: v.file, TypePath: v.typePath})
}
func (v *nameTarget) UnmarshalJSON(data []byte) error {
	var r recoveryNameTarget
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	v.file = r.File
	v.typePath = r.TypePath
	return nil
}

type recoveryModuleThemeTarget struct {
	Own    nameTarget
	Shared nameTarget
}

func (v moduleThemeTarget) MarshalJSON() ([]byte, error) {
	return json.Marshal(recoveryModuleThemeTarget{Own: v.own, Shared: v.shared})
}
func (v *moduleThemeTarget) UnmarshalJSON(data []byte) error {
	var r recoveryModuleThemeTarget
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	v.own = r.Own
	v.shared = r.Shared
	return nil
}

type recoveryCostSource struct {
	File     string
	Raw      string
	Explicit bool
	Value    PartCosts
}

func (v costSource) MarshalJSON() ([]byte, error) {
	return json.Marshal(recoveryCostSource{File: v.file, Raw: v.raw, Explicit: v.explicit, Value: v.value})
}
func (v *costSource) UnmarshalJSON(data []byte) error {
	var r recoveryCostSource
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	v.file = r.File
	v.raw = r.Raw
	v.explicit = r.Explicit
	v.value = r.Value
	return nil
}

type recoveryRoomEditing struct {
	ShortName, PortName string
	PortLabel           nameTarget
	Manifest            string
	Details             nameTarget
	Base                Hull
	Sources             map[string]FileChange
	Targets             map[string]string
	Names               map[string]nameTarget
	Descriptions        map[string]nameTarget
	MapFields           map[string]nameTarget
	Removals            map[string]nameTarget
	Defaults            map[string]nameTarget
	ThemeTargets        map[string]nameTarget
	ThemeList           nameTarget
	ThemeOrder          []string
	ThemeJobs           map[string]string
	ThemeFile           string
	ModuleSlots         map[string]nameTarget
	ModuleThemes        map[string]moduleThemeTarget
	Code                string
}

func (v roomEditing) MarshalJSON() ([]byte, error) {
	return json.Marshal(recoveryRoomEditing{Base: v.base, Details: v.details, PortLabel: v.portLabel, ShortName: v.shortName, PortName: v.portName, Manifest: v.manifest, Sources: v.sources, Targets: v.targets, Names: v.names, Descriptions: v.descriptions, MapFields: v.mapFields, Removals: v.removals, Defaults: v.defaults, ThemeTargets: v.themeTargets, ThemeList: v.themeList, ThemeOrder: v.themeOrder, ThemeJobs: v.themeJobs, ThemeFile: v.themeFile, ModuleSlots: v.moduleSlots, ModuleThemes: v.moduleThemes, Code: v.code})
}
func (v *roomEditing) UnmarshalJSON(data []byte) error {
	var r recoveryRoomEditing
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	v.base = r.Base
	v.details = r.Details
	v.portLabel = r.PortLabel
	v.shortName, v.portName = r.ShortName, r.PortName
	v.manifest = r.Manifest
	v.sources = r.Sources
	v.targets = r.Targets
	v.names = r.Names
	v.descriptions = r.Descriptions
	v.mapFields = r.MapFields
	v.removals = r.Removals
	v.defaults = r.Defaults
	v.themeTargets = r.ThemeTargets
	v.themeList = r.ThemeList
	v.themeOrder = r.ThemeOrder
	v.themeJobs = r.ThemeJobs
	v.themeFile = r.ThemeFile
	v.moduleSlots = r.ModuleSlots
	v.moduleThemes = r.ModuleThemes
	v.code = r.Code
	return nil
}
