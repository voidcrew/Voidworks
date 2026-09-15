package dm

import "testing"

func filterFixture() *PathsFilter {
	children := map[string][]string{
		"/obj":                 {"/obj/structure", "/obj/item"},
		"/obj/structure":       {"/obj/structure/table", "/obj/structure/chair"},
		"/obj/structure/table": {"/obj/structure/table/wood"},
		"/obj/item":            {"/obj/item/wrench"},
	}
	return NewPathsFilter(func(path string) []string { return children[path] })
}

func TestPathsFilterIndividualUnhide(t *testing.T) {
	f := filterFixture()
	f.TogglePath("/obj")
	for _, path := range []string{"/obj", "/obj/structure/table", "/obj/structure/table/wood", "/obj/structure/chair", "/obj/new_draft"} {
		if !f.IsHiddenPath(path) {
			t.Fatalf("bulk hide missed %s", path)
		}
	}
	f.TogglePath("/obj/structure/table")
	for _, path := range []string{"/obj/structure/table", "/obj/structure/table/wood", "/obj/structure/table/new_draft"} {
		if !f.IsVisiblePath(path) {
			t.Fatalf("individual unhide missed %s", path)
		}
	}
	for _, path := range []string{"/obj", "/obj/structure/chair", "/obj/item/wrench", "/obj/structure/new_draft"} {
		if !f.IsHiddenPath(path) {
			t.Fatalf("individual unhide revealed unrelated %s", path)
		}
	}
	if f.HasHiddenChildPath("/obj/structure/table") {
		t.Fatal("visible exception is reported as hidden")
	}
	if !f.HasHiddenChildPath("/obj") {
		t.Fatal("other hidden children were lost")
	}
	f.TogglePath("/obj/structure/table/wood")
	if !f.HasHiddenChildPath("/obj/structure/table") {
		t.Fatal("nested hidden exception was lost")
	}
	f.SetVisible("/obj/structure/table", true)
	if !f.IsVisiblePath("/obj/structure/table/wood") || f.HasHiddenChildPath("/obj/structure/table") {
		t.Fatal("show subtree did not clear nested hiding")
	}
	f.SetVisible("/obj", false)
	if !f.IsHiddenPath("/obj/structure/table/new_draft") {
		t.Fatal("bulk hide kept an old unhide exception")
	}
	f.SetVisible("/obj", true)
	if f.IsHiddenPath("/obj/structure/table") || f.HasHiddenChildPath("/obj") {
		t.Fatal("bulk show did not clear hiding")
	}
}

func TestPathsFilterCopyPreservesExceptions(t *testing.T) {
	f := filterFixture()
	f.SetVisible("/obj", false)
	f.SetVisible("/obj/structure/table", true)
	copy := f.Copy()
	f.Clear()
	if copy.IsHiddenPath("/obj/structure/table/wood") || !copy.IsHiddenPath("/obj/structure/chair") {
		t.Fatal("clipboard filter lost visible exception or hidden sibling")
	}
	copy.SetVisible("/obj/structure/table", false)
	if f.IsHiddenPath("/obj/structure/table") {
		t.Fatal("filter copy changed the original")
	}
	copy.Clear()
	if copy.IsHiddenPath("/obj/new_draft") || copy.HasHiddenChildPath("/obj") {
		t.Fatal("clear kept inherited rules")
	}
}

func TestPathsFilterUsesWholeTypeNames(t *testing.T) {
	f := NewPathsFilterEmpty()
	f.SetVisible("/obj/item/wrench", false)
	if f.HasHiddenChildPath("/obj/item/wren") || f.IsHiddenPath("/obj/item/wrenchlike") {
		t.Fatal("similarly named types share filter state")
	}
	f.SetVisible("/obj/item/wren", true)
	if !f.IsHiddenPath("/obj/item/wrench") {
		t.Fatal("showing a similar name cleared a different type")
	}
}

func TestPathsFilterDraftExceptionsWithoutTreeEntries(t *testing.T) {
	f := NewPathsFilterEmpty()
	f.SetVisible("/area", false)
	f.TogglePath("/area/shuttle/new_ship")
	if !f.IsVisiblePath("/area/shuttle/new_ship/bridge") || !f.IsHiddenPath("/area/shuttle/another_ship") {
		t.Fatal("draft visibility exception was not inherited")
	}
	f.SetVisible("/area", false)
	if !f.IsHiddenPath("/area/shuttle/new_ship/bridge") {
		t.Fatal("bulk hide failed to replace a draft exception")
	}
}
