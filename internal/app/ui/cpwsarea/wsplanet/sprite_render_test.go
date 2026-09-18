package wsplanet

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/wspreview"
	"sdmm/internal/app/ui/cpwsarea/wssprite"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmi"
	"sdmm/internal/planet"
)

type planetSpriteTestApp struct {
	*testApp
	planet     *Workspace
	sprite     *wssprite.Workspace
	opened     *dmmprefab.Prefab
	openError  error
	showSprite bool
}

func (a *planetSpriteTestApp) DoEditSprite(p *dmmprefab.Prefab) {
	a.opened = p
	path := p.Vars().TextV("icon", "")
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.dme.RootDir, path)
	}
	doc, err := dmi.Load(path)
	if err != nil {
		a.openError = err
		return
	}
	scene, env := a.planet.SpriteContext()
	a.sprite = wssprite.New(a, doc, wspreview.NewSprite(scene, env))
	a.sprite.Select(p.Vars().TextV("icon_state", ""), p.Vars().IntV("dir", 2))
}

func testPlanetSpriteMenu(t *testing.T, w *Workspace, a *planetSpriteTestApp, io imgui.IO, render func(), capture func(string), name string) {
	t.Helper()
	before := planet.Clone(w.project.State)
	preview := w.preview
	pos := imgui.MousePos()
	want := w.hovered.Prefab()
	a.opened, a.openError = nil, nil
	io.SetMouseButtonDown(1, true)
	render()
	io.SetMouseButtonDown(1, false)
	render()
	capture(name + "-sprite-menu")
	io.SetMousePosition(pos.Plus(imgui.Vec2{X: 45, Y: 48}))
	render()
	io.SetMouseButtonDown(0, true)
	render()
	io.SetMouseButtonDown(0, false)
	render()
	if a.opened != want || a.sprite == nil || a.openError != nil {
		t.Fatalf("planet context action did not open the displayed sprite: got %v, want %v, error %v", a.opened, want, a.openError)
	}
	defer func() {
		a.showSprite = false
		a.sprite.Dispose()
		a.sprite = nil
		w.OnFocusChange(true)
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		render()
	}()
	doc := a.sprite.Document
	originalFile, err := os.ReadFile(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	if a.sprite.Preview == nil {
		t.Fatal("sprite editor has no planet context")
	}
	a.showSprite = true
	capture(name + "-sprite-editor")
	a.showSprite = false
	render()
	originalPixels := append([]byte(nil), w.canvas.ReadPixels()...)
	a.sprite.Delete()
	render()
	if !a.sprite.IsModified() || bytes.Equal(originalPixels, w.canvas.ReadPixels()) {
		t.Fatal("sprite edits did not update the planet canvas")
	}
	a.sprite.Undo()
	render()
	if a.sprite.IsModified() || !bytes.Equal(originalPixels, w.canvas.ReadPixels()) {
		t.Fatal("sprite Undo did not restore the planet canvas")
	}
	a.sprite.Redo()
	render()
	if bytes.Equal(originalPixels, w.canvas.ReadPixels()) {
		t.Fatal("sprite Redo did not update the planet canvas")
	}
	a.sprite.Undo()
	// Changing the atlas layout must refresh terrain units and thumbnails too.
	_, _, err = doc.Apply(func(icon *dmi.Icon) error { icon.AddState("planet_editor_test"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	a.showSprite = true
	render()
	a.showSprite = false
	render()
	if w.iconRevision != dmicon.LayoutRevision || w.thumbnail(want.Path()) == nil {
		t.Fatal("structural sprite edit left a stale planet appearance")
	}
	saved := filepath.Join(t.TempDir(), "planet-edited.dmi")
	if err = doc.Save(saved, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	reopened, err := dmi.Open(saved)
	if err != nil || len(reopened.States) != len(doc.Icon.States) {
		t.Fatal("edited DMI did not save and reopen", err)
	}
	unchanged, err := os.ReadFile(filepath.Join(a.dme.RootDir, want.Vars().TextV("icon", "")))
	if err != nil || !bytes.Equal(originalFile, unchanged) {
		t.Fatal("native test changed the project's DMI", err)
	}
	if preview != w.preview || !reflect.DeepEqual(before, w.project.State) {
		t.Fatal("sprite editing regenerated or modified the planet definition")
	}
	t.Log(fmt.Sprintf("%s: native sprite action, planet preview, Undo/Redo, atlas refresh and DMI save/reopen passed", name))
}

func testPlanetSpriteRow(t *testing.T, w *Workspace, a *planetSpriteTestApp, io imgui.IO, render func(), capture func(string)) {
	t.Helper()
	pos := imgui.Vec2{X: 1360, Y: 477}
	want := w.spritePrefab("/obj/structure/reagent_dispensers/fueltank")
	a.opened = nil
	io.SetMousePosition(pos)
	render()
	io.SetMouseButtonDown(1, true)
	render()
	io.SetMouseButtonDown(1, false)
	render()
	capture("biome-choice-sprite-menu")
	io.SetMousePosition(pos.Plus(imgui.Vec2{X: 45, Y: 48}))
	render()
	io.SetMouseButtonDown(0, true)
	render()
	io.SetMouseButtonDown(0, false)
	render()
	if a.opened == nil || a.sprite == nil || a.opened.Path() != want.Path() || a.opened.Vars().TextV("icon_state", "") != want.Vars().TextV("icon_state", "") {
		t.Fatal("biome choice context menu did not open its thumbnail's sprite")
	}
	a.sprite.Dispose()
	a.sprite = nil
	w.OnFocusChange(true)
	io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
	render()
}
