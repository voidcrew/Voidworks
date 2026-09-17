package wsship

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext/icon"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/ship"
	"sdmm/internal/shippreview"
)

const (
	stepChoose = iota
	stepBuild
	stepReview
)

type reviewProject struct {
	name, id, error string
	files           []reviewFile
}
type reviewFile struct {
	path    string
	existed bool
}

func (ws *WsShip) prepareReview() {
	ws.reviewed = nil
	for _, h := range ws.catalog.Hulls {
		p := ws.projects[h.Type]
		if p == nil {
			continue
		}
		changes, err := p.Changes()
		item := reviewProject{name: p.Hull.Name, id: h.Type}
		if err != nil {
			item.error = err.Error()
		}
		for _, c := range changes {
			rel, _ := filepath.Rel(ws.catalog.Root, c.Path)
			item.files = append(item.files, reviewFile{filepath.ToSlash(rel), c.Existed})
		}
		if len(item.files) > 0 || err != nil {
			ws.reviewed = append(ws.reviewed, item)
		}
	}
	ws.reviewReady = true
}

func space() { workshop.Gap() }

func heading(text string) { workshop.Section(text, style.Teal) }

func title(text string) { workshop.Title(text) }

func hint(text string) { workshop.Muted(text) }

func tooltip(text string) { workshop.Tooltip(text) }

func textField(label, placeholder string, value *string) bool {
	imgui.Text(label)
	imgui.PushItemWidth(-1)
	changed := imgui.InputTextWithHint("##"+label, placeholder, value)
	imgui.PopItemWidth()
	return changed
}
func numberField(label string, value *int32) {
	imgui.Text(label)
	imgui.PushItemWidth(-1)
	imgui.InputInt("##"+label, value)
	imgui.PopItemWidth()
}
func combo(label, preview string) bool {
	return comboHelp(label, preview, "")
}
func comboHelp(label, preview, help string) bool {
	imgui.Text(label)
	if help != "" {
		tooltip(help)
	}
	// An open combo switches to the popup window. Set the next item's width
	// without leaving a width-stack entry to pop from the wrong window.
	imgui.SetNextItemWidth(-1)
	open := imgui.BeginCombo("##"+label, preview)
	if help != "" {
		if open {
			hint(help)
			imgui.Separator()
		} else {
			tooltip(help)
		}
	}
	return open
}
func actionButton(label string, primary bool) bool { return workshop.Button(label, primary) }

func (ws *WsShip) setStage(stage int) {
	if !ws.commitDraft() {
		return
	}
	ws.flush()
	ws.OnFocusChange(false)
	ws.stage, ws.wizard = stage, false
	ws.endShape()
	ws.task = taskPaint
	if tools.IsSelected(tools.TNRegion) || tools.IsSelected(tools.TNRoomShape) {
		tools.SetSelected(tools.TNAdd)
	}
	if stage == stepReview {
		ws.rebuild()
	}
	ws.OnFocusChange(true)
}

func (ws *WsShip) Process() {
	workshop.PushStyle()
	defer workshop.PopStyle()
	if ws.recoveryPrompt() {
		return
	}
	scale := window.PointSize()
	context := "Fleet library"
	if ws.project != nil && ws.stage != stepChoose {
		context = ws.project.Hull.Name
		if t := ws.currentTheme(); t.Name != "" {
			context += " - " + t.Name
		}
	}
	if ws.wizard {
		context = "New ship / Create a starting canvas"
	}
	workshop.Banner("Ship Workshop", context, style.Teal)
	available := imgui.ContentRegionAvail().X
	rail := min(276*scale, max(210*scale, available*.23))
	workshop.Panel("ship-controls", imgui.Vec2{X: rail}, false)
	ws.controls()
	workshop.EndPanel()
	imgui.SameLine()
	flags := imgui.WindowFlagsAlwaysUseWindowPadding
	if ws.stage == stepBuild && !ws.wizard {
		flags |= imgui.WindowFlagsNoScrollbar | imgui.WindowFlagsNoScrollWithMouse
	}
	imgui.BeginChildV("ship-content", imgui.Vec2{}, false, flags)
	switch {
	case ws.wizard:
		ws.newShip()
	case ws.stage == stepChoose:
		ws.chooseShip()
	case ws.stage == stepReview:
		ws.review()
	case ws.task == taskCrew:
		ws.crewContent()
	case ws.task == taskCosts:
		ws.costsContent()
	case ws.pane != nil && !ws.invalid:
		ws.canvasHeader()
		ws.pane.Process()
	default:
		imgui.TextWrapped(ws.message)
	}
	imgui.EndChild()
}

func (ws *WsShip) controls() {
	imgui.TextColored(style.Muted, "WORKSPACE")
	labels := []string{"Choose a ship", "Build", "Review & save"}
	details := []string{"Your fleet & new ships", "Map, rooms & crew", "Checks & project changes"}
	for i, label := range labels {
		if ws.stage != stepChoose {
			details[i] = ""
		}
		imgui.BeginDisabledV(i != stepChoose && (ws.project == nil || ws.wizard))
		if workshop.Row(fmt.Sprintf("stage-%d", i), label, details[i], fmt.Sprintf("0%d", i+1), ws.stage == i, style.Teal, 0, i != stepChoose && (ws.project == nil || ws.wizard)) {
			ws.setStage(i)
		}
		imgui.EndDisabled()
	}
	if ws.catalog == nil {
		imgui.TextWrapped(ws.message)
		return
	}
	if ws.wizard {
		heading("NEW SHIP")
		hint("Name your ship, then choose a starting canvas.")
		return
	}
	switch ws.stage {
	case stepChoose:
		heading("FLEET OVERVIEW")
		imgui.PushFont(window.FontH1)
		imgui.Text(fmt.Sprintf("%02d", len(ws.catalog.Hulls)))
		imgui.PopFont()
		hint("Ships in this project")
		space()
		hint("Open a ship to build its hull, configure rooms, or equip its crew.")
	case stepReview:
		heading("SAVE YOUR WORK")
		hint("Check your changes, then save them to the project.")
		space()
		if actionButton("Back to building", false) {
			ws.setStage(stepBuild)
		}
	default:
		ws.buildControls()
	}
	if ws.message != "" && ws.stage != stepReview {
		space()
		imgui.Separator()
		imgui.TextWrapped(ws.message)
	}
	ws.removalRecovery()
	ws.recoveryStatus()
	if app, ok := ws.app.(interface{ ShipPreviewStatus() shippreview.Status }); ok {
		resume := ws.notifySaved
		var stop func()
		if controls, ok := ws.app.(interface {
			ResumeShipPreviews()
			StopShipPreviews()
		}); ok {
			resume, stop = controls.ResumeShipPreviews, controls.StopShipPreviews
		}
		workshop.PreviewStatus(app.ShipPreviewStatus(), resume, stop)
	}
}

func (ws *WsShip) chooseShip() {
	imgui.PushFont(window.FontH1)
	imgui.TextWrapped("Your fleet")
	imgui.PopFont()
	hint("Choose a ship. Make it your own.")
	space()
	if workshop.Row("create-ship", icon.Add+"  Create a new ship", "Start with a blank hull and build from there.", "+", false, style.Teal, 0) {
		ws.BeginNewShip()
	}
	heading("SHIP LIBRARY")
	textField("Find a ship", "Search the fleet...", &ws.shipFilter)
	if ws.catalog == nil {
		return
	}
	modular, fixed := 0, 0
	for _, h := range ws.catalog.Hulls {
		if h.Fixed {
			fixed++
		} else {
			modular++
		}
	}
	imgui.Text("Show")
	imgui.SameLine()
	for kind, label := range []string{fmt.Sprintf("All (%d)", modular+fixed), fmt.Sprintf("Modular (%d)", modular), fmt.Sprintf("Fixed layout (%d)", fixed)} {
		if kind > 0 {
			imgui.SameLine()
		}
		if imgui.RadioButton(label, ws.shipKind == kind) {
			ws.shipKind = kind
		}
		switch kind {
		case 1:
			tooltip("Ships with upgrade slots: they are sold in the shipyard and can start rounds.")
		case 2:
			tooltip("Ships without upgrade slots. Make an upgrade room on one to turn it modular.")
		}
	}
	hint("Right-click a ship for removal.")
	space()
	imgui.BeginChild("ship-list")
	count := 0
	for i, h := range ws.catalog.Hulls {
		if !strings.Contains(strings.ToLower(h.Name), strings.ToLower(strings.TrimSpace(ws.shipFilter))) {
			continue
		}
		if (ws.shipKind == 1 && h.Fixed) || (ws.shipKind == 2 && !h.Fixed) {
			continue
		}
		count++
		detail := fmt.Sprintf("%d room options   /   %d variants", len(h.Modules), len(h.Themes))
		if h.Fixed {
			detail = "Fixed layout   /   not modular yet"
		}
		if workshop.Row(h.Type, h.Name, detail, "Open  >", ws.project != nil && i == ws.hull, style.Teal, 0) {
			ws.flush()
			ws.hull, ws.theme = i, 0
			ws.isolated = false
			ws.defaults()
			ws.rebuild()
			ws.setStage(stepBuild)
			if ws.pane != nil {
				ws.pane.FitView()
			}
		}
		if imgui.BeginPopupContextItemV("ship-actions-"+h.Type, 1) {
			if imgui.Selectable("Remove ship...") {
				ws.requestRemoval(h)
			}
			imgui.EndPopup()
		}
	}
	if count == 0 {
		title("No ships found")
		hint("Try another name, or show all ships to see the whole fleet.")
		if imgui.Button("Show all ships") {
			ws.shipFilter = ""
			ws.shipKind = 0
		}
	}
	imgui.EndChild()
}

func (ws *WsShip) buildControls() {
	if ws.project == nil {
		return
	}
	if ws.task == taskCrew {
		ws.crewControls()
		return
	}
	if ws.task == taskCosts {
		ws.costsControls()
		return
	}
	if ws.task != taskPaint {
		ws.authorControls()
		return
	}
	// A confirmation that is no longer on screen was abandoned. Rooms and
	// variants share the pending key, so both panels draw before it expires.
	ws.pendingShown = false
	ws.roomsControls()
	space()
	ws.variantsControls()
	if !ws.pendingShown {
		ws.pendingDelete = ""
	}
	if ws.assembly != nil && ws.source < len(ws.assembly.Sources) {
		if d := ws.project.Documents[ws.assembly.Sources[ws.source].File]; d != nil && len(d.Unknown) > 0 {
			hint(fmt.Sprintf("%d types on this map are not in the loaded environment. They show as placeholders and are kept on save.", len(d.Unknown)))
			tooltip(strings.Join(d.Unknown, "\n"))
		}
	}
	space()
	workshop.Section("SHIP SYSTEMS", style.Violet)
	areaAction := "Ship areas"
	if workshop.Row("open-crew", "Crew & equipment", "", ">", false, style.Violet, 0) {
		ws.beginCrew()
	}
	if _, _, selected := tools.SelectionBounds(); selected {
		areaAction = "Make or assign an area..."
	}
	if workshop.Row("open-areas", areaAction, "", ">", false, style.Teal, 0) {
		ws.beginTask(taskArea)
	}
	if workshop.Row("open-docking", "Docking port", "", ">", false, style.Amber, 0) {
		ws.beginTask(taskDocking)
	}
	tooltip("Select an entrance with Grab (3), then place or move this ship's mobile docking port there.")
	space()
	heading("CONFIGURATION")
	if workshop.Row("open-costs", "Ship & upgrade prices", "Base ship, variants & room options", ">", false, style.Amber, 0) {
		ws.beginCosts("ship")
	}
	if workshop.DangerButton("Remove ship...") {
		ws.requestRemoval(ws.project.Hull)
	}
	if ws.project.Settings != nil && imgui.CollapsingHeader("Ship details & canvas size") {
		if actionButton("Edit ship details...", false) {
			ws.beginTask(taskSettings)
		}
		if actionButton("Change canvas size...", false) {
			ws.beginTask(taskResize)
		}
	}
	if imgui.CollapsingHeader("Advanced view & source files") {
		if imgui.Checkbox("Show only the part being edited", &ws.isolated) {
			ws.flush()
			ws.rebuild()
			if ws.pane != nil {
				ws.pane.FitView()
			}
		}
		if ws.assembly != nil && ws.source < len(ws.assembly.Sources) {
			s := ws.assembly.Sources[ws.source]
			rel, _ := filepath.Rel(ws.catalog.Root, s.File)
			hint(filepath.ToSlash(rel))
			hint(fmt.Sprintf("%d x %d tiles; placed at %d, %d", s.Data.MaxX, s.Data.MaxY, s.Offset.X, s.Offset.Y))
		}
	}
	space()
	if actionButton("Continue to review & save", true) {
		ws.setStage(stepReview)
	}
}

func (ws *WsShip) canvasHeader() {
	if imgui.Button("Show whole ship") {
		if ws.isolated {
			ws.flush()
			ws.isolated = false
			ws.rebuild()
		}
		ws.pane.FitView()
	}
	tooltip("Centers the ship and adjusts the zoom so it fits on screen.")
	imgui.SameLine()
	areas := ws.app.PathsFilter().IsVisiblePath("/area")
	if imgui.Checkbox("Areas", &areas) {
		ws.app.PathsFilter().TogglePath("/area")
	}
	tooltip("Show area markers. This is the same setting as View > Areas (Ctrl+1).")
	imgui.SameLine()
	ws.trackHoveredRoom()
	grabTask := tools.IsSelected(tools.TNGrab) && (ws.task == taskArea || ws.task == taskDocking)
	switch {
	case ws.usesShapeTool():
		if tools.IsSelected(tools.TNRoomShape) {
			imgui.Text("Select the room's tiles on the hull")
		} else {
			ws.editingText()
		}
	case grabTask:
		imgui.Text("Select tiles in the part being edited")
	case ws.hoverRoom != "" && ws.task == taskPaint:
		name := ship.SlotDisplayName(ws.hoverRoom)
		if ws.source == 0 {
			imgui.Text(name + " - this room's items live in its option, not the hull.")
		} else {
			imgui.Text(name + " - a different room.")
		}
		imgui.SameLine()
		if imgui.SmallButton("Edit " + name) {
			ws.editRoom(ws.hoverRoom)
		}
	case ws.assembly != nil && ws.source < len(ws.assembly.Sources):
		ws.editingText()
	}
	if ws.usesShapeTool() && tools.IsSelected(tools.TNRoomShape) {
		hint("Click or drag to add tiles  |  Click a tile again to remove it  |  Shift+drag adds a box  |  Alt+drag removes a box")
	} else if grabTask {
		hint("Grab selection (3) is used by the action in the left panel.")
	} else {
		instruction := "Scroll to zoom  |  Middle mouse to pan  |  Ctrl+Z to undo"
		if p, ok := ws.app.SelectedPrefab(); ok && tools.IsSelected(tools.TNAdd) {
			name, err := strconv.Unquote(p.Vars().ValueV("name", ""))
			if err != nil || name == "" {
				name = filepath.Base(p.Path())
			}
			instruction = "Placing: " + name + "  |  " + instruction
		}
		hint(instruction)
	}
	imgui.Separator()
}

// editingText names the part being edited, prefixed with its variant and
// marked when the room's map is shared with the ship's other variants.
func (ws *WsShip) editingText() {
	label := "Editing: " + ws.editingLabel()
	if t := ws.currentTheme(); t.Name != "" {
		label = t.Name + "  ·  " + label
	}
	suffix, about := ws.editingShare()
	if suffix != "" {
		label += "  " + suffix
	}
	imgui.Text(label)
	if about != "" {
		tooltip(about)
	}
}

// editingShare reports whether the edited option uses the shared room or this
// variant's own copy. The answer compares maps, so it is kept per rebuild.
func (ws *WsShip) editingShare() (string, string) {
	if ws.share.ready {
		return ws.share.suffix, ws.share.about
	}
	ws.share.ready = true
	slot := ws.editingSlot()
	if ws.project == nil || slot == "" || len(ws.project.Hull.Themes) < 2 {
		return "", ""
	}
	m, ok := ws.module(ws.selected[slot])
	if !ok {
		return "", ""
	}
	forked, _, err := ws.project.ModuleThemeStatus(m.ID, ws.currentTheme().ID)
	if err != nil {
		return "", ""
	}
	switch {
	case forked:
		ws.share.suffix, ws.share.about = "(this variant)", "Only this variant uses this room."
	case len(m.Themes) > 1:
		ws.share.suffix, ws.share.about = "(shared)", "Changes here affect every variant using this room."
	}
	return ws.share.suffix, ws.share.about
}

// editingLabel names the part being edited the way the shipyard does.
func (ws *WsShip) editingLabel() string {
	if ws.assembly == nil || ws.source >= len(ws.assembly.Sources) {
		return ""
	}
	s := ws.assembly.Sources[ws.source]
	if s.Slot == "" {
		return s.Name
	}
	return ship.SlotDisplayName(s.Slot) + " - " + s.Name + " option"
}

func (ws *WsShip) selectRoomOption(slot, id string) {
	ws.flush()
	ws.selected[slot] = id
	ws.source = 0
	ws.rebuild()
	if id == "" || ws.invalid || ws.assembly == nil {
		return
	}
	for i, source := range ws.assembly.Sources {
		if source.Slot == slot {
			ws.source = i
			ws.rebuild()
			return
		}
	}
}

func (ws *WsShip) review() {
	if ws.project == nil {
		return
	}
	title("Review & save")
	imgui.TextWrapped(ws.project.Hull.Name)
	hint("You can save a draft at any point and keep building later.")
	space()
	if actionButton("Save all changes", true) {
		ws.Save()
	}
	if ws.message != "" {
		imgui.TextWrapped(ws.message)
	}
	heading("MAP CHECKS")
	if ws.invalid || ws.assembly == nil {
		imgui.TextWrapped("The ship could not be assembled. Return to Build to resolve the reported problem.")
	} else if len(ws.assembly.Issues) == 0 {
		imgui.Text("No assembly warnings for this combination.")
	} else {
		imgui.Text(fmt.Sprintf("%d things to check", len(ws.assembly.Issues)))
		for _, issue := range ws.assembly.Issues {
			imgui.BulletText(issue.Message)
		}
	}
	heading("CHANGES TO SAVE")
	if !ws.reviewReady {
		ws.prepareReview()
	}
	for _, item := range ws.reviewed {
		if item.error != "" {
			imgui.TextWrapped(item.name + ": " + item.error)
			continue
		}
		imgui.Text(fmt.Sprintf("%s - %d files", item.name, len(item.files)))
		if imgui.TreeNode("Show files##" + item.id) {
			for _, c := range item.files {
				prefix := "Update "
				if !c.existed {
					prefix = "Create "
				}
				hint(prefix + c.path)
			}
			imgui.TreePop()
		}
	}
	if len(ws.reviewed) == 0 {
		hint("All changes are saved.")
	}
	space()
	hint("Purchase previews regenerate after saving. Before using a new ship in-game, compile the project and playtest.")
}
