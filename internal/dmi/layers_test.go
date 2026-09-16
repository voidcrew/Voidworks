package dmi

import (
	"bytes"
	"image/color"
	"reflect"
	"testing"
)

func layeredIcon(t *testing.T) *Icon {
	t.Helper()
	i := testIcon(t)
	s := i.States[0]
	l := s.AddLayer(i.Width, i.Height)
	for n := range s.Cels {
		p := CopyImage(s.RawCel(l, n))
		p.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
		if err := s.SetCel(l, n, p); err != nil {
			t.Fatal(err)
		}
	}
	return i
}

func TestLayerPersistenceAndFrameOperations(t *testing.T) {
	i := layeredIcon(t)
	for _, step := range []func() error{
		func() error { return nil },
		func() error { return i.InsertFrame(0, 0, true) },
		func() error { return i.InsertFrame(0, 1, false) },
		func() error { return i.MoveFrame(0, 0, 2) },
		func() error { return i.DeleteFrame(0, 1) },
		func() error { return i.SetDirections(0, 4) },
		func() error { return i.SetDirections(0, 8) },
		func() error { return i.Resize(6, 4, 1, 1, true) },
		func() error { return i.SplitState(0, true) },
		func() error { return i.SplitState(0, false) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
		data, err := Encode(i)
		if err != nil {
			t.Fatal(err)
		}
		read, err := Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		compareIcons(t, i, read)
		for n, s := range i.States {
			if !reflect.DeepEqual(s.Layers, read.States[n].Layers) {
				t.Fatalf("state %d layers changed", n)
			}
		}
	}
}

func TestLayerVisibilityLocksAndHistory(t *testing.T) {
	i := layeredIcon(t)
	original := i.Clone()
	s := i.States[0]
	s.Layers[1].Visible = false
	s.RecomposeAll()
	if s.Cels[0].NRGBAAt(0, 0).A != 0 || original.States[0].Cels[0].NRGBAAt(0, 0).A != 128 {
		t.Fatal("layer visibility mutated history or failed to hide pixels")
	}
	s.Layers[1].Locked = true
	if err := s.SetCel(1, 0, CopyImage(s.Cels[0])); err == nil {
		t.Fatal("locked layer accepted paint")
	}
	if !bytes.Equal(s.Layers[0].Cels[0].Pix, original.States[0].Layers[0].Cels[0].Pix) {
		t.Fatal("base layer changed")
	}
	// Hidden RGB survives even when introducing an empty layer.
	x := testIcon(t)
	x.States[0].AddLayer(x.Width, x.Height)
	data, err := Encode(x)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Decode(data); err != nil {
		t.Fatal(err)
	}
}

func TestStaleLayerDataRejected(t *testing.T) {
	i := layeredIcon(t)
	data, err := i.encodeLayers()
	if err != nil {
		t.Fatal(err)
	}
	i.States[0].Name = "renamed elsewhere"
	if err = i.decodeLayers(data); err == nil {
		t.Fatal("stale layer data silently accepted")
	}
}

func TestLayerGrowthRejectedBeforeAllocation(t *testing.T) {
	i, _ := New(1024, 1024)
	s := i.States[0]
	// Shared immutable buffers let the fixture reach the logical layer limit
	// without consuming the memory the rejected operations would allocate.
	for n := 0; n < 16; n++ {
		s.Layers = append(s.Layers, Layer{Name: "layer", Visible: true, Opacity: 255, Cels: s.Cels})
	}
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { _, err := i.AddLayer(0); return err },
		func() error { _, err := i.DuplicateLayer(0, 0); return err },
		func() error { return i.InsertFrame(0, 0, false) },
		func() error { return i.SetDirections(0, 4) },
	} {
		if err := operation(); err == nil {
			t.Fatal("layer growth exceeded the editing limit")
		}
		if len(s.Layers) != 16 || s.Dirs() != 1 || s.Frames() != 1 || len(s.Cels) != 1 {
			t.Fatal("failed layer edit partially mutated the document")
		}
	}
}
