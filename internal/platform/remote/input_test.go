package remote

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// HostOptions.Input: consumed events do not reach the child, the pointer's
// hover ends and resumes cleanly around consumed ground.

// covered is the ground the test's Input consumes: pointer and scroll events
// from y = 50 down, and an Escape press.
func covered(e platform.Event) bool {
	switch e := e.(type) {
	case platform.PointerEvent:
		return e.Pos.Y >= 50
	case platform.ScrollEvent:
		return e.Pos.Y >= 50
	case platform.KeyEvent:
		return e.Key == platform.KeyEscape && e.Action == platform.KeyPress
	}
	return false
}

// seen summarizes the input events of a child's record.
func seen(evs []appEvent) []string {
	var out []string
	for _, e := range evs {
		switch e := e.e.(type) {
		case platform.PointerEvent:
			out = append(out, fmt.Sprintf("%s(%g,%g)", e.Action, e.Pos.X, e.Pos.Y))
		case platform.ScrollEvent:
			out = append(out, fmt.Sprintf("scroll(%g,%g)", e.Pos.X, e.Pos.Y))
		case platform.KeyEvent:
			out = append(out, fmt.Sprintf("key %s %s", e.Action, e.Key))
		}
	}
	return out
}

func TestInputConsumesEvents(t *testing.T) {
	var asked []platform.Event
	p := newPair(t, HostOptions{Input: func(w platform.Window, e platform.Event) bool {
		asked = append(asked, e) // UI thread only
		return covered(e)
	}}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 100, H: 100}})
	a.configured()
	hw := p.window(0)
	waitFrame(t, hw, "first frame", a.presentNow(w, 1, nil))
	hw.SetFocused(true)
	a.waitEvent("focus", isType[platform.FocusEvent])
	a.take()

	// settle injects a key the host does not consume and waits for it, so
	// everything injected before it has been handled.
	settle := func() []string {
		t.Helper()
		hw.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeKeyZ, Key: "z", Text: "z"})
		var evs []appEvent
		waitFor(t, "the sentinel key", func() bool {
			a.mu.Lock()
			defer a.mu.Unlock()
			for _, e := range a.events {
				if k, ok := e.e.(platform.KeyEvent); ok && k.Key == "z" {
					evs = a.events[:len(a.events)-1]
					a.events = nil
					return true
				}
			}
			return false
		})
		got := seen(evs)
		hw.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeKeyZ, Key: "z"})
		a.waitEvent("sentinel release", isType[platform.KeyEvent])
		return got
	}
	ptr := func(act platform.PointerAction, x, y float32) {
		hw.Inject(platform.PointerEvent{Action: act, Pos: platform.Point{X: x, Y: y}})
	}
	eq := func(what string, got []string, want ...string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: the child saw %v, want %v", what, got, want)
		}
	}

	// Outside the consumed ground everything goes through.
	ptr(platform.PointerEnter, 5, 5)
	ptr(platform.PointerMove, 6, 6)
	eq("uncovered", settle(), "enter(5,5)", "move(6,6)")

	// Moving onto it ends the hover where the child last saw the pointer;
	// the events there, scrolls included, are the host's.
	ptr(platform.PointerMove, 5, 60)
	ptr(platform.PointerDown, 5, 60)
	ptr(platform.PointerUp, 5, 60)
	hw.Inject(platform.ScrollEvent{Pos: platform.Point{X: 5, Y: 70}, Steps: platform.Point{Y: 1}})
	eq("covered", settle(), "leave(6,6)")

	// A scroll elsewhere is the child's.
	hw.Inject(platform.ScrollEvent{Pos: platform.Point{X: 5, Y: 10}, Steps: platform.Point{Y: 1}})
	// Coming back enters first, then moves.
	ptr(platform.PointerMove, 7, 10)
	ptr(platform.PointerMove, 8, 10)
	eq("back", settle(), "scroll(5,10)", "enter(7,10)", "move(7,10)", "move(8,10)")

	// Leaving the window from the covered ground is the child's leave
	// already: it gets one, not two, and the next event enters again.
	ptr(platform.PointerMove, 9, 80)
	ptr(platform.PointerLeave, 9, 80)
	ptr(platform.PointerEnter, 3, 3)
	eq("window leave", settle(), "leave(8,10)", "enter(3,3)")

	// A consumed key press never reaches the child, and the release that
	// follows is dropped by the child's view of its keyboard; other keys
	// pass.
	hw.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeEscape, Key: platform.KeyEscape})
	hw.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeEscape, Key: platform.KeyEscape})
	hw.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeKeyA, Key: "a", Text: "a"})
	hw.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeKeyA, Key: "a"})
	eq("keys", settle(), "key press a", "key release a")
}

func TestInputConsumedEventsAreNotQueuedWhileFrozen(t *testing.T) {
	p := newPair(t, HostOptions{Input: func(w platform.Window, e platform.Event) bool { return covered(e) }}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 100, H: 100}})
	a.configured()
	hw := p.window(0)
	waitFrame(t, hw, "first frame", a.presentNow(w, 1, nil))
	hw.SetFocused(true)
	a.waitEvent("focus", isType[platform.FocusEvent])
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 4, Y: 4}})
	a.waitEvent("move", isType[platform.PointerEvent])
	a.take()

	p.ui(p.host.Freeze)
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 4, Y: 90}}) // consumed: leave queued
	hw.Inject(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: 4, Y: 90}, Button: platform.ButtonPrimary, Buttons: 1})
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 4, Y: 5}}) // enter and move queued
	p.ui(func() {})
	if got := seen(a.take()); len(got) != 0 {
		t.Fatalf("the child saw %v while frozen", got)
	}
	p.ui(p.host.Unfreeze)
	// In order: the leave of the covered ground, the enter that ended it
	// and the move; never the press on the panel.
	var got []string
	waitFor(t, "the queued events", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		got = seen(a.events)
		return len(got) >= 3
	})
	if want := "[leave(4,4) enter(4,5) move(4,5)]"; fmt.Sprint(got) != want {
		t.Errorf("after the freeze the child saw %v, want %v", got, want)
	}
}

func TestNoInputOptionDeliversEverything(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	w := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 100, H: 100}})
	a.configured()
	hw := p.window(0)
	waitFrame(t, hw, "first frame", a.presentNow(w, 1, nil))
	a.take()
	hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 4, Y: 90}})
	got := a.waitEvent("move", isType[platform.PointerEvent]).(platform.PointerEvent)
	if got.Action != platform.PointerMove || got.Pos.Y != 90 {
		t.Errorf("got %+v", got)
	}
}
