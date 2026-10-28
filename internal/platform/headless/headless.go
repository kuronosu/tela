// Package headless is an in-memory platform backend for tests, snapshots,
// telatest and the CLI. It implements the platform contract (see package
// platform) without any OS: windows are records, Present keeps a copy of the
// frame, the clock is controllable and input is injected.
//
// # Driving it
//
// Two ways, sharing the same semantics:
//
//   - Synchronous, for tests: [Backend.Step] runs one loop iteration (wake,
//     queued events in FIFO order, then due frames) without blocking, and
//     [Backend.Drain] repeats Step until nothing is pending at the current
//     clock time. Combine with [Backend.Advance] to move the clock.
//   - Blocking: [Backend.Run], like a real backend. It blocks (0% CPU) until
//     an event is injected, Wake or Quit is called, the clock is advanced,
//     or (in real-time mode) a paced frame becomes due.
//
// # Clock and pacing
//
// By default the clock is manual: it starts at 0 and only moves with
// Advance, so frame timestamps are deterministic. With Options.RealTime it
// follows the monotonic wall clock instead (for the CLI and interactive
// tools), and Advance panics.
//
// Frames are paced like a display with a fixed refresh interval
// (Options.FrameInterval, default 60 Hz): a requested frame is due
// immediately if the window has not had a frame in the last interval,
// otherwise at lastFrame + interval. A ConfigureEvent's frame is due
// immediately. With a manual clock, an animation that keeps requesting
// frames therefore only progresses when the test advances the clock; Drain
// never loops forever on it.
//
// # Input
//
// Window.Inject queues pointer, scroll, key, focus and close events as if
// the OS produced them. Key events must follow the platform keyboard
// contract (focus first, presses before releases, Repeat only while held),
// and Inject panics with the reason when a sequence could not come from a
// real backend; losing focus releases the held keys synthetically, as real
// backends do. SetTextInput records the app's text input state
// (Window.TextInput, TextInputChanges, TextInputResets); the IME session is
// active while the window has focus and a state, and IMEEvents injected
// into it are checked against the platform text input contract.
//
// # Window control and desktop services
//
// The headless "window manager" grants every window control request at
// once and deterministically: maximized and fullscreen windows take the
// size of a simulated screen (Options.Screen), and return to their earlier
// size; minimized and hidden windows get no frames and lose the focus;
// Activate un-minimizes and moves the focus; positions are known once the
// app sets one. SetState simulates the window manager or the user changing
// the state, and the inspection methods (OSState, Visible, SizeLimits,
// Icon, HitRegions, Activations, CaptionDrags, WindowMenus, Children,
// Blocked) report what a real backend would have sent to the OS. Pointer
// presses injected into caption hit regions are handled as the OS would
// (not delivered; a double click toggles maximize). Parents and modality
// are recorded (Parent, Children, Blocked) but input to a blocked parent
// is still delivered, so tests can check that core refuses it. Closing a
// window closes its children first.
//
// Backend.Desktop records every request (Dialogs, Opened, Notifications,
// Trays). Dialogs and message boxes are answered by a responder
// (SetDialogResponder) or one by one (AnswerDialog), through the loop like
// a real backend; notification and tray events are injected with
// InjectApp; SetUnsupported makes services return platform.ErrUnsupported
// as on a backend without them.
//
// # Concurrency
//
// All state is guarded by one mutex, and the handler is never called with it
// held. The injection API (Window.Inject, Resize, SetScale, RequestClose,
// SetFocused, SetState, Backend.SetSettings, Backend.InjectApp,
// Backend.AnswerDialog, Advance) and the inspection API (Frame, Damage,
// Presents, Title, OSState, Dialogs, ...) are safe from any goroutine, so a tool can
// drive a Run loop from another goroutine. The platform.Window and
// platform.Backend methods follow the platform contract (UI thread), but
// are also internally synchronized.
package headless

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// DefaultFrameInterval is the default refresh interval: 60 Hz.
const DefaultFrameInterval = time.Second / 60

// drainLimit bounds Drain so that a handler that generates work forever (for
// example by calling Wake on every WakeEvent) fails loudly instead of
// hanging the test.
const drainLimit = 10000

// Options configures a headless backend. The zero value is valid.
type Options struct {
	// Size is the logical size given to windows created with a zero
	// WindowOptions.Size. Zero means platform.DefaultWindowSize.
	Size platform.Size
	// Scale is the display scale of new windows. Zero means 1.
	Scale float64
	// Settings are the initial system settings.
	Settings platform.SystemSettings
	// FrameInterval is the simulated refresh interval. Zero means
	// DefaultFrameInterval.
	FrameInterval time.Duration
	// RealTime makes the clock follow the monotonic wall clock instead of
	// the manual clock.
	RealTime bool
	// Screen is the logical size of the simulated screen: the size of
	// maximized and fullscreen windows. Zero means DefaultScreen.
	Screen platform.Size
	// NoServerDecorations makes the fake window manager draw no title bars,
	// as GNOME on Wayland: every window reports ClientSideDecorations, and
	// the app draws its own bar.
	NoServerDecorations bool
}

// Backend is the headless backend. Create it with New.
type Backend struct {
	opts  Options
	start time.Time // RealTime origin

	// notify wakes a blocked Run. Buffer 1, non-blocking sends: signals
	// coalesce and are never lost (see Run).
	notify chan struct{}

	iterations atomic.Int64 // loop iterations, for idle tests

	mu sync.Mutex
	// loop is the shared queue, wake/quit flags and frame pacing, guarded
	// by mu (Options.Lock): the injection API runs on any goroutine.
	loop      *loop.Loop[*Window]
	clock     time.Duration // manual clock
	settings  platform.SystemSettings
	clipboard string
	ran       bool // Run was called
	active    bool // a Run, Step or Drain is executing
	created   int  // windows created, for restore tokens
	desktop   desktopState
}

var _ platform.Backend = (*Backend)(nil)

// New returns a headless backend.
func New(opts Options) *Backend {
	if opts.Scale == 0 {
		opts.Scale = 1
	}
	platform.PhysicalSize(platform.Size{}, opts.Scale) // validates the scale
	platform.CheckSize("headless.Options.Size", opts.Size)
	if opts.Size == (platform.Size{}) {
		opts.Size = platform.DefaultWindowSize
	}
	platform.CheckSize("headless.Options.Screen", opts.Screen)
	if opts.Screen.Empty() {
		opts.Screen = DefaultScreen
	}
	if opts.FrameInterval < 0 {
		panic(fmt.Sprintf("headless: negative Options.FrameInterval %v", opts.FrameInterval))
	}
	if opts.FrameInterval == 0 {
		opts.FrameInterval = DefaultFrameInterval
	}
	b := &Backend{
		opts:     opts,
		start:    time.Now(),
		notify:   make(chan struct{}, 1),
		settings: opts.Settings,
	}
	b.loop = loop.New(loop.Options[*Window]{
		Now:      b.now,
		Notify:   b.signal,
		Interval: opts.FrameInterval,
		Lock:     &b.mu,
	})
	return b
}

// Name returns "headless".
func (b *Backend) Name() string { return "headless" }

// Now returns the backend clock.
func (b *Backend) Now() time.Duration {
	if b.opts.RealTime {
		return time.Since(b.start)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.clock
}

// now returns the clock; b.mu must be held.
func (b *Backend) now() time.Duration {
	if b.opts.RealTime {
		return time.Since(b.start)
	}
	return b.clock
}

// Advance moves the manual clock forward by d. Frames that become due are
// delivered by the next Step (or by Run, which Advance wakes). It panics in
// real-time mode or for a negative d.
func (b *Backend) Advance(d time.Duration) {
	if b.opts.RealTime {
		panic("headless: Advance called on a real-time backend; create it with Options{RealTime: false} to control the clock")
	}
	if d < 0 {
		panic(fmt.Sprintf("headless: Advance(%v): the clock cannot go backwards", d))
	}
	b.mu.Lock()
	b.clock += d
	b.mu.Unlock()
	b.signal()
}

// FrameInterval returns the simulated refresh interval.
func (b *Backend) FrameInterval() time.Duration { return b.opts.FrameInterval }

// NewWindow creates a window. Its ConfigureEvent is queued immediately,
// followed by a WindowStateEvent when its initial state is not the zero
// one (maximized, fullscreen, frameless or placed). A Parent must be an
// open window of this backend.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	if opts.Size == (platform.Size{}) {
		opts.Size = b.opts.Size
	}
	opts = opts.Resolve()
	w := &Window{b: b, opts: opts, title: opts.Title}
	b.mu.Lock()
	b.created++
	n := b.created
	b.mu.Unlock()
	size := w.initCtl(opts, b.opts.Screen, n)
	if b.opts.NoServerDecorations {
		w.ctl.osState.ClientSideDecorations = true
	}
	w.os = platform.Metrics{Size: size, Scale: b.opts.Scale}
	b.mu.Lock()
	b.loop.Add(w)
	b.loop.Enqueue(w, platform.ConfigureEvent{Metrics: w.os})
	b.loop.UpdateState(w, w.ctl.osState)
	b.mu.Unlock()
	b.signal()
	return w, nil
}

// Windows returns the open windows in creation order.
func (b *Backend) Windows() []*Window {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.loop.Windows())
}

// Wake queues a WakeEvent. Safe from any goroutine; coalesces.
func (b *Backend) Wake() { b.loop.Wake() }

// Quit makes Run return, and makes Step and Drain deliver nothing more.
// Safe from any goroutine.
func (b *Backend) Quit() { b.loop.Quit() }

// Quitting reports whether Quit was called.
func (b *Backend) Quitting() bool { return b.loop.Quitting() }

// Settings returns the current system settings.
func (b *Backend) Settings() platform.SystemSettings {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.settings
}

// SetSettings changes the system settings and queues a SettingsEvent, as if
// the user changed them. Safe from any goroutine.
func (b *Backend) SetSettings(s platform.SystemSettings) {
	b.mu.Lock()
	b.settings = s
	b.loop.Enqueue(nil, platform.SettingsEvent{Settings: s})
	b.mu.Unlock()
	b.signal()
}

// Clipboard returns the in-memory clipboard.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

// signal wakes a blocked Run without blocking the caller.
func (b *Backend) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

// Run delivers events to h until Quit. It blocks without spinning while
// there is no work. Run may be called once.
func (b *Backend) Run(h platform.Handler) error {
	if h == nil {
		panic("headless: Run called with a nil Handler")
	}
	b.mu.Lock()
	if b.ran {
		b.mu.Unlock()
		panic("headless: Run called twice; a Backend runs one event loop")
	}
	if b.active {
		b.mu.Unlock()
		panic("headless: Run called while Step or Drain is executing")
	}
	b.ran, b.active = true, true
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.active = false
		b.desktop.trays = nil // trays are removed when Run returns
		b.mu.Unlock()
	}()

	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for !b.loop.Quitting() {
		b.iterations.Add(1)
		if b.step(h) > 0 {
			continue
		}
		// Nothing was ready. Block until a signal or, in real-time mode,
		// until the next paced frame is due. A signal sent after the
		// readiness check above sits in the buffered channel, so it is
		// never lost.
		b.mu.Lock()
		ready := b.loop.Ready()
		deadline, hasDeadline := b.loop.Pace()
		now := b.now()
		b.mu.Unlock()
		if ready {
			continue
		}
		var timeout <-chan time.Time
		if hasDeadline && b.opts.RealTime {
			if timer == nil {
				timer = time.NewTimer(deadline - now)
			} else {
				timer.Reset(deadline - now)
			}
			timeout = timer.C
		}
		select {
		case <-b.notify:
		case <-timeout:
		}
		if timer != nil {
			timer.Stop()
		}
	}
	return nil
}

// Step runs one loop iteration without blocking: a WakeEvent if Wake was
// called, then the events queued before the iteration started (FIFO), then
// one FrameEvent per window whose frame is due, in window creation order.
// Work produced by the handler during the iteration is left for the next
// Step, except frame requests, which are evaluated after the events (so an
// input event that requests a frame gets it in the same iteration, as on a
// real backend). It returns the number of events and callbacks delivered.
//
// Step must be called from the goroutine acting as UI thread, never from
// inside the handler and never while Run executes.
func (b *Backend) Step(h platform.Handler) int {
	b.enter("Step")
	defer b.leave()
	return b.step(h)
}

// Drain calls Step until no work is ready at the current clock time and
// returns the total delivered. Frames that pacing schedules later are not
// waited for: advance the clock and Drain again. It panics if the handler
// keeps generating work for 10000 iterations.
func (b *Backend) Drain(h platform.Handler) int {
	b.enter("Drain")
	defer b.leave()
	total := 0
	for i := 0; ; i++ {
		if i == drainLimit {
			panic("headless: Drain did not settle after 10000 iterations; the handler keeps generating work (for example by calling Wake on every WakeEvent or SetSize on every ConfigureEvent)")
		}
		if b.loop.Quitting() {
			return total
		}
		b.mu.Lock()
		ready := b.loop.Ready()
		b.mu.Unlock()
		if !ready {
			return total
		}
		total += b.step(h)
	}
}

// Pending reports whether a Step would deliver anything now.
func (b *Backend) Pending() bool {
	if b.loop.Quitting() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loop.Ready()
}

// NextFrameTime reports when the earliest pending frame request that is
// not yet due becomes due. ok is false if no request is waiting on pacing.
// telatest uses it to advance the clock exactly to the next frame.
func (b *Backend) NextFrameTime() (t time.Duration, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loop.Pace()
}

func (b *Backend) enter(what string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active {
		panic("headless: " + what + " called from inside a handler, or while Run, Step or Drain is executing; drive the backend from one goroutine")
	}
	b.active = true
}

func (b *Backend) leave() {
	b.mu.Lock()
	b.active = false
	b.mu.Unlock()
}

// step is one loop iteration; see Step.
func (b *Backend) step(h platform.Handler) int { return b.loop.Deliver(h) }

// clipboard is the in-memory clipboard.
type clipboard struct{ b *Backend }

func (c clipboard) ReadText(done func(string, error)) {
	if done == nil {
		panic("headless: Clipboard.ReadText called with a nil callback")
	}
	c.b.mu.Lock()
	c.b.loop.Post(func() {
		c.b.mu.Lock()
		s := c.b.clipboard
		c.b.mu.Unlock()
		done(s, nil)
	})
	c.b.mu.Unlock()
	c.b.signal()
}

func (c clipboard) WriteText(s string) error {
	c.b.mu.Lock()
	c.b.clipboard = s
	c.b.mu.Unlock()
	return nil
}
