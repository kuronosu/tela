package remote

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// handshakeTimeout bounds a child's hello.
const handshakeTimeout = 10 * time.Second

// HostOptions configures a Host. Callbacks run on the UI thread of the
// host's backend.
type HostOptions struct {
	// Addr is the socket path to listen on. Empty means DefaultAddr().
	Addr string
	// Scale, when > 0, is the scale children render at (tela dev
	// --scale): their ConfigureEvents carry it instead of the real
	// window's scale (the logical size is the real one), and the host
	// resamples their frames to the real window's physical size with
	// nearest-neighbor sampling. Pointer and IME coordinates are logical,
	// so they are unchanged.
	Scale float64
	// WindowOptions, when set, rewrites the options a child asked for
	// before the host creates a real window with them (tela dev --size
	// overrides the size). id is the window ID. It is not called when a
	// child maps onto an existing real window, which keeps its size. The
	// result is resolved (platform.WindowOptions.Resolve) and must be
	// valid; an invalid one fails the window like a backend error.
	WindowOptions func(id uint32, opts platform.WindowOptions) platform.WindowOptions
	// Settings, when set, rewrites the real backend's system settings
	// before children see them, in their welcome and in every
	// SettingsEvent (tela dev --dark forces DarkMode). It is called on the
	// UI thread with the backend's latest settings.
	Settings func(platform.SystemSettings) platform.SystemSettings
	// OnConnect is called when a child connected (it is pending or, if no
	// child was active and input is not frozen, active).
	OnConnect func(c *Child)
	// OnFirstFrame is called after a child's first Present.
	OnFirstFrame func(c *Child)
	// OnDisconnect is called when a child's connection ended: err is nil
	// when the child closed it (it quit, or its process exited or
	// crashed: the socket cannot tell), ErrClosed when the host closed it
	// (Child.Close, Host.Close), otherwise the protocol or I/O error. The
	// child is
	// detached first; the last frame of a child that was active stays on
	// its windows.
	OnDisconnect func(c *Child, err error)
	// OnUnhandled receives the real windows' input events that no child
	// receives (no child is active and input is not frozen), such as a
	// CloseRequestedEvent after the app crashed.
	OnUnhandled func(w platform.Window, e platform.Event)
	// OnError reports a non-fatal error: a failed Present or window
	// creation, a shared buffer that could not be mapped.
	OnError func(err error)
}

// Host is the supervisor side: it owns the real windows (on any
// platform.Backend) and shows the windows of child processes that use a
// remote Backend. It is a platform.Handler and must receive every event of
// its backend: run the backend with Run, or call HandleEvent from your own
// handler for every event. See the package documentation.
//
// Methods are called on the backend's UI thread, except where noted.
type Host struct {
	b    platform.Backend
	name string // b.Name(), read on the UI thread for the handshakes
	opts HostOptions
	ln   net.Listener
	addr string

	mu       sync.Mutex
	inbox    []func() // work for the UI thread, run at the next WakeEvent
	settings platform.SystemSettings
	handlers map[string]func(*Child, []byte) ([]byte, error)
	all      map[*Child]struct{} // connected children, for Close
	closed   bool

	// UI thread.
	children []*Child // connected and not yet disconnected, in order
	active   *Child
	frozen   bool
	queue    []queuedEvent // input held while frozen
	byID     map[uint32]*realWin
	byWin    map[platform.Window]*realWin
	order    []*realWin // real windows in creation order
	overlay  func(w platform.Window, fb *image.RGBA)
}

type queuedEvent struct {
	rw *realWin
	e  platform.Event
}

var _ platform.Handler = (*Host)(nil)

// NewHost starts listening for children on opts.Addr. b is the backend of
// the real windows; NewHost must be called on its UI thread, before Run or
// from its handler. An invalid opts.Scale panics.
func NewHost(b platform.Backend, opts HostOptions) (*Host, error) {
	if !supported {
		return nil, fmt.Errorf("remote: no shared memory on this system: %w", platform.ErrUnavailable)
	}
	if opts.Scale != 0 {
		platform.PhysicalSize(platform.Size{}, opts.Scale) // validates
	}
	addr := opts.Addr
	if addr == "" {
		addr = DefaultAddr()
	}
	ln, err := net.Listen("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("remote: listen: %w", err)
	}
	h := &Host{
		b:        b,
		name:     b.Name(),
		opts:     opts,
		ln:       ln,
		addr:     addr,
		settings: opts.filterSettings(b.Settings()),
		handlers: map[string]func(*Child, []byte) ([]byte, error){},
		all:      map[*Child]struct{}{},
		byID:     map[uint32]*realWin{},
		byWin:    map[platform.Window]*realWin{},
	}
	go h.accept()
	return h, nil
}

// filterSettings applies the Settings option.
func (o *HostOptions) filterSettings(s platform.SystemSettings) platform.SystemSettings {
	if o.Settings != nil {
		s = o.Settings(s)
	}
	return s
}

// Addr returns the socket path children dial. Safe from any goroutine.
func (h *Host) Addr() string { return h.addr }

// Backend returns the backend of the real windows.
func (h *Host) Backend() platform.Backend { return h.b }

// Run runs the backend's event loop with the host as its handler.
func (h *Host) Run() error { return h.b.Run(h) }

// Close stops listening and closes every child connection. Real windows
// are left to the backend (they go away when its Run returns). Safe from
// any goroutine; idempotent.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	cs := make([]*Child, 0, len(h.all))
	for c := range h.all {
		cs = append(cs, c)
	}
	h.mu.Unlock()
	err := h.ln.Close()
	for _, c := range cs {
		c.c.fail(ErrClosed)
	}
	return err
}

// Post runs fn on the UI thread, in order with the host's own work, during
// a later WakeEvent. Safe from any goroutine: it is how a supervisor's
// goroutines call the UI-thread methods (Freeze, Promote, SetOverlay).
func (h *Host) Post(fn func()) {
	h.mu.Lock()
	h.inbox = append(h.inbox, fn)
	h.mu.Unlock()
	h.b.Wake()
}

// Handle registers fn for the children's requests and notifications of
// the given kind (nil removes it). fn runs on the UI thread; for a
// request, its result is the reply. Safe from any goroutine.
func (h *Host) Handle(kind string, fn func(c *Child, payload []byte) ([]byte, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if fn == nil {
		delete(h.handlers, kind)
	} else {
		h.handlers[kind] = fn
	}
}

// Children returns the connected children, in connection order.
func (h *Host) Children() []*Child { return slices.Clone(h.children) }

// Active returns the active child, or nil.
func (h *Host) Active() *Child { return h.active }

// Windows returns the real windows, in creation order.
func (h *Host) Windows() []platform.Window {
	ws := make([]platform.Window, len(h.order))
	for i, rw := range h.order {
		ws[i] = rw.w
	}
	return ws
}

// --- freeze and swap ---

// Freeze holds input: from now on the real windows' input events (pointer,
// scroll, key, IME, focus, close requests) are queued instead of being
// delivered to the active child, until Promote or Unfreeze delivers them
// in order. ConfigureEvents, FrameEvents and SettingsEvents keep flowing
// to every child, and focus changes still reach pending children. A child
// that connects while input is frozen is pending.
func (h *Host) Freeze() { h.frozen = true }

// Frozen reports whether input is frozen.
func (h *Host) Frozen() bool { return h.frozen }

// Unfreeze delivers the queued input to the active child (or to
// OnUnhandled) and resumes normal delivery.
func (h *Host) Unfreeze() {
	if !h.frozen {
		return
	}
	h.frozen = false
	q := h.queue
	h.queue = nil
	for _, it := range q {
		h.route(it.rw, it.e)
	}
}

// Promote makes c the active child: its windows take over their real
// windows (title, cursor and text input state are applied, the IME session
// restarts), its latest frame is presented in full, and it receives input,
// starting with the input queued since Freeze (Promote implies Unfreeze).
// The previously active child is detached: it receives nothing more, and
// the caller stops it. If c has windows, real windows it does not use are
// closed. Promoting the active child only unfreezes.
func (h *Host) Promote(c *Child) error {
	if c.h != h {
		panic("remote: Promote called with a child of another host")
	}
	switch c.state {
	case stateDetached:
		return errors.New("remote: Promote: the child is detached or disconnected")
	case stateActive:
		h.Unfreeze()
		return nil
	}
	if old := h.active; old != nil {
		h.detach(old)
	}
	c.state = stateActive
	h.active = c
	used := map[*realWin]bool{}
	for _, cw := range c.windows {
		rw := cw.rw
		if rw == nil {
			continue
		}
		used[rw] = true
		rw.pending = slices.DeleteFunc(rw.pending, func(x *childWin) bool { return x == cw })
		h.activate(rw, cw)
	}
	if len(used) > 0 {
		for _, rw := range slices.Clone(h.order) {
			if !used[rw] && rw.active == nil && len(rw.pending) == 0 {
				h.closeReal(rw)
			}
		}
	}
	for rw := range used {
		h.flush(rw)
	}
	h.Unfreeze()
	return nil
}

// Detach stops routing anything to c: it is no longer active or pending,
// and its windows keep their last frame on screen (a detached child's
// frame is what an error overlay is drawn over). It does not close the
// connection (control requests still work); use Child.Close for that.
func (h *Host) Detach(c *Child) { h.detach(c) }

func (h *Host) detach(c *Child) {
	if c.state == stateDetached {
		return
	}
	if h.active == c {
		h.active = nil
	}
	c.state = stateDetached
	for _, cw := range c.windows {
		rw := cw.rw
		if rw == nil {
			continue
		}
		if rw.active == cw {
			rw.active = nil
			h.setTextInput(rw, nil)
		}
		rw.pending = slices.DeleteFunc(rw.pending, func(x *childWin) bool { return x == cw })
	}
}

// --- overlay ---

// SetOverlay installs fn, which draws the supervisor's pixels (an error
// overlay) over the frame of each real window right before the host
// presents it: fb has the window's physical size and holds the frame of
// its active child, or of the last active one if it was detached (black if
// there is none). While an overlay is set every present is a full frame
// composed into a scratch buffer, and fn draws all of its pixels every
// time. nil removes it. Either way the windows are presented again at
// once.
func (h *Host) SetOverlay(fn func(w platform.Window, fb *image.RGBA)) {
	h.overlay = fn
	h.Invalidate()
}

// Invalidate presents every real window again, in full (after the
// overlay's content changed).
func (h *Host) Invalidate() {
	for _, rw := range h.order {
		rw.full = true
		h.flush(rw)
	}
}

// Frame returns a copy of the frame the real window w shows (before
// resampling and overlay), at the child's resolution, or nil.
func (h *Host) Frame(w platform.Window) *image.RGBA {
	rw := h.byWin[w]
	if rw == nil || rw.src == nil {
		return nil
	}
	cw := rw.src
	cw.mu.Lock()
	defer cw.mu.Unlock()
	if cw.frame == nil {
		return nil
	}
	f := image.NewRGBA(cw.frame.Rect)
	copy(f.Pix, cw.frame.Pix)
	return f
}

// --- events ---

// HandleEvent routes one event of the host's backend; see Host.
func (h *Host) HandleEvent(w platform.Window, e platform.Event) {
	switch e := e.(type) {
	case platform.WakeEvent:
		h.drain()
		return
	case platform.SettingsEvent:
		s := h.opts.filterSettings(e.Settings)
		h.mu.Lock()
		same := s == h.settings
		h.settings = s
		h.mu.Unlock()
		if same && h.opts.Settings != nil {
			return // the override hides the change (--dark and a dark-mode toggle)
		}
		e.Settings = s
		for _, c := range h.children {
			if c.state != stateDetached {
				c.sendEvent(0, e)
			}
		}
		return
	}
	if rw := h.byWin[w]; rw != nil {
		h.windowEvent(rw, e)
	} else if fn := h.opts.OnUnhandled; fn != nil {
		fn(w, e)
	}
}

func (h *Host) drain() {
	h.mu.Lock()
	items := h.inbox
	h.inbox = nil
	h.mu.Unlock()
	for _, fn := range items {
		fn()
	}
}

// post queues UI-thread work from a goroutine of the host.
func (h *Host) post(fn func()) { h.Post(fn) }

func (h *Host) reportError(err error) {
	if fn := h.opts.OnError; fn != nil {
		fn(err)
	}
}

// postError reports err from a goroutine.
func (h *Host) postError(err error) { h.post(func() { h.reportError(err) }) }

// windowEvent routes an event of a real window.
func (h *Host) windowEvent(rw *realWin, e platform.Event) {
	switch ev := e.(type) {
	case platform.ConfigureEvent:
		rw.metrics = ev.Metrics
		rw.full = true
		for _, cw := range rw.attached() {
			h.configure(rw, cw)
		}
	case platform.FrameEvent:
		for _, cw := range rw.attached() {
			if cw.wantFrame {
				cw.wantFrame = false
				h.deliver(cw, ev)
			}
		}
		h.flush(rw)
	case platform.FocusEvent:
		rw.focused = ev.Focused
		for _, cw := range rw.pending {
			h.deliver(cw, ev)
		}
		h.input(rw, e)
	default:
		h.input(rw, e)
	}
}

// input delivers or queues an input event.
func (h *Host) input(rw *realWin, e platform.Event) {
	if h.frozen {
		h.queue = append(h.queue, queuedEvent{rw, e})
		return
	}
	h.route(rw, e)
}

func (h *Host) route(rw *realWin, e platform.Event) {
	if rw.closed {
		return
	}
	// The pointer as of the routed stream (not of the queue's end): a
	// child promoted during a freeze gets its enter at the position that
	// precedes the queued moves.
	if pe, ok := e.(platform.PointerEvent); ok {
		if pe.Action == platform.PointerLeave {
			rw.pointer = nil
		} else {
			rw.pointer = &pe
		}
	}
	if rw.active != nil {
		h.deliver(rw.active, e)
	} else if fn := h.opts.OnUnhandled; fn != nil {
		fn(rw.w, e)
	}
}

// childMetrics returns the metrics children are told for rw.
func (h *Host) childMetrics(rw *realWin) platform.Metrics {
	m := rw.metrics
	if h.opts.Scale > 0 {
		m.Scale = h.opts.Scale
	}
	return m
}

// configure sends cw the metrics of rw if they changed; a frame follows.
func (h *Host) configure(rw *realWin, cw *childWin) {
	if rw.metrics.Scale == 0 {
		return
	}
	m := h.childMetrics(rw)
	if m == cw.sent {
		return
	}
	cw.sent = m
	cw.c.sendEvent(cw.id, platform.ConfigureEvent{Metrics: m})
	cw.wantFrame = true
	rw.w.RequestFrame()
}

// deliver sends e to cw through cw's view of the window, which keeps every
// child's event stream valid under the platform contract whatever it
// missed while pending: nothing before its ConfigureEvent, focus and IME
// session changes only when they change its view, key events only while
// focused and releases only of presses it received, IME events only in an
// active session.
func (h *Host) deliver(cw *childWin, e platform.Event) {
	if cw.closed || cw.c.state == stateDetached || cw.sent.Scale == 0 {
		return
	}
	switch ev := e.(type) {
	case platform.FocusEvent:
		if ev.Focused == cw.focused {
			return
		}
		cw.focused = ev.Focused
		if !ev.Focused {
			// The real backend already released the held keys; these
			// are presses the child got and the real window did not
			// release in its view (none, normally).
			for _, r := range cw.held.ReleaseAll(h.b.Now()) {
				cw.c.sendEvent(cw.id, r)
			}
		}
	case platform.KeyEvent:
		if !cw.focused || !cw.held.Accept(ev) {
			return
		}
	case platform.IMESessionEvent:
		if ev.Active == cw.imeActive {
			return
		}
		cw.imeActive = ev.Active
	case platform.IMEEvent:
		if !cw.imeActive {
			return
		}
	}
	cw.c.sendEvent(cw.id, e)
}

// --- connections (goroutines) ---

func (h *Host) accept() {
	for {
		nc, err := h.ln.Accept()
		if err != nil {
			h.mu.Lock()
			closed := h.closed
			h.mu.Unlock()
			if !closed {
				h.postError(fmt.Errorf("remote: accept: %w", err))
			}
			return
		}
		go h.serve(nc.(*net.UnixConn))
	}
}

// serve performs the handshake and runs the child's reader.
func (h *Host) serve(nc *net.UnixConn) {
	t := newTransport(nc)
	r := newFrameReader(t)
	t.SetReadDeadline(time.Now().Add(handshakeTimeout))
	typ, p, err := r.next()
	if err != nil {
		t.Close()
		return
	}
	reject := func(reason string) {
		e := newMsg(msgReject)
		e.str(reason)
		t.write(e.bytes(), nil)
		t.Close()
	}
	d := &dec{b: p}
	mg := string(d.take(len(magic)))
	version := d.u16()
	pid := d.u32()
	tag := d.str()
	if typ != msgHello || mg != magic || d.end() != nil {
		reject("not a tela remote hello")
		return
	}
	if version != ProtocolVersion {
		reject(fmt.Sprintf("protocol version %d, the host speaks %d; rebuild the app and the host with the same tela version", version, ProtocolVersion))
		return
	}
	t.SetReadDeadline(time.Time{})
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		reject("the host is closing")
		return
	}
	settings := h.settings
	h.mu.Unlock()
	e := newMsg(msgWelcome)
	e.u16(ProtocolVersion)
	e.dur(h.b.Now())
	e.settings(settings)
	e.str(h.name)
	if err := t.write(e.bytes(), nil); err != nil {
		t.Close()
		return
	}
	c := &Child{
		h:          h,
		pid:        int(pid),
		tag:        tag,
		welcomed:   settings,
		wins:       map[uint32]*childWin{},
		firstFrame: make(chan struct{}),
		done:       make(chan struct{}),
	}
	c.c = newConn(t, r)
	c.ctl = newCtl(c.c)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		c.c.fail(ErrClosed)
		return
	}
	h.all[c] = struct{}{}
	h.mu.Unlock()
	h.post(func() { h.connected(c) })
	c.readLoop()
}

// --- UI-thread handlers of child messages ---

func (h *Host) connected(c *Child) {
	h.children = append(h.children, c)
	if h.active == nil && !h.frozen {
		c.state = stateActive
		h.active = c
	} else {
		c.state = statePending
	}
	h.mu.Lock()
	s := h.settings
	h.mu.Unlock()
	if s != c.welcomed {
		c.sendEvent(0, platform.SettingsEvent{Settings: s})
	}
	if fn := h.opts.OnConnect; fn != nil {
		fn(c)
	}
}

func (h *Host) lost(c *Child, err error) {
	h.detach(c)
	h.children = slices.DeleteFunc(h.children, func(x *Child) bool { return x == c })
	h.mu.Lock()
	delete(h.all, c)
	h.mu.Unlock()
	c.err = err
	close(c.done)
	if fn := h.opts.OnDisconnect; fn != nil {
		fn(c, err)
	}
}

// attach maps a new child window onto the real window with its ID,
// creating that window if needed.
func (h *Host) attach(cw *childWin) {
	c := cw.c
	if c.state == stateDetached || cw.closed {
		return
	}
	rw := h.byID[cw.id]
	if rw == nil {
		opts := cw.opts
		var err error
		if fn := h.opts.WindowOptions; fn != nil {
			err = try(func() { opts = fn(cw.id, opts).Resolve() })
		}
		var w platform.Window
		if err == nil {
			w, err = h.b.NewWindow(opts)
		}
		if err != nil {
			h.reportError(fmt.Errorf("remote: creating window %d: %w", cw.id, err))
			c.Close()
			return
		}
		rw = &realWin{id: cw.id, w: w}
		h.byID[cw.id] = rw
		h.byWin[w] = rw
		h.order = append(h.order, rw)
	}
	cw.rw = rw
	c.windows = append(c.windows, cw)
	if c.state == stateActive {
		h.activate(rw, cw)
	} else {
		rw.pending = append(rw.pending, cw)
	}
	if rw.metrics.Scale > 0 {
		h.configure(rw, cw)
		if rw.focused {
			h.deliver(cw, platform.FocusEvent{Focused: true})
		}
		h.enter(rw, cw)
	}
}

// activate makes cw the active window of rw and applies its state.
func (h *Host) activate(rw *realWin, cw *childWin) {
	rw.active = cw
	rw.src = cw
	rw.full = true
	rw.w.SetTitle(cw.title)
	rw.w.SetCursor(cw.cursor)
	if rw.textInput != nil && cw.textInput != nil {
		// Another process's field: restart the IME session.
		h.setTextInput(rw, nil)
	}
	h.setTextInput(rw, cw.textInput)
	h.enter(rw, cw)
}

// enter tells the active cw that the pointer is over the window, if it
// is: a child that takes over a window under the pointer shows hover
// state at once.
func (h *Host) enter(rw *realWin, cw *childWin) {
	if rw.active != cw || rw.pointer == nil || cw.sent.Scale == 0 || cw.entered {
		return
	}
	cw.entered = true
	p := rw.pointer
	h.deliver(cw, platform.PointerEvent{Action: platform.PointerEnter, Kind: p.Kind, ID: p.ID, Pos: p.Pos, Mods: p.Mods, Time: h.b.Now()})
}

func (h *Host) setTextInput(rw *realWin, st *platform.TextInputState) {
	if rw.closed || rw.textInput.Equal(st) {
		return
	}
	rw.w.SetTextInput(st)
	rw.textInput = st.Clone()
}

// closeReal closes a real window.
func (h *Host) closeReal(rw *realWin) {
	if rw.closed {
		return
	}
	rw.closed = true
	rw.w.Close()
	delete(h.byID, rw.id)
	delete(h.byWin, rw.w)
	h.order = slices.DeleteFunc(h.order, func(x *realWin) bool { return x == rw })
	for _, cw := range rw.pending {
		cw.rw = nil
	}
	if rw.active != nil {
		rw.active.rw = nil
	}
	rw.active, rw.pending, rw.src = nil, nil, nil
}

// live reports whether cw's operations take effect: its child is not
// detached and it is open.
func (cw *childWin) live() bool { return !cw.closed && cw.c.state != stateDetached }

// isActive reports whether cw is shown and receives input.
func (cw *childWin) isActive() bool { return cw.rw != nil && cw.rw.active == cw }

func (h *Host) closeWindow(cw *childWin) {
	if cw.closed {
		return
	}
	cw.closed = true
	c := cw.c
	c.windows = slices.DeleteFunc(c.windows, func(x *childWin) bool { return x == cw })
	rw := cw.rw
	if rw == nil {
		return
	}
	if rw.active == cw && c.state == stateActive {
		h.closeReal(rw)
		return
	}
	rw.pending = slices.DeleteFunc(rw.pending, func(x *childWin) bool { return x == cw })
}

func (h *Host) presented(cw *childWin) {
	cw.notified.Store(false)
	if cw.closed || cw.rw == nil {
		return
	}
	if cw.rw.src == cw && cw.c.state != stateDetached {
		h.flush(cw.rw)
	}
}

func (h *Host) clipRead(c *Child, id uint32) {
	h.b.Clipboard().ReadText(func(text string, err error) {
		e := newMsg(msgClipReply)
		e.u32(id)
		e.str(text)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		e.str(msg)
		e.boolean(err != nil)
		c.c.send(e.bytes())
	})
}

func (h *Host) clipWrite(c *Child, text string) {
	if c.state == stateDetached {
		return
	}
	if err := h.b.Clipboard().WriteText(text); err != nil {
		h.reportError(err)
	}
}

func (h *Host) runHandler(c *Child, in incoming) {
	h.mu.Lock()
	fn := h.handlers[in.kind]
	h.mu.Unlock()
	var f func([]byte) ([]byte, error)
	if fn != nil {
		f = func(p []byte) ([]byte, error) { return fn(c, p) }
	}
	c.ctl.run(in, f)
}

// --- Child ---

type childState uint8

const (
	stateNew childState = iota
	statePending
	stateActive
	stateDetached
)

// Child is a connected child process, as seen by the host. Methods are
// called on the host's UI thread, except where noted.
type Child struct {
	h        *Host
	c        *conn
	ctl      *ctl
	pid      int
	tag      string
	welcomed platform.SystemSettings // the settings sent in the welcome

	// UI thread.
	state   childState
	windows []*childWin // attached windows, in creation order

	mu   sync.Mutex
	wins map[uint32]*childWin // the reader goroutine's open windows

	firstOnce  sync.Once
	firstFrame chan struct{}
	done       chan struct{}
	err        error // set before done is closed
}

// PID returns the child's process ID (as the child reported it). Safe
// from any goroutine.
func (c *Child) PID() int { return c.pid }

// Tag returns the child's DialOptions.Tag. Safe from any goroutine.
func (c *Child) Tag() string { return c.tag }

// Active reports whether c is the active child.
func (c *Child) Active() bool { return c.state == stateActive }

// Pending reports whether c is connected but neither active nor detached:
// it renders offscreen, its frames are kept and not shown.
func (c *Child) Pending() bool { return c.state == statePending }

// FirstFrame is closed when the child presented its first frame. Safe from
// any goroutine.
func (c *Child) FirstFrame() <-chan struct{} { return c.firstFrame }

// Done is closed when the connection ended (after OnDisconnect's child is
// detached). Safe from any goroutine.
func (c *Child) Done() <-chan struct{} { return c.done }

// Err returns why the connection ended (see HostOptions.OnDisconnect);
// valid once Done is closed. Safe from any goroutine.
func (c *Child) Err() error {
	select {
	case <-c.done:
		return c.err
	default:
		return nil
	}
}

// Request sends a request to the child and waits for its reply (computed
// on the child's UI thread), for ctx, or for the end of the connection.
// Safe from any goroutine; do not call it on the host's UI thread if the
// child's handler may wait for the host.
func (c *Child) Request(ctx context.Context, kind string, payload []byte) ([]byte, error) {
	return c.ctl.request(ctx, kind, payload)
}

// Notify sends a notification to the child. Safe from any goroutine.
func (c *Child) Notify(kind string, payload []byte) error { return c.ctl.notify(kind, payload) }

// Close closes the connection: the child's Run returns an error. Safe from
// any goroutine.
func (c *Child) Close() { c.c.fail(ErrClosed) }

func (c *Child) sendEvent(id uint32, ev platform.Event) {
	e := newMsg(msgEvent)
	e.u32(id)
	e.event(ev)
	c.c.send(e.bytes())
}

func (c *Child) win(id uint32) *childWin {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wins[id]
}

// readLoop is the child's reader goroutine on the host.
func (c *Child) readLoop() {
	var err error
	for {
		t, p, e := c.c.r.next()
		if e != nil {
			err = e
			break
		}
		if e := c.dispatch(t, &dec{b: p}); e != nil {
			err = e
			break
		}
	}
	c.c.fail(err)
	final := c.c.Err()
	if errors.Is(final, io.EOF) {
		final = nil
	}
	c.ctl.fail(fmt.Errorf("remote: connection to the child lost: %w", c.c.Err()))
	c.mu.Lock()
	for _, cw := range c.wins {
		cw.mu.Lock()
		cw.seg.close()
		cw.seg = nil
		cw.mu.Unlock()
	}
	c.mu.Unlock()
	c.h.post(func() { c.h.lost(c, final) })
}

// dispatch handles one message of the child, on its reader goroutine.
func (c *Child) dispatch(t msgType, d *dec) error {
	h := c.h
	switch t {
	case msgNewWindow:
		id := d.u32()
		opts := d.windowOptions()
		if err := d.end(); err != nil {
			return err
		}
		if id == 0 {
			return errors.New("remote: window ID 0")
		}
		if err := try(func() { opts = opts.Resolve() }); err != nil {
			return err
		}
		cw := &childWin{c: c, id: id, opts: opts, title: opts.Title}
		c.mu.Lock()
		dup := c.wins[id] != nil
		if !dup {
			c.wins[id] = cw
		}
		c.mu.Unlock()
		if dup {
			return fmt.Errorf("remote: window %d created twice", id)
		}
		h.post(func() { h.attach(cw) })
	case msgCloseWindow:
		id := d.u32()
		if err := d.end(); err != nil {
			return err
		}
		c.mu.Lock()
		cw := c.wins[id]
		delete(c.wins, id)
		c.mu.Unlock()
		if cw != nil {
			cw.mu.Lock()
			cw.seg.close()
			cw.seg = nil
			cw.mu.Unlock()
			h.post(func() { h.closeWindow(cw) })
		}
	case msgSetTitle:
		id, s := d.u32(), d.str()
		return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
			cw.title = s
			if active {
				rw.w.SetTitle(s)
			}
		})
	case msgSetCursor:
		id, cur := d.u32(), platform.Cursor(d.u8())
		if !cur.Valid() {
			return fmt.Errorf("remote: invalid cursor %d", cur)
		}
		return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
			cw.cursor = cur
			if active {
				rw.w.SetCursor(cur)
			}
		})
	case msgSetSize:
		id, size := d.u32(), d.size()
		if err := try(func() { platform.CheckSize("SetSize size", size) }); err != nil {
			return err
		}
		return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
			if active {
				rw.w.SetSize(size)
			}
		})
	case msgSetTextInput:
		id, st := d.u32(), d.textInput()
		if d.err == nil {
			if err := try(func() { platform.CheckTextInputState(st) }); err != nil {
				return err
			}
		}
		return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
			cw.textInput = st
			if active {
				h.setTextInput(rw, st)
			}
		})
	case msgResetTextInput:
		id := d.u32()
		return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
			if active && rw.textInput != nil {
				rw.w.ResetTextInput()
			}
		})
	case msgRequestFrame:
		id := d.u32()
		return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
			cw.wantFrame = true
			rw.w.RequestFrame()
		})
	case msgBuffers:
		return c.buffers(d)
	case msgPresent:
		return c.present(d)
	case msgClipRead:
		id := d.u32()
		if err := d.end(); err != nil {
			return err
		}
		h.post(func() { h.clipRead(c, id) })
	case msgClipWrite:
		text := d.str()
		if err := d.end(); err != nil {
			return err
		}
		h.post(func() { h.clipWrite(c, text) })
	case msgCtlRequest, msgCtlNotify:
		var in incoming
		var err error
		if t == msgCtlRequest {
			in, err = decodeRequest(d)
		} else {
			in, err = decodeNotify(d)
		}
		if err != nil {
			return err
		}
		h.post(func() { h.runHandler(c, in) })
	case msgCtlReply:
		return c.ctl.replied(d)
	default:
		return fmt.Errorf("remote: unexpected message %d from a child", t)
	}
	return nil
}

// op posts a window operation for the UI thread; it is dropped if the
// window is unknown or closed, its child detached or it has no real
// window.
func (c *Child) op(d *dec, id uint32, fn func(cw *childWin, rw *realWin, active bool)) error {
	if err := d.end(); err != nil {
		return err
	}
	cw := c.win(id)
	if cw == nil {
		return nil
	}
	c.h.post(func() {
		if cw.live() && cw.rw != nil {
			fn(cw, cw.rw, cw.isActive())
		}
	})
	return nil
}

// buffers maps a window's new shared segment.
func (c *Child) buffers(d *dec) error {
	id, gen := d.u32(), d.u32()
	w, hgt, stride, slots := int(d.u32()), int(d.u32()), int(d.u32()), int(d.u8())
	name := d.str()
	if err := d.end(); err != nil {
		return err
	}
	f := c.c.t.takeFile() // always, to keep descriptors in step with messages
	if w <= 0 || hgt <= 0 || w > maxSide || hgt > maxSide || stride != 4*w || slots < 1 || slots > 4 {
		if f != nil {
			f.Close()
		}
		return fmt.Errorf("remote: invalid buffers %dx%d stride %d slots %d", w, hgt, stride, slots)
	}
	ack := func() {
		e := newMsg(msgBuffersAck)
		e.u32(id)
		e.u32(gen)
		c.c.send(e.bytes())
	}
	cw := c.win(id)
	if cw == nil {
		if f != nil {
			f.Close()
		}
		ack()
		return nil
	}
	seg, err := openSegment(f, name, slots*stride*hgt)
	cw.mu.Lock()
	cw.seg.close()
	cw.seg, cw.gen, cw.segSize, cw.slots = seg, gen, image.Pt(w, hgt), slots
	cw.mu.Unlock()
	if err != nil {
		// Not fatal: on Windows the child may have closed the window
		// (and its mapping) before we got here. Its presents into this
		// generation are released unread.
		c.h.postError(err)
	}
	ack()
	return nil
}

// present copies a Present's damage out of shared memory into the
// window's retained frame and releases the slot.
func (c *Child) present(d *dec) error {
	id, gen, slot := d.u32(), d.u32(), int(d.u8())
	m := d.metrics()
	if d.err == nil && (!(m.Scale > 0) || m.Scale > 1e3 || try(func() { platform.CheckSize("present size", m.Size) }) != nil) {
		return fmt.Errorf("remote: present with invalid metrics %v", m)
	}
	phys := image.Point{}
	if d.err == nil {
		phys = m.Physical()
	}
	rects := d.rects(image.Rectangle{Max: phys})
	if err := d.end(); err != nil {
		return err
	}
	cw := c.win(id)
	if cw == nil {
		return nil
	}
	release := func() {
		e := newMsg(msgRelease)
		e.u32(id)
		e.u32(gen)
		e.u8(uint8(slot))
		c.c.send(e.bytes())
	}
	cw.mu.Lock()
	if gen != cw.gen || cw.seg == nil {
		cw.mu.Unlock()
		release()
		return nil
	}
	if phys != cw.segSize || slot >= cw.slots {
		cw.mu.Unlock()
		return fmt.Errorf("remote: present of %v in slot %d of %v buffers", phys, slot, cw.segSize)
	}
	if cw.frame == nil || cw.frame.Rect.Max != phys {
		cw.frame = image.NewRGBA(image.Rectangle{Max: phys})
		cw.full = true
	}
	stride := 4 * phys.X
	src := cw.seg.data[slot*stride*phys.Y:]
	for _, r := range rects {
		n := 4 * r.Dx()
		for y := r.Min.Y; y < r.Max.Y; y++ {
			so := y*stride + 4*r.Min.X
			do := cw.frame.PixOffset(r.Min.X, y)
			copy(cw.frame.Pix[do:do+n], src[so:so+n])
		}
	}
	cw.frameMetrics = m
	cw.damage = addDamage(cw.damage, rects)
	cw.presents++
	cw.mu.Unlock()
	release()
	c.firstOnce.Do(func() {
		close(c.firstFrame)
		if fn := c.h.opts.OnFirstFrame; fn != nil {
			c.h.post(func() { fn(c) })
		}
	})
	if !cw.notified.Swap(true) {
		c.h.post(func() { c.h.presented(cw) })
	}
	return nil
}

// addDamage accumulates damage, collapsing long lists into their bounds.
func addDamage(acc, rects []image.Rectangle) []image.Rectangle {
	acc = append(acc, rects...)
	if len(acc) <= 16 {
		return acc
	}
	u := acc[0]
	for _, r := range acc[1:] {
		u = u.Union(r)
	}
	return append(acc[:0], u)
}

// childWin is a child's window on the host.
type childWin struct {
	c    *Child
	id   uint32
	opts platform.WindowOptions

	// UI thread.
	rw        *realWin
	closed    bool
	sent      platform.Metrics // the latest ConfigureEvent sent
	wantFrame bool
	focused   bool // the child's view of the window
	imeActive bool
	held      keyboard.Held
	entered   bool
	title     string
	cursor    platform.Cursor
	textInput *platform.TextInputState

	notified atomic.Bool // a presented() call is queued

	// The reader goroutine writes these; the UI thread reads the frame
	// when presenting. Guarded by mu.
	mu           sync.Mutex
	seg          *segment
	gen          uint32
	segSize      image.Point
	slots        int
	frame        *image.RGBA      // the latest complete frame
	frameMetrics platform.Metrics // the child's metrics when it presented it
	damage       []image.Rectangle
	full         bool // frame was reallocated: present it in full
	presents     int
}
