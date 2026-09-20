package wssprite

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/dmi"
	"sdmm/internal/env"
	"sdmm/internal/recovery"
)

func decodeDraft(data []byte) (*dmi.Document, error) {
	var snapshot draft
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version != 1 || (snapshot.Path != "" && len(snapshot.Baseline) != 32) {
		return nil, fmt.Errorf("unsupported or incomplete sprite recovery")
	}
	current, err := dmi.Decode(snapshot.Current)
	if err != nil {
		return nil, err
	}
	var saved *dmi.Icon
	if len(snapshot.Saved) > 0 {
		saved, err = dmi.Decode(snapshot.Saved)
		if err != nil {
			return nil, fmt.Errorf("saved image in recovery is damaged: %w", err)
		}
	}
	return &dmi.Document{Path: snapshot.Path, Icon: current, Saved: saved, Baseline: snapshot.Baseline, Revision: 1}, nil
}

type recoveryDialog struct {
	entries []recovery.Entry
	open    func(*dmi.Document) *Workspace
	message string
}

func ShowRecovery(open func(*dmi.Document) *Workspace) {
	root, err := env.ProfileDir()
	var entries []recovery.Entry
	if err == nil {
		entries, err = recovery.PendingAll(filepath.Join(root, "sprite-recovery"))
	}
	if err != nil {
		dialog.Open(dialog.TypeInformation{Title: "Sprite recovery", Information: err.Error()})
		return
	}
	dialog.Open(&recoveryDialog{entries: entries, open: open})
}
func (*recoveryDialog) Name() string         { return "Recover DMI drafts" }
func (*recoveryDialog) HasCloseButton() bool { return false }
func (r *recoveryDialog) close() {
	for _, entry := range r.entries {
		entry.Store.Close()
	}
	imgui.CloseCurrentPopup()
}
func (r *recoveryDialog) recover(entry recovery.Entry) error {
	doc, err := decodeDraft(entry.Snapshot.Data)
	if err != nil {
		return err
	}
	doc.Path, doc.Baseline, doc.Saved = "", nil, nil
	ws := r.open(doc)
	ws.lastRecovery = time.Time{}
	if ws.autosave() {
		_ = entry.Store.Discard()
	}
	return nil
}
func (r *recoveryDialog) Process() {
	imgui.TextWrapped("Recover as a new DMI, then use Save as to choose its destination. Original files stay unchanged.")
	if len(r.entries) == 0 {
		imgui.Text("No inactive sprite drafts were found.")
	}
	imgui.BeginChildV("drafts", imgui.Vec2{X: 640, Y: 280}, true, 0)
	for n, entry := range r.entries {
		imgui.PushIDInt(n)
		if entry.Err != nil {
			imgui.TextWrapped("Unreadable recovery: " + entry.Err.Error())
		} else {
			s := entry.Snapshot
			imgui.TextWrapped(s.Environment)
			imgui.TextDisabled(s.Created.Local().Format("Jan 2 15:04:05"))
			if imgui.Button("Recover as new DMI") {
				err := r.recover(entry)
				if err != nil {
					r.message = err.Error()
				} else {
					r.close()
				}
			}
		}
		imgui.Separator()
		imgui.PopID()
	}
	imgui.EndChild()
	if r.message != "" {
		imgui.TextWrapped(r.message)
	}
	if imgui.Button("Close") {
		r.close()
	}
}
