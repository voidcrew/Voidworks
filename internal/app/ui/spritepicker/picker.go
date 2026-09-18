package spritepicker

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/SpaiR/imgui-go"
	filedialog "github.com/sqweek/dialog"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/imguiext/style"
)

type scanResult struct {
	files []string
	err   error
}

type picker struct {
	root, target                                string
	original                                    *dmmprefab.Prefab
	apply                                       func(*dmmprefab.Prefab) error
	current, file, state                        string
	fileFilter, stateFilter, message, loadError string
	all, selected                               bool
	files, states                               []string
	scan                                        chan scanResult
	direction                                   int
	loadedFile                                  string
	revision                                    uint64
}

func Open(root string, original *dmmprefab.Prefab, target string, apply func(*dmmprefab.Prefab) error) {
	dialog.Open(newPicker(root, original, target, apply))
}

func newPicker(root string, original *dmmprefab.Prefab, target string, apply func(*dmmprefab.Prefab) error) *picker {
	current := original.Vars().TextV("icon", "")
	if rel, err := resourcePath(root, current); err == nil {
		current = rel
	}
	return &picker{root: root, original: original, target: target, apply: apply,
		current: current, file: current, state: original.Vars().TextV("icon_state", ""),
		direction: original.Vars().IntV("dir", dm.DirDefault)}
}

func (*picker) Name() string         { return "Replace sprite" }
func (*picker) HasCloseButton() bool { return true }

func (*picker) BeforePopup() {
	// Keep the action buttons reachable when the editor window is resized.
	imgui.SetNextWindowPosV(imgui.MainViewport().Center(), imgui.ConditionAlways, imgui.Vec2{X: .5, Y: .5})
}

func (p *picker) refreshFiles() {
	if p.scan != nil {
		return
	}
	// Only filenames are collected off-thread. Textures are loaded on demand on
	// the GL thread, so browsing never decodes every DMI in a large project.
	result := make(chan scanResult, 1)
	p.scan = result
	go func() { files, err := scanDMIs(p.root); result <- scanResult{files, err} }()
}

func (p *picker) chooseFile(path string) {
	p.file, p.state, p.selected = path, "", false
	if path == p.current {
		p.state = p.original.Vars().TextV("icon_state", "")
	}
	p.stateFilter, p.message, p.loadedFile = "", "", ""
}

func (p *picker) loadStates() *dmicon.Dmi {
	d, err := dmicon.Cache.Get(p.file)
	if err != nil {
		p.loadError, p.selected, p.states = "Cannot read DMI: "+err.Error(), false, nil
		return nil
	}
	p.loadError = ""
	if p.loadedFile != p.file || p.revision != dmicon.LayoutRevision {
		p.states = nil
		for name := range d.States {
			p.states = append(p.states, name)
		}
		alphabetical(p.states)
		_, p.selected = d.States[p.state]
		p.loadedFile, p.revision = p.file, dmicon.LayoutRevision
	}
	return d
}

func (p *picker) useSelected() bool {
	if !p.selected {
		return false
	}
	path, err := resourcePath(p.root, p.file)
	if err == nil {
		d, loadErr := dmicon.Cache.Get(p.file)
		err = loadErr
		if err == nil && d.States[p.state] == nil {
			err = fmt.Errorf("This sprite is no longer available. Choose another state.")
		}
	}
	if err == nil {
		err = p.apply(replacement(p.original, path, p.state))
	}
	if err != nil {
		p.message = err.Error()
		return false
	}
	return true
}

func (p *picker) Process() {
	s := window.PointSize()
	workshop.PushStyle()
	defer workshop.PopStyle()
	size := imgui.MainViewport().Size()
	width, height := min(940*s, size.X-60*s), min(710*s, size.Y-90*s)
	imgui.BeginChildV("sprite-picker", imgui.Vec2{X: width, Y: height}, false, 0)
	workshop.Wrapped(p.original.Vars().TextV("name", p.original.Path()))
	workshop.Muted(p.target)
	imgui.Separator()
	show := "Current DMI"
	if p.all {
		show = "All DMIs"
	}
	imgui.SetNextItemWidth(180 * s)
	if imgui.BeginCombo("Show", show) {
		if imgui.SelectableV("Current DMI", !p.all, 0, imgui.Vec2{}) {
			p.all = false
			p.chooseFile(p.current)
		}
		if imgui.SelectableV("All DMIs", p.all, 0, imgui.Vec2{}) {
			p.all = true
			if p.files == nil {
				p.refreshFiles()
			}
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	if imgui.Button("Browse DMI...") {
		path, err := filedialog.File().Title("Choose replacement DMI").Filter("BYOND icon", "dmi").SetStartDir(p.root).Load()
		if err == nil {
			var rel string
			rel, err = resourcePath(p.root, path)
			if err == nil {
				p.all = true
				p.chooseFile(rel)
				p.refreshFiles()
			}
		}
		if err != nil && !errors.Is(err, filedialog.ErrCancelled) {
			p.message = err.Error()
		}
	}
	imgui.SameLine()
	if imgui.Button("Refresh") {
		// Also clears failed disk loads after a file is saved externally.
		dmicon.Cache.Invalidate(p.file)
		p.loadedFile = ""
		p.refreshFiles()
	}
	if p.scan != nil {
		select {
		case result := <-p.scan:
			p.files, p.scan = result.files, nil
			if result.err != nil {
				p.message = "Some folders could not be read: " + result.err.Error()
			}
		default:
		}
	}
	if p.message != "" {
		workshop.Muted(p.message)
	}
	imgui.BeginChildV("picker-body", imgui.Vec2{Y: -48 * s}, false, 0)
	left := min(285*s, width*.36)
	imgui.BeginChildV("dmi-files", imgui.Vec2{X: left}, false, 0)
	if p.all {
		imgui.SetNextItemWidth(-1)
		imgui.InputTextWithHint("##dmi-search", "Search DMI files", &p.fileFilter)
		if p.scan != nil {
			workshop.Muted("Finding DMI files...")
		} else {
			workshop.Muted(fmt.Sprintf("%d DMI files", len(p.files)))
		}
		imgui.BeginChild("dmi-file-results")
		files := filter(p.files, p.fileFilter)
		var clipper imgui.ListClipper
		clipper.Begin(len(files))
		for clipper.Step() {
			for row := clipper.DisplayStart; row < clipper.DisplayEnd; row++ {
				path := files[row]
				directory := filepath.ToSlash(filepath.Dir(path))
				if directory == "." {
					directory = "Project folder"
				}
				if workshop.Row(path, filepath.Base(path), directory, "", path == p.file, style.Teal, 0) {
					p.chooseFile(path)
				}
			}
		}
		if len(files) == 0 && p.scan == nil {
			workshop.Muted("No matching DMIs. Save a DMI in this project or browse to it.")
		}
		imgui.EndChild()
	} else {
		workshop.Muted("Current DMI")
		workshop.Wrapped(p.current)
		workshop.Gap()
		workshop.Muted("Current sprite")
		pos := imgui.CursorScreenPos()
		if d, err := dmicon.Cache.Get(p.current); err == nil {
			drawSprite(d.States[p.original.Vars().TextV("icon_state", "")], p.direction, pos, 96*s)
		}
		imgui.Dummy(imgui.Vec2{X: 96 * s, Y: 96 * s})
		workshop.Wrapped(stateName(p.original.Vars().TextV("icon_state", "")))
		workshop.Gap()
		workshop.Muted("Choose a sprite on the right, then Apply. To use a new DMI, save it inside this project and select All DMIs.")
	}
	imgui.EndChild()
	imgui.SameLine()
	imgui.BeginChild("dmi-state-panel")
	workshop.Wrapped(p.file)
	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##state-search", "Search sprite states", &p.stateFilter)
	d := p.loadStates()
	states := filter(p.states, p.stateFilter)
	workshop.Muted(fmt.Sprintf("%d sprites", len(states)))
	imgui.BeginChild("sprite-state-results")
	if p.loadError != "" {
		workshop.Muted(p.loadError)
	}
	var clipper imgui.ListClipper
	clipper.Begin(len(states))
	for clipper.Step() {
		for row := clipper.DisplayStart; row < clipper.DisplayEnd; row++ {
			name := states[row]
			state := d.States[name]
			pos := imgui.CursorScreenPos()
			badge := ""
			if p.file == p.current && name == p.original.Vars().TextV("icon_state", "") {
				badge = "Current"
			}
			if workshop.Row("state-"+name, stateName(name), fmt.Sprintf("%d directions · %d frames", state.Dirs, state.Frames), badge, p.selected && p.state == name, style.Teal, 52) {
				p.state, p.selected = name, true
			}
			drawSprite(state, p.direction, pos.Plus(imgui.Vec2{X: 9 * s, Y: 6 * s}), 44*s)
		}
	}
	if d != nil && len(states) == 0 {
		workshop.Muted("No matching sprite states.")
	}
	imgui.EndChild()
	imgui.EndChild()
	imgui.EndChild()
	if imgui.Button("Cancel") {
		imgui.CloseCurrentPopup()
	}
	imgui.SameLine()
	if !p.selected {
		imgui.BeginDisabled()
	}
	if imgui.Button("Apply sprite") && p.useSelected() {
		imgui.CloseCurrentPopup()
	}
	if !p.selected {
		imgui.EndDisabled()
	}
	imgui.SameLine()
	if p.selected {
		workshop.Muted(stateName(p.state))
	}
	imgui.EndChild()
}

func stateName(name string) string {
	if name == "" {
		return "(default)"
	}
	return name
}

func drawSprite(state *dmicon.State, dir int, pos imgui.Vec2, size float32) {
	if state == nil || state.Frames == 0 || len(state.Sprites) == 0 {
		return
	}
	sprite := state.SpriteV(dir)
	scale := size / float32(max(sprite.IconWidth(), sprite.IconHeight()))
	w, h := float32(sprite.IconWidth())*scale, float32(sprite.IconHeight())*scale
	pos = pos.Plus(imgui.Vec2{X: (size - w) / 2, Y: (size - h) / 2})
	imgui.WindowDrawList().AddImageV(imgui.TextureID(sprite.Texture()), pos, pos.Plus(imgui.Vec2{X: w, Y: h}), imgui.Vec2{X: sprite.U1, Y: sprite.V1}, imgui.Vec2{X: sprite.U2, Y: sprite.V2}, 0xffffffff)
}
