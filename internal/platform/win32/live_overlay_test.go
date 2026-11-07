//go:build windows

package win32

import (
	"image"
	"image/color"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of TitleBarOverlay windows (ADR-236): DWM keeps its caption
// buttons over the client area of a Windows 11 window, reports their
// rectangle, answers their hit test, and the app's title bar fills the
// rest. They need DWM composing an attended desktop (the buttons are read
// from the screen, and an unattended runner's DWM may not compose the
// window), so they run with TELA_LIVE=1, on Windows 11 only: elsewhere a
// TitleBarOverlay window is Frameless (overlaySupported).

func requireLiveOverlay(t *testing.T) {
	t.Helper()
	requireBuild(t, buildOverlay)
	requireLiveScreen(t)
}

// overlayRegions is the title bar of a 480 px wide overlay window: a
// caption along the top with a search field punched into it.
var overlayRegions = []platform.HitRegion{
	{Kind: platform.HitCaption, Rect: platform.Rect{W: 480, H: 32}},
	{Kind: platform.HitClient, Rect: platform.Rect{X: 100, Y: 4, W: 120, H: 24}},
}

// newOverlay creates a TitleBarOverlay window with overlayRegions.
func newOverlay(t *testing.T, b *Backend, opts platform.WindowOptions) *Window {
	t.Helper()
	opts.TitleBarOverlay = true
	if opts.Size == (platform.Size{}) {
		opts.Size = platform.Size{W: 480, H: 300}
	}
	pw, err := b.NewWindow(opts)
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	if !w.overlay || !w.opts.Frameless {
		t.Fatalf("a TitleBarOverlay window on build %d: overlay %v, frameless %v; want both", windowsBuild(), w.overlay, w.opts.Frameless)
	}
	w.SetHitRegions(overlayRegions)
	return w
}

// checkControls checks a reported TitleBarControls of a window whose
// logical client width is width: about the size of Windows 11's three
// caption buttons, at the top and flush with the right edge.
func checkControls(t *testing.T, what string, r platform.Rect, width float32) {
	t.Helper()
	switch {
	case r.W < 90 || r.W > 220 || r.H < 20 || r.H > 50:
		t.Errorf("%s: TitleBarControls %v: not the size of three caption buttons", what, r)
	case r.Y < 0 || r.Y > 4:
		t.Errorf("%s: TitleBarControls %v: not at the top of the client area", what, r)
	case r.X+r.W < width-2 || r.X+r.W > width+0.01:
		t.Errorf("%s: TitleBarControls %v: does not end at the right edge (%v)", what, r, width)
	}
}

// thirds returns the screen centers of the three buttons in r (minimize,
// maximize, close, which Windows lays out with the same width).
func thirds(w *Window, r platform.Rect) [3]image.Point {
	var out [3]image.Point
	for i := range out {
		out[i] = screenAt(w, float64(r.X+r.W*(float32(i)+0.5)/3), float64(r.Y+r.H/2))
	}
	return out
}

// checkButtonsHit checks that WM_NCHITTEST answers DWM's buttons in r.
func checkButtonsHit(t *testing.T, what string, w *Window, r platform.Rect) {
	t.Helper()
	for i, want := range [3]uintptr{htMinButton, htMaxButton, htClose} {
		p := thirds(w, r)[i]
		if got := sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(p)); got != want {
			t.Errorf("%s: WM_NCHITTEST at %v (button %d of %v) = %d, want %d", what, p, i, r, got, want)
		}
	}
}

// glyphPixels counts the pixels of the screen rectangle that differ from
// fill by more than a little: what DWM drew there.
func glyphPixels(r image.Rectangle, fill color.RGBA) int {
	img := captureScreenRect(r)
	n := 0
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			c := img.RGBAAt(x, y)
			if absInt(int(c.R)-int(fill.R))+absInt(int(c.G)-int(fill.G))+absInt(int(c.B)-int(fill.B)) > 60 {
				n++
			}
		}
	}
	return n
}

// screenRect returns the screen rectangle of a logical client rectangle.
func screenRect(w *Window, r platform.Rect) image.Rectangle {
	return image.Rectangle{Min: screenAt(w, float64(r.X), float64(r.Y)), Max: screenAt(w, float64(r.X+r.W), float64(r.Y+r.H))}
}

// checkDrawn reads the screen: DWM's buttons are drawn over the app's fill
// (the close button's glyph), and the rest of the title bar and the content
// are the app's pixels (no classic caption, no black strip).
func checkDrawn(t *testing.T, what string, w *Window, r platform.Rect, fill color.RGBA) {
	t.Helper()
	settle()
	closeBtn := platform.Rect{X: r.X + r.W*2/3, Y: r.Y, W: r.W / 3, H: r.H}
	if n := glyphPixels(screenRect(w, closeBtn), fill); n < 6 {
		t.Errorf("%s: %d pixels of the close button differ from the app's fill: DWM drew no button", what, n)
	}
	width := float64(w.Metrics().Size.W)
	for _, p := range [][2]float64{{12, 6}, {60, 16}, {width / 2, 2}, {width / 2, 16}, {width / 2, 31}, {float64(r.X) - 8, 16}, {width / 2, 150}} {
		sp := screenAt(w, p[0], p[1])
		if c := captureScreenRect(image.Rectangle{Min: sp, Max: sp.Add(image.Pt(1, 1))}).RGBAAt(0, 0); !nearColor(c, fill) {
			t.Errorf("%s: screen at %v,%v = %02x%02x%02x, want the app's %02x%02x%02x", what, p[0], p[1], c.R, c.G, c.B, fill.R, fill.G, fill.B)
		}
	}
}

func nearColor(a, b color.RGBA) bool {
	return absInt(int(a.R)-int(b.R)) <= 3 && absInt(int(a.G)-int(b.G)) <= 3 && absInt(int(a.B)-int(b.B)) <= 3
}

// TestLiveOverlay: the caption is client area (the side and bottom frame
// stay), DWM's buttons are reported, hit-tested and drawn, the app's bar
// answers its regions and the top band resizes; maximized, the client area
// is the work area with the buttons at its top; in fullscreen there are no
// buttons; and back.
func TestLiveOverlay(t *testing.T) {
	requireLiveOverlay(t)
	b := liveBackend(t)
	var w *Window
	fill := color.RGBA{0x30, 0x34, 0x3c, 0xff}
	frame, waited := 0, 0
	var normal platform.Rect
	runLive(t, b, func() {
		w = newOverlay(t, b, platform.WindowOptions{Title: "Overlay"})
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		frame++
		presentSolid(t, ew, fill)
		st := b.loop.PendingState(w)
		width := ew.Metrics().Size.W
		hit := func(x, y float64) uintptr {
			return sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(screenAt(w, x, y)))
		}
		if frame == 1 && st.TitleBarControls == (platform.Rect{}) && waited < 5 {
			// The first Present showed the window: DWM may report its
			// buttons a frame later (recheckControls).
			waited++
			frame--
			ew.RequestFrame()
			return
		}
		switch frame {
		case 1:
			if waited > 0 {
				t.Logf("the buttons were reported %d frames after the first", waited)
			}
			if !st.ClientSideDecorations {
				t.Errorf("state %v: an overlay window has client-side decorations", st)
			}
			wr, c := getWindowRect(w.hwnd).image(), getClientRect(w.hwnd)
			origin := clientOrigin(w.hwnd)
			if origin.Y != wr.Min.Y {
				t.Errorf("client origin %v, window %v: the caption is not client area", origin, wr)
			}
			if side := origin.X - wr.Min.X; side <= 0 || int(c.width()) != wr.Dx()-2*side || int(c.height()) != wr.Dy()-side {
				t.Errorf("client %dx%d at %v in window %v: want the side and bottom frame kept", c.width(), c.height(), origin, wr)
			}
			normal = st.TitleBarControls
			checkControls(t, "normal", normal, width)
			checkButtonsHit(t, "normal", w, normal)
			for _, tc := range []struct {
				x, y float64
				want uintptr
			}{
				{60, 16, htCaption},
				{150, 16, htClient}, // the search field
				{float64(normal.X) - 8, 16, htCaption},
				{200, 1, htTop},
				{240, 200, htClient},
			} {
				if got := hit(tc.x, tc.y); got != tc.want {
					t.Errorf("WM_NCHITTEST at %v,%v = %d, want %d", tc.x, tc.y, got, tc.want)
				}
			}
			mid := (wr.Min.Y + wr.Max.Y) / 2
			if got := sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(image.Pt(wr.Min.X+2, mid))); got != htLeft {
				t.Errorf("WM_NCHITTEST on the left frame = %d, want %d", got, htLeft)
			}
			if got := sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(image.Pt((wr.Min.X+wr.Max.X)/2, wr.Max.Y-2))); got != htBottom {
				t.Errorf("WM_NCHITTEST on the bottom frame = %d, want %d", got, htBottom)
			}
			checkDrawn(t, "normal", w, normal, fill)
			w.SetMaximized(true)
		case 2:
			if !st.Maximized {
				t.Fatalf("state after SetMaximized(true) = %v", st)
			}
			work := monitorInfoOf(monitorFromWindow(w.hwnd)).work.image()
			c := getClientRect(w.hwnd)
			if got := (image.Rectangle{Min: clientOrigin(w.hwnd), Max: clientOrigin(w.hwnd).Add(image.Pt(int(c.width()), int(c.height())))}); got != work {
				t.Errorf("maximized client %v, want the work area %v", got, work)
			}
			checkControls(t, "maximized", st.TitleBarControls, width)
			checkButtonsHit(t, "maximized", w, st.TitleBarControls)
			if got := hit(200, 1); got != htCaption {
				t.Errorf("maximized: WM_NCHITTEST at the top = %d, want the caption %d (no resize band)", got, htCaption)
			}
			checkDrawn(t, "maximized", w, st.TitleBarControls, fill)
			w.SetMaximized(false)
		case 3:
			if st.Maximized || st.TitleBarControls != normal {
				t.Errorf("restored: state %v, want normal with the buttons at %v", st, normal)
			}
			w.SetFullscreen(true)
		case 4:
			if !st.Fullscreen || st.TitleBarControls != (platform.Rect{}) {
				t.Errorf("fullscreen: state %v, want no buttons", st)
			}
			if got := hit(200, 1); got != htClient {
				t.Errorf("fullscreen: WM_NCHITTEST at the top = %d, want %d", got, htClient)
			}
			w.SetFullscreen(false)
		case 5:
			if st.Fullscreen || st.TitleBarControls != normal {
				t.Errorf("after fullscreen: state %v, want the buttons at %v", st, normal)
			}
			b.Quit()
		}
	})
	if frame < 5 {
		t.Errorf("only %d frames", frame)
	}
}

// TestLiveOverlayPointer: the pointer over DWM's buttons is not the app's:
// moving there from the app's caption is a leave, and no press is delivered.
func TestLiveOverlayPointer(t *testing.T) {
	requireLiveOverlay(t)
	b := liveBackend(t)
	var w *Window
	var events []platform.PointerEvent
	done := false
	parkCursor(t)
	runLive(t, b, func() {
		w = newOverlay(t, b, platform.WindowOptions{})
		requireCursorOutside(t, w.hwnd)
	}, func(ew platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			events = append(events, e)
		case platform.FrameEvent:
			presentSolid(t, ew, color.RGBA{0x30, 0x30, 0x30, 0xff})
			if done {
				return
			}
			done = true
			r := b.loop.PendingState(w).TitleBarControls
			if r == (platform.Rect{}) {
				t.Errorf("no TitleBarControls")
				b.Quit()
				return
			}
			caption := screenAt(w, 60, 16)
			sendMessage(w.hwnd, wmNCMouseMove, htCaption, screenLParam(caption))
			closeBtn := thirds(w, r)[2]
			sendMessage(w.hwnd, wmNCMouseMove, htClose, screenLParam(closeBtn))
			// A middle press on the close button: DWM's or DefWindowProc's,
			// never the app's (a left one would close the window, a right one
			// may open the system menu).
			sendMessage(w.hwnd, wmNCMButtonDown, htClose, screenLParam(closeBtn))
			sendMessage(w.hwnd, wmNCMButtonUp, htClose, screenLParam(closeBtn))
			time.AfterFunc(300*time.Millisecond, b.Wake)
		case platform.WakeEvent:
			b.Quit()
		}
	})
	var actions []platform.PointerAction
	for _, e := range events {
		actions = append(actions, e.Action)
	}
	want := []platform.PointerAction{platform.PointerEnter, platform.PointerMove, platform.PointerLeave}
	if !slices.Equal(actions, want) {
		t.Errorf("pointer actions over the caption, then DWM's close button = %v, want %v", actions, want)
	}
}

// TestLiveOverlayGlyphs: DWM's buttons draw for the app's title bar color
// (DWMWA_USE_IMMERSIVE_DARK_MODE from captionDark): dark glyphs over a white
// bar, light ones over a navy bar, whatever the color scheme.
func TestLiveOverlayGlyphs(t *testing.T) {
	requireLiveOverlay(t)
	b := liveBackend(t)
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	navy := color.RGBA{0x1e, 0x1b, 0x4b, 0xff}
	wc := func(c color.RGBA) platform.TitleBarColors {
		return platform.TitleBarColors{Background: platform.WindowColor{R: c.R, G: c.G, B: c.B, Set: true}}
	}
	var w *Window
	frame := 0
	runLive(t, b, func() {
		w = newOverlay(t, b, platform.WindowOptions{TitleBar: wc(white)})
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		frame++
		fill := white
		if frame > 1 {
			fill = navy
		}
		presentSolid(t, ew, fill)
		r := b.loop.PendingState(w).TitleBarControls
		dark, ok := dwmUint(w, dwmwaUseImmersiveDarkMode)
		closeBtn := screenRect(w, platform.Rect{X: r.X + r.W*2/3, Y: r.Y, W: r.W / 3, H: r.H})
		settle()
		img := captureScreenRect(closeBtn)
		// The glyph: the pixel of the button farthest from the fill.
		var far color.RGBA
		best := -1
		for y := 0; y < img.Rect.Dy(); y++ {
			for x := 0; x < img.Rect.Dx(); x++ {
				c := img.RGBAAt(x, y)
				if d := absInt(int(c.R)-int(fill.R)) + absInt(int(c.G)-int(fill.G)) + absInt(int(c.B)-int(fill.B)); d > best {
					best, far = d, c
				}
			}
		}
		lum := int(far.R) + int(far.G) + int(far.B)
		switch frame {
		case 1:
			if ok && dark != 0 {
				t.Errorf("white bar: DWMWA_USE_IMMERSIVE_DARK_MODE = %d, want 0", dark)
			}
			if lum > 3*0x80 {
				t.Errorf("white bar: the close glyph is %02x%02x%02x, want dark", far.R, far.G, far.B)
			}
			w.SetTitleBarColors(wc(navy))
			ew.RequestFrame()
		case 2:
			if ok && dark != 1 {
				t.Errorf("navy bar: DWMWA_USE_IMMERSIVE_DARK_MODE = %d, want 1", dark)
			}
			if lum < 3*0x80 {
				t.Errorf("navy bar: the close glyph is %02x%02x%02x, want light", far.R, far.G, far.B)
			}
			b.Quit()
		}
	})
}

// TestLiveOverlayMica: with a client-area backdrop the frame is the sheet
// of glass, and DWM's buttons stay.
func TestLiveOverlayMica(t *testing.T) {
	requireLiveOverlay(t)
	requireBuild(t, buildBackdrop)
	b := liveBackend(t)
	var w *Window
	done := false
	runLive(t, b, func() {
		w = newOverlay(t, b, platform.WindowOptions{Backdrop: platform.BackdropMica})
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || done {
			return
		}
		done = true
		defer b.Quit()
		presentSolid(t, ew, color.RGBA{0x30, 0x30, 0x30, 0xff})
		if !w.PresentsAlpha() {
			t.Errorf("a Mica overlay window does not present alpha")
		}
		r := b.loop.PendingState(w).TitleBarControls
		checkControls(t, "mica", r, ew.Metrics().Size.W)
		checkButtonsHit(t, "mica", w, r)
	})
}
