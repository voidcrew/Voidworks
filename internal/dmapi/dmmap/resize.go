package dmmap

import "sdmm/internal/util"

// ResizeDirection chooses the edges where space is added or removed.
// The zero value retains the original north/east resize behavior.
type ResizeDirection int

const (
	ResizeNorthEast ResizeDirection = iota
	ResizeNorth
	ResizeNorthWest
	ResizeEast
	ResizeCenter
	ResizeWest
	ResizeSouthEast
	ResizeSouth
	ResizeSouthWest
)

func (d ResizeDirection) String() string {
	names := [...]string{"North / East", "North", "North / West", "East", "Evenly around the map", "West", "South / East", "South", "South / West"}
	if d < ResizeNorthEast || d > ResizeSouthWest {
		return names[0]
	}
	return names[d]
}

// Offset keeps odd centered growth and shrinkage reversible. Any extra tile
// goes to the north/east edge. Z levels retain their existing numbering.
func (d ResizeDirection) Offset(deltaX, deltaY int) util.Point {
	switch d {
	case ResizeNorth:
		return util.Point{X: deltaX / 2}
	case ResizeNorthWest:
		return util.Point{X: deltaX}
	case ResizeEast:
		return util.Point{Y: deltaY / 2}
	case ResizeCenter:
		return util.Point{X: deltaX / 2, Y: deltaY / 2}
	case ResizeWest:
		return util.Point{X: deltaX, Y: deltaY / 2}
	case ResizeSouthEast:
		return util.Point{Y: deltaY}
	case ResizeSouth:
		return util.Point{X: deltaX / 2, Y: deltaY}
	case ResizeSouthWest:
		return util.Point{X: deltaX, Y: deltaY}
	default:
		return util.Point{}
	}
}
