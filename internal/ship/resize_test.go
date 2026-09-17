package ship

import (
	"bytes"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

func TestResizeTowardMovesRoomsAndRoundTrips(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "resize_ship", "Resize ship", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.AddSlot(0, "cargo", "Cargo", pt(4, 4), FullFootprint(3, 3)); err != nil {
		t.Fatal(err)
	}
	selected := map[string]string{"cargo": p.Hull.Modules[0].ID}
	a, err := p.Assemble(p.Hull.Themes[0], selected)
	if err != nil {
		t.Fatal(err)
	}
	before := p.Capture()
	moduleBytes := RawData(a.Sources[1].Live).EncodeTGM()
	originalOffset := a.Sources[1].Offset
	originalMarker := a.Markers["cargo"]
	for direction := dmmap.ResizeNorthEast; direction <= dmmap.ResizeSouthWest; direction++ {
		t.Run(direction.String(), func(t *testing.T) {
			p.Restore(before)
			if err := p.ResizeToward(p.Hull.Themes[0], 23, 25, direction); err != nil {
				t.Fatal(err)
			}
			a, err := p.Assemble(p.Hull.Themes[0], selected)
			if err != nil {
				t.Fatal(err)
			}
			offset := direction.Offset(3, 5)
			if a.Markers["cargo"] != originalMarker.Plus(offset) || a.Sources[1].Offset != originalOffset.Plus(offset) {
				t.Fatal("room did not move with the hull helper")
			}
			if !bytes.Equal(moduleBytes, RawData(a.Sources[1].Live).EncodeTGM()) {
				t.Fatal("resizing the hull changed its room source")
			}
			if err := p.ResizeToward(p.Hull.Themes[0], 20, 20, direction); err != nil {
				t.Fatal(err)
			}
			for file, old := range before.Maps {
				if !bytes.Equal(RawData(&old).EncodeTGM(), RawData(p.Documents[file].Map).EncodeTGM()) {
					t.Fatal("inverse resize did not preserve original map bytes")
				}
			}
		})
	}
	p.Restore(before)
	if err := p.ResizeToward(p.Hull.Themes[0], 23, 25, dmmap.ResizeSouthWest); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenProject(c, env, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	a, err = reopened.Assemble(reopened.Hull.Themes[0], selected)
	if err != nil || a.Markers["cargo"] != originalMarker.Plus(util.Point{X: 3, Y: 5}) {
		t.Fatalf("save/reopen lost the shifted room: %v", err)
	}
	baseline := RawData(a.Sources[0].Live).EncodeTGM()
	if err := reopened.ResizeToward(reopened.Hull.Themes[0], 5, 5, dmmap.ResizeSouthWest); err == nil {
		t.Fatal("resize cropped occupied hull tiles")
	}
	if !bytes.Equal(baseline, RawData(a.Sources[0].Live).EncodeTGM()) {
		t.Fatal("failed resize changed the map")
	}
}

func TestResizeRejectsCroppingRoomOption(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "crop_room", "Crop room", 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddSlot(0, "cargo", "Cargo", pt(4, 4), FullFootprint(5, 5)); err != nil {
		t.Fatal(err)
	}
	a, err := p.Assemble(p.Hull.Themes[0], map[string]string{"cargo": p.Hull.Modules[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	// Keep the mobile port and marker, but crop a room tile outside both.
	a.Sources[1].Live.GetTile(pt(1, 5)).InstancesAdd(dmmap.PrefabStorage.Initial("/obj/item/test"))
	before := RawData(a.Sources[0].Live).EncodeTGM()
	if err := p.ResizeToward(p.Hull.Themes[0], 20, 5, dmmap.ResizeNorthEast); err == nil {
		t.Fatal("resize cropped room contents over transparent hull tiles")
	}
	if !bytes.Equal(before, RawData(a.Sources[0].Live).EncodeTGM()) {
		t.Fatal("rejected room crop changed the hull")
	}
}
