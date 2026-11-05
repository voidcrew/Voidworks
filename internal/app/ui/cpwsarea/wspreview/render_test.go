package wspreview

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/mappreview"
	"sdmm/internal/platform"
	"sdmm/internal/util"
)

// Uses real tg definitions and DMI sprites, without saving or modifying any map.
func TestRenderPreview(t *testing.T) {
	path := os.Getenv("SHIP_RENDER_TEST_DME")
	if path == "" {
		t.Skip("set SHIP_RENDER_TEST_DME for the native rendering test")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := glfw.Init(); err != nil {
		t.Fatal(err)
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	const width, height = 1100, 850
	w, err := glfw.CreateWindow(width, height, "Map preview test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Destroy()
	w.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		t.Fatal(err)
	}
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	window.ApplyDefaultTheme()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: width, Y: height})
	io.SetDisplayFrameBufferScale(imgui.Vec2{X: 1, Y: 1})
	io.SetDeltaTime(1.0 / 60)
	platform.InitImGuiGL()
	defer platform.DisposeImGuiGL()
	window.SetPointSize(1)
	dme, err := dmenv.New(path)
	if err != nil {
		t.Fatal(err)
	}
	dmmap.PrefabStorage.Free()
	dmicon.Cache.Free()
	dmmap.Init(dme)
	dmicon.Cache.SetRootDirPath(dme.RootDir)
	get := func(path string, fields map[string]string) *dmmprefab.Prefab {
		t.Helper()
		object := dme.Objects[path]
		if object == nil {
			t.Fatalf("missing fixture type %s", path)
		}
		vars := dmvars.FromParent(object.Vars)
		for name, value := range fields {
			vars = dmvars.Set(vars, name, value)
		}
		return dmmprefab.New(dmmprefab.IdStage, path, vars)
	}
	m := &dmmap.Dmm{Name: "tg-preview-test.dmm", MaxX: 15, MaxY: 11, MaxZ: 1}
	floor := get("/turf/open/floor/iron", nil)
	wall := get("/turf/closed/wall", nil)
	area := get("/area", map[string]string{"static_lighting": "1", "base_lighting_alpha": "0"})
	glass := get("/obj/structure/window/fulltile", nil)
	for y := 1; y <= m.MaxY; y++ {
		for x := 1; x <= m.MaxX; x++ {
			tile := &dmmap.Tile{Coord: util.Point{X: x, Y: y, Z: 1}}
			tile.InstancesAdd(area)
			if x == 1 || y == 1 || x == m.MaxX || y == m.MaxY || (x == 8 && (y < 5 || y > 7)) {
				tile.InstancesAdd(wall)
			} else {
				tile.InstancesAdd(floor)
			}
			if x == 8 && y >= 5 && y <= 7 {
				tile.InstancesAdd(glass)
			}
			if x == 4 && y == 8 {
				tile.InstancesAdd(get("/obj/machinery/light", map[string]string{"dir": "1", "bulb_colour": `"#ff8877"`}))
			}
			if x == 12 && y == 4 {
				tile.InstancesAdd(get("/obj/machinery/light", map[string]string{"dir": "2", "bulb_colour": `"#7799ff"`}))
			}
			m.Tiles = append(m.Tiles, tile)
		}
	}
	before := make([]*dmmprefab.Prefab, 0)
	for _, tile := range m.Tiles {
		for _, instance := range tile.Instances() {
			before = append(before, instance.Prefab())
		}
	}
	p := New(mappreview.Snapshot(m, 1), dme)
	defer p.Dispose()
	if p.message != "" {
		t.Fatal(p.message)
	}
	if space := p.dme.Objects["/turf/open/space"]; space != nil && space.Vars.TextV("icon_state", "") != "space" {
		t.Fatalf("compiled space appearance not selected: %s", space.Vars.ValueV("icon_state", ""))
	}
	if p.scene.Smoothed < 50 || p.scene.Lights != 2 || len(p.scene.Notes) != 0 {
		t.Fatalf("unexpected scene: smoothed=%d lights=%d notes=%v", p.scene.Smoothed, p.scene.Lights, p.scene.Notes)
	}
	render := func(viewWidth float32) {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.Viewport(0, 0, width, height)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: viewWidth, Y: height})
		imgui.BeginV("Map preview", nil, imgui.WindowFlagsNoResize|imgui.WindowFlagsNoMove|imgui.WindowFlagsNoCollapse)
		p.Process()
		imgui.End()
		imgui.Render()
		platform.Render(imgui.RenderedDrawData())
		gl.Finish()
	}
	dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT")
	if dst == "" {
		dst = t.TempDir()
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	var previous []byte
	for _, mode := range []struct {
		name                          string
		smoothing, lighting, exterior bool
	}{{"lit", true, true, true}, {"interior-only", true, true, false}, {"smooth", true, false, false}, {"raw", false, false, false}} {
		p.options.Smoothing, p.options.Lighting, p.options.ExteriorLight = mode.smoothing, mode.lighting, mode.exterior
		p.rebuild()
		for i := 0; i < 3; i++ {
			render(width)
		}
		pixels := make([]byte, width*height*4)
		gl.ReadPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
		frame := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			copy(frame.Pix[y*frame.Stride:(y+1)*frame.Stride], pixels[(height-1-y)*width*4:(height-y)*width*4])
		}
		f, err := os.Create(filepath.Join(dst, "preview-"+mode.name+"-ui.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, frame)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dst, "preview-"+mode.name+".png")
		if err := p.exportTo(file); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && bytes.Equal(data, previous) {
			t.Fatal("preview toggles did not change exported image")
		}
		previous = data
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if config.Width != 480 || config.Height != 352 {
			t.Fatalf("wrong export dimensions: %+v", config)
		}
	}
	p.options.Smoothing, p.options.Lighting, p.options.ExteriorLight = true, true, true
	p.rebuild()
	p.fit = true
	for i := 0; i < 3; i++ {
		render(650)
	}
	// The view follows the canvas size until the user pans or zooms it.
	camera := p.canvas.Render().Camera
	fitted := camera.Scale
	render(1000)
	wider := camera.Scale
	if wider <= fitted {
		t.Fatalf("view did not refit a wider canvas: %v -> %v", fitted, wider)
	}
	io.SetMousePosition(imgui.Vec2{X: 500, Y: 500})
	render(1000)
	io.AddMouseWheelDelta(0, 1)
	render(1000)
	io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
	zoomed := camera.Scale
	if zoomed <= wider {
		t.Fatalf("the wheel did not zoom: %v -> %v", wider, zoomed)
	}
	render(650)
	if camera.Scale != zoomed {
		t.Fatalf("resizing discarded the user's zoom: %v -> %v", zoomed, camera.Scale)
	}
	p.Fit()
	render(650)
	if camera.Scale != fitted {
		t.Fatalf("Fit did not frame the map: %v, wanted %v", camera.Scale, fitted)
	}
	i := 0
	for _, tile := range m.Tiles {
		for _, instance := range tile.Instances() {
			if before[i] != instance.Prefab() {
				t.Fatal("preview modified source map")
			}
			i++
		}
	}
	if err := gl.GetError(); err != gl.NO_ERROR {
		t.Fatalf("OpenGL error: %x", err)
	}
	exerciseDoorOcclusion(t, dme)
	exerciseAssembledPreview(t, dme, dst)
	if file := os.Getenv("MAP_PREVIEW_TEST_MAP"); file != "" {
		original, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		data, err := dmmdata.New(file)
		if err != nil {
			t.Fatal(err)
		}
		mapped, unknown := dmmap.New(dme, data, "")
		if len(unknown) > 0 {
			t.Fatalf("unknown map types: %v", unknown)
		}
		start := time.Now()
		actual := New(mappreview.Snapshot(mapped, 1), dme)
		defer actual.Dispose()
		t.Logf("%s: %dx%d, %d smoothed, %d lights, %s; notes: %v", mapped.Name, mapped.MaxX, mapped.MaxY, actual.scene.Smoothed, actual.scene.Lights, time.Since(start), actual.scene.Notes)
		if err := actual.exportTo(filepath.Join(dst, "preview-map-lit.png")); err != nil {
			t.Fatal(err)
		}
		actual.options.ExteriorLight = false
		actual.rebuild()
		if err := actual.exportTo(filepath.Join(dst, "preview-map-interior-only.png")); err != nil {
			t.Fatal(err)
		}
		actual.options.Lighting = false
		actual.rebuild()
		if err := actual.exportTo(filepath.Join(dst, "preview-map-smooth.png")); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, after) {
			t.Fatal("preview changed source map file")
		}
	}
}
