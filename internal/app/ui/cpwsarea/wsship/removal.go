package wsship

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/ship"
)

func (ws *WsShip) removalBlocker(plan *ship.RemovalPlan) error {
	for _, change := range plan.Changes {
		if strings.EqualFold(filepath.Ext(change.Path), ".dmm") && ws.SourceBusy != nil && ws.SourceBusy(change.Path) {
			return fmt.Errorf("close the open map tab for %s before removing this ship", filepath.Base(change.Path))
		}
	}
	for id, p := range ws.projects {
		if !plan.RemovesType(id) && p.Modified() {
			return fmt.Errorf("save changes to the other ships before removing this one")
		}
	}
	return nil
}

func (ws *WsShip) requestRemoval(h ship.Hull) *workshop.RemovalConfirmation {
	if ws.project != nil && ws.project.Hull.Type != h.Type && !strings.HasPrefix(ws.project.Hull.Type, h.Type+"/") && !ws.commitDraft() {
		return nil
	}
	ws.flush()
	var others []*ship.Project
	for id, p := range ws.projects {
		if id != h.Type && !strings.HasPrefix(id, h.Type+"/") && p.Modified() {
			others = append(others, p)
		}
	}
	if len(others) > 0 {
		dialog.Open(dialog.TypeConfirmation{Title: "Save other ships first?", Question: "Save changes to the other ships before reviewing removal? Changes to " + h.Name + " will not be saved.", ActionYes: func() {
			written, err := ship.SaveProjectsReporting(others)
			if err != nil {
				ws.message = err.Error()
				return
			}
			ws.notifySaved(written...)
			ws.requestRemoval(h)
		}, ActionCancel: func() {}})
		return nil
	}
	if ws.project != nil && ws.project.Hull.Type == h.Type {
		h = ws.project.Hull
	}
	id := strings.ReplaceAll(strings.TrimPrefix(h.Type, ship.HullType+"/"), "/", "__")
	_, err := os.Stat(filepath.Join(ws.catalog.Root, "voidcrew/mapping/ship_projects", id+".ship.json"))
	draft := os.IsNotExist(err) && ws.projects[h.Type] != nil && ws.projects[h.Type].Settings != nil
	dme := ws.app.LoadedEnvironment()
	snapshot := ship.SnapshotRemovalEnvironment(dme)
	catalog := ws.catalog.RemovalSnapshot()
	h = h.RemovalSnapshot()
	r := &workshop.RemovalConfirmation{Kind: "ship", Entry: h.Name, Warning: "Unsaved changes to this ship will be discarded. Ship Workshop undo history will be cleared."}
	r.Prepare(func() (*ship.RemovalPlan, error) { return catalog.Removal(snapshot, h, draft) })
	r.Check = func() error {
		if ws.app.LoadedEnvironment() != dme {
			return fmt.Errorf("the loaded project changed; review removal again")
		}
		return ws.removalBlocker(r.Plan)
	}
	r.Removed = ws.finishRemoval
	workshop.OpenRemoval(r)
	return r
}

func (ws *WsShip) finishRemoval(plan *ship.RemovalPlan, backup string) {
	ws.OnFocusChange(false)
	// Keep saved session hulls: registrations authored since the last DME load
	// may not yet be represented in the parser's object catalog.
	var hulls []ship.Hull
	for _, h := range ws.catalog.Hulls {
		if plan.RemovesType(h.Type) {
			continue
		}
		if p := ws.projects[h.Type]; p != nil {
			h = p.Hull
		}
		hulls = append(hulls, h)
	}
	ws.catalog.Hulls = hulls
	for _, pane := range ws.panes {
		pane.Dispose()
	}
	ws.panes = map[string]*pmap.PaneMap{}
	ws.pane, ws.assembly, ws.project = nil, nil, nil
	ws.projects = map[string]*ship.Project{}
	ws.crew = crewEditor{}
	ws.costs = costEditor{}
	ws.reviewed = nil
	ws.reviewReady = false
	ws.wizard, ws.invalid, ws.isolated = false, false, false
	ws.stage, ws.task, ws.hull, ws.theme = stepChoose, taskPaint, 0, 0
	ws.app.CommandStorage().DisposeStack(ws.CommandStackId())
	ws.app.CommandStorage().SetStack(ws.CommandStackId())
	ws.app.LoadedEnvironment().RemoveTypeTrees(plan.Types)
	if app, ok := ws.app.(interface{ OnTemplatesRemoved([]ship.FileChange) }); ok {
		app.OnTemplatesRemoved(plan.Changes)
	} else {
		ws.app.SyncPrefabs()
	}
	ws.message = "Removed " + plan.Name + "."
	ws.notifySaved()
	ws.removalBackup = backup
	ws.clearRecovery()
	tools.SetEnabled(false)
	ws.app.OnWorkspaceSwitched()
}

func (ws *WsShip) removalRecovery() {
	if ws.removalBackup != "" && imgui.Button("Open recovery folder") {
		workshop.OpenRemovalRecovery(ws.removalBackup)
	}
}

func (ws *WsShip) RebaseRemoval(changes []ship.FileChange) {
	for _, p := range ws.projects {
		p.RebaseRemoval(changes)
	}
}
