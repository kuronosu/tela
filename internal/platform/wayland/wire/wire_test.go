//go:build linux

package wire

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFixed(t *testing.T) {
	cases := []struct {
		f    float64
		want Fixed
	}{
		{0, 0},
		{1, 256},
		{-1, -256},
		{1.5, 384},
		{-1.5, -384},
		{0.00390625, 1},   // 1/256
		{-0.00390625, -1}, // -1/256
		{0.001, 0},        // rounds to nearest
		{0.003, 1},
		{8388607.99609375, 0x7fffffff},
		{1e12, 0x7fffffff}, // saturates
		{-1e12, -0x80000000},
	}
	for _, c := range cases {
		if got := FixedFromFloat(c.f); got != c.want {
			t.Errorf("FixedFromFloat(%v) = %d, want %d", c.f, got, c.want)
		}
	}
	if got := Fixed(-384).Float64(); got != -1.5 {
		t.Errorf("Fixed(-384).Float64() = %v", got)
	}
	if got := Fixed(-384).Int(); got != -1 {
		t.Errorf("Fixed(-384).Int() = %v, want -1 (truncate toward zero)", got)
	}
	if got := FixedFromInt(-3); got != -768 {
		t.Errorf("FixedFromInt(-3) = %d", got)
	}
}

func TestEncodeArgs(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name  string
		write func(w *Writer)
		want  []byte // body only
	}{
		{"int negative", func(w *Writer) { w.Int(-2) }, newMsg(0, 0).i32(-2).b[8:]},
		{"uint max", func(w *Writer) { w.Uint(0xffffffff) }, newMsg(0, 0).u32(0xffffffff).b[8:]},
		{"fixed negative", func(w *Writer) { w.Fixed(FixedFromFloat(-1.5)) }, newMsg(0, 0).i32(-384).b[8:]},
		{"object null", func(w *Writer) { w.Object(0) }, newMsg(0, 0).u32(0).b[8:]},
		{"object id", func(w *Writer) { w.Object(7) }, newMsg(0, 0).u32(7).b[8:]},
		{"string 3 chars", func(w *Writer) { w.String("abc") }, []byte{4, 0, 0, 0, 'a', 'b', 'c', 0}},
		{"string needs padding", func(w *Writer) { w.String("abcd") },
			[]byte{5, 0, 0, 0, 'a', 'b', 'c', 'd', 0, 0, 0, 0}},
		{"string empty is one NUL", func(w *Writer) { w.String("") }, []byte{1, 0, 0, 0, 0, 0, 0, 0}},
		{"string ptr null", func(w *Writer) { w.StringPtr(nil) }, []byte{0, 0, 0, 0}},
		{"string ptr empty", func(w *Writer) { w.StringPtr(str("")) }, []byte{1, 0, 0, 0, 0, 0, 0, 0}},
		{"string ptr value", func(w *Writer) { w.StringPtr(str("hi")) }, []byte{3, 0, 0, 0, 'h', 'i', 0, 0}},
		{"string utf8", func(w *Writer) { w.String("é") }, []byte{3, 0, 0, 0, 0xc3, 0xa9, 0, 0}},
		{"array", func(w *Writer) { w.Array([]byte{1, 2, 3, 4, 5}) },
			[]byte{5, 0, 0, 0, 1, 2, 3, 4, 5, 0, 0, 0}},
		{"array aligned", func(w *Writer) { w.Array([]byte{1, 2, 3, 4}) }, []byte{4, 0, 0, 0, 1, 2, 3, 4}},
		{"array empty", func(w *Writer) { w.Array(nil) }, []byte{0, 0, 0, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, srv := newPair(t)
			o := newTestObj(c)
			w := o.Begin(reqInts)
			tc.write(w)
			w.Done()
			if err := c.Flush(); err != nil {
				t.Fatal(err)
			}
			m := srv.next()
			if m.id != o.ID() || m.opcode != reqInts {
				t.Errorf("header = (%d, %d), want (%d, %d)", m.id, m.opcode, o.ID(), reqInts)
			}
			if !bytes.Equal(m.body, tc.want) {
				t.Errorf("body = %v, want %v", m.body, tc.want)
			}
		})
	}
}

func TestDecodeArgs(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	peer := newTestObj(c)

	type all struct {
		i     int32
		u     uint32
		f     float64
		s     string
		empty *string
		null  *string
		obj   Object
		arr   []byte
	}
	var got all
	var events int
	o.handle = func(op uint16, r *Reader) error {
		events++
		if op != evAll {
			t.Errorf("opcode = %d", op)
		}
		got.i = r.Int()
		got.u = r.Uint()
		got.f = r.Fixed().Float64()
		got.s = r.String()
		got.empty = r.StringPtr()
		got.null = r.StringPtr()
		got.obj = r.Object()
		got.arr = r.Array()
		return nil
	}

	srv.send(newMsg(o.ID(), evAll).
		i32(-5).u32(4000000000).i32(-384).str("héllo").str("").null().u32(peer.ID()).arr([]byte{9, 8, 7}).bytes())
	// Second event: null object, empty array.
	srv.send(newMsg(o.ID(), evAll).
		i32(0).u32(0).i32(0).str("").null().null().u32(0).arr(nil).bytes())
	pump(t, c, 2)

	if events != 2 {
		t.Fatalf("handler ran %d times", events)
	}
	// The first event's values were overwritten by the second; re-run the
	// first alone for the detailed assertions.
	if got.obj != nil || got.arr != nil || got.null != nil || got.empty != nil {
		t.Errorf("second event decoded to %+v, want nulls", got)
	}

	srv.send(newMsg(o.ID(), evAll).
		i32(-5).u32(4000000000).i32(-384).str("héllo").str("").null().u32(peer.ID()).arr([]byte{9, 8, 7}).bytes())
	pump(t, c, 1)
	if got.i != -5 || got.u != 4000000000 || got.f != -1.5 || got.s != "héllo" {
		t.Errorf("scalars = %+v", got)
	}
	if got.empty == nil || *got.empty != "" {
		t.Errorf("empty string decoded as %v, want pointer to \"\"", got.empty)
	}
	if got.null != nil {
		t.Errorf("null string decoded as %q, want nil", *got.null)
	}
	if got.obj != Object(peer) {
		t.Errorf("object = %v, want the live peer", got.obj)
	}
	if !bytes.Equal(got.arr, []byte{9, 8, 7}) {
		t.Errorf("array = %v", got.arr)
	}
}

func TestDecodeMalformed(t *testing.T) {
	cases := map[string][]byte{
		"truncated string":   newMsg(0, evAll).i32(1).u32(2).i32(3).u32(100).bytes(),
		"no NUL":             newMsg(0, evAll).i32(1).u32(2).i32(3).blob([]byte("abc"), false).bytes(),
		"body too short":     newMsg(0, evAll).i32(1).bytes(),
		"size below header":  {0, 0, 0, 0, 0, 0, 4, 0},
		"size not multiple4": {2, 0, 0, 0, 0, 0, 10, 0, 1, 1},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			c, srv := newPair(t)
			o := newTestObj(c)
			o.handle = func(op uint16, r *Reader) error {
				r.Int()
				r.Uint()
				r.Fixed()
				_ = r.String()
				return nil
			}
			// Patch the object id (messages were built with id 0).
			data = append([]byte(nil), data...)
			if len(data) >= 4 && name != "size not multiple4" {
				nativeEndian.PutUint32(data, o.ID())
			}
			srv.send(data)
			if err := c.wait(unix.POLLIN, time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Read(); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Dispatch(); err == nil {
				t.Fatal("Dispatch succeeded on a malformed message")
			}
			if c.Err() == nil {
				t.Error("connection is not dead after a malformed message")
			}
			if _, err := c.Dispatch(); err == nil {
				t.Error("second Dispatch succeeded on a dead connection")
			}
		})
	}
}

func TestSendFd(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)

	fd := memfd(t, "hello from the client")
	before := openFds(t)
	w := o.Begin(reqFd)
	w.Fd(fd)
	w.Done()
	// The caller may close its descriptor right away: the runtime dup'd it.
	unix.Close(fd)
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	m := srv.next()
	if m.opcode != reqFd || len(m.body) != 0 {
		t.Fatalf("message = %+v", m)
	}
	if len(srv.fds) != 1 {
		t.Fatalf("server received %d descriptors, want 1", len(srv.fds))
	}
	if got := readFd(t, srv.fds[0]); got != "hello from the client" {
		t.Errorf("descriptor content = %q", got)
	}
	// Client side: the dup was closed after sending; only the server's copy
	// is open (before counted the original, which was closed above).
	if after := openFds(t); after != before {
		t.Errorf("open descriptors: %d before, %d after (leak or double close)", before, after)
	}
}

func TestReceiveFd(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)

	var got int = -1
	o.handle = func(op uint16, r *Reader) error {
		got = r.Fd()
		return nil
	}
	fd := memfd(t, "hello from the server")
	srv.send(newMsg(o.ID(), evFd).bytes(), fd)
	unix.Close(fd)
	pump(t, c, 1)
	if got < 0 {
		t.Fatal("handler did not get a descriptor")
	}
	defer unix.Close(got)
	if s := readFd(t, got); s != "hello from the server" {
		t.Errorf("content = %q", s)
	}
	if flags, err := unix.FcntlInt(uintptr(got), unix.F_GETFD, 0); err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Errorf("received descriptor is not close-on-exec (flags %#x, err %v)", flags, err)
	}
}

func TestMissingFd(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	o.handle = func(op uint16, r *Reader) error {
		r.Fd()
		return nil
	}
	srv.send(newMsg(o.ID(), evFd).bytes()) // no ancillary data
	if err := c.wait(unix.POLLIN, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	c.Read()
	if _, err := c.Dispatch(); err == nil || !strings.Contains(err.Error(), "file descriptor") {
		t.Fatalf("Dispatch error = %v, want a missing descriptor error", err)
	}
}

// Messages arriving in pieces stay buffered until complete.
func TestSplitMessages(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	var pings []uint32
	o.handle = func(op uint16, r *Reader) error {
		pings = append(pings, r.Uint())
		return r.Err()
	}
	a := newMsg(o.ID(), evPing).u32(11).bytes()
	b := newMsg(o.ID(), evPing).u32(22).bytes()
	stream := append(append([]byte(nil), a...), b...) // 24 bytes

	step := func(chunk []byte, wantTotal int) {
		t.Helper()
		srv.send(chunk)
		deadline := time.Now().Add(time.Second)
		if err := c.wait(unix.POLLIN, deadline); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Read(); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Dispatch(); err != nil {
			t.Fatal(err)
		}
		if len(pings) != wantTotal {
			t.Fatalf("after %d bytes: %d events dispatched, want %d", len(stream), len(pings), wantTotal)
		}
	}
	step(stream[:3], 0)   // part of a header
	step(stream[3:10], 0) // header done, body incomplete
	step(stream[10:14], 1)
	step(stream[14:20], 1) // second header + part of body
	step(stream[20:], 2)
	if pings[0] != 11 || pings[1] != 22 {
		t.Errorf("pings = %v", pings)
	}

	// Two whole messages in one read.
	pings = nil
	srv.send(stream)
	pump(t, c, 2)
	if len(pings) != 2 {
		t.Errorf("pings = %v", pings)
	}
}

func TestDeleteIDReuse(t *testing.T) {
	c, srv := newPair(t)
	parent := newTestObj(c) // id 2
	a := parent.create()    // id 3
	b := parent.create()    // id 4
	if a.ID() != 3 || b.ID() != 4 {
		t.Fatalf("ids = %d, %d, want 3, 4", a.ID(), b.ID())
	}

	a.destroy()
	if !a.Destroyed() {
		t.Error("proxy not marked destroyed")
	}
	// The ID is still reserved: a new object must not take it.
	cobj := parent.create()
	if cobj.ID() != 5 {
		t.Errorf("new object got id %d before delete_id, want 5", cobj.ID())
	}

	// Server acknowledges: id 3 is free again.
	srv.send(newMsg(DisplayID, 1).u32(3).bytes())
	pump(t, c, 1)
	d := parent.create()
	if d.ID() != 3 {
		t.Errorf("new object got id %d after delete_id, want reuse of 3", d.ID())
	}

	// Several free IDs are reused last-freed first, like libwayland.
	b.destroy()
	cobj.destroy()
	srv.send(newMsg(DisplayID, 1).u32(4).bytes())
	srv.send(newMsg(DisplayID, 1).u32(5).bytes())
	pump(t, c, 2)
	if e := parent.create(); e.ID() != 5 {
		t.Errorf("got %d, want 5", e.ID())
	}
	if e := parent.create(); e.ID() != 4 {
		t.Errorf("got %d, want 4", e.ID())
	}
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteIDForLiveObject(t *testing.T) {
	// The server may delete an object that was not destroyed locally (a
	// wl_callback after done). The ID is released and the proxy marked dead.
	c, srv := newPair(t)
	o := newTestObj(c)
	srv.send(newMsg(DisplayID, 1).u32(o.ID()).bytes())
	pump(t, c, 1)
	if !o.Destroyed() {
		t.Error("proxy not marked destroyed by delete_id")
	}
	if c.Object(o.ID()) != nil {
		t.Error("object still in the map")
	}
	if n := newTestObj(c); n.ID() != o.ID() {
		t.Errorf("id %d not reused, got %d", o.ID(), n.ID())
	}
}

func TestServerCreatedObjects(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)

	var child *tobj
	var childPings []uint32
	o.handle = func(op uint16, r *Reader) error {
		if op != evChild {
			return nil
		}
		child = &tobj{handle: func(op uint16, r *Reader) error {
			childPings = append(childPings, r.Uint())
			return nil
		}}
		r.NewObject(child, o.Version())
		return nil
	}
	const sid = ServerIDStart
	srv.send(newMsg(o.ID(), evChild).u32(sid).bytes())
	srv.send(newMsg(sid, evPing).u32(5).bytes())
	pump(t, c, 2)
	if child == nil || child.ID() != sid || child.Version() != o.Version() {
		t.Fatalf("child = %+v", child)
	}
	if len(childPings) != 1 || childPings[0] != 5 {
		t.Errorf("child events = %v", childPings)
	}

	// An ID below the server range is a protocol violation.
	srv.send(newMsg(o.ID(), evChild).u32(77).bytes())
	if err := c.wait(unix.POLLIN, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	c.Read()
	if _, err := c.Dispatch(); err == nil || !strings.Contains(err.Error(), "server range") {
		t.Errorf("Dispatch error = %v", err)
	}
}

func TestUnknownObject(t *testing.T) {
	c, srv := newPair(t)
	srv.send(newMsg(99, 0).bytes())
	if err := c.wait(unix.POLLIN, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	c.Read()
	if _, err := c.Dispatch(); err == nil || !strings.Contains(err.Error(), "unknown object 99") {
		t.Errorf("Dispatch error = %v", err)
	}
}

// Events for an object destroyed client-side are dropped, but their
// descriptors are still consumed and closed, and any object they would have
// created is tracked as a zombie too.
func TestZombieEventWithFd(t *testing.T) {
	c, srv := newPair(t)
	parent := newTestObj(c)
	victim := parent.create()
	victimID := victim.ID()
	called := false
	victim.handle = func(uint16, *Reader) error { called = true; return nil }
	victim.destroy()
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	// Drain the three requests (create, destroy) on the server side.
	srv.next()
	srv.next()

	other := newTestObj(c)
	var otherPings []uint32
	other.handle = func(op uint16, r *Reader) error { otherPings = append(otherPings, r.Uint()); return nil }

	before := openFds(t)
	f1, f2 := memfd(t, "a"), memfd(t, "b")
	// An event with an fd for the zombie, then one that also creates a
	// server object, then a normal event for another object whose fd must
	// not be stolen by the zombie handling.
	srv.send(newMsg(victimID, evFd).bytes(), f1)
	childID := uint32(ServerIDStart + 1)
	srv.send(newMsg(victimID, evFdChild).u32(childID).bytes(), f2)
	srv.send(newMsg(childID, evPing).u32(1).bytes()) // event for the zombie's would-be child
	srv.send(newMsg(other.ID(), evPing).u32(42).bytes())
	unix.Close(f1)
	unix.Close(f2)
	pump(t, c, 4)

	if called {
		t.Error("handler of a destroyed object was called")
	}
	if len(otherPings) != 1 || otherPings[0] != 42 {
		t.Errorf("live object events = %v", otherPings)
	}
	if after := openFds(t); after != before {
		t.Errorf("open descriptors %d -> %d; the two zombie descriptors must be closed", before, after)
	}
	if len(c.inFds) != 0 {
		t.Errorf("%d descriptors left in the queue", len(c.inFds))
	}
}

func TestProtocolError(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	srv.send(newMsg(DisplayID, 0).u32(o.ID()).u32(3).str("bad things happened").bytes())
	// A message after the error must never be dispatched.
	dispatched := false
	o.handle = func(uint16, *Reader) error { dispatched = true; return nil }
	srv.send(newMsg(o.ID(), evPing).u32(1).bytes())
	if err := c.wait(unix.POLLIN, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	c.Read()
	_, err := c.Dispatch()
	var pe *ProtocolError
	if !errors.As(err, &pe) {
		t.Fatalf("Dispatch error = %v (%T), want *ProtocolError", err, err)
	}
	if pe.ObjectID != o.ID() || pe.Code != 3 || pe.Message != "bad things happened" || pe.Interface != "test_obj" {
		t.Errorf("error = %+v", pe)
	}
	if !strings.Contains(pe.Error(), "test_obj#2") || !strings.Contains(pe.Error(), "bad things") {
		t.Errorf("Error() = %q", pe.Error())
	}
	if dispatched {
		t.Error("event after wl_display.error was dispatched")
	}
	// The connection is dead.
	if c.Err() != err {
		t.Errorf("Err() = %v", c.Err())
	}
	if _, derr := c.Dispatch(); derr != err {
		t.Errorf("second Dispatch = %v", derr)
	}
	if _, rerr := c.Read(); rerr != err {
		t.Errorf("Read = %v", rerr)
	}
	o.Begin(reqInts).Done() // must not panic or grow the buffer
	if ferr := c.Flush(); ferr != err {
		t.Errorf("Flush = %v", ferr)
	}
}

func TestEOF(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	var pings int
	o.handle = func(uint16, *Reader) error { pings++; return nil }
	srv.send(newMsg(o.ID(), evPing).u32(1).bytes())
	unix.Close(srv.fd)
	srv.fd = -1

	if _, err := c.Read(); err != nil {
		t.Fatalf("first Read: %v", err)
	}
	if _, err := c.Read(); !errors.Is(err, io.EOF) {
		t.Fatalf("second Read = %v, want EOF", err)
	}
	if _, err := c.Dispatch(); err != nil {
		t.Fatalf("Dispatch after EOF: %v", err)
	}
	if pings != 1 {
		t.Errorf("buffered message not delivered after EOF (pings=%d)", pings)
	}
}

// A compositor that sends wl_display.error and hangs up must not hide the
// error behind the EPIPE of the next Flush.
func TestErrorSurvivesHangup(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	srv.send(newMsg(DisplayID, 0).u32(o.ID()).u32(1).str("you did it wrong").bytes())
	unix.Close(srv.fd)
	srv.fd = -1

	o.Begin(reqInts).Done()
	if err := c.Flush(); err == nil {
		t.Fatal("Flush to a closed peer succeeded")
	}
	if c.Err() != nil {
		t.Fatalf("EPIPE made the connection permanently dead (%v) before the protocol error was read", c.Err())
	}
	if err := c.RoundtripTimeout(time.Second); err == nil {
		t.Fatal("Roundtrip succeeded")
	} else {
		var pe *ProtocolError
		if !errors.As(err, &pe) || pe.Message != "you did it wrong" {
			t.Errorf("Roundtrip = %v, want the protocol error", err)
		}
	}
}

func TestVersionAndDestroyedPanics(t *testing.T) {
	c, _ := newPair(t)
	o := &tobj{}
	c.adopt(o, c.allocID(), 1) // bound at version 1; "newer" needs 2

	expectPanic := func(name, contains string, fn func()) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s: no panic", name)
				return
			}
			if s, _ := r.(string); !strings.Contains(s, contains) {
				t.Errorf("%s: panic %q does not contain %q", name, s, contains)
			}
		}()
		fn()
	}
	expectPanic("too new", "requires version 2", func() { o.Begin(reqNewer) })
	expectPanic("unbound", "not bound", func() { (&tobj{}).Begin(reqInts) })
	o.destroy()
	expectPanic("destroyed", "destroyed", func() { o.Begin(reqInts) })
}

func TestFlushWouldBlockAndFdLimit(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	// A tiny send buffer makes the socket fill up quickly.
	if err := unix.SetsockoptInt(c.Fd(), unix.SOL_SOCKET, unix.SO_SNDBUF, 4096); err != nil {
		t.Fatal(err)
	}

	devnull, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(devnull)

	// 100 messages, each with one descriptor, interleaved with padding data so
	// the socket buffer fills up while the server is not reading.
	const msgs = 100
	payload := bytes.Repeat([]byte{'x'}, 1000)
	sent := 0
	for i := 0; i < msgs; i++ {
		w := o.Begin(reqFd)
		w.Fd(devnull)
		w.Array(payload)
		w.Done()
		sent++
	}
	blocked := false
	err = c.Flush()
	if err == ErrWouldBlock {
		blocked = true
		if !c.HasPendingWrites() {
			t.Error("Flush reported would-block but nothing is pending")
		}
	} else if err != nil {
		t.Fatal(err)
	}

	// Drain on the server, checking the invariant that a message's
	// descriptor never arrives after the message itself, then flush more.
	got := 0
	for got < msgs {
		for len(srv.in) < 8 || len(srv.in) < int(nativeEndian.Uint32(srv.in[4:])>>16) {
			srv.fill()
			if blocked {
				if err := c.Flush(); err != nil && err != ErrWouldBlock {
					t.Fatal(err)
				}
			}
		}
		m := srv.next()
		got++
		if len(srv.fds) < got {
			t.Fatalf("message %d completed with only %d descriptors received: descriptors must not trail their message", got, len(srv.fds))
		}
		if m.opcode != reqFd {
			t.Fatalf("opcode %d", m.opcode)
		}
	}
	for c.HasPendingWrites() {
		if err := c.Flush(); err != nil && err != ErrWouldBlock {
			t.Fatal(err)
		}
	}
	if len(srv.fds) != msgs {
		t.Errorf("server got %d descriptors, want %d", len(srv.fds), msgs)
	}
	if !blocked {
		t.Log("socket buffer never filled; would-block path not exercised on this system")
	}
}

func TestMoreThanMaxFdsPerFlush(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	devnull, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(devnull)

	const msgs = MaxFdsPerMessage + 12
	for i := 0; i < msgs; i++ {
		w := o.Begin(reqFd)
		w.Fd(devnull)
		w.Done()
	}
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	got := 0
	for got < msgs {
		m := srv.next()
		_ = m
		got++
		if len(srv.fds) < got {
			t.Fatalf("message %d arrived before its descriptor (%d fds)", got, len(srv.fds))
		}
	}
}

func TestMessageTooLarge(t *testing.T) {
	c, _ := newPair(t)
	o := newTestObj(c)
	w := o.Begin(reqArray)
	w.Array(make([]byte, MaxMessageSize))
	w.Done()
	if c.HasPendingWrites() {
		t.Error("oversized message was queued")
	}
	if err := c.Flush(); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("Flush = %v, want a size error", err)
	}
}

// TestDroppedRequestDropsItsObject: when a request that creates an object
// cannot be encoded (here, a bad descriptor), the message is dropped, so
// the server never hears of the new ID. The object must not stay
// registered: its ID is free again, the proxy reads as destroyed, and
// requests on it (and on objects it would create) are dropped instead of
// panicking, because the caller cannot know the creation failed.
func TestDroppedRequestDropsItsObject(t *testing.T) {
	c, _ := newPair(t)
	o := newTestObj(c)

	child := &tobj{}
	w := o.Begin(reqCreate)
	w.NewObject(child, o.Version())
	w.Fd(-1) // EBADF: the message is dropped
	w.Done()

	id := child.ID()
	if c.Object(id) != nil {
		t.Errorf("the object of a dropped request is still registered as %d", id)
	}
	if !child.Destroyed() {
		t.Errorf("the proxy of a dropped request does not read as destroyed")
	}
	if c.HasPendingWrites() {
		t.Errorf("the dropped request was queued")
	}

	// Requests on it, including ones that create objects, are dropped
	// without a panic, and their objects are dropped too.
	child.Begin(reqInts).Done()
	grandchild := child.create()
	if c.Object(grandchild.ID()) != nil || !grandchild.Destroyed() {
		t.Errorf("an object created on a dropped proxy is registered")
	}
	child.destroy()
	if c.HasPendingWrites() {
		t.Errorf("requests on a dropped proxy were queued")
	}

	// The ID was never used on the wire, so it is free at once (no
	// delete_id will come for it).
	if next := c.allocID(); next != id && next != grandchild.ID() {
		t.Errorf("allocID after the drop = %d, want a freed ID (%d or %d)", next, id, grandchild.ID())
	}

	// Flush reports the error that dropped the request, not a later one.
	if err := c.Flush(); err == nil || !strings.Contains(err.Error(), "duplicate file descriptor") {
		t.Errorf("Flush = %v, want the descriptor error", err)
	}
}

func TestRoundtrip(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	var pings []uint32
	o.handle = func(op uint16, r *Reader) error { pings = append(pings, r.Uint()); return nil }

	done := make(chan struct{})
	go func() {
		defer close(done)
		m := srv.nextNoFatal()
		if m.id != DisplayID || m.opcode != 0 {
			return
		}
		cb := nativeEndian.Uint32(m.body)
		// An event before the barrier, then done + delete_id.
		srv.send(newMsg(o.ID(), evPing).u32(7).bytes())
		srv.send(newMsg(cb, 0).u32(0).bytes())
		srv.send(newMsg(DisplayID, 1).u32(cb).bytes())
	}()

	if err := c.RoundtripTimeout(3 * time.Second); err != nil {
		t.Fatalf("Roundtrip: %v", err)
	}
	<-done
	if len(pings) != 1 || pings[0] != 7 {
		t.Errorf("event before the barrier was not delivered: %v", pings)
	}
	// The callback ID is released by delete_id, which may still be in flight.
	deadline := time.Now().Add(2 * time.Second)
	for len(c.freeIDs) < 1 && time.Now().Before(deadline) {
		if err := c.wait(unix.POLLIN, deadline); err != nil {
			break
		}
		c.Read()
		if _, err := c.Dispatch(); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(c.freeIDs); n != 1 {
		t.Errorf("free IDs = %d, want 1 (callback)", n)
	}
}

// nextNoFatal is next for goroutines: it returns a zero message on error
// instead of calling t.Fatal off the test goroutine.
func (s *server) nextNoFatal() rawMsg {
	for {
		if len(s.in) >= 8 {
			size := int(nativeEndian.Uint32(s.in[4:]) >> 16)
			if len(s.in) >= size {
				m := rawMsg{
					id:     nativeEndian.Uint32(s.in),
					opcode: uint16(nativeEndian.Uint32(s.in[4:])),
					body:   append([]byte(nil), s.in[8:size]...),
				}
				s.in = s.in[size:]
				return m
			}
		}
		buf := make([]byte, 4096)
		n, err := unix.Read(s.fd, buf)
		if err != nil || n == 0 {
			return rawMsg{}
		}
		s.in = append(s.in, buf[:n]...)
	}
}

func TestRoundtripTimeoutAndError(t *testing.T) {
	c, _ := newPair(t) // the server never answers
	start := time.Now()
	err := c.RoundtripTimeout(100 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("RoundtripTimeout = %v, want timeout", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout took too long")
	}

	c2, srv2 := newPair(t)
	go func() {
		m := srv2.nextNoFatal()
		_ = m
		srv2.send(newMsg(DisplayID, 0).u32(DisplayID).u32(1).str("nope").bytes())
	}()
	var pe *ProtocolError
	if err := c2.RoundtripTimeout(3 * time.Second); !errors.As(err, &pe) {
		t.Errorf("Roundtrip = %v, want protocol error", err)
	}
}

func TestDialSocketPath(t *testing.T) {
	dir := t.TempDir()
	if len(dir) > 80 {
		dir, _ = os.MkdirTemp("/tmp", "wire")
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_SOCKET", "")
	t.Setenv("WAYLAND_DISPLAY", "wl-test")
	ln, err := net.Listen("unix", filepath.Join(dir, "wl-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	c, err := Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	srvConn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer srvConn.Close()

	// An absolute WAYLAND_DISPLAY ignores XDG_RUNTIME_DIR.
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("WAYLAND_DISPLAY", filepath.Join(dir, "wl-test"))
	c2, err := Dial()
	if err != nil {
		t.Fatalf("absolute WAYLAND_DISPLAY: %v", err)
	}
	c2.Close()
}

func TestDialErrors(t *testing.T) {
	t.Setenv("WAYLAND_SOCKET", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	if _, err := Dial(); err == nil || !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Errorf("no runtime dir: %v", err)
	}

	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	_, err := Dial()
	if err == nil || !strings.Contains(err.Error(), "wayland-0") {
		t.Errorf("default display name not used or not reported: %v", err)
	}

	t.Setenv("WAYLAND_SOCKET", "banana")
	if _, err := Dial(); err == nil || !strings.Contains(err.Error(), "WAYLAND_SOCKET") {
		t.Errorf("bad WAYLAND_SOCKET: %v", err)
	}

	t.Setenv("WAYLAND_SOCKET", "")
	t.Setenv("WAYLAND_DISPLAY", strings.Repeat("a", 200))
	t.Setenv("XDG_RUNTIME_DIR", "/tmp")
	if _, err := Dial(); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Errorf("long path: %v", err)
	}
}

func TestDialInheritedSocket(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	t.Setenv("WAYLAND_SOCKET", strconv.Itoa(fds[0]))
	c, err := Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Fd() != fds[0] {
		t.Errorf("Fd() = %d, want the inherited %d", c.Fd(), fds[0])
	}
	if os.Getenv("WAYLAND_SOCKET") != "" {
		t.Error("WAYLAND_SOCKET was not unset")
	}
}

func TestUntypedBindEncoding(t *testing.T) {
	c, srv := newPair(t)
	o := newTestObj(c)
	target := &tobj{}
	w := o.Begin(reqInts)
	w.Uint(9) // name
	w.NewObjectUntyped(target, 2)
	w.Done()
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}
	m := srv.next()
	want := newMsg(0, 0).u32(9).str("test_obj").u32(2).u32(target.ID()).b[8:]
	if !bytes.Equal(m.body, want) {
		t.Errorf("body = %v, want %v", m.body, want)
	}
	if target.Version() != 2 {
		t.Errorf("version = %d", target.Version())
	}
	defer func() {
		if recover() == nil {
			t.Error("binding above the known version did not panic")
		}
	}()
	o.Begin(reqInts)
	c.writer.NewObjectUntyped(&tobj{}, 99)
}
