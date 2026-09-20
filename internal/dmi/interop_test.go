package dmi

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// Optional fixture for external readers (BYOND and Pillow), separate from our
// codec's own encode/decode round trips. Never writes into the game project.
func TestWriteInteropFixture(t *testing.T) {
	root := os.Getenv("DMI_INTEROP_OUTPUT")
	if root == "" {
		t.Skip("set DMI_INTEROP_OUTPUT to produce the independent-reader fixture")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	i := testIcon(t)
	i.States[0].Remove("future_property")
	i.States[1].Remove("future_property")
	i.States[0].Name = "walk"
	i.States[1].Name = "walk"
	for n, cel := range i.States[1].Cels {
		c := CopyImage(cel)
		c.SetNRGBA(1, 1, color.NRGBA{R: uint8(200 - n*8), G: 90, B: 80, A: 255})
		i.States[1].Cels[n] = c
	}
	if err := i.DuplicateStates([]int{0}); err != nil {
		t.Fatal(err)
	}
	i.States[2].Name = "quote\"slash\\"
	if os.Getenv("DMI_INTEROP_LAYERS") == "1" {
		s := i.States[0]
		layer := s.AddLayer(i.Width, i.Height)
		cel := CopyImage(s.RawCel(layer, 0))
		cel.SetNRGBA(1, 1, color.NRGBA{R: 40, G: 200, B: 100, A: 128})
		if err := s.SetCel(layer, 0, cel); err != nil {
			t.Fatal(err)
		}
	}
	data, err := Encode(i)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "fixture.dmi"), data, 0600); err != nil {
		t.Fatal(err)
	}
	type sample struct {
		State              string
		Moving, Dir, Frame int
		RGBA               color.NRGBA
	}
	var samples []sample
	for _, s := range i.States {
		for n, cel := range s.Cels {
			samples = append(samples, sample{s.Name, s.Int("movement", 0), []int{2, 1, 4, 8, 6, 10, 5, 9}[n%s.Dirs()], n/s.Dirs() + 1, cel.NRGBAAt(1, 1)})
		}
	}
	manifest, _ := json.Marshal(samples)
	if err = os.WriteFile(filepath.Join(root, "expected.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
}
