package raster

import (
	"bytes"
	"fmt"
	"image"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// fromScene is a list without backdrop filters: stripes, clipped rounded
// boxes under a translation, a faded layer, text-like small fills and a
// masked layer, so that a render from a command index replays clips and
// transforms.
func fromScene() *gfx.DisplayList {
	return build(func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 400, Y1: 300}, gfx.LinearGradient{End: gfx.Pt(400, 300), Stops: stops(indigo, rose, amber)})
		for x := float32(0); x < 400; x += 26 {
			b.FillRect(gfx.RectXYWH(x, 0, 9, 300), gfx.White.WithAlpha(0.5))
		}
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(20, 20, 300, 240), 18))
		b.PushTransform(gfx.Translate(7, 3))
		for y := float32(10); y < 260; y += 31 {
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(30, y, 220, 22), 6), slate.WithAlpha(0.4))
			b.FillRect(gfx.RectXYWH(40, y+8, 120, 6), gfx.White.WithAlpha(0.8))
		}
		b.PushLayer(0.7, gfx.SrcOver)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(150, 80, 120, 90), 12), amber)
		b.PopLayer()
		b.PopTransform()
		b.PopClip()
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(250, 150, 130, 120), 20), rose.WithAlpha(0.6))
		// A masked layer late in the list: its mask list is drawn whole
		// whatever Options.From says (it names commands of this list).
		mask := build(func(m *gfx.Builder) {
			m.FillRRect(gfx.NewRRect(gfx.RectXYWH(60, 200, 80, 60), 30), gfx.Black)
			m.FillRect(gfx.RectXYWH(70, 210, 20, 20), gfx.Black)
		})
		b.PushMaskLayer(gfx.Mask{List: mask}, 1, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(40, 190, 140, 90), indigo)
		b.PopLayer()
	})
}

// TestRenderFrom renders lists from command indices over the pixels a
// snapshot took before them (Options.From, Base and Snap): every region
// equals a full render, whatever the scale, the workers, the cache of
// filtered backdrops and the region the snapshot was taken over, and the
// pixels outside the region are not touched. The base is either the
// snapshot (Base) or the destination holding its pixels.
func TestRenderFrom(t *testing.T) {
	scenes := map[string]*gfx.DisplayList{
		"plain": fromScene(),
		"glass": glassState{stripe: 120, box: [2]gfx.Point{gfx.Pt(60, 60), gfx.Pt(20, 20)}, pane2: gfx.Pt(170, 120), fade: 0.6}.list(-1),
	}
	for name, dl := range scenes {
		for _, scale := range []float32{1, 1.5} {
			for _, workers := range []int{1, 0} {
				for _, cached := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s_x%g_w%d_cache%v", name, scale, workers, cached), func(t *testing.T) {
						testRenderFrom(t, dl, scale, workers, cached)
					})
				}
			}
		}
	}
}

func testRenderFrom(t *testing.T, dl *gfx.DisplayList, scale float32, workers int, cached bool) {
	w, h := int(400*scale), int(300*scale)
	bounds := image.Rect(0, 0, w, h)
	want := image.NewRGBA(bounds)
	Clear(want, image.Rectangle{}, gfx.White)
	NewRenderer().Render(want, dl, Options{Scale: scale, Workers: 1})
	rng := rand.New(rand.NewPCG(275, uint64(scale*10)+uint64(workers)))
	var r Renderer
	var c *BackdropCache
	if cached {
		c = new(BackdropCache)
	}
	region := func() image.Rectangle {
		x, y := rng.IntN(w), rng.IntN(h)
		return image.Rect(x, y, x+1+rng.IntN(w/2), y+1+rng.IntN(h/2)).Intersect(bounds)
	}
	noise := func(img *image.RGBA) {
		for i := range img.Pix {
			img.Pix[i] = uint8(rng.IntN(256))
		}
	}
	tried := 0
	// Every command index in short mode is a few thousand renders under
	// the race detector: a sample of them, with both ends.
	step := 1
	if testing.Short() || raceOn {
		step = 5
	}
	for at := 0; at <= dl.Len(); at += step {
		if at+step > dl.Len() {
			at = dl.Len()
		}
		if layerOpenAt(dl, at) {
			continue
		}
		tried++
		// The snapshot of a full render or of a region render.
		fb := image.NewRGBA(bounds)
		copy(fb.Pix, want.Pix)
		snapRegion := bounds
		if at%2 == 1 {
			snapRegion = region()
			Clear(fb, snapRegion, gfx.White)
		} else {
			Clear(fb, image.Rectangle{}, gfx.White)
		}
		sn := &Snapshot{At: at}
		r.Render(fb, dl, Options{Scale: scale, Workers: workers, Region: snapRegion, Backdrops: c, Snap: sn})
		if !bytes.Equal(fb.Pix, want.Pix) {
			p, _ := firstDiffRGBA(fb, want)
			t.Fatalf("at %d: the render that took the snapshot differs at %v", at, p)
		}
		if sn.Rect.Empty() {
			t.Fatalf("at %d: no snapshot over %v", at, snapRegion)
		}
		if want := c.RegionFrom(dl, scale, snapRegion, bounds, at); sn.Rect != want {
			t.Fatalf("at %d: the snapshot holds %v, want %v", at, sn.Rect, want)
		}
		base := sn.Image()
		for range 3 {
			rg := region().Intersect(snapRegion)
			if rg.Empty() || !c.RegionFrom(dl, scale, rg, bounds, at).In(base.Rect) {
				continue
			}
			for _, fromDst := range []bool{false, true} {
				got := image.NewRGBA(bounds)
				noise(got)
				before := bytes.Clone(got.Pix)
				opts := Options{Scale: scale, Workers: workers, Region: rg, Backdrops: c, From: at, Base: base}
				if fromDst {
					opts.Base = nil
					copyRect(got, base, base.Rect)
					before = bytes.Clone(got.Pix)
				}
				r.Render(got, dl, opts)
				exp := &image.RGBA{Pix: before, Stride: got.Stride, Rect: got.Rect}
				copyRect(exp, want, rg)
				if p, ok := firstDiffRGBA(got, exp); !ok {
					t.Fatalf("at %d, region %v (snapshot %v, base in dst %v): pixel %v = %v, want %v",
						at, rg, sn.Rect, fromDst, p, pixel(got, p.X, p.Y), pixel(exp, p.X, p.Y))
				}
			}
		}
	}
	if tried < 4 {
		t.Fatalf("only %d command indices tried", tried)
	}
}

// TestRenderFromLayer: a render cannot start inside a layer (its buffer
// is not in the base), and a snapshot inside one takes no pixels.
func TestRenderFromLayer(t *testing.T) {
	dl := fromScene()
	at := -1
	for i := range dl.Len() {
		if layerOpenAt(dl, i) {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("no command inside a layer")
	}
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	sn := &Snapshot{At: at, Rect: image.Rect(0, 0, 1, 1)}
	NewRenderer().Render(img, dl, Options{Snap: sn})
	if !sn.Rect.Empty() || sn.Image() != nil {
		t.Errorf("a snapshot inside a layer holds %v", sn.Rect)
	}
	defer func() {
		if v := recover(); v == nil || !strings.Contains(fmt.Sprint(v), "inside a layer") {
			t.Errorf("From inside a layer: recovered %v, want a panic", v)
		}
	}()
	NewRenderer().Render(img, dl, Options{From: at})
}

// TestBackdropCacheRevive: the entries of filters whose input changed
// after it rested are found again when the same input comes back in a
// later list (a theme switched back), and are drawn as an uncached render
// draws them; an input that differs, or another canvas, finds nothing, and
// the entries of an input that did not rest (the stripe moved twice) are
// not kept.
func TestBackdropCacheRevive(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		for _, workers := range []int{1, 0} {
			t.Run(fmt.Sprintf("x%g_w%d", scale, workers), func(t *testing.T) {
				w, h := int(400*scale), int(300*scale)
				state := func(stripe float32) glassState {
					return glassState{stripe: stripe, box: [2]gfx.Point{gfx.Pt(60, 60), gfx.Pt(20, 20)}, pane2: gfx.Pt(170, 120), fade: 0.6}
				}
				var c BackdropCache
				var r Renderer
				fb := image.NewRGBA(image.Rect(0, 0, w, h))
				render := func(what string, dl *gfx.DisplayList, canvas gfx.Color, revived int) {
					t.Helper()
					none := make([]int, dl.NumBackdrops())
					for k := range none {
						none[k] = -1 // every input changed
					}
					c.Retain(dl, scale, none)
					before := c.revived
					c.Revive(canvas)
					if got := c.revived - before; got != revived {
						t.Errorf("%s: %d entries revived, want %d", what, got, revived)
					}
					Clear(fb, image.Rectangle{}, canvas)
					r.Render(fb, dl, Options{Scale: scale, Workers: workers, Backdrops: &c})
					want := image.NewRGBA(fb.Rect)
					Clear(want, image.Rectangle{}, canvas)
					NewRenderer().Render(want, dl, Options{Scale: scale, Workers: 1})
					if p, ok := firstDiffRGBA(fb, want); !ok {
						t.Fatalf("%s: pixel %v = %v, want %v", what, p, pixel(fb, p.X, p.Y), pixel(want, p.X, p.Y))
					}
				}
				// The first frame, then another input under the panes (the
				// stripe moved: every filter's input changed), then the first
				// input again in a new list: the three filters come back.
				// The first input rests for a frame: its entries are carried.
				Clear(fb, image.Rectangle{}, gfx.White)
				c.Revive(gfx.White)
				r.Render(fb, state(120).list(-1), Options{Scale: scale, Workers: workers, Backdrops: &c})
				rest := state(120).list(-1)
				c.Retain(rest, scale, []int{0, 1, 2})
				r.Render(fb, rest, Options{Scale: scale, Workers: workers, Backdrops: &c})
				render("moved", state(200).list(-1), gfx.White, 0)
				// Frames between the switches (a hover, a tooltip) do not age
				// the past entries.
				for range 2 * maxIdleFrames {
					c.EndFrame()
				}
				hits := c.hits
				render("back", state(120).list(-1), gfx.White, 3)
				if c.hits-hits < 2 {
					t.Errorf("%d filters drawn from revived entries, want at least 2", c.hits-hits)
				}
				// The same input over another canvas is not one seen before.
				render("canvas", state(120).list(-1), gfx.Black, 0)
				// A stripe that moved by a pixel is another input.
				render("pixel", state(121).list(-1), gfx.White, 0)
				// A window at rest keeps none.
				if len(c.past) == 0 {
					t.Fatal("no past entries before the trim")
				}
				c.Trim()
				if len(c.past) != 0 || c.SpareBytes() != 0 {
					t.Errorf("after Trim: %d past entries, %d spare bytes", len(c.past), c.SpareBytes())
				}
			})
		}
	}
}
