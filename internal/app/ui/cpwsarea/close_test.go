package cpwsarea

import (
	"testing"

	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/workspace"
)

type closeTestApp struct {
	App
	commands *command.Storage
}

func (a *closeTestApp) CommandStorage() *command.Storage { return a.commands }

type closeTestContent struct {
	workspace.Content
	saveOK   bool
	saves    int
	disposed bool
}

func (*closeTestContent) Name() string  { return "unsaved map" }
func (*closeTestContent) Title() string { return "unsaved map" }
func (c *closeTestContent) Save() bool  { c.saves++; return c.saveOK }
func (c *closeTestContent) Dispose()    { c.disposed = true }

// Workshops holding several drafts expose SaveAll; closing must use it.
type closeTestDrafts struct {
	closeTestContent
	saveAlls int
}

func (c *closeTestDrafts) SaveAll() bool { c.saveAlls++; return c.saveOK }

type closeTestDocument struct {
	closeTestContent
	discarded bool
}

func (c *closeTestDocument) DiscardChanges()      { c.discarded = true }
func (*closeTestDocument) CommandStackId() string { return "sprite-test" }

func TestDiscardDocumentDoesNotReplayHistory(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		content := &closeTestDocument{}
		ws := workspace.New(content)
		commands := command.NewStorage()
		commands.SetStack(ws.CommandStackId())
		commands.Push(command.Make("must not replay", func() { t.Fatal("discard replayed pixel history") }, func() {}))
		area := &WsArea{app: &closeTestApp{commands: commands}, workspaces: []*workspace.Workspace{ws}}
		if multiple {
			area.makeCloseWorkspacesDialog(area.workspaces, area.workspaces, nil).ActionNo()
		} else {
			area.makeCloseWorkspaceDialog(ws, nil).ActionNo()
		}
		if !content.discarded || !content.disposed {
			t.Fatal("document was not discarded and closed")
		}
	}
}

// Updates use the same close confirmation as Exit. Cancellation and a failed
// save must keep all workspaces alive and deny permission to restart.
func TestCloseConfirmationProtectsUnsavedWork(t *testing.T) {
	for _, choice := range []string{"cancel", "failed save", "save", "discard"} {
		t.Run(choice, func(t *testing.T) {
			first := &closeTestContent{saveOK: true}
			second := &closeTestDrafts{closeTestContent: closeTestContent{saveOK: choice != "failed save"}}
			workspaces := []*workspace.Workspace{workspace.New(first), workspace.New(second)}
			area := &WsArea{app: &closeTestApp{commands: command.NewStorage()}, workspaces: workspaces}
			called, accepted := 0, false
			confirmation := area.makeCloseWorkspacesDialog(workspaces, workspaces, func(ok bool) { called++; accepted = ok })
			switch choice {
			case "cancel":
				confirmation.ActionCancel()
			case "discard":
				confirmation.ActionNo()
			default:
				confirmation.ActionYes()
			}
			wantClose := choice == "save" || choice == "discard"
			if called != 1 || accepted != wantClose || first.disposed != wantClose || second.disposed != wantClose {
				t.Fatalf("unsafe close: accepted=%v, disposed=%v/%v, callback=%d", accepted, first.disposed, second.disposed, called)
			}
			if !wantClose && len(area.workspaces) != 2 {
				t.Fatal("unsaved workspaces were removed")
			}
			if (choice == "cancel" || choice == "discard") && (first.saves != 0 || second.saves != 0 || second.saveAlls != 0) {
				t.Fatal("saved without choosing Save")
			}
			if choice == "save" && (first.saves != 1 || second.saveAlls != 1 || second.saves != 0) {
				t.Fatalf("closing saved the wrong drafts: saves=%d, saveAlls=%d/%d", first.saves, second.saves, second.saveAlls)
			}
		})
	}
}

// Closing one tab (File > Close) confirms through its own dialog; saving from
// it must cover every draft as well.
func TestSingleCloseSavesEveryDraft(t *testing.T) {
	for _, ok := range []bool{false, true} {
		content := &closeTestDrafts{closeTestContent: closeTestContent{saveOK: ok}}
		ws := workspace.New(content)
		area := &WsArea{app: &closeTestApp{commands: command.NewStorage()}, workspaces: []*workspace.Workspace{ws}}
		closed, called := false, 0
		area.makeCloseWorkspaceDialog(ws, func(v bool) { closed = v; called++ }).ActionYes()
		if called != 1 || closed != ok || content.disposed != ok || content.saveAlls != 1 || content.saves != 0 {
			t.Fatalf("single close saveOK=%v: closed=%v, disposed=%v, saveAlls=%d, saves=%d", ok, closed, content.disposed, content.saveAlls, content.saves)
		}
	}
}
