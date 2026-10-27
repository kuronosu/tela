// Package platformtest is a backend-agnostic conformance suite for the
// platform contract. Every backend runs it from its tests:
//
//	func TestConformance(t *testing.T) {
//		platformtest.Run(t, platformtest.Config{
//			New: func(t *testing.T) platform.Backend { ... },
//		})
//	}
//
// The suite only uses what every backend can do without a user: create
// windows, wait for configure and frame events, present, request frames,
// wake from other goroutines, close and quit; and, for the window control
// and desktop contract (phase 12), call every window control method, hide
// and show windows, open child and modal windows, and check that invalid
// desktop service requests panic (no dialog, notification or tray icon is
// ever shown). Input injection is backend-specific and tested by each
// backend.
//
// Windowed backends need a display (they should skip when their constructor
// returns platform.ErrUnavailable). Backends that must run on the OS main
// thread (Cocoa) provide Config.OnUIThread.
package platformtest

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Config configures a conformance run.
type Config struct {
	// New returns a fresh backend. Each subtest creates one and calls its
	// Run once. Call t.Skip from New if the backend is unavailable.
	New func(t *testing.T) platform.Backend
	// OnUIThread runs f on the thread allowed to call Run and returns
	// after f returns. Nil runs f on the calling goroutine (enough for
	// Linux backends and headless; Cocoa needs the main thread).
	OnUIThread func(f func())
	// Timeout bounds each subtest. Zero means 10s.
	Timeout time.Duration
	// Skip names subtests to skip (for example "Clipboard" where the
	// compositor refuses selections without user input).
	Skip []string
}

type subtest struct {
	name string
	fn   func(s *session)
}

var subtests = []subtest{
	{"ConfigureThenFrame", testConfigureThenFrame},
	{"RequestFrameCoalesces", testRequestFrameCoalesces},
	{"Animation", testAnimation},
	{"AnimationWithoutPresent", testAnimationWithoutPresent},
	{"PresentChecksSize", testPresentChecksSize},
	{"WakeFromGoroutines", testWakeFromGoroutines},
	{"QuitFromGoroutine", testQuitFromGoroutine},
	{"Close", testClose},
	{"NewWindowFromHandler", testNewWindowFromHandler},
	{"NoReentrancy", testNoReentrancy},
	{"Clipboard", testClipboard},
	{"TextInput", testTextInput},
	{"Misc", testMisc},
	{"WindowControl", testWindowControl},
	{"HideShow", testHideShow},
	{"ParentAndModal", testParentAndModal},
	{"Desktop", testDesktop},
}

// Run runs the conformance suite as subtests of t.
func Run(t *testing.T, cfg Config) {
	if cfg.New == nil {
		panic("platformtest: Config.New is nil")
	}
	if cfg.OnUIThread == nil {
		cfg.OnUIThread = func(f func()) { f() }
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	for _, st := range subtests {
		t.Run(st.name, func(t *testing.T) {
			if slices.Contains(cfg.Skip, st.name) {
				t.Skip("skipped by Config.Skip")
			}
			s := &session{t: t, cfg: cfg, b: cfg.New(t)}
			st.fn(s)
		})
	}
}

// session runs one backend's event loop under a timeout and checks the
// invariants that hold for every event: no re-entrant handler calls, no
// events for closed windows, ConfigureEvent first, Window.State matching
// the WindowStateEvent being delivered, only app-scoped events with a nil
// window.
type session struct {
	t   *testing.T
	cfg Config
	b   platform.Backend

	// UI-thread state (touched only by the handler and the setup function).
	depth      int
	configured map[platform.Window]bool
	closed     map[platform.Window]bool
	fbs        map[platform.Window]*image.RGBA

	timedOut atomic.Bool
}

// run calls setup and then Run with h on the UI thread, failing the test if
// Run does not return within the timeout.
func (s *session) run(setup func(), h func(w platform.Window, e platform.Event)) {
	s.t.Helper()
	s.configured = map[platform.Window]bool{}
	s.closed = map[platform.Window]bool{}
	s.fbs = map[platform.Window]*image.RGBA{}
	timer := time.AfterFunc(s.cfg.Timeout, func() {
		s.timedOut.Store(true)
		s.b.Quit()
	})
	defer timer.Stop()
	var err error
	s.cfg.OnUIThread(func() {
		if setup != nil {
			setup()
		}
		err = s.b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
			s.depth++
			defer func() { s.depth-- }()
			if s.depth > 1 {
				s.t.Errorf("handler called re-entrantly with %T", e)
			}
			if w != nil {
				if s.closed[w] {
					s.t.Errorf("event %T delivered for a closed window", e)
				}
				if _, ok := e.(platform.ConfigureEvent); ok {
					s.configured[w] = true
				} else if !s.configured[w] {
					s.t.Errorf("first event of a window is %T, want ConfigureEvent", e)
				}
				if se, ok := e.(platform.WindowStateEvent); ok && w.State() != se.State {
					s.t.Errorf("State() = %v during WindowStateEvent %v", w.State(), se.State)
				}
			} else {
				switch e.(type) {
				case platform.WakeEvent, platform.SettingsEvent, platform.NotificationEvent, platform.TrayEvent:
				default:
					s.t.Errorf("window-scoped event %T delivered with a nil window", e)
				}
			}
			h(w, e)
		}))
	})
	if err != nil {
		s.t.Errorf("Run returned error: %v", err)
	}
	if s.timedOut.Load() {
		s.t.Fatalf("Run did not finish within %v", s.cfg.Timeout)
	}
}

// newWindow creates a window or fails the test.
func (s *session) newWindow(opts platform.WindowOptions) platform.Window {
	s.t.Helper()
	w, err := s.b.NewWindow(opts)
	if err != nil {
		s.t.Fatalf("NewWindow: %v", err)
	}
	return w
}

// present draws a solid frame of the window's current size and presents it.
func (s *session) present(w platform.Window, c color.RGBA) {
	s.t.Helper()
	m := w.Metrics()
	p := m.Physical()
	fb := s.fbs[w]
	if fb == nil || fb.Rect.Max != p {
		fb = image.NewRGBA(image.Rectangle{Max: p})
		s.fbs[w] = fb
	}
	for i := 0; i < len(fb.Pix); i += 4 {
		fb.Pix[i+0], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	if err := w.Present(fb, nil); err != nil {
		s.t.Errorf("Present: %v", err)
	}
}

func (s *session) close(w platform.Window) {
	w.Close()
	s.closed[w] = true
}

// wakeAfter calls Wake from another goroutine after d.
func (s *session) wakeAfter(d time.Duration) {
	time.AfterFunc(d, s.b.Wake)
}

var opaqueBlue = color.RGBA{0x20, 0x40, 0xc0, 0xff}

// quietPeriod is how long the suite waits to check that nothing else
// arrives.
const quietPeriod = 100 * time.Millisecond

func testConfigureThenFrame(s *session) {
	var w platform.Window
	var cfg platform.ConfigureEvent
	frames := 0
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{Title: "conformance", AppID: "dev.telaui.platformtest", Size: platform.Size{W: 320, H: 240}})
	}, func(ew platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			if ew != w {
				s.t.Errorf("ConfigureEvent for an unknown window")
			}
			if !(e.Scale > 0) || e.Size.Empty() {
				s.t.Errorf("first ConfigureEvent has unusable metrics %v", e.Metrics)
			}
			if got := w.Metrics(); got != e.Metrics {
				s.t.Errorf("Metrics() = %v during ConfigureEvent %v", got, e.Metrics)
			}
			cfg = e
		case platform.FrameEvent:
			frames++
			if cfg.Scale == 0 {
				s.t.Errorf("FrameEvent before ConfigureEvent")
			}
			s.present(w, opaqueBlue)
			s.b.Quit()
		}
	})
	if frames != 1 {
		s.t.Errorf("got %d FrameEvents after ConfigureEvent, want 1", frames)
	}
}

func testRequestFrameCoalesces(s *session) {
	// The compositor may reconfigure the window at any time (activation,
	// tiling); frames caused by such ConfigureEvents are not counted.
	var w platform.Window
	frames, reconfigures := 0, 0
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{})
	}, func(_ platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.ConfigureEvent:
			if frames > 0 {
				reconfigures++
			}
		case platform.FrameEvent:
			frames++
			s.present(w, opaqueBlue)
			switch frames {
			case 1:
				for range 5 {
					w.RequestFrame()
				}
			case 2:
				s.wakeAfter(quietPeriod)
			}
		case platform.WakeEvent:
			s.b.Quit()
		}
	})
	if after := frames - 1; after < 1 || after > 1+reconfigures {
		s.t.Errorf("5 coalesced RequestFrame calls produced %d FrameEvents (%d reconfigures), want 1", after, reconfigures)
	}
}

func testAnimation(s *session) {
	var w platform.Window
	var times []time.Duration
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{})
	}, func(_ platform.Window, e platform.Event) {
		if e, ok := e.(platform.FrameEvent); ok {
			times = append(times, e.Time)
			s.present(w, color.RGBA{uint8(len(times) * 40), 0, 0, 0xff})
			if len(times) < 6 {
				w.RequestFrame()
			} else {
				s.b.Quit()
			}
		}
	})
	if len(times) != 6 {
		s.t.Fatalf("got %d frames, want 6", len(times))
	}
	for i := 1; i < len(times); i++ {
		if times[i] < times[i-1] {
			s.t.Errorf("FrameEvent.Time went backwards: %v", times)
		}
	}
	if now := s.b.Now(); now < times[len(times)-1] {
		s.t.Errorf("Backend.Now() = %v is before the last frame time %v", now, times[len(times)-1])
	}
}

// testAnimationWithoutPresent checks pacing when the app animates without
// drawing: FrameEvent handlers that only call RequestFrame (an animation
// tick with no pixel change, which the contract allows). Frames must stay
// paced by the display (vsync, frame callbacks or the refresh interval), not
// delivered as fast as the loop can spin, and they must keep coming.
func testAnimationWithoutPresent(s *session) {
	const window = 200 * time.Millisecond
	var w platform.Window
	var start time.Time
	frames := 0
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{})
	}, func(_ platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			if start.IsZero() {
				// The first frame maps the window; the rest only tick.
				s.present(w, opaqueBlue)
				start = time.Now()
				s.wakeAfter(window)
			} else {
				frames++
			}
			w.RequestFrame()
		case platform.WakeEvent:
			if !start.IsZero() {
				s.b.Quit()
			}
		}
	})
	elapsed := time.Since(start)
	// A busy-looping backend delivers tens of thousands of frames here; any
	// real display paces them at most a few hundred per second. The bound
	// allows a 2 kHz "display" so fast test doubles pass too.
	if limit := int(elapsed/(500*time.Microsecond)) + 10; frames > limit {
		s.t.Errorf("%d FrameEvents in %v without Present, want at most %d: frames are not paced", frames, elapsed, limit)
	}
	if frames == 0 {
		s.t.Errorf("no FrameEvent in %v while RequestFrame was called in every frame without Present", elapsed)
	}
	s.t.Logf("%d FrameEvents in %v without Present", frames, elapsed)
}

func testPresentChecksSize(s *session) {
	var w platform.Window
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{})
	}, func(_ platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		p := w.Metrics().Physical()
		for _, bad := range []*image.RGBA{
			nil,
			image.NewRGBA(image.Rect(0, 0, p.X+1, p.Y)),
			image.NewRGBA(image.Rect(1, 1, p.X+1, p.Y+1)),
		} {
			func() {
				defer func() {
					if recover() == nil {
						s.t.Errorf("Present with framebuffer %v did not panic (window physical size %v)", rectOf(bad), p)
					}
				}()
				_ = w.Present(bad, nil)
			}()
		}
		// A valid present with partial and out-of-bounds damage works.
		fb := image.NewRGBA(image.Rectangle{Max: p})
		if err := w.Present(fb, nil); err != nil {
			s.t.Errorf("Present: %v", err)
		}
		if err := w.Present(fb, []image.Rectangle{image.Rect(0, 0, 4, 4), image.Rect(-10, -10, 1e6, 1e6), {}}); err != nil {
			s.t.Errorf("Present with damage: %v", err)
		}
		// Damage entirely outside the frame: nothing changed, and the
		// Present is a successful no-op (it is not a full frame).
		if err := w.Present(fb, []image.Rectangle{image.Rect(p.X, 0, p.X+10, p.Y), image.Rect(-10, -10, 0, 0)}); err != nil {
			s.t.Errorf("Present with fully clipped damage: %v", err)
		}
		s.b.Quit()
	})
}

func rectOf(fb *image.RGBA) any {
	if fb == nil {
		return nil
	}
	return fb.Rect
}

func testWakeFromGoroutines(s *session) {
	const goroutines, perGoroutine = 32, 200
	var finished atomic.Bool
	wakes := 0
	s.run(func() {
		s.newWindow(platform.WindowOptions{})
		var wg sync.WaitGroup
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range perGoroutine {
					s.b.Wake()
				}
			}()
		}
		go func() {
			wg.Wait()
			finished.Store(true)
			// This Wake starts after finished is set, so some WakeEvent
			// must observe finished == true.
			s.b.Wake()
		}()
	}, func(_ platform.Window, e platform.Event) {
		if _, ok := e.(platform.WakeEvent); ok {
			wakes++
			if finished.Load() {
				s.b.Quit()
			}
		}
	})
	if wakes == 0 {
		s.t.Errorf("no WakeEvent delivered")
	}
	if wakes > goroutines*perGoroutine+1 {
		s.t.Errorf("%d WakeEvents for %d Wake calls", wakes, goroutines*perGoroutine+1)
	}
	s.t.Logf("%d Wake calls coalesced into %d WakeEvents", goroutines*perGoroutine+1, wakes)
}

func testQuitFromGoroutine(s *session) {
	start := time.Now()
	s.run(func() {
		s.newWindow(platform.WindowOptions{})
		time.AfterFunc(50*time.Millisecond, s.b.Quit)
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); ok {
			s.present(w, opaqueBlue)
		}
	})
	if d := time.Since(start); d < 50*time.Millisecond {
		s.t.Errorf("Run returned after %v, before Quit was called", d)
	}
}

func testClose(s *session) {
	var a, b platform.Window
	framesB := 0
	s.run(func() {
		a = s.newWindow(platform.WindowOptions{Title: "a"})
		b = s.newWindow(platform.WindowOptions{Title: "b"})
	}, func(w platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			s.present(w, opaqueBlue)
			if w == a {
				s.close(a)
				// Methods on a closed window are no-ops.
				a.RequestFrame()
				a.SetTitle("closed")
				a.SetCursor(platform.CursorPointer)
				a.SetSize(platform.Size{W: 10, H: 10})
				a.SetTextInput(&platform.TextInputState{})
				a.ResetTextInput()
				a.Minimize()
				a.SetMaximized(true)
				a.SetFullscreen(true)
				a.SetVisible(false)
				a.SetAlwaysOnTop(true)
				a.SetMinSize(platform.Size{W: 10, H: 10})
				a.SetMaxSize(platform.Size{W: 20, H: 20})
				a.SetPosition(image.Pt(1, 2))
				a.Activate()
				a.SetIcon([]image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))})
				a.SetHitRegions([]platform.HitRegion{{Kind: platform.HitCaption, Rect: platform.Rect{W: 10, H: 10}}})
				_ = a.State()
				_ = a.RestoreToken()
				if err := a.Present(image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
					s.t.Errorf("Present on a closed window: %v", err)
				}
				a.Close()
			} else {
				framesB++
				if framesB < 3 {
					b.RequestFrame()
				} else {
					s.wakeAfter(quietPeriod)
				}
			}
		case platform.WakeEvent:
			if !s.closed[a] {
				s.t.Errorf("window a never got a frame")
			}
			s.b.Quit()
		}
	})
}

func testNewWindowFromHandler(s *session) {
	var second platform.Window
	gotFrame := false
	s.run(func() {
		s.newWindow(platform.WindowOptions{})
		s.b.Wake()
	}, func(w platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.WakeEvent:
			if second == nil {
				second = s.newWindow(platform.WindowOptions{Title: "second", Size: platform.Size{W: 200, H: 100}})
			}
		case platform.FrameEvent:
			s.present(w, opaqueBlue)
			if w == second {
				gotFrame = true
				s.b.Quit()
			}
		}
	})
	if !gotFrame {
		s.t.Errorf("window created from the handler got no FrameEvent")
	}
}

func testNoReentrancy(s *session) {
	// SetSize inside the handler makes some OSes deliver a resize
	// synchronously (Win32 WM_SIZE); the session wrapper fails on nested
	// handler calls.
	var w platform.Window
	frames := 0
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{Size: platform.Size{W: 300, H: 200}})
	}, func(_ platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			frames++
			s.present(w, opaqueBlue)
			switch frames {
			case 1:
				w.SetSize(platform.Size{W: 340, H: 220})
				w.SetTitle("resized")
				s.wakeAfter(quietPeriod)
			}
		case platform.WakeEvent:
			s.b.Quit()
		}
	})
}

func testClipboard(s *session) {
	const text = "tela clipboard ✓"
	var inReadText bool
	var got string
	called := false
	s.run(func() {
		s.newWindow(platform.WindowOptions{})
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || called {
			return
		}
		s.present(w, opaqueBlue)
		called = true
		cb := s.b.Clipboard()
		if err := cb.WriteText(text); err != nil {
			s.t.Errorf("WriteText: %v", err)
			s.b.Quit()
			return
		}
		inReadText = true
		cb.ReadText(func(t string, err error) {
			if inReadText {
				s.t.Errorf("ReadText called its callback synchronously")
			}
			if err != nil {
				s.t.Errorf("ReadText: %v", err)
			}
			got = t
			s.b.Quit()
		})
		inReadText = false
	})
	if got != text {
		s.t.Errorf("clipboard round trip = %q, want %q", got, text)
	}
}

func testTextInput(s *session) {
	// The suite has no IME to drive: it checks that the text input methods
	// accept valid states, reject invalid ones, and produce no IMEEvents,
	// and that IMESessionEvents (if the backend attaches an IME) alternate.
	var w platform.Window
	session := false
	called := false
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{Size: platform.Size{W: 300, H: 200}})
		// Before the first frame, from setup: allowed (UI thread).
		w.SetTextInput(&platform.TextInputState{CursorArea: platform.Rect{X: 10, Y: 10, H: 20}})
	}, func(ew platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.IMEEvent:
			s.t.Errorf("IMEEvent %+v without an IME", e)
		case platform.IMESessionEvent:
			if e.Active == session {
				s.t.Errorf("IMESessionEvent{Active: %v} twice in a row", e.Active)
			}
			session = e.Active
		case platform.FrameEvent:
			s.present(w, opaqueBlue)
			if called {
				return
			}
			called = true
			text := "héllo wörld"
			st := &platform.TextInputState{
				CursorArea:  platform.Rect{X: 12.5, Y: 40.25, W: 0, H: 18},
				Purpose:     platform.TextPurposeEmail,
				Surrounding: &platform.SurroundingText{Text: text, Cursor: 6, Anchor: 1},
			}
			w.SetTextInput(st)
			st.Surrounding.Cursor = 7 // the backend kept a copy
			w.SetTextInput(st)
			w.SetTextInput(st) // equal: free
			w.ResetTextInput()
			st2 := st.Clone()
			st2.Purpose, st2.Multiline = platform.TextPurposeNormal, true
			w.SetTextInput(st2)
			w.SetTextInput(nil)
			w.SetTextInput(nil)
			w.ResetTextInput() // without a state: nothing
			w.SetTextInput(&platform.TextInputState{Purpose: platform.TextPurposePassword})
			for _, bad := range []*platform.TextInputState{
				{CursorArea: platform.Rect{W: -1}},
				{Purpose: platform.TextPurpose(200)},
				{Surrounding: &platform.SurroundingText{Text: text, Cursor: 2}}, // inside "é"
				{Surrounding: &platform.SurroundingText{Text: text, Anchor: len(text) + 1}},
			} {
				func() {
					defer func() {
						if recover() == nil {
							s.t.Errorf("SetTextInput(%+v) did not panic", bad)
						}
					}()
					w.SetTextInput(bad)
				}()
			}
			w.SetTextInput(nil)
			s.wakeAfter(quietPeriod)
		case platform.WakeEvent:
			s.b.Quit()
		}
	})
	if !called {
		s.t.Errorf("no FrameEvent")
	}
}

func testMisc(s *session) {
	if s.b.Name() == "" {
		s.t.Errorf("Name() is empty")
	}
	_ = s.b.Settings()
	s.run(func() {
		s.newWindow(platform.WindowOptions{MinSize: platform.Size{W: 100, H: 100}, FixedSize: true})
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		for c := platform.CursorDefault; c.Valid(); c++ {
			w.SetCursor(c)
		}
		w.SetCursor(platform.CursorDefault)
		func() {
			defer func() {
				if recover() == nil {
					s.t.Errorf("SetCursor with an invalid cursor did not panic")
				}
			}()
			w.SetCursor(platform.Cursor(255))
		}()
		w.SetTitle(fmt.Sprintf("misc %s", s.b.Name()))
		s.present(w, opaqueBlue)
		s.b.Quit()
	})
}

// expectPanic fails the test if f does not panic.
func (s *session) expectPanic(what string, f func()) {
	s.t.Helper()
	defer func() {
		if recover() == nil {
			s.t.Errorf("%s did not panic", what)
		}
	}()
	f()
}

// testWindowControl calls every window control method with valid and
// invalid arguments. It checks the general invariants (no re-entrant
// handler calls when the OS answers synchronously, as Win32 does to
// ShowWindow; State consistent with WindowStateEvents) and that invalid
// arguments panic; it does not require the OS to grant anything (tiling
// window managers refuse to maximize, Wayland has no positions).
func testWindowControl(s *session) {
	var w, f platform.Window
	called := false
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{Title: "control", Size: platform.Size{W: 320, H: 240},
			MinSize: platform.Size{W: 100, H: 80}, MaxSize: platform.Size{W: 2000, H: 1500}})
		f = s.newWindow(platform.WindowOptions{Title: "frameless", Size: platform.Size{W: 200, H: 120}, Frameless: true,
			HasPosition: true, Position: image.Pt(60, 70)})
		// Before the first frame, from setup: allowed (UI thread).
		w.SetIcon([]image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16)), image.NewNRGBA(image.Rect(0, 0, 32, 32))})
		_ = w.State()
	}, func(ew platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			s.present(ew, opaqueBlue)
			if ew != w || called {
				return
			}
			called = true
			_ = w.RestoreToken()
			_ = f.RestoreToken()
			regions := []platform.HitRegion{
				{Kind: platform.HitCaption, Rect: platform.Rect{W: 320, H: 32}},
				{Kind: platform.HitClient, Rect: platform.Rect{X: 100, Y: 4, W: 80, H: 24}},
				{Kind: platform.HitMinimize, Rect: platform.Rect{X: 224, W: 32, H: 32}},
				{Kind: platform.HitMaximize, Rect: platform.Rect{X: 256, W: 32, H: 32}},
				{Kind: platform.HitClose, Rect: platform.Rect{X: 288, W: 32, H: 32}},
			}
			f.SetHitRegions(regions)
			f.SetHitRegions(slices.Clone(regions)) // equal: free
			w.SetHitRegions(regions[:1])
			w.SetHitRegions(nil)
			w.SetMinSize(platform.Size{W: 120, H: 90})
			w.SetMaxSize(platform.Size{W: 1800, H: 1200})
			w.SetMaxSize(platform.Size{})
			w.SetMinSize(platform.Size{})
			w.SetAlwaysOnTop(true)
			w.SetAlwaysOnTop(false)
			w.SetPosition(image.Pt(40, 50))
			w.SetMaximized(true)
			w.SetMaximized(false)
			w.SetFullscreen(true)
			w.SetFullscreen(false)
			w.SetIcon(nil)
			w.Activate()
			s.expectPanic("SetIcon with a nil image", func() { w.SetIcon([]image.Image{nil}) })
			s.expectPanic("SetIcon with an empty image", func() { w.SetIcon([]image.Image{image.NewRGBA(image.Rect(0, 0, 0, 16))}) })
			s.expectPanic("SetHitRegions with an invalid kind", func() {
				w.SetHitRegions([]platform.HitRegion{{Kind: platform.HitKind(200), Rect: platform.Rect{W: 1, H: 1}}})
			})
			s.expectPanic("SetHitRegions with a negative size", func() {
				w.SetHitRegions([]platform.HitRegion{{Kind: platform.HitCaption, Rect: platform.Rect{W: -1, H: 1}}})
			})
			s.expectPanic("SetMinSize with a negative size", func() { w.SetMinSize(platform.Size{W: -1}) })
			nan := float32(math.NaN())
			s.expectPanic("SetMaxSize with NaN", func() { w.SetMaxSize(platform.Size{W: nan, H: 1}) })
			s.wakeAfter(3 * quietPeriod)
		case platform.WakeEvent:
			s.b.Quit()
		}
	})
	if !called {
		s.t.Errorf("no FrameEvent")
	}
}

// testHideShow checks that hidden windows get no FrameEvents and that the
// requests made while hidden are delivered once shown, for a window hidden
// after its first frame and for one created hidden (which still gets its
// ConfigureEvent).
func testHideShow(s *session) {
	var w, h platform.Window
	phase := 0 // 0: before w's first frame; 1: w hidden; 2: both shown
	gotW, gotH, configuredH := false, false, false
	s.run(func() {
		w = s.newWindow(platform.WindowOptions{Title: "hide"})
		h = s.newWindow(platform.WindowOptions{Title: "created hidden", Size: platform.Size{W: 200, H: 100}, Hidden: true})
	}, func(ew platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.ConfigureEvent:
			if ew == h {
				configuredH = true
			}
		case platform.FrameEvent:
			switch {
			case ew == h && phase < 2:
				s.t.Errorf("a window created hidden got a FrameEvent before SetVisible(true)")
			case ew == w && phase == 1:
				s.t.Errorf("a hidden window got a FrameEvent")
			}
			s.present(ew, opaqueBlue)
			switch {
			case ew == w && phase == 0:
				phase = 1
				w.SetVisible(false)
				w.RequestFrame()
				h.RequestFrame()
				s.wakeAfter(quietPeriod)
			case phase == 2 && ew == w:
				gotW = true
			case phase == 2 && ew == h:
				gotH = true
			}
			if gotW && gotH {
				s.b.Quit()
			}
		case platform.WakeEvent:
			if phase == 1 {
				phase = 2
				w.SetVisible(true)
				h.SetVisible(true)
			}
		}
	})
	if !configuredH {
		s.t.Errorf("a window created hidden got no ConfigureEvent")
	}
	if !gotW {
		s.t.Errorf("the frame requested while hidden was not delivered after SetVisible(true)")
	}
	if !gotH {
		s.t.Errorf("a window created hidden got no FrameEvent after SetVisible(true)")
	}
}

// testParentAndModal opens a modal and a non-modal child window and checks
// that closing the parent closes both (no more events for them, and their
// methods are no-ops).
func testParentAndModal(s *session) {
	var p, modal, child platform.Window
	framed := map[platform.Window]bool{}
	closed := false
	s.run(func() {
		s.expectPanic("NewWindow with Modal and no Parent", func() { _, _ = s.b.NewWindow(platform.WindowOptions{Modal: true}) })
		p = s.newWindow(platform.WindowOptions{Title: "parent"})
		modal = s.newWindow(platform.WindowOptions{Title: "modal child", Size: platform.Size{W: 200, H: 150}, Parent: p, Modal: true})
		child = s.newWindow(platform.WindowOptions{Title: "child", Size: platform.Size{W: 180, H: 120}, Parent: p})
	}, func(ew platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			s.present(ew, opaqueBlue)
			framed[ew] = true
			if closed || !framed[p] || !framed[modal] || !framed[child] {
				return
			}
			closed = true
			s.close(p)
			// The children closed with their parent.
			s.closed[modal], s.closed[child] = true, true
			for _, c := range []platform.Window{modal, child} {
				c.RequestFrame()
				c.SetTitle("closed")
				c.SetVisible(true)
				if err := c.Present(image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
					s.t.Errorf("Present on a child closed with its parent: %v", err)
				}
				c.Close()
			}
			s.wakeAfter(quietPeriod)
		case platform.WakeEvent:
			s.b.Quit()
		}
	})
	if !closed {
		s.t.Errorf("the parent and its children did not all get a FrameEvent")
	}
}

// testDesktop checks the desktop services without showing anything: the
// service set exists, and invalid requests panic (or, for URIs, fail) on
// every backend, whether it supports the service or not.
func testDesktop(s *session) {
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	files := func([]string, error) { s.t.Errorf("a file dialog callback was called after an invalid request") }
	file := func(string, error) { s.t.Errorf("a file dialog callback was called after an invalid request") }
	answer := func(platform.MessageAnswer, error) {
		s.t.Errorf("a message box callback was called after an invalid request")
	}
	s.run(func() {
		s.newWindow(platform.WindowOptions{})
		d := s.b.Desktop()
		if d == nil {
			s.t.Errorf("Desktop() is nil; return platform.UnsupportedDesktop{} for a backend without services")
			return
		}
		s.expectPanic("OpenFiles with a nil callback", func() { _ = d.OpenFiles(platform.FileDialogOptions{}, nil) })
		s.expectPanic("SaveFile with a nil callback", func() { _ = d.SaveFile(platform.FileDialogOptions{}, nil) })
		s.expectPanic("PickFolder with a nil callback", func() { _ = d.PickFolder(platform.FileDialogOptions{}, nil) })
		s.expectPanic("MessageBox with a nil callback", func() { _ = d.MessageBox(platform.MessageBoxOptions{}, nil) })
		s.expectPanic("OpenFiles with a filter without patterns", func() {
			_ = d.OpenFiles(platform.FileDialogOptions{Filters: []platform.FileFilter{{Name: "Images"}}}, files)
		})
		s.expectPanic("SaveFile with a pattern containing ';'", func() {
			_ = d.SaveFile(platform.FileDialogOptions{Filters: []platform.FileFilter{{Name: "x", Patterns: []string{"*.a;*.b"}}}}, file)
		})
		s.expectPanic("MessageBox with an invalid kind", func() { _ = d.MessageBox(platform.MessageBoxOptions{Kind: 99}, answer) })
		s.expectPanic("MessageBox with invalid buttons", func() { _ = d.MessageBox(platform.MessageBoxOptions{Buttons: 99}, answer) })
		s.expectPanic("Notify with a nil icon image", func() { _, _ = d.Notify(platform.Notification{Icon: []image.Image{nil}}) })
		s.expectPanic("Notify with the action ID \"default\"", func() {
			_, _ = d.Notify(platform.Notification{Actions: []platform.NotificationAction{{ID: "default", Label: "Open"}}})
		})
		s.expectPanic("NewTray without an icon", func() { _, _ = d.NewTray(platform.TrayOptions{Tooltip: "x"}) })
		s.expectPanic("NewTray with an item without ID", func() {
			_, _ = d.NewTray(platform.TrayOptions{Icon: icon, Menu: []platform.MenuItem{{Label: "Open"}}})
		})
		s.expectPanic("NewTray with a duplicate item ID", func() {
			_, _ = d.NewTray(platform.TrayOptions{Icon: icon, Menu: []platform.MenuItem{{ID: 1, Label: "a"},
				{Label: "more", Submenu: []platform.MenuItem{{ID: 1, Label: "b"}}}}})
		})
		s.expectPanic("UpdateTray without an icon", func() { _ = d.UpdateTray(1, platform.TrayOptions{}) })
		for _, uri := range []string{"no scheme", "file:///etc/hosts", "%zz"} {
			if err := d.OpenURI(nil, uri); err == nil {
				s.t.Errorf("OpenURI(%q) = nil, want an error", uri)
			}
		}
		// Unknown IDs: nothing to do (nil, or ErrUnsupported).
		if err := d.CloseNotification(0); err != nil && !errors.Is(err, platform.ErrUnsupported) {
			s.t.Errorf("CloseNotification(0) = %v", err)
		}
		if err := d.RemoveTray(0); err != nil && !errors.Is(err, platform.ErrUnsupported) {
			s.t.Errorf("RemoveTray(0) = %v", err)
		}
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); ok {
			s.present(w, opaqueBlue)
			s.b.Quit()
		}
	})
}
