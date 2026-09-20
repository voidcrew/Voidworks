package wssprite

import (
	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmi"
	"strings"
)

func (w *Workspace) conversionControls() {
	issues := w.Document.Icon.ConversionIssues()
	if len(issues) == 0 {
		return
	}
	imgui.TextWrapped("This image needs conversion before editing. Its original file is preserved until you choose Save.")
	if imgui.Button("Prepare for editing...") {
		w.confirm("Convert image for editing?", strings.Join(issues, "\n")+"\n\nUndo restores the original. Use Save as to keep both versions.", func() {
			before, after, err := w.Document.Apply(func(i *dmi.Icon) error { i.ConvertForEditing(); return nil })
			if err != nil {
				w.message = err.Error()
				return
			}
			w.history("Prepare image for editing", before, after)
			w.before = false
			w.publish()
			w.message = "Image ready for editing"
		})
	}
}
