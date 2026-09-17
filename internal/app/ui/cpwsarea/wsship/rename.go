package wsship

import (
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/window"
	"sdmm/internal/ship"
)

func (ws *WsShip) beginRename(task buildTask, id, name string) {
	ws.beginTask(task)
	ws.itemID, ws.itemName = id, name
	ws.renameOriginal = name
	ws.itemDescription = ""
	if task != taskRenameRoom && task != taskRenameShip {
		ws.itemDescription = ws.project.Description(ws.renameScope())
	}
	ws.descriptionOriginal = ws.itemDescription
}

func (ws *WsShip) renameScope() string {
	if ws.task == taskRenameTheme {
		return "theme/" + ws.itemID
	}
	return "module/" + ws.itemID
}

func (ws *WsShip) renameControls() {
	if ws.task == taskRenameShip {
		heading("RENAME SHIP")
		textField("Ship name", "Name shown to players", &ws.itemName)
		hint("Renames this ship's maps and dedicated source files together. Shared files stay available to other ships; game IDs stay the same.")
		hint("Review the changed files before saving. Undo also restores filenames.")
	} else if ws.task == taskRenameRoom {
		heading("MODULE NAME")
		if textField("Module name", "e.g. Cargo bay", &ws.itemName) {
			ws.message = ""
		}
		hint("Applies to this module across all ship themes.")
	} else if ws.task == taskRenameTheme {
		heading("SHIP THEME DETAILS")
		textField("Theme name", "e.g. Salvager", &ws.itemName)
	} else {
		heading("MODULE OPTION DETAILS")
		textField("Module option name", "e.g. Medical bay", &ws.itemName)
	}
	if ws.task != taskRenameRoom && ws.task != taskRenameShip {
		descriptionField(&ws.itemDescription)
	}
	nameErr := ws.renameNameError()
	if nameErr != nil {
		hint(nameErr.Error())
	}
	space()
	imgui.BeginDisabledV(nameErr != nil)
	label := "Apply details"
	if ws.task == taskRenameShip {
		label = "Rename ship"
	}
	if ws.task == taskRenameRoom {
		label = "Rename module"
	}
	if actionButton(label, true) {
		ws.applyRename()
	}
	imgui.EndDisabled()
	if actionButton("Cancel", false) {
		ws.cancelRename()
	}
}

func (ws *WsShip) applyRename() {
	if ws.commitRename() {
		ws.task = taskPaint
	}
}

func (ws *WsShip) renamePending() bool {
	return (ws.task == taskRenameTheme || ws.task == taskRenameModule || ws.task == taskRenameRoom || ws.task == taskRenameShip) &&
		(strings.TrimSpace(ws.itemName) != ws.renameOriginal || ws.itemDescription != ws.descriptionOriginal)
}

func (ws *WsShip) cancelRename() {
	ws.task, ws.message = taskPaint, ""
}

func (ws *WsShip) commitRename() bool {
	if !ws.renamePending() {
		return true
	}
	if ws.task == taskRenameShip {
		ws.message = ""
		ws.change("Rename ship", func() error { return ws.project.RenameShip(ws.itemName) })
		if ws.message != "" {
			return false
		}
		ws.task = taskPaint
		return true
	}
	if ws.task == taskRenameRoom {
		return ws.commitRoomRename()
	}
	scope, name := ws.renameScope(), ws.itemName
	if err := ws.project.RenameNameError(scope, name); err != nil {
		ws.message = err.Error()
		return false
	}
	label := "Edit module details"
	if ws.task == taskRenameTheme {
		label = "Edit theme details"
	}
	ws.message = ""
	ws.change(label, func() error {
		if err := ws.project.Rename(scope, name); err != nil {
			return err
		}
		return ws.project.SetDescription(scope, ws.itemDescription)
	})
	if ws.message != "" {
		return false
	}
	ws.task = taskPaint
	return true
}

func descriptionField(value *string) {
	imgui.Text("Description")
	imgui.InputTextMultilineV("##component-description", value, imgui.Vec2{X: -1, Y: 95 * window.PointSize()}, 0, nil)
	hint("Shown to players when choosing this option in the shipyard.")
}

func (ws *WsShip) renameNameError() error {
	if ws.task == taskRenameShip {
		return ship.ShipNameError(ws.catalog, ws.app.LoadedEnvironment(), ws.itemName, ws.project.Hull.Type)
	}
	if ws.task == taskRenameRoom {
		return ws.project.RenameSlotNameError(ws.itemID, ws.itemName)
	}
	return ws.project.RenameNameError(ws.renameScope(), ws.itemName)
}

func (ws *WsShip) commitRoomRename() bool {
	slot, name := ws.itemID, ws.itemName
	if err := ws.project.PrepareSlotRename(slot, name); err != nil {
		ws.message = err.Error()
		return false
	}
	if ws.SourceBusy != nil {
		for path, d := range ws.project.Documents {
			if d.Active && ws.SourceBusy(path) {
				ws.message = "Close the ordinary map tab before renaming this module."
				return false
			}
		}
	}
	ws.message = ""
	ws.change("Rename module", func() error {
		next, err := ws.project.RenameSlot(slot, name)
		if err != nil {
			return err
		}
		if selected, ok := ws.selected[slot]; ok {
			delete(ws.selected, slot)
			ws.selected[next] = selected
		}
		return nil
	})
	if ws.message != "" {
		return false
	}
	ws.hoverRoom = ""
	ws.task = taskPaint
	return true
}
