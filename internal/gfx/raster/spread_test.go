package raster

import (
	"hash/fnv"
	"image"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Gradient spread modes (SVG spreadMethod): a 3x4 grid of 100x100 cells,
// one column per mode (pad, reflect, repeat) and one row per gradient
// (linear, radial, elliptical radial under a rotation, focal). The same
// gradients are compared with Chromium as SVG in internal/svg
// (TestChromiumSVG, the spread-* cells).

var spreadStops = []gfx.GradientStop{
	{Offset: 0, Color: gfx.MustParseHex("#f43f5e")},
	{Offset: 0.5, Color: gfx.MustParseHex("#fbbf24")},
	{Offset: 1, Color: gfx.MustParseHex("#4f46e580")},
}

func drawSpread(b *gfx.Builder) {
	modes := []gfx.SpreadMode{gfx.SpreadPad, gfx.SpreadReflect, gfx.SpreadRepeat}
	for col, m := range modes {
		x := float32(col * 100)
		cell := func(row int) gfx.Rect { return gfx.RectXYWH(x, float32(row*100), 100, 100) }
		b.FillRect(cell(0), gfx.LinearGradient{Start: gfx.Pt(x+35, 30), End: gfx.Pt(x+55, 45), Stops: spreadStops, Interp: gfx.InterpSRGB, Spread: m})
		b.FillRect(cell(1), gfx.RadialGradient{Center: gfx.Pt(x+45, 150), Radius: gfx.Pt(18, 18), Stops: spreadStops, Interp: gfx.InterpSRGB, Spread: m})
		b.PushClipRect(cell(2))
		b.PushTransform(gfx.RotateAbout(0.5, gfx.Pt(x+50, 250)))
		b.FillRect(cell(2).Outset(30), gfx.RadialGradient{Center: gfx.Pt(x+50, 250), Radius: gfx.Pt(22, 10), Stops: spreadStops, Spread: m})
		b.PopTransform()
		b.PopClip()
		b.FillRRect(gfx.NewRRect(cell(3).Inset(5), 12), gfx.FocalGradient{Focal: gfx.Pt(x+40, 343), FocalRadius: 3, Center: gfx.Pt(x+50, 350), Radius: 24,
			Stops: spreadStops, Interp: gfx.InterpSRGB, Spread: m})
	}
}

var spreadScene = scene{"spread", 300, 400, drawSpread}

func TestGoldenSpread(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		name := "spread_x1"
		if scale == 2 {
			name = "spread_x2"
		}
		t.Run(name, func(t *testing.T) {
			golden.Image(t, name, renderScene(spreadScene, scale))
		})
	}
}

func TestSpreadT(t *testing.T) {
	cases := []struct {
		t               float64
		reflect, repeat float64
	}{
		{0, 0, 0}, {0.25, 0.25, 0.25}, {1, 1, 0}, {1.25, 0.75, 0.25}, {2, 0, 0}, {2.5, 0.5, 0.5},
		{-0.25, 0.25, 0.75}, {-1, 1, 0}, {-1.75, 0.25, 0.25}, {3.75, 0.25, 0.75},
	}
	for _, c := range cases {
		if got := spreadT(c.t, gfx.SpreadReflect); got != c.reflect {
			t.Errorf("reflect(%g) = %g, want %g", c.t, got, c.reflect)
		}
		if got := spreadT(c.t, gfx.SpreadRepeat); got != c.repeat {
			t.Errorf("repeat(%g) = %g, want %g", c.t, got, c.repeat)
		}
		if got := spreadT(c.t, gfx.SpreadPad); got != c.t {
			t.Errorf("pad(%g) = %g", c.t, got)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, m := range []gfx.SpreadMode{gfx.SpreadReflect, gfx.SpreadRepeat} {
			if got := spreadT(v, m); !math.IsNaN(got) {
				t.Errorf("%v(%g) = %g, want NaN (first color)", m, v, got)
			}
		}
	}
}

// TestSpreadPixels checks repeated and reflected linear gradients pixel by
// pixel against the padded gradient at the folded offset.
func TestSpreadPixels(t *testing.T) {
	stops := []gfx.GradientStop{{Offset: 0, Color: gfx.RGB8(0, 0, 0)}, {Offset: 1, Color: gfx.RGB8(250, 100, 0)}}
	render := func(p gfx.Paint) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 100, 1))
		b := gfx.NewBuilder()
		b.FillRect(gfx.RectXYWH(0, 0, 100, 1), p)
		Render(img, b.Finish(), Options{})
		return img
	}
	pad := render(gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(100, 0), Stops: stops, Interp: gfx.InterpSRGB})
	for _, m := range []gfx.SpreadMode{gfx.SpreadReflect, gfx.SpreadRepeat} {
		// Ten pixels per period: pixel x has t = (x + 0.5) / 10.
		img := render(gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(10, 0), Stops: stops, Interp: gfx.InterpSRGB, Spread: m})
		for x := 0; x < 100; x++ {
			u := spreadT((float64(x)+0.5)/10, m)
			px := int(u * 100) // the pad pixel whose center is at u
			want := pad.Pix[4*px]
			got := img.Pix[4*x]
			if d := int(got) - int(want); d < -3 || d > 3 {
				t.Errorf("%v x=%d: red %d, padded gradient at %.3f gives %d", m, x, got, u, want)
			}
		}
	}
}

// TestSpreadEqualDump checks that gfx compares and prints the spread mode,
// and that padded gradients print as before.
func TestSpreadEqualDump(t *testing.T) {
	pad := gfx.LinearGradient{End: gfx.Pt(1, 0), Stops: spreadStops}
	rep := pad
	rep.Spread = gfx.SpreadRepeat
	if gfx.PaintEqual(pad, rep) || !gfx.PaintEqual(rep, rep) {
		t.Error("PaintEqual ignores Spread")
	}
	b := gfx.NewBuilder()
	b.FillRect(gfx.RectXYWH(0, 0, 1, 1), pad)
	b.FillRect(gfx.RectXYWH(0, 0, 1, 1), rep)
	b.FillRect(gfx.RectXYWH(0, 0, 1, 1), gfx.FocalGradient{Center: gfx.Pt(1, 0), Radius: 2, Stops: spreadStops, Spread: gfx.SpreadReflect})
	got := b.Finish().String()
	for _, want := range []string{"linear((0,0)->(1,0) oklab [", "linear((0,0)->(1,0) oklab repeat [", "oklab reflect ["} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestSpreadUnknownPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unknown SpreadMode did not panic")
		}
	}()
	gfx.NewBuilder().FillRect(gfx.RectXYWH(0, 0, 1, 1), gfx.LinearGradient{End: gfx.Pt(1, 0), Stops: spreadStops, Spread: 7})
}

// spreadDigest hashes spreadT on random input and the pixels of the spread
// scene at two scales.
func spreadDigest() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	rng := rand.New(rand.NewPCG(95, 8))
	for i := 0; i < 4000; i++ {
		v := float64(rng.Float64()*40) - 20
		for _, m := range []gfx.SpreadMode{gfx.SpreadReflect, gfx.SpreadRepeat} {
			b := math.Float64bits(spreadT(v, m))
			for k := range buf {
				buf[k] = byte(b >> (8 * k))
			}
			h.Write(buf[:])
		}
	}
	for _, scale := range []float32{1, 1.5} {
		h.Write(renderScene(spreadScene, scale).Pix)
	}
	return h.Sum64()
}

// wantSpreadDigest is spreadDigest() recorded on amd64 (GOAMD64=v1). Every
// architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static -run Deterministic ./internal/gfx/raster
const wantSpreadDigest uint64 = 0x67879e41ddaf2df0

func TestSpreadDeterministic(t *testing.T) {
	if got := spreadDigest(); got != wantSpreadDigest {
		t.Errorf("spreadDigest = %#x, want %#x (the same on every architecture)", got, wantSpreadDigest)
	}
}
