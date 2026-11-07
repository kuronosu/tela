package win32

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// Tests of the pure half of TitleBarOverlay windows (overlay.go); they run on
// every OS. The window itself is checked live (live_overlay_test.go).

func TestOverlaySupported(t *testing.T) {
	for _, tc := range []struct {
		build          uint32
		composing, dwm bool
		want           bool
	}{
		{22000, true, true, true},
		{26200, true, true, true},
		{19045, true, true, false}, // Windows 10 22H2: Frameless
		{26200, false, true, false},
		{26200, true, false, false},
	} {
		if got := overlaySupported(tc.build, tc.composing, tc.dwm); got != tc.want {
			t.Errorf("overlaySupported(%d, composing %v, dwm %v) = %v, want %v", tc.build, tc.composing, tc.dwm, got, tc.want)
		}
	}
}

func TestOverlayClient(t *testing.T) {
	for _, tc := range []struct {
		name               string
		proposed, def, out image.Rectangle
		zoomed             bool
		work               image.Rectangle
	}{
		{
			// The side and bottom frame stay, the caption is client area.
			name:     "normal",
			proposed: image.Rect(100, 100, 616, 439),
			def:      image.Rect(108, 139, 608, 431),
			out:      image.Rect(108, 100, 608, 431),
		},
		{
			// Windows puts the frame outside the work area: the top is
			// clipped like the sides DefWindowProc already moved in.
			name:     "maximized",
			proposed: image.Rect(-8, -8, 1928, 1048),
			def:      image.Rect(0, 31, 1920, 1040),
			zoomed:   true,
			work:     image.Rect(0, 0, 1920, 1040),
			out:      image.Rect(0, 0, 1920, 1040),
		},
		{
			name:     "maximized under a task bar at the top",
			proposed: image.Rect(-8, 32, 1928, 1088),
			def:      image.Rect(0, 71, 1920, 1080),
			zoomed:   true,
			work:     image.Rect(0, 40, 1920, 1080),
			out:      image.Rect(0, 40, 1920, 1080),
		},
		{
			name:     "maximized on a monitor at 150%",
			proposed: image.Rect(1909, -11, 4491, 1451),
			def:      image.Rect(1920, 34, 4480, 1440),
			zoomed:   true,
			work:     image.Rect(1920, 0, 4480, 1440),
			out:      image.Rect(1920, 0, 4480, 1440),
		},
	} {
		if got := overlayClient(tc.proposed, tc.def, tc.zoomed, tc.work); got != tc.out {
			t.Errorf("%s: overlayClient = %v, want %v", tc.name, got, tc.out)
		}
	}
}

func TestOverlayOuter(t *testing.T) {
	client := image.Rect(0, 0, 400, 300)
	adjusted := image.Rect(-8, -31, 408, 308)
	if got, want := overlayOuter(client, adjusted), image.Rect(-8, 0, 408, 308); got != want {
		t.Errorf("overlayOuter = %v, want %v", got, want)
	}
	// The outer size of a 400x300 client: the frame on three sides only.
	if got := overlayOuter(client, adjusted).Size(); got != image.Pt(416, 308) {
		t.Errorf("outer size %v, want 416x308", got)
	}
}

func TestControlsRect(t *testing.T) {
	for _, tc := range []struct {
		name        string
		bounds      image.Rectangle // window-relative
		off, client image.Point
		scale       float64
		want        platform.Rect
		wantPx      image.Rectangle
	}{
		{
			// A 400x300 window at 100%: the client starts 8 px right of
			// the window rectangle (the invisible side frame) and at its
			// top.
			name:   "normal",
			bounds: image.Rect(270, 1, 408, 32),
			off:    image.Pt(8, 0), client: image.Pt(400, 300), scale: 1,
			want:   platform.Rect{X: 262, Y: 1, W: 138, H: 31},
			wantPx: image.Rect(262, 1, 400, 32),
		},
		{
			name:   "150%",
			bounds: image.Rect(405, 1, 612, 47),
			off:    image.Pt(12, 0), client: image.Pt(600, 450), scale: 1.5,
			want:   platform.Rect{X: 262, Y: 2.0 / 3, W: 138, H: 46.0 / 1.5},
			wantPx: image.Rect(393, 1, 600, 47),
		},
		{
			// Maximized: the window rectangle starts above and left of
			// the client by the frame.
			name:   "maximized",
			bounds: image.Rect(1790, 8, 1928, 38),
			off:    image.Pt(8, 8), client: image.Pt(1920, 1032), scale: 1,
			want:   platform.Rect{X: 1782, Y: 0, W: 138, H: 30},
			wantPx: image.Rect(1782, 0, 1920, 30),
		},
		{
			// Whatever lies outside the client area is cut.
			name:   "clipped",
			bounds: image.Rect(270, -4, 420, 32),
			off:    image.Pt(8, 0), client: image.Pt(400, 300), scale: 1,
			want:   platform.Rect{X: 262, Y: 0, W: 138, H: 32},
			wantPx: image.Rect(262, 0, 400, 32),
		},
		{
			name:   "outside",
			bounds: image.Rect(0, -40, 100, -10),
			off:    image.Pt(8, 0), client: image.Pt(400, 300), scale: 1,
		},
		{
			name: "no bounds",
			off:  image.Pt(8, 0), client: image.Pt(400, 300), scale: 1,
		},
	} {
		got, px := controlsRect(tc.bounds, tc.off, tc.client, tc.scale)
		if !nearRect(got, tc.want) || px != tc.wantPx {
			t.Errorf("%s: controlsRect = %v %v, want %v %v", tc.name, got, px, tc.want, tc.wantPx)
		}
	}
}

func nearRect(a, b platform.Rect) bool {
	near := func(x, y float32) bool { return x-y < 1e-4 && y-x < 1e-4 }
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H)
}

func TestCaptionDark(t *testing.T) {
	c := func(r, g, b uint8) platform.TitleBarColors {
		return platform.TitleBarColors{Background: platform.WindowColor{R: r, G: g, B: b, Set: true}}
	}
	for _, tc := range []struct {
		name   string
		colors platform.TitleBarColors
		scheme bool
		want   bool
	}{
		{"no color, light scheme", platform.TitleBarColors{}, false, false},
		{"no color, dark scheme", platform.TitleBarColors{}, true, true},
		// A color of the app's wins over the scheme.
		{"white bar, dark scheme", c(0xff, 0xff, 0xff), true, false},
		{"black bar, light scheme", c(0, 0, 0), false, true},
		{"navy bar", c(0x1e, 0x1b, 0x4b), false, true},
		{"zinc-100 bar", c(0xf4, 0xf4, 0xf5), true, false},
		{"zinc-800 bar", c(0x27, 0x27, 0x2a), false, true},
		// Around the crossover (relative luminance 0.179).
		{"gray 0x80", c(0x80, 0x80, 0x80), true, false},
		{"gray 0x70", c(0x70, 0x70, 0x70), false, true},
		// A text color alone says nothing about the bar.
		{"text only", platform.TitleBarColors{Text: platform.WindowColor{R: 0xff, G: 0xff, B: 0xff, Set: true}}, false, false},
	} {
		if got := captionDark(tc.colors, tc.scheme); got != tc.want {
			t.Errorf("%s: captionDark = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// overlayBar is the title bar of a 400 px wide TitleBarOverlay window at
// 100%: a caption with a search field punched into it, and DWM's buttons at
// the right (138x32 physical pixels, the rectangle of controlsRect).
var overlayBar = []platform.HitRegion{
	{Kind: platform.HitCaption, Rect: platform.Rect{W: 400, H: 32}},
	{Kind: platform.HitClient, Rect: platform.Rect{X: 100, Y: 4, W: 120, H: 24}},
}

func TestClientHitOverlay(t *testing.T) {
	controls := image.Rect(262, 0, 400, 32)
	in := hitInput{client: image.Pt(400, 300), scale: 1, border: 8, regions: overlayBar, topOnly: true, controls: controls}
	for _, tc := range []struct {
		x, y int
		want uintptr
	}{
		// Only the top is a band: the other edges are the OS's frame,
		// outside the client area.
		{200, 0, htTop},
		{200, 7, htTop},
		{0, 0, htTopLeft},
		{7, 3, htTopLeft},
		{8, 3, htTop},
		{396, 3, htTopRight}, // the band over DWM's buttons, where DWM leaves it
		{0, 150, htClient},
		{399, 150, htClient},
		{200, 299, htClient},
		{0, 299, htClient},
		// The app's regions below the band.
		{50, 16, htCaption},
		{150, 16, htClient}, // the search field
		{240, 16, htCaption},
		// The buttons' rectangle that DWM does not claim is caption.
		{300, 16, htCaption},
		{399, 31, htCaption},
		{200, 200, htClient},
	} {
		in.pt = image.Pt(tc.x, tc.y)
		if got := clientHit(in); got != tc.want {
			t.Errorf("clientHit(%d,%d) = %d, want %d", tc.x, tc.y, got, tc.want)
		}
	}

	// The OS's buttons take the place of the app's own button regions,
	// even over the band.
	withButtons := append(overlayBar[:len(overlayBar):len(overlayBar)], titleBar[2:]...)
	in = hitInput{client: image.Pt(400, 300), scale: 1, border: 8, regions: withButtons, topOnly: true, controls: controls}
	for _, p := range []image.Point{{370, 16}, {290, 16}, {330, 2}} {
		in.pt = p
		if got := clientHit(in); isAppButton(got) {
			t.Errorf("clientHit(%v) over the OS's buttons = %d, an app's button", p, got)
		}
	}

	// Maximized: no band, the top row is the app's caption.
	in = hitInput{client: image.Pt(400, 300), scale: 1, regions: overlayBar, topOnly: true, controls: controls}
	for _, tc := range []struct {
		x, y int
		want uintptr
	}{{200, 0, htCaption}, {0, 0, htCaption}, {300, 0, htCaption}, {200, 200, htClient}} {
		in.pt = image.Pt(tc.x, tc.y)
		if got := clientHit(in); got != tc.want {
			t.Errorf("maximized: clientHit(%d,%d) = %d, want %d", tc.x, tc.y, got, tc.want)
		}
	}

	// No regions: the band, else content (the app has not declared its
	// bar yet).
	in = hitInput{client: image.Pt(400, 300), scale: 1, border: 8, topOnly: true}
	for _, tc := range []struct {
		x, y int
		want uintptr
	}{{200, 2, htTop}, {200, 16, htClient}} {
		in.pt = image.Pt(tc.x, tc.y)
		if got := clientHit(in); got != tc.want {
			t.Errorf("no regions: clientHit(%d,%d) = %d, want %d", tc.x, tc.y, got, tc.want)
		}
	}
}
