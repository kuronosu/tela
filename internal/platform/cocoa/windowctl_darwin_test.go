//go:build darwin

package cocoa

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Window control tests (windowctl_darwin.go). Each runs a backend with a
// few windows and a script of steps, run from WakeEvent handler calls
// (inside a delivery pass, where app code runs). A step's do runs once;
// its until is polled every 50 ms until it holds or the step times out,
// which fails the test with the step's name and the windows' state and
// stops the script; check runs once until held. Everything the windows
// report is recorded with the state delivered at the time, and logged on
// failure. A watchdog panics with the step's name if the main thread does
// not come back (AppKit blocked in a tracking loop), so a hang is a
// precise failure, not a timeout of the whole package.

const ctlPoll = 50 * time.Millisecond

type ctlStep struct {
	name    string
	do      func(c *ctl)
	until   func(c *ctl) bool // nil: holds at the first poll
	check   func(c *ctl)
	timeout time.Duration // 0: 5 s
}

func (s ctlStep) limit() time.Duration {
	if s.timeout == 0 {
		return 5 * time.Second
	}
	return s.timeout
}

type ctlEvent struct {
	w     int
	e     platform.Event
	state platform.WindowState // State() when the event was delivered
}

type ctl struct {
	t      *testing.T
	b      *Backend
	w      []*Window
	events []ctlEvent
	// paint, if set, presents the frames of window i (default: a solid
	// gray).
	paint func(c *ctl, i int, w *Window)

	steps    []ctlStep
	cur      int
	started  bool
	start    time.Time
	mark     int     // len(events) when the step started
	ts       float64 // system uptime when the step started (event timestamps)
	stepName atomic.Value
}

// ctlRun opens windows with opts, waits until every window that is not
// Hidden is shown, and runs the steps. It returns nil when no window
// could be created.
func ctlRun(t *testing.T, opts []platform.WindowOptions, paint func(c *ctl, i int, w *Window), steps ...ctlStep) *ctl {
	return ctlRunRounds(t, [][]platform.WindowOptions{opts}, nil, paint, steps...)
}

// ctlRunRounds is ctlRun with the windows made in rounds; then, if not
// nil, returns the options of one more round, which may name the windows
// of the first ones (Parent).
func ctlRunRounds(t *testing.T, rounds [][]platform.WindowOptions, then func(c *ctl) []platform.WindowOptions,
	paint func(c *ctl, i int, w *Window), steps ...ctlStep) *ctl {
	b := newBackend(t)
	c := &ctl{t: t, b: b, paint: paint, steps: steps}
	c.stepName.Store("opening the windows")
	total := 10 * time.Second
	for _, s := range steps {
		total += s.limit() + time.Second
	}
	finished := make(chan struct{})
	go func() {
		select {
		case <-finished:
		case <-time.After(total + 20*time.Second):
			panic(fmt.Sprintf("%s: the main thread did not come back within %v, during step %q (blocked in AppKit?)",
				t.Name(), total+20*time.Second, c.stepName.Load()))
		}
	}()
	defer close(finished)
	onMain(func() {
		open := func(opts []platform.WindowOptions) bool {
			for _, o := range opts {
				pw, err := b.NewWindow(o)
				if err != nil {
					t.Errorf("NewWindow(%q): %v", o.Title, err)
					return false
				}
				c.w = append(c.w, pw.(*Window))
			}
			return true
		}
		for _, r := range rounds {
			if !open(r) {
				return
			}
		}
		if then != nil && !open(then(c)) {
			return
		}
		time.AfterFunc(total, b.Quit) // a failure, not a hang
		waiting, scheduled := true, false
		_ = b.Run(platform.HandlerFunc(func(ew platform.Window, e platform.Event) {
			switch e.(type) {
			case platform.FrameEvent:
				c.present(ew)
				if waiting && !scheduled && c.allShown() {
					scheduled = true
					time.AfterFunc(250*time.Millisecond, b.Wake)
				}
				return
			case platform.WakeEvent:
				if !scheduled {
					return
				}
				waiting = false
				c.wake()
				return
			}
			c.record(ew, e)
		}))
		if c.cur < len(c.steps) && !t.Failed() {
			t.Errorf("the script stopped at step %q (Quit before the end); %s", c.stepName.Load(), c.describe())
		}
	})
	if len(c.w) == 0 {
		return nil
	}
	return c
}

func (c *ctl) index(pw platform.Window) int {
	for i, w := range c.w {
		if platform.Window(w) == pw {
			return i
		}
	}
	return -1
}

func (c *ctl) present(pw platform.Window) {
	i := c.index(pw)
	if i < 0 {
		return
	}
	if c.paint != nil {
		c.paint(c, i, c.w[i])
		return
	}
	presentSolid(c.t, pw, color.RGBA{0x70, 0x80, 0x90, 0xff})
}

func (c *ctl) allShown() bool {
	for _, w := range c.w {
		if !w.opts.Hidden && !w.shown {
			return false
		}
	}
	return true
}

func (c *ctl) record(pw platform.Window, e platform.Event) {
	if i := c.index(pw); i >= 0 {
		c.events = append(c.events, ctlEvent{w: i, e: e, state: pw.State()})
	}
}

// wake runs the script: it starts the current step, or polls it.
func (c *ctl) wake() {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	if c.cur >= len(c.steps) {
		c.b.Quit()
		return
	}
	s := c.steps[c.cur]
	if !c.started {
		c.started = true
		c.start, c.mark, c.ts = time.Now(), len(c.events), uptime()
		c.stepName.Store(s.name)
		c.t.Logf("step %q", s.name)
		if s.do != nil {
			s.do(c)
		}
		time.AfterFunc(ctlPoll, c.b.Wake)
		return
	}
	if s.until == nil || s.until(c) {
		c.t.Logf("step %q held after %v; %s", s.name, time.Since(c.start).Round(time.Millisecond), c.describe())
		if s.check != nil {
			s.check(c)
		}
		c.cur++
		c.started = false
		time.AfterFunc(ctlPoll, c.b.Wake)
		return
	}
	if time.Since(c.start) > s.limit() {
		c.t.Errorf("step %q did not hold within %v; %s", s.name, s.limit(), c.describe())
		c.logEvents(c.mark)
		c.b.Quit()
		return
	}
	time.AfterFunc(ctlPoll, c.b.Wake)
}

// after reports whether d has passed since the step started.
func (c *ctl) after(d time.Duration) bool { return time.Since(c.start) >= d }

// describe returns the state of every window, for logs.
func (c *ctl) describe() string {
	var parts []string
	for i, w := range c.w {
		if w.Closed() {
			parts = append(parts, fmt.Sprintf("window %d: closed", i))
			continue
		}
		f := darwin.SendRect(w.ns, wsel.frame)
		parts = append(parts, fmt.Sprintf("window %d: state %+v, metrics %v, frame (%.1f,%.1f %.1fx%.1f), key %v, zoomed %v, "+
			"miniaturized %v, styleMask %#x, fs{want %v in %v transition %v asked %v entered %d exited %d}",
			i, w.State(), w.Metrics(), f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height,
			isKey(w), darwin.SendBool(w.ns, wsel.isZoomed), darwin.SendBool(w.ns, wsel.isMiniaturized),
			darwin.SendUint(w.ns, wsel.styleMask), w.ctl.fsWant, w.ctl.fullscreen, w.ctl.fsTransition, w.ctl.fsAsked != 0,
			w.ctl.fsEntered, w.ctl.fsExited))
	}
	return strings.Join(parts, "; ")
}

func (c *ctl) logEvents(from int) {
	for _, e := range c.events[from:] {
		c.t.Logf("  window %d: %s (state then %+v)", e.w, eventString(e.e), e.state)
	}
}

func eventString(e platform.Event) string {
	switch e := e.(type) {
	case platform.PointerEvent:
		return "pointer " + pointerString(e)
	case platform.WindowStateEvent:
		return fmt.Sprintf("state %+v", e.State)
	case platform.ConfigureEvent:
		return fmt.Sprintf("configure %v", e.Metrics)
	}
	return fmt.Sprintf("%T %+v", e, e)
}

// stepEvents returns the events of window i since the step started.
func (c *ctl) stepEvents(i int) []ctlEvent {
	var out []ctlEvent
	for _, e := range c.events[c.mark:] {
		if e.w == i {
			out = append(out, e)
		}
	}
	return out
}

// mouse builds a mouse NSEvent at the view point p of window i, stamped
// ms after the step started.
func (c *ctl) mouse(i, typ int, p platform.Point, ms, clicks int) darwin.ID {
	w := c.w[i]
	wp := darwin.SendPoint(w.view, isel.convertPointToView, darwin.CGPoint{X: float64(p.X), Y: float64(p.Y)}, darwin.ID(0))
	ev := darwin.SendClass(app.event, tsel.mouseEvent, uint(typ), wp, uint(0), c.ts+float64(ms)/1000,
		darwin.SendInt(w.ns, sel.windowNumber), darwin.ID(0), 0, clicks, float32(1))
	if ev == 0 {
		c.t.Errorf("building a mouse event of type %d failed", typ)
	}
	return ev
}

// send hands ev to NSApp, the path of real events (the window's hit test
// picks the view).
func (c *ctl) send(ev darwin.ID) {
	if ev != 0 {
		darwin.Send(app.ns, isel.sendEvent, ev)
	}
}

func sizeIs(s platform.Size, w, h float32) bool {
	return math.Abs(float64(s.W-w)) < 0.5 && math.Abs(float64(s.H-h)) < 0.5
}

func isKey(w *Window) bool { return darwin.SendBool(w.ns, wsel.isKeyWindow) }

// buttonEnabled reports whether the standard button b of w is enabled.
func buttonEnabled(w *Window, b uintptr) bool {
	btn := darwin.Send(w.ns, wsel.standardWindowButton, b)
	return btn != 0 && darwin.SendBool(btn, darwin.Sel("isEnabled"))
}

// screenInfo logs the screens and the title bar setting, once.
func screenInfo(t *testing.T) {
	if logOnce["screens"] {
		return
	}
	logOnce["screens"] = true
	onMain(func() {
		// What New checks first: the bindings and a window server
		// session, without which AppKit aborts.
		if darwin.Load() != nil {
			return
		}
		session := darwin.CGSessionCopyCurrentDictionary()
		if session == 0 {
			return
		}
		darwin.CFRelease(session)
		if err := setup(); err != nil {
			return
		}
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		screens := darwin.SendClass(app.screen, wsel.screens)
		n := int(darwin.SendUint(screens, wsel.count))
		for i := range n {
			s := darwin.Send(screens, wsel.objectAtIndex, uint(i))
			f := darwin.SendRect(s, wsel.frame)
			v := darwin.SendRect(s, darwin.Sel("visibleFrame"))
			t.Logf("screen %d: frame (%v,%v %vx%v), visible (%v,%v %vx%v), scale %v", i,
				f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height, v.Origin.X, v.Origin.Y, v.Size.Width, v.Size.Height,
				darwin.SendFloat(s, sel.backingScale))
		}
		action, has, mini := userDoubleClickSetting()
		t.Logf("AppleActionOnDoubleClick %q (set %v), AppleMiniaturizeOnDoubleClick %v: a title bar double click will %v",
			action, has, mini, doubleClickAction(action, has, mini))
		ws := darwin.SendClass(darwin.GetClass("NSWorkspace"), darwin.Sel("sharedWorkspace"))
		t.Logf("reduce transparency %v", darwin.SendBool(ws, darwin.Sel("accessibilityDisplayShouldReduceTransparency")))
	})
}

// --- requests and state ---

// TestWindowStateRequests: zoom and back (the state before the size it
// comes with), minimize and Activate, native full screen and back, a
// full-screen request made during a transition (it waits for the end),
// and requests undone in the same pass (free).
func TestWindowStateRequests(t *testing.T) {
	screenInfo(t)
	var before platform.WindowState
	var fsBefore image.Point
	askedDuring, sawTransition := false, false
	var entered int
	c := ctlRun(t, []platform.WindowOptions{{Title: "tela state", Size: platform.Size{W: 300, H: 200}}}, nil,
		ctlStep{name: "initial state", until: func(c *ctl) bool { return c.w[0].State().HasPosition },
			check: func(c *ctl) {
				st := c.w[0].State()
				if st.Maximized || st.Fullscreen || st.Minimized || st.ClientSideDecorations {
					t.Errorf("initial state %+v, want only HasPosition", st)
				}
				if got, _, _ := contentTopLeft(c.w[0]); got != st.Position {
					t.Errorf("initial position %v, but the content's top-left is at %v", st.Position, got)
				}
				before = st
			}},
		ctlStep{name: "SetMaximized(true)",
			do: func(c *ctl) { c.w[0].SetMaximized(true) },
			until: func(c *ctl) bool {
				return c.w[0].State().Maximized && darwin.SendBool(c.w[0].ns, wsel.isZoomed) && !sizeIs(c.w[0].Metrics().Size, 300, 200)
			},
			check: func(c *ctl) {
				// The configure of the zoomed size is delivered with the
				// state already maximized.
				var last *ctlEvent
				evs := c.stepEvents(0)
				for i := range evs {
					if _, ok := evs[i].e.(platform.ConfigureEvent); ok {
						last = &evs[i]
					}
				}
				switch {
				case last == nil:
					t.Errorf("zoom: no ConfigureEvent")
				case !last.state.Maximized:
					t.Errorf("zoom: the ConfigureEvent %v came before the maximized state (state then %+v)", eventString(last.e), last.state)
					c.logEvents(c.mark)
				}
			}},
		ctlStep{name: "SetMaximized(false)",
			do: func(c *ctl) { c.w[0].SetMaximized(false) },
			until: func(c *ctl) bool {
				return !c.w[0].State().Maximized && !darwin.SendBool(c.w[0].ns, wsel.isZoomed) && sizeIs(c.w[0].Metrics().Size, 300, 200)
			}},
		ctlStep{name: "Minimize",
			do:    func(c *ctl) { c.w[0].Minimize() },
			until: func(c *ctl) bool { return c.w[0].State().Minimized && darwin.SendBool(c.w[0].ns, wsel.isMiniaturized) },
			check: func(c *ctl) {
				if !c.w[0].Hidden() {
					t.Errorf("a minimized window still gets frames (loop not hidden)")
				}
			}},
		ctlStep{name: "Activate",
			do: func(c *ctl) { c.w[0].Activate() },
			until: func(c *ctl) bool {
				return !c.w[0].State().Minimized && !darwin.SendBool(c.w[0].ns, wsel.isMiniaturized)
			},
			check: func(c *ctl) {
				if c.w[0].Hidden() {
					t.Errorf("a restored window gets no frames (loop still hidden)")
				}
				if st := c.w[0].State(); st.Position != before.Position {
					t.Logf("note: the position after minimize and restore is %v, it was %v", st.Position, before.Position)
				}
			}},
		ctlStep{name: "SetFullscreen(true)", timeout: 20 * time.Second,
			do: func(c *ctl) {
				fsBefore = c.w[0].State().Position
				c.w[0].SetFullscreen(true)
			},
			until: func(c *ctl) bool {
				return c.w[0].State().Fullscreen && darwin.SendUint(c.w[0].ns, wsel.styleMask)&darwin.NSWindowStyleMaskFullScreen != 0
			},
			check: func(c *ctl) {
				st := c.w[0].State()
				if st.Maximized || st.Position != fsBefore {
					t.Errorf("in full screen the state is %+v; want Maximized false and the position from before, %v", st, fsBefore)
				}
				if sizeIs(c.w[0].Metrics().Size, 300, 200) {
					t.Errorf("in full screen the window is still %v", c.w[0].Metrics().Size)
				}
			}},
		ctlStep{name: "SetFullscreen(false)", timeout: 20 * time.Second,
			do: func(c *ctl) { c.w[0].SetFullscreen(false) },
			until: func(c *ctl) bool {
				w := c.w[0]
				return !w.State().Fullscreen && !w.ctl.fsTransition && darwin.SendUint(w.ns, wsel.styleMask)&darwin.NSWindowStyleMaskFullScreen == 0 &&
					sizeIs(w.Metrics().Size, 300, 200)
			},
			check: func(c *ctl) {
				if p := c.w[0].State().Position; p != fsBefore {
					t.Errorf("after full screen the position is %v, want %v", p, fsBefore)
				}
			}},
		ctlStep{name: "SetFullscreen(false) during the transition to full screen", timeout: 30 * time.Second,
			do: func(c *ctl) {
				entered = c.w[0].ctl.fsEntered
				c.w[0].SetFullscreen(true)
			},
			until: func(c *ctl) bool {
				w := c.w[0]
				if !askedDuring && (w.ctl.fsTransition || w.ctl.fsAsked != 0 || w.ctl.fullscreen) {
					sawTransition = w.ctl.fsTransition || w.ctl.fsAsked != 0
					askedDuring = true
					t.Logf("asking to leave full screen: transition %v, asked %v, fullscreen %v", w.ctl.fsTransition, w.ctl.fsAsked != 0, w.ctl.fullscreen)
					w.SetFullscreen(false)
				}
				return askedDuring && !w.ctl.fsTransition && w.ctl.fsAsked == 0 && !w.ctl.fullscreen && !w.State().Fullscreen &&
					w.ctl.fsEntered > entered && sizeIs(w.Metrics().Size, 300, 200)
			},
			check: func(c *ctl) {
				if !sawTransition {
					t.Logf("note: the transition had ended when the request came; the wait was not exercised")
				}
			}},
		ctlStep{name: "requests undone in the same pass",
			do: func(c *ctl) {
				entered = c.w[0].ctl.fsEntered
				w := c.w[0]
				w.SetFullscreen(true)
				w.SetFullscreen(false)
				w.SetMaximized(true)
				w.SetMaximized(false)
			},
			until: func(c *ctl) bool { return c.after(800 * time.Millisecond) },
			check: func(c *ctl) {
				w := c.w[0]
				if w.ctl.fsEntered != entered || w.ctl.fsAsked != 0 || w.ctl.fsTransition || w.State().Fullscreen || w.State().Maximized {
					t.Errorf("requests undone in the same pass did something: %s", c.describe())
				}
				for _, e := range c.stepEvents(0) {
					if _, ok := e.e.(platform.WindowStateEvent); ok {
						t.Errorf("requests undone in the same pass reported %s", eventString(e.e))
					}
				}
			}},
	)
	if c != nil && t.Failed() {
		c.logEvents(0)
	}
}

// contentTopLeft returns the content view's top-left in the contract's
// coordinates, computed here from AppKit (the test's own conversion), and
// the content and frame rectangles in screen coordinates.
func contentTopLeft(w *Window) (image.Point, darwin.CGRect, darwin.CGRect) {
	inWindow := darwin.SendRect(w.view, wsel.convertRectToView, darwin.SendRect(w.view, sel.bounds), darwin.ID(0))
	r := darwin.SendRect(w.ns, wsel.convertRectToScreen, inWindow)
	main := darwin.Send(darwin.SendClass(app.screen, wsel.screens), wsel.objectAtIndex, uint(0))
	h := darwin.SendRect(main, wsel.frame).Size.Height
	return image.Pt(int(math.Round(r.Origin.X)), int(math.Round(h-(r.Origin.Y+r.Size.Height)))), r, darwin.SendRect(w.ns, wsel.frame)
}

// TestWindowPosition: WindowOptions.Position, SetPosition and the round
// trip of a reported position, for a titled and a frameless window.
func TestWindowPosition(t *testing.T) {
	screenInfo(t)
	var frames [2]darwin.CGRect
	at := func(c *ctl, i int, want image.Point) bool {
		w := c.w[i]
		got, _, _ := contentTopLeft(w)
		return w.State().HasPosition && w.State().Position == want && got == want
	}
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela position", Size: platform.Size{W: 240, H: 160}, HasPosition: true, Position: image.Pt(120, 160)},
		{Title: "tela position frameless", Size: platform.Size{W: 200, H: 120}, Frameless: true, HasPosition: true, Position: image.Pt(420, 260)},
	}, nil,
		ctlStep{name: "the positions of the options",
			until: func(c *ctl) bool { return at(c, 0, image.Pt(120, 160)) && at(c, 1, image.Pt(420, 260)) },
			check: func(c *ctl) {
				_, content, frame := contentTopLeft(c.w[1])
				if content.Origin.Y+content.Size.Height != frame.Origin.Y+frame.Size.Height || content.Size != frame.Size {
					t.Errorf("frameless: the content (%v) is not the frame (%v)", content, frame)
				}
				_, content, frame = contentTopLeft(c.w[0])
				t.Logf("titled: content %v, frame %v (title bar %v points)", content, frame, frame.Size.Height-content.Size.Height)
			}},
		ctlStep{name: "SetPosition",
			do: func(c *ctl) {
				c.w[0].SetPosition(image.Pt(200, 230))
				c.w[1].SetPosition(image.Pt(500, 300))
			},
			until: func(c *ctl) bool { return at(c, 0, image.Pt(200, 230)) && at(c, 1, image.Pt(500, 300)) }},
		ctlStep{name: "the reported positions given back",
			do: func(c *ctl) {
				for i, w := range c.w {
					frames[i] = darwin.SendRect(w.ns, wsel.frame)
					w.SetPosition(w.State().Position)
				}
			},
			until: func(c *ctl) bool { return c.after(400 * time.Millisecond) },
			check: func(c *ctl) {
				for i, w := range c.w {
					if f := darwin.SendRect(w.ns, wsel.frame); f != frames[i] {
						t.Errorf("window %d moved from %v to %v when given its own position %v", i, frames[i], f, w.State().Position)
					}
					for _, e := range c.stepEvents(i) {
						if _, ok := e.e.(platform.WindowStateEvent); ok {
							t.Errorf("window %d: giving back its position reported %s", i, eventString(e.e))
						}
					}
				}
			}},
	)
}

// TestWindowSizeLimits: SetMinSize and SetMaxSize resize a window outside
// them, the minimum wins over a smaller maximum, zero removes a limit and
// SetSize is clamped, for a titled and a frameless window (whose limits
// are the frame's).
func TestWindowSizeLimits(t *testing.T) {
	both := func(c *ctl, w, h float32) bool {
		return sizeIs(c.w[0].Metrics().Size, w, h) && sizeIs(c.w[1].Metrics().Size, w, h)
	}
	limits := func(w *Window) (lo, hi darwin.CGSize) {
		if w.opts.Frameless {
			return darwin.SendSize(w.ns, darwin.Sel("minSize")), darwin.SendSize(w.ns, darwin.Sel("maxSize"))
		}
		return darwin.SendSize(w.ns, darwin.Sel("contentMinSize")), darwin.SendSize(w.ns, darwin.Sel("contentMaxSize"))
	}
	// A zero minimum may read back as AppKit's own floor for a titled
	// frame (up to a title bar), and no maximum as any huge value.
	same := func(got, want float64) bool {
		switch {
		case want == 0:
			return got >= 0 && got <= 40
		case want == noMaxSize:
			return got >= 1e30
		}
		return got == want
	}
	checkLimits := func(c *ctl, what string, lo, hi darwin.CGSize) {
		for i, w := range c.w {
			gl, gh := limits(w)
			if !same(gl.Width, lo.Width) || !same(gl.Height, lo.Height) || !same(gh.Width, hi.Width) || !same(gh.Height, hi.Height) {
				t.Errorf("%s: window %d (frameless %v) has limits %v..%v, want %v..%v", what, i, w.opts.Frameless, gl, gh, lo, hi)
			} else if gl != lo || gh != hi {
				t.Logf("%s: window %d (frameless %v) reads back limits %v..%v for %v..%v", what, i, w.opts.Frameless, gl, gh, lo, hi)
			}
		}
	}
	all := func(f func(w *Window)) func(c *ctl) {
		return func(c *ctl) {
			for _, w := range c.w {
				f(w)
			}
		}
	}
	none := darwin.CGSize{Width: noMaxSize, Height: noMaxSize}
	// AppKit's isZoomed holds for a window at its maximum size (its
	// standard frame is clamped to it), which nobody zoomed.
	notMaximized := func(c *ctl, what string) {
		for i, w := range c.w {
			if w.State().Maximized {
				t.Errorf("%s: window %d reports Maximized (isZoomed %v); nobody zoomed it", what, i, darwin.SendBool(w.ns, wsel.isZoomed))
			}
			for _, e := range c.stepEvents(i) {
				if s, ok := e.e.(platform.WindowStateEvent); ok && s.State.Maximized {
					t.Errorf("%s: window %d reported %s", what, i, eventString(e.e))
				}
			}
		}
	}
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela limits", Size: platform.Size{W: 300, H: 200}, HasPosition: true, Position: image.Pt(60, 120)},
		{Title: "tela limits frameless", Size: platform.Size{W: 300, H: 200}, Frameless: true, HasPosition: true, Position: image.Pt(560, 120)},
	}, nil,
		ctlStep{name: "the sizes of the options", until: func(c *ctl) bool { return both(c, 300, 200) },
			check: func(c *ctl) {
				if f := darwin.SendRect(c.w[1].ns, wsel.frame); f.Size.Width != 300 || f.Size.Height != 200 {
					t.Errorf("frameless: the frame is %v, want the content size 300x200", f.Size)
				}
				checkLimits(c, "no limits", darwin.CGSize{}, none)
			}},
		ctlStep{name: "SetMinSize(400x300)", do: all(func(w *Window) { w.SetMinSize(platform.Size{W: 400, H: 300}) }),
			until: func(c *ctl) bool { return both(c, 400, 300) },
			check: func(c *ctl) { checkLimits(c, "minimum 400x300", darwin.CGSize{Width: 400, Height: 300}, none) }},
		ctlStep{name: "SetMaxSize(350x250) below the minimum", do: all(func(w *Window) { w.SetMaxSize(platform.Size{W: 350, H: 250}) }),
			until: func(c *ctl) bool { return c.after(400 * time.Millisecond) },
			check: func(c *ctl) {
				if !both(c, 400, 300) {
					t.Errorf("a maximum below the minimum changed the size: %s", c.describe())
				}
				checkLimits(c, "maximum below the minimum", darwin.CGSize{Width: 400, Height: 300}, darwin.CGSize{Width: 400, Height: 300})
				notMaximized(c, "at the limits 400x300..400x300")
			}},
		ctlStep{name: "SetMinSize(0)", do: all(func(w *Window) { w.SetMinSize(platform.Size{}) }),
			until: func(c *ctl) bool { return both(c, 350, 250) },
			check: func(c *ctl) {
				checkLimits(c, "maximum 350x250", darwin.CGSize{}, darwin.CGSize{Width: 350, Height: 250})
				notMaximized(c, "at the maximum size 350x250")
			}},
		ctlStep{name: "SetMaxSize(500x400), SetSize(800x600)", do: all(func(w *Window) {
			w.SetMaxSize(platform.Size{W: 500, H: 400})
			w.SetSize(platform.Size{W: 800, H: 600})
		}), until: func(c *ctl) bool { return both(c, 500, 400) && c.after(300*time.Millisecond) },
			check: func(c *ctl) { notMaximized(c, "at the maximum size 500x400") }},
		ctlStep{name: "SetMaximized(true) at the maximum size", do: all(func(w *Window) { w.SetMaximized(true) }),
			until: func(c *ctl) bool { return c.w[0].State().Maximized && c.w[1].State().Maximized },
			check: func(c *ctl) {
				if !both(c, 500, 400) {
					t.Logf("note: maximizing a window at its maximum size resized it: %s", c.describe())
				}
			}},
		ctlStep{name: "SetMaximized(false) at the maximum size", do: all(func(w *Window) { w.SetMaximized(false) }),
			timeout: 4 * time.Second,
			until: func(c *ctl) bool {
				return !c.w[0].State().Maximized && !c.w[1].State().Maximized && c.after(300*time.Millisecond)
			},
			check: func(c *ctl) { notMaximized(c, "after SetMaximized(false)") }},
		ctlStep{name: "SetMaxSize(0)", do: all(func(w *Window) { w.SetMaxSize(platform.Size{}) }),
			check: func(c *ctl) { checkLimits(c, "no limits again", darwin.CGSize{}, none) }},
	)
}

// TestAlwaysOnTop: the floating window level, from the options and from
// SetAlwaysOnTop.
func TestAlwaysOnTop(t *testing.T) {
	level := func(w *Window) int { return darwin.SendInt(w.ns, wsel.level) }
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela normal", Size: platform.Size{W: 160, H: 100}},
		{Title: "tela on top", Size: platform.Size{W: 160, H: 100}, AlwaysOnTop: true},
	}, nil,
		ctlStep{name: "the levels of the options", check: func(c *ctl) {
			if l0, l1 := level(c.w[0]), level(c.w[1]); l0 != darwin.NSNormalWindowLevel || l1 != darwin.NSFloatingWindowLevel {
				t.Errorf("levels %d and %d, want %d (normal) and %d (floating)", l0, l1, darwin.NSNormalWindowLevel, darwin.NSFloatingWindowLevel)
			}
		}},
		ctlStep{name: "SetAlwaysOnTop", do: func(c *ctl) {
			c.w[0].SetAlwaysOnTop(true)
			c.w[1].SetAlwaysOnTop(false)
		}, check: func(c *ctl) {
			if l0, l1 := level(c.w[0]), level(c.w[1]); l0 != darwin.NSFloatingWindowLevel || l1 != darwin.NSNormalWindowLevel {
				t.Errorf("after SetAlwaysOnTop: levels %d and %d, want %d and %d", l0, l1, darwin.NSFloatingWindowLevel, darwin.NSNormalWindowLevel)
			}
		}},
	)
}

// --- parent and modal windows ---

// orderedIndex returns w's index in [NSApp orderedWindows] (front to
// back), or -1.
func orderedIndex(w *Window) int {
	list := darwin.Send(app.ns, darwin.Sel("orderedWindows"))
	n := int(darwin.SendUint(list, wsel.count))
	for i := range n {
		if darwin.Send(list, wsel.objectAtIndex, uint(i)) == w.ns {
			return i
		}
	}
	return -1
}

func childAbove(c *ctl, parent, child int) bool {
	pi, ci := orderedIndex(c.w[parent]), orderedIndex(c.w[child])
	return ci >= 0 && pi >= 0 && ci < pi
}

// TestChildWindow: a child window is attached above its parent, centered
// on it, stays above it when the parent is ordered front, and is closed
// with it.
func TestChildWindow(t *testing.T) {
	var childFrame darwin.CGRect
	ctlRunRounds(t, [][]platform.WindowOptions{{{Title: "tela parent", Size: platform.Size{W: 400, H: 300}, HasPosition: true, Position: image.Pt(100, 120)}}},
		func(c *ctl) []platform.WindowOptions {
			return []platform.WindowOptions{{Title: "tela child", Size: platform.Size{W: 200, H: 120}, Parent: c.w[0]}}
		}, nil,
		ctlStep{name: "the child is shown above its parent", until: func(c *ctl) bool { return childAbove(c, 0, 1) },
			check: func(c *ctl) {
				if p := darwin.Send(c.w[1].ns, wsel.parentWindow); p != c.w[0].ns {
					t.Errorf("the child's parentWindow is %#x, want the parent %#x", p, c.w[0].ns)
				}
				pf, cf := darwin.SendRect(c.w[0].ns, wsel.frame), darwin.SendRect(c.w[1].ns, wsel.frame)
				dx := (pf.Origin.X + pf.Size.Width/2) - (cf.Origin.X + cf.Size.Width/2)
				dy := (pf.Origin.Y + pf.Size.Height/2) - (cf.Origin.Y + cf.Size.Height/2)
				if math.Abs(dx) > 1 || math.Abs(dy) > 1 {
					t.Errorf("the child (%v) is not centered on its parent (%v): off by %v,%v", cf, pf, dx, dy)
				}
				t.Logf("ordered windows: parent %d, child %d", orderedIndex(c.w[0]), orderedIndex(c.w[1]))
			}},
		ctlStep{name: "the parent ordered front",
			do:    func(c *ctl) { darwin.Send(c.w[0].ns, sel.makeKeyAndOrderFront, darwin.ID(0)) },
			until: func(c *ctl) bool { return c.after(300 * time.Millisecond) },
			check: func(c *ctl) {
				if !childAbove(c, 0, 1) {
					t.Errorf("the parent ordered front covers its child: parent %d, child %d in orderedWindows", orderedIndex(c.w[0]), orderedIndex(c.w[1]))
				}
			}},
		ctlStep{name: "the parent moves",
			do: func(c *ctl) {
				childFrame = darwin.SendRect(c.w[1].ns, wsel.frame)
				c.w[0].SetPosition(image.Pt(160, 150))
			},
			until: func(c *ctl) bool { return c.after(300 * time.Millisecond) },
			check: func(c *ctl) {
				t.Logf("child frame before %v, after the parent moved by (60,30): %v", childFrame, darwin.SendRect(c.w[1].ns, wsel.frame))
			}},
		ctlStep{name: "the parent closes", do: func(c *ctl) {
			child := c.w[1]
			c.w[0].Close()
			if !child.Closed() {
				t.Errorf("closing the parent left the child open")
			}
			if darwin.SendBool(child.ns, darwin.Sel("isVisible")) {
				t.Errorf("the child is still visible after its parent closed")
			}
		}},
	)
}

// TestModalWindow: while a modal child is shown, its parent's standard
// buttons are disabled, and making the parent key (as a click, Command-`
// or Mission Control do), clicking it or asking it to close brings the
// child forward with the focus instead. Hiding or closing the child gives
// the parent its buttons and the focus back.
func TestModalWindow(t *testing.T) {
	appActive := false
	var parentFocus int
	countFocus := func(c *ctl) int {
		n := 0
		for _, e := range c.stepEvents(0) {
			if f, ok := e.e.(platform.FocusEvent); ok && f.Focused {
				n++
			}
		}
		return n
	}
	buttons := func(c *ctl, enabled bool) {
		for _, b := range []uintptr{darwin.NSWindowCloseButton, darwin.NSWindowMiniaturizeButton, darwin.NSWindowZoomButton} {
			if got := buttonEnabled(c.w[0], b); got != enabled {
				t.Errorf("the parent's standard button %d is enabled %v, want %v (blocked %v)", b, got, enabled, c.w[0].ctl.blocked)
			}
		}
	}
	childKey := func(c *ctl) bool { return !appActive || (isKey(c.w[1]) && !isKey(c.w[0])) }
	parentKey := func(c *ctl) bool { return !appActive || isKey(c.w[0]) }
	ctlRunRounds(t, [][]platform.WindowOptions{{{Title: "tela modal parent", Size: platform.Size{W: 400, H: 300}, HasPosition: true, Position: image.Pt(100, 120)}}},
		func(c *ctl) []platform.WindowOptions {
			return []platform.WindowOptions{{Title: "tela modal", Size: platform.Size{W: 220, H: 140}, Parent: c.w[0], Modal: true}}
		}, nil,
		ctlStep{name: "the modal child is shown", until: func(c *ctl) bool { return isKey(c.w[1]) || c.after(3*time.Second) },
			check: func(c *ctl) {
				appActive = darwin.SendBool(app.ns, darwin.Sel("isActive")) && isKey(c.w[1])
				if !appActive {
					t.Logf("note: the app is not active or the child is not key (active %v); the focus checks are skipped",
						darwin.SendBool(app.ns, darwin.Sel("isActive")))
				}
				buttons(c, false)
				if !childAbove(c, 0, 1) {
					t.Errorf("the modal child is not above its parent")
				}
			}},
		ctlStep{name: "the parent made key",
			do:    func(c *ctl) { darwin.Send(c.w[0].ns, sel.makeKeyAndOrderFront, darwin.ID(0)) },
			until: func(c *ctl) bool { return c.after(200*time.Millisecond) && childKey(c) },
			check: func(c *ctl) {
				if n := countFocus(c); n != 0 {
					t.Errorf("the parent reported %d FocusEvent{true} while its modal child is shown", n)
				}
			}},
		ctlStep{name: "a click on the parent",
			do: func(c *ctl) {
				c.send(c.mouse(0, darwin.NSEventTypeLeftMouseDown, platform.Point{X: 30, Y: 250}, 0, 1))
				c.send(c.mouse(0, darwin.NSEventTypeLeftMouseUp, platform.Point{X: 30, Y: 250}, 20, 1))
			},
			until: func(c *ctl) bool { return c.after(200*time.Millisecond) && childKey(c) },
			check: func(c *ctl) { parentFocus = countFocus(c) }},
		ctlStep{name: "a close request on the parent",
			do: func(c *ctl) {
				darwin.SendBool(c.w[0].delegate, darwin.Sel("windowShouldClose:"), c.w[0].ns)
			},
			until: func(c *ctl) bool { return c.after(200 * time.Millisecond) },
			check: func(c *ctl) {
				for _, e := range c.stepEvents(0) {
					if _, ok := e.e.(platform.CloseRequestedEvent); ok {
						t.Errorf("the parent got a CloseRequestedEvent while its modal child is shown")
					}
				}
				if parentFocus != 0 {
					t.Errorf("the click on the parent gave it the focus (%d FocusEvent{true})", parentFocus)
				}
			}},
		ctlStep{name: "the modal child hidden",
			do:    func(c *ctl) { c.w[1].SetVisible(false) },
			until: func(c *ctl) bool { return parentKey(c) },
			check: func(c *ctl) { buttons(c, true) }},
		ctlStep{name: "the modal child shown again",
			do:    func(c *ctl) { c.w[1].SetVisible(true) },
			until: func(c *ctl) bool { return c.w[1].shown && childKey(c) },
			check: func(c *ctl) { buttons(c, false) }},
		ctlStep{name: "the modal child closed",
			do:    func(c *ctl) { c.w[1].Close() },
			until: func(c *ctl) bool { return parentKey(c) },
			check: func(c *ctl) { buttons(c, true) }},
	)
}

// --- frameless windows ---

// captureImage returns the window as the window server composited it
// (CGWindowListCreateImage, without the shadow), or nil.
func captureImage(w *Window) *image.RGBA {
	num := darwin.SendInt(w.ns, sel.windowNumber)
	img := darwin.CGWindowListCreateImage(darwin.CGRectNull, darwin.CGWindowListOptionIncludingWindow, uint32(num),
		darwin.CGWindowImageBoundsIgnoreFraming|darwin.CGWindowImageBestResolution)
	if img == 0 {
		return nil
	}
	defer darwin.CGImageRelease(img)
	return bitmap(img, darwin.CGImageGetWidth(img), darwin.CGImageGetHeight(img), w.b.srgb)
}

// TestFrameless: a frameless window is a titled window with a full-size
// content view, a transparent title bar, no title and hidden standard
// buttons; it is resizable unless FixedSize, has the system's shadow and
// rounded corners, becomes key, reports client-side decorations, and its
// content fills the frame pixel for pixel.
func TestFrameless(t *testing.T) {
	pattern := false
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela frameless", Size: platform.Size{W: 260, H: 180}, Frameless: true, HasPosition: true, Position: image.Pt(100, 150)},
		{Title: "tela frameless fixed", Size: platform.Size{W: 200, H: 120}, Frameless: true, FixedSize: true, HasPosition: true, Position: image.Pt(500, 150)},
	}, func(c *ctl, i int, w *Window) {
		if i == 0 && pattern {
			if err := w.Present(testPattern(w.Metrics().Physical()), nil); err != nil {
				t.Errorf("Present: %v", err)
			}
			return
		}
		presentSolid(t, w, color.RGBA{0x70, 0x80, 0x90, 0xff})
	},
		ctlStep{name: "the frameless style", until: func(c *ctl) bool { return c.w[0].State().ClientSideDecorations && c.w[1].State().ClientSideDecorations },
			check: func(c *ctl) {
				for i, w := range c.w {
					mask := darwin.SendUint(w.ns, wsel.styleMask)
					want := uint64(darwin.NSWindowStyleMaskTitled | darwin.NSWindowStyleMaskFullSizeContentView)
					if mask&want != want {
						t.Errorf("window %d: styleMask %#x lacks Titled|FullSizeContentView", i, mask)
					}
					if resizable := mask&darwin.NSWindowStyleMaskResizable != 0; resizable == w.opts.FixedSize {
						t.Errorf("window %d: resizable %v with FixedSize %v", i, resizable, w.opts.FixedSize)
					}
					for _, b := range []uintptr{darwin.NSWindowCloseButton, darwin.NSWindowMiniaturizeButton, darwin.NSWindowZoomButton} {
						if btn := darwin.Send(w.ns, wsel.standardWindowButton, b); btn != 0 && !darwin.SendBool(btn, darwin.Sel("isHidden")) {
							t.Errorf("window %d: standard button %d is shown", i, b)
						}
					}
					if !darwin.SendBool(w.ns, darwin.Sel("titlebarAppearsTransparent")) || darwin.SendInt(w.ns, darwin.Sel("titleVisibility")) != darwin.NSWindowTitleHidden {
						t.Errorf("window %d: the title bar is drawn or the title shown", i)
					}
					if !darwin.SendBool(w.ns, darwin.Sel("hasShadow")) {
						t.Errorf("window %d has no shadow", i)
					}
					f, b := darwin.SendRect(w.ns, wsel.frame), darwin.SendRect(w.view, sel.bounds)
					if f.Size != b.Size || !sizeIs(w.Metrics().Size, float32(w.opts.Size.W), float32(w.opts.Size.H)) {
						t.Errorf("window %d: frame %v, view %v, metrics %v; want all %v", i, f.Size, b.Size, w.Metrics().Size, w.opts.Size)
					}
				}
			}},
		ctlStep{name: "a frameless window becomes key",
			do:    func(c *ctl) { c.w[0].Activate() },
			until: func(c *ctl) bool { return isKey(c.w[0]) && darwin.SendBool(c.w[0].ns, darwin.Sel("isMainWindow")) }},
		ctlStep{name: "the content fills the frame",
			do:    func(c *ctl) { pattern = true; c.w[0].RequestFrame() },
			until: func(c *ctl) bool { return c.after(500 * time.Millisecond) },
			check: func(c *ctl) {
				w := c.w[0]
				pix := captureImage(w)
				if pix == nil {
					t.Logf("note: the window capture was refused; the pixels are not checked")
					return
				}
				phys := w.Metrics().Physical()
				if pix.Rect.Size() != phys {
					t.Errorf("the capture is %v, want the content's %v (a frameless window is its content)", pix.Rect.Size(), phys)
					return
				}
				cmp := compare(pix)
				t.Logf("frameless capture: max channel difference %d (placement %v, checker %v),%s", cmp.maxDiff, cmp.placement, cmp.checker, cmp.detail)
				if !cmp.ok(false) || cmp.upsideDown {
					t.Errorf("the frameless window's pixels are not the frame presented")
				}
				corner := pix.RGBAAt(0, 0)
				t.Logf("top-left corner pixel %v (rounded corners are transparent there)", corner)
				if corner.A == 255 {
					t.Errorf("the top-left corner pixel is opaque %v: no rounded corners", corner)
				}
			}},
	)
}

// --- custom title bars ---

// TestCaptionRegions: in a frameless window (whose title bar strip is
// over the content), presses in a caption region start a window drag and
// are not delivered with their drags and releases, other buttons there
// are swallowed, presses in a HitClient hole, an app button region and
// the content are delivered, hover is delivered, and a double click by
// the shared rule performs the user's title bar action.
func TestCaptionRegions(t *testing.T) {
	screenInfo(t)
	var drags []platform.Point
	saved := windowDrag
	windowDrag = func(w *Window, ev darwin.ID) {
		w.ctl.drags++
		drags = append(drags, w.eventPos(ev))
	}
	defer func() { windowDrag = saved }()
	pt := func(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }
	const (
		lDown, lUp, rDown, rUp = darwin.NSEventTypeLeftMouseDown, darwin.NSEventTypeLeftMouseUp, darwin.NSEventTypeRightMouseDown, darwin.NSEventTypeRightMouseUp
		moved, lDragged        = darwin.NSEventTypeMouseMoved, darwin.NSEventTypeLeftMouseDragged
	)
	var action titleBarAction
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela caption", Size: platform.Size{W: 300, H: 200}, Frameless: true, HasPosition: true, Position: image.Pt(150, 150)},
	}, nil,
		ctlStep{name: "presses in and out of the caption",
			do: func(c *ctl) {
				w := c.w[0]
				w.SetHitRegions([]platform.HitRegion{
					{Kind: platform.HitCaption, Rect: platform.Rect{W: 300, H: 40}},
					{Kind: platform.HitClient, Rect: platform.Rect{X: 100, Y: 5, W: 60, H: 30}},
					{Kind: platform.HitClose, Rect: platform.Rect{X: 260, Y: 5, W: 30, H: 30}},
				})
				// The real pointer may be over the window: start from a
				// pointer outside, as the script assumes.
				w.ptr = pointerState{}
				darwin.Send(w.view, darwin.Sel("mouseMoved:"), c.mouse(0, moved, pt(50, 20), 0, 0))
				c.send(c.mouse(0, lDown, pt(50, 20), 20, 1))
				c.send(c.mouse(0, lDragged, pt(55, 22), 40, 1))
				c.send(c.mouse(0, lUp, pt(55, 22), 60, 1))
				c.send(c.mouse(0, rDown, pt(50, 20), 100, 1))
				c.send(c.mouse(0, rUp, pt(50, 20), 120, 1))
				c.send(c.mouse(0, lDown, pt(120, 20), 1000, 1))
				c.send(c.mouse(0, lUp, pt(120, 20), 1020, 1))
				c.send(c.mouse(0, lDown, pt(270, 20), 1100, 1))
				c.send(c.mouse(0, lUp, pt(270, 20), 1120, 1))
				c.send(c.mouse(0, lDown, pt(50, 100), 1200, 1))
				c.send(c.mouse(0, lUp, pt(50, 100), 1220, 1))
			},
			until: func(c *ctl) bool { return c.after(300 * time.Millisecond) },
			check: func(c *ctl) {
				var got []string
				for _, e := range c.stepEvents(0) {
					if pe, ok := e.e.(platform.PointerEvent); ok {
						got = append(got, pointerString(pe))
					}
				}
				want := []string{
					"enter 50,20", "move 50,20",
					"down 120,20 button=1 held=1", "up 120,20 button=1",
					"down 270,20 button=1 held=1", "up 270,20 button=1",
					"down 50,100 button=1 held=1", "up 50,100 button=1",
				}
				if strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Errorf("pointer events differ:\n got:\n  %s\n want:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
				}
				if len(drags) != 1 || drags[0] != pt(50, 20) {
					t.Errorf("window drags started at %v, want one at 50,20", drags)
				}
			}},
		ctlStep{name: "a double click in the caption",
			do: func(c *ctl) {
				action = doubleClickAction(userDoubleClickSetting())
				t.Logf("the user's title bar double-click action: %v", action)
				c.send(c.mouse(0, lDown, pt(60, 20), 0, 1))
				c.send(c.mouse(0, lUp, pt(60, 20), 20, 1))
				c.send(c.mouse(0, lDown, pt(61, 20), 150, 2))
				c.send(c.mouse(0, lUp, pt(61, 20), 170, 2))
			},
			until: func(c *ctl) bool {
				st := c.w[0].State()
				switch action {
				case titleBarZoom:
					return st.Maximized
				case titleBarMinimize:
					return st.Minimized
				}
				return c.after(500 * time.Millisecond)
			},
			check: func(c *ctl) {
				if c.w[0].ctl.doubleClicks != 1 || len(drags) != 2 {
					t.Errorf("%d double clicks and %d drags, want 1 and 2 (the second press is not a drag)", c.w[0].ctl.doubleClicks, len(drags))
				}
				for _, e := range c.stepEvents(0) {
					if pe, ok := e.e.(platform.PointerEvent); ok && pe.Action != platform.PointerMove {
						t.Errorf("a caption double click delivered %s", pointerString(pe))
					}
				}
			}},
		ctlStep{name: "back from the double click",
			do: func(c *ctl) {
				c.w[0].SetMaximized(false)
				c.w[0].Activate()
			},
			until: func(c *ctl) bool { st := c.w[0].State(); return !st.Maximized && !st.Minimized }},
	)
}

// --- backdrop ---

// TestBackdrop: a vibrancy backdrop puts an NSVisualEffectView with the
// mapped material behind TelaView and makes the window non-opaque, so a
// transparent frame shows the material instead of today's opaque window
// background; other backdrops change the material, and none or default
// make the window opaque again.
func TestBackdrop(t *testing.T) {
	screenInfo(t)
	transparent := func(c *ctl, i int, w *Window) {
		fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()}) // premultiplied {0,0,0,0}
		if err := w.Present(fb, nil); err != nil {
			t.Errorf("Present: %v", err)
		}
	}
	material := func(w *Window) int { return darwin.SendInt(w.ctl.effect, darwin.Sel("material")) }
	expectGlass := func(c *ctl, i, want int) {
		w := c.w[i]
		switch {
		case w.ctl.effect == 0:
			t.Errorf("window %d (%v): no effect view", i, w.ctl.backdrop)
			return
		case material(w) != want:
			t.Errorf("window %d (%v): material %d, want %d", i, w.ctl.backdrop, material(w), want)
		}
		if m := darwin.SendInt(w.ctl.effect, darwin.Sel("blendingMode")); m != darwin.NSVisualEffectBlendingModeBehindWindow {
			t.Errorf("window %d: blending mode %d, want behind the window", i, m)
		}
		if s := darwin.SendInt(w.ctl.effect, darwin.Sel("state")); s != darwin.NSVisualEffectStateFollowsWindowActiveState {
			t.Errorf("window %d: effect state %d, want following the window's active state", i, s)
		}
		subs := darwin.Send(w.ctl.container, darwin.Sel("subviews"))
		if darwin.SendUint(subs, wsel.count) != 2 || darwin.Send(subs, wsel.objectAtIndex, uint(0)) != w.ctl.effect ||
			darwin.Send(subs, wsel.objectAtIndex, uint(1)) != w.view {
			t.Errorf("window %d: the content view's subviews are not [effect view, TelaView]", i)
		}
		if darwin.SendBool(w.ns, darwin.Sel("isOpaque")) || darwin.SendBool(w.view, darwin.Sel("isOpaque")) || !w.PresentsAlpha() {
			t.Errorf("window %d: opaque window %v, opaque view %v, PresentsAlpha %v; want false, false, true", i,
				darwin.SendBool(w.ns, darwin.Sel("isOpaque")), darwin.SendBool(w.view, darwin.Sel("isOpaque")), w.PresentsAlpha())
		}
	}
	expectOpaque := func(c *ctl, i int) {
		w := c.w[i]
		if w.ctl.effect != 0 || !darwin.SendBool(w.ns, darwin.Sel("isOpaque")) || !darwin.SendBool(w.view, darwin.Sel("isOpaque")) || w.PresentsAlpha() {
			t.Errorf("window %d (%v): effect view %#x, opaque window %v, opaque view %v, PresentsAlpha %v; want none, true, true, false",
				i, w.ctl.backdrop, w.ctl.effect, darwin.SendBool(w.ns, darwin.Sel("isOpaque")), darwin.SendBool(w.view, darwin.Sel("isOpaque")), w.PresentsAlpha())
		}
		if n := darwin.SendUint(darwin.Send(w.ctl.container, darwin.Sel("subviews")), wsel.count); n != 1 {
			t.Errorf("window %d: the content view has %d subviews, want TelaView only", i, n)
		}
	}
	center := func(w *Window) (color.RGBA, bool) {
		pix := captureImage(w)
		if pix == nil {
			return color.RGBA{}, false
		}
		// The content is at the bottom of the capture (title bar on top).
		phys := w.Metrics().Physical()
		x := pix.Rect.Dx() / 2
		y := pix.Rect.Dy() - phys.Y/2
		return pix.RGBAAt(x, y), true
	}
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela opaque", Size: platform.Size{W: 200, H: 150}, Backdrop: platform.BackdropNone, HasPosition: true, Position: image.Pt(100, 150)},
		{Title: "tela acrylic", Size: platform.Size{W: 200, H: 150}, Backdrop: platform.BackdropAcrylic, HasPosition: true, Position: image.Pt(400, 150)},
	}, transparent,
		ctlStep{name: "the backdrops of the options",
			until: func(c *ctl) bool { return c.after(600 * time.Millisecond) },
			check: func(c *ctl) {
				expectOpaque(c, 0)
				expectGlass(c, 1, materialSidebar)
				opaque, ok0 := center(c.w[0])
				glass, ok1 := center(c.w[1])
				if !ok0 || !ok1 {
					t.Logf("note: the window capture was refused; the pixels are not checked")
					return
				}
				t.Logf("a transparent frame reads back as %v without a backdrop and %v with Acrylic", opaque, glass)
				if opaque.A != 255 {
					t.Errorf("without a backdrop the window is not opaque: %v", opaque)
				}
				if glass.A >= 250 && channelDiff(glass, opaque) <= 6 {
					t.Errorf("with a backdrop a transparent frame reads back as %v, the opaque background of a window without one (%v)", glass, opaque)
				}
			}},
		ctlStep{name: "the backdrops changed",
			do: func(c *ctl) {
				c.w[0].SetBackdrop(platform.BackdropMica)
				c.w[1].SetBackdrop(platform.BackdropMicaAlt)
			},
			check: func(c *ctl) {
				expectGlass(c, 0, materialWindowBackground)
				expectGlass(c, 1, materialUnderWindowBackground)
			}},
		ctlStep{name: "none and default",
			do: func(c *ctl) {
				c.w[0].SetBackdrop(platform.BackdropNone)
				c.w[1].SetBackdrop(platform.BackdropDefault)
			},
			check: func(c *ctl) {
				expectOpaque(c, 0)
				expectOpaque(c, 1)
			}},
		ctlStep{name: "a backdrop again",
			do: func(c *ctl) { c.w[1].SetBackdrop(platform.BackdropAcrylic) },
			check: func(c *ctl) {
				expectGlass(c, 1, materialSidebar)
			}},
	)
}

// --- title bar overlays ---

func rectNear(a, b platform.Rect) bool {
	near := func(x, y float32) bool { return math.Abs(float64(x-y)) < 0.5 }
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H)
}

func centerY(r platform.Rect) float32 { return r.Y + r.H/2 }

// overlayHit reports whether a press at the center of standard button b
// would reach that button: the theme frame's hit test at that point finds
// the button or a view inside it (a button moved out of its superviews'
// bounds is drawn but not hit).
func overlayHit(w *Window, b int) (bool, string) {
	btn := w.ctl.overlay.buttons[b]
	bounds := darwin.SendRect(btn, sel.bounds)
	center := darwin.CGPoint{X: bounds.Origin.X + bounds.Size.Width/2, Y: bounds.Origin.Y + bounds.Size.Height/2}
	inWindow := darwin.SendPoint(btn, isel.convertPointToView, center, darwin.ID(0))
	theme := darwin.Send(w.ctl.container, osel.superview)
	hit := darwin.Send(theme, darwin.Sel("hitTest:"), inWindow)
	name := "nil"
	if hit != 0 {
		name = darwin.GoString(darwin.Send(hit, osel.className))
	}
	return hit != 0 && (hit == btn || darwin.SendBool(hit, darwin.Sel("isDescendantOf:"), btn)), name
}

// TestTitleBarOverlay: a TitleBarOverlay window is a frameless window with
// AppKit's standard buttons shown, reports client-side decorations and the
// buttons' rectangle (TitleBarControls), centers the buttons in the height
// of a caption region along the top and follows it, keeps that place
// across a resize and a minimization, goes back to AppKit's place without
// the caption, reports an empty rectangle in full screen, and its close
// and minimize buttons ask to close and minimize.
func TestTitleBarOverlay(t *testing.T) {
	screenInfo(t)
	var native platform.Rect // AppKit's place
	caption := func(h float32) []platform.HitRegion {
		return []platform.HitRegion{
			{Kind: platform.HitCaption, Rect: platform.Rect{W: 2000, H: h}},
			{Kind: platform.HitClient, Rect: platform.Rect{X: 200, Y: 8, W: 100, H: h - 16}}, // a search field
		}
	}
	// placed reports whether the buttons and the reported rectangle are
	// centered at cy (AppKit's x kept).
	placed := func(w *Window, cy float32) bool {
		live, ok := w.buttonsRect()
		st := w.State().TitleBarControls
		return ok && math.Abs(float64(centerY(live)-cy)) < 0.5 && rectNear(st, live) && math.Abs(float64(live.X-native.X)) < 0.5 &&
			math.Abs(float64(live.W-native.W)) < 0.5
	}
	closes := func(c *ctl) int {
		n := 0
		for _, e := range c.stepEvents(0) {
			if _, ok := e.e.(platform.CloseRequestedEvent); ok {
				n++
			}
		}
		return n
	}
	hits := func(c *ctl, what string) {
		for b := range 3 {
			if ok, name := overlayHit(c.w[0], b); !ok {
				t.Errorf("%s: a press at the center of button %d hits %s, not the button; %s", what, b, name, c.w[0].overlayDescribe())
			}
		}
	}
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela overlay", Size: platform.Size{W: 400, H: 260}, TitleBarOverlay: true, HasPosition: true, Position: image.Pt(150, 150)},
	}, nil,
		ctlStep{name: "the overlay style", until: func(c *ctl) bool {
			st := c.w[0].State()
			return st.ClientSideDecorations && st.TitleBarControls != (platform.Rect{})
		},
			check: func(c *ctl) {
				w := c.w[0]
				mask := darwin.SendUint(w.ns, wsel.styleMask)
				if want := uint64(darwin.NSWindowStyleMaskTitled | darwin.NSWindowStyleMaskFullSizeContentView); mask&want != want {
					t.Errorf("styleMask %#x lacks Titled|FullSizeContentView", mask)
				}
				for _, b := range overlayButtons {
					if btn := darwin.Send(w.ns, wsel.standardWindowButton, b); btn == 0 || darwin.SendBool(btn, osel.isHidden) {
						t.Errorf("standard button %d is missing or hidden", b)
					}
				}
				if !darwin.SendBool(w.ns, darwin.Sel("titlebarAppearsTransparent")) || darwin.SendInt(w.ns, darwin.Sel("titleVisibility")) != darwin.NSWindowTitleHidden {
					t.Errorf("the title bar is drawn or the title shown")
				}
				live, _ := w.buttonsRect()
				native = w.State().TitleBarControls
				if !rectNear(native, live) {
					t.Errorf("TitleBarControls %+v, but the buttons cover %+v", native, live)
				}
				f, b := darwin.SendRect(w.ns, wsel.frame), darwin.SendRect(w.view, sel.bounds)
				if f.Size != b.Size {
					t.Errorf("the content view (%v) is not the frame (%v)", b.Size, f.Size)
				}
				t.Logf("AppKit's place: TitleBarControls %+v (center y %v); %s", native, centerY(native), w.overlayDescribe())
				hits(c, "AppKit's place")
			}},
		ctlStep{name: "a 40-point caption (for telatest.MacTitleBarControls)",
			do:    func(c *ctl) { c.w[0].SetHitRegions(caption(40)) },
			until: func(c *ctl) bool { return placed(c.w[0], 20) },
			check: func(c *ctl) {
				t.Logf("a 40-point caption: TitleBarControls %+v; %s", c.w[0].State().TitleBarControls, c.w[0].overlayDescribe())
			}},
		ctlStep{name: "a 52-point caption",
			do:    func(c *ctl) { c.w[0].SetHitRegions(caption(52)) },
			until: func(c *ctl) bool { return placed(c.w[0], 26) },
			check: func(c *ctl) {
				t.Logf("a 52-point caption: TitleBarControls %+v; %s", c.w[0].State().TitleBarControls, c.w[0].overlayDescribe())
				hits(c, "a 52-point caption")
				var reported bool
				for _, e := range c.stepEvents(0) {
					if s, ok := e.e.(platform.WindowStateEvent); ok && math.Abs(float64(centerY(s.State.TitleBarControls)-26)) < 0.5 {
						reported = true
					}
				}
				if !reported {
					t.Errorf("no WindowStateEvent reported the buttons centered at 26")
					c.logEvents(c.mark)
				}
			}},
		ctlStep{name: "a resize keeps the place",
			do: func(c *ctl) { c.w[0].SetSize(platform.Size{W: 480, H: 300}) },
			until: func(c *ctl) bool {
				return sizeIs(c.w[0].Metrics().Size, 480, 300) && c.after(300*time.Millisecond) && placed(c.w[0], 26)
			}},
		ctlStep{name: "the caption removed",
			do: func(c *ctl) { c.w[0].SetHitRegions(nil) },
			until: func(c *ctl) bool {
				live, _ := c.w[0].buttonsRect()
				return rectNear(c.w[0].State().TitleBarControls, native) && rectNear(live, native)
			}},
		ctlStep{name: "the caption again",
			do:    func(c *ctl) { c.w[0].SetHitRegions(caption(52)) },
			until: func(c *ctl) bool { return placed(c.w[0], 26) }},
		ctlStep{name: "the close button",
			do: func(c *ctl) {
				darwin.Send(c.w[0].ctl.overlay.buttons[0], darwin.Sel("performClick:"), darwin.ID(0))
			},
			until: func(c *ctl) bool { return c.after(300 * time.Millisecond) },
			check: func(c *ctl) {
				if n := closes(c); n != 1 {
					t.Errorf("the close button gave %d CloseRequestedEvents, want 1", n)
				}
				if c.w[0].Closed() || !darwin.SendBool(c.w[0].ns, darwin.Sel("isVisible")) {
					t.Errorf("the close button closed the window; closing is the app's decision")
				}
			}},
		ctlStep{name: "the minimize button",
			do: func(c *ctl) {
				darwin.Send(c.w[0].ctl.overlay.buttons[1], darwin.Sel("performClick:"), darwin.ID(0))
			},
			until: func(c *ctl) bool { return c.w[0].State().Minimized }},
		ctlStep{name: "back from the Dock",
			do: func(c *ctl) { c.w[0].Activate() },
			until: func(c *ctl) bool {
				return !c.w[0].State().Minimized && c.after(300*time.Millisecond) && placed(c.w[0], 26)
			}},
		ctlStep{name: "full screen", timeout: 20 * time.Second,
			do:    func(c *ctl) { c.w[0].SetFullscreen(true) },
			until: func(c *ctl) bool { return c.w[0].State().Fullscreen && !c.w[0].fsBusy() },
			check: func(c *ctl) {
				w := c.w[0]
				if r := w.State().TitleBarControls; r != (platform.Rect{}) {
					t.Errorf("in full screen TitleBarControls is %+v, want empty (the buttons are in the auto-hiding title bar)", r)
				}
				t.Logf("in full screen: %s", w.overlayDescribe())
			}},
		ctlStep{name: "full screen out", timeout: 20 * time.Second,
			do: func(c *ctl) { c.w[0].SetFullscreen(false) },
			until: func(c *ctl) bool {
				w := c.w[0]
				return !w.State().Fullscreen && !w.fsBusy() && sizeIs(w.Metrics().Size, 480, 300) && placed(w, 26)
			},
			check: func(c *ctl) {
				t.Logf("after full screen: TitleBarControls %+v; %s", c.w[0].State().TitleBarControls, c.w[0].overlayDescribe())
				hits(c, "after full screen")
			}},
	)
}

// TestCaptionDragReal runs the real performWindowDragWithEvent: for a
// caption press built in-process, which the window server gets with no
// button down. It must come back (the watchdog fails the test with this
// step's name otherwise), and the press is not delivered. It is the last
// test of the package's files on purpose: a hang loses nothing else.
func TestCaptionDragReal(t *testing.T) {
	var before image.Point
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela drag", Size: platform.Size{W: 240, H: 160}, Frameless: true, HasPosition: true, Position: image.Pt(200, 200)},
	}, nil,
		ctlStep{name: "a real window drag",
			do: func(c *ctl) {
				w := c.w[0]
				before = w.State().Position
				w.SetHitRegions([]platform.HitRegion{{Kind: platform.HitCaption, Rect: platform.Rect{W: 240, H: 32}}})
				start := time.Now()
				c.send(c.mouse(0, darwin.NSEventTypeLeftMouseDown, platform.Point{X: 40, Y: 16}, 0, 1))
				t.Logf("mouseDown with performWindowDragWithEvent: came back after %v", time.Since(start).Round(time.Millisecond))
				c.send(c.mouse(0, darwin.NSEventTypeLeftMouseUp, platform.Point{X: 40, Y: 16}, 20, 1))
				c.send(c.mouse(0, darwin.NSEventTypeLeftMouseDown, platform.Point{X: 40, Y: 100}, 400, 1))
				c.send(c.mouse(0, darwin.NSEventTypeLeftMouseUp, platform.Point{X: 40, Y: 100}, 420, 1))
			},
			until: func(c *ctl) bool { return c.after(400 * time.Millisecond) },
			check: func(c *ctl) {
				w := c.w[0]
				var downs []string
				for _, e := range c.stepEvents(0) {
					if pe, ok := e.e.(platform.PointerEvent); ok && (pe.Action == platform.PointerDown || pe.Action == platform.PointerUp) {
						downs = append(downs, pointerString(pe))
					}
				}
				if w.ctl.drags != 1 {
					t.Errorf("%d window drags, want 1", w.ctl.drags)
				}
				if len(downs) != 2 || !strings.HasPrefix(downs[0], "down 40,100") {
					t.Errorf("presses delivered %v, want only the content press at 40,100 and its release", downs)
				}
				t.Logf("position before %v, after %v", before, w.State().Position)
			}},
	)
}
