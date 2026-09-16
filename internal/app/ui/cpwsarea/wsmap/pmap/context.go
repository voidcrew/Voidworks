package pmap

import (
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/canvas"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/tilemenu"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/util"
)

// EditContext changes only presentation. Tools, snapshots and saving retain the
// active source document's native coordinates and instance identities.
type EditContext struct {
	View          *dmmap.Dmm
	Offset        util.Point
	StackID       string
	Refresh       func()
	BeforeHistory func()
	Filter        func(string) bool
	Editable      map[uint64]*dmminstance.Instance
	// Inspect locates the owner of a displayed instance outside this source.
	Inspect func(*dmminstance.Instance) (string, func())
	// Overlay draws the owner's own shapes each frame in view coordinates.
	Overlay func(OverlayPainter)
}

// OverlayPainter pushes overlays addressed by view tiles (1-based, already
// including any context offset), so an owner can mark tiles outside the part
// being edited.
type OverlayPainter struct{ p *PaneMap }

func (o OverlayPainter) Tile(coord util.Point, fill util.Color) {
	size := float32(dmmap.WorldIconSize)
	x, y := float32(coord.X-1)*size, float32(coord.Y-1)*size
	o.p.canvasOverlay.PushArea(canvas.OverlayArea{Bounds_: util.Bounds{X1: x, Y1: y, X2: x + size, Y2: y + size}, FillColor_: fill})
}

// Edges outlines only the named sides of a tile.
func (o OverlayPainter) Edges(coord util.Point, sides util.Sides, color util.Color) {
	size := float32(dmmap.WorldIconSize)
	x, y := float32(coord.X-1)*size, float32(coord.Y-1)*size
	var borders []util.Bounds
	if sides.North {
		borders = append(borders, util.Bounds{X1: x, Y1: y + size, X2: x + size, Y2: y + size})
	}
	if sides.East {
		borders = append(borders, util.Bounds{X1: x + size, Y1: y, X2: x + size, Y2: y + size})
	}
	if sides.South {
		borders = append(borders, util.Bounds{X1: x, Y1: y, X2: x + size, Y2: y})
	}
	if sides.West {
		borders = append(borders, util.Bounds{X1: x, Y1: y, X2: x, Y2: y + size})
	}
	if len(borders) > 0 {
		o.p.canvasOverlay.PushAreaBorder(canvas.OverlayAreaBorder{Borders_: borders, Color_: color})
	}
}

func (p *PaneMap) SetEditContext(context *EditContext) {
	p.context = context
	p.canvasControl.AtCursor = context != nil
	if context != nil {
		p.canvasState.Offset = context.Offset
		p.canvas.ClearColor = canvas.Color{R: 0.065, G: 0.075, B: 0.09, A: 1}
	}
}

func (p *PaneMap) ViewDmm() *dmmap.Dmm {
	if p.context != nil && p.context.View != nil {
		return p.context.View
	}
	return p.dmm
}

func (p *PaneMap) CommandStackId() string {
	if p.context != nil {
		return p.context.StackID
	}
	return p.dmm.Path.Absolute
}

func (p *PaneMap) InContext() bool { return p.context != nil }
func (p *PaneMap) BeforeHistory() {
	if p.context != nil && p.context.BeforeHistory != nil {
		p.context.BeforeHistory()
	}
}

func (p *PaneMap) MapToView(coord util.Point) util.Point {
	if p.context != nil {
		coord.X += p.context.Offset.X
		coord.Y += p.context.Offset.Y
	}
	return coord
}

// TileMenuContents uses the same assembled tile as the renderer, including
// tiles outside the active room. Map tools still use the source's own bounds.
func (p *PaneMap) TileMenuContents(coord util.Point) tilemenu.Contents {
	coord = p.MapToView(coord)
	contents := tilemenu.Contents{Coord: coord}
	view := p.ViewDmm()
	if !view.HasTile(coord) {
		return contents
	}
	for _, instance := range view.GetTile(coord).Instances().Sorted() {
		entry := tilemenu.Entry{Instance: instance, Editable: true}
		if p.context != nil && view != p.dmm {
			if source := p.context.Editable[instance.Id()]; source != nil {
				entry.Instance = source
			} else {
				entry.Editable = false
				if p.context.Inspect != nil {
					entry.Source, entry.EditSource = p.context.Inspect(instance)
				}
			}
		}
		contents.Entries = append(contents.Entries, entry)
	}
	return contents
}

func (p *PaneMap) Refresh(coords []util.Point) {
	if p.context != nil && p.context.Refresh != nil {
		p.context.Refresh()
		return
	}
	p.canvas.Render().UpdateBucketV(p.dmm, p.activeLevel, coords)
}

func (p *PaneMap) RenderContext() { p.canvas.Render().ReplaceBucket(p.ViewDmm(), p.activeLevel) }

func (p *PaneMap) FitView() {
	p.centered = false
}
