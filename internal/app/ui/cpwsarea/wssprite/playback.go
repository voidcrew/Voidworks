package wssprite

import (
	"sdmm/internal/dmi"
	"strconv"
	"time"
)

func (w *Workspace) playbackTime(now time.Time) float64 {
	elapsed := w.playhead
	if w.playing {
		elapsed += max(0, now.Sub(w.started).Seconds())
	}
	return elapsed
}

func (w *Workspace) togglePlayback(now time.Time) {
	w.finishStroke()
	w.updatePlayback(now)
	if w.playing {
		w.playhead = w.playbackTime(now)
		if len(w.Document.Icon.States) > 0 {
			s := w.Document.Icon.States[w.state]
			w.cel = s.FrameAt(w.playhead)*s.Dirs() + w.cel%s.Dirs()
			w.cells = map[int]bool{w.cel: true}
		}
	} else if len(w.Document.Icon.States) > 0 && w.Document.Icon.States[w.state].AnimationFinished(w.playhead) {
		w.playhead = 0
	}
	w.playing = !w.playing
	w.started = now
}

func (w *Workspace) selectCel(cel int, multiple bool) {
	w.finishStroke()
	w.playing = false
	w.cel = cel
	w.seekSelectedFrame()
	if multiple {
		w.cells[cel] = !w.cells[cel]
	} else {
		w.cells = map[int]bool{cel: true}
	}
}

func (w *Workspace) seekSelectedFrame() {
	w.playhead = 0
	if len(w.Document.Icon.States) > 0 {
		s := w.Document.Icon.States[w.state]
		for _, delay := range s.Delays()[:w.cel/s.Dirs()] {
			w.playhead += delay / 10
		}
		w.frameMilliseconds = strconv.FormatFloat(s.Delays()[w.cel/s.Dirs()]*100, 'f', -1, 64)
	}
}

func (w *Workspace) updatePlayback(now time.Time) {
	if !w.playing || len(w.Document.Icon.States) == 0 {
		return
	}
	s := w.Document.Icon.States[w.state]
	elapsed := w.playbackTime(now)
	if s.AnimationFinished(elapsed) {
		w.playhead, w.playing = elapsed, false
		w.cel = s.FrameAt(elapsed)*s.Dirs() + w.cel%s.Dirs()
		w.cells = map[int]bool{w.cel: true}
		w.frameMilliseconds = strconv.FormatFloat(s.Delays()[w.cel/s.Dirs()]*100, 'f', -1, 64)
	}
}

func (w *Workspace) changeAnimation(label string, edit func(*dmi.Icon) error) {
	now := time.Now()
	finished := w.Document.Icon.States[w.state].AnimationFinished(w.playbackTime(now))
	if w.change(label, edit) {
		w.resetPlaybackAfterEdit(now, finished)
	}
}

func (w *Workspace) resetPlaybackAfterEdit(now time.Time, wasFinished bool) {
	w.started = now
	if w.playing || wasFinished {
		w.cel %= w.Document.Icon.States[w.state].Dirs()
		w.cells = map[int]bool{w.cel: true}
	}
	w.seekSelectedFrame()
}
