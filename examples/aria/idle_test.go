package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// TestGoesIdle: once the entrance transitions end, a window with an overlay
// open asks for no frames: no timer, no endless animation (the hooks'
// delays run on the frame clock and the floating elements follow their
// anchors without a frame of their own). The clock is advanced by hand
// (NoSettle), so the test does not depend on the machine's speed.
//
// The dialog's text field has the focus, so its caret blinks: those frames
// only paint it, and nothing is built, styled or laid out again.
func TestGoesIdle(t *testing.T) {
	for _, name := range Previews.Names() {
		t.Run(name, func(t *testing.T) {
			a := mount(t, Previews[name], telatest.NoSettle())
			a.Advance(3 * time.Second)
			before := a.Stats()
			a.Advance(2 * time.Second)
			after := a.Stats()
			if name == "6 Dialog" {
				if after.Builds != before.Builds || after.Styles != before.Styles || after.Layouts != before.Layouts {
					t.Errorf("a blinking caret rebuilt, styled or laid out the window: %+v then %+v", before, after)
				}
				return
			}
			if after.Frames != before.Frames {
				t.Errorf("still requesting frames 3 s after the mount: %d frames in the next 2 s (%+v)", after.Frames-before.Frames, after)
			}
		})
	}
}
