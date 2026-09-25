package gamecompat

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sdmm/internal/dmapi/dmenv"
)

const currentGame = `/datum
	var/name
/datum/map_template/shuttle/voidcrew
	var/suffix
/obj/modular_map_root/ship_upgrade
	var/footprint
/datum/ship_upgrade_module
	var/list/job_slots_add_by_theme = list()
/datum/job
	var/ship_role = "crew"
`

func project(t *testing.T, code, declaration string, previewFolders bool) *dmenv.Dme {
	t.Helper()
	root := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("test.dme", code)
	if declaration != "" {
		write(File, declaration)
	}
	write("voidcrew/modules/ship_upgrades/previews/manifest.json", "{}")
	if previewFolders {
		write("voidcrew/modules/ship_upgrades/previews/hulls/test.preview.json", "{}")
	}
	dme, err := dmenv.New(filepath.Join(root, "test.dme"))
	if err != nil {
		t.Fatal(err)
	}
	return dme
}

func TestCurrentCodeWithoutDeclarationIsCompatible(t *testing.T) {
	if result := Check(project(t, currentGame, "", true)); result.Status != Compatible || len(result.Missing) != 0 {
		t.Fatalf("current code was flagged: %+v", result)
	}
	if result := Check(project(t, currentGame, `{"voidworks_api": 1}`, true)); result.Status != Compatible || result.GameAPI != 1 {
		t.Fatalf("matching declaration was flagged: %+v", result)
	}
}

func TestOlderCodeIsNamedFeatureByFeature(t *testing.T) {
	old := `/datum
	var/name
/datum/map_template/shuttle/voidcrew
	var/suffix
/obj/modular_map_root/ship_upgrade
/datum/ship_upgrade_module
/datum/job
`
	result := Check(project(t, old, "", false))
	want := []string{"room shapes other than rectangles", "separate room crew for each ship variant",
		"cyborg and AI ship crew", "purchase preview metadata folders"}
	if result.Status != GameOutdated || !slices.Equal(result.Missing, want) {
		t.Fatalf("old code: %+v", result)
	}
}

func TestDeclaredVersions(t *testing.T) {
	if result := Check(project(t, currentGame, `{"voidworks_api": 2}`, true)); result.Status != EditorOutdated || result.GameAPI != 2 {
		t.Fatalf("newer game code did not ask for a newer editor: %+v", result)
	}
	for _, broken := range []string{"not json", `{"voidworks_api": 0}`, `{}`} {
		if result := Check(project(t, currentGame, broken, true)); result.Status != GameOutdated || len(result.Missing) != 1 {
			t.Fatalf("invalid declaration %q: %+v", broken, result)
		}
	}
}

func TestOtherCodebasesAreNotChecked(t *testing.T) {
	if result := Check(project(t, "/datum\n\tvar/name\n", "", false)); result.Status != Compatible || result.Missing != nil {
		t.Fatalf("non-Voidcrew project was checked: %+v", result)
	}
	if Check(nil).Status != Compatible {
		t.Fatal("no project was flagged")
	}
}

// Opt-in: the real game checkout must pass, or every mapper sees the warning.
func TestRealGameCheckout(t *testing.T) {
	file := os.Getenv("GAME_COMPAT_TEST_DME")
	if file == "" {
		t.Skip("set GAME_COMPAT_TEST_DME")
	}
	dme, err := dmenv.New(file)
	if err != nil {
		t.Fatal(err)
	}
	if result := Check(dme); result.Status != Compatible {
		t.Fatalf("current game code was flagged: %+v", result)
	}
}
