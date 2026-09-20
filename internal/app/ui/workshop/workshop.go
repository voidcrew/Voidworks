// Package workshop contains the shared visual language for project workspaces.
package workshop

import (
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/window"
	"sdmm/internal/imguiext/style"
)

func Gap() { imgui.Dummy(imgui.Vec2{Y: 8 * window.PointSize()}) }

func PushStyle() {
	s := window.PointSize()
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 18 * s, Y: 16 * s})
	imgui.PushStyleVarVec2(imgui.StyleVarItemSpacing, imgui.Vec2{X: 12 * s, Y: 8 * s})
	imgui.PushStyleVarVec2(imgui.StyleVarFramePadding, imgui.Vec2{X: 10 * s, Y: 7 * s})
	imgui.PushStyleVarFloat(imgui.StyleVarFrameRounding, 5*s)
	imgui.PushStyleVarFloat(imgui.StyleVarChildRounding, 8*s)
	imgui.PushStyleVarFloat(imgui.StyleVarChildBorderSize, 1*s)
}

func PopStyle() { imgui.PopStyleVarV(6) }

func Panel(id string, size imgui.Vec2, raised bool) {
	color := style.Surface
	if raised {
		color = style.Raised
	}
	imgui.PushStyleColor(imgui.StyleColorChildBg, color)
	imgui.BeginChildV(id, size, true, imgui.WindowFlagsAlwaysUseWindowPadding)
	imgui.PopStyleColor()
}

func EndPanel() { imgui.EndChild() }

func Muted(text string) {
	imgui.PushStyleColor(imgui.StyleColorText, style.Muted)
	Wrapped(text)
	imgui.PopStyleColor()
}

// The binding's TextWrapped passes its argument as a printf format. Use
// unformatted text under a wrap position so labels can safely contain '%'.
func Wrapped(text string) {
	imgui.PushTextWrapPosV(0)
	imgui.Text(text)
	imgui.PopTextWrapPos()
}

func Section(text string, accent imgui.Vec4) {
	imgui.Dummy(imgui.Vec2{Y: 2 * window.PointSize()})
	imgui.TextColored(accent, text)
	imgui.Separator()
}

func Title(text string) {
	imgui.PushFont(window.FontH2)
	imgui.TextWrapped(text)
	imgui.PopFont()
	imgui.Spacing()
}

func Tooltip(text string) {
	if imgui.IsItemHovered() {
		imgui.BeginTooltip()
		imgui.PushTextWrapPosV(360 * window.PointSize())
		imgui.Text(text)
		imgui.PopTextWrapPos()
		imgui.EndTooltip()
	}
}

func Button(label string, primary bool) bool {
	if primary {
		imgui.PushStyleColor(imgui.StyleColorButton, style.Teal)
		imgui.PushStyleColor(imgui.StyleColorButtonHovered, style.RGB(0x83ead9))
		imgui.PushStyleColor(imgui.StyleColorButtonActive, style.RGB(0x36b9a7))
		imgui.PushStyleColor(imgui.StyleColorText, style.Background)
	}
	clicked := imgui.ButtonV(label, imgui.Vec2{X: -1, Y: 36 * window.PointSize()})
	if primary {
		imgui.PopStyleColorV(4)
	}
	return clicked
}

// Ellipsis measures the current font so names cannot cover adjacent columns.
func Ellipsis(text string, width float32) string {
	if imgui.CalcTextSize(text, false, -1).X <= width {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if imgui.CalcTextSize(string(runes[:mid])+"...", false, -1).X <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ""
	}
	return strings.TrimSpace(string(runes[:lo])) + "..."
}

// Row keeps native focus and keyboard interaction with a single hit target.
func Row(id, label, detail, badge string, selected bool, accent imgui.Vec4, inset float32, disabled ...bool) bool {
	s := window.PointSize()
	pos := imgui.CursorScreenPos()
	size := imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 2*imgui.TextLineHeight() + 22*s}
	if detail == "" {
		size.Y = imgui.TextLineHeight() + 22*s
	}
	draw := imgui.WindowDrawList()
	draw.AddRectFilledV(pos, imgui.Vec2{X: pos.X + size.X, Y: pos.Y + size.Y}, imgui.PackedColorFromVec4(style.Raised), 5*s, 0)
	imgui.PushStyleVarVec2(imgui.StyleVarItemSpacing, imgui.Vec2{})
	clicked := imgui.SelectableV("##"+id, selected, 0, size)
	imgui.PopStyleVar()
	alpha := float32(1)
	if len(disabled) > 0 && disabled[0] {
		alpha = .4
	}
	packed := func(c imgui.Vec4) imgui.PackedColor { c.W *= alpha; return imgui.PackedColorFromVec4(c) }
	if selected {
		draw.AddRectFilledV(pos, imgui.Vec2{X: pos.X + 3*s, Y: pos.Y + size.Y}, packed(accent), 2*s, 0)
	}
	x := pos.X + (12+inset)*s
	width := size.X - (24+inset)*s
	if badge != "" {
		badgeWidth := imgui.CalcTextSize(badge, false, -1).X
		if badgeWidth < width*.45 {
			draw.AddText(imgui.Vec2{X: pos.X + size.X - badgeWidth - 12*s, Y: pos.Y + (size.Y-imgui.TextLineHeight())/2}, packed(accent), badge)
			width -= badgeWidth + 16*s
		}
	}
	y := pos.Y + (size.Y-imgui.TextLineHeight())/2
	if detail != "" {
		y = pos.Y + 9*s
	}
	draw.AddText(imgui.Vec2{X: x, Y: y}, packed(style.Text), Ellipsis(label, width))
	if detail != "" {
		draw.AddText(imgui.Vec2{X: x, Y: y + imgui.TextLineHeight() + 4*s}, packed(style.Muted), Ellipsis(detail, width))
	}
	full := label
	if detail != "" {
		full += "\n" + detail
	}
	Tooltip(full)
	// Advance without replacing the selectable as the last item (tooltips and
	// interaction checks at the call site still refer to the whole row).
	cursor := imgui.CursorPos()
	cursor.Y += 10 * s
	imgui.SetCursorPos(cursor)
	return clicked
}

func Banner(name, context string, accent imgui.Vec4) {
	s := window.PointSize()
	Panel("workshop-banner", imgui.Vec2{Y: 60 * s}, false)
	imgui.PushFont(window.FontH3)
	imgui.TextColored(accent, name)
	imgui.SameLine()
	imgui.TextColored(style.Muted, "/")
	imgui.SameLine()
	imgui.Text(Ellipsis(context, imgui.ContentRegionAvail().X))
	imgui.PopFont()
	EndPanel()
}
