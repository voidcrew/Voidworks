package ship

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sdmm/internal/dmapi/dmenv"
)

func (c *Catalog) RemovalSnapshot() *Catalog {
	copy := *c
	copy.Hulls = make([]Hull, len(c.Hulls))
	for i, h := range c.Hulls {
		copy.Hulls[i] = cloneHull(h)
	}
	return &copy
}

func (h Hull) RemovalSnapshot() Hull { return cloneHull(h) }

// Removal uses the catalog rather than opening/assembling a ship, so a ship
// with a missing map or invalid crew can still be removed from the library.
func (c *Catalog) Removal(dme *dmenv.Dme, h Hull, draft bool) (*RemovalPlan, error) {
	if !strings.HasPrefix(h.Type, HullType+"/") {
		return nil, fmt.Errorf("choose a ship to remove")
	}
	spec := RemovalSpec{Name: h.Name, Types: []string{h.Type}, Draft: draft, ModuleDir: c.ModuleDir}
	for path, obj := range dme.Objects {
		if (strings.HasPrefix(path, "/datum/ship_theme/") || strings.HasPrefix(path, "/datum/ship_upgrade_module/")) && withinTypes(obj.Vars.ValueV("for_ship", ""), []string{h.Type}) {
			spec.Types = append(spec.Types, path)
		}
	}
	// Include the current in-memory draft as well as saved siblings and hidden
	// ships: hidden registrations can also reference a shared module map.
	hulls := map[string]Hull{h.Type: h}
	for _, other := range c.Hulls {
		if other.Type != h.Type {
			hulls[other.Type] = other
		}
	}
	for _, other := range hulls {
		files, err := c.removalMapPaths(other)
		if err != nil {
			return nil, err
		}
		if !withinTypes(other.Type, []string{h.Type}) {
			spec.SharedMaps = append(spec.SharedMaps, files...)
			continue
		}
		spec.Maps = append(spec.Maps, files...)
		id := strings.ReplaceAll(strings.TrimPrefix(other.Type, HullType+"/"), "/", "__")
		if err := ValidID(id); err != nil {
			return nil, err
		}
		// Unsaved edits may have removed a theme or module from the live hull.
		// Include the on-disk layout so its previously saved maps are removed too.
		fileID := id
		if _, data, err := readShipProject(c, other); err == nil {
			var settings Settings
			if err := json.Unmarshal(data, &settings); err != nil {
				return nil, err
			}
			if settings.Hull.Type != other.Type {
				return nil, fmt.Errorf("ship metadata does not match %s", other.Type)
			}
			if settings.FileID != "" {
				if err := ValidID(settings.FileID); err != nil {
					return nil, err
				}
				fileID = settings.FileID
			}
			files, err := c.removalMapPaths(settings.Hull)
			if err != nil {
				return nil, err
			}
			spec.Maps = append(spec.Maps, files...)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		for _, suffix := range []string{".ship.json", ".shipinfo.json", ".areas.json", ".crew.json"} {
			spec.Metadata = append(spec.Metadata, filepath.Join(c.Root, "voidcrew/mapping/ship_projects", fileID+suffix))
		}
		shipPath := strings.TrimPrefix(other.Type, HullType+"/")
		spec.Helpers = append(spec.Helpers, "/area/shuttle/voidcrew/"+shipPath, "/obj/docking_port/mobile/voidcrew/"+shipPath)
		if data, err := os.ReadFile(filepath.Join(c.Root, "voidcrew/mapping/ship_projects", fileID+".crew.json")); err == nil {
			var crew CrewConfig
			if err := json.Unmarshal(data, &crew); err != nil {
				return nil, err
			}
			for _, jobs := range crew.Rosters {
				for _, job := range jobs {
					if job.ID != "" && job.BaseOutfit != "" {
						spec.Helpers = append(spec.Helpers, "/datum/outfit/job/workshop_"+id+"_"+job.ID)
					}
				}
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	// Hidden or inherited module registrations may not be listed by Discover.
	for path, obj := range dme.Objects {
		if !strings.HasPrefix(path, "/datum/ship_upgrade_module/") {
			continue
		}
		file := text(obj.Vars, "map_file")
		if file == "" {
			continue
		}
		themes, err := stringList(obj.Vars.ValueV("for_theme", "null"))
		if err != nil {
			return nil, err
		}
		files := []string{filepath.Join(c.Root, c.ModuleDir, file)}
		for _, theme := range themes {
			files = append(files, filepath.Join(c.Root, c.ModuleDir, strings.TrimSuffix(file, ".dmm")+"_"+theme+".dmm"))
		}
		if withinTypes(path, spec.Types) {
			spec.Maps = append(spec.Maps, files...)
		} else {
			spec.SharedMaps = append(spec.SharedMaps, files...)
		}
	}
	spec.Types = uniqueRemovalPaths(spec.Types)
	spec.Helpers = uniqueRemovalPaths(spec.Helpers)
	spec.Maps = uniqueRemovalPaths(spec.Maps)
	return PlanRemoval(dme, spec)
}

func uniqueRemovalPaths(paths []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, path := range paths {
		if !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

func (c *Catalog) removalMapPaths(h Hull) ([]string, error) {
	var files []string
	if h.Suffix != "" {
		file, err := c.HullFile(h, Theme{})
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	for _, theme := range h.Themes {
		file, err := c.HullFile(h, theme)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	for _, module := range h.Modules {
		base, err := Inside(c.Root, filepath.Join(c.ModuleDir, module.File))
		if err != nil {
			return nil, err
		}
		files = append(files, base)
		for _, theme := range module.Themes {
			files = append(files, strings.TrimSuffix(base, ".dmm")+"_"+theme+".dmm")
		}
	}
	return files, nil
}
