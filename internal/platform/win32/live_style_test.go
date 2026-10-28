//go:build windows

package win32

import (
	"image"
	"image/color"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of the window styling (ADR-187): the title bar colors and the
// backdrop are DWM attributes, and a backdrop in the client area needs the
// frame extended and the alpha of the frames presented. They run on a
// Windows 11 desktop and skip on older builds. A VM without a GPU may draw
// the backdrop as a solid fallback color, so the tests check what Tela
// controls (the attributes, the alpha of the retained copy and of what is
// presented) and, from the screen, only that a transparent pixel is not the
// black of an opaque one.

func requireBuild(t *testing.T, build uint32) {
	t.Helper()
	if got := windowsBuild(); got < build {
		t.Skipf("Windows build %d: this test needs %d or later", got, build)
	}
}

// dwmUint reads a 4-byte DWM window attribute.
func dwmUint(w *Window, attr uintptr) (v uint32, ok bool) {
	if !available(procDwmGetWindowAttribute) {
		return 0, false
	}
	hr, _, _ := procDwmGetWindowAttribute.Call(w.hwnd, attr, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return v, hr == 0
}

// presentHalves presents a frame whose left half is opaque red and whose
// right half is fully transparent (premultiplied {0,0,0,0}).
func presentHalves(t *testing.T, w platform.Window) {
	t.Helper()
	size := w.Metrics().Physical()
	fb := image.NewRGBA(image.Rectangle{Max: size})
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X/2; x++ {
			fb.SetRGBA(x, y, color.RGBA{0xff, 0, 0, 0xff})
		}
	}
	if err := w.Present(fb, nil); err != nil {
		t.Errorf("Present: %v", err)
	}
}

// settle lets DWM compose what was presented.
func settle() {
	for range 4 {
		procDwmFlush.Call()
		time.Sleep(60 * time.Millisecond)
	}
}

// TestLiveTitleBarColors: the colors reach DWM at creation and on change,
// and the system's are restored with the zero value. DWM does not give the
// caption color back through DwmGetWindowAttribute, so the title bar is read
// from the screen.
func TestLiveTitleBarColors(t *testing.T) {
	requireBuild(t, buildColors)
	b := liveBackend(t)
	var w *Window
	stage := 0
	navy := platform.WindowColor{R: 0x1e, G: 0x1b, B: 0x4b, Set: true}
	green := platform.WindowColor{R: 0x10, G: 0x80, B: 0x20, Set: true}
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "Colors", Size: platform.Size{W: 400, H: 200},
			TitleBar: platform.TitleBarColors{Background: navy}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || stage > 0 {
			return
		}
		stage++
		defer b.Quit()
		presentSolid(t, ew, color.RGBA{0xff, 0xff, 0xff, 0xff})
		// The caption halfway between the top of the window and of its
		// client area, at the middle of the window: away from its title,
		// icon and buttons.
		caption := func() color.RGBA {
			settle()
			r := getWindowRect(w.hwnd).image()
			var origin point
			procClientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&origin)))
			x, y := r.Min.X+r.Dx()/2, (r.Min.Y+int(origin.y))/2
			return captureScreenRect(image.Rect(x, y, x+1, y+1)).RGBAAt(0, 0)
		}
		near := func(a, b color.RGBA) bool {
			return absInt(int(a.R)-int(b.R)) <= 2 && absInt(int(a.G)-int(b.G)) <= 2 && absInt(int(a.B)-int(b.B)) <= 2
		}
		rgba := func(c platform.WindowColor) color.RGBA { return color.RGBA{c.R, c.G, c.B, 0xff} }
		if got := caption(); !near(got, rgba(navy)) {
			t.Errorf("title bar at creation = %02x%02x%02x, want %02x%02x%02x", got.R, got.G, got.B, navy.R, navy.G, navy.B)
		}
		w.SetTitleBarColors(platform.TitleBarColors{Background: green})
		if got := caption(); !near(got, rgba(green)) {
			t.Errorf("title bar after a change = %02x%02x%02x, want %02x%02x%02x", got.R, got.G, got.B, green.R, green.G, green.B)
		}
		w.SetTitleBarColors(platform.TitleBarColors{})
		if got := caption(); near(got, rgba(navy)) || near(got, rgba(green)) {
			t.Errorf("title bar after the zero value = %02x%02x%02x: still our color", got.R, got.G, got.B)
		}
	})
}

// TestLiveBackdropAttribute: the backdrop type reaches DWM at creation and
// on change.
func TestLiveBackdropAttribute(t *testing.T) {
	requireBuild(t, buildBackdrop)
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 120, H: 90}, Backdrop: platform.BackdropMicaAlt})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	defer w.Close()
	for _, bd := range []platform.Backdrop{platform.BackdropMicaAlt, platform.BackdropAcrylic, platform.BackdropNone, platform.BackdropMica} {
		w.SetBackdrop(bd)
		got, ok := dwmUint(w, dwmwaSystemBackdropType)
		if !ok {
			t.Skip("DwmGetWindowAttribute(DWMWA_SYSTEMBACKDROP_TYPE) is not supported here")
		}
		if want := uint32(backdropType(bd)); got != want {
			t.Errorf("SetBackdrop(%v): DWM has type %d, want %d", bd, got, want)
		}
		if w.PresentsAlpha() != bd.ClientBackdrop() {
			t.Errorf("SetBackdrop(%v): PresentsAlpha = %v", bd, w.PresentsAlpha())
		}
	}
}

// TestLiveBackdropAlpha: a window with a client-area backdrop presents the
// alpha of its frames: transparent where the app drew nothing, opaque where
// it did. Decorated and frameless windows alike.
func TestLiveBackdropAlpha(t *testing.T) {
	requireBuild(t, buildBackdrop)
	for _, frameless := range []bool{false, true} {
		b := liveBackend(t)
		var w *Window
		stage := 0
		runLive(t, b, func() {
			pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 160, H: 100}, Frameless: frameless, Backdrop: platform.BackdropMica})
			if err != nil {
				t.Fatal(err)
			}
			w = pw.(*Window)
		}, func(ew platform.Window, e platform.Event) {
			if _, ok := e.(platform.FrameEvent); !ok || stage > 0 {
				return
			}
			stage++
			defer b.Quit()
			if !w.PresentsAlpha() {
				t.Errorf("frameless=%v: a Mica window does not present alpha", frameless)
			}
			presentHalves(t, ew)
			size := w.Metrics().Physical()
			left, right := w.dib.pix[4*(size.Y/2*size.X+size.X/4)+3], w.dib.pix[4*(size.Y/2*size.X+3*size.X/4)+3]
			if left != 0xFF || right != 0 {
				t.Errorf("frameless=%v: retained alpha %#x on the opaque half and %#x on the transparent one, want 0xff and 0", frameless, left, right)
			}
			settle()
			var origin point
			procClientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&origin)))
			shot := captureScreenRect(image.Rect(int(origin.x), int(origin.y), int(origin.x)+size.X, int(origin.y)+size.Y))
			l, r := shot.RGBAAt(size.X/4, size.Y/2), shot.RGBAAt(3*size.X/4, size.Y/2)
			t.Logf("frameless=%v: screen shows %02x%02x%02x on the opaque half, %02x%02x%02x on the transparent one", frameless, l.R, l.G, l.B, r.R, r.G, r.B)
			if l.R < 0xe0 || l.G > 0x20 || l.B > 0x20 {
				t.Errorf("frameless=%v: the opaque half shows %02x%02x%02x, want red", frameless, l.R, l.G, l.B)
			}
			if r.R == 0 && r.G == 0 && r.B == 0 {
				t.Errorf("frameless=%v: the transparent half shows black: DWM did not treat the alpha of the pixels as transparent", frameless)
			}
			if r == l {
				t.Errorf("frameless=%v: the transparent half shows the opaque half's color", frameless)
			}
		})
	}
}

// TestLiveBackdropToggle: turning a client-area backdrop on and off at run
// time switches the retained copy between alpha and opaque, and asks for a
// full Present. A window without one keeps forcing opaque pixels (a
// transparent window after a session reconnect is worse), and a frameless
// one keeps its frame unextended (the snap layouts flyout).
func TestLiveBackdropToggle(t *testing.T) {
	requireBuild(t, buildBackdrop)
	for _, frameless := range []bool{false, true} {
		b := liveBackend(t)
		var w *Window
		stage := 0
		runLive(t, b, func() {
			pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 120, H: 90}, Frameless: frameless})
			if err != nil {
				t.Fatal(err)
			}
			w = pw.(*Window)
		}, func(ew platform.Window, e platform.Event) {
			if _, ok := e.(platform.FrameEvent); !ok || stage > 0 {
				return
			}
			stage++
			defer b.Quit()
			if w.PresentsAlpha() || w.glass || w.dib.alpha {
				t.Errorf("frameless=%v: a window without a backdrop presents alpha", frameless)
			}
			presentHalves(t, ew)
			size := w.Metrics().Physical()
			alphaAt := func(x int) byte { return w.dib.pix[4*(size.Y/2*size.X+x)+3] }
			if alphaAt(3*size.X/4) != 0xFF {
				t.Errorf("frameless=%v: a window without a backdrop kept alpha %#x", frameless, alphaAt(3*size.X/4))
			}
			if frameless {
				if _, ok := dwmUint(w, dwmwaSystemBackdropType); ok {
					if v, _ := dwmUint(w, dwmwaSystemBackdropType); v != dwmsbtAuto {
						t.Errorf("a window without a backdrop has DWM type %d", v)
					}
				}
			}
			w.SetBackdrop(platform.BackdropAcrylic)
			if !w.PresentsAlpha() || !w.dib.alpha || !w.NeedsFull() {
				t.Errorf("frameless=%v: after SetBackdrop(Acrylic): PresentsAlpha %v, dib.alpha %v, NeedsFull %v", frameless, w.PresentsAlpha(), w.dib.alpha, w.NeedsFull())
			}
			presentHalves(t, ew)
			if alphaAt(3*size.X/4) != 0 || alphaAt(size.X/4) != 0xFF {
				t.Errorf("frameless=%v: with Acrylic the alpha bytes are %#x and %#x", frameless, alphaAt(size.X/4), alphaAt(3*size.X/4))
			}
			// The composition changes: the margins and attributes are redone.
			sendMessage(w.hwnd, wmDwmCompositionChanged, 0, 0)
			if !w.PresentsAlpha() || !w.dib.alpha {
				t.Errorf("frameless=%v: a composition change dropped the backdrop", frameless)
			}
			w.SetBackdrop(platform.BackdropNone)
			if w.PresentsAlpha() || w.dib.alpha || !w.NeedsFull() {
				t.Errorf("frameless=%v: after SetBackdrop(None): PresentsAlpha %v, dib.alpha %v, NeedsFull %v", frameless, w.PresentsAlpha(), w.dib.alpha, w.NeedsFull())
			}
			if alphaAt(3*size.X/4) != 0xFF {
				t.Errorf("frameless=%v: leaving the backdrop left alpha %#x in the retained copy", frameless, alphaAt(3*size.X/4))
			}
			presentHalves(t, ew)
			if alphaAt(3*size.X/4) != 0xFF {
				t.Errorf("frameless=%v: after None the transparent half has alpha %#x", frameless, alphaAt(3*size.X/4))
			}
		})
	}
}

// TestLiveFramelessGlassKeepsAppTitleBar: a frameless window with a
// client-area backdrop must not get DWM's caption buttons over its own title
// bar (a non-zero top margin of the extended frame does that): the pixels at
// the top right are the app's.
func TestLiveFramelessGlassKeepsAppTitleBar(t *testing.T) {
	requireBuild(t, buildBackdrop)
	b := liveBackend(t)
	var w *Window
	stage := 0
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 400, H: 200}, Frameless: true, Backdrop: platform.BackdropMica})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || stage > 0 {
			return
		}
		stage++
		defer b.Quit()
		presentSolid(t, ew, color.RGBA{0xff, 0, 0, 0xff})
		settle()
		size := w.Metrics().Physical()
		var origin point
		procClientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&origin)))
		shot := captureScreenRect(image.Rect(int(origin.x), int(origin.y), int(origin.x)+size.X, int(origin.y)+size.Y))
		for _, p := range []image.Point{{size.X - 20, 10}, {size.X - 70, 10}, {size.X - 120, 10}, {size.X / 2, 10}, {10, 10}} {
			if c := shot.RGBAAt(p.X, p.Y); c.R < 0xe0 || c.G > 0x20 || c.B > 0x20 {
				t.Errorf("pixel %v = %02x%02x%02x, want the app's red: DWM drew over the title bar", p, c.R, c.G, c.B)
			}
		}
	})
}
