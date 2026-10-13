//go:build linux

package x11

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/bigreq"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// FrameInterval is the frame pacing interval: a fixed 60 Hz. The core
// protocol has no vsync signal; the Present extension (or RandR's refresh
// rate) can replace this later.
const FrameInterval = time.Second / 60

const (
	// eventBatch bounds how many events one loop iteration translates
	// before delivering, so a flood of motion cannot starve frames.
	eventBatch = 1024
	// pumpBuffer is the capacity of the pump → UI thread channel. xgb
	// itself buffers 5000 events behind it.
	pumpBuffer = 256
	// maxPutImageBytes caps one PutImage request (and the reusable request
	// buffer) even when BIG-REQUESTS allows up to 16 MiB.
	maxPutImageBytes = 4 << 20
)

// errConnLost is returned by Run and Present when the X connection fails.
var errConnLost = errors.New("x11: the connection to the X server was lost")

// errClosed is returned by NewWindow after Run returned.
var errClosed = errors.New("x11: the backend is closed (Run has returned)")

// Backend is the X11 backend. Create it with New.
type Backend struct {
	conn   *xgb.Conn
	screen *xproto.ScreenInfo
	root   xproto.Window
	visual xproto.Visualid
	depth  byte
	atoms  atoms
	start  time.Time

	// Request limits for PutImage (see present.go).
	bigRequests bool
	maxRequest  int // bytes, capped at maxPutImageBytes
	reqBuf      []byte

	// Cross-goroutine state.
	notify     chan struct{} // buffer 1; Wake and Quit signal the loop
	lost       atomic.Bool   // set by the pump when the connection fails
	events     chan xevent   // pump → UI thread; closed when the connection ends
	done       chan struct{}
	iterations atomic.Int64 // loop iterations, for idle tests

	// loop holds the open windows (creation order), the event queue, the
	// wake/quit flags and the frame pacing state shared with the other
	// backends. Its Wake and Quit are safe from any goroutine.
	loop *loop.Loop[*Window]

	// Input extensions and the keyboard (UI thread).
	xi  xiState
	xkb xkbExt
	kbd kbd

	// UI-thread state.
	scale    float64
	windows  map[xproto.Window]*Window
	cursors  map[platform.Cursor]xproto.Cursor
	cursorFt xproto.Font // core "cursor" font; 0 if unavailable
	fontOK   bool        // cursorFt was opened successfully
	timer    *time.Timer
	ran      bool
	closed   bool
}

// xevent is an event or an X error received by the pump.
type xevent struct {
	ev  xgb.Event
	err xgb.Error
}

var _ platform.Backend = (*Backend)(nil)

var quietLogger sync.Once

// silenceXgbNoise filters the xgb log lines that are expected on Xwayland:
// Xwayland usually runs without an Xauthority file and accepts local
// connections without a cookie, but xgb logs two lines on every connect
// when it cannot read one. Every other xgb log line still reaches stderr.
// xgb.Logger is a package global; Tela is its only user in the process.
func silenceXgbNoise() {
	quietLogger.Do(func() {
		xgb.Logger = log.New(filterWriter{os.Stderr}, "XGB: ", log.Lshortfile)
	})
}

type filterWriter struct{ w io.Writer }

func (f filterWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("Could not get authority info")) ||
		bytes.Contains(p, []byte("Trying connection without authority info")) {
		return len(p), nil
	}
	return f.w.Write(p)
}

// New connects to the X server named by $DISPLAY. The error wraps
// platform.ErrUnavailable when DISPLAY is unset or the connection fails, so
// the backend selector can fall back. A server whose default screen does not
// offer the pixel layout this backend writes (24-bit TrueColor, 32 bits per
// pixel, LSBFirst) yields an error that does not wrap ErrUnavailable.
func New() (*Backend, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return nil, fmt.Errorf("x11: DISPLAY is not set: %w", platform.ErrUnavailable)
	}
	silenceXgbNoise()
	conn, err := xgb.NewConnDisplay(display)
	if err != nil {
		return nil, fmt.Errorf("x11: cannot connect to the X server at DISPLAY=%q: %w: %w", display, platform.ErrUnavailable, err)
	}
	b, err := newBackend(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return b, nil
}

func newBackend(conn *xgb.Conn) (*Backend, error) {
	setup := xproto.Setup(conn)
	if setup == nil || len(setup.Roots) == 0 {
		return nil, fmt.Errorf("x11: the X server sent no screens: %w", platform.ErrUnavailable)
	}
	screen := setup.DefaultScreen(conn)
	if err := checkPixelLayout(setup, screen); err != nil {
		return nil, err
	}
	b := &Backend{
		conn:    conn,
		screen:  screen,
		root:    screen.Root,
		visual:  screen.RootVisual,
		depth:   screen.RootDepth,
		start:   time.Now(),
		notify:  make(chan struct{}, 1),
		events:  make(chan xevent, pumpBuffer),
		done:    make(chan struct{}),
		scale:   1,
		windows: map[xproto.Window]*Window{},
		cursors: map[platform.Cursor]xproto.Cursor{},
	}
	b.initLoop()
	b.kbd.fetch = b.coreMapping
	var err error
	if b.atoms, err = internAtoms(conn); err != nil {
		return nil, fmt.Errorf("x11: interning atoms: %w", err)
	}
	b.maxRequest = maxRequestBytes(conn, setup)
	b.bigRequests = b.maxRequest > int(setup.MaximumRequestLength)*4
	b.scale = b.readScale()

	// PropertyChange on the root window reports RESOURCE_MANAGER changes
	// (Xft.dpi). Other root properties change too (_NET_ACTIVE_WINDOW on
	// every focus change) and are filtered in translate.
	xproto.ChangeWindowAttributes(conn, b.root, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange})

	b.initXKB()
	b.initXI2()
	b.kbd.km() // read the keyboard mapping now, not at the first key

	b.timer = time.NewTimer(time.Hour)
	b.timer.Stop()
	return b, nil
}

// checkPixelLayout verifies that the default screen can show the BGRX
// pixels Present writes: a depth-24 TrueColor root visual with the masks
// 0xff0000/0xff00/0xff, 32 bits per pixel for depth 24, and LSBFirst image
// byte order. Every X server on a little-endian Linux machine (Xorg,
// Xwayland, Xvfb) satisfies this.
func checkPixelLayout(setup *xproto.SetupInfo, screen *xproto.ScreenInfo) error {
	if screen.RootDepth != 24 {
		return fmt.Errorf("x11: unsupported root window depth %d; Tela needs a 24-bit TrueColor default visual", screen.RootDepth)
	}
	var vis *xproto.VisualInfo
	for _, d := range screen.AllowedDepths {
		if d.Depth != screen.RootDepth {
			continue
		}
		for i := range d.Visuals {
			if d.Visuals[i].VisualId == screen.RootVisual {
				vis = &d.Visuals[i]
			}
		}
	}
	if vis == nil {
		return fmt.Errorf("x11: the root visual 0x%x is not listed for depth 24", screen.RootVisual)
	}
	if vis.Class != xproto.VisualClassTrueColor || vis.RedMask != 0xff0000 || vis.GreenMask != 0xff00 || vis.BlueMask != 0xff {
		return fmt.Errorf("x11: unsupported root visual (class %d, masks R %#x G %#x B %#x); Tela needs TrueColor with R 0xff0000, G 0xff00, B 0xff",
			vis.Class, vis.RedMask, vis.GreenMask, vis.BlueMask)
	}
	bpp := 0
	for _, f := range setup.PixmapFormats {
		if f.Depth == 24 {
			bpp = int(f.BitsPerPixel)
			if f.ScanlinePad > 32 {
				return fmt.Errorf("x11: unsupported scanline pad %d for depth 24", f.ScanlinePad)
			}
		}
	}
	if bpp != 32 {
		return fmt.Errorf("x11: unsupported pixmap format: depth 24 uses %d bits per pixel, Tela needs 32", bpp)
	}
	if setup.ImageByteOrder != xproto.ImageOrderLSBFirst {
		return errors.New("x11: the X server uses MSBFirst image byte order; Tela writes LSBFirst (BGRX) pixels and does not support big-endian servers yet")
	}
	return nil
}

// initLoop creates the shared loop: timer pacing at FrameInterval (the core
// protocol has no frame callbacks), Wake and Quit through the notify
// channel. It also links the keyboard to the backend.
func (b *Backend) initLoop() {
	b.kbd.b = b
	b.loop = loop.New(loop.Options[*Window]{
		Now:      b.Now,
		Notify:   b.signal,
		Interval: FrameInterval,
	})
}

// maxRequestBytes enables BIG-REQUESTS when available and returns the
// largest PutImage request this backend sends, in bytes.
func maxRequestBytes(conn *xgb.Conn, setup *xproto.SetupInfo) int {
	max := int(setup.MaximumRequestLength) * 4
	if err := bigreq.Init(conn); err == nil {
		if r, err := bigreq.Enable(conn).Reply(); err == nil && r != nil && int(r.MaximumRequestLength)*4 > max {
			max = int(r.MaximumRequestLength) * 4
		}
	}
	return min(max, maxPutImageBytes)
}

// Name returns "x11".
func (b *Backend) Name() string { return "x11" }

// Now returns the time since the backend was created (monotonic). Safe from
// any goroutine.
func (b *Backend) Now() time.Duration { return time.Since(b.start) }

// Wake makes the loop deliver a WakeEvent. Safe from any goroutine; never
// blocks; coalesces.
func (b *Backend) Wake() { b.loop.Wake() }

// Quit makes Run return after the current handler call. Safe from any
// goroutine; idempotent.
func (b *Backend) Quit() { b.loop.Quit() }

func (b *Backend) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

// Settings returns zero values: X11 settings (XSETTINGS) are not read yet.
func (b *Backend) Settings() platform.SystemSettings { return platform.SystemSettings{} }

// Clipboard returns a clipboard whose methods report that X11 selections
// are not supported yet.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

// Scale returns the display scale derived from Xft.dpi (1 if unset). UI
// thread only.
func (b *Backend) Scale() float64 { return b.scale }

// Run runs the event loop until Quit (nil) or until the connection to the X
// server fails (non-nil). On return, every window is destroyed and the
// connection is closed.
func (b *Backend) Run(h platform.Handler) error {
	if h == nil {
		panic("x11: Run called with a nil Handler")
	}
	if b.ran {
		panic("x11: Run called twice; a Backend runs one event loop")
	}
	b.ran = true
	go b.pump()
	defer b.shutdown()

	for {
		b.iterations.Add(1)
		if b.loop.Quitting() {
			return nil
		}
		if !b.pollEvents() {
			return errConnLost
		}
		if !b.ready() {
			var timerC <-chan time.Time
			if deadline, ok := b.nextDeadline(); ok {
				b.timer.Reset(max(deadline-b.Now(), 0))
				timerC = b.timer.C
			}
			select {
			case e, ok := <-b.events:
				if !ok {
					return errConnLost
				}
				b.translate(e)
			case <-b.notify:
			case <-timerC:
			}
			b.timer.Stop()
			if b.loop.Quitting() {
				return nil
			}
			if !b.pollEvents() {
				return errConnLost
			}
		}
		b.dispatch(h)
	}
}

// pump forwards xgb events to the UI thread. WaitForEvent returns (nil, nil)
// once the connection is closed, either by shutdown or because the server
// went away; xgb does not report which (it only logs the read error), so
// the pump checks done.
//
// After Run returns (done closed), the pump keeps draining until the
// connection is gone and drops the events: xgb's reader goroutine blocks
// when its 5000-event buffer is full, and Close waits for a final round
// trip that the blocked reader would never read.
func (b *Backend) pump() {
	for {
		ev, err := b.conn.WaitForEvent()
		if ev == nil && err == nil {
			select {
			case <-b.done:
			default:
				b.lost.Store(true)
			}
			close(b.events)
			return
		}
		select {
		case b.events <- xevent{ev, err}:
		case <-b.done:
		}
	}
}

// pollEvents translates the events already received, without blocking. It
// returns false if the connection was lost. A key event still waiting for
// its StateNotify is queued at the end: nothing more arrived.
func (b *Backend) pollEvents() bool {
	defer b.kbd.flushAll()
	for range eventBatch {
		select {
		case e, ok := <-b.events:
			if !ok {
				return false
			}
			b.translate(e)
		default:
			return true
		}
	}
	return true
}

// shutdown releases the OS resources when Run returns.
func (b *Backend) shutdown() {
	b.timer.Stop()
	if b.closed {
		return
	}
	b.closed = true
	for _, w := range slices.Clone(b.loop.Windows()) {
		w.Close()
	}
	b.loop.Discard()
	close(b.done)
	b.conn.Close()
}

// ready reports whether dispatch has anything to deliver now (or Quit was
// called).
func (b *Backend) ready() bool { return b.loop.Ready() }

// nextDeadline returns when the earliest requested, not yet due frame
// becomes due. ok is false when no frame is waiting on pacing, which is when
// the loop sleeps without a timer.
func (b *Backend) nextDeadline() (time.Duration, bool) { return b.loop.Pace() }

// dispatch delivers a pending WakeEvent, the queued items (FIFO) and the
// due frames (see loop.Loop.Deliver). Items queued while the handler runs
// wait for the next iteration; frame requests are evaluated after the
// items, so an input event that requests a frame gets it in the same
// iteration if pacing allows.
func (b *Backend) dispatch(h platform.Handler) { b.loop.Deliver(h) }

// enqueue appends an event for w (nil for app events).
func (b *Backend) enqueue(w *Window, e platform.Event) { b.loop.Enqueue(w, e) }

// queueConfigure queues a coalesced ConfigureEvent for w with the metrics
// of the latest server size at the current scale: any number of
// ConfigureNotify events (or scale changes) before it is delivered yield
// one ConfigureEvent with the latest values, and none if nothing changed.
func (b *Backend) queueConfigure(w *Window) { b.loop.Reconfigure(w, w.osMetrics()) }

// readScale returns Xft.dpi / 96 from the root window's RESOURCE_MANAGER,
// or 1 if it is unset or unusable.
func (b *Backend) readScale() float64 {
	r, err := xproto.GetProperty(b.conn, false, b.root, xproto.AtomResourceManager, xproto.AtomString, 0, 1<<16).Reply()
	if err != nil || r == nil || r.Format != 8 {
		return 1
	}
	return scaleFromResources(string(r.Value))
}

// updateScale re-reads Xft.dpi after RESOURCE_MANAGER changed, and
// reconfigures every window if the scale changed. The physical size of the
// windows is kept; their logical size changes.
func (b *Backend) updateScale() {
	s := b.readScale()
	if s == b.scale {
		return
	}
	b.scale = s
	for _, w := range b.loop.Windows() {
		w.updateNormalHints()
		b.queueConfigure(w)
	}
}
