package dmicon

import (
	"fmt"
	"image"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"sdmm/internal/dmi"
	"sdmm/internal/platform"
)

type liveIcon struct {
	model    *dmi.Icon
	rendered *Dmi
	playing  bool
	started  time.Time
	playhead float64
}

var liveIcons = map[string]*liveIcon{}
var LayoutRevision uint64

func iconKey(path string) string {
	path, _ = filepath.Abs(path)
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
func sameLayout(a, b *dmi.Icon) bool {
	if a.Width != b.Width || a.Height != b.Height || a.Columns != b.Columns || len(a.States) != len(b.States) {
		return false
	}
	for n, s := range a.States {
		o := b.States[n]
		if s.Name != o.Name || s.Dirs() != o.Dirs() || s.Frames() != o.Frames() || s.Movement() != o.Movement() {
			return false
		}
	}
	return true
}
func FromIcon(model *dmi.Icon) (*Dmi, error) {
	sheet, e := model.Sheet()
	if e != nil {
		return nil, e
	}
	var maxSize int32
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxSize)
	if sheet.Rect.Dx() > int(maxSize) || sheet.Rect.Dy() > int(maxSize) {
		// Imported icons can be tall strips. Repack only the display atlas; the
		// document retains its original layout for lossless no-op saves.
		copy := model.Clone()
		copy.Columns = int(maxSize) / model.Width
		if copy.Columns == 0 || ((model.CelCount()+copy.Columns-1)/copy.Columns)*model.Height > int(maxSize) {
			return nil, fmt.Errorf("icon exceeds this graphics card's %d pixel atlas limit", maxSize)
		}
		sheet, e = copy.Sheet()
		if e != nil {
			return nil, e
		}
	}
	result := &Dmi{IconWidth: model.Width, IconHeight: model.Height, TextureWidth: sheet.Rect.Dx(), TextureHeight: sheet.Rect.Dy(), Cols: sheet.Rect.Dx() / model.Width, Rows: sheet.Rect.Dy() / model.Height, Image: sheet, Texture: platform.CreateTexture(sheet), States: map[string]*State{}}
	offset := 0
	for index, s := range model.States {
		state := &State{Dirs: s.Dirs(), Frames: s.Frames(), modelIndex: index}
		for n := range s.Cels {
			sprite := newDmiSprite(result, offset)
			sprite.animation, sprite.direction = state, n%s.Dirs()
			state.Sprites = append(state.Sprites, sprite)
			offset++
		}
		// Duplicate and movement states remain editable; normal map appearance uses
		// the first non-movement state, as BYOND does.
		if _, ok := result.States[s.Name]; !ok && !s.Movement() {
			result.States[s.Name] = state
		}
	}
	return result, nil
}

// Preview publishes an unsaved icon on the UI/GL thread. Pixel edits upload only
// replaced cels; structural edits rebuild the atlas and notify scene caches.
func Preview(path string, model *dmi.Icon) error {
	key := iconKey(path)
	old := liveIcons[key]
	if old != nil && sameLayout(old.model, model) {
		var binding int32
		gl.GetIntegerv(gl.TEXTURE_BINDING_2D, &binding)
		gl.BindTexture(gl.TEXTURE_2D, old.rendered.Texture)
		defer gl.BindTexture(gl.TEXTURE_2D, uint32(binding))
		atlas := old.rendered.Image.(*image.NRGBA)
		offset := 0
		changed := false
		for n, s := range model.States {
			for c, cel := range s.Cels {
				if cel != old.model.States[n].Cels[c] {
					x, y := (offset%old.rendered.Cols)*model.Width, (offset/old.rendered.Cols)*model.Height
					for row := 0; row < model.Height; row++ {
						copy(atlas.Pix[(y+row)*atlas.Stride+x*4:], cel.Pix[row*cel.Stride:row*cel.Stride+model.Width*4])
					}
					gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(model.Width), int32(model.Height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(cel.Pix))
					changed = true
				}
				offset++
			}
		}
		if changed {
			gl.GenerateMipmap(gl.TEXTURE_2D)
		}
		old.model = model
		return nil
	}
	rendered, e := FromIcon(model)
	if e != nil {
		return e
	}
	if old != nil {
		old.rendered.free()
	}
	live := &liveIcon{model: model, rendered: rendered}
	if old != nil {
		live.playing, live.started = old.playing, old.started
		live.playhead = old.playhead
	}
	liveIcons[key] = live
	LayoutRevision++
	return nil
}

func PlayPreview(path string, playing bool, started time.Time) {
	SetPreviewPlayback(path, playing, started, 0)
}

func SetPreviewPlayback(path string, playing bool, started time.Time, playhead float64) {
	if live := liveIcons[iconKey(path)]; live != nil {
		live.playing, live.started = playing, started
		live.playhead = playhead
	}
}

// Called once for each rendered surface, so thousands of matching map objects
// share one timing calculation and existing buckets/texture coordinates.
func AdvanceLiveAnimations() {
	for _, live := range liveIcons {
		elapsed := live.playhead
		if live.playing {
			elapsed += max(0, time.Since(live.started).Seconds())
		}
		for _, state := range live.rendered.States {
			s := live.model.States[state.modelIndex]
			state.previewFrame = s.FrameAt(elapsed)
		}
	}
}
func EndPreview(path string) {
	key := iconKey(path)
	if live := liveIcons[key]; live != nil {
		live.rendered.free()
		delete(liveIcons, key)
	}
	// Reload the disk version after Save or Discard, including cached failures.
	for name, icon := range Cache.icons {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(Cache.rootDirPath, name)
		}
		if iconKey(path) == key {
			if icon != nil {
				icon.free()
			}
			delete(Cache.icons, name)
		}
	}
	LayoutRevision++
}
