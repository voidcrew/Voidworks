package tools

import (
	"sdmm/internal/app/prefs"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/util"

	"github.com/rs/zerolog/log"
)

const (
	TNAdd     = "Add"
	TNFill    = "Fill"
	TNGrab    = "Grab"
	TNMove    = "Move"
	TNPick    = "Pick"
	TNDelete  = "Delete"
	TNReplace = "Replace"
	TNRegion  = "Select region"
	// Workshop-only: collects an upgrade room's tiles. Not in the tool bar.
	TNRoomShape = "Room shape"
)

func init() {
	window.RunRepeat(func() {
		process(imguiext.IsAltDown()) // Enable tools alt-behaviour when Alt button is down.
	})
}

type canvasControl interface {
	Dragging() bool
}

type canvasState interface {
	HoverOutOfBounds() bool
	HoveredTile() util.Point
	LastHoveredTile() util.Point
}

type editor interface {
	Dmm() *dmmap.Dmm

	CommitChanges(commitMsg string)

	UpdateCanvasByCoords([]util.Point)
	UpdateCanvasByTiles([]dmmap.Tile)

	SelectedPrefab() (*dmmprefab.Prefab, bool)

	OverlayPushTile(coord util.Point, colFill, colBorder util.Color)
	OverlayPushArea(area util.Bounds, colFill, colBorder util.Color)

	InstanceSelect(i *dmminstance.Instance)
	InstanceDelete(i *dmminstance.Instance)

	TileReplace(coord util.Point, prefabs dmmdata.Prefabs)

	TileDeleteSelected()
	TileDelete(util.Point)
	HoveredInstance() *dmminstance.Instance
	ZoomLevel() float32
	Prefs() prefs.Prefs
}

var (
	cc canvasControl
	cs canvasState
	ed editor

	active   bool
	enabled  = true
	oldCoord util.Point
	// Finishing a stroke early must not reuse the same held mouse press.
	pressHandled bool

	tools = map[string]Tool{
		TNAdd:     newAdd(),
		TNFill:    newFill(),
		TNGrab:    newGrab(),
		TNMove:    newMove(),
		TNPick:    newPick(),
		TNDelete:  newDelete(),
		TNReplace: newReplace(),
		TNRegion:  &ToolRegion{},
		TNRoomShape: newRoomShape(),
	}

	selectedToolName = TNAdd

	startedTool Tool
)

func SetSelected(toolName string) Tool {
	if selectedToolName != toolName {
		log.Print("selecting:", toolName)
		tools[selectedToolName].OnDeselect()
		selectedToolName = toolName
	}
	return Selected()
}

func IsSelected(toolName string) bool {
	return selectedToolName == toolName
}

func SetEditor(editor editor) {
	// Regaining focus can rebind the same editor after a stroke has started.
	if ed == editor {
		return
	}
	FinishStroke()
	Selected().OnDeselect()
	ed = editor
}

func FinishStroke() {
	if active {
		active = false
		startedTool.onStop(oldCoord)
	}
}

// SetEnabled pauses tools while a read-only canvas owns focus.
func SetEnabled(value bool) {
	if !value {
		FinishStroke()
	}
	enabled = value
}

func SetCanvasControl(canvasControl canvasControl) {
	cc = canvasControl
}

func SetCanvasState(canvasState canvasState) {
	cs = canvasState
}

func Selected() Tool {
	return tools[selectedToolName]
}

func Tools() map[string]Tool {
	return tools
}

func process(altBehaviour bool) {
	if cc != nil && !cc.Dragging() {
		pressHandled = false
	}
	if !enabled {
		return
	}
	if active && startedTool != Selected() {
		startedTool.onStop(oldCoord)
	}

	Selected().process()
	Selected().setAltBehaviour(altBehaviour)
	processSelectedToolStart()
	processSelectedToolsStop()
}

func OnMouseMove() {
	if !enabled {
		return
	}
	processSelectedToolMove()
}

func SelectedTiles() []util.Point {
	if selectTool, ok := Selected().(*ToolGrab); ok {
		if len(selectTool.initTiles) > 0 {
			tiles := make([]util.Point, 0, len(selectTool.initTiles))
			for _, tile := range selectTool.initTiles {
				tiles = append(tiles, tile.Coord)
			}
			return tiles
		}
	}
	return []util.Point{cs.LastHoveredTile()}
}

func processSelectedToolStart() {
	if cs == nil || cc == nil || cs.HoverOutOfBounds() && !Selected().IgnoreBounds() {
		return
	}
	if cc.Dragging() && !active && !pressHandled {
		pressHandled = true
		startedTool = Selected()
		Selected().onStart(cs.HoveredTile())
		active = true
	}
}

func processSelectedToolMove() {
	if cs == nil || cs.HoverOutOfBounds() && !Selected().IgnoreBounds() {
		return
	}
	coord := cs.HoveredTile()
	if coord != oldCoord && active {
		Selected().onMove(coord)
	}
	oldCoord = coord
}

func processSelectedToolsStop() {
	if cc != nil && !cc.Dragging() && active {
		FinishStroke()
	}
}
