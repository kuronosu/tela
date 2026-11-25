package core

import (
	"bytes"
	"fmt"
	"image"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// compositeScene is the state of a synthetic frame with layers composited
// with the Porter-Duff operators, some of which clear their whole clip
// (gfx.BlendMode.Unbounded), at the root and in an isolated group, and with
// image patterns.
type compositeScene struct {
	bg      [2]gfx.Rect
	cover   bool
	op      [3]gfx.BlendMode
	opacity [3]float32
	clip    [3]gfx.Rect
	src     [3]gfx.Rect // empty: no content
	angle   float32
	pattern gfx.ImagePattern
	mask    gfx.Rect
	noMask  bool
}

var compositeTile = gfx.NewImagePremul(4, 4, func() []uint8 {
	pix := make([]uint8, 0, 64)
	for i := range 16 {
		v := uint8(16 * i)
		pix = append(pix, v, 255-v, uint8(60*(i%4)), 255)
	}
	return pix
}())

func (s *compositeScene) list() *gfx.DisplayList {
	b := gfx.NewBuilder()
	for i := range s.bg {
		b.FillRect(s.bg[i], gfx.RGB8(uint8(60+120*i), 99, 241).WithAlpha(0.8))
	}
	// At the root, under a clip: the frame's own pixels.
	b.PushClipRRect(gfx.NewRRect(s.clip[0], 9))
	b.PushLayer(s.opacity[0], s.op[0])
	b.FillRect(s.src[0], s.pattern)
	b.PopLayer()
	b.PopClip()
	if s.cover {
		b.FillRect(gfx.RectXYWH(60, 30, 90, 80), s.pattern)
	}
	// In a group, under a transform and a clip.
	b.PushClipRect(gfx.RectXYWH(10, 40, 130, 70))
	b.PushLayer(1, gfx.SrcOver)
	b.FillRect(gfx.RectXYWH(14, 44, 100, 60), gfx.MustParseHex("#10b981").WithAlpha(0.7))
	b.PushTransform(gfx.RotateAbout(s.angle, gfx.Pt(70, 70)))
	b.PushClipRect(s.clip[1])
	b.PushLayer(s.opacity[1], s.op[1], gfx.Blur{StdDev: 2})
	b.FillRect(s.src[1], gfx.MustParseHex("#f43f5e"))
	b.PopLayer()
	b.PopClip()
	b.PopTransform()
	mb := gfx.NewBuilder()
	if !s.noMask {
		mb.FillRect(s.mask, gfx.Black.WithAlpha(0.7))
	}
	b.PushClipRect(s.clip[2])
	b.PushMaskLayer(gfx.Mask{List: mb.Finish()}, s.opacity[2], s.op[2])
	b.FillRect(s.src[2], gfx.MustParseHex("#f59e0b"))
	b.PopLayer()
	b.PopClip()
	b.PopLayer()
	b.PopClip()
	return b.Finish()
}

func (s *compositeScene) mutate(rng *rand.Rand) string {
	f := func(lo, hi float32) float32 { return lo + float32(rng.Float64())*(hi-lo) }
	rect := func() gfx.Rect { return gfx.RectXYWH(f(0, 120), f(0, 90), f(8, 50), f(8, 40)) }
	i := rng.IntN(3)
	switch rng.IntN(9) {
	case 0:
		s.bg[rng.IntN(2)] = rect()
		return "move backdrop"
	case 1:
		s.op[i] = gfx.SrcIn + gfx.BlendMode(rng.IntN(int(gfx.Xor-gfx.SrcIn)+1))
		return "operator " + s.op[i].String()
	case 2:
		s.opacity[i] = f(0.2, 1)
		if rng.IntN(3) == 0 {
			s.opacity[i] = 0
		}
		return "opacity"
	case 3:
		s.clip[i] = rect()
		return "clip"
	case 4:
		s.src[i] = rect()
		return "move source"
	case 5:
		s.src[i] = gfx.Rect{}
		return "no source"
	case 6:
		s.angle = f(0, 0.5)
		return "rotate"
	case 7:
		s.cover = !s.cover
		s.pattern.Transform = gfx.Translate(f(0, 4), 0)
		return "cover and pattern"
	default:
		s.mask, s.noMask = rect(), rng.IntN(4) == 0
		return "mask"
	}
}

// TestDamageComposite checks that incremental frames stay bit-identical to
// full rasterizations when layers composited with the Porter-Duff operators
// and patterns change, or what lies under them does.
func TestDamageComposite(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		rng := rand.New(rand.NewPCG(319, uint64(scale*2)))
		r := gfx.RectXYWH
		s := &compositeScene{
			bg: [2]gfx.Rect{r(10, 10, 50, 30), r(90, 20, 40, 60)}, op: [3]gfx.BlendMode{gfx.SrcIn, gfx.DstAtop, gfx.Copy},
			opacity: [3]float32{1, 0.8, 1}, clip: [3]gfx.Rect{r(20, 10, 60, 40), r(30, 50, 50, 40), r(80, 60, 40, 40)},
			src: [3]gfx.Rect{r(30, 15, 40, 30), r(40, 60, 30, 30), r(90, 70, 20, 20)}, mask: r(85, 62, 30, 30),
			pattern: gfx.ImagePattern{Image: compositeTile, Transform: gfx.Identity},
		}
		w, h := int(160*scale), int(120*scale)
		fb := image.NewRGBA(image.Rect(0, 0, w, h))
		prev := &frame{dl: s.list(), scale: scale}
		raster.Clear(fb, image.Rectangle{}, gfx.White)
		raster.Render(fb, prev.dl, raster.Options{Scale: scale})
		partial := 0
		for step := range 200 {
			what := s.mutate(rng)
			next := &frame{dl: s.list(), scale: scale}
			d := frameDamage(prev, next, fb.Rect, nil)
			if d.Full {
				raster.Clear(fb, image.Rectangle{}, gfx.White)
				raster.Render(fb, next.dl, raster.Options{Scale: scale})
			} else {
				for _, rc := range d.Rects {
					raster.Clear(fb, rc, gfx.White)
					raster.Render(fb, next.dl, raster.Options{Scale: scale, Region: rc})
				}
				partial++
			}
			want := image.NewRGBA(fb.Rect)
			raster.Clear(want, image.Rectangle{}, gfx.White)
			raster.Render(want, next.dl, raster.Options{Scale: scale})
			if !bytes.Equal(fb.Pix, want.Pix) {
				p, _ := firstDiff(fb, want)
				t.Fatalf("x%g step %d (%s): incremental frame differs from a full rasterization at %v (damage %v)",
					scale, step, what, p, fmt.Sprint(d.Rects))
			}
			prev = next
		}
		if partial == 0 {
			t.Errorf("x%g: every frame was a full rasterization", scale)
		}
	}
}

// TestOpaquePattern checks which patterns hide what is under them: an
// opaque image that repeats along both axes.
func TestOpaquePattern(t *testing.T) {
	clear := gfx.NewImagePremul(1, 1, make([]uint8, 4))
	for _, c := range []struct {
		p    gfx.ImagePattern
		want bool
	}{
		{gfx.ImagePattern{Image: compositeTile, Transform: gfx.Identity}, true},
		{gfx.ImagePattern{Image: compositeTile, Repeat: gfx.RepeatX, Transform: gfx.Identity}, false},
		{gfx.ImagePattern{Image: compositeTile, Repeat: gfx.RepeatNone, Transform: gfx.Identity}, false},
		{gfx.ImagePattern{Image: clear, Transform: gfx.Identity}, false},
	} {
		if got := new(damageScratch).opaquePaint(c.p); got != c.want {
			t.Errorf("%s: opaque %v, want %v", c.p.Repeat, got, c.want)
		}
	}
}
