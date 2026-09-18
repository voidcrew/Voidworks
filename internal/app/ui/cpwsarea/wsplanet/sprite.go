package wsplanet

import (
	"math"
	"strconv"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/mappreview"
	"sdmm/internal/util"
)

type spriteEditor interface {
	DoEditSprite(*dmmprefab.Prefab)
}

func (w *Workspace) editSpriteMenu(prefab *dmmprefab.Prefab) {
	app, ok := w.app.(spriteEditor)
	if !ok {
		return
	}
	imgui.BeginDisabledV(prefab == nil || prefab.Vars().TextV("icon", "") == "")
	if imgui.MenuItem("Edit sprite") {
		app.DoEditSprite(prefab)
	}
	imgui.EndDisabled()
}

// Use the same resolved appearance as the row's thumbnail, including compiled
// icons and the initial smoothing state, rather than an inherited empty state.
func (w *Workspace) spriteRowMenu(path string) {
	if _, ok := w.app.(spriteEditor); !ok {
		return
	}
	if imgui.BeginPopupContextItemV("sprite-actions-"+path, 1) {
		imgui.TextDisabled(w.itemName(path))
		prefab := w.spritePrefab(path)
		w.editSpriteMenu(prefab)
		imgui.EndPopup()
	}
}

func (w *Workspace) spritePrefab(path string) *dmmprefab.Prefab {
	vars, state, _ := w.spriteAppearance(path)
	if vars == nil {
		return nil
	}
	return dmmprefab.New(0, path, dmvars.Set(dmvars.FromParent(vars), "icon_state", strconv.Quote(state)))
}

// Freeze the rendered appearances when opening the menu. Moving onto the popup
// must not retarget the action, and an overhanging plant belongs to its origin.
func (w *Workspace) previewSprites() []*dmmprefab.Prefab {
	if w.scene == nil {
		return nil
	}
	coord := util.Point{X: int(math.Floor(float64(w.mouseWorld.X/32))) + 1, Y: int(math.Floor(float64(w.mouseWorld.Y/32))) + 1, Z: 1}
	var result []*dmmprefab.Prefab
	seen := map[uint64]bool{}
	add := func(p *dmmprefab.Prefab) {
		if p == nil {
			return
		}
		id := dmmprefab.Id(p.Path(), p.Vars())
		if !seen[id] && mappreview.Visible(p) && p.Vars().TextV("icon", "") != "" {
			seen[id] = true
			result = append(result, p)
		}
	}
	if w.hovered != nil {
		coord = w.hovered.Coord()
		add(w.hovered.Prefab())
	}
	if coord.X < 1 || coord.Y < 1 || coord.X > w.scene.Map.MaxX || coord.Y > w.scene.Map.MaxY {
		return result
	}
	if tile := w.scene.Map.GetTile(coord); tile != nil {
		instances := tile.Instances()
		for i := len(instances) - 1; i >= 0; i-- {
			add(instances[i].Prefab())
		}
	}
	return result
}

func (w *Workspace) previewSpriteMenu() {
	if _, ok := w.app.(spriteEditor); !ok {
		return
	}
	if w.control.Active() && imgui.IsMouseClicked(imgui.MouseButtonRight) {
		w.spriteTargets = w.previewSprites()
		if len(w.spriteTargets) > 0 {
			imgui.OpenPopup("planet-sprite-actions")
		}
	}
	if imgui.BeginPopup("planet-sprite-actions") {
		if len(w.spriteTargets) > 0 {
			p := w.spriteTargets[0]
			imgui.TextDisabled(w.itemName(p.Path()))
			w.editSpriteMenu(p)
			if len(w.spriteTargets) > 1 {
				imgui.Separator()
				imgui.TextDisabled("Also on this tile")
				for i, p := range w.spriteTargets[1:] {
					if imgui.BeginMenu(w.itemName(p.Path()) + "##sprite-" + strconv.Itoa(i)) {
						w.editSpriteMenu(p)
						imgui.EndMenu()
					}
				}
			}
		}
		imgui.EndPopup()
	}
}
