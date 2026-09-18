package wsship

import (
	"fmt"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/overlay"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

// optionInfo caches per-option summaries; the lookups walk the whole
// environment, so they are refreshed only when the project changes.
type optionInfo struct {
	price string
	crew  int
}

// Keep disclosure separate from edit selection and ImGui's window storage.
// Rebuilding maps or changing tasks must not reset the workshop's view state.
type roomSection struct {
	hull, theme, slot string
	variants          bool
}

func (ws *WsShip) setRoomCollapsed(section roomSection, collapsed bool) {
	if ws.collapsedSections == nil {
		ws.collapsedSections = map[roomSection]bool{}
	}
	ws.collapsedSections[section] = collapsed
}

func (ws *WsShip) roomExpanded(slot string) bool {
	section := roomSection{hull: ws.project.Hull.Type, theme: ws.currentTheme().ID, slot: slot}
	return ws.editingSlot() == slot && !ws.collapsedSections[section]
}

func (ws *WsShip) clickRoomHeading(slot string) {
	section := roomSection{hull: ws.project.Hull.Type, theme: ws.currentTheme().ID, slot: slot}
	if ws.editingSlot() == slot {
		ws.setRoomCollapsed(section, !ws.collapsedSections[section])
	} else {
		ws.editRoom(slot)
		ws.setRoomCollapsed(section, false)
	}
}

func (ws *WsShip) option(id string) optionInfo {
	if info, ok := ws.optionInfos[id]; ok {
		return info
	}
	info := optionInfo{price: "Free"}
	if costs, err := ws.project.DisplayPartCosts("module/" + id); err == nil {
		info.price = costs.Summary()
	}
	if jobs, err := ws.project.CrewJobs(ws.project.RoomCrewScope(id, ws.currentTheme().ID)); err == nil {
		info.crew = len(jobs)
	}
	if ws.optionInfos == nil {
		ws.optionInfos = map[string]optionInfo{}
	}
	ws.optionInfos[id] = info
	return info
}

func (ws *WsShip) slotOptions(slot string) []ship.Module {
	var options []ship.Module
	for _, m := range ws.project.Hull.Modules {
		if m.Slot == slot && m.Available(ws.currentTheme().ID) {
			options = append(options, m)
		}
	}
	return options
}

// displayedOption is the option a room row edits: what is shown, else the default.
func (ws *WsShip) displayedOption(slot string) string {
	if id := ws.selected[slot]; id != "" {
		return id
	}
	options := ws.slotOptions(slot)
	for _, m := range options {
		if m.Default {
			return m.ID
		}
	}
	if len(options) > 0 {
		return options[0].ID
	}
	return ""
}

// editingSlot names the room whose option is the edit target.
func (ws *WsShip) editingSlot() string {
	if ws.assembly == nil || ws.source <= 0 || ws.source >= len(ws.assembly.Sources) {
		return ""
	}
	return ws.assembly.Sources[ws.source].Slot
}

// editRoom shows a room's option and makes it the edit target.
func (ws *WsShip) editRoom(slot string) {
	if ws.task != taskPaint || ws.project == nil {
		return
	}
	if id := ws.displayedOption(slot); id != "" {
		ws.selectRoomOption(slot, id)
	}
	ws.hoverRoom = ""
}

func (ws *WsShip) roomsControls() {
	h := ws.project.Hull
	heading("MODULES")
	if workshop.Row("room-hull", "Hull", "Floors, walls, permanent equipment", "", ws.source == 0, style.Teal, 0) && ws.source != 0 {
		ws.flush()
		ws.source = 0
		ws.rebuild()
	}
	editing := ws.editingSlot()
	for _, slot := range h.SlotsFor(ws.currentTheme()) {
		options := ws.slotOptions(slot)
		detail := fmt.Sprintf("%d options", len(options))
		if len(options) == 1 {
			detail = "1 option"
		}
		if room, ok := ws.roomShape(slot); ok {
			if room.Footprint.IsFull() {
				detail += fmt.Sprintf("  ·  %d x %d", room.Footprint.W, room.Footprint.H)
			} else {
				detail += fmt.Sprintf("  ·  %d tiles in %d x %d", room.Footprint.Count(), room.Footprint.W, room.Footprint.H)
			}
		}
		badge := ""
		if ws.selected[slot] == "" && len(options) > 0 {
			badge = "hull only"
		}
		selected := slot == editing
		if workshop.Row("room-"+slot, ship.SlotDisplayName(slot), detail, badge, selected, style.Amber, 14) {
			ws.clickRoomHeading(slot)
		}
		roomDisclosureArrow(ws.roomExpanded(slot))
		if imgui.BeginPopupContextItemV("room-menu-"+slot, 1) {
			ws.roomMenu(slot, len(options))
			imgui.EndPopup()
		}
		if ws.roomExpanded(slot) {
			ws.optionRows(slot, options)
		}
	}
	detail := ""
	if h.Fixed {
		detail = "Make this ship modular"
	}
	if workshop.Row("room-new", "+ Make an upgrade module", detail, "", false, style.Amber, 0) {
		ws.beginTask(taskRoom)
	}
	tooltip("Select the module's tiles on the hull, then turn them into a swappable upgrade module.")
	ws.checksSummary()
}

// The whole room heading remains one mouse/keyboard target, including its arrow.
func roomDisclosureArrow(expanded bool) {
	s := window.PointSize()
	center := imgui.ItemRectMin().Plus(imgui.Vec2{X: 17 * s, Y: 9*s + imgui.TextLineHeight()/2})
	a, b, c := imgui.Vec2{X: -3, Y: -4}, imgui.Vec2{X: -3, Y: 4}, imgui.Vec2{X: 3}
	if expanded {
		a, b, c = imgui.Vec2{X: -4, Y: -3}, imgui.Vec2{X: 4, Y: -3}, imgui.Vec2{Y: 3}
	}
	imgui.WindowDrawList().AddTriangleFilled(center.Plus(a.Times(s)), center.Plus(b.Times(s)), center.Plus(c.Times(s)), imgui.PackedColorFromVec4(style.Muted))
}

func (ws *WsShip) roomShape(slot string) (ship.RoomShape, bool) {
	if ws.assembly == nil {
		return ship.RoomShape{}, false
	}
	room, ok := ws.assembly.Rooms[slot]
	return room, ok
}

// smallButtonWidth is the space a SmallButton takes, for right-aligned rows.
func smallButtonWidth(label string) float32 {
	st := imgui.CurrentStyle()
	return imgui.CalcTextSize(label, false, -1).X + 2*st.FramePadding().X + st.ItemSpacing().X
}

func (ws *WsShip) optionRows(slot string, options []ship.Module) {
	s := window.PointSize()
	imgui.IndentV(14 * s)
	menuWidth, roomWidth := smallButtonWidth("..."), smallButtonWidth("Module...")
	for _, m := range options {
		imgui.PushID("option-" + m.ID)
		shown := ws.selected[slot] == m.ID
		if imgui.RadioButton("##show", shown) && !shown {
			ws.selectRoomOption(slot, m.ID)
		}
		tooltip("Show and edit " + m.Name + " in this module.")
		imgui.SameLine()
		label := m.Name
		if m.Default {
			label += "  (default)"
		}
		if imgui.SelectableV(label, shown, imgui.SelectableFlagsAllowItemOverlap, imgui.Vec2{X: imgui.ContentRegionAvail().X - menuWidth}) && !shown {
			ws.selectRoomOption(slot, m.ID)
		}
		if imgui.BeginPopupContextItemV("option-menu", 1) {
			ws.optionMenu(slot, m, len(options))
			imgui.EndPopup()
		}
		imgui.SameLine()
		if imgui.SmallButton("...") {
			imgui.OpenPopup("option-menu")
		}
		info := ws.option(m.ID)
		crew := "no crew"
		if info.crew == 1 {
			crew = "1 crew"
		} else if info.crew > 1 {
			crew = fmt.Sprintf("%d crew", info.crew)
		}
		imgui.IndentV(22 * s)
		hint(info.price + "  ·  " + crew)
		imgui.UnindentV(22 * s)
		imgui.PopID()
	}
	if imgui.SelectableV("+ Add option", false, 0, imgui.Vec2{X: imgui.ContentRegionAvail().X - roomWidth}) {
		ws.beginAddOption(slot)
	}
	tooltip("Create another option for this module. Players choose one in the shipyard.")
	imgui.SameLine()
	if imgui.SmallButton("Module...") {
		imgui.OpenPopup("room-menu-" + slot)
	}
	tooltip("Rename this module, change its shape, show the bare hull, or delete it.")
	imgui.UnindentV(14 * s)
}

// beginAddOption opens the option form for a room, showing an option first
// because the form copies the displayed option.
func (ws *WsShip) beginAddOption(slot string) {
	if ws.editingSlot() != slot {
		ws.editRoom(slot)
	}
	if ws.editingSlot() != slot {
		return
	}
	ws.beginTask(taskModule)
}

func (ws *WsShip) roomMenu(slot string, options int) {
	name := ship.SlotDisplayName(slot)
	if imgui.Selectable("Rename module...") {
		ws.beginRename(taskRenameRoom, slot, name)
	}
	if imgui.Selectable("Change shape...") {
		ws.reshapeSlot = slot
		ws.beginTask(taskReshape)
	}
	if ws.selected[slot] != "" {
		if imgui.Selectable("Show none (hull only)") {
			ws.selectRoomOption(slot, "")
		}
		tooltip("Hide the option so the bare hull tiles show. Editing switches to the hull.")
	} else if id := ws.displayedOption(slot); id != "" && imgui.Selectable("Show default option") {
		ws.selectRoomOption(slot, id)
	}
	imgui.Separator()
	key := "slot/" + slot
	if ws.pendingDelete == key {
		ws.pendingShown = true
		imgui.TextColored(style.Amber, fmt.Sprintf("Moves the default option's furniture back into the hull and deletes %d options.", options))
		if imgui.Selectable("Delete " + name) {
			ws.pendingDelete = ""
			ws.deleteRoom(slot)
		}
		if imgui.Selectable("Keep it") {
			ws.pendingDelete = ""
		}
	} else if imgui.SelectableV("Delete module...", false, imgui.SelectableFlagsDontClosePopups, imgui.Vec2{}) {
		ws.pendingDelete, ws.pendingShown = key, true
	}
}

func (ws *WsShip) optionMenu(slot string, m ship.Module, options int) {
	if imgui.Selectable("Rename & describe") {
		ws.beginRename(taskRenameModule, m.ID, m.Name)
	}
	if imgui.Selectable("Set price") {
		ws.beginCosts("module/" + m.ID)
	}
	if imgui.Selectable("Crew") {
		ws.beginCrew()
		ws.loadCrewScope("module/" + m.ID)
	}
	imgui.BeginDisabledV(m.Default)
	if imgui.Selectable("Make default") {
		ws.change("Set default option", func() error { return ws.project.SetDefaultModule(slot, m.ID) })
	}
	imgui.EndDisabled()
	if m.Default {
		tooltip("This is already the default option.")
	}
	imgui.Separator()
	blocked := ""
	switch {
	case options <= 1:
		blocked = "This is the module's only option. Delete the module instead."
	case m.Default:
		blocked = "Make another option the default first."
	}
	key := "module/" + m.ID
	imgui.BeginDisabledV(blocked != "")
	if ws.pendingDelete == key {
		ws.pendingShown = true
		imgui.TextColored(style.Amber, "Deletes this option with its maps, crew and price.")
		if imgui.Selectable("Delete " + m.Name) {
			ws.pendingDelete = ""
			ws.deleteOption(slot, m.ID)
		}
		if imgui.Selectable("Keep it") {
			ws.pendingDelete = ""
		}
	} else if imgui.SelectableV("Delete option...", false, imgui.SelectableFlagsDontClosePopups, imgui.Vec2{}) {
		ws.pendingDelete, ws.pendingShown = key, true
	}
	imgui.EndDisabled()
	if blocked != "" {
		tooltip(blocked)
	}
}

func (ws *WsShip) deleteRoom(slot string) {
	ws.change("Delete module", func() error { return ws.project.RemoveSlot(ws.theme, slot) })
	if ws.message == "" {
		ws.defaults()
		ws.rebuild()
	}
}

func (ws *WsShip) deleteOption(slot, id string) {
	ws.change("Delete module option", func() error { return ws.project.RemoveModule(id) })
	if ws.message == "" {
		if ws.selected[slot] == id {
			ws.selected[slot] = ws.displayedOption(slot)
		}
		ws.source = 0
		ws.rebuild()
	}
}

// checksSummary surfaces assembly warnings while building, not only in Review.
func (ws *WsShip) checksSummary() {
	if ws.assembly == nil || len(ws.assembly.Issues) == 0 {
		return
	}
	n := len(ws.assembly.Issues)
	label := fmt.Sprintf("%d things to check", n)
	if n == 1 {
		label = "1 thing to check"
	}
	imgui.TextColored(style.Amber, label)
	var lines []string
	for _, issue := range ws.assembly.Issues {
		lines = append(lines, "- "+issue.Message)
	}
	tooltip(strings.Join(lines, "\n"))
}

// trackHoveredRoom remembers the inactive room under the cursor. The value
// stays while the mouse travels to the header button, and clears once the
// cursor is back on the canvas over anything else.
func (ws *WsShip) trackHoveredRoom() {
	if ws.pane == nil || ws.assembly == nil || ws.source >= len(ws.assembly.Sources) || ws.hoverRoom == ws.editingSlot() {
		ws.hoverRoom = ""
	}
	if ws.pane == nil || ws.assembly == nil || !ws.pane.CanvasControl().Active() {
		return
	}
	slot, ok := ws.hoveredRoom()
	if !ok || slot == ws.editingSlot() {
		ws.hoverRoom = ""
		return
	}
	ws.hoverRoom = slot
	if tools.IsSelected(tools.TNGrab) && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
		ws.editRoom(slot)
	}
}

// hoveredRoom maps the cursor tile of the edited part to a hull room.
func (ws *WsShip) hoveredRoom() (string, bool) {
	tile := ws.pane.CanvasState().HoveredTile()
	if tile.Z < 1 {
		return "", false
	}
	offset := ws.assembly.Sources[ws.source].Offset
	return ws.assembly.RoomAt(util.Point{X: tile.X + offset.X, Y: tile.Y + offset.Y, Z: tile.Z})
}

// paintRooms outlines every room on the canvas; the edited one reads
// stronger, the hovered one brighter, and the room-shape tool's tiles are
// outlined while a room is being made or reshaped.
func (ws *WsShip) paintRooms(o pmap.OverlayPainter) {
	if ws.assembly == nil || ws.source >= len(ws.assembly.Sources) {
		return
	}
	editing := ws.editingSlot()
	shift := util.Point{}
	if ws.isolated {
		shift = ws.assembly.Sources[ws.source].Offset
	}
	view := func(p util.Point) util.Point { return util.Point{X: p.X - shift.X, Y: p.Y - shift.Y, Z: p.Z} }
	for slot, room := range ws.assembly.Rooms {
		if ws.isolated && slot != editing {
			continue
		}
		fill, border := overlay.ColorRoomOtherFill, overlay.ColorRoomOtherBorder
		switch slot {
		case editing:
			fill, border = overlay.ColorRoomActiveFill, overlay.ColorRoomActiveBorder
		case ws.hoverRoom:
			fill, border = overlay.ColorRoomHoverFill, overlay.ColorRoomHoverBorder
		}
		tiles := map[util.Point]bool{}
		for _, tile := range roomTiles(room) {
			tiles[tile] = true
			o.Tile(view(tile), fill)
		}
		for tile, sides := range util.OuterSides(tiles) {
			o.Edges(view(tile), sides, border)
		}
	}
	if ws.usesShapeTool() && ws.source == 0 {
		tiles := map[util.Point]bool{}
		for _, tile := range tools.RoomShapeTiles() {
			tiles[tile] = true
		}
		for tile, sides := range util.OuterSides(tiles) {
			o.Edges(tile, sides, overlay.ColorRoomShapeBorder)
		}
	}
}
