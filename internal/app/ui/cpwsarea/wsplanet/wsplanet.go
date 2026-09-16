package wsplanet

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/canvas"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/mappreview"
	"sdmm/internal/planet"
)

type App interface {
	LoadedEnvironment() *dmenv.Dme
	CommandStorage() *command.Storage
}
type Workspace struct {
	climateView        climateView
	climateTexture     uint32
	pickingRiver       bool
	pickingEnvironment bool
	ruinFilter         string
	lighting           bool
	settingsTab        int
	workspace.Content
	nameDirty                                                             bool
	app                                                                   App
	catalog                                                               *planet.Catalog
	project                                                               *planet.Project
	dme                                                                   *dmenv.Dme
	canvas                                                                *canvas.Canvas
	canvasSize                                                            imgui.Vec2
	control                                                               *canvas.Control
	preview                                                               *planet.Preview
	scene                                                                 *mappreview.Scene
	selected, message, filter, newName, biomeName                         string
	mode, table, row, col, replace, base, pickerScope                     int
	picking, creating, blank, fit, cave, population, narrowEditor, review bool
	last                                                                  planet.State
	renderKey                                                             string
	iconRevision                                                          uint64
	lastBuild                                                             time.Time
	items                                                                 []string
	deleting                                                              bool
	deleteReplacement                                                     string
	hovered                                                               *dmminstance.Instance
	mouseWorld                                                            imgui.Vec2
	hoverActive                                                           bool
	selectedEntry                                                         string
	revealEntry                                                           bool
	drafts                                                                map[string]*planetDraft
	previousPlanet                                                        string
	cancelling                                                            bool
	reordering                                                            bool
	biomeRows                                                             map[string]imgui.Vec2
	padMin                                                                imgui.Vec2
	padSize                                                               float32
}

func New(app App) *Workspace {
	c := canvas.New()
	c.ClearColor = canvas.Color{R: .035, G: .045, B: .05, A: 1}
	return build(app, c)
}

// Drafts and saves need no GL context, so tests build without a canvas.
func build(app App, c *canvas.Canvas) *Workspace {
	w := &Workspace{app: app, population: true, fit: true, replace: -1, blank: true, canvas: c, control: canvas.NewControl()}
	w.control.AtCursor = true
	var err error
	w.catalog, err = planet.Discover(app.LoadedEnvironment())
	if err != nil {
		w.message = err.Error()
		return w
	}
	w.dme = app.LoadedEnvironment()
	if compiled, err := w.dme.PreviewEnvironment(); err == nil {
		w.dme = compiled
	}
	for path := range w.dme.Objects {
		w.items = append(w.items, path)
	}
	sort.Slice(w.items, func(i, j int) bool {
		a, b := w.itemName(w.items[i]), w.itemName(w.items[j])
		if a == b {
			return w.items[i] < w.items[j]
		}
		return a < b
	})
	app.CommandStorage().SetStack(w.CommandStackId())
	if len(w.catalog.Planets) > 0 {
		w.open(0)
	}
	return w
}
func (w *Workspace) Name() string {
	prefix := ""
	if w.IsModified() {
		prefix = "* "
	}
	return prefix + "Planet Workshop###" + w.Id()
}
func (w *Workspace) Title() string { return "Planet Workshop" }
func (w *Workspace) CommandStackId() string {
	if w.project == nil {
		return command.NullSpaceStackId
	}
	return w.planetStack(w.project.State.Definition.Path)
}
func (w *Workspace) planetStack(path string) string { return "planet:" + w.Id() + ":" + path }
func (w *Workspace) IsModified() bool {
	if w.pendingCreation() || w.currentModified() {
		return true
	}
	for _, draft := range w.drafts {
		if draft.project != w.project && (draft.nameDirty || draft.project.Modified()) {
			return true
		}
	}
	return false
}
func (*Workspace) Ini() workspace.Ini {
	return workspace.Ini{WindowFlags: imgui.WindowFlagsNoScrollbar | imgui.WindowFlagsNoScrollWithMouse}
}
func (*Workspace) OnFocusChange(f bool) {
	if f {
		tools.SetEnabled(false)
	}
}
func (w *Workspace) Dispose() {
	texture := w.climateTexture
	window.RunLater(func() { gl.DeleteTextures(1, &texture) })
	if w.canvas != nil {
		w.canvas.Dispose()
	}
	for path := range w.drafts {
		w.app.CommandStorage().DisposeStack(w.planetStack(path))
	}
}

func (w *Workspace) open(index int) {
	w.switchPlanet(w.catalog.Planets[index].Path)
}
func (w *Workspace) bind(p *planet.Project) {
	w.climateView = climateView{}
	w.nameDirty = false
	w.project = p
	w.last = planet.Clone(p.State)
	w.selected = ""
	w.table = 0
	w.selectedEntry, w.revealEntry = "", false
	w.picking = false
	w.pickingRiver, w.pickingEnvironment = false, false
	w.deleting = false
	w.renderKey = ""
	w.preview, w.scene = nil, nil
	w.fit = true
	w.mode = 0
	w.cave = false
	w.review = false
	used := p.State.UsedBiomes()
	if len(used) > 0 {
		w.selected = used[0]
	}
	w.biomeName = p.State.Biomes[w.selected].Name
	w.app.CommandStorage().DisposeStack(w.CommandStackId())
	w.app.CommandStorage().SetStack(w.CommandStackId())
	w.rememberCurrent()
}
func (w *Workspace) selectBiome(path string) bool {
	previous := w.selected
	if !w.commitName() {
		return false
	}
	if path == previous {
		path = w.selected // Committing a name can create a local copy.
	}
	changed := w.selected != path
	w.selected = path
	w.biomeName = w.project.State.Biomes[path].Name
	w.table = 0
	w.selectedEntry, w.revealEntry = "", false
	w.picking = false
	w.deleting = false
	if w.mode == 1 && changed {
		w.fit = true
	}
	return true
}
func (w *Workspace) record() {
	w.recordNamed("Edit planet")
}
func (w *Workspace) recordNamed(label string) {
	if w.project == nil || reflect.DeepEqual(w.last, w.project.State) {
		return
	}
	p := w.project
	focus := w.selected
	before, after := planet.Clone(w.last), planet.Clone(p.State)
	apply := func(s planet.State) {
		w.climateView.stroke = false
		w.climateView.before = nil
		w.nameDirty = false
		p.State = planet.Clone(s)
		w.project = p
		w.last = planet.Clone(s)
		w.renderKey = ""
		w.deleting = false
		w.message = ""
		for _, path := range s.VisibleBiomes() {
			if path == focus {
				w.selected = focus
			}
		}
		visible := false
		for _, path := range s.VisibleBiomes() {
			if path == w.selected {
				visible = true
			}
		}
		if !visible {
			w.selectBiome(s.UsedBiomes()[0])
		}
		w.biomeName = s.Biomes[w.selected].Name
		w.picking = false
		w.selectedEntry, w.revealEntry = "", false
	}
	w.app.CommandStorage().PushV(w.CommandStackId(), command.Make(label, func() { apply(before) }, func() { apply(after) }))
	w.last = after
}
func (w *Workspace) rebuild() {
	if w.project == nil {
		return
	}
	if w.iconRevision != dmicon.LayoutRevision && w.preview != nil {
		w.iconRevision = dmicon.LayoutRevision
		w.scene = mappreview.Build(w.preview.Map, w.dme, mappreview.Options{Smoothing: true, Lighting: w.lighting}, func(icon, state string) bool {
			d, e := dmicon.Cache.Get(icon)
			if e != nil {
				return false
			}
			_, ok := d.States[state]
			return ok
		})
		w.canvas.Render().ReplaceBucket(w.scene.Map, 1)
		w.canvas.Render().SetPreviewLighting(w.scene.Lighting)
	}
	o := planet.PreviewOptions{Caves: w.cave, Populate: w.population}
	if w.mode == 1 {
		o.Biome = w.selected
	}
	data, _ := json.Marshal(struct {
		S        planet.State
		O        planet.PreviewOptions
		Lighting bool
	}{w.project.State, o, w.lighting})
	key := string(data)
	if key == w.renderKey || time.Since(w.lastBuild) < 180*time.Millisecond {
		return
	}
	w.lastBuild = time.Now()
	w.renderKey = key
	p, err := planet.Generate(w.catalog, w.project.State, w.dme, o)
	if err != nil {
		w.message = err.Error()
		w.preview = nil
		w.scene = nil
		return
	}
	if w.preview == nil || w.preview.Map.MaxX != p.Map.MaxX || w.preview.Map.MaxY != p.Map.MaxY {
		w.fit = true
	}
	w.preview = p
	w.scene = mappreview.Build(p.Map, w.dme, mappreview.Options{Smoothing: true, Lighting: w.lighting}, func(icon, state string) bool {
		d, e := dmicon.Cache.Get(icon)
		if e != nil {
			return false
		}
		_, ok := d.States[state]
		return ok
	})
	w.canvas.Render().ReplaceBucket(w.scene.Map, 1)
	w.canvas.Render().SetPreviewLighting(w.scene.Lighting)
	w.canvas.Render().SetUnitProcessor(w)
}

func (w *Workspace) SpriteContext() (*dmmap.Dmm, *dmenv.Dme) {
	if w.preview == nil {
		return nil, w.dme
	}
	return w.preview.Map, w.dme
}

func (w *Workspace) commitName() bool {
	if !w.nameDirty || w.project == nil {
		return true
	}
	name := strings.TrimSpace(w.biomeName)
	if name == "" || len(name) > 80 {
		w.message = "Enter a biome name (1-80 characters)."
		return false
	}
	for _, path := range w.project.State.VisibleBiomes() {
		if path != w.selected && strings.EqualFold(strings.Join(strings.Fields(w.project.State.Biomes[path].Name), " "), strings.Join(strings.Fields(name), " ")) {
			w.message = "Another biome already uses that name."
			return false
		}
	}
	if w.project.State.Biomes[w.selected].Name != name {
		w.local()
		b := w.project.State.Biomes[w.selected]
		b.Name = name
		w.project.State.Biomes[w.selected] = b
	}
	w.nameDirty = false
	w.biomeName = name
	w.message = ""
	return true
}

func (w *Workspace) Process() {
	workshop.PushStyle()
	defer workshop.PopStyle()
	context := "Planets & biomes"
	if w.project != nil {
		context = w.project.State.Definition.Name
	}
	workshop.Banner("Planet Workshop", context, style.Teal)
	if w.catalog == nil {
		imgui.TextWrapped(w.message)
		return
	}
	s := window.PointSize()
	available := imgui.ContentRegionAvail().X
	rail := min(238*s, max(190*s, available*.18))
	workshop.Panel("planet-library", imgui.Vec2{X: rail}, false)
	w.library()
	workshop.EndPanel()
	imgui.SameLine()
	imgui.BeginChildV("planet-workspace", imgui.Vec2{}, false, imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
	switch {
	case w.creating:
		w.createForm()
	case w.project == nil:
		workshop.Title("Choose a planet")
	case w.cancelling:
		w.cancelPlanetForm()
	case w.review:
		w.reviewPanel()
	default:
		w.rebuild()
		if w.mode == 2 {
			w.visual()
			break
		}
		wide := imgui.ContentRegionAvail().X >= 840*s
		if !wide {
			if imgui.Button("Preview") {
				w.narrowEditor = false
			}
			imgui.SameLine()
			if imgui.Button("Edit biome") {
				w.narrowEditor = true
			}
		}
		showEditor := wide || w.narrowEditor
		if wide || !w.narrowEditor {
			width := float32(0)
			if wide {
				width = imgui.ContentRegionAvail().X - 330*s - 12*s
			}
			imgui.BeginChildV("planet-visual", imgui.Vec2{X: width}, false, imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
			w.visual()
			imgui.EndChild()
		}
		if wide {
			imgui.SameLine()
		}
		if showEditor {
			workshop.Panel("biome-editor", imgui.Vec2{}, false)
			w.editor()
			workshop.EndPanel()
		}
	}
	imgui.EndChild()
	if w.climateView.stroke && !imgui.IsMouseDown(imgui.MouseButtonLeft) {
		w.finishClimateStroke()
	}
	if !imgui.IsAnyItemActive() && !w.climateView.stroke {
		w.record()
	}
}

func (w *Workspace) visual() {
	for i, label := range []string{"Planet", "Biome sample", "Climate"} {
		if i > 0 {
			imgui.SameLine()
		}
		if imgui.Button(label) {
			w.mode = i
			w.fit = true
		}
	}
	if w.mode == 2 {
		w.climate()
		return
	}
	if w.mode == 1 {
		workshop.Title(w.project.State.Biomes[w.selected].Name)
	} else {
		workshop.Title(w.project.State.Definition.Name)
	}
	if w.mode != 1 && len(w.project.State.Definition.Caves) > 0 {
		imgui.Checkbox("Underground", &w.cave)
		imgui.SameLine()
	}
	imgui.Checkbox("Life & features", &w.population)
	if w.project.State.Definition.Environment != nil {
		imgui.SameLine()
		imgui.Checkbox("Daylight", &w.lighting)
	}
	imgui.SameLine()
	if imgui.Button("Fit") {
		w.fit = true
	}
	workshop.Muted("Click terrain, plants, features or creatures to inspect their choices. Scroll to zoom; middle-drag to pan.")
	if w.preview == nil {
		imgui.TextWrapped(w.message)
		return
	}
	w.previewCanvas(imgui.Vec2{Y: -46 * window.PointSize()})
	workshop.Muted("Seeded terrain and rivers. Ruins, weather and runtime mob behavior are not simulated.")
}

func (w *Workspace) previewCanvas(extent imgui.Vec2) {
	imgui.BeginChildV("planet-canvas", extent, false, imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
	size := imgui.ContentRegionAvail()
	if size.X > 1 && size.Y > 1 {
		camera := w.canvas.Render().Camera
		if !w.fit && w.canvasSize.X > 0 && w.canvasSize.Y > 0 {
			camera.ShiftX += (size.X - w.canvasSize.X) / (2 * camera.Scale)
			camera.ShiftY += (size.Y - w.canvasSize.Y) / (2 * camera.Scale)
		}
		w.canvasSize = size
		tileSize := float32(32)
		width := float32(w.preview.Map.MaxX) * tileSize
		if w.fit {
			camera.Scale = min(size.X/width, size.Y/width) * .96
			camera.ShiftX, camera.ShiftY = (size.X/camera.Scale-width)/2, (size.Y/camera.Scale-width)/2
			w.fit = false
		}
		w.control.Process(size)
		if w.control.Moving() {
			d := imgui.CurrentIO().MouseDelta()
			camera.Translate(d.X/camera.Scale, -d.Y/camera.Scale)
		}
		if w.control.Active() {
			_, wheel := imgui.CurrentIO().MouseWheel()
			if wheel != 0 {
				mouse := imgui.MousePos().Minus(w.control.PosMin())
				before := camera.Scale
				camera.Scale = max(.03, min(12, camera.Scale*float32(math.Pow(math.Sqrt2, float64(wheel)))))
				camera.ShiftX += mouse.X/camera.Scale - mouse.X/before
				camera.ShiftY += (size.Y-mouse.Y)/camera.Scale - (size.Y-mouse.Y)/before
			}
		}
		mouse := imgui.MousePos().Minus(w.control.PosMin())
		w.mouseWorld = imgui.Vec2{X: mouse.X/camera.Scale - camera.ShiftX, Y: (size.Y-mouse.Y)/camera.Scale - camera.ShiftY}
		if w.mode == 2 {
			w.climateMapInput()
		}
		w.hoverActive = w.control.Active()
		w.hovered = nil
		w.canvas.Process(size)
		imgui.WindowDrawList().AddImageV(imgui.TextureID(w.canvas.Texture()), w.control.PosMin(), w.control.PosMax(), imgui.Vec2{Y: 1}, imgui.Vec2{X: 1}, style.ColorWhitePacked)
		if w.mode == 2 {
			w.climateOverlay()
		} else if w.control.Active() {
			if target, ok := w.previewTarget(); ok {
				if imgui.IsMouseClicked(imgui.MouseButtonLeft) && !w.control.Moving() {
					w.inspectPreview(target)
				}
				imgui.BeginTooltip()
				imgui.Text(w.itemName(target.path))
				imgui.Text(w.project.State.Biomes[target.cell.Biome].Name + " / " + target.group)
				imgui.TextDisabled(fmt.Sprintf("Heat %.0f%% / Moisture %.0f%%", target.cell.Heat*100, target.cell.Moisture*100))
				imgui.EndTooltip()
			}
		}
	}
	imgui.EndChild()
}
