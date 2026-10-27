package headless

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// recorder is a Handler that records events as strings and can run an
// optional reaction.
type recorder struct {
	events []string
	react  func(w platform.Window, e platform.Event)
}

func (r *recorder) HandleEvent(w platform.Window, e platform.Event) {
	name := "app"
	if w != nil {
		name = w.(*Window).Title()
	}
	r.events = append(r.events, fmt.Sprintf("%s:%s", name, describe(e)))
	if r.react != nil {
		r.react(w, e)
	}
}

func (r *recorder) take() string {
	s := strings.Join(r.events, " ")
	r.events = nil
	return s
}

func describe(e platform.Event) string {
	switch e := e.(type) {
	case platform.ConfigureEvent:
		return "configure(" + e.Metrics.String() + ")"
	case platform.FrameEvent:
		return fmt.Sprintf("frame(%v)", e.Time)
	case platform.PointerEvent:
		return fmt.Sprintf("pointer(%v %v,%v)", e.Action, e.Pos.X, e.Pos.Y)
	case platform.ScrollEvent:
		return fmt.Sprintf("scroll(%v,%v)", e.Steps.X, e.Steps.Y)
	case platform.FocusEvent:
		return fmt.Sprintf("focus(%v)", e.Focused)
	case platform.CloseRequestedEvent:
		return "close"
	case platform.WakeEvent:
		return "wake"
	case platform.SettingsEvent:
		return fmt.Sprintf("settings(dark=%v)", e.Settings.DarkMode)
	case platform.WindowStateEvent:
		return "state" + e.State.String()
	case platform.NotificationEvent:
		return fmt.Sprintf("notification(%d %v %s)", e.ID, e.Kind, e.Action)
	case platform.TrayEvent:
		return fmt.Sprintf("tray(%d %v %d)", e.Tray, e.Kind, e.Item)
	}
	return fmt.Sprintf("%T", e)
}

func newWindow(t *testing.T, b *Backend, opts platform.WindowOptions) *Window {
	t.Helper()
	w, err := b.NewWindow(opts)
	if err != nil {
		t.Fatal(err)
	}
	return w.(*Window)
}

func fill(fb *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			fb.SetRGBA(x, y, c)
		}
	}
}

func expectPanic(t *testing.T, contains string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Errorf("no panic, want one containing %q", contains)
			return
		}
		if s := fmt.Sprint(r); !strings.Contains(s, contains) {
			t.Errorf("panic %q does not contain %q", s, contains)
		}
	}()
	f()
}

var (
	red  = color.RGBA{0xff, 0, 0, 0xff}
	blue = color.RGBA{0, 0, 0xff, 0xff}
)

func TestNewWindowConfigureThenFrame(t *testing.T) {
	b := New(Options{Size: platform.Size{W: 200, H: 100}, Scale: 2})
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	if w.Metrics() != (platform.Metrics{}) {
		t.Errorf("Metrics before the first ConfigureEvent = %v, want zero", w.Metrics())
	}
	r := &recorder{}
	if n := b.Step(r); n != 2 {
		t.Errorf("Step delivered %d, want 2", n)
	}
	if got, want := r.take(), "w:configure(200x100@2x (400x200 px)) w:frame(0s)"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
	if got := w.Metrics().Physical(); got != image.Pt(400, 200) {
		t.Errorf("physical = %v", got)
	}
	if b.Pending() || b.Step(r) != 0 {
		t.Errorf("idle backend has pending work")
	}
}

func TestRequestFrameCoalescesAndPaces(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{}
	b.Drain(r)
	r.take()

	for range 5 {
		w.RequestFrame()
	}
	// The last frame was at t=0: the next one waits for the refresh
	// interval.
	if n := b.Drain(r); n != 0 {
		t.Fatalf("frame delivered before it was due: %s", r.take())
	}
	if !w.FrameRequested() {
		t.Errorf("request lost")
	}
	if next, ok := b.NextFrameTime(); !ok || next != DefaultFrameInterval {
		t.Errorf("NextFrameTime = %v, %v; want %v", next, ok, DefaultFrameInterval)
	}
	b.Advance(DefaultFrameInterval - 1)
	if b.Pending() {
		t.Errorf("frame due 1ns early")
	}
	b.Advance(1)
	b.Drain(r)
	if got, want := r.take(), fmt.Sprintf("w:frame(%v)", DefaultFrameInterval); got != want {
		t.Errorf("events = %q, want %q", got, want)
	}

	// After an idle period the next request is served immediately.
	b.Advance(time.Second)
	w.RequestFrame()
	w.RequestFrame()
	b.Drain(r)
	if got, want := r.take(), fmt.Sprintf("w:frame(%v)", time.Second+DefaultFrameInterval); got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
	if _, ok := b.NextFrameTime(); ok {
		t.Errorf("NextFrameTime reports a frame with no request")
	}
}

func TestAnimationOnlyProgressesWithClock(t *testing.T) {
	b := New(Options{FrameInterval: 10 * time.Millisecond})
	var w *Window
	frames := 0
	r := &recorder{react: func(_ platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); ok {
			frames++
			w.RequestFrame() // animate forever
		}
	}}
	w = newWindow(t, b, platform.WindowOptions{Title: "w"})
	b.Drain(r) // must not loop forever
	if frames != 1 {
		t.Fatalf("frames = %d, want 1", frames)
	}
	for range 3 {
		b.Advance(10 * time.Millisecond)
		b.Drain(r)
	}
	if frames != 4 {
		t.Errorf("frames = %d, want 4", frames)
	}
	if !strings.HasSuffix(r.take(), "w:frame(30ms)") {
		t.Errorf("last frame not at 30ms")
	}
}

func TestPresentCopiesAndTracksDamage(t *testing.T) {
	b := New(Options{Size: platform.Size{W: 8, H: 4}})
	w := newWindow(t, b, platform.WindowOptions{})
	r := &recorder{}
	b.Drain(r)

	if w.Frame() != nil || w.Presents() != 0 {
		t.Fatalf("frame before any Present")
	}
	fb := image.NewRGBA(image.Rect(0, 0, 8, 4))
	fill(fb, fb.Rect, red)
	if err := w.Present(fb, []image.Rectangle{image.Rect(0, 0, 1, 1)}); err != nil {
		t.Fatal(err)
	}
	// The first Present after a ConfigureEvent is full whatever the damage.
	if got := w.Damage(); len(got) != 1 || got[0] != fb.Rect {
		t.Errorf("first damage = %v, want full", got)
	}
	// The backend copied: changing fb does not change the stored frame.
	fill(fb, fb.Rect, blue)
	if got := w.Frame().RGBAAt(3, 3); got != red {
		t.Errorf("stored frame changed with the caller's buffer: %v", got)
	}

	// Partial damage: only the damaged pixels are taken from fb.
	if err := w.Present(fb, []image.Rectangle{image.Rect(2, 1, 4, 3), image.Rect(7, 3, 20, 20)}); err != nil {
		t.Fatal(err)
	}
	want := []image.Rectangle{image.Rect(2, 1, 4, 3), image.Rect(7, 3, 8, 4)}
	if got := w.Damage(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("damage = %v, want %v", got, want)
	}
	frame := w.Frame()
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			wantC := red
			if image.Pt(x, y).In(want[0]) || image.Pt(x, y).In(want[1]) {
				wantC = blue
			}
			if got := frame.RGBAAt(x, y); got != wantC {
				t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, wantC)
			}
		}
	}
	// Frame returns a copy too.
	frame.SetRGBA(0, 0, blue)
	if w.Frame().RGBAAt(0, 0) != red {
		t.Errorf("Frame() exposes internal storage")
	}

	// nil damage is the full frame.
	if err := w.Present(fb, nil); err != nil {
		t.Fatal(err)
	}
	if got := w.Damage(); len(got) != 1 || got[0] != fb.Rect || w.Presents() != 3 {
		t.Errorf("nil damage = %v (presents %d)", got, w.Presents())
	}
}

// A Present whose damage lies entirely outside the frame changes nothing:
// like the Wayland and X11 backends, headless returns early, and Damage and
// Presents keep describing the last real Present (nil damage would mean
// "full frame").
func TestPresentFullyClippedDamageIsNoop(t *testing.T) {
	b := New(Options{Size: platform.Size{W: 8, H: 4}})
	w := newWindow(t, b, platform.WindowOptions{})
	b.Drain(&recorder{})

	fb := image.NewRGBA(image.Rect(0, 0, 8, 4))
	fill(fb, fb.Rect, red)
	if err := w.Present(fb, nil); err != nil {
		t.Fatal(err)
	}
	fill(fb, fb.Rect, blue)
	part := []image.Rectangle{image.Rect(1, 1, 2, 2)}
	if err := w.Present(fb, part); err != nil {
		t.Fatal(err)
	}
	for _, damage := range [][]image.Rectangle{
		{image.Rect(8, 0, 12, 4)},                          // right of the frame
		{image.Rect(-5, -5, 0, 0), image.Rect(0, 4, 8, 9)}, // above-left and below
		{image.Rectangle{}},                                // one empty rectangle
	} {
		if err := w.Present(fb, damage); err != nil {
			t.Fatalf("Present(%v): %v", damage, err)
		}
		if got := w.Presents(); got != 2 {
			t.Errorf("Present(%v) was counted: Presents() = %d, want 2", damage, got)
		}
		if got := w.Damage(); fmt.Sprint(got) != fmt.Sprint(part) {
			t.Errorf("Present(%v) changed Damage() to %v, want the last real damage %v", damage, got, part)
		}
		if got := w.Frame().RGBAAt(0, 0); got != red {
			t.Errorf("Present(%v) changed pixels outside every damage: %v", damage, got)
		}
	}
}

func TestPresentProgrammerErrors(t *testing.T) {
	b := New(Options{Size: platform.Size{W: 10, H: 10}})
	w := newWindow(t, b, platform.WindowOptions{})
	expectPanic(t, "before the window's first ConfigureEvent", func() {
		_ = w.Present(image.NewRGBA(image.Rect(0, 0, 10, 10)), nil)
	})
	b.Drain(&recorder{})
	expectPanic(t, "do not match the window", func() {
		_ = w.Present(image.NewRGBA(image.Rect(0, 0, 11, 10)), nil)
	})
	expectPanic(t, "nil framebuffer", func() { _ = w.Present(nil, nil) })
	expectPanic(t, "invalid cursor", func() { w.SetCursor(platform.Cursor(200)) })
}

func TestResizeAndScaleUseSizingRule(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{Title: "w", Size: platform.Size{W: 101, H: 51}, MinSize: platform.Size{W: 50, H: 20}})
	r := &recorder{}
	b.Drain(r)
	r.take()

	w.SetScale(1.5)
	if w.Metrics().Scale != 1 {
		t.Errorf("Metrics changed before the ConfigureEvent was delivered")
	}
	b.Drain(r)
	// 101*1.5 = 151.5 and 51*1.5 = 76.5 round half away from zero.
	if got, want := r.take(), "w:configure(101x51@1.5x (152x77 px)) w:frame(0s)"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
	fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	if err := w.Present(fb, nil); err != nil {
		t.Fatal(err)
	}

	w.Resize(platform.Size{W: 10, H: 300}) // clamped to MinSize
	w.Resize(platform.Size{W: 10, H: 300}) // no change: no event
	b.Drain(r)
	if got, want := r.take(), "w:configure(50x300@1.5x (75x450 px)) w:frame(0s)"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
	// The old framebuffer no longer fits.
	expectPanic(t, "do not match the window", func() { _ = w.Present(fb, nil) })
	fb = image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	if err := w.Present(fb, []image.Rectangle{image.Rect(0, 0, 1, 1)}); err != nil {
		t.Fatal(err)
	}
	if got := w.Damage(); len(got) != 1 || got[0] != fb.Rect {
		t.Errorf("first Present after resize must be full, damage = %v", got)
	}

	// SetSize from the app goes through the same path.
	w.SetSize(platform.Size{W: 60, H: 30})
	b.Drain(r)
	if got, want := r.take(), "w:configure(60x30@1.5x (90x45 px)) w:frame(0s)"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestEmptyWindowGetsNoFrames(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{Title: "w", Size: platform.Size{W: 10, H: 10}})
	r := &recorder{}
	b.Drain(r)
	r.take()

	w.Resize(platform.Size{}) // minimized
	b.Advance(time.Second)
	b.Drain(r)
	w.RequestFrame()
	b.Advance(time.Second)
	b.Drain(r)
	if got, want := r.take(), "w:configure(0x0@1x (0x0 px))"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
	w.Resize(platform.Size{W: 10, H: 10})
	b.Drain(r)
	if got, want := r.take(), "w:configure(10x10@1x (10x10 px)) w:frame(2s)"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestEventOrdering(t *testing.T) {
	b := New(Options{})
	a := newWindow(t, b, platform.WindowOptions{Title: "a"})
	c := newWindow(t, b, platform.WindowOptions{Title: "c"})
	r := &recorder{}
	b.Drain(r)
	r.take()

	b.Advance(time.Second)
	a.Inject(platform.PointerEvent{Action: platform.PointerEnter, Pos: platform.Point{X: 1, Y: 2}})
	c.Inject(platform.ScrollEvent{Steps: platform.Point{Y: 1}})
	a.Inject(platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Pos: platform.Point{X: 3, Y: 4}})
	c.RequestFrame() // frames come after the iteration's events
	a.SetFocused(true)
	a.SetFocused(true) // no change: no event
	b.SetSettings(platform.SystemSettings{DarkMode: true})
	b.Wake() // wakes come first in an iteration
	a.RequestClose()
	a.RequestFrame()

	b.Step(r)
	want := "app:wake a:pointer(enter 1,2) c:scroll(0,1) a:pointer(down 3,4) a:focus(true) app:settings(dark=true) a:close a:frame(1s) c:frame(1s)"
	if got := r.take(); got != want {
		t.Errorf("events =\n  %s\nwant\n  %s", got, want)
	}
	if !b.Settings().DarkMode {
		t.Errorf("Settings not updated")
	}
}

func TestInjectStampsTime(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	b.Drain(&recorder{})
	b.Advance(42 * time.Millisecond)
	w.Inject(platform.PointerEvent{Action: platform.PointerMove})
	w.Inject(platform.ScrollEvent{Time: 7})
	var times []time.Duration
	b.Drain(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			times = append(times, e.Time)
		case platform.ScrollEvent:
			times = append(times, e.Time)
		}
	}))
	if fmt.Sprint(times) != "[42ms 7ns]" {
		t.Errorf("times = %v", times)
	}
	for _, e := range []platform.Event{nil, platform.FrameEvent{}, platform.ConfigureEvent{}, platform.WakeEvent{}, platform.SettingsEvent{}} {
		expectPanic(t, "Inject", func() { w.Inject(e) })
	}
}

func TestCloseDropsEvents(t *testing.T) {
	b := New(Options{})
	a := newWindow(t, b, platform.WindowOptions{Title: "a"})
	c := newWindow(t, b, platform.WindowOptions{Title: "c"})
	r := &recorder{}
	b.Drain(r)
	r.take()

	a.Inject(platform.PointerEvent{Action: platform.PointerMove})
	c.Inject(platform.PointerEvent{Action: platform.PointerMove})
	a.RequestFrame()
	a.Close()
	a.Close()                                                     // idempotent
	a.Inject(platform.PointerEvent{Action: platform.PointerMove}) // dropped
	a.Resize(platform.Size{W: 1, H: 1})                           // no-op
	a.RequestFrame()                                              // no-op
	if err := a.Present(image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Errorf("Present on a closed window: %v", err)
	}
	b.Drain(r)
	if got, want := r.take(), "c:pointer(move 0,0)"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
	if ws := b.Windows(); len(ws) != 1 || ws[0] != c || !a.Closed() {
		t.Errorf("Windows() = %v", ws)
	}
}

func TestCloseInsideHandlerSkipsLaterEvents(t *testing.T) {
	b := New(Options{})
	a := newWindow(t, b, platform.WindowOptions{Title: "a"})
	r := &recorder{react: func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.CloseRequestedEvent); ok {
			w.Close()
		}
	}}
	b.Drain(r)
	r.take()
	a.RequestClose()
	a.Inject(platform.PointerEvent{Action: platform.PointerMove})
	a.RequestFrame()
	b.Advance(time.Second)
	b.Drain(r)
	if got, want := r.take(), "a:close"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestWakeCoalescesAcrossGoroutines(t *testing.T) {
	b := New(Options{})
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				b.Wake()
			}
		}()
	}
	wg.Wait()
	r := &recorder{}
	b.Drain(r)
	if got := r.take(); got != "app:wake" {
		t.Errorf("events = %q, want a single wake", got)
	}
	// A Wake from inside the handler yields another WakeEvent, not a lost
	// one.
	r.react = func(_ platform.Window, e platform.Event) {
		if _, ok := e.(platform.WakeEvent); ok && len(r.events) == 1 {
			b.Wake()
		}
	}
	b.Wake()
	b.Drain(r)
	if got := r.take(); got != "app:wake app:wake" {
		t.Errorf("events = %q", got)
	}
}

func TestClipboardIsAsynchronous(t *testing.T) {
	b := New(Options{})
	cb := b.Clipboard()
	if err := cb.WriteText("hello"); err != nil {
		t.Fatal(err)
	}
	var got []string
	cb.ReadText(func(s string, err error) { got = append(got, s) })
	if len(got) != 0 {
		t.Fatalf("ReadText called back synchronously")
	}
	_ = cb.WriteText("world") // the read sees the clipboard at delivery
	if n := b.Drain(&recorder{}); n != 1 || fmt.Sprint(got) != "[world]" {
		t.Errorf("delivered %d, got %v", n, got)
	}
	expectPanic(t, "nil callback", func() { cb.ReadText(nil) })
}

func TestDrainDetectsRunawayHandler(t *testing.T) {
	b := New(Options{})
	h := platform.HandlerFunc(func(_ platform.Window, e platform.Event) { b.Wake() })
	b.Wake()
	expectPanic(t, "did not settle", func() { b.Drain(h) })
}

func TestReentrantSteppingPanics(t *testing.T) {
	b := New(Options{})
	newWindow(t, b, platform.WindowOptions{})
	var inner any
	b.Drain(platform.HandlerFunc(func(platform.Window, platform.Event) {
		func() {
			defer func() { inner = recover() }()
			b.Step(&recorder{})
		}()
	}))
	if s := fmt.Sprint(inner); !strings.Contains(s, "from inside a handler") {
		t.Errorf("nested Step: recover() = %v", inner)
	}
}

func TestQuitStopsStepping(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	r := &recorder{react: func(platform.Window, platform.Event) { b.Quit() }}
	if n := b.Step(r); n != 1 {
		t.Errorf("Step after Quit in the first handler call delivered %d, want 1", n)
	}
	w.Inject(platform.PointerEvent{})
	if b.Pending() || b.Drain(r) != 0 || !b.Quitting() {
		t.Errorf("work delivered after Quit")
	}
}

// runAsync runs b.Run(h) on a new goroutine and returns a channel with its
// result.
func runAsync(b *Backend, h platform.Handler) <-chan error {
	done := make(chan error, 1)
	go func() { done <- b.Run(h) }()
	return done
}

func waitDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Run did not return")
	}
}

func TestRunQuitsWhenAppClosesLastWindow(t *testing.T) {
	// Run itself never quits on the last close; the app's policy does.
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	h := platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.CloseRequestedEvent); ok {
			w.Close()
			if len(b.Windows()) == 0 {
				b.Quit()
			}
		}
	})
	done := runAsync(b, h)
	w.RequestClose() // from another goroutine, while Run blocks
	waitDone(t, done)
	if !w.Closed() {
		t.Errorf("window not closed")
	}
	expectPanic(t, "Run called twice", func() { _ = b.Run(h) })
}

func TestRunBlocksWhenIdle(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	frames := make(chan time.Duration, 16)
	done := runAsync(b, platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		if f, ok := e.(platform.FrameEvent); ok {
			_ = w.Present(image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()}), nil)
			frames <- f.Time
			w.RequestFrame() // keep animating: only the clock can pace it
		}
	}))
	if got := <-frames; got != 0 {
		t.Errorf("first frame at %v", got)
	}

	// The loop is now blocked: an outstanding frame request with a manual
	// clock that nobody advances must not make it spin.
	time.Sleep(20 * time.Millisecond)
	before := b.iterations.Load()
	time.Sleep(100 * time.Millisecond)
	if after := b.iterations.Load(); after != before {
		t.Errorf("idle Run looped %d times in 100ms", after-before)
	}

	// Advancing the clock from another goroutine releases exactly the due
	// frame.
	b.Advance(DefaultFrameInterval)
	select {
	case got := <-frames:
		if got != DefaultFrameInterval {
			t.Errorf("paced frame at %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Advance did not wake Run")
	}
	b.Quit()
	waitDone(t, done)
	if w.Presents() != 2 {
		t.Errorf("presents = %d", w.Presents())
	}
}

func TestRunRealTimePacing(t *testing.T) {
	const interval = 5 * time.Millisecond
	b := New(Options{RealTime: true, FrameInterval: interval})
	newWindow(t, b, platform.WindowOptions{})
	var times []time.Duration
	done := runAsync(b, platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		if f, ok := e.(platform.FrameEvent); ok {
			times = append(times, f.Time)
			if len(times) == 5 {
				b.Quit()
				return
			}
			w.RequestFrame()
		}
	}))
	waitDone(t, done)
	for i := 1; i < len(times); i++ {
		if d := times[i] - times[i-1]; d < interval {
			t.Errorf("frames %d and %d are %v apart, want >= %v", i-1, i, d, interval)
		}
	}
	expectPanic(t, "real-time", func() { b.Advance(time.Second) })
}

func TestConformance(t *testing.T) {
	platformtest.Run(t, platformtest.Config{
		New: func(*testing.T) platform.Backend {
			return New(Options{RealTime: true, FrameInterval: time.Millisecond})
		},
	})
}

func TestOptionsValidation(t *testing.T) {
	expectPanic(t, "invalid display scale", func() { New(Options{Scale: -1}) })
	expectPanic(t, "FrameInterval", func() { New(Options{FrameInterval: -1}) })
	b := New(Options{})
	expectPanic(t, "WindowOptions.Size", func() { _, _ = b.NewWindow(platform.WindowOptions{Size: platform.Size{W: -1, H: 1}}) })
	w := newWindow(t, b, platform.WindowOptions{})
	expectPanic(t, "invalid display scale", func() { w.SetScale(0) })
	if w.Options().Size != platform.DefaultWindowSize {
		t.Errorf("default size = %v", w.Options().Size)
	}
}
