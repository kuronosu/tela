package win32

import (
	"image"
	"slices"
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
			// Windows puts the frame outside the work area: the sides
			// and bottom are clipped to it, the top stays at the
			// window's top for DWM (the hidden band, overlayInset).
			name:     "maximized",
			proposed: image.Rect(-8, -8, 1928, 1048),
			def:      image.Rect(0, 31, 1920, 1040),
			zoomed:   true,
			work:     image.Rect(0, 0, 1920, 1040),
			out:      image.Rect(0, -8, 1920, 1040),
		},
		{
			name:     "maximized under a task bar at the top",
			proposed: image.Rect(-8, 32, 1928, 1088),
			def:      image.Rect(0, 71, 1920, 1080),
			zoomed:   true,
			work:     image.Rect(0, 40, 1920, 1080),
			out:      image.Rect(0, 32, 1920, 1080),
		},
		{
			name:     "maximized on a monitor at 150%",
			proposed: image.Rect(1909, -11, 4491, 1451),
			def:      image.Rect(1920, 34, 4480, 1440),
			zoomed:   true,
			work:     image.Rect(1920, 0, 4480, 1440),
			out:      image.Rect(1920, -11, 4480, 1440),
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

func TestOverlayInset(t *testing.T) {
	for _, tc := range []struct {
		name        string
		top, height int
		work        image.Rectangle
		want        int
	}{
		{"maximized", -8, 1048, image.Rect(0, 0, 1920, 1040), 8},
		{"under a task bar at the top", 32, 1048, image.Rect(0, 40, 1920, 1080), 8},
		{"150%", -11, 1451, image.Rect(1920, 0, 4480, 1440), 11},
		{"inside the work area", 100, 300, image.Rect(0, 0, 1920, 1040), 0},
		{"unknown work area", -8, 1048, image.Rectangle{}, 0},
		{"never the whole client area", -50, 20, image.Rect(0, 0, 1920, 1040), 19},
	} {
		if got := overlayInset(tc.top, tc.height, tc.work); got != tc.want {
			t.Errorf("%s: overlayInset = %d, want %d", tc.name, got, tc.want)
		}
	}
	// The app's area of a maximized window is the work area: the client
	// rectangle of overlayClient without the band.
	client := overlayClient(image.Rect(-8, -8, 1928, 1048), image.Rect(0, 31, 1920, 1040), true, image.Rect(0, 0, 1920, 1040))
	client.Min.Y += overlayInset(client.Min.Y, client.Dy(), image.Rect(0, 0, 1920, 1040))
	if client != image.Rect(0, 0, 1920, 1040) {
		t.Errorf("the app's area of a maximized window = %v, want the work area", client)
	}
}

func TestCaptionDark(t *testing.T) {
	c := func(r, g, b uint8) platform.WindowColor {
		return platform.WindowColor{R: r, G: g, B: b, Set: true}
	}
	for _, tc := range []struct {
		name   string
		bg     platform.WindowColor
		scheme bool
		want   bool
	}{
		{"no color, light scheme", platform.WindowColor{}, false, false},
		{"no color, dark scheme", platform.WindowColor{}, true, true},
		// The color behind the buttons wins over the scheme.
		{"white bar, dark scheme", c(0xff, 0xff, 0xff), true, false},
		{"black bar, light scheme", c(0, 0, 0), false, true},
		{"navy bar", c(0x1e, 0x1b, 0x4b), false, true},
		{"zinc-100 bar", c(0xf4, 0xf4, 0xf5), true, false},
		{"zinc-800 bar", c(0x27, 0x27, 0x2a), false, true},
		// Around the crossover (relative luminance 0.179).
		{"gray 0x80", c(0x80, 0x80, 0x80), true, false},
		{"gray 0x70", c(0x70, 0x70, 0x70), false, true},
	} {
		if got := captionDark(tc.bg, tc.scheme); got != tc.want {
			t.Errorf("%s: captionDark = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The caption glyphs of an overlay window follow the app's bar, not the
// system's scheme, when the app's theme differs from it and it gives no
// TitleBar.Background (ADR-252): the color sampled from its bar decides.
func TestCaptionColor(t *testing.T) {
	c := func(r, g, b uint8) platform.WindowColor {
		return platform.WindowColor{R: r, G: g, B: b, Set: true}
	}
	white, navy, zinc950 := c(0xff, 0xff, 0xff), c(0x1e, 0x1b, 0x4b), c(0x09, 0x09, 0x0b)
	for _, tc := range []struct {
		name              string
		titleBar, sampled platform.WindowColor
		glass, scheme     bool
		want              platform.WindowColor
		dark              bool
	}{
		{"light app on a dark system", platform.WindowColor{}, white, false, true, white, false},
		{"dark app on a light system", platform.WindowColor{}, zinc950, false, false, zinc950, true},
		{"the app's background wins over the sample", navy, white, false, false, navy, true},
		{"nothing sampled yet: the scheme", platform.WindowColor{}, platform.WindowColor{}, false, true, platform.WindowColor{}, true},
		{"a backdrop's bar is the material: the scheme", platform.WindowColor{}, white, true, true, platform.WindowColor{}, true},
		{"a backdrop with the app's background", navy, white, true, false, navy, true},
	} {
		got := captionColor(tc.titleBar, tc.sampled, tc.glass)
		if got != tc.want {
			t.Errorf("%s: captionColor = %+v, want %+v", tc.name, got, tc.want)
		}
		if d := captionDark(got, tc.scheme); d != tc.dark {
			t.Errorf("%s: dark glyph variant = %v, want %v", tc.name, d, tc.dark)
		}
	}

	// From the pixels: a light bar presented under a dark system scheme.
	const w, h = 200, 40
	pix := make([]byte, w*h*4)
	for i := range pix {
		pix[i] = 0xfa // zinc-50, B G R X
	}
	ctl := image.Rect(140, 0, 200, 32)
	sampled, ok := captionSample(pix, w*4, image.Pt(w, h), ctl, 1)
	if !ok || captionDark(captionColor(platform.WindowColor{}, sampled, false), true) {
		t.Errorf("a light bar on a dark system: sampled %+v (%v), want light glyph variant", sampled, ok)
	}
}

func TestCaptionSample(t *testing.T) {
	// A 400x100 copy: a navy bar 32 px tall at 100%, 48 px at 150%, with
	// a white mark where the gap ends at 100%.
	bar := func(barH int) dib {
		d := dib{}
		d.resize(image.Pt(400, 100))
		for y := 0; y < barH; y++ {
			for x := 0; x < 400; x++ {
				i := y*d.stride() + 4*x
				d.pix[i], d.pix[i+1], d.pix[i+2] = 0x4b, 0x1b, 0x1e
			}
		}
		return d
	}
	navy := platform.WindowColor{R: 0x1e, G: 0x1b, B: 0x4b, Set: true}
	d := bar(32)
	if c, ok := captionSample(d.pix, d.stride(), d.size, image.Rect(262, 0, 400, 32), 1); !ok || c != navy {
		t.Errorf("100%%: captionSample = %v %v, want the bar's %v", c, ok, navy)
	}
	// The point is captionGap logical pixels left, in the middle row.
	i := 16*d.stride() + 4*(262-4)
	d.pix[i], d.pix[i+1], d.pix[i+2] = 0xff, 0xff, 0xff
	if c, _ := captionSample(d.pix, d.stride(), d.size, image.Rect(262, 0, 400, 32), 1); c != (platform.WindowColor{R: 0xff, G: 0xff, B: 0xff, Set: true}) {
		t.Errorf("captionSample = %v, want the pixel 4 px left of the buttons in their middle row", c)
	}
	d = bar(48)
	if c, ok := captionSample(d.pix, d.stride(), d.size, image.Rect(193, 0, 400, 47), 1.5); !ok || c != navy {
		t.Errorf("150%%: captionSample = %v %v, want %v", c, ok, navy)
	}
	for _, ctl := range []image.Rectangle{{}, image.Rect(2, 0, 100, 32), image.Rect(262, 120, 400, 150)} {
		if c, ok := captionSample(d.pix, d.stride(), d.size, ctl, 1); ok {
			t.Errorf("buttons at %v: captionSample = %v, want none", ctl, c)
		}
	}
	if _, ok := captionSample(nil, 0, image.Point{}, image.Rect(262, 0, 400, 32), 1); ok {
		t.Errorf("captionSample of no pixels reported a color")
	}
}

func TestBlitParts(t *testing.T) {
	hole := image.Rect(262, 8, 400, 40)
	for _, tc := range []struct {
		name        string
		r           image.Rectangle
		top         int
		hole        image.Rectangle
		from, clear []image.Rectangle
	}{
		{name: "plain window", r: image.Rect(0, 0, 400, 300), from: []image.Rectangle{image.Rect(0, 0, 400, 300)}},
		{name: "empty", r: image.Rectangle{}},
		{
			name: "everything of a maximized window", r: image.Rect(0, 0, 400, 300), top: 8, hole: hole,
			from:  []image.Rectangle{image.Rect(0, 40, 400, 300), image.Rect(0, 8, 262, 40)},
			clear: []image.Rectangle{image.Rect(0, 0, 400, 8), image.Rect(262, 8, 400, 40)},
		},
		{
			name: "only the band", r: image.Rect(10, 0, 50, 6), top: 8, hole: hole,
			clear: []image.Rectangle{image.Rect(10, 0, 50, 6)},
		},
		{
			name: "a damage rectangle across the hole", r: image.Rect(250, 20, 300, 60), hole: hole,
			from:  []image.Rectangle{image.Rect(250, 40, 300, 60), image.Rect(250, 20, 262, 40)},
			clear: []image.Rectangle{image.Rect(262, 20, 300, 40)},
		},
		{
			name: "inside the hole", r: image.Rect(300, 10, 320, 20), hole: hole,
			clear: []image.Rectangle{image.Rect(300, 10, 320, 20)},
		},
		{
			name: "around the hole", r: image.Rect(250, 0, 410, 50), hole: hole,
			from:  []image.Rectangle{image.Rect(250, 0, 410, 8), image.Rect(250, 40, 410, 50), image.Rect(250, 8, 262, 40), image.Rect(400, 8, 410, 40)},
			clear: []image.Rectangle{hole},
		},
	} {
		from, clear := blitParts(tc.r, tc.top, tc.hole)
		if !slices.Equal(from, tc.from) || !slices.Equal(clear, tc.clear) {
			t.Errorf("%s: blitParts = %v, %v; want %v, %v", tc.name, from, clear, tc.from, tc.clear)
		}
		// The parts tile r exactly.
		area := 0
		for _, p := range append(from, clear...) {
			area += p.Dx() * p.Dy()
			if !p.In(tc.r) {
				t.Errorf("%s: part %v outside %v", tc.name, p, tc.r)
			}
		}
		if area != tc.r.Dx()*tc.r.Dy() {
			t.Errorf("%s: the parts cover %d pixels, want %d", tc.name, area, tc.r.Dx()*tc.r.Dy())
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

	// Maximized, the hidden band above the app's area (negative y, from a
	// monitor above) is the title bar's.
	in = hitInput{client: image.Pt(400, 300), scale: 1, regions: overlayBar, topOnly: true, controls: controls}
	for _, p := range []image.Point{{200, -1}, {200, -8}, {300, -4}} {
		in.pt = p
		if got := clientHit(in); got != htCaption {
			t.Errorf("hidden band: clientHit(%v) = %d, want %d", p, got, htCaption)
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
