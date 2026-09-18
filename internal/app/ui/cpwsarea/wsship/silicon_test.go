package wsship

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/ship"
)

func exerciseSiliconCrew(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	if !ws.project.SupportsSiliconCrew() {
		return
	}
	ws.beginCrew()
	ws.loadCrewScope("ship")
	original := ship.CloneCrewJobs(ws.crew.jobs)
	ws.crew.selected = 1
	ws.setCrewRole("cyborg")
	if ws.crew.error != "" {
		t.Fatal(ws.crew.error)
	}
	job := ws.crew.jobs[1]
	if job.Role != "cyborg" || job.BorgModel != "/obj/item/robot_model/engineering" || job.Outfit != "" || len(job.Equipment) != 0 {
		t.Fatalf("role switch did not clear human loadout: %+v", job)
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "crew-cyborg.png"), 1400, 960)
	}
	ws.setCrewRole("ai")
	if !strings.Contains(ws.crew.error, "AI core") && !strings.Contains(ws.crew.error, "AI crew") {
		t.Fatalf("missing core was not shown in UI: %s", ws.crew.error)
	}
	if !ws.crew.dirty {
		t.Fatal("rejected AI edit was committed")
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "crew-ai-missing-core.png"), 1400, 960)
	}
	stored, err := ws.project.CrewJobs("ship")
	if err != nil || stored[1].Role != "cyborg" {
		t.Fatal("invalid UI edit replaced valid roster", err)
	}
	ws.loadCrewScope("ship")
	ws.crew.jobs, ws.crew.dirty = original, true
	if !ws.commitCrew() {
		t.Fatal(ws.crew.error)
	}
	ws.finishTask()
}
