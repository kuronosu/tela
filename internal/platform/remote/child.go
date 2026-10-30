package remote

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// Environment variables a host's supervisor sets for its child process
// (DialEnv reads them).
const (
	// EnvAddr is the host's socket path.
	EnvAddr = "TELA_REMOTE"
	// EnvTag is the child's tag (DialOptions.Tag).
	EnvTag = "TELA_REMOTE_TAG"
)

// DialOptions configures Dial.
type DialOptions struct {
	// Tag is a free-form label the host reports as [Child.Tag] (tela dev
	// uses the build generation), so a supervisor can tell its children
	// apart beyond their process IDs.
	Tag string
	// Timeout bounds the connection and the handshake. Zero means 10 s.
	Timeout time.Duration
}

// Backend is the child side: a platform.Backend whose windows live in a
// host process. Create it with Dial. See the package documentation.
type Backend struct {
	c      *conn
	ctl    *ctl
	start  time.Time
	offset time.Duration // host clock = time.Since(start) + offset
	// offsetErr bounds the error of offset: half the round trip of the
	// handshake (the host read its clock somewhere between the send and
	// the receive). Tens of µs on an idle machine, milliseconds when the
	// machine is loaded; the tests use it as the tolerance of Now.
	offsetErr time.Duration
	// frameTime is the latest FrameEvent.Time from the host: Now never
	// returns less, since the estimate of offset may be up to offsetErr
	// behind the host's clock that stamped it (the platform contract
	// keeps Now at or after the last frame time).
	frameTime atomic.Int64
	host      string // the host backend's name

	// notify wakes a blocked Run. Buffer 1, non-blocking sends.
	notify chan struct{}
	// ackedCh wakes Close's wait for acknowledgments (waitAcks). Buffer
	// 1, non-blocking sends.
	ackedCh chan struct{}

	mu       sync.Mutex // the loop's lock (loop.Options.Lock) and everything below
	loop     *loop.Loop[*Window]
	settings platform.SystemSettings
	windows  map[uint32]*Window
	order    []*Window          // open windows in creation order
	closing  map[uint32]*Window // closed, with buffers the host has not acknowledged
	nextWin  uint32
	clip     map[uint32]func(string, error)
	nextClip uint32
	// dialogs are the desktop dialogs waiting for the host's answer
	// (services.go), by token.
	dialogs    map[uint32]func(*dec, error)
	nextDialog uint32
	handlers   map[string]func([]byte) ([]byte, error)
	ran        bool
	lost       error // why the connection ended; Run returns it
}

var _ platform.Backend = (*Backend)(nil)

// DialEnv dials the host named by $TELA_REMOTE with the tag in
// $TELA_REMOTE_TAG. Without $TELA_REMOTE the error wraps
// platform.ErrUnavailable.
func DialEnv() (*Backend, error) {
	addr := os.Getenv(EnvAddr)
	if addr == "" {
		return nil, fmt.Errorf("remote: %s is not set: %w", EnvAddr, platform.ErrUnavailable)
	}
	return Dial(addr, DialOptions{Tag: os.Getenv(EnvTag)})
}

// Dial connects to the host listening at addr (a socket path, [Host.Addr])
// and performs the handshake: protocol version, clock offset, initial
// system settings. Events, frames and control messages may flow as soon as
// it returns; they are delivered once Run is called.
func Dial(addr string, opts DialOptions) (*Backend, error) {
	if !supported {
		return nil, fmt.Errorf("remote: no shared memory on this system: %w", platform.ErrUnavailable)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	t, err := dialUnix(addr, opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("remote: dial %s: %w", addr, err)
	}
	r := newFrameReader(t)
	start := time.Now()
	t.SetReadDeadline(start.Add(opts.Timeout))
	e := newMsg(msgHello)
	e.b = append(e.b, magic...)
	e.u16(ProtocolVersion)
	e.u32(uint32(os.Getpid()))
	e.str(opts.Tag)
	sent := time.Since(start)
	if err := t.write(e.bytes(), nil); err != nil {
		t.Close()
		return nil, fmt.Errorf("remote: handshake: %w", err)
	}
	typ, p, err := r.next()
	got := time.Since(start)
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("remote: handshake: %w", err)
	}
	t.SetReadDeadline(time.Time{})
	d := &dec{b: p}
	switch typ {
	case msgReject:
		reason := d.str()
		t.Close()
		return nil, fmt.Errorf("remote: the host refused the connection: %s", reason)
	case msgWelcome:
	default:
		t.Close()
		return nil, fmt.Errorf("remote: handshake: unexpected message %d", typ)
	}
	version := d.u16()
	hostNow := d.dur()
	settings := d.settings()
	name := d.str()
	if err := d.end(); err != nil || version != ProtocolVersion {
		t.Close()
		return nil, fmt.Errorf("remote: handshake: bad welcome (version %d, want %d): %v", version, ProtocolVersion, err)
	}

	b := &Backend{
		start: start,
		// The host read its clock between our send and our receive; the
		// midpoint bounds the error by half the round trip (tens of µs
		// on one machine). Both sides read the same monotonic clock, so
		// the offset does not drift.
		offset:    hostNow - (sent+got)/2,
		offsetErr: (got - sent) / 2,
		host:      name,
		notify:    make(chan struct{}, 1),
		ackedCh:   make(chan struct{}, 1),
		settings:  settings,
		windows:   map[uint32]*Window{},
		closing:   map[uint32]*Window{},
		clip:      map[uint32]func(string, error){},
		dialogs:   map[uint32]func(*dec, error){},
		handlers:  map[string]func([]byte) ([]byte, error){},
	}
	b.c = newConn(t, r)
	b.ctl = newCtl(b.c)
	b.loop = loop.New(loop.Options[*Window]{
		Now:    b.Now,
		Notify: b.signal,
		Lock:   &b.mu,
	})
	go b.readLoop()
	return b, nil
}

// Name returns "remote".
func (b *Backend) Name() string { return "remote" }

// HostBackend returns the name of the host's backend ("wayland", "win32",
// "headless", ...).
func (b *Backend) HostBackend() string { return b.host }

// Now returns the host backend's clock: the time base of every event the
// host forwards. Safe from any goroutine.
func (b *Backend) Now() time.Duration {
	now := time.Since(b.start) + b.offset
	if f := time.Duration(b.frameTime.Load()); now < f {
		return f
	}
	return now
}

// Wake makes the loop deliver a WakeEvent soon. Safe from any goroutine;
// local to this process.
func (b *Backend) Wake() { b.loop.Wake() }

// Quit makes Run return nil after the current handler call. Safe from any
// goroutine.
func (b *Backend) Quit() { b.loop.Quit() }

// Settings returns the host's system settings, as of the latest
// SettingsEvent received.
func (b *Backend) Settings() platform.SystemSettings {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.settings
}

// Clipboard returns the host's clipboard.
func (b *Backend) Clipboard() platform.Clipboard { return clipboard{b} }

func (b *Backend) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

// Run delivers events to h until Quit (nil) or until the connection to the
// host is lost (an error). Events received before the loss are delivered
// first. When Run returns the connection is closed.
func (b *Backend) Run(h platform.Handler) error {
	if h == nil {
		panic("remote: Run called with a nil Handler")
	}
	b.mu.Lock()
	if b.ran {
		b.mu.Unlock()
		panic("remote: Run called twice; a Backend runs one event loop")
	}
	b.ran = true
	b.mu.Unlock()
	defer b.Close()
	// The host learns that what the app set up before its loop (windows,
	// their visibility) is all sent: a child whose windows are all hidden
	// is then ready to take over without a frame.
	b.c.send(newMsg(msgRunning).bytes())
	for {
		if b.loop.Quitting() {
			return nil
		}
		if b.loop.Deliver(h) > 0 {
			continue
		}
		b.mu.Lock()
		ready := b.loop.Ready()
		lost := b.lost
		b.mu.Unlock()
		if ready {
			continue
		}
		if lost != nil {
			return lost
		}
		<-b.notify
	}
}

// Close closes the connection and releases the shared buffers. Run calls
// it when it returns; call it directly only to abandon a Backend that
// never ran.
func (b *Backend) Close() error {
	// What the app sent last (a window's close, a notification) leaves
	// before the connection closes.
	deadline := time.Now().Add(closeTimeout)
	b.c.flush(closeTimeout)
	// Then the host acknowledges the segments it has not mapped yet: on
	// Windows it opens a segment by name, and a mapping whose last handle
	// closes is gone, so releasing it before the host's acknowledgment
	// made a Present right before the end fail on the host
	// ("OpenFileMapping …: The system cannot find the file specified").
	b.waitAcks(deadline)
	b.c.fail(ErrClosed)
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, w := range b.windows {
		w.releaseBuffers()
	}
	for _, w := range b.closing {
		w.releaseBuffers()
	}
	clear(b.closing)
	b.loop.Discard()
	return nil
}

// closeTimeout bounds how long Close waits for the host to read what was
// sent and to acknowledge the segments it has not mapped yet.
const closeTimeout = time.Second

// waitAcks waits until the host has acknowledged the latest segment of
// every window, open or closed, until the connection ends or until
// deadline.
func (b *Backend) waitAcks(deadline time.Time) {
	t := time.NewTimer(time.Until(deadline))
	defer t.Stop()
	for b.unacked() {
		select {
		case <-b.ackedCh:
		case <-b.c.done:
			return
		case <-t.C:
			return
		}
	}
}

// unacked reports whether a window has a segment the host has not
// acknowledged.
func (b *Backend) unacked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.closing) > 0 {
		return true
	}
	for _, w := range b.windows {
		if w.seg != nil && w.ackedGen < w.gen {
			return true
		}
	}
	return false
}

// readLoop is the reader goroutine.
func (b *Backend) readLoop() {
	var err error
	for {
		t, p, e := b.c.r.next()
		if e != nil {
			err = e
			break
		}
		if e := b.dispatch(t, &dec{b: p}); e != nil {
			err = e
			break
		}
	}
	b.c.fail(err)
	lost := fmt.Errorf("remote: connection to the host lost: %w", b.c.Err())
	b.ctl.fail(lost)
	b.mu.Lock()
	if b.lost == nil {
		b.lost = lost
	}
	b.mu.Unlock()
	b.signal()
}

// dispatch handles one message from the host, on the reader goroutine.
func (b *Backend) dispatch(t msgType, d *dec) error {
	switch t {
	case msgEvent:
		id := d.u32()
		ev := d.event()
		if err := d.end(); err != nil {
			return err
		}
		return b.event(id, ev)
	case msgRelease:
		id, gen, slot := d.u32(), d.u32(), int(d.u8())
		if err := d.end(); err != nil {
			return err
		}
		b.mu.Lock()
		if w := b.windows[id]; w != nil && gen == w.gen && slot < len(w.busy) {
			w.busy[slot] = false
			select {
			case w.released <- struct{}{}:
			default:
			}
		}
		b.mu.Unlock()
	case msgBuffersAck:
		id, gen := d.u32(), d.u32()
		if err := d.end(); err != nil {
			return err
		}
		b.mu.Lock()
		if w := b.windows[id]; w != nil {
			w.acked(gen)
		} else if w := b.closing[id]; w != nil {
			w.acked(gen)
			if gen >= w.gen {
				w.releaseBuffers()
				delete(b.closing, id)
			}
		}
		b.mu.Unlock()
		select {
		case b.ackedCh <- struct{}{}:
		default:
		}
	case msgClipReply:
		id, text, msg, failed := d.u32(), d.str(), d.str(), d.boolean()
		if err := d.end(); err != nil {
			return err
		}
		var err error
		if failed {
			err = errors.New(msg)
		}
		b.mu.Lock()
		done := b.clip[id]
		delete(b.clip, id)
		if done != nil {
			b.loop.Post(func() { done(text, err) })
		}
		b.mu.Unlock()
		b.signal()
	case msgWindowInfo:
		return b.windowInfo(d)
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
		b.mu.Lock()
		if in.kind == kindDesktopDone && in.id == 0 {
			// A dialog's answer (services.go): done runs on the UI
			// thread through the loop.
			b.loop.Post(func() { b.desktopDone(in.payload) })
			b.mu.Unlock()
			b.signal()
			return nil
		}
		b.loop.Post(func() {
			b.mu.Lock()
			fn := b.handlers[in.kind]
			b.mu.Unlock()
			b.ctl.run(in, fn)
		})
		b.mu.Unlock()
		b.signal()
	case msgCtlReply:
		return b.ctl.replied(d)
	default:
		return fmt.Errorf("remote: unexpected message %d from the host", t)
	}
	return nil
}

// event queues an event from the host.
func (b *Backend) event(id uint32, ev platform.Event) error {
	if err := checkHostEvent(id, ev); err != nil {
		return err
	}
	b.mu.Lock()
	if id == 0 {
		if s, ok := ev.(platform.SettingsEvent); ok {
			b.settings = s.Settings
		}
		b.loop.Enqueue(nil, ev)
	} else if w := b.windows[id]; w != nil {
		if f, ok := ev.(platform.FrameEvent); ok {
			if int64(f.Time) > b.frameTime.Load() {
				b.frameTime.Store(int64(f.Time)) // writes are under b.mu
			}
			w.frameAsked = false
			if !w.visible {
				// The host sent it before it saw SetVisible(false): a
				// hidden window gets no FrameEvents; it is asked for again
				// when the window is shown.
				w.frameOwed = true
				b.mu.Unlock()
				return nil
			}
		}
		b.loop.Enqueue(w, ev)
	}
	b.mu.Unlock()
	b.signal()
	return nil
}

// windowInfo handles the host's msgWindowInfo: the real window's restore
// token and visibility.
func (b *Backend) windowInfo(d *dec) error {
	id, hidden, token := d.u32(), d.boolean(), d.str()
	if err := d.end(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if w := b.windows[id]; w != nil {
		w.token = token
		if !w.gotInfo {
			w.gotInfo, w.hostHidden = true, hidden
			close(w.info)
		}
	}
	return nil
}

// checkHostEvent rejects events the host never sends (a corrupt stream
// must not crash the child in Metrics.Physical).
func checkHostEvent(id uint32, ev platform.Event) error {
	switch e := ev.(type) {
	case platform.SettingsEvent, platform.NotificationEvent, platform.TrayEvent:
		if id != 0 {
			return fmt.Errorf("remote: app-scoped %T for window %d", ev, id)
		}
		return nil
	case platform.WakeEvent:
		return errors.New("remote: the host sent a WakeEvent")
	case platform.ConfigureEvent:
		m := e.Metrics
		if !(m.Scale > 0) || math.IsInf(m.Scale, 0) || try(func() { platform.CheckSize("ConfigureEvent size", m.Size) }) != nil {
			return fmt.Errorf("remote: invalid ConfigureEvent %v", m)
		}
	}
	if id == 0 {
		return fmt.Errorf("remote: window event %T without a window", ev)
	}
	return nil
}

// try runs f and turns a panic into an error (validation of a peer's
// values with the platform's panicking checks).
func try(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	f()
	return nil
}

// --- control channel ---

// Handle registers fn for the host's requests and notifications of the
// given kind (replacing any previous handler; nil removes it). fn runs on
// the UI thread, between events; for a request, its result is the reply
// (for a notification it is discarded). Requests whose kind has no handler
// fail with an error wrapping ErrNoHandler. Safe from any goroutine.
func (b *Backend) Handle(kind string, fn func(payload []byte) ([]byte, error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if fn == nil {
		delete(b.handlers, kind)
	} else {
		b.handlers[kind] = fn
	}
}

// Request sends a request to the host and waits for its reply, for ctx,
// or for the end of the connection. The host answers from its UI thread.
// Safe from any goroutine, including the UI thread (nothing the host does
// to answer needs this process's UI thread).
func (b *Backend) Request(ctx context.Context, kind string, payload []byte) ([]byte, error) {
	return b.ctl.request(ctx, kind, payload)
}

// Notify sends a notification to the host and returns once it is written
// to the socket, so the process may exit right after (a panic report).
// Safe from any goroutine.
func (b *Backend) Notify(kind string, payload []byte) error { return b.ctl.notify(kind, payload) }

// --- clipboard ---

type clipboard struct{ b *Backend }

func (c clipboard) ReadText(done func(string, error)) {
	if done == nil {
		panic("remote: Clipboard.ReadText called with a nil callback")
	}
	b := c.b
	b.mu.Lock()
	if err := b.lost; err != nil {
		b.loop.Post(func() { done("", err) })
		b.mu.Unlock()
		b.signal()
		return
	}
	b.nextClip++
	id := b.nextClip
	b.clip[id] = done
	b.mu.Unlock()
	e := newMsg(msgClipRead)
	e.u32(id)
	if err := b.c.send(e.bytes()); err != nil {
		b.mu.Lock()
		delete(b.clip, id)
		b.loop.Post(func() { done("", err) })
		b.mu.Unlock()
		b.signal()
	}
}

func (c clipboard) WriteText(s string) error {
	e := newMsg(msgClipWrite)
	e.str(s)
	return c.b.c.send(e.bytes())
}
