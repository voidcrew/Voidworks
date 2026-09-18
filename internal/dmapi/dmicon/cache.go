package dmicon

import (
	"errors"
	"fmt"
	"path/filepath"

	"sdmm/internal/dmapi/dm"

	"github.com/rs/zerolog/log"
)

var Cache = &IconsCache{icons: make(map[string]*Dmi)}

type IconsCache struct {
	rootDirPath string
	icons       map[string]*Dmi
}

func (i *IconsCache) Free() {
	for _, dmi := range i.icons {
		if dmi != nil {
			dmi.free()
		}
	}
	log.Printf("cache free; [%d] icons disposed", len(i.icons))
	i.rootDirPath = ""
	i.icons = make(map[string]*Dmi)
}

func (i *IconsCache) SetRootDirPath(rootDirPath string) {
	i.rootDirPath = rootDirPath
	log.Print("cache root dir:", rootDirPath)
}

// Invalidate reloads disk data without discarding an open sprite editor's live
// preview. Both absolute and project-relative cache aliases refer to one file.
func (i *IconsCache) Invalidate(path string) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(i.rootDirPath, path)
	}
	key := iconKey(path)
	for name, icon := range i.icons {
		full := name
		if !filepath.IsAbs(full) {
			full = filepath.Join(i.rootDirPath, full)
		}
		if iconKey(full) == key {
			if icon != nil {
				icon.free()
			}
			delete(i.icons, name)
		}
	}
	LayoutRevision++
}

func (i *IconsCache) Get(icon string) (*Dmi, error) {
	if len(icon) == 0 {
		return nil, errors.New("dmi icon is empty")
	}
	path := icon
	if !filepath.IsAbs(path) {
		path = filepath.Join(i.rootDirPath, path)
	}
	if live := liveIcons[iconKey(path)]; live != nil {
		return live.rendered, nil
	}

	if dmi, ok := i.icons[icon]; ok {
		if dmi == nil {
			return nil, fmt.Errorf("dmi [%s] is nil", icon)
		}
		return dmi, nil
	}

	dmi, err := New(path)
	i.icons[icon] = dmi
	return dmi, err
}

func (i *IconsCache) GetState(icon, state string) (*State, error) {
	dmi, err := i.Get(icon)
	if err != nil {
		return nil, err
	}
	return dmi.State(state)
}

func (i *IconsCache) GetSpriteV(icon, state string, dir int) (*Sprite, error) {
	dmiState, err := i.GetState(icon, state)
	if err != nil {
		return nil, err
	}
	return dmiState.SpriteV(dir), nil
}

func (i *IconsCache) GetSprite(icon, state string) (*Sprite, error) {
	return i.GetSpriteV(icon, state, dm.DirDefault)
}

func (i *IconsCache) GetSpriteOrPlaceholder(icon, state string) *Sprite {
	return i.GetSpriteOrPlaceholderV(icon, state, dm.DirDefault)
}

func (i *IconsCache) GetSpriteOrPlaceholderV(icon, state string, dir int) *Sprite {
	if s, err := i.GetSpriteV(icon, state, dir); err == nil {
		return s
	}
	return SpritePlaceholder()
}
