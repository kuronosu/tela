package telatest

import (
	"image"
	"time"
)

// MaxFilmFrames is the most frames [App.Film] captures in one call.
const MaxFilmFrames = 100

// FilmFrame is a frame of [App.Film]: the window's content At after the
// action that started the film (the clock of the app, not the wall clock).
type FilmFrame struct {
	At    time.Duration
	Image *image.RGBA
}

// Film captures the motion that act starts, as the frames of a film: the
// window right after act (At 0), then one frame each time the app's clock has
// moved by every, up to span, the last frame at exactly span. It is what a test does by hand
// under [NoSettle] (act, Screenshot, Advance, Screenshot...), for the
// transitions, springs and exits that a settled Screenshot never shows:
//
//	frames := app.Film(600*time.Millisecond, 50*time.Millisecond, func() {
//	    app.Click(telatest.ByRole("button", "Open"))
//	})
//
// The app is settled before act, and act runs, and every frame is taken,
// with the animations kept where they are, as under NoSettle (the app may
// be mounted with it or not). Afterwards the app is as before: with
// NoSettle off, its next action, query or screenshot settles it, and the
// motion the span did not reach runs to its end then. act may be nil: the
// film then starts at the app as it is (for an app mounted under NoSettle,
// what its mount started, such as the entrance of a screen).
//
// At most [MaxFilmFrames] frames are captured: a longer span needs a longer
// interval. The frames are copies, in physical pixels like [App.Screenshot].
func (a *App) Film(span, every time.Duration, act func()) []FilmFrame {
	a.tb.Helper()
	a.mustBeOpen()
	switch {
	case every <= 0:
		a.tb.Fatalf("telatest: Film: every must be positive, such as 50*time.Millisecond")
	case span < 0:
		a.tb.Fatalf("telatest: Film: span cannot be negative; pass the time to film, such as 600*time.Millisecond")
	}
	n := int((span+every-1)/every) + 1
	if n > MaxFilmFrames {
		a.tb.Fatalf("telatest: Film: a span of %v every %v is %d frames, more than the %d it captures; film a shorter span or use an interval of at least %v",
			span, every, n, MaxFilmFrames, span/(MaxFilmFrames-1))
	}
	if !a.cfg.noSettle {
		a.settle()
		a.cfg.noSettle = true
		defer func() { a.cfg.noSettle = false }()
	}
	if act != nil {
		act()
	}
	frames := []FilmFrame{{0, a.Screenshot()}}
	for at := time.Duration(0); at < span; {
		step := min(every, span-at)
		a.Advance(step)
		at += step
		frames = append(frames, FilmFrame{at, a.Screenshot()})
	}
	return frames
}
