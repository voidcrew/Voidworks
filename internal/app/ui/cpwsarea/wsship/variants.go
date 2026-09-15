package wsship

import (
	"fmt"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/ship"
)

// variantInfo caches one variant's summary. The full form compares every room
// copy with the shared room, so it is gathered only for the open variant.
type variantInfo struct {
	info ship.ThemeInfo
	full bool
	err  string
}

// editShare remembers whether the edited room is shared, so the canvas header
// does not compare maps on every frame.
type editShare struct {
	ready         bool
	suffix, about string
}

func (ws *WsShip) variant(t ship.Theme, full bool) (ship.ThemeInfo, string) {
	if cached, ok := ws.variantInfos[t.ID]; ok && (cached.full || !full) {
		return cached.info, cached.err
	}
	entry := variantInfo{full: full}
	if full {
		info, err := ws.project.ThemeSummary(t.ID)
		entry.info = info
		if err != nil {
			entry.err = err.Error()
		}
	} else {
		entry.info, entry.err = ws.variantOverview(t)
	}
	if ws.variantInfos == nil {
		ws.variantInfos = map[string]variantInfo{}
	}
	ws.variantInfos[t.ID] = entry
	return entry.info, entry.err
}

// variantOverview is the summary a closed row needs: no map comparisons.
func (ws *WsShip) variantOverview(t ship.Theme) (ship.ThemeInfo, string) {
	info := ship.ThemeInfo{ID: t.ID, Name: t.Name, Default: t.Default, Slots: ws.project.Hull.SlotsFor(t), Inherited: t.Slots == nil}
	cost, err := ws.project.PartCosts("theme/" + t.ID)
	if err != nil {
		return info, err.Error()
	}
	info.Price = cost.Summary()
	jobs, err := ws.project.CrewJobs("theme/" + t.ID)
	if err != nil {
		return info, err.Error()
	}
	for _, job := range jobs {
		info.Crew += job.Slots
	}
	return info, ""
}

// themeDetail is a variant row's second line: what it offers and what it costs.
func themeDetail(info ship.ThemeInfo) string {
	var parts []string
	if info.Default {
		parts = append(parts, "default")
	}
	switch n := len(info.Slots); n {
	case 0:
		parts = append(parts, "no rooms")
	case 1:
		parts = append(parts, "1 room")
	default:
		parts = append(parts, fmt.Sprintf("%d rooms", n))
	}
	if info.Price != "" {
		parts = append(parts, info.Price)
	}
	if info.Crew == 1 {
		parts = append(parts, "1 crew")
	} else if info.Crew > 1 {
		parts = append(parts, fmt.Sprintf("%d crew", info.Crew))
	}
	return strings.Join(parts, "  ·  ")
}

// optionStatusText says where the variant's copy of a room option comes from.
// Without a base room every variant keeps a copy, so there is nothing to differ
// from.
func optionStatusText(o ship.OptionStatus, shared bool) string {
	switch {
	case !o.Available:
		return "Not offered here"
	case o.Forked && shared && o.Differs:
		return "Own copy  ·  differs"
	case o.Forked:
		return "Own copy"
	}
	return "Shared"
}

// sharedRoom caches the display status until the workshop rebuilds. Resolving
// a room source checks every path component on disk, including for missing
// shared rooms, so it must not run again for each sidebar row on every frame.
// Model operations still resolve and validate the source when an action runs.
func (ws *WsShip) sharedRoom(m ship.Module) bool {
	if shared, ok := ws.sharedRooms[m.File]; ok {
		return shared
	}
	_, err := ws.project.ModuleSource(m, "")
	shared := err == nil
	if ws.sharedRooms == nil {
		ws.sharedRooms = map[string]bool{}
	}
	ws.sharedRooms[m.File] = shared
	return shared
}

// keptRoom picks what stays on screen across a variant switch: the edited room
// when the new variant has it, keeping its option when that is offered too.
func keptRoom(h ship.Hull, t ship.Theme, slot, option string) (string, bool) {
	if slot == "" || !ship.Contains(h.SlotsFor(t), slot) {
		return "", false
	}
	for _, m := range h.Modules {
		if m.ID == option && m.Slot == slot && m.Available(t.ID) {
			return option, true
		}
	}
	return "", true
}

func (ws *WsShip) themeIndexByID(id string) int {
	for i, t := range ws.project.Hull.Themes {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (ws *WsShip) defaultThemeIndex() int {
	for i, t := range ws.project.Hull.Themes {
		if t.Default {
			return i
		}
	}
	return 0
}

func (ws *WsShip) module(id string) (ship.Module, bool) {
	for _, m := range ws.project.Hull.Modules {
		if m.ID == id {
			return m, true
		}
	}
	return ship.Module{}, false
}

// shipSlots lists every room the ship has, including one a variant still
// places after it left the hull's own list.
func (ws *WsShip) shipSlots(variantSlots []string) []string {
	slots := append([]string{}, ws.project.Hull.Slots...)
	for _, slot := range variantSlots {
		if !ship.Contains(slots, slot) {
			slots = append(slots, slot)
		}
	}
	return slots
}

// switchTheme makes another variant the one being edited, keeping the room on
// screen when that variant has it too.
func (ws *WsShip) switchTheme(i int) {
	h := ws.project.Hull
	if i < 0 || i >= len(h.Themes) || i == ws.theme {
		return
	}
	slot, option := ws.editingSlot(), ""
	if slot != "" {
		option = ws.selected[slot]
	}
	option, keep := keptRoom(h, h.Themes[i], slot, option)
	ws.flush()
	ws.theme = i
	ws.defaults()
	ws.rebuild()
	if !keep || ws.invalid {
		return
	}
	if option == "" {
		option = ws.displayedOption(slot)
	}
	if option != "" {
		ws.selectRoomOption(slot, option)
	}
}

func (ws *WsShip) variantsControls() {
	h := ws.project.Hull
	heading("VARIANTS")
	if len(h.Themes) == 0 {
		if workshop.Row("variant-new", "+ Add a variant", "Variants are alternate hulls of this ship with their own rooms, crew and price.", "", false, style.Violet, 0) {
			ws.beginTask(taskTheme)
		}
		return
	}
	for i, t := range h.Themes {
		active := i == ws.theme
		info, err := ws.variant(t, active)
		detail := err
		if err == "" {
			detail = themeDetail(info)
		}
		badge := ""
		if active {
			badge = "editing"
		}
		if workshop.Row("variant-"+t.ID, t.Name, detail, badge, active, style.Violet, 0) && !active {
			ws.switchTheme(i)
		}
		if imgui.BeginPopupContextItemV("variant-menu-"+t.ID, 1) {
			ws.variantMenu(t)
			imgui.EndPopup()
		}
		if active && err == "" {
			ws.variantRooms(info)
		}
	}
	if workshop.Row("variant-new", "+ Add a variant", "", "", false, style.Violet, 0) {
		ws.beginTask(taskTheme)
	}
	tooltip("Variants are alternate hulls of this ship with their own rooms, crew and price.")
}

// variantRooms expands the open variant: its rooms, and under each the options
// players can choose from.
func (ws *WsShip) variantRooms(info ship.ThemeInfo) {
	s := window.PointSize()
	imgui.IndentV(14 * s)
	if info.Inherited {
		hint("inherits ship's rooms")
	}
	for _, slot := range ws.shipSlots(info.Slots) {
		enabled := ship.Contains(info.Slots, slot)
		imgui.PushID("variant-room-" + slot)
		on := enabled
		if imgui.Checkbox(ship.SlotDisplayName(slot), &on) {
			ws.setVariantRoom(info.ID, slot, on)
		}
		if enabled {
			tooltip("Room stays in the map; this variant will not load it.")
		} else {
			tooltip("Load this room in this variant.")
		}
		if enabled {
			ws.variantOptions(info, slot)
		}
		imgui.PopID()
	}
	if imgui.SmallButton("Variant...") {
		imgui.OpenPopup("variant-menu-" + info.ID)
	}
	tooltip("Rename, price, crew, default variant or delete.")
	imgui.UnindentV(14 * s)
}

func (ws *WsShip) variantOptions(info ship.ThemeInfo, slot string) {
	s := window.PointSize()
	imgui.IndentV(14 * s)
	menuWidth := smallButtonWidth("...")
	for _, o := range info.Options {
		if o.Slot != slot {
			continue
		}
		m, ok := ws.module(o.ModuleID)
		if !ok {
			continue
		}
		shared := ws.sharedRoom(m)
		imgui.PushID("variant-option-" + o.ModuleID)
		available := o.Available
		if imgui.Checkbox(m.Name, &available) {
			ws.setVariantOption(info.ID, o.ModuleID, available)
		}
		tooltip("Offer " + m.Name + " in this variant.")
		imgui.SameLine()
		if width := imgui.ContentRegionAvail().X; width > menuWidth {
			imgui.Dummy(imgui.Vec2{X: width - menuWidth})
			imgui.SameLine()
		}
		if imgui.SmallButton("...") {
			imgui.OpenPopup("variant-option-menu")
		}
		if imgui.BeginPopupContextItemV("variant-option-menu", 1) {
			ws.variantOptionMenu(info.ID, m, o, shared)
			imgui.EndPopup()
		}
		imgui.IndentV(22 * s)
		hint(optionStatusText(o, shared))
		imgui.UnindentV(22 * s)
		imgui.PopID()
	}
	imgui.UnindentV(14 * s)
}

func (ws *WsShip) variantOptionMenu(themeID string, m ship.Module, o ship.OptionStatus, shared bool) {
	imgui.BeginDisabledV(!o.Available || o.Forked)
	if imgui.Selectable("Make variant-specific") {
		ws.forkOption(m.ID, themeID)
	}
	imgui.EndDisabled()
	switch {
	case !o.Available:
		tooltip("Offer this option in the variant first.")
	case o.Forked:
		tooltip("This variant already has its own copy.")
	default:
		tooltip("Copies the room so this variant can be edited on its own.")
	}
	key := "fork/" + themeID + "/" + m.ID
	confirm := o.Differs && shared
	imgui.BeginDisabledV(!o.Forked || !shared)
	if ws.pendingDelete == key {
		ws.pendingShown = true
		imgui.TextColored(style.Amber, "Discards this variant's changes to "+m.Name+".")
		if imgui.Selectable("Use shared room") {
			ws.pendingDelete = ""
			ws.unforkOption(m.ID, themeID)
		}
		if imgui.Selectable("Keep the copy") {
			ws.pendingDelete = ""
		}
	} else if imgui.SelectableV("Use shared room", false, confirmFlags(confirm), imgui.Vec2{}) {
		if confirm {
			ws.pendingDelete, ws.pendingShown = key, true
		} else {
			ws.unforkOption(m.ID, themeID)
		}
	}
	imgui.EndDisabled()
	switch {
	case !shared:
		tooltip(m.Name + " has no shared room; every variant keeps its own copy.")
	case !o.Forked:
		tooltip("This variant already uses the shared room.")
	}
}

func confirmFlags(confirm bool) imgui.SelectableFlags {
	if confirm {
		return imgui.SelectableFlagsDontClosePopups
	}
	return 0
}

func (ws *WsShip) variantMenu(t ship.Theme) {
	if imgui.Selectable("Rename & describe") {
		ws.beginRename(taskRenameTheme, t.ID, t.Name)
	}
	if imgui.Selectable("Set price") {
		ws.beginCosts("theme/" + t.ID)
	}
	if imgui.Selectable("Crew") {
		ws.beginCrew()
		ws.loadCrewScope("theme/" + t.ID)
	}
	imgui.BeginDisabledV(t.Default)
	if imgui.Selectable("Make default") {
		ws.change("Set default variant", func() error { return ws.project.SetDefaultTheme(t.ID) })
	}
	imgui.EndDisabled()
	if t.Default {
		tooltip("This is already the default variant.")
	}
	imgui.Separator()
	blocked := ""
	switch {
	case len(ws.project.Hull.Themes) < 2:
		blocked = "This is the ship's only variant."
	case t.Default:
		blocked = "Make another variant the default first."
	}
	key := "theme/" + t.ID
	imgui.BeginDisabledV(blocked != "")
	if ws.pendingDelete == key {
		ws.pendingShown = true
		imgui.TextColored(style.Amber, "Deletes this variant's hull, its own room copies, crew and price.")
		if imgui.Selectable("Delete " + t.Name) {
			ws.pendingDelete = ""
			ws.deleteVariant(t.ID)
		}
		if imgui.Selectable("Keep it") {
			ws.pendingDelete = ""
		}
	} else if imgui.SelectableV("Delete variant...", false, imgui.SelectableFlagsDontClosePopups, imgui.Vec2{}) {
		ws.pendingDelete, ws.pendingShown = key, true
	}
	imgui.EndDisabled()
	if blocked != "" {
		tooltip(blocked)
	}
}

// setVariantRoom turns one room on or off for a variant, writing the variant's
// own room list in the ship's order.
func (ws *WsShip) setVariantRoom(themeID, slot string, on bool) {
	i := ws.themeIndexByID(themeID)
	if i < 0 {
		return
	}
	h := ws.project.Hull
	current := h.SlotsFor(h.Themes[i])
	enabled := map[string]bool{}
	for _, s := range current {
		enabled[s] = true
	}
	enabled[slot] = on
	next := []string{}
	for _, s := range ws.shipSlots(current) {
		if enabled[s] {
			next = append(next, s)
		}
	}
	editing := ws.editingSlot()
	ws.change("Set variant rooms", func() error { return ws.project.SetThemeSlots(themeID, next) })
	if ws.message == "" && !on && editing == slot {
		ws.source = 0
		ws.rebuild()
	}
}

// setVariantOption chooses whether a variant offers one room option.
func (ws *WsShip) setVariantOption(themeID, moduleID string, on bool) {
	m, ok := ws.module(moduleID)
	if !ok {
		return
	}
	offered := map[string]bool{}
	for _, id := range m.Themes {
		offered[id] = true
	}
	offered[themeID] = on
	next := []string{}
	for _, t := range ws.project.Hull.Themes {
		if offered[t.ID] {
			next = append(next, t.ID)
		}
	}
	ws.change("Set variant options", func() error { return ws.project.SetModuleThemes(moduleID, next) })
}

func (ws *WsShip) forkOption(moduleID, themeID string) {
	ws.change("Make room variant-specific", func() error { return ws.project.ForkModuleForTheme(moduleID, themeID) })
}

func (ws *WsShip) unforkOption(moduleID, themeID string) {
	ws.change("Use shared room", func() error { return ws.project.UnforkModuleForTheme(moduleID, themeID) })
}

func (ws *WsShip) deleteVariant(id string) {
	active := ws.currentTheme().ID
	ws.change("Delete variant", func() error { return ws.project.RemoveTheme(id) })
	if ws.message != "" {
		return
	}
	i := ws.themeIndexByID(active)
	if i < 0 {
		i = ws.defaultThemeIndex()
	}
	ws.theme = i
	ws.defaults()
	ws.rebuild()
}
