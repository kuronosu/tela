package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// TestGoesIdle: after mounting, the page's springs and starting
// transitions end within a bounded number of frames of app time and it
// stops requesting frames (nothing syncs at mount, so no endless
// animation runs). It does not depend on the machine's speed: the clock is
// advanced by hand (NoSettle), unlike the goldens, whose settle gives up
// after 5 s of real time on a starved machine and would capture the page
// mid-animation.
func TestGoesIdle(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		a := mount(t, windowW, windowH, telatest.NoSettle(), telatest.Scale(scale))
		// The springs of the numbers and bars come to rest in about 1.5 s
		// (91 frames at 60 Hz).
		a.Advance(3 * time.Second)
		s := a.Stats()
		if s.Frames > 200 {
			t.Errorf("x%g: %d frames in the first 3 s, want at most 200", scale, s.Frames)
		}
		a.Advance(2 * time.Second)
		if s2 := a.Stats(); s2.Frames != s.Frames {
			t.Errorf("x%g: still requesting frames 3 s after the mount: %d frames more in the next 2 s (%+v)", scale, s2.Frames-s.Frames, s2)
		}
	}
}
