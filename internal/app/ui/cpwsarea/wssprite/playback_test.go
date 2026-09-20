package wssprite

import (
	"sdmm/internal/dmi"
	"testing"
	"time"
)

func TestPauseResumeAndSeekPreserveFrameAndDirection(t *testing.T) {
	doc, _ := dmi.Create(1, 1)
	doc.Icon.SetDirections(0, 4)
	doc.Icon.InsertFrame(0, 0, true)
	doc.Icon.InsertFrame(0, 1, true)
	doc.Icon.States[0].SetDelays([]float64{1, 2, 3})
	w := &Workspace{Document: doc, cells: map[int]bool{}}
	w.selectCel(1, false) // north
	now := time.Unix(123, 0)
	w.togglePlayback(now)
	w.togglePlayback(now.Add(150 * time.Millisecond))
	if w.playing || w.cel != 5 || w.playbackTime(now.Add(time.Hour)) != .15 {
		t.Fatal("pause reset playback or lost direction")
	}
	w.togglePlayback(now.Add(10 * time.Second))
	if got := w.playbackTime(now.Add(10*time.Second + 50*time.Millisecond)); got < .199 || got > .201 {
		t.Fatal("resume counted paused time", got)
	}
	w.selectCel(9, false)
	if w.playing || w.playhead < .299 || w.playhead > .301 || !w.cells[9] {
		t.Fatal("seeking did not follow frame timing")
	}
}

func TestPausedPreviewStaysOnSelectedFrameAfterTimingAndFrameEdits(t *testing.T) {
	doc, _ := dmi.Create(1, 1)
	doc.Icon.InsertFrame(0, 0, true)
	doc.Icon.InsertFrame(0, 1, true)
	w := &Workspace{Document: doc, selected: map[int]bool{0: true}, cells: map[int]bool{}}
	w.selectCel(2, false)
	doc.Icon.States[0].SetDelays([]float64{20, 20, 20})
	w.syncFields()
	if doc.Icon.States[0].FrameAt(w.playhead) != 2 || w.playing {
		t.Fatal("changing timing moved the paused map preview to another frame")
	}
	doc.Icon.DeleteFrame(0, 2)
	w.syncFields()
	if w.cel != 1 || doc.Icon.States[0].FrameAt(w.playhead) != 1 {
		t.Fatal("deleting the selected frame left a stale preview position")
	}
}

func TestFinitePlaybackFinishesAndPlayReplays(t *testing.T) {
	doc, _ := dmi.Create(1, 1)
	doc.Icon.SetDirections(0, 4)
	doc.Icon.InsertFrame(0, 0, true)
	doc.Icon.States[0].Set("loop", "1")
	w := &Workspace{Document: doc, cells: map[int]bool{}}
	w.selectCel(1, false)
	now := time.Unix(100, 0)
	w.togglePlayback(now)
	w.updatePlayback(now.Add(time.Second))
	if w.playing || w.cel != 5 {
		t.Fatal("finished animation kept playing or lost its final frame/direction")
	}
	w.togglePlayback(now.Add(time.Minute))
	if !w.playing || w.playbackTime(now.Add(time.Minute)) != 0 || doc.Icon.States[0].FrameAt(w.playhead) != 0 {
		t.Fatal("Play after completion did not replay from the beginning")
	}
	// Changing loop settings after a long continuous preview starts a fresh cycle.
	w.started = now.Add(-time.Hour)
	w.resetPlaybackAfterEdit(now, false)
	if w.playbackTime(now) != 0 || w.cel != 1 || !w.playing {
		t.Fatal("animation edit retained old elapsed time")
	}
	w.updatePlayback(now.Add(50 * time.Millisecond))
	if !w.playing {
		t.Fatal("edited animation stopped before the new cycle finished")
	}
}
