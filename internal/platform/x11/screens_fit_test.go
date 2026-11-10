//go:build linux

package x11

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
)

// _GTK_WORKAREAS_D<current desktop> cuts each monitor to its own work
// area when the window manager lists _GTK_WORKAREAS; else _NET_WORKAREA
// applies as before.
func TestGTKWorkAreas(t *testing.T) {
	b := fakeEmpty(1)
	f := b.srv.(*fakeServer)
	a := &b.atoms
	laptop, external := image.Rect(0, 0, 1920, 1080), image.Rect(1920, 0, 4480, 1440)
	f.mons = []image.Rectangle{laptop, external}
	f.set(b.root, a.netWorkarea, 0, 0, 4480, 1040, 0, 0, 4480, 1040)
	f.set(b.root, a.netCurrentDesktop, 1)
	// Mutter: a top bar on the laptop (desktop 1), a dock on the left of
	// the external monitor, listed in another order than the monitors.
	f.named = map[string][]uint32{
		"_GTK_WORKAREAS_D0": {0, 0, 1920, 1080, 1920, 0, 2560, 1440},
		"_GTK_WORKAREAS_D1": {1990, 0, 2490, 1440, 0, 32, 1920, 1048},
	}
	want := []image.Rectangle{image.Rect(0, 32, 1920, 1080), image.Rect(1990, 0, 4480, 1440)}
	if got := b.workAreas(); !slices.Equal(got, want) {
		t.Errorf("_GTK_WORKAREAS_D1: %v, want %v", got, want)
	}
	// Not listed in _NET_SUPPORTED: a property left by an earlier mutter.
	delete(b.supported, a.gtkWorkareas)
	netWork := []image.Rectangle{image.Rect(0, 0, 1920, 1040), external}
	if got := b.workAreas(); !slices.Equal(got, netWork) {
		t.Errorf("without _GTK_WORKAREAS supported: %v, want %v", got, netWork)
	}
	b.supported[a.gtkWorkareas] = true
	// A malformed list falls back to _NET_WORKAREA.
	f.named["_GTK_WORKAREAS_D1"] = []uint32{0, 32, 1920}
	if got := b.workAreas(); !slices.Equal(got, netWork) {
		t.Errorf("malformed _GTK_WORKAREAS_D1: %v, want %v", got, netWork)
	}
	// No current desktop: the first.
	f.unset(b.root, a.netCurrentDesktop)
	if got := b.workAreas(); !slices.Equal(got, f.mons) {
		t.Errorf("_GTK_WORKAREAS_D0: %v, want %v", got, f.mons)
	}
}

func TestParseWorkAreas(t *testing.T) {
	neg := uint32(0xffffff00) // -256
	for _, tt := range []struct {
		name string
		v    []uint32
		want []image.Rectangle
	}{
		{"none", nil, nil},
		{"not a multiple of four", []uint32{0, 0, 10}, nil},
		{"two", []uint32{0, 32, 1920, 1048, neg, 0, 256, 100}, []image.Rectangle{image.Rect(0, 32, 1920, 1080), image.Rect(-256, 0, 0, 100)}},
		{"empty skipped", []uint32{0, 0, 0, 10, 5, 5, 10, 10}, []image.Rectangle{image.Rect(5, 5, 15, 15)}},
	} {
		if got := parseWorkAreas(tt.v); !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCutToWorkAreas(t *testing.T) {
	mons := []image.Rectangle{image.Rect(0, 0, 1920, 1080), image.Rect(1920, 0, 4480, 1440), image.Rect(0, 1080, 1920, 2160)}
	works := []image.Rectangle{image.Rect(1920, 0, 4480, 1400), image.Rect(0, 32, 1920, 1080)}
	want := []image.Rectangle{image.Rect(0, 32, 1920, 1080), image.Rect(1920, 0, 4480, 1400), image.Rect(0, 1080, 1920, 2160)}
	if got := cutToWorkAreas(mons, works); !slices.Equal(got, want) {
		t.Errorf("cutToWorkAreas = %v, want %v", got, want)
	}
}

func TestParseFrameExtents(t *testing.T) {
	if _, ok := parseFrameExtents([]uint32{1, 2, 3}); ok {
		t.Error("three values accepted")
	}
	if _, ok := parseFrameExtents([]uint32{1, 2, 1 << 20, 4}); ok {
		t.Error("a huge extent accepted")
	}
	if got, ok := parseFrameExtents([]uint32{1, 2, 37, 4}); !ok || got != (frameExtents{left: 1, right: 2, top: 37, bottom: 4}) {
		t.Errorf("parseFrameExtents = %v, %v", got, ok)
	}
}

// fitFramed fits the frame, so that the title bar of a window whose
// content is at the top of the work area (or the frame at its right edge)
// ends up on screen.
func TestFitFramed(t *testing.T) {
	works := []image.Rectangle{image.Rect(0, 32, 1920, 1080)}
	ext := frameExtents{left: 1, right: 1, top: 37, bottom: 1}
	content := image.Rect(100, 40, 900, 640)
	for _, tt := range []struct {
		name    string
		content image.Rectangle
		ext     frameExtents
		works   []image.Rectangle
		want    image.Point
	}{
		{"no extents, on screen", content, frameExtents{}, works, image.Pt(100, 40)},
		{"title bar above the work area", content, ext, works, image.Pt(100, 69)},
		{"frame on screen", content.Add(image.Pt(0, 100)), ext, works, image.Pt(100, 140)},
		{"gone monitor on the right", content.Add(image.Pt(3000, 100)), ext, works, image.Pt(1119, 140)},
		{"no work areas", content.Add(image.Pt(3000, 100)), ext, nil, image.Pt(3100, 140)},
	} {
		if got := fitFramed(tt.content, tt.ext, tt.works); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// timerRig is a rig with timers the test fires.
type timerRig struct {
	*rig
	timers []*fakeTimer
}

func newTimerRig(t *testing.T) *timerRig {
	g := &timerRig{rig: newRig(t)}
	g.b.timers.hook = func(d time.Duration, fn func()) func() {
		ft := &fakeTimer{d: d, fn: fn}
		g.timers = append(g.timers, ft)
		return func() { ft.stopped = true }
	}
	return g
}

// fire runs the pending timers and returns how many ran.
func (g *timerRig) fire() int {
	n := 0
	for _, ft := range slices.Clone(g.timers) {
		if !ft.stopped {
			ft.stopped = true
			ft.fn()
			n++
		}
	}
	return n
}

// answer sets the frame extents of win as the window manager would, and
// feeds the PropertyNotify.
func (g *rig) answer(win xproto.Window, ext ...uint32) (events, sent string) {
	g.f.set(win, g.b.atoms.netFrameExtents, ext...)
	return g.feed(xproto.PropertyNotifyEvent{Window: win, Atom: g.b.atoms.netFrameExtents})
}

// mapped is what the first map of a window at x, y sends.
func mapped(win, x, y int) string {
	return fmt.Sprintf("configure win=%d x=%d y=%d; prop WM_NORMAL_HINTS win=%d = pos=%d,%d gravity=10; prop _NET_WM_STATE win=%d = []; map win=%d",
		win, x, y, win, x, y, win, win)
}

// A window created at a position on a monitor that is gone is mapped on
// the nearest one, in physical pixels, with its frame (from the window
// manager's answer to _NET_REQUEST_FRAME_EXTENTS) on screen; SetPosition
// is never moved.
func TestCreationPositionOnScreens(t *testing.T) {
	g := newRig(t)
	g.f.mons = []image.Rectangle{image.Rect(0, 0, 1920, 1080)}
	g.f.set(g.b.root, g.b.atoms.netWorkarea, 0, 0, 1920, 1040)
	opts := platform.WindowOptions{Size: platform.Size{W: 800, H: 600}, HasPosition: true, Position: image.Pt(2500, 900)}
	w := fakeWindow(g.b, 7, image.Pt(800, 600), opts)
	g.sent("NewWindow", "prop _NET_WM_WINDOW_TYPE win=7 = [_NET_WM_WINDOW_TYPE_NORMAL]; msg _NET_REQUEST_FRAME_EXTENTS win=7 []")
	g.answer(7, 2, 3, 37, 4)
	g.present(w)
	g.sent("first Present", mapped(7, 1117, 436))
	w.SetPosition(image.Pt(5000, 5000))
	g.sent("SetPosition", "configure win=7 x=5000 y=5000")

	// On a monitor present: where it was asked.
	opts.Position = image.Pt(100, 200)
	w = fakeWindow(g.b, 8, image.Pt(800, 600), opts)
	g.answer(8, 2, 3, 37, 4)
	g.present(w)
	g.sent("first Present on screen", mapped(8, 100, 200))

	// Content at the top of the work area: the title bar goes below it.
	opts.Position = image.Pt(100, 0)
	w = fakeWindow(g.b, 9, image.Pt(800, 600), opts)
	g.answer(9, 2, 3, 37, 4)
	g.present(w)
	g.sent("first Present at the top", mapped(9, 100, 37))
}

// A window manager that lists _NET_REQUEST_FRAME_EXTENTS but answers late
// costs one wait (ADR-262): after a timeout, later windows still ask but
// map at their first Present, fitted with the extents it last gave for a
// window of their kind (a late answer, or the extents it sets once it
// manages the window), else by their content.
func TestCreationFrameExtentsLate(t *testing.T) {
	g := newTimerRig(t)
	g.f.mons = []image.Rectangle{image.Rect(0, 0, 1920, 1080)}
	opts := platform.WindowOptions{Size: platform.Size{W: 800, H: 600}, HasPosition: true, Position: image.Pt(100, 0)}

	w := fakeWindow(g.b, 7, image.Pt(800, 600), opts)
	g.feed()
	g.f.take()
	g.present(w)
	if n := g.fire(); n != 1 {
		t.Fatalf("%d timers fired, want 1", n)
	}
	g.sent("timeout", mapped(7, 100, 0))
	if _, sent := g.answer(7, 0, 0, 30, 0); sent != "" {
		t.Errorf("late answer: sent %s", sent)
	}

	// The next window asks, and maps at once with the late answer's
	// extents.
	w = fakeWindow(g.b, 8, image.Pt(800, 600), opts)
	g.sent("NewWindow after a late answer", "prop _NET_WM_WINDOW_TYPE win=8 = [_NET_WM_WINDOW_TYPE_NORMAL]; msg _NET_REQUEST_FRAME_EXTENTS win=8 []")
	g.feed()
	g.present(w)
	g.sent("first Present after a late answer", mapped(8, 100, 30))
	if len(g.timers) != 1 {
		t.Errorf("%d timers, want none after the first window's", len(g.timers)-1)
	}
	// Its own answer, or a later change, teaches the next window.
	g.answer(8, 0, 0, 40, 0)
	w = fakeWindow(g.b, 9, image.Pt(800, 600), opts)
	g.feed()
	g.f.take()
	g.present(w)
	g.sent("first Present with newer extents", mapped(9, 100, 40))

	// A dialog has extents of its own: none known yet, so it is fitted by
	// its content, without waiting either.
	dlg := opts
	dlg.Parent, dlg.Modal = w, true
	d := fakeWindow(g.b, 10, image.Pt(800, 600), dlg)
	g.feed()
	g.f.take()
	g.present(d)
	if got := g.f.take(); !strings.Contains(got, "configure win=10 x=100 y=0;") || !strings.HasSuffix(got, "map win=10") {
		t.Errorf("first Present of a dialog: sent %s", got)
	}
	if len(g.timers) != 1 {
		t.Errorf("%d timers, want none after the first window's", len(g.timers)-1)
	}

	// The extents of a fullscreen or maximized window (none, or no side
	// borders) are not a normal window's frame: not learned.
	a := &g.b.atoms
	g.f.set(9, a.netWMState, uint32(a.netWMStateFull))
	g.feed(xproto.PropertyNotifyEvent{Window: 9, Atom: a.netWMState})
	g.answer(9, 0, 0, 0, 0)
	g.f.set(9, a.netWMState, uint32(a.netWMStateMaxVert), uint32(a.netWMStateMaxHorz))
	g.feed(xproto.PropertyNotifyEvent{Window: 9, Atom: a.netWMState})
	g.answer(9, 0, 0, 24, 0)
	// Nor all-zero extents of a decorated window in the normal state.
	g.f.set(9, a.netWMState)
	g.feed(xproto.PropertyNotifyEvent{Window: 9, Atom: a.netWMState})
	g.answer(9, 0, 0, 0, 0)
	w = fakeWindow(g.b, 11, image.Pt(800, 600), opts)
	g.feed()
	g.f.take()
	g.present(w)
	g.sent("first Present after fullscreen and maximized extents", mapped(11, 100, 40))
}

// The first map waits for the window manager's frame extents, at most
// frameExtentsTimeout after the request: a Present before the answer maps
// nothing, the PropertyNotify maps, and without one the timeout maps with
// the content fitted. Frameless windows and window managers without the
// request do not wait.
func TestCreationFrameExtentsWait(t *testing.T) {
	g := newTimerRig(t)
	a := &g.b.atoms
	g.f.mons = []image.Rectangle{image.Rect(0, 0, 1920, 1080)}
	opts := platform.WindowOptions{Size: platform.Size{W: 800, H: 600}, HasPosition: true, Position: image.Pt(100, 0)}

	// The answer arrives after the first Present.
	w := fakeWindow(g.b, 7, image.Pt(800, 600), opts)
	g.feed()
	g.f.take()
	g.present(w)
	// The timeout's time passes before the next Present, its callback not
	// run yet (a loaded runner, the race detector): the wait goes on.
	w.extSent -= frameExtentsTimeout
	g.present(w)
	g.sent("Present before the answer", "")
	if len(g.timers) != 1 || g.timers[0].d <= 0 || g.timers[0].d > frameExtentsTimeout {
		t.Fatalf("%d timers, want one of at most %v", len(g.timers), frameExtentsTimeout)
	}
	if _, sent := g.answer(7, 0, 0, 30, 0); sent != mapped(7, 100, 30) {
		t.Errorf("the answer: sent\n got %s\nwant %s", sent, mapped(7, 100, 30))
	}
	if !g.timers[0].stopped {
		t.Error("the timeout was not stopped")
	}
	// A later change of the extents (the window manager's own) is no fit.
	if _, sent := g.answer(7, 0, 0, 60, 0); sent != "" {
		t.Errorf("later extents: sent %s", sent)
	}

	// Hidden while waiting: the answer maps nothing; showing it maps.
	w = fakeWindow(g.b, 9, image.Pt(800, 600), opts)
	g.feed()
	g.f.take()
	g.present(w)
	w.SetVisible(false)
	if _, sent := g.answer(9, 0, 0, 30, 0); sent != "" {
		t.Errorf("answer while hidden: sent %s", sent)
	}
	w.SetVisible(true)
	g.sent("shown", mapped(9, 100, 30))

	// No answer: the timeout maps with the content fitted (and later
	// windows no longer wait: TestCreationFrameExtentsLate).
	w = fakeWindow(g.b, 8, image.Pt(800, 600), opts)
	g.feed()
	g.f.take()
	g.present(w)
	g.sent("Present without an answer", "")
	if n := g.fire(); n != 1 {
		t.Fatalf("%d timers fired, want 1", n)
	}
	g.sent("timeout", mapped(8, 100, 0))
	if _, sent := g.answer(8, 0, 0, 30, 0); sent != "" {
		t.Errorf("late answer: sent %s", sent)
	}

	// Frameless: no request, no wait, no extents.
	fl := opts
	fl.Frameless = true
	w = fakeWindow(g.b, 10, image.Pt(800, 600), fl)
	if got := g.f.take(); strings.Contains(got, "_NET_REQUEST_FRAME_EXTENTS") {
		t.Errorf("a frameless window asked for frame extents: %s", got)
	}
	g.feed()
	g.f.take()
	g.present(w)
	g.sent("frameless first Present", mapped(10, 100, 0))

	// A window manager without _NET_REQUEST_FRAME_EXTENTS.
	delete(g.b.supported, a.netReqFrameExt)
	w = fakeWindow(g.b, 11, image.Pt(800, 600), opts)
	g.sent("NewWindow without the request", "prop _NET_WM_WINDOW_TYPE win=11 = [_NET_WM_WINDOW_TYPE_NORMAL]")
	g.feed()
	g.f.take()
	g.present(w)
	g.sent("first Present without the request", mapped(11, 100, 0))

	// A position set before the first map is the app's: not fitted, and
	// no wait.
	g.b.supported[a.netReqFrameExt] = true
	w = fakeWindow(g.b, 12, image.Pt(800, 600), opts)
	w.SetPosition(image.Pt(5000, 5000))
	g.feed()
	g.f.take()
	g.present(w)
	g.sent("first Present after SetPosition", mapped(12, 5000, 5000))
}
