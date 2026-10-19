package raster

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// Composition (gfx.Builder.AppendList) must not change pixels: a list
// appended under a transform renders exactly like the same commands
// recorded there, and its bounds stay conservative, so regions of the
// composed list render like a full render.

func TestAppendListMatchesRecording(t *testing.T) {
	all := append(append(append(append([]scene(nil), scenes...), scenes2...), scenes3[1:]...), textScenes...)
	for seed := uint64(1); seed <= 3; seed++ {
		all = append(all, randomScene(seed, 150, 110))
	}
	transforms := []gfx.Affine{
		gfx.Translate(3.5, -2),
		gfx.Translate(10, 20).Mul(gfx.Scale(0.75, 0.75)),
		gfx.Translate(40, -10).Mul(gfx.Rotate(0.2)),
	}
	for _, s := range all {
		for k, m := range transforms {
			t.Run(fmt.Sprintf("%s_t%d", s.name, k), func(t *testing.T) {
				direct := build(func(b *gfx.Builder) {
					b.PushTransform(m)
					s.draw(b)
					b.PopTransform()
				})
				inner := build(s.draw)
				composed := build(func(b *gfx.Builder) {
					b.PushTransform(m)
					b.AppendList(inner, nil)
					b.PopTransform()
				})
				for _, scale := range []float32{1, 2} {
					w := int(math.Ceil(float64(float32(s.w) * scale)))
					h := int(math.Ceil(float64(float32(s.h) * scale)))
					bg := image.NewRGBA(image.Rect(0, 0, w, h))
					background(bg)
					want, got := cloneRGBA(bg), cloneRGBA(bg)
					Render(want, direct, Options{Scale: scale})
					Render(got, composed, Options{Scale: scale})
					checkRegion(t, got, want, bg, bg.Rect)
					rng := rand.New(rand.NewPCG(uint64(w), uint64(k)))
					for range 8 {
						x0, y0 := rng.IntN(w), rng.IntN(h)
						region := image.Rect(x0, y0, x0+1+rng.IntN(w/2), y0+1+rng.IntN(h/2))
						reg := cloneRGBA(bg)
						Render(reg, composed, Options{Scale: scale, Region: region})
						checkRegion(t, reg, want, bg, region)
					}
				}
			})
		}
	}
}

// TestAppendListMarkers composes a list whose marker, inside a clip and a
// transform, stands for another list appended under a translation, and
// checks the tags and sources of the result.
func TestAppendListMarkers(t *testing.T) {
	child := build(func(b *gfx.Builder) {
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 60, 40), 8), gfx.MustParseHex("#22c55e"))
		b.DrawShadow(gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(10, 10, 30, 20), 4), Blur: 6, Color: gfx.Color{A: 0.5}})
	})
	draw := func(b *gfx.Builder, marker bool) {
		b.FillRect(gfx.Rect{X1: 200, Y1: 120}, gfx.MustParseHex("#e2e8f0"))
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(20, 10, 150, 100), 12))
		b.PushTransform(gfx.Rotate(0.1))
		if marker {
			b.Marker(7)
		} else {
			b.PushTransform(gfx.Translate(30, 25))
			b.AppendList(child, nil)
			b.PopTransform()
		}
		b.PopTransform()
		b.PopClip()
		b.FillRect(gfx.RectXYWH(150, 80, 40, 30), gfx.Color{R: 1, A: 0.5})
	}
	direct := build(func(b *gfx.Builder) { draw(b, false) })
	parent := build(func(b *gfx.Builder) { draw(b, true) })
	calls := 0
	composed := build(func(b *gfx.Builder) {
		b.SetTag(1)
		b.AppendList(parent, func(m gfx.Marker) {
			calls++
			if m.ID != 7 {
				t.Errorf("marker %d, want 7", m.ID)
			}
			old := b.SetTag(2)
			b.PushTransform(gfx.Translate(30, 25))
			b.AppendList(child, nil)
			b.PopTransform()
			b.SetTag(old)
		})
	})
	if calls != 1 {
		t.Fatalf("%d marker calls, want 1", calls)
	}
	for i := range composed.Len() {
		c, tag, src := composed.At(i), composed.Tag(i), composed.Source(i)
		switch {
		case tag == 2 && src >= 0 && !gfx.Equal(c, child.At(src)):
			t.Errorf("command %d (%s) is not source %d of the child", i, c.Name(), src)
		case tag == 1 && src >= 0 && !gfx.Equal(c, parent.At(src)):
			t.Errorf("command %d (%s) is not source %d of the parent", i, c.Name(), src)
		case tag != 1 && tag != 2:
			t.Errorf("command %d has tag %d", i, tag)
		}
		if _, ok := c.(gfx.Marker); ok {
			t.Errorf("marker left in the composed list at %d", i)
		}
	}
	for _, scale := range []float32{1, 2} {
		r := image.Rect(0, 0, int(200*scale), int(120*scale))
		want, got := image.NewRGBA(r), image.NewRGBA(r)
		Render(want, direct, Options{Scale: scale})
		Render(got, composed, Options{Scale: scale})
		checkRegion(t, got, want, image.NewRGBA(r), r)
	}
}

// TestBackdropRegionGrow checks that rendering the region grown by
// BackdropRegion, and no more, gives the region the pixels of a full
// render, and that Render gives them for the region alone; the growth
// reaches what the backdrop filters read around their output in the region.
func TestBackdropRegionGrow(t *testing.T) {
	dl := build(func(b *gfx.Builder) {
		backdropScene(b)
		b.FillRect(gfx.RectXYWH(150, 0, 50, 100), gfx.MustParseHex("#22c55e"))
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(160, 30, 30, 40), 4), gfx.Blur{StdDev: 3})
	})
	for _, scale := range []float32{1, 2} {
		bounds := image.Rect(0, 0, int(200*scale), int(100*scale))
		full := image.NewRGBA(bounds)
		background(full)
		bg := cloneRGBA(full)
		Render(full, dl, Options{Scale: scale})
		for _, region := range []image.Rectangle{
			image.Rect(int(60*scale), int(40*scale), int(70*scale), int(50*scale)),   // inside the first backdrop
			image.Rect(0, 0, int(20*scale), int(20*scale)),                           // touches none
			image.Rect(int(140*scale), int(50*scale), int(170*scale), int(60*scale)), // both
		} {
			grown := BackdropRegion(dl, scale, region, bounds)
			if !region.In(grown) {
				t.Fatalf("grown region %v does not contain %v", grown, region)
			}
			for k := range dl.NumBackdrops() {
				if rd := BackdropReadRegion(dl, scale, k, region).Intersect(bounds); !rd.In(grown) {
					t.Errorf("region %v grown to %v misses the reads %v of backdrop %d", region, grown, rd, k)
				}
			}
			// The grown region rendered as is (Render would grow it again).
			got := cloneRGBA(bg)
			r := NewRenderer()
			r.render(got, dl, gfx.Scale(scale, scale), grown)
			for y := region.Min.Y; y < region.Max.Y; y++ {
				i, j := got.PixOffset(region.Min.X, y), got.PixOffset(region.Max.X, y)
				if !bytes.Equal(got.Pix[i:j], full.Pix[i:j]) {
					t.Fatalf("region %v grown to %v: row %d differs from a full render", region, grown, y)
				}
			}
			got = cloneRGBA(bg)
			r.Render(got, dl, Options{Scale: scale, Region: region})
			checkRegion(t, got, full, bg, region)
		}
	}
}
