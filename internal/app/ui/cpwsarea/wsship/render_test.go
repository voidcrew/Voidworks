package wsship

import (
	"crypto/sha256"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/platform"
)

// Opt-in native UI smoke test. It creates only a hidden window and reads maps.
func TestRenderShipWorkspace(t *testing.T) {
	path := os.Getenv("SHIP_RENDER_TEST_DME")
	if path == "" {
		t.Skip("set SHIP_RENDER_TEST_DME for the native rendering test")
	}
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", t.TempDir())
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
	const width, height = 1400, 960
	w, err := glfw.CreateWindow(width, height, "Ship workspace test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Destroy()
	w.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		t.Fatal(err)
	}
	context := imgui.CreateContext(nil)
	defer context.Destroy()
	window.ApplyDefaultTheme()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: width, Y: height})
	io.SetDisplayFrameBufferScale(imgui.Vec2{X: 1, Y: 1})
	io.SetDeltaTime(1.0 / 60)
	platform.InitImGuiGL()
	defer platform.DisposeImGuiGL()
	window.SetPointSize(1)
	exerciseDropdowns(t)
	dme, err := dmenv.New(path)
	if err != nil {
		t.Fatal(err)
	}
	dmmap.PrefabStorage.Free()
	dmicon.Cache.Free()
	dmmap.Init(dme)
	dmicon.Cache.SetRootDirPath(dme.RootDir)
	app := &previewApp{dme: dme}
	ws := New(app)
	defer ws.Dispose()
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	viewWidth, viewHeight := width, height
	render := func() {
		io.SetDisplaySize(imgui.Vec2{X: float32(viewWidth), Y: float32(viewHeight)})
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.Viewport(0, 0, width, height)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: float32(viewWidth), Y: float32(viewHeight)})
		imgui.BeginV("Voidcrew Ship Workspace", nil, imgui.WindowFlagsNoResize|imgui.WindowFlagsNoMove|imgui.WindowFlagsNoCollapse)
		ws.Process()
		imgui.End()
		dialog.Process()
		imgui.Render()
		platform.Render(imgui.RenderedDrawData())
		gl.Finish()
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "choose-ship.png"), width, height)
	}
	if ws.Map() != nil {
		t.Fatal("ship chooser exposes an editable map")
	}
	hashes := map[string][32]byte{}
	remember := func() {
		for _, source := range ws.assembly.Sources {
			data, err := os.ReadFile(source.File)
			if err != nil {
				t.Fatal(err)
			}
			hashes[source.File] = sha256.Sum256(data)
		}
	}
	for _, hull := range []string{"delta", "scarab"} {
		found := false
		for i, h := range ws.catalog.Hulls {
			if strings.HasSuffix(h.Type, "/"+hull) {
				ws.hull = i
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing hull %s", hull)
		}
		ws.theme = 0
		ws.defaults()
		ws.rebuild()
		ws.setStage(stepBuild)
		ws.pane.FitView()
		if ws.message != "" {
			t.Fatal(ws.message)
		}
		remember()
		ws.source = 1
		ws.rebuild()
		ws.OnFocusChange(true)
		for i := 0; i < 3; i++ {
			render()
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
			pixels := make([]byte, width*height*4)
			gl.ReadPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
			frame := image.NewRGBA(image.Rect(0, 0, width, height))
			for y := 0; y < height; y++ {
				copy(frame.Pix[y*frame.Stride:(y+1)*frame.Stride], pixels[(height-1-y)*width*4:(height-y)*width*4])
			}
			file, err := os.Create(filepath.Join(dst, hull+"-workspace.png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, frame); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}
		exerciseVisibilityExceptions(t, ws, render)
		// Exercise a theme switch, module switch and return to the bare hull.
		if len(ws.catalog.Hulls[ws.hull].Themes) > 1 {
			ws.theme = 1
			ws.defaults()
			ws.rebuild()
			if ws.message != "" {
				t.Fatal(ws.message)
			}
			remember()
			render()
		}
		for _, m := range ws.catalog.Hulls[ws.hull].Modules {
			if m.Available(ws.currentTheme().ID) && ws.selected[m.Slot] != m.ID {
				ws.selected[m.Slot] = m.ID
				ws.rebuild()
				if ws.message != "" {
					t.Fatal(ws.message)
				}
				remember()
				render()
				break
			}
		}
		ws.selected = map[string]string{}
		ws.rebuild()
		render()
	}
	for path, before := range hashes {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if sha256.Sum256(data) != before {
			t.Fatalf("preview changed source %s", path)
		}
	}
	exerciseFleetRoomRename(t, ws)
	exerciseJeanShorts(t, ws, render)
	exerciseResponsiveCrew(t, ws, render, func(w, h int) { viewWidth, viewHeight = w, h })
	exerciseResponsiveCosts(t, ws, render, func(w, h int) { viewWidth, viewHeight = w, h })
	viewWidth, viewHeight = width, height
	exerciseAuthoring(t, ws, dme, render)
	if code := gl.GetError(); code != gl.NO_ERROR {
		t.Fatalf("OpenGL error: %x", code)
	}
}
