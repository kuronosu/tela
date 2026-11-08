package raster

import (
	"bytes"
	"fmt"
	"image"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// paletteScene is a frame with what a command palette over a page adds: a
// full-window backdrop blur, a faded layer with a shadow, a rounded clip
// and a blurred filter layer, over stripes and a gradient (w x h logical
// pixels).
func paletteScene(w, h float32) *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: w, Y1: h}, gfx.LinearGradient{End: gfx.Pt(w, h), Stops: stops(indigo, rose, amber)})
	for x := float32(0); x < w; x += 22 {
		b.FillRect(gfx.RectXYWH(x, 0, 8, h), gfx.White.WithAlpha(0.6))
	}
	b.BackdropFilter(gfx.NewRRect(gfx.Rect{X1: w, Y1: h}, 0), gfx.Blur{StdDev: 8})
	b.FillRect(gfx.Rect{X1: w, Y1: h}, gfx.Black.WithAlpha(0.3))
	panel := gfx.NewRRect(gfx.RectXYWH(w/5, h/6, w*3/5, h/2), 14)
	b.PushLayer(0.9, gfx.SrcOver)
	b.DrawShadow(gfx.DrawShadow{RRect: panel, Offset: gfx.Pt(0, 8), Blur: 24, Color: gfx.Black.WithAlpha(0.4)})
	b.FillRRect(panel, gfx.White)
	b.PushClipRRect(panel)
	for y := panel.Rect.Y0 + 10; y < panel.Rect.Y1; y += 18 {
		b.FillRect(gfx.RectXYWH(panel.Rect.X0, y, panel.Rect.X1-panel.Rect.X0, 9), sky.WithAlpha(0.3))
	}
	b.PopClip()
	b.PopLayer()
	b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: 3})
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(w/3, h*3/4, w/3, h/10), 8), slate)
	b.PopLayer()
	return b.Finish()
}

// plainScene is a frame that needs no scratch buffer: opaque rectangles.
func plainScene(w, h float32) *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: w, Y1: h}, gfx.White)
	b.FillRect(gfx.RectXYWH(10, 10, w/3, h/3), slate)
	return b.Finish()
}

// TestEndFrameAges checks that a Renderer keeps the buffers frames use and
// releases those no frame used for more than maxIdleFrames frames.
func TestEndFrameAges(t *testing.T) {
	dl := paletteScene(300, 200)
	fb := image.NewRGBA(image.Rect(0, 0, 300, 200))
	var r Renderer
	// The first frames reach the steady state (the second one may add a
	// buffer that the order of the first one's requests did not need, and
	// the first one's surplus goes after maxIdleFrames frames).
	for range 2 * maxIdleFrames {
		r.Render(fb, dl, Options{Workers: 1})
		r.EndFrame()
	}
	used := r.ScratchBytes()
	if used == 0 {
		t.Fatal("the scene kept no scratch memory")
	}
	// Frames that use it keep it, without growing.
	for range 3 * maxIdleFrames {
		r.Render(fb, dl, Options{Workers: 1})
		r.EndFrame()
	}
	if got := r.ScratchBytes(); got != used {
		t.Errorf("after frames of the same scene: %d bytes of scratch, want %d", got, used)
	}
	// Frames that do not use it keep it for maxIdleFrames frames (a buffer
	// that frames of the scene used every other frame, because two of them
	// take turns, goes one frame earlier).
	for i := range maxIdleFrames {
		r.Render(fb, plainScene(300, 200), Options{Workers: 1})
		r.EndFrame()
		if got := r.ScratchBytes(); (got != used && i < maxIdleFrames-1) || got == 0 {
			t.Fatalf("after %d frames without the scene: %d bytes of scratch, want %d", i+1, got, used)
		}
	}
	r.Render(fb, plainScene(300, 200), Options{Workers: 1})
	r.EndFrame()
	if got := r.ScratchBytes(); got != 0 {
		t.Errorf("after %d frames without the scene: %d bytes of scratch, want 0", maxIdleFrames+1, got)
	}
}

// TestTrim checks that Trim releases all the scratch memory, in the worker
// Renderers too, and that EndFrame alone keeps what the frame used.
func TestTrim(t *testing.T) {
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("w%d", workers), func(t *testing.T) {
			fb := image.NewRGBA(image.Rect(0, 0, 600, 400))
			var r Renderer
			r.tune.minPixels = 1
			r.Render(fb, paletteScene(600, 400), Options{Workers: workers})
			if workers > 1 && (r.par == nil || len(r.par.workers) == 0) {
				t.Fatal("not rendered in parallel")
			}
			r.EndFrame()
			used := r.ScratchBytes()
			if used == 0 {
				t.Fatal("EndFrame released what the frame used")
			}
			r.Trim()
			if got := r.ScratchBytes(); got != 0 {
				t.Errorf("after Trim: %d bytes of scratch (of %d)", got, used)
			}
			// The next frame allocates again what it needs.
			r.Render(fb, paletteScene(600, 400), Options{Workers: workers})
			if r.ScratchBytes() == 0 {
				t.Error("a frame after Trim kept no scratch memory")
			}
		})
	}
}

// TestEndFramePixels renders a sequence of frames (full and regions, with
// backdrop filters, layers, shadows and filter layers), ending frames
// between them, idle or not, with a Renderer whose buffers come and go: each
// frame equals the render of a new Renderer.
func TestEndFramePixels(t *testing.T) {
	scenes := []*gfx.DisplayList{paletteScene(300, 240), build(multiBackdrop), build(fadedBackdrop), plainScene(300, 240), paletteScene(240, 300)}
	bounds := image.Rect(0, 0, 300, 240)
	bg := image.NewRGBA(bounds)
	background(bg)
	for _, workers := range []int{1, 0} {
		t.Run(fmt.Sprintf("w%d", workers), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(241, uint64(workers)))
			var r Renderer
			r.tune.minPixels, r.tune.bandRows = 1, 9
			for step := range 60 {
				dl := scenes[rng.IntN(len(scenes))]
				var region image.Rectangle
				if rng.IntN(2) == 0 {
					x, y := rng.IntN(300), rng.IntN(240)
					region = image.Rect(x, y, x+1+rng.IntN(150), y+1+rng.IntN(120))
				}
				want := cloneRGBA(bg)
				NewRenderer().Render(want, dl, Options{Workers: 1, Region: region})
				got := cloneRGBA(bg)
				r.Render(got, dl, Options{Workers: workers, Region: region})
				if !bytes.Equal(got.Pix, want.Pix) {
					t.Fatalf("step %d (region %v): the frame differs from a new Renderer's", step, region)
				}
				for range rng.IntN(3) {
					r.EndFrame()
				}
				if rng.IntN(6) == 0 {
					r.Trim()
				}
			}
		})
	}
}

// TestPhasedOneWindow checks that a phased render with several barrier
// filters of the same size holds one filtered window at a time: the
// buffer of a barrier serves the next one.
func TestPhasedOneWindow(t *testing.T) {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 400, Y1: 300}, gfx.LinearGradient{End: gfx.Pt(400, 300), Stops: stops(indigo, rose, amber)})
	for k := range 3 {
		shape := gfx.NewRRect(gfx.RectXYWH(20+float32(k)*10, 20+float32(k)*10, 300, 200), 12)
		b.BackdropFilter(shape, gfx.Blur{StdDev: 6})
		b.FillRRect(shape, gfx.White.WithAlpha(0.2))
	}
	dl := b.Finish()
	fb := image.NewRGBA(image.Rect(0, 0, 400, 300))
	var r Renderer
	r.tune.minPixels = 1
	r.Render(fb, dl, Options{Workers: 4})
	if r.par == nil || r.par.comp == nil {
		t.Fatal("not rendered in phases")
	}
	if n := len(r.par.comp.free); n != 1 {
		t.Errorf("the phased render kept %d filtered windows, want 1", n)
	}
}

// TestBackdropCacheSpares checks that the buffer of an entry whose filter
// left the list is a spare that EndFrame frees after a few frames, and
// Trim at once.
func TestBackdropCacheSpares(t *testing.T) {
	ground := func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 200, Y1: 100}, gfx.LinearGradient{End: gfx.Pt(200, 100), Stops: stops(indigo, rose, amber)})
		for x := float32(0); x < 200; x += 12 {
			b.FillRect(gfx.RectXYWH(x, 0, 5, 100), gfx.White.WithAlpha(0.6))
		}
	}
	two := build(func(b *gfx.Builder) {
		ground(b)
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(10, 10, 80, 60), 8), gfx.Blur{StdDev: 4})
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(100, 20, 90, 70), 8), gfx.Blur{StdDev: 6})
	})
	one := build(func(b *gfx.Builder) {
		ground(b)
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(10, 10, 80, 60), 8), gfx.Blur{StdDev: 4})
	})
	for _, trim := range []bool{false, true} {
		t.Run(fmt.Sprintf("trim=%v", trim), func(t *testing.T) {
			fb := image.NewRGBA(image.Rect(0, 0, 200, 100))
			var c BackdropCache
			var r Renderer
			r.Render(fb, two, Options{Workers: 1, Backdrops: &c})
			both := c.Bytes()
			first := cap(c.ents[0].pix)
			if !c.ents[0].ok || !c.ents[1].ok || both != first+cap(c.ents[1].pix) {
				t.Fatalf("the entries were not stored (%d bytes)", both)
			}
			// The second filter leaves the list: its entry is dropped.
			c.Retain(one, 1, []int{0})
			r.Render(fb, one, Options{Workers: 1, Backdrops: &c, Region: image.Rect(0, 0, 1, 1)})
			if c.Bytes() != both || len(c.spare) != 1 {
				t.Fatalf("the dropped entry's buffer is not a spare: %d bytes, %d spares", c.Bytes(), len(c.spare))
			}
			// The frame that dropped the entry ends, then the spare waits
			// maxIdleFrames frames for an entry to take it.
			frames := 0
			if trim {
				c.Trim()
			} else {
				frames = maxIdleFrames + 2
			}
			for i := range frames {
				if len(c.spare) != 1 {
					t.Fatalf("the spare was freed after %d frames", i)
				}
				c.EndFrame()
			}
			if c.Bytes() != first || len(c.spare) != 0 {
				t.Errorf("after %d frames: %d bytes, %d spares; want %d bytes, the entry of the remaining filter", frames, c.Bytes(), len(c.spare), first)
			}
			if !c.ents[0].ok {
				t.Error("the entry of the remaining filter was dropped")
			}
		})
	}
}

// TestPhasedMaskBarrier renders, in phases, a masked layer before a
// backdrop filter whose command index is smaller than the length of the
// mask's list: the mask's commands must not pass the main list's barriers
// (a band that did would skip a mask command and compute the filter before
// the bands painted what it reads). The pixels are the sequential ones.
func TestPhasedMaskBarrier(t *testing.T) {
	mb := gfx.NewBuilder()
	for k := range 40 {
		mb.FillRect(gfx.RectXYWH(float32(k)*6, 0, 4, 200), gfx.White.WithAlpha(float32(k%5+1)/5))
	}
	mask := mb.Finish()
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: 300, Y1: 200}, gfx.LinearGradient{End: gfx.Pt(300, 200), Stops: stops(indigo, rose, amber)})
	b.PushMaskLayer(gfx.Mask{List: mask, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(20, 10, 260, 180), slate)
	b.PopLayer()
	shape := gfx.NewRRect(gfx.RectXYWH(40, 30, 200, 140), 12)
	b.BackdropFilter(shape, gfx.Blur{StdDev: 6})
	b.FillRRect(shape, gfx.White.WithAlpha(0.2))
	dl := b.Finish()
	bfs, ok := phaseBarriers(dl)
	if !ok || len(bfs) != 1 || bfs[0] >= mask.Len() {
		t.Fatalf("barriers %v (phased %v): want one, at an index below the mask's %d commands", bfs, ok, mask.Len())
	}
	bounds := image.Rect(0, 0, 300, 200)
	bg := image.NewRGBA(bounds)
	background(bg)
	want := cloneRGBA(bg)
	NewRenderer().Render(want, dl, Options{Workers: 1})
	for _, workers := range []int{2, 4} {
		r := NewRenderer()
		r.tune.minPixels = 1
		got := cloneRGBA(bg)
		r.Render(got, dl, Options{Workers: workers})
		if r.par == nil || r.par.comp == nil {
			t.Fatalf("%d workers: not rendered in phases", workers)
		}
		if p, same := firstDiffRGBA(got, want); !same {
			t.Fatalf("%d workers: the phased render differs from the sequential one at %v: %v, want %v", workers, p, pixel(got, p.X, p.Y), pixel(want, p.X, p.Y))
		}
	}
}
