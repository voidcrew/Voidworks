package dmi

import (
	"fmt"
	"image"
	"image/color"
)

// TransformSelection moves only selected pixels, including transparent pixels.
// Holes in an irregular selection do not erase unrelated pixels. A rotation
// that would lose selected pixels is rejected before modifying the document.
func TransformSelection(src *image.NRGBA, rect image.Rectangle, mask *image.Alpha, kind Transform) (*image.NRGBA, *image.Alpha, error) {
	if rect.Empty() {
		rect = src.Rect
	}
	rect = rect.Intersect(src.Rect)
	dst := CopyImage(src)
	next := image.NewAlpha(src.Rect)
	if rect.Empty() {
		return dst, next, nil
	}
	w, h := rect.Dx(), rect.Dy()
	mapPoint := func(p image.Point) image.Point {
		x, y := p.X-rect.Min.X, p.Y-rect.Min.Y
		switch kind {
		case FlipHorizontal:
			x = w - 1 - x
		case FlipVertical:
			y = h - 1 - y
		case RotateClockwise:
			x, y = h-1-y+(w-h)/2, x+(h-w)/2
		case ShiftLeft:
			x = (x + w - 1) % w
		case ShiftRight:
			x = (x + 1) % w
		case ShiftUp:
			y = (y + h - 1) % h
		case ShiftDown:
			y = (y + 1) % h
		}
		return rect.Min.Add(image.Pt(x, y))
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			p := image.Pt(x, y)
			if !Selected(rect, mask, p) {
				continue
			}
			if !mapPoint(p).In(src.Rect) {
				return nil, nil, fmt.Errorf("rotation would move selected pixels outside the canvas; move the selection inward or enlarge the canvas first")
			}
			dst.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			p := image.Pt(x, y)
			if !Selected(rect, mask, p) {
				continue
			}
			q := mapPoint(p)
			dst.SetNRGBA(q.X, q.Y, src.NRGBAAt(x, y))
			next.SetAlpha(q.X, q.Y, color.Alpha{A: 255})
		}
	}
	return dst, next, nil
}

func Selected(rect image.Rectangle, mask *image.Alpha, p image.Point) bool {
	if !rect.Empty() && !p.In(rect) {
		return false
	}
	return mask == nil || mask.AlphaAt(p.X, p.Y).A != 0
}

func SelectionBounds(mask *image.Alpha) image.Rectangle {
	var bounds image.Rectangle
	for y := mask.Rect.Min.Y; y < mask.Rect.Max.Y; y++ {
		for x := mask.Rect.Min.X; x < mask.Rect.Max.X; x++ {
			if mask.AlphaAt(x, y).A != 0 {
				bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return bounds
}

// PasteImage composites clipboard pixels within the current selection. Clear
// clipboard pixels leave existing artwork intact, including hidden RGB values.
func PasteImage(src, clipboard *image.NRGBA, origin image.Point, rect image.Rectangle, mask *image.Alpha) (*image.NRGBA, *image.Alpha) {
	dst := CopyImage(src)
	selected := image.NewAlpha(src.Rect)
	for y := 0; y < clipboard.Rect.Dy(); y++ {
		for x := 0; x < clipboard.Rect.Dx(); x++ {
			p := origin.Add(image.Pt(x, y))
			if !p.In(src.Rect) || !Selected(rect, mask, p) {
				continue
			}
			front := clipboard.NRGBAAt(x+clipboard.Rect.Min.X, y+clipboard.Rect.Min.Y)
			if front.A == 0 {
				continue
			}
			dst.SetNRGBA(p.X, p.Y, over(front, src.NRGBAAt(p.X, p.Y)))
			selected.SetAlpha(p.X, p.Y, color.Alpha{A: 255})
		}
	}
	return dst, selected
}

func over(front, back color.NRGBA) color.NRGBA {
	alpha := uint32(front.A)
	if alpha == 0 {
		return back
	}
	out := alpha*255 + uint32(back.A)*(255-alpha)
	mix := func(f, b uint8) uint8 {
		return uint8((uint32(f)*alpha*255 + uint32(b)*uint32(back.A)*(255-alpha) + out/2) / out)
	}
	return color.NRGBA{R: mix(front.R, back.R), G: mix(front.G, back.G), B: mix(front.B, back.B), A: uint8((out + 127) / 255)}
}

func Wand(src *image.NRGBA, p image.Point, tolerance uint8) *image.Alpha {
	mask := image.NewAlpha(src.Rect)
	if !p.In(src.Rect) {
		return mask
	}
	target := src.NRGBAAt(p.X, p.Y)
	match := func(p image.Point) bool {
		c := src.NRGBAAt(p.X, p.Y)
		if c.A == 0 && target.A == 0 {
			return true
		}
		return max(abs(int(c.R)-int(target.R)), abs(int(c.G)-int(target.G)), abs(int(c.B)-int(target.B)), abs(int(c.A)-int(target.A))) <= int(tolerance)
	}
	stack := []image.Point{p}
	mask.SetAlpha(p.X, p.Y, color.Alpha{A: 255})
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, q := range []image.Point{{p.X - 1, p.Y}, {p.X + 1, p.Y}, {p.X, p.Y - 1}, {p.X, p.Y + 1}} {
			if q.In(src.Rect) && mask.AlphaAt(q.X, q.Y).A == 0 && match(q) {
				mask.SetAlpha(q.X, q.Y, color.Alpha{A: 255})
				stack = append(stack, q)
			}
		}
	}
	return mask
}

func Lasso(bounds image.Rectangle, points []image.Point) *image.Alpha {
	mask := image.NewAlpha(bounds)
	if len(points) < 3 {
		return mask
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			inside := false
			j := len(points) - 1
			for n, a := range points {
				b := points[j]
				if (float64(a.Y) > .5+float64(y)) != (float64(b.Y) > .5+float64(y)) && float64(x)+.5 < float64(b.X-a.X)*(float64(y)+.5-float64(a.Y))/float64(b.Y-a.Y)+float64(a.X) {
					inside = !inside
				}
				j = n
			}
			if inside {
				mask.SetAlpha(x, y, color.Alpha{A: 255})
			}
		}
	}
	return mask
}

func MoveSelection(src *image.NRGBA, rect image.Rectangle, mask *image.Alpha, delta image.Point, copyPixels bool) (*image.NRGBA, *image.Alpha) {
	dst := CopyImage(src)
	selection := image.NewAlpha(src.Rect)
	if rect.Empty() {
		rect = src.Rect
	}
	if !copyPixels {
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				if image.Pt(x, y).In(src.Rect) && Selected(rect, mask, image.Pt(x, y)) {
					dst.SetNRGBA(x, y, color.NRGBA{})
				}
			}
		}
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			p := image.Pt(x, y)
			q := p.Add(delta)
			if p.In(src.Rect) && q.In(src.Rect) && Selected(rect, mask, p) {
				dst.SetNRGBA(q.X, q.Y, src.NRGBAAt(x, y))
				selection.SetAlpha(q.X, q.Y, color.Alpha{A: 255})
			}
		}
	}
	return dst, selection
}

func FillSelected(src *image.NRGBA, p image.Point, c color.NRGBA, rect image.Rectangle, mask *image.Alpha) *image.NRGBA {
	dst := CopyImage(src)
	if !p.In(src.Rect) || !Selected(rect, mask, p) {
		return dst
	}
	target := src.NRGBAAt(p.X, p.Y)
	if target == c {
		return dst
	}
	queue := []image.Point{p}
	dst.SetNRGBA(p.X, p.Y, c)
	for len(queue) > 0 {
		p := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, q := range []image.Point{{p.X - 1, p.Y}, {p.X + 1, p.Y}, {p.X, p.Y - 1}, {p.X, p.Y + 1}} {
			if q.In(src.Rect) && Selected(rect, mask, q) && dst.NRGBAAt(q.X, q.Y) == target {
				dst.SetNRGBA(q.X, q.Y, c)
				queue = append(queue, q)
			}
		}
	}
	return dst
}

// PixelPerfect removes the middle point of a one-pixel right-angle corner.
// Points are consecutive samples along the stroke, not the whole image.
func PixelPerfect(points []image.Point) []image.Point {
	result := make([]image.Point, 0, len(points))
	for _, p := range points {
		if len(result) > 0 && p == result[len(result)-1] {
			continue
		}
		if len(result) > 1 {
			a, b := result[len(result)-2], result[len(result)-1]
			if abs(a.X-p.X) == 1 && abs(a.Y-p.Y) == 1 && abs(a.X-b.X)+abs(a.Y-b.Y) == 1 && abs(p.X-b.X)+abs(p.Y-b.Y) == 1 {
				result = result[:len(result)-1]
			}
		}
		result = append(result, p)
	}
	return result
}
