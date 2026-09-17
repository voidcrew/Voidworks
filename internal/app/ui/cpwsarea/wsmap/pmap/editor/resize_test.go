package editor

import (
	"testing"

	"sdmm/internal/app/command"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/util"
)

type resizeTestApp struct {
	app
	commands *command.Storage
}

func (a *resizeTestApp) CommandStorage() *command.Storage { return a.commands }
func (*resizeTestApp) SyncVarEditor()                     {}

type resizeTestMap struct {
	attachedMap
	snap  *dmmsnap.DmmSnap
	level int
}

func (m *resizeTestMap) Snapshot() *dmmsnap.DmmSnap { return m.snap }
func (m *resizeTestMap) ActiveLevel() int           { return m.level }
func (m *resizeTestMap) SetActiveLevel(z int)       { m.level = z }
func (*resizeTestMap) OnMapSizeChange()             {}
func (*resizeTestMap) CommandStackId() string       { return "resize-test" }

func TestDirectionalResizeHistoryDoesNotDrift(t *testing.T) {
	oldTurf, oldArea := dmmap.BaseTurf, dmmap.BaseArea
	defer func() { dmmap.BaseTurf, dmmap.BaseArea = oldTurf, oldArea }()
	dmmap.BaseTurf = dmmprefab.New(1, "/turf/space", nil)
	dmmap.BaseArea = dmmprefab.New(2, "/area/space", nil)
	m := &dmmap.Dmm{}
	m.SetMapSize(5, 5, 2)
	origin := util.Point{X: 2, Y: 2, Z: 2}
	m.GetTile(origin).InstancesAdd(dmmprefab.New(3, "/obj/modular_map_root/ship_upgrade", nil))
	pane := &resizeTestMap{snap: dmmsnap.New(m), level: 2}
	a := &resizeTestApp{commands: command.NewStorage()}
	a.commands.SetStack(pane.CommandStackId())
	e := &Editor{app: a, dmm: m, pMap: pane}
	check := func(w, h, z int, want util.Point, count int) {
		t.Helper()
		if m.MaxX != w || m.MaxY != h || m.MaxZ != z {
			t.Fatal("history restored incorrect dimensions")
		}
		got := 0
		for _, tile := range m.Tiles {
			for _, instance := range tile.Instances() {
				if instance.Prefab().Id() == 3 {
					got++
					if instance.Coord() != want || tile.Coord != want {
						t.Fatal("helper shifted incorrectly during undo/redo")
					}
				}
			}
		}
		if got != count {
			t.Fatalf("history has %d helpers, want %d", got, count)
		}
	}
	m.Resize(8, 7, 2, dmmap.ResizeSouthWest)
	e.CommitMapSizeChange(5, 5, 2)
	dest := util.Point{X: 5, Y: 4, Z: 2}
	check(8, 7, 2, dest, 1)
	for range 3 {
		a.commands.Undo()
		check(5, 5, 2, origin, 1)
		a.commands.Redo()
		check(8, 7, 2, dest, 1)
	}
	// Cropped contents must return on undo, but never leak into the redo state.
	m.Resize(4, 3, 1, dmmap.ResizeNorthEast)
	e.CommitMapSizeChange(8, 7, 2)
	check(4, 3, 1, util.Point{}, 0)
	for range 3 {
		a.commands.Undo()
		check(8, 7, 2, dest, 1)
		a.commands.Redo()
		check(4, 3, 1, util.Point{}, 0)
	}
	if pane.level != 1 {
		t.Fatal("active level outside cropped map")
	}
}
