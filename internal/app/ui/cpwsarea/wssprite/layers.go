package wssprite

import (
	"fmt"
	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmi"
)

func (w *Workspace) layerControls() {
	if len(w.Document.Icon.States) == 0 || !imgui.CollapsingHeader("Layers") {
		return
	}
	s := w.Document.Icon.States[w.state]
	if imgui.Button("+ Layer") {
		w.change("Add layer", func(i *dmi.Icon) error {
			layer, err := i.AddLayer(w.state)
			if err == nil {
				w.layer = layer
			}
			return err
		})
	}
	if len(s.Layers) == 0 {
		imgui.TextDisabled("One flattened image per frame")
		return
	}
	imgui.SameLine()
	if imgui.Button("Layers...") {
		imgui.OpenPopup("layer-actions")
	}
	if imgui.BeginPopup("layer-actions") {
		if imgui.MenuItem("Duplicate layer") {
			w.change("Duplicate layer", func(i *dmi.Icon) error {
				layer, err := i.DuplicateLayer(w.state, w.layer)
				if err == nil {
					w.layer = layer
				}
				return err
			})
		}
		if imgui.MenuItem("Move up") {
			w.moveLayer(1)
		}
		if imgui.MenuItem("Move down") {
			w.moveLayer(-1)
		}
		if imgui.MenuItem("Delete layer") {
			w.confirm("Delete layer?", "Remove the selected layer from every frame and direction? Undo restores it.", func() {
				w.change("Delete layer", func(i *dmi.Icon) error {
					s := i.States[w.state]
					if len(s.Layers) < 2 {
						return fmt.Errorf("keep at least one layer, or clear its pixels")
					}
					s.Layers = append(s.Layers[:w.layer], s.Layers[w.layer+1:]...)
					s.RecomposeAll()
					return nil
				})
			})
		}
		if imgui.MenuItem("Flatten layers") {
			w.confirm("Flatten layers?", "Keep the visible image and remove this state's layer stack? Undo restores it.", func() {
				w.change("Flatten layers", func(i *dmi.Icon) error { i.States[w.state].Layers = nil; return nil })
			})
		}
		imgui.EndPopup()
	}
	s = w.Document.Icon.States[w.state]
	imgui.BeginChildV("layer-list", imgui.Vec2{Y: min(140, float32(len(s.Layers))*28+10)}, true, 0)
	for n := len(s.Layers) - 1; n >= 0; n-- {
		l := s.Layers[n]
		imgui.PushIDInt(n)
		visible := l.Visible
		if imgui.Checkbox("##visible", &visible) {
			w.change("Layer visibility", func(i *dmi.Icon) error {
				s := i.States[w.state]
				s.Layers[n].Visible = visible
				s.RecomposeAll()
				return nil
			})
		}
		imgui.SameLine()
		if imgui.SelectableV(l.Name, w.layer == n, 0, imgui.Vec2{}) {
			w.finishStroke()
			w.layer = n
			w.layerName = l.Name
		}
		imgui.PopID()
	}
	imgui.EndChild()
	if len(s.Layers) == 0 {
		return
	}
	l := s.Layers[min(w.layer, len(s.Layers)-1)]
	imgui.SetNextItemWidth(-1)
	if imgui.InputTextV("##layer-name", &w.layerName, imgui.InputTextFlagsEnterReturnsTrue, nil) {
		name := w.layerName
		w.change("Rename layer", func(i *dmi.Icon) error { i.States[w.state].Layers[w.layer].Name = name; return nil })
	}
	opacity := int32(l.Opacity)
	imgui.SetNextItemWidth(-1)
	if imgui.SliderInt("##opacity", &opacity, 0, 255) {
		w.change("Layer opacity", func(i *dmi.Icon) error {
			s := i.States[w.state]
			s.Layers[w.layer].Opacity = uint8(opacity)
			s.RecomposeAll()
			return nil
		})
	}
	locked := l.Locked
	if imgui.Checkbox("Lock pixels", &locked) {
		w.change("Lock layer", func(i *dmi.Icon) error { i.States[w.state].Layers[w.layer].Locked = locked; return nil })
	}
	imgui.TextWrapped("Layers are saved in this DMI by Voidworks. Other editors may flatten them.")
}
func (w *Workspace) moveLayer(delta int) {
	w.change("Reorder layers", func(i *dmi.Icon) error {
		s := i.States[w.state]
		to := w.layer + delta
		if to < 0 || to >= len(s.Layers) {
			return fmt.Errorf("layer is already at the edge")
		}
		s.Layers[w.layer], s.Layers[to] = s.Layers[to], s.Layers[w.layer]
		w.layer = to
		s.RecomposeAll()
		return nil
	})
}
