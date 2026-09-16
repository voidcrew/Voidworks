// Package dmi edits BYOND icons without depending on a project or graphics context.
package dmi

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
)

const MaxPixels = 64 * 1024 * 1024

// Field retains unrecognised metadata as well as the spelling of numeric values.
type Field struct{ Key, Value string }
type Chunk struct {
	Kind string
	Data []byte
}
type State struct {
	Name   string
	Fields []Field
	// Cels are frame-major, with directions S,N,E,W,SE,SW,NE,NW.
	Cels   []*image.NRGBA
	Layers []Layer
}
type Icon struct {
	Width, Height, Columns int
	Fields                 []Field
	States                 []*State
	Chunks                 []Chunk
	Original               []byte
	Changed                bool
	BitDepth               byte
}

func New(width, height int) (*Icon, error) {
	if width < 1 || height < 1 || width > 8192 || height > 8192 || int64(width)*int64(height) > MaxPixels {
		return nil, fmt.Errorf("canvas dimensions must be between 1 and 8192 pixels")
	}
	i := &Icon{Width: width, Height: height, Columns: 1, Changed: true, BitDepth: 8}
	i.States = []*State{NewState("", width, height)}
	return i, nil
}
func NewState(name string, width, height int) *State {
	return &State{Name: name, Fields: []Field{{"dirs", "1"}, {"frames", "1"}}, Cels: []*image.NRGBA{image.NewNRGBA(image.Rect(0, 0, width, height))}}
}
func value(fields []Field, key, fallback string) string {
	for _, f := range fields {
		if f.Key == key {
			return f.Value
		}
	}
	return fallback
}
func (s *State) Value(key, fallback string) string { return value(s.Fields, key, fallback) }
func (s *State) Set(key, v string) {
	for n := range s.Fields {
		if s.Fields[n].Key == key {
			s.Fields[n].Value = v
			return
		}
	}
	s.Fields = append(s.Fields, Field{key, v})
}
func (s *State) Remove(key string) {
	fields := make([]Field, 0, len(s.Fields))
	for _, f := range s.Fields {
		if f.Key != key {
			fields = append(fields, f)
		}
	}
	s.Fields = fields
}
func (s *State) Int(key string, fallback int) int {
	n, e := strconv.Atoi(s.Value(key, strconv.Itoa(fallback)))
	if e != nil {
		return fallback
	}
	return n
}
func (s *State) Dirs() int      { return s.Int("dirs", 1) }
func (s *State) Frames() int    { return s.Int("frames", 1) }
func (s *State) Movement() bool { return s.Int("movement", 0) != 0 }
func (s *State) Delays() []float64 {
	result := make([]float64, s.Frames())
	for n := range result {
		result[n] = 1
	}
	for n, v := range strings.Split(s.Value("delay", ""), ",") {
		if n >= len(result) {
			break
		}
		f, e := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if e == nil && f > 0 && !math.IsInf(f, 0) && !math.IsNaN(f) {
			result[n] = f
		}
	}
	return result
}
func (s *State) SetDelays(delays []float64) {
	parts := make([]string, len(delays))
	for n, v := range delays {
		parts[n] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	s.Set("delay", strings.Join(parts, ","))
}
func (s *State) Clone() *State {
	c := *s
	c.Fields = append([]Field(nil), s.Fields...)
	c.Cels = append([]*image.NRGBA(nil), s.Cels...)
	c.Layers = append([]Layer(nil), s.Layers...)
	for n := range c.Layers {
		c.Layers[n].Cels = append([]*image.NRGBA(nil), s.Layers[n].Cels...)
	}
	return &c
}

// Clone shares immutable pixel buffers. Editing a cel must replace its buffer.
func (i *Icon) Clone() *Icon {
	c := *i
	c.Fields = append([]Field(nil), i.Fields...)
	c.States = make([]*State, len(i.States))
	for n, s := range i.States {
		c.States[n] = s.Clone()
	}
	return &c
}
func CopyImage(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, src.Rect.Dx(), src.Rect.Dy()))
	for y := 0; y < src.Rect.Dy(); y++ {
		copy(dst.Pix[y*dst.Stride:], src.Pix[y*src.Stride:y*src.Stride+src.Rect.Dx()*4])
	}
	return dst
}
func NRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.SetNRGBA(x, y, color.NRGBAModel.Convert(src.At(x+b.Min.X, y+b.Min.Y)).(color.NRGBA))
		}
	}
	return dst
}
func (i *Icon) Validate() error {
	if i.Width < 1 || i.Height < 1 || i.Width > 8192 || i.Height > 8192 {
		return fmt.Errorf("invalid DMI canvas dimensions")
	}
	var total, layerPixels int64
	for n, s := range i.States {
		if s == nil {
			return fmt.Errorf("state %d is missing", n+1)
		}
		dirs, frames := s.Dirs(), s.Frames()
		if (dirs != 1 && dirs != 4 && dirs != 8) || frames < 1 || frames > 65536 {
			return fmt.Errorf("state %q has invalid directions or frames", s.Name)
		}
		if len(s.Cels) != dirs*frames {
			return fmt.Errorf("state %q has %d images, expected %d", s.Name, len(s.Cels), dirs*frames)
		}
		if strings.ContainsAny(s.Name, "\r\n\x00") {
			return fmt.Errorf("state names cannot contain line breaks or NUL")
		}
		for _, c := range s.Cels {
			if c == nil || c.Bounds() != image.Rect(0, 0, i.Width, i.Height) {
				return fmt.Errorf("state %q has an incorrectly sized image", s.Name)
			}
		}
		for _, l := range s.Layers {
			if len(l.Cels) != len(s.Cels) {
				return fmt.Errorf("layer %q has the wrong frame count", l.Name)
			}
			for _, c := range l.Cels {
				if c == nil || c.Rect != image.Rect(0, 0, i.Width, i.Height) {
					return fmt.Errorf("layer %q has an incorrectly sized image", l.Name)
				}
			}
			layerPixels += int64(len(l.Cels)) * int64(i.Width) * int64(i.Height)
			if layerPixels > maxLayerPixels {
				return fmt.Errorf("layers exceed the %d pixel editing limit", maxLayerPixels)
			}
		}
		total += int64(len(s.Cels)) * int64(i.Width) * int64(i.Height)
		if total > MaxPixels {
			return fmt.Errorf("DMI exceeds the %d pixel editing limit", MaxPixels)
		}
	}
	return nil
}
func (i *Icon) CelCount() int {
	n := 0
	for _, s := range i.States {
		n += len(s.Cels)
	}
	return n
}
func (s *State) AnimationDuration() float64 {
	delays := s.Delays()
	duration := 0.0
	for n, delay := range delays {
		duration += delay / 10
		if s.Int("rewind", 0) != 0 && n > 0 && n < len(delays)-1 {
			duration += delay / 10
		}
	}
	return duration
}

func (s *State) AnimationFinished(seconds float64) bool {
	loops := s.Int("loop", 0)
	return loops > 0 && seconds >= s.AnimationDuration()*float64(loops)
}

func (s *State) FrameAt(seconds float64) int {
	if s.Frames() < 2 {
		return 0
	}
	delays := s.Delays()
	sequence := make([]int, 0, len(delays)*2)
	for n := range delays {
		sequence = append(sequence, n)
	}
	if s.Int("rewind", 0) != 0 {
		for n := len(delays) - 2; n > 0; n-- {
			sequence = append(sequence, n)
		}
	}
	duration := 0.0
	for _, n := range sequence {
		duration += delays[n] / 10
	}
	if duration <= 0 {
		return 0
	}
	if loops := s.Int("loop", 0); loops > 0 && seconds >= duration*float64(loops) {
		return sequence[len(sequence)-1]
	}
	position := math.Mod(math.Max(0, seconds), duration)
	for _, n := range sequence {
		position -= delays[n] / 10
		if position < 0 {
			return n
		}
	}
	return 0
}
