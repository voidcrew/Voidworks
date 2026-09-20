package wsship

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

type buildTask int

const (
	taskPaint buildTask = iota
	taskArea
	taskRoom
	taskTheme
	taskModule
	taskResize
	taskSettings
	taskDocking
	taskCrew
	taskCosts
	taskRenameTheme
	taskRenameModule
	taskReshape
	taskRenameRoom
	taskRenameShip
	taskRenameShipFiles
)

// usesShapeTool reports the tasks that collect room tiles on the hull canvas.
func (ws *WsShip) usesShapeTool() bool { return ws.task == taskRoom || ws.task == taskReshape }

type settingsForm struct {
	name, description string
	crew              int32
	hidden            bool
	costs             ship.PartCosts
}

// IDs are a storage detail. Names remain unrestricted; generated IDs are valid,
// short, and unique among the existing project entries.
func suggestedID(name string, used func(string) bool) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if separator && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	base := b.String()
	if base == "" {
		base = "ship"
	}
	if base[0] < 'a' || base[0] > 'z' {
		base = "ship_" + base
	}
	if len(base) > 40 {
		base = strings.TrimRight(base[:40], "_")
	}
	id := base
	for n := 2; used(id); n++ {
		id = base + "_" + strconv.Itoa(n)
	}
	return id
}
func (ws *WsShip) shipIDUsed(id string) bool {
	typePath := ship.HullType + "/" + id
	if ws.app.LoadedEnvironment().Objects[typePath] != nil {
		return true
	}
	for _, h := range ws.catalog.Hulls {
		if h.Type == typePath {
			return true
		}
	}
	return false
}
func (ws *WsShip) itemIDUsed(id string) bool {
	h := ws.project.Hull
	for _, t := range h.Themes {
		if t.ID == id || ship.Contains(t.Slots, id) {
			return true
		}
	}
	for _, slot := range h.Slots {
		if slot == id {
			return true
		}
	}
	for _, m := range h.Modules {
		if m.ID == id || m.ID == id+"_basic" {
			return true
		}
	}
	return false
}

func (ws *WsShip) BeginNewShip() {
	if !ws.commitDraft() {
		return
	}
	if ws.catalog == nil {
		return
	}
	ws.flush()
	ws.OnFocusChange(false)
	ws.stage, ws.wizard, ws.wizardStep = stepChoose, true, 0
	ws.newID, ws.newName, ws.message = "", "", ""
	ws.customID = false
	ws.sizePreset, ws.width, ws.height = 1, 32, 32
	tools.SetEnabled(false)
}
func (ws *WsShip) newShip() {
	// Keep the setup form readable even when the canvas is very wide.
	available := imgui.ContentRegionAvail().X
	formWidth := min(520*window.PointSize(), available)
	pos := imgui.CursorPos()
	pos.X += max(0, (available-formWidth)/2)
	imgui.SetCursorPos(pos)
	imgui.BeginChildV("new-ship-form", imgui.Vec2{X: formWidth}, false, 0)
	title("Create a new ship")
	space()
	if !ws.customID {
		ws.newID = suggestedID(ws.newName, ws.shipIDUsed)
	}
	if ws.wizardStep == 0 {
		imgui.Text("1 of 2  /  Name your ship")
		hint("Choose the name mappers and players will see.")
		space()
		if textField("Ship name", "e.g. Wayfarer", &ws.newName) && !ws.customID {
			ws.newID = suggestedID(ws.newName, ws.shipIDUsed)
		}
		space()
		if imgui.CollapsingHeader("Advanced") {
			if textField("File identifier", "lowercase_letters", &ws.newID) {
				ws.customID = true
			}
			hint("Used for generated file names. The automatic value is usually all you need.")
		}
		nameErr := ship.ShipNameError(ws.catalog, ws.app.LoadedEnvironment(), ws.newName, "")
		valid := nameErr == nil && ship.ValidID(ws.newID) == nil && !ws.shipIDUsed(ws.newID)
		if strings.TrimSpace(ws.newName) != "" && nameErr != nil {
			hint(nameErr.Error())
		}
		if ws.customID && !valid {
			hint("Use a unique lowercase identifier starting with a letter.")
		}
		space()
		imgui.BeginDisabledV(!valid)
		if actionButton("Next: choose a canvas", true) {
			ws.wizardStep = 1
			ws.message = ""
		}
		imgui.EndDisabled()
	} else {
		imgui.Text("2 of 2  /  Choose a starting canvas")
		imgui.TextWrapped(ws.newName)
		hint("This is the space available to build in. You can resize it later.")
		space()
		for i, size := range []int32{24, 32, 48} {
			name := []string{"Small", "Medium", "Large"}[i]
			if actionButton(fmt.Sprintf("%s  -  %d x %d tiles", name, size, size), ws.sizePreset == i) {
				ws.sizePreset, ws.width, ws.height = i, size, size
			}
		}
		if actionButton("Custom size...", ws.sizePreset == 3) {
			ws.sizePreset = 3
		}
		if ws.sizePreset == 3 {
			numberField("Width in tiles", &ws.width)
			numberField("Height in tiles", &ws.height)
		}
		space()
		valid := ws.width >= 5 && ws.height >= 5 && ws.width <= 128 && ws.height <= 128
		if !valid {
			hint("Choose a width and height between 5 and 128 tiles.")
		}
		space()
		imgui.BeginDisabledV(!valid)
		if actionButton("Create ship & start building", true) {
			ws.createShip()
		}
		imgui.EndDisabled()
		if actionButton("Back to name", false) {
			ws.wizardStep = 0
		}
	}
	if actionButton("Cancel", false) {
		ws.setStage(stepChoose)
	}
	if ws.message != "" {
		imgui.TextWrapped(ws.message)
	}
	imgui.EndChild()
}

func (ws *WsShip) createShip() {
	p, err := ship.NewProject(ws.catalog, ws.app.LoadedEnvironment(), ws.newID, strings.TrimSpace(ws.newName), int(ws.width), int(ws.height))
	if err != nil {
		ws.message = err.Error()
		return
	}
	ws.projects[p.Hull.Type] = p
	ws.catalog.Hulls = append(ws.catalog.Hulls, p.Hull)
	ws.hull, ws.theme = len(ws.catalog.Hulls)-1, 0
	ws.wizard, ws.isolated, ws.stage, ws.task = false, false, stepBuild, taskPaint
	ws.defaults()
	ws.rebuild()
	ws.OnFocusChange(true)
	if ws.pane != nil {
		ws.pane.FitView()
	}
	tools.SetSelected(tools.TNAdd)
}

func (ws *WsShip) beginTask(task buildTask) {
	ws.flush()
	ws.task, ws.itemName, ws.itemID, ws.message = task, "", "", ""
	ws.itemDescription = ""
	ws.customID, ws.emptyModule = false, false
	if task == taskTheme || task == taskModule {
		ws.itemCosts = ship.PartCosts{}
		ws.copyRooms = false
	}
	if task == taskRoom || task == taskDocking || task == taskReshape {
		lo, hi, selected := tools.SelectionBounds()
		if selected && ws.assembly != nil && ws.source < len(ws.assembly.Sources) {
			offset := ws.assembly.Sources[ws.source].Offset
			lo.X, lo.Y = lo.X+offset.X, lo.Y+offset.Y
			hi.X, hi.Y = hi.X+offset.X, hi.Y+offset.Y
		}
		ws.source, ws.isolated = 0, false
		ws.rebuild()
		if task == taskDocking {
			if selected {
				tools.SetGrabSelection(lo, hi)
			}
			ws.dockOutward = 0
		} else {
			ws.beginShape(task, selected, lo, hi)
		}
	}
	if task == taskArea {
		ws.areaPath, ws.areaIcon = "", "station"
		if !ws.app.PathsFilter().IsVisiblePath("/area") {
			ws.app.PathsFilter().TogglePath("/area")
		}
	}
	if task == taskSettings {
		s := ws.project.ShipDetails()
		costs, err := ws.project.PartCosts("ship")
		if err != nil {
			ws.message = err.Error()
		}
		ws.settings = settingsForm{name: ws.project.Hull.Name, description: s.Description, crew: int32(s.Crew), hidden: s.Hidden, costs: costs}
	}
	if task == taskResize && ws.assembly != nil {
		s := ws.assembly.Sources[0]
		ws.width, ws.height = int32(s.Data.MaxX), int32(s.Data.MaxY)
	}
}

func (ws *WsShip) finishTask() {
	if !ws.commitDraft() {
		return
	}
	ws.endShape()
	ws.task = taskPaint
}

// beginShape arms the room-shape tool on the hull. A Grab rectangle carried
// into the task becomes the first tiles; reshaping starts from the room's
// current footprint.
func (ws *WsShip) beginShape(task buildTask, selected bool, lo, hi util.Point) {
	tools.SetRoomShapeAccept(ws.acceptShapeTile)
	tools.ClearRoomShape()
	tools.SetSelected(tools.TNRoomShape)
	if task == taskReshape {
		if ws.assembly != nil {
			if room, ok := ws.assembly.Rooms[ws.reshapeSlot]; ok {
				tools.SetRoomShapeTiles(roomTiles(room))
			}
		}
		return
	}
	if selected {
		tools.AddRoomShapeRect(lo, hi)
	}
}

// roomTiles lists a room's footprint on the hull, without its marker.
func roomTiles(room ship.RoomShape) []util.Point {
	var tiles []util.Point
	for cell, in := range room.Footprint.Cells {
		if in {
			tiles = append(tiles, util.Point{X: room.Origin.X + cell.X - 1, Y: room.Origin.Y + cell.Y - 1, Z: 1})
		}
	}
	return tiles
}

func (ws *WsShip) endShape() {
	if !ws.usesShapeTool() {
		return
	}
	tools.ClearRoomShape()
	tools.SetRoomShapeAccept(nil)
	if tools.IsSelected(tools.TNRoomShape) {
		tools.SetSelected(tools.TNAdd)
	}
}

// acceptShapeTile keeps room tiles on the hull and out of other rooms.
func (ws *WsShip) acceptShapeTile(p util.Point) (bool, string) {
	if ws.assembly == nil || len(ws.assembly.Sources) == 0 || !ws.assembly.Sources[0].Live.HasTile(p) {
		return false, "That tile is outside the hull."
	}
	if slot, ok := ws.assembly.RoomAt(p); ok && !(ws.task == taskReshape && slot == ws.reshapeSlot) {
		return false, "That tile already belongs to " + ship.SlotDisplayName(slot) + "."
	}
	return true, ""
}

// importGrabSelection adds a Grab rectangle drawn on the part being edited.
func (ws *WsShip) importGrabSelection() {
	lo, hi, ok := tools.SelectionBounds()
	if !ok || ws.assembly == nil || ws.source >= len(ws.assembly.Sources) {
		return
	}
	offset := ws.assembly.Sources[ws.source].Offset
	lo.X, lo.Y = lo.X+offset.X, lo.Y+offset.Y
	hi.X, hi.Y = hi.X+offset.X, hi.Y+offset.Y
	if ws.source != 0 {
		ws.flush()
		ws.source = 0
		ws.rebuild()
	}
	tools.SetSelected(tools.TNRoomShape)
	tools.AddRoomShapeRect(lo, hi)
}

func (ws *WsShip) authorControls() {
	if actionButton("< Back to ship", false) {
		ws.finishTask()
		return
	}
	switch ws.task {
	case taskArea:
		ws.areaControls()
	case taskRoom, taskReshape:
		ws.regionControls()
	case taskDocking:
		ws.dockingControls()
	case taskTheme, taskModule:
		ws.copyControls()
	case taskRenameTheme, taskRenameModule, taskRenameRoom, taskRenameShip, taskRenameShipFiles:
		ws.renameControls()
	case taskSettings:
		ws.settingsControls()
	case taskResize:
		heading("CHANGE CANVAS SIZE")
		hint("Add module to build. Shrinking is allowed only where the canvas is empty.")
		numberField("Width in tiles", &ws.width)
		numberField("Height in tiles", &ws.height)
		if combo("Add/remove space at", ws.resizeDirection.String()) {
			for direction := dmmap.ResizeNorthEast; direction <= dmmap.ResizeSouthWest; direction++ {
				if imgui.SelectableV(direction.String(), direction == ws.resizeDirection, 0, imgui.Vec2{}) {
					ws.resizeDirection = direction
				}
			}
			imgui.EndCombo()
		}
		valid := ws.width >= 5 && ws.height >= 5 && ws.width <= 128 && ws.height <= 128
		if !valid {
			hint("Use 5 to 128 tiles in each direction.")
		}
		imgui.BeginDisabledV(!valid)
		if actionButton("Apply canvas size", true) {
			ws.change("Resize hull", func() error {
				return ws.project.ResizeToward(ws.currentTheme(), int(ws.width), int(ws.height), ws.resizeDirection)
			})
			if ws.message == "" {
				ws.finishTask()
				ws.pane.FitView()
			}
		}
		imgui.EndDisabled()
	}
}

func (ws *WsShip) regionControls() {
	if ws.task == taskReshape {
		heading("CHANGE MODULE SHAPE")
		hint(ship.SlotDisplayName(ws.reshapeSlot) + ": tiles leaving the module must be empty in every option.")
	} else {
		heading("MAKE AN UPGRADE MODULE")
		hint("Turn a furnished module into a swappable upgrade. Its walls and floor stay in the hull.")
	}
	if ws.project != nil && ws.project.Hull.Fixed && ws.task == taskRoom && !ws.fixedConfirmed[ws.project.Hull.Type] {
		space()
		imgui.TextWrapped("This turns " + ws.project.Hull.Name + " into a modular ship: its walls stay in the hull, the furniture in the selected tiles moves into the module's first option, and the ship becomes purchasable in the shipyard.")
		space()
		if actionButton("Continue", true) {
			if ws.fixedConfirmed == nil {
				ws.fixedConfirmed = map[string]bool{}
			}
			ws.fixedConfirmed[ws.project.Hull.Type] = true
		}
		if actionButton("Cancel", false) {
			ws.finishTask()
		}
		return
	}
	if ws.source != 0 && ws.assembly != nil {
		// Shapes live on the hull; a switch mid-task moved editing elsewhere.
		ws.flush()
		ws.source = 0
		ws.rebuild()
	}
	tiles := tools.RoomShapeTiles()
	shape, _, shapeErr := ship.FootprintFromTiles(tiles)
	supported := ws.project == nil || ws.project.SupportsFootprints()
	if len(tiles) == 0 {
		hint("Click hull tiles to add them to the module.")
	} else {
		imgui.Text(fmt.Sprintf("%d tiles selected, %d x %d box", len(tiles), shape.W, shape.H))
		if !shape.Connected() {
			imgui.TextColored(style.Amber, "Module is in two pieces")
		}
	}
	hint("Drag to paint  |  Shift+drag adds a box  |  Alt+drag removes a box")
	if !supported {
		hint("This project's game code does not support custom module shapes yet. Modules must be rectangles.")
	}
	if reason := tools.RoomShapeRejection(); reason != "" {
		imgui.PushStyleColor(imgui.StyleColorText, style.Amber)
		workshop.Wrapped(reason)
		imgui.PopStyleColor()
	}
	if !tools.IsSelected(tools.TNRoomShape) {
		if _, _, ready := tools.SelectionBounds(); ready {
			if actionButton("Use Grab selection", false) {
				ws.importGrabSelection()
			}
			tooltip("Adds the Grab rectangle to the module's tiles.")
		}
		if actionButton("Select module tiles", false) {
			tools.SetSelected(tools.TNRoomShape)
		}
	} else if len(tiles) > 0 && actionButton("Clear tiles", false) {
		tools.ClearRoomShape()
	}
	valid := shapeErr == nil && (supported || shape.IsFull())
	label := "Apply new shape"
	if ws.task == taskRoom {
		textField("Module name", "e.g. Cargo bay", &ws.itemName)
		ws.itemIdentifier()
		nameErr := ws.project.ModuleNameError(ws.itemName)
		valid = valid && nameErr == nil && ship.ValidID(ws.itemID) == nil && !ws.itemIDUsed(ws.itemID)
		if strings.TrimSpace(ws.itemName) != "" && nameErr != nil {
			hint(nameErr.Error())
		}
		label = "Create module"
	}
	space()
	imgui.BeginDisabledV(!valid)
	if actionButton(label, true) {
		ws.applyRegion()
	}
	imgui.EndDisabled()
	hint("You can undo this with Ctrl+Z.")
}

func (ws *WsShip) applyRegion() {
	shape, origin, err := ship.FootprintFromTiles(tools.RoomShapeTiles())
	if err != nil || ws.source != 0 || ws.pane == nil {
		ws.message = "Select the module's tiles on the hull first."
		return
	}
	if ws.task == taskReshape {
		slot := ws.reshapeSlot
		ws.change("Change module shape", func() error { return ws.project.ReshapeSlot(ws.theme, slot, origin, shape) })
		if ws.message == "" {
			ws.finishTask()
			ws.editRoom(slot)
		}
		return
	}
	ws.change("Make upgrade module", func() error {
		return ws.project.AddSlot(ws.theme, ws.itemID, strings.TrimSpace(ws.itemName), origin, shape)
	})
	if ws.message == "" {
		ws.defaults()
		ws.rebuild()
		for i, s := range ws.assembly.Sources {
			if s.Slot == ws.itemID {
				ws.source = i
				break
			}
		}
		ws.rebuild()
		ws.finishTask()
	}
}

func (ws *WsShip) areaControls() {
	heading("SHIP AREAS")
	if ws.assembly == nil || ws.source >= len(ws.assembly.Sources) {
		return
	}
	imgui.TextWrapped("Editing: " + ws.assembly.Sources[ws.source].Name)
	areas, err := ws.project.Areas(ws.currentTheme())
	if err != nil {
		imgui.TextWrapped(err.Error())
		return
	}
	root, _ := ws.project.AreaRoot(ws.currentTheme())
	hint("Areas share this ship's ownership across themes and modules.")
	space()
	imgui.BeginChildV("ship-area-list", imgui.Vec2{Y: 130 * window.PointSize()}, true, 0)
	for _, area := range areas {
		imgui.PushID(area.Path)
		if imgui.SelectableV(area.Name, ws.areaPath == area.Path, 0, imgui.Vec2{Y: 26 * window.PointSize()}) {
			ws.areaPath = area.Path
		}
		tooltip(area.Path)
		imgui.PopID()
	}
	imgui.EndChild()
	imgui.BeginDisabledV(ws.areaPath == "")
	if actionButton("Paint selected area", false) {
		ws.app.DoSelectPrefab(dmmap.PrefabStorage.Initial(ws.areaPath))
		ws.finishTask()
		tools.SetSelected(tools.TNAdd)
	}
	imgui.EndDisabled()
	lo, hi, ready := tools.SelectionBounds()
	if ready {
		imgui.Text(fmt.Sprintf("Selected: %d x %d tiles", hi.X-lo.X+1, hi.Y-lo.Y+1))
	} else {
		hint("Use Grab (3) to select tiles for area assignment.")
	}
	imgui.BeginDisabledV(!ready || ws.areaPath == "")
	if actionButton("Assign area to selected tiles", true) {
		file := ws.assembly.Sources[ws.source].File
		ws.change("Assign ship area", func() error { return ws.project.AssignArea(ws.currentTheme(), file, ws.areaPath, lo, hi) })
	}
	imgui.EndDisabled()
	hint("Changes only the area assignment. Floors, walls and objects stay as mapped.")
	space()
	imgui.Separator()
	heading("CREATE AREA")
	textField("Area name", "e.g. Bridge or Cargo bay", &ws.itemName)
	if !ws.customID {
		ws.itemID = suggestedID(ws.itemName, func(id string) bool { return ws.project.AreaIDUsed(root, id) })
	}
	markers := []struct{ name, state string }{{"General", "station"}, {"Bridge", "bridge"}, {"Cargo", "quart"}, {"Engineering", "engie"}, {"Medical", "medbay"}, {"Crew", "commons"}}
	markerName := "General"
	for _, marker := range markers {
		if marker.state == ws.areaIcon {
			markerName = marker.name
		}
	}
	if combo("Map marker", markerName) {
		for _, marker := range markers {
			if imgui.SelectableV(marker.name, marker.state == ws.areaIcon, 0, imgui.Vec2{}) {
				ws.areaIcon = marker.state
			}
		}
		imgui.EndCombo()
	}
	if imgui.CollapsingHeader("Advanced") {
		if textField("File identifier", "lowercase_letters", &ws.itemID) {
			ws.customID = true
		}
		hint(root + "/" + ws.itemID)
	}
	nameErr := ws.project.AreaNameError(ws.currentTheme(), ws.itemName)
	valid := nameErr == nil && ship.ValidID(ws.itemID) == nil && !ws.project.AreaIDUsed(root, ws.itemID)
	if strings.TrimSpace(ws.itemName) != "" && nameErr != nil {
		hint(nameErr.Error())
	}
	imgui.BeginDisabledV(!valid)
	createLabel := "Create area"
	if ready {
		createLabel = "Create area for selection"
	}
	if actionButton(createLabel, true) {
		ws.createArea()
	}
	imgui.EndDisabled()
	hint("Saving writes the area definition and adds its code to the project.")
}

func (ws *WsShip) createArea() {
	lo, hi, selected := tools.SelectionBounds()
	ws.change("Create ship area", func() error {
		var err error
		ws.areaPath, err = ws.project.AddArea(ws.currentTheme(), ws.itemID, ws.itemName, ws.areaIcon)
		if err == nil && selected {
			if ws.assembly == nil || ws.source >= len(ws.assembly.Sources) {
				return fmt.Errorf("select a ship part first")
			}
			err = ws.project.AssignArea(ws.currentTheme(), ws.assembly.Sources[ws.source].File, ws.areaPath, lo, hi)
		}
		return err
	})
	if ws.message == "" {
		ws.itemName, ws.itemID, ws.customID = "", "", false
	}
}

func (ws *WsShip) itemIdentifier() {
	if !ws.customID {
		ws.itemID = suggestedID(ws.itemName, ws.itemIDUsed)
	}
	if imgui.CollapsingHeader("Advanced") {
		if textField("File identifier", "lowercase_letters", &ws.itemID) {
			ws.customID = true
		}
		if ship.ValidID(ws.itemID) != nil || ws.itemIDUsed(ws.itemID) {
			hint("Use a unique lowercase identifier starting with a letter.")
		}
	}
}
func (ws *WsShip) copyControls() {
	label := "Create ship theme"
	if ws.task == taskTheme {
		heading("NEW SHIP THEME")
		hint("Another hull of this ship, with its own modules, crew and price.")
		textField("Theme name", "e.g. Salvager", &ws.itemName)
	} else {
		heading("NEW MODULE OPTION")
		hint("Create another option for the module you are editing.")
		textField("Module option name", "e.g. Medical bay", &ws.itemName)
		imgui.Checkbox("Start with an empty module", &ws.emptyModule)
		hint("Otherwise, the current module's contents are copied.")
		label = "Create module option"
	}
	descriptionField(&ws.itemDescription)
	if ws.task == taskTheme {
		imgui.Text("Modules")
		if imgui.RadioButton("Share with the ship", !ws.copyRooms) {
			ws.copyRooms = false
		}
		hint("Recommended. Edits to shared modules show in every theme that shares them.")
		if imgui.RadioButton("Copy every module", ws.copyRooms) {
			ws.copyRooms = true
		}
		hint("This theme gets its own copy of each module file.")
	}
	ws.itemIdentifier()
	nameErr := ws.project.ModuleNameError(ws.itemName)
	if ws.task == taskTheme {
		nameErr = ws.project.ThemeNameError(ws.itemName)
	}
	if imgui.CollapsingHeader("Part costs###new-part-costs") {
		for _, class := range ship.PartClasses {
			value := int32(ws.itemCosts[class.ID])
			numberField(class.Name, &value)
			ws.itemCosts[class.ID] = int(value)
		}
	}
	costErr := ws.itemCosts.Validate()
	if costErr != nil {
		hint(costErr.Error())
	} else if ws.itemCosts.Summary() == "Free" {
		hint("Free in the shipyard until you set a price.")
	}
	valid := nameErr == nil && costErr == nil && ship.ValidID(ws.itemID) == nil && !ws.itemIDUsed(ws.itemID)
	if strings.TrimSpace(ws.itemName) != "" && nameErr != nil {
		hint(nameErr.Error())
	}
	imgui.BeginDisabledV(!valid)
	if actionButton(label, true) {
		if ws.task == taskTheme {
			ws.createVariant()
		} else if ws.assembly != nil && ws.source > 0 && ws.source < len(ws.assembly.Sources) {
			slot := ws.assembly.Sources[ws.source].Slot
			for _, m := range ws.project.Hull.Modules {
				if m.ID == ws.selected[slot] {
					ws.change("Create module option", func() error {
						if err := ws.project.AddModule(ws.theme, m, ws.itemID, strings.TrimSpace(ws.itemName), ws.emptyModule); err != nil {
							return err
						}
						if err := ws.project.SetDescription("module/"+ws.itemID, ws.itemDescription); err != nil {
							return err
						}
						return ws.project.SetPartCosts("module/"+ws.itemID, ws.itemCosts)
					})
					if ws.message == "" {
						ws.selected[slot] = ws.itemID
						ws.rebuild()
					}
					break
				}
			}
		}
		if ws.message == "" {
			ws.finishTask()
		}
	}
	imgui.EndDisabled()
}

// createVariant adds the variant the form describes and opens it for editing.
func (ws *WsShip) createVariant() {
	ws.change("Create ship theme", func() error {
		if err := ws.project.AddTheme(ws.theme, ws.itemID, strings.TrimSpace(ws.itemName), ws.copyRooms); err != nil {
			return err
		}
		if err := ws.project.SetDescription("theme/"+ws.itemID, ws.itemDescription); err != nil {
			return err
		}
		return ws.project.SetPartCosts("theme/"+ws.itemID, ws.itemCosts)
	})
	if ws.message == "" {
		ws.theme = len(ws.project.Hull.Themes) - 1
		ws.defaults()
		ws.rebuild()
	}
}

func (ws *WsShip) settingsControls() {
	heading("SHIP DETAILS")
	s := &ws.settings
	textField("Ship name", "Name shown to players", &s.name)
	textField("Description", "What is this ship for?", &s.description)
	heading("BASE SHIP PRICE")
	if imgui.BeginTableV("base-ship-price", 2, imgui.TableFlagsSizingStretchSame, imgui.Vec2{}, 0) {
		for _, class := range ship.PartClasses {
			imgui.TableNextColumn()
			imgui.Text(class.Name + " parts")
			value := int32(s.costs[class.ID])
			imgui.SetNextItemWidth(-1)
			imgui.InputIntV("##base-cost-"+class.ID, &value, 0, 0, imgui.InputTextFlagsNone)
			s.costs[class.ID] = int(value)
		}
		imgui.EndTable()
	}
	hint(s.costs.Summary())
	if ws.project.Settings != nil && ws.project.Crew == nil {
		numberField("Starting crew capacity", &s.crew)
	} else {
		hint("Crew capacity is set by the slots in Crew & equipment.")
	}
	imgui.Checkbox("Hide from the player ship list", &s.hidden)
	hint("Check this to hide an unfinished ship.")
	nameErr := ship.ShipNameError(ws.catalog, ws.app.LoadedEnvironment(), s.name, ws.project.Hull.Type)
	costErr := s.costs.Validate()
	valid := nameErr == nil && costErr == nil && (ws.project.Settings == nil || s.crew >= 1 && s.crew <= 32)
	if costErr != nil {
		hint(costErr.Error())
	}
	if strings.TrimSpace(s.name) != "" && nameErr != nil {
		hint(nameErr.Error())
	}
	if !valid {
		hint("Enter a name and 1 to 32 crew.")
	}
	space()
	imgui.BeginDisabledV(!valid)
	if actionButton("Apply ship details", true) {
		if ws.commitSettings() {
			ws.finishTask()
		}
	}
	imgui.EndDisabled()
}

func (ws *WsShip) settingsPending() bool {
	if ws.task != taskSettings || ws.project == nil {
		return false
	}
	s, original := ws.settings, ws.project.ShipDetails()
	costs, err := ws.project.PartCosts("ship")
	return err != nil || strings.TrimSpace(s.name) != ws.project.Hull.Name || s.description != original.Description || int(s.crew) != original.Crew || s.hidden != original.Hidden || !s.costs.Equal(costs)
}

func (ws *WsShip) commitSettings() bool {
	if !ws.settingsPending() {
		return true
	}
	s := ws.settings
	if err := s.costs.Validate(); err != nil {
		ws.message = err.Error()
		return false
	}
	if ws.project.Settings != nil && (s.crew < 1 || s.crew > 32) {
		ws.message = "Enter 1 to 32 crew."
		return false
	}
	ws.message = ""
	ws.change("Ship details", func() error {
		if err := ws.project.RenameShip(s.name); err != nil {
			return err
		}
		if err := ws.project.SetPartCosts("ship", s.costs); err != nil {
			return err
		}
		return ws.project.SetShipDetails(ship.ShipDetails{Description: s.description, Crew: int(s.crew), Hidden: s.hidden})
	})
	if ws.message != "" {
		return false
	}
	ws.task = taskPaint
	return true
}
