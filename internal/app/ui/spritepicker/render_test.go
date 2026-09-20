package spritepicker

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/dmi"
	"sdmm/internal/platform"
)

func TestNativeSpritePicker(t *testing.T) {
	output := os.Getenv("SPRITE_PICKER_TEST_OUTPUT")
	if output == "" {
		t.Skip("set SPRITE_PICKER_TEST_OUTPUT for native picker tests")
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
	const width, height = 1200, 900
	win, err := glfw.CreateWindow(width, height, "Sprite picker test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer win.Destroy()
	win.MakeContextCurrent()
	if err = gl.Init(); err != nil {
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
	platform.InitImGuiGLFW()
	platform.InitImGuiGL()
	defer platform.DisposeImGuiGL()
	window.SetPointSize(1)
	root := t.TempDir()
	doc, _ := dmi.Create(32, 32)
	doc.Icon.States = nil
	for n, name := range []string{"zebra", "idle", "Alpha", ""} {
		state := dmi.NewState(name, 32, 32)
		for y := 4; y < 28; y++ {
			for x := 8; x < 24; x++ {
				state.Cels[0].SetNRGBA(x, y, color.NRGBA{R: uint8(80 + n*45), G: 150, B: 100, A: 255})
			}
		}
		doc.Icon.States = append(doc.Icon.States, state)
	}
	if err = doc.Save(filepath.Join(root, "current.dmi"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	dmicon.Cache.Free()
	dmicon.Cache.SetRootDirPath(root)
	defer dmicon.Cache.Free()
	vars := dmvars.Set(&dmvars.Variables{}, "icon", "'current.dmi'")
	vars = dmvars.Set(vars, "icon_state", `"idle"`)
	vars = dmvars.Set(vars, "name", `"Test chair"`)
	original := dmmprefab.New(0, "/obj/chair", vars)
	var applied *dmmprefab.Prefab
	p := newPicker(root, original, "Replace this instance · test map", func(next *dmmprefab.Prefab) error { applied = next; return nil })
	dialog.Open(p)
	defer dialog.Close(p)
	viewWidth, viewHeight := width, height
	popupOpen := false
	render := func() {
		gl.Viewport(0, 0, int32(viewWidth), int32(viewHeight))
		gl.Clear(gl.COLOR_BUFFER_BIT)
		imgui.NewFrame()
		dialog.Process()
		popupOpen = imgui.IsPopupOpen(p.Name())
		imgui.Render()
		platform.Render(imgui.RenderedDrawData())
		gl.Finish()
	}
	click := func(x, y float32) {
		io.SetMousePosition(imgui.Vec2{X: x, Y: y})
		render()
		io.SetMouseButtonDown(0, true)
		render()
		io.SetMouseButtonDown(0, false)
		render()
	}
	capture := func(name string) {
		width, height := viewWidth, viewHeight
		pixels := make([]byte, width*height*4)
		gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
		frame := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			copy(frame.Pix[y*frame.Stride:(y+1)*frame.Stride], pixels[(height-1-y)*width*4:(height-y)*width*4])
		}
		for n := 3; n < len(frame.Pix); n += 4 {
			frame.Pix[n] = 255
		}
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(output, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, frame); err != nil {
			t.Fatal(err)
		}
	}
	for range 4 {
		render()
	}
	if p.all || p.file != "current.dmi" || p.state != "idle" || !p.selected || !reflect.DeepEqual(p.states, []string{"", "Alpha", "idle", "zebra"}) {
		t.Fatal("wrong initial picker", p)
	}
	capture("current-dmi")
	click(550, 377)
	if p.state != "Alpha" || applied != nil {
		t.Fatal("sprite row did not select without applying", p.state)
	}
	click(160, 791)
	if applied != nil || popupOpen {
		t.Fatal("Cancel applied a sprite or left picker open")
	}
	dialog.Open(p)
	for range 4 {
		render()
	}
	// Saving an entirely new document makes it available without reparsing DM.
	newDoc, _ := dmi.Create(64, 32)
	newDoc.Icon.States[0].Name = "new sprite"
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			newDoc.Icon.States[0].Cels[0].SetNRGBA(x, y, color.NRGBA{G: 180, B: 220, A: 255})
		}
	}
	if err = newDoc.Save(filepath.Join(root, "fresh.dmi"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p.files, err = scanDMIs(root)
	if err != nil {
		t.Fatal(err)
	}
	click(240, 177)
	click(220, 244)
	if !p.all {
		t.Fatal("All DMIs control failed")
	}
	for range 3 {
		render()
	}
	click(240, 351)
	for range 3 {
		render()
	}
	if p.selected || !reflect.DeepEqual(p.states, []string{"new sprite"}) {
		t.Fatal("new DMI not loaded")
	}
	capture("all-dmis")
	click(550, 313)
	if p.state != "new sprite" || !p.selected || applied != nil {
		t.Fatal("new sprite row failed")
	}
	click(250, 791)
	if applied == nil || applied.Vars().TextV("icon", "") != "fresh.dmi" || applied.Path() != original.Path() || popupOpen {
		t.Fatal("apply failed", p.message)
	}
	dialog.Open(p)
	// A missing/corrupt file disables applying and leaves the instance alone.
	p.chooseFile("missing.dmi")
	render()
	if p.selected || p.useSelected() {
		t.Fatal("missing file was selectable")
	}
	if err = os.WriteFile(filepath.Join(root, "broken.dmi"), []byte("not a PNG"), 0600); err != nil {
		t.Fatal(err)
	}
	p.chooseFile("broken.dmi")
	render()
	if p.selected || p.loadError == "" {
		t.Fatal("corrupt file not reported")
	}
	// Refresh must preserve unsaved work published by an open sprite editor.
	if err = dmicon.Preview(filepath.Join(root, "fresh.dmi"), newDoc.Icon); err != nil {
		t.Fatal(err)
	}
	live, _ := dmicon.Cache.Get("fresh.dmi")
	dmicon.Cache.Invalidate("fresh.dmi")
	if got, _ := dmicon.Cache.Get("fresh.dmi"); got != live {
		t.Fatal("refresh discarded live sprite work")
	}
	dmicon.EndPreview(filepath.Join(root, "fresh.dmi"))
	p.chooseFile("current.dmi")
	p.all = false
	viewWidth, viewHeight = 800, 600
	io.SetDisplaySize(imgui.Vec2{X: 800, Y: 600})
	for range 4 {
		render()
	}
	capture("small-window")
	if project := os.Getenv("SPRITE_PICKER_PROJECT"); project != "" {
		dialog.Close(p)
		viewWidth, viewHeight = width, height
		io.SetDisplaySize(imgui.Vec2{X: width, Y: height})
		dmicon.Cache.Free()
		dmicon.Cache.SetRootDirPath(project)
		vars = dmvars.Set(dmvars.Set(vars, "icon", "'icons/obj/chairs.dmi'"), "icon_state", `"chair"`)
		vars = dmvars.Set(vars, "name", `"Chair"`)
		p = newPicker(project, dmmprefab.New(0, "/obj/structure/chair", vars), "Replace this instance", func(*dmmprefab.Prefab) error { return nil })
		dialog.Open(p)
		defer dialog.Close(p)
		for range 4 {
			render()
		}
		capture("project-current-dmi")
		p.files, err = scanDMIs(project)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Project DMI catalog contains %d files", len(p.files))
		p.all = true
		p.fileFilter = "chair"
		for range 4 {
			render()
		}
		capture("project-all-dmis")
	}
}
