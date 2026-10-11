package raster

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// coverageImage renders one shape in opaque black on a transparent image, so
// that the alpha channel is the 8-bit coverage.
func coverageImage(w, h int, scale float32, draw func(b *gfx.Builder)) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	Render(img, build(draw), Options{Scale: scale})
	return img
}

// alphaDiff returns the largest alpha difference and the sum of all alpha
// differences of two images of the same size.
func alphaDiff(a, b *image.RGBA) (maxD, sum int, at image.Point) {
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i := a.PixOffset(x, y) + 3
			d := int(a.Pix[i]) - int(b.Pix[i])
			if d < 0 {
				d = -d
			}
			sum += d
			if d > maxD {
				maxD, at = d, image.Pt(x, y)
			}
		}
	}
	return maxD, sum, at
}

// withTolerance runs fn with FlattenTolerance set to tol.
func withTolerance(tol float32, fn func()) {
	old := FlattenTolerance
	FlattenTolerance = tol
	defer func() { FlattenTolerance = old }()
	fn()
}

// The rect fast path computes exactly what the path rasterizer computes for
// the same rectangle.
func TestRectFastPathMatchesPath(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	transforms := []gfx.Affine{
		gfx.Identity,
		gfx.Translate(0.37, 0.61),
		gfx.Scale(1.7, 0.45),
		gfx.Translate(60, 5).Mul(gfx.Scale(-1.3, 1.1)),
		gfx.Translate(60, 0).Mul(gfx.Rotate(math.Pi / 2)),
	}
	for i := 0; i < 300; i++ {
		r := gfx.RectXYWH(f(-5, 50), f(-5, 50), f(0.01, 30), f(0.01, 30))
		m := transforms[i%len(transforms)]
		for _, scale := range []float32{1, 1.25} {
			fast := coverageImage(64, 64, scale, func(b *gfx.Builder) {
				b.PushTransform(m)
				b.FillRect(r, gfx.Black)
				b.PopTransform()
			})
			slow := coverageImage(64, 64, scale, func(b *gfx.Builder) {
				b.PushTransform(m)
				b.FillPath(gfx.RectPath(r), gfx.NonZero, gfx.Black)
				b.PopTransform()
			})
			if d, _, at := alphaDiff(fast, slow); d != 0 {
				t.Fatalf("rect %v under %v at scale %g: fast path differs from the path fill by %d at %v", r, m, scale, d, at)
			}
		}
	}
}

func randomRRect(rng *rand.Rand) gfx.RRect {
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	r := gfx.RectXYWH(f(1, 20), f(1, 20), f(0.5, 40), f(0.5, 40))
	var rad [4]gfx.Point
	for i := range rad {
		switch rng.IntN(4) {
		case 0: // square
		case 1: // circular
			v := f(0, 25)
			rad[i] = gfx.Pt(v, v)
		default: // elliptical, sometimes huge (normalized down: pills)
			rad[i] = gfx.Pt(f(0, 40), f(0, 40))
		}
	}
	return gfx.RRectCorners(r, rad[0], rad[1], rad[2], rad[3])
}

// The rrect fast path agrees with filling the rrect's outline as a path.
// With a fine flattening tolerance the only differences left are the cubic
// approximation of the ellipse in gfx.Path.AddRRect (0.01 logical px) and
// 8-bit rounding, so they agree within a few levels. At the default
// tolerance the polyline's chords cut up to 0.1 px into the curve, which
// moves the coverage of a pixel by up to about 0.1 (25 levels).
func TestRRectFastPathMatchesPath(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 3))
	transforms := []gfx.Affine{
		gfx.Identity,
		gfx.Translate(0.3, 0.7),
		gfx.Scale(1.4, 0.8),
		gfx.Translate(64, 0).Mul(gfx.Scale(-1, 1.2)),
	}
	worstFine, worstDefault := 0, 0
	for i := 0; i < 200; i++ {
		rr := randomRRect(rng)
		m := transforms[i%len(transforms)]
		scale := []float32{1, 2}[i%2]
		size := int(64 * scale)
		fast := coverageImage(size, size, scale, func(b *gfx.Builder) {
			b.PushTransform(m)
			b.FillRRect(rr, gfx.Black)
			b.PopTransform()
		})
		slowPath := func() *image.RGBA {
			return coverageImage(size, size, scale, func(b *gfx.Builder) {
				b.PushTransform(m)
				b.FillPath(rr.Path(), gfx.NonZero, gfx.Black)
				b.PopTransform()
			})
		}
		var fine, def *image.RGBA
		withTolerance(0.002, func() { fine = slowPath() })
		def = slowPath()
		d, _, at := alphaDiff(fast, fine)
		worstFine = max(worstFine, d)
		if d > 6 {
			t.Errorf("rrect %v under %v at scale %g: fast path differs from a fine path fill by %d at %v", rr, m, scale, d, at)
		}
		d, _, _ = alphaDiff(fast, def)
		worstDefault = max(worstDefault, d)
		if d > 25 {
			t.Errorf("rrect %v under %v at scale %g: fast path differs from the default path fill by %d", rr, m, scale, d)
		}
	}
	t.Logf("largest alpha difference: %d (fine flattening), %d (default flattening)", worstFine, worstDefault)
}

// supersampled returns the coverage of every pixel of a w x h image by
// point-sampling contains on an n x n grid per pixel.
func supersampled(w, h, n int, contains func(x, y float64) bool) []float64 {
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := 0
			for j := 0; j < n; j++ {
				for i := 0; i < n; i++ {
					if contains(float64(x)+(float64(i)+0.5)/float64(n), float64(y)+(float64(j)+0.5)/float64(n)) {
						c++
					}
				}
			}
			out[y*w+x] = float64(c) / float64(n*n)
		}
	}
	return out
}

// TestRRectFastPathExact compares the analytic rrect coverage with a 32x32
// supersampled reference, including radii whose opposite corner boxes
// overlap (the outside regions of opposite corners never overlap, which the
// fast path relies on).
func TestRRectFastPathExact(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	cases := []gfx.RRect{
		gfx.RRectCorners(gfx.RectXYWH(2.3, 3.6, 30, 26), gfx.Pt(28, 24), gfx.Pt(0, 0), gfx.Pt(28, 24), gfx.Pt(0, 0)),
		gfx.RRectCorners(gfx.RectXYWH(1.5, 1.5, 33, 33), gfx.Pt(0, 0), gfx.Pt(33, 33), gfx.Pt(0, 0), gfx.Pt(33, 33)),
		gfx.NewRRect(gfx.RectXYWH(4.2, 2.7, 29.1, 29.1), 999),
		gfx.RRectXY(gfx.RectXYWH(0.6, 8.2, 34, 12.5), 17, 6.25),
	}
	for i := 0; i < 40; i++ {
		var rad [4]gfx.Point
		for k := range rad {
			rad[k] = gfx.Pt(f(0, 30), f(0, 30))
		}
		cases = append(cases, gfx.RRectCorners(gfx.RectXYWH(f(0, 3), f(0, 3), f(4, 33), f(4, 33)), rad[0], rad[1], rad[2], rad[3]))
	}
	const w, h, n = 38, 38, 32
	worst := 0.0
	for _, rr := range cases {
		img := coverageImage(w, h, 1, func(b *gfx.Builder) { b.FillRRect(rr, gfx.Black) })
		ref := supersampled(w, h, n, func(x, y float64) bool { return rr.Contains(gfx.Pt(float32(x), float32(y))) })
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				got := float64(img.Pix[img.PixOffset(x, y)+3]) / 255
				// 1/255 rounding + the sampling error of the reference
				// (a few boundary samples) + 1/256 px edge snapping.
				d := math.Abs(got - ref[y*w+x])
				worst = max(worst, d)
				if d > 0.03 {
					t.Fatalf("rrect %v: pixel (%d, %d) coverage %.4f, supersampled %.4f", rr, x, y, got, ref[y*w+x])
				}
			}
		}
	}
	t.Logf("largest difference from the supersampled reference: %.4f", worst)
}

// TestDiskArea checks the closed form of the quarter-disk area against
// numerical integration.
func TestDiskArea(t *testing.T) {
	integ := func(u, v float64) float64 {
		const n = 200000
		s := 0.0
		for i := 0; i < n; i++ {
			x := (float64(i) + 0.5) / n * u
			s += min(v, math.Sqrt(max(0, 1-x*x)))
		}
		return s * u / n
	}
	for _, uv := range [][2]float64{{1, 1}, {0.5, 0.5}, {0.9, 0.9}, {1, 0.1}, {0.1, 1}, {0.99, 0.3}, {0.7072, 0.7072}, {0.3, 0.99999}} {
		got, want := diskArea(uv[0], uv[1]), integ(uv[0], uv[1])
		if math.Abs(got-want) > 1e-7 {
			t.Errorf("diskArea(%v, %v) = %.10f, want %.10f", uv[0], uv[1], got, want)
		}
	}
	if got := diskArea(1, 1); math.Abs(got-math.Pi/4) > 1e-15 {
		t.Errorf("diskArea(1, 1) = %v, want π/4", got)
	}
}
