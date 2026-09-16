package wspreview

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/sqweek/dialog"
	"sdmm/internal/app/render/bucket/level/chunk/unit"
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/canvas"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/mappreview"
)

type Preview struct {
	workspace.Content
	source       *dmmap.Dmm
	dme          *dmenv.Dme
	scene        *mappreview.Scene
	options      mappreview.Options
	canvas       *canvas.Canvas
	control      *canvas.Control
	fit          bool
	message      string
	iconRevision uint64
}

func New(source *dmmap.Dmm, dme *dmenv.Dme) *Preview {
	return NewWithOptions(source, dme, mappreview.Options{Smoothing: true, Lighting: true, PoweredFixtures: true, ExteriorLight: true})
}

func NewWithOptions(source *dmmap.Dmm, dme *dmenv.Dme, options mappreview.Options) *Preview {
	return newPreview(source, dme, options, true)
}

// NewSprite keeps the map's loaded appearances. Opening an icon from a map
// must not synchronously parse the whole game again just to show its context.
func NewSprite(source *dmmap.Dmm, dme *dmenv.Dme) *Preview {
	return newPreview(source, dme, mappreview.Options{Smoothing: true, Lighting: true, PoweredFixtures: true, ExteriorLight: true}, false)
}

func newPreview(source *dmmap.Dmm, dme *dmenv.Dme, options mappreview.Options, compile bool) *Preview {
	p := &Preview{source: source, dme: dme, canvas: canvas.New(), control: canvas.NewControl(), fit: true,
		options: options}
	p.control.AtCursor = true
	if compile {
		if compiled, err := dme.PreviewEnvironment(); err == nil {
			p.source = mappreview.CompiledAppearances(source, dme, compiled)
			p.dme = compiled
		} else {
			p.message = "Compiled appearances could not be loaded; showing editor sprites. " + err.Error()
		}
	}
	p.canvas.ClearColor = canvas.Color{R: .045, G: .05, B: .06, A: 1}
	p.rebuild()
	return p
}

func (p *Preview) Name() string  { return "Preview: " + p.source.Name }
func (p *Preview) Title() string { return p.Name() }
func (*Preview) Ini() workspace.Ini {
	return workspace.Ini{WindowFlags: imgui.WindowFlagsNoScrollbar | imgui.WindowFlagsNoScrollWithMouse}
}
func (*Preview) OnFocusChange(focused bool) {
	if focused {
		tools.SetEnabled(false)
	}
}
func (p *Preview) Dispose() { p.canvas.Dispose() }

// ReplaceSource refreshes an embedded preview while retaining its view controls.
func (p *Preview) ReplaceSource(source *dmmap.Dmm, editor *dmenv.Dme) {
	p.fit = p.source.MaxX != source.MaxX || p.source.MaxY != source.MaxY
	p.source = mappreview.CompiledAppearances(source, editor, p.dme)
	p.rebuild()
}
func (*Preview) ProcessUnit(u unit.Unit) bool { return mappreview.Visible(u.Instance().Prefab()) }

func hasState(icon, state string) bool {
	dmi, err := dmicon.Cache.Get(icon)
	if err != nil {
		return false
	}
	_, ok := dmi.States[state]
	return ok
}

func (p *Preview) rebuild() {
	p.iconRevision = dmicon.LayoutRevision
	p.scene = mappreview.Build(p.source, p.dme, p.options, hasState)
	p.canvas.Render().ReplaceBucket(p.scene.Map, 1)
	p.canvas.Render().SetUnitProcessor(p)
	p.canvas.Render().SetPreviewLighting(p.scene.Lighting)
}

func (p *Preview) Process() {
	if p.iconRevision != dmicon.LayoutRevision {
		p.rebuild()
	}
	width := imgui.ContentRegionAvail().X
	changed := imgui.Checkbox("Smoothing", &p.options.Smoothing)
	imgui.SameLine()
	changed = imgui.Checkbox("Lighting estimate", &p.options.Lighting) || changed
	if p.options.Lighting {
		imgui.SameLine()
		changed = imgui.Checkbox("Powered fixtures", &p.options.PoweredFixtures) || changed
		if width >= 600 {
			imgui.SameLine()
		}
		changed = imgui.Checkbox("Exterior light", &p.options.ExteriorLight) || changed
	}
	if width >= 740 {
		imgui.SameLine()
	}
	if imgui.Button("Fit") {
		p.fit = true
	}
	imgui.SameLine()
	if imgui.Button("Export PNG") {
		p.export()
	}
	if changed {
		p.rebuild()
	}
	imgui.TextDisabled(fmt.Sprintf("%d smoothed / %d lights | Current map snapshot", p.scene.Smoothed, p.scene.Lights))
	if p.options.Lighting {
		imgui.TextWrapped("Estimated lighting. Power simulation and runtime-created effects may differ in game.")
	}
	if len(p.scene.Notes) > 0 && imgui.CollapsingHeader(fmt.Sprintf("Preview notes (%d)", len(p.scene.Notes))) {
		imgui.BeginChildV("preview-notes", imgui.Vec2{Y: 100}, true, 0)
		for _, note := range p.scene.Notes {
			imgui.TextWrapped(note)
		}
		imgui.EndChild()
	}
	if p.message != "" {
		imgui.TextWrapped(p.message)
	}
	imgui.BeginChildV("preview-canvas", imgui.Vec2{}, false, imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
	size := imgui.ContentRegionAvail()
	if size.X >= 1 && size.Y >= 1 {
		camera := p.canvas.Render().Camera
		if p.fit {
			width, height := float32(p.source.MaxX*dmmap.WorldIconSize), float32(p.source.MaxY*dmmap.WorldIconSize)
			camera.Scale = min(size.X/width, size.Y/height) * .95
			camera.ShiftX, camera.ShiftY = (size.X/camera.Scale-width)/2, (size.Y/camera.Scale-height)/2
			p.fit = false
		}
		p.control.Process(size)
		if p.control.Moving() {
			delta := imgui.CurrentIO().MouseDelta()
			camera.Translate(delta.X/camera.Scale, -delta.Y/camera.Scale)
		}
		if p.control.Active() {
			_, wheel := imgui.CurrentIO().MouseWheel()
			if wheel != 0 {
				mouse := imgui.MousePos().Minus(p.control.PosMin())
				before := camera.Scale
				camera.Scale = max(.02, min(32, camera.Scale*float32(math.Pow(math.Sqrt2, float64(wheel)))))
				camera.ShiftX += mouse.X/camera.Scale - mouse.X/before
				camera.ShiftY += (size.Y-mouse.Y)/camera.Scale - (size.Y-mouse.Y)/before
			}
		}
		p.canvas.Process(size)
		imgui.WindowDrawList().AddImageV(imgui.TextureID(p.canvas.Texture()), p.control.PosMin(), p.control.PosMax(), imgui.Vec2{X: 0, Y: 1}, imgui.Vec2{X: 1, Y: 0}, style.ColorWhitePacked)
	}
	imgui.EndChild()
}

func (p *Preview) SpriteContext() (*dmmap.Dmm, *dmenv.Dme) { return p.source, p.dme }

func (p *Preview) export() {
	name := strings.TrimSuffix(filepath.Base(p.source.Name), filepath.Ext(p.source.Name)) + "-preview.png"
	path, err := dialog.File().Title("Export map preview").Filter("PNG image", "png").SetStartFile(name).Save()
	if err != nil {
		return
	}
	if filepath.Ext(path) == "" {
		path += ".png"
	}
	if err = p.exportTo(path); err != nil {
		p.message = "Export failed: " + err.Error()
	} else {
		p.message = "Saved " + filepath.Base(path)
	}
}

func (p *Preview) exportTo(path string) error {
	width, height := p.source.MaxX*dmmap.WorldIconSize, p.source.MaxY*dmmap.WorldIconSize
	var limit int32
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &limit)
	if width > int(limit) || height > int(limit) || width*height > 64*1024*1024 {
		return fmt.Errorf("map is too large for a single image (%d x %d)", width, height)
	}
	c := canvas.New()
	defer c.Dispose()
	c.ClearColor = canvas.Color{A: 1}
	c.Render().UpdateBucket(p.scene.Map, 1)
	c.Render().SetUnitProcessor(p)
	c.Render().SetPreviewLighting(p.scene.Lighting)
	c.Process(imgui.Vec2{X: float32(width), Y: float32(height)})
	pixels := c.ReadPixels()
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		copy(result.Pix[y*result.Stride:(y+1)*result.Stride], pixels[(height-1-y)*width*4:(height-y)*width*4])
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(file, result)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
