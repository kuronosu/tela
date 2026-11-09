package win32

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The app's resize handles (h.WindowResize, ADR-257) answer the resize
// codes from WM_NCHITTEST while the window can be resized, and are content
// otherwise; they are not the app's hits (the OS frame's cursor and loop).
func TestClientHitResizeHandles(t *testing.T) {
	regions := []platform.HitRegion{
		{Kind: platform.HitCaption, Rect: platform.Rect{X: 20, Y: 20, W: 200, H: 30}},
		{Kind: platform.HitResizeTop, Rect: platform.Rect{X: 20, Y: 20, W: 200, H: 5}},
		{Kind: platform.HitResizeLeft, Rect: platform.Rect{X: 20, Y: 20, W: 5, H: 100}},
		{Kind: platform.HitResizeTopLeft, Rect: platform.Rect{X: 20, Y: 20, W: 16, H: 5}},
		{Kind: platform.HitResizeBottomRight, Rect: platform.Rect{X: 204, Y: 115, W: 16, H: 5}},
	}
	for _, scale := range []float64{1, 2} {
		in := hitInput{client: image.Pt(int(300*scale), int(200*scale)), scale: scale, regions: regions, resizable: true}
		for _, tc := range []struct {
			x, y float64
			want uintptr
		}{
			{100, 22, htTop}, // over the caption: the handle is on top
			{22, 60, htLeft},
			{22, 22, htTopLeft},
			{210, 117, htBottomRight},
			{100, 40, htCaption},
			{100, 100, htClient},
		} {
			in.pt = image.Pt(int(tc.x*scale), int(tc.y*scale))
			if got := clientHit(in); got != tc.want {
				t.Errorf("scale %v: clientHit at %v,%v = %d, want %d", scale, tc.x, tc.y, got, tc.want)
			}
			if tc.want >= htLeft && tc.want <= htBottomRight && isAppHit(tc.want) {
				t.Errorf("a resize code is an app hit")
			}
		}
		in.resizable = false // FixedSize, maximized or fullscreen
		in.pt = image.Pt(int(22*scale), int(60*scale))
		if got := clientHit(in); got != htClient {
			t.Errorf("scale %v: a handle of a window that cannot resize = %d, want HTCLIENT", scale, got)
		}
	}
	for e, want := range map[platform.Edge]uintptr{
		platform.EdgeTop: htTop, platform.EdgeBottom: htBottom, platform.EdgeLeft: htLeft, platform.EdgeRight: htRight,
		platform.EdgeTop | platform.EdgeLeft: htTopLeft, platform.EdgeTop | platform.EdgeRight: htTopRight,
		platform.EdgeBottom | platform.EdgeLeft: htBottomLeft, platform.EdgeBottom | platform.EdgeRight: htBottomRight,
		0: htClient,
	} {
		if got := resizeHitCode(e); got != want {
			t.Errorf("resizeHitCode(%v) = %d, want %d", e, got, want)
		}
	}
}

// A caption press whose move loop moved nothing is a click (ADR-252).
func TestCaptionWasClick(t *testing.T) {
	r := image.Rect(100, 100, 500, 400)
	p := image.Pt(200, 110)
	for _, tc := range []struct {
		name    string
		after   image.Rectangle
		zoomed  bool
		release image.Point
		want    bool
	}{
		{"still", r, false, p, true},
		{"within the slop", r, false, p.Add(image.Pt(5, 5)), true},
		{"moved", r.Add(image.Pt(30, 0)), false, p.Add(image.Pt(30, 0)), false},
		{"there and back", r, false, p.Add(image.Pt(8, 0)), false},
		{"snapped maximized", r, true, p, false},
	} {
		if got := captionWasClick(r, tc.after, false, tc.zoomed, p, tc.release, 8); got != tc.want {
			t.Errorf("%s: captionWasClick = %v, want %v", tc.name, got, tc.want)
		}
	}
}
