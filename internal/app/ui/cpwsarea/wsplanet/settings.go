package wsplanet

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/planet"
)

func (w *Workspace) environmentSettings() {
	workshop.Title("Environment")
	e := w.project.State.Definition.Environment
	if e == nil {
		workshop.Muted("Editable environment settings require the planet-definition update.")
		return
	}
	workshop.Muted("The air, weather and daylight shared by this planet's biomes.")
	if !w.lighting || w.cave {
		if workshop.Button("Preview daylight", false) {
			w.lighting = true
			w.cave = false
		}
	}
	airChoices := []struct {
		name string
		air  *string
	}{{"Use each biome's air", nil}}
	for _, choice := range []struct{ name, air string }{
		{"Breathable", "o2=22;n2=82;TEMP=293.15"},
		{"Cold breathable", "o2=22;n2=82;TEMP=180"},
		{"Airless", "TEMP=2.7"},
		{"Hot volcanic", "o2=14;n2=23;TEMP=300"},
	} {
		air := choice.air
		airChoices = append(airChoices, struct {
			name string
			air  *string
		}{choice.name, &air})
	}
	current := "Custom atmosphere"
	for _, choice := range airChoices {
		if choice.air == nil && e.Atmosphere == nil || choice.air != nil && e.Atmosphere != nil && *choice.air == *e.Atmosphere {
			current = choice.name
		}
	}
	if combo("Air", current) {
		for _, choice := range airChoices {
			if imgui.Selectable(choice.name) {
				e.Atmosphere = choice.air
			}
		}
		imgui.EndCombo()
	}
	if e.Atmosphere != nil && imgui.CollapsingHeader("Atmosphere details") {
		workshop.Muted(*e.Atmosphere)
	}
	weather := "Calm"
	if e.Weather != "" {
		weather = w.catalog.Dme.Objects[e.Weather].Vars.TextV("name", planet.Label(strings.TrimPrefix(e.Weather, "/datum/weather/")))
	}
	if combo("Weather", weather) {
		if imgui.Selectable("Calm") {
			e.Weather, e.WeatherTrait = "", ""
		}
		seen := map[string]bool{}
		for _, d := range w.catalog.Planets {
			if env := d.Environment; env != nil && env.Weather != "" && !seen[env.Weather] {
				seen[env.Weather] = true
				name := w.catalog.Dme.Objects[env.Weather].Vars.TextV("name", planet.Label(strings.TrimPrefix(env.Weather, "/datum/weather/")))
				if imgui.Selectable(name) {
					e.Weather, e.WeatherTrait = env.Weather, env.WeatherTrait
				}
			}
		}
		imgui.EndCombo()
	}
	workshop.Muted("Weather runs over time in game.")
	workshop.Gap()
	imgui.Text("Daylight")
	colors := []struct{ name, value string }{{"White", "#FFFFFF"}, {"Warm", "#FAE48E"}, {"Volcanic", "#F98511"}, {"Cool", "#A4D8FF"}, {"Green", "#A4FFB0"}}
	label := e.LightColor
	for _, c := range colors {
		if strings.EqualFold(c.value, e.LightColor) {
			label = c.name
		}
	}
	if combo("Light color", label) {
		for _, c := range colors {
			if imgui.Selectable(c.name) {
				e.LightColor = c.value
			}
		}
		imgui.EndCombo()
	}
	slider("Brightness", &e.LightAlpha, 0, 255, "%.0f / 255")
	slider("Gravity", &e.Gravity, 0, 3, "%.2f g")
	workshop.Gap()
	workshop.Section("GROUND BENEATH THE SURFACE", style.Teal)
	pos := imgui.CursorScreenPos()
	if workshop.Row("planet-baseturf", w.itemName(e.Baseturf), "Exposed when floors are removed", "", false, style.Teal, 38) {
		w.picking, w.pickingEnvironment, w.pickingRiver = true, true, false
		w.pickerScope, w.replace, w.filter = 0, 0, ""
	}
	w.spriteRowMenu(e.Baseturf)
	w.sprite(e.Baseturf, imgui.Vec2{X: pos.X + 9*window.PointSize(), Y: pos.Y + 12*window.PointSize()}, 32*window.PointSize())
}

func (w *Workspace) ruinSettings() {
	workshop.Title("Ruins")
	r := w.project.State.Definition.Ruins
	if r == nil {
		workshop.Muted("Editable ruin settings require the planet-definition update.")
		return
	}
	imgui.Checkbox("Generate ruins", &r.Enabled)
	if !r.Enabled {
		workshop.Muted("This planet will generate without ruins. Its ruin choices are kept here.")
		return
	}
	themes := map[string]bool{}
	for _, d := range w.catalog.Planets {
		if d.Ruins != nil && d.Ruins.Theme != "" {
			themes[d.Ruins.Theme] = true
		}
	}
	var names []string
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	if combo("Ruin collection", strings.TrimSuffix(r.Theme, " Ruins")) {
		for _, name := range names {
			if imgui.Selectable(strings.TrimSuffix(name, " Ruins")) {
				r.Theme, r.Templates = name, nil
			}
		}
		imgui.EndCombo()
	}
	workshop.Muted("Templates keep their own costs, placement rules and linked rooms.")
	slider("Ruin budget", &r.Budget, 0, 5, "%.2f x normal")
	workshop.Tooltip("A larger budget gives the game more room to place ruins. Large ruins still need open space.")
	if imgui.CollapsingHeader("Mineral deposits") {
		slider("Mineral budget", &r.Minerals, 0, 100, "%.0f")
	}
	var choices []string
	for path, o := range w.catalog.Dme.Objects {
		if strings.HasPrefix(path, "/datum/map_template/ruin/") && o.Vars.TextV("id", "") != "" && o.Vars.TextV("ruin_type", "") == r.Theme {
			choices = append(choices, path)
		}
	}
	sort.Slice(choices, func(i, j int) bool {
		return w.catalog.Dme.Objects[choices[i]].Vars.TextV("name", choices[i]) < w.catalog.Dme.Objects[choices[j]].Vars.TextV("name", choices[j])
	})
	all := r.Templates == nil
	if imgui.Checkbox("Use all ruins in this collection", &all) {
		if all {
			r.Templates = nil
		} else {
			r.Templates = append([]string{}, choices...)
		}
	}
	selected := map[string]bool{}
	for _, p := range r.Templates {
		selected[p] = true
	}
	if !all {
		for _, p := range r.Templates {
			if o := w.catalog.Dme.Objects[p]; o != nil && o.Vars.TextV("ruin_type", "") != r.Theme {
				choices = append(choices, p)
			}
		}
	}
	count := len(selected)
	if all {
		count = len(choices)
	}
	workshop.Muted(fmt.Sprintf("%d ruins allowed", count))
	textField("Find a ruin", "Name", &w.ruinFilter)
	imgui.BeginChild("planet-ruin-choices")
	for _, path := range choices {
		o := w.catalog.Dme.Objects[path]
		name := o.Vars.TextV("name", planet.Label(path))
		if !strings.Contains(strings.ToLower(name), strings.ToLower(w.ruinFilter)) {
			continue
		}
		checked := all || selected[path]
		if imgui.Checkbox("##"+path, &checked) {
			if all {
				r.Templates = append([]string{}, choices...)
				all = false
			}
			if checked {
				r.Templates = append(r.Templates, path)
			} else {
				for i, p := range r.Templates {
					if p == path {
						r.Templates = append(r.Templates[:i], r.Templates[i+1:]...)
						break
					}
				}
			}
		}
		imgui.SameLine()
		imgui.TextWrapped(strings.TrimPrefix(strings.TrimPrefix(name, strings.TrimSuffix(r.Theme, " Ruins")+"-Ruin "), strings.TrimSuffix(r.Theme, " Ruins")+"-Micro "))
		workshop.Tooltip(name + "\n" + o.Vars.TextV("description", "") + fmt.Sprintf("\nCost: %s", o.Vars.ValueV("cost", "1")))
	}
	imgui.EndChild()
}
