package wssprite

import (
	"fmt"
	"image/color"
	"sort"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmi"
)

func rgbaFloats(c color.NRGBA) [4]float32 {
	return [4]float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255, float32(c.A) / 255}
}
func (w *Workspace) paletteControls() {
	if !imgui.CollapsingHeader("Palette and color replacement") {
		return
	}
	if imgui.Button("Extract selected states") {
		counts := map[color.NRGBA]int{}
		for _, n := range w.indices() {
			for _, cel := range w.Document.Icon.States[n].Cels {
				for y := 0; y < cel.Rect.Dy(); y++ {
					for x := 0; x < cel.Rect.Dx(); x++ {
						c := cel.NRGBAAt(x, y)
						if c.A > 0 {
							counts[c]++
						}
					}
				}
			}
		}
		w.palette = nil
		for c := range counts {
			w.palette = append(w.palette, c)
		}
		sort.Slice(w.palette, func(a, b int) bool {
			x, y := w.palette[a], w.palette[b]
			if counts[x] != counts[y] {
				return counts[x] > counts[y]
			}
			return uint32(x.R)<<24|uint32(x.G)<<16|uint32(x.B)<<8|uint32(x.A) < uint32(y.R)<<24|uint32(y.G)<<16|uint32(y.B)<<8|uint32(y.A)
		})
		if len(w.palette) > 256 {
			w.palette = w.palette[:256]
		}
	}
	if imgui.Button("Add drawing color") && len(w.palette) < 256 {
		c := w.paintColor()
		found := false
		for _, old := range w.palette {
			found = found || old == c
		}
		if !found {
			w.palette = append(w.palette, c)
		}
	}
	columns := max(1, int(imgui.ContentRegionAvail().X/25))
	for n, c := range w.palette {
		if n%columns != 0 {
			imgui.SameLine()
		}
		v := rgbaFloats(c)
		imgui.PushStyleColor(imgui.StyleColorButton, imgui.Vec4{X: v[0], Y: v[1], Z: v[2], W: v[3]})
		if imgui.ButtonV(fmt.Sprintf("##swatch%d", n), imgui.Vec2{X: 20, Y: 20}) {
			w.color = v
			w.replaceFrom = v
		}
		imgui.PopStyleColor()
		if imgui.IsItemHovered() {
			imgui.SetTooltip(fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A))
		}
	}
	imgui.SetNextItemWidth(-1)
	imgui.ColorEdit4("From", &w.replaceFrom)
	imgui.TextWrapped("Replace this exact RGBA color with the drawing color in every frame of the selected states.")
	if imgui.Button("Replace in selected states") {
		c := w.replaceFrom
		from := color.NRGBA{R: uint8(c[0]*255 + .5), G: uint8(c[1]*255 + .5), B: uint8(c[2]*255 + .5), A: uint8(c[3]*255 + .5)}
		to := w.paintColor()
		w.change("Replace color", func(i *dmi.Icon) error {
			for _, n := range w.indices() {
				for c := range i.States[n].Cels {
					s := i.States[n]
					if err := s.SetCel(w.layer, c, dmi.ReplaceColor(s.RawCel(w.layer, c), from, to)); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
}
