package wsship

import (
	"fmt"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
)

func dockingDirectionName(dir int) string {
	switch dir {
	case 1:
		return "North (up)"
	case 2:
		return "South (down)"
	case 4:
		return "East (right)"
	case 8:
		return "West (left)"
	}
	return "Choose the way out"
}

func (ws *WsShip) dockingControls() {
	heading("SET UP DOCKING PORT")
	hint("Use Grab (3) to select the entrance airlock. The port belongs to the hull.")
	lo, hi, selected := tools.SelectionBounds()
	if !selected {
		hint("Select an airlock, or a single entrance tile, to continue.")
		return
	}
	site, err := ws.project.InspectDockingSite(ws.currentTheme(), lo, hi)
	if err != nil {
		hint(err.Error())
		return
	}
	space()
	if site.Airlock {
		imgui.Text("Entrance airlock selected")
	} else {
		hint("Entrance tile selected. You can add an airlock with the normal mapping tools.")
	}
	valid := false
	for _, dir := range site.Directions {
		valid = valid || ws.dockOutward == dir
	}
	if !valid {
		ws.dockOutward = 0
		if len(site.Directions) == 1 {
			ws.dockOutward = site.Directions[0]
		} else {
			for _, dir := range site.Directions {
				if dir == site.CurrentOutward {
					ws.dockOutward = dir
				}
			}
		}
	}
	if comboHelp("Airlock opens to space", dockingDirectionName(ws.dockOutward), "The way out from the airlock on this map. The helper sets the port's facing and ship-relative direction together.") {
		for _, dir := range site.Directions {
			if imgui.SelectableV(dockingDirectionName(dir), ws.dockOutward == dir, 0, imgui.Vec2{}) {
				ws.dockOutward = dir
			}
		}
		imgui.EndCombo()
	}
	hint("Only directions clear of the hull are available.")
	space()
	label := "Place docking port here"
	if site.Existing {
		label = "Move docking port here"
		hint("Moves the existing port and keeps its ship identity and area ownership.")
	}
	imgui.BeginDisabledV(ws.dockOutward == 0)
	if actionButton(label, true) {
		ws.applyDocking()
	}
	imgui.EndDisabled()
	hint("Applies to this ship theme. Ctrl+Z undoes the whole change.")
	if imgui.CollapsingHeader("Automatic port settings") {
		hint("Port facing, relative docking direction and travel orientation are set together. The game calculates width, height and offsets from the hull map.")
		hint(fmt.Sprintf("Entrance tile: %d, %d", site.Position.X, site.Position.Y))
	}
}

func (ws *WsShip) applyDocking() {
	lo, hi, selected := tools.SelectionBounds()
	if !selected || ws.source != 0 {
		ws.message = "Use Grab (3) to select an entrance in the hull first."
		return
	}
	ws.message = ""
	ws.change("Set up docking port", func() error {
		return ws.project.PlaceDockingPort(ws.currentTheme(), lo, hi, ws.dockOutward)
	})
	if ws.message == "" {
		ws.finishTask()
	}
}
