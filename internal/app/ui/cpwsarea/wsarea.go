package cpwsarea

import (
	"fmt"

	"sdmm/internal/app/config"
	"sdmm/internal/app/ui/component"
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"sdmm/internal/app/ui/cpwsarea/wschangelog"
	"sdmm/internal/app/ui/cpwsarea/wscreatemap"
	"sdmm/internal/app/ui/cpwsarea/wsempty"
	"sdmm/internal/app/ui/cpwsarea/wsmap"
	"sdmm/internal/app/ui/cpwsarea/wsplanet"
	"sdmm/internal/app/ui/cpwsarea/wsprefs"
	"sdmm/internal/app/ui/cpwsarea/wspreview"
	"sdmm/internal/app/ui/cpwsarea/wsruin"
	"sdmm/internal/app/ui/cpwsarea/wsship"
	"sdmm/internal/app/ui/cpwsarea/wssprite"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/planet"
	"sdmm/internal/rsc"
	"sdmm/internal/util"

	"sdmm/internal/app/command"
	"sdmm/internal/dmapi/dmmap"

	"github.com/rs/zerolog/log"
)

type App interface {
	wsempty.App
	wsmap.App
	wscreatemap.App
	wschangelog.App
	wsruin.App

	DoClose()

	OnWorkspaceSwitched()
	CommandStorage() *command.Storage
	IsLayoutReset() bool

	ConfigRegister(config.Config)
	ConfigFind(name string) config.Config
}

type WsArea struct {
	component.Component

	app App

	activeWs  *workspace.Workspace
	focusedWs *workspace.Workspace

	activeWsContentId string

	workspaces []*workspace.Workspace
}

func (w *WsArea) Init(app App) {
	w.app = app
	w.loadConfig()
	w.AddEmptyWorkspaceIfNone()
}

func (w *WsArea) Free() {
	for _, ws := range append([]*workspace.Workspace(nil), w.workspaces...) {
		if _, ok := ws.Content().(*wssprite.Workspace); ok {
			w.closeWorkspace(ws)
			continue
		}
		if _, ok := ws.Content().(*wsplanet.Workspace); ok {
			w.closeWorkspace(ws)
			continue
		}
		if _, ok := ws.Content().(*wspreview.Preview); ok {
			w.closeWorkspace(ws)
			continue
		}
		if _, ok := ws.Content().(*wsruin.WsRuin); ok {
			w.closeWorkspace(ws)
			continue
		}
		if _, ok := ws.Content().(*wsship.WsShip); ok {
			w.closeWorkspace(ws)
		}
	}
	w.closeWorkspaces(w.findMapWorkspaces())
	w.closeWorkspaces(w.findCreateMapWorkspaces()) // close new map creation as well
	log.Print("workspace area free")
}

func (w *WsArea) OpenPreview(source *dmmap.Dmm) {
	ws := workspace.New(wspreview.New(source, w.app.LoadedEnvironment()))
	w.addWorkspace(ws)
	ws.SetTriggerFocus(true)
}

func (w *WsArea) OpenPlanet() *wsplanet.Workspace {
	for _, ws := range w.workspaces {
		if content, ok := ws.Content().(*wsplanet.Workspace); ok {
			ws.SetTriggerFocus(true)
			return content
		}
	}
	content := wsplanet.New(w.app)
	ws := workspace.New(content)
	w.addWorkspace(ws)
	ws.SetTriggerFocus(true)
	return content
}

func (w *WsArea) OpenShip() *wsship.WsShip {
	for _, ws := range w.workspaces {
		if content, ok := ws.Content().(*wsship.WsShip); ok {
			ws.SetTriggerFocus(true)
			return content
		}
	}
	content := wsship.New(w.app, func(file string) bool {
		for _, ws := range w.workspaces {
			if m, ok := ws.Content().(*wsmap.WsMap); ok && util.SamePath(m.Map().Dmm().Path.Absolute, file) {
				return true
			}
		}
		return false
	})
	ws := workspace.New(content)
	w.addWorkspace(ws)
	ws.SetTriggerFocus(true)
	return content
}

func (w *WsArea) OpenRuin() *wsruin.WsRuin {
	for _, ws := range w.workspaces {
		if content, ok := ws.Content().(*wsruin.WsRuin); ok {
			ws.SetTriggerFocus(true)
			return content
		}
	}
	content := wsruin.New(w.app)
	content.LiveMap = func(file string) *dmmap.Dmm {
		for _, ws := range w.workspaces {
			if m, ok := ws.Content().(*wsmap.WsMap); ok && util.SamePath(m.Map().Dmm().Path.Absolute, file) {
				return m.Map().Dmm()
			}
		}
		return nil
	}
	content.PlanetDrafts = func() []planet.State {
		for _, ws := range w.workspaces {
			if p, ok := ws.Content().(*wsplanet.Workspace); ok {
				return p.PreviewStates()
			}
		}
		return nil
	}
	content.SourceBusy = func(file string) bool {
		for _, ws := range w.workspaces {
			if m, ok := ws.Content().(*wsmap.WsMap); ok && util.SamePath(m.Map().Dmm().Path.Absolute, file) {
				return true
			}
			if s, ok := ws.Content().(*wsship.WsShip); ok && s.Owns(file) {
				return true
			}
		}
		return false
	}
	ws := workspace.New(content)
	w.addWorkspace(ws)
	ws.SetTriggerFocus(true)
	return content
}

func (w *WsArea) OpenPreferences(prefsView wsprefs.Prefs) {
	for _, ws := range w.workspaces {
		if _, ok := ws.Content().(*wsprefs.WsPrefs); ok {
			ws.SetTriggerFocus(true)
			return
		}
	}
	w.addWorkspace(workspace.New(wsprefs.New(prefsView)))
}

func (w *WsArea) OpenChangelog() {
	for _, ws := range w.workspaces {
		if _, ok := ws.Content().(*wschangelog.WsChangelog); ok {
			ws.SetTriggerFocus(true)
			return
		}
	}
	w.addWorkspace(workspace.New(wschangelog.New(w.app)))
}

func (w *WsArea) OpenCreateMap() {
	for _, ws := range w.workspaces {
		if _, ok := ws.Content().(*wscreatemap.WsCreateMap); ok {
			ws.SetTriggerFocus(true)
			return
		}
	}

	wsCnt := wscreatemap.New(w.app)
	ws := workspace.New(wsCnt)

	w.addWorkspace(ws)
}

func (w *WsArea) OpenMap(dmm *dmmap.Dmm, ws *workspace.Workspace) bool {
	if wsMap, ok := w.findMapWorkspace(dmm.Path); ok {
		wsMap.SetTriggerFocus(true)
		return false
	}

	wsCnt := wsmap.New(w.app, dmm)
	if ws != nil {
		ws.SetContent(wsCnt)
	} else {
		ws = workspace.New(wsCnt)
		w.addWorkspace(ws)
	}
	ws.SetTriggerFocus(true)

	return true
}

func (w *WsArea) Close() {
	if w.activeWs != nil {
		w.closeWorkspaceGently(w.activeWs)
	}
}

func (w *WsArea) CloseAll() {
	w.closeWorkspacesGently(w.workspaces)
}

func (w *WsArea) CloseAllMaps(callback func(closed bool)) {
	log.Print("closing all maps...")
	w.closeWorkspacesGentlyV(w.findMapWorkspaces(), callback)
}

func (w *WsArea) CloseAllCreateMaps() {
	log.Print("closing all new maps...")
	w.closeWorkspaces(w.findCreateMapWorkspaces())
}

func (w *WsArea) WorkspaceTitle() string {
	if w.activeWs != nil {
		return w.activeWs.Title()
	}
	return ""
}

func (w *WsArea) ActiveWorkspace() *workspace.Workspace {
	return w.activeWs
}

func (w *WsArea) MapWorkspaces() []*workspace.Workspace {
	return w.findMapWorkspaces()
}

func (w *WsArea) addWorkspace(ws *workspace.Workspace) {
	w.addWorkspaceV(ws, len(w.workspaces))
}

func (w *WsArea) addWorkspaceV(ws *workspace.Workspace, idx int) {
	ws.Initialize()
	w.workspaces = append(w.workspaces[:idx], append([]*workspace.Workspace{ws}, w.workspaces[idx:]...)...)
	log.Printf("workspace opened in index [%d]: %s", idx, ws.Name())
}

func (w *WsArea) closeWorkspacesGently(wsToClose []*workspace.Workspace) {
	w.closeWorkspacesGentlyV(wsToClose, nil)
}

func (w *WsArea) closeWorkspacesGentlyV(wsToClose []*workspace.Workspace, callback func(closed bool)) {
	var unsavedWorkspaces []*workspace.Workspace
	for _, ws := range wsToClose {
		if w.isWorkspaceUnsaved(ws) {
			unsavedWorkspaces = append(unsavedWorkspaces, ws)
		}
	}

	if len(unsavedWorkspaces) == 0 {
		w.closeWorkspaces(wsToClose)
		if callback != nil {
			callback(true)
		}
		return
	}

	dialog.Open(w.makeCloseWorkspacesDialog(wsToClose, unsavedWorkspaces, callback))
}

func (w *WsArea) makeCloseWorkspacesDialog(wsToClose, unsavedWorkspaces []*workspace.Workspace, callback func(bool)) dialog.TypeConfirmation {
	var dType dialog.TypeConfirmation
	if len(unsavedWorkspaces) > 1 {
		dType = makeSaveMultipleWorkspacesDialogType(unsavedWorkspaces)
	} else {
		dType = makeSaveSingleWorkspaceDialogType(unsavedWorkspaces[0])
	}

	dType.ActionYes = func() {
		for _, ws := range unsavedWorkspaces {
			// Closing must not drop a workshop's hidden drafts.
			if !ws.SaveAll() {
				if callback != nil {
					callback(false)
				}
				return
			}
		}
		w.closeWorkspaces(wsToClose)
		if callback != nil {
			callback(true)
		}
	}
	dType.ActionNo = func() {
		for _, ws := range unsavedWorkspaces {
			w.discardWorkspaceChanges(ws)
		}
		w.closeWorkspaces(wsToClose)
		if callback != nil {
			callback(true)
		}
	}
	dType.ActionCancel = func() {
		if callback != nil {
			callback(false)
		}
	}

	return dType
}

func (w *WsArea) closeWorkspaces(wsToClose []*workspace.Workspace) {
	workspaces := make([]*workspace.Workspace, len(wsToClose))
	copy(workspaces, wsToClose)
	for _, ws := range workspaces {
		w.closeWorkspace(ws)
	}
}

func (w *WsArea) closeWorkspaceGently(ws *workspace.Workspace) {
	w.closeWorkspaceGentlyV(ws, nil)
}

func (w *WsArea) closeWorkspaceGentlyV(ws *workspace.Workspace, callback func(closed bool)) {
	if !w.isWorkspaceUnsaved(ws) {
		w.closeWorkspace(ws)
		return
	}

	dialog.Open(w.makeCloseWorkspaceDialog(ws, callback))
}

func (w *WsArea) makeCloseWorkspaceDialog(ws *workspace.Workspace, callback func(closed bool)) dialog.TypeConfirmation {
	dType := makeSaveSingleWorkspaceDialogType(ws)
	dType.ActionYes = func() {
		if !ws.SaveAll() {
			if callback != nil {
				callback(false)
			}
			return
		}
		w.closeWorkspace(ws)
		if callback != nil {
			callback(true)
		}
	}
	dType.ActionNo = func() {
		w.discardWorkspaceChanges(ws)
		w.closeWorkspace(ws)
		if callback != nil {
			callback(true)
		}
	}
	dType.ActionCancel = func() {
		if callback != nil {
			callback(false)
		}
	}

	return dType
}

func (w *WsArea) discardWorkspaceChanges(ws *workspace.Workspace) {
	if document, ok := ws.Content().(interface{ DiscardChanges() }); ok {
		document.DiscardChanges()
		return
	}
	w.app.CommandStorage().Balance(ws.CommandStackId())
}

func makeSaveSingleWorkspaceDialogType(ws *workspace.Workspace) dialog.TypeConfirmation {
	return dialog.TypeConfirmation{
		Title:    "Save Workspace?",
		Question: fmt.Sprintf("Workspace \"%s\" has been modified. Save changes?", ws.Title()),
	}
}

func makeSaveMultipleWorkspacesDialogType(workspaces []*workspace.Workspace) dialog.TypeConfirmation {
	var wsNames string
	for _, ws := range workspaces {
		wsNames += " - " + ws.Title() + "\n"
	}
	return dialog.TypeConfirmation{
		Title:    "Save All Workspaces?",
		Question: fmt.Sprintf("You have multiple unsaved workspaces:\n%sSave changes?", wsNames),
	}
}

func (w *WsArea) closeWorkspace(ws *workspace.Workspace) {
	if idx := w.findWorkspaceIdx(ws); idx != -1 {
		w.closeWorkspaceByIdx(idx)
	}
}

func (w *WsArea) closeWorkspaceByIdx(idx int) {
	ws := w.workspaces[idx]
	w.workspaces = append(w.workspaces[:idx], w.workspaces[idx+1:]...)
	ws.Dispose()
	log.Printf("workspace closed in idx [%d]: %s", idx, ws.Name())
	w.app.CommandStorage().DisposeStack(ws.CommandStackId())
}

func (w *WsArea) AddEmptyWorkspace() {
	wsCnt := wsempty.New(w.app)
	ws := workspace.New(wsCnt)
	w.addWorkspace(ws)
}

func (w *WsArea) AddEmptyWorkspaceIfNone() {
	if w.findEmptyWorkspaceIdx() == -1 {
		w.AddEmptyWorkspace()
	}
}

func (w *WsArea) findWorkspaceIdx(ws *workspace.Workspace) int {
	for idx, lws := range w.workspaces {
		if lws == ws {
			return idx
		}
	}
	return -1
}

func (w *WsArea) findMapWorkspace(path dmmap.DmmPath) (*workspace.Workspace, bool) {
	for _, ws := range w.workspaces {
		if ship, ok := ws.Content().(*wsship.WsShip); ok && ship.Owns(path.Absolute) {
			ship.FocusSource(path.Absolute)
			return ws, true
		}
		if wsCnt, ok := ws.Content().(*wsmap.WsMap); ok {
			if wsCnt.Map().Dmm().Path == path {
				return ws, true
			}
		}
	}
	return nil, false
}

func (w *WsArea) findMapWorkspaces() []*workspace.Workspace {
	var workspaces []*workspace.Workspace
	for _, ws := range w.workspaces {
		if _, ok := ws.Content().(*wssprite.Workspace); ok {
			workspaces = append(workspaces, ws)
			continue
		}
		if _, ok := ws.Content().(*wsplanet.Workspace); ok {
			workspaces = append(workspaces, ws)
			continue
		}
		if _, ok := ws.Content().(*wsruin.WsRuin); ok {
			workspaces = append(workspaces, ws)
			continue
		}
		if _, ok := ws.Content().(*wsship.WsShip); ok {
			workspaces = append(workspaces, ws)
			continue
		}
		if _, ok := ws.Content().(*wsmap.WsMap); ok {
			workspaces = append(workspaces, ws)
		}
	}
	return workspaces
}

func (w *WsArea) findCreateMapWorkspaces() []*workspace.Workspace {
	var workspaces []*workspace.Workspace
	for _, ws := range w.workspaces {
		if _, ok := ws.Content().(*wscreatemap.WsCreateMap); ok {
			workspaces = append(workspaces, ws)
		}
	}
	return workspaces
}

func (w *WsArea) findEmptyWorkspaceIdx() int {
	for idx, ws := range w.workspaces {
		if _, ok := ws.Content().(*wsempty.WsEmpty); ok {
			return idx
		}
	}
	return -1
}

func (w *WsArea) switchFocusedWorkspace(focusedWs *workspace.Workspace) {
	if w.focusedWs != focusedWs {
		log.Print("switch focused workspace:", focusedWs)

		if focusedWs != nil {
			log.Print("focused workspace content:", focusedWs.Content().Id())
		}

		if w.focusedWs != nil {
			w.focusedWs.OnFocusChange(false)
		}

		w.focusedWs = focusedWs

		if w.focusedWs != nil {
			w.focusedWs.OnFocusChange(true)
		}
	}
}

func (w *WsArea) switchActiveWorkspace(activeWs *workspace.Workspace) {
	if w.activeWs != activeWs || (activeWs != nil && w.activeWsContentId != activeWs.Content().Id()) {
		log.Print("switch active workspace:", activeWs)

		if activeWs != nil {
			log.Print("active workspace content:", activeWs.Content().Id())
		}

		w.activeWs = activeWs

		if activeWs != nil {
			w.activeWsContentId = activeWs.Content().Id()
		} else {
			w.activeWsContentId = ""
		}

		w.app.OnWorkspaceSwitched()

		if activeWs == nil {
			w.app.CommandStorage().SetStack(command.NullSpaceStackId)
		} else {
			w.app.CommandStorage().SetStack(w.activeWs.CommandStackId())
		}
	}
}

func (w *WsArea) isWorkspaceUnsaved(ws *workspace.Workspace) bool {
	if content, ok := ws.Content().(interface{ IsModified() bool }); ok {
		return content.IsModified()
	}
	if planet, ok := ws.Content().(*wsplanet.Workspace); ok {
		return planet.IsModified()
	}
	if ruin, ok := ws.Content().(*wsruin.WsRuin); ok {
		return ruin.IsModified()
	}
	if ship, ok := ws.Content().(*wsship.WsShip); ok {
		return ship.IsModified()
	}
	return w.app.CommandStorage().IsModified(ws.CommandStackId())
}

func (w *WsArea) isChangelogHashModified() bool {
	if hash := util.Djb2(rsc.ChangelogMd); hash != w.config().LastChangelogHash {
		w.config().LastChangelogHash = hash
		return true
	}
	return false
}
