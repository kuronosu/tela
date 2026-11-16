//go:build linux

package x11

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// fakePresent stands in for the server side of frames (presentServer): it
// records the requests in the fake server's log, answers the queries from
// what the test sets, and numbers the pixmaps from 500.
type fakePresent struct {
	f    *fakeServer
	caps dmabufCaps
	// The reply of GetSupportedModifiers, or its error, and how often it
	// was asked.
	window, screen []uint64
	modErr         error
	modQueries     int
	// pixErr fails PixmapFromBuffers; noEID fails SelectInput's id.
	pixErr error
	noEID  bool
	pixmap xproto.Pixmap
	seq    uint16
}

const (
	fakePresentOpcode = 146
	fakeEID           = 900
	// Modifiers of the fake server: two tiled ones and the linear one.
	modTiledA, modTiledB = 0x0300000000606014, 0x0300000000606010
)

func (p *fakePresent) putImage(w *Window, r image.Rectangle) { p.f.log("put win=%d %v", w.id, r) }

func (p *fakePresent) dmabufCaps() dmabufCaps { return p.caps }

func (p *fakePresent) modifiers(win xproto.Window, depth, bpp byte) ([]uint64, []uint64, error) {
	p.modQueries++
	if depth != 24 || bpp != 32 {
		p.f.log("modifiers asked for depth %d at %d bits", depth, bpp)
	}
	return p.window, p.screen, p.modErr
}

func (p *fakePresent) selectPresent(win xproto.Window) uint32 {
	if p.noEID {
		return 0
	}
	p.f.log("select present win=%d", win)
	return fakeEID
}

func (p *fakePresent) pixmapFromBuffers(win xproto.Window, depth, bpp byte, d platform.DmabufDescriptor) (xproto.Pixmap, error) {
	if p.pixErr != nil {
		return 0, p.pixErr
	}
	if p.pixmap == 0 {
		p.pixmap = 499
	}
	p.pixmap++
	p.f.log("pixmap %d win=%d %dx%d depth %d/%d mod=%#x planes=%d", p.pixmap, win, d.Size.X, d.Size.Y, depth, bpp, d.Modifier, len(d.Planes))
	return p.pixmap, nil
}

func (p *fakePresent) freePixmap(pix xproto.Pixmap) { p.f.log("free %d", pix) }

func (p *fakePresent) presentPixmap(win xproto.Window, pix xproto.Pixmap, serial uint32, update []image.Rectangle) uint16 {
	p.seq++
	what := "all"
	if update != nil {
		what = fmt.Sprint(update)
	}
	p.f.log("present %d win=%d serial=%d update=%s", pix, win, serial, what)
	return p.seq
}

func (p *fakePresent) copyPixmap(win xproto.Window, gc xproto.Gcontext, pix xproto.Pixmap, r image.Rectangle) {
	p.f.log("copy %d win=%d %v", pix, win, r)
}

// dmabufRig is a fake backend whose server presents dmabufs, with one
// window of 300x200 pixels that got its first ConfigureEvent and frame.
type dmabufRig struct {
	*rig
	p *fakePresent
	w *Window
}

func newDmabufRig(t *testing.T, opts platform.WindowOptions) *dmabufRig {
	g := newRig(t)
	p := &fakePresent{
		f: g.f,
		caps: dmabufCaps{ok: true, dri3Major: 1, dri3Minor: 2, presentMajor: 1, presentMinor: 2,
			presentOpcode: fakePresentOpcode, regions: true, hasDevice: true, major: 226, minor: 128},
		screen: []uint64{modTiledA, modTiledB, platform.DRMModifierLinear},
	}
	g.b.ps = p
	if opts.Size == (platform.Size{}) {
		opts.Size = platform.Size{W: 300, H: 200}
	}
	w := fakeWindow(g.b, 7, image.Pt(300, 200), opts)
	w.depth = 24
	g.feed() // the first ConfigureEvent and its frame
	return &dmabufRig{rig: g, p: p, w: w}
}

// buffer registers a buffer of a size.
func (g *dmabufRig) buffer(size image.Point) platform.DmabufBuffer {
	g.t.Helper()
	buf, err := g.w.CreateDmabuf(dmabufDesc(size, modTiledA))
	if err != nil {
		g.t.Fatalf("CreateDmabuf: %v", err)
	}
	return buf
}

// swapchain registers two buffers of 300x200, numbered 500 and 501 when
// they are the window's first, and forgets their requests.
func (g *dmabufRig) swapchain() (a, b platform.DmabufBuffer) {
	g.t.Helper()
	a, b = g.buffer(image.Pt(300, 200)), g.buffer(image.Pt(300, 200))
	g.f.take()
	return a, b
}

// present presents a buffer with the damage.
func (g *dmabufRig) presentBuf(buf platform.DmabufBuffer, damage ...image.Rectangle) {
	g.t.Helper()
	if err := g.w.PresentDmabuf(buf, damage); err != nil {
		g.t.Fatalf("PresentDmabuf: %v", err)
	}
}

// mapped feeds the MapNotify and the Expose that follow a map.
func (g *dmabufRig) mapped(what, wantSent string) {
	g.t.Helper()
	size := g.w.osSize
	_, sent := g.feed(xproto.MapNotifyEvent{Event: g.w.id, Window: g.w.id},
		xproto.ExposeEvent{Window: g.w.id, Width: uint16(size.X), Height: uint16(size.Y)})
	if sent != wantSent {
		g.t.Errorf("%s: requests\n got %s\nwant %s", what, sent, wantSent)
	}
}

// busy checks which buffers are busy.
func (g *dmabufRig) busy(what string, want string, bufs ...platform.DmabufBuffer) {
	g.t.Helper()
	var got []string
	for _, b := range bufs {
		got = append(got, fmt.Sprint(b.Busy()))
	}
	if s := strings.Join(got, " "); s != want {
		g.t.Errorf("%s: busy = %s, want %s", what, s, want)
	}
}

func dmabufDesc(size image.Point, modifier uint64) platform.DmabufDescriptor {
	return platform.DmabufDescriptor{
		Size: size, Fourcc: platform.DRMFormatXRGB8888, Modifier: modifier,
		Planes: []platform.DmabufPlane{{FD: 0, Stride: uint32(4 * size.X)}},
	}
}

// idleEvent is a PresentIdleNotify, completeEvent a PresentCompleteNotify
// of a pixmap, as the server sends them: GenericEvents of the extension.
func idleEvent(win xproto.Window, pix xproto.Pixmap, serial uint32) xgb.RawEvent {
	raw := make([]byte, 32)
	raw[0], raw[1] = xgb.GenericEventCode, fakePresentOpcode
	le.PutUint16(raw[8:], presentEvIdle)
	le.PutUint32(raw[12:], fakeEID)
	le.PutUint32(raw[16:], uint32(win))
	le.PutUint32(raw[20:], serial)
	le.PutUint32(raw[24:], uint32(pix))
	return raw
}

func completeEvent(win xproto.Window, serial uint32, mode byte) xgb.RawEvent {
	raw := make([]byte, 40)
	raw[0], raw[1] = xgb.GenericEventCode, fakePresentOpcode
	le.PutUint32(raw[4:], 2)
	le.PutUint16(raw[8:], presentEvComplete)
	raw[10], raw[11] = presentKindPixmap, mode
	le.PutUint32(raw[12:], fakeEID)
	le.PutUint32(raw[16:], uint32(win))
	le.PutUint32(raw[20:], serial)
	return raw
}

func mustPanic(t *testing.T, what, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Errorf("%s did not panic", what)
		} else if !strings.Contains(fmt.Sprint(r), want) {
			t.Errorf("%s panicked with %q, want a message with %q", what, r, want)
		}
	}()
	f()
}

// TestDmabufFeedback: the feedback lists XRGB8888 with the window's
// modifiers before the screen's, names the device of DRI3 Open, and is
// asked once per size; its Serial changes only with its content.
func TestDmabufFeedback(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	g.p.window = []uint64{modTiledB}
	fb, ok := g.w.DmabufFeedback()
	if !ok {
		t.Fatal("no feedback on a server with DRI3 1.2 and Present")
	}
	want := []platform.DmabufFormat{
		{Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiledB}, // the window's first, once
		{Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiledA},
		{Fourcc: platform.DRMFormatXRGB8888, Modifier: platform.DRMModifierLinear},
	}
	if !slices.Equal(fb.Formats, want) {
		t.Errorf("formats = %v, want %v", fb.Formats, want)
	}
	if !fb.HasDevice || fb.DeviceMajor != 226 || fb.DeviceMinor != 128 {
		t.Errorf("device = %v %d:%d, want 226:128", fb.HasDevice, fb.DeviceMajor, fb.DeviceMinor)
	}
	if fb.Serial == 0 {
		t.Error("the Serial is 0")
	}
	if fb.Accepts(platform.DRMFormatXBGR8888, modTiledA) || fb.Accepts(platform.DRMFormatARGB8888, modTiledA) {
		t.Error("the feedback lists a format a depth-24 pixmap is not")
	}

	// Asked once: every frame may compare the serial.
	again, _ := g.w.DmabufFeedback()
	if again.Serial != fb.Serial || g.p.modQueries != 1 {
		t.Errorf("a second call: serial %d after %d, %d queries", again.Serial, fb.Serial, g.p.modQueries)
	}

	// A new size asks again; the same answer keeps the serial.
	g.feed(xproto.ConfigureNotifyEvent{Event: 7, Window: 7, Width: 640, Height: 480})
	again, _ = g.w.DmabufFeedback()
	if again.Serial != fb.Serial || g.p.modQueries != 2 {
		t.Errorf("after a resize: serial %d after %d, %d queries", again.Serial, fb.Serial, g.p.modQueries)
	}
	// Another answer (a fullscreen window the server can scan out) gets a
	// new one.
	g.p.window = []uint64{platform.DRMModifierLinear}
	g.feed(xproto.ConfigureNotifyEvent{Event: 7, Window: 7, Width: 1920, Height: 1080})
	changed, _ := g.w.DmabufFeedback()
	if changed.Serial == fb.Serial || changed.Formats[0].Modifier != platform.DRMModifierLinear {
		t.Errorf("after the modifiers changed: serial %d (was %d), formats %v", changed.Serial, fb.Serial, changed.Formats)
	}
	// A failed query keeps the answer.
	g.p.modErr = errors.New("BadWindow")
	g.feed(xproto.ConfigureNotifyEvent{Event: 7, Window: 7, Width: 800, Height: 600})
	if kept, ok := g.w.DmabufFeedback(); !ok || kept.Serial != changed.Serial {
		t.Errorf("after a failed query: %v, serial %d, want the answer of serial %d", ok, kept.Serial, changed.Serial)
	}
	// A window whose first query fails has none, and does not ask again
	// at every frame.
	g = newDmabufRig(t, platform.WindowOptions{})
	g.p.modErr = errors.New("BadWindow")
	for range 3 {
		if _, ok := g.w.DmabufFeedback(); ok {
			t.Error("feedback from a failed query")
		}
	}
	if g.p.modQueries != 1 {
		t.Errorf("a failed query was made %d times, want once", g.p.modQueries)
	}

	// A server that lists no modifier takes implicit ones.
	g = newDmabufRig(t, platform.WindowOptions{})
	g.p.window, g.p.screen = nil, nil
	fb, ok = g.w.DmabufFeedback()
	if !ok || !slices.Equal(fb.Formats, []platform.DmabufFormat{{Fourcc: platform.DRMFormatXRGB8888, Modifier: platform.DRMModifierInvalid}}) {
		t.Errorf("without modifiers: %v %v, want XRGB8888 with the implicit modifier", ok, fb.Formats)
	}
	// No device: the feedback says so.
	g = newDmabufRig(t, platform.WindowOptions{})
	g.p.caps.hasDevice = false
	if fb, ok = g.w.DmabufFeedback(); !ok || fb.HasDevice {
		t.Errorf("without a device: %v, HasDevice %v", ok, fb.HasDevice)
	}
	// A closed window has none.
	g.w.Close()
	if _, ok := g.w.DmabufFeedback(); ok {
		t.Error("a closed window reports feedback")
	}
}

// TestDmabufUnavailable: a server without DRI3 1.2 or Present, and a
// transparent window, report no feedback and register nothing; the caller
// presents with Present.
func TestDmabufUnavailable(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	g.p.caps = dmabufCaps{reason: "the server has DRI3 1.0, and pixmaps with modifiers need 1.2"}
	if _, ok := g.w.DmabufFeedback(); ok {
		t.Error("feedback from a server with DRI3 1.0")
	}
	_, err := g.w.CreateDmabuf(dmabufDesc(image.Pt(300, 200), modTiledA))
	if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), "DRI3 1.0") {
		t.Errorf("CreateDmabuf = %v, want ErrUnsupported naming the reason", err)
	}
	if g.p.modQueries != 0 {
		t.Error("the modifiers were asked of a server without the extensions")
	}
	g.sent("no dmabufs", "")

	g = newDmabufRig(t, platform.WindowOptions{Transparent: true, Frameless: true})
	if _, ok := g.w.DmabufFeedback(); ok {
		t.Error("a transparent window reports feedback: its input shape needs the pixels")
	}
	if _, err := g.w.CreateDmabuf(dmabufDesc(image.Pt(300, 200), modTiledA)); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("CreateDmabuf on a transparent window = %v, want ErrUnsupported", err)
	}

	// A fake backend without a server side, as other tests make them.
	b, w := fakeBackend(1)
	if _, ok := w.DmabufFeedback(); ok || b.dmabuf.caps.ok {
		t.Error("feedback without a connection")
	}
}

// TestDmabufCreate: a buffer is a pixmap of the window's depth; what the
// server refuses is an error of the call, and what cannot work is not
// sent.
func TestDmabufCreate(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	size := image.Pt(300, 200)
	g.f.take()
	buf := g.buffer(size)
	g.sent("the first buffer", "select present win=7; pixmap 500 win=7 300x200 depth 24/32 mod=0x300000000606014 planes=1")
	if buf.Size() != size || buf.Busy() {
		t.Errorf("a new buffer: size %v, busy %v", buf.Size(), buf.Busy())
	}
	g.buffer(size)
	g.sent("the second buffer", "pixmap 501 win=7 300x200 depth 24/32 mod=0x300000000606014 planes=1")

	for _, tc := range []struct {
		name        string
		d           platform.DmabufDescriptor
		unsupported bool
	}{
		{"a format the server does not list", platform.DmabufDescriptor{Size: size, Fourcc: platform.DRMFormatXBGR8888, Modifier: modTiledA,
			Planes: []platform.DmabufPlane{{Stride: 1200}}}, true},
		{"a modifier the server does not list", dmabufDesc(size, 0x0100000000000002), true},
		{"alpha", platform.DmabufDescriptor{Size: size, Fourcc: platform.DRMFormatXRGB8888, Modifier: modTiledA, Alpha: true,
			Planes: []platform.DmabufPlane{{Stride: 1200}}}, true},
		{"a size beyond a pixmap's", dmabufDesc(image.Pt(40000, 200), modTiledA), false},
	} {
		_, err := g.w.CreateDmabuf(tc.d)
		if err == nil || errors.Is(err, platform.ErrUnsupported) != tc.unsupported {
			t.Errorf("%s: CreateDmabuf = %v (ErrUnsupported wanted: %v)", tc.name, err, tc.unsupported)
		}
	}
	g.sent("refused before the request", "")

	// The server's refusal: an error, and the window goes on.
	g.p.pixErr = xproto.AllocError{NiceName: "Alloc"}
	_, err := g.w.CreateDmabuf(dmabufDesc(size, modTiledA))
	var alloc xproto.AllocError
	if !errors.As(err, &alloc) || errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("a failed import = %v, want the server's BadAlloc", err)
	}
	g.p.pixErr = nil
	g.buffer(size)
	g.sent("after a failed import", "pixmap 502 win=7 300x200 depth 24/32 mod=0x300000000606014 planes=1")

	// A descriptor that cannot describe a buffer is a programmer error.
	mustPanic(t, "CreateDmabuf without planes", "planes", func() {
		g.w.CreateDmabuf(platform.DmabufDescriptor{Size: size, Fourcc: platform.DRMFormatXRGB8888})
	})

	g = newDmabufRig(t, platform.WindowOptions{})
	g.p.noEID = true
	if _, err := g.w.CreateDmabuf(dmabufDesc(size, modTiledA)); err == nil {
		t.Error("CreateDmabuf without an id for the Present events succeeded")
	}
}

// TestDmabufPresent follows a window's frames: the first present maps the
// window and is painted at its Expose, the next ones are PresentPixmap
// requests with the damage as update region unless one is still waiting,
// and a buffer is busy while the server reads it or it is the frame on
// screen.
func TestDmabufPresent(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	a, b := g.swapchain()

	if !g.w.NeedsFull() {
		t.Fatal("the first present is not marked full")
	}
	// The first present: full whatever the damage, and no PresentPixmap
	// for a window without contents.
	g.presentBuf(a, image.Rect(10, 10, 20, 20))
	g.sent("the first present", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7")
	g.busy("the first present", "true false", a, b)
	g.mapped("the Expose of the map", "copy 500 win=7 (0,0)-(300,200)")

	// A frame that changed a rectangle: the update region.
	patch := image.Rect(100, 50, 180, 120)
	g.presentBuf(b, patch)
	g.sent("partial damage", "present 501 win=7 serial=1 update=[(100,50)-(180,120)]")
	// The old front buffer was never given to the server: free at once.
	g.busy("the second present", "false true", a, b)
	mustPanic(t, "presenting a busy buffer", "busy buffer", func() { g.w.PresentDmabuf(b, nil) })

	// Faster than the server shows: while a present waits, the next has
	// no region, so that the server replaces the waiting one instead of
	// queueing both.
	g.presentBuf(a, patch)
	g.sent("a present behind a waiting one", "present 500 win=7 serial=2 update=all")
	g.busy("two waiting", "true true", a, b)
	// The server skips the first (its answers), then shows the second.
	g.expect("the skipped present", "", "", completeEvent(7, 1, presentModeSkip), idleEvent(7, 501, 1))
	g.busy("after the skip", "true false", a, b)
	g.expect("the flip", "", "", completeEvent(7, 2, presentModeFlip))
	// A flipped buffer is not idle until the next one replaced it, and
	// it is the front buffer too.
	g.busy("after the flip", "true false", a, b)
	if st := &g.w.dmabuf; st.skips != 1 || st.flips != 1 || st.copies != 0 {
		t.Errorf("completions: %d flips, %d copies, %d skips", st.flips, st.copies, st.skips)
	}

	// Nothing waits: the region again, with several rectangles.
	g.presentBuf(b, patch, image.Rect(0, 0, 10, 10), image.Rect(290, 190, 400, 400))
	g.sent("several rectangles", "present 501 win=7 serial=3 update=[(100,50)-(180,120) (0,0)-(10,10) (290,190)-(300,200)]")
	// The idle of the replaced flip frees it; the copy's own idle leaves
	// the front buffer busy.
	g.busy("before the answers", "true true", a, b)
	g.expect("the answers", "", "", completeEvent(7, 3, 0), idleEvent(7, 500, 2), idleEvent(7, 501, 3))
	g.busy("a copied front buffer", "false true", a, b)

	// Damage that covers everything, and no damage, are whole updates.
	g.presentBuf(a, image.Rect(-5, -5, 500, 500))
	g.sent("damage of the whole buffer", "present 500 win=7 serial=4 update=all")
	g.expect("its answers", "", "", completeEvent(7, 4, 0), idleEvent(7, 500, 4))
	g.presentBuf(b)
	g.sent("no damage", "present 501 win=7 serial=5 update=all")
	g.expect("its answers", "", "", completeEvent(7, 5, 0), idleEvent(7, 501, 5))

	// Damage outside the buffer: nothing to present, the buffer stays
	// free and the front buffer the front.
	g.presentBuf(a, image.Rect(400, 400, 500, 500))
	g.sent("damage clipped away", "")
	g.busy("damage clipped away", "false true", a, b)

	// Expose: from the front buffer, the rectangle asked, without the app.
	g.expect("Expose", "", "copy 501 win=7 (20,30)-(120,80)",
		xproto.ExposeEvent{Window: 7, X: 20, Y: 30, Width: 100, Height: 50})
	// A series is answered once, with its bounds.
	g.expect("Expose series", "", "copy 501 win=7 (0,0)-(300,200)",
		xproto.ExposeEvent{Window: 7, X: 0, Y: 0, Width: 10, Height: 10, Count: 1},
		xproto.ExposeEvent{Window: 7, X: 250, Y: 150, Width: 200, Height: 200})

	// Events of another window's id, or for a pixmap that is gone, change
	// nothing.
	g.presentBuf(a)
	g.f.take()
	stray := idleEvent(7, 500, 6)
	le.PutUint32(stray[12:], fakeEID+1)
	g.feed(stray, idleEvent(7, 777, 6), idleEvent(8, 500, 6), idleEvent(7, 500, 99))
	g.busy("stray events", "true false", a, b)

	// Without XFixes there are no regions.
	g.feed(completeEvent(7, 6, 0), idleEvent(7, 500, 6))
	g.b.dmabuf.caps.regions = false
	g.presentBuf(b, patch)
	g.sent("without XFixes", "present 501 win=7 serial=7 update=all")
}

// TestDmabufPresentPanics: what Present panics for, PresentDmabuf does
// too.
func TestDmabufPresentPanics(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	size := image.Pt(300, 200)
	other := newDmabufRig(t, platform.WindowOptions{})
	foreign := other.buffer(size)
	mustPanic(t, "a buffer of another window", "not this window's", func() { g.w.PresentDmabuf(foreign, nil) })
	mustPanic(t, "a nil buffer", "not this window's", func() { g.w.PresentDmabuf(nil, nil) })

	small := g.buffer(image.Pt(100, 100))
	mustPanic(t, "a buffer of another size", "100x100 buffer on a window of 300x200", func() { g.w.PresentDmabuf(small, nil) })

	gone := g.buffer(size)
	gone.Destroy()
	mustPanic(t, "a destroyed buffer", "destroyed buffer", func() { g.w.PresentDmabuf(gone, nil) })

	// Before the first ConfigureEvent there is no size to check.
	early := newRig(t)
	early.b.ps = g.p
	w := fakeWindow(early.b, 9, size, platform.WindowOptions{Size: platform.Size{W: 300, H: 200}})
	w.depth = 24
	buf, err := w.CreateDmabuf(dmabufDesc(size, modTiledA))
	if err != nil {
		t.Fatal(err)
	}
	mustPanic(t, "a present before the first ConfigureEvent", "first ConfigureEvent", func() { w.PresentDmabuf(buf, nil) })

	// A closed window drops the present.
	live := g.buffer(size)
	g.w.Close()
	g.f.take()
	if err := g.w.PresentDmabuf(live, nil); err != nil {
		t.Errorf("PresentDmabuf on a closed window: %v", err)
	}
	g.sent("a closed window", "")
}

// TestDmabufResize: after a ConfigureEvent the caller destroys its
// buffers and registers new ones. The front buffer's pixmap stays until
// the first frame of the new size, so that an Expose in between still has
// the old frame.
func TestDmabufResize(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	a, b := g.swapchain()
	g.presentBuf(a)
	g.mapped("map", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7; copy 500 win=7 (0,0)-(300,200)")
	g.presentBuf(b)
	g.f.take()

	g.expect("the resize", "state {at 0,0}; configure 400x300@1x (400x300 px); frame", "",
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, Width: 400, Height: 300})
	mustPanic(t, "a buffer of the old size", "300x200 buffer on a window of 400x300", func() { g.w.PresentDmabuf(a, nil) })
	// The swapchain goes: the front buffer's pixmap is kept.
	a.Destroy()
	b.Destroy()
	g.sent("destroying the swapchain", "free 500")
	g.busy("destroyed buffers", "false false", a, b)
	// The server answers the present of the destroyed front buffer.
	g.expect("answers for a destroyed buffer", "", "", completeEvent(7, 1, 0), idleEvent(7, 501, 1))
	// The window grew: the Expose of the new area is clipped to the old
	// frame.
	g.expect("Expose after the resize", "", "copy 501 win=7 (0,0)-(300,200)",
		xproto.ExposeEvent{Window: 7, Width: 400, Height: 300})

	c := g.buffer(image.Pt(400, 300))
	g.f.take()
	// The first present after a ConfigureEvent is full.
	g.presentBuf(c, image.Rect(0, 0, 10, 10))
	g.sent("the first frame of the new size", "free 501; present 502 win=7 serial=2 update=all")
	g.expect("Expose", "", "copy 502 win=7 (0,0)-(400,300)", xproto.ExposeEvent{Window: 7, Width: 400, Height: 300})
	b.Destroy() // twice: nothing
	g.sent("Destroy twice", "")
}

// TestDmabufAlternate: Present and PresentDmabuf follow each other on one
// window, the first of a kind being a full one, and there is always one
// frame to answer Expose from.
func TestDmabufAlternate(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	size := image.Pt(300, 200)
	bounds := image.Rectangle{Max: size}
	fb := image.NewRGBA(bounds)
	patch := []image.Rectangle{image.Rect(10, 10, 20, 20)}

	// PutImage first: the window is mapped with the retained copy.
	if err := g.w.Present(fb, nil); err != nil {
		t.Fatal(err)
	}
	g.mapped("map", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7; put win=7 (0,0)-(300,200)")
	if err := g.w.Present(fb, patch); err != nil {
		t.Fatal(err)
	}
	g.sent("a partial Present", "put win=7 (10,10)-(20,20)")

	// A dmabuf after PutImage: the whole window, whatever its damage, and
	// the retained copy goes.
	a, b := g.swapchain()
	g.presentBuf(a, patch...)
	g.sent("a dmabuf after PutImage", "present 500 win=7 serial=1 update=all")
	if g.w.pix != nil {
		t.Error("the retained copy outlives a dmabuf present")
	}
	g.expect("Expose after a dmabuf", "", "copy 500 win=7 (0,0)-(300,200)",
		completeEvent(7, 1, presentModeFlip), xproto.ExposeEvent{Window: 7, Width: 300, Height: 200})
	g.presentBuf(b, patch...)
	g.sent("the next dmabuf", "present 501 win=7 serial=2 update=[(10,10)-(20,20)]")
	g.feed(completeEvent(7, 2, presentModeFlip), idleEvent(7, 500, 1))

	// PutImage after a dmabuf: the whole framebuffer, whatever its
	// damage; the front buffer is no longer the frame.
	if err := g.w.Present(fb, patch); err != nil {
		t.Fatal(err)
	}
	g.sent("PutImage after a dmabuf", "put win=7 (0,0)-(300,200)")
	if g.w.dmabuf.front != nil {
		t.Error("the front buffer outlives a Present")
	}
	// The flipped buffer is the server's until it says so.
	g.busy("after the change of kind", "false true", a, b)
	g.expect("Expose after PutImage", "", "put win=7 (5,5)-(25,25)", xproto.ExposeEvent{Window: 7, X: 5, Y: 5, Width: 20, Height: 20})
	if err := g.w.Present(fb, patch); err != nil {
		t.Fatal(err)
	}
	g.sent("the next Present", "put win=7 (10,10)-(20,20)")

	// And back: full again.
	g.presentBuf(a, patch...)
	g.sent("a dmabuf again", "present 500 win=7 serial=3 update=all")
	g.feed(completeEvent(7, 3, presentModeFlip), idleEvent(7, 501, 2))
	g.busy("the replaced flip", "true false", a, b)

	// A buffer destroyed while it is the front buffer is freed by the
	// Present that replaces its frame.
	a.Destroy()
	g.sent("destroying the front buffer", "")
	if err := g.w.Present(fb, nil); err != nil {
		t.Fatal(err)
	}
	g.sent("a Present over a destroyed front buffer", "free 500; put win=7 (0,0)-(300,200)")
}

// TestDmabufHideShow: a hidden window drops its presents and keeps its
// front buffer, with which it is mapped again; a present dropped
// meanwhile is asked for again.
func TestDmabufHideShow(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	size := image.Pt(300, 200)
	a, b := g.swapchain()
	g.presentBuf(a)
	g.mapped("map", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7; copy 500 win=7 (0,0)-(300,200)")

	g.w.SetVisible(false)
	g.expect("hidden", "", "unmap win=7; withdrawn win=7", xproto.UnmapNotifyEvent{Event: 7, Window: 7})
	g.busy("hidden", "true false", a, b)
	// Shown again with nothing dropped: mapped with the front buffer, and
	// no frame is asked for.
	g.w.SetVisible(true)
	g.sent("shown", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7")
	events, sent := g.feed(xproto.MapNotifyEvent{Event: 7, Window: 7}, xproto.ExposeEvent{Window: 7, Width: 300, Height: 200})
	if events != "" || sent != "copy 500 win=7 (0,0)-(300,200)" {
		t.Errorf("shown again: events %q, requests %q", events, sent)
	}

	// A present while hidden is dropped: the buffer stays free, the next
	// present is still full, and the show asks for a frame.
	g.w.SetVisible(false)
	g.feed(xproto.UnmapNotifyEvent{Event: 7, Window: 7})
	g.f.take()
	g.presentBuf(b)
	g.sent("a present while hidden", "")
	g.busy("a present while hidden", "true false", a, b)
	g.w.SetVisible(true)
	g.sent("shown after a dropped present", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7")
	// The frame comes when the pacing allows (the window had one just
	// now); the old frame shows meanwhile.
	if !g.w.FramePending() || !g.w.NeedsFull() {
		t.Errorf("shown after a dropped present: frame requested %v, full %v; want both", g.w.FramePending(), g.w.NeedsFull())
	}
	if _, sent = g.feed(xproto.MapNotifyEvent{Event: 7, Window: 7}, xproto.ExposeEvent{Window: 7, Width: 300, Height: 200}); sent != "copy 500 win=7 (0,0)-(300,200)" {
		t.Errorf("shown after a dropped present: requests %q, want the old frame meanwhile", sent)
	}
	g.presentBuf(b, image.Rect(0, 0, 5, 5))
	g.sent("the frame asked for", "present 501 win=7 serial=1 update=all")

	// A window created hidden: its first dmabuf is dropped, and the show
	// maps nothing; the frame that follows does.
	h := newDmabufRig(t, platform.WindowOptions{Hidden: true})
	buf := h.buffer(size)
	h.f.take()
	h.presentBuf(buf)
	h.sent("the first present of a hidden window", "")
	h.w.SetVisible(true)
	h.sent("shown without a frame", "")
	if events, _ := h.feed(); events != "frame" {
		t.Errorf("shown without a frame: events %q, want a frame", events)
	}
	h.presentBuf(buf)
	h.sent("its first present", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7")

	// An iconified window gets no PresentPixmap: the Expose of its return
	// paints the front buffer.
	i := newDmabufRig(t, platform.WindowOptions{})
	x, y := i.swapchain()
	i.presentBuf(x)
	i.mapped("map", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7; copy 500 win=7 (0,0)-(300,200)")
	i.feed(xproto.UnmapNotifyEvent{Event: 7, Window: 7})
	i.f.take()
	i.presentBuf(y)
	i.sent("a present while iconified", "")
	i.busy("a present while iconified", "false true", x, y)
	i.mapped("back", "copy 501 win=7 (0,0)-(300,200)")
}

// TestDmabufError: the X error of a PresentPixmap frees its buffer, which
// no PresentIdleNotify will; errors of other requests do not.
func TestDmabufError(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	a, b := g.swapchain()
	g.presentBuf(a)
	g.mapped("map", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7; copy 500 win=7 (0,0)-(300,200)")
	g.presentBuf(b) // sequence 1
	g.presentBuf(a) // sequence 2
	g.busy("two presents", "true true", a, b)

	// Another request with the same sequence number, 65536 requests
	// later: not ours.
	g.b.translate(xevent{err: xproto.WindowError{Sequence: 1, MajorOpcode: 12}})
	g.busy("an error of another request", "true true", a, b)
	g.b.translate(xevent{err: xproto.MatchError{Sequence: 1, MajorOpcode: fakePresentOpcode, MinorOpcode: presentOpPixmap}})
	g.busy("the error of a present", "true false", a, b)
	if g.w.dmabuf.failed != 1 {
		t.Errorf("failed presents = %d, want 1", g.w.dmabuf.failed)
	}
	// An error xgb has no type for (an extension's own).
	raw := make(xgb.RawError, 32)
	raw[1], raw[10] = 170, fakePresentOpcode
	le.PutUint16(raw[2:], 2)
	g.b.translate(xevent{err: raw})
	// The front buffer stays busy as the front buffer.
	g.busy("the error of the front buffer's present", "true false", a, b)
	if st := &g.w.dmabuf; st.failed != 2 || st.sent != st.completed {
		t.Errorf("after both errors: %d failed, sent %d, completed %d", st.failed, st.sent, st.completed)
	}
}

// TestDmabufClose: closing the window frees its pixmaps, which the server
// does not free with it, and the caller's buffers have nothing left to
// do.
func TestDmabufClose(t *testing.T) {
	g := newDmabufRig(t, platform.WindowOptions{})
	size := image.Pt(300, 200)
	a, b := g.swapchain()
	c := g.buffer(size)
	g.f.take()
	g.presentBuf(a)
	g.mapped("map", "prop WM_NORMAL_HINTS win=7 = gravity=10; prop _NET_WM_STATE win=7 = []; map win=7; copy 500 win=7 (0,0)-(300,200)")
	g.presentBuf(b)
	b.Destroy() // the front buffer: its pixmap is kept
	g.f.take()
	if g.b.dmabuf.users != 1 {
		t.Errorf("windows with Present events = %d, want 1", g.b.dmabuf.users)
	}
	g.w.Close()
	g.sent("Close", "free 501; free 502; free 500; destroy win=7")
	g.busy("after Close", "false false false", a, b, c)
	a.Destroy()
	c.Destroy()
	g.sent("Destroy after Close", "")
	if g.b.dmabuf.users != 0 {
		t.Errorf("windows with Present events after Close = %d", g.b.dmabuf.users)
	}
	if _, err := g.w.CreateDmabuf(dmabufDesc(size, modTiledA)); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("CreateDmabuf on a closed window = %v", err)
	}
}
