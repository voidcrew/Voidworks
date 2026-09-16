package app

import (
	"github.com/rs/zerolog/log"
	"path/filepath"
	"time"

	"sdmm/internal/app/ui/cpwsarea/wssprite"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmi"
)

func (a *app) DoNewDMI() {
	doc, e := dmi.Create(32, 32)
	if e != nil {
		return
	}
	a.layout.WsArea.OpenSprite(doc, "", 2)
}

func (a *app) DoRecoverDMI() {
	wssprite.ShowRecovery(func(doc *dmi.Document) *wssprite.Workspace {
		return a.layout.WsArea.OpenSprite(doc, "", 2)
	})
}
func (a *app) openDMI(path, state string, dir int) {
	started := time.Now()
	log.Debug().Str("file", path).Msg("opening sprite editor")
	defer func() { log.Debug().Dur("duration_ms", time.Since(started)).Msg("sprite editor opened") }()
	if a.layout.WsArea.FocusSprite(path, state, dir) {
		return
	}
	doc, e := dmi.Load(path)
	if e != nil {
		dialog.Open(dialog.TypeInformation{Title: "Cannot open DMI", Information: e.Error()})
		return
	}
	a.layout.WsArea.OpenSprite(doc, state, dir)
}
func (a *app) DoEditSprite(prefab *dmmprefab.Prefab) {
	if prefab == nil || a.LoadedEnvironment() == nil {
		return
	}
	path := prefab.Vars().TextV("icon", "")
	if path == "" {
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.LoadedEnvironment().RootDir, path)
	}
	a.openDMI(path, prefab.Vars().TextV("icon_state", ""), prefab.Vars().IntV("dir", 2))
}
func (a *app) activeSprite() *wssprite.Workspace {
	ws := a.layout.WsArea.ActiveWorkspace()
	if ws == nil {
		return nil
	}
	sprite, _ := ws.Content().(*wssprite.Workspace)
	return sprite
}
func (a *app) CanPaste() bool { return a.activeSprite() != nil || a.Clipboard().HasData() }
