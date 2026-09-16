package wsship

import (
	"fmt"
	"path/filepath"
	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/ship"
	"sdmm/internal/util"
	"time"
)

type App interface {
	pmap.App
	OnWorkspaceSwitched()
}
type WsShip struct {
	recovery recoveryState
	workspace.Content
	isolated, invalid                bool
	app                              App
	catalog                          *ship.Catalog
	hull, theme, source              int
	selected                         map[string]string
	assembly                         *ship.Assembly
	project                          *ship.Project
	projects                         map[string]*ship.Project
	panes                            map[string]*pmap.PaneMap
	pane                             *pmap.PaneMap
	message                          string
	wizard, focused                  bool
	newID, newName, itemID, itemName string
	renameOriginal                   string
	itemDescription                  string
	descriptionOriginal              string
	width, height                    int32
	stage                            int
	task                             buildTask
	wizardStep, sizePreset           int
	customID                         bool
	areaPath, areaIcon               string
	emptyModule                      bool
	copyRooms                        bool // a new variant copies every room instead of sharing
	shipFilter                       string
	shipKind                         int // 0 every ship, 1 modular only, 2 fixed layouts only
	removalBackup                    string
	settings                         settingsForm
	dockOutward                      int
	reviewReady                      bool
	reviewed                         []reviewProject
	SourceBusy                       func(string) bool
	crew                             crewEditor
	crewVisual                       crewVisual
	costs                            costEditor
	itemCosts                        ship.PartCosts
	reshapeSlot, hoverRoom           string
	pendingDelete                    string // "slot/x" or "module/x" awaiting its second click
	pendingShown                     bool
	fixedConfirmed                   map[string]bool // hull types whose modular conversion was confirmed
	collapsedSections                map[roomSection]bool
	optionInfos                      map[string]optionInfo
	variantInfos                     map[string]variantInfo
	sharedRooms                      map[string]bool
	share                            editShare
}

func New(app App, busy ...func(string) bool) *WsShip {
	ws := &WsShip{app: app, projects: map[string]*ship.Project{}, panes: map[string]*pmap.PaneMap{}, width: 32, height: 32, sizePreset: 1}
	if len(busy) > 0 {
		ws.SourceBusy = busy[0]
	}
	var err error
	ws.catalog, err = ship.ReloadCatalog(app.LoadedEnvironment())
	if err != nil {
		ws.message = err.Error()
		return ws
	}
	ws.app.CommandStorage().SetStack(ws.CommandStackId())
	// Start with the ship itself visible. This is the normal View filter, so
	// the Areas checkbox and existing keyboard shortcut stay in sync.
	if app.PathsFilter().IsVisiblePath("/area") {
		app.PathsFilter().TogglePath("/area")
	}
	tools.SetEnabled(false)
	if app, ok := app.(interface{ ShipRecoveryDirectory() string }); ok {
		ws.startRecovery(app.ShipRecoveryDirectory())
	}
	return ws
}
func (ws *WsShip) Name() string {
	prefix := ""
	dirty := ws.app.CommandStorage().IsModified(ws.CommandStackId()) || (ws.task == taskCrew && ws.crew.dirty) || (ws.task == taskCosts && ws.costs.dirty) || ws.renamePending() || ws.settingsPending()
	for _, project := range ws.projects {
		for _, d := range project.Documents {
			if d.Active && !d.Existed {
				dirty = true
			}
		}
	}
	if dirty || ws.recovery.restored {
		prefix = "* "
	}
	return prefix + "Ship Workshop###" + ws.Id()
}
func (ws *WsShip) Title() string {
	if ws.project != nil {
		return "Ship Workshop - " + ws.project.Hull.Name
	}
	return "Ship Workshop"
}
func (ws *WsShip) Map() *pmap.PaneMap {
	if len(ws.recovery.pending) > 0 || ws.wizard || ws.invalid || ws.stage != stepBuild || ws.task == taskCrew || ws.task == taskCosts {
		return nil
	}
	return ws.pane
}
func (ws *WsShip) CommandStackId() string { return "ship:" + ws.Id() }
func (ws *WsShip) IsModified() bool {
	if ws.renamePending() || ws.settingsPending() {
		return true
	}
	if ws.task == taskCrew && ws.crew.dirty {
		return true
	}
	if ws.task == taskCosts && ws.costs.dirty {
		return true
	}
	for _, p := range ws.projects {
		if p.Modified() {
			return true
		}
	}
	return false
}
func (ws *WsShip) PreProcess() {
	ws.autosave(time.Now())
	for _, p := range ws.panes {
		p.SetShortcutsVisible(false)
	}
}
func (ws *WsShip) Focused() bool {
	return ws.Content.Focused() || (ws.pane != nil && ws.pane.Focused())
}
func (ws *WsShip) OnFocusChange(f bool) {
	ws.focused = f
	if !f && (tools.IsSelected(tools.TNRegion) || tools.IsSelected(tools.TNRoomShape)) {
		tools.SetSelected(tools.TNAdd)
	}
	if ws.pane != nil && !ws.wizard && ws.stage == stepBuild && ws.task != taskCrew && ws.task != taskCosts {
		if f && !ws.invalid {
			ws.pane.OnActivate()
		} else {
			ws.pane.OnDeactivate()
		}
	} else if f {
		tools.SetEnabled(false)
	}
}
func (ws *WsShip) Dispose() {
	ws.disposeRecovery()
	ws.endShape()
	if ws.pane != nil {
		ws.pane.OnDeactivate()
	}
	for _, p := range ws.panes {
		p.Dispose()
	}
	ws.app.CommandStorage().DisposeStack(ws.CommandStackId())
}
func (ws *WsShip) currentTheme() ship.Theme {
	if ws.catalog == nil || ws.hull < 0 || ws.hull >= len(ws.catalog.Hulls) {
		return ship.Theme{}
	}
	h := ws.catalog.Hulls[ws.hull]
	if len(h.Themes) == 0 {
		return ship.Theme{}
	}
	if ws.theme < 0 || ws.theme >= len(h.Themes) {
		ws.theme = 0
	}
	return h.Themes[ws.theme]
}
func (ws *WsShip) defaults() {
	ws.selected = map[string]string{}
	if ws.catalog == nil || ws.hull < 0 || ws.hull >= len(ws.catalog.Hulls) {
		return
	}
	h := ws.catalog.Hulls[ws.hull]
	t := ws.currentTheme()
	for _, slot := range h.SlotsFor(t) {
		for _, m := range h.Modules {
			if m.Slot == slot && m.Default && m.Available(t.ID) {
				ws.selected[slot] = m.ID
				break
			}
		}
	}
	ws.source = 0
}

func (ws *WsShip) rebuild() {
	if ws.catalog == nil {
		return
	}
	// Any failed transition must detach the previous ship's editing context.
	ws.invalid = true
	defer func() {
		if ws.invalid {
			ws.assembly = nil
			tools.SetEnabled(false)
			if ws.pane != nil {
				ws.pane.OnDeactivate()
			}
		}
	}()
	if ws.hull < 0 || ws.hull >= len(ws.catalog.Hulls) {
		ws.project = nil
		ws.message = "Choose a ship to edit."
		return
	}
	h := ws.catalog.Hulls[ws.hull]
	p := ws.projects[h.Type]
	if p == nil {
		var err error
		p, err = ship.OpenProject(ws.catalog, ws.app.LoadedEnvironment(), h)
		if err != nil {
			ws.project = nil
			ws.message = err.Error()
			return
		}
		ws.projects[h.Type] = p
	}
	ws.project = p
	p.BeforeOpen = func(file string) error {
		if ws.SourceBusy != nil && ws.SourceBusy(file) {
			return fmt.Errorf("close the ordinary map tab before opening %s in Ship Workshop", filepath.Base(file))
		}
		return nil
	}
	ws.catalog.Hulls[ws.hull] = p.Hull
	ws.optionInfos, ws.variantInfos, ws.sharedRooms, ws.share = nil, nil, nil, editShare{}
	ws.sanitizeSelection()
	a, err := p.Assemble(ws.currentTheme(), ws.selected)
	if err != nil {
		ws.message = err.Error()
		if !ws.isolated || ws.pane == nil {
			ws.invalid = true
			tools.SetEnabled(false)
			return
		}
		file := ws.pane.Dmm().Path.Absolute
		doc := p.Documents[file]
		if doc == nil || !doc.Active || doc.Map != ws.pane.Dmm() {
			ws.invalid = true
			tools.SetEnabled(false)
			return
		}
		a = &ship.Assembly{Sources: []ship.Source{{Name: "Source repair", File: file, Live: doc.Map, Data: ship.RawData(doc.Map)}}, MaxX: doc.Map.MaxX, MaxY: doc.Map.MaxY, MaxZ: 1}
		ws.source = 0
	}
	if ws.SourceBusy != nil {
		for _, s := range a.Sources {
			if ws.SourceBusy(s.File) {
				ws.message = "Close the ordinary map tab before editing this source in Ship Workshop: " + filepath.Base(s.File)
				return
			}
		}
	}
	var display *dmmap.Dmm
	if !ws.isolated {
		display, err = a.Display(ws.app.LoadedEnvironment())
		if err != nil {
			ws.message = err.Error()
			ws.invalid = true
			tools.SetEnabled(false)
			return
		}
	}
	ws.invalid = false
	if ws.focused && ws.stage == stepBuild && !ws.wizard && ws.task != taskCrew && ws.task != taskCosts {
		tools.SetEnabled(true)
	}
	ws.assembly = a
	if ws.source >= len(a.Sources) {
		ws.source = 0
	}
	for i, s := range a.Sources {
		pane := ws.panes[s.File]
		if pane == nil {
			pane = pmap.New(ws.app, s.Live)
			ws.panes[s.File] = pane
		}
		editable := map[uint64]*dmminstance.Instance{}
		for _, tile := range s.Live.Tiles {
			for _, inst := range tile.Instances() {
				editable[inst.Id()] = inst
			}
		}
		index, theme, hull := i, ws.theme, ws.hull
		selection := copySelection(ws.selected)
		view, offset := display, s.Offset
		if ws.isolated {
			view = s.Live
			offset = util.Point{}
		}
		pane.SetEditContext(&pmap.EditContext{View: view, Offset: offset, StackID: ws.CommandStackId(), Editable: editable, Refresh: ws.refresh, BeforeHistory: func() {
			ws.endShape()
			ws.stage = stepBuild
			ws.wizard = false
			ws.task = taskPaint
			ws.hull = hull
			ws.theme = theme
			ws.selected = copySelection(selection)
			ws.source = index
			ws.rebuild()
			ws.OnFocusChange(true)
		}, Filter: ws.visible, Overlay: ws.paintRooms})
	}
	ws.activate(ws.panes[a.Sources[ws.source].File])
	ws.pane.RenderContext()
	ws.message = ""
}
func (ws *WsShip) refresh() { ws.rebuild() }

// sanitizeSelection drops displayed options that no longer exist, falling
// back to the slot's default so removals and their redo still assemble.
func (ws *WsShip) sanitizeSelection() {
	if ws.project == nil {
		return
	}
	theme := ws.currentTheme().ID
	for slot, id := range ws.selected {
		if id == "" {
			continue
		}
		var fallback *ship.Module
		ok := false
		for i := range ws.project.Hull.Modules {
			m := &ws.project.Hull.Modules[i]
			if m.Slot != slot || !m.Available(theme) {
				continue
			}
			if m.ID == id {
				ok = true
				break
			}
			if m.Default || fallback == nil {
				fallback = m
			}
		}
		if ok {
			continue
		}
		ws.selected[slot] = ""
		if fallback != nil {
			ws.selected[slot] = fallback.ID
		}
	}
}
func copySelection(s map[string]string) map[string]string {
	r := map[string]string{}
	for k, v := range s {
		r[k] = v
	}
	return r
}
func (ws *WsShip) activate(p *pmap.PaneMap) {
	if ws.pane == p {
		if !p.Focused() && ws.focused && !ws.invalid && ws.stage == stepBuild && !ws.wizard && ws.task != taskCrew && ws.task != taskCosts {
			p.OnActivate()
		}
		return
	}
	if ws.pane != nil {
		p.Canvas().Render().Camera = ws.pane.Canvas().Render().Camera
		ws.pane.OnDeactivate()
	}
	ws.pane = p
	if ws.focused && ws.stage == stepBuild && !ws.wizard && ws.task != taskCrew && ws.task != taskCosts {
		p.OnActivate()
	}
	ws.app.OnWorkspaceSwitched()
}
func (ws *WsShip) visible(path string) bool {
	// Template placeholders are transparent; normal View controls govern all
	// map content, including areas, pipes and cables.
	if path == "/turf/template_noop" || path == "/area/template_noop" {
		return false
	}

	return true
}
func (ws *WsShip) Owns(file string) bool {
	for _, p := range ws.projects {
		for path, d := range p.Documents {
			if d.Active && util.SamePath(path, file) {
				return true
			}
		}
	}
	return false
}
func (ws *WsShip) FocusSource(file string) {
	if !ws.commitDraft() {
		return
	}
	ws.flush()
	ws.OnFocusChange(false)
	ws.stage = stepBuild
	ws.wizard = false
	ws.task = taskPaint
	for h, hull := range ws.catalog.Hulls {
		p := ws.projects[hull.Type]
		if p == nil {
			continue
		}
		owned := false
		for path, d := range p.Documents {
			if d.Active && util.SamePath(path, file) {
				file, owned = path, true
				break
			}
		}
		if !owned {
			continue
		}
		ws.hull = h
		for ti := 0; ti < max(1, len(hull.Themes)); ti++ {
			ws.theme = ti
			ws.defaults()
			theme := ws.currentTheme()
			for _, m := range p.Hull.Modules {
				if !m.Available(theme.ID) || !ship.Contains(p.Hull.SlotsFor(theme), m.Slot) {
					continue
				}
				path, err := p.ModuleSource(m, theme.ID)
				if err == nil && util.SamePath(path, file) {
					ws.selected[m.Slot] = m.ID
				}
			}
			ws.rebuild()
			if ws.assembly != nil {
				for i, s := range ws.assembly.Sources {
					if s.File == file {
						ws.source = i
						ws.rebuild()
						ws.OnFocusChange(true)
						return
					}
				}
			}
		}
	}
}
func (ws *WsShip) Save() bool {
	if !ws.commitDraft() {
		return false
	}
	ws.flush()
	projects := []*ship.Project{}
	for _, p := range ws.projects {
		projects = append(projects, p)
	}
	if err := ship.SaveProjects(projects); err != nil {
		ws.message = err.Error()
		return false
	}
	ws.app.CommandStorage().ForceBalance(ws.CommandStackId())
	ws.clearRecovery()
	ws.message = "Saved. Your ship files are up to date."
	ws.notifySaved()
	return true
}

func (ws *WsShip) notifySaved() {
	if app, ok := ws.app.(interface{ ShipFilesSaved() }); ok {
		app.ShipFilesSaved()
	}
}
func (ws *WsShip) flush() {
	ws.reviewReady = false
	tools.FinishStroke()
	for _, p := range ws.panes {
		p.Editor().CommitContextNow("Edit ship source")
	}
}
func (ws *WsShip) change(label string, action func() error) {
	ws.flush()
	p := ws.project
	before := p.Capture()
	h, t := ws.hull, ws.theme
	crewTask, crewScope, crewSelection := ws.task == taskCrew, ws.crew.scope, ws.crew.selected
	costTask, costScope := ws.task == taskCosts, "ship"
	if costTask && ws.costs.selected >= 0 {
		costScope = ws.costs.entries[ws.costs.selected].scope.ID
	}
	sel := copySelection(ws.selected)
	if err := action(); err != nil {
		p.Restore(before)
		ws.message = err.Error()
		return
	}
	after := p.Capture()
	afterSelection := copySelection(ws.selected)
	restore := func(state ship.State, selection map[string]string) {
		ws.endShape()
		ws.stage, ws.wizard, ws.task = stepBuild, false, taskPaint
		p.Restore(state)
		ws.hull = h
		ws.theme = t
		// Variants can be gone on the other side of an undo.
		if n := len(p.Hull.Themes); ws.theme >= n {
			ws.theme = max(0, n-1)
		}
		ws.selected = copySelection(selection)
		ws.source = 0
		ws.catalog.Hulls[h] = p.Hull
		if selected, ok := ws.app.SelectedPrefab(); ok && !p.AreaActive(selected.Path()) {
			if root, err := p.AreaRoot(ws.currentTheme()); err == nil {
				ws.app.DoSelectPrefab(dmmap.PrefabStorage.Initial(root))
			}
		}
		for _, pane := range ws.panes {
			pane.Snapshot().Sync()
			pane.CanvasState().SetMaxX(pane.Dmm().MaxX)
			pane.CanvasState().SetMaxY(pane.Dmm().MaxY)
		}
		ws.rebuild()
		if crewTask {
			ws.beginCrew()
			ws.loadCrewScope(crewScope)
			ws.crew.selected = min(crewSelection, len(ws.crew.jobs)-1)
		}
		if costTask {
			ws.beginCosts(costScope)
		}
		ws.OnFocusChange(true)
		tools.RefreshGrabSelection()
	}
	for _, pane := range ws.panes {
		pane.Snapshot().Sync()
		pane.CanvasState().SetMaxX(pane.Dmm().MaxX)
		pane.CanvasState().SetMaxY(pane.Dmm().MaxY)
	}
	ws.app.CommandStorage().PushV(ws.CommandStackId(), command.Make(label, func() { restore(before, sel) }, func() { restore(after, afterSelection) }))
	ws.catalog.Hulls[h] = p.Hull
	ws.rebuild()
	tools.RefreshGrabSelection()
}
