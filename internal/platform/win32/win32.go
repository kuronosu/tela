//go:build windows

package win32

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// errClosed is returned by NewWindow after Run returned.
var errClosed = errors.New("win32: the backend is closed (Run has returned)")

// Backend is the Win32 backend. Create it with New on the goroutine that
// will call Run.
type Backend struct {
	start    time.Time
	hinst    uintptr
	msgHwnd  uintptr // message-only window: target of Wake, Quit and vsync posts
	interval time.Duration

	// loop holds the open windows, the event queue, the wake/quit flags and
	// the frame pacing state shared with the other backends.
	loop *loop.Loop[*Window]

	// Cross-goroutine state.
	notifyPosted atomic.Bool // a wmTelaNotify is in the queue
	vsyncBusy    atomic.Bool // the vsync goroutine is running
	stopped      atomic.Bool // the message-only window is gone

	// UI-thread state.
	handler platform.Handler
	pumping bool // a delivery pass is running
	dirty   bool // events were queued since the last pass
	ran     bool
	closed  bool
	failed  bool // the handler panicked inside WndProc
	failure any  // the panic value, re-raised by Run
	cursors map[platform.Cursor]uintptr
	msg     msg
	// settings are the system settings as last read (see settings.go).
	settings platform.SystemSettings

	iterations atomic.Int64 // GetMessageW returns, for idle tests
}

var _ platform.Backend = (*Backend)(nil)

// New creates the backend: it opts the process in to per-monitor DPI
// awareness, registers the window class and creates the message-only
// window. It also locks the calling goroutine to its OS thread until Run
// returns, because Win32 windows belong to the thread that creates them: Run
// and every Backend and Window method must be called from this goroutine.
func New() (*Backend, error) {
	runtime.LockOSThread()
	b, err := newBackend()
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}
	return b, nil
}

func newBackend() (*Backend, error) {
	initDPIAwareness()
	b := &Backend{
		start:    time.Now(),
		hinst:    getModuleHandle(),
		interval: displayInterval(),
		cursors:  map[platform.Cursor]uintptr{},
		settings: readSettings(),
	}
	b.loop = loop.New(loop.Options[*Window]{
		Now:      b.Now,
		Notify:   b.notify,
		Interval: b.interval,
		Pace:     b.paceHook,
	})
	if err := registerClass(b.hinst); err != nil {
		return nil, err
	}
	hwnd, _, e := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(classNamePtr)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, b.hinst, 0)
	if hwnd == 0 {
		return nil, winError("creating the message-only window", e)
	}
	b.msgHwnd = hwnd
	registry.mu.Lock()
	registry.backends[hwnd] = b
	registry.mu.Unlock()
	return b, nil
}

// Name returns "win32".
func (b *Backend) Name() string { return "win32" }

// Now returns the time since the backend was created (monotonic). Safe from
// any goroutine.
func (b *Backend) Now() time.Duration { return time.Since(b.start) }

// Wake makes the loop deliver a WakeEvent. Safe from any goroutine; never
// blocks; coalesces.
func (b *Backend) Wake() { b.loop.Wake() }

// Quit makes Run return after the current handler call. Safe from any
// goroutine; idempotent.
func (b *Backend) Quit() { b.loop.Quit() }

// notify unblocks GetMessageW: it posts the private message that makes
// WndProc run a delivery pass. At most one is in the queue, so any number of
// Wake calls costs one message. It posts to a window, not to the thread:
// thread messages are dropped while a modal loop runs.
func (b *Backend) notify() {
	if b.stopped.Load() {
		return
	}
	if b.notifyPosted.CompareAndSwap(false, true) && !postMessage(b.msgHwnd, wmTelaNotify, 0, 0) {
		b.notifyPosted.Store(false)
	}
}

// Run runs the message loop until Quit (nil) or until GetMessageW fails
// (non-nil). On return, every window is destroyed. If the handler panicked,
// Run panics with the same value.
func (b *Backend) Run(h platform.Handler) error {
	if h == nil {
		panic("win32: Run called with a nil Handler")
	}
	if b.ran {
		panic("win32: Run called twice; a Backend runs one event loop")
	}
	b.ran = true
	b.handler = h
	defer b.shutdown()

	b.pump() // the ConfigureEvents queued by NewWindow
	for !b.loop.Quitting() && !b.failed {
		b.iterations.Add(1)
		r, _, e := procGetMessageW.Call(uintptr(unsafe.Pointer(&b.msg)), 0, 0, 0)
		switch int32(r) {
		case 0: // WM_QUIT
			return b.exit()
		case -1:
			return winError("GetMessageW", e)
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&b.msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&b.msg)))
	}
	return b.exit()
}

// exit ends Run: it re-raises a panic of the handler, otherwise it is a
// normal return.
func (b *Backend) exit() error {
	if b.failed {
		panic(b.failure)
	}
	return nil
}

// shutdown releases the OS resources when Run returns.
func (b *Backend) shutdown() {
	if b.closed {
		return
	}
	b.closed = true
	for _, w := range slices.Clone(b.loop.Windows()) {
		w.Close()
	}
	b.loop.Discard()
	b.stopped.Store(true)
	registry.mu.Lock()
	delete(registry.backends, b.msgHwnd)
	registry.mu.Unlock()
	procDestroyWindow.Call(b.msgHwnd)
	// PostQuitMessage leaves a WM_QUIT in the thread's queue when the loop
	// ended without reading it (Quit seen between messages). The thread
	// goes back to the Go scheduler's pool, where the next Run on it would
	// stop at once. A PeekMessage restricted to WM_QUIT does not find it on
	// Windows (it did under Wine), so everything is drained: the windows are
	// gone, and what is left in the queue belongs to nobody.
	for {
		r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&b.msg)), 0, 0, 0, pmRemove)
		if r == 0 {
			break
		}
	}
	runtime.UnlockOSThread()
}

// pump runs a delivery pass: the queued events, the due frames, arming the
// frame pacer for the requested ones that are not due, and, if the handler
// left work behind (events that arrived while it ran), a follow-up pass. It
// does nothing inside a pass: the handler is never called re-entrantly, and
// what was queued in the meantime is picked up by the follow-up.
func (b *Backend) pump() {
	if b.pumping || b.handler == nil {
		return
	}
	b.dirty = false
	b.pumping = true
	func() {
		defer func() { b.pumping = false }()
		b.loop.Deliver(b.handler)
	}()
	if b.loop.Quitting() {
		postQuitMessage()
		return
	}
	// The Pace hook arranges a display signal for every frame that waits,
	// so the loop never needs a timer deadline here.
	b.loop.Pace()
	if b.loop.Ready() {
		b.notify()
	}
}

// fail records a panic of the handler, which cannot unwind through the
// Windows frames above WndProc, and stops the loop; Run raises it again.
func (b *Backend) fail(r any) {
	if !b.failed {
		b.failed, b.failure = true, r
	}
	b.loop.Quit()
	postQuitMessage()
}

// enqueue, coalesce and reconfigure queue work for the next pass.
func (b *Backend) enqueue(w *Window, e platform.Event) {
	b.dirty = true
	b.loop.Enqueue(w, e)
}

func (b *Backend) coalesce(w *Window, e platform.Event, replace func(prev platform.Event) bool) {
	b.dirty = true
	b.loop.Coalesce(w, e, replace)
}

func (b *Backend) reconfigure(w *Window, m platform.Metrics) {
	b.dirty = true
	b.loop.Reconfigure(w, m)
}

// message handles the messages of the message-only window.
func (b *Backend) message(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmTelaNotify:
		// Clear first: a Wake that races with the pass either sets the
		// wake flag before the pass reads it, or posts a new message.
		b.notifyPosted.Store(false)
		b.pump()
		return 0
	case wmTelaVsync:
		for _, w := range b.loop.Windows() {
			if w.VsyncPending() {
				w.VsyncDone()
			}
		}
		b.pump()
		return 0
	}
	return defWindowProc(hwnd, uintptr(msg), wparam, lparam)
}

// paceHook is the loop's Pace hook: a frame is waiting for the display, so
// the window records that a display signal is outstanding and the vsync
// goroutine is started. It always succeeds.
func (b *Backend) paceHook(w *Window, _ time.Duration) bool {
	w.VsyncRequested()
	if !b.vsyncBusy.Swap(true) {
		go b.waitVsync()
	}
	return true
}

// waitVsync blocks until the compositor's next vsync (DwmFlush) and posts
// the private message that turns it into the loop's display signal. If
// DwmFlush returns at once, because composition is off or Windows is
// emulated, it sleeps out the refresh interval instead, so frames stay
// paced and the loop never spins.
func (b *Backend) waitVsync() {
	start := time.Now()
	if available(procDwmFlush) {
		procDwmFlush.Call()
	}
	if d := time.Since(start); d < b.interval/32 {
		time.Sleep(b.interval - d)
	}
	b.vsyncBusy.Store(false)
	if !b.stopped.Load() {
		postMessage(b.msgHwnd, wmTelaVsync, 0, 0)
	}
}

// cursorHandle returns the system cursor that shows c (0 hides it).
func (b *Backend) cursorHandle(c platform.Cursor) uintptr {
	if h, ok := b.cursors[c]; ok {
		return h
	}
	var h uintptr
	if idc, ok := cursorResource(c); ok {
		h, _, _ = procLoadCursorW.Call(0, idc)
	}
	b.cursors[c] = h
	return h
}

// --- process setup ---

var dpiOnce sync.Once

// initDPIAwareness opts the process in to the best DPI awareness the system
// offers: per-monitor v2 (Windows 10 1703), per-monitor (8.1), system
// (Vista). It must run before the first window exists. If the process
// already has an awareness, from a manifest or an earlier call, the calls
// fail with access denied and that awareness stays.
func initDPIAwareness() {
	dpiOnce.Do(func() {
		if available(procSetProcessDpiAwarenessContext) {
			r, _, e := procSetProcessDpiAwarenessContext.Call(dpiPerMonitorAwareV2)
			if r != 0 || e == syscall.Errno(errAccessDenied) {
				return
			}
		}
		if available(procSetProcessDpiAwareness) {
			const eAccessDenied = 0x80070005
			hr, _, _ := procSetProcessDpiAwareness.Call(processPerMonitorDPIAware)
			if hr == 0 || uint32(hr) == eAccessDenied {
				return
			}
		}
		if available(procSetProcessDPIAware) {
			procSetProcessDPIAware.Call()
		}
	})
}

// displayInterval returns the refresh interval of the primary display (60 Hz
// when it cannot be read).
func displayInterval() time.Duration {
	dm := devMode{size: uint16(unsafe.Sizeof(devMode{}))}
	r, _, _ := procEnumDisplaySetting.Call(0, uintptr(enumCurrentSets), uintptr(unsafe.Pointer(&dm)))
	if r == 0 {
		return refreshInterval(0)
	}
	return refreshInterval(dm.displayFrequency)
}

// monitorDPI returns the effective DPI of a monitor.
func monitorDPI(mon uintptr) uint32 {
	if mon != 0 && available(procGetDpiForMonitor) {
		var x, y uint32
		hr, _, _ := procGetDpiForMonitor.Call(mon, mdtEffectiveDPI, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
		if hr == 0 && x != 0 {
			return x
		}
	}
	return systemDPI()
}

// systemDPI returns the DPI of the primary display.
func systemDPI() uint32 {
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return baseDPI
	}
	defer procReleaseDC.Call(0, dc)
	dpi, _, _ := procGetDeviceCaps.Call(dc, logPixelsX)
	if dpi == 0 {
		return baseDPI
	}
	return uint32(dpi)
}

// windowDPI returns the DPI of the monitor a window is on.
func windowDPI(hwnd uintptr) uint32 {
	if available(procGetDpiForWindow) {
		if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi != 0 {
			return uint32(dpi)
		}
	}
	mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultNr)
	return monitorDPI(mon)
}

// primaryDPI returns the DPI of the primary monitor, where a new window is
// placed.
func primaryDPI() uint32 {
	mon, _, _ := procMonitorFromPoint.Call(0, 0, monitorDefaultPri)
	return monitorDPI(mon)
}

// --- window class and WndProc ---

var (
	classOnce    sync.Once
	classErr     error
	classNamePtr *uint16
)

// registerClass registers the window class of every Tela window and of the
// message-only window, once per process.
func registerClass(hinst uintptr) error {
	classOnce.Do(func() {
		name, err := syscall.UTF16PtrFromString("TelaWindow")
		if err != nil {
			classErr = err
			return
		}
		classNamePtr = name
		wc := wndClassEx{
			size:      uint32(unsafe.Sizeof(wndClassEx{})),
			wndProc:   wndProcPtr(),
			instance:  hinst,
			className: name,
		}
		r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if r == 0 && e != syscall.Errno(errClassExists) {
			classErr = winError("registering the window class", e)
		}
	})
	return classErr
}

// utf16 converts a title to UTF-16 for the W functions: invalid UTF-8 and
// NUL characters, which would end the string early, are replaced.
func utf16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(sanitizeTitle(s))
	if err != nil { // unreachable after sanitizeTitle
		panic(fmt.Sprintf("win32: converting %q to UTF-16: %v", s, err))
	}
	return p
}
