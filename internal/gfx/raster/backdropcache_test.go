package raster

import (
	"bytes"
	"fmt"
	"image"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// glassState is a frame of glassFrames: stripes behind two glass panes
// (the second one overlaps the first, so it reads its output), a box
// inside each pane above its filter, and a faded third pane.
type glassState struct {
	stripe float32 // the x of a stripe behind the panes
	box    [2]gfx.Point
	pane2  gfx.Point
	fade   float32
}

// list builds the frame; with upto >= 0 it stops before the upto-th
// backdrop filter (what that filter reads).
func (s glassState) list(upto int) *gfx.DisplayList {
	b := gfx.NewBuilder()
	k := 0
	stop := func() bool { k++; return upto >= 0 && k > upto }
	b.FillRect(gfx.Rect{X1: 400, Y1: 300}, gfx.LinearGradient{End: gfx.Pt(400, 300), Stops: stops(indigo, rose, amber)})
	for x := float32(0); x < 400; x += 22 {
		b.FillRect(gfx.RectXYWH(x, 0, 8, 300), gfx.White.WithAlpha(0.6))
	}
	b.FillRect(gfx.RectXYWH(s.stripe, 40, 14, 220), slate)
	p1 := gfx.NewRRect(gfx.RectXYWH(30, 30, 220, 200), 16)
	if stop() {
		return b.Finish()
	}
	b.BackdropFilter(p1, gfx.Blur{StdDev: 30}) // a downsampled blur at scale 2
	b.FillRRect(p1, gfx.White.WithAlpha(0.15))
	b.PushClipRRect(p1)
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(s.box[0].X, s.box[0].Y, 40, 30), 6), gfx.MustParseHex("#0ea5e9"))
	b.PopClip()
	p2 := gfx.NewRRect(gfx.RectXYWH(s.pane2.X, s.pane2.Y, 150, 120), 12)
	if stop() {
		return b.Finish()
	}
	b.BackdropFilter(p2, gfx.Blur{StdDev: 6}, gfx.Saturate(1.4))
	b.FillRRect(p2, gfx.Black.WithAlpha(0.2))
	b.FillRect(gfx.RectXYWH(s.pane2.X+s.box[1].X, s.pane2.Y+s.box[1].Y, 30, 20), gfx.MustParseHex("#f59e0b"))
	if stop() {
		return b.Finish()
	}
	b.BackdropFilterOpacity(gfx.NewRRect(gfx.RectXYWH(260, 180, 120, 100), 10), s.fade, gfx.Blur{StdDev: 12})
	return b.Finish()
}

// TestBackdropCache renders frames of glassFrames into one destination,
// each one only inside the pixels that changed (plus random regions), with
// a BackdropCache that carries the filters whose input (the pixels before
// them) did not change: every frame equals an uncached full render, and
// filters are drawn from the cache, at scales 1 and 2, sequentially and in
// phases.
func TestBackdropCache(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, workers := range []int{1, 0} {
			t.Run(fmt.Sprintf("x%g_w%d", scale, workers), func(t *testing.T) {
				testBackdropCache(t, scale, workers)
			})
		}
	}
}

func testBackdropCache(t *testing.T, scale float32, workers int) {
	w, h := int(400*scale), int(300*scale)
	full := func(dl *gfx.DisplayList) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		Clear(img, image.Rectangle{}, gfx.White)
		NewRenderer().Render(img, dl, Options{Scale: scale, Workers: 1})
		return img
	}
	rng := rand.New(rand.NewPCG(198, uint64(scale)))
	s := glassState{stripe: 120, box: [2]gfx.Point{gfx.Pt(60, 60), gfx.Pt(20, 20)}, pane2: gfx.Pt(170, 120), fade: 0.6}
	var c BackdropCache
	var r Renderer
	fb := image.NewRGBA(image.Rect(0, 0, w, h))
	Clear(fb, image.Rectangle{}, gfx.White)
	dl := s.list(-1)
	r.Render(fb, dl, Options{Scale: scale, Workers: workers, Backdrops: &c})
	prev, prevImg := s, full(dl)
	steps := 60
	if testing.Short() {
		steps = 10 // arm64 under qemu (ADR-027)
	}
	for step := range steps {
		var what string
		switch rng.IntN(5) {
		case 0, 1:
			s.box[0] = gfx.Pt(40+float32(rng.IntN(150)), 40+float32(rng.IntN(150)))
			what = "box 1"
		case 2:
			s.box[1] = gfx.Pt(float32(rng.IntN(110)), float32(rng.IntN(90)))
			what = "box 2"
		case 3:
			s.stripe = float32(rng.IntN(380))
			what = "behind"
		default:
			if rng.IntN(2) == 0 {
				s.pane2 = gfx.Pt(120+float32(rng.IntN(100)), 80+float32(rng.IntN(100)))
				what = "pane 2"
			} else {
				s.fade = float32(rng.IntN(10)) / 10
				what = "fade"
			}
		}
		dl = s.list(-1)
		want := full(dl)
		// The oracle of the carry: a filter whose input pixels are the
		// same in both frames keeps its entry.
		carry := make([]int, dl.NumBackdrops())
		for k := range carry {
			carry[k] = -1
			if bytes.Equal(full(prev.list(k)).Pix, full(s.list(k)).Pix) && filterSame(prev, s, k) {
				carry[k] = k
			}
		}
		c.Retain(dl, scale, carry)
		changed := diffBounds(prevImg, want)
		regions := []image.Rectangle{changed}
		for range rng.IntN(3) {
			x, y := rng.IntN(w), rng.IntN(h)
			regions = append(regions, image.Rect(x, y, x+1+rng.IntN(w/3), y+1+rng.IntN(h/3)))
		}
		for _, rg := range regions {
			if rg.Empty() {
				continue
			}
			Clear(fb, rg, gfx.White)
			r.Render(fb, dl, Options{Scale: scale, Workers: workers, Region: rg, Backdrops: &c})
		}
		if !bytes.Equal(fb.Pix, want.Pix) {
			p, _ := firstDiffRGBA(fb, want)
			t.Fatalf("step %d (%s): pixel %v = %v, want %v (regions %v, carry %v)", step, what, p, pixel(fb, p.X, p.Y), pixel(want, p.X, p.Y), regions, carry)
		}
		prev, prevImg = s, want
	}
	if c.hits == 0 {
		t.Error("no backdrop filter was drawn from the cache")
	}
	t.Logf("%d filters drawn from the cache, %d bytes held", c.hits, c.bytes)
	if budget := 2 * 4 * w * h; c.bytes > budget {
		t.Errorf("the cache holds %d bytes, over its budget %d", c.bytes, budget)
	}
}

// filterSame reports whether the k-th backdrop filter is the same command
// in both frames.
func filterSame(a, b glassState, k int) bool {
	da, db := a.list(-1), b.list(-1)
	i, _, _, _ := da.Backdrop(k)
	j, _, _, _ := db.Backdrop(k)
	return gfx.Equal(da.At(i), db.At(j)) && da.BoundsAt(i) == db.BoundsAt(j)
}

// diffBounds returns the bounding box of the pixels that differ.
func diffBounds(a, b *image.RGBA) image.Rectangle {
	var out image.Rectangle
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i := a.PixOffset(x, y)
			if !bytes.Equal(a.Pix[i:i+4], b.Pix[i:i+4]) {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}

func firstDiffRGBA(a, b *image.RGBA) (image.Point, bool) {
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i := a.PixOffset(x, y)
			if !bytes.Equal(a.Pix[i:i+4], b.Pix[i:i+4]) {
				return image.Pt(x, y), false
			}
		}
	}
	return image.Point{}, true
}

// TestBackdropCacheKey: an entry carried for a filter that moved, or
// whose blur changed, is not drawn (the cache checks what it can), and
// Reset empties the cache.
func TestBackdropCacheKey(t *testing.T) {
	list := func(x, sigma float32) *gfx.DisplayList {
		return build(func(b *gfx.Builder) {
			backdropScene(b)
			b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(x, 10, 60, 40), 8), gfx.Blur{StdDev: sigma})
		})
	}
	var c BackdropCache
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for _, next := range []*gfx.DisplayList{list(20, 5), list(40, 5), list(40, 9)} {
		c.Retain(next, 1, []int{0, 1}) // carried, though they changed
		Clear(img, image.Rectangle{}, gfx.White)
		NewRenderer().Render(img, next, Options{Workers: 1, Backdrops: &c})
		want := image.NewRGBA(img.Rect)
		Clear(want, image.Rectangle{}, gfx.White)
		NewRenderer().Render(want, next, Options{Workers: 1})
		if !bytes.Equal(img.Pix, want.Pix) {
			t.Fatal("a stale entry was drawn")
		}
	}
	if c.hits != 2 { // the first filter, unchanged, in the second and third frames
		t.Errorf("%d hits, want 2", c.hits)
	}
	c.Reset()
	if c.dl != nil {
		t.Error("Reset kept the list")
	}
}

// TestBackdropCacheFlip: a filter carried with a partial cover whose
// transform changed while its bounds, filters and device rectangle did
// not (a mirrored pane: the same pixels in, the same pixels out) is drawn
// as an uncached render draws it.
func TestBackdropCacheFlip(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, workers := range []int{1, 0} {
			t.Run(fmt.Sprintf("x%g_w%d", scale, workers), func(t *testing.T) {
				list := func(flip bool) *gfx.DisplayList {
					return build(func(bl *gfx.Builder) {
						bl.FillRect(gfx.Rect{X1: 400, Y1: 300}, gfx.LinearGradient{End: gfx.Pt(400, 300), Stops: stops(indigo, rose, amber)})
						for x := float32(0); x < 400; x += 11 {
							bl.FillRect(gfx.RectXYWH(x, 0, 4, 300), gfx.White.WithAlpha(0.7))
						}
						if flip {
							bl.PushTransform(gfx.ScaleAbout(-1, 1, gfx.Pt(200, 150)))
						}
						bl.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(120, 80, 160, 120), 12), gfx.Blur{StdDev: 10})
						if flip {
							bl.PopTransform()
						}
						bl.FillRect(gfx.RectXYWH(150, 100, 20, 20), slate)
					})
				}
				w, h := int(400*scale), int(300*scale)
				var c BackdropCache
				var r Renderer
				r.tune.minPixels, r.tune.bandRows = 1, 7
				fb := image.NewRGBA(image.Rect(0, 0, w, h))
				Clear(fb, image.Rectangle{}, gfx.White)
				l0, l1 := list(false), list(true)
				if l0.BoundsAt(l0.Len()-2) != l1.BoundsAt(l1.Len()-3) {
					t.Fatalf("the flip moved the filter's bounds: %v, %v", l0.BoundsAt(l0.Len()-2), l1.BoundsAt(l1.Len()-3))
				}
				r.Render(fb, l0, Options{Scale: scale, Workers: workers, Backdrops: &c})
				rg := devRootRect(gfx.RectXYWH(180, 120, 30, 30), scale)
				for frame, f := range []struct {
					dl    *gfx.DisplayList
					carry int
				}{{l0, -1}, {l1, 0}} {
					c.Retain(f.dl, scale, []int{f.carry})
					want := image.NewRGBA(fb.Rect)
					Clear(want, image.Rectangle{}, gfx.White)
					NewRenderer().Render(want, f.dl, Options{Scale: scale, Workers: 1})
					Clear(fb, rg, gfx.White)
					r.Render(fb, f.dl, Options{Scale: scale, Workers: workers, Region: rg, Backdrops: &c})
					if p, same := firstDiffRGBA(fb, want); !same {
						t.Fatalf("frame %d: pixel %v = %v, want %v", frame+1, p, pixel(fb, p.X, p.Y), pixel(want, p.X, p.Y))
					}
				}
			})
		}
	}
}

// pane is a glass pane of stackScene: a backdrop blur over rect.
type pane struct {
	rect  gfx.Rect
	sigma float32
}

// stackScene is a static frame with overlapping backdrop filters, one per
// pane, each one painted above the previous ones (it reads their output).
func stackScene(panes ...pane) *gfx.DisplayList {
	return build(func(bl *gfx.Builder) {
		bl.FillRect(gfx.Rect{X1: 400, Y1: 300}, gfx.LinearGradient{End: gfx.Pt(400, 300), Stops: stops(indigo, rose, amber)})
		for x := float32(0); x < 400; x += 11 {
			bl.FillRect(gfx.RectXYWH(x, 0, 4, 300), gfx.White.WithAlpha(0.7))
		}
		for y := float32(3); y < 300; y += 13 {
			bl.FillRect(gfx.RectXYWH(0, y, 400, 3), slate.WithAlpha(0.5))
		}
		for k, p := range panes {
			bl.BackdropFilter(gfx.NewRRect(p.rect, 12), gfx.Blur{StdDev: p.sigma})
			bl.FillRRect(gfx.NewRRect(p.rect, 12), gfx.Black.WithAlpha(0.1+0.05*float32(k)))
		}
	})
}

// TestBackdropCacheStack: the later of two overlapping backdrop filters
// is carried with a partial cover (a frame rendered only a region) and
// drawn from the cache, while the earlier one, not carried, grows the
// region by its reach into the later one's shape. Every frame equals an
// uncached full render (the list never changes, so carrying any entry is
// honest), sequentially and in phases, and the later filter is drawn from
// the cache.
func TestBackdropCacheStack(t *testing.T) {
	type geo struct {
		name string
		a, b pane
		dmg  gfx.Rect // the region of the second and third frames
	}
	geos := []geo{
		{"inside", pane{gfx.RectXYWH(20, 20, 360, 260), 4}, pane{gfx.RectXYWH(120, 80, 160, 120), 10}, gfx.RectXYWH(180, 120, 30, 30)},
		{"edge", pane{gfx.RectXYWH(20, 20, 360, 260), 4}, pane{gfx.RectXYWH(120, 80, 160, 120), 10}, gfx.RectXYWH(110, 120, 20, 30)},
		{"wide a", pane{gfx.RectXYWH(20, 20, 360, 260), 16}, pane{gfx.RectXYWH(120, 80, 160, 120), 6}, gfx.RectXYWH(180, 120, 30, 30)},
		{"partial b", pane{gfx.RectXYWH(20, 20, 260, 260), 8}, pane{gfx.RectXYWH(200, 60, 180, 140), 12}, gfx.RectXYWH(240, 100, 30, 30)},
	}
	for _, g := range geos {
		for _, scale := range []float32{1, 2} {
			for _, workers := range []int{1, 0} {
				t.Run(fmt.Sprintf("%s_x%g_w%d", g.name, scale, workers), func(t *testing.T) {
					w, h := int(400*scale), int(300*scale)
					dl := stackScene(g.a, g.b)
					want := image.NewRGBA(image.Rect(0, 0, w, h))
					Clear(want, image.Rectangle{}, gfx.White)
					NewRenderer().Render(want, dl, Options{Scale: scale, Workers: 1})
					var c BackdropCache
					var r Renderer
					r.tune.minPixels, r.tune.bandRows = 1, 7
					fb := image.NewRGBA(want.Rect)
					Clear(fb, image.Rectangle{}, gfx.White)
					r.Render(fb, dl, Options{Scale: scale, Workers: workers, Backdrops: &c})
					rg := devRootRect(g.dmg, scale)
					for frame, carry := range [][]int{{-1, -1}, {-1, 1}, {-1, 1}} {
						c.Retain(dl, scale, carry)
						hits := c.hits
						Clear(fb, rg, gfx.White)
						r.Render(fb, dl, Options{Scale: scale, Workers: workers, Region: rg, Backdrops: &c})
						if p, same := firstDiffRGBA(fb, want); !same {
							t.Fatalf("frame %d (carry %v, region %v, %d hits): pixel %v = %v, want %v", frame+1, carry, rg, c.hits-hits, p, pixel(fb, p.X, p.Y), pixel(want, p.X, p.Y))
						}
						if frame > 0 && c.hits == hits {
							t.Errorf("frame %d: the carried filter was not drawn from the cache", frame+1)
						}
					}
				})
			}
		}
	}
}

// TestBackdropCacheCarry renders a static list of three overlapping
// backdrop filters into one destination, each frame inside random regions
// after carrying a random subset of the entries (all honest: the input
// never changes): every frame equals an uncached full render, whatever the
// covers the entries were left with, sequentially and in phases.
func TestBackdropCacheCarry(t *testing.T) {
	dl := stackScene(
		pane{gfx.RectXYWH(10, 10, 300, 270), 6},
		pane{gfx.RectXYWH(90, 50, 290, 160), 14},
		pane{gfx.RectXYWH(150, 120, 120, 150), 3},
	)
	steps := 80
	if testing.Short() {
		steps = 12 // arm64 under qemu (ADR-027)
	}
	for _, scale := range []float32{1, 2} {
		for _, workers := range []int{1, 0} {
			t.Run(fmt.Sprintf("x%g_w%d", scale, workers), func(t *testing.T) {
				w, h := int(400*scale), int(300*scale)
				want := image.NewRGBA(image.Rect(0, 0, w, h))
				Clear(want, image.Rectangle{}, gfx.White)
				NewRenderer().Render(want, dl, Options{Scale: scale, Workers: 1})
				rng := rand.New(rand.NewPCG(200, uint64(scale)+uint64(workers)))
				var c BackdropCache
				var r Renderer
				r.tune.minPixels, r.tune.bandRows = 1, 7
				fb := image.NewRGBA(want.Rect)
				Clear(fb, image.Rectangle{}, gfx.White)
				r.Render(fb, dl, Options{Scale: scale, Workers: workers, Backdrops: &c})
				for step := range steps {
					carry := make([]int, dl.NumBackdrops())
					for k := range carry {
						carry[k] = -1
						if rng.IntN(3) > 0 {
							carry[k] = k
						}
					}
					c.Retain(dl, scale, carry)
					var regions []image.Rectangle
					for range 1 + rng.IntN(2) {
						x, y := rng.IntN(w), rng.IntN(h)
						rg := image.Rect(x, y, x+1+rng.IntN(w/4), y+1+rng.IntN(h/4))
						regions = append(regions, rg)
						Clear(fb, rg, gfx.White)
						r.Render(fb, dl, Options{Scale: scale, Workers: workers, Region: rg, Backdrops: &c})
					}
					if p, same := firstDiffRGBA(fb, want); !same {
						t.Fatalf("step %d (carry %v, regions %v): pixel %v = %v, want %v", step, carry, regions, p, pixel(fb, p.X, p.Y), pixel(want, p.X, p.Y))
					}
				}
				if c.hits == 0 {
					t.Error("no backdrop filter was drawn from the cache")
				}
				t.Logf("%d filters drawn from the cache", c.hits)
			})
		}
	}
}
