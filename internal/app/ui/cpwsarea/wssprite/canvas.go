package wssprite

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/dmi"
)

var toolNames = []string{"Pencil", "Eraser", "Fill", "Pick color", "Select rectangle", "Line", "Rectangle", "Ellipse", "Magic wand", "Lasso", "Move selection"}

func packed(r, g, b, a float32) imgui.PackedColor {
	return imgui.PackedColorFromVec4(imgui.Vec4{X: r, Y: g, Z: b, W: a})
}
func (w *Workspace) drawingTools() {
	imgui.SetNextItemWidth(120)
	if imgui.BeginCombo("Tool", toolNames[w.tool]) {
		for n, name := range toolNames {
			if imgui.Selectable(name) {
				w.finishStroke()
				w.tool = int32(n)
			}
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	imgui.SetNextItemWidth(180)
	imgui.ColorEdit4("Color", &w.color)
	imgui.SameLine()
	if imgui.Button("Fit") {
		w.zoom = 0
		w.pan = imgui.Vec2{}
	}
	imgui.Checkbox("Grid", &w.grid)
	imgui.SameLine()
	imgui.Checkbox("Tile", &w.tiled)
	imgui.SameLine()
	imgui.Checkbox("Mirror X", &w.symX)
	imgui.SameLine()
	imgui.Checkbox("Mirror Y", &w.symY)
	imgui.SameLine()
	imgui.Checkbox("Pixel perfect", &w.pixelPerfect)
	imgui.SetNextItemWidth(90)
	imgui.SliderInt("Brush size", &w.brushSize, 1, 16)
	if w.tool == 8 {
		imgui.SameLine()
		imgui.SetNextItemWidth(100)
		imgui.SliderInt("Tolerance", &w.tolerance, 0, 255)
	}
	if imgui.Button("Pixels...") {
		imgui.OpenPopup("pixel-actions")
	}
	if imgui.BeginPopup("pixel-actions") {
		if imgui.MenuItem("Copy pixels") {
			w.Copy()
		}
		if imgui.MenuItem("Paste pixels") {
			w.Paste()
		}
		if imgui.MenuItem("Cut pixels") {
			w.Cut()
		}
		if imgui.MenuItem("Clear pixels") {
			w.Delete()
		}
		if imgui.MenuItem("Deselect") {
			w.Deselect()
		}
		imgui.Separator()
		for n, name := range []string{"Flip horizontally", "Flip vertically", "Rotate 90 degrees", "Shift left (wrap)", "Shift right (wrap)", "Shift up (wrap)", "Shift down (wrap)"} {
			if imgui.MenuItem(name) {
				w.transform(name, dmi.Transform(n))
			}
		}
		imgui.EndPopup()
	}
	imgui.SameLine()
	imgui.Checkbox("Apply transforms to all selected states/frames", &w.allCels)
}
func (w *Workspace) transform(label string, kind dmi.Transform) {
	selected := !w.selection.Empty() || w.selectionMask != nil
	var nextMask *image.Alpha
	w.change(label, func(i *dmi.Icon) error {
		indices := []int{w.state}
		if w.allCels {
			indices = w.indices()
		}
		for _, state := range indices {
			for n := range i.States[state].Cels {
				if w.allCels || w.cells[n] {
					s := i.States[state]
					pixels, mask, err := dmi.TransformSelection(s.RawCel(w.layer, n), w.selection, w.selectionMask, kind)
					if err != nil {
						return err
					}
					nextMask = mask
					if err := s.SetCel(w.layer, n, pixels); err != nil {
						return err
					}
				}
			}
		}
		if selected && nextMask != nil {
			w.selectionMask = nextMask
			w.selection = dmi.SelectionBounds(nextMask)
		}
		return nil
	})
}
func (w *Workspace) canvas() {
	if w.rendered == nil {
		return
	}
	available := imgui.ContentRegionAvail()
	if available.X < 1 || available.Y < 1 {
		return
	}
	imgui.InvisibleButton("pixel-canvas", available)
	viewMin, viewMax := imgui.ItemRectMin(), imgui.ItemRectMax()
	hovered := imgui.IsItemHovered()
	active := imgui.IsItemActive()
	draw := imgui.WindowDrawList()
	draw.PushClipRectV(viewMin, viewMax, true)
	defer draw.PopClipRect()
	for y := viewMin.Y; y < viewMax.Y; y += 16 {
		for x := viewMin.X; x < viewMax.X; x += 16 {
			v := float32(.15)
			if (int((x-viewMin.X)/16)+int((y-viewMin.Y)/16))%2 == 0 {
				v = .2
			}
			draw.AddRectFilled(imgui.Vec2{X: x, Y: y}, imgui.Vec2{X: x + 16, Y: y + 16}, packed(v, v, v, 1))
		}
	}
	i := w.Document.Icon
	if w.zoom <= 0 {
		w.zoom = max(float32(1), float32(math.Floor(float64(minf(available.X/float32(i.Width), available.Y/float32(i.Height))*.8))))
	}
	mouse := imgui.MousePos()
	center := viewMin.Plus(imgui.Vec2{X: available.X / 2, Y: available.Y / 2})
	if hovered {
		_, wheel := imgui.CurrentIO().MouseWheel()
		if wheel != 0 {
			old := w.zoom
			w.zoom = max(float32(.125), minf(64, w.zoom*float32(math.Pow(1.25, float64(wheel)))))
			relative := mouse.Minus(center).Minus(w.pan)
			w.pan = w.pan.Plus(relative).Minus(imgui.Vec2{X: relative.X * w.zoom / old, Y: relative.Y * w.zoom / old})
		}
		if imgui.IsMouseDown(imgui.MouseButtonMiddle) || (imgui.IsKeyDown(int(glfw.KeySpace)) && imgui.IsMouseDown(imgui.MouseButtonLeft)) {
			w.pan = w.pan.Plus(imgui.CurrentIO().MouseDelta())
		}
	}
	size := imgui.Vec2{X: float32(i.Width) * w.zoom, Y: float32(i.Height) * w.zoom}
	origin := center.Plus(w.pan).Minus(imgui.Vec2{X: size.X / 2, Y: size.Y / 2})
	w.canvasOrigin = origin
	state, cel := w.state, w.cel
	valid := state < len(w.published.States) && cel < len(w.published.States[state].Cels)
	if valid {
		s := w.published.States[state]
		if w.playing {
			cel = s.FrameAt(w.playbackTime(time.Now()))*s.Dirs() + cel%s.Dirs()
		}
		a, b := w.uv(state, cel)
		repeat := 0
		if w.tiled {
			repeat = 1
		}
		for y := -repeat; y <= repeat; y++ {
			for x := -repeat; x <= repeat; x++ {
				pos := origin.Plus(imgui.Vec2{X: float32(x) * size.X, Y: float32(y) * size.Y})
				draw.AddImageV(imgui.TextureID(w.rendered.Texture), pos, pos.Plus(size), a, b, packed(1, 1, 1, 1))
			}
		}
		if w.onion && !w.playing {
			for _, offset := range []int{-s.Dirs(), s.Dirs()} {
				n := cel + offset
				if n >= 0 && n < len(s.Cels) {
					a, b := w.uv(state, n)
					tint := packed(1, .25, .3, .25)
					if offset > 0 {
						tint = packed(.2, .6, 1, .25)
					}
					draw.AddImageV(imgui.TextureID(w.rendered.Texture), origin, origin.Plus(size), a, b, tint)
				}
			}
		}
	}
	draw.AddRect(origin, origin.Plus(size), packed(.5, .7, .9, 1))
	if w.grid && w.zoom >= 6 {
		for x := max(0, int((viewMin.X-origin.X)/w.zoom)); x <= minInt(i.Width, int((viewMax.X-origin.X)/w.zoom)+1); x++ {
			px := origin.X + float32(x)*w.zoom
			draw.AddLine(imgui.Vec2{X: px, Y: maxf(viewMin.Y, origin.Y)}, imgui.Vec2{X: px, Y: minf(viewMax.Y, origin.Y+size.Y)}, packed(.4, .45, .5, .35))
		}
		for y := max(0, int((viewMin.Y-origin.Y)/w.zoom)); y <= minInt(i.Height, int((viewMax.Y-origin.Y)/w.zoom)+1); y++ {
			py := origin.Y + float32(y)*w.zoom
			draw.AddLine(imgui.Vec2{X: maxf(viewMin.X, origin.X), Y: py}, imgui.Vec2{X: minf(viewMax.X, origin.X+size.X), Y: py}, packed(.4, .45, .5, .35))
		}
	}
	if !w.selection.Empty() {
		r := w.selection
		draw.AddRect(origin.Plus(imgui.Vec2{X: float32(r.Min.X) * w.zoom, Y: float32(r.Min.Y) * w.zoom}), origin.Plus(imgui.Vec2{X: float32(r.Max.X) * w.zoom, Y: float32(r.Max.Y) * w.zoom}), packed(1, .9, .3, 1))
	}
	if w.selectionMask != nil {
		for y := max(0, int((viewMin.Y-origin.Y)/w.zoom)); y < min(i.Height, int((viewMax.Y-origin.Y)/w.zoom)+1); y++ {
			for x := max(0, int((viewMin.X-origin.X)/w.zoom)); x < min(i.Width, int((viewMax.X-origin.X)/w.zoom)+1); x++ {
				if w.selectionMask.AlphaAt(x, y).A != 0 {
					p := origin.Plus(imgui.Vec2{X: float32(x) * w.zoom, Y: float32(y) * w.zoom})
					draw.AddRectFilled(p, p.Plus(imgui.Vec2{X: w.zoom, Y: w.zoom}), packed(.2, .7, 1, .15))
				}
			}
		}
	}
	for n := 1; n < len(w.lasso); n++ {
		a, b := w.lasso[n-1], w.lasso[n]
		draw.AddLine(origin.Plus(imgui.Vec2{X: float32(a.X) * w.zoom, Y: float32(a.Y) * w.zoom}), origin.Plus(imgui.Vec2{X: float32(b.X) * w.zoom, Y: float32(b.Y) * w.zoom}), packed(1, .9, .3, 1))
	}
	point := image.Pt(int(math.Floor(float64((mouse.X-origin.X)/w.zoom))), int(math.Floor(float64((mouse.Y-origin.Y)/w.zoom))))
	rawPoint := point
	if w.tiled {
		point.X = ((point.X % i.Width) + i.Width) % i.Width
		point.Y = ((point.Y % i.Height) + i.Height) % i.Height
	}
	inBounds := point.In(image.Rect(0, 0, i.Width, i.Height))
	if w.tiled {
		inBounds = rawPoint.In(image.Rect(-i.Width, -i.Height, 2*i.Width, 2*i.Height))
	}
	if hovered && inBounds {
		p := origin.Plus(imgui.Vec2{X: float32(point.X) * w.zoom, Y: float32(point.Y) * w.zoom})
		draw.AddRect(p, p.Plus(imgui.Vec2{X: w.zoom, Y: w.zoom}), packed(1, 1, 1, .8))
	}
	if w.before || w.playing || imgui.IsKeyDown(int(glfw.KeySpace)) {
		return
	}
	if hovered && inBounds && imgui.IsMouseClicked(imgui.MouseButtonLeft) {
		w.start, w.last = point, point
		switch w.tool {
		case 3:
			c := i.States[w.state].Cels[w.cel].NRGBAAt(point.X, point.Y)
			w.color = [4]float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255, float32(c.A) / 255}
		case 4:
			w.selection = image.Rectangle{}
			w.selectionMask = nil
		case 8:
			w.selectionMask = dmi.Wand(i.States[w.state].RawCel(w.layer, w.cel), point, uint8(w.tolerance))
			w.selection = dmi.SelectionBounds(w.selectionMask)
		case 9:
			w.lasso = []image.Point{point}
		case 2:
			w.change("Fill pixels", func(next *dmi.Icon) error {
				src := next.States[w.state].RawCel(w.layer, w.cel)
				bounds := w.selection
				if bounds.Empty() {
					bounds = src.Rect
				}
				return next.States[w.state].SetCel(w.layer, w.cel, dmi.FillSelected(src, point, w.paintColor(), bounds, w.selectionMask))
			})
		default:
			w.stroke = i
			w.strokePoints = nil
			w.strokeSelection, w.strokeMask = w.selection, w.selectionMask
			if w.tiled && w.tool != 10 {
				w.start, w.last = rawPoint, rawPoint
				w.paint(rawPoint)
			} else {
				w.paint(point)
			}
		}
	}
	if active && imgui.IsMouseDown(imgui.MouseButtonLeft) {
		point.X = max(0, minInt(i.Width-1, point.X))
		point.Y = max(0, minInt(i.Height-1, point.Y))
		if w.tool == 4 {
			w.selection = image.Rect(minInt(w.start.X, point.X), minInt(w.start.Y, point.Y), max(w.start.X, point.X)+1, max(w.start.Y, point.Y)+1)
		} else if w.tool == 9 && len(w.lasso) > 0 && point != w.lasso[len(w.lasso)-1] {
			w.lasso = append(w.lasso, point)
		} else if w.stroke != nil {
			if w.tiled && w.tool != 10 {
				point = image.Pt(max(-i.Width, min(2*i.Width-1, rawPoint.X)), max(-i.Height, min(2*i.Height-1, rawPoint.Y)))
			}
			if point != w.last {
				w.paint(point)
			}
		}
	}
}
func (w *Workspace) paintColor() color.NRGBA {
	return color.NRGBA{uint8(w.color[0]*255 + .5), uint8(w.color[1]*255 + .5), uint8(w.color[2]*255 + .5), uint8(w.color[3]*255 + .5)}
}
func (w *Workspace) paint(point image.Point) {
	if len(w.Document.Icon.ConversionIssues()) > 0 {
		w.message = "Choose Prepare for editing before changing this image."
		return
	}
	base := w.Document.Icon
	if w.tool >= 5 {
		base = w.stroke
	}
	if w.pixelPerfect && w.tool == 0 && w.brushSize == 1 {
		base = w.stroke
	}
	next := base.Clone()
	next.Changed = true
	src := base.States[w.state].RawCel(w.layer, w.cel)
	dst := dmi.CopyImage(src)
	if w.tool == 10 {
		var mask *image.Alpha
		dst, mask = dmi.MoveSelection(src, w.strokeSelection, w.strokeMask, point.Sub(w.start), false)
		if err := next.States[w.state].SetCel(w.layer, w.cel, dst); err != nil {
			w.message = err.Error()
			return
		}
		w.selectionMask = mask
		w.selection = dmi.SelectionBounds(mask)
		w.Document.Restore(next)
		w.last = point
		w.publish()
		return
	}
	ink := w.paintColor()
	if w.tool == 1 {
		ink = color.NRGBA{}
	}
	bounds := w.selection
	if bounds.Empty() {
		bounds = src.Rect
	}
	plotPixel := func(x, y int) {
		if w.tiled {
			x = ((x % src.Rect.Dx()) + src.Rect.Dx()) % src.Rect.Dx()
			y = ((y % src.Rect.Dy()) + src.Rect.Dy()) % src.Rect.Dy()
		}
		points := []image.Point{{X: x, Y: y}}
		if w.symX {
			points = append(points, image.Pt(src.Rect.Dx()-1-x, y))
		}
		if w.symY {
			points = append(points, image.Pt(x, src.Rect.Dy()-1-y))
		}
		if w.symX && w.symY {
			points = append(points, image.Pt(src.Rect.Dx()-1-x, src.Rect.Dy()-1-y))
		}
		for _, p := range points {
			if p.In(src.Rect) && dmi.Selected(bounds, w.selectionMask, p) {
				dst.SetNRGBA(p.X, p.Y, ink)
			}
		}
	}
	plot := func(x, y int) {
		size := max(1, int(w.brushSize))
		for dy := 0; dy < size; dy++ {
			for dx := 0; dx < size; dx++ {
				plotPixel(x+dx-size/2, y+dy-size/2)
			}
		}
	}
	switch w.tool {
	case 5:
		dmi.Line(w.start, point, plot)
	case 6:
		a, b := w.start, point
		dmi.Line(a, image.Pt(b.X, a.Y), plot)
		dmi.Line(image.Pt(b.X, a.Y), b, plot)
		dmi.Line(b, image.Pt(a.X, b.Y), plot)
		dmi.Line(image.Pt(a.X, b.Y), a, plot)
	case 7:
		rx, ry := math.Abs(float64(point.X-w.start.X))/2, math.Abs(float64(point.Y-w.start.Y))/2
		cx, cy := float64(point.X+w.start.X)/2, float64(point.Y+w.start.Y)/2
		steps := max(16, int(2*math.Pi*math.Max(rx, ry)*2))
		for n := 0; n <= steps; n++ {
			angle := float64(n) * 2 * math.Pi / float64(steps)
			plot(int(math.Round(cx+rx*math.Cos(angle))), int(math.Round(cy+ry*math.Sin(angle))))
		}
	default:
		if w.pixelPerfect && w.tool == 0 && w.brushSize == 1 {
			dmi.Line(w.last, point, func(x, y int) { w.strokePoints = append(w.strokePoints, image.Pt(x, y)) })
			for _, p := range dmi.PixelPerfect(w.strokePoints) {
				plot(p.X, p.Y)
			}
		} else {
			dmi.Line(w.last, point, plot)
		}
	}
	if err := next.States[w.state].SetCel(w.layer, w.cel, dst); err != nil {
		w.message = err.Error()
		return
	}
	w.Document.Restore(next)
	w.last = point
	w.publish()
}
func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
