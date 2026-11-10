package platform

import (
	"image"
	"testing"
)

// cardHandles are h.WindowResize's handles of a 200x100 card at 20,10.
var cardHandles = []HitRegion{
	{Kind: HitCaption, Rect: Rect{X: 20, Y: 10, W: 200, H: 32}},
	{Kind: HitResizeTop, Rect: Rect{X: 20, Y: 10, W: 200, H: 5}},
	{Kind: HitResizeBottom, Rect: Rect{X: 20, Y: 105, W: 200, H: 5}},
	{Kind: HitResizeLeft, Rect: Rect{X: 20, Y: 10, W: 5, H: 100}},
	{Kind: HitResizeRight, Rect: Rect{X: 215, Y: 10, W: 5, H: 100}},
	{Kind: HitResizeTopLeft, Rect: Rect{X: 20, Y: 10, W: 16, H: 5}},
	{Kind: HitResizeBottomRight, Rect: Rect{X: 204.5, Y: 105, W: 15.5, H: 5}},
}

// The handles' rectangles in physical pixels, rounded outward and cut to
// the frame; the caption is not a handle.
func TestHandleRects(t *testing.T) {
	got := HandleRects(cardHandles, 1.5, image.Pt(320, 160))
	want := []image.Rectangle{
		image.Rect(30, 15, 330, 23).Intersect(image.Rect(0, 0, 320, 160)),
		image.Rect(30, 157, 320, 160),
		image.Rect(30, 15, 38, 160),
		image.Rect(322, 15, 320, 160).Intersect(image.Rect(0, 0, 320, 160)), // cut away
		image.Rect(30, 15, 54, 23),
		image.Rect(306, 157, 320, 160),
	}
	var keep []image.Rectangle
	for _, r := range want {
		if !r.Empty() {
			keep = append(keep, r)
		}
	}
	if !SameRects(got, keep) {
		t.Errorf("HandleRects\n got %v\nwant %v", got, keep)
	}
	if HandleRects(cardHandles[:1], 1, image.Pt(320, 160)) != nil {
		t.Error("a caption alone has handle rectangles")
	}
}

// The input of a transparent window with its handles: the mask's runs
// and the handles merged row by row into YX bands, the mask unchanged.
func TestInputMaskRectsWith(t *testing.T) {
	fb := image.NewRGBA(image.Rect(0, 0, 10, 6))
	for y := 2; y < 5; y++ {
		for x := 3; x < 8; x++ {
			fb.Pix[fb.PixOffset(x, y)+3] = 255
		}
	}
	var m InputMask
	m.Update(fb, []image.Rectangle{fb.Rect})
	got := m.RectsWith([]image.Rectangle{image.Rect(1, 1, 4, 3), image.Rect(7, 4, 12, 5)}, 100)
	want := []image.Rectangle{
		image.Rect(1, 1, 4, 2),
		image.Rect(1, 2, 8, 3),
		image.Rect(3, 3, 8, 4),
		image.Rect(3, 4, 10, 5),
	}
	if !SameRects(got, want) {
		t.Errorf("RectsWith\n got %v\nwant %v", got, want)
	}
	if got := m.Rects(100); !SameRects(got, []image.Rectangle{image.Rect(3, 2, 8, 5)}) {
		t.Errorf("the mask changed: %v", got)
	}
	if got := m.RectsWith([]image.Rectangle{image.Rect(0, 0, 1, 1), image.Rect(9, 0, 10, 1)}, 1); !SameRects(got, []image.Rectangle{image.Rect(0, 0, 10, 6)}) {
		t.Errorf("RectsWith limited to one = %v, want the cover", got)
	}
}

// The transparent pixels under the handles get HandleAlpha, and lose it
// when the handles move; painted pixels are left as they are.
func TestHandleCatch(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Pix[img.PixOffset(1, 1)+3] = 200 // a painted pixel
	img.Pix[img.PixOffset(1, 1)] = 100
	var c HandleCatch
	first := []image.Rectangle{image.Rect(0, 1, 3, 2)}
	if d := c.Update(img.Pix, img.Stride, first, nil); d != first[0] {
		t.Errorf("first catch dirty %v, want %v", d, first[0])
	}
	for x := range 4 {
		for y := range 3 {
			a := img.Pix[img.PixOffset(x, y)+3]
			want := uint8(0)
			switch {
			case x == 1 && y == 1:
				want = 200
			case y == 1 && x < 3:
				want = HandleAlpha
			}
			if a != want {
				t.Errorf("caught: alpha at %d,%d = %d, want %d", x, y, a, want)
			}
		}
	}
	if !c.Contains(image.Pt(2, 1)) || c.Contains(image.Pt(3, 1)) {
		t.Error("Contains disagrees with the handles")
	}
	// A copy of damage cleared two caught pixels: the same handles catch
	// again the one in the damage only (the other stays as the copy left
	// it: nothing else touched it) and report nothing dirty.
	img.Pix[img.PixOffset(0, 1)+3] = 0
	img.Pix[img.PixOffset(2, 1)+3] = 0
	if d := c.Update(img.Pix, img.Stride, first, []image.Rectangle{image.Rect(0, 0, 1, 3)}); !d.Empty() ||
		img.Pix[img.PixOffset(0, 1)+3] != HandleAlpha || img.Pix[img.PixOffset(2, 1)+3] != 0 {
		t.Errorf("same handles: dirty %v, alpha %d and %d", d, img.Pix[img.PixOffset(0, 1)+3], img.Pix[img.PixOffset(2, 1)+3])
	}
	img.Pix[img.PixOffset(2, 1)+3] = HandleAlpha
	// The handles move: the old pixels are released.
	next := []image.Rectangle{image.Rect(3, 0, 4, 3)}
	if d := c.Update(img.Pix, img.Stride, next, nil); d != image.Rect(0, 0, 4, 3) {
		t.Errorf("moved handles dirty %v", d)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		x := (i / 4) % 4
		want := uint8(0)
		switch {
		case i == img.PixOffset(1, 1)+3:
			want = 200
		case x == 3:
			want = HandleAlpha
		}
		if img.Pix[i] != want {
			t.Errorf("moved: alpha %d at pixel %d, want %d", img.Pix[i], i/4, want)
		}
	}
	if img.Pix[img.PixOffset(1, 1)] != 100 {
		t.Error("the painted pixel changed")
	}
}

// The floor keeps the handles' extent at two corners, the margin around
// them as it is; it never grows a window, and a window without handles
// keeps two corners of its own band.
func TestResizeFloor(t *testing.T) {
	for _, c := range []struct {
		name    string
		size    Size
		regions []HitRegion
		want    Size
	}{
		// The card spans 20..220 by 10..110 in a 240x120 window: margins
		// 40 and 20.
		{"card", Size{W: 240, H: 120}, cardHandles, Size{W: 72, H: 52}},
		{"no handles", Size{W: 240, H: 120}, nil, Size{W: 32, H: 32}},
		{"small window", Size{W: 30, H: 20}, nil, Size{W: 30, H: 20}},
		// A wide shadow margin of 60: the 32 pt floor of before left no card.
		{"wide margin", Size{W: 400, H: 300}, []HitRegion{
			{Kind: HitResizeLeft, Rect: Rect{X: 60, Y: 60, W: 5, H: 180}},
			{Kind: HitResizeRight, Rect: Rect{X: 335, Y: 60, W: 5, H: 180}},
			{Kind: HitResizeTop, Rect: Rect{X: 60, Y: 60, W: 280, H: 5}},
			{Kind: HitResizeBottom, Rect: Rect{X: 60, Y: 235, W: 280, H: 5}},
		}, Size{W: 152, H: 152}},
		// Only the right side resizes: the window's left edge is the
		// other end.
		{"right only", Size{W: 400, H: 300}, []HitRegion{
			{Kind: HitResizeRight, Rect: Rect{X: 375, Y: 20, W: 5, H: 260}},
		}, Size{W: 52, H: 32}},
		// A badge whose card is the whole window.
		{"badge", Size{W: 100, H: 30}, []HitRegion{
			{Kind: HitResizeLeft, Rect: Rect{X: 0, Y: 0, W: 5, H: 30}},
			{Kind: HitResizeRight, Rect: Rect{X: 95, Y: 0, W: 5, H: 30}},
			{Kind: HitResizeBottomRight, Rect: Rect{X: 84, Y: 25, W: 16, H: 5}},
		}, Size{W: 32, H: 30}},
	} {
		if got := ResizeFloor(c.size, c.regions); got != c.want {
			t.Errorf("%s: ResizeFloor = %v, want %v", c.name, got, c.want)
		}
	}
}

// benchHandles are h.WindowResize's handles of a 760x560 card in an
// 800x600 window at scale 2, in physical pixels.
func benchHandles() (*image.RGBA, []image.Rectangle) {
	var regions []HitRegion
	for _, e := range []Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight} {
		r := Rect{X: 20, Y: 20, W: 760, H: 560}
		switch e {
		case EdgeTop:
			r.H = 5
		case EdgeBottom:
			r.Y, r.H = 575, 5
		case EdgeLeft:
			r.W = 5
		case EdgeRight:
			r.X, r.W = 775, 5
		}
		regions = append(regions, HitRegion{Kind: HitResize(e), Rect: r})
	}
	fb := image.NewRGBA(image.Rect(0, 0, 1600, 1200))
	return fb, HandleRects(regions, 2, fb.Rect.Size())
}

// BenchmarkHandleCatchUnchanged is a Present's catch of unchanged handles
// (a frame that damaged a small part of the card, away from its edges);
// BenchmarkHandleCatchEdge the same with damage across the card's left
// edge.
func BenchmarkHandleCatchUnchanged(b *testing.B) {
	benchCatch(b, image.Rect(400, 400, 600, 440))
}
func BenchmarkHandleCatchEdge(b *testing.B) { benchCatch(b, image.Rect(0, 400, 600, 440)) }

func benchCatch(b *testing.B, damage image.Rectangle) {
	fb, handles := benchHandles()
	var c HandleCatch
	c.Update(fb.Pix, fb.Stride, handles, nil)
	d := []image.Rectangle{damage}
	for b.Loop() {
		c.Update(fb.Pix, fb.Stride, handles, d)
	}
}
