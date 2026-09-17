package wsship

import (
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/shippreview"
)

func exercisePreviewButtons(t *testing.T) {
	t.Helper()
	io := imgui.CurrentIO()
	defer io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
	status := shippreview.Status{Phase: "running", Message: "Preview progress: 1 rendered, 3 reused."}
	stops, resumes := 0, 0
	var button imgui.Vec2
	frame := func() {
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{X: 20, Y: 20})
		imgui.SetNextWindowSize(imgui.Vec2{X: 360, Y: 280})
		imgui.BeginV("Preview controls regression", nil, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsNoResize|imgui.WindowFlagsNoMove)
		workshop.PreviewStatus(status, func() { resumes++ }, func() {
			stops++
			status.Phase, status.Message = "stopped", "Preview generation is paused."
		})
		lo, hi := imgui.ItemRectMin(), imgui.ItemRectMax()
		button = imgui.Vec2{X: (lo.X + hi.X) / 2, Y: (lo.Y + hi.Y) / 2}
		imgui.End()
		imgui.Render()
	}
	for i := 0; i < 2; i++ {
		for j := 0; j < 3; j++ {
			frame()
		}
		io.SetMousePosition(button)
		frame()
		io.SetMouseButtonDown(0, true)
		frame()
		io.SetMouseButtonDown(0, false)
		frame()
	}
	if stops != 1 || resumes != 1 {
		t.Fatalf("preview controls did not dispatch Stop and Resume: stops=%d resumes=%d", stops, resumes)
	}
}
