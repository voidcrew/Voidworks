package tools

import (
	"sdmm/internal/app/prefs"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/imguiext"
	"sdmm/internal/util"
	"strconv"

	"github.com/SpaiR/imgui-go"
)

// ToolMove can be used move a single object.
type ToolMove struct {
	tool
	instance        *dmminstance.Instance
	lastTile        *dmmap.Tile
	lastMouseCoords imgui.Vec2
	lastOffsets     [2]int
	initialPrefab   *dmmprefab.Prefab
}

func (ToolMove) Name() string {
	return TNMove
}

func newMove() *ToolMove {
	return &ToolMove{}
}

func (t *ToolMove) Stale() bool {
	return t.instance == nil
}

func (ToolMove) AltBehaviour() bool {
	return false
}

func (t *ToolMove) onStart(util.Point) {
	if hoveredInstance := ed.HoveredInstance(); hoveredInstance != nil {
		ed.InstanceSelect(hoveredInstance)
		t.instance = hoveredInstance
		t.initialPrefab = hoveredInstance.Prefab()
		t.lastMouseCoords = imgui.MousePos()
		vars := t.instance.Prefab().Vars()
		switch ed.Prefs().Editor.NudgeMode {
		case prefs.SaveNudgeModePixel:
			t.lastOffsets = [2]int{vars.IntV("pixel_x", 0), vars.IntV("pixel_y", 0)}
		case prefs.SaveNudgeModeStep:
			t.lastOffsets = [2]int{vars.IntV("step_x", 0), vars.IntV("step_y", 0)}
		case prefs.SaveNudgeModePixelAlt:
			t.lastOffsets = [2]int{vars.IntV("pixel_w", 0), vars.IntV("pixel_z", 0)}
		}
	}
}

func (t *ToolMove) process() {
	if t.instance == nil || !imguiext.IsShiftDown() {
		return
	}
	xAxis := "pixel_x"
	yAxis := "pixel_y"
	if ed.Prefs().Editor.NudgeMode == prefs.SaveNudgeModeStep {
		xAxis = "step_x"
		yAxis = "step_y"
	} else if ed.Prefs().Editor.NudgeMode == prefs.SaveNudgeModePixelAlt {
		xAxis = "pixel_w"
		yAxis = "pixel_z"
	}
	mouseCoords := imgui.MousePos()
	offsetX := (mouseCoords.X - t.lastMouseCoords.X) / ed.ZoomLevel()
	offsetY := (t.lastMouseCoords.Y - mouseCoords.Y) / ed.ZoomLevel()
	x, y := t.lastOffsets[0]+int(offsetX), t.lastOffsets[1]+int(offsetY)
	currentVars := t.instance.Prefab().Vars()
	if currentVars.IntV(xAxis, 0) == x && currentVars.IntV(yAxis, 0) == y {
		return
	}

	// Derive every preview from the starting prefab so returning to the start
	// restores its exact overrides, including inherited offsets.
	prefab := t.initialPrefab
	newVars := prefab.Vars()
	if x != t.lastOffsets[0] {
		newVars = dmvars.Set(newVars, xAxis, strconv.Itoa(x))
	}
	if y != t.lastOffsets[1] {
		newVars = dmvars.Set(newVars, yAxis, strconv.Itoa(y))
	}
	if newVars != prefab.Vars() {
		prefab = dmmprefab.New(dmmprefab.IdNone, prefab.Path(), newVars)
	}
	t.instance.SetPrefab(prefab)

	ed.UpdateCanvasByCoords([]util.Point{t.instance.Coord()})
}

func (t *ToolMove) onMove(coord util.Point) {
	if t.instance == nil || imguiext.IsShiftDown() {
		return
	}

	prefab := t.instance.Prefab()
	if t.lastTile != nil {
		t.lastTile.InstancesRegenerate() //should stop some issues
	}
	t.lastTile = ed.Dmm().GetTile(coord)
	ed.InstanceDelete(t.instance)
	t.lastTile.InstancesAdd(prefab)
	t.lastTile.InstancesRegenerate()
	for _, found := range t.lastTile.Instances() {
		if found.Prefab().Id() == prefab.Id() {
			t.instance = found
			break
		}
	}
	ed.UpdateCanvasByCoords([]util.Point{coord})
}

func (t *ToolMove) onStop(util.Point) {
	if t.instance == nil {
		return
	}
	//remove other turfs if we moved a turf
	if t.lastTile != nil {
		if dm.IsPath(t.instance.Prefab().Path(), "/turf") {
			for _, found := range t.lastTile.Instances() {
				if dm.IsPath(found.Prefab().Path(), "/turf") && found != t.instance {
					ed.InstanceDelete(found)
				}
			}
		}
	}
	// Only the released position belongs in the reusable prefab list.
	t.instance.SetPrefab(dmmap.PrefabStorage.Put(t.instance.Prefab()))
	ed.InstanceSelect(t.instance)
	t.instance = nil
	t.lastTile = nil
	t.initialPrefab = nil
	ed.CommitChanges("Moved Prefab")
}
