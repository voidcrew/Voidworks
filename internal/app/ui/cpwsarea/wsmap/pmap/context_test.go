package pmap

import (
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/util"
	"testing"
)

func TestTileMenuUsesDisplayedContentsAndNativeOwnership(t *testing.T) {
	local := util.Point{X: 1, Y: 1, Z: 1}
	native := dmminstance.New(local, dmmprefab.New(1, "/obj/wardrobe", nil))
	other := dmminstance.New(local, dmmprefab.New(2, "/obj/plush", nil))
	sourceTile := &dmmap.Tile{Coord: local}
	sourceTile.Set(dmmap.Instances{native})
	source := &dmmap.Dmm{MaxX: 1, MaxY: 1, MaxZ: 1, Tiles: []*dmmap.Tile{sourceTile}}
	p := &PaneMap{dmm: source}
	plain := p.TileMenuContents(local)
	if len(plain.Entries) != 1 || !plain.Entries[0].Editable || plain.Entries[0].Instance != native {
		t.Fatal("ordinary map did not retain its native instance")
	}
	view := &dmmap.Dmm{MaxX: 2, MaxY: 1, MaxZ: 1}
	for x := 1; x <= 2; x++ {
		coord := util.Point{X: x, Y: 1, Z: 1}
		tile := &dmmap.Tile{Coord: coord}
		if x == 1 {
			tile.Set(dmmap.Instances{other.CopyAt(coord)})
		} else {
			tile.Set(dmmap.Instances{native.CopyAt(coord), other.CopyAt(coord)})
		}
		view.Tiles = append(view.Tiles, tile)
	}
	selected := false
	p.context = &EditContext{View: view, Offset: util.Point{X: 1}, Editable: map[uint64]*dmminstance.Instance{native.Id(): native}, Inspect: func(i *dmminstance.Instance) (string, func()) {
		if i.Id() != other.Id() {
			t.Fatal("resolved the wrong object")
		}
		return "Other room", func() { selected = true }
	}}
	contents := p.TileMenuContents(local)
	if contents.Coord.X != 2 || len(contents.Entries) != 2 {
		t.Fatal("menu differs from assembled tile")
	}
	for _, entry := range contents.Entries {
		if entry.Instance.Id() == native.Id() {
			if entry.Instance != native || !entry.Editable {
				t.Fatal("editing a projected copy instead of native source")
			}
		} else {
			if entry.Editable || entry.Source != "Other room" || entry.EditSource == nil {
				t.Fatal("foreign object lacks safe ownership routing")
			}
			entry.EditSource()
		}
	}
	if !selected {
		t.Fatal("source action was unavailable")
	}
	// Local x=0 is outside the active room, but inside the assembled hull.
	outsideRoom := p.TileMenuContents(util.Point{X: 0, Y: 1, Z: 1})
	if len(outsideRoom.Entries) != 1 || outsideRoom.Entries[0].Instance.Id() != other.Id() {
		t.Fatal("cannot inspect outside active room")
	}
	if len(p.TileMenuContents(util.Point{X: -1, Y: 1, Z: 1}).Entries) != 0 {
		t.Fatal("menu opened outside the view")
	}
	p.context.View = source
	if got := p.TileMenuContents(util.Point{X: 0, Y: 1, Z: 1}); len(got.Entries) != 1 || got.Entries[0].Instance != native || !got.Entries[0].Editable {
		t.Fatal("isolated source lost editing")
	}
}
