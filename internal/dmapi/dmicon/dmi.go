package dmicon

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"os"
	"sdmm/internal/dmi"

	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dm"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/rs/zerolog/log"
)

type Dmi struct {
	IconWidth     int
	IconHeight    int
	TextureWidth  int
	TextureHeight int
	Cols, Rows    int
	Image         image.Image
	Texture       uint32
	States        map[string]*State
}

func (d *Dmi) free() {
	window.RunLater(func() {
		gl.DeleteTextures(1, &d.Texture)
	})
}

func (d *Dmi) State(state string) (*State, error) {
	if dmiState, ok := d.States[state]; ok {
		return dmiState, nil
	}
	if dmiState, ok := d.States[""]; ok {
		return dmiState, nil
	}
	return nil, fmt.Errorf("no dmi state by name [%s]", state)
}

func New(path string) (*Dmi, error) {
	log.Printf("creating new: [%s]...", path)
	model, err := dmi.Open(path)
	if err != nil {
		log.Printf("unable to parse icon metadata [%s]: %s", path, err)
		return nil, err
	}

	return FromIcon(model)
}

func loadRgbaImage(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("unable to open image file [%s]: %s", path, err)
		return nil, err
	}
	defer f.Close()

	imgOs, _, err := image.Decode(f)
	if err != nil {
		log.Printf("unable to decode image file [%s]: %s", path, err)
		return nil, err
	}

	rgba := image.NewNRGBA(imgOs.Bounds())
	draw.Draw(rgba, rgba.Bounds(), imgOs, image.Pt(0, 0), draw.Src)
	if rgba.Stride != rgba.Rect.Size().X*4 {
		return nil, fmt.Errorf("unable to convert image to NRGBA")
	}

	return rgba, nil
}

type State struct {
	Dirs, Frames int
	Sprites      []*Sprite
	previewFrame int
	modelIndex   int
}

func (s State) Sprite() *Sprite {
	return s.SpriteV(dm.DirDefault)
}

func (s State) SpriteV(dir int) *Sprite {
	return s.SpriteByFrame(dir, 0)
}

func (s State) SpriteByFrame(dir, frame int) *Sprite {
	return s.Sprites[s.dir2idx(dir)+frame%s.Frames*s.Dirs]
}

func (s State) dir2idx(dir int) int {
	if s.Dirs == 1 || dir < dm.DirNorth || dir > dm.DirSouthwest {
		return 0
	}

	idx := 0
	switch dir {
	case dm.DirSouth:
		idx = 0
	case dm.DirNorth:
		idx = 1
	case dm.DirEast:
		idx = 2
	case dm.DirWest:
		idx = 3
	case dm.DirSoutheast:
		idx = 4
	case dm.DirSouthwest:
		idx = 5
	case dm.DirNortheast:
		idx = 6
	case dm.DirNorthwest:
		idx = 7
	}

	if idx < s.Dirs {
		return idx
	}
	return 0
}

type Sprite struct {
	dmi            *Dmi
	animation      *State
	direction      int
	X1, Y1, X2, Y2 int
	U1, V1, U2, V2 float32
}

func (s *Sprite) Current() *Sprite {
	if s.animation != nil && s.animation.previewFrame > 0 {
		return s.animation.Sprites[s.animation.previewFrame*s.animation.Dirs+s.direction]
	}
	return s
}

func (s *Sprite) Dmi() *Dmi {
	return s.dmi
}

func (s Sprite) Image() image.Image {
	return s.dmi.Image
}

func (s Sprite) Texture() uint32 {
	return s.dmi.Texture
}

func (s Sprite) TextureWidth() int {
	return s.dmi.TextureWidth
}

func (s Sprite) TextureHeight() int {
	return s.dmi.TextureHeight
}

func (s Sprite) IconWidth() int {
	return s.dmi.IconWidth
}

func (s Sprite) IconHeight() int {
	return s.dmi.IconHeight
}

func newDmiSprite(dmi *Dmi, idx int) *Sprite {
	const uvMargin = .000001
	x := idx % dmi.Cols
	y := idx / dmi.Cols
	return &Sprite{
		dmi: dmi,
		X1:  x * dmi.IconWidth,
		Y1:  y * dmi.IconHeight,
		X2:  (x + 1) * dmi.IconWidth,
		Y2:  (y + 1) * dmi.IconHeight,
		U1:  float32(x)/float32(dmi.Cols) + uvMargin,
		V1:  float32(y)/float32(dmi.Rows) + uvMargin,
		U2:  float32(x+1)/float32(dmi.Cols) - uvMargin,
		V2:  float32(y+1)/float32(dmi.Rows) - uvMargin,
	}
}
