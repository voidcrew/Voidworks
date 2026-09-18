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
	pixelOffset     [2]int
	offsetAxes      [2]string
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
		t.pixelOffset = [2]int{}
		t.offsetAxes = [2]string{"pixel_x", "pixel_y"}
		t.lastMouseCoords = imgui.MousePos()
		vars := t.instance.Prefab().Vars()
		switch ed.Prefs().Editor.NudgeMode {
		case prefs.SaveNudgeModePixel:
			t.lastOffsets = [2]int{vars.IntV("pixel_x", 0), vars.IntV("pixel_y", 0)}
		case prefs.SaveNudgeModeStep:
			t.offsetAxes = [2]string{"step_x", "step_y"}
			t.lastOffsets = [2]int{vars.IntV("step_x", 0), vars.IntV("step_y", 0)}
		case prefs.SaveNudgeModePixelAlt:
			t.offsetAxes = [2]string{"pixel_w", "pixel_z"}
			t.lastOffsets = [2]int{vars.IntV("pixel_w", 0), vars.IntV("pixel_z", 0)}
		}
	}
}

func (t *ToolMove) process() {
	if t.instance == nil || !imguiext.IsShiftDown() {
		return
	}
	mouseCoords := imgui.MousePos()
	offsetX := (mouseCoords.X - t.lastMouseCoords.X) / ed.ZoomLevel()
	offsetY := (t.lastMouseCoords.Y - mouseCoords.Y) / ed.ZoomLevel()
	nextOffset := [2]int{int(offsetX), int(offsetY)}
	if t.pixelOffset == nextOffset {
		return
	}
	t.pixelOffset = nextOffset
	ed.PreviewPixelOffset(t.instance, nextOffset[0], nextOffset[1])
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
	// Shift can be released while the mouse is still held, changing to a tile
	// move. Keep the temporary pixel preview attached to the new instance.
	if t.pixelOffset != [2]int{} {
		ed.PreviewPixelOffset(t.instance, t.pixelOffset[0], t.pixelOffset[1])
	}
}

func (t *ToolMove) onStop(util.Point) {
	if t.instance == nil {
		return
	}
	ed.ClearPixelOffsetPreview()
	if t.pixelOffset != [2]int{} {
		vars := t.initialPrefab.Vars()
		for axis, delta := range t.pixelOffset {
			if delta != 0 {
				vars = dmvars.Set(vars, t.offsetAxes[axis], strconv.Itoa(t.lastOffsets[axis]+delta))
			}
		}
		t.instance.SetPrefab(dmmprefab.New(dmmprefab.IdNone, t.initialPrefab.Path(), vars))
		// Refresh once on release so the settled sprite replaces the preview
		// immediately, including ordinary maps with asynchronous history commits.
		ed.UpdateCanvasByCoords([]util.Point{t.instance.Coord()})
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
