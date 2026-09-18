package wsplanet

import (
	"reflect"
	"testing"
	"time"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/render/bucket/level/chunk/unit"
	"sdmm/internal/planet"
)

// Exercise real ImGui clicks against the native renderer, not just the selection
// method. This catches canvas transforms, alpha holes and sprite overhangs.
func testPreviewInspection(t *testing.T, w *Workspace, app *planetSpriteTestApp, io imgui.IO, render func(), capture func(string), width, height *int) {
	t.Helper()
	saved, wide, tall := planet.Clone(w.project.State), *width, *height
	defer func() {
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		*width, *height = wide, tall
		w.project.State = saved
		w.bind(w.project)
		w.narrowEditor = false
	}()
	for _, tc := range []struct {
		name, field, path string
		overhang, compact bool
	}{
		{"inspect-fuel-tank", "feature_spawn_list", "/obj/structure/reagent_dispensers/fueltank", false, false},
		{"inspect-tree", "flora_spawn_list", "/obj/structure/flora/tree/jungle", true, false},
		{"inspect-creature", "mob_spawn_list", "/mob/living/basic/crab/beach", false, false},
		{"inspect-creature-compact", "mob_spawn_list", "/mob/living/basic/crab/beach", false, true},
	} {
		w.project.State = planet.Clone(saved)
		w.bind(w.project)
		w.mode, w.narrowEditor = 1, false
		w.selected = "/datum/biome/grass"
		b := w.project.State.Biomes[w.selected]
		if b.Path == "" || w.dme.Objects[tc.path] == nil {
			t.Fatal("missing inspection fixture", tc.path)
		}
		for i := range b.Tables {
			if b.Tables[i].ChanceField != "" {
				b.Tables[i].Chance = 0
			}
			if b.Tables[i].Field == tc.field {
				b.Tables[i].Entries = []planet.Entry{{Path: tc.path, Weight: 1}}
				b.Tables[i].Chance = 20
			}
		}
		w.project.State.Biomes[w.selected] = b
		w.biomeName = b.Name
		w.lastBuild = time.Time{}
		*width, *height = wide, tall
		if tc.compact {
			*width, *height = 1000, 760
		}
		render()
		render()
		before := planet.Clone(w.project.State)
		camera := w.canvas.Render().Camera
		cameraBefore := *camera
		found := false
		for _, tile := range w.scene.Map.Tiles {
			if found {
				break
			}
			if tile.Coord.X < 10 || tile.Coord.X > 35 || tile.Coord.Y < 10 || tile.Coord.Y > 35 {
				continue
			}
			for _, instance := range tile.Instances() {
				if instance.Prefab().Path() != tc.path {
					continue
				}
				u := unit.Make(tile.Coord.X, tile.Coord.Y, instance, 32)
				bounds := u.ViewBounds()
				for py := bounds.Y1 + 2.5; py < bounds.Y2 && !found; py += 4 {
					for px := bounds.X1 + 2.5; px < bounds.X2 && !found; px += 4 {
						point := imgui.Vec2{X: px, Y: py}
						if tc.overhang && int(px/32) == tile.Coord.X-1 && int(py/32) == tile.Coord.Y-1 {
							continue
						}
						if !spriteHit(u, point) {
							continue
						}
						pos := imgui.Vec2{X: w.control.PosMin().X + (px+camera.ShiftX)*camera.Scale, Y: w.control.PosMax().Y - (py+camera.ShiftY)*camera.Scale}
						io.SetMousePosition(pos)
						render()
						if w.hovered == nil || w.hovered.Id() != instance.Id() {
							continue
						}
						io.SetMouseButtonDown(0, true)
						render()
						io.SetMouseButtonDown(0, false)
						render()
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatal("could not click a visible sprite", tc.name)
		}
		if w.project.State.Biomes[w.selected].Tables[w.table].Field != tc.field || w.selectedEntry != tc.path || w.picking || !w.narrowEditor {
			t.Fatal("click did not open and highlight the correct contents", tc.name, w.table, w.selectedEntry)
		}
		if !reflect.DeepEqual(before, w.project.State) || *camera != cameraBefore {
			t.Fatal("inspection changed the planet or moved the preview", tc.name)
		}
		if !tc.compact {
			testPlanetSpriteMenu(t, w, app, io, render, capture, tc.name)
		}
		if tc.name == "inspect-fuel-tank" {
			testPlanetSpriteRow(t, w, app, io, render, capture)
		}
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		capture(tc.name)
	}
}
