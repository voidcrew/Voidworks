package app

import (
	"os"

	"sdmm/internal/app/window"

	"github.com/SpaiR/imgui-go"
	"github.com/rs/zerolog/log"
	"github.com/skratchdot/open-golang/open"
)

const gameCodeNoticeTitle = "Update your Voidcrew code"

// Keep acknowledgments separate so older editor builds cannot overwrite them
// when saving their own application settings.
type noticesConfig struct {
	RoomCrewUpdateSeen bool
}

func (noticesConfig) Name() string { return "notices" }

func (noticesConfig) TryMigrate(_ map[string]any) (map[string]any, bool) {
	return nil, false
}

func (a *app) loadNotices() {
	a.notices = &noticesConfig{}
	a.ConfigRegister(a.notices)
}

func (a *app) showGameCodeUpdateNotice() {
	cfg := a.notices
	if cfg == nil || cfg.RoomCrewUpdateSeen {
		return
	}
	if !imgui.IsPopupOpen(gameCodeNoticeTitle) {
		// Let loading, recovery and save dialogs finish before showing the notice.
		if imgui.IsPopupOpenV("", imgui.PopupFlagsAnyPopup) {
			return
		}
		imgui.OpenPopup(gameCodeNoticeTitle)
	}
	viewport := imgui.MainViewport()
	width := min(470*window.PointSize(), max(1, viewport.WorkSize().X-32))
	imgui.SetNextWindowPosV(viewport.WorkCenter(), imgui.ConditionAlways, imgui.Vec2{X: .5, Y: .5})
	imgui.SetNextWindowSizeConstraints(imgui.Vec2{X: width}, imgui.Vec2{X: width, Y: max(1, viewport.WorkSize().Y-32)})
	if imgui.BeginPopupModalV(gameCodeNoticeTitle, nil, imgui.WindowFlagsAlwaysAutoResize|imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsNoMove) {
		imgui.TextWrapped("To use separate room crew for each ship variant, update your Voidcrew game code to the latest version.")
		imgui.Spacing()
		imgui.TextWrapped("Update your existing checkout or download the latest code, then reopen your project in Voidworks.")
		imgui.Spacing()
		imgui.TextWrapped("Updating Voidworks does not update your game code. Existing ships do not need manual code changes.")
		imgui.Separator()
		if imgui.Button("Open Voidcrew on GitHub") {
			if err := open.Run("https://github.com/voidcrew/Voidcrew"); err != nil {
				log.Print("unable to open Voidcrew repository:", err)
			}
		}
		if imgui.ContentRegionAvail().X > imgui.CalcTextSize("Got it", false, 0).X+40*window.PointSize()+imgui.ItemRectMax().X-imgui.ItemRectMin().X {
			imgui.SameLine()
		}
		if imgui.Button("Got it") {
			a.acknowledgeGameCodeUpdate(cfg)
			imgui.CloseCurrentPopup()
		}
		imgui.EndPopup()
	}
}

func (a *app) acknowledgeGameCodeUpdate(cfg *noticesConfig) {
	cfg.RoomCrewUpdateSeen = true
	if err := os.MkdirAll(a.configDir, 0700); err != nil {
		log.Print("unable to create notice settings directory:", err)
		return
	}
	a.configSaveV(cfg)
}
