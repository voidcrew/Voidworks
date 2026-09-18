package ship

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/util"
)

// Footprint is the set of tiles a room occupies inside its module's bounding
// box. Cells are 1-based module coordinates on Z 1, like module map tiles.
type Footprint struct {
	W, H  int
	Cells map[util.Point]bool
}

// The hull marker's footprint mask lists rows from the top of the module down,
// "/"-separated; "#" is part of the room and "." stays hull. Absent means full.
const footprintVar = "footprint"

func FullFootprint(w, h int) Footprint {
	f := Footprint{W: w, H: h, Cells: map[util.Point]bool{}}
	for y := 1; y <= h; y++ {
		for x := 1; x <= w; x++ {
			f.Cells[util.Point{X: x, Y: y, Z: 1}] = true
		}
	}
	return f
}

// parseMask reads a mask whose dimensions come from its own rows.
func parseMask(mask string) (Footprint, error) {
	rows := strings.Split(mask, "/")
	f := Footprint{W: len(rows[0]), H: len(rows), Cells: map[util.Point]bool{}}
	if f.W == 0 {
		return Footprint{}, fmt.Errorf("module shape is empty")
	}
	for r, row := range rows {
		if len(row) != f.W {
			return Footprint{}, fmt.Errorf("module shape rows differ in width")
		}
		for i := 0; i < len(row); i++ {
			switch row[i] {
			case '#':
				f.Cells[util.Point{X: i + 1, Y: f.H - r, Z: 1}] = true
			case '.':
			default:
				return Footprint{}, fmt.Errorf("module shape uses %q; use # and .", row[i])
			}
		}
	}
	if len(f.Cells) == 0 {
		return Footprint{}, fmt.Errorf("module shape has no tiles")
	}
	return f, nil
}

func ParseFootprint(mask string, w, h int) (Footprint, error) {
	f, err := parseMask(mask)
	if err != nil {
		return Footprint{}, err
	}
	if f.W != w || f.H != h {
		return Footprint{}, fmt.Errorf("module shape is %dx%d but the module is %dx%d", f.W, f.H, w, h)
	}
	return f, nil
}

func (f Footprint) String() string {
	rows := make([]string, 0, f.H)
	for y := f.H; y >= 1; y-- {
		row := make([]byte, f.W)
		for x := 1; x <= f.W; x++ {
			row[x-1] = '.'
			if f.Cells[util.Point{X: x, Y: y, Z: 1}] {
				row[x-1] = '#'
			}
		}
		rows = append(rows, string(row))
	}
	return strings.Join(rows, "/")
}

func (f Footprint) Valid() error {
	if f.W < 1 || f.H < 1 {
		return fmt.Errorf("module shape is empty")
	}
	for cell := range f.Cells {
		if cell.X < 1 || cell.Y < 1 || cell.X > f.W || cell.Y > f.H {
			return fmt.Errorf("module shape tile %d,%d is outside its %dx%d box", cell.X, cell.Y, f.W, f.H)
		}
	}
	if f.Count() == 0 {
		return fmt.Errorf("module shape has no tiles")
	}
	return nil
}

func (f Footprint) Contains(rel util.Point) bool {
	return f.Cells[util.Point{X: rel.X, Y: rel.Y, Z: 1}]
}

func (f Footprint) Count() int {
	n := 0
	for _, in := range f.Cells {
		if in {
			n++
		}
	}
	return n
}

func (f Footprint) IsFull() bool { return f.Count() == f.W*f.H }

// Connected reports whether every tile touches the rest orthogonally. Shapes
// may be disconnected; the editor only warns.
func (f Footprint) Connected() bool {
	var start util.Point
	found := false
	for cell, in := range f.Cells {
		if in {
			start, found = cell, true
			break
		}
	}
	if !found {
		return false
	}
	seen := map[util.Point]bool{start: true}
	queue := []util.Point{start}
	for len(queue) > 0 {
		cell := queue[0]
		queue = queue[1:]
		for _, d := range []util.Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}} {
			next := util.Point{X: cell.X + d.X, Y: cell.Y + d.Y, Z: 1}
			if f.Cells[next] && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return len(seen) == f.Count()
}

// FootprintFromTiles turns absolute hull tiles into a bounding box mask and
// its bottom-left origin.
func FootprintFromTiles(tiles []util.Point) (Footprint, util.Point, error) {
	if len(tiles) == 0 {
		return Footprint{}, util.Point{}, fmt.Errorf("select the module's tiles first")
	}
	min, max := tiles[0], tiles[0]
	for _, t := range tiles {
		if t.X < min.X {
			min.X = t.X
		}
		if t.Y < min.Y {
			min.Y = t.Y
		}
		if t.X > max.X {
			max.X = t.X
		}
		if t.Y > max.Y {
			max.Y = t.Y
		}
	}
	min.Z = 1
	f := Footprint{W: max.X - min.X + 1, H: max.Y - min.Y + 1, Cells: map[util.Point]bool{}}
	for _, t := range tiles {
		f.Cells[util.Point{X: t.X - min.X + 1, Y: t.Y - min.Y + 1, Z: 1}] = true
	}
	return f, min, nil
}

// RoomShape places a slot's footprint on the hull. Origin is the hull tile
// under module tile 1,1; Marker is where the hull marker sits, which is the
// same tile whenever the module's connector is at 1,1.
type RoomShape struct {
	Slot           string
	Origin, Marker util.Point
	Footprint      Footprint
}

func (r RoomShape) Contains(hull util.Point) bool {
	return hull.Z == 1 && r.Footprint.Contains(util.Point{X: hull.X - r.Origin.X + 1, Y: hull.Y - r.Origin.Y + 1})
}

// Tiles lists the hull tiles the room occupies, including its marker tile.
func (r RoomShape) Tiles() map[util.Point]bool {
	tiles := map[util.Point]bool{util.Point{X: r.Marker.X, Y: r.Marker.Y, Z: 1}: true}
	for cell, in := range r.Footprint.Cells {
		if in {
			tiles[util.Point{X: r.Origin.X + cell.X - 1, Y: r.Origin.Y + cell.Y - 1, Z: 1}] = true
		}
	}
	return tiles
}

// RoomAt returns the slot whose footprint covers a hull tile.
func (a *Assembly) RoomAt(hull util.Point) (string, bool) {
	for slot, room := range a.Rooms {
		if room.Contains(hull) {
			return slot, true
		}
	}
	return "", false
}

// SlotDisplayName mirrors the purchase screen: underscores become spaces and
// the first letter is capitalized.
func SlotDisplayName(id string) string {
	s := strings.ReplaceAll(id, "_", " ")
	if s == "" {
		return s
	}
	_, size := utf8.DecodeRuneInString(s)
	return strings.ToUpper(s[:size]) + s[size:]
}

// SupportsFootprints reports whether the loaded game declares the marker's
// footprint variable. Older game code only knows rectangular rooms.
func SupportsFootprints(dme *dmenv.Dme) bool {
	if dme == nil || dme.Objects[SlotMarker] == nil {
		return false
	}
	_, ok := dme.Objects[SlotMarker].Vars.Value(footprintVar)
	return ok
}

func (p *Project) SupportsFootprints() bool { return SupportsFootprints(p.Dme) }

func (p *Project) footprintError(f Footprint) error {
	if err := f.Valid(); err != nil {
		return err
	}
	if !f.IsFull() && !p.SupportsFootprints() {
		return fmt.Errorf("This project's game code does not support custom module shapes yet; update tg-voidcrew.")
	}
	return nil
}
