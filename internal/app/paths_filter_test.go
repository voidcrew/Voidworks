package app

import (
	"sdmm/internal/dmapi/dmenv"
	"testing"
)

func TestPathsFilterAcceptsTypesMissingFromEnvironment(t *testing.T) {
	f := newPathsFilter(&dmenv.Dme{Objects: map[string]*dmenv.Object{}})
	f.TogglePath("/obj/missing")
	if !f.IsHiddenPath("/obj/missing") {
		t.Fatal("unknown type was not hidden")
	}
	f.TogglePath("/obj/missing")
	if !f.IsVisiblePath("/obj/missing") {
		t.Fatal("unknown type was not unhidden")
	}
}
