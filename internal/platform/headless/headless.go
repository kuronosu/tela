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
// # Concurrency
//
// All state is guarded by one mutex, and the handler is never called with it
// held. The injection API (Window.Inject, Resize, SetScale, RequestClose,
// SetFocused, Backend.SetSettings, Advance) and the inspection API (Frame,
// Damage, Presents, Title, ...) are safe from any goroutine, so a tool can
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
}

// Backend is the headless backend. Create it with New.
type Backend struct {
	opts  Options
	start time.Time // RealTime origin

	// notify wakes a blocked Run. Buffer 1, non-blocking sends: signals
	// coalesce and are never lost (see Run).
	notify chan struct{}

	wakePending atomic.Bool
	quitting    atomic.Bool
	iterations  atomic.Int64 // loop iterations, for idle tests

	mu        sync.Mutex
	clock     time.Duration // manual clock
	queue     []item
	windows   []*Window // open windows, creation order
	settings  platform.SystemSettings
	clipboard string
	ran       bool // Run was called
	active    bool // a Run, Step or Drain is executing
}

// item is a queued unit of work: an event for a window (w nil for app
// events) or a function to run on the loop (fn non-nil).
type item struct {
	w  *Window
	ev platform.Event
	fn func()
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
	if opts.FrameInterval < 0 {
		panic(fmt.Sprintf("headless: negative Options.FrameInterval %v", opts.FrameInterval))
	}
	if opts.FrameInterval == 0 {
		opts.FrameInterval = DefaultFrameInterval
	}
	return &Backend{
		opts:     opts,
		start:    time.Now(),
		notify:   make(chan struct{}, 1),
		settings: opts.Settings,
	}
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

// NewWindow creates a window. Its ConfigureEvent is queued immediately.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	if opts.Size == (platform.Size{}) {
		opts.Size = b.opts.Size
	}
	opts = opts.Resolve()
	w := &Window{
		b:     b,
		opts:  opts,
		title: opts.Title,
		os:    platform.Metrics{Size: opts.Size, Scale: b.opts.Scale},
	}
	b.mu.Lock()
	b.windows = append(b.windows, w)
	b.queue = append(b.queue, item{w: w, ev: platform.ConfigureEvent{Metrics: w.os}})
	b.mu.Unlock()
	b.signal()
	return w, nil
}

// Windows returns the open windows in creation order.
func (b *Backend) Windows() []*Window {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.windows)
}

// Wake queues a WakeEvent. Safe from any goroutine; coalesces.
func (b *Backend) Wake() {
	b.wakePending.Store(true)
	b.signal()
}

// Quit makes Run return, and makes Step and Drain deliver nothing more.
// Safe from any goroutine.
func (b *Backend) Quit() {
	b.quitting.Store(true)
	b.signal()
}

// Quitting reports whether Quit was called.
func (b *Backend) Quitting() bool { return b.quitting.Load() }

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
	b.queue = append(b.queue, item{ev: platform.SettingsEvent{Settings: s}})
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

// enqueue appends an item and wakes Run.
func (b *Backend) enqueue(it item) {
	b.mu.Lock()
	b.queue = append(b.queue, it)
	b.mu.Unlock()
	b.signal()
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
		b.mu.Unlock()
	}()

	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for !b.quitting.Load() {
		b.iterations.Add(1)
		if b.step(h) > 0 {
			continue
		}
		// Nothing was ready. Block until a signal or, in real-time mode,
		// until the next paced frame is due. A signal sent after the
		// readiness check above sits in the buffered channel, so it is
		// never lost.
		b.mu.Lock()
		ready := b.readyLocked()
		deadline, hasDeadline := b.nextDeadlineLocked()
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
		if b.quitting.Load() {
			return total
		}
		b.mu.Lock()
		ready := b.readyLocked()
		b.mu.Unlock()
		if !ready {
			return total
		}
		total += b.step(h)
	}
}

// Pending reports whether a Step would deliver anything now.
func (b *Backend) Pending() bool {
	if b.quitting.Load() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.readyLocked()
}

// NextFrameTime reports when the earliest pending frame request that is
// not yet due becomes due. ok is false if no request is waiting on pacing.
// telatest uses it to advance the clock exactly to the next frame.
func (b *Backend) NextFrameTime() (t time.Duration, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nextDeadlineLocked()
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

// readyLocked reports whether there is work to deliver now; b.mu held.
func (b *Backend) readyLocked() bool {
	if b.wakePending.Load() || len(b.queue) > 0 {
		return true
	}
	now := b.now()
	for _, w := range b.windows {
		if w.frameDueLocked(now) {
			return true
		}
	}
	return false
}

// nextDeadlineLocked returns the earliest time a paced, not yet due frame
// request becomes due; b.mu held.
func (b *Backend) nextDeadlineLocked() (time.Duration, bool) {
	now := b.now()
	var best time.Duration
	found := false
	for _, w := range b.windows {
		if !w.frameRequested || !w.visibleLocked() || w.frameDueLocked(now) {
			continue
		}
		t := w.lastFrame + b.opts.FrameInterval
		if !found || t < best {
			best, found = t, true
		}
	}
	return best, found
}

// step is one loop iteration; see Step.
func (b *Backend) step(h platform.Handler) int {
	if b.quitting.Load() {
		return 0
	}
	n := 0
	if b.wakePending.Swap(false) {
		h.HandleEvent(nil, platform.WakeEvent{})
		n++
	}

	b.mu.Lock()
	items := b.queue
	b.queue = nil
	b.mu.Unlock()
	for _, it := range items {
		if b.quitting.Load() {
			return n
		}
		if it.fn != nil {
			it.fn()
			n++
			continue
		}
		if it.w == nil {
			h.HandleEvent(nil, it.ev)
			n++
			continue
		}
		b.mu.Lock()
		if it.w.closed {
			b.mu.Unlock()
			continue
		}
		if ce, ok := it.ev.(platform.ConfigureEvent); ok {
			it.w.metrics = ce.Metrics
			it.w.needsFull = true
			it.w.forcedFrame = true
		}
		b.mu.Unlock()
		h.HandleEvent(it.w, it.ev)
		n++
	}

	// Frames: decide which are due once, then deliver.
	b.mu.Lock()
	now := b.now()
	var due []*Window
	for _, w := range b.windows {
		if w.frameDueLocked(now) {
			w.frameRequested, w.forcedFrame = false, false
			w.lastFrame, w.hasFramed = now, true
			due = append(due, w)
		}
	}
	b.mu.Unlock()
	for _, w := range due {
		if b.quitting.Load() {
			return n
		}
		b.mu.Lock()
		closed := w.closed
		b.mu.Unlock()
		if closed {
			continue
		}
		h.HandleEvent(w, platform.FrameEvent{Time: now})
		n++
	}
	return n
}

// clipboard is the in-memory clipboard.
type clipboard struct{ b *Backend }

func (c clipboard) ReadText(done func(string, error)) {
	if done == nil {
		panic("headless: Clipboard.ReadText called with a nil callback")
	}
	c.b.enqueue(item{fn: func() {
		c.b.mu.Lock()
		s := c.b.clipboard
		c.b.mu.Unlock()
		done(s, nil)
	}})
}

func (c clipboard) WriteText(s string) error {
	c.b.mu.Lock()
	c.b.clipboard = s
	c.b.mu.Unlock()
	return nil
}
