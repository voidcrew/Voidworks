package wsship

import (
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/ship"
	"strings"
	"testing"
)

func TestSuggestedIdentifiers(t *testing.T) {
	used := func(id string) bool { return id == "cargo_bay" || id == "cargo_bay_2" }
	for _, name := range []string{"Cargo Bay", "3rd Salvager", "", "船", strings.Repeat("Long name ", 30), "Medical / Engineering"} {
		id := suggestedID(name, used)
		if err := ship.ValidID(id); err != nil {
			t.Fatalf("%q -> %q: %v", name, id, err)
		}
		if used(id) {
			t.Fatalf("generated an existing identifier %q", id)
		}
	}
	if id := suggestedID("Cargo Bay", used); id != "cargo_bay_3" {
		t.Fatalf("collision suffix: %q", id)
	}
}

func TestGuidedActionRejectsMissingSelection(t *testing.T) {
	tools.ClearRoomShape()
	ws := &WsShip{task: taskRoom}
	ws.applyRegion()
	if ws.message != "Select the module's tiles on the hull first." {
		t.Fatal("missing selection did not produce guidance")
	}
}
