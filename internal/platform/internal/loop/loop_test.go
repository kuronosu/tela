package loop

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// win is a minimal backend window.
type win struct {
	WindowState
	name string
}

func (w *win) Present(*image.RGBA, []image.Rectangle) error { return nil }
func (w *win) SetTitle(string)                              {}
func (w *win) SetCursor(platform.Cursor)                    {}
func (w *win) SetSize(platform.Size)                        {}
func (w *win) Close()                                       {}

// rig is a Loop with a manual clock and a recording handler.
type rig struct {
	t       *testing.T
	l       *Loop[*win]
	clock   time.Duration
	notifed int
	log     []string
	on      func(w platform.Window, e platform.Event)
}

func newRig(t *testing.T, pace func(*win, time.Duration) bool) *rig {
	r := &rig{t: t}
	r.l = New(Options[*win]{
		Now:      func() time.Duration { return r.clock },
		Notify:   func() { r.notifed++ },
		Interval: 10 * time.Millisecond,
		Pace:     pace,
	})
	return r
}

var metrics = platform.Metrics{Size: platform.Size{W: 10, H: 10}, Scale: 1}

func (r *rig) window(name string) *win {
	w := &win{name: name}
	r.l.Add(w)
	return w
}

func (r *rig) HandleEvent(w platform.Window, e platform.Event) {
	name := "app"
	if w != nil {
		name = w.(*win).name
	}
	var s string
	switch e := e.(type) {
	case platform.ConfigureEvent:
		s = fmt.Sprintf("%s configure %v", name, e.Size)
		if w.Metrics() != e.Metrics {
			r.t.Errorf("Metrics() = %v during ConfigureEvent %v", w.Metrics(), e.Metrics)
		}
	case platform.FrameEvent:
		s = fmt.Sprintf("%s frame %v", name, e.Time)
	default:
		s = fmt.Sprintf("%s %T", name, e)
	}
	r.log = append(r.log, strings.TrimPrefix(s, "app platform."))
	if r.on != nil {
		r.on(w, e)
	}
}

func (r *rig) pass() string {
	r.t.Helper()
	r.l.Deliver(r)
	s := strings.Join(r.log, "; ")
	r.log = nil
	return s
}

func (r *rig) expect(want string) {
	r.t.Helper()
	if got := r.pass(); got != want {
		r.t.Errorf("pass:\n got %s\nwant %s", got, want)
	}
}

func TestDeliveryOrder(t *testing.T) {
	r := newRig(t, nil)
	a, b := r.window("a"), r.window("b")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	r.l.Enqueue(nil, platform.SettingsEvent{})
	r.l.Enqueue(b, platform.ConfigureEvent{Metrics: metrics})
	r.l.Post(func() { r.log = append(r.log, "posted") })
	r.l.Enqueue(a, platform.FocusEvent{Focused: true})
	r.l.Wake()
	r.l.Wake()
	if r.notifed != 2 {
		t.Errorf("Wake notified %d times, want once per call", r.notifed)
	}
	if !r.l.Ready() {
		t.Fatalf("not ready with queued events")
	}
	// Wake first (coalesced), then FIFO, then frames in creation order:
	// the ConfigureEvents forced them.
	r.expect("WakeEvent; a configure 10x10; SettingsEvent; b configure 10x10; posted; a platform.FocusEvent; a frame 0s; b frame 0s")
	if r.l.Ready() {
		t.Errorf("ready after a full pass")
	}
	if !a.NeedsFull() {
		t.Errorf("a ConfigureEvent does not force a full Present")
	}
}

func TestWorkQueuedDuringPassWaits(t *testing.T) {
	r := newRig(t, nil)
	a := r.window("a")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	r.on = func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.ConfigureEvent); ok {
			r.l.Enqueue(a, platform.CloseRequestedEvent{})
			r.l.Wake()
		}
	}
	r.expect("a configure 10x10; a frame 0s")
	r.on = nil
	r.expect("WakeEvent; a platform.CloseRequestedEvent")
}

func TestReconfigureCoalesces(t *testing.T) {
	r := newRig(t, nil)
	a := r.window("a")
	r.l.Reconfigure(a, metrics)
	r.l.Enqueue(a, platform.FocusEvent{})
	big := platform.Metrics{Size: platform.Size{W: 20, H: 30}, Scale: 2}
	r.l.Reconfigure(a, big)
	// One ConfigureEvent, at the position of the first, with the latest
	// metrics.
	r.expect("a configure 20x30; a platform.FocusEvent; a frame 0s")
	// Back and forth before delivery: nothing changed, nothing delivered.
	r.l.Reconfigure(a, metrics)
	r.l.Reconfigure(a, big)
	r.expect("")
	if a.FramePending() {
		t.Errorf("an unchanged coalesced configure forced a frame")
	}
}

func TestFramePacing(t *testing.T) {
	r := newRig(t, nil)
	a := r.window("a")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	r.expect("a configure 10x10; a frame 0s")

	// Requested within the interval: paced by the timer.
	r.clock = 3 * time.Millisecond
	a.RequestFrame()
	a.RequestFrame()
	if r.l.Ready() {
		t.Errorf("a request within the interval is due at once")
	}
	if d, ok := r.l.Pace(); !ok || d != 10*time.Millisecond {
		t.Errorf("Pace = %v, %v; want 10ms", d, ok)
	}
	r.expect("")
	r.clock = 10 * time.Millisecond
	r.expect("a frame 10ms")

	// Not drawn recently: due on the next pass.
	r.clock = 100 * time.Millisecond
	a.RequestFrame()
	r.expect("a frame 100ms")

	// Idle: no deadline, not ready.
	if _, ok := r.l.Pace(); ok || r.l.Ready() {
		t.Errorf("idle loop has a deadline or is ready")
	}

	// Hidden windows keep their request until shown again.
	a.RequestFrame()
	a.SetHidden(true)
	r.clock = 200 * time.Millisecond
	if _, ok := r.l.Pace(); ok || r.l.Ready() {
		t.Errorf("a hidden window is paced")
	}
	a.SetHidden(false)
	r.expect("a frame 200ms")
}

func TestEmptyWindowGetsNoFrames(t *testing.T) {
	r := newRig(t, nil)
	a := r.window("a")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: platform.Metrics{Scale: 1}})
	r.expect("a configure 0x0")
	a.RequestFrame()
	if _, ok := r.l.Pace(); ok || r.l.Ready() {
		t.Errorf("an empty window is paced")
	}
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	r.expect("a configure 10x10; a frame 0s")
}

func TestVsyncGate(t *testing.T) {
	var paced []string
	hook := true
	r := newRig(t, func(w *win, now time.Duration) bool {
		paced = append(paced, w.name)
		if hook {
			w.VsyncRequested()
		}
		return hook
	})
	a := r.window("a")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	r.expect("a configure 10x10; a frame 0s")

	// Not due: the hook arranges a display signal, no timer.
	a.RequestFrame()
	if d, ok := r.l.Pace(); ok {
		t.Errorf("Pace returned a deadline %v although the hook arranged a signal", d)
	}
	if len(paced) != 1 || !a.VsyncPending() {
		t.Fatalf("hook calls %v, pending %v", paced, a.VsyncPending())
	}
	// Even after the interval, a pending signal holds the frame back.
	r.clock = time.Second
	if r.l.Ready() {
		t.Errorf("a frame is due while the display signal is outstanding")
	}
	r.l.Pace()
	if len(paced) != 1 {
		t.Errorf("the hook was offered a frame that already waits for a signal")
	}
	a.VsyncDone()
	r.expect("a frame 1s")

	// A signal that arrived since the last frame makes the next request
	// due at once, even within the interval.
	a.VsyncRequested()
	a.VsyncDone()
	a.RequestFrame()
	r.expect("a frame 1s")

	// Without a signal, the hook declines: timer pacing.
	hook = false
	a.RequestFrame()
	if d, ok := r.l.Pace(); !ok || d != time.Second+10*time.Millisecond {
		t.Errorf("Pace = %v, %v; want the interval deadline", d, ok)
	}
}

func TestRemoveDropsEvents(t *testing.T) {
	r := newRig(t, nil)
	a, b := r.window("a"), r.window("b")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	r.l.Enqueue(b, platform.ConfigureEvent{Metrics: metrics})
	r.expect("a configure 10x10; b configure 10x10; a frame 0s; b frame 0s")

	r.l.Enqueue(a, platform.FocusEvent{})
	r.l.Enqueue(b, platform.FocusEvent{})
	r.l.Reconfigure(a, platform.Metrics{Size: platform.Size{W: 1, H: 1}, Scale: 1})
	r.l.Remove(a)
	a.RequestFrame()
	r.l.Enqueue(a, platform.FocusEvent{}) // dropped
	r.expect("b platform.FocusEvent")
	if !a.Closed() || len(r.l.Windows()) != 1 || a.FramePending() {
		t.Errorf("after Remove: closed %v, windows %d, frame pending %v", a.Closed(), len(r.l.Windows()), a.FramePending())
	}

	// Closing a window inside the pass skips its later events and frames.
	r.l.Enqueue(b, platform.CloseRequestedEvent{})
	r.l.Enqueue(b, platform.FocusEvent{})
	b.RequestFrame()
	r.on = func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.CloseRequestedEvent); ok {
			r.l.Remove(b)
		}
	}
	r.expect("b platform.CloseRequestedEvent")
}

func TestCoalesce(t *testing.T) {
	r := newRig(t, nil)
	a, b := r.window("a"), r.window("b")
	move := func(x float32) platform.PointerEvent {
		return platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: x}}
	}
	isMove := func(e platform.Event) bool {
		p, ok := e.(platform.PointerEvent)
		return ok && p.Action == platform.PointerMove
	}
	r.l.Coalesce(a, move(1), isMove)
	r.l.Coalesce(a, move(2), isMove) // replaces 1
	r.l.Coalesce(b, move(3), isMove) // another window: appended
	r.l.Coalesce(b, move(4), isMove) // replaces 3
	r.l.Post(func() {})
	r.l.Coalesce(b, move(5), isMove) // after a callback: appended
	var got []float32
	r.on = func(_ platform.Window, e platform.Event) {
		if p, ok := e.(platform.PointerEvent); ok {
			got = append(got, p.Pos.X)
		}
	}
	r.pass()
	if fmt.Sprint(got) != "[2 4 5]" {
		t.Errorf("coalesced moves = %v, want [2 4 5]", got)
	}
}

func TestQuitStopsThePass(t *testing.T) {
	r := newRig(t, nil)
	a := r.window("a")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	r.l.Enqueue(a, platform.FocusEvent{})
	r.on = func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.ConfigureEvent); ok {
			r.l.Quit()
		}
	}
	r.expect("a configure 10x10")
	if !r.l.Ready() || !r.l.Quitting() {
		t.Errorf("Quit: ready %v, quitting %v", r.l.Ready(), r.l.Quitting())
	}
	r.l.Wake()
	r.expect("") // nothing after Quit, not even a WakeEvent
}

func TestDeliverIsNotReentrant(t *testing.T) {
	r := newRig(t, nil)
	a := r.window("a")
	r.l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	nested := -1
	r.on = func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.ConfigureEvent); ok {
			// A Win32 WndProc running inside SetWindowPos would do this.
			r.l.Enqueue(a, platform.FocusEvent{})
			nested = r.l.Deliver(r)
		}
	}
	r.expect("a configure 10x10; a frame 0s")
	if nested != 0 {
		t.Errorf("nested Deliver delivered %d events", nested)
	}
	r.on = nil
	r.expect("a platform.FocusEvent")
}

func TestLockIsReleasedAroundHandler(t *testing.T) {
	var mu sync.Mutex
	clock := time.Duration(0)
	l := New(Options[*win]{
		Now:    func() time.Duration { return clock }, // reads under mu
		Notify: func() {},
		Lock:   &mu,
	})
	a := &win{name: "a"}
	mu.Lock()
	l.Add(a)
	l.Enqueue(a, platform.ConfigureEvent{Metrics: metrics})
	l.Post(func() {
		if !mu.TryLock() {
			t.Errorf("the lock is held while a posted callback runs")
			return
		}
		mu.Unlock()
	})
	mu.Unlock()
	n := l.Deliver(platform.HandlerFunc(func(platform.Window, platform.Event) {
		if !mu.TryLock() {
			t.Errorf("the lock is held while the handler runs")
			return
		}
		mu.Unlock()
	}))
	if n != 3 {
		t.Errorf("Deliver = %d, want 3 (configure, callback, frame)", n)
	}
}
