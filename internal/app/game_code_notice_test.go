package app

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/SpaiR/imgui-go"
)

func TestGameCodeNoticeAcknowledgmentPersists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	a := &app{configDir: dir}
	a.loadNotices()
	cfg := a.ConfigFind("notices").(*noticesConfig)
	if cfg.RoomCrewUpdateSeen {
		t.Fatal("new installs should show the game-code notice")
	}
	// Opening the notice is not an acknowledgment, including after a restart.
	a.configSave()
	restarted := &app{configDir: dir}
	loaded := &noticesConfig{}
	restarted.ConfigRegister(loaded)
	if loaded.RoomCrewUpdateSeen {
		t.Fatal("notice was dismissed without acknowledgment")
	}
	a.acknowledgeGameCodeUpdate(cfg)
	// An older build can keep saving app.json without losing the acknowledgment.
	older := &app{configDir: dir}
	older.loadConfig()
	older.configSave()
	restarted = &app{configDir: dir}
	loaded = &noticesConfig{}
	restarted.ConfigRegister(loaded)
	if !loaded.RoomCrewUpdateSeen {
		t.Fatal("acknowledged notice reappeared after restart")
	}
}

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
	a.loadNotices()
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
		a.showGameCodeUpdateNotice()
		shown := imgui.IsPopupOpen(gameCodeNoticeTitle)
		imgui.Render()
		return shown
	}
	if frame(true, false) || frame(false, false) {
		t.Fatal("startup notice replaced the loading dialog")
	}
	frame(false, true)
	if !frame(false, false) || a.notices.RoomCrewUpdateSeen {
		t.Fatal("notice did not appear after loading or was acknowledged automatically")
	}
}
