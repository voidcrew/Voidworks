package tools

import (
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
	"testing"
)

type selectionEditor struct {
	editor
	dmm *dmmap.Dmm
}

func (e *selectionEditor) Dmm() *dmmap.Dmm         { return e.dmm }
func (e *selectionEditor) TileDelete(p util.Point) { e.dmm.GetTile(p).InstancesSet(nil) }
func (e *selectionEditor) TileReplace(p util.Point, prefabs dmmdata.Prefabs) {
	e.dmm.GetTile(p).InstancesSet(prefabs)
}
func (*selectionEditor) UpdateCanvasByCoords([]util.Point) {}
func (*selectionEditor) CommitChanges(string)              {}

func TestGrabSelectionSurvivesRefocusButNotEditorChange(t *testing.T) {
	previousEditor, previousTool := ed, Selected().Name()
	defer func() { SetSelected(previousTool); ed = previousEditor }()
	coord := util.Point{X: 1, Y: 1, Z: 1}
	m := &dmmap.Dmm{MaxX: 1, MaxY: 1, MaxZ: 1, Tiles: []*dmmap.Tile{{Coord: coord}}}
	first, second := &selectionEditor{dmm: m}, &selectionEditor{dmm: m}
	SetEditor(first)
	if !SetGrabSelection(coord, coord) {
		t.Fatal("could not select tile")
	}
	SetEditor(first)
	if _, _, ready := SelectionBounds(); !ready {
		t.Fatal("returning from a menu cleared the same document's selection")
	}
	SetEditor(second)
	if _, _, ready := SelectionBounds(); ready {
		t.Fatal("selection leaked into another document")
	}
}

func TestGrabSelectionUsesCurrentBoundsAndContents(t *testing.T) {
	previousEditor, previousTool := ed, Selected().Name()
	defer func() { SetSelected(previousTool); ed = previousEditor }()
	m := &dmmap.Dmm{MaxX: 10, MaxY: 10, MaxZ: 1}
	for y := 1; y <= 10; y++ {
		for x := 1; x <= 10; x++ {
			m.Tiles = append(m.Tiles, &dmmap.Tile{Coord: util.Point{X: x, Y: y, Z: 1}})
		}
	}
	ed = &selectionEditor{dmm: m}
	grab := SetSelected(TNGrab).(*ToolGrab)
	grab.Reset()
	if _, _, ready := SelectionBounds(); ready {
		t.Fatal("empty selection accepted")
	}
	grab.onStart(util.Point{X: 5, Y: 6, Z: 1})
	grab.onMove(util.Point{X: 2, Y: 3, Z: 1})
	if _, _, ready := SelectionBounds(); ready {
		t.Fatal("unfinished selection accepted")
	}
	grab.onStop(util.Point{})
	lo, hi, ready := SelectionBounds()
	if !ready || lo != (util.Point{X: 2, Y: 3, Z: 1}) || hi != (util.Point{X: 5, Y: 6, Z: 1}) {
		t.Fatalf("wrong backwards bounds: %v %v", lo, hi)
	}
	vars := dmvars.MutableVariables{}
	prefab := dmmprefab.New(0, "/area/ship/bridge", vars.ToImmutable())
	m.GetTile(lo).InstancesAdd(prefab)
	RefreshGrabSelection()
	grab.onStart(lo)
	grab.onMove(lo.Plus(util.Point{X: 1}))
	if _, _, ready := SelectionBounds(); ready {
		t.Fatal("moving selection accepted before release")
	}
	grab.onStop(util.Point{})
	nextLo, nextHi, ready := SelectionBounds()
	if !ready || nextLo.X != 3 || nextHi.X != 6 {
		t.Fatal("selection returned old position after moving")
	}
	instances := m.GetTile(nextLo).Instances()
	if len(instances) != 1 || instances[0].Prefab().Path() != prefab.Path() {
		t.Fatal("moving selection lost externally assigned area")
	}
	SetSelected(TNAdd)
	if _, _, ready := SelectionBounds(); ready {
		t.Fatal("tool switch kept selection")
	}
	if !SetGrabSelection(lo, hi) {
		t.Fatal("could not transfer selection")
	}
	SetSelected(TNGrab).OnDeselect()
	if _, _, ready := SelectionBounds(); ready {
		t.Fatal("deselection kept bounds")
	}
}
