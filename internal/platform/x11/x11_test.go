//go:build linux

package x11

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
)

func TestNewWithoutDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	_, err := New()
	if !errors.Is(err, platform.ErrUnavailable) {
		t.Fatalf("New() with DISPLAY unset = %v, want an error wrapping ErrUnavailable", err)
	}
}

func TestNewNoServer(t *testing.T) {
	// A display number nobody listens on: the connection fails.
	t.Setenv("DISPLAY", ":4093")
	_, err := New()
	if !errors.Is(err, platform.ErrUnavailable) {
		t.Fatalf("New() with no server = %v, want an error wrapping ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), ":4093") {
		t.Errorf("error %q does not name the display", err)
	}
}

func TestCursorGlyph(t *testing.T) {
	for c := platform.CursorDefault; c.Valid(); c++ {
		g, ok := cursorGlyph(c)
		if c == platform.CursorNone {
			if ok {
				t.Errorf("CursorNone maps to glyph %d, want an invisible cursor", g)
			}
			continue
		}
		// Glyphs are even (odd indices are the masks) and below 154.
		if !ok || g%2 != 0 || g > xcXterm {
			t.Errorf("cursor %v maps to glyph %d (ok %v)", c, g, ok)
		}
	}
	for c, want := range map[platform.Cursor]uint16{
		platform.CursorDefault:     xcLeftPtr,
		platform.CursorPointer:     xcHand2,
		platform.CursorText:        xcXterm,
		platform.CursorWait:        xcWatch,
		platform.CursorCrosshair:   xcCrosshair,
		platform.CursorMove:        xcFleur,
		platform.CursorEWResize:    xcSbHDoubleArrow,
		platform.CursorNSResize:    xcSbVDoubleArrow,
		platform.CursorNWResize:    xcTopLeftCorner,
		platform.CursorSEResize:    xcBottomRightCorn,
		platform.CursorZoomIn:      xcLeftPtr, // no glyph: default
		platform.CursorContextMenu: xcLeftPtr,
	} {
		if g, _ := cursorGlyph(c); g != want {
			t.Errorf("cursorGlyph(%v) = %d, want %d", c, g, want)
		}
	}
}

// TestCursorFailureIsCached: without the core cursor font, cursorFor returns
// None and caches it, and it allocates no cursor XID (the fake backend has
// no connection, so an allocation would panic). The old code allocated an
// XID first and retried on every call, leaking one XID per SetCursor.
func TestCursorFailureIsCached(t *testing.T) {
	b, _ := fakeBackend(1)
	b.cursors = map[platform.Cursor]xproto.Cursor{}
	b.cursorFt, b.fontOK = 1, false // the font failed to open earlier
	for range 3 {
		if id := b.cursorFor(platform.CursorText); id != 0 {
			t.Fatalf("cursorFor without a cursor font = %d, want 0 (None)", id)
		}
	}
	if id, ok := b.cursors[platform.CursorText]; !ok || id != 0 {
		t.Errorf("failure not cached: cursors[CursorText] = %d, %v", id, ok)
	}
}

func TestScaleFromResources(t *testing.T) {
	for _, tc := range []struct {
		db   string
		want float64
	}{
		{"", 1},
		{"Xft.dpi:\t96\n", 1},
		{"Xft.dpi:\t144\n", 1.5},
		{"Xft.antialias:\t1\nXft.dpi:\t192\nXft.hinting:\t1\n", 2},
		{"Xft.dpi: 120.0", 1.25},
		{"*dpi:\t192\n", 2},
		{"Xft*dpi:\t144\n", 1.5},
		{"*.dpi:\t144\n", 1.5},
		{"?.dpi:\t144\n", 1.5},
		{"Xft.Dpi:\t144\n", 1.5}, // class
		// The more specific entry wins regardless of order.
		{"Xft.dpi:\t144\n*dpi:\t192\n", 1.5},
		{"*dpi:\t192\nXft.dpi:\t144\n", 1.5},
		{"*dpi:\t192\nXft*dpi:\t144\n", 1.5},
		// Later entries win ties.
		{"Xft.dpi:\t144\nXft.dpi:\t192\n", 2},
		// Not Xft.dpi.
		{"Xft.dpix:\t144\n", 1},
		{"Foo.dpi:\t144\n", 1},
		{"Xft.dpi.extra:\t144\n", 1},
		{"dpi:\t144\n", 1},
		{"! Xft.dpi:\t144\n", 1},
		// Unusable values fall back to 1.
		{"Xft.dpi:\tlarge\n", 1},
		{"Xft.dpi:\t0\n", 1},
		{"Xft.dpi:\t-96\n", 1},
		{"Xft.dpi:\tNaN\n", 1},
		{"Xft.dpi:\t100000\n", 1},
	} {
		if got := scaleFromResources(tc.db); got != tc.want {
			t.Errorf("scaleFromResources(%q) = %v, want %v", tc.db, got, tc.want)
		}
	}
}

func TestSplitForRequests(t *testing.T) {
	for _, tc := range []struct {
		r        image.Rectangle
		maxBytes int
		header   int
	}{
		{image.Rect(0, 0, 1920, 1080), 65535 * 4, 24},
		{image.Rect(0, 0, 1920, 1080), 4 << 20, 28},
		{image.Rect(13, 7, 301, 229), 24 + 4*37, 24},  // rows do not fit: column runs
		{image.Rect(5, 5, 6, 6), 28, 24},              // one pixel per request
		{image.Rect(0, 0, 32767, 3), 4096 * 4, 24},    // a row wider than the minimum limit
		{image.Rect(100, 50, 200, 51), 65535 * 4, 24}, // single row
	} {
		t.Run(fmt.Sprint(tc.r, tc.maxBytes), func(t *testing.T) {
			chunks := splitForRequests(tc.r, tc.maxBytes, tc.header)
			area := 0
			for i, c := range chunks {
				if c.Empty() || !c.In(tc.r) {
					t.Fatalf("chunk %d %v is empty or outside %v", i, c, tc.r)
				}
				if n := tc.header + 4*c.Dx()*c.Dy(); n > tc.maxBytes {
					t.Fatalf("chunk %d %v needs %d bytes, limit %d", i, c, n, tc.maxBytes)
				}
				for j := range i {
					if !c.Intersect(chunks[j]).Empty() {
						t.Fatalf("chunks %d and %d overlap", i, j)
					}
				}
				area += c.Dx() * c.Dy()
			}
			if want := tc.r.Dx() * tc.r.Dy(); area != want {
				t.Fatalf("chunks cover %d pixels, want %d", area, want)
			}
		})
	}
	if got := splitForRequests(image.Rectangle{}, 1000, 24); got != nil {
		t.Errorf("empty rect split into %v", got)
	}
	// The common case is one request per damage rect.
	if got := splitForRequests(image.Rect(0, 0, 800, 600), 4<<20, 28); len(got) != 1 {
		t.Errorf("800x600 with a 4 MiB limit: %d requests, want 1", len(got))
	}
	// Without BIG-REQUESTS a 1920-wide frame goes in bands of 34 rows.
	if got := splitForRequests(image.Rect(0, 0, 1920, 1080), 65535*4, 24); len(got) != 32 || got[0].Dy() != 34 {
		t.Errorf("1920x1080 with a 256 KiB limit: %d requests of %d rows, want 32 of 34", len(got), got[0].Dy())
	}
}

func TestEncodePutImage(t *testing.T) {
	const pixW, pixH = 5, 4
	pix := make([]byte, 4*pixW*pixH)
	for i := range pix {
		pix[i] = byte(i)
	}
	r := image.Rect(1, 2, 4, 4) // 3x2
	le := binary.LittleEndian
	for _, big := range []bool{false, true} {
		buf := encodePutImage(nil, big, 0x1234, 0x5678, 24, pix, pixW, r)
		hdr := putImageHeaderLen(big)
		if len(buf) != hdr+4*3*2 {
			t.Fatalf("big=%v: request is %d bytes, want %d", big, len(buf), hdr+24)
		}
		if buf[0] != 72 || buf[1] != xproto.ImageFormatZPixmap {
			t.Errorf("big=%v: opcode/format = %d/%d", big, buf[0], buf[1])
		}
		o := 4
		if big {
			if le.Uint16(buf[2:]) != 0 || le.Uint32(buf[4:]) != uint32(len(buf)/4) {
				t.Errorf("big request length fields = %d, %d; want 0, %d", le.Uint16(buf[2:]), le.Uint32(buf[4:]), len(buf)/4)
			}
			o = 8
		} else if le.Uint16(buf[2:]) != uint16(len(buf)/4) {
			t.Errorf("request length = %d, want %d", le.Uint16(buf[2:]), len(buf)/4)
		}
		if le.Uint32(buf[o:]) != 0x1234 || le.Uint32(buf[o+4:]) != 0x5678 {
			t.Errorf("big=%v: drawable/gc wrong", big)
		}
		if le.Uint16(buf[o+8:]) != 3 || le.Uint16(buf[o+10:]) != 2 || le.Uint16(buf[o+12:]) != 1 || le.Uint16(buf[o+14:]) != 2 {
			t.Errorf("big=%v: geometry wrong: % x", big, buf[o+8:o+16])
		}
		if buf[o+16] != 0 || buf[o+17] != 24 {
			t.Errorf("big=%v: left-pad/depth = %d/%d", big, buf[o+16], buf[o+17])
		}
		data := buf[hdr:]
		for y := 0; y < 2; y++ {
			for x := 0; x < 3; x++ {
				src := 4 * ((r.Min.Y+y)*pixW + r.Min.X + x)
				dst := 4 * (y*3 + x)
				if string(data[dst:dst+4]) != string(pix[src:src+4]) {
					t.Fatalf("big=%v: pixel (%d,%d) = % x, want % x", big, x, y, data[dst:dst+4], pix[src:src+4])
				}
			}
		}
	}
	// The buffer is reused when large enough.
	buf := make([]byte, 0, 1000)
	if out := encodePutImage(buf, false, 1, 2, 24, pix, pixW, r); &out[0] != &buf[:1][0] {
		t.Errorf("encodePutImage did not reuse a large enough buffer")
	}
}

func TestMapButton(t *testing.T) {
	for _, tc := range []struct {
		detail uint32
		btn    platform.PointerButton
		steps  platform.Point
		ok     bool
	}{
		{1, platform.ButtonPrimary, platform.Point{}, true},
		{2, platform.ButtonMiddle, platform.Point{}, true},
		{3, platform.ButtonSecondary, platform.Point{}, true},
		{4, platform.ButtonNone, platform.Point{Y: -1}, true}, // wheel up: DOM negative
		{5, platform.ButtonNone, platform.Point{Y: 1}, true},
		{6, platform.ButtonNone, platform.Point{X: -1}, true},
		{7, platform.ButtonNone, platform.Point{X: 1}, true},
		{8, platform.ButtonBack, platform.Point{}, true},
		{9, platform.ButtonForward, platform.Point{}, true},
		{10, platform.ButtonNone, platform.Point{}, false},
		{0, platform.ButtonNone, platform.Point{}, false},
	} {
		btn, steps, ok := mapButton(tc.detail)
		if btn != tc.btn || steps != tc.steps || ok != tc.ok {
			t.Errorf("mapButton(%d) = %v, %v, %v; want %v, %v, %v", tc.detail, btn, steps, ok, tc.btn, tc.steps, tc.ok)
		}
	}
}

func TestStateMasks(t *testing.T) {
	state := uint16(xproto.KeyButMaskShift | xproto.KeyButMaskControl | xproto.KeyButMaskMod1 | xproto.KeyButMaskMod4 |
		xproto.KeyButMaskButton1 | xproto.KeyButMaskButton3)
	b, _ := fakeBackend(1) // no keyboard mapping: the xkeyboard-config defaults
	if m := b.kbd.mods(uint32(state)); m != platform.ModShift|platform.ModCtrl|platform.ModAlt|platform.ModSuper {
		t.Errorf("mods = %b", m)
	}
	if m := b.kbd.mods(xproto.KeyButMaskLock | xproto.KeyButMaskMod2); m != 0 {
		t.Errorf("Caps Lock and Num Lock reported as modifiers: %b", m)
	}
	if b := buttonsFromState(state); b != platform.ButtonPrimary.Mask()|platform.ButtonSecondary.Mask() {
		t.Errorf("buttonsFromState = %b", b)
	}
	if b := buttonsFromState(xproto.KeyButMaskButton2); b != platform.ButtonMiddle.Mask() {
		t.Errorf("Button2Mask = %b, want middle", b)
	}
}

func TestRealFocusChange(t *testing.T) {
	for _, tc := range []struct {
		mode, detail byte
		want         bool
	}{
		{xproto.NotifyModeNormal, xproto.NotifyDetailNonlinear, true},
		{xproto.NotifyModeNormal, xproto.NotifyDetailAncestor, true},
		{xproto.NotifyModeWhileGrabbed, xproto.NotifyDetailNonlinear, true},
		{xproto.NotifyModeGrab, xproto.NotifyDetailNonlinear, false},
		{xproto.NotifyModeUngrab, xproto.NotifyDetailNonlinear, false},
		{xproto.NotifyModeNormal, xproto.NotifyDetailPointer, false},
		{xproto.NotifyModeNormal, xproto.NotifyDetailInferior, false},
	} {
		if got := realFocusChange(tc.mode, tc.detail); got != tc.want {
			t.Errorf("realFocusChange(mode %d, detail %d) = %v", tc.mode, tc.detail, got)
		}
	}
}

func TestProperties(t *testing.T) {
	le := binary.LittleEndian
	const always = hintPWinGravity // StaticGravity in every hint
	h := sizeHints(normalHints{size: image.Pt(800, 600), min: image.Pt(200, 100)})
	if len(h) != 18*4 || le.Uint32(h) != always|hintPMinSize || le.Uint32(h[20:]) != 200 || le.Uint32(h[24:]) != 100 || le.Uint32(h[28:]) != 0 || le.Uint32(h[68:]) != gravityStatic {
		t.Errorf("min-size hints = % x", h)
	}
	h = sizeHints(normalHints{size: image.Pt(800, 600), fixed: true})
	if le.Uint32(h) != always|hintPMinSize|hintPMaxSize || le.Uint32(h[20:]) != 800 || le.Uint32(h[28:]) != 800 || le.Uint32(h[32:]) != 600 {
		t.Errorf("fixed-size hints = % x", h)
	}
	if h = sizeHints(normalHints{size: image.Pt(800, 600)}); le.Uint32(h) != always {
		t.Errorf("no-constraint hints have flags %#x", le.Uint32(h))
	}

	if got := string(wmClass("dev.telaui.demo", "demo")); got != "dev.telaui.demo\x00dev.telaui.demo\x00" {
		t.Errorf("wmClass with AppID = %q", got)
	}
	if got := string(wmClass("", "shapes")); got != "shapes\x00Shapes\x00" {
		t.Errorf("wmClass without AppID = %q", got)
	}
	if got := string(wmClass("", "")); got != "tela\x00Tela\x00" {
		t.Errorf("wmClass without AppID or executable = %q", got)
	}
	if got := latin1("Café ✓ ü"); string(got) != "Caf\xe9 ? \xfc" {
		t.Errorf("latin1 = %q", got)
	}
}

// fakeBackend returns a backend without a connection, enough to test event
// translation and dispatch (which send no requests for input events).
//
// Window control requests go to a fakeServer (windowctl_test.go), which
// records them and answers property and position queries.
func fakeBackend(scale float64) (*Backend, *Window) {
	b := fakeEmpty(scale)
	w := fakeWindow(b, 7, image.Pt(300, 200), platform.WindowOptions{})
	b.srv.(*fakeServer).take()
	return b, w
}

// fakeEmpty returns a fake backend without windows.
func fakeEmpty(scale float64) *Backend {
	b := &Backend{
		start:   time.Now(),
		scale:   scale,
		notify:  make(chan struct{}, 1),
		windows: map[xproto.Window]*Window{},
		root:    1,
	}
	b.initLoop()
	b.srv = newFakeServer(b)
	b.readSupported()
	return b
}

type recorder struct{ events []string }

func (r *recorder) HandleEvent(_ platform.Window, e platform.Event) {
	var s string
	switch e := e.(type) {
	case platform.ConfigureEvent:
		s = "configure " + e.Metrics.String()
	case platform.FrameEvent:
		s = "frame"
	case platform.PointerEvent:
		s = fmt.Sprintf("pointer %v %v,%v btn=%d held=%b mods=%b", e.Action, e.Pos.X, e.Pos.Y, e.Button, e.Buttons, e.Mods)
	case platform.ScrollEvent:
		s = fmt.Sprintf("scroll %v,%v at %v,%v", e.Steps.X, e.Steps.Y, e.Pos.X, e.Pos.Y)
		if e.Delta != (platform.Point{}) {
			s += fmt.Sprintf(" delta %v,%v", e.Delta.X, e.Delta.Y)
		}
		if e.Mods != 0 {
			s += fmt.Sprintf(" mods=%b", e.Mods)
		}
		if e.Phase != platform.ScrollPhaseNone {
			// X11 has no gesture boundaries (xi2.go).
			s += " phase=" + e.Phase.String()
		}
	case platform.KeyEvent:
		s = fmt.Sprintf("key %v %v %q %q mods=%b", e.Action, e.Code, e.Key, e.Text, e.Mods)
		if e.Repeat {
			s += " repeat"
		}
		if e.Synthetic {
			s += " synthetic"
		}
		if e.Location != e.Code.Location() {
			s += " BAD-LOCATION"
		}
	case platform.FocusEvent:
		s = fmt.Sprintf("focus %v", e.Focused)
	case platform.CloseRequestedEvent:
		s = "close"
	case platform.WakeEvent:
		s = "wake"
	case platform.WindowStateEvent:
		s = "state " + e.State.String()
	default:
		s = fmt.Sprintf("%T", e)
	}
	r.events = append(r.events, s)
}

func (r *recorder) take() string {
	s := strings.Join(r.events, "; ")
	r.events = nil
	return s
}

func TestTranslateAndDispatch(t *testing.T) {
	b, w := fakeBackend(2)
	b.atoms.wmProtocols, b.atoms.wmDeleteWindow = 100, 101
	r := &recorder{}
	feed := func(evs ...xgb.Event) {
		for _, e := range evs {
			b.translate(xevent{ev: e})
		}
		b.dispatch(r)
	}

	feed()
	if got, want := r.take(), "configure 150x100@2x (300x200 px); frame"; got != want {
		t.Fatalf("initial dispatch = %q, want %q", got, want)
	}

	// Repeated ConfigureNotify coalesces into one ConfigureEvent with the
	// latest size; a ConfigureNotify that only moves the window is
	// ignored.
	feed(
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, Width: 400, Height: 300},
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, Width: 500, Height: 301},
		xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 50, Width: 500, Height: 301},
	)
	if got, want := r.take(), "configure 250x150.5@2x (500x301 px); frame"; got != want {
		t.Errorf("configure batch = %q, want %q", got, want)
	}
	feed(xproto.ConfigureNotifyEvent{Event: 7, Window: 7, X: 10, Y: 10, Width: 500, Height: 301})
	if got := r.take(); got != "" {
		t.Errorf("a move delivered %q", got)
	}

	// Enter, motion coalescing, press/release with state, wheel.
	feed(
		xproto.EnterNotifyEvent{Event: 7, EventX: 10, EventY: 20},
		xproto.EnterNotifyEvent{Event: 7, EventX: 10, EventY: 20}, // duplicate
		xproto.MotionNotifyEvent{Event: 7, EventX: 11, EventY: 21},
		xproto.MotionNotifyEvent{Event: 7, EventX: 12, EventY: 22},
		xproto.MotionNotifyEvent{Event: 7, EventX: 14, EventY: 24, State: xproto.KeyButMaskShift},
		xproto.ButtonPressEvent{Event: 7, Detail: 1, EventX: 14, EventY: 24},
		xproto.MotionNotifyEvent{Event: 7, EventX: 16, EventY: 26, State: xproto.KeyButMaskButton1},
		xproto.ButtonPressEvent{Event: 7, Detail: 8, EventX: 16, EventY: 26, State: xproto.KeyButMaskButton1},
		xproto.ButtonReleaseEvent{Event: 7, Detail: 1, EventX: 16, EventY: 26, State: xproto.KeyButMaskButton1},
		xproto.ButtonReleaseEvent{Event: 7, Detail: 8, EventX: 16, EventY: 26},
		xproto.ButtonPressEvent{Event: 7, Detail: 4, EventX: 16, EventY: 26, State: xproto.KeyButMaskControl},
		xproto.ButtonReleaseEvent{Event: 7, Detail: 4, EventX: 16, EventY: 26},
		xproto.ButtonPressEvent{Event: 7, Detail: 7, EventX: 16, EventY: 26},
		xproto.LeaveNotifyEvent{Event: 7, EventX: -2, EventY: 26},
		xproto.LeaveNotifyEvent{Event: 7, EventX: -2, EventY: 26}, // duplicate
	)
	want := strings.Join([]string{
		"pointer enter 5,10 btn=0 held=0 mods=0",
		"pointer move 6,11 btn=0 held=0 mods=0",
		"pointer move 7,12 btn=0 held=0 mods=1",
		"pointer down 7,12 btn=1 held=1 mods=0",
		"pointer move 8,13 btn=0 held=1 mods=0",
		"pointer down 8,13 btn=4 held=1001 mods=0",
		"pointer up 8,13 btn=1 held=1000 mods=0",
		"pointer up 8,13 btn=4 held=0 mods=0",
		"scroll 0,-1 at 8,13 mods=10",
		"scroll 1,0 at 8,13",
		"pointer leave -1,13 btn=0 held=0 mods=0",
	}, "; ")
	if got := r.take(); got != want {
		t.Errorf("pointer batch:\n got %s\nwant %s", got, want)
	}

	// Focus: grab-mode and pointer-detail events are filtered, duplicates
	// dropped.
	feed(
		xproto.FocusInEvent{Event: 7, Mode: xproto.NotifyModeGrab, Detail: xproto.NotifyDetailNonlinear},
		xproto.FocusInEvent{Event: 7, Mode: xproto.NotifyModeNormal, Detail: xproto.NotifyDetailPointer},
		xproto.FocusInEvent{Event: 7, Mode: xproto.NotifyModeNormal, Detail: xproto.NotifyDetailNonlinear},
		xproto.FocusInEvent{Event: 7, Mode: xproto.NotifyModeNormal, Detail: xproto.NotifyDetailNonlinear},
		xproto.FocusOutEvent{Event: 7, Mode: xproto.NotifyModeUngrab, Detail: xproto.NotifyDetailNonlinear},
		xproto.FocusOutEvent{Event: 7, Mode: xproto.NotifyModeNormal, Detail: xproto.NotifyDetailNonlinear},
	)
	if got, want := r.take(), "focus true; focus false"; got != want {
		t.Errorf("focus batch = %q, want %q", got, want)
	}

	// WM_DELETE_WINDOW.
	feed(xproto.ClientMessageEvent{Window: 7, Type: 100, Format: 32, Data: xproto.ClientMessageDataUnionData32New([]uint32{101, 0, 0, 0, 0})})
	if got := r.take(); got != "close" {
		t.Errorf("WM_DELETE_WINDOW delivered %q, want close", got)
	}

	// Events for unknown windows are ignored.
	feed(xproto.ButtonPressEvent{Event: 99, Detail: 1}, xproto.ConfigureNotifyEvent{Event: 99, Window: 99, Width: 1, Height: 1})
	if got := r.take(); got != "" {
		t.Errorf("events for an unknown window delivered %q", got)
	}

	// Frames: a request is delivered now if no frame was delivered within
	// the interval, else paced; an iconified window gets none.
	b.start = b.start.Add(-2 * FrameInterval) // the last frame is two intervals old
	w.RequestFrame()
	w.RequestFrame()
	feed()
	if got := r.take(); got != "frame" {
		t.Errorf("RequestFrame after idle delivered %q, want one frame", got)
	}
	w.RequestFrame()
	feed()
	if got := r.take(); got != "" {
		t.Errorf("RequestFrame right after a frame delivered %q immediately, want pacing", got)
	}
	if d, ok := b.nextDeadline(); !ok || d != w.LastFrame()+FrameInterval {
		t.Errorf("nextDeadline = %v, %v; want lastFrame + interval", d, ok)
	}
	w.mapped = true
	feed(xproto.UnmapNotifyEvent{Event: 7, Window: 7})
	if _, ok := b.nextDeadline(); ok || b.ready() {
		t.Errorf("an iconified window keeps the frame timer armed")
	}
	feed(xproto.MapNotifyEvent{Event: 7, Window: 7})
	if _, ok := b.nextDeadline(); !ok {
		t.Errorf("the pending frame request was lost while iconified")
	}

	// No frame request: no deadline (0% CPU when idle). The pending
	// request is consumed by delivering its frame once it is due.
	b.start = b.start.Add(-FrameInterval)
	feed()
	if got := r.take(); got != "frame" {
		t.Errorf("the paced frame delivered %q, want frame", got)
	}
	if _, ok := b.nextDeadline(); ok || b.ready() {
		t.Errorf("idle backend has a deadline or is ready")
	}
}

func TestLeaveDeferredDuringImplicitGrab(t *testing.T) {
	b, _ := fakeBackend(1)
	r := &recorder{}
	feed := func(evs ...xgb.Event) string {
		for _, e := range evs {
			b.translate(xevent{ev: e})
		}
		b.dispatch(r)
		return r.take()
	}
	feed()
	const b1 = xproto.KeyButMaskButton1

	// Drag out of the window and release outside: the leave follows the
	// release.
	got := feed(
		xproto.EnterNotifyEvent{Event: 7, EventX: 10, EventY: 10},
		xproto.ButtonPressEvent{Event: 7, Detail: 1, EventX: 10, EventY: 10},
		xproto.LeaveNotifyEvent{Event: 7, EventX: -1, EventY: 10, State: b1},
		xproto.MotionNotifyEvent{Event: 7, EventX: -20, EventY: 10, State: b1},
		xproto.ButtonReleaseEvent{Event: 7, Detail: 1, EventX: -20, EventY: 10, State: b1},
		xproto.LeaveNotifyEvent{Event: 7, EventX: -20, EventY: 10, Mode: xproto.NotifyModeUngrab}, // ignored
	)
	want := "pointer enter 10,10 btn=0 held=0 mods=0; pointer down 10,10 btn=1 held=1 mods=0; " +
		"pointer move -20,10 btn=0 held=1 mods=0; pointer up -20,10 btn=1 held=0 mods=0; pointer leave -20,10 btn=0 held=0 mods=0"
	if got != want {
		t.Errorf("drag out:\n got %s\nwant %s", got, want)
	}

	// Drag out and back in before releasing: no leave, no enter.
	got = feed(
		xproto.EnterNotifyEvent{Event: 7, EventX: 10, EventY: 10},
		xproto.ButtonPressEvent{Event: 7, Detail: 1, EventX: 10, EventY: 10},
		xproto.LeaveNotifyEvent{Event: 7, EventX: -1, EventY: 10, State: b1},
		xproto.EnterNotifyEvent{Event: 7, EventX: 1, EventY: 10, State: b1},
		xproto.ButtonReleaseEvent{Event: 7, Detail: 1, EventX: 5, EventY: 10, State: b1},
	)
	want = "pointer enter 10,10 btn=0 held=0 mods=0; pointer down 10,10 btn=1 held=1 mods=0; pointer up 5,10 btn=1 held=0 mods=0"
	if got != want {
		t.Errorf("drag out and back:\n got %s\nwant %s", got, want)
	}
}

// TestPointerGrabCancelsPress: when another grab takes the pointer while
// the app has a press (the window manager's Alt+drag, a screenshot tool),
// the release never reaches the window. The press is cancelled before the
// leave, like on Wayland, and no held button survives it.
func TestPointerGrabCancelsPress(t *testing.T) {
	b, _ := fakeBackend(1)
	r := &recorder{}
	feed := func(evs ...xgb.Event) string {
		for _, e := range evs {
			b.translate(xevent{ev: e})
		}
		b.dispatch(r)
		return r.take()
	}
	feed()
	const b1 = xproto.KeyButMaskButton1

	// Primary and back held, then a NotifyGrab leave: cancel, leave with
	// nothing held. Back (tracked outside the core state) is forgotten,
	// so later events do not report it.
	got := feed(
		xproto.EnterNotifyEvent{Event: 7, EventX: 10, EventY: 10},
		xproto.ButtonPressEvent{Event: 7, Detail: 1, EventX: 10, EventY: 10},
		xproto.ButtonPressEvent{Event: 7, Detail: 8, EventX: 10, EventY: 10, State: b1},
		xproto.LeaveNotifyEvent{Event: 7, EventX: 12, EventY: 10, State: b1, Mode: xproto.NotifyModeGrab},
		xproto.EnterNotifyEvent{Event: 7, EventX: 30, EventY: 20, Mode: xproto.NotifyModeUngrab},
		xproto.MotionNotifyEvent{Event: 7, EventX: 31, EventY: 20},
	)
	want := "pointer enter 10,10 btn=0 held=0 mods=0; pointer down 10,10 btn=1 held=1 mods=0; " +
		"pointer down 10,10 btn=4 held=1001 mods=0; " +
		"pointer cancel 12,10 btn=0 held=0 mods=0; pointer leave 12,10 btn=0 held=0 mods=0; " +
		"pointer enter 30,20 btn=0 held=0 mods=0; pointer move 31,20 btn=0 held=0 mods=0"
	if got != want {
		t.Errorf("grab with buttons held:\n got %s\nwant %s", got, want)
	}

	// A grab without a press is a plain leave (the Ungrab enter brings
	// the pointer back).
	got = feed(
		xproto.LeaveNotifyEvent{Event: 7, EventX: 31, EventY: 20, Mode: xproto.NotifyModeGrab},
		xproto.EnterNotifyEvent{Event: 7, EventX: 31, EventY: 20, Mode: xproto.NotifyModeUngrab},
	)
	if want := "pointer leave 31,20 btn=0 held=0 mods=0; pointer enter 31,20 btn=0 held=0 mods=0"; got != want {
		t.Errorf("grab without a press:\n got %s\nwant %s", got, want)
	}
}

// TestPointerGrabAfterDeferredLeave: the pointer left during the implicit
// grab (leave deferred to the release), then a grab stole the release. The
// app must still get the cancel and the leave, and the next enter must not
// be swallowed as "back inside before the release".
func TestPointerGrabAfterDeferredLeave(t *testing.T) {
	const b1 = xproto.KeyButMaskButton1
	press := []xgb.Event{
		xproto.EnterNotifyEvent{Event: 7, EventX: 10, EventY: 10},
		xproto.ButtonPressEvent{Event: 7, Detail: 1, EventX: 10, EventY: 10},
		xproto.LeaveNotifyEvent{Event: 7, EventX: -1, EventY: 10, State: b1}, // deferred
		xproto.MotionNotifyEvent{Event: 7, EventX: -5, EventY: 10, State: b1},
	}
	const pressed = "pointer enter 10,10 btn=0 held=0 mods=0; pointer down 10,10 btn=1 held=1 mods=0; " +
		"pointer move -5,10 btn=0 held=1 mods=0"
	for _, tc := range []struct {
		name string
		then []xgb.Event
		want string
	}{
		{
			// The grab ends ours: an Ungrab leave while the leave is
			// deferred, then the pointer comes back normally.
			name: "ungrab leave, then enter",
			then: []xgb.Event{
				xproto.LeaveNotifyEvent{Event: 7, EventX: -6, EventY: 10, Mode: xproto.NotifyModeUngrab},
				xproto.EnterNotifyEvent{Event: 7, EventX: 4, EventY: 10},
			},
			want: "; pointer cancel -6,10 btn=0 held=0 mods=0; pointer leave -6,10 btn=0 held=0 mods=0; " +
				"pointer enter 4,10 btn=0 held=0 mods=0",
		},
		{
			// No leave at all: the next enter shows no button held (or
			// comes from an ungrab), so the release was lost.
			name: "enter without buttons",
			then: []xgb.Event{
				xproto.EnterNotifyEvent{Event: 7, EventX: 4, EventY: 10},
			},
			want: "; pointer cancel -5,10 btn=0 held=0 mods=0; pointer leave -5,10 btn=0 held=0 mods=0; " +
				"pointer enter 4,10 btn=0 held=0 mods=0",
		},
		{
			name: "ungrab enter with the button still down",
			then: []xgb.Event{
				xproto.EnterNotifyEvent{Event: 7, EventX: 4, EventY: 10, State: b1, Mode: xproto.NotifyModeUngrab},
			},
			want: "; pointer cancel -5,10 btn=0 held=0 mods=0; pointer leave -5,10 btn=0 held=0 mods=0; " +
				"pointer enter 4,10 btn=0 held=1 mods=0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, w := fakeBackend(1)
			r := &recorder{}
			b.dispatch(r)
			r.take()
			for _, e := range append(append([]xgb.Event(nil), press...), tc.then...) {
				b.translate(xevent{ev: e})
			}
			b.dispatch(r)
			if got := r.take(); got != pressed+tc.want {
				t.Errorf("\n got %s\nwant %s", got, pressed+tc.want)
			}
			if w.leavePending || w.extra != 0 || !w.inside {
				t.Errorf("state after the grab: leavePending %v, extra %b, inside %v", w.leavePending, w.extra, w.inside)
			}
		})
	}
}

func TestScaleChangeReconfigures(t *testing.T) {
	b, w := fakeBackend(1)
	r := &recorder{}
	b.dispatch(r)
	r.take()
	// Simulate what updateScale does after reading a new Xft.dpi.
	b.scale = 1.5
	b.queueConfigure(w)
	b.dispatch(r)
	if got, want := r.take(), "configure 200x133.33333@1.5x (300x200 px); frame"; got != want {
		t.Errorf("scale change = %q, want %q", got, want)
	}
	if !w.NeedsFull() {
		t.Errorf("the Present after a ConfigureEvent is not forced full")
	}
}

func TestClipboardIsAsyncAndUnsupported(t *testing.T) {
	b, _ := fakeBackend(1)
	cb := b.Clipboard()
	if err := cb.WriteText("x"); !errors.Is(err, ErrClipboardUnsupported) {
		t.Errorf("WriteText = %v", err)
	}
	called := false
	cb.ReadText(func(text string, err error) {
		called = true
		if text != "" || !errors.Is(err, ErrClipboardUnsupported) {
			t.Errorf("ReadText callback got %q, %v", text, err)
		}
	})
	if called {
		t.Fatalf("ReadText called its callback synchronously")
	}
	b.dispatch(&recorder{})
	if !called {
		t.Errorf("ReadText callback never ran")
	}
}
