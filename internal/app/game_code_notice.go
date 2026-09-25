package app

import (
	"fmt"

	"sdmm/internal/app/window"
	"sdmm/internal/env"
	"sdmm/internal/gamecompat"

	"github.com/SpaiR/imgui-go"
	"github.com/rs/zerolog/log"
	"github.com/skratchdot/open-golang/open"
)

const gameCodeNoticeTitle = "Voidcrew code version"

// checkGameCode runs after a project loads. A mismatch is shown once per load.
func (a *app) checkGameCode() {
	a.gameCode = gamecompat.Check(a.loadedEnvironment)
	a.gameCodePending = a.gameCode.Status != gamecompat.Compatible
	if a.gameCodePending {
		log.Printf("game code mismatch: status %d, game api %d, editor api %d, missing %v",
			a.gameCode.Status, a.gameCode.GameAPI, gamecompat.API, a.gameCode.Missing)
	}
}

func (a *app) showGameCodeNotice() {
	if !a.gameCodePending {
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
		result := a.gameCode
		if result.Status == gamecompat.EditorOutdated {
			imgui.TextWrapped(fmt.Sprintf("This project's Voidcrew code is newer than Voidworks %s supports "+
				"(code version %d, this Voidworks understands version %d).", env.Version, result.GameAPI, gamecompat.API))
			imgui.Spacing()
			imgui.TextWrapped("Update Voidworks before saving ships. An older Voidworks can write ship files the newer game code does not expect.")
			imgui.Separator()
			if imgui.Button("Check for updates") {
				a.dismissGameCodeNotice()
				a.DoCheckForUpdates()
			}
		} else {
			imgui.TextWrapped(fmt.Sprintf("This project's Voidcrew code is older than Voidworks %s expects.", env.Version))
			if len(result.Missing) > 0 {
				imgui.Spacing()
				imgui.TextWrapped("Missing from this code:")
				for _, feature := range result.Missing {
					imgui.BulletText(feature)
				}
			}
			imgui.Spacing()
			imgui.TextWrapped("Pull the latest master into your game checkout, or download the latest code, then reopen the project. " +
				"Until then, Voidworks turns off features the code cannot use.")
			imgui.Spacing()
			imgui.TextWrapped("Updating Voidworks does not update your game code.")
			imgui.Separator()
			if imgui.Button("Open Voidcrew on GitHub") {
				if err := open.Run("https://github.com/voidcrew/Voidcrew"); err != nil {
					log.Print("unable to open Voidcrew repository:", err)
				}
			}
		}
		if imgui.ContentRegionAvail().X > imgui.CalcTextSize("Got it", false, 0).X+40*window.PointSize()+imgui.ItemRectMax().X-imgui.ItemRectMin().X {
			imgui.SameLine()
		}
		if imgui.Button("Got it") {
			a.dismissGameCodeNotice()
		}
		imgui.EndPopup()
	}
}

func (a *app) dismissGameCodeNotice() {
	a.gameCodePending = false
	imgui.CloseCurrentPopup()
}
