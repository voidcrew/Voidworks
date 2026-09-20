package wsship

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/recovery"
	"sdmm/internal/ship"
)

const autosaveInterval = 30 * time.Second

type recoveryState struct {
	root              string
	store             *recovery.Store
	pending           []recovery.Entry
	next, saved       time.Time
	writing           chan error
	hash, writingHash [32]byte
	error             string
	discard, restored bool
}

type recoveryForm struct {
	Task                                                                   buildTask
	ItemID, ItemName, ItemDescription, RenameOriginal, DescriptionOriginal string
	SettingsName, SettingsDescription                                      string
	SettingsCrew                                                           int32
	SettingsHidden                                                         bool
	SettingsCosts                                                          ship.PartCosts
	CrewScope                                                              string
	CrewJobs                                                               []ship.CrewJob
	CrewSelected                                                           int
	CrewDirty                                                              bool
	CostScope                                                              string
	CostValues                                                             ship.PartCosts
	CostDirty                                                              bool
}
type recoveryWorkspace struct {
	Version              int
	Projects             []json.RawMessage
	Current              string
	Theme, Source, Stage int
	Selected             map[string]string
	Isolated             bool
	Form                 recoveryForm
}

func (ws *WsShip) startRecovery(root string) {
	if root == "" {
		return
	}
	r := &ws.recovery
	r.root = root
	var err error
	r.store, err = recovery.Open(root, ws.app.LoadedEnvironment().RootFile)
	if err != nil {
		r.error = err.Error()
		return
	}
	r.next = time.Now().Add(autosaveInterval)
	ws.findRecovery()
}
func (ws *WsShip) findRecovery() {
	r := &ws.recovery
	if r.store == nil || len(r.pending) > 0 {
		return
	}
	var err error
	r.pending, err = recovery.Pending(r.root, ws.app.LoadedEnvironment().RootFile)
	if err != nil {
		r.error = err.Error()
	}
}
func (ws *WsShip) captureRecovery() ([]byte, []string, error) {
	s := recoveryWorkspace{Version: 1, Theme: ws.theme, Source: ws.source, Stage: ws.stage, Selected: ws.selected, Isolated: ws.isolated}
	if ws.project != nil {
		s.Current = ws.project.Hull.Type
	}
	var names, ids []string
	for id := range ws.projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if s.Current == "" && len(ids) > 0 {
		s.Current = ids[0]
		s.Theme = 0
		s.Source = 0
		s.Stage = stepBuild
	}
	for _, id := range ids {
		p := ws.projects[id]
		data, err := p.CaptureRecovery()
		if err != nil {
			return nil, nil, err
		}
		s.Projects = append(s.Projects, data)
		names = append(names, p.Hull.Name)
	}
	s.Form = recoveryForm{Task: ws.task, ItemID: ws.itemID, ItemName: ws.itemName, ItemDescription: ws.itemDescription, RenameOriginal: ws.renameOriginal, DescriptionOriginal: ws.descriptionOriginal,
		SettingsName: ws.settings.name, SettingsDescription: ws.settings.description, SettingsCrew: ws.settings.crew, SettingsHidden: ws.settings.hidden, SettingsCosts: ws.settings.costs,
		CrewScope: ws.crew.scope, CrewJobs: ws.crew.jobs, CrewSelected: ws.crew.selected, CrewDirty: ws.crew.dirty,
		CostValues: ws.costs.values, CostDirty: ws.costs.dirty}
	if ws.costs.selected >= 0 && ws.costs.selected < len(ws.costs.entries) {
		s.Form.CostScope = ws.costs.entries[ws.costs.selected].scope.ID
	}
	data, err := json.Marshal(s)
	return data, names, err
}
func (ws *WsShip) finishAutosave(wait bool) {
	r := &ws.recovery
	if r.writing == nil {
		return
	}
	var err error
	if wait {
		err = <-r.writing
	} else {
		select {
		case err = <-r.writing:
		default:
			return
		}
	}
	r.writing = nil
	if err != nil {
		r.error = err.Error()
		return
	}
	r.error = ""
	r.saved = time.Now()
	r.hash = r.writingHash
}

// Called even while the workshop's tab is hidden. Capture stays on the UI
// thread; compression and disk writes use only the immutable captured bytes.
func (ws *WsShip) autosave(now time.Time) {
	r := &ws.recovery
	ws.finishAutosave(false)
	if r.store == nil || r.writing != nil || now.Before(r.next) || len(r.pending) > 0 {
		return
	}
	r.next = now.Add(autosaveInterval)
	if !ws.IsModified() {
		ws.clearRecovery()
		return
	}
	data, names, err := ws.captureRecovery()
	if err != nil {
		r.error = err.Error()
		return
	}
	sum := sha256.Sum256(data)
	if sum == r.hash {
		return
	}
	r.writingHash = sum
	result := make(chan error, 1)
	r.writing = result
	store := r.store
	go func() { result <- store.Save(data, names, now) }()
}
func (ws *WsShip) clearRecovery() {
	r := &ws.recovery
	ws.finishAutosave(true)
	if r.store != nil {
		if err := r.store.Clear(); err != nil {
			r.error = err.Error()
			return
		}
	}
	r.hash = [32]byte{}
	r.saved = time.Time{}
	r.restored = false
}
func (ws *WsShip) disposeRecovery() {
	ws.clearRecovery()
	r := &ws.recovery
	r.store.Close()
	for _, entry := range r.pending {
		entry.Store.Close()
	}
	r.pending = nil
}
func (ws *WsShip) restoreRecovery(entry recovery.Entry) error {
	if entry.Snapshot == nil {
		return fmt.Errorf("this recovery copy could not be read")
	}
	if ws.IsModified() {
		return fmt.Errorf("save your current edits before recovering another session")
	}
	var s recoveryWorkspace
	if err := json.Unmarshal(entry.Snapshot.Data, &s); err != nil {
		return err
	}
	if s.Version != 1 || len(s.Projects) == 0 {
		return fmt.Errorf("unsupported or empty recovery copy")
	}
	projects := map[string]*ship.Project{}
	for _, data := range s.Projects {
		p, err := ship.RecoverProject(ws.catalog, ws.app.LoadedEnvironment(), data)
		if err != nil {
			return err
		}
		if projects[p.Hull.Type] != nil {
			return fmt.Errorf("duplicate recovered ship")
		}
		for path, d := range p.Documents {
			if d.Active && ws.SourceBusy != nil && ws.SourceBusy(path) {
				return fmt.Errorf("close the ordinary map tab before recovering this ship")
			}
		}
		projects[p.Hull.Type] = p
	}
	if projects[s.Current] == nil {
		return fmt.Errorf("the selected ship is missing from the recovery copy")
	}
	// Transfer to this live session before retiring the interrupted session's copy.
	ws.finishAutosave(true)
	if err := ws.recovery.store.Save(entry.Snapshot.Data, entry.Snapshot.Names, time.Now()); err != nil {
		return err
	}
	ws.OnFocusChange(false)
	ws.endShape()
	for _, pane := range ws.panes {
		pane.Dispose()
	}
	ws.panes = map[string]*pmap.PaneMap{}
	ws.pane = nil
	ws.assembly = nil
	ws.projects = projects
	for id, p := range projects {
		found := false
		for i, h := range ws.catalog.Hulls {
			if h.Type == id {
				ws.catalog.Hulls[i] = p.Hull
				found = true
				break
			}
		}
		if !found {
			ws.catalog.Hulls = append(ws.catalog.Hulls, p.Hull)
		}
	}
	for i, h := range ws.catalog.Hulls {
		if h.Type == s.Current {
			ws.hull = i
			break
		}
	}
	ws.project = projects[s.Current]
	ws.theme, ws.source, ws.stage = max(0, s.Theme), max(0, s.Source), s.Stage
	if ws.stage < stepChoose || ws.stage > stepReview {
		ws.stage = stepBuild
	}
	ws.selected, ws.isolated = s.Selected, s.Isolated
	if ws.selected == nil {
		ws.selected = map[string]string{}
	}
	ws.task, ws.wizard = taskPaint, false
	ws.app.CommandStorage().DisposeStack(ws.CommandStackId())
	ws.app.CommandStorage().SetStack(ws.CommandStackId())
	ws.rebuild()
	f := s.Form
	switch f.Task {
	case taskCrew:
		ws.beginCrew()
		ws.loadCrewScope(f.CrewScope)
		ws.crew.jobs, ws.crew.dirty, ws.crew.selected = f.CrewJobs, f.CrewDirty, f.CrewSelected
		ws.crew.selected = min(ws.crew.selected, len(ws.crew.jobs)-1)
	case taskCosts:
		ws.beginCosts(f.CostScope)
		ws.costs.values, ws.costs.dirty = f.CostValues, f.CostDirty
	case taskSettings, taskRenameTheme, taskRenameModule, taskRenameRoom, taskRenameShip, taskRenameShipFiles:
		ws.task = f.Task
		ws.settings = settingsForm{f.SettingsName, f.SettingsDescription, f.SettingsCrew, f.SettingsHidden, f.SettingsCosts}
		ws.itemID, ws.itemName, ws.itemDescription = f.ItemID, f.ItemName, f.ItemDescription
		ws.renameOriginal, ws.descriptionOriginal = f.RenameOriginal, f.DescriptionOriginal
	}
	ws.recovery.restored = true
	ws.recovery.saved = time.Now()
	ws.recovery.next = time.Now().Add(autosaveInterval)
	ws.message = "Recovered unsaved work. Review your ships, then Save to keep the changes."
	ws.OnFocusChange(true)
	if err := entry.Store.Discard(); err != nil {
		ws.recovery.error = err.Error()
	}
	entry.Store.Close()
	return nil
}

func (ws *WsShip) recoveryPrompt() bool {
	r := &ws.recovery
	if len(r.pending) == 0 {
		return false
	}
	entry := r.pending[0]
	workshop.Banner("Ship Workshop", "Recover unsaved work", style.Teal)
	imgui.TextWrapped("Unsaved ship edits were found from an interrupted session.")
	if entry.Snapshot != nil {
		imgui.TextWrapped(strings.Join(entry.Snapshot.Names, ", "))
		imgui.Text("Saved " + entry.Snapshot.Created.Local().Format("Jan 2, 15:04:05"))
		imgui.TextWrapped("Recover opens these drafts for review. Save writes your changes to the project. Files edited outside Voidworks are checked when you save.")
	} else {
		imgui.TextWrapped("The recovery copy could not be read: " + entry.Err.Error())
	}
	if r.error != "" {
		imgui.TextWrapped(r.error)
	}
	if r.discard {
		imgui.TextWrapped("Discard this session's recovery copies? This cannot be undone.")
		if imgui.Button("Discard recovery copies") {
			if err := entry.Store.Discard(); err != nil {
				r.error = err.Error()
			} else {
				r.pending = r.pending[1:]
				r.discard = false
				r.error = ""
			}
		}
		imgui.SameLine()
		if imgui.Button("Keep copies") {
			r.discard = false
		}
		return true
	}
	busy := ws.IsModified()
	if busy {
		imgui.TextWrapped("Save your current edits before recovering another session. Choose Later to return to your work.")
	}
	imgui.BeginDisabledV(entry.Snapshot == nil || busy)
	if imgui.Button("Recover unsaved ships") {
		if err := ws.restoreRecovery(entry); err != nil {
			r.error = err.Error()
		} else {
			r.pending = r.pending[1:]
			r.discard = false
		}
	}
	imgui.EndDisabled()
	imgui.SameLine()
	if imgui.Button("Later") {
		entry.Store.Close()
		r.pending = r.pending[1:]
		r.discard = false
	}
	imgui.SameLine()
	if imgui.Button("Discard...") {
		r.discard = true
	}
	return true
}
func (ws *WsShip) recoveryStatus() {
	r := &ws.recovery
	if r.root == "" {
		return
	}
	if r.error != "" {
		imgui.TextWrapped("Autosave needs attention: " + r.error)
	} else if r.writing != nil {
		hint("Saving recovery copy...")
	} else if !r.saved.IsZero() {
		hint("Recovery copy saved at " + r.saved.Format("15:04:05"))
	} else {
		hint("Recovery copies every 30 seconds")
	}
	if r.store == nil {
		if imgui.Button("Retry autosave") {
			ws.startRecovery(r.root)
		}
		return
	}
	if imgui.Button("Recover unsaved work...") {
		ws.findRecovery()
		if len(r.pending) == 0 {
			ws.message = "No interrupted ship sessions were found for this project."
		}
	}
}
