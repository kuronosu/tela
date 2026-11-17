package main

import (
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/testutil/idle"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

// The idle budget of the gallery (ARCHITECTURE §3.5, "no change, no
// frame"; ADR-290). idle_test.go checks that every page stops asking for
// frames; these tests count all the work and what is pending, over
// telatest and over the host itself (internal/testutil/idle).

// galleryConfig is App's theme and root class for a host without telatest.
func galleryConfig() core.Config {
	return core.Config{Theme: (*core.Theme)(App.Theme), RootClass: App.RootClass}
}

// TestIdleBudget: the gallery as it opens, on the Introduction, does
// nothing once drawn: no frame, build, style, layout or paint over a
// second of frames and five more seconds, and no frame request, animation
// or timer pending.
func TestIdleBudget(t *testing.T) {
	idle.Zero(t, "the gallery", idle.Watch(mount(t, Gallery{})))
	idle.Mount(t, Gallery{}, App.Width, App.Height, galleryConfig()).Settle().Idle("the gallery")
}

// TestIdleBudgetPulse: the Progress page has an indeterminate bar, which
// pulses forever by design (animate-pulse, the opacity of its indicator),
// so its budget cannot be zero: it asks for a frame at every vsync while it
// is shown. Each of those frames only paints the pulse: no build, style or
// layout of any box, its damage inside the bar, and no timer or deadline
// pending beside the animation's frames.
func TestIdleBudgetPulse(t *testing.T) {
	a := mount(t, Gallery{Page: "Progress"}, telatest.NoSettle())
	a.Advance(time.Second) // the page's entrance and first frames
	for i := range idle.Frames {
		w := a.Measure(func() { a.Advance(idle.Frame) })
		if w.Frames == 0 || w.Builds != 0 || w.Styles != 0 || w.LaidOut != 0 {
			t.Fatalf("frame %d of the pulsing Progress page: %s; want frames that paint only (no build, style or box laid out)", i, idle.Work(w))
		}
	}

	h := idle.Mount(t, Gallery{Page: "Progress"}, App.Width, App.Height, galleryConfig()).SettleEndless()
	if !h.OnlyEndless() {
		t.Fatalf("the Progress page did not settle to its pulse alone: %q", h.Busy())
	}
	bar := rawwin.FindNode(h.H.Tree().Semantics(), "progressbar", "Loading")
	if bar == nil {
		t.Fatal(`no progressbar "Loading" on the Progress page`)
	}
	// Device pixels at scale 1, with the pixel of antialiasing margin the
	// damage of a layer keeps around its bounds.
	area := bar.Bounds.RoundOut().Inset(-1)
	if !area.In(image.Rect(0, 0, int(App.Width), int(App.Height))) {
		t.Fatalf("the indeterminate bar %v is outside the window: its pulse paints nothing", area)
	}
	presents := h.Win.Presents()
	frames := 0
	h.Run(func(w core.Stats) {
		if w.Frames == 0 {
			return
		}
		frames += w.Frames
		if w.Builds != 0 || w.Styles != 0 || w.LaidOut != 0 {
			t.Fatalf("a frame of the pulse did %s; want only paint", idle.Work(w))
		}
		if d := h.Win.Damage(); !idle.Within(d, area) {
			t.Fatalf("a frame of the pulse damaged %v, outside the bar %v", d, area)
		}
		if !h.OnlyEndless() {
			t.Fatalf("the pulsing Progress page has more pending than its pulse: %q", h.Busy())
		}
	})
	if frames == 0 || h.Win.Presents() == presents {
		t.Errorf("the indeterminate bar ran %d frames and presented %d: it no longer pulses, so this page can take the zero budget", frames, h.Win.Presents()-presents)
	}
}
