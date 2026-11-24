package core

import (
	"testing"
	"time"
)

// FrameTime.Refresh is the display's as of each frame (ADR-317): it changes
// when the window moves to another display, and may be unknown.
func TestTextureNextFrameRefresh(t *testing.T) {
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	w := x.windows()[0]
	next := func() FrameTime {
		t.Helper()
		l := startTexLoop(a.tex, 1, nil)
		waitingFrame(t, a.tex)
		x.drain()
		ft := <-l.frames
		if err := <-l.errs; err != nil {
			t.Fatal(err)
		}
		return ft
	}
	if got, want := next().Refresh, x.b.FrameInterval(); got != want {
		t.Errorf("Refresh = %v, want headless's %v", got, want)
	}
	for _, want := range []time.Duration{time.Second / 144, 0, time.Second / 60} {
		w.SetRefresh(want)
		if got := next().Refresh; got != want {
			t.Errorf("Refresh = %v, want %v", got, want)
		}
	}
	x.expectIdle("once nothing waits for a frame")
}
