package render

import (
	"sdmm/internal/app/render/bucket/level/chunk/unit"
	"sdmm/internal/util"
)

type pixelOffsetPreview struct {
	id    uint64
	coord util.Point
	x, y  int
}

// PreviewPixelOffset moves only the rendered sprite. Its source instance,
// cached render units and map history remain unchanged until the drag ends.
func (r *Render) PreviewPixelOffset(id uint64, coord util.Point, x, y int) {
	r.pixelPreview = pixelOffsetPreview{id: id, coord: coord, x: x, y: y}
}

func (r *Render) ClearPixelOffsetPreview() { r.pixelPreview = pixelOffsetPreview{} }

func (r *Render) previewUnit(u unit.Unit) unit.Unit {
	if r.pixelPreview.id != 0 && u.Instance().Id() == r.pixelPreview.id {
		return u.WithOffset(float32(r.pixelPreview.x), float32(r.pixelPreview.y))
	}
	return u
}
