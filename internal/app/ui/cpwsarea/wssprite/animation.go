package wssprite

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmi"
)

func (w *Workspace) applyFrameDuration(all bool) {
	ms, err := strconv.ParseFloat(strings.TrimSpace(w.frameMilliseconds), 64)
	if err != nil || ms <= 0 || math.IsNaN(ms) || math.IsInf(ms, 0) {
		w.message = "Frame duration must be greater than zero. For example, 100 ms gives 10 frames per second."
		return
	}
	w.changeAnimation("Frame duration", func(i *dmi.Icon) error {
		s := i.States[w.state]
		delays := s.Delays()
		for n := range delays {
			if all || n == w.cel/s.Dirs() {
				delays[n] = ms / 100
			}
		}
		s.SetDelays(delays)
		return nil
	})
}

func (w *Workspace) applyTimingList() {
	text := strings.TrimSpace(w.delays)
	w.changeAnimation("Frame timing", func(i *dmi.Icon) error {
		s := i.States[w.state]
		if text == "" {
			s.Remove("delay")
			return nil
		}
		parts := strings.Split(text, ",")
		if len(parts) != 1 && len(parts) != s.Frames() {
			return fmt.Errorf("enter one delay for all frames, or %d comma-separated delays", s.Frames())
		}
		values := make([]float64, s.Frames())
		for n := range values {
			p := parts[min(n, len(parts)-1)]
			v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("delays must be positive numbers; 1 tick = 100 ms")
			}
			values[n] = v
		}
		s.SetDelays(values)
		return nil
	})
}

func (w *Workspace) animationControls() {
	s := w.Document.Icon.States[w.state]
	if imgui.CollapsingHeaderV("Animation", imgui.TreeNodeFlagsDefaultOpen) {
		imgui.TextDisabled(fmt.Sprintf("Frame %d of %d. Select a frame in the timeline below.", w.cel/s.Dirs()+1, s.Frames()))
		imgui.SetNextItemWidth(110)
		imgui.InputText("Frame duration (ms)", &w.frameMilliseconds)
		if imgui.Button("Apply to this frame") {
			w.applyFrameDuration(false)
		}
		imgui.SameLine()
		if imgui.Button("Apply to all frames") {
			w.applyFrameDuration(true)
		}
		imgui.SameLine()
		imgui.TextDisabled("100 ms = 0.1 seconds")
		s = w.Document.Icon.States[w.state]
		forever := s.Int("loop", 0) == 0
		if imgui.Checkbox("Loop forever", &forever) {
			loops := "1"
			if forever {
				loops = "0"
			}
			w.changeAnimation("Animation loops", func(i *dmi.Icon) error { i.States[w.state].Set("loop", loops); return nil })
		}
		if !forever {
			imgui.SameLine()
			loops := int32(max(1, s.Int("loop", 1)))
			imgui.SetNextItemWidth(100)
			if imgui.InputInt("Play count", &loops) && loops > 0 {
				w.changeAnimation("Animation loops", func(i *dmi.Icon) error { i.States[w.state].Set("loop", strconv.Itoa(int(loops))); return nil })
			}
		}
		imgui.SameLine()
		rewind := s.Int("rewind", 0) != 0
		if imgui.Checkbox("Ping-pong (rewind)", &rewind) {
			w.changeAnimation("Rewind animation", func(i *dmi.Icon) error { i.States[w.state].Set("rewind", boolNumber(rewind)); return nil })
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("Play forwards, then backwards, before the next cycle.")
		}
		if s.Frames() == 1 {
			imgui.TextDisabled("This state has one frame. Use + Blank or Duplicate frame to animate it.")
		} else if s.AnimationFinished(w.playbackTime(time.Now())) {
			imgui.TextDisabled("Playback finished. Press Play to replay from the beginning.")
		}
		if imgui.TreeNode("Advanced timing: BYOND tick list") {
			imgui.TextWrapped("1 tick = 100 ms. Enter one value for all frames, or one value per frame separated by commas. Leave blank for 100 ms per frame.")
			imgui.SetNextItemWidth(240)
			imgui.InputText("Delays", &w.delays)
			imgui.SameLine()
			if imgui.Button("Apply timing") {
				w.applyTimingList()
			}
			imgui.TreePop()
		}
	}
	if imgui.CollapsingHeader("Advanced DMI metadata") {
		movement := s.Movement()
		if imgui.Checkbox("Movement state", &movement) {
			w.change("Movement state", func(i *dmi.Icon) error { i.States[w.state].Set("movement", boolNumber(movement)); return nil })
		}
		imgui.TextWrapped("Hotspot is an optional reference point stored in the DMI. It does not control animation timing. Leave it unchanged unless your sprite needs it.")
		imgui.SetNextItemWidth(180)
		imgui.InputText("Hotspot", &w.hotspot)
		imgui.SameLine()
		if imgui.Button("Apply hotspot") {
			text := strings.TrimSpace(w.hotspot)
			w.change("Hotspot", func(i *dmi.Icon) error {
				if text == "" {
					i.States[w.state].Remove("hotspot")
					return nil
				}
				for _, v := range strings.Split(text, ",") {
					if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
						return fmt.Errorf("hotspot must contain comma-separated integers")
					}
				}
				i.States[w.state].Set("hotspot", text)
				return nil
			})
		}
		if imgui.TreeNode("All metadata") {
			for _, f := range s.Fields {
				imgui.TextWrapped(f.Key + " = " + f.Value)
			}
			imgui.TreePop()
		}
	}
}
