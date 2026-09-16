package wssprite

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmi"
)

type sheetImport struct {
	image                   *image.NRGBA
	name                    string
	width, height, dirs     int32
	directionRows, separate bool
}

func (w *Workspace) stageSheet(icon *dmi.Icon, path string) {
	w.importSheet = &sheetImport{image: icon.States[0].Cels[0], name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), width: int32(w.Document.Icon.Width), height: int32(w.Document.Icon.Height), dirs: 1}
	w.openImport = true
}

func (w *Workspace) importDialog() {
	if w.openImport {
		imgui.OpenPopup("Import sprite sheet")
		w.openImport = false
	}
	if !imgui.BeginPopup("Import sprite sheet") {
		return
	}
	defer imgui.EndPopup()
	v := w.importSheet
	if v == nil {
		imgui.CloseCurrentPopup()
		return
	}
	imgui.Text(fmt.Sprintf("Sheet: %d x %d pixels", v.image.Rect.Dx(), v.image.Rect.Dy()))
	imgui.InputText("State name", &v.name)
	imgui.InputInt("Cell width", &v.width)
	imgui.InputInt("Cell height", &v.height)
	if imgui.BeginCombo("Directions", fmt.Sprint(v.dirs)) {
		for _, d := range []int32{1, 4, 8} {
			if imgui.Selectable(fmt.Sprint(d)) {
				v.dirs = d
			}
		}
		imgui.EndCombo()
	}
	imgui.Checkbox("Separate state for each group of directions", &v.separate)
	imgui.Checkbox("Sheet order: all frames of S, then N, E, W...", &v.directionRows)
	imgui.TextWrapped("Unchecked order: S, N, E, W for frame 1, then frame 2. Cells are read left to right, then top to bottom.")
	if imgui.Button("Import cells") {
		icon, err := dmi.SliceSheet(v.image, int(v.width), int(v.height), int(v.dirs), v.directionRows, v.separate, v.name)
		if err != nil {
			w.message = err.Error()
		} else {
			w.importIcon(icon)
			if w.message == "" {
				imgui.CloseCurrentPopup()
				w.importSheet = nil
			}
		}
	}
	imgui.SameLine()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
		w.importSheet = nil
	}
	if w.message != "" {
		imgui.TextWrapped(w.message)
	}
}
