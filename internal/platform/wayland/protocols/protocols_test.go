//go:build linux

package protocols_test

import (
	"encoding/binary"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	. "github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// fakeServer is the compositor end of a socketpair, enough to check what the
// generated code puts on the wire and how it decodes events.
type fakeServer struct {
	t   *testing.T
	fd  int
	in  []byte
	fds []int
}

var ne = binary.NativeEndian

func newFake(t *testing.T) (*wire.Conn, *fakeServer) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := wire.NewConn(fds[0])
	if err != nil {
		t.Fatal(err)
	}
	tv := unix.NsecToTimeval(int64(3 * time.Second))
	unix.SetsockoptTimeval(fds[1], unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
	s := &fakeServer{t: t, fd: fds[1]}
	t.Cleanup(func() { conn.Close(); unix.Close(s.fd) })
	return conn, s
}

type request struct {
	id     uint32
	opcode uint16
	body   []byte
}

func (s *fakeServer) next() request {
	s.t.Helper()
	for {
		if len(s.in) >= 8 {
			size := int(ne.Uint32(s.in[4:]) >> 16)
			if len(s.in) >= size {
				r := request{ne.Uint32(s.in), uint16(ne.Uint32(s.in[4:])), append([]byte(nil), s.in[8:size]...)}
				s.in = s.in[size:]
				return r
			}
		}
		buf := make([]byte, 4096)
		oob := make([]byte, unix.CmsgSpace(4*8))
		n, oobn, _, _, err := unix.Recvmsg(s.fd, buf, oob, unix.MSG_CMSG_CLOEXEC)
		if err != nil {
			s.t.Fatal(err)
		}
		s.in = append(s.in, buf[:n]...)
		if oobn > 0 {
			msgs, _ := unix.ParseSocketControlMessage(oob[:oobn])
			for i := range msgs {
				if fds, err := unix.ParseUnixRights(&msgs[i]); err == nil {
					s.fds = append(s.fds, fds...)
				}
			}
		}
	}
}

// event builds an event from 32-bit words and pre-encoded blobs.
func event(id uint32, opcode uint16, parts ...any) []byte {
	b := ne.AppendUint32(nil, id)
	b = ne.AppendUint32(b, uint32(opcode))
	for _, p := range parts {
		switch v := p.(type) {
		case uint32:
			b = ne.AppendUint32(b, v)
		case int32:
			b = ne.AppendUint32(b, uint32(v))
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
		}
	}
	ne.PutUint32(b[4:], uint32(len(b))<<16|ne.Uint32(b[4:])&0xffff)
	return b
}

func (s *fakeServer) send(b []byte) {
	s.t.Helper()
	if _, err := unix.Write(s.fd, b); err != nil {
		s.t.Fatal(err)
	}
}

func pump(t *testing.T, conn *wire.Conn, srvMessages int) {
	t.Helper()
	total := 0
	deadline := time.Now().Add(3 * time.Second)
	for total < srvMessages && time.Now().Before(deadline) {
		if _, err := conn.Read(); err != nil {
			t.Fatal(err)
		}
		n, err := conn.Dispatch()
		if err != nil {
			t.Fatal(err)
		}
		total += n
		time.Sleep(time.Millisecond)
	}
	if total < srvMessages {
		t.Fatalf("dispatched %d of %d messages", total, srvMessages)
	}
}

func TestRegistryAndBind(t *testing.T) {
	conn, srv := newFake(t)
	display := NewWlDisplay(conn)
	registry := display.GetRegistry()
	var globals []WlRegistryGlobalEvent
	registry.SetGlobalHandler(func(e WlRegistryGlobalEvent) { globals = append(globals, e) })
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}

	m := srv.next()
	if m.id != wire.DisplayID || m.opcode != 1 || ne.Uint32(m.body) != registry.ID() {
		t.Fatalf("get_registry = %+v", m)
	}

	srv.send(event(registry.ID(), 0, uint32(7), "wl_compositor", uint32(6)))
	pump(t, conn, 1)
	if len(globals) != 1 || globals[0].Name != 7 || globals[0].Interface != "wl_compositor" || globals[0].Version != 6 {
		t.Fatalf("globals = %+v", globals)
	}

	comp := WlRegistryBind[WlCompositor](registry, 7, 5)
	surface := comp.CreateSurface()
	if comp.Version() != 5 || surface.Version() != 5 {
		t.Errorf("versions = %d, %d; children must inherit the parent's version", comp.Version(), surface.Version())
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}

	bind := srv.next()
	want := event(0, 0, uint32(7), "wl_compositor", uint32(5), comp.ID())[8:]
	if bind.id != registry.ID() || bind.opcode != 0 || string(bind.body) != string(want) {
		t.Errorf("bind = %+v, want body %v", bind, want)
	}
	create := srv.next()
	if create.id != comp.ID() || create.opcode != 0 || ne.Uint32(create.body) != surface.ID() {
		t.Errorf("create_surface = %+v", create)
	}

	// Binding above what the bindings know is a programming error.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("bind above WlCompositorVersion did not panic")
			}
		}()
		WlRegistryBind[WlCompositor](registry, 7, WlCompositorVersion+1)
	}()
}

func TestShmPoolFdAndDestructor(t *testing.T) {
	conn, srv := newFake(t)
	display := NewWlDisplay(conn)
	registry := display.GetRegistry()
	shm := WlRegistryBind[WlShm](registry, 3, 1)

	fd, err := unix.MemfdCreate("pool", unix.MFD_CLOEXEC)
	if err != nil {
		t.Skip(err)
	}
	defer unix.Close(fd)
	unix.Write(fd, []byte("pixels"))

	pool := shm.CreatePool(fd, 4096)
	buf := pool.CreateBuffer(0, 16, 16, 64, WlShmFormatArgb8888)
	pool.Destroy()
	if !pool.Destroyed() {
		t.Error("pool not marked destroyed after Destroy")
	}
	if buf.Destroyed() {
		t.Error("buffer destroyed with its pool")
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}

	srv.next() // get_registry
	srv.next() // bind
	cp := srv.next()
	if cp.id != shm.ID() || cp.opcode != 0 {
		t.Fatalf("create_pool = %+v", cp)
	}
	if len(srv.fds) != 1 {
		t.Fatalf("server got %d fds", len(srv.fds))
	}
	got := make([]byte, 16)
	n, _ := unix.Pread(srv.fds[0], got, 0)
	if string(got[:n]) != "pixels" {
		t.Errorf("fd content = %q", got[:n])
	}
	if size := int32(ne.Uint32(cp.body[4:])); size != 4096 || ne.Uint32(cp.body) != pool.ID() {
		t.Errorf("create_pool body = %v", cp.body)
	}
	cb := srv.next()
	if cb.opcode != 0 || ne.Uint32(cb.body[20:]) != uint32(WlShmFormatArgb8888) {
		t.Errorf("create_buffer = %+v", cb)
	}
	if d := srv.next(); d.id != pool.ID() || d.opcode != 1 {
		t.Errorf("destroy = %+v", d)
	}

	// Using a destroyed proxy is a programming error.
	defer func() {
		if recover() == nil {
			t.Error("request on a destroyed pool did not panic")
		}
	}()
	pool.Resize(8192)
}

func TestToplevelConfigureEvent(t *testing.T) {
	conn, srv := newFake(t)
	display := NewWlDisplay(conn)
	registry := display.GetRegistry()
	wm := WlRegistryBind[XdgWmBase](registry, 1, 6)
	comp := WlRegistryBind[WlCompositor](registry, 2, 6)
	xsurf := wm.GetXdgSurface(comp.CreateSurface())
	top := xsurf.GetToplevel()
	top.SetTitle("héllo")

	var cfg XdgToplevelConfigureEvent
	top.SetConfigureHandler(func(e XdgToplevelConfigureEvent) { cfg = e })
	closed := false
	top.SetCloseHandler(func(XdgToplevelCloseEvent) { closed = true })
	var serial uint32
	xsurf.SetConfigureHandler(func(e XdgSurfaceConfigureEvent) { serial = e.Serial })
	var pinged uint32
	wm.SetPingHandler(func(e XdgWmBasePingEvent) { pinged = e.Serial })

	states := ne.AppendUint32(ne.AppendUint32(nil, uint32(XdgToplevelStateActivated)), uint32(XdgToplevelStateMaximized))
	srv.send(event(top.ID(), 0, int32(800), int32(600), states))
	srv.send(event(xsurf.ID(), 0, uint32(41)))
	srv.send(event(wm.ID(), 0, uint32(42)))
	srv.send(event(top.ID(), 1))
	pump(t, conn, 4)

	if cfg.Width != 800 || cfg.Height != 600 {
		t.Errorf("configure = %+v", cfg)
	}
	var list []XdgToplevelState
	for _, s := range wire.Uint32s(cfg.States) {
		list = append(list, XdgToplevelState(s))
	}
	if len(list) != 2 || list[0] != XdgToplevelStateActivated || list[1] != XdgToplevelStateMaximized {
		t.Errorf("states = %v", list)
	}
	if serial != 41 || pinged != 42 || !closed {
		t.Errorf("serial=%d pinged=%d closed=%v", serial, pinged, closed)
	}

	xsurf.AckConfigure(serial)
	wm.Pong(pinged)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ { // registry, 2 binds, create_surface, get_xdg_surface, get_toplevel, set_title, ack, pong
		srv.next()
	}
}

func TestEnums(t *testing.T) {
	if got := XdgToplevelStateMaximized.String(); got != "maximized" {
		t.Errorf("String() = %q", got)
	}
	if got := XdgToplevelState(999).String(); got != "XdgToplevelState(999)" {
		t.Errorf("unknown String() = %q", got)
	}
	f := WlSeatCapabilityPointer | WlSeatCapabilityKeyboard
	if !f.Has(WlSeatCapabilityPointer) || f.Has(WlSeatCapabilityTouch) {
		t.Errorf("Has on %v", f)
	}
	if f.String() != "pointer|keyboard" {
		t.Errorf("bitfield String() = %q", f.String())
	}
	if WlSeatCapability(0).String() != "0" {
		t.Errorf("zero bitfield = %q", WlSeatCapability(0).String())
	}
}

func TestFixedEvents(t *testing.T) {
	conn, srv := newFake(t)
	display := NewWlDisplay(conn)
	registry := display.GetRegistry()
	seat := WlRegistryBind[WlSeat](registry, 1, 7)
	ptr := seat.GetPointer()
	surface := WlRegistryBind[WlCompositor](registry, 2, 6).CreateSurface()

	var enter WlPointerEnterEvent
	ptr.SetEnterHandler(func(e WlPointerEnterEvent) { enter = e })
	var motion WlPointerMotionEvent
	ptr.SetMotionHandler(func(e WlPointerMotionEvent) { motion = e })

	srv.send(event(ptr.ID(), 0, uint32(9), surface.ID(), int32(-384), int32(640)))
	srv.send(event(ptr.ID(), 2, uint32(1000), int32(-25), int32(3200)))
	pump(t, conn, 2)
	if enter.Serial != 9 || enter.Surface != surface || enter.SurfaceX != -1.5 || enter.SurfaceY != 2.5 {
		t.Errorf("enter = %+v", enter)
	}
	if motion.Time != 1000 || motion.SurfaceX != -25.0/256 || motion.SurfaceY != 12.5 {
		t.Errorf("motion = %+v", motion)
	}
}
