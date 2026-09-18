package ship

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/util"
)

type Source struct {
	Name, File, Slot string
	Data             *dmmdata.DmmData
	Live             *dmmap.Dmm
	Offset           util.Point
}

type Atom struct {
	Prefab   *dmmprefab.Prefab
	Source   int
	Local    util.Point
	Instance *dmminstance.Instance
}

type Issue struct {
	Message string
	Coord   util.Point
}

type Assembly struct {
	Sources          []Source
	Cells            map[util.Point][]Atom
	Markers          map[string]util.Point
	Rooms            map[string]RoomShape // slots with a known module box or shape
	Issues           []Issue
	MaxX, MaxY, MaxZ int
}

// Load constructs a disposable preview. It never serializes the assembled map.
// An explicitly empty selection means bare hull; it is not a game loadout.
func (c Catalog) Load(h Hull, theme Theme, selected map[string]string) (*Assembly, error) {
	file, err := c.HullFile(h, theme)
	if err != nil {
		return nil, err
	}
	data, err := dmmdata.New(file)
	if err != nil {
		return nil, err
	}
	sources := []Source{{Name: "Hull", File: file, Data: data}}
	for _, slot := range h.SlotsFor(theme) {
		id := selected[slot]
		if id == "" {
			continue
		}
		var module *Module
		for i := range h.Modules {
			if h.Modules[i].ID == id && h.Modules[i].Slot == slot && h.Modules[i].Available(theme.ID) {
				module = &h.Modules[i]
				break
			}
		}
		if module == nil {
			return nil, fmt.Errorf("module %s is not available for %s", id, slot)
		}
		file, err := c.ModuleFile(*module, theme.ID)
		if err != nil {
			return nil, err
		}
		data, err := dmmdata.New(file)
		if err != nil {
			return nil, err
		}
		sources = append(sources, Source{Name: module.Name, File: file, Slot: slot, Data: data})
	}
	a, err := Compose(sources)
	if err != nil {
		return nil, err
	}
	for _, slot := range h.SlotsFor(theme) {
		if _, ok := a.Markers[slot]; !ok {
			a.Issues = append(a.Issues, Issue{Message: "Missing hull marker: " + slot})
		}
	}
	for slot, coord := range a.Markers {
		if !Contains(h.SlotsFor(theme), slot) {
			a.Issues = append(a.Issues, Issue{Message: "Marker is not in this theme's slots: " + slot, Coord: coord})
		}
	}
	return a, nil
}

func mappingMarker(path string) bool {
	return path == Connector || strings.HasPrefix(path, Connector+"/") || path == SlotMarker || strings.HasPrefix(path, SlotMarker+"/")
}

func sortedCoords(data *dmmdata.DmmData) []util.Point {
	coords := make([]util.Point, 0, len(data.Grid))
	for coord := range data.Grid {
		coords = append(coords, coord)
	}
	sort.Slice(coords, func(i, j int) bool {
		a, b := coords[i], coords[j]
		if a.Z != b.Z {
			return a.Z < b.Z
		}
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	return coords
}

func Compose(sources []Source) (*Assembly, error) {
	if len(sources) == 0 || sources[0].Data == nil {
		return nil, fmt.Errorf("a hull is required")
	}
	hull := sources[0].Data
	if hull.MaxZ != 1 {
		return nil, fmt.Errorf("ship workspace currently supports single-level hulls")
	}
	a := &Assembly{Sources: append([]Source(nil), sources...), Cells: map[util.Point][]Atom{}, Markers: map[string]util.Point{}, Rooms: map[string]RoomShape{}, MaxX: hull.MaxX, MaxY: hull.MaxY, MaxZ: hull.MaxZ}
	masks := map[string]string{}
	claimedTurfs := map[util.Point]string{}
	claimedAreas := map[util.Point]string{}
	for idx := range a.Sources {
		src := &a.Sources[idx]
		if src.Data == nil || src.Data.MaxZ != 1 {
			return nil, fmt.Errorf("%s must be a single-level map", src.Name)
		}
		coords := sortedCoords(src.Data)
		if len(coords) != src.Data.MaxX*src.Data.MaxY {
			return nil, fmt.Errorf("%s has incomplete map data", src.Name)
		}
		if idx > 0 {
			marker, ok := a.Markers[src.Slot]
			if !ok {
				return nil, fmt.Errorf("%s has no hull marker", src.Slot)
			}
			var connectors []util.Point
			for _, coord := range coords {
				for _, prefab := range src.Data.Dictionary[src.Data.Grid[coord]] {
					if prefab.Path() == Connector || strings.HasPrefix(prefab.Path(), Connector+"/") {
						connectors = append(connectors, coord)
					}
				}
			}
			if len(connectors) != 1 {
				return nil, fmt.Errorf("%s needs exactly one connector; found %d", src.Name, len(connectors))
			}
			src.Offset = util.Point{X: marker.X - connectors[0].X, Y: marker.Y - connectors[0].Y}
			a.Rooms[src.Slot] = a.roomShape(src.Slot, marker, util.Point{X: src.Offset.X + 1, Y: src.Offset.Y + 1, Z: 1}, masks[src.Slot], src.Data.MaxX, src.Data.MaxY)
		}
		for _, local := range coords {
			coord := util.Point{X: local.X + src.Offset.X, Y: local.Y + src.Offset.Y, Z: local.Z}
			prefabs := src.Data.Dictionary[src.Data.Grid[local]]
			var instances dmmap.Instances
			if src.Live != nil {
				instances = src.Live.GetTile(local).Instances()
				prefabs = instances.Prefabs()
			}
			for prefabIndex, prefab := range prefabs {
				path := prefab.Path()
				if idx == 0 && (path == SlotMarker || strings.HasPrefix(path, SlotMarker+"/")) {
					slot := text(prefab.Vars(), "key")
					if slot == "" {
						return nil, fmt.Errorf("hull marker at %v has no key", coord)
					}
					if _, exists := a.Markers[slot]; exists {
						return nil, fmt.Errorf("duplicate hull marker %s", slot)
					}
					a.Markers[slot] = coord
					masks[slot] = text(prefab.Vars(), footprintVar)
				}
				if mappingMarker(path) || path == "/turf/template_noop" || path == "/area/template_noop" {
					continue
				}
				if coord.X < 1 || coord.Y < 1 || coord.X > a.MaxX || coord.Y > a.MaxY {
					return nil, fmt.Errorf("%s content extends outside hull at %v", src.Name, coord)
				}
				group := ""
				if strings.HasPrefix(path, "/turf/") {
					group = "/turf/"
				}
				if strings.HasPrefix(path, "/area/") {
					group = "/area/"
				}
				if group != "" {
					if idx > 0 {
						claims := claimedTurfs
						if group == "/area/" {
							claims = claimedAreas
						}
						if prior, ok := claims[coord]; ok && prior != src.Slot {
							return nil, fmt.Errorf("%s and %s replace the same %s at %v", prior, src.Slot, strings.Trim(group, "/"), coord)
						}
						claims[coord] = src.Slot
					}
					kept := make([]Atom, 0, len(a.Cells[coord]))
					for _, old := range a.Cells[coord] {
						if !strings.HasPrefix(old.Prefab.Path(), group) {
							kept = append(kept, old)
						}
					}
					a.Cells[coord] = kept
				}
				atom := Atom{Prefab: prefab, Source: idx, Local: local}
				if instances != nil {
					atom.Instance = instances[prefabIndex]
				}
				a.Cells[coord] = append(a.Cells[coord], atom)
			}
		}
	}
	// A shaped room describes its own box even before an option is chosen.
	for slot, marker := range a.Markers {
		if _, ok := a.Rooms[slot]; ok || masks[slot] == "" {
			continue
		}
		f, err := parseMask(masks[slot])
		if err != nil {
			a.Issues = append(a.Issues, Issue{Message: SlotDisplayName(slot) + ": " + err.Error(), Coord: marker})
			continue
		}
		a.Rooms[slot] = RoomShape{Slot: slot, Origin: marker, Marker: marker, Footprint: f}
	}
	return a, nil
}

// roomShape reads a marker's mask against its module box, falling back to the
// full rectangle when the shape does not fit.
func (a *Assembly) roomShape(slot string, marker, origin util.Point, mask string, w, h int) RoomShape {
	room := RoomShape{Slot: slot, Origin: origin, Marker: marker, Footprint: FullFootprint(w, h)}
	if mask == "" {
		return room
	}
	f, err := ParseFootprint(mask, w, h)
	if err != nil {
		a.Issues = append(a.Issues, Issue{Message: SlotDisplayName(slot) + ": " + err.Error(), Coord: marker})
		return room
	}
	room.Footprint = f
	return room
}

// Display converts ownership-tagged cells into a render-only DMM. The original
// files are opened separately for edits, using Voidworks's normal save and undo.
func (a *Assembly) Display(dme *dmenv.Dme) (*dmmap.Dmm, error) {
	// Construct display instances directly. Loading this as an editable DMM
	// would persist every temporary Move/Quick Edit offset during a drag.
	const name = "assembled-ship-preview"
	dmm := &dmmap.Dmm{
		Name: name, Path: dmmap.DmmPath{Readable: name, Absolute: filepath.Join(dme.RootDir, name)},
		MaxX: a.MaxX, MaxY: a.MaxY, MaxZ: a.MaxZ,
		Tiles: make([]*dmmap.Tile, 0, a.MaxX*a.MaxY*a.MaxZ),
	}
	for z := 1; z <= a.MaxZ; z++ {
		for y := 1; y <= a.MaxY; y++ {
			for x := 1; x <= a.MaxX; x++ {
				coord := util.Point{X: x, Y: y, Z: z}
				tile := &dmmap.Tile{Coord: coord}
				instances := make(dmmap.Instances, 0, len(a.Cells[coord]))
				for _, atom := range a.Cells[coord] {
					if obj := dme.Objects[atom.Prefab.Path()]; obj != nil && !atom.Prefab.Vars().HasParent() {
						atom.Prefab.Vars().LinkParent(obj.Vars)
					}
					if atom.Instance != nil {
						instances = append(instances, atom.Instance.CopyAt(coord))
					} else {
						instances = append(instances, dmminstance.New(coord, atom.Prefab))
					}
				}
				tile.Set(instances)
				dmm.Tiles = append(dmm.Tiles, tile)
			}
		}
	}
	return dmm, nil
}

// CheckAccess is a static hint, not a runtime pathfinding or atmos simulation.
// Doors count as traversable; access permissions and runtime movement do not.
func (a *Assembly) CheckAccess(dme *dmenv.Dme) {
	defer func() {
		sort.SliceStable(a.Issues, func(i, j int) bool {
			left, right := a.Issues[i], a.Issues[j]
			if left.Coord.Z != right.Coord.Z {
				return left.Coord.Z < right.Coord.Z
			}
			if left.Coord.Y != right.Coord.Y {
				return left.Coord.Y < right.Coord.Y
			}
			if left.Coord.X != right.Coord.X {
				return left.Coord.X < right.Coord.X
			}
			return left.Message < right.Message
		})
	}()
	dense := func(p *dmmprefab.Prefab) bool {
		if v, ok := p.Vars().Int("density"); ok {
			return v != 0
		}
		if obj := dme.Objects[p.Path()]; obj != nil {
			return obj.Vars.IntV("density", 0) != 0
		}
		return false
	}
	passable := func(coord util.Point) bool {
		floor := false
		for _, atom := range a.Cells[coord] {
			path := atom.Prefab.Path()
			if strings.HasPrefix(path, "/turf/open/") && !strings.HasPrefix(path, "/turf/open/space") && !strings.HasPrefix(path, "/turf/open/lava") && !strings.HasPrefix(path, "/turf/open/chasm") {
				floor = true
			}
			if strings.HasPrefix(path, "/obj/machinery/door/") {
				continue
			}
			if dense(atom.Prefab) {
				return false
			}
		}
		return floor
	}
	for coord, atoms := range a.Cells {
		for _, atom := range atoms {
			path := atom.Prefab.Path()
			if path != "/obj/machinery/cryopod" && !strings.HasPrefix(path, "/obj/machinery/cryopod/") {
				continue
			}
			exit := false
			for _, d := range []util.Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}} {
				if passable(util.Point{X: coord.X + d.X, Y: coord.Y + d.Y, Z: coord.Z}) {
					exit = true
				}
			}
			if !exit {
				a.Issues = append(a.Issues, Issue{Message: "Cryopod has no clear adjacent floor (static check)", Coord: coord})
			}
		}
	}
}
