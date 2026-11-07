//go:build windows

package win32

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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
// DWM takes those hit tests as the pointer's (it highlights the buttons):
// a WM_NCMOUSELEAVE ends that.
func checkButtonsHit(t *testing.T, what string, w *Window, r platform.Rect) {
	t.Helper()
	for i, want := range [3]uintptr{htMinButton, htMaxButton, htClose} {
		p := thirds(w, r)[i]
		if got := sendMessage(w.hwnd, wmNCHitTest, 0, screenLParam(p)); got != want {
			t.Errorf("%s: WM_NCHITTEST at %v (button %d of %v) = %d, want %d", what, p, i, r, got, want)
		}
	}
	sendMessage(w.hwnd, wmNCMouseLeave, 0, 0)
}

// glyphPixels counts the pixels of the screen rectangle that differ from
// fill by more than a little: what DWM drew there.
func glyphPixels(r image.Rectangle, fill color.RGBA) int {
	img := captureScreenRect(r)
	n := 0
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			if colorDist(img.RGBAAt(x, y), fill) > 60 {
				n++
			}
		}
	}
	return n
}

func colorDist(a, b color.RGBA) int {
	return absInt(int(a.R)-int(b.R)) + absInt(int(a.G)-int(b.G)) + absInt(int(a.B)-int(b.B))
}

// screenRect returns the screen rectangle of a logical client rectangle.
func screenRect(w *Window, r platform.Rect) image.Rectangle {
	return image.Rectangle{Min: screenAt(w, float64(r.X), float64(r.Y)), Max: screenAt(w, float64(r.X+r.W), float64(r.Y+r.H))}
}

// buttonRect returns the screen rectangle of DWM's button i (0 minimize, 1
// maximize, 2 close) in the reported rectangle r, without its top two
// physical rows: DWM draws the window's one-pixel border over the top row
// of a window that is not maximized, which is no button.
func buttonRect(w *Window, r platform.Rect, i int) image.Rectangle {
	b := screenRect(w, platform.Rect{X: r.X + r.W*float32(i)/3, Y: r.Y, W: r.W / 3, H: r.H})
	b.Min.Y += 2
	return b
}

// glyphColor returns the pixel of a screen rectangle farthest from fill:
// the glyph DWM drew there.
func glyphColor(r image.Rectangle, fill color.RGBA) color.RGBA {
	img := captureScreenRect(r)
	var far color.RGBA
	best := -1
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			if c := img.RGBAAt(x, y); colorDist(c, fill) > best {
				best, far = colorDist(c, fill), c
			}
		}
	}
	return far
}

// checkDrawn reads the screen: DWM's buttons are drawn over the app's fill
// (the close button's glyph), the background behind each button is the
// bar's color (DWMWA_CAPTION_COLOR, not a block of the system's caption
// color), and the rest of the title bar and the content are the app's
// pixels (no classic caption, no black strip).
func checkDrawn(t *testing.T, what string, w *Window, r platform.Rect, fill color.RGBA) {
	t.Helper()
	settle()
	if n := glyphPixels(buttonRect(w, r, 2), fill); n < 6 {
		t.Errorf("%s: %d pixels of the close button differ from the app's fill: DWM drew no button", what, n)
	}
	checkButtonsBackground(t, what, w, r, fill)
	width := float64(w.Metrics().Size.W)
	for _, p := range [][2]float64{{12, 6}, {60, 16}, {width / 2, 2}, {width / 2, 16}, {width / 2, 31}, {float64(r.X) - 8, 16}, {width / 2, 150}} {
		sp := screenAt(w, p[0], p[1])
		if c := captureScreenRect(image.Rectangle{Min: sp, Max: sp.Add(image.Pt(1, 1))}).RGBAAt(0, 0); !nearColor(c, fill) {
			t.Errorf("%s: screen at %v,%v = %02x%02x%02x, want the app's %02x%02x%02x", what, p[0], p[1], c.R, c.G, c.B, fill.R, fill.G, fill.B)
		}
	}
}

// checkButtonsBackground checks that the background of each of DWM's
// buttons in r is the bar's fill: two points near its left and right
// edges, at its middle height, away from the glyph.
func checkButtonsBackground(t *testing.T, what string, w *Window, r platform.Rect, fill color.RGBA) {
	t.Helper()
	for i := range 3 {
		b := buttonRect(w, r, i)
		img := captureScreenRect(b)
		for _, x := range [2]int{3, b.Dx() - 4} {
			if c := img.RGBAAt(x, img.Rect.Dy()/2); colorDist(c, fill) > 9 {
				t.Errorf("%s: button %d's background at %v = %02x%02x%02x, want the bar's %02x%02x%02x", what, i,
					image.Pt(b.Min.X+x, b.Min.Y+img.Rect.Dy()/2), c.R, c.G, c.B, fill.R, fill.G, fill.B)
			}
		}
	}
}

func nearColor(a, b color.RGBA) bool {
	return absInt(int(a.R)-int(b.R)) <= 3 && absInt(int(a.G)-int(b.G)) <= 3 && absInt(int(a.B)-int(b.B)) <= 3
}

// saveShot writes a capture of the right end of w's title bar (the app's
// coordinates, 300 x 60 logical pixels) to the temporary folder, for a
// person to look at.
func saveShot(t *testing.T, w *Window, name string) {
	t.Helper()
	width := float64(w.Metrics().Size.W)
	r := image.Rectangle{Min: screenAt(w, width-300, 0), Max: screenAt(w, width, 60)}
	path := filepath.Join(os.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Logf("capture %s: %v", name, err)
		return
	}
	defer f.Close()
	if err := png.Encode(f, captureScreenRect(r)); err != nil {
		t.Logf("capture %s: %v", name, err)
		return
	}
	t.Logf("capture: %s", path)
}

// activate makes DWM draw w's buttons as in an active window
// (WM_NCACTIVATE TRUE; a scheduled task's window is never the foreground
// one), or as in an inactive one.
func activate(w *Window, active bool) {
	var v uintptr
	if active {
		v = 1
	}
	sendMessage(w.hwnd, wmNCActivate, v, 0)
}

// TestLiveOverlay: the caption is client area (the side and bottom frame
// stay), DWM's buttons are reported, hit-tested and drawn, the app's bar
// answers its regions and the top band resizes; maximized, the app's area
// is the work area (the client area keeps a hidden band above it) with the
// buttons at its top, and positions are the app's; in fullscreen there are
// no buttons; and back.
func TestLiveOverlay(t *testing.T) {
	requireLiveOverlay(t)
	b := liveBackend(t)
	var w *Window
	fill := color.RGBA{0x30, 0x34, 0x3c, 0xff}
	frame, waited := 0, 0
	var normal platform.Rect
	var presses []platform.PointerEvent
	runLive(t, b, func() {
		w = newOverlay(t, b, platform.WindowOptions{Title: "Overlay"})
	}, func(ew platform.Window, e platform.Event) {
		if pe, ok := e.(platform.PointerEvent); ok && pe.Action == platform.PointerDown {
			presses = append(presses, pe)
		}
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
			wr, c, origin := getWindowRect(w.hwnd).image(), getClientRect(w.hwnd), clientOrigin(w.hwnd)
			if origin.Y != wr.Min.Y || w.top <= 0 || origin.Y+w.top != work.Min.Y {
				t.Errorf("maximized: client origin %v, window %v, hidden band %d: want the client area from the window's top, the band down to the work area %v", origin, wr, w.top, work)
			}
			app := image.Rectangle{Min: origin.Add(image.Pt(0, w.top)), Max: origin.Add(image.Pt(int(c.width()), int(c.height())))}
			if app != work || w.client != app.Size() {
				t.Errorf("maximized: the app's area %v (size %v), want the work area %v", app, w.client, work)
			}
			if got, want := ew.Metrics().Physical(), work.Size(); got != want {
				t.Errorf("maximized: Metrics %v (physical %v), want the work area's size %v", ew.Metrics(), got, want)
			}
			t.Logf("maximized: window %v, client origin %v, hidden band %d, buttons %v", wr, origin, w.top, st.TitleBarControls)
			checkControls(t, "maximized", st.TitleBarControls, width)
			checkButtonsHit(t, "maximized", w, st.TitleBarControls)
			if got := hit(200, 1); got != htCaption {
				t.Errorf("maximized: WM_NCHITTEST at the top = %d, want the caption %d (no resize band)", got, htCaption)
			}
			checkDrawn(t, "maximized", w, st.TitleBarControls, fill)
			// A press at the app's y = 1 is at the app's y = 1: client
			// messages carry client coordinates, the band above.
			s := w.scale()
			lp := mouseLParam(int(240*s), w.top+int(s))
			sendMessage(w.hwnd, wmMouseMove, 0, lp)
			sendMessage(w.hwnd, wmLButtonDown, mkLButton, lp)
			sendMessage(w.hwnd, wmLButtonUp, 0, lp)
			w.SetMaximized(false)
		case 3:
			if st.Maximized || st.TitleBarControls != normal || w.top != 0 {
				t.Errorf("restored: state %v, hidden band %d, want normal with the buttons at %v", st, w.top, normal)
			}
			want := platform.Point{X: float32(int(240*w.scale())) / float32(w.scale()), Y: float32(int(w.scale())) / float32(w.scale())}
			if len(presses) != 1 || presses[0].Pos != want {
				t.Errorf("maximized: presses %v, want one at the app's %v", presses, want)
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

// TestLiveOverlayGlyphs: with the app's title bar background, DWM's buttons
// sit on that color (DWMWA_CAPTION_COLOR) and draw for it
// (DWMWA_USE_IMMERSIVE_DARK_MODE from captionDark): dark glyphs over a white
// bar, light ones over a navy bar, whatever the color scheme, when the
// window is active; gray ones, closer to the bar, when it is not.
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
	parkCursor(t)
	runLive(t, b, func() {
		w = newOverlay(t, b, platform.WindowOptions{TitleBar: wc(white)})
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		frame++
		fill, what := white, "white bar"
		if frame > 1 {
			fill, what = navy, "navy bar"
		}
		presentSolid(t, ew, fill)
		r := b.loop.PendingState(w).TitleBarControls
		if r == (platform.Rect{}) {
			t.Fatalf("%s: no TitleBarControls", what)
		}
		dark, ok := dwmUint(w, dwmwaUseImmersiveDarkMode)
		closeBtn := buttonRect(w, r, 2)
		activate(w, false)
		settle()
		inactive := glyphColor(closeBtn, fill)
		activate(w, true)
		settle()
		active := glyphColor(closeBtn, fill)
		lum := int(active.R) + int(active.G) + int(active.B)
		t.Logf("%s: close glyph %02x%02x%02x active, %02x%02x%02x inactive", what, active.R, active.G, active.B, inactive.R, inactive.G, inactive.B)
		if d := colorDist(inactive, fill); d < 60 || d >= colorDist(active, fill) {
			t.Errorf("%s: the inactive close glyph %02x%02x%02x is %d from the bar: want drawn, closer to the bar than the active one", what, inactive.R, inactive.G, inactive.B, d)
		}
		checkButtonsBackground(t, what, w, r, fill)
		switch frame {
		case 1:
			if ok && dark != 0 {
				t.Errorf("white bar: DWMWA_USE_IMMERSIVE_DARK_MODE = %d, want 0", dark)
			}
			if lum > 3*0x80 {
				t.Errorf("white bar: the close glyph is %02x%02x%02x, want dark", active.R, active.G, active.B)
			}
			w.SetTitleBarColors(wc(navy))
			ew.RequestFrame()
		case 2:
			if ok && dark != 1 {
				t.Errorf("navy bar: DWMWA_USE_IMMERSIVE_DARK_MODE = %d, want 1", dark)
			}
			if lum < 3*0x80 {
				t.Errorf("navy bar: the close glyph is %02x%02x%02x, want light", active.R, active.G, active.B)
			}
			b.Quit()
		}
	})
}

// TestLiveOverlayCaption: without a title bar background of the app's, the
// color behind DWM's buttons is the bar's own, sampled from the presented
// pixels beside them (captionSample), and the glyphs follow it: a light
// and a dark bar, restored and maximized. It saves the four captures in the
// temporary folder (tela-overlay-*.png).
func TestLiveOverlayCaption(t *testing.T) {
	requireLiveOverlay(t)
	b := liveBackend(t)
	light := color.RGBA{0xf4, 0xf4, 0xf5, 0xff} // zinc-100
	dark := color.RGBA{0x1e, 0x1b, 0x4b, 0xff}  // indigo-950
	var w *Window
	frame, waited := 0, 0
	parkCursor(t)
	check := func(what string, fill color.RGBA, wantDark bool, shot string) {
		t.Helper()
		r := b.loop.PendingState(w).TitleBarControls
		checkControls(t, what, r, w.Metrics().Size.W)
		if want := (platform.WindowColor{R: fill.R, G: fill.G, B: fill.B, Set: true}); w.sampled != want || w.capRef != colorRef(want) {
			t.Errorf("%s: sampled %v, caption color %#x; want %v", what, w.sampled, w.capRef, want)
		}
		if v, ok := dwmUint(w, dwmwaUseImmersiveDarkMode); ok && (v != 0) != wantDark {
			t.Errorf("%s: DWMWA_USE_IMMERSIVE_DARK_MODE = %d, want dark %v", what, v, wantDark)
		}
		activate(w, true)
		checkDrawn(t, what, w, r, fill)
		g := glyphColor(buttonRect(w, r, 2), fill)
		if lum := int(g.R) + int(g.G) + int(g.B); (lum > 3*0x80) != wantDark {
			t.Errorf("%s: the close glyph is %02x%02x%02x, want light %v", what, g.R, g.G, g.B, wantDark)
		}
		saveShot(t, w, shot)
		checkButtonsHit(t, what, w, r)
	}
	runLive(t, b, func() {
		w = newOverlay(t, b, platform.WindowOptions{Title: "Caption"})
	}, func(ew platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		frame++
		fill := light
		if frame > 2 {
			fill = dark
		}
		presentSolid(t, ew, fill)
		if frame == 1 && b.loop.PendingState(w).TitleBarControls == (platform.Rect{}) && waited < 5 {
			waited++
			frame--
			ew.RequestFrame()
			return
		}
		switch frame {
		case 1:
			check("light, restored", fill, false, "tela-overlay-light-restored.png")
			w.SetMaximized(true)
		case 2:
			if !b.loop.PendingState(w).Maximized {
				t.Fatalf("not maximized")
			}
			check("light, maximized", fill, false, "tela-overlay-light-maximized.png")
			ew.RequestFrame()
		case 3:
			check("dark, maximized", fill, true, "tela-overlay-dark-maximized.png")
			w.SetMaximized(false)
		case 4:
			if b.loop.PendingState(w).Maximized {
				t.Fatalf("still maximized")
			}
			check("dark, restored", fill, true, "tela-overlay-dark-restored.png")
			b.Quit()
		}
	})
	if frame < 4 {
		t.Errorf("only %d frames", frame)
	}
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
