package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sdmm/internal/dmapi/dmenv"
)

func TestAvailableMapsIncludesVoidcrewOutposts(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"_maps/voidcrew/ships/ship_delta_a.dmm",
		"voidcrew/_maps/map_files/outposts/trader_outpost_general.dmm",
		"voidcrew/_maps/map_files/outposts/nested/outpost.DMM",
		"voidcrew/_maps/map_files/events/arena.dmm",
		"voidcrew/_maps/map_files/outposts/readme.txt",
		"data/testing-copy/_maps/unrelated.dmm",
	}
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{loadedEnvironment: &dmenv.Dme{RootDir: root, Objects: map[string]*dmenv.Object{
		"/datum/map_template/shuttle/voidcrew": {},
	}}}
	check := func(want []string) {
		t.Helper()
		got := a.AvailableMaps()
		for i := range got {
			rel, err := filepath.Rel(root, got[i])
			if err != nil {
				t.Fatal(err)
			}
			got[i] = filepath.ToSlash(rel)
		}
		slices.Sort(got)
		want = slices.Clone(want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("available maps = %v, want %v", got, want)
		}
	}
	check(files[:4])
	// Projects with only the Voidcrew map tree must still be searchable.
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(files[0]))); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"_maps/voidcrew/ships", "_maps/voidcrew", "_maps"} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(dir))); err != nil {
			t.Fatal(err)
		}
	}
	check(files[1:4])
	// Other BYOND projects retain their existing project-wide search.
	a.loadedEnvironment.Objects = nil
	check(append(slices.Clone(files[1:4]), files[5]))
	a.loadedEnvironment = nil
	check(nil)
}
