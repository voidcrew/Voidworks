package dmi

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strconv"
	"strings"
)

func (i *Icon) UniqueName(base string) string {
	used := map[string]bool{}
	for _, s := range i.States {
		used[s.Name] = true
	}
	if !used[base] {
		return base
	}
	for n := 2; ; n++ {
		v := fmt.Sprintf("%s_%d", base, n)
		if !used[v] {
			return v
		}
	}
}
func (i *Icon) AddState(name string) int {
	i.States = append(i.States, NewState(i.UniqueName(name), i.Width, i.Height))
	i.Changed = true
	return len(i.States) - 1
}
func (i *Icon) DeleteStates(indices []int) error {
	selected := map[int]bool{}
	for _, n := range indices {
		if n < 0 || n >= len(i.States) {
			return fmt.Errorf("invalid state selection")
		}
		selected[n] = true
	}
	states := make([]*State, 0, len(i.States))
	for n, s := range i.States {
		if !selected[n] {
			states = append(states, s)
		}
	}
	i.States = states
	i.Changed = true
	return nil
}
func (i *Icon) DuplicateStates(indices []int) error {
	for _, n := range indices {
		if n < 0 || n >= len(i.States) {
			return fmt.Errorf("invalid state selection")
		}
	}
	names := map[string]string{}
	for _, n := range indices {
		s := i.States[n].Clone()
		name, exists := names[s.Name]
		if !exists {
			name = i.UniqueName(s.Name)
			names[s.Name] = name
		}
		s.Name = name
		i.States = append(i.States, s)
	}
	i.Changed = true
	return i.Validate()
}
func (i *Icon) MoveState(from, to int) error {
	if from < 0 || to < 0 || from >= len(i.States) || to >= len(i.States) {
		return fmt.Errorf("invalid state position")
	}
	s := i.States[from]
	if from < to {
		copy(i.States[from:to], i.States[from+1:to+1])
	} else {
		copy(i.States[to+1:from+1], i.States[to:from])
	}
	i.States[to] = s
	i.Changed = true
	return nil
}
func (i *Icon) SortStates() {
	sort.SliceStable(i.States, func(a, b int) bool { return strings.ToLower(i.States[a].Name) < strings.ToLower(i.States[b].Name) })
	i.Changed = true
}
func (i *Icon) Rename(indices []int, prefix, find, replacement, suffix string) error {
	for _, n := range indices {
		if n < 0 || n >= len(i.States) {
			return fmt.Errorf("invalid state selection")
		}
		s := i.States[n]
		name := s.Name
		if find != "" {
			name = strings.ReplaceAll(name, find, replacement)
		}
		s.Name = prefix + name + suffix
	}
	i.Changed = true
	return i.Validate()
}
func (i *Icon) Resize(width, height, anchorX, anchorY int, scale bool) error {
	if width < 1 || height < 1 || width > 8192 || height > 8192 || int64(width)*int64(height)*int64(max(1, i.CelCount())) > MaxPixels {
		return fmt.Errorf("new canvas exceeds the editing limit")
	}
	var layeredCels int64
	for _, s := range i.States {
		for _, l := range s.Layers {
			layeredCels += int64(len(l.Cels))
		}
	}
	if int64(width)*int64(height)*layeredCels > maxLayerPixels {
		return fmt.Errorf("resized layers exceed editing limit")
	}
	for _, s := range i.States {
		buffers := [][]*image.NRGBA{s.Cels}
		for n := range s.Layers {
			buffers = append(buffers, s.Layers[n].Cels)
		}
		for _, cels := range buffers {
			for n, src := range cels {
				dst := image.NewNRGBA(image.Rect(0, 0, width, height))
				dx, dy := (width-i.Width)*anchorX/2, (height-i.Height)*anchorY/2
				for y := 0; y < height; y++ {
					for x := 0; x < width; x++ {
						sx, sy := x-dx, y-dy
						if scale {
							sx = x * i.Width / width
							sy = y * i.Height / height
						}
						if image.Pt(sx, sy).In(src.Bounds()) {
							dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
						}
					}
				}
				cels[n] = dst
			}
		}
		s.RecomposeAll()
	}
	i.Width, i.Height = width, height
	i.Changed = true
	return nil
}
func (i *Icon) SetDirections(state, dirs int) error {
	if state < 0 || state >= len(i.States) || (dirs != 1 && dirs != 4 && dirs != 8) {
		return fmt.Errorf("choose 1, 4 or 8 directions")
	}
	s := i.States[state]
	old := s.Dirs()
	if err := i.checkLayerGrowth(int64(s.Frames() * (dirs - old) * len(s.Layers))); err != nil {
		return err
	}
	if int64(i.CelCount()+s.Frames()*(dirs-old))*int64(i.Width)*int64(i.Height) > MaxPixels {
		return fmt.Errorf("too many images")
	}
	cels := make([]*image.NRGBA, 0, dirs*s.Frames())
	for f := 0; f < s.Frames(); f++ {
		for d := 0; d < dirs; d++ {
			source := d
			if source >= old {
				source = 0
			}
			cels = append(cels, s.Cels[f*old+source])
		}
	}
	s.Cels = cels
	for n := range s.Layers {
		original := s.Layers[n].Cels
		var mapped []*image.NRGBA
		for f := 0; f < s.Frames(); f++ {
			for d := 0; d < dirs; d++ {
				source := d
				if source >= old {
					source = 0
				}
				mapped = append(mapped, original[f*old+source])
			}
		}
		s.Layers[n].Cels = mapped
	}
	s.Set("dirs", strconv.Itoa(dirs))
	i.Changed = true
	return nil
}
func (i *Icon) InsertFrame(state, after int, duplicate bool) error {
	if state < 0 || state >= len(i.States) {
		return fmt.Errorf("invalid state")
	}
	s := i.States[state]
	if after < 0 || after >= s.Frames() {
		return fmt.Errorf("invalid frame")
	}
	if s.Frames() >= 65536 {
		return fmt.Errorf("too many frames")
	}
	if err := i.checkLayerGrowth(int64(s.Dirs() * len(s.Layers))); err != nil {
		return err
	}
	if int64(i.CelCount()+s.Dirs())*int64(i.Width)*int64(i.Height) > MaxPixels {
		return fmt.Errorf("too many images")
	}
	pos := (after + 1) * s.Dirs()
	added := make([]*image.NRGBA, s.Dirs())
	for d := range added {
		if duplicate {
			added[d] = s.Cels[after*s.Dirs()+d]
		} else {
			added[d] = image.NewNRGBA(image.Rect(0, 0, i.Width, i.Height))
		}
	}
	cels := append([]*image.NRGBA(nil), s.Cels[:pos]...)
	cels = append(cels, added...)
	s.Cels = append(cels, s.Cels[pos:]...)
	for n := range s.Layers {
		original := s.Layers[n].Cels
		mapped := append([]*image.NRGBA(nil), original[:pos]...)
		for d := 0; d < s.Dirs(); d++ {
			var cel *image.NRGBA
			if duplicate {
				cel = original[after*s.Dirs()+d]
			} else {
				cel = image.NewNRGBA(image.Rect(0, 0, i.Width, i.Height))
			}
			mapped = append(mapped, cel)
		}
		s.Layers[n].Cels = append(mapped, original[pos:]...)
	}
	delays := s.Delays()
	delays = append(delays[:after+1], append([]float64{delays[after]}, delays[after+1:]...)...)
	s.Set("frames", strconv.Itoa(s.Frames()+1))
	s.SetDelays(delays)
	i.Changed = true
	return nil
}
func (i *Icon) DeleteFrame(state, frame int) error {
	if state < 0 || state >= len(i.States) {
		return fmt.Errorf("invalid state")
	}
	s := i.States[state]
	if frame < 0 || frame >= s.Frames() || s.Frames() == 1 {
		return fmt.Errorf("a state needs at least one frame")
	}
	delays := s.Delays()
	delays = append(delays[:frame], delays[frame+1:]...)
	s.Cels = append(s.Cels[:frame*s.Dirs()], s.Cels[(frame+1)*s.Dirs():]...)
	for n := range s.Layers {
		c := s.Layers[n].Cels
		s.Layers[n].Cels = append(c[:frame*s.Dirs()], c[(frame+1)*s.Dirs():]...)
	}
	s.Set("frames", strconv.Itoa(s.Frames()-1))
	s.SetDelays(delays)
	i.Changed = true
	return nil
}
func (i *Icon) MoveFrame(state, from, to int) error {
	if state < 0 || state >= len(i.States) {
		return fmt.Errorf("invalid state")
	}
	s := i.States[state]
	if from < 0 || to < 0 || from >= s.Frames() || to >= s.Frames() {
		return fmt.Errorf("invalid frame")
	}
	order := make([]int, s.Frames())
	for n := range order {
		order[n] = n
	}
	v := order[from]
	if from < to {
		copy(order[from:to], order[from+1:to+1])
	} else {
		copy(order[to+1:from+1], order[to:from])
	}
	order[to] = v
	delays := s.Delays()
	nextDelays := make([]float64, len(delays))
	cels := make([]*image.NRGBA, 0, len(s.Cels))
	for n, f := range order {
		cels = append(cels, s.Cels[f*s.Dirs():(f+1)*s.Dirs()]...)
		nextDelays[n] = delays[f]
	}
	s.Cels = cels
	for n := range s.Layers {
		original := s.Layers[n].Cels
		var mapped []*image.NRGBA
		for _, f := range order {
			mapped = append(mapped, original[f*s.Dirs():(f+1)*s.Dirs()]...)
		}
		s.Layers[n].Cels = mapped
	}
	s.SetDelays(nextDelays)
	i.Changed = true
	return nil
}
func (i *Icon) SplitState(state int, byDirection bool) error {
	if state < 0 || state >= len(i.States) {
		return fmt.Errorf("invalid state")
	}
	s := i.States[state]
	var added []*State
	names := []string{"south", "north", "east", "west", "southeast", "southwest", "northeast", "northwest"}
	if byDirection {
		for d := 0; d < s.Dirs(); d++ {
			next := s.Clone()
			next.Name = i.UniqueName(s.Name + "_" + names[d])
			next.Set("dirs", "1")
			next.Cels = nil
			for n := range next.Layers {
				next.Layers[n].Cels = nil
			}
			for f := 0; f < s.Frames(); f++ {
				next.Cels = append(next.Cels, s.Cels[f*s.Dirs()+d])
				for n := range next.Layers {
					next.Layers[n].Cels = append(next.Layers[n].Cels, s.Layers[n].Cels[f*s.Dirs()+d])
				}
			}
			added = append(added, next)
		}
	} else {
		delays := s.Delays()
		for f := 0; f < s.Frames(); f++ {
			next := s.Clone()
			next.Name = i.UniqueName(fmt.Sprintf("%s_%d", s.Name, f+1))
			next.Set("frames", "1")
			next.SetDelays([]float64{delays[f]})
			next.Cels = append([]*image.NRGBA(nil), s.Cels[f*s.Dirs():(f+1)*s.Dirs()]...)
			for n := range next.Layers {
				next.Layers[n].Cels = append([]*image.NRGBA(nil), s.Layers[n].Cels[f*s.Dirs():(f+1)*s.Dirs()]...)
			}
			added = append(added, next)
		}
	}
	i.States = append(i.States, added...)
	i.Changed = true
	return i.Validate()
}

// CombineStates retains originals, so information that cannot be combined is not lost.
func (i *Icon) CombineStates(indices []int, byDirection bool) error {
	if len(indices) < 2 {
		return fmt.Errorf("select at least two states")
	}
	for _, n := range indices {
		if n < 0 || n >= len(i.States) {
			return fmt.Errorf("invalid state")
		}
	}
	first := i.States[indices[0]]
	next := first.Clone()
	next.Layers = nil // originals retain their independent layer stacks
	next.Name = i.UniqueName(first.Name + "_combined")
	next.Cels = nil
	if byDirection {
		if len(indices) != 4 && len(indices) != 8 {
			return fmt.Errorf("select 4 or 8 single-direction states in S,N,E,W,SE,SW,NE,NW order")
		}
		for _, n := range indices {
			if i.States[n].Dirs() != 1 || i.States[n].Frames() != first.Frames() {
				return fmt.Errorf("direction states must have matching frame counts and one direction")
			}
		}
		for f := 0; f < first.Frames(); f++ {
			for _, n := range indices {
				next.Cels = append(next.Cels, i.States[n].Cels[f])
			}
		}
		next.Set("dirs", strconv.Itoa(len(indices)))
	} else {
		var delays []float64
		for _, n := range indices {
			s := i.States[n]
			if s.Dirs() != first.Dirs() {
				return fmt.Errorf("states must have the same directions")
			}
			next.Cels = append(next.Cels, s.Cels...)
			delays = append(delays, s.Delays()...)
		}
		next.Set("frames", strconv.Itoa(len(delays)))
		next.SetDelays(delays)
	}
	i.States = append(i.States, next)
	i.Changed = true
	return i.Validate()
}

type Transform int

const (
	FlipHorizontal Transform = iota
	FlipVertical
	RotateClockwise
	ShiftLeft
	ShiftRight
	ShiftUp
	ShiftDown
)

func TransformImage(src *image.NRGBA, kind Transform) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewNRGBA(src.Rect)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx, sy := x, y
			switch kind {
			case FlipHorizontal:
				sx = w - 1 - x
			case FlipVertical:
				sy = h - 1 - y
			case RotateClockwise:
				sx = y + (w-h)/2
				sy = h - 1 - x + (w-h)/2
			case ShiftLeft:
				sx = (x + 1) % w
			case ShiftRight:
				sx = (x + w - 1) % w
			case ShiftUp:
				sy = (y + 1) % h
			case ShiftDown:
				sy = (y + h - 1) % h
			}
			if image.Pt(sx, sy).In(src.Rect) {
				dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
			}
		}
	}
	return dst
}
func ReplaceColor(src *image.NRGBA, from, to color.NRGBA) *image.NRGBA {
	dst := CopyImage(src)
	for y := 0; y < dst.Rect.Dy(); y++ {
		for x := 0; x < dst.Rect.Dx(); x++ {
			if dst.NRGBAAt(x, y) == from {
				dst.SetNRGBA(x, y, to)
			}
		}
	}
	return dst
}
func Fill(src *image.NRGBA, point image.Point, c color.NRGBA, selection image.Rectangle) *image.NRGBA {
	dst := CopyImage(src)
	bounds := src.Rect.Intersect(selection)
	if !point.In(bounds) {
		return dst
	}
	old := src.NRGBAAt(point.X, point.Y)
	if old == c {
		return dst
	}
	stack := []image.Point{point}
	dst.SetNRGBA(point.X, point.Y, c)
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, q := range []image.Point{{p.X - 1, p.Y}, {p.X + 1, p.Y}, {p.X, p.Y - 1}, {p.X, p.Y + 1}} {
			if q.In(bounds) && dst.NRGBAAt(q.X, q.Y) == old {
				dst.SetNRGBA(q.X, q.Y, c)
				stack = append(stack, q)
			}
		}
	}
	return dst
}
func Line(a, b image.Point, plot func(int, int)) {
	dx, dy := abs(b.X-a.X), -abs(b.Y-a.Y)
	sx, sy := -1, -1
	if a.X < b.X {
		sx = 1
	}
	if a.Y < b.Y {
		sy = 1
	}
	err := dx + dy
	for {
		plot(a.X, a.Y)
		if a == b {
			return
		}
		e := 2 * err
		if e >= dy {
			err += dy
			a.X += sx
		}
		if e <= dx {
			err += dx
			a.Y += sy
		}
	}
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
