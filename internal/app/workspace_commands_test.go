package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/command"
	"sdmm/internal/app/config"
	"sdmm/internal/app/prefs"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/layout"
	"sdmm/internal/app/ui/menu"
	"sdmm/internal/app/ui/shortcut"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmclip"
	"sdmm/internal/platform"
	"sdmm/internal/ship"
	"sdmm/internal/shippreview"
	"sdmm/internal/util"
)

// Keep the production command handlers and layout, with OS integration and
// background preview requests replaced by counters in this disposable fixture.
type commandTestApp struct {
	app
	previewRequests int
}

func (a *commandTestApp) OnWorkspaceSwitched() {
	if a.layout != nil {
		a.SyncVarEditor()
	}
}
func (*commandTestApp) AddMouseChangeCallback(func(uint, uint)) int { return 0 }
func (*commandTestApp) RemoveMouseChangeCallback(int)               {}
func (a *commandTestApp) ShipFilesSaved()                           { a.previewRequests++ }
func (*commandTestApp) ShipPreviewStatus() shippreview.Status       { return shippreview.Status{} }

func commandShipFixture(t *testing.T) *dmenv.Dme {
	t.Helper()
	root := t.TempDir()
	d, err := dmenv.New(os.Getenv("SHIP_RENDER_TEST_DME"))
	if err != nil {
		t.Fatal(err)
	}
	include := filepath.ToSlash(d.RootFile)
	profilePath := "voidcrew/modules/ship_upgrades/ship_upgrades.toml"
	profile, err := os.ReadFile(filepath.Join(d.RootDir, profilePath))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(root, filepath.Dir(profilePath)), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, profilePath), profile, 0600); err != nil {
		t.Fatal(err)
	}
	iconRoot := d.RootDir
	d.RootDir = root
	d.RootFile = filepath.Join(root, "test.dme")
	if err := os.WriteFile(d.RootFile, []byte("#include \""+include+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dmmap.PrefabStorage.Free()
	dmicon.Cache.Free()
	dmicon.Cache.SetRootDirPath(iconRoot)
	dmmap.Init(d)
	c := &ship.Catalog{Root: root, ModuleDir: "_maps/voidcrew/ship_modules/"}
	p, err := ship.NewProject(c, d, "command_fixture", "Command Fixture", 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNativeWorkspaceCommands(t *testing.T) {
	output := os.Getenv("VOIDWORKS_COMMAND_TEST_OUTPUT")
	if output == "" {
		t.Skip("set VOIDWORKS_COMMAND_TEST_OUTPUT for the native command routing test")
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
	w, err := glfw.CreateWindow(width, height, "Command routing test", nil, nil)
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
	io.SetConfigFlags(imgui.ConfigFlagsDockingEnable)
	io.SetDisplaySize(imgui.Vec2{X: width, Y: height})
	io.SetDisplayFrameBufferScale(imgui.Vec2{X: 1, Y: 1})
	io.SetDeltaTime(1.0 / 60)
	platform.InitImGuiGL()
	defer platform.DisposeImGuiGL()
	window.SetPointSize(1)
	dme := commandShipFixture(t)
	a := &commandTestApp{app: app{configDir: t.TempDir(), internalDir: t.TempDir(), loadedEnvironment: dme, commandStorage: command.NewStorage(), pathsFilter: dm.NewPathsFilterEmpty(), clipboard: dmmclip.New(), configs: map[string]config.Config{}, tmpWindowCond: imgui.ConditionAlways}}
	a.ConfigRegister(&preferencesConfig{Prefs: prefs.Prefs{Interface: prefs.Interface{Scale: 100}, Editor: prefs.Editor{NudgeMode: prefs.SaveNudgeModePixelAlt}}})
	a.ConfigRegister(&projectConfig{})
	a.layout = layout.New(a)
	a.menu = menu.New(a)
	ws := a.layout.WsArea.OpenShip()
	defer ws.Dispose()
	frame := func() {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.Viewport(0, 0, width, height)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		imgui.NewFrame()
		shortcut.Process()
		a.menu.Process()
		a.layout.Process()
		imgui.Render()
		platform.Render(imgui.RenderedDrawData())
		gl.Finish()
		a.tmpWindowCond = imgui.ConditionFirstUseEver
	}
	capture := func(name string) {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		pixels := make([]byte, width*height*4)
		gl.ReadPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
		im := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			copy(im.Pix[y*im.Stride:(y+1)*im.Stride], pixels[(height-1-y)*width*4:(height-y)*width*4])
		}
		f, err := os.Create(filepath.Join(output, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, im); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ {
		frame()
	}
	capture("library")
	click := func(p imgui.Vec2) {
		io.SetMousePosition(p)
		frame()
		io.SetMouseButtonDown(0, true)
		frame()
		io.SetMouseButtonDown(0, false)
		frame()
		frame()
	}
	click(imgui.Vec2{X: 700, Y: 360})
	io.AddInputCharacters("Command Fixture")
	frame()
	capture("filtered")
	click(imgui.Vec2{X: 650, Y: 490})
	capture("opened")
	if ws.Map() == nil {
		t.Fatal("fixture ship did not open")
	}
	if a.layout.WsArea.ActiveWorkspace().Content() != ws {
		t.Fatal("ship is visible but commands target another workspace")
	}
	m := ws.Map().Dmm()
	before := ship.RawData(m).EncodeTGM()
	coord := util.Point{X: 3, Y: 3, Z: 1}
	m.GetTile(coord).InstancesAdd(dmmap.PrefabStorage.Initial("/obj/item/crowbar"))
	ws.Map().Editor().CommitContextNow("Place test object")
	frame()
	if !a.commandStorage.HasUndo() {
		t.Fatal("Edit > Undo disabled after map edit")
	}
	a.DoUndo()
	frame()
	if !bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("application Undo did not restore map")
	}
	a.DoRedo()
	frame()
	if bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("application Redo did not restore edit")
	}
	a.DoSave()
	frame()
	if a.previewRequests != 1 || ws.IsModified() {
		t.Fatal("application Save did not save ship")
	}
	press := func(key glfw.Key) {
		io.KeyPress(int(glfw.KeyLeftControl))
		io.KeyPress(int(key))
		frame()
		io.KeyRelease(int(key))
		io.KeyRelease(int(glfw.KeyLeftControl))
		frame()
	}
	press(glfw.KeyS)
	if a.previewRequests != 2 {
		t.Fatal("Ctrl+S did not reach the ship")
	}
	press(glfw.KeyZ)
	if !bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Ctrl+Z did not undo the ship edit")
	}
	press(glfw.KeyY)
	if bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Ctrl+Y did not redo the ship edit")
	}
	click(imgui.Vec2{X: 20, Y: 10})
	capture("file-menu")
	if !a.HasSaveableWorkspace() || a.layout.WsArea.ActiveWorkspace().Content() != ws {
		t.Fatal("File menu lost ship command routing")
	}
	click(imgui.Vec2{X: 75, Y: 190})
	capture("after-file-save")
	if a.previewRequests != 3 {
		t.Fatal("File > Save did not reach the ship")
	}
	ordinary := m.Copy()
	ordinary.Name = "ordinary.dmm"
	ordinary.Path = dmmap.DmmPath{Absolute: filepath.Join(t.TempDir(), ordinary.Name), Readable: ordinary.Name}
	a.layout.WsArea.OpenMap(&ordinary, nil)
	for i := 0; i < 4; i++ {
		frame()
	}
	defer a.layout.WsArea.ActiveWorkspace().Dispose()
	a.layout.WsArea.OpenShip()
	for i := 0; i < 4; i++ {
		frame()
	}
	capture("returned-to-ship")
	if a.layout.WsArea.ActiveWorkspace().Content() != ws {
		t.Fatal("returning from ordinary map left commands targeting the hidden map")
	}
	if !a.commandStorage.HasUndo() {
		t.Fatal("returning from ordinary map disabled ship undo")
	}
	ordinaryBefore := ship.RawData(&ordinary).EncodeTGM()
	press(glfw.KeyZ)
	if !bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Ctrl+Z after tab switch did not undo ship edit")
	}
	press(glfw.KeyY)
	press(glfw.KeyS)
	if a.previewRequests != 4 || ws.IsModified() {
		t.Fatal("Ctrl+S after tab switch did not save ship")
	}
	click(imgui.Vec2{X: 20, Y: 10})
	click(imgui.Vec2{X: 75, Y: 190})
	if a.previewRequests != 5 {
		t.Fatal("File > Save after tab switch did not save ship")
	}
	click(imgui.Vec2{X: 130, Y: 10})
	capture("edit-menu")
	click(imgui.Vec2{X: 175, Y: 32})
	if !bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Edit > Undo after tab switch did not undo ship edit")
	}
	click(imgui.Vec2{X: 130, Y: 10})
	click(imgui.Vec2{X: 175, Y: 52})
	if bytes.Equal(before, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Edit > Redo after tab switch did not restore ship edit")
	}
	a.layout.WsArea.OpenShip()
	frame()
	frame()
	if !tools.SetGrabSelection(coord, coord) {
		t.Fatal("could not select ship tile")
	}
	press(glfw.KeyC)
	if !a.clipboard.HasData() || len(a.clipboard.Buffer().Buffer) != 1 {
		t.Fatal("Ctrl+C did not copy selected ship tile")
	}
	beforePaste := ship.RawData(m).EncodeTGM()
	ws.Map().CanvasState().SetMousePosition(4*dmmap.WorldIconSize, 4*dmmap.WorldIconSize, 1)
	press(glfw.KeyV)
	// The production window drains deferred context commits at frame start.
	// This hidden render harness commits explicitly instead.
	ws.Map().Editor().CommitContextNow("Paste Tile")
	if bytes.Equal(beforePaste, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Ctrl+V did not paste into ship")
	}
	press(glfw.KeyZ)
	if !bytes.Equal(beforePaste, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Ctrl+Z did not undo pasted ship tiles")
	}
	if !tools.SetGrabSelection(coord, coord) {
		t.Fatal("could not select ship tile for menu Copy")
	}
	a.clipboard.Free()
	click(imgui.Vec2{X: 130, Y: 10})
	click(imgui.Vec2{X: 175, Y: 75})
	if !a.clipboard.HasData() || len(a.clipboard.Buffer().Buffer) != 1 || a.clipboard.Buffer().Buffer[0].Coord != coord {
		t.Fatal("Edit > Copy did not copy selected ship tile")
	}
	ws.Map().CanvasState().SetMousePosition(5*dmmap.WorldIconSize, 5*dmmap.WorldIconSize, 1)
	click(imgui.Vec2{X: 130, Y: 10})
	click(imgui.Vec2{X: 175, Y: 94})
	ws.Map().Editor().CommitContextNow("Paste Tile")
	if bytes.Equal(beforePaste, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Edit > Paste did not paste into ship")
	}
	click(imgui.Vec2{X: 130, Y: 10})
	click(imgui.Vec2{X: 175, Y: 32})
	if !bytes.Equal(beforePaste, ship.RawData(m).EncodeTGM()) {
		t.Fatal("Edit > Undo did not undo pasted ship tiles")
	}
	click(imgui.Vec2{X: 130, Y: 10})
	click(imgui.Vec2{X: 175, Y: 52})
	press(glfw.KeyS)
	saved, err := dmmdata.New(m.Path.Absolute)
	if err != nil {
		t.Fatal(err)
	}
	if saved.MaxX != m.MaxX || saved.MaxY != m.MaxY || saved.MaxZ != m.MaxZ {
		t.Fatal("saved ship dimensions changed")
	}
	for _, tile := range m.Tiles {
		if !tile.Instances().Prefabs().Sorted().Equals(saved.Dictionary[saved.Grid[tile.Coord]].Sorted()) {
			t.Fatalf("reopened ship tile %v does not contain the saved edits", tile.Coord)
		}
	}
	if !bytes.Equal(ordinaryBefore, ship.RawData(&ordinary).EncodeTGM()) {
		t.Fatal("ship commands changed the hidden ordinary map")
	}
	// Selections survive menus, but must not address tiles in another document.
	if !tools.SetGrabSelection(coord, coord) {
		t.Fatal("could not select ship tile before changing documents")
	}
	a.layout.WsArea.OpenMap(&ordinary, nil)
	for i := 0; i < 4; i++ {
		frame()
	}
	if a.commandStorage.HasUndo() {
		t.Fatal("ordinary map inherited the ship's undo history")
	}
	if _, _, selected := tools.SelectionBounds(); selected {
		t.Fatal("ship selection leaked into ordinary map")
	}
	a.layout.WsArea.OpenShip()
	for i := 0; i < 4; i++ {
		frame()
	}
	if !a.commandStorage.HasUndo() {
		t.Fatal("ship history lost after second tab switch")
	}
	capture("verified-commands")
}
