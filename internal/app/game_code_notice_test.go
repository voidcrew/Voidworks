package app

import (
	"runtime"
	"testing"

	"sdmm/internal/gamecompat"

	"github.com/SpaiR/imgui-go"
)

func TestGameCodeNoticeWaitsForOtherDialogs(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	context := imgui.CreateContext(nil)
	defer context.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 860, Y: 500})
	io.SetDeltaTime(1.0 / 60)
	io.Fonts().TextureDataRGBA32()
	a := &app{configDir: t.TempDir()}
	frame := func(loading, closeLoading bool) bool {
		imgui.NewFrame()
		if loading {
			imgui.OpenPopup("Loading project")
		}
		if imgui.BeginPopupModalV("Loading project", nil, imgui.WindowFlagsAlwaysAutoResize) {
			imgui.Text("Loading project")
			if closeLoading {
				imgui.CloseCurrentPopup()
			}
			imgui.EndPopup()
		}
		a.showGameCodeNotice()
		shown := imgui.IsPopupOpen(gameCodeNoticeTitle)
		imgui.Render()
		return shown
	}
	// Compatible code never shows the notice.
	if frame(false, false) {
		t.Fatal("notice shown for compatible game code")
	}
	for _, status := range []gamecompat.Status{gamecompat.GameOutdated, gamecompat.EditorOutdated} {
		a.gameCode = gamecompat.Result{Status: status, GameAPI: 2, Missing: []string{"a feature"}}
		a.gameCodePending = true
		if frame(true, false) || frame(false, false) {
			t.Fatal("game code notice replaced the loading dialog")
		}
		frame(false, true)
		if !frame(false, false) {
			t.Fatal("notice did not appear after loading")
		}
		imgui.NewFrame()
		if imgui.BeginPopupModalV(gameCodeNoticeTitle, nil, 0) {
			a.dismissGameCodeNotice()
			imgui.EndPopup()
		}
		imgui.Render()
		if a.gameCodePending || frame(false, false) {
			t.Fatal("dismissed notice came back before the next project load")
		}
	}
}
