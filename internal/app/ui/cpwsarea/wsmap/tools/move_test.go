package tools

import (
	"fmt"
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/prefs"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

type moveEditor struct {
	editor
	dmm                *dmmap.Dmm
	dme                *dmenv.Dme
	instance, selected *dmminstance.Instance
	preferences        prefs.Prefs
	snapshot           *dmmsnap.DmmSnap
	refreshes, commits int
	previews           int
	previewX, previewY int
	view               *dmmap.Dmm
}

func (e *moveEditor) Dmm() *dmmap.Dmm                        { return e.dmm }
func (e *moveEditor) HoveredInstance() *dmminstance.Instance { return e.instance }
func (e *moveEditor) InstanceSelect(i *dmminstance.Instance) { e.selected = i }
func (e *moveEditor) ZoomLevel() float32                     { return 2 }
func (e *moveEditor) Prefs() prefs.Prefs                     { return e.preferences }
func (e *moveEditor) PreviewPixelOffset(_ *dmminstance.Instance, x, y int) {
	e.previews++
	e.previewX, e.previewY = x, y
}
func (e *moveEditor) ClearPixelOffsetPreview() { e.previewX, e.previewY = 0, 0 }
func (e *moveEditor) CommitChanges(string) {
	_, changed := e.snapshot.Commit()
	if len(changed) != 0 {
		e.commits++
	}
}
func (e *moveEditor) UpdateCanvasByCoords([]util.Point) {
	e.refreshes++
	if e.dme == nil {
		return
	}
	// Exercise the same render-only assembly conversion as Ship Workshop.
	a := &ship.Assembly{MaxX: 1, MaxY: 1, MaxZ: 1, Cells: map[util.Point][]ship.Atom{}}
	i := e.instance
	a.Cells[i.Coord()] = []ship.Atom{{Prefab: i.Prefab(), Instance: i}}
	e.view, _ = a.Display(e.dme)
}

func moveFixture(t *testing.T, mode string, workshop bool) (*ToolMove, *moveEditor, *dmmprefab.Prefab) {
	t.Helper()
	context := imgui.CreateContext(nil)
	t.Cleanup(context.Destroy)
	previous := ed
	t.Cleanup(func() { ed = previous })
	path := "/obj/move_test/" + t.Name()
	vars := &dmvars.MutableVariables{}
	vars.Put("name", `"sticker pack"`)
	p := dmmprefab.New(0, path, vars.ToImmutable())
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)
	dmmap.PrefabStorage.Put(p)
	coord := at(1, 1)
	tile := &dmmap.Tile{Coord: coord}
	tile.InstancesAdd(p)
	e := &moveEditor{dmm: &dmmap.Dmm{MaxX: 1, MaxY: 1, MaxZ: 1, Tiles: []*dmmap.Tile{tile}}, instance: tile.Instances()[0]}
	e.preferences.Editor.NudgeMode = mode
	e.snapshot = dmmsnap.New(e.dmm)
	if workshop {
		e.dme = &dmenv.Dme{Objects: map[string]*dmenv.Object{path: {Vars: vars.ToImmutable()}}}
	}
	ed = e
	io := imgui.CurrentIO()
	io.SetMousePosition(imgui.Vec2{X: 100, Y: 100})
	io.KeyPress(int(glfw.KeyLeftShift))
	m := newMove()
	m.onStart(coord)
	return m, e, p
}

func TestMoveDragOnlyPersistsReleasedOffset(t *testing.T) {
	for mode, axes := range [][2]string{{"pixel_x", "pixel_y"}, {"step_x", "step_y"}, {"pixel_w", "pixel_z"}} {
		for _, workshop := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode%d/workshop%t", mode, workshop), func(t *testing.T) {
				m, e, original := moveFixture(t, []string{prefs.SaveNudgeModePixel, prefs.SaveNudgeModeStep, prefs.SaveNudgeModePixelAlt}[mode], workshop)
				io := imgui.CurrentIO()
				for n := 1; n <= 40; n++ {
					io.SetMousePosition(imgui.Vec2{X: 100 - float32(n)*2, Y: 100 - float32(n)*4})
					m.process()
					if e.instance.Prefab() != original || e.refreshes != 0 {
						t.Fatal("pixel drag changed map data or rebuilt the canvas before release")
					}
					if got := len(dmmap.PrefabStorage.GetAllByPath(original.Path())); got != 1 {
						t.Fatalf("drag step %d persisted %d prefabs before release, want original only", n, got)
					}
					if e.previewX != -n || e.previewY != 2*n {
						t.Fatal("live preview lost current offset")
					}
				}
				if e.commits != 0 {
					t.Fatal("drag committed before release")
				}
				m.onStop(at(1, 1))
				final := e.instance.Prefab()
				if final.Vars().IntV(axes[0], 0) != -40 || final.Vars().IntV(axes[1], 0) != 80 {
					t.Fatalf("incorrect zoomed offset: %v", final.Vars())
				}
				if e.previewX != 0 || e.previewY != 0 {
					t.Fatal("release left a temporary preview offset active")
				}
				if e.commits != 1 || len(dmmap.PrefabStorage.GetAllByPath(original.Path())) != 2 {
					t.Fatal("release must save one final prefab and one history entry")
				}
				if e.selected != e.instance {
					t.Fatal("final instance not selected")
				}
				e.snapshot.GoTo(0)
				if e.dmm.Tiles[0].Instances()[0].Prefab().Id() != original.Id() {
					t.Fatal("undo lost original prefab")
				}
				e.snapshot.GoTo(1)
				if e.dmm.Tiles[0].Instances()[0].Prefab().Id() != final.Id() {
					t.Fatal("redo lost released offset")
				}
			})
		}
	}
}

func TestMoveStationaryAndReturnedDragDoNotCreateChanges(t *testing.T) {
	m, e, original := moveFixture(t, prefs.SaveNudgeModePixelAlt, false)
	for n := 0; n < 60; n++ {
		m.process()
	}
	if e.refreshes != 0 || e.previews != 0 {
		t.Fatalf("stationary drag refreshed canvas %d times", e.refreshes)
	}
	io := imgui.CurrentIO()
	io.SetMousePosition(imgui.Vec2{X: 102, Y: 96})
	m.process()
	for n := 0; n < 60; n++ {
		m.process()
	}
	if e.refreshes != 0 || e.previews != 1 {
		t.Fatalf("unchanged offset rebuilt canvas %d times, preview updates %d", e.refreshes, e.previews)
	}
	io.SetMousePosition(imgui.Vec2{X: 100, Y: 100})
	m.process()
	m.onStop(at(1, 1))
	if e.instance.Prefab().Id() != original.Id() || e.commits != 0 || len(dmmap.PrefabStorage.GetAllByPath(original.Path())) != 1 {
		t.Fatal("returning to starting position created overrides or history")
	}
}

func TestMovePreservesExistingAndInheritedOffsets(t *testing.T) {
	m, e, base := moveFixture(t, prefs.SaveNudgeModePixelAlt, true)
	parent := dmvars.Set(base.Vars(), "pixel_w", "9")
	vars := dmvars.Set(dmvars.FromParent(parent), "pixel_z", "-3")
	vars = dmvars.Set(vars, "dir", "8")
	original := dmmap.PrefabStorage.Put(dmmprefab.New(0, base.Path(), vars))
	e.instance.SetPrefab(original)
	e.snapshot.Sync()
	m.onStart(at(1, 1))
	io := imgui.CurrentIO()
	io.SetMousePosition(imgui.Vec2{X: 90, Y: 100})
	m.process()
	if e.instance.Prefab() != original || e.previewX != -5 || e.previewY != 0 {
		t.Fatal("drag changed source overrides before release or lost inherited offset preview")
	}
	m.onStop(at(1, 1))
	changed := e.instance.Prefab().Vars()
	if changed.IntV("pixel_w", 0) != 4 || changed.IntV("pixel_z", 0) != -3 || changed.IntV("dir", 0) != 8 {
		t.Fatal("drag lost existing overrides or inherited offsets")
	}
	// Returning a second drag to its starting position must be a no-op too.
	e.commits = 0
	m.onStart(at(1, 1))
	start := e.instance.Prefab()
	io.SetMousePosition(imgui.Vec2{X: 100, Y: 100})
	m.process()
	io.SetMousePosition(imgui.Vec2{X: 90, Y: 100})
	m.process()
	m.onStop(at(1, 1))
	if e.instance.Prefab() != start || e.commits != 0 {
		t.Fatal("returning to inherited offset created an unnecessary override")
	}
}

func TestPixelMoveRegistersOncePerMouseStroke(t *testing.T) {
	for _, interruption := range []string{"release", "refocus", "save", "disable"} {
		t.Run(interruption, func(t *testing.T) {
			m, e, original := moveFixture(t, prefs.SaveNudgeModePixelAlt, false)
			previousCC, previousCS := cc, cs
			previousActive, previousEnabled, previousHandled := active, enabled, pressHandled
			previousTools, previousSelected, previousStarted := tools, selectedToolName, startedTool
			defer func() {
				cc, cs = previousCC, previousCS
				active, enabled, pressHandled = previousActive, previousEnabled, previousHandled
				tools, selectedToolName, startedTool = previousTools, previousSelected, previousStarted
			}()
			control := &fillControl{dragging: true}
			cc, cs = control, &fillState{coord: at(1, 1)}
			tools, selectedToolName, startedTool = map[string]Tool{TNMove: m}, TNMove, m
			active, enabled, pressHandled = true, true, true
			imgui.CurrentIO().SetMousePosition(imgui.Vec2{X: 112, Y: 96})
			for n := 0; n < 5; n++ {
				process(false)
			}
			if e.commits != 0 || e.instance.Prefab() != original || e.refreshes != 0 {
				t.Fatal("held mouse stroke registered a change before release")
			}
			switch interruption {
			case "refocus":
				SetEditor(e)
				if e.commits != 0 {
					t.Fatal("refocus ended a held pixel drag")
				}
			case "save":
				FinishStroke()
			case "disable":
				SetEnabled(false)
				SetEnabled(true)
			}
			for n := 0; n < 5; n++ {
				process(false)
			}
			control.dragging = false
			for n := 0; n < 5; n++ {
				process(false)
			}
			if e.commits != 1 || e.previewX != 0 || e.previewY != 0 || len(dmmap.PrefabStorage.GetAllByPath(original.Path())) != 2 {
				t.Fatal("completed pixel stroke must clear preview and register one final change")
			}
		})
	}
}
