package wssprite

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/cpwsarea/wspreview"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmi"
	"sdmm/internal/env"
	"sdmm/internal/recovery"
)

type App interface{ CommandStorage() *command.Storage }
type pixelSelection struct {
	rect image.Rectangle
	mask *image.Alpha
}
type Workspace struct {
	workspace.Content
	app                                                      App
	Document                                                 *dmi.Document
	Preview                                                  *wspreview.Preview
	rendered                                                 *dmicon.Dmi
	published                                                *dmi.Icon
	previewPath                                              string
	state, cel                                               int
	layer                                                    int
	layerName                                                string
	selected                                                 map[int]bool
	cells                                                    map[int]bool
	filter, name, delays, hotspot, message                   string
	tool                                                     int32
	color                                                    [4]float32
	zoom                                                     float32
	pan                                                      imgui.Vec2
	grid, tiled, onion, symX, symY, playing, before, allCels bool
	started                                                  time.Time
	playhead                                                 float64
	frameMilliseconds                                        string
	stroke                                                   *dmi.Icon
	start, last                                              image.Point
	selection                                                image.Rectangle
	selectionMask, strokeMask                                *image.Alpha
	strokeSelection                                          image.Rectangle
	lasso, strokePoints                                      []image.Point
	pixelPerfect                                             bool
	brushSize, tolerance                                     int32
	clipboard                                                *image.NRGBA
	resizeWidth, resizeHeight, anchorX, anchorY              int32
	scaleResize                                              bool
	prefix, find, replacement, suffix                        string
	store                                                    *recovery.Store
	pendingRecovery                                          *recovery.Store
	canvasOrigin                                             imgui.Vec2
	openResize                                               bool
	openImport                                               bool
	importSheet                                              *sheetImport
	palette                                                  []color.NRGBA
	replaceFrom                                              [4]float32
	lastRecovery                                             time.Time
	recoveredRevision                                        uint64
	lastExternal                                             time.Time
	external                                                 bool
}

func New(app App, doc *dmi.Document, preview *wspreview.Preview) *Workspace {
	w := &Workspace{app: app, Document: doc, Preview: preview, selected: map[int]bool{0: true}, cells: map[int]bool{0: true}, zoom: 10, grid: true, color: [4]float32{.8, .3, .7, 1}, started: time.Now()}
	w.previewPath = doc.Path
	if w.previewPath == "" {
		w.previewPath, _ = filepath.Abs(filepath.Join(".voidworks-unsaved", w.Id()+".dmi"))
	}
	w.resizeWidth, w.resizeHeight = int32(doc.Icon.Width), int32(doc.Icon.Height)
	w.brushSize = 1
	app.CommandStorage().SetStack(w.CommandStackId())
	w.syncFields()
	w.publish()
	w.openRecovery()
	return w
}
func (w *Workspace) Name() string {
	name := filepath.Base(w.Document.Path)
	if w.Document.Path == "" {
		name = "Untitled.dmi"
	}
	if w.IsModified() {
		name = "* " + name
	}
	return name + "###" + w.Id()
}
func (w *Workspace) Title() string { return "DMI Editor: " + filepath.Base(w.Document.Path) }
func (w *Workspace) Select(name string, dir int) {
	for n, s := range w.Document.Icon.States {
		if s.Name == name && !s.Movement() {
			w.selectState(n, false, false)
			for idx, value := range []int{2, 1, 4, 8, 6, 10, 5, 9} {
				if value == dir && idx < s.Dirs() {
					w.cel = idx
					w.cells = map[int]bool{idx: true}
					break
				}
			}
			return
		}
	}
}
func (w *Workspace) CommandStackId() string { return "sprite:" + w.Id() }
func (w *Workspace) IsModified() bool       { return w.Document.Modified() || w.stroke != nil }
func (w *Workspace) Undo() {
	w.finishStroke()
	w.app.CommandStorage().UndoV(w.CommandStackId())
}
func (w *Workspace) Redo() {
	w.finishStroke()
	w.app.CommandStorage().RedoV(w.CommandStackId())
}

// DiscardChanges is followed by Dispose, which removes the live override.
// Discarding a pixel document must not replay a bounded or branched history.
func (w *Workspace) DiscardChanges() { w.stroke = nil }
func (w *Workspace) Ini() workspace.Ini {
	return workspace.Ini{WindowFlags: imgui.WindowFlagsNoScrollbar | imgui.WindowFlagsNoScrollWithMouse}
}
func (w *Workspace) OnFocusChange(focused bool) {
	if focused {
		tools.SetEnabled(false)
	} else {
		w.finishStroke()
	}
}
func (w *Workspace) PreProcess() {
	w.updatePlayback(time.Now())
	if !imgui.IsMouseDown(imgui.MouseButtonLeft) {
		w.finishStroke()
		if len(w.lasso) > 0 {
			w.selectionMask = dmi.Lasso(image.Rect(0, 0, w.Document.Icon.Width, w.Document.Icon.Height), w.lasso)
			w.selection = dmi.SelectionBounds(w.selectionMask)
			w.lasso = nil
		}
	}
	if time.Since(w.lastExternal) > 2*time.Second {
		w.external = w.Document.ExternalChange()
		w.lastExternal = time.Now()
	}
	w.autosave()
}
func (w *Workspace) Dispose() {
	if w.pendingRecovery != nil {
		w.pendingRecovery.Close()
	}
	w.finishStroke()
	dmicon.EndPreview(w.previewPath)
	if w.Preview != nil {
		w.Preview.Dispose()
	}
	if w.store != nil {
		_ = w.store.Discard()
	}
}
func (w *Workspace) syncFields() {
	i := w.Document.Icon
	w.state = max(0, min(w.state, len(i.States)-1))
	for n := range w.selected {
		if n >= len(i.States) {
			delete(w.selected, n)
		}
	}
	if len(w.selected) == 0 {
		w.selected[w.state] = true
	}
	if len(i.States) == 0 {
		w.name, w.delays, w.hotspot = "", "", ""
		w.cel = 0
		return
	}
	s := i.States[w.state]
	w.layer = max(0, min(w.layer, len(s.Layers)-1))
	if len(s.Layers) > 0 {
		w.layerName = s.Layers[w.layer].Name
	}
	w.cel = max(0, min(w.cel, len(s.Cels)-1))
	if !w.playing && s.FrameAt(w.playhead) != w.cel/s.Dirs() {
		w.seekSelectedFrame()
	}
	for n := range w.cells {
		if n >= len(s.Cels) {
			delete(w.cells, n)
		}
	}
	if len(w.cells) == 0 {
		w.cells[w.cel] = true
	}
	w.name = s.Name
	w.delays = s.Value("delay", "")
	w.frameMilliseconds = strconv.FormatFloat(s.Delays()[w.cel/s.Dirs()]*100, 'f', -1, 64)
	w.hotspot = s.Value("hotspot", "")
}
func (w *Workspace) publish() {
	i := w.Document.Icon
	if w.before && w.Document.Saved != nil {
		i = w.Document.Saved
	}
	if w.published == i {
		return
	}
	if e := dmicon.Preview(w.previewPath, i); e != nil {
		w.message = e.Error()
		return
	}
	w.rendered, _ = dmicon.Cache.Get(w.previewPath)
	w.published = i
}
func (w *Workspace) restore(i *dmi.Icon) {
	w.finishStroke()
	w.Document.Restore(i)
	w.before = false
	w.syncFields()
	w.publish()
}
func (w *Workspace) history(label string, before, after *dmi.Icon) {
	selection := pixelSelection{w.selection, w.selectionMask}
	w.historySelection(label, before, after, selection, selection)
}
func (w *Workspace) historySelection(label string, before, after *dmi.Icon, oldSelection, newSelection pixelSelection) {
	if before == after {
		return
	}
	restore := func(icon *dmi.Icon, selection pixelSelection) {
		w.restore(icon)
		w.selection, w.selectionMask = selection.rect, selection.mask
	}
	cost := dmi.HistoryCost(before, after)
	if oldSelection.mask != newSelection.mask {
		for _, mask := range []*image.Alpha{oldSelection.mask, newSelection.mask} {
			if mask != nil {
				cost += int64(len(mask.Pix))
			}
		}
	}
	w.app.CommandStorage().PushV(w.CommandStackId(), command.Make(label, func() { restore(before, oldSelection) }, func() { restore(after, newSelection) }).WithCost(cost))
	w.app.CommandStorage().LimitHistory(w.CommandStackId(), 200, 128<<20)
}
func (w *Workspace) change(label string, change func(*dmi.Icon) error) bool {
	w.finishStroke()
	if len(w.Document.Icon.ConversionIssues()) > 0 {
		w.message = "Choose Prepare for editing before changing this image."
		return false
	}
	selection := pixelSelection{w.selection, w.selectionMask}
	before, after, e := w.Document.Apply(change)
	if e != nil {
		w.selection, w.selectionMask = selection.rect, selection.mask
		w.message = e.Error()
		w.syncFields()
		return false
	}
	w.historySelection(label, before, after, selection, pixelSelection{w.selection, w.selectionMask})
	w.before = false
	w.syncFields()
	w.publish()
	w.message = ""
	return true
}
func (w *Workspace) finishStroke() {
	if w.stroke == nil {
		return
	}
	before := w.stroke
	w.stroke = nil
	w.historySelection("Draw pixels", before, w.Document.Icon, pixelSelection{w.strokeSelection, w.strokeMask}, pixelSelection{w.selection, w.selectionMask})
}
func (w *Workspace) indices() []int {
	var result []int
	for n, yes := range w.selected {
		if yes && n >= 0 && n < len(w.Document.Icon.States) {
			result = append(result, n)
		}
	}
	sort.Ints(result)
	return result
}
func (w *Workspace) selectState(n int, multi, span bool) {
	w.finishStroke()
	if span {
		if !multi {
			w.selected = map[int]bool{}
		}
		for idx := min(w.state, n); idx <= max(w.state, n); idx++ {
			w.selected[idx] = true
		}
	} else if multi {
		w.selected[n] = !w.selected[n]
	} else {
		w.selected = map[int]bool{n: true}
	}
	w.state = n
	w.cel = 0
	w.cells = map[int]bool{0: true}
	w.selection = image.Rectangle{}
	w.selectionMask = nil
	w.started = time.Now()
	w.playhead = 0
	w.syncFields()
}
func (w *Workspace) confirm(title, message string, action func()) {
	dialog.Open(dialog.TypeConfirmation{Title: title, Question: message, ActionYes: action})
}
func (w *Workspace) Process() {
	w.publish()
	dmicon.SetPreviewPlayback(w.previewPath, w.playing, w.started, w.playhead)
	w.toolbar()
	w.conversionControls()
	if w.external {
		imgui.TextColored(imgui.Vec4{X: 1, Y: .7, Z: .2, W: 1}, "File changed outside Voidworks. Reload or save a separate copy.")
	}
	if w.message != "" {
		imgui.TextWrapped(w.message)
	}
	available := imgui.ContentRegionAvail()
	left := min(float32(250), available.X*.27)
	imgui.BeginChildV("states", imgui.Vec2{X: left}, true, 0)
	w.stateList()
	imgui.EndChild()
	imgui.SameLine()
	imgui.BeginChildV("editing", imgui.Vec2{}, false, 0)
	if len(w.Document.Icon.States) == 0 {
		imgui.TextWrapped("This DMI has no states. Add a state to begin.")
	} else {
		w.properties()
		imgui.Separator()
		w.drawingTools()
		remaining := imgui.ContentRegionAvail().Y
		timelineHeight := min(remaining*.42, float32(50+w.Document.Icon.States[w.state].Dirs()*44))
		height := max(float32(80), remaining-timelineHeight)
		width := imgui.ContentRegionAvail().X
		if w.Preview != nil && width >= 550 {
			width = width * .58
		}
		imgui.BeginChildV("pixels", imgui.Vec2{X: width, Y: height}, true, imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
		w.canvas()
		imgui.EndChild()
		if w.Preview != nil && imgui.ContentRegionAvail().X >= 550 {
			imgui.SameLine()
			imgui.BeginChildV("world-preview", imgui.Vec2{Y: height}, true, 0)
			w.Preview.Process()
			imgui.EndChild()
		}
		w.timeline()
	}
	imgui.EndChild()
}
func (w *Workspace) backupRoot() string {
	p, e := env.ProfileDir()
	if e != nil {
		w.message = e.Error()
		return ""
	}
	return filepath.Join(p, "sprite-backups")
}
func (w *Workspace) uv(state, cel int) (imgui.Vec2, imgui.Vec2) {
	if w.published == nil || w.rendered == nil || state < 0 || state >= len(w.published.States) || cel < 0 || cel >= len(w.published.States[state].Cels) {
		return imgui.Vec2{}, imgui.Vec2{}
	}
	idx := cel
	for n := 0; n < state; n++ {
		idx += len(w.published.States[n].Cels)
	}
	cols, rows := w.rendered.Cols, w.rendered.Rows
	x, y := idx%cols, idx/cols
	return imgui.Vec2{X: float32(x) / float32(cols), Y: float32(y) / float32(rows)}, imgui.Vec2{X: float32(x+1) / float32(cols), Y: float32(y+1) / float32(rows)}
}
func (w *Workspace) status() string {
	i := w.Document.Icon
	return fmt.Sprintf("%d x %d | %d states | %d images", i.Width, i.Height, len(i.States), i.CelCount())
}
