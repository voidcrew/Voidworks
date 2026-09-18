package wsplanet

import (
	"fmt"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/planet"
)

func (w *Workspace) beginRiverPicker() {
	w.picking, w.pickingRiver = true, true
	w.pickingEnvironment = false
	w.pickerScope, w.replace, w.filter = 0, 0, ""
}

func (w *Workspace) riverSettings() {
	workshop.Title("Rivers")
	r := w.project.State.Definition.Rivers
	if r == nil {
		workshop.Muted("This project uses fixed river defaults. Editable rivers require the planet-definition update.")
		if path := planet.RiverTurf(w.catalog, w.project.State); path != "" {
			workshop.Muted(w.itemName(path))
		}
		return
	}
	if imgui.Checkbox("Generate rivers", &r.Enabled) && r.Enabled && r.Turf == "" {
		r.Enabled = false
		w.beginRiverPicker()
	}
	if r.Turf != "" {
		pos := imgui.CursorScreenPos()
		if workshop.Row("river-material", w.itemName(r.Turf), "Click to change river material", "", false, style.Teal, 38) {
			w.beginRiverPicker()
		}
		w.spriteRowMenu(r.Turf)
		w.sprite(r.Turf, imgui.Vec2{X: pos.X + 9*window.PointSize(), Y: pos.Y + 12*window.PointSize()}, 32*window.PointSize())
	} else if workshop.Button("Choose river material", true) {
		w.beginRiverPicker()
	}
	if !r.Enabled {
		workshop.Muted("This planet will generate without rivers. Its river choices are kept here.")
		return
	}
	workshop.Muted("Rivers appear across the surface, including mountain caves. Switch to Planet to see the whole network.")
	if w.cave {
		if workshop.Button("Show surface rivers", false) {
			w.cave = false
		}
	}
	imgui.Text("River network")
	nodes := int32(r.Nodes)
	imgui.SetNextItemWidth(-1)
	if imgui.SliderIntV("##river-nodes", &nodes, 2, 12, "%d connection points", 0) {
		r.Nodes = int(nodes)
	}
	workshop.Tooltip("More connection points create a denser network with more branches.")
	width := "Custom"
	widths := []struct {
		name         string
		spread, loss float64
	}{{"Narrow", 0, 25}, {"Standard", 25, 11}, {"Wide", 40, 10}}
	for _, choice := range widths {
		if r.Spread == choice.spread && r.Loss == choice.loss {
			width = choice.name
		}
	}
	if combo("River width", width) {
		for _, choice := range widths {
			if imgui.Selectable(choice.name) {
				r.Spread, r.Loss = choice.spread, choice.loss
			}
		}
		imgui.EndCombo()
	}
	winding := "Custom"
	windings := []struct {
		name  string
		value float64
	}{{"Straight", 0}, {"Gentle", 10}, {"Meandering", 20}, {"Winding", 40}}
	for _, choice := range windings {
		if r.Detour == choice.value {
			winding = choice.name
		}
	}
	if combo("River course", winding) {
		for _, choice := range windings {
			if imgui.Selectable(choice.name) {
				r.Detour = choice.value
			}
		}
		imgui.EndCombo()
	}
	workshop.Gap()
	restricted := r.Biomes != nil
	if imgui.Checkbox("Only in selected biomes", &restricted) {
		if restricted {
			r.Biomes = append([]string{}, w.project.State.UsedBiomes()...)
		} else {
			r.Biomes = nil
		}
	}
	if restricted {
		workshop.Muted("Only these biomes can contain river tiles.")
		imgui.BeginChildV("river-biomes", imgui.Vec2{Y: 175 * window.PointSize()}, false, 0)
		for _, path := range w.project.State.VisibleBiomes() {
			selected := false
			for _, allowed := range r.Biomes {
				selected = selected || path == allowed
			}
			if imgui.Checkbox(w.project.State.Biomes[path].Name+"##"+path, &selected) {
				if selected {
					r.Biomes = append(r.Biomes, path)
				} else {
					for i, allowed := range r.Biomes {
						if allowed == path {
							r.Biomes = append(r.Biomes[:i], r.Biomes[i+1:]...)
							break
						}
					}
				}
			}
		}
		imgui.EndChild()
		if len(r.Biomes) == 0 {
			workshop.Muted("No biomes selected: no rivers will appear.")
		}
	}
	if imgui.CollapsingHeader("Fine adjustment") {
		slider("Bank spread chance", &r.Spread, 0, 40, "%.0f%%")
		slider("Spread falloff", &r.Loss, 10, 100, "%.0f")
		slider("Detour chance", &r.Detour, 0, 60, "%.0f%%")
	}
	workshop.Muted(fmt.Sprintf("%d connection points. Ruin interiors and protected tiles are respected in game.", r.Nodes))
}
