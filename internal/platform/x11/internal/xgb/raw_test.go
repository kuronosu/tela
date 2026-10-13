// Tests of Tela's patch to the vendored reader (see raw.go).

package xgb_test

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

var le = binary.LittleEndian

// TestMain silences xgb's logger once: assigning it per test races with the
// reader goroutines of earlier tests' connections.
func TestMain(m *testing.M) {
	xgb.Logger = log.New(io.Discard, "", 0)
	os.Exit(m.Run())
}

// fakeServer accepts the connection setup on the server end of a pipe and
// returns that end.
func fakeServer(t *testing.T) (*xgb.Conn, net.Conn) {
	t.Helper()
	t.Setenv("XAUTHORITY", t.TempDir()+"/none") // no cookie: a predictable setup request
	client, server := net.Pipe()
	type result struct {
		c   *xgb.Conn
		err error
	}
	done := make(chan result, 1)
	go func() {
		c, err := xgb.NewConnNet(client)
		done <- result{c, err}
	}()
	req := make([]byte, 12)
	if _, err := io.ReadFull(server, req); err != nil {
		t.Fatal(err)
	}
	pad := func(n int) int { return (n + 3) &^ 3 }
	auth := make([]byte, pad(int(le.Uint16(req[6:])))+pad(int(le.Uint16(req[8:]))))
	if _, err := io.ReadFull(server, auth); err != nil {
		t.Fatal(err)
	}
	// Success, protocol 11.0, 8 words of setup data: only the resource
	// id base and mask are read by the connection.
	reply := make([]byte, 8+32)
	reply[0] = 1
	le.PutUint16(reply[2:], 11)
	le.PutUint16(reply[6:], 8)
	le.PutUint32(reply[12:], 0x200000)
	le.PutUint32(reply[16:], 0x1fffff)
	if _, err := server.Write(reply); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil {
		t.Fatalf("NewConnNet: %v", r.err)
	}
	return r.c, server
}

func shutdown(c *xgb.Conn, server net.Conn) {
	server.Close()
	c.Close()
}

func waitEvent(t *testing.T, c *xgb.Conn) (xgb.Event, xgb.Error) {
	t.Helper()
	type ev struct {
		e   xgb.Event
		err xgb.Error
	}
	ch := make(chan ev, 1)
	go func() {
		e, err := c.WaitForEvent()
		ch <- ev{e, err}
	}()
	select {
	case r := <-ch:
		return r.e, r.err
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5 s: the reader is stuck or desynchronized")
		return nil, nil
	}
}

// TestGenericEventFraming: a GenericEvent with extra bytes is read whole
// and the core events after it are decoded correctly. The unpatched
// reader read every event as 32 bytes and parsed the GenericEvent's tail
// as events.
func TestGenericEventFraming(t *testing.T) {
	c, server := fakeServer(t)
	defer shutdown(c, server)

	// A GenericEvent of extension 131, event type 6, with 3 extra words
	// whose bytes would look like a ButtonPress (code 4) if misread.
	ge := make([]byte, 32+12)
	ge[0] = xgb.GenericEventCode
	ge[1] = 131
	le.PutUint32(ge[4:], 3)
	le.PutUint16(ge[8:], 6)
	for i := 32; i < len(ge); i++ {
		ge[i] = 4
	}
	// A MotionNotify after it.
	motion := xproto.MotionNotifyEvent{Event: 0x1234, EventX: 17, EventY: -3, State: xproto.KeyButMaskShift}.Bytes()
	// An event of an extension without a Go package (XKB at code 85,
	// StateNotify subtype 2).
	xkbEv := make([]byte, 32)
	xkbEv[0], xkbEv[1] = 85, 2
	// A GenericEvent without extra bytes.
	ge0 := make([]byte, 32)
	ge0[0], ge0[1] = xgb.GenericEventCode, 140
	go func() {
		for _, b := range [][]byte{ge, motion, xkbEv, ge0, motion} {
			server.Write(b)
		}
	}()

	e, _ := waitEvent(t, c)
	raw, ok := e.(xgb.RawEvent)
	if !ok || len(raw) != len(ge) || raw[1] != 131 || le.Uint16(raw[8:]) != 6 || raw[len(raw)-1] != 4 {
		t.Fatalf("first event = %v (%T, %d bytes), want the whole GenericEvent", e, e, len(raw))
	}
	e, _ = waitEvent(t, c)
	if m, ok := e.(xproto.MotionNotifyEvent); !ok || m.Event != 0x1234 || m.EventX != 17 || m.EventY != -3 || m.State != xproto.KeyButMaskShift {
		t.Fatalf("second event = %v, want the MotionNotify", e)
	}
	e, _ = waitEvent(t, c)
	if raw, ok := e.(xgb.RawEvent); !ok || len(raw) != 32 || raw.Code() != 85 || raw[1] != 2 {
		t.Fatalf("third event = %v, want the undecoded XKB event", e)
	}
	e, _ = waitEvent(t, c)
	if raw, ok := e.(xgb.RawEvent); !ok || len(raw) != 32 || raw.Code() != xgb.GenericEventCode || raw[1] != 140 {
		t.Fatalf("fourth event = %v, want the 32-byte GenericEvent", e)
	}
	e, _ = waitEvent(t, c)
	if _, ok := e.(xproto.MotionNotifyEvent); !ok {
		t.Fatalf("fifth event = %v, want the MotionNotify", e)
	}
}

// TestUnknownErrorReachesCookie: an error with a code no package
// registered (an extension error such as XInput2's BadDevice) is returned
// by the request's Reply. The unpatched reader dropped it and the Reply
// blocked forever.
func TestUnknownErrorReachesCookie(t *testing.T) {
	c, server := fakeServer(t)
	defer shutdown(c, server)

	go func() {
		// Read the request (4 bytes) and answer with error 170 for
		// sequence 1.
		req := make([]byte, 4)
		if _, err := io.ReadFull(server, req); err != nil {
			return
		}
		e := make([]byte, 32)
		e[0], e[1] = 0, 170
		le.PutUint16(e[2:], 1)
		le.PutUint32(e[4:], 0xabc)
		le.PutUint16(e[8:], 48)
		e[10] = 131
		server.Write(e)
	}()
	cookie := c.NewCookie(true, true)
	c.NewRequest([]byte{131, 48, 1, 0}, cookie)
	done := make(chan error, 1)
	go func() {
		_, err := cookie.Reply()
		done <- err
	}()
	select {
	case err := <-done:
		var raw xgb.RawError
		if !errors.As(err, &raw) || raw.Code() != 170 || raw.BadId() != 0xabc || raw.MajorOpcode() != 131 || raw.MinorOpcode() != 48 {
			t.Fatalf("Reply error = %v, want the raw error 170", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Reply blocked: the unknown error never reached the cookie")
	}
}
