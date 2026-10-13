package win32

import (
	"image"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// The tests in this file are about the pure logic of the backend and run on
// every OS; live_test.go (Windows only) runs the real backend.

func TestScaleFromDPI(t *testing.T) {
	for dpi, want := range map[uint32]float64{0: 1, 96: 1, 120: 1.25, 144: 1.5, 168: 1.75, 192: 2, 288: 3, 384: 4} {
		if got := scaleFromDPI(dpi); got != want {
			t.Errorf("scaleFromDPI(%d) = %v, want %v", dpi, got, want)
		}
	}
}

func TestClampPhysical(t *testing.T) {
	for _, tc := range []struct{ in, want image.Point }{
		{image.Pt(800, 600), image.Pt(800, 600)},
		{image.Pt(0, 0), image.Pt(1, 1)},
		{image.Pt(-5, 10), image.Pt(1, 10)},
		{image.Pt(1e6, 2e6), image.Pt(maxWindowSide, maxWindowSide)},
	} {
		if got := clampPhysical(tc.in); got != tc.want {
			t.Errorf("clampPhysical(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestMetricsRoundTrip: the logical size derived from a client rect maps back
// to the same framebuffer size at every DPI Windows offers (100% to 500%
// plus the odd custom values), which Present's size check depends on.
func TestMetricsRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, dpi := range []uint32{96, 108, 120, 125, 144, 168, 192, 216, 240, 288, 384, 480} {
		for range 500 {
			client := image.Pt(1+rng.Intn(4096), 1+rng.Intn(2304))
			m := metricsFor(client, dpi)
			if got := m.Physical(); got != client {
				t.Fatalf("dpi %d: client %v -> metrics %v -> physical %v", dpi, client, m, got)
			}
			if m.Scale != scaleFromDPI(dpi) {
				t.Fatalf("dpi %d: scale %v", dpi, m.Scale)
			}
		}
	}
	// A window's requested logical size maps to the client size Present
	// expects, and back.
	for _, dpi := range []uint32{96, 120, 144, 192} {
		scale := scaleFromDPI(dpi)
		req := platform.Size{W: 800, H: 600}
		client := clampPhysical(platform.PhysicalSize(req, scale))
		if m := metricsFor(client, dpi); m.Size != req {
			t.Errorf("dpi %d: requested %v, client %v, reported %v", dpi, req, client, m.Size)
		}
	}
	// A minimized window has an empty client area: empty metrics, scale kept.
	if m := metricsFor(image.Point{}, 144); !m.Size.Empty() || m.Scale != 1.5 || m.Physical() != (image.Point{}) {
		t.Errorf("metricsFor(empty) = %v", m)
	}
}

func TestLogicalPoint(t *testing.T) {
	if got := logicalPoint(150, -30, 1.5); got != (platform.Point{X: 100, Y: -20}) {
		t.Errorf("logicalPoint(150, -30, 1.5) = %v", got)
	}
}

func TestRefreshInterval(t *testing.T) {
	for hz, want := range map[uint32]time.Duration{0: time.Second / 60, 1: time.Second / 60, 60: time.Second / 60, 144: time.Second / 144, 165: time.Second / 165, 5000: time.Second / 60} {
		if got := refreshInterval(hz); got != want {
			t.Errorf("refreshInterval(%d) = %v, want %v", hz, got, want)
		}
	}
}

func TestCursorResource(t *testing.T) {
	for c := platform.CursorDefault; c.Valid(); c++ {
		idc, ok := cursorResource(c)
		if c == platform.CursorNone {
			if ok {
				t.Errorf("CursorNone maps to IDC %d, want a hidden cursor", idc)
			}
			continue
		}
		if !ok || idc < 32512 || idc > 32651 {
			t.Errorf("cursor %v maps to IDC %d (ok %v), want a system cursor id", c, idc, ok)
		}
	}
	for c, want := range map[platform.Cursor]uintptr{
		platform.CursorDefault:     idcArrow,
		platform.CursorPointer:     idcHand,
		platform.CursorText:        idcIBeam,
		platform.CursorWait:        idcWait,
		platform.CursorProgress:    idcAppStarting,
		platform.CursorCrosshair:   idcCross,
		platform.CursorMove:        idcSizeAll,
		platform.CursorNotAllowed:  idcNo,
		platform.CursorHelp:        idcHelp,
		platform.CursorEWResize:    idcSizeWE,
		platform.CursorColResize:   idcSizeWE,
		platform.CursorNSResize:    idcSizeNS,
		platform.CursorRowResize:   idcSizeNS,
		platform.CursorNESWResize:  idcSizeNESW,
		platform.CursorNWSEResize:  idcSizeNWSE,
		platform.CursorNEResize:    idcSizeNESW,
		platform.CursorSEResize:    idcSizeNWSE,
		platform.CursorZoomIn:      idcArrow, // no system cursor
		platform.CursorContextMenu: idcArrow,
	} {
		if got, _ := cursorResource(c); got != want {
			t.Errorf("cursorResource(%v) = %d, want %d", c, got, want)
		}
	}
}

func TestPointFromLParam(t *testing.T) {
	pack := func(x, y int16) uintptr { return uintptr(uint16(x)) | uintptr(uint16(y))<<16 }
	for _, tc := range []struct{ x, y int16 }{{0, 0}, {10, 20}, {-1, -1}, {-32768, 32767}, {300, -5}} {
		x, y := pointFromLParam(pack(tc.x, tc.y))
		if x != int(tc.x) || y != int(tc.y) {
			t.Errorf("pointFromLParam(%d, %d) = %d, %d", tc.x, tc.y, x, y)
		}
	}
}

func TestButtonFromMessage(t *testing.T) {
	xb := func(n uint16) uintptr { return uintptr(n) << 16 }
	for _, tc := range []struct {
		msg  uint32
		wp   uintptr
		btn  platform.PointerButton
		down bool
		ok   bool
	}{
		{wmLButtonDown, 0, platform.ButtonPrimary, true, true},
		{wmLButtonUp, 0, platform.ButtonPrimary, false, true},
		{wmRButtonDown, 0, platform.ButtonSecondary, true, true},
		{wmRButtonUp, 0, platform.ButtonSecondary, false, true},
		{wmMButtonDown, 0, platform.ButtonMiddle, true, true},
		{wmMButtonUp, 0, platform.ButtonMiddle, false, true},
		{wmXButtonDown, xb(xButton1), platform.ButtonBack, true, true},
		{wmXButtonUp, xb(xButton1), platform.ButtonBack, false, true},
		{wmXButtonDown, xb(xButton2), platform.ButtonForward, true, true},
		{wmXButtonUp, xb(xButton2) | 0x0008, platform.ButtonForward, false, true}, // MK flags in the low word
		{wmXButtonDown, xb(3), platform.ButtonNone, false, false},
		{wmMouseWheel, 0, platform.ButtonNone, false, false},
	} {
		btn, down, ok := buttonFromMessage(tc.msg, tc.wp)
		if btn != tc.btn || down != tc.down || ok != tc.ok {
			t.Errorf("buttonFromMessage(%#x, %#x) = %v, %v, %v; want %v, %v, %v", tc.msg, tc.wp, btn, down, ok, tc.btn, tc.down, tc.ok)
		}
	}
}

func TestWheelSteps(t *testing.T) {
	wp := func(delta int16) uintptr { return uintptr(uint16(delta)) << 16 }
	for _, tc := range []struct {
		msg  uint32
		wp   uintptr
		want platform.Point
		ok   bool
	}{
		{wmMouseWheel, wp(120), platform.Point{Y: -1}, true},   // wheel away from the user: scroll up
		{wmMouseWheel, wp(-120), platform.Point{Y: 1}, true},   // toward the user: scroll down
		{wmMouseWheel, wp(-240), platform.Point{Y: 2}, true},   // two notches in one message
		{wmMouseWheel, wp(30), platform.Point{Y: -0.25}, true}, // high-resolution wheel
		{wmMouseHWheel, wp(120), platform.Point{X: 1}, true},   // tilt right: scroll right
		{wmMouseHWheel, wp(-60), platform.Point{X: -0.5}, true},
		{wmMouseWheel, 0x0008 | wp(120), platform.Point{Y: -1}, true}, // MK flags in the low word
		{wmLButtonDown, wp(120), platform.Point{}, false},
	} {
		got, ok := wheelSteps(tc.msg, tc.wp)
		if got != tc.want || ok != tc.ok {
			t.Errorf("wheelSteps(%#x, %#x) = %v, %v; want %v, %v", tc.msg, tc.wp, got, ok, tc.want, tc.ok)
		}
	}
}

func TestModifiersFrom(t *testing.T) {
	if got := modifiersFrom(false, false, false, false); got != 0 {
		t.Errorf("no keys: %v", got)
	}
	if got := modifiersFrom(true, true, true, true); got != platform.ModShift|platform.ModCtrl|platform.ModAlt|platform.ModSuper {
		t.Errorf("all keys: %v", got)
	}
	if got := modifiersFrom(false, true, false, false); got != platform.ModCtrl {
		t.Errorf("ctrl: %v", got)
	}
}

func TestSanitizeTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Tela":        "Tela",
		"a\x00b":      "ab",
		"bad\xffutf8": "bad�utf8",
		"héllo ✓":     "héllo ✓",
		"":            "",
	} {
		if got := sanitizeTitle(in); got != want {
			t.Errorf("sanitizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- retained copy ---

func randomRGBA(rng *rand.Rand, w, h int) *image.RGBA {
	fb := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(fb.Pix); i += 4 {
		a := uint8(rng.Intn(256))
		// Premultiplied: color channels never exceed alpha.
		fb.Pix[i+0] = uint8(rng.Intn(int(a) + 1))
		fb.Pix[i+1] = uint8(rng.Intn(int(a) + 1))
		fb.Pix[i+2] = uint8(rng.Intn(int(a) + 1))
		fb.Pix[i+3] = a
	}
	return fb
}

func TestDIBUpdateConvertsAndKeepsUndamagedPixels(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const w, h = 37, 23
	var d dib
	if !d.resize(image.Pt(w, h)) {
		t.Fatal("resize of an empty dib reported no change")
	}
	if d.resize(image.Pt(w, h)) {
		t.Fatal("resize to the same size reported a change")
	}
	first := randomRGBA(rng, w, h)
	d.update(first, []image.Rectangle{first.Rect})
	second := randomRGBA(rng, w, h)
	damage := image.Rect(5, 3, 20, 11)
	d.update(second, []image.Rectangle{damage})

	for y := range h {
		for x := range w {
			src := first
			if image.Pt(x, y).In(damage) {
				src = second
			}
			i := src.PixOffset(x, y)
			want := [4]byte{src.Pix[i+2], src.Pix[i+1], src.Pix[i], src.Pix[i+3]} // B, G, R, A
			j := y*d.stride() + 4*x
			if got := [4]byte(d.pix[j : j+4]); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v (damaged: %v)", x, y, got, want, image.Pt(x, y).In(damage))
			}
		}
	}
}

func TestDIBResizeDiscardsContent(t *testing.T) {
	var d dib
	d.resize(image.Pt(4, 4))
	d.pix[0] = 9
	if !d.resize(image.Pt(5, 4)) || d.pix[0] != 0 || len(d.pix) != 4*5*4 {
		t.Errorf("resize to a new size kept stale content: changed, pix[0]=%d, len %d", d.pix[0], len(d.pix))
	}
	d.release()
	if d.pix != nil || d.size != (image.Point{}) {
		t.Errorf("release kept %d bytes at %v", len(d.pix), d.size)
	}
	if _, bits, _ := d.region(image.Rect(0, 0, 1, 1)); bits != nil {
		t.Errorf("region of a released dib returned bits")
	}
}

func TestDIBRegion(t *testing.T) {
	var d dib
	d.resize(image.Pt(10, 8))
	for i := range d.pix {
		d.pix[i] = byte(i)
	}
	clipped, bits, hdr := d.region(image.Rect(3, 2, 7, 5))
	if clipped != image.Rect(3, 2, 7, 5) {
		t.Errorf("clipped = %v", clipped)
	}
	// The bits start at the first row of the rectangle, in whole rows.
	if len(bits) != len(d.pix)-2*d.stride() || bits[0] != d.pix[2*d.stride()] {
		t.Errorf("bits start at the wrong row: len %d, first %d", len(bits), bits[0])
	}
	if hdr.size != 40 || hdr.width != 10 || hdr.height != -3 || hdr.planes != 1 || hdr.bitCount != 32 || hdr.compression != biRGB {
		t.Errorf("header = %+v, want a top-down 32-bit BI_RGB bitmap 10 wide and 3 tall", hdr)
	}
	// Out-of-bounds rectangles are clipped; disjoint ones vanish.
	if clipped, _, hdr := d.region(image.Rect(-5, -5, 100, 100)); clipped != image.Rect(0, 0, 10, 8) || hdr.height != -8 {
		t.Errorf("clipped = %v, header height %d", clipped, hdr.height)
	}
	if clipped, bits, _ := d.region(image.Rect(10, 0, 20, 8)); !clipped.Empty() || bits != nil {
		t.Errorf("a rectangle outside the dib gave %v", clipped)
	}
}

// --- pointer state machine ---

type recorder struct{ events []platform.PointerEvent }

func (r *recorder) emit(e platform.PointerEvent) { r.events = append(r.events, e) }

func (r *recorder) actions() []platform.PointerAction {
	var out []platform.PointerAction
	for _, e := range r.events {
		out = append(out, e.Action)
	}
	return out
}

func (r *recorder) reset() { r.events = nil }

func at(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }

const (
	mv     = platform.PointerMove
	enter  = platform.PointerEnter
	leave  = platform.PointerLeave
	down   = platform.PointerDown
	up     = platform.PointerUp
	cancel = platform.PointerCancel
)

func TestPointerEnterMoveLeave(t *testing.T) {
	var p pointerState
	var r recorder
	if track := p.move(at(5, 5), true, r.emit); !track {
		t.Error("a move inside must arm WM_MOUSELEAVE tracking")
	}
	p.move(at(6, 5), true, r.emit)
	p.leaveMessage(r.emit)
	p.leaveMessage(r.emit) // a second leave is dropped
	if got, want := r.actions(), []platform.PointerAction{enter, mv, mv, leave}; !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if r.events[0].Pos != at(5, 5) || r.events[3].Pos != at(6, 5) {
		t.Errorf("positions: enter %v, leave %v", r.events[0].Pos, r.events[3].Pos)
	}
}

func TestPointerPressReleaseCapture(t *testing.T) {
	var p pointerState
	var r recorder
	p.move(at(1, 1), true, r.emit)
	r.reset()

	if got := p.button(platform.ButtonPrimary, true, at(1, 1), r.emit); got != captureSet {
		t.Errorf("first press capture = %v, want set", got)
	}
	if got := p.button(platform.ButtonSecondary, true, at(1, 1), r.emit); got != captureKeep {
		t.Errorf("second press capture = %v, want keep", got)
	}
	if got := p.button(platform.ButtonPrimary, false, at(1, 1), r.emit); got != captureKeep {
		t.Errorf("release with a button still held capture = %v, want keep", got)
	}
	if got := p.button(platform.ButtonSecondary, false, at(1, 1), r.emit); got != captureRelease {
		t.Errorf("last release capture = %v, want release", got)
	}
	if got, want := r.actions(), []platform.PointerAction{down, down, up, up}; !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	// Buttons is the set held after the event.
	both := platform.ButtonPrimary.Mask() | platform.ButtonSecondary.Mask()
	for i, want := range []platform.Buttons{platform.ButtonPrimary.Mask(), both, platform.ButtonSecondary.Mask(), 0} {
		if r.events[i].Buttons != want {
			t.Errorf("event %d Buttons = %b, want %b", i, r.events[i].Buttons, want)
		}
	}
	if r.events[1].Button != platform.ButtonSecondary || r.events[2].Button != platform.ButtonPrimary {
		t.Errorf("Button fields: %v, %v", r.events[1].Button, r.events[2].Button)
	}
}

func TestPointerPressWithoutMoveEntersFirst(t *testing.T) {
	var p pointerState
	var r recorder
	p.button(platform.ButtonPrimary, true, at(3, 3), r.emit)
	if got, want := r.actions(), []platform.PointerAction{enter, down}; !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}

func TestPointerReleaseOfForeignPressIsDropped(t *testing.T) {
	// A press that began on the title bar and ends over the client area.
	var p pointerState
	var r recorder
	p.move(at(1, 1), true, r.emit)
	r.reset()
	if got := p.button(platform.ButtonPrimary, false, at(1, 1), r.emit); got != captureKeep || len(r.events) != 0 {
		t.Errorf("release without press: capture %v, events %v", got, r.events)
	}
}

func TestPointerLeaveDuringDragIsDeferredToRelease(t *testing.T) {
	var p pointerState
	var r recorder
	p.move(at(1, 1), true, r.emit)
	p.button(platform.ButtonPrimary, true, at(1, 1), r.emit)
	r.reset()

	// The drag leaves the window: moves continue (captured), no leave yet.
	if track := p.move(at(-20, 5), false, r.emit); track {
		t.Error("a move outside must not arm tracking: Windows would send the leave at once")
	}
	p.leaveMessage(r.emit)
	p.move(at(-30, 5), false, r.emit)
	if got, want := r.actions(), []platform.PointerAction{mv, mv}; !slices.Equal(got, want) {
		t.Fatalf("during the drag: %v, want %v", got, want)
	}
	if r.events[1].Buttons != platform.ButtonPrimary.Mask() {
		t.Errorf("moves during the drag lost the held button: %b", r.events[1].Buttons)
	}
	// The release ends the stay: up, then the deferred leave at the
	// release position.
	r.reset()
	if got := p.button(platform.ButtonPrimary, false, at(-40, 5), r.emit); got != captureRelease {
		t.Errorf("capture = %v, want release", got)
	}
	if got, want := r.actions(), []platform.PointerAction{up, leave}; !slices.Equal(got, want) {
		t.Fatalf("at release: %v, want %v", got, want)
	}
	if r.events[1].Pos != at(-40, 5) || r.events[1].Buttons != 0 {
		t.Errorf("leave = %+v", r.events[1])
	}
	// Moving in later is a fresh enter.
	r.reset()
	p.move(at(2, 2), true, r.emit)
	if got, want := r.actions(), []platform.PointerAction{enter, mv}; !slices.Equal(got, want) {
		t.Errorf("after the drag: %v, want %v", got, want)
	}
}

func TestPointerReturnDuringDragForgetsTheLeave(t *testing.T) {
	var p pointerState
	var r recorder
	p.move(at(1, 1), true, r.emit)
	p.button(platform.ButtonPrimary, true, at(1, 1), r.emit)
	p.move(at(-5, 1), false, r.emit)
	p.move(at(4, 1), true, r.emit) // back inside before the release
	r.reset()
	p.button(platform.ButtonPrimary, false, at(4, 1), r.emit)
	if got, want := r.actions(), []platform.PointerAction{up}; !slices.Equal(got, want) {
		t.Errorf("actions = %v, want only %v: the app never saw a leave", got, want)
	}
}

func TestPointerCaptureLostCancelsThePress(t *testing.T) {
	var p pointerState
	var r recorder
	p.move(at(1, 1), true, r.emit)
	p.button(platform.ButtonPrimary, true, at(1, 1), r.emit)
	p.move(at(-9, 1), false, r.emit) // dragged out
	r.reset()
	p.captureLost(r.emit)
	if got, want := r.actions(), []platform.PointerAction{cancel, leave}; !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if r.events[0].Buttons != 0 || r.events[0].Pos != at(-9, 1) {
		t.Errorf("cancel = %+v", r.events[0])
	}
	// The release that follows (if any) is not ours.
	r.reset()
	if got := p.button(platform.ButtonPrimary, false, at(0, 0), r.emit); got != captureKeep || len(r.events) != 0 {
		t.Errorf("release after cancel: %v, %v", got, r.events)
	}
	// Losing the capture with nothing held (our own ReleaseCapture) is
	// silent.
	p.captureLost(r.emit)
	if len(r.events) != 0 {
		t.Errorf("captureLost with no buttons emitted %v", r.events)
	}
}

func TestPointerLeaveWithButtonsHeldInsideWindow(t *testing.T) {
	// WM_MOUSELEAVE while a button is held and the pointer is back inside
	// (a spurious leave): deferred, then forgotten by the next move inside.
	var p pointerState
	var r recorder
	p.move(at(1, 1), true, r.emit)
	p.button(platform.ButtonPrimary, true, at(1, 1), r.emit)
	r.reset()
	p.leaveMessage(r.emit)
	if track := p.move(at(2, 2), true, r.emit); !track {
		t.Error("the move back inside must re-arm tracking")
	}
	p.button(platform.ButtonPrimary, false, at(2, 2), r.emit)
	if got, want := r.actions(), []platform.PointerAction{mv, up}; !slices.Equal(got, want) {
		t.Errorf("actions = %v, want %v", got, want)
	}
}
