//go:build linux

package x11

import (
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// TestDRI3Encodings checks the hand-written requests against the layouts
// of dri3proto.h, presentproto.h and xfixesproto.h, byte by byte.
func TestDRI3Encodings(t *testing.T) {
	check := func(name string, got []byte, want ...byte) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Errorf("%s\n got % x\nwant % x", name, got, want)
		}
	}
	check("DRI3 QueryVersion 1.2", versionRequest(147, dri3OpQueryVersion, 1, 2),
		147, 0, 3, 0, 1, 0, 0, 0, 2, 0, 0, 0)
	check("DRI3 Open", dri3OpenRequest(147, 0x1234),
		147, 1, 3, 0, 0x34, 0x12, 0, 0, 0, 0, 0, 0)
	check("DRI3 GetSupportedModifiers", modifiersRequest(147, 0x00a00001, 24, 32),
		147, 6, 3, 0, 1, 0, 0xa0, 0, 24, 32, 0, 0)

	d := platform.DmabufDescriptor{
		Size: image.Pt(1920, 1080), Fourcc: platform.DRMFormatXRGB8888, Modifier: 0x0300000000606014,
		Planes: []platform.DmabufPlane{{FD: 5, Offset: 0, Stride: 7680}, {FD: 5, Offset: 0x7e9000, Stride: 256}},
	}
	check("DRI3 PixmapFromBuffers", pixmapFromBuffersRequest(147, 0x00a00010, 0x00a00001, 24, 32, d),
		147, 7, 16, 0, // 64 bytes
		0x10, 0, 0xa0, 0, // pixmap
		1, 0, 0xa0, 0, // window
		2, 0, 0, 0, // planes, padding
		0x80, 0x07, 0x38, 0x04, // 1920 x 1080
		0, 0x1e, 0, 0, 0, 0, 0, 0, // stride 0, offset 0
		0, 1, 0, 0, 0, 0x90, 0x7e, 0, // stride 1, offset 1
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		24, 32, 0, 0, // depth, bits per pixel, padding
		0x14, 0x60, 0x60, 0, 0, 0, 0, 3) // modifier

	check("Present SelectInput", presentSelectInputRequest(146, 0x00a00020, 0x00a00001, presentCompleteMask|presentIdleMask),
		146, 3, 4, 0, 0x20, 0, 0xa0, 0, 1, 0, 0xa0, 0, 6, 0, 0, 0)
	want := make([]byte, 72)
	copy(want, []byte{146, 1, 18, 0,
		1, 0, 0xa0, 0, // window
		0x10, 0, 0xa0, 0, // pixmap
		7, 0, 0, 0, // serial
		0, 0, 0, 0, // valid: None
		0x30, 0, 0xa0, 0}) // update; then offsets, CRTC, fences, options, MSC, divisor, remainder: 0
	check("PresentPixmap", presentPixmapRequest(146, 0x00a00001, 0x00a00010, 7, 0x00a00030), want...)

	check("XFixes SetRegion", regionRequest(138, xfixesOpSetRegion, 0x00a00030, []image.Rectangle{image.Rect(10, 20, 110, 70), image.Rect(0, 0, 1, 1)}),
		138, 11, 6, 0, 0x30, 0, 0xa0, 0,
		10, 0, 20, 0, 100, 0, 50, 0,
		0, 0, 0, 0, 1, 0, 1, 0)
	check("XFixes CreateRegion, empty", regionRequest(138, xfixesOpCreateRegion, 0x00a00030, nil),
		138, 5, 2, 0, 0x30, 0, 0xa0, 0)
}

// TestDecodeModifiers: the reply of GetSupportedModifiers, and replies
// that are shorter than they claim.
func TestDecodeModifiers(t *testing.T) {
	r := make([]byte, 32+3*8)
	le.PutUint32(r[8:], 1)
	le.PutUint32(r[12:], 2)
	le.PutUint64(r[32:], 0x0300000000606014)
	le.PutUint64(r[40:], 0)
	le.PutUint64(r[48:], platform.DRMModifierInvalid)
	window, screen, err := decodeModifiers(r)
	if err != nil || !slices.Equal(window, []uint64{0x0300000000606014}) || !slices.Equal(screen, []uint64{0, platform.DRMModifierInvalid}) {
		t.Errorf("decodeModifiers = %#x, %#x, %v", window, screen, err)
	}
	le.PutUint32(r[12:], 3) // one more than the reply holds
	if _, _, err := decodeModifiers(r); err == nil {
		t.Error("a reply shorter than its counts was decoded")
	}
	le.PutUint32(r[8:], 0xffffffff)
	if _, _, err := decodeModifiers(r); err == nil {
		t.Error("a reply with an absurd count was decoded")
	}
	if _, _, err := decodeModifiers(r[:16]); err == nil {
		t.Error("a truncated reply was decoded")
	}
	if w, s, err := decodeModifiers(make([]byte, 32)); err != nil || len(w)+len(s) != 0 {
		t.Errorf("an empty reply = %v, %v, %v", w, s, err)
	}
}

// TestDecodePresentEvent: the two events by their wire layouts.
func TestDecodePresentEvent(t *testing.T) {
	ev, ok := decodePresentEvent(idleEvent(0x00a00001, 0x00a00010, 9))
	if !ok || ev.kind != presentEvIdle || ev.eid != fakeEID || ev.window != 0x00a00001 || ev.serial != 9 || ev.pixmap != 0x00a00010 {
		t.Errorf("IdleNotify = %+v, %v", ev, ok)
	}
	ev, ok = decodePresentEvent(completeEvent(0x00a00001, 11, presentModeFlip))
	if !ok || ev.kind != presentEvComplete || ev.window != 0x00a00001 || ev.serial != 11 || ev.completeKind != presentKindPixmap || ev.mode != presentModeFlip {
		t.Errorf("CompleteNotify = %+v, %v", ev, ok)
	}
	if _, ok := decodePresentEvent(completeEvent(1, 1, 0)[:32]); ok {
		t.Error("a truncated CompleteNotify was decoded")
	}
	configure := make([]byte, 40) // PresentConfigureNotify, not selected
	configure[0] = xgb.GenericEventCode
	if _, ok := decodePresentEvent(configure); ok {
		t.Error("a ConfigureNotify was decoded as an event of a present")
	}
}

func TestErrorOpcode(t *testing.T) {
	if op, ok := errorOpcode(xproto.MatchError{MajorOpcode: 146}); !ok || op != 146 {
		t.Errorf("errorOpcode(MatchError) = %d, %v", op, ok)
	}
	raw := make(xgb.RawError, 32)
	raw[10] = 147
	if op, ok := errorOpcode(raw); !ok || op != 147 {
		t.Errorf("errorOpcode(RawError) = %d, %v", op, ok)
	}
}

// --- a scripted X server on a unix socket ---

// wireX is the server end of a real xgb connection over a pair of unix
// sockets: it answers the requests of dri3.go as an X server would, with
// the extensions and versions the test gives it, keeps the file
// descriptors that came with requests, and sends the descriptor of
// device with the reply of DRI3 Open.
type wireX struct {
	t    *testing.T
	conn *net.UnixConn
	// What the server has. A zero version is an absent extension.
	dri3Minor, presentMinor, xfixesMajor uint32
	window, screen                       []uint64
	device                               *os.File // DRI3 Open's reply; nil answers BadMatch
	// RandR (refresh_test.go): its version, and its replies by request
	// (and CRTC), whose sequence numbers are filled in.
	randrMinor uint32
	resources  []byte
	crtcs      map[uint32][]byte
	// failImport makes PixmapFromBuffers answer BadAlloc, failPresent
	// PresentPixmap BadDrawable. The test changes them while the server
	// runs.
	failImport, failPresent atomic.Bool

	mu   sync.Mutex
	log  []string
	fds  []int // received with PixmapFromBuffers
	seq  uint16
	done chan struct{}
}

const (
	wireDRI3, wirePresent, wireXFixes, wireRandR = 147, 146, 138, 139
	// The first event of RandR.
	wireRandREvent = 89
	// The test's window and its graphics context: ids outside the range
	// the connection allocates from (0x00a00001 and up).
	wireWindow, wireGC = 0x00c00001, 0x00c00002
)

// newWireX connects a Backend to a scripted server. The backend has the
// real connection side (connServer) for frames and the fake window
// manager for the rest, and one window, wireWindow, of 300x200 pixels.
func newWireX(t *testing.T, configure func(x *wireX)) (*wireX, *Backend, *Window) {
	t.Helper()
	t.Setenv("XAUTHORITY", t.TempDir()+"/none")
	pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	end := func(fd int) *net.UnixConn {
		f := os.NewFile(uintptr(fd), "x11")
		defer f.Close() // FileConn duplicates it
		c, err := net.FileConn(f)
		if err != nil {
			t.Fatal(err)
		}
		return c.(*net.UnixConn)
	}
	client, server := end(pair[0]), end(pair[1])
	x := &wireX{t: t, conn: server, dri3Minor: 2, presentMinor: 2, xfixesMajor: 5, done: make(chan struct{}),
		screen: []uint64{modTiledA, platform.DRMModifierInvalid}}
	if configure != nil {
		configure(x)
	}
	type result struct {
		c   *xgb.Conn
		err error
	}
	connected := make(chan result, 1)
	go func() {
		c, err := xgb.NewConnNet(client)
		connected <- result{c, err}
	}()
	// The connection setup: only the resource id base and mask are read.
	req := make([]byte, 12)
	if _, err := io.ReadFull(server, req); err != nil {
		t.Fatal(err)
	}
	pad := func(n int) int { return (n + 3) &^ 3 }
	if _, err := io.ReadFull(server, make([]byte, pad(int(le.Uint16(req[6:])))+pad(int(le.Uint16(req[8:]))))); err != nil {
		t.Fatal(err)
	}
	setup := make([]byte, 8+32)
	setup[0] = 1
	le.PutUint16(setup[2:], 11)
	le.PutUint16(setup[6:], 8)
	le.PutUint32(setup[12:], 0x00a00000)
	le.PutUint32(setup[16:], 0x001fffff)
	if _, err := server.Write(setup); err != nil {
		t.Fatal(err)
	}
	r := <-connected
	if r.err != nil {
		t.Fatalf("NewConnNet: %v", r.err)
	}
	go x.serve()

	b := fakeEmpty(1)
	b.conn = r.c
	b.ps = connServer{b}
	w := fakeWindow(b, wireWindow, image.Pt(300, 200), platform.WindowOptions{Size: platform.Size{W: 300, H: 200}})
	w.depth, w.gc = 24, wireGC
	b.dispatch(&recorder{}) // the first ConfigureEvent
	t.Cleanup(func() {
		r.c.Close()
		server.Close()
		<-x.done
		for _, fd := range x.fds {
			unix.Close(fd)
		}
	})
	return x, b, w
}

func (x *wireX) logf(format string, args ...any) {
	x.mu.Lock()
	x.log = append(x.log, fmt.Sprintf(format, args...))
	x.mu.Unlock()
}

// take returns what the server received since the last call. The caller
// makes a round trip first (sync), so that everything sent has arrived.
func (x *wireX) take() string {
	x.mu.Lock()
	defer x.mu.Unlock()
	s := strings.Join(x.log, "; ")
	x.log = nil
	return s
}

// read reads n bytes with the file descriptors that come with them.
func (x *wireX) read(n int) ([]byte, []int, error) {
	buf, oob := make([]byte, n), make([]byte, unix.CmsgSpace(4*8))
	var fds []int
	for read := 0; read < n; {
		k, oobn, _, _, err := x.conn.ReadMsgUnix(buf[read:], oob)
		if err != nil {
			return nil, fds, err
		}
		if k == 0 && oobn == 0 {
			return nil, fds, io.ErrUnexpectedEOF
		}
		read += k
		msgs, _ := unix.ParseSocketControlMessage(oob[:oobn])
		for i := range msgs {
			got, _ := unix.ParseUnixRights(&msgs[i])
			fds = append(fds, got...)
		}
	}
	return buf, fds, nil
}

func (x *wireX) reply(extra int, fill func(r []byte)) {
	r := make([]byte, 32+extra)
	r[0] = 1
	le.PutUint16(r[2:], x.seq)
	le.PutUint32(r[4:], uint32(extra/4))
	if fill != nil {
		fill(r)
	}
	x.conn.Write(r)
}

func (x *wireX) fail(code byte, req []byte) {
	e := make([]byte, 32)
	e[1] = code
	le.PutUint16(e[2:], x.seq)
	le.PutUint16(e[8:], uint16(req[1]))
	e[10] = req[0]
	x.conn.Write(e)
}

func (x *wireX) serve() {
	defer close(x.done)
	for {
		head, fds, err := x.read(4)
		if err != nil {
			return
		}
		req := head
		if n := int(le.Uint16(head[2:]))*4 - 4; n > 0 {
			rest, more, err := x.read(n)
			if err != nil {
				return
			}
			req, fds = append(head, rest...), append(fds, more...)
		}
		x.seq++
		x.handle(req, fds)
	}
}

func (x *wireX) handle(req []byte, fds []int) {
	const badRequest, badMatch, badDrawable, badAlloc = 1, 8, 9, 11
	versions := map[byte]uint32{wireDRI3: x.dri3Minor, wirePresent: x.presentMinor, wireXFixes: x.xfixesMajor, wireRandR: x.randrMinor}
	switch op, minor := req[0], req[1]; {
	case op == 98: // QueryExtension
		code := map[string]byte{"DRI3": wireDRI3, "Present": wirePresent, "XFIXES": wireXFixes, "RANDR": wireRandR}[string(req[8:8+le.Uint16(req[4:])])]
		x.reply(0, func(r []byte) {
			if code != 0 && versions[code] != 0 {
				r[8], r[9] = 1, code
				if code == wireRandR {
					r[10] = wireRandREvent
				}
			}
		})
	case op == 43: // GetInputFocus: the round trip of Sync and Check
		x.reply(0, nil)
	case op == 54: // FreePixmap
		x.logf("FreePixmap %#x", le.Uint32(req[4:]))
	case op == 62: // CopyArea
		x.logf("CopyArea %#x to %#x gc %#x from %d,%d to %d,%d %dx%d", le.Uint32(req[4:]), le.Uint32(req[8:]), le.Uint32(req[12:]),
			le.Uint16(req[16:]), le.Uint16(req[18:]), le.Uint16(req[20:]), le.Uint16(req[22:]), le.Uint16(req[24:]), le.Uint16(req[26:]))
	case versions[op] == 0:
		x.fail(badRequest, req)
	case op == wireRandR && minor == randrOpSelectInput:
		x.logf("RRSelectInput %#x mask %d", le.Uint32(req[4:]), le.Uint16(req[8:]))
	case op == wireRandR && minor == randrOpGetScreenResourcesCurrent:
		x.logf("RRGetScreenResourcesCurrent %#x", le.Uint32(req[4:]))
		x.reply(len(x.resources)-32, func(r []byte) { copy(r[8:], x.resources[8:]) })
	case op == wireRandR && minor == randrOpGetCrtcInfo:
		id := le.Uint32(req[4:])
		x.logf("RRGetCrtcInfo %d at %d", id, le.Uint32(req[8:]))
		rep, ok := x.crtcs[id]
		if !ok {
			x.fail(badRequest, req) // BadRRCrtc of a server, whose code its errors start at
			return
		}
		x.reply(len(rep)-32, func(r []byte) { r[1] = rep[1]; copy(r[8:], rep[8:]) })
	case minor == 0: // QueryVersion: the smaller of the client's and the server's
		x.logf("QueryVersion %d: client %d.%d", op, le.Uint32(req[4:]), le.Uint32(req[8:]))
		x.reply(0, func(r []byte) {
			if op == wireXFixes {
				le.PutUint32(r[8:], min(x.xfixesMajor, le.Uint32(req[4:])))
				return
			}
			le.PutUint32(r[8:], 1)
			le.PutUint32(r[12:], min(versions[op], le.Uint32(req[8:])))
		})
	case op == wireDRI3 && minor == dri3OpOpen:
		if x.device == nil {
			x.fail(badMatch, req)
			return
		}
		r := make([]byte, 32)
		r[0], r[1] = 1, 1
		le.PutUint16(r[2:], x.seq)
		x.conn.WriteMsgUnix(r, unix.UnixRights(int(x.device.Fd())), nil)
	case op == wireDRI3 && minor == dri3OpGetSupportedModifiers:
		x.logf("GetSupportedModifiers %#x depth %d bpp %d", le.Uint32(req[4:]), req[8], req[9])
		x.reply(8*(len(x.window)+len(x.screen)), func(r []byte) {
			le.PutUint32(r[8:], uint32(len(x.window)))
			le.PutUint32(r[12:], uint32(len(x.screen)))
			for i, m := range append(slices.Clone(x.window), x.screen...) {
				le.PutUint64(r[32+8*i:], m)
			}
		})
	case op == wireDRI3 && minor == dri3OpPixmapFromBuffers:
		x.mu.Lock()
		x.fds = append(x.fds, fds...)
		x.mu.Unlock()
		x.logf("PixmapFromBuffers %#x planes %d fds %d %dx%d modifier %#x", le.Uint32(req[4:]), req[12], len(fds),
			le.Uint16(req[16:]), le.Uint16(req[18:]), le.Uint64(req[56:]))
		if x.failImport.Load() || len(fds) != int(req[12]) {
			x.fail(badAlloc, req)
		}
	case op == wirePresent && minor == presentOpSelectInput:
		x.logf("SelectInput eid %#x window %#x mask %d", le.Uint32(req[4:]), le.Uint32(req[8:]), le.Uint32(req[12:]))
	case op == wirePresent && minor == presentOpPixmap:
		x.logf("PresentPixmap window %#x pixmap %#x serial %d update %#x", le.Uint32(req[4:]), le.Uint32(req[8:]), le.Uint32(req[12:]), le.Uint32(req[20:]))
		if x.failPresent.Load() {
			x.fail(badDrawable, req)
		}
	case op == wireXFixes && (minor == xfixesOpCreateRegion || minor == xfixesOpSetRegion):
		var rects []string
		for o := 8; o+8 <= len(req); o += 8 {
			rects = append(rects, fmt.Sprintf("%d,%d %dx%d", le.Uint16(req[o:]), le.Uint16(req[o+2:]), le.Uint16(req[o+4:]), le.Uint16(req[o+6:])))
		}
		x.logf("region %d %#x [%s]", minor, le.Uint32(req[4:]), strings.Join(rects, "; "))
	default:
		x.logf("request %d.%d", op, minor)
	}
}

// send writes an event to the client and feeds it to the backend.
func (x *wireX) send(b *Backend, ev []byte) {
	x.t.Helper()
	x.conn.Write(ev)
	x.next(b)
}

// next feeds the next event or X error the client's reader delivers to
// the backend, as the loop would.
func (x *wireX) next(b *Backend) {
	x.t.Helper()
	got := make(chan xevent, 1)
	go func() {
		e, err := b.conn.WaitForEvent()
		got <- xevent{e, err}
	}()
	select {
	case e := <-got:
		b.translate(e)
	case <-time.After(5 * time.Second):
		x.t.Fatal("no event arrived")
	}
}

// TestDRI3Negotiation: what a server must have for dmabufs, and what the
// backend reports of a server that lacks it.
func TestDRI3Negotiation(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	// The device of the reply's descriptor: /dev/null stands for the DRM
	// node.
	var st unix.Stat_t
	if err := unix.Fstat(int(null.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	major, minor := unix.Major(uint64(st.Rdev)), unix.Minor(uint64(st.Rdev))

	t.Run("a server with everything", func(t *testing.T) {
		x, b, w := newWireX(t, func(x *wireX) { x.device = null; x.window = []uint64{modTiledB} })
		fb, ok := w.DmabufFeedback()
		if !ok {
			t.Fatalf("no feedback: %s", b.dmabuf.caps.reason)
		}
		caps := b.dmabuf.caps
		if caps.dri3Major != 1 || caps.dri3Minor != 2 || caps.presentMajor != 1 || caps.presentMinor != 2 || !caps.regions || caps.presentOpcode != wirePresent {
			t.Errorf("caps = %+v", caps)
		}
		if !fb.HasDevice || fb.DeviceMajor != major || fb.DeviceMinor != minor {
			t.Errorf("device = %v %d:%d, want %d:%d (the node of the descriptor DRI3 Open returned)", fb.HasDevice, fb.DeviceMajor, fb.DeviceMinor, major, minor)
		}
		want := []platform.DmabufFormat{
			{Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiledB},
			{Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiledA},
			{Fourcc: platform.DRMFormatXRGB8888, Modifier: platform.DRMModifierInvalid},
		}
		if !slices.Equal(fb.Formats, want) {
			t.Errorf("formats = %v, want %v", fb.Formats, want)
		}
		if got, want := x.take(), "QueryVersion 147: client 1.2; QueryVersion 146: client 1.2; QueryVersion 138: client 5.0; GetSupportedModifiers 0xc00001 depth 24 bpp 32"; got != want {
			t.Errorf("the server received\n got %s\nwant %s", got, want)
		}
		// Asked once.
		w.DmabufFeedback()
		b.dmabufCaps()
		if got := x.take(); got != "" {
			t.Errorf("a second call sent %s", got)
		}
	})

	for _, tc := range []struct {
		name      string
		configure func(x *wireX)
		reason    string
	}{
		{"no DRI3", func(x *wireX) { x.dri3Minor = 0 }, "no DRI3"},
		{"no Present", func(x *wireX) { x.presentMinor = 0 }, "no Present"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, b, w := newWireX(t, tc.configure)
			if _, ok := w.DmabufFeedback(); ok {
				t.Fatal("feedback from a server without the extension")
			}
			if !strings.Contains(b.dmabuf.caps.reason, tc.reason) {
				t.Errorf("reason = %q, want it to say %q", b.dmabuf.caps.reason, tc.reason)
			}
			_, err := w.CreateDmabuf(dmabufDesc(image.Pt(300, 200), modTiledA))
			if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("CreateDmabuf = %v, want ErrUnsupported with the reason", err)
			}
		})
	}

	t.Run("DRI3 1.1", func(t *testing.T) {
		x, b, w := newWireX(t, func(x *wireX) { x.dri3Minor = 1 })
		if _, ok := w.DmabufFeedback(); ok {
			t.Fatal("feedback from a server with DRI3 1.1")
		}
		if want := "the server has DRI3 1.1, and pixmaps with modifiers need 1.2"; b.dmabuf.caps.reason != want {
			t.Errorf("reason = %q, want %q", b.dmabuf.caps.reason, want)
		}
		// Nothing of Present or XFixes is asked of it.
		if got, want := x.take(), "QueryVersion 147: client 1.2"; got != want {
			t.Errorf("the server received %s, want %s", got, want)
		}
	})

	t.Run("no XFixes and no device", func(t *testing.T) {
		_, b, w := newWireX(t, func(x *wireX) { x.xfixesMajor = 0 })
		fb, ok := w.DmabufFeedback()
		if !ok {
			t.Fatalf("no feedback: %s", b.dmabuf.caps.reason)
		}
		if fb.HasDevice {
			t.Error("a device from a DRI3 Open the server refused")
		}
		if b.dmabuf.caps.regions {
			t.Error("update regions without XFixes")
		}
	})

	t.Run("XFixes 1", func(t *testing.T) {
		_, b, w := newWireX(t, func(x *wireX) { x.xfixesMajor = 1 })
		if _, ok := w.DmabufFeedback(); !ok || b.dmabuf.caps.regions {
			t.Errorf("feedback %v, regions %v; want dmabufs without update regions", ok, b.dmabuf.caps.regions)
		}
	})

	t.Run("no unix socket", func(t *testing.T) {
		// A connection over TCP (here a pipe) carries no descriptors.
		t.Setenv("XAUTHORITY", t.TempDir()+"/none")
		client, server := net.Pipe()
		defer server.Close()
		go func() {
			req := make([]byte, 12)
			io.ReadFull(server, req)
			setup := make([]byte, 8+32)
			setup[0] = 1
			le.PutUint16(setup[2:], 11)
			le.PutUint16(setup[6:], 8)
			le.PutUint32(setup[16:], 0x001fffff)
			server.Write(setup)
		}()
		conn, err := xgb.NewConnNet(client)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		b := fakeEmpty(1)
		b.conn = conn
		b.ps = connServer{b}
		if caps := b.dmabufCaps(); caps.ok || !strings.Contains(caps.reason, "unix socket") {
			t.Errorf("caps over a pipe = %+v", caps)
		}
	})
}

// TestDRI3Wire runs a window's dmabuf frames over a real connection: the
// file descriptors arrive with PixmapFromBuffers, a refused import is the
// call's error, and the requests and events are the ones the server
// expects.
func TestDRI3Wire(t *testing.T) {
	x, b, w := newWireX(t, nil)
	size := image.Pt(300, 200)
	// Two planes in one dmabuf, and the caller's descriptor.
	file, err := os.CreateTemp(t.TempDir(), "dmabuf")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	d := dmabufDesc(size, modTiledA)
	d.Planes = []platform.DmabufPlane{{FD: int(file.Fd()), Stride: 1200}, {FD: int(file.Fd()), Offset: 240000, Stride: 64}}

	buf, err := w.CreateDmabuf(d)
	if err != nil {
		t.Fatalf("CreateDmabuf: %v", err)
	}
	// CreateDmabuf returned after the server's answer: everything arrived.
	got := x.take()
	want := "QueryVersion 147: client 1.2; QueryVersion 146: client 1.2; QueryVersion 138: client 5.0; " +
		"GetSupportedModifiers 0xc00001 depth 24 bpp 32; SelectInput eid 0xa00001 window 0xc00001 mask 6; " +
		"PixmapFromBuffers 0xa00002 planes 2 fds 2 300x200 modifier 0x300000000606014"
	if got != want {
		t.Errorf("registering a buffer: the server received\n got %s\nwant %s", got, want)
	}
	// The descriptors the server received are the caller's file, and the
	// caller's is still open.
	x.mu.Lock()
	received := slices.Clone(x.fds)
	x.mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("the server received %d descriptors, want 2", len(received))
	}
	var mine, theirs unix.Stat_t
	unix.Fstat(int(file.Fd()), &mine)
	for _, fd := range received {
		if err := unix.Fstat(fd, &theirs); err != nil || theirs.Ino != mine.Ino || theirs.Dev != mine.Dev {
			t.Errorf("a received descriptor is not the caller's file (%v)", err)
		}
	}
	if _, err := file.WriteString("x"); err != nil {
		t.Errorf("the caller's descriptor after CreateDmabuf: %v", err)
	}

	// A refused import: the X error is the call's error, the connection
	// lives.
	x.failImport.Store(true)
	_, err = w.CreateDmabuf(d)
	var alloc xproto.AllocError
	if !errors.As(err, &alloc) || errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("a refused import = %v, want the server's BadAlloc", err)
	}
	x.failImport.Store(false)
	second, err := w.CreateDmabuf(d)
	if err != nil {
		t.Fatalf("CreateDmabuf after a refused import: %v", err)
	}
	x.take()

	// The first present maps the window (the fake window manager); the
	// Expose is answered with a CopyArea of the front buffer.
	if err := w.PresentDmabuf(buf, nil); err != nil {
		t.Fatal(err)
	}
	b.translate(xevent{ev: xproto.MapNotifyEvent{Event: wireWindow, Window: wireWindow}})
	b.translate(xevent{ev: xproto.ExposeEvent{Window: wireWindow, X: 10, Y: 20, Width: 100, Height: 50}})
	b.conn.Sync()
	if got, want := x.take(), "CopyArea 0xa00002 to 0xc00001 gc 0xc00002 from 10,20 to 10,20 100x50"; got != want {
		t.Errorf("Expose: the server received\n got %s\nwant %s", got, want)
	}

	// A partial present: the region is created, then PresentPixmap names
	// it. The next one, behind it, has none.
	if err := w.PresentDmabuf(second, []image.Rectangle{image.Rect(10, 20, 110, 70)}); err != nil {
		t.Fatal(err)
	}
	b.conn.Sync()
	if got, want := x.take(), "region 5 0xa00005 [10,20 100x50]; PresentPixmap window 0xc00001 pixmap 0xa00004 serial 1 update 0xa00005"; got != want {
		t.Errorf("a partial present: the server received\n got %s\nwant %s", got, want)
	}
	if !second.Busy() || buf.Busy() {
		t.Errorf("busy after the second present: %v %v, want the first free and the second busy", buf.Busy(), second.Busy())
	}
	eid := w.dmabuf.eid
	event := func(raw xgb.RawEvent) []byte {
		raw[1] = wirePresent
		le.PutUint32(raw[12:], eid)
		return raw
	}
	// The server's answers, as GenericEvents on the wire: the 40 bytes of
	// CompleteNotify are read whole.
	x.send(b, event(completeEvent(wireWindow, 1, presentModeFlip)))
	if err := w.PresentDmabuf(buf, []image.Rectangle{image.Rect(0, 0, 5, 5)}); err != nil {
		t.Fatal(err)
	}
	b.conn.Sync()
	if got, want := x.take(), "region 11 0xa00005 [0,0 5x5]; PresentPixmap window 0xc00001 pixmap 0xa00002 serial 2 update 0xa00005"; got != want {
		t.Errorf("the next partial present: the server received\n got %s\nwant %s", got, want)
	}
	x.send(b, event(idleEvent(wireWindow, 0x00a00004, 1)))
	if second.Busy() || !buf.Busy() || w.dmabuf.flips != 1 {
		t.Errorf("after the flip was replaced: busy %v %v, %d flips", buf.Busy(), second.Busy(), w.dmabuf.flips)
	}

	// The X error of a PresentPixmap, as the reader delivers it (matched
	// to the request by its sequence number), frees the buffer.
	x.failPresent.Store(true)
	if err := w.PresentDmabuf(second, nil); err != nil {
		t.Fatal(err)
	}
	x.next(b)
	x.failPresent.Store(false)
	if len(second.(*dmabufBuffer).pending) != 0 || w.dmabuf.failed != 1 {
		t.Errorf("after the error of a present: %d pending, %d failed", len(second.(*dmabufBuffer).pending), w.dmabuf.failed)
	}

	// Destroy and Close free the pixmaps.
	x.take()
	buf.Destroy()
	w.Close()
	b.conn.Sync()
	if got, want := x.take(), "FreePixmap 0xa00002; FreePixmap 0xa00004"; got != want {
		t.Errorf("Close: the server received\n got %s\nwant %s", got, want)
	}
}
