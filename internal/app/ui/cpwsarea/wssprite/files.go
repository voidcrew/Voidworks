package wssprite

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skratchdot/open-golang/open"
	filedialog "github.com/sqweek/dialog"
	"golang.design/x/clipboard"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmi"
	"sdmm/internal/env"
	"sdmm/internal/platform"
	"sdmm/internal/recovery"
)

func (w *Workspace) Save() bool {
	w.finishStroke()
	if w.Document.Path == "" || !strings.EqualFold(filepath.Ext(w.Document.Path), ".dmi") {
		return w.saveAs()
	}
	return w.saveTo(w.Document.Path)
}
func (w *Workspace) saveAs() bool {
	w.finishStroke()
	name := "Untitled.dmi"
	if w.Document.Path != "" {
		name = strings.TrimSuffix(filepath.Base(w.Document.Path), filepath.Ext(w.Document.Path)) + ".dmi"
	}
	chooser := filedialog.File().Title("Save DMI as").Filter("BYOND icon", "dmi").SetStartFile(name)
	if w.Document.Path != "" {
		chooser.SetStartDir(filepath.Dir(w.Document.Path))
	}
	path, e := chooser.Save()
	if e != nil {
		if !errors.Is(e, filedialog.ErrCancelled) {
			w.message = "Cannot open Save DMI dialog: " + e.Error()
			dialog.Open(dialog.TypeInformation{Title: "DMI save failed", Information: w.message})
		}
		return false
	}
	if filepath.Ext(path) == "" {
		path += ".dmi"
	}
	return w.saveTo(path)
}
func (w *Workspace) saveTo(path string) bool {
	if e := w.Document.Save(path, w.backupRoot()); e != nil {
		w.message = "Save failed: " + e.Error()
		dialog.Open(dialog.TypeInformation{Title: "DMI save failed", Information: w.message})
		return false
	}
	if w.previewPath != w.Document.Path {
		dmicon.EndPreview(w.previewPath)
		w.previewPath = w.Document.Path
		w.published = nil
		w.publish()
		if w.store != nil {
			_ = w.store.Discard()
			w.store = nil
		}
		w.openRecovery()
	}
	w.app.CommandStorage().ForceBalance(w.CommandStackId())
	if w.store != nil {
		_ = w.store.Clear()
	}
	w.external = false
	w.message = "Saved " + filepath.Base(path)
	return true
}
func (w *Workspace) reload() {
	doc, e := dmi.Load(w.Document.Path)
	if e != nil {
		w.message = e.Error()
		return
	}
	w.finishStroke()
	w.Document = doc
	w.app.CommandStorage().DisposeStack(w.CommandStackId())
	w.app.CommandStorage().SetStack(w.CommandStackId())
	w.external = false
	w.syncFields()
	w.publish()
	if w.store != nil {
		_ = w.store.Clear()
	}
}
func (w *Workspace) importFile() {
	path, e := filedialog.File().Title("Import DMI or PNG").Filter("Sprites", "dmi", "png").Load()
	if e != nil {
		return
	}
	icon, e := dmi.Open(path)
	if e != nil {
		w.message = e.Error()
		return
	}
	w.stageImport(icon, path)
}
func (w *Workspace) stageImport(icon *dmi.Icon, path string) {
	if issues := icon.ConversionIssues(); len(issues) > 0 {
		w.confirm("Convert imported image?", strings.Join(issues, "\n")+"\nThe source file will stay unchanged.", func() {
			prepared := icon.Clone()
			prepared.ConvertForEditing()
			w.stageImport(prepared, path)
		})
		return
	}
	if strings.EqualFold(filepath.Ext(path), ".png") && len(icon.States) == 1 && len(icon.States[0].Cels) == 1 {
		w.stageSheet(icon, path)
	} else {
		w.importIcon(icon)
	}
}
func (w *Workspace) importIcon(icon *dmi.Icon) {
	w.change("Import states", func(target *dmi.Icon) error {
		if target.Width != icon.Width || target.Height != icon.Height {
			return fmt.Errorf("import canvas is %d x %d; destination is %d x %d. Resize the destination or import matching frames", icon.Width, icon.Height, target.Width, target.Height)
		}
		names := map[string]string{}
		for _, s := range icon.States {
			clone := s.Clone()
			name, ok := names[s.Name]
			if !ok {
				name = target.UniqueName(s.Name)
				names[s.Name] = name
			}
			clone.Name = name
			target.States = append(target.States, clone)
		}
		return nil
	})
}

const stateClipboard = "VOIDWORKS_DMI_STATES_1:"

func (w *Workspace) CopyStates() {
	i := w.Document.Icon.Clone()
	i.States = nil
	for _, n := range w.indices() {
		i.States = append(i.States, w.Document.Icon.States[n].Clone())
	}
	if len(i.States) == 0 {
		return
	}
	i.Changed = true
	data, e := dmi.Encode(i)
	if e != nil {
		w.message = e.Error()
		return
	}
	if len(data) > 16<<20 {
		w.message = "Selected states are too large for clipboard transfer; export them as a DMI."
		return
	}
	platform.SetClipboard(stateClipboard + base64.StdEncoding.EncodeToString(data))
	w.message = fmt.Sprintf("Copied %d states", len(i.States))
}
func (w *Workspace) PasteStates() {
	text := platform.GetClipboard()
	if !strings.HasPrefix(text, stateClipboard) {
		w.message = "Copy DMI states first."
		return
	}
	if len(text) > 24<<20 {
		w.message = "Clipboard data is too large"
		return
	}
	data, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(text, stateClipboard))
	if e != nil {
		w.message = e.Error()
		return
	}
	icon, e := dmi.Decode(data)
	if e != nil {
		w.message = e.Error()
		return
	}
	w.importIcon(icon)
}
func (w *Workspace) exportFile(states bool) {
	ext, label := "png", "Current frame"
	var data []byte
	var e error
	if states {
		ext, label = "dmi", "Selected states"
		icon := w.Document.Icon.Clone()
		icon.States = nil
		for _, n := range w.indices() {
			icon.States = append(icon.States, w.Document.Icon.States[n].Clone())
		}
		icon.Changed = true
		data, e = dmi.Encode(icon)
	} else {
		if len(w.Document.Icon.States) == 0 {
			return
		}
		var b bytes.Buffer
		e = png.Encode(&b, w.Document.Icon.States[w.state].Cels[w.cel])
		data = b.Bytes()
	}
	if e != nil {
		w.message = e.Error()
		return
	}
	path, e := filedialog.File().Title("Export "+label).Filter(label, ext).Save()
	if e != nil {
		return
	}
	if filepath.Ext(path) == "" {
		path += "." + ext
	}
	if dmi.SamePath(path, w.Document.Path) {
		w.message = "Use Save to update this document. Choose a new filename for exported images or states."
		return
	}
	if e := dmi.Export(path, data); e != nil {
		w.message = e.Error()
	} else {
		w.message = "Exported " + filepath.Base(path)
	}
}
func (w *Workspace) Copy() { w.copyPixels() }
func (w *Workspace) copyPixels() bool {
	if len(w.Document.Icon.States) == 0 {
		return false
	}
	src := w.Document.Icon.States[w.state].RawCel(w.layer, w.cel)
	rect := w.selection
	if rect.Empty() {
		rect = src.Rect
	}
	rect = rect.Intersect(src.Rect)
	w.clipboard = dmi.NRGBA(src.SubImage(rect))
	if w.selectionMask != nil {
		for y := 0; y < rect.Dy(); y++ {
			for x := 0; x < rect.Dx(); x++ {
				if w.selectionMask.AlphaAt(x+rect.Min.X, y+rect.Min.Y).A == 0 {
					w.clipboard.SetNRGBA(x, y, color.NRGBA{})
				}
			}
		}
	}
	if err := clipboard.Init(); err != nil {
		w.message = err.Error()
		return false
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, w.clipboard); err != nil {
		w.message = err.Error()
		return false
	}
	if clipboard.Write(clipboard.FmtImage, buffer.Bytes()) == nil {
		w.message = "The image clipboard is busy. Try Copy again."
		return false
	}
	w.message = "Copied pixels"
	return true
}
func (w *Workspace) Paste() {
	if err := clipboard.Init(); err != nil {
		w.message = err.Error()
		return
	}
	data := clipboard.Read(clipboard.FmtImage)
	if len(data) == 0 {
		w.message = "Copy an image to the clipboard first, or use Paste states from the state menu."
		return
	}
	icon, err := dmi.Decode(data)
	if err != nil || len(icon.States) == 0 {
		w.message = "Clipboard image could not be decoded"
		return
	}
	if icon.BitDepth == 16 {
		w.confirm("Convert clipboard image?", "Reduce this clipboard image from 16-bit to 8-bit color channels for pasting?", func() { w.pastePixels(icon.States[0].Cels[0]) })
		return
	}
	w.pastePixels(icon.States[0].Cels[0])
}
func (w *Workspace) pastePixels(pixels *image.NRGBA) {
	w.clipboard = pixels
	w.change("Paste pixels", func(i *dmi.Icon) error {
		if len(i.States) == 0 {
			return fmt.Errorf("select a state")
		}
		dst, mask := dmi.PasteImage(i.States[w.state].RawCel(w.layer, w.cel), pixels, w.selection.Min, w.selection, w.selectionMask)
		if err := i.States[w.state].SetCel(w.layer, w.cel, dst); err != nil {
			return err
		}
		w.selection, w.selectionMask = dmi.SelectionBounds(mask), mask
		return nil
	})
}
func (w *Workspace) Delete() {
	w.change("Clear pixels", func(i *dmi.Icon) error {
		if len(i.States) == 0 {
			return nil
		}
		src := i.States[w.state].RawCel(w.layer, w.cel)
		dst := dmi.CopyImage(src)
		rect := w.selection
		if rect.Empty() {
			rect = src.Rect
		}
		rect = rect.Intersect(src.Rect)
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				if dmi.Selected(rect, w.selectionMask, image.Pt(x, y)) {
					dst.SetNRGBA(x, y, color.NRGBA{})
				}
			}
		}
		return i.States[w.state].SetCel(w.layer, w.cel, dst)
	})
}
func (w *Workspace) Cut() {
	if w.copyPixels() {
		w.Delete()
	}
}
func (w *Workspace) Deselect() { w.selection = image.Rectangle{}; w.selectionMask = nil }
func (w *Workspace) openBackups() {
	path := w.backupRoot()
	if path != "" {
		_ = os.MkdirAll(path, 0700)
		_ = open.Run(path)
	}
}

type draft struct {
	Version                  int
	Path                     string
	Baseline, Saved, Current []byte
}

func (w *Workspace) openRecovery() {
	root, e := env.ProfileDir()
	if e != nil {
		w.message = e.Error()
		return
	}
	root = filepath.Join(root, "sprite-recovery")
	key := w.Document.Path
	if key == "" {
		key = "untitled-dmi"
	}
	entries, e := recovery.Pending(root, key)
	if e != nil {
		w.message = e.Error()
		return
	}
	w.store, e = recovery.Open(root, key)
	if e != nil {
		for _, entry := range entries {
			entry.Store.Close()
		}
		w.message = "Recovery unavailable: " + e.Error()
		return
	}
	if len(entries) == 0 {
		return
	}
	entry := entries[0]
	for _, old := range entries[1:] {
		old.Store.Close()
	}
	if entry.Err != nil {
		w.message = "An earlier recovery copy could not be read: " + entry.Err.Error()
		entry.Store.Close()
		return
	}
	recovered, e := decodeDraft(entry.Snapshot.Data)
	if e == nil && !dmi.SamePath(recovered.Path, w.Document.Path) {
		e = fmt.Errorf("recovery belongs to a different DMI")
	}
	if e != nil {
		entry.Store.Close()
		w.message = e.Error()
		return
	}
	w.pendingRecovery = entry.Store
	closePending := func() { entry.Store.Close(); w.pendingRecovery = nil }
	dialog.Open(dialog.TypeConfirmation{Title: "Recover sprite edits?", Question: "Recover unsaved edits to " + filepath.Base(key) + " from " + entry.Snapshot.Created.Local().Format("Jan 2 15:04") + "?", ActionNo: closePending, ActionCancel: closePending, ActionYes: func() {
		w.Document.Saved = recovered.Saved
		w.Document.Baseline = recovered.Baseline
		w.Document.Restore(recovered.Icon)
		w.syncFields()
		w.publish()
		w.recoveredRevision = 0
		w.lastRecovery = time.Time{}
		if w.autosave() {
			_ = entry.Store.Discard()
		}
		closePending()
	}})
}
func (w *Workspace) autosave() bool {
	if w.store == nil || !w.IsModified() || w.stroke != nil || time.Since(w.lastRecovery) < 30*time.Second || w.recoveredRevision == w.Document.Revision {
		return false
	}
	w.lastRecovery = time.Now()
	current, e := dmi.Encode(w.Document.Icon)
	if e != nil {
		w.message = "Recovery failed: " + e.Error()
		return false
	}
	var saved []byte
	if w.Document.Saved != nil {
		saved, e = dmi.Encode(w.Document.Saved)
		if e != nil {
			w.message = e.Error()
			return false
		}
	}
	data, e := json.Marshal(draft{1, w.Document.Path, w.Document.Baseline, saved, current})
	if e == nil {
		e = w.store.Save(data, []string{filepath.Base(w.previewPath)}, time.Now())
	}
	if e != nil {
		w.message = "Recovery failed: " + e.Error()
		return false
	}
	w.recoveredRevision = w.Document.Revision
	return true
}
