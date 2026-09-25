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

func TestMissingIncludesFromParserError(t *testing.T) {
	message := `parser error: compilation errors
  C:\Voidcrew\tgstation.dme - [7147:1] | failed to find #include "voidcrew/modules/nanites/code/machines/nanite_program_hub.dm"
  C:\Voidcrew\tgstation.dme - [7148:1] | failed to find #include "voidcrew/modules/nanites/code/machines/nanite_programmer.dm"
  C:\Voidcrew\tgstation.dme - [7148:1] | failed to find #include "voidcrew/modules/nanites/code/machines/nanite_programmer.dm"`
	got := missingIncludes(message)
	if len(got) != 2 || got[0] != "voidcrew/modules/nanites/code/machines/nanite_program_hub.dm" {
		t.Fatalf("missing includes: %q", got)
	}
	if missingIncludes("parser error: unexpected token") != nil {
		t.Fatal("other parser errors were treated as missing includes")
	}
}
