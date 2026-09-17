package cpvareditor

import (
	"reflect"
	"testing"

	"sdmm/internal/app/config"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/editor"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

type missingTypeApp struct {
	App
	dme    *dmenv.Dme
	cfg    vareditorConfig
	editor *editor.Editor
}

func (a *missingTypeApp) LoadedEnvironment() *dmenv.Dme   { return a.dme }
func (a *missingTypeApp) ConfigFind(string) config.Config { return &a.cfg }
func (a *missingTypeApp) CurrentEditor() *editor.Editor   { return a.editor }

func TestSyncFollowsResizedInstance(t *testing.T) {
	a := &missingTypeApp{dme: &dmenv.Dme{Objects: map[string]*dmenv.Object{}}}
	prefab := dmmprefab.New(42, "/obj/helper", &dmvars.Variables{})
	old := dmminstance.New(util.Point{X: 1, Y: 1, Z: 1}, prefab)
	live := old.CopyAt(util.Point{X: 2, Y: 1, Z: 1})
	tile := &dmmap.Tile{Coord: live.Coord()}
	tile.Set(dmmap.Instances{live})
	m := &dmmap.Dmm{MaxX: 2, MaxY: 1, MaxZ: 1, Tiles: []*dmmap.Tile{{Coord: old.Coord()}, tile}}
	a.editor = editor.New(nil, nil, m)
	v := &VarEditor{app: a}
	v.EditInstance(old)
	v.Sync()
	if instance, ok := v.EditedInstance(); !ok || instance != live {
		t.Fatal("variable editor retained an instance at the old position")
	}
	tile.InstancesRemoveByInstance(live)
	v.Sync()
	if _, ok := v.EditedInstance(); ok {
		t.Fatal("deleted/cropped instance remains editable")
	}
}

func TestEditMissingTypePreservesMappedVariables(t *testing.T) {
	app := &missingTypeApp{dme: &dmenv.Dme{Objects: map[string]*dmenv.Object{}}}
	vars := &dmvars.MutableVariables{}
	vars.Put("name", `"Surgical supplies"`)
	vars.Put("pixel_x", "8")
	prefab := dmmprefab.New(42, "/obj/item/storage/backpack/duffelbag/med/surgery", vars.ToImmutable())
	instance := dmminstance.New(util.Point{X: 1, Y: 1, Z: 1}, prefab)
	v := &VarEditor{app: app}
	for _, edit := range []func(){func() { v.EditInstance(instance) }, func() { v.EditPrefab(prefab) }} {
		edit()
		if !reflect.DeepEqual(v.variablesNames, []string{"name", "pixel_x"}) || !reflect.DeepEqual(v.variablesNamesByPaths[prefab.Path()], v.variablesNames) {
			t.Fatalf("missing mapped variables: %v, %v", v.variablesNames, v.variablesNamesByPaths)
		}
		for _, name := range v.variablesNames {
			if v.initialVarValue(name) != dmvars.NullValue || v.isReadOnly(name) || v.isFilteredVariable(name) {
				t.Fatalf("unknown variable cannot be inspected: %s", name)
			}
		}
		if v.currentVars().ValueV("pixel_x", "") != "8" || len(app.dme.Objects) != 0 {
			t.Fatal("selecting an unknown type changed its values or the environment")
		}
	}
	// Missing types with no overrides must also be safe to select.
	v.EditPrefab(dmmprefab.New(43, "/obj/removed", &dmvars.Variables{}))
	if len(v.variablesNames) != 0 || len(v.variablesPaths) != 1 {
		t.Fatal("empty missing type was not retained")
	}
}
