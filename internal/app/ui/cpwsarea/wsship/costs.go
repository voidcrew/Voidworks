package wsship

import (
	"fmt"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/ship"
)

type costEntry struct {
	scope ship.CostScope
	value ship.PartCosts
	error string
}

type costEditor struct {
	entries              []costEntry
	selected             int
	values, initial      ship.PartCosts
	dirty                bool
	filter, group, error string
}

func (ws *WsShip) commitDraft() bool {
	return ws.commitCrew() && ws.commitCosts() && ws.commitRename() && ws.commitSettings()
}

func (ws *WsShip) beginCosts(scope string) {
	if !ws.commitDraft() {
		return
	}
	ws.flush()
	ws.OnFocusChange(false)
	ws.task = taskCosts
	ws.costs = costEditor{selected: -1}
	for _, s := range ws.project.CostScopes() {
		cost, err := ws.project.PartCosts(s.ID)
		entry := costEntry{scope: s, value: cost}
		if err != nil {
			entry.error = err.Error()
		}
		ws.costs.entries = append(ws.costs.entries, entry)
		if s.ID == scope {
			ws.costs.selected = len(ws.costs.entries) - 1
		}
	}
	ws.loadCostEntry(max(0, ws.costs.selected))
	tools.SetEnabled(false)
}

func copyCosts(cost ship.PartCosts) ship.PartCosts {
	copy := ship.PartCosts{}
	for k, v := range cost {
		copy[k] = v
	}
	return copy
}

func (ws *WsShip) loadCostEntry(index int) {
	c := &ws.costs
	if index < 0 || index >= len(c.entries) {
		return
	}
	e := c.entries[index]
	c.selected = index
	c.values, c.initial = copyCosts(e.value), copyCosts(e.value)
	c.dirty, c.error = false, e.error
}

func (ws *WsShip) commitCosts() bool {
	if ws.task != taskCosts || !ws.costs.dirty {
		return true
	}
	c := &ws.costs
	if err := c.values.Validate(); err != nil {
		c.error = "" // The form displays the current validation error directly.
		return false
	}
	scope := c.entries[c.selected].scope.ID
	values := copyCosts(c.values)
	ws.message = ""
	ws.change("Edit part costs", func() error { return ws.project.SetPartCosts(scope, values) })
	if ws.message != "" {
		c.error = ws.message
		return false
	}
	c.entries[c.selected].value = values
	c.entries[c.selected].error = ""
	ws.loadCostEntry(c.selected)
	tools.SetEnabled(false)
	return true
}

func (ws *WsShip) costsControls() {
	if actionButton("< Back to ship", false) && ws.commitCosts() {
		ws.finishTask()
		ws.OnFocusChange(true)
		return
	}
	workshop.Section("PART COSTS", style.Amber)
	hint("Set the parts needed to unlock each component.")
	space()
	textField("Find a component", "Search names...", &ws.costs.filter)
	group := ws.costs.group
	if group == "" {
		group = "All components"
	}
	if combo("Show", group) {
		for _, kind := range []string{"All components", "Hull", "Ship theme", "Module option"} {
			if imgui.Selectable(kind) {
				ws.costs.group = kind
				if kind == "All components" {
					ws.costs.group = ""
				}
			}
		}
		imgui.EndCombo()
	}
	space()
	hint("Apply costs to keep editing. Review & save writes them to the project.")
}

func (ws *WsShip) costVisible(entry costEntry) bool {
	c := &ws.costs
	return (c.group == "" || c.group == entry.scope.Kind) && strings.Contains(strings.ToLower(entry.scope.Name), strings.ToLower(strings.TrimSpace(c.filter)))
}

func (ws *WsShip) costsContent() {
	c := &ws.costs
	title("Part costs")
	hint("Hull, ship themes, and module options")
	space()
	scale := window.PointSize()
	wide := imgui.ContentRegionAvail().X >= 780*scale
	if wide {
		workshop.Panel("cost-components", imgui.Vec2{X: 280 * scale}, false)
		count := 0
		for i, entry := range c.entries {
			if !ws.costVisible(entry) {
				continue
			}
			count++
			detail := entry.scope.Kind + " / " + entry.value.Summary()
			badge := fmt.Sprint(entry.value.Total())
			if entry.error != "" {
				detail = entry.scope.Kind + " / Check source"
				badge = "!"
			}
			if workshop.Row(entry.scope.ID, entry.scope.Name, detail, badge, i == c.selected, style.Amber, 0) && ws.commitCosts() {
				ws.loadCostEntry(i)
			}
		}
		if count == 0 {
			hint("No matching components.")
		}
		workshop.EndPanel()
		imgui.SameLine()
	} else if c.selected >= 0 && combo("Component", c.entries[c.selected].scope.Kind+": "+c.entries[c.selected].scope.Name) {
		for i, entry := range c.entries {
			if ws.costVisible(entry) && imgui.Selectable(entry.scope.Kind+": "+entry.scope.Name) && ws.commitCosts() {
				ws.loadCostEntry(i)
			}
		}
		imgui.EndCombo()
	}
	workshop.Panel("cost-form", imgui.Vec2{}, false)
	defer workshop.EndPanel()
	if c.selected < 0 || c.selected >= len(c.entries) {
		hint("Choose a component.")
		return
	}
	entry := c.entries[c.selected]
	workshop.Section(strings.ToUpper(entry.scope.Kind), style.Amber)
	title(entry.scope.Name)
	if entry.scope.ID == "ship" {
		hint("Set the price to unlock this base ship.")
	} else if entry.scope.Default {
		hint("Base hulls and default components normally stay free.")
	} else {
		hint("Alternative themes and module options normally have an unlock cost.")
	}
	if entry.scope.Kind == "Module option" {
		hint("This price applies to every theme that uses this module option.")
	}
	space()
	if entry.error != "" {
		imgui.TextWrapped(entry.error)
		return
	}
	partCostFields(c.values)
	c.dirty = !c.values.Equal(c.initial)
	space()
	workshop.Section("TOTAL", style.Teal)
	imgui.TextWrapped(c.values.Summary())
	err := c.values.Validate()
	if err != nil {
		imgui.TextWrapped(err.Error())
	} else if c.error != "" {
		imgui.TextWrapped(c.error)
	}
	space()
	imgui.BeginDisabledV(!c.dirty || err != nil)
	if actionButton("Apply costs", true) {
		ws.commitCosts()
	}
	imgui.EndDisabled()
	if c.dirty && actionButton("Reset changes", false) {
		ws.loadCostEntry(c.selected)
	}
}

func partCostFields(values ship.PartCosts) {
	scale := window.PointSize()
	columns := 1
	if imgui.ContentRegionAvail().X >= 360*scale {
		columns = 2
	}
	width := (imgui.ContentRegionAvail().X - float32(columns-1)*12*scale) / float32(columns)
	colors := []imgui.Vec4{style.RGB(0xf2a7b6), style.Violet, style.Teal, style.Amber}
	for i, class := range ship.PartClasses {
		workshop.Panel("cost-"+class.ID, imgui.Vec2{X: width, Y: 100 * scale}, true)
		imgui.TextColored(colors[i], class.Name)
		value := int32(values[class.ID])
		imgui.SetNextItemWidth(-1)
		if imgui.InputInt("##parts", &value) {
			values[class.ID] = int(value)
		}
		workshop.EndPanel()
		if (i+1)%columns != 0 {
			imgui.SameLine()
		}
	}
}
