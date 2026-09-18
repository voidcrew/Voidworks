package wsplanet

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/planet"
)

// Mineral subtypes commonly inherit names like "voidcrew" or "volcanic".
// Describe the terrain and distinguish the ore rolls without exposing code paths.
func (w *Workspace) terrainName(path string) string {
	if dm.IsPath(path, "/turf/open/lava/plasma") {
		return "Liquid plasma"
	}
	if dm.IsPath(path, "/turf/open/lava/smooth/lava_land_surface") {
		return "Lava"
	}
	if path == "/turf/closed/wall/rust" {
		return "Rusted wall"
	}
	if path == "/turf/closed/wall/r_wall/rust" {
		return "Rusted reinforced wall"
	}
	if !dm.IsPath(path, "/turf/closed/mineral/random") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(path, "/turf/closed/mineral/random"), "/")
	var names []string
	rich := false
	for _, part := range parts {
		switch part {
		case "", "voidcrew":
		case "high_chance":
			rich = true
		case "snow":
			names = append(names, "snowy")
		default:
			names = append(names, strings.ReplaceAll(part, "_", " "))
		}
	}
	name := strings.Join(names, " ")
	if name != "" {
		name += " "
	}
	name += "rock"
	if rich {
		name += " (more ore)"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func (w *Workspace) thumbnail(path string) *dmicon.Sprite {
	_, _, sprite := w.spriteAppearance(path)
	return sprite
}

func (w *Workspace) spriteAppearance(path string) (*dmvars.Variables, string, *dmicon.Sprite) {
	for _, env := range []*dmenv.Dme{w.dme, w.catalog.Dme} {
		o := env.Objects[path]
		if o == nil {
			continue
		}
		d, err := dmicon.Cache.Get(o.Vars.TextV("icon", ""))
		if err != nil {
			continue
		}
		base := o.Vars.TextV("base_icon_state", "")
		states := []string{o.Vars.TextV("icon_state", "")}
		if o.Vars.IntV("smoothing_flags", 0) != 0 && base != "" {
			states = append([]string{base + "-0"}, states...)
		}
		for _, name := range states {
			state := d.States[name]
			if state != nil && state.Frames > 0 && len(state.Sprites) > 0 {
				return o.Vars, name, state.SpriteV(o.Vars.IntV("dir", dm.DirDefault)).Current()
			}
		}
	}
	return nil, "", nil
}

func (w *Workspace) sprite(path string, pos imgui.Vec2, size float32) {
	sprite := w.thumbnail(path)
	if sprite == nil {
		// A missing source appearance must be visible, not an empty space.
		imgui.WindowDrawList().AddText(imgui.Vec2{X: pos.X + size*.35, Y: pos.Y + size*.2}, style.ColorWhitePacked, "?")
		return
	}
	scale := size / float32(max(sprite.IconWidth(), sprite.IconHeight()))
	width, height := float32(sprite.IconWidth())*scale, float32(sprite.IconHeight())*scale
	pos.X += (size - width) / 2
	pos.Y += (size - height) / 2
	imgui.WindowDrawList().AddImageV(imgui.TextureID(sprite.Texture()), pos, imgui.Vec2{X: pos.X + width, Y: pos.Y + height}, imgui.Vec2{X: sprite.U1, Y: sprite.V1}, imgui.Vec2{X: sprite.U2, Y: sprite.V2}, 0xffffffff)
}

func (w *Workspace) pickerItems(field string) []string {
	known := map[string]bool{}
	if field == "open_turf_types" && w.pickingEnvironment {
		for _, d := range w.catalog.Planets {
			if d.Environment != nil {
				known[d.Environment.Baseturf] = true
			}
		}
		known[w.project.State.Definition.Environment.Baseturf] = true
	}
	if field == "river_turf" {
		for _, d := range w.catalog.Planets {
			if d.Rivers != nil && d.Rivers.Turf != "" {
				known[d.Rivers.Turf] = true
			}
		}
		for _, path := range w.items {
			if dm.IsPath(path, "/turf/open/water") && !strings.Contains(path, "/debug") {
				known[path] = true
			}
		}
		if r := w.project.State.Definition.Rivers; r != nil {
			known[r.Turf] = true
		}
	}
	collect := func(b planet.Biome, allowDebug bool) {
		for _, table := range b.Tables {
			if table.Field != field {
				continue
			}
			for _, e := range table.Entries {
				if allowDebug || !strings.Contains(e.Path, "/debug") {
					known[e.Path] = true
				}
			}
		}
	}
	for _, d := range w.catalog.Planets {
		for _, grid := range [][][]string{d.Surface, d.Caves} {
			for _, row := range grid {
				for _, path := range row {
					collect(w.catalog.Biomes[path], false)
				}
			}
		}
	}
	for _, b := range w.project.State.Biomes {
		collect(b, true)
	}
	var items []string
	query := strings.ToLower(w.filter)
	for _, path := range w.items {
		if !planet.Fits(field, path) {
			continue
		}
		if w.pickerScope == 0 && !known[path] {
			continue
		}
		if field == "closed_turf_types" {
			if w.pickerScope == 1 && (!dm.IsPath(path, "/turf/closed/mineral") || strings.Contains(path, "/debug")) {
				continue
			}
			if w.pickerScope == 2 && !dm.IsPath(path, "/turf/closed/wall") {
				continue
			}
		}
		if strings.Contains(strings.ToLower(w.itemName(path)+" "+path), query) {
			items = append(items, path)
		}
	}
	// This is a special runtime roll, not a DME object.
	if known[planet.MegafaunaRoll] && strings.Contains("megafauna roll", query) {
		items = append(items, planet.MegafaunaRoll)
	}
	sort.Slice(items, func(i, j int) bool { return w.itemName(items[i])+items[i] < w.itemName(items[j])+items[j] })
	return items
}

func (w *Workspace) pickerDetail(path string) string {
	if dm.IsPath(path, "/turf/closed/mineral/random") {
		return "Random ore deposits"
	}
	if dm.IsPath(path, "/turf/closed/mineral") {
		return "Natural rock"
	}
	if dm.IsPath(path, "/turf/closed/wall") {
		return "Built wall"
	}
	return ""
}

func (w *Workspace) picker() {
	b := w.project.State.Biomes[w.selected]
	var t planet.Table
	if w.pickingRiver {
		r := w.project.State.Definition.Rivers
		t = planet.Table{Field: "river_turf", Name: "River material", Entries: []planet.Entry{{Path: r.Turf, Weight: 1}}}
	} else if w.pickingEnvironment {
		t = planet.Table{Field: "open_turf_types", Name: "Ground beneath the surface", Entries: []planet.Entry{{Path: w.project.State.Definition.Environment.Baseturf, Weight: 1}}}
	} else if w.table >= 0 && w.table < len(b.Tables) {
		t = b.Tables[w.table]
	} else {
		w.picking = false
		return
	}
	workshop.Title("Choose " + strings.ToLower(t.Name))
	if imgui.Button("Cancel") {
		w.picking = false
		w.pickingRiver, w.pickingEnvironment = false, false
		return
	}
	if w.replace >= 0 && w.replace < len(t.Entries) && t.Entries[w.replace].Path != "" {
		workshop.Muted("Replacing: " + w.itemName(t.Entries[w.replace].Path))
	} else if !w.pickingRiver && !w.pickingEnvironment {
		workshop.Muted("Add a choice to " + b.Name + ".")
	}
	if t.Field == "closed_turf_types" {
		workshop.Muted("These fill the solid parts of caves. Change Ground to choose the cave floor.")
	}
	textField("Search", "Name or type path", &w.filter)
	scopes := []string{"Planet choices", "All types (advanced)"}
	if t.Field == "closed_turf_types" {
		scopes = []string{"Planet cave walls", "Natural rock", "Built walls", "All types (advanced)"}
	}
	w.pickerScope = min(w.pickerScope, len(scopes)-1)
	if combo("Show", scopes[w.pickerScope]) {
		for i, name := range scopes {
			if imgui.Selectable(name) {
				w.pickerScope = i
			}
		}
		imgui.EndCombo()
	}
	items := w.pickerItems(t.Field)
	workshop.Muted(fmt.Sprintf("%d choices · Click a tile to use it", len(items)))
	if _, ok := w.app.(spriteEditor); ok {
		workshop.Muted("Right-click a choice to edit its sprite.")
	}
	imgui.BeginChild("planet-picker-results")
	for index, path := range items {
		pos := imgui.CursorScreenPos()
		detail := w.pickerDetail(path)
		badge := ""
		for i, e := range t.Entries {
			if e.Path == path {
				badge = "In biome"
				if i == w.replace {
					badge = "Current"
				}
			}
		}
		if workshop.Row(path, w.itemName(path), detail, badge, badge == "Current", style.Teal, 38) {
			w.chooseItem(path)
		}
		w.spriteRowMenu(path)
		workshop.Tooltip(w.itemName(path) + "\n" + path)
		y := float32(5)
		if detail != "" {
			y = 12
		}
		w.sprite(path, imgui.Vec2{X: pos.X + 9*window.PointSize(), Y: pos.Y + y*window.PointSize()}, 32*window.PointSize())
		if index >= 149 {
			workshop.Muted("Type more to narrow the results.")
			break
		}
	}
	if len(items) == 0 {
		workshop.Muted("No matches. Try another search or change Show above.")
	}
	imgui.EndChild()
}

func (w *Workspace) chooseItem(path string) {
	if w.pickingEnvironment {
		w.project.State.Definition.Environment.Baseturf = path
		w.picking, w.pickingEnvironment = false, false
		return
	}
	if w.pickingRiver {
		r := w.project.State.Definition.Rivers
		if r.Turf == "" {
			r.Enabled = true
		}
		r.Turf = path
		w.picking, w.pickingRiver = false, false
		return
	}
	b := w.project.State.Biomes[w.selected]
	for i, e := range b.Tables[w.table].Entries {
		if e.Path == path && i != w.replace {
			w.message = "That choice is already in this biome."
			return
		}
	}
	w.local()
	b = w.project.State.Biomes[w.selected]
	entries := b.Tables[w.table].Entries
	if w.replace >= 0 && w.replace < len(entries) {
		entries[w.replace].Path = path
	} else {
		entries = append(entries, planet.Entry{Path: path, Weight: 1})
	}
	w.selectedEntry, w.revealEntry = path, true
	b.Tables[w.table].Entries = entries
	w.project.State.Biomes[w.selected] = b
	w.picking = false
	w.message = ""
}
