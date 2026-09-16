package wsship

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/ship"
)

type crewEditor struct {
	group                                   int
	settings, picking                       bool
	scope                                   string
	jobs                                    []ship.CrewJob
	scopes                                  []ship.CrewScope
	selected                                int
	slot                                    int
	filter, outfitFilter, lastPrefab, error string
	items, outfits                          []string
	direction                               int
	dirty                                   bool
	copySources                             []ship.RoomCrewSource
	copyVariant, copyJob                    int
	copyOpen                                bool
	contents                                int // 0 equipment, 1 backpack, 2 belt
}

func (ws *WsShip) beginCrew() {
	ws.flush()
	ws.OnFocusChange(false)
	ws.task = taskCrew
	ws.crew = crewEditor{selected: -1, direction: 2}
	ws.crewVisual = crewVisual{}
	for path, o := range ws.project.Dme.Objects {
		if strings.HasPrefix(path, "/obj/item/") {
			ws.crew.items = append(ws.crew.items, path)
		}
		if strings.HasPrefix(path, "/datum/outfit/") && ws.project.Dme.Objects[o.Vars.ValueV("jobtype", "")] != nil && !strings.Contains(path, "/workshop_") {
			ws.crew.outfits = append(ws.crew.outfits, path)
		}
	}
	sort.Strings(ws.crew.items)
	sort.Strings(ws.crew.outfits)
	ws.loadCrewScope(ws.variantCrewScope())
	tools.SetEnabled(false)
}

func (ws *WsShip) variantCrewScope() string {
	if theme := ws.currentTheme(); theme.ID != "" {
		return "theme/" + theme.ID
	}
	return "ship"
}

func (ws *WsShip) switchCrewTheme(index int) {
	if index < 0 || index >= len(ws.project.Hull.Themes) || index == ws.theme || !ws.commitCrew() {
		return
	}
	moduleID, _ := ship.RoomCrewIDs(ws.crew.scope)
	ws.switchTheme(index)
	ws.crew.scopes = ws.project.CrewScopesForTheme(ws.currentTheme())
	// Keep a room roster only if that option is available in the new variant.
	// Otherwise select the variant's own crew, never an invisible room.
	for _, s := range ws.crew.scopes {
		if module, _ := ship.RoomCrewIDs(s.ID); moduleID != "" && module == moduleID {
			ws.loadCrewScope(s.ID)
			return
		}
	}
	ws.loadCrewScope(ws.variantCrewScope())
}

func (ws *WsShip) loadCrewScope(scope string) {
	moduleID, roomTheme := ship.RoomCrewIDs(scope)
	// Context menus, undo and recovery can target a variant other than the
	// one currently displayed. Keep the selector and its room list in sync.
	if strings.HasPrefix(scope, "theme/") {
		if index := ws.themeIndexByID(strings.TrimPrefix(scope, "theme/")); index >= 0 {
			ws.switchTheme(index)
		}
	} else if m, ok := ws.module(moduleID); moduleID != "" && ok {
		if index := ws.themeIndexByID(roomTheme); roomTheme != "" && index >= 0 {
			ws.switchTheme(index)
		}
		theme := ws.currentTheme()
		if !m.Available(theme.ID) || !ship.Contains(ws.project.Hull.SlotsFor(theme), m.Slot) {
			for i, candidate := range ws.project.Hull.Themes {
				if m.Available(candidate.ID) && ship.Contains(ws.project.Hull.SlotsFor(candidate), m.Slot) {
					ws.switchTheme(i)
					break
				}
			}
		}
		scope = ws.project.RoomCrewScope(moduleID, ws.currentTheme().ID)
	}
	ws.crew.scopes = ws.project.CrewScopesForTheme(ws.currentTheme())
	jobs, err := ws.project.CrewJobs(scope)
	if err != nil {
		ws.crew.error = err.Error()
		return
	}
	ws.crew.scope, ws.crew.jobs, ws.crew.selected, ws.crew.dirty = scope, jobs, -1, false
	ws.crew.copySources, ws.crew.copyOpen = nil, false
	ws.crew.copyVariant, ws.crew.copyJob = 0, 0
	if moduleID != "" {
		ws.crew.copySources, err = ws.project.RoomCrewSources(moduleID, ws.currentTheme().ID)
	}
	if len(jobs) > 0 {
		ws.crew.selected = 0
	}
	ws.armCrewPicker()
	ws.crew.error = ""
	if err != nil {
		ws.crew.error = err.Error()
	}
}
func (ws *WsShip) armCrewPicker() {
	ws.crew.lastPrefab = ""
	if p, ok := ws.app.SelectedPrefab(); ok {
		ws.crew.lastPrefab = p.Path()
	}
}
func (ws *WsShip) commitCrew() bool {
	if ws.task != taskCrew || !ws.crew.dirty {
		return true
	}
	if e := ws.project.ValidateCrew(ws.crew.jobs); e != nil {
		ws.crew.error = e.Error()
		return false
	}
	ws.message = ""
	ws.change("Edit crew roster", func() error { return ws.project.SetCrewJobs(ws.crew.scope, ws.crew.jobs) })
	if ws.message != "" {
		ws.crew.error = ws.message
		return false
	}
	jobs, e := ws.project.CrewJobs(ws.crew.scope)
	if e != nil {
		ws.crew.error = e.Error()
		return false
	}
	ws.crew.jobs, ws.crew.dirty, ws.crew.error = jobs, false, ""
	tools.SetEnabled(false)
	return true
}

func (ws *WsShip) crewScopeLabel(s ship.CrewScope) string {
	if s.ID == "ship" && len(ws.project.Hull.Themes) > 0 {
		return "Ship crew (shared)"
	}
	if strings.HasPrefix(s.ID, "theme/") {
		return "Variant crew"
	}
	return s.Name
}

func (ws *WsShip) crewControls() {
	c := &ws.crew
	if actionButton("< Back to ship", false) && ws.commitCrew() {
		ws.finishTask()
		ws.OnFocusChange(true)
		return
	}
	workshop.Section("CREW ROSTER", style.Violet)
	if len(ws.project.Hull.Themes) > 0 && combo("Variant", ws.currentTheme().Name) {
		for i, theme := range ws.project.Hull.Themes {
			if imgui.SelectableV(theme.Name, i == ws.theme, 0, imgui.Vec2{}) {
				ws.switchCrewTheme(i)
			}
		}
		imgui.EndCombo()
	}
	preview := "Ship crew"
	for _, s := range c.scopes {
		if s.ID == c.scope {
			preview = ws.crewScopeLabel(s)
		}
	}
	if comboHelp("Roster", preview, "Ship crew is the base roster. A variant can replace it. Room options add jobs when installed.") {
		for _, s := range c.scopes {
			if imgui.SelectableV(ws.crewScopeLabel(s)+"##"+s.ID, c.scope == s.ID, 0, imgui.Vec2{}) && ws.commitCrew() {
				ws.loadCrewScope(s.ID)
			}
		}
		imgui.EndCombo()
	}
	if strings.HasPrefix(c.scope, "theme/") {
		hint("An empty variant roster uses the ship's crew.")
	}
	if strings.HasPrefix(c.scope, "module/") {
		hint("These jobs belong to this room option in " + ws.currentTheme().Name + ".")
		ws.roomCrewCopyControls()
	}
	space()
	total := 0
	for _, j := range c.jobs {
		total += j.Slots
	}
	imgui.TextColored(style.Violet, fmt.Sprintf("%d jobs / %d slots", len(c.jobs), total))
	if actionButton("+ Create job", true) && ws.commitCrew() {
		name := "New job"
		for n := 2; ; n++ {
			used := false
			for _, j := range c.jobs {
				if strings.EqualFold(strings.TrimSpace(j.Name), name) {
					used = true
				}
			}
			if !used {
				break
			}
			name = fmt.Sprintf("New job %d", n)
		}
		c.jobs = append(c.jobs, ship.CrewJob{Name: name, Slots: 1, Category: "Assistant", Outfit: "/datum/outfit/job/assistant"})
		c.selected = len(c.jobs) - 1
		c.dirty = true
		ws.commitCrew()
		ws.armCrewPicker()
	}
	if strings.HasPrefix(c.scope, "theme/") && len(c.jobs) == 0 && actionButton("Copy ship crew", false) {
		jobs, e := ws.project.CrewJobs("ship")
		if e != nil {
			c.error = e.Error()
		} else {
			for i := range jobs {
				jobs[i].ID = ""
			}
			c.jobs = jobs
			c.selected = 0
			c.dirty = true
			ws.commitCrew()
		}
	}
	space()
	imgui.BeginChildV("crew-job-list", imgui.Vec2{Y: max(100, imgui.ContentRegionAvail().Y-90*window.PointSize())}, false, 0)
	for i, j := range c.jobs {
		imgui.PushIDInt(i)
		if workshop.Row("job", j.Name, j.Category, fmt.Sprintf("%d", j.Slots), c.selected == i, crewCategoryColor(j.Category), 0) && ws.commitCrew() {
			if c.selected == i {
				c.selected = -1
			} else {
				c.selected = i
			}
			ws.armCrewPicker()
		}
		imgui.PopID()
	}
	imgui.EndChild()
	if c.error != "" {
		imgui.PushStyleColor(imgui.StyleColorText, style.Danger)
		imgui.TextWrapped(c.error)
		imgui.PopStyleColor()
	} else {
		hint("Save from Review & save.")
	}
}
func (ws *WsShip) crewItemName(path string) string {
	if path == "" {
		return "Empty"
	}
	if o := ws.project.Dme.Objects[path]; o != nil {
		if name := o.Vars.TextV("name", ""); name != "" {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(name, `\improper `), `\proper `))
		}
	}
	return path
}
func (ws *WsShip) crewContent() {
	c := &ws.crew
	if c.selected < 0 || c.selected >= len(c.jobs) {
		if len(c.jobs) == 0 && strings.HasPrefix(c.scope, "theme/") {
			title("Uses ship crew")
			hint("Copy the ship crew into this variant to customize it, or create a job.")
		} else if len(c.jobs) == 0 {
			title("Build your crew")
			hint("Create a job in the roster to set its role and starting equipment.")
		} else {
			title("Select a job")
			hint("Choose a job from the roster to edit it.")
		}
		return
	}
	if p, ok := ws.app.SelectedPrefab(); ok && p.Path() != c.lastPrefab {
		c.lastPrefab = p.Path()
		if strings.HasPrefix(p.Path(), "/obj/item/") {
			ws.chooseCrewItem(p.Path())
		}
	}
	j := c.jobs[c.selected]
	title(j.Name)
	slotLabel := "crew slots"
	if j.Slots == 1 {
		slotLabel = "crew slot"
	}
	hint(fmt.Sprintf("%s  /  %d %s", j.Category, j.Slots, slotLabel))
	space()
	scale := window.PointSize()
	available := imgui.ContentRegionAvail().X
	// Keep usable form and picker widths before adding a third column.
	if available >= 1040*scale {
		workshop.Panel("crew-details", imgui.Vec2{X: 260 * scale}, false)
		ws.crewDetails()
		workshop.EndPanel()
		if c.selected < 0 || c.selected >= len(c.jobs) {
			return
		}
		imgui.SameLine()
		imgui.BeginChildV("crew-loadout", imgui.Vec2{}, false, imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
		ws.crewLoadout()
		imgui.EndChild()
		return
	}
	// These controls also work with keyboard navigation and preserve each view's scroll.
	if available < 610*scale {
		width := max(1, (available-24*scale)/3)
		if crewTab("Loadout", !c.settings && !c.picking, width) {
			c.settings, c.picking = false, false
		}
		imgui.SameLine()
		if crewTab("Items", !c.settings && c.picking, width) {
			c.settings, c.picking = false, true
		}
		imgui.SameLine()
		if crewTab("Job settings", c.settings, width) {
			c.settings = true
		}
	} else {
		width := min(190*scale, (available-12*scale)/2)
		if crewTab("Equipment", !c.settings, width) {
			c.settings = false
		}
		imgui.SameLine()
		if crewTab("Job settings", c.settings, width) {
			c.settings = true
		}
	}
	space()
	if c.settings {
		workshop.Panel("crew-details", imgui.Vec2{}, false)
		ws.crewDetails()
		workshop.EndPanel()
	} else {
		ws.crewLoadout()
	}
}

func crewTab(label string, selected bool, width float32) bool {
	if selected {
		imgui.PushStyleColor(imgui.StyleColorButton, style.RGB(0x3c3556))
		imgui.PushStyleColor(imgui.StyleColorText, style.Violet)
	}
	clicked := imgui.ButtonV(label, imgui.Vec2{X: width, Y: 34 * window.PointSize()})
	if selected {
		imgui.PopStyleColorV(2)
	}
	return clicked
}

func crewCategoryColor(category string) imgui.Vec4 {
	switch category {
	case "Engineering":
		return style.Amber
	case "Medical":
		return style.Teal
	case "Security":
		return style.Danger
	case "Command":
		return style.Violet
	default:
		return style.Muted
	}
}

func (ws *WsShip) crewLoadout() {
	c := &ws.crew
	scale := window.PointSize()
	available := imgui.ContentRegionAvail().X
	if available < 610*scale {
		if c.picking {
			workshop.Panel("crew-item-picker", imgui.Vec2{}, false)
			ws.crewPicker()
		} else {
			workshop.Panel("crew-mannequin", imgui.Vec2{}, false)
			ws.crewEquipment()
		}
		workshop.EndPanel()
		return
	}
	workshop.Panel("crew-mannequin", imgui.Vec2{X: min(340*scale, available*.46)}, false)
	ws.crewEquipment()
	workshop.EndPanel()
	imgui.SameLine()
	workshop.Panel("crew-item-picker", imgui.Vec2{}, false)
	ws.crewPicker()
	workshop.EndPanel()
}

func (ws *WsShip) crewEquipment() {
	c := &ws.crew
	imgui.TextColored(style.Violet, "LOADOUT")
	ws.crewMannequin()
	space()
	width := max(1, (imgui.ContentRegionAvail().X-24*window.PointSize())/3)
	for i, label := range []string{"Clothing", "Gear", "Carry"} {
		if i > 0 {
			imgui.SameLine()
		}
		if crewTab(label, c.group == i, width) {
			c.group = i
		}
	}
	imgui.BeginChild(fmt.Sprintf("crew-slots-%d", c.group))
	equipment := ws.project.CrewEquipment(c.jobs[c.selected])
	groups := []struct {
		name string
		ids  []string
	}{
		{"CLOTHING", []string{"uniform", "suit", "head", "mask", "glasses", "gloves", "shoes", "neck", "accessory"}},
		{"GEAR & STORAGE", []string{"ears", "back", "belt", "id", "suit_store", "box"}},
		{"POCKETS & HANDS", []string{"l_pocket", "r_pocket", "l_hand", "r_hand"}},
	}
	for groupIndex, group := range groups {
		if groupIndex != c.group {
			continue
		}
		workshop.Section(group.name, style.Muted)
		for _, id := range group.ids {
			for i, slot := range ship.EquipmentSlots {
				if slot.ID != id {
					continue
				}
				path := equipment[id]
				name := ws.crewItemName(path)
				badge := ""
				if c.contents == 0 && c.slot == i {
					badge = "Edit"
				}
				if workshop.Row(id, slot.Name, name, badge, c.contents == 0 && c.slot == i, style.Violet, 0) {
					c.slot, c.contents = i, 0
					c.picking = true
					ws.armCrewPicker()
				}
			}
		}
	}
	if c.group == 2 {
		workshop.Section("CONTAINER CONTENTS", style.Amber)
		if actionButton("Backpack contents", c.contents == 1) {
			c.contents, c.picking = 1, true
			ws.armCrewPicker()
		}
		if actionButton("Belt contents", c.contents == 2) {
			c.contents, c.picking = 2, true
			ws.armCrewPicker()
		}
	}
	imgui.EndChild()
}

func (ws *WsShip) crewDetails() {
	c := &ws.crew
	j := &c.jobs[c.selected]
	workshop.Section("JOB SETTINGS", style.Violet)
	if textField("Job name", "e.g. Salvage Engineer", &j.Name) {
		c.dirty = true
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		ws.commitCrew()
		j = &c.jobs[c.selected]
	}
	slots := int32(j.Slots)
	numberField("Number of slots", &slots)
	if int(slots) != j.Slots {
		j.Slots = int(slots)
		c.dirty = true
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		ws.commitCrew()
		j = &c.jobs[c.selected]
	}
	if combo("Category", j.Category) {
		for _, category := range ship.CrewCategories {
			if imgui.Selectable(category) {
				j.Category = category
				c.dirty = true
			}
		}
		imgui.EndCombo()
		if c.dirty {
			ws.commitCrew()
			j = &c.jobs[c.selected]
		}
	}
	if imgui.Checkbox("Officer", &j.Officer) {
		c.dirty = true
		ws.commitCrew()
		j = &c.jobs[c.selected]
	}
	tooltip("Marks this job as an officer in the ship roster.")
	space()
	if comboHelp("Starting job outfit", ws.crewItemName(ws.project.CrewOutfit(*j)), "Supplies the underlying job, ID access and default gear. Your equipment selections override this preset.") {
		textField("Find an outfit", "Name or type path", &c.outfitFilter)
		for _, path := range c.outfits {
			if !strings.Contains(strings.ToLower(ws.crewItemName(path)+" "+path), strings.ToLower(c.outfitFilter)) {
				continue
			}
			if imgui.Selectable(ws.crewItemName(path) + "##" + path) {
				j.Outfit = path
				j.BaseOutfit = ""
				j.Equipment = nil
				j.Backpack = nil
				j.Belt = nil
				c.dirty = true
			}
			tooltip(path)
		}
		imgui.EndCombo()
		if c.dirty {
			ws.commitCrew()
			j = &c.jobs[c.selected]
		}
	}
	if o := ws.project.Dme.Objects[ws.project.CrewOutfit(*j)]; o != nil {
		hint("Job: " + ws.crewItemName(o.Vars.ValueV("jobtype", "")))
	}
	space()
	workshop.Section("CARRIED ITEMS", style.Amber)
	if actionButton("Backpack contents...", c.contents == 1) {
		c.contents, c.settings, c.picking = 1, false, true
		ws.armCrewPicker()
	}
	if actionButton("Belt contents...", c.contents == 2) {
		c.contents, c.settings, c.picking = 2, false, true
		ws.armCrewPicker()
	}
	tooltip("Extra items placed inside the backpack or belt.")
	space()
	ws.crewJobActions()
}

func (ws *WsShip) crewJobActions() {
	c := &ws.crew
	// A field can auto-save on mouse-down. Keep these rows in place so Delete
	// cannot move underneath a click aimed at Apply or Discard.
	imgui.BeginDisabledV(!c.dirty)
	if actionButton("Apply changes", true) {
		ws.commitCrew()
	}
	if actionButton("Discard unfinished changes", false) {
		ws.loadCrewScope(c.scope)
	}
	imgui.EndDisabled()
	if c.selected < 0 || c.selected >= len(c.jobs) {
		return
	}
	j := &c.jobs[c.selected]
	space()
	imgui.PushStyleColor(imgui.StyleColorText, style.Danger)
	deleting := actionButton("Delete job...", false)
	imgui.PopStyleColor()
	if deleting {
		imgui.OpenPopup("Delete crew job")
	}
	if imgui.BeginPopupModal("Delete crew job") {
		imgui.TextWrapped("Remove " + j.Name + " from this roster? You can undo this change.")
		if imgui.Button("Remove job") {
			c.jobs = append(c.jobs[:c.selected], c.jobs[c.selected+1:]...)
			c.selected = min(c.selected, len(c.jobs)-1)
			c.dirty = true
			ws.commitCrew()
			imgui.CloseCurrentPopup()
		}
		imgui.SameLine()
		if imgui.Button("Cancel") {
			imgui.CloseCurrentPopup()
		}
		imgui.EndPopup()
	}
}
func (ws *WsShip) crewItemFits(path string) bool {
	c := &ws.crew
	o := ws.project.Dme.Objects[path]
	if o == nil || !strings.HasPrefix(path, "/obj/item/") {
		return false
	}
	if c.contents != 0 {
		return true
	}
	s := ship.EquipmentSlots[c.slot]
	if s.ID == "accessory" {
		return strings.HasPrefix(path, "/obj/item/clothing/accessory/")
	}
	return s.Flag == 0 || int(o.Vars.FloatV("slot_flags", 0))&s.Flag != 0
}
func (ws *WsShip) crewItemSprite(path string) *dmicon.Sprite {
	o := ws.project.Dme.Objects[path]
	if o == nil {
		return nil
	}
	stateName := o.Vars.TextV("icon_state", "")
	// Base items may use an empty state that only draws a diagnostic sprite.
	if stateName == "" {
		return nil
	}
	dmi, err := dmicon.Cache.Get(o.Vars.TextV("icon", ""))
	if err != nil {
		return nil
	}
	// Check the inherited state directly: the map renderer's empty-state and
	// placeholder fallbacks can make abstract item types appear usable here.
	state := dmi.States[stateName]
	if state == nil || state.Frames == 0 || len(state.Sprites) == 0 {
		return nil
	}
	return state.Sprite()
}
func (ws *WsShip) chooseCrewItem(path string) {
	c := &ws.crew
	if c.selected < 0 || c.selected >= len(c.jobs) {
		return
	}
	if !ws.crewItemFits(path) {
		c.error = "That item cannot go in " + ship.EquipmentSlots[c.slot].Name + ". Choose a matching item or another slot."
		return
	}
	j := &c.jobs[c.selected]
	if c.contents == 0 {
		if j.Equipment == nil {
			j.Equipment = map[string]string{}
		}
		j.Equipment[ship.EquipmentSlots[c.slot].ID] = path
	} else {
		bag, e := ws.project.CrewContents(*j, c.contents == 2)
		if e != nil {
			c.error = e.Error()
			return
		}
		bag[path]++
		if c.contents == 1 {
			j.Backpack = bag
		} else {
			j.Belt = bag
		}
	}
	c.dirty = true
	ws.commitCrew()
	ws.app.DoSelectPrefab(dmmap.PrefabStorage.Initial(path))
	c.lastPrefab = path
}
func (ws *WsShip) crewPicker() {
	c := &ws.crew
	if c.selected < 0 || c.selected >= len(c.jobs) {
		return
	}
	j := &c.jobs[c.selected]
	label := ship.EquipmentSlots[c.slot].Name
	if c.contents > 0 {
		label = []string{"", "Backpack contents", "Belt contents"}[c.contents]
	}
	imgui.TextColored(style.Violet, "ITEM LIBRARY")
	title(label)
	if c.contents == 0 {
		equipment := ws.project.CrewEquipment(*j)
		hint(ws.crewItemName(equipment[ship.EquipmentSlots[c.slot].ID]))
		if imgui.Button("Empty slot") {
			if j.Equipment == nil {
				j.Equipment = map[string]string{}
			}
			j.Equipment[ship.EquipmentSlots[c.slot].ID] = ""
			c.dirty = true
			ws.commitCrew()
			j = &c.jobs[c.selected]
		}
		imgui.SameLine()
		if imgui.Button("Use preset") {
			delete(j.Equipment, ship.EquipmentSlots[c.slot].ID)
			c.dirty = true
			ws.commitCrew()
			j = &c.jobs[c.selected]
		}
	} else {
		bag, e := ws.project.CrewContents(*j, c.contents == 2)
		if e != nil {
			hint(e.Error())
		} else {
			keys := []string{}
			for k := range bag {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			imgui.BeginChildV("bag-contents", imgui.Vec2{Y: 150 * window.PointSize()}, true, 0)
			for _, path := range keys {
				imgui.PushID(path)
				n := int32(bag[path])
				imgui.SetNextItemWidth(90 * window.PointSize())
				if imgui.InputInt("##quantity", &n) {
					if n <= 0 {
						delete(bag, path)
					} else {
						bag[path] = int(n)
					}
					if c.contents == 1 {
						j.Backpack = bag
					} else {
						j.Belt = bag
					}
					c.dirty = true
				}
				imgui.SameLine()
				imgui.Text(ws.crewItemName(path))
				imgui.PopID()
			}
			imgui.EndChild()
			if c.dirty && !imgui.IsAnyItemActive() {
				ws.commitCrew()
				j = &c.jobs[c.selected]
			}
		}
		hint("Pick an item below to add one. Set its quantity to zero to remove it.")
	}
	space()
	textField("Find an item", "Search item names or type paths", &c.filter)
	tooltip("You can also equip an item by selecting it in the Environment panel.")
	if p, ok := ws.app.SelectedPrefab(); ok && ws.crewItemFits(p.Path()) {
		if actionButton("Equip selected item", false) {
			ws.chooseCrewItem(p.Path())
		}
	}
	space()
	imgui.BeginChild("crew-search-results")
	currentItem := ws.project.CrewEquipment(*j)[ship.EquipmentSlots[c.slot].ID]
	count := 0
	for _, path := range c.items {
		if !ws.crewItemFits(path) || !strings.Contains(strings.ToLower(ws.crewItemName(path)+" "+path), strings.ToLower(c.filter)) {
			continue
		}

		sprite := ws.crewItemSprite(path)
		if sprite == nil {
			continue
		}
		pos := imgui.CursorScreenPos()
		selected := c.contents == 0 && currentItem == path
		badge := ""
		if selected {
			badge = "On"
		}
		if workshop.Row(path, ws.crewItemName(path), strings.TrimPrefix(path, "/obj/item/"), badge, selected, style.Teal, 40) {
			ws.chooseCrewItem(path)
			j = &c.jobs[c.selected]
		}
		s := window.PointSize()
		imgui.WindowDrawList().AddImageV(imgui.TextureID(sprite.Texture()), imgui.Vec2{X: pos.X + 10*s, Y: pos.Y + 13*s}, imgui.Vec2{X: pos.X + 42*s, Y: pos.Y + 45*s}, imgui.Vec2{X: sprite.U1, Y: sprite.V1}, imgui.Vec2{X: sprite.U2, Y: sprite.V2}, 0xffffffff)

		count++
		if count == 200 {
			hint("Type more to narrow the results.")
			break
		}
	}
	if count == 0 {
		hint("No matching items for this slot.")
	}
	imgui.EndChild()
}
