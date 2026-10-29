package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// The double-click rule is one (ADR-192): content clicks and a title bar
// the backend handles itself (here the headless window manager, which
// toggles maximize like Wayland and X11) both follow the system's
// DoubleClickInterval, read at start and again when the settings change,
// with the platform default when the OS reports none.
func TestDoubleClickFollowsTheSystemSetting(t *testing.T) {
	const ms = time.Millisecond
	dbl := 0
	root := div(Class("flex flex-col"),
		div(Class("h-10"), WindowDrag()),
		div(Class("h-20"), On("dblclick", func(MouseEvent) { dbl++ })))
	b := headless.New(headless.Options{
		Size:     platform.Size{W: 400, H: 300},
		Settings: platform.SystemSettings{DoubleClickInterval: 200 * ms},
	})
	h, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopTimer)
	x := &winHarness{t: t, b: b, h: h}
	x.drain()
	w := x.windows()[0]
	if len(w.HitRegions()) == 0 {
		t.Fatal("no caption region")
	}

	// pair clicks twice at y, gap apart, and reports whether that made a
	// double click: a toggled maximize in the caption, OnDoubleClick in
	// the content.
	click := func(y float32) { // at the clock's time
		w.Inject(move(100, y))
		w.Inject(down(100, y))
		w.Inject(up(100, y))
		b.Drain(h)
	}
	pair := func(y float32, gap time.Duration) bool {
		maxed, before := w.OSState().Maximized, dbl
		click(y)
		b.Advance(gap)
		click(y)
		b.Advance(2 * time.Second) // the next pair starts a new sequence
		x.drain()
		if y < 40 {
			return w.OSState().Maximized != maxed
		}
		return dbl != before
	}
	for _, c := range []struct {
		interval, gap time.Duration
		want          bool
	}{
		{200 * ms, 150 * ms, true},
		{200 * ms, 300 * ms, false}, // a double click by the 500 ms default
		{0, 300 * ms, true},         // unknown: the default
		{0, 600 * ms, false},
		{800 * ms, 600 * ms, true},
	} {
		if c.interval != b.Settings().DoubleClickInterval {
			b.SetSettings(platform.SystemSettings{DoubleClickInterval: c.interval})
			x.drain()
		}
		caption, content := pair(20, c.gap), pair(80, c.gap)
		if caption != c.want || content != c.want {
			t.Errorf("interval %v, clicks %v apart: caption double click %v, content %v, want %v",
				c.interval, c.gap, caption, content, c.want)
		}
	}
}
