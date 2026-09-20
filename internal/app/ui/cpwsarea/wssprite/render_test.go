package wssprite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/wspreview"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmi"
	"sdmm/internal/platform"
	"sdmm/internal/recovery"
)

type spriteTestApp struct{ commands *command.Storage }

func (a *spriteTestApp) CommandStorage() *command.Storage { return a.commands }

// Exercises real ImGui input and GPU pixels in an isolated hidden window.
func TestNativeSpriteEditing(t *testing.T) {
	if os.Getenv("DMI_RENDER_TEST") != "1" {
		t.Skip("set DMI_RENDER_TEST=1 for native drawing tests")
	}
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", t.TempDir())
	} else {
		t.Setenv("HOME", t.TempDir())
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
	win, err := glfw.CreateWindow(width, height, "DMI editor validation", nil, nil)
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
	app := &spriteTestApp{command.NewStorage()}
	doc, _ := dmi.Create(32, 32)
	doc.Icon.States[0].Name = "idle"
	doc.Icon.States[0].Set("hotspot", "3,4,1")
	if err = doc.Icon.SetDirections(0, 4); err != nil {
		t.Fatal(err)
	}
	if err = doc.Icon.InsertFrame(0, 0, true); err != nil {
		t.Fatal(err)
	}
	doc.Icon.States[0].SetDelays([]float64{1, 2.5})
	for n, cel := range doc.Icon.States[0].Cels {
		for y := 8; y < 24; y++ {
			for x := 8; x < 24; x++ {
				cel.SetNRGBA(x, y, color.NRGBA{R: uint8(80 + n*20), G: 160, B: 80, A: 255})
			}
		}
	}
	path := filepath.Join(t.TempDir(), "native-test.dmi")
	if err = doc.Save(path, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(filepath.Dir(path), "fixture.dme")
	if err = os.WriteFile(projectPath, []byte("world\n    turf = /turf/test\n/turf/test\n    icon = 'native-test.dmi'\n    icon_state = \"idle\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dme, err := dmenv.New(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	dmmap.PrefabStorage.Free()
	dmicon.Cache.Free()
	dmmap.Init(dme)
	defer dmmap.Free()
	dmicon.Cache.SetRootDirPath(dme.RootDir)
	scene := &dmmap.Dmm{Name: "Live sprite validation"}
	scene.SetMapSize(3, 3, 1)
	// The context is already loaded; sprite editing must work even when its
	// source cannot be reparsed, and must never invoke a second game parse.
	dme.RootFile = filepath.Join(t.TempDir(), "unavailable.dme")
	ws := New(app, doc, wspreview.NewSprite(scene, dme))
	if ws.Preview == nil {
		t.Fatal("sprite context missing")
	}
	defer ws.Dispose()
	viewWidth, viewHeight := float32(width), float32(height)
	render := func() {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.Viewport(0, 0, width, height)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: viewWidth, Y: viewHeight})
		imgui.BeginV("DMI Sprite Editor", nil, imgui.WindowFlagsNoResize|imgui.WindowFlagsNoMove|imgui.WindowFlagsNoCollapse)
		ws.PreProcess()
		ws.Process()
		imgui.End()
		dialog.Process()
		imgui.Render()
		platform.Render(imgui.RenderedDrawData())
		gl.Finish()
	}
	for range 3 {
		render()
	}
	if ws.message != "" {
		t.Fatal(ws.message)
	}
	texture := ws.rendered.Texture
	revision := dmicon.LayoutRevision
	original := doc.Icon
	ws.color = [4]float32{1, 0, 0, 1}
	mouse := func(x, y int, down bool) {
		io.SetMousePosition(ws.canvasOrigin.Plus(imgui.Vec2{X: (float32(x) + .5) * ws.zoom, Y: (float32(y) + .5) * ws.zoom}))
		io.SetMouseButtonDown(0, down)
		render()
	}
	mouse(2, 2, false)
	mouse(2, 2, true)
	mouse(8, 2, true)
	mouse(8, 2, false)
	for x := 2; x <= 8; x++ {
		if c := doc.Icon.States[0].Cels[0].NRGBAAt(x, 2); c != (color.NRGBA{R: 255, A: 255}) {
			t.Fatalf("drag skipped pixel %d: %v", x, c)
		}
	}
	if !ws.IsModified() || ws.stroke != nil {
		t.Fatal("released stroke was not committed")
	}
	if ws.rendered.Texture != texture || dmicon.LayoutRevision != revision {
		t.Fatal("pixel stroke unnecessarily recreated scene/atlas")
	}
	gl.BindTexture(gl.TEXTURE_2D, texture)
	pixels := make([]byte, ws.rendered.TextureWidth*ws.rendered.TextureHeight*4)
	gl.GetTexImage(gl.TEXTURE_2D, 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
	idx := (2*ws.rendered.TextureWidth + 4) * 4
	if !bytes.Equal(pixels[idx:idx+4], []byte{255, 0, 0, 255}) {
		t.Fatal("live atlas did not update")
	}
	// The red stroke must appear in the map surface, not just the pixel canvas.
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	screen := make([]byte, width*height*4)
	gl.ReadPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(screen))
	red := 0
	for y := 210; y < 700; y++ {
		for x := 1000; x < 1380; x++ {
			n := ((height-1-y)*width + x) * 4
			if screen[n] > 200 && screen[n+1] < 30 && screen[n+2] < 30 {
				red++
			}
		}
	}
	if red < 20 {
		t.Fatalf("map preview did not reflect live drawing (%d red pixels)", red)
	}
	if target := os.Getenv("DMI_RENDER_OUTPUT"); target != "" {
		if err := os.MkdirAll(target, 0700); err != nil {
			t.Fatal(err)
		}
		captureSpriteFrame(t, filepath.Join(target, "live-map.png"), width, height)
	}
	app.commands.UndoV(ws.CommandStackId())
	if doc.Icon != original || ws.IsModified() {
		t.Fatal("one Undo did not restore entire stroke")
	}
	app.commands.RedoV(ws.CommandStackId())
	if doc.Icon.States[0].Cels[0].NRGBAAt(4, 2).R != 255 {
		t.Fatal("Redo did not restore pixels")
	}
	ws.before = true
	ws.publish()
	if ws.published != original {
		t.Fatal("Before did not use saved image")
	}
	ws.before = false
	ws.publish()
	ws.change("Duplicate state", func(i *dmi.Icon) error { return i.DuplicateStates([]int{0}) })
	if dmicon.LayoutRevision == revision {
		t.Fatal("structural edit did not invalidate scene")
	}
	ws.change("Add layer", func(i *dmi.Icon) error {
		ws.layer = i.States[0].AddLayer(i.Width, i.Height)
		return nil
	})
	ws.color = [4]float32{0, 0, 1, 1}
	render()
	mouse(10, 10, false)
	mouse(10, 10, true)
	mouse(10, 10, false)
	blue := color.NRGBA{B: 255, A: 255}
	if doc.Icon.States[0].Cels[0].NRGBAAt(10, 10) != blue || doc.Icon.States[0].Layers[0].Cels[0].NRGBAAt(10, 10) == blue {
		t.Fatal("painting a layer changed the underlying image")
	}
	ws.tool = 8 // select the blue pixel with the wand
	mouse(10, 10, true)
	mouse(10, 10, false)
	ws.tool = 10
	mouse(10, 10, true)
	mouse(12, 11, true)
	mouse(12, 11, false)
	if doc.Icon.States[0].Cels[0].NRGBAAt(12, 11) != blue || doc.Icon.States[0].Layers[1].Cels[0].NRGBAAt(10, 10).A != 0 {
		t.Fatal("moving a wand selection did not preserve layer pixels")
	}
	ws.Undo()
	if doc.Icon.States[0].Cels[0].NRGBAAt(10, 10) != blue || ws.selection != image.Rect(10, 10, 11, 11) {
		t.Fatal("undo did not restore the moved selection")
	}
	ws.Redo()
	if ws.selection != image.Rect(12, 11, 13, 12) {
		t.Fatal("redo did not restore the selection position")
	}
	ws.selection, ws.selectionMask = image.Rect(10, 10, 14, 13), nil
	ws.transform("Flip selection", dmi.FlipHorizontal)
	if doc.Icon.States[0].Layers[1].Cels[0].NRGBAAt(11, 11) != blue || doc.Icon.States[0].Layers[1].Cels[0].NRGBAAt(12, 11).A != 0 {
		t.Fatal("transform ignored the selected region")
	}
	ws.Undo()
	if doc.Icon.States[0].Layers[1].Cels[0].NRGBAAt(12, 11) != blue || ws.selectionMask != nil || ws.selection != image.Rect(10, 10, 14, 13) {
		t.Fatal("undo did not restore transformed selection")
	}
	ws.Deselect()
	ws.tool = 0
	// Timing changes must update the map's existing sprites without rebuilding
	// the atlas or resetting the chosen direction.
	live, err := dmicon.Cache.Get(path)
	if err != nil {
		t.Fatal(err)
	}
	south, north := live.States["idle"].SpriteV(2), live.States["idle"].SpriteV(1)
	dmicon.PlayPreview(path, true, time.Now().Add(-150*time.Millisecond))
	dmicon.AdvanceLiveAnimations()
	if south.Current() != live.States["idle"].SpriteByFrame(2, 1) || north.Current() != live.States["idle"].SpriteByFrame(1, 1) {
		t.Fatal("live animation lost its frame or direction")
	}
	dmicon.SetPreviewPlayback(path, false, time.Now().Add(-time.Hour), .15)
	dmicon.AdvanceLiveAnimations()
	if south.Current() != live.States["idle"].SpriteByFrame(2, 1) {
		t.Fatal("paused live preview reset to the first frame")
	}
	dmicon.PlayPreview(path, false, time.Now())
	dmicon.AdvanceLiveAnimations()
	ws.selectCel(4, false)
	ws.frameMilliseconds = "350"
	ws.applyFrameDuration(false)
	if got := doc.Icon.States[0].Delays(); got[0] != 1 || got[1] != 3.5 {
		t.Fatal("frame duration did not edit only the selected frame", got)
	}
	ws.Undo()
	ws.delays = "2"
	ws.applyTimingList()
	if got := doc.Icon.States[0].Delays(); got[0] != 2 || got[1] != 2 {
		t.Fatal("single timing value was not applied to every frame", got)
	}
	ws.Undo()
	beforeInvalid := doc.Icon
	ws.frameMilliseconds = "0"
	ws.applyFrameDuration(true)
	if doc.Icon != beforeInvalid {
		t.Fatal("invalid duration changed the animation")
	}
	ws.syncFields()
	ws.message = ""
	if !ws.saveTo(path) {
		t.Fatal(ws.message)
	}
	reopened, err := dmi.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.Icon.States) != 2 || reopened.Icon.States[0].Value("hotspot", "") != "3,4,1" || reopened.Icon.States[0].Delays()[1] != 2.5 {
		t.Fatal("save/reopen lost states or animation metadata")
	}
	if len(reopened.Icon.States[0].Layers) != 2 || reopened.Icon.States[0].Layers[1].Cels[0].NRGBAAt(12, 11) != blue {
		t.Fatal("save/reopen lost editable layers")
	}
	if !bytes.Equal(reopened.Icon.States[0].Cels[0].Pix, doc.Icon.States[0].Cels[0].Pix) {
		t.Fatal("save/reopen changed pixels")
	}
	ws.change("Crash recovery fixture", func(i *dmi.Icon) error { i.States[0].Name = "recovered"; return nil })
	ws.lastRecovery = time.Time{}
	if !ws.autosave() {
		t.Fatal("recovery snapshot failed", ws.message)
	}
	folder := filepath.Dir(filepath.Dir(ws.store.Folder()))
	ws.store.Close()
	entries, err := recovery.Pending(folder, path)
	if err != nil || len(entries) != 1 {
		t.Fatalf("recovery absent: %v (%d)", err, len(entries))
	}
	for _, entry := range entries {
		entry.Store.Close()
	}
	all, err := recovery.PendingAll(folder)
	if err != nil || len(all) != 1 {
		t.Fatal("global recovery did not discover the sprite draft", err)
	}
	recovered, err := decodeDraft(all[0].Snapshot.Data)
	if err != nil || recovered.Icon.States[0].Name != "recovered" {
		t.Fatal("draft could not be restored", err)
	}
	// Restore the lock so normal disposal clears this test's recovery data.
	ws.store = nil
	render()
	if target := os.Getenv("DMI_RENDER_OUTPUT"); target != "" {
		if err := os.MkdirAll(target, 0700); err != nil {
			t.Fatal(err)
		}
		captureSpriteFrame(t, filepath.Join(target, "editor.png"), width, height)
	}
	var recoveredWorkspace *Workspace
	recoveryUI := &recoveryDialog{entries: all, open: func(doc *dmi.Document) *Workspace {
		recoveredWorkspace = New(app, doc, nil)
		return recoveredWorkspace
	}}
	dialog.Open(recoveryUI)
	for range 3 {
		render()
	}
	if target := os.Getenv("DMI_RENDER_OUTPUT"); target != "" {
		captureSpriteFrame(t, filepath.Join(target, "recovery.png"), width, height)
	}
	dialog.Close(recoveryUI)
	diskBefore, _ := os.ReadFile(path)
	if err := recoveryUI.recover(all[0]); err != nil {
		t.Fatal(err)
	}
	defer recoveredWorkspace.Dispose()
	if recoveredWorkspace.rendered == nil {
		t.Fatal("untitled recovered document has no drawable atlas while a game is loaded")
	}
	if recoveredWorkspace.Document.Path != "" || !recoveredWorkspace.IsModified() || recoveredWorkspace.Document.Icon.States[0].Name != "recovered" {
		t.Fatal("recovery did not open an independent unsaved DMI")
	}
	diskAfter, _ := os.ReadFile(path)
	if !bytes.Equal(diskBefore, diskAfter) {
		t.Fatal("recovery overwrote the original DMI")
	}
	for _, entry := range all {
		entry.Store.Close()
	}
	// Exercise the same standalone view opened by File > Open, at 4K where
	// the canvas can exceed a 16-bit draw list even for a small DMI.
	if file := os.Getenv("DMI_OPEN_TEST_FILE"); file != "" {
		openedDoc, err := dmi.Load(file)
		if err != nil {
			t.Fatal(err)
		}
		opened := New(app, openedDoc, nil)
		defer opened.Dispose()
		ws = opened
		viewWidth, viewHeight = 3840, 2160
		io.SetDisplaySize(imgui.Vec2{X: viewWidth, Y: viewHeight})
		for range 3 {
			render()
		}
		if ws.rendered == nil {
			t.Fatal("standalone DMI has no drawable atlas")
		}
	}
	if control := os.Getenv("DMI_SAVE_DIALOG_TEST"); control != "" {
		fresh, _ := dmi.Create(32, 32)
		fresh.Icon.States[0].Cels[0].SetNRGBA(3, 4, blue)
		ws = New(app, fresh, nil)
		defer ws.Dispose()
		folder := t.TempDir()
		for step, action := range []string{"save", "saveas", "cancel"} {
			target := filepath.Join(folder, fmt.Sprintf("saved-%d.dmi", step))
			request, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "step": step, "action": action, "target": target})
			if err := os.WriteFile(control, request, 0600); err != nil {
				t.Fatal(err)
			}
			var ok bool
			if action == "save" {
				ok = ws.Save()
			} else {
				ok = ws.saveAs()
			}
			if action == "cancel" {
				if ok || fresh.Path == target {
					t.Fatal("cancel changed the document destination")
				}
				continue
			}
			if !ok {
				t.Fatal("native Save dialog failed", ws.message)
			}
			saved, err := dmi.Load(target)
			if err != nil || saved.Icon.States[0].Cels[0].NRGBAAt(3, 4) != blue || fresh.Modified() {
				t.Fatal("new DMI save/reopen lost pixels or retained dirty state", err)
			}
		}
	}
}

func captureSpriteFrame(t *testing.T, path string, width, height int) {
	t.Helper()
	pixels := make([]byte, width*height*4)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		copy(frame.Pix[y*frame.Stride:(y+1)*frame.Stride], pixels[(height-1-y)*width*4:(height-y)*width*4])
	}
	for n := 3; n < len(frame.Pix); n += 4 {
		frame.Pix[n] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, frame); err != nil {
		t.Fatal(err)
	}
}
