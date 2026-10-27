package win32

import (
	"image"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// Tests of the pure window control logic (frame.go); they run on every OS.

func TestHitCodeOf(t *testing.T) {
	for _, tc := range []struct {
		kind       platform.HitKind
		fullscreen bool
		want       uintptr
	}{
		{platform.HitClient, false, htClient},
		{platform.HitCaption, false, htCaption},
		{platform.HitCaption, true, htClient}, // no move or maximize in fullscreen
		{platform.HitMinimize, false, htMinButton},
		{platform.HitMaximize, false, htMaxButton},
		{platform.HitMaximize, true, htMaxButton},
		{platform.HitClose, false, htClose},
	} {
		if got := hitCodeOf(tc.kind, tc.fullscreen); got != tc.want {
			t.Errorf("hitCodeOf(%v, fullscreen %v) = %d, want %d", tc.kind, tc.fullscreen, got, tc.want)
		}
	}
}

func TestIsAppHit(t *testing.T) {
	for code, want := range map[uintptr]bool{
		htClient: true, htCaption: true, htMinButton: true, htMaxButton: true, htClose: true,
		htLeft: false, htTop: false, htBottomRight: false, 0: false, 0xFFFE: false, // HTERROR as a WORD
	} {
		if got := isAppHit(code); got != want {
			t.Errorf("isAppHit(%d) = %v, want %v", code, got, want)
		}
	}
	for code, want := range map[uintptr]bool{htMinButton: true, htMaxButton: true, htClose: true, htCaption: false, htClient: false} {
		if got := isAppButton(code); got != want {
			t.Errorf("isAppButton(%d) = %v, want %v", code, got, want)
		}
	}
}

// titleBar is a 400 px wide custom title bar: a caption with a search field
// punched into it and three buttons on the right.
var titleBar = []platform.HitRegion{
	{Kind: platform.HitCaption, Rect: platform.Rect{W: 400, H: 32}},
	{Kind: platform.HitClient, Rect: platform.Rect{X: 100, Y: 4, W: 120, H: 24}},
	{Kind: platform.HitMinimize, Rect: platform.Rect{X: 280, W: 40, H: 32}},
	{Kind: platform.HitMaximize, Rect: platform.Rect{X: 320, W: 40, H: 32}},
	{Kind: platform.HitClose, Rect: platform.Rect{X: 360, W: 40, H: 32}},
}

func TestClientHitRegions(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		in := hitInput{client: image.Pt(int(400*scale), int(300*scale)), scale: scale, regions: titleBar}
		for _, tc := range []struct {
			x, y float64 // logical
			want uintptr
		}{
			{10, 10, htCaption},
			{0, 0, htCaption}, // no band: a decorated window
			{150, 10, htClient},
			{290, 10, htMinButton},
			{330, 31, htMaxButton},
			{399, 0, htClose},
			{200, 32, htClient}, // below the bar
			{200, 200, htClient},
		} {
			in.pt = image.Pt(int(tc.x*scale), int(tc.y*scale))
			if got := clientHit(in); got != tc.want {
				t.Errorf("scale %v: clientHit at %v,%v = %d, want %d", scale, tc.x, tc.y, got, tc.want)
			}
		}
	}
}

func TestClientHitResizeBand(t *testing.T) {
	in := hitInput{client: image.Pt(400, 300), scale: 1, border: 8, regions: titleBar}
	for _, tc := range []struct {
		x, y int
		want uintptr
	}{
		{0, 0, htTopLeft},
		{15, 0, htTopLeft}, // corners reach twice the band along the edges
		{0, 15, htTopLeft},
		{16, 0, htTop},
		{200, 7, htTop}, // the band takes precedence over the caption
		{200, 8, htCaption},
		{399, 0, htTopRight}, // and over the close button
		{392, 20, htRight},
		{391, 20, htClose},
		{0, 150, htLeft},
		{399, 150, htRight},
		{200, 299, htBottom},
		{0, 299, htBottomLeft},
		{399, 299, htBottomRight},
		{385, 299, htBottomRight},
		{200, 150, htClient},
	} {
		in.pt = image.Pt(tc.x, tc.y)
		if got := clientHit(in); got != tc.want {
			t.Errorf("clientHit at %d,%d = %d, want %d", tc.x, tc.y, got, tc.want)
		}
	}
	// Fullscreen: no band (the caller passes 0) and the caption is content.
	in.border, in.fullscreen = 0, true
	in.pt = image.Pt(10, 10)
	if got := clientHit(in); got != htClient {
		t.Errorf("fullscreen caption = %d, want HTCLIENT", got)
	}
	in.pt = image.Pt(370, 10)
	if got := clientHit(in); got != htClose {
		t.Errorf("fullscreen close button = %d, want HTCLOSE", got)
	}
}

func TestNCButtonFromMessage(t *testing.T) {
	for _, tc := range []struct {
		msg    uint32
		wparam uintptr
		btn    platform.PointerButton
		down   bool
		hit    uintptr
		ok     bool
	}{
		{wmNCLButtonDown, htClose, platform.ButtonPrimary, true, htClose, true},
		{wmNCLButtonDblClk, htMaxButton, platform.ButtonPrimary, true, htMaxButton, true},
		{wmNCLButtonUp, htMinButton, platform.ButtonPrimary, false, htMinButton, true},
		{wmNCRButtonDown, htCaption, platform.ButtonSecondary, true, htCaption, true},
		{wmNCRButtonDblClk, htClose, platform.ButtonSecondary, true, htClose, true},
		{wmNCRButtonUp, htClose, platform.ButtonSecondary, false, htClose, true},
		{wmNCMButtonDown, htClose, platform.ButtonMiddle, true, htClose, true},
		{wmNCMButtonDblClk, htClose, platform.ButtonMiddle, true, htClose, true},
		{wmNCMButtonUp, htClose, platform.ButtonMiddle, false, htClose, true},
		{wmNCXButtonDown, htClose | xButton1<<16, platform.ButtonBack, true, htClose, true},
		{wmNCXButtonDblClk, htMaxButton | xButton2<<16, platform.ButtonForward, true, htMaxButton, true},
		{wmNCXButtonUp, htMinButton | xButton2<<16, platform.ButtonForward, false, htMinButton, true},
		{wmNCXButtonDown, htClose | 7<<16, platform.ButtonNone, false, htClose, false},
		{wmNCMouseMove, htClose, platform.ButtonNone, false, htClose, false},
	} {
		btn, down, hit, ok := ncButtonFromMessage(tc.msg, tc.wparam)
		if btn != tc.btn || down != tc.down || hit != tc.hit || ok != tc.ok {
			t.Errorf("ncButtonFromMessage(%#x, %#x) = %v %v %d %v, want %v %v %d %v",
				tc.msg, tc.wparam, btn, down, hit, ok, tc.btn, tc.down, tc.hit, tc.ok)
		}
	}
}

func TestReportedState(t *testing.T) {
	pos := image.Pt(-1200, 40)
	for _, tc := range []struct {
		iconic, zoomed, fs, fsMax, frameless bool
		want                                 platform.WindowState
	}{
		{want: platform.WindowState{HasPosition: true, Position: pos}},
		{iconic: true, frameless: true, want: platform.WindowState{Minimized: true, ClientSideDecorations: true, HasPosition: true, Position: pos}},
		{zoomed: true, want: platform.WindowState{Maximized: true, HasPosition: true, Position: pos}},
		{fs: true, want: platform.WindowState{Fullscreen: true, HasPosition: true, Position: pos}},
		{fs: true, fsMax: true, want: platform.WindowState{Fullscreen: true, Maximized: true, HasPosition: true, Position: pos}},
		// In fullscreen Maximized is the state to return to, whatever
		// the style says.
		{fs: true, zoomed: true, want: platform.WindowState{Fullscreen: true, HasPosition: true, Position: pos}},
	} {
		if got := reportedState(tc.iconic, tc.zoomed, tc.fs, tc.fsMax, tc.frameless, pos); got != tc.want {
			t.Errorf("reportedState(%+v) = %v, want %v", tc, got, tc.want)
		}
	}
}

func TestCenterOver(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1040)
	for _, tc := range []struct {
		parent image.Rectangle
		size   image.Point
		work   image.Rectangle
		want   image.Point
	}{
		{image.Rect(100, 100, 900, 700), image.Pt(400, 300), work, image.Pt(300, 250)},
		// Near the right edge: moved inside the work area.
		{image.Rect(1700, 100, 1920, 300), image.Pt(400, 300), work, image.Pt(1520, 50)},
		// Above the top: moved down.
		{image.Rect(0, -400, 400, 0), image.Pt(200, 200), work, image.Pt(100, 0)},
		// Larger than the work area: its top-left stays visible.
		{image.Rect(0, 0, 800, 600), image.Pt(3000, 2000), work, image.Pt(0, 0)},
		// A secondary monitor on the left.
		{image.Rect(-1500, 200, -700, 800), image.Pt(400, 300), image.Rect(-1920, 0, 0, 1080), image.Pt(-1300, 350)},
		// No work area known: plain centering.
		{image.Rect(0, 0, 100, 100), image.Pt(400, 300), image.Rectangle{}, image.Pt(-150, -100)},
	} {
		if got := centerOver(tc.parent, tc.size, tc.work); got != tc.want {
			t.Errorf("centerOver(%v, %v, %v) = %v, want %v", tc.parent, tc.size, tc.work, got, tc.want)
		}
	}
}

func TestMaximizedClient(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1040) // task bar at the bottom
	// Windows puts the 8 px borders of a maximized window outside the work
	// area.
	proposed := image.Rect(-8, -8, 1928, 1048)
	if got := maximizedClient(proposed, work); got != work {
		t.Errorf("maximizedClient = %v, want the work area %v", got, work)
	}
	// Clamped by a maximum size: only the part on the monitor.
	small := image.Rect(-8, -8, 1000, 700)
	if got, want := maximizedClient(small, work), image.Rect(0, 0, 1000, 700); got != want {
		t.Errorf("maximizedClient(%v) = %v, want %v", small, got, want)
	}
	// Unknown work area, or no overlap: unchanged.
	if got := maximizedClient(proposed, image.Rectangle{}); got != proposed {
		t.Errorf("maximizedClient without work area = %v", got)
	}
	if got := maximizedClient(proposed, image.Rect(5000, 0, 6000, 1000)); got != proposed {
		t.Errorf("maximizedClient without overlap = %v", got)
	}
}

func TestIconBGRA(t *testing.T) {
	// Exact size: a straight conversion, non-premultiplied BGRA.
	img := image.NewRGBA(image.Rect(10, 10, 26, 26)) // not at the origin
	for y := 10; y < 26; y++ {
		for x := 10; x < 26; x++ {
			img.SetRGBA(x, y, color.RGBA{0x80, 0x20, 0x00, 0x80}) // premultiplied, half transparent
		}
	}
	px := iconBGRA(img, 16)
	if len(px) != 4*16*16 {
		t.Fatalf("len = %d", len(px))
	}
	if got, want := [4]byte(px[:4]), [4]byte{0x00, 0x3f, 0xff, 0x80}; got != want { // G: 0x2020 * 0xffff / 0x8080 rounds down
		t.Errorf("first pixel BGRA = %v, want %v", got, want)
	}

	// Scaled down: a red disc on transparent stays red where it is
	// visible (no dark fringe from scaling non-premultiplied pixels).
	big := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			dx, dy := x-32, y-32
			if dx*dx+dy*dy < 24*24 {
				big.SetNRGBA(x, y, color.NRGBA{0xff, 0, 0, 0xff})
			}
		}
	}
	px = iconBGRA(big, 20)
	if len(px) != 4*20*20 {
		t.Fatalf("len = %d", len(px))
	}
	opaque, clear := false, false
	for i := 0; i < len(px); i += 4 {
		b, g, r, a := px[i], px[i+1], px[i+2], px[i+3]
		if a > 0x20 && (r < 0xf0 || g > 0x10 || b > 0x10) {
			t.Fatalf("pixel %d is %v: a visible pixel lost its color", i/4, px[i:i+4])
		}
		opaque = opaque || a == 0xff
		clear = clear || a == 0
	}
	if !opaque || !clear {
		t.Errorf("scaled icon has opaque pixels %v and transparent pixels %v; want both", opaque, clear)
	}
}
