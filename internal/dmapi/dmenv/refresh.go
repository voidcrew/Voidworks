package dmenv

import (
	"sort"
	"strings"

	"sdmm/internal/dmapi/dmvars"
)

// RefreshTypeTrees replaces definition trees without replacing the environment
// or unrelated map types. Callers must finish editing the selected trees first.
func (d *Dme) RefreshTypeTrees(source *Dme, roots []string) {
	selected := func(path string) bool {
		for _, root := range roots {
			if path == root || strings.HasPrefix(path, root+"/") {
				return true
			}
		}
		return false
	}
	d.RemoveTypeTrees(roots)
	var paths []string
	for path, original := range source.Objects {
		if !selected(path) {
			continue
		}
		vars := dmvars.MutableVariables{}
		for _, name := range original.Vars.Iterate() {
			vars.Put(name, original.Vars.ValueV(name, ""))
		}
		copy := *original
		copy.env, copy.parent = d, nil
		copy.Vars = vars.ToImmutable()
		copy.DirectChildren = append([]string(nil), original.DirectChildren...)
		d.Objects[path] = &copy
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		obj, original := d.Objects[path], source.Objects[path]
		if original.parent != nil {
			obj.parent = d.Objects[original.parent.Path]
			if obj.parent != nil {
				obj.Vars.LinkParent(obj.parent.Vars)
			}
		}
		// DirectChildren follows the path hierarchy, including explicit
		// parent_type overrides that use a different inheritance parent.
		if i := strings.LastIndex(path, "/"); i > 0 {
			parent := path[:i]
			if !selected(parent) && d.Objects[parent] != nil {
				d.Objects[parent].DirectChildren = append(d.Objects[parent].DirectChildren, path)
			}
		}
	}
}
