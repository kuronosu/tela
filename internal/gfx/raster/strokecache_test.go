package raster

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// strokeScene strokes dashed and solid paths: a dashed border around a
// rounded rectangle, an icon-like polyline with round joins, and a dashed
// circle under a rotation, so that bands cross every stroke.
func strokeScene(w, h float32) *gfx.DisplayList {
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: w, Y1: h}, gfx.White)
	var border gfx.Path
	border.AddRRect(gfx.NewRRect(gfx.RectXYWH(10.5, 10.5, w-21, h-21), 12))
	dashed := gfx.NewStrokeStyle(2)
	dashed.Dashes = []float32{6, 3}
	b.StrokePath(border, dashed, slate)
	var icon gfx.Path
	icon.MoveTo(gfx.Pt(30, h-30))
	icon.LineTo(gfx.Pt(w/3, 40))
	icon.LineTo(gfx.Pt(w/2, h-50))
	icon.CubicTo(gfx.Pt(w*2/3, 20), gfx.Pt(w*3/4, h), gfx.Pt(w-30, 40))
	round := gfx.NewStrokeStyle(3)
	round.Join, round.Cap = gfx.JoinRound, gfx.CapRound
	b.StrokePath(icon, round, rose)
	b.PushTransform(gfx.RotateAbout(0.4, gfx.Pt(w/2, h/2)))
	dotted := gfx.NewStrokeStyle(4)
	dotted.Dashes, dotted.Cap, dotted.DashOffset = []float32{0.1, 7}, gfx.CapRound, 2
	b.StrokePath(gfx.CirclePath(gfx.Pt(w/2, h/2), h/3), dotted, indigo)
	b.PopTransform()
	return b.Finish()
}

// TestStrokeCacheAllocs: a Renderer that renders a list of strokes again
// (the next frame, or another band of a parallel render) reuses the
// outlines it computed instead of flattening, dashing and offsetting the
// paths again: the second render does not allocate.
func TestStrokeCacheAllocs(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 120))
	dl := strokeScene(200, 120)
	r := NewRenderer()
	r.Render(img, dl, Options{Workers: 1})
	if n := testing.AllocsPerRun(10, func() { r.Render(img, dl, Options{Workers: 1}) }); n > 0 {
		t.Errorf("%v allocations per Render of strokes already rendered", n)
	}
	// A list recorded again holds copies of the paths: equal content is
	// a hit too.
	again := strokeScene(200, 120)
	if n := testing.AllocsPerRun(10, func() { r.Render(img, again, Options{Workers: 1}) }); n > 0 {
		t.Errorf("%v allocations per Render of the same strokes recorded again", n)
	}
}

// TestStrokeCachePixels: renders that hit the cache (the same list again,
// at another scale, in parallel bands, after EndFrame and Trim) give the
// pixels of a Renderer that computes every outline.
func TestStrokeCachePixels(t *testing.T) {
	const w, h = 300, 180
	dl := strokeScene(w, h)
	for _, scale := range []float32{1, 1.5, 2} {
		for _, workers := range []int{1, 4} {
			t.Run(fmt.Sprintf("x%g/w%d", scale, workers), func(t *testing.T) {
				rect := image.Rect(0, 0, int(w*scale), int(h*scale))
				want := image.NewRGBA(rect)
				fresh := NewRenderer()
				fresh.strk = nil
				fresh.render(want, dl, gfx.Scale(scale, scale), rect)

				var r Renderer
				r.tune.minPixels, r.tune.bandRows = 1, 16
				for i := range 4 {
					got := image.NewRGBA(rect)
					r.Render(got, dl, Options{Scale: scale, Workers: workers})
					if !bytes.Equal(got.Pix, want.Pix) {
						t.Fatalf("render %d differs from outlines computed anew", i)
					}
					switch i {
					case 1:
						r.EndFrame()
					case 2:
						r.Trim()
						if n := r.strokes.size(); n != 0 {
							t.Errorf("after Trim the cache keeps %d bytes", n)
						}
					}
				}
			})
		}
	}
}

// TestStrokeCacheKey: outlines are told apart by every input: the path's
// points, the width, the dashes and their offset, and the tolerance.
func TestStrokeCacheKey(t *testing.T) {
	var c strokeCache
	var p gfx.Path
	p.MoveTo(gfx.Pt(0, 0))
	p.LineTo(gfx.Pt(40, 0))
	p.LineTo(gfx.Pt(40, 30))
	st := gfx.NewStrokeStyle(2)
	st.Dashes = []float32{5, 3}
	base := c.stroke(p, st, 0.25)
	if again := c.stroke(p.Clone(), st, 0.25); !samePath(again, base) {
		t.Fatal("equal inputs gave another outline")
	}
	q := p.Clone()
	q.LineTo(gfx.Pt(0, 30))
	wider := st
	wider.Width = 3
	shifted := st
	shifted.DashOffset = 1
	other := st
	other.Dashes = []float32{5, 4}
	for name, out := range map[string]gfx.Path{
		"path":      c.stroke(q, st, 0.25),
		"width":     c.stroke(p, wider, 0.25),
		"offset":    c.stroke(p, shifted, 0.25),
		"dashes":    c.stroke(p, other, 0.25),
		"tolerance": c.stroke(p, st, 0.1),
	} {
		want := map[string]gfx.Path{
			"path":      q.Stroke(st, 0.25),
			"width":     p.Stroke(wider, 0.25),
			"offset":    p.Stroke(shifted, 0.25),
			"dashes":    p.Stroke(other, 0.25),
			"tolerance": p.Stroke(st, 0.1),
		}[name]
		if !samePath(out, want) {
			t.Errorf("another %s: the cache gave a wrong outline", name)
		}
	}
	if len(c.entries) != 6 {
		t.Errorf("%d entries, want 6", len(c.entries))
	}
	// Aging: entries no frame uses go after maxIdleFrames frames.
	for range maxIdleFrames + 2 {
		c.endFrame(false)
	}
	if len(c.entries) != 0 || c.bytes != 0 {
		t.Errorf("after %d idle frames: %d entries, %d bytes", maxIdleFrames+2, len(c.entries), c.bytes)
	}
}
