package wssprite

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/dmi"
)

func (w *Workspace) toolbar() {
	if imgui.Button("Save") {
		w.Save()
	}
	imgui.SameLine()
	if imgui.Button("Save as...") {
		w.saveAs()
	}
	imgui.SameLine()
	if imgui.Button("Import...") {
		w.importFile()
	}
	imgui.SameLine()
	if imgui.Button("Export...") {
		imgui.OpenPopup("export")
	}
	if imgui.BeginPopup("export") {
		if imgui.MenuItem("Current frame as PNG") {
			w.exportFile(false)
		}
		if imgui.MenuItem("Selected states as DMI") {
			w.exportFile(true)
		}
		imgui.EndPopup()
	}
	imgui.SameLine()
	if imgui.Button("Undo") {
		w.Undo()
	}
	imgui.SameLine()
	if imgui.Button("Redo") {
		w.Redo()
	}
	imgui.SameLine()
	if imgui.Checkbox("Before", &w.before) {
		w.publish()
	}
	imgui.SameLine()
	if imgui.Button("File...") {
		imgui.OpenPopup("file-actions")
	}
	if imgui.BeginPopup("file-actions") {
		if imgui.MenuItem("Reload from disk...") {
			w.confirm("Reload DMI?", "Discard unsaved edits and reload the file?", w.reload)
		}
		if imgui.MenuItem("Resize all states...") {
			w.openResize = true
		}
		if imgui.MenuItem("Open recovery backups") {
			w.openBackups()
		}
		imgui.EndPopup()
	}
	if w.openResize {
		w.openResize = false
		imgui.OpenPopup("resize-dmi")
	}
	w.resizeDialog()
	w.importDialog()
	imgui.TextDisabled(w.status())
}
func (w *Workspace) stateList() {
	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##search", "Search states", &w.filter)
	if imgui.Button("+ State") {
		w.change("Add state", func(i *dmi.Icon) error { w.state = i.AddState("new_state"); return nil })
		w.selected = map[int]bool{w.state: true}
	}
	imgui.SameLine()
	if imgui.Button("Actions") {
		imgui.OpenPopup("state-actions")
	}
	if imgui.BeginPopup("state-actions") {
		indices := w.indices()
		if imgui.MenuItem("Select all") {
			for n := range w.Document.Icon.States {
				w.selected[n] = true
			}
		}
		if imgui.MenuItem("Duplicate selected") {
			w.change("Duplicate states", func(i *dmi.Icon) error { return i.DuplicateStates(indices) })
		}
		if imgui.MenuItem("Copy selected states") {
			w.CopyStates()
		}
		if imgui.MenuItem("Paste states") {
			w.PasteStates()
		}
		if imgui.MenuItem("Delete selected...") {
			w.confirm("Delete states?", fmt.Sprintf("Delete %d selected states? Map and code references will keep their existing names. This is undoable.", len(indices)), func() {
				w.change("Delete states", func(i *dmi.Icon) error { return i.DeleteStates(indices) })
				w.selected = map[int]bool{w.state: true}
			})
		}
		imgui.Separator()
		if imgui.MenuItem("Sort by name") {
			w.change("Sort states", func(i *dmi.Icon) error { i.SortStates(); return nil })
		}
		if imgui.MenuItem("Split into frames (keep original)") {
			w.change("Split frames", func(i *dmi.Icon) error { return i.SplitState(w.state, false) })
		}
		if imgui.MenuItem("Split into directions (keep original)") {
			w.change("Split directions", func(i *dmi.Icon) error { return i.SplitState(w.state, true) })
		}
		if imgui.MenuItem("Combine as animation (keep originals)") {
			w.change("Combine frames", func(i *dmi.Icon) error { return i.CombineStates(indices, false) })
		}
		if imgui.MenuItem("Combine as directions (keep originals)") {
			w.change("Combine directions", func(i *dmi.Icon) error { return i.CombineStates(indices, true) })
		}
		imgui.EndPopup()
	}
	if imgui.CollapsingHeader("Batch rename") {
		imgui.InputText("Prefix", &w.prefix)
		imgui.InputText("Find", &w.find)
		imgui.InputText("Replace", &w.replacement)
		imgui.InputText("Suffix", &w.suffix)
		if imgui.Button("Rename selected") {
			w.change("Rename states", func(i *dmi.Icon) error { return i.Rename(w.indices(), w.prefix, w.find, w.replacement, w.suffix) })
		}
	}
	if imgui.Button("Resize canvas...") {
		w.openResize = true
	}
	w.paletteControls()
	w.layerControls()
	imgui.BeginChild("state-rows")
	var filtered []int
	filter := strings.ToLower(w.filter)
	for n, s := range w.Document.Icon.States {
		if strings.Contains(strings.ToLower(s.Name), filter) {
			filtered = append(filtered, n)
		}
	}
	var clipper imgui.ListClipper
	clipper.Begin(len(filtered))
	for clipper.Step() {
		for row := clipper.DisplayStart; row < clipper.DisplayEnd; row++ {
			n := filtered[row]
			if n >= len(w.Document.Icon.States) {
				break
			}
			s := w.Document.Icon.States[n]
			imgui.PushIDInt(n)
			start := imgui.CursorScreenPos()
			label := s.Name
			if label == "" {
				label = "(default)"
			}
			if s.Movement() {
				label += " [movement]"
			}
			if imgui.SelectableV("##state", w.selected[n], 0, imgui.Vec2{Y: 46}) {
				w.selectState(n, imgui.IsKeyDown(int(glfw.KeyLeftControl)) || imgui.IsKeyDown(int(glfw.KeyRightControl)), imgui.IsKeyDown(int(glfw.KeyLeftShift)) || imgui.IsKeyDown(int(glfw.KeyRightShift)))
			}
			if imgui.BeginDragDropSource(0) {
				imgui.SetDragDropPayload("DMI_STATE", []byte(w.Id()+":"+strconv.Itoa(n)), imgui.ConditionOnce)
				imgui.Text(label)
				imgui.EndDragDropSource()
			}
			if imgui.BeginDragDropTarget() {
				if payload := imgui.AcceptDragDropPayload("DMI_STATE", 0); payload != nil {
					owner, number, _ := strings.Cut(string(payload), ":")
					from, e := strconv.Atoi(number)
					if e == nil && owner == w.Id() {
						w.change("Move state", func(i *dmi.Icon) error { return i.MoveState(from, n) })
						w.selectState(n, false, false)
					}
				}
				imgui.EndDragDropTarget()
			}
			if w.rendered != nil && !w.before {
				elapsed := time.Since(w.started).Seconds()
				if n == w.state {
					elapsed = w.playbackTime(time.Now())
				}
				f := s.FrameAt(elapsed)
				a, b := w.uv(n, f*s.Dirs())
				imgui.WindowDrawList().AddImageV(imgui.TextureID(w.rendered.Texture), start.Plus(imgui.Vec2{X: 3, Y: 3}), start.Plus(imgui.Vec2{X: 43, Y: 43}), a, b, imgui.PackedColorFromVec4(imgui.Vec4{X: 1, Y: 1, Z: 1, W: 1}))
			}
			imgui.WindowDrawList().AddText(start.Plus(imgui.Vec2{X: 49, Y: 4}), imgui.PackedColorFromVec4(imgui.Vec4{X: 1, Y: 1, Z: 1, W: 1}), label)
			imgui.WindowDrawList().AddText(start.Plus(imgui.Vec2{X: 49, Y: 24}), imgui.PackedColorFromVec4(imgui.Vec4{X: .6, Y: .65, Z: .7, W: 1}), fmt.Sprintf("%d directions | %d frames", s.Dirs(), s.Frames()))
			imgui.PopID()
		}
	}
	imgui.EndChild()
}
func (w *Workspace) properties() {
	s := w.Document.Icon.States[w.state]
	imgui.SetNextItemWidth(180)
	imgui.InputText("State", &w.name)
	imgui.SameLine()
	if imgui.Button("Rename") {
		w.change("Rename state", func(i *dmi.Icon) error { i.States[w.state].Name = w.name; return nil })
	}
	imgui.SameLine()
	imgui.SetNextItemWidth(65)
	if imgui.BeginCombo("Directions", strconv.Itoa(s.Dirs())) {
		for _, n := range []int{1, 4, 8} {
			if imgui.Selectable(strconv.Itoa(n)) {
				apply := func() {
					w.changeAnimation("Change directions", func(i *dmi.Icon) error { return i.SetDirections(w.state, n) })
				}
				if n < s.Dirs() {
					w.confirm("Remove directions?", "This removes the extra directions from every frame. Undo restores them.", apply)
				} else {
					apply()
				}
			}
		}
		imgui.EndCombo()
	}
	w.animationControls()
}
func boolNumber(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
func (w *Workspace) timeline() {
	if w.rendered == nil {
		return
	}
	s := w.Document.Icon.States[w.state]
	frame := w.cel / s.Dirs()
	label := "Play"
	if w.playing {
		label = "Pause"
	}
	if imgui.Button(label) {
		w.togglePlayback(time.Now())
	}
	imgui.SameLine()
	if imgui.Button("Restart") {
		w.selectCel(w.cel%s.Dirs(), false)
		w.playing, w.started = true, time.Now()
	}
	imgui.SameLine()
	imgui.Checkbox("Onion skin", &w.onion)
	imgui.SameLine()
	if imgui.Button("+ Blank") {
		w.changeAnimation("Insert frame", func(i *dmi.Icon) error { return i.InsertFrame(w.state, frame, false) })
	}
	imgui.SameLine()
	if imgui.Button("Duplicate frame") {
		w.changeAnimation("Duplicate frame", func(i *dmi.Icon) error { return i.InsertFrame(w.state, frame, true) })
	}
	imgui.SameLine()
	if imgui.Button("Delete frame") {
		w.changeAnimation("Delete frame", func(i *dmi.Icon) error { return i.DeleteFrame(w.state, frame) })
	}
	s = w.Document.Icon.States[w.state]
	imgui.BeginChildV("timeline", imgui.Vec2{}, true, imgui.WindowFlagsHorizontalScrollbar)
	names := []string{"S", "N", "E", "W", "SE", "SW", "NE", "NW"}
	for dir := 0; dir < s.Dirs(); dir++ {
		imgui.Text(names[dir])
		for f := 0; f < s.Frames(); f++ {
			imgui.SameLine()
			cel := f*s.Dirs() + dir
			imgui.PushIDInt(cel)
			a, b := w.uv(w.state, cel)
			bg := imgui.Vec4{X: .12, Y: .14, Z: .18, W: 1}
			if w.cells[cel] {
				bg = imgui.Vec4{X: .2, Y: .45, Z: .6, W: 1}
			}
			if imgui.ImageButtonV(imgui.TextureID(w.rendered.Texture), imgui.Vec2{X: 32, Y: 32}, a, b, 3, bg, imgui.Vec4{X: 1, Y: 1, Z: 1, W: 1}) {
				w.selectCel(cel, imgui.IsKeyDown(int(glfw.KeyLeftControl)) || imgui.IsKeyDown(int(glfw.KeyRightControl)))
			}
			if imgui.IsItemHovered() {
				imgui.SetTooltip(fmt.Sprintf("Frame %d, %s: %g ticks", f+1, names[dir], s.Delays()[f]))
			}
			if imgui.BeginDragDropSource(0) {
				imgui.SetDragDropPayload("DMI_FRAME", []byte(fmt.Sprintf("%s:%d:%d", w.Id(), w.state, f)), imgui.ConditionOnce)
				imgui.Text(fmt.Sprintf("Frame %d", f+1))
				imgui.EndDragDropSource()
			}
			if imgui.BeginDragDropTarget() {
				if payload := imgui.AcceptDragDropPayload("DMI_FRAME", 0); payload != nil {
					prefix := fmt.Sprintf("%s:%d:", w.Id(), w.state)
					from, e := strconv.Atoi(strings.TrimPrefix(string(payload), prefix))
					if e == nil && strings.HasPrefix(string(payload), prefix) {
						w.changeAnimation("Reorder frame", func(i *dmi.Icon) error { return i.MoveFrame(w.state, from, f) })
					}
				}
				imgui.EndDragDropTarget()
			}
			imgui.PopID()
		}
	}
	imgui.EndChild()
}
func (w *Workspace) resizeDialog() {
	if imgui.BeginPopup("resize-dmi") {
		imgui.InputInt("Width", &w.resizeWidth)
		imgui.InputInt("Height", &w.resizeHeight)
		imgui.Checkbox("Scale artwork (nearest pixel)", &w.scaleResize)
		imgui.SliderInt("Horizontal anchor", &w.anchorX, 0, 2)
		imgui.SliderInt("Vertical anchor", &w.anchorY, 0, 2)
		imgui.TextDisabled("0 = left/top, 1 = center, 2 = right/bottom")
		if imgui.Button("Resize every state") {
			w.confirm("Resize DMI?", "Apply the new canvas size to every frame and direction? Cropping can remove pixels; Undo restores them.", func() {
				w.change("Resize DMI", func(i *dmi.Icon) error {
					return i.Resize(int(w.resizeWidth), int(w.resizeHeight), int(w.anchorX), int(w.anchorY), w.scaleResize)
				})
			})
			imgui.CloseCurrentPopup()
		}
		imgui.EndPopup()
	}
}
