package dmi

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
)

// Layers are optional authoring data. Cels always contain the flattened image
// read by BYOND. The private PNG chunk is unsafe-to-copy so external editors
// cannot accidentally keep stale authoring data after changing the image.
const layerChunk = "vwLY"
const maxLayerPixels = 16 * 1024 * 1024

type Layer struct {
	Name            string
	Visible, Locked bool
	Opacity         uint8
	Cels            []*image.NRGBA
}

func (i *Icon) checkLayerGrowth(extraCels int64) error {
	total := extraCels
	for _, s := range i.States {
		for _, l := range s.Layers {
			total += int64(len(l.Cels))
		}
	}
	if total*int64(i.Width)*int64(i.Height) > maxLayerPixels {
		return fmt.Errorf("layers exceed the %d pixel editing limit", maxLayerPixels)
	}
	return nil
}

func (i *Icon) AddLayer(state int) (int, error) {
	if state < 0 || state >= len(i.States) {
		return 0, fmt.Errorf("select a state")
	}
	s := i.States[state]
	extra := int64(len(s.Cels))
	if len(s.Layers) == 0 {
		extra *= 2
	}
	if err := i.checkLayerGrowth(extra); err != nil {
		return 0, err
	}
	i.Changed = true
	return s.AddLayer(i.Width, i.Height), nil
}

func (i *Icon) DuplicateLayer(state, layer int) (int, error) {
	if state < 0 || state >= len(i.States) {
		return 0, fmt.Errorf("select a state")
	}
	s := i.States[state]
	if layer < 0 || layer >= len(s.Layers) {
		return 0, fmt.Errorf("select a layer")
	}
	if err := i.checkLayerGrowth(int64(len(s.Cels))); err != nil {
		return 0, err
	}
	l := s.Layers[layer]
	l.Name += " copy"
	l.Cels = append([]*image.NRGBA(nil), l.Cels...)
	s.Layers = append(s.Layers, l)
	s.RecomposeAll()
	i.Changed = true
	return len(s.Layers) - 1, nil
}

func (s *State) AddLayer(width, height int) int {
	if len(s.Layers) == 0 {
		s.Layers = append(s.Layers, Layer{Name: "Base", Visible: true, Opacity: 255, Cels: append([]*image.NRGBA(nil), s.Cels...)})
	}
	l := Layer{Name: fmt.Sprintf("Layer %d", len(s.Layers)+1), Visible: true, Opacity: 255}
	for range s.Cels {
		l.Cels = append(l.Cels, image.NewNRGBA(image.Rect(0, 0, width, height)))
	}
	s.Layers = append(s.Layers, l)
	return len(s.Layers) - 1
}
func (s *State) RawCel(layer, cel int) *image.NRGBA {
	if len(s.Layers) == 0 {
		return s.Cels[cel]
	}
	return s.Layers[max(0, min(layer, len(s.Layers)-1))].Cels[cel]
}
func (s *State) SetCel(layer, cel int, pixels *image.NRGBA) error {
	if len(s.Layers) == 0 {
		s.Cels[cel] = pixels
		return nil
	}
	l := &s.Layers[max(0, min(layer, len(s.Layers)-1))]
	if l.Locked {
		return fmt.Errorf("the selected layer is locked")
	}
	l.Cels[cel] = pixels
	s.Recompose(cel)
	return nil
}
func (s *State) Recompose(cel int) {
	if len(s.Layers) == 0 {
		return
	}
	dst := image.NewNRGBA(s.Cels[cel].Rect)
	base := true
	for _, layer := range s.Layers {
		if !layer.Visible || layer.Opacity == 0 {
			continue
		}
		src := layer.Cels[cel]
		if base {
			dst = CopyImage(src)
			for n := 3; n < len(dst.Pix); n += 4 {
				dst.Pix[n] = uint8(uint32(dst.Pix[n]) * uint32(layer.Opacity) / 255)
			}
			base = false
			continue
		}
		for y := 0; y < dst.Rect.Dy(); y++ {
			for x := 0; x < dst.Rect.Dx(); x++ {
				a, b := src.NRGBAAt(x, y), dst.NRGBAAt(x, y)
				alpha := uint32(a.A) * uint32(layer.Opacity) / 255
				if alpha == 0 {
					continue
				}
				out := alpha*255 + uint32(b.A)*(255-alpha)
				mix := func(front, back uint8) uint8 {
					return uint8((uint32(front)*alpha*255 + uint32(back)*uint32(b.A)*(255-alpha) + out/2) / out)
				}
				dst.SetNRGBA(x, y, color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: uint8((out + 127) / 255)})
			}
		}
	}
	s.Cels[cel] = dst
}
func (s *State) RecomposeAll() {
	for n := range s.Cels {
		s.Recompose(n)
	}
}

type savedLayer struct {
	Name            string
	Visible, Locked bool
	Opacity         uint8
	Images          [][]byte
}
type savedLayers struct {
	Version       int
	CompositeHash []byte
	States        [][]savedLayer
}

func (i *Icon) compositeHash() []byte {
	h := sha256.New()
	h.Write([]byte(i.Metadata()))
	for _, s := range i.States {
		for _, cel := range s.Cels {
			h.Write(cel.Pix)
		}
	}
	return h.Sum(nil)
}
func (i *Icon) HasLayers() bool {
	for _, s := range i.States {
		if len(s.Layers) > 0 {
			return true
		}
	}
	return false
}
func (i *Icon) encodeLayers() ([]byte, error) {
	if !i.HasLayers() {
		return nil, nil
	}
	data := savedLayers{Version: 1, CompositeHash: i.compositeHash(), States: make([][]savedLayer, len(i.States))}
	for n, s := range i.States {
		for _, l := range s.Layers {
			layer := savedLayer{Name: l.Name, Visible: l.Visible, Locked: l.Locked, Opacity: l.Opacity}
			for _, cel := range l.Cels {
				var b bytes.Buffer
				if err := png.Encode(&b, cel); err != nil {
					return nil, err
				}
				layer.Images = append(layer.Images, b.Bytes())
			}
			data.States[n] = append(data.States[n], layer)
		}
	}
	var out bytes.Buffer
	z := zlib.NewWriter(&out)
	if err := json.NewEncoder(z).Encode(data); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func (i *Icon) decodeLayers(data []byte) error {
	z, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer z.Close()
	raw, err := io.ReadAll(io.LimitReader(z, 128<<20+1))
	if err != nil || len(raw) > 128<<20 {
		return fmt.Errorf("layer data exceeds editing limit or is damaged")
	}
	var stored savedLayers
	if err = json.Unmarshal(raw, &stored); err != nil {
		return err
	}
	if stored.Version != 1 || len(stored.States) != len(i.States) || !bytes.Equal(stored.CompositeHash, i.compositeHash()) {
		return fmt.Errorf("stored layers do not match the DMI image; reopen a backup or flatten with another editor")
	}
	var pixels int64
	for n, state := range stored.States {
		for _, layer := range state {
			if len(layer.Images) != len(i.States[n].Cels) {
				return fmt.Errorf("stored layer frame count does not match")
			}
			l := Layer{Name: layer.Name, Visible: layer.Visible, Locked: layer.Locked, Opacity: layer.Opacity}
			for _, data := range layer.Images {
				cfg, err := png.DecodeConfig(bytes.NewReader(data))
				if err != nil || cfg.Width != i.Width || cfg.Height != i.Height {
					return fmt.Errorf("invalid stored layer dimensions")
				}
				pixels += int64(cfg.Width) * int64(cfg.Height)
				if pixels > maxLayerPixels {
					return fmt.Errorf("stored layers exceed editing limit")
				}
				img, err := png.Decode(bytes.NewReader(data))
				if err != nil {
					return err
				}
				l.Cels = append(l.Cels, NRGBA(img))
			}
			i.States[n].Layers = append(i.States[n].Layers, l)
		}
	}
	// Check the encoded composite rather than replacing it from untrusted layers.
	check := i.Clone()
	for _, s := range check.States {
		s.RecomposeAll()
	}
	if !bytes.Equal(check.compositeHash(), stored.CompositeHash) {
		return fmt.Errorf("stored layer pixels do not match the flattened image")
	}
	return nil
}
