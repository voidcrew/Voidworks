package wsplanet

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/command"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/platform"
)

type testApp struct {
	dme      *dmenv.Dme
	commands *command.Storage
}

func (a *testApp) LoadedEnvironment() *dmenv.Dme    { return a.dme }
func (a *testApp) CommandStorage() *command.Storage { return a.commands }

func TestNativePlanetWorkshop(t *testing.T) {
	path := os.Getenv("PLANET_TEST_DME")
	output := os.Getenv("PLANET_TEST_OUTPUT")
	if path == "" || output == "" {
		t.Skip("set PLANET_TEST_DME and PLANET_TEST_OUTPUT for native UI acceptance")
	}
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", t.TempDir())
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := glfw.Init(); e != nil {
		t.Fatal(e)
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	const width, height = 1600, 1000
	h, e := glfw.CreateWindow(width, height, "Planet workshop test", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Destroy()
	h.MakeContextCurrent()
	if e = gl.Init(); e != nil {
		t.Fatal(e)
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
	dme, e := dmenv.New(path)
	if e != nil {
		t.Fatal(e)
	}
	dmmap.PrefabStorage.Free()
	dmicon.Cache.Free()
	dmmap.Init(dme)
	dmicon.Cache.SetRootDirPath(dme.RootDir)
	defer dmicon.Cache.Free()
	a := &planetSpriteTestApp{testApp: &testApp{dme, command.NewStorage()}}
	ws := New(a)
	a.planet = ws
	defer ws.Dispose()
	if ws.project == nil {
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
		imgui.BeginV("Planet Workshop", nil, imgui.WindowFlagsNoResize|imgui.WindowFlagsNoMove|imgui.WindowFlagsNoCollapse)
		if a.showSprite {
			a.sprite.Process()
		} else {
			ws.Process()
		}
		imgui.End()
		imgui.Render()
		platform.Render(imgui.RenderedDrawData())
		gl.Finish()
	}
	if e = os.MkdirAll(output, 0700); e != nil {
		t.Fatal(e)
	}
	capture := func(name string) {
		t.Helper()
		ws.lastBuild = time.Time{}
		for i := 0; i < 3; i++ {
			render()
		}
		pixels := make([]byte, width*height*4)
		gl.ReadPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
		// The desktop framebuffer is opaque. Its blend alpha is not premultiplied
		// PNG alpha; preserving it corrupts translucent overlays in screenshots.
		for i := 3; i < len(pixels); i += 4 {
			pixels[i] = 255
		}
		frame := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			copy(frame.Pix[y*frame.Stride:(y+1)*frame.Stride], pixels[(height-1-y)*width*4:(height-y)*width*4])
		}
		f, e := os.Create(filepath.Join(output, name+".png"))
		if e != nil {
			t.Fatal(e)
		}
		if e = png.Encode(f, frame); e != nil {
			t.Fatal(e)
		}
		_ = f.Close()
	}
	capture("planet")
	// Restoring a maximized editor must keep the planet visible, even when
	// the first fit centered it in a much wider canvas.
	viewWidth, viewHeight = 3200, 1100
	ws.fit = true
	capture("wide-planet")
	viewWidth, viewHeight = 1000, 720
	capture("restored-planet")
	colors := map[[3]byte]bool{}
	pixels := ws.canvas.ReadPixels()
	for i := 0; i+3 < len(pixels); i += 4 {
		colors[[3]byte{pixels[i], pixels[i+1], pixels[i+2]}] = true
	}
	if len(colors) < 20 {
		t.Fatal("restoring a smaller window moved the planet out of view")
	}
	viewWidth, viewHeight = width, height
	capture("planet")
	if ws.preview == nil || ws.preview.Map.MaxX != 123 {
		t.Fatal("no planet rendered", ws.message)
	}
	testPreviewInspection(t, ws, a, io, render, capture, &viewWidth, &viewHeight)
	testPlanetDraftNavigation(t, ws, capture)
	testBiomeReorder(t, ws, io, render, capture)
	testClimatePad(t, ws, io, render, capture)
	testCavePicker(t, ws, capture)
	testRiverPreview(t, ws, io, render, capture)
	ws.mode = 1
	ws.fit = true
	capture("biome")
	ws.picking = true
	ws.filter = "sand"
	capture("picker")
	ws.picking = false
	ws.mode = 2
	capture("climate")
	testClimatePainting(t, ws, io, render, capture, &viewWidth, &viewHeight)
	ws.mode = 3
	capture("terrain")
	ws.mode = 0
	// Deletion has its own undo step, including every climate assignment.
	deleted := ws.selected
	ws.beginDeleteBiome()
	capture("delete-biome")
	_, caves := ws.project.BiomeUse(deleted)
	for _, choice := range ws.project.BiomeChoices(caves > 0) {
		if choice.Path != deleted {
			ws.deleteReplacement = choice.Path
			break
		}
	}
	if ws.deleteReplacement == "" {
		t.Fatal("missing deletion replacement")
	}
	capture("delete-replacement")
	ws.deleteBiome()
	capture("deleted-planet")
	if _, ok := ws.project.State.Biomes[deleted]; ok || ws.preview == nil {
		t.Fatal("deletion failed", ws.message)
	}
	if ws.preview.Counts[deleted] != 0 {
		t.Fatal("deleted biome remains in the planet preview")
	}
	a.commands.Undo()
	if ws.IsModified() || ws.selected != deleted {
		t.Fatal("deletion undo did not restore the biome and clean state")
	}
	a.commands.Redo()
	if _, ok := ws.project.State.Biomes[deleted]; ok || !ws.IsModified() {
		t.Fatal("deletion redo failed")
	}
	a.commands.Undo()
	ws.addBiome()
	added := ws.selected
	ws.beginDeleteBiome()
	capture("delete-unassigned")
	ws.deleteBiome()
	if _, ok := ws.project.State.Biomes[added]; ok {
		t.Fatal("unassigned biome deletion failed")
	}
	a.commands.Undo()
	if ws.selected != added {
		t.Fatal("unassigned biome deletion undo failed")
	}
	a.commands.Undo() // Undo creation as well.
	ws.mode = 0
	// A local turf replacement must update the native preview and remain undoable.
	before := ws.selected
	ws.local()
	b := ws.project.State.Biomes[ws.selected]
	b.Name = "Local test meadow"
	ws.project.State.Biomes[ws.selected] = b
	ws.record()
	render()
	if !ws.IsModified() {
		t.Fatal("biome edit is not dirty")
	}
	a.commands.Undo()
	if ws.IsModified() || ws.selected != before {
		t.Fatal("undo did not restore original biome")
	}
	a.commands.Redo()
	if !ws.IsModified() {
		t.Fatal("redo lost edit")
	}
	a.commands.Undo()
	// Restore an edited biome without discarding an unrelated terrain edit.
	ws.local()
	localPath := ws.selected
	edited := ws.project.State.Biomes[localPath]
	edited.Name = "Discard test biome"
	ws.project.State.Biomes[localPath] = edited
	ws.project.State.Seeds.Heat++
	ws.record()
	ws.discardBiomeChanges()
	if ws.selected != before || ws.project.State.Seeds.Heat != ws.last.Seeds.Heat {
		t.Fatal("discard affected another setting")
	}
	a.commands.Undo()
	if ws.project.State.Biomes[localPath].Name != edited.Name {
		t.Fatal("discard biome changes could not be undone")
	}
	a.commands.Undo()
	for i, d := range ws.catalog.Planets {
		if strings.HasSuffix(d.Path, "/lava") {
			ws.open(i)
			capture("lava")
			if ws.preview == nil {
				t.Fatal(ws.message)
			}
		}
	}
	viewWidth, viewHeight = 1000, 760
	ws.fit = true
	capture("compact-preview")
	ws.narrowEditor = true
	capture("compact-editor")
	viewWidth, viewHeight = width, height
	if ws.project.State.Definition.Environment != nil {
		testPlanetSettingsScreens(t, ws, capture, &viewWidth, &viewHeight)
	}
	ws.creating = true
	capture("new-planet")
	if err := gl.GetError(); err != gl.NO_ERROR {
		t.Fatalf("OpenGL error %x", err)
	}
}
