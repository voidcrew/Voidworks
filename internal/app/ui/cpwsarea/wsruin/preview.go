package wsruin

import (
	"fmt"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wspreview"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/mappreview"
)

const (
	previewDetailsWidth = 340
	// Below this width the preview and the properties stack instead.
	previewSideBySideWidth = 760
)

func (ws *WsRuin) previewVisible() bool {
	return !ws.creating && !ws.viewOnPlanet && ws.selected != nil && ws.selected.Problem == ""
}

func (ws *WsRuin) loadRuinPreview() {
	ws.previewStale = false
	ws.previewError = ""
	source, err := ws.loadPreviewRuin()
	if err != nil {
		ws.previewError = "This ruin's map could not be read: " + err.Error()
		return
	}
	ws.previewLevels = source.MaxZ
	if ws.ruinPreview == nil {
		ws.ruinPreview = wspreview.NewInstant(source, ws.catalog.Dme, mappreview.Options{Smoothing: true, PoweredFixtures: true, ExteriorLight: true})
	} else {
		ws.ruinPreview.ReplaceSource(source, ws.catalog.Dme)
		if ws.previewFile != ws.selected.File {
			ws.ruinPreview.Fit()
		}
	}
	ws.previewFile = ws.selected.File
}

// previewPanel draws beside the details panel, not inside it: zooming the map
// with the wheel must not also scroll the properties.
func (ws *WsRuin) previewPanel() {
	if !ws.previewVisible() {
		return
	}
	if ws.previewStale {
		ws.loadRuinPreview()
	}
	scale := window.PointSize()
	available := imgui.ContentRegionAvail()
	sideBySide := available.X >= previewSideBySideWidth*scale
	size := imgui.Vec2{Y: available.Y * .5}
	if sideBySide {
		size = imgui.Vec2{X: available.X - previewDetailsWidth*scale}
	}
	workshop.Panel("ruin-preview", size, false)
	if ws.previewError != "" {
		imgui.TextWrapped(ws.previewError)
	} else if ws.ruinPreview != nil {
		if ws.previewLevels > 1 {
			hint(fmt.Sprintf("Showing level 1 of %d.", ws.previewLevels))
		}
		ws.ruinPreview.Process()
	}
	imgui.EndChild()
	if sideBySide {
		imgui.SameLine()
	}
}
