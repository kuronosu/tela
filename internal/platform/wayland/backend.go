//go:build linux

package wayland

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// Highest protocol versions this backend has been written against. Globals
// are bound at min(advertised, these): binding a newer version could bring
// events or requirements this code does not know about.
const (
	compositorVersion  = 6 // 4: damage_buffer; 5: attach offset must be 0; 6: preferred_buffer_scale
	shmVersion         = 1
	wmBaseVersion      = 6 // 6: suspended state; 7 (constrained_*) not needed
	seatVersion        = 8 // 5: wl_pointer.frame; 8: axis_value120
	outputVersion      = 4 // 2: scale + done; 3: release
	cursorShapeVersion = 1
	decorationVersion  = 1
)

// startupTimeout bounds the roundtrips done by New.
const startupTimeout = 5 * time.Second

// Backend is the Wayland backend. Create it with New. Except for Wake, Quit
// and Now, its methods must be called on the UI thread (see package
// platform).
type Backend struct {
	conn     *wire.Conn
	display  *protocols.WlDisplay
	registry *protocols.WlRegistry

	compositor  *protocols.WlCompositor
	shm         *protocols.WlShm
	wm          *protocols.XdgWmBase
	cursorShape *protocols.WpCursorShapeManagerV1
	decoration  *protocols.ZxdgDecorationManagerV1

	shmFormats map[protocols.WlShmFormat]bool
	format     protocols.WlShmFormat // pixel format of every buffer
	appID      string                // default xdg_toplevel.app_id

	seats   []*seat
	outputs []*output

	windows  []*Window // open windows, creation order
	surfaces map[*protocols.WlSurface]*Window

	queue []item // events waiting to be delivered, FIFO

	start time.Time // origin of Now

	// eventfd wakes poll. efdMu guards it against a Wake or Quit from
	// another goroutine racing with release: a write after close could hit
	// a reused descriptor.
	efdMu        sync.Mutex
	eventfd      int
	wakePending  atomic.Bool // a WakeEvent is owed
	wakeSignaled atomic.Bool // the eventfd was written and not yet drained
	quitting     atomic.Bool

	ran      bool
	released bool
	// lastSerial is the serial of the latest input event of any seat, for
	// requests that need one (clipboard, move, popups).
	lastSerial uint32
}

// item is a queued unit of work: an event for a window (w nil for app
// events) or a function to run on the loop (fn non-nil).
type item struct {
	w  *Window
	ev platform.Event
	fn func()
}

var _ platform.Backend = (*Backend)(nil)

// New connects to the compositor named by WAYLAND_DISPLAY (or the socket
// inherited through WAYLAND_SOCKET) and binds the globals the backend
// needs. Errors that mean "Wayland cannot be used here" (no display, the
// connection fails, a required global is missing) wrap
// platform.ErrUnavailable, so a backend selector can fall back to X11.
func New() (*Backend, error) {
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("WAYLAND_SOCKET") == "" {
		return nil, fmt.Errorf("wayland: WAYLAND_DISPLAY is not set: %w", platform.ErrUnavailable)
	}
	conn, err := wire.Dial()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", platform.ErrUnavailable, err)
	}
	b, err := newBackend(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return b, nil
}

// newBackend sets up a backend on an established connection. Tests call it
// with a connection to a fake compositor.
func newBackend(conn *wire.Conn) (*Backend, error) {
	efd, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("wayland: cannot create eventfd: %w", err)
	}
	b := &Backend{
		conn:       conn,
		eventfd:    efd,
		start:      time.Now(),
		shmFormats: map[protocols.WlShmFormat]bool{},
		surfaces:   map[*protocols.WlSurface]*Window{},
		appID:      defaultAppID(),
	}
	b.display = protocols.NewWlDisplay(conn)
	b.registry = b.display.GetRegistry()
	b.registry.SetGlobalHandler(b.onGlobal)
	b.registry.SetGlobalRemoveHandler(b.onGlobalRemove)

	// First roundtrip: the globals (bound from onGlobal). Second: the
	// events of the bound globals (shm formats, output scales, seat
	// capabilities).
	for range 2 {
		if err := conn.RoundtripTimeout(startupTimeout); err != nil {
			unix.Close(efd)
			return nil, fmt.Errorf("%w: wayland: initial roundtrip failed: %w", platform.ErrUnavailable, err)
		}
	}
	var missing []string
	if b.compositor == nil {
		missing = append(missing, protocols.WlCompositorInterfaceName)
	}
	if b.shm == nil {
		missing = append(missing, protocols.WlShmInterfaceName)
	}
	if b.wm == nil {
		missing = append(missing, protocols.XdgWmBaseInterfaceName)
	}
	if len(missing) > 0 {
		unix.Close(efd)
		return nil, fmt.Errorf("%w: wayland: the compositor does not provide %v", platform.ErrUnavailable, missing)
	}
	b.format = chooseFormat(b.shmFormats)
	return b, nil
}

// chooseFormat picks the buffer format. XBGR8888 has the byte order of Go's
// image.RGBA (R, G, B, X in memory), so frames are copied without a
// swizzle; it is optional in wl_shm, so XRGB8888 (always supported) is the
// fallback. Windows are opaque in phase 1, so the X variants are used: the
// compositor can skip blending. TELA_WAYLAND_SHM_FORMAT=xrgb8888 forces the
// fallback (the live tests check both paths on screen).
func chooseFormat(formats map[protocols.WlShmFormat]bool) protocols.WlShmFormat {
	if formats[protocols.WlShmFormatXbgr8888] && os.Getenv("TELA_WAYLAND_SHM_FORMAT") != "xrgb8888" {
		return protocols.WlShmFormatXbgr8888
	}
	return protocols.WlShmFormatXrgb8888
}

// defaultAppID is the executable name, the documented default of
// WindowOptions.AppID.
func defaultAppID() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "tela"
	}
	return filepath.Base(exe)
}

func (b *Backend) onGlobal(e protocols.WlRegistryGlobalEvent) {
	switch e.Interface {
	case protocols.WlCompositorInterfaceName:
		if b.compositor == nil {
			b.compositor = protocols.WlRegistryBind[protocols.WlCompositor](b.registry, e.Name, min(e.Version, compositorVersion))
		}
	case protocols.WlShmInterfaceName:
		if b.shm == nil {
			b.shm = protocols.WlRegistryBind[protocols.WlShm](b.registry, e.Name, min(e.Version, shmVersion))
			b.shm.SetFormatHandler(func(f protocols.WlShmFormatEvent) { b.shmFormats[f.Format] = true })
		}
	case protocols.XdgWmBaseInterfaceName:
		if b.wm == nil {
			b.wm = protocols.WlRegistryBind[protocols.XdgWmBase](b.registry, e.Name, min(e.Version, wmBaseVersion))
			// The compositor pings to detect unresponsive clients; answer
			// at once (the loop flushes right after dispatching).
			b.wm.SetPingHandler(func(p protocols.XdgWmBasePingEvent) { b.wm.Pong(p.Serial) })
		}
	case protocols.WpCursorShapeManagerV1InterfaceName:
		if b.cursorShape == nil {
			b.cursorShape = protocols.WlRegistryBind[protocols.WpCursorShapeManagerV1](b.registry, e.Name, min(e.Version, cursorShapeVersion))
			for _, s := range b.seats {
				s.attachCursorShape()
			}
		}
	case protocols.ZxdgDecorationManagerV1InterfaceName:
		if b.decoration == nil {
			b.decoration = protocols.WlRegistryBind[protocols.ZxdgDecorationManagerV1](b.registry, e.Name, min(e.Version, decorationVersion))
		}
	case protocols.WlSeatInterfaceName:
		b.addSeat(e.Name, e.Version)
	case protocols.WlOutputInterfaceName:
		b.addOutput(e.Name, e.Version)
	}
}

func (b *Backend) onGlobalRemove(e protocols.WlRegistryGlobalRemoveEvent) {
	for i, s := range b.seats {
		if s.name == e.Name {
			s.destroy()
			b.seats = append(b.seats[:i], b.seats[i+1:]...)
			return
		}
	}
	for i, o := range b.outputs {
		if o.name == e.Name {
			b.outputs = append(b.outputs[:i], b.outputs[i+1:]...)
			b.removeOutput(o)
			return
		}
	}
	// Removing a singleton (compositor, shm, xdg_wm_base) only happens
	// when the compositor shuts down; the connection error follows.
}

// Name returns "wayland".
func (b *Backend) Name() string { return "wayland" }

// Now returns the monotonic time since the backend was created. Safe from
// any goroutine.
func (b *Backend) Now() time.Duration { return time.Since(b.start) }

// eventTime converts a wl_pointer/wl_keyboard timestamp (milliseconds,
// 32 bits, undefined base) to the time base of Now. Compositors in practice
// use CLOCK_MONOTONIC, which is also Go's monotonic clock on Linux; the
// timestamp is trusted when it lies within the last second (computed modulo
// 2^32, so wrap-around is harmless), otherwise the arrival time is used.
func (b *Backend) eventTime(ms uint32) time.Duration {
	now := b.Now()
	var ts unix.Timespec
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts) != nil {
		return now
	}
	monoMs := uint32(ts.Nano() / int64(time.Millisecond))
	age := time.Duration(int32(monoMs-ms)) * time.Millisecond
	if age < 0 || age > time.Second || age > now {
		return now
	}
	return now - age
}

// Wake makes the loop deliver a WakeEvent. Safe from any goroutine; never
// blocks; coalesces. The flag avoids an eventfd write per call when a
// signal is already pending.
func (b *Backend) Wake() {
	b.wakePending.Store(true)
	b.signal()
}

// Quit makes Run return after the current handler call. Safe from any
// goroutine; idempotent.
func (b *Backend) Quit() {
	b.quitting.Store(true)
	b.signal()
}

func (b *Backend) signal() {
	if b.wakeSignaled.Swap(true) {
		return
	}
	var one [8]byte
	one[0] = 1 // eventfd counters are host-endian; Linux targets are little-endian
	b.efdMu.Lock()
	defer b.efdMu.Unlock()
	if b.eventfd < 0 {
		return // released: nobody to wake
	}
	for {
		// EAGAIN means the counter is saturated: still readable.
		if _, err := unix.Write(b.eventfd, one[:]); err != unix.EINTR {
			return
		}
	}
}

// Settings returns zero values (unknown): reading the desktop settings
// portal over D-Bus is a later task.
func (b *Backend) Settings() platform.SystemSettings { return platform.SystemSettings{} }

// Clipboard returns the clipboard. wl_data_device is not implemented yet.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

// errClipboard is reported by every clipboard operation in phase 1.
var errClipboard = errors.New("wayland: the clipboard is not supported yet (wl_data_device arrives with text editing in phase 5)")

type clipboard struct{ b *Backend }

func (c clipboard) ReadText(done func(string, error)) {
	if done == nil {
		panic("wayland: Clipboard.ReadText called with a nil callback")
	}
	// The contract requires an asynchronous callback, even for an error.
	c.b.queue = append(c.b.queue, item{fn: func() { done("", errClipboard) }})
}

func (c clipboard) WriteText(string) error { return errClipboard }

// enqueue queues an event for delivery by the loop.
func (b *Backend) enqueue(w *Window, ev platform.Event) {
	b.queue = append(b.queue, item{w: w, ev: ev})
}

// Run runs the event loop on the calling goroutine until Quit (nil) or a
// connection or protocol error (non-nil). When it returns, every window and
// the connection are closed.
func (b *Backend) Run(h platform.Handler) error {
	if h == nil {
		panic("wayland: Run called with a nil Handler")
	}
	if b.ran {
		panic("wayland: Run called twice; a Backend runs one event loop and releases its connection when Run returns")
	}
	b.ran = true
	defer b.release()

	for {
		if b.quitting.Load() {
			return nil
		}
		b.deliver(h)
		if b.quitting.Load() {
			return nil
		}
		if err := b.flush(); err != nil {
			return err
		}
		timeout := -1
		if b.ready() {
			timeout = 0 // still read what arrived, but do not block
		}
		if err := b.poll(timeout); err != nil {
			return err
		}
	}
}

// deliver runs one delivery pass: a WakeEvent if Wake was called, the
// events queued before the pass (FIFO), then one FrameEvent per window whose
// frame is due. Work queued by the handler during the pass waits for the
// next pass, except frame requests, which are evaluated after the events
// (an input event that requests a frame gets it in the same pass).
func (b *Backend) deliver(h platform.Handler) {
	if b.wakePending.Swap(false) {
		h.HandleEvent(nil, platform.WakeEvent{})
	}

	items := b.queue
	b.queue = nil
	for _, it := range items {
		if b.quitting.Load() {
			return
		}
		switch {
		case it.fn != nil:
			it.fn()
		case it.w == nil:
			h.HandleEvent(nil, it.ev)
		default:
			w := it.w
			if w.closed {
				continue
			}
			if ce, ok := it.ev.(platform.ConfigureEvent); ok {
				w.metrics = ce.Metrics
				w.needsFull = true
				w.forcedFrame = true
			}
			h.HandleEvent(w, it.ev)
		}
	}

	var due []*Window
	for _, w := range b.windows {
		if w.frameDue() {
			w.frameRequested, w.forcedFrame = false, false
			due = append(due, w)
		}
	}
	now := b.Now()
	for _, w := range due {
		if b.quitting.Load() {
			return
		}
		if w.closed {
			continue
		}
		h.HandleEvent(w, platform.FrameEvent{Time: now})
	}
}

// ready reports whether a delivery pass would deliver something now.
func (b *Backend) ready() bool {
	if b.wakePending.Load() || len(b.queue) > 0 || b.quitting.Load() {
		return true
	}
	for _, w := range b.windows {
		if w.frameDue() {
			return true
		}
	}
	return false
}

// flush sends buffered requests. A full socket is not an error: poll waits
// for POLLOUT. A hang-up usually follows a protocol error that is still
// waiting in the socket, so it is read and returned instead.
func (b *Backend) flush() error {
	err := b.conn.Flush()
	if err == nil || err == wire.ErrWouldBlock {
		return nil
	}
	b.conn.Read()
	if _, derr := b.conn.Dispatch(); derr != nil {
		return derr
	}
	return err
}

// poll blocks until the display socket or the eventfd is ready (or timeout
// ms pass; -1 blocks indefinitely), then reads and dispatches.
func (b *Backend) poll(timeout int) error {
	fds := [2]unix.PollFd{
		{Fd: int32(b.conn.Fd()), Events: unix.POLLIN},
		{Fd: int32(b.eventfd), Events: unix.POLLIN},
	}
	if b.conn.HasPendingWrites() {
		fds[0].Events |= unix.POLLOUT
	}
	_, err := unix.Poll(fds[:], timeout)
	if err == unix.EINTR {
		return nil
	}
	if err != nil {
		return fmt.Errorf("wayland: poll failed: %w", err)
	}
	if fds[1].Revents != 0 {
		// Drain first, then clear the flag. The other order loses
		// wakeups: a Wake between the clear and the drain writes, the
		// drain consumes that write too (an eventfd read resets the
		// counter), and the flag stays set with nothing left to read, so
		// every later Wake and Quit skips its write and poll never
		// returns. In this order a Wake that sees the flag still set has
		// already stored wakePending (or quitting), which the delivery
		// pass after this poll reads.
		var buf [8]byte
		unix.Read(b.eventfd, buf[:])
		b.wakeSignaled.Store(false)
	}
	if fds[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) != 0 {
		if _, err := b.conn.Read(); err != nil {
			// On EOF, messages already buffered may include the
			// compositor's wl_display.error: dispatch them first.
			if _, derr := b.conn.Dispatch(); derr != nil {
				return derr
			}
			return err
		}
	}
	_, err = b.conn.Dispatch()
	return err
}

// release destroys every window and closes the connection.
func (b *Backend) release() {
	if b.released {
		return
	}
	b.released = true
	for len(b.windows) > 0 {
		b.windows[len(b.windows)-1].Close()
	}
	if b.conn.Err() == nil {
		for _, s := range b.seats {
			s.destroy()
		}
		for _, o := range b.outputs {
			o.release()
		}
		if b.cursorShape != nil {
			b.cursorShape.Destroy()
		}
		if b.decoration != nil {
			b.decoration.Destroy()
		}
		b.wm.Destroy()
		// Best effort: the compositor frees everything when the socket
		// closes anyway.
		b.conn.Flush()
	}
	b.queue = nil
	b.conn.Close()
	b.efdMu.Lock()
	unix.Close(b.eventfd)
	b.eventfd = -1
	b.efdMu.Unlock()
}
