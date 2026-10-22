package remote

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// The in-process pair: a Host on a real-time headless backend, run on its
// own goroutine (its UI thread), and child Backends dialed from the test,
// each run on a goroutine of its own driving a fake app. No upper layer is
// involved.

const waitTimeout = 10 * time.Second

type pair struct {
	t    *testing.T
	hb   *headless.Backend
	host *Host
	done chan error

	mu           sync.Mutex
	connected    []*Child
	disconnected map[*Child]error
	unhandled    []platform.Event
	errors       []error
}

func newPair(t *testing.T, opts HostOptions, hopts headless.Options) *pair {
	t.Helper()
	hopts.RealTime = true
	p := &pair{t: t, hb: headless.New(hopts), done: make(chan error, 1), disconnected: map[*Child]error{}}
	user := opts
	opts.OnConnect = func(c *Child) {
		p.mu.Lock()
		p.connected = append(p.connected, c)
		p.mu.Unlock()
		if user.OnConnect != nil {
			user.OnConnect(c)
		}
	}
	opts.OnDisconnect = func(c *Child, err error) {
		p.mu.Lock()
		p.disconnected[c] = err
		p.mu.Unlock()
		if user.OnDisconnect != nil {
			user.OnDisconnect(c, err)
		}
	}
	opts.OnUnhandled = func(w platform.Window, e platform.Event) {
		p.mu.Lock()
		p.unhandled = append(p.unhandled, e)
		p.mu.Unlock()
	}
	opts.OnError = func(err error) {
		p.mu.Lock()
		p.errors = append(p.errors, err)
		p.mu.Unlock()
	}
	h, err := NewHost(p.hb, opts)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	p.host = h
	go func() { p.done <- h.Run() }()
	t.Cleanup(func() {
		h.Close()
		p.hb.Quit()
		select {
		case err := <-p.done:
			if err != nil {
				t.Errorf("host Run: %v", err)
			}
		case <-time.After(waitTimeout):
			t.Errorf("host Run did not return")
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, err := range p.errors {
			t.Errorf("host error: %v", err)
		}
	})
	return p
}

// ui runs fn on the host's UI thread and waits for it.
func (p *pair) ui(fn func()) {
	p.t.Helper()
	ch := make(chan struct{})
	p.host.Post(func() { fn(); close(ch) })
	select {
	case <-ch:
	case <-time.After(waitTimeout):
		p.t.Fatalf("host UI thread did not run a posted function")
	}
}

// child returns the n-th connected child (0-based), waiting for it.
func (p *pair) child(n int) *Child {
	p.t.Helper()
	var c *Child
	waitFor(p.t, fmt.Sprintf("child %d to connect", n), func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.connected) > n {
			c = p.connected[n]
			return true
		}
		return false
	})
	return c
}

// window returns the n-th headless window, waiting for it.
func (p *pair) window(n int) *headless.Window {
	p.t.Helper()
	var w *headless.Window
	waitFor(p.t, fmt.Sprintf("host window %d", n), func() bool {
		ws := p.hb.Windows()
		if len(ws) > n {
			w = ws[n]
			return true
		}
		return false
	})
	return w
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// app is a fake app on a child Backend.
type app struct {
	t    *testing.T
	b    *Backend
	done chan error

	// react runs on the child's UI thread for every event (after it is
	// recorded). Set it before start.
	react func(a *app, w platform.Window, e platform.Event)

	mu     sync.Mutex
	events []appEvent
	calls  []func()
	win    platform.Window
	fbs    map[platform.Window]*image.RGBA
}

type appEvent struct {
	w platform.Window
	e platform.Event
}

func (p *pair) dial(tag string) *app {
	p.t.Helper()
	b, err := Dial(p.host.Addr(), DialOptions{Tag: tag})
	if err != nil {
		p.t.Fatalf("Dial: %v", err)
	}
	return &app{t: p.t, b: b, done: make(chan error, 1), fbs: map[platform.Window]*image.RGBA{}}
}

// start runs the app's loop. The app quits at cleanup.
func (a *app) start() *app {
	go func() {
		a.done <- a.b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
			if _, ok := e.(platform.WakeEvent); ok {
				a.mu.Lock()
				calls := a.calls
				a.calls = nil
				a.mu.Unlock()
				for _, fn := range calls {
					fn()
				}
				return
			}
			a.mu.Lock()
			a.events = append(a.events, appEvent{w, e})
			a.mu.Unlock()
			if a.react != nil {
				a.react(a, w, e)
			}
		}))
	}()
	a.t.Cleanup(func() {
		a.b.Quit()
		select {
		case <-a.done:
		case <-time.After(waitTimeout):
			a.t.Errorf("child Run did not return")
		}
	})
	return a
}

// ui runs fn on the child's UI thread and waits for it.
func (a *app) ui(fn func()) {
	a.t.Helper()
	ch := make(chan struct{})
	a.mu.Lock()
	a.calls = append(a.calls, func() { fn(); close(ch) })
	a.mu.Unlock()
	a.b.Wake()
	select {
	case <-ch:
	case <-time.After(waitTimeout):
		a.t.Fatalf("child UI thread did not run a function")
	}
}

// newWindow creates a window on the child's UI thread.
func (a *app) newWindow(opts platform.WindowOptions) platform.Window {
	a.t.Helper()
	var w platform.Window
	a.ui(func() {
		var err error
		w, err = a.b.NewWindow(opts)
		if err != nil {
			a.t.Errorf("NewWindow: %v", err)
		}
	})
	a.mu.Lock()
	if a.win == nil {
		a.win = w
	}
	a.mu.Unlock()
	return w
}

// take returns and forgets the recorded events.
func (a *app) take() []appEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	ev := a.events
	a.events = nil
	return ev
}

// waitEvent waits until an event satisfying match has been recorded and
// returns it; earlier events are dropped.
func (a *app) waitEvent(what string, match func(platform.Event) bool) platform.Event {
	a.t.Helper()
	var got platform.Event
	waitFor(a.t, what, func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		for i, e := range a.events {
			if match(e.e) {
				got = e.e
				a.events = a.events[i+1:]
				return true
			}
		}
		return false
	})
	return got
}

func isType[T platform.Event](e platform.Event) bool { _, ok := e.(T); return ok }

// paint returns a framebuffer for w filled with a pattern that depends on
// seed and the pixel position, reusing the app's buffer.
func (a *app) paint(w platform.Window, seed byte) *image.RGBA {
	m := w.Metrics()
	fb := a.fbs[w]
	if fb == nil || fb.Rect.Max != m.Physical() {
		fb = image.NewRGBA(image.Rectangle{Max: m.Physical()})
		a.fbs[w] = fb
	}
	pattern(fb, fb.Rect, seed)
	return fb
}

// pattern fills r of img with a position- and seed-dependent opaque
// pattern.
func pattern(img *image.RGBA, r image.Rectangle, seed byte) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, color.RGBA{byte(x*7) ^ seed, byte(y*13) + seed, byte(x+y) ^ (seed * 3), 255})
		}
	}
}

// presentNow paints and presents w on the child's UI thread.
func (a *app) presentNow(w platform.Window, seed byte, damage []image.Rectangle) *image.RGBA {
	a.t.Helper()
	var out *image.RGBA
	a.ui(func() {
		fb := a.paint(w, seed)
		if err := w.Present(fb, damage); err != nil {
			a.t.Errorf("Present: %v", err)
		}
		out = image.NewRGBA(fb.Rect)
		copy(out.Pix, fb.Pix)
	})
	return out
}

// waitFrame waits until the headless window shows want.
func waitFrame(t *testing.T, hw *headless.Window, what string, want *image.RGBA) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for {
		got := hw.Frame()
		if got != nil && got.Rect == want.Rect && bytes.Equal(got.Pix, want.Pix) {
			return
		}
		if time.Now().After(deadline) {
			if got == nil {
				t.Fatalf("%s: nothing presented", what)
			}
			t.Fatalf("%s: frame %v differs from the expected %v (%s)", what, got.Rect, want.Rect, firstDiff(got, want))
		}
		time.Sleep(time.Millisecond)
	}
}

func firstDiff(a, b *image.RGBA) string {
	if a.Rect != b.Rect {
		return "bounds differ"
	}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return fmt.Sprintf("first difference at (%d,%d): got %v, want %v", x, y, a.RGBAAt(x, y), b.RGBAAt(x, y))
			}
		}
	}
	return "no pixel differs"
}

// configured waits for the child window's first ConfigureEvent.
func (a *app) configured() platform.ConfigureEvent {
	a.t.Helper()
	return a.waitEvent("ConfigureEvent", isType[platform.ConfigureEvent]).(platform.ConfigureEvent)
}
