package wsship

import (
	"runtime"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/ship"
)

func TestDeleteMenusKeepNewConfirmation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 900, Y: 700})
	io.SetDeltaTime(1.0 / 60)
	io.Fonts().TextureDataRGBA32()
	theme := ship.Theme{ID: "engineering", Name: "Engineering"}
	module := ship.Module{ID: "medical", Name: "Medical"}
	ws := &WsShip{project: &ship.Project{Hull: ship.Hull{Themes: []ship.Theme{{ID: "standard", Default: true}, theme}}}, selected: map[string]string{}}
	for _, c := range []struct {
		name, key string
		draw      func()
	}{
		{"room", "slot/cargo", func() { ws.roomMenu("cargo", 2) }},
		{"option", "module/medical", func() { ws.optionMenu("cargo", module, 2) }},
		{"variant", "theme/engineering", func() { ws.variantMenu(theme) }},
		{"shared room", "fork/engineering/medical", func() {
			ws.variantOptionMenu(theme.ID, module, ship.OptionStatus{Available: true, Forked: true, Differs: true}, true)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ws.pendingDelete = ""
			open := true
			var last imgui.Vec2
			frame := func() {
				imgui.NewFrame()
				imgui.SetNextWindowPos(imgui.Vec2{X: 20, Y: 20})
				imgui.SetNextWindowSize(imgui.Vec2{X: 850, Y: 650})
				imgui.BeginV("Confirmation regression", nil, imgui.WindowFlagsNoSavedSettings)
				if open {
					imgui.OpenPopup("actions")
					open = false
				}
				imgui.SetNextWindowPos(imgui.Vec2{X: 50, Y: 50})
				ws.pendingShown = false
				if imgui.BeginPopup("actions") {
					c.draw()
					lo, hi := imgui.ItemRectMin(), imgui.ItemRectMax()
					last = imgui.Vec2{X: (lo.X + hi.X) / 2, Y: (lo.Y + hi.Y) / 2}
					imgui.EndPopup()
				}
				imgui.End()
				imgui.Render()
			}
			click := func(pos imgui.Vec2) {
				io.SetMousePosition(pos)
				frame()
				io.SetMouseButtonDown(0, true)
				frame()
				io.SetMouseButtonDown(0, false)
				frame()
			}
			io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
			for i := 0; i < 3; i++ {
				frame()
			}
			click(last)
			// The host expires unshown confirmations at the end of this frame.
			if ws.pendingDelete != c.key || !ws.pendingShown {
				t.Fatalf("new confirmation expires before it can be shown: %q, shown=%v", ws.pendingDelete, ws.pendingShown)
			}
			for i := 0; i < 3; i++ {
				frame()
			}
			if ws.pendingDelete != c.key || !ws.pendingShown {
				t.Fatal("confirmation disappeared")
			}
			// The last row is now Keep it / Keep the copy, never the destructive action.
			click(last)
			if ws.pendingDelete != "" {
				t.Fatal("cancel did not clear confirmation")
			}
			frame()
		})
	}
}
