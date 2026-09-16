package wsship

import (
	"fmt"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/window"
	"sdmm/internal/ship"
)

func (ws *WsShip) roomCrewCopyControls() {
	c := &ws.crew
	if !ws.project.SupportsRoomCrewVariants() {
		hint("Update the game project to edit room crew separately for each variant.")
		return
	}
	if len(c.copySources) == 0 {
		return
	}
	if actionButton("Copy job from variant...", false) && ws.commitCrew() {
		c.copyOpen = true
	}
	if c.copyOpen {
		imgui.OpenPopup("Copy room job")
	}
	viewport := imgui.MainViewport()
	width := min(460*window.PointSize(), max(1, viewport.WorkSize().X-32))
	imgui.SetNextWindowPosV(viewport.WorkCenter(), imgui.ConditionAlways, imgui.Vec2{X: .5, Y: .5})
	imgui.SetNextWindowSize(imgui.Vec2{X: width})
	imgui.SetNextWindowSizeConstraints(imgui.Vec2{X: width}, imgui.Vec2{X: width, Y: max(1, viewport.WorkSize().Y-32)})
	if imgui.BeginPopupModalV("Copy room job", &c.copyOpen, imgui.WindowFlagsNoMove|imgui.WindowFlagsAlwaysAutoResize) {
		source := c.copySources[c.copyVariant]
		if combo("From variant", source.Theme.Name) {
			for i, candidate := range c.copySources {
				if imgui.SelectableV(candidate.Theme.Name, i == c.copyVariant, 0, imgui.Vec2{}) {
					c.copyVariant, c.copyJob = i, 0
				}
			}
			imgui.EndCombo()
		}
		source = c.copySources[c.copyVariant]
		label := func(job ship.CrewJob) string {
			unit := "slots"
			if job.Slots == 1 {
				unit = "slot"
			}
			return fmt.Sprintf("%s (%d %s)", job.Name, job.Slots, unit)
		}
		if combo("Job", label(source.Jobs[c.copyJob])) {
			for i, job := range source.Jobs {
				if imgui.SelectableV(label(job), i == c.copyJob, 0, imgui.Vec2{}) {
					c.copyJob = i
				}
			}
			imgui.EndCombo()
		}
		hint("Adds a copy with its slots and equipment. You can edit it independently.")
		if c.error != "" {
			hint(c.error)
		}
		if imgui.Button("Copy job") && ws.copyRoomCrewJob(source.Theme.ID, c.copyJob) {
			imgui.CloseCurrentPopup()
		}
		imgui.SameLine()
		if imgui.Button("Cancel") {
			c.copyOpen = false
			imgui.CloseCurrentPopup()
		}
		imgui.EndPopup()
	}
}

func (ws *WsShip) copyRoomCrewJob(source string, index int) bool {
	if !ws.commitCrew() {
		return false
	}
	module, theme := ship.RoomCrewIDs(ws.crew.scope)
	selected := -1
	ws.message = ""
	ws.change("Copy room job", func() (err error) {
		selected, err = ws.project.CopyRoomCrewJob(module, source, theme, index)
		return err
	})
	if ws.message != "" {
		ws.crew.error = ws.message
		return false
	}
	ws.loadCrewScope(ws.crew.scope)
	ws.crew.selected = selected
	return true
}
