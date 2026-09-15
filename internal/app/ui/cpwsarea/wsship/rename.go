package wsship

import (
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/window"
)

func (ws *WsShip) beginRename(task buildTask, id, name string) {
	ws.beginTask(task)
	ws.itemID, ws.itemName = id, name
	ws.renameOriginal = name
	ws.itemDescription = ""
	if task != taskRenameRoom {
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
	if ws.task == taskRenameRoom {
		heading("ROOM NAME")
		if textField("Room name", "e.g. Cargo bay", &ws.itemName) {
			ws.message = ""
		}
		hint("Applies to this room across all ship variants.")
	} else if ws.task == taskRenameTheme {
		heading("SHIP VARIANT DETAILS")
		textField("Variant name", "e.g. Salvager", &ws.itemName)
	} else {
		heading("ROOM OPTION DETAILS")
		textField("Room option name", "e.g. Medical bay", &ws.itemName)
	}
	if ws.task != taskRenameRoom {
		descriptionField(&ws.itemDescription)
	}
	nameErr := ws.renameNameError()
	if nameErr != nil {
		hint(nameErr.Error())
	}
	space()
	imgui.BeginDisabledV(nameErr != nil)
	label := "Apply details"
	if ws.task == taskRenameRoom {
		label = "Rename room"
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
	return (ws.task == taskRenameTheme || ws.task == taskRenameModule || ws.task == taskRenameRoom) &&
		(strings.TrimSpace(ws.itemName) != ws.renameOriginal || ws.itemDescription != ws.descriptionOriginal)
}

func (ws *WsShip) cancelRename() {
	ws.task, ws.message = taskPaint, ""
}

func (ws *WsShip) commitRename() bool {
	if !ws.renamePending() {
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
	label := "Edit room details"
	if ws.task == taskRenameTheme {
		label = "Edit variant details"
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
				ws.message = "Close the ordinary map tab before renaming this room."
				return false
			}
		}
	}
	ws.message = ""
	ws.change("Rename room", func() error {
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
