package dmenv

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRefreshTypeTreesPreservesMapTypesAndRelinksDefinitions(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.dme")
	parse := func(source string) *Dme {
		t.Helper()
		if err := os.WriteFile(file, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		d, err := New(file)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	d := parse("/datum/ship\n\tvar/slots = 0\n/datum/ship/old\n/obj/kept\n\tname = \"Original\"\n")
	kept := d.Objects["/obj/kept"]
	if err := d.AddDraftType("/obj/draft", map[string]string{"name": `"Unsaved"`}); err != nil {
		t.Fatal(err)
	}
	draft := d.Objects["/obj/draft"]
	fresh := parse("/datum/ship\n\tvar/slots = 2\n/datum/ship/new\n/datum/ship/alias\n\tparent_type = /datum/ship/new\n/obj/kept\n\tname = \"Changed\"\n")
	for range 2 {
		d.RefreshTypeTrees(fresh, []string{"/datum/ship"})
		if d.Objects["/datum/ship/old"] != nil || d.Objects["/datum/ship/new"].Vars.IntV("slots", 0) != 2 {
			t.Fatal("registration tree was not replaced")
		}
		if d.Objects["/datum/ship/alias"].Parent() != d.Objects["/datum/ship/new"] || d.Objects["/datum/ship"].Parent() != d.Objects["/datum"] {
			t.Fatal("refreshed definitions still refer to the other environment")
		}
		if d.Objects["/obj/kept"] != kept || kept.Vars.ValueV("name", "") != `"Original"` || d.Objects["/obj/draft"] != draft {
			t.Fatal("refresh replaced a live map type or unsaved draft")
		}
		if got := d.Objects["/datum"].DirectChildren; !reflect.DeepEqual(got, []string{"/datum/ship"}) {
			t.Fatal("stale or duplicate type-tree entries", got)
		}
		if d.Objects["/datum/ship/new"].env != d || d.Objects["/datum/ship/new"] == fresh.Objects["/datum/ship/new"] {
			t.Fatal("refreshed objects were not copied into the loaded environment")
		}
	}
}
