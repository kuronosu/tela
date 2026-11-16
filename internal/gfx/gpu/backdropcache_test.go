package gpu

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The cache of filtered backdrops, the covers and the rest of an Encoder,
// without a device: what a frame's passes are when a filter is drawn from
// the cache, computed into it or computed without it, and what the
// Encoder keeps (the pixels are compared on a device, textures_test.go's
// TestBackdropCachePixels).

// filterBatches counts the instances of the filter pipeline in a frame,
// and those that copy.
func filterBatches(f *Frame) (n, copies int) {
	for pi := range f.Passes {
		for _, b := range f.Passes[pi].Batches {
			if b.Pipeline == PipelineFilter {
				n += b.Count
				if head(f, b.First)[0] == FilterCopy {
					copies += b.Count
				}
			}
		}
	}
	return n, copies
}

// panes is a page of two glass panes over an opaque background, with a
// label in each, painted after its filter: what the tests change from
// frame to frame is under the panes (stripe) or over them (label).
func panes(stripe, label float32) *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 400, 300), slate)
		b.FillRect(gfx.RectXYWH(stripe, 0, 12, 300), rose)
		for i, x := range []float32{20, 210} {
			box := gfx.NewRRect(gfx.RectXYWH(x, 20, 170, 260), 16)
			b.BackdropFilter(box, gfx.Blur{StdDev: 8})
			b.FillRRect(box, gfx.White.WithAlpha(0.3))
			b.FillRect(gfx.RectXYWH(x+20+label*float32(i+1), 60, 40, 12), emerald)
		}
	})
}

// kept returns the IDs of the targets the frame lists that the Encoder
// keeps for a cache.
func keptTargets(e *Encoder) []int {
	var ids []int
	for _, t := range e.res.targets {
		if t.kept {
			ids = append(ids, t.id)
		}
	}
	return ids
}

func TestBackdropCache(t *testing.T) {
	size := image.Pt(400, 300)
	inside := image.Rect(60, 100, 120, 140) // in the first pane, away from its edges
	var e Encoder
	var bc BackdropCache
	white := gfx.White
	opts := func(region image.Rectangle) Options {
		return Options{Region: region, Clear: &white, Backdrops: &bc}
	}
	same := []int{0, 1}

	// A full frame computes both filters and leaves their outputs to the
	// cache: two targets leave the pool, each as large as its pane.
	a := panes(100, 0)
	f := e.Encode(a, size, opts(image.Rectangle{}))
	checkFrame(t, "full", f)
	if n, _ := filterBatches(f); n == 0 || e.glass != (BackdropStats{Stored: 2}) || len(keptTargets(&e)) != 2 {
		t.Fatalf("a full frame: %d filter instances, %+v, %d kept targets; want both filters stored", n, e.glass, len(keptTargets(&e)))
	}
	// The frames that draw from the entries need a device that drew this
	// one: it changes the resources, though it uploads nothing.
	if f.Res.Next == f.Res.Base {
		t.Errorf("a frame that stores backdrops leaves the resources at %+v", f.Res)
	}
	if got, want := bc.Bytes(), e.Memory().Backdrops; got != want || got < 2*4*170*260 {
		t.Errorf("the cache holds %d bytes, the Encoder counts %d", got, want)
	}
	// A region inside a pane without a cache: the region grows by the
	// blur's reach and is rendered apart, from the start of the list.
	plain := Encoder{}
	g := plain.Encode(a, size, Options{Region: inside, Clear: &white})
	if n, _ := filterBatches(g); n < 3 || g.Passes[0].Target == 0 {
		t.Fatalf("a region in a pane without a cache: %d filter instances, passes %q", n, passesOf(g))
	}

	// The next frame, whose label moved: the filters kept their input.
	// The region is one pass into the destination, which starts at the
	// pane: the backdrop from its entry, the pane, the label.
	b := panes(100, 3)
	bc.Retain(b, 1, same, nil)
	f = e.Encode(b, size, opts(inside))
	checkFrameKept(t, "hit", f, keptTargets(&e))
	if n, _ := filterBatches(f); n != 0 || len(f.Passes) != 1 || f.Passes[0].Target != 0 || e.glass != (BackdropStats{Hits: 1}) {
		t.Fatalf("a region in a pane whose backdrop is cached: %d filter instances, passes %q, %+v", n, passesOf(f), e.glass)
	}
	if e.culled != 2 || instances(f) != 2 {
		t.Errorf("a region in a cached pane: %d commands hidden and %d instances, want the 2 fills under the pane hidden and the backdrop and the pane drawn", e.culled, instances(f))
	}
	ids := keptTargets(&e)
	if src := f.Passes[0].Batches[0].Sources[0]; int(src) != ids[0] && int(src) != ids[1] {
		t.Errorf("the backdrop is painted from the target %d, the cache keeps %v", src, ids)
	}
	if grown := bc.Region(b, 1, inside, image.Rectangle{Max: size}); grown != inside {
		t.Errorf("the region of a cached pane grows to %v", grown)
	}
	// A full frame draws both from the cache, and nothing of what they
	// hide entirely... which is nothing here: the fills show between the
	// panes.
	f = e.Encode(b, size, opts(image.Rectangle{}))
	if n, _ := filterBatches(f); n != 0 || e.glass.Hits != 2 || e.culled != 0 {
		t.Errorf("a full frame with both backdrops cached: %d filter instances, %+v, %d hidden", n, e.glass, e.culled)
	}

	// A list the cache was not moved to empties it: its entries belong to
	// another list.
	c := panes(100, 6)
	f = e.Encode(c, size, opts(inside))
	if n, _ := filterBatches(f); n == 0 || e.glass.Hits != 0 || len(keptTargets(&e)) != 0 {
		t.Fatalf("a list the cache was not moved to: %d filter instances, %+v, %d kept targets", n, e.glass, len(keptTargets(&e)))
	}
	// Nothing of it was computed whole, so nothing is kept; the next frame
	// with the same input computes all of the filter it draws, once, and
	// the frames after it draw it from the cache.
	d := panes(100, 9)
	bc.Retain(d, 1, same, nil)
	if grown := bc.Region(d, 1, inside, image.Rectangle{Max: size}); !image.Rect(20, 20, 190, 280).In(grown) {
		t.Errorf("a filter whose input rested and has no entry: the region grows to %v, want the whole pane", grown)
	}
	f = e.Encode(d, size, opts(inside))
	checkFrameKept(t, "whole", f, nil)
	if e.glass != (BackdropStats{Stored: 1}) || len(keptTargets(&e)) != 1 {
		t.Fatalf("a filter whose input rested: %+v, %d kept targets, want it stored", e.glass, len(keptTargets(&e)))
	}
	if last := f.Passes[len(f.Passes)-1]; last.Target != 0 || last.Batches[0].Scissor != inside {
		t.Errorf("the region is not what reaches the destination: %+v", last)
	}
	d2 := panes(100, 12)
	bc.Retain(d2, 1, same, nil)
	f = e.Encode(d2, size, opts(inside))
	if n, _ := filterBatches(f); n != 0 || e.glass.Hits != 1 {
		t.Errorf("the frame after: %d filter instances, %+v", n, e.glass)
	}

	// The stripe moves under the first pane: its input changed, inside the
	// frame's damage. Its entry stays, and the render writes into it what
	// it computes: a copy into the entry's target.
	bc.Reset()
	e.Encode(d2, size, opts(image.Rectangle{}))
	ids = keptTargets(&e)
	moved := panes(104, 12)
	bc.Retain(moved, 1, []int{-1, 1}, []int{0, -1})
	stripe := image.Rect(100, 0, 116, 300)
	damage := stripe.Union(raster.BackdropOutputRegion(moved, 1, 0, stripe))
	f = e.Encode(moved, size, opts(damage))
	checkFrameKept(t, "patch", f, ids)
	if e.glass != (BackdropStats{Patched: 1}) {
		t.Fatalf("a filter whose input changed in the damage: %+v, want it patched", e.glass)
	}
	wrote := false
	for pi := range f.Passes {
		p := &f.Passes[pi]
		if p.Target == ids[0] || p.Target == ids[1] {
			bt := p.Batches[0]
			wrote = len(p.Batches) == 1 && bt.Pipeline == PipelineFilter && head(f, bt.First)[0] == FilterCopy && p.Clear == nil
			if !bt.Scissor.In(damage) {
				t.Errorf("the entry is written over %v, outside the damage %v", bt.Scissor, damage)
			}
		}
	}
	if !wrote || len(keptTargets(&e)) != 2 {
		t.Errorf("the entry was not written by a copy (passes %q), or was dropped (%d kept)", passesOf(f), len(keptTargets(&e)))
	}
	// The frame after it: both are drawn from the cache.
	after := panes(104, 15)
	bc.Retain(after, 1, same, nil)
	f = e.Encode(after, size, opts(image.Rectangle{}))
	if n, _ := filterBatches(f); n != 0 || e.glass.Hits != 2 {
		t.Errorf("after a patched frame: %d filter instances, %+v", n, e.glass)
	}
	// A filter whose input changed otherwise (neither kept nor patched)
	// loses its entry, and the other keeps its own.
	next := panes(104, 18)
	bc.Retain(next, 1, []int{0, -1}, nil)
	if len(keptTargets(&e)) != 1 {
		t.Errorf("after one filter's input changed: %d kept targets, want 1", len(keptTargets(&e)))
	}
	f = e.Encode(next, size, opts(image.Rectangle{}))
	checkFrameKept(t, "one entry", f, keptTargets(&e))
	if e.glass != (BackdropStats{Hits: 1, Stored: 1}) {
		t.Errorf("a full frame with one entry: %+v", e.glass)
	}

	// The frames end and nothing is drawn for a long while: the pool's
	// targets go, the cache's stay. So do they when the windows rest, and
	// the frame after the rest draws from them.
	for range 3 * idleFrames {
		e.EndFrame()
	}
	if m := e.Memory(); m.Targets != 0 || m.Backdrops != bc.Bytes() || bc.Bytes() == 0 {
		t.Errorf("after idle frames: %+v, the cache holds %d", m, bc.Bytes())
	}
	rest := e.Rest(next)
	if len(rest.Passes) != 0 || len(rest.Targets) != 2 {
		t.Errorf("the frame of a rest: %d passes, %d targets listed, want the cache's 2", len(rest.Passes), len(rest.Targets))
	}
	f = e.Encode(next, size, opts(inside))
	if n, _ := filterBatches(f); n != 0 || e.glass.Hits != 1 {
		t.Errorf("after a rest: %d filter instances, %+v", n, e.glass)
	}
	// An Encoder that starts over holds no target: the cache's entries
	// are gone with them, and made again.
	e.Trim()
	f = e.Encode(next, size, opts(image.Rectangle{}))
	if e.glass != (BackdropStats{Stored: 2}) || len(f.Targets) < 2 {
		t.Errorf("after the Encoder started over: %+v", e.glass)
	}
	// Reset gives the targets back to the pool, which lets them age.
	bc.Reset()
	if len(keptTargets(&e)) != 0 || bc.Bytes() != 0 {
		t.Errorf("after Reset: %d kept targets, %d bytes", len(keptTargets(&e)), bc.Bytes())
	}
	for range idleFrames + 2 {
		e.EndFrame()
	}
	if m := e.Memory(); m.Targets != 0 || m.Backdrops != 0 {
		t.Errorf("after Reset and idle frames: %+v", m)
	}
}

// TestBackdropCacheLimits checks what is never cached, and that the
// attempts stop: a filter drawn through a clip that cuts it, one inside a
// layer, one that only recolors, and the entries beyond the budget.
func TestBackdropCacheLimits(t *testing.T) {
	size := image.Pt(300, 200)
	white := gfx.White
	box := gfx.NewRRect(gfx.RectXYWH(20, 20, 200, 160), 10)
	scenes := []struct {
		name string
		draw func(b *gfx.Builder)
	}{
		{"a clip that cuts it", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 300, 200), slate)
			b.PushClipRect(gfx.RectXYWH(0, 0, 120, 200))
			b.BackdropFilter(box, gfx.Blur{StdDev: 6})
			b.PopClip()
		}},
		{"a layer", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 300, 200), slate)
			b.PushLayer(0.5, gfx.SrcOver)
			b.BackdropFilter(box, gfx.Blur{StdDev: 6})
			b.PopLayer()
		}},
		{"a color matrix alone", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 300, 200), slate)
			b.BackdropFilter(box, gfx.Saturate(1.5))
		}},
	}
	for _, sc := range scenes {
		var e Encoder
		var bc BackdropCache
		for i := range 4 {
			dl := gfxtest.Build(sc.draw)
			if i > 0 {
				bc.Retain(dl, 1, []int{0}, nil)
			}
			region := image.Rect(40, 40, 60, 60)
			grown := bc.Region(dl, 1, region, image.Rectangle{Max: size})
			e.Encode(dl, size, Options{Region: region, Clear: &white, Backdrops: &bc})
			if e.glass.Stored != 0 || e.glass.Hits != 0 || bc.Bytes() != 0 {
				t.Errorf("%s, frame %d: %+v", sc.name, i, e.glass)
			}
			// Once it could not be kept, the region is the one of a render
			// without a cache again.
			if want := raster.BackdropRegion(dl, 1, region, image.Rectangle{Max: size}); i >= 2 && grown != want {
				t.Errorf("%s, frame %d: the region grows to %v, without a cache %v", sc.name, i, grown, want)
			}
		}
	}

	// Three filters over the whole target: two fit the budget.
	var e Encoder
	var bc BackdropCache
	all := gfx.RRect{Rect: gfx.RectXYWH(0, 0, 300, 200)}
	draw := func(b *gfx.Builder) {
		b.FillRect(all.Rect, slate)
		for range 3 {
			b.BackdropFilter(all, gfx.Blur{StdDev: 4})
			b.FillRect(gfx.RectXYWH(10, 10, 50, 50), rose.WithAlpha(0.5))
		}
	}
	e.Encode(gfxtest.Build(draw), size, Options{Clear: &white, Backdrops: &bc})
	if e.glass.Stored != 2 || e.glass.Computed != 1 || bc.Bytes() > 2*e.res.targets[0].bytes()+1<<20 {
		t.Errorf("three filters of the size of the target: %+v, %d bytes", e.glass, bc.Bytes())
	}
	dl := gfxtest.Build(draw)
	bc.Retain(dl, 1, []int{0, 1, 2}, nil)
	// The second one's backdrop comes from the cache, opaque, over the
	// whole target: what is painted before it is hidden, the first filter
	// included. The third reads what the second painted, so that is drawn.
	f := e.Encode(dl, size, Options{Clear: &white, Backdrops: &bc})
	if e.glass != (BackdropStats{Hits: 1, Computed: 1}) || e.culled != 3 {
		t.Errorf("the frame after: %+v, %d commands hidden (%d instances)", e.glass, e.culled, instances(f))
	}
}

// TestCull checks what an opaque command hides and what keeps a command
// drawn: something that reads it in between, a cover that is not one, a
// cover that does not cover it.
func TestCull(t *testing.T) {
	size := image.Pt(200, 200)
	under := func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(40, 40, 60, 60), 8), emerald.WithAlpha(0.5))
	}
	whole := gfx.RectXYWH(0, 0, 200, 200)
	translucent := func() *gfx.Image {
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		img.Pix[4*20+3], img.Pix[4*20], img.Pix[4*20+1], img.Pix[4*20+2] = 128, 128, 128, 128
		return gfx.NewImage(img)
	}()
	opaque := func() *gfx.Image {
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		return gfx.NewImage(img)
	}()
	cases := []struct {
		name   string
		draw   func(b *gfx.Builder)
		hidden int // commands hidden
	}{
		{"an opaque rectangle", func(b *gfx.Builder) { under(b); b.FillRect(whole, slate) }, 2},
		{"a translucent rectangle", func(b *gfx.Builder) { under(b); b.FillRect(whole, slate.WithAlpha(0.99)) }, 0},
		{"a gradient", func(b *gfx.Builder) {
			under(b)
			b.FillRect(whole, gfx.LinearGradient{End: gfx.Pt(200, 0), Stops: []gfx.GradientStop{{Offset: 0, Color: slate}, {Offset: 1, Color: rose}}})
		}, 0},
		{"an opaque image", func(b *gfx.Builder) { under(b); b.DrawImage(opaque, gfx.Rect{}, whole, gfx.SampleLinear) }, 2},
		{"an image with transparent pixels", func(b *gfx.Builder) { under(b); b.DrawImage(translucent, gfx.Rect{}, whole, gfx.SampleLinear) }, 0},
		{"a rounded rectangle, over its middle", func(b *gfx.Builder) {
			under(b)
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 200, 200), 15), slate) // the band between its corners holds the fills
		}, 2},
		{"a rounded rectangle, over its middle and beyond one band", func(b *gfx.Builder) {
			under(b)
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 200, 200), 30), slate) // the first fill is in neither band whole
		}, 1},
		{"a rounded rectangle, over its corner", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 100, 100), rose) // reaches the corner, which is not covered
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 200, 200), 30), slate)
		}, 0},
		{"a cover that leaves a part out", func(b *gfx.Builder) { under(b); b.FillRect(gfx.RectXYWH(30, 0, 170, 200), slate) }, 1},
		{"a cover over the middle of a large fill", func(b *gfx.Builder) {
			b.FillRect(whole, rose)
			b.FillRect(gfx.RectXYWH(40, 50, 120, 100), slate)
		}, 0},
		{"a small cover", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(10, 10, 20, 20), rose)
			b.FillRect(gfx.RectXYWH(0, 0, 40, 40), slate)
		}, 0},
		{"a cover inside a rounded clip", func(b *gfx.Builder) {
			under(b)
			b.PushClipRRect(gfx.NewRRect(whole, 20))
			b.FillRect(whole, slate)
			b.PopClip()
		}, 0},
		{"a cover inside a pixel aligned clip", func(b *gfx.Builder) {
			under(b)
			b.PushClipRect(gfx.RectXYWH(30, 30, 100, 100))
			b.FillRect(whole, slate)
			b.PopClip()
		}, 1},
		{"a cover under a rotation", func(b *gfx.Builder) {
			under(b)
			b.PushTransform(gfx.RotateAbout(0.1, gfx.Pt(100, 100)))
			b.FillRect(gfx.RectXYWH(-100, -100, 400, 400), slate)
			b.PopTransform()
		}, 0},
		{"a cover in a layer", func(b *gfx.Builder) { under(b); b.PushLayer(0.5, gfx.SrcOver); b.FillRect(whole, slate); b.PopLayer() }, 0},
		{"a backdrop filter in between", func(b *gfx.Builder) {
			under(b)
			b.BackdropFilter(gfx.RRect{Rect: gfx.RectXYWH(0, 0, 50, 50)}, gfx.Blur{StdDev: 2}) // reads the fills
			b.FillRect(gfx.RectXYWH(100, 0, 100, 200), slate.WithAlpha(0.5))                   // so that the filter shows
			b.FillRect(gfx.RectXYWH(0, 60, 200, 140), slate)
		}, 0},
		{"a backdrop filter in between, which reads elsewhere", func(b *gfx.Builder) {
			under(b)
			b.BackdropFilter(gfx.RRect{Rect: gfx.RectXYWH(150, 0, 50, 15)}, gfx.Blur{StdDev: 2})
			b.FillRect(gfx.RectXYWH(0, 16, 200, 184), slate)
		}, 2},
		{"a backdrop filter under the cover", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 200, 10), rose)
			b.BackdropFilter(gfx.RRect{Rect: gfx.RectXYWH(60, 60, 50, 50)}, gfx.Blur{StdDev: 2}) // hidden itself
			b.FillRect(gfx.RectXYWH(0, 20, 200, 180), slate)
		}, 1},
		{"a blend mode in between", func(b *gfx.Builder) {
			under(b)
			b.PushLayer(1, gfx.Multiply)
			b.FillRect(gfx.RectXYWH(0, 0, 50, 50), rose)
			b.PopLayer()
			b.FillRect(whole, slate)
		}, 0},
		{"a layer, a shadow and text under a cover", func(b *gfx.Builder) {
			b.PushLayer(0.5, gfx.SrcOver)
			under(b)
			b.PopLayer()
			shadow(b, gfx.NewRRect(gfx.RectXYWH(50, 50, 60, 60), 6), 0, 2, 8, 0, gfx.Black, false)
			text(b, "SoAl", 14, 40, 90, gfx.White)
			b.FillRect(whole, slate)
		}, 3},
		{"a command after the cover", func(b *gfx.Builder) { b.FillRect(whole, slate); under(b) }, 0},
	}
	for _, c := range cases {
		dl := gfxtest.Build(c.draw)
		var e, ref Encoder
		f := e.Encode(dl, size, Options{})
		checkFrame(t, c.name, f)
		if e.culled != c.hidden {
			t.Errorf("%s: %d commands hidden, want %d (passes %q)", c.name, e.culled, c.hidden, passesOf(f))
		}
		// A region hides at least what the full frame hides: its instances
		// are instances of the full frame, in order. (Not with layers and
		// filters, whose targets are as large as the region needs.)
		if len(f.Passes) != 1 {
			continue
		}
		all := append([]byte(nil), f.Instances...)
		for _, region := range []image.Rectangle{image.Rect(30, 30, 90, 90), image.Rect(0, 0, 45, 45), image.Rect(100, 20, 200, 110)} {
			if g := ref.Encode(dl, size, Options{Region: region}); len(g.Passes) <= 1 {
				subsequence(t, c.name, g.Instances, all)
			}
		}
	}
	// A large fill under a cover is the strips around the cover, the same
	// in a full frame and in a region; and their pixels with the cover's
	// are the target's, once each.
	{
		var e Encoder
		dl := gfxtest.Build(func(b *gfx.Builder) {
			b.FillRect(whole, rose)
			b.FillRect(gfx.RectXYWH(40, 50, 120, 100), slate)
			b.FillRect(gfx.RectXYWH(150, 20, 50, 100), sky) // over the first cover's side
		})
		f := e.Encode(dl, size, Options{})
		seen := make([]int, 200*200)
		under := 0
		for i := range instances(f) {
			bd := floats(f, i, SlotBounds)
			r := image.Rect(int(bd[0]), int(bd[1]), int(bd[2]), int(bd[3]))
			if head(f, i)[2] == packRGBA8(premul8(rose)) {
				under++
				if r.Overlaps(image.Rect(40, 50, 160, 150)) || r.Overlaps(image.Rect(150, 20, 200, 120)) {
					t.Errorf("a part of the fill under the covers, %v, lies under one", r)
				}
			}
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					seen[y*200+x]++
				}
			}
		}
		for k, n := range seen {
			in := image.Pt(k%200, k/200).In(image.Rect(150, 50, 160, 120)) // where the two covers overlap
			if n != 1 && !(in && n == 2) {
				t.Fatalf("the pixel (%d, %d) is drawn %d times", k%200, k/200, n)
			}
		}
		if under < 4 || under > maxCuts {
			t.Errorf("the fill under two covers is %d parts", under)
		}
		all := append([]byte(nil), f.Instances...)
		for _, region := range []image.Rectangle{image.Rect(0, 0, 60, 60), image.Rect(50, 60, 150, 140), image.Rect(140, 0, 200, 200)} {
			var ref Encoder
			g := ref.Encode(dl, size, Options{Region: region})
			subsequence(t, "a fill under two covers", g.Instances, all)
			if region == image.Rect(50, 60, 150, 140) {
				// Inside the first cover: its parts alone.
				for i := range instances(g) {
					if head(g, i)[2] != packRGBA8(premul8(slate)) {
						t.Errorf("a region inside a cover: instance %d is not the cover's", i)
					}
				}
			}
		}
	}
	// A region inside a cover hides what the full frame shows around it.
	var e Encoder
	dl := gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(whole, rose)
		b.FillRect(gfx.RectXYWH(50, 50, 100, 100), slate)
	})
	if e.Encode(dl, size, Options{}); e.culled != 0 {
		t.Errorf("a background around a cover: %d hidden in a full frame", e.culled)
	}
	if f := e.Encode(dl, size, Options{Region: image.Rect(60, 60, 140, 140)}); e.culled != 1 || instances(f) != 1 {
		t.Errorf("a region inside the cover: %d hidden, %d instances", e.culled, instances(f))
	}
}

// TestRest checks what an Encoder keeps for windows that rest: the atlases
// and the images on screen, so that the frame after the rest uploads
// nothing; and what it gives up: the pool's targets and the images that no
// window shows.
func TestRest(t *testing.T) {
	size := image.Pt(640, 400)
	other := gfx.NewImage(image.NewRGBA(image.Rect(0, 0, 16, 16)))
	page := gfxtest.Build(func(b *gfx.Builder) { glassDesktop(b, 640, 400) })
	both := gfxtest.Build(func(b *gfx.Builder) {
		glassDesktop(b, 640, 400)
		b.DrawImage(other, gfx.Rect{}, gfx.RectXYWH(600, 360, 32, 32), gfx.SampleLinear)
	})
	var e Encoder
	if f := e.Rest(page); f != nil {
		t.Errorf("an Encoder that holds nothing rests with a frame: %+v", f)
	}
	f := e.Encode(both, size, Options{})
	if len(f.Images) != 2 || uploads(f) == 0 || len(f.Targets) == 0 {
		t.Fatalf("the first frame: %d images, %d uploads, %d targets", len(f.Images), uploads(f), len(f.Targets))
	}
	e.EndFrame()
	before, first := e.Memory(), f.Res
	// The windows show page: the image only both draws goes, with the
	// targets of the layers and the filters.
	rest := e.Rest(page)
	if len(rest.Passes) != 0 || len(rest.Targets) != 0 || len(rest.Releases) != 1 || rest.Releases[0] != other.ID() || len(rest.Images) != 0 {
		t.Fatalf("the frame of a rest: %d passes, %d targets, releases %v, want the other image's ID %d", len(rest.Passes), len(rest.Targets), rest.Releases, other.ID())
	}
	if rest.Res.Base != first.Next || rest.Res.Next != rest.Res.Base+1 || rest.Res.ID != first.ID {
		t.Errorf("the frame of a rest follows %+v with %+v", first, rest.Res)
	}
	restNext := rest.Res.Next
	after := e.Memory()
	if after.Targets != 0 || after.Atlases != before.Atlases || after.Atlases == 0 || after.Images >= before.Images || after.Images == 0 {
		t.Errorf("a rest: from %+v to %+v, want the atlases kept, the targets gone and one image less", before, after)
	}
	// The frame after it uploads nothing: every glyph, path and image of
	// the page is where it was.
	f = e.Encode(page, size, Options{})
	checkFrame(t, "after a rest", f)
	if uploads(f) != 0 || len(f.Images) != 0 || len(f.Data) != 0 || f.Res.Base != restNext {
		t.Errorf("the frame after a rest: %d uploads, %d images, %d bytes", uploads(f), len(f.Images), len(f.Data))
	}
	// The image that was given up comes back with the frame that draws it.
	e.EndFrame()
	if f = e.Encode(both, size, Options{}); len(f.Images) != 1 || f.Images[0].ID != other.ID() {
		t.Errorf("the image a rest gave up: %d images uploaded by the frame that draws it again", len(f.Images))
	}
	// A rest with no list keeps no image.
	e.EndFrame()
	if rest = e.Rest(); len(rest.Releases) != 2 || e.Memory().Images != 0 {
		t.Errorf("a rest without lists: releases %v, %d bytes of images", rest.Releases, e.Memory().Images)
	}
}

// TestImagesInUse checks that the images of the frame's list keep their
// textures while no frame draws them: hidden under an opaque command, or
// outside the regions rendered, for longer than an unused image is kept.
// And that an image no list holds any more goes.
func TestImagesInUse(t *testing.T) {
	size := image.Pt(400, 300)
	img := func() *gfx.Image {
		m := image.NewRGBA(image.Rect(0, 0, 40, 30))
		for i := range m.Pix {
			m.Pix[i] = 255
		}
		return gfx.NewImage(m)
	}
	back, corner := img(), img()
	page := func(with bool, label float32) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) {
			b.DrawImage(back, gfx.Rect{}, gfx.RectXYWH(0, 0, 400, 300), gfx.SampleLinear)
			b.FillRect(gfx.RectXYWH(20, 20, 300, 200), slate) // hides the picture where the frames draw
			b.FillRect(gfx.RectXYWH(40+label, 40, 30, 10), rose)
			if with {
				b.DrawImage(corner, gfx.Rect{}, gfx.RectXYWH(360, 260, 40, 30), gfx.SampleLinear)
			}
		})
	}
	var e Encoder
	if f := e.Encode(page(true, 0), size, Options{}); len(f.Images) != 2 {
		t.Fatalf("the first frame uploads %d images", len(f.Images))
	}
	e.EndFrame()
	for i := range 3 * idleFrames {
		f := e.Encode(page(true, float32(i%7)), size, Options{Region: image.Rect(40, 40, 120, 50)})
		e.EndFrame()
		if len(f.Images) != 0 || len(f.Releases) != 0 || instances(f) > 2 {
			t.Fatalf("frame %d, inside the cover: %d images uploaded, %d released, %d instances", i, len(f.Images), len(f.Releases), instances(f))
		}
	}
	if f := e.Encode(page(true, 0), size, Options{}); len(f.Images) != 0 {
		t.Errorf("a full frame after %d frames that drew no image uploads %d", 3*idleFrames, len(f.Images))
	}
	// The corner's picture leaves the page: its texture goes after a while,
	// the other one stays.
	released := 0
	for i := range 3 * idleFrames {
		f := e.Encode(page(false, float32(i%7)), size, Options{Region: image.Rect(40, 40, 120, 50)})
		e.EndFrame()
		released += len(f.Releases)
		if len(f.Images) != 0 {
			t.Fatalf("frame %d uploads an image", i)
		}
	}
	if m := e.Memory(); released != 1 || m.Images != int64(len(back.Pix())) {
		t.Errorf("an image that left the page: %d released, %d bytes of images held, want 1 and %d", released, m.Images, len(back.Pix()))
	}
}

// TestSteadyFrame checks that a frame that draws what the frame before it
// drew costs the Encoder no allocation and uploads nothing, full and in a
// region, with the cache of filtered backdrops and without.
func TestSteadyFrame(t *testing.T) {
	size := image.Pt(1280, 800)
	dl := gfxtest.Build(func(b *gfx.Builder) { glassDesktop(b, 640, 400) })
	white := gfx.White
	for _, cached := range []bool{false, true} {
		var e Encoder
		var bc BackdropCache
		opts := Options{Scale: 2, Clear: &white}
		if cached {
			opts.Backdrops = &bc
		}
		for _, region := range []image.Rectangle{{}, image.Rect(100, 150, 260, 230)} {
			opts.Region = region
			for range 3 {
				e.Encode(dl, size, opts)
				e.EndFrame()
			}
			var f *Frame
			allocs := testing.AllocsPerRun(20, func() {
				f = e.Encode(dl, size, opts)
				e.EndFrame()
			})
			if allocs != 0 || uploads(f) != 0 || len(f.Images) != 0 || len(f.Data) != 0 {
				t.Errorf("a steady frame (cache %v, region %v): %.0f allocations, %d uploads, %d images, %d bytes", cached, region, allocs, uploads(f), len(f.Images), len(f.Data))
			}
		}
	}
}
