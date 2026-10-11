//go:build linux

package wayland

import (
	"encoding/binary"
	"fmt"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// This file is a scripted fake compositor on the other end of a socketpair.
// It decodes every request with the generated interface tables, answers
// the plumbing (wl_display.sync, the registry, delete_id for destructors)
// by itself, and hands every request to the test, which sends events back.

var ne = binary.NativeEndian

// fakeGlobal is a global the fake advertises. onBind runs (on the fake's
// goroutine) when the client binds it, to send the initial events.
type fakeGlobal struct {
	iface   string
	version uint32
	onBind  func(fc *fakeComp, id uint32)
}

// freq is a decoded request. args holds uint32 (uint, object, new_id),
// int32 (int, fixed raw), string, []byte (array) and fdArg values.
type freq struct {
	id    uint32
	iface string
	name  string
	args  []any
}

type fdArg int

func (r freq) String() string { return fmt.Sprintf("%s#%d.%s%v", r.iface, r.id, r.name, r.args) }

func (r freq) is(name string) bool { return r.iface+"."+r.name == name }

func (r freq) u(i int) uint32 { return r.args[i].(uint32) }
func (r freq) i(i int) int32  { return r.args[i].(int32) }

// fakeBuffer is a wl_buffer as the compositor sees it.
type fakeBuffer struct {
	fd                   int // pool memfd (owned by the fake)
	offset, w, h, stride int32
	format               uint32
}

type fakeComp struct {
	t  *testing.T
	fd int

	wmu sync.Mutex // serializes writes

	mu      sync.Mutex // guards the maps below
	ifaces  map[uint32]*wire.Interface
	latest  map[string]uint32 // interface name -> most recent object id
	pools   map[uint32]fakeBuffer
	buffers map[uint32]fakeBuffer
	fds     []int // received, not yet consumed

	globals []fakeGlobal
	reqs    chan freq
	closed  chan struct{}

	// automatic, if non-nil, makes the fake behave like a compositor on
	// its own: see autoState.
	automatic *autoState
}

func newFakeComp(t *testing.T, fd int, globals []fakeGlobal) *fakeComp {
	return startFake(t, fd, globals, nil)
}

func startFake(t *testing.T, fd int, globals []fakeGlobal, auto *autoState) *fakeComp {
	fc := &fakeComp{
		t: t, fd: fd, globals: globals, automatic: auto,
		ifaces:  map[uint32]*wire.Interface{wire.DisplayID: wire.Lookup("wl_display")},
		latest:  map[string]uint32{},
		pools:   map[uint32]fakeBuffer{},
		buffers: map[uint32]fakeBuffer{},
		reqs:    make(chan freq, 4096),
		closed:  make(chan struct{}),
	}
	go fc.serve()
	return fc
}

// event encodes an event from words, strings, arrays and fixed values.
func event(id uint32, opcode uint16, args ...any) []byte {
	b := ne.AppendUint32(nil, id)
	b = ne.AppendUint32(b, uint32(opcode))
	for _, a := range args {
		switch v := a.(type) {
		case uint32:
			b = ne.AppendUint32(b, v)
		case int32:
			b = ne.AppendUint32(b, uint32(v))
		case int:
			b = ne.AppendUint32(b, uint32(int32(v)))
		case float64: // fixed
			b = ne.AppendUint32(b, uint32(int32(wire.FixedFromFloat(v))))
		case string:
			b = ne.AppendUint32(b, uint32(len(v)+1))
			b = append(b, v...)
			b = append(b, 0)
			for len(b)%4 != 0 {
				b = append(b, 0)
			}
		case []byte:
			b = ne.AppendUint32(b, uint32(len(v)))
			b = append(b, v...)
			for len(b)%4 != 0 {
				b = append(b, 0)
			}
		default:
			panic(fmt.Sprintf("event: unsupported arg %T", a))
		}
	}
	ne.PutUint32(b[4:], uint32(len(b))<<16|uint32(opcode))
	return b
}

// send writes one event to the client. Errors (the client hung up) are
// ignored: tests check what the client received, not what the fake sent.
func (fc *fakeComp) send(id uint32, opcode uint16, args ...any) {
	fc.sendRaw(event(id, opcode, args...))
}

// closeFd closes the fake's socket; later sends are dropped.
func (fc *fakeComp) closeFd() {
	fc.wmu.Lock()
	defer fc.wmu.Unlock()
	if fc.fd >= 0 {
		unix.Close(fc.fd)
		fc.fd = -1
	}
}

// id returns the most recent object of an interface (0 if none).
func (fc *fakeComp) id(iface string) uint32 {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.latest[iface]
}

func (fc *fakeComp) buffer(id uint32) fakeBuffer {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.buffers[id]
}

// pixels reads a buffer's memory (as the compositor would).
func (fc *fakeComp) pixels(id uint32) []byte {
	b := fc.buffer(id)
	data := make([]byte, int(b.stride)*int(b.h))
	if _, err := unix.Pread(b.fd, data, int64(b.offset)); err != nil {
		fc.t.Fatalf("pread buffer %d: %v", id, err)
	}
	return data
}

func (fc *fakeComp) serve() {
	defer close(fc.closed)
	defer close(fc.reqs)
	fd := fc.fd // closeFd may reset the field; this goroutine ends at EOF
	var in []byte
	buf := make([]byte, 64*1024)
	oob := make([]byte, unix.CmsgSpace(4*28))
	for {
		n, oobn, _, _, err := unix.Recvmsg(fd, buf, oob, unix.MSG_CMSG_CLOEXEC)
		if err == unix.EINTR {
			continue
		}
		if err != nil || (n == 0 && oobn == 0) {
			return
		}
		if oobn > 0 {
			msgs, _ := unix.ParseSocketControlMessage(oob[:oobn])
			for i := range msgs {
				if fds, err := unix.ParseUnixRights(&msgs[i]); err == nil {
					fc.mu.Lock()
					fc.fds = append(fc.fds, fds...)
					fc.mu.Unlock()
				}
			}
		}
		in = append(in, buf[:n]...)
		for len(in) >= 8 {
			size := int(ne.Uint32(in[4:]) >> 16)
			if size < 8 || len(in) < size {
				break
			}
			id := ne.Uint32(in)
			op := uint16(ne.Uint32(in[4:]))
			body := in[8:size]
			in = in[size:]
			fc.handle(id, op, body)
		}
	}
}

func (fc *fakeComp) handle(id uint32, op uint16, body []byte) {
	fc.mu.Lock()
	iface := fc.ifaces[id]
	fc.mu.Unlock()
	if iface == nil {
		fc.t.Errorf("fake: request %d on unknown object %d", op, id)
		return
	}
	if int(op) >= len(iface.Requests) {
		fc.t.Errorf("fake: unknown request %d on %s", op, iface.Name)
		return
	}
	m := iface.Requests[op]
	r := freq{id: id, iface: iface.Name, name: m.Name}
	off := 0
	word := func() uint32 { v := ne.Uint32(body[off:]); off += 4; return v }
	blob := func() []byte {
		n := int(word())
		v := body[off : off+n]
		off += (n + 3) &^ 3
		return v
	}
	for _, a := range m.Args {
		switch a.Type {
		case wire.ArgInt, wire.ArgFixed:
			r.args = append(r.args, int32(word()))
		case wire.ArgUint, wire.ArgObject:
			r.args = append(r.args, word())
		case wire.ArgString:
			s := blob()
			r.args = append(r.args, strings.TrimSuffix(string(s), "\x00"))
		case wire.ArgArray:
			r.args = append(r.args, append([]byte(nil), blob()...))
		case wire.ArgNewID:
			name := a.Interface
			if name == "" { // wl_registry.bind: interface, version, id
				s := blob()
				name = strings.TrimSuffix(string(s), "\x00")
				r.args = append(r.args, name, word())
			}
			nid := word()
			r.args = append(r.args, nid)
			fc.mu.Lock()
			fc.ifaces[nid] = wire.Lookup(name)
			fc.latest[name] = nid
			fc.mu.Unlock()
		case wire.ArgFd:
			fc.mu.Lock()
			fd := -1
			if len(fc.fds) > 0 {
				fd, fc.fds = fc.fds[0], fc.fds[1:]
			}
			fc.mu.Unlock()
			r.args = append(r.args, fdArg(fd))
		}
	}
	fc.auto(r)
	if fc.automatic != nil {
		fc.automatic.handle(fc, r)
	}
	if m.Destructor {
		fc.mu.Lock()
		delete(fc.ifaces, id)
		fc.mu.Unlock()
		fc.send(wire.DisplayID, 1, id) // delete_id
	}
	if fc.automatic != nil {
		select { // nobody reads requests in automatic mode
		case fc.reqs <- r:
		default:
		}
		return
	}
	fc.reqs <- r
}

// autoState drives a fake that needs no script: it configures each
// toplevel (0x0, activated) at its initial commit, releases the previous
// buffer when a new one is committed, and fires frame callbacks 8 ms after
// the commit that requested them, like a 120 Hz display. Only the fake's
// goroutine uses it (timers only send).
type autoState struct {
	xdgOf      map[uint32]uint32 // wl_surface -> xdg_surface
	topOf      map[uint32]uint32 // xdg_surface -> xdg_toplevel
	configured map[uint32]bool
	attached   map[uint32]uint32 // wl_surface -> buffer attached, not committed
	current    map[uint32]uint32 // wl_surface -> buffer shown
	callbacks  map[uint32][]uint32
	serial     uint32
}

func newAutoState() *autoState {
	return &autoState{
		xdgOf: map[uint32]uint32{}, topOf: map[uint32]uint32{}, configured: map[uint32]bool{},
		attached: map[uint32]uint32{}, current: map[uint32]uint32{}, callbacks: map[uint32][]uint32{},
	}
}

func (a *autoState) handle(fc *fakeComp, r freq) {
	switch {
	case r.is("xdg_wm_base.get_xdg_surface"):
		a.xdgOf[r.u(1)] = r.u(0)
	case r.is("xdg_surface.get_toplevel"):
		a.topOf[r.id] = r.u(0)
	case r.is("wl_surface.frame"):
		a.callbacks[r.id] = append(a.callbacks[r.id], r.u(0))
	case r.is("wl_surface.attach"):
		a.attached[r.id] = r.u(0)
	case r.is("wl_surface.destroy"):
		delete(a.xdgOf, r.id)
	case r.is("wl_surface.commit"):
		s := r.id
		xs := a.xdgOf[s]
		if top := a.topOf[xs]; top != 0 && !a.configured[s] {
			a.configured[s] = true
			a.serial++
			fc.send(top, 0, int32(0), int32(0), ne.AppendUint32(nil, uint32(protocols.XdgToplevelStateActivated)))
			fc.send(xs, 0, a.serial)
		}
		if buf, ok := a.attached[s]; ok {
			delete(a.attached, s)
			if prev := a.current[s]; prev != 0 && prev != buf {
				fc.send(prev, 0) // wl_buffer.release
			}
			a.current[s] = buf
		}
		for _, cb := range a.callbacks[s] {
			time.AfterFunc(8*time.Millisecond, func() {
				fc.send(cb, 0, uint32(0))
				fc.send(wire.DisplayID, 1, cb)
			})
		}
		delete(a.callbacks, s)
	}
}

// auto answers the requests every test needs answered the same way.
func (fc *fakeComp) auto(r freq) {
	switch {
	case r.is("wl_display.sync"):
		cb := r.u(0)
		fc.send(cb, 0, uint32(0))
		fc.mu.Lock()
		delete(fc.ifaces, cb)
		fc.mu.Unlock()
		fc.send(wire.DisplayID, 1, cb)
	case r.is("wl_display.get_registry"):
		for i, g := range fc.globals {
			fc.send(r.u(0), 0, uint32(i+1), g.iface, g.version)
		}
	case r.is("wl_registry.bind"):
		g := fc.globals[r.u(0)-1]
		if g.onBind != nil {
			g.onBind(fc, r.u(3))
		}
	case r.is("wl_shm.create_pool"):
		fc.mu.Lock()
		fc.pools[r.u(0)] = fakeBuffer{fd: int(r.args[1].(fdArg))}
		fc.mu.Unlock()
	case r.is("wl_shm_pool.create_buffer"):
		fc.mu.Lock()
		p := fc.pools[r.id]
		fc.buffers[r.u(0)] = fakeBuffer{fd: p.fd, offset: r.i(1), w: r.i(2), h: r.i(3), stride: r.i(4), format: r.u(5)}
		fc.mu.Unlock()
	}
}

// next returns the next request, failing the test after a timeout.
func (fc *fakeComp) next() freq {
	fc.t.Helper()
	select {
	case r, ok := <-fc.reqs:
		if !ok {
			fc.t.Fatalf("fake: client disconnected")
		}
		return r
	case <-time.After(3 * time.Second):
		fc.t.Fatalf("fake: timed out waiting for a request")
	}
	return freq{}
}

// until returns the requests up to and including the first one called
// name ("iface.request").
func (fc *fakeComp) until(name string) []freq {
	fc.t.Helper()
	var got []freq
	for {
		r := fc.next()
		got = append(got, r)
		if r.is(name) {
			return got
		}
	}
}

// quiet fails if a request named name arrives within d. Other requests are
// returned.
func (fc *fakeComp) quiet(d time.Duration, name string) []freq {
	fc.t.Helper()
	var got []freq
	deadline := time.After(d)
	for {
		select {
		case r, ok := <-fc.reqs:
			if !ok {
				return got
			}
			if r.is(name) {
				fc.t.Errorf("unexpected %v", r)
			}
			got = append(got, r)
		case <-deadline:
			return got
		}
	}
}

func names(rs []freq) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.iface+"."+r.name)
	}
	return out
}

func find(rs []freq, name string) (freq, int) {
	for i, r := range rs {
		if r.is(name) {
			return r, i
		}
	}
	return freq{}, -1
}

// standardGlobals is a compositor like Hyprland: wl_compositor 6, XBGR and
// XRGB shm formats, one output at outputScale, one seat with a pointer,
// cursor-shape and xdg-decoration.
func standardGlobals(outputScale int32, formats ...protocols.WlShmFormat) []fakeGlobal {
	if formats == nil {
		formats = []protocols.WlShmFormat{protocols.WlShmFormatArgb8888, protocols.WlShmFormatXrgb8888}
	}
	return []fakeGlobal{
		{iface: "wl_compositor", version: 6},
		{iface: "wl_shm", version: 1, onBind: func(fc *fakeComp, id uint32) {
			for _, f := range formats {
				fc.send(id, 0, uint32(f))
			}
		}},
		{iface: "xdg_wm_base", version: 6},
		{iface: "wl_seat", version: 8, onBind: func(fc *fakeComp, id uint32) {
			fc.send(id, 0, uint32(protocols.WlSeatCapabilityPointer)) // capabilities
		}},
		{iface: "wl_output", version: 4, onBind: func(fc *fakeComp, id uint32) {
			fc.send(id, 3, outputScale) // scale
			fc.send(id, 2)              // done
		}},
		{iface: "wp_cursor_shape_manager_v1", version: 1},
		{iface: "zxdg_decoration_manager_v1", version: 1},
	}
}

// record is an event delivered to the test handler.
type record struct {
	w  platform.Window
	ev platform.Event
}

// harness runs a backend connected to a fake compositor. Run executes on
// its own goroutine, which acts as the UI thread: the test reaches it with
// ui, which runs a function from the handler (on a WakeEvent).
type harness struct {
	t      *testing.T
	b      *Backend
	fc     *fakeComp
	events chan record
	calls  chan func()
	done   chan error
	// onFrame, if set, runs on the UI thread for every FrameEvent
	// (before the event is recorded).
	onFrame func(w platform.Window, e platform.FrameEvent)
}

func newHarness(t *testing.T, globals []fakeGlobal) *harness {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := wire.NewConn(fds[0])
	if err != nil {
		t.Fatal(err)
	}
	fc := newFakeComp(t, fds[1], globals)
	b, err := newBackend(conn)
	if err != nil {
		t.Fatalf("newBackend: %v", err)
	}
	h := &harness{t: t, b: b, fc: fc, events: make(chan record, 4096), calls: make(chan func(), 64), done: make(chan error, 1)}
	t.Cleanup(func() {
		b.Quit()
		select {
		case <-h.done:
		case <-time.After(3 * time.Second):
			t.Errorf("Run did not return after Quit")
		}
		<-fc.closed
		fc.closeFd()
	})
	return h
}

// start runs the event loop.
func (h *harness) start() {
	go func() {
		err := h.b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
			if _, ok := e.(platform.WakeEvent); ok {
				for {
					select {
					case f := <-h.calls:
						f()
						continue
					default:
					}
					break
				}
				return
			}
			if fe, ok := e.(platform.FrameEvent); ok && h.onFrame != nil {
				h.onFrame(w, fe)
			}
			h.events <- record{w, e}
		}))
		h.done <- err
	}()
}

// ui runs f on the UI thread and waits for it.
func (h *harness) ui(f func()) {
	h.t.Helper()
	done := make(chan struct{})
	h.calls <- func() { f(); close(done) }
	h.b.Wake()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		h.t.Fatalf("ui call timed out")
	}
}

// event waits for the next handler event.
func (h *harness) event() record {
	h.t.Helper()
	select {
	case r := <-h.events:
		return r
	case <-time.After(3 * time.Second):
		h.t.Fatalf("timed out waiting for an event")
	}
	return record{}
}

// expect waits for the next event and checks its type.
func expect[T platform.Event](h *harness) T {
	h.t.Helper()
	r := h.event()
	e, ok := r.ev.(T)
	if !ok {
		var want T
		h.t.Fatalf("got event %T %+v, want %T", r.ev, r.ev, want)
	}
	return e
}

// noEvent fails if an event arrives within d.
func (h *harness) noEvent(d time.Duration) {
	h.t.Helper()
	select {
	case r := <-h.events:
		h.t.Errorf("unexpected event %T %+v", r.ev, r.ev)
	case <-time.After(d):
	}
}

// solid returns a framebuffer of the window's size filled with c.
func solid(w platform.Window, r, g, b uint8) *image.RGBA {
	fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	for i := 0; i < len(fb.Pix); i += 4 {
		fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = r, g, b, 0xff
	}
	return fb
}

// configureMsg encodes a toplevel configure sequence for the latest
// window.
func (h *harness) configureMsg(width, height int32, serial uint32, states ...protocols.XdgToplevelState) []byte {
	var arr []byte
	for _, s := range states {
		arr = ne.AppendUint32(arr, uint32(s))
	}
	b := event(h.fc.id("xdg_toplevel"), 0, width, height, arr)
	return append(b, event(h.fc.id("xdg_surface"), 0, serial)...)
}

// configure sends a configure sequence in one write, so the client
// dispatches it in one go.
func (h *harness) configure(width, height int32, serial uint32, states ...protocols.XdgToplevelState) {
	h.fc.sendRaw(h.configureMsg(width, height, serial, states...))
}

// sendRaw writes pre-encoded events in one write.
func (fc *fakeComp) sendRaw(b []byte) {
	fc.wmu.Lock()
	defer fc.wmu.Unlock()
	if fc.fd >= 0 {
		unix.Write(fc.fd, b)
	}
}

// newBackendOnFake runs only the backend setup against a fake.
func newBackendOnFake(t *testing.T, globals []fakeGlobal) (*Backend, error) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := wire.NewConn(fds[0])
	if err != nil {
		t.Fatal(err)
	}
	fc := newFakeComp(t, fds[1], globals)
	b, err := newBackend(conn)
	conn.Close()
	<-fc.closed
	fc.closeFd()
	return b, err
}
