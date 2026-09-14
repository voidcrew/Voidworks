package cpvareditor

import (
	"reflect"
	"testing"

	"sdmm/internal/app/config"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

type missingTypeApp struct {
	App
	dme *dmenv.Dme
	cfg vareditorConfig
}

func (a *missingTypeApp) LoadedEnvironment() *dmenv.Dme   { return a.dme }
func (a *missingTypeApp) ConfigFind(string) config.Config { return &a.cfg }

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
