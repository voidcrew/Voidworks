package dmi

import (
	"bytes"
	"image"
	"image/color"
	"reflect"
	"testing"
)

func TestTransformSelectionPreservesUnselectedPixelsAndHoles(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	red, blue := color.NRGBA{R: 200, A: 128}, color.NRGBA{B: 255, A: 255}
	src.SetNRGBA(1, 1, red)
	src.SetNRGBA(2, 2, red)
	src.SetNRGBA(2, 1, blue) // hole in the selection
	src.SetNRGBA(7, 5, blue)
	mask := image.NewAlpha(src.Rect)
	mask.SetAlpha(1, 1, color.Alpha{A: 255})
	mask.SetAlpha(2, 2, color.Alpha{A: 255})
	before := append([]byte(nil), src.Pix...)
	flipped, selection, err := TransformSelection(src, image.Rect(1, 1, 4, 3), mask, FlipHorizontal)
	if err != nil {
		t.Fatal(err)
	}
	if flipped.NRGBAAt(3, 1) != red || flipped.NRGBAAt(1, 1).A != 0 || flipped.NRGBAAt(2, 1) != blue || flipped.NRGBAAt(7, 5) != blue || selection.AlphaAt(3, 1).A != 255 || selection.AlphaAt(2, 1).A != 0 {
		t.Fatal("selection transform moved unrelated pixels or filled holes")
	}
	if !bytes.Equal(before, src.Pix) {
		t.Fatal("transform modified the history image")
	}
}

func TestRotationDoesNotSilentlyCropPixels(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 6, 2))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if _, _, err := TransformSelection(src, src.Rect, nil, RotateClockwise); err == nil {
		t.Fatal("accepted a rotation that crops the canvas")
	}
	square := image.NewNRGBA(image.Rect(0, 0, 6, 6))
	square.SetNRGBA(1, 2, color.NRGBA{R: 255, A: 255})
	dst, mask, err := TransformSelection(square, image.Rect(1, 2, 5, 4), nil, RotateClockwise)
	if err != nil || dst.NRGBAAt(3, 1).R != 255 || SelectionBounds(mask) != image.Rect(2, 1, 4, 5) {
		t.Fatal("incorrect rectangular rotation", err)
	}
}

func TestPasteHonorsTransparencySelectionAndAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	clip := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	clip.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	clip.SetNRGBA(2, 0, color.NRGBA{G: 255, A: 255})
	clip.SetNRGBA(1, 1, color.NRGBA{G: 255, A: 255})
	mask := image.NewAlpha(src.Rect)
	mask.SetAlpha(1, 1, color.Alpha{A: 255})
	mask.SetAlpha(2, 1, color.Alpha{A: 255})
	dst, selection := PasteImage(src, clip, image.Pt(1, 1), image.Rect(1, 1, 3, 3), mask)
	if dst.NRGBAAt(1, 1) != (color.NRGBA{R: 128, B: 127, A: 255}) || dst.NRGBAAt(2, 1) != src.NRGBAAt(2, 1) || dst.NRGBAAt(3, 1) != src.NRGBAAt(3, 1) || dst.NRGBAAt(2, 2) != src.NRGBAAt(2, 2) || SelectionBounds(selection) != image.Rect(1, 1, 2, 2) {
		t.Fatal("paste erased transparent pixels or escaped selection")
	}
}

func TestMaskedFillAndMove(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 5, 5))
	ink := color.NRGBA{R: 255, A: 128}
	src.SetNRGBA(1, 1, ink)
	src.SetNRGBA(2, 1, ink)
	src.SetNRGBA(1, 2, ink)
	src.SetNRGBA(4, 4, ink)
	mask := Wand(src, image.Pt(1, 1), 0)
	if mask.AlphaAt(4, 4).A != 0 || mask.AlphaAt(1, 2).A != 255 {
		t.Fatal("wand crossed a disconnected region")
	}
	filled := FillSelected(src, image.Pt(1, 1), color.NRGBA{G: 255, A: 255}, SelectionBounds(mask), mask)
	if filled.NRGBAAt(4, 4) != ink || filled.NRGBAAt(1, 2).G != 255 {
		t.Fatal("masked fill escaped selection")
	}
	moved, selected := MoveSelection(src, SelectionBounds(mask), mask, image.Pt(1, 1), false)
	if moved.NRGBAAt(1, 1).A != 0 || moved.NRGBAAt(2, 2) != ink || moved.NRGBAAt(4, 4) != ink || selected.AlphaAt(3, 2).A != 255 {
		t.Fatal("move did not preserve holes, alpha and unrelated pixels")
	}
	if src.NRGBAAt(1, 1) != ink {
		t.Fatal("move changed source history")
	}
}
func TestLassoAndPixelPerfect(t *testing.T) {
	mask := Lasso(image.Rect(0, 0, 8, 8), []image.Point{{1, 1}, {5, 1}, {5, 5}, {1, 5}})
	if SelectionBounds(mask) != image.Rect(1, 1, 5, 5) {
		t.Fatal("incorrect polygon rasterization")
	}
	path := []image.Point{{1, 1}, {2, 1}, {2, 2}, {3, 2}, {3, 3}}
	if got := PixelPerfect(path); !reflect.DeepEqual(got, []image.Point{{1, 1}, {2, 2}, {3, 3}}) {
		t.Fatal("pixel-perfect corner cleanup", got)
	}
}
