package cpwsarea

import (
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap"
	"sdmm/internal/app/ui/cpwsarea/wspreview"
	"sdmm/internal/app/ui/cpwsarea/wssprite"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmi"
	"sdmm/internal/util"
)

func (w *WsArea) FocusSprite(path, state string, dir int) bool {
	for _, ws := range w.workspaces {
		if s, ok := ws.Content().(*wssprite.Workspace); ok && s.Document.Path != "" && util.SamePath(path, s.Document.Path) {
			if scene, environment := w.spriteContext(); scene != nil && environment != nil {
				if s.Preview != nil {
					s.Preview.Dispose()
				}
				s.Preview = wspreview.NewSprite(scene, environment)
			}
			s.Select(state, dir)
			ws.SetTriggerFocus(true)
			return true
		}
	}
	return false
}
func (w *WsArea) spriteContext() (*dmmap.Dmm, *dmenv.Dme) {
	var scene *dmmap.Dmm
	environment := w.app.LoadedEnvironment()
	if active := w.ActiveWorkspace(); active != nil {
		if owner, ok := active.Content().(interface {
			SpriteContext() (*dmmap.Dmm, *dmenv.Dme)
		}); ok {
			scene, environment = owner.SpriteContext()
		} else if owner, ok := active.Content().(interface{ Map() *pmap.PaneMap }); ok && owner.Map() != nil {
			scene = owner.Map().ViewDmm()
		}
	}
	return scene, environment
}

func (w *WsArea) OpenSprite(doc *dmi.Document, state string, dir int) *wssprite.Workspace {
	scene, environment := w.spriteContext()
	var preview *wspreview.Preview
	if scene != nil && environment != nil {
		preview = wspreview.NewSprite(scene, environment)
	}
	content := wssprite.New(w.app, doc, preview)
	content.Select(state, dir)
	ws := workspace.New(content)
	w.addWorkspace(ws)
	ws.SetTriggerFocus(true)
	return content
}
