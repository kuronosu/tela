package raster

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// refShape is a device-space rounded rectangle for the reference
// computations (radii TL, TR, BR, BL; normalized).
type refShape struct {
	x0, y0, x1, y1 float64
	rad            [4][2]float64
}

// span returns the shape's horizontal extent at height v.
func (q *refShape) span(v float64) (a, b float64) {
	a, b = q.x0, q.x1
	arc := func(r [2]float64, d float64) float64 { // horizontal inset of a corner at depth d into its box
		if !(r[0] > 0 && r[1] > 0) || d <= 0 {
			return 0
		}
		t := math.Min(d/r[1], 1)
		return r[0] - r[0]*math.Sqrt(1-t*t)
	}
	if r := q.rad[0]; v < q.y0+r[1] {
		a = math.Max(a, q.x0+arc(r, q.y0+r[1]-v))
	}
	if r := q.rad[3]; v > q.y1-r[1] {
		a = math.Max(a, q.x0+arc(r, v-(q.y1-r[1])))
	}
	if r := q.rad[1]; v < q.y0+r[1] {
		b = math.Min(b, q.x1-arc(r, q.y0+r[1]-v))
	}
	if r := q.rad[2]; v > q.y1-r[1] {
		b = math.Min(b, q.x1-arc(r, v-(q.y1-r[1])))
	}
	return a, b
}

func refPhi(t float64) float64 { return 0.5 * math.Erfc(-t/math.Sqrt2) }

func refI(t float64) float64 {
	return t*refPhi(t) + math.Exp(-t*t/2)/math.Sqrt(2*math.Pi)
}

// refStep is the average over the pixel [p, p+1] of Phi((x-a)/s).
func refStep(p, a, s float64) float64 {
	return s * (refI((p+1-a)/s) - refI((p-a)/s))
}

// refBlur returns, for the pixels of zone, the shape convolved with a
// Gaussian (sx, sy) and averaged over each pixel. The horizontal integral of
// each row of the shape is closed form; the vertical one is a midpoint rule
// with 32 samples per pixel, 512 near the top and bottom edges (where the
// corners' arcs have a square-root singularity).
func refBlur(q refShape, sx, sy float64, zone image.Rectangle) [][]float64 {
	type sample struct{ v, dv float64 }
	var ss []sample
	add := func(lo, hi, per float64) {
		if !(hi > lo) {
			return
		}
		n := int(math.Ceil((hi - lo) * per))
		dv := (hi - lo) / float64(n)
		for k := 0; k < n; k++ {
			ss = append(ss, sample{lo + (float64(k)+0.5)*dv, dv})
		}
	}
	rmax := 0.0
	for _, r := range q.rad {
		rmax = math.Max(rmax, r[1])
	}
	e := math.Min(rmax, (q.y1-q.y0)/2)
	add(q.y0, q.y0+e, 512)
	add(q.y0+e, q.y1-e, 32)
	add(q.y1-e, q.y1, 512)

	w, h := zone.Dx(), zone.Dy()
	// H[s][x]: the row's horizontal integral; K[s][y]: the vertical weight.
	H := make([][]float64, len(ss))
	K := make([][]float64, len(ss))
	for i, s := range ss {
		a, b := q.span(s.v)
		H[i] = make([]float64, w)
		if b > a {
			for x := 0; x < w; x++ {
				p := float64(zone.Min.X + x)
				H[i][x] = refStep(p, a, sx) - refStep(p, b, sx)
			}
		}
		K[i] = make([]float64, h)
		for y := 0; y < h; y++ {
			p := float64(zone.Min.Y + y)
			K[i][y] = (refPhi((p+1-s.v)/sy) - refPhi((p-s.v)/sy)) * s.dv
		}
	}
	out := make([][]float64, h)
	lo := 0
	for y := range out {
		out[y] = make([]float64, w)
		// The samples are sorted by v: only those within 9 sigma matter.
		top := float64(zone.Min.Y+y) - 9*sy
		for lo < len(ss) && ss[lo].v < top {
			lo++
		}
		for i := lo; i < len(ss) && ss[i].v < top+1+18*sy; i++ {
			k := K[i][y]
			for x, hv := range H[i] {
				out[y][x] += k * hv
			}
		}
	}
	return out
}

// deviceShape returns the device-space shadow shape of c under a
// scale/translate transform.
func deviceShape(c gfx.DrawShadow, m gfx.Affine) refShape {
	shape, _ := shadowShape(c)
	var g rrBlur
	g.setDevice(&m, shape, 1, 1)
	return refShape{g.x0, g.y0, g.x1, g.y1, g.rad}
}

// evalShadow returns the analytic values of the shadow of c under m for the
// pixels of zone, and the name of the method used.
func evalShadow(c gfx.DrawShadow, m gfx.Affine, zone image.Rectangle) ([][]float64, string) {
	r := NewRenderer()
	r.emitFn = r.emit
	r.ctm = m
	r.blit.clip = zone
	shape, ok := shadowShape(c)
	ev, _ := r.shadowEvaluator(c, shape, ok, float64(c.Blur)/2, zone)
	out := make([][]float64, zone.Dy())
	want := make([]uint8, zone.Dx())
	for i := range want {
		want[i] = 1
	}
	for y := range out {
		out[y] = make([]float64, zone.Dx())
		ev.row(zone.Min.Y+y, out[y], want)
	}
	return out, fmt.Sprintf("%T", ev)
}

// shadowCases are the geometries of the exactness tests.
var shadowCases = []struct {
	name string
	c    gfx.DrawShadow
}{
	{"uniform", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(20.3, 18.6, 70, 50), 12), Blur: 10}},
	{"uniform-sharp", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(20.3, 18.6, 70, 50), 12), Blur: 1}},
	{"tiny-blur", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(20.3, 18.6, 70, 50), 12), Blur: 0.2}},
	{"huge-blur", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(40, 40, 40, 30), 10), Blur: 60}},
	{"per-corner", gfx.DrawShadow{RRect: gfx.RRectCorners(gfx.RectXYWH(15.5, 20, 80, 60),
		gfx.Pt(0, 0), gfx.Pt(24, 24), gfx.Pt(6, 6), gfx.Pt(30, 12)), Blur: 8}},
	{"elliptical", gfx.DrawShadow{RRect: gfx.RRectXY(gfx.RectXYWH(20, 20, 80, 50), 30, 10), Blur: 6}},
	{"opposite-overlap", gfx.DrawShadow{RRect: gfx.RRectCorners(gfx.RectXYWH(25, 25, 52, 54),
		gfx.Pt(48, 48), gfx.Pt(0, 0), gfx.Pt(48, 48), gfx.Pt(0, 0)), Blur: 5}},
	{"pill", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(20, 40, 90, 24), 999), Blur: 7}},
	{"circle", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(30, 30, 50, 50), 999), Blur: 3}},
	{"offset-spread", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(30, 25, 60, 45), 8), Offset: gfx.Pt(4, 9), Blur: 12, Spread: 6}},
	{"small-radius-spread", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(30, 25, 60, 45), 2), Blur: 4, Spread: 10}},
	{"negative-spread", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(30, 25, 60, 45), 16), Offset: gfx.Pt(0, 12), Blur: 20, Spread: -6}},
	{"inset", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(20, 20, 80, 60), 14), Offset: gfx.Pt(3, 5), Blur: 10, Spread: 2, Inset: true}},
	{"inset-negative-spread", gfx.DrawShadow{RRect: gfx.NewRRect(gfx.RectXYWH(20, 20, 80, 60), 4), Blur: 6, Spread: -8, Inset: true}},
	{"square", gfx.DrawShadow{RRect: gfx.RRectFromRect(gfx.RectXYWH(30.25, 30.75, 50, 40)), Blur: 5}},
}

// TestShadowExact compares the analytic blur with the brute-force reference
// for the inner (pre-clip) value of every pixel, at scales 1 and 2 and under
// a flip with a non-uniform scale.
func TestShadowExact(t *testing.T) {
	transforms := []struct {
		name string
		m    gfx.Affine
	}{
		{"x1", gfx.Identity},
		{"x2", gfx.Scale(2, 2)},
		{"flip-aniso", gfx.Translate(130, 0).Mul(gfx.Scale(-1, 1.5))},
	}
	if testing.Short() {
		transforms = transforms[:1]
	}
	for _, tc := range shadowCases {
		for _, tr := range transforms {
			t.Run(tc.name+"_"+tr.name, func(t *testing.T) {
				t.Parallel()
				c := tc.c
				c.Color = gfx.Black
				sx, sy := deviceSigmas(&tr.m, float64(c.Blur)/2)
				q := deviceShape(c, tr.m)
				zone := image.Rect(int(q.x0-3*sx)-2, int(q.y0-3*sy)-2, int(q.x1+3*sx)+3, int(q.y1+3*sy)+3)
				got, method := evalShadow(c, tr.m, zone)
				want := refBlur(q, sx, sy, zone)
				worst, at := 0.0, image.Point{}
				for y := range got {
					for x := range got[y] {
						if d := math.Abs(got[y][x] - want[y][x]); d > worst {
							worst, at = d, image.Pt(x, y).Add(zone.Min)
						}
					}
				}
				t.Logf("%s: max error %.4f levels at %v", method, worst*255, at)
				limit := 0.5 // levels of 255
				if method != "*raster.sepEval" {
					limit = 2.5 // hard shadows and blurred masks
				}
				if worst*255 > limit {
					t.Errorf("%s: max error %.3f levels at %v (got %.5f want %.5f)", method, worst*255, at,
						got[at.Y-zone.Min.Y][at.X-zone.Min.X], want[at.Y-zone.Min.Y][at.X-zone.Min.X])
				}
			})
		}
	}
}

// TestShadowSpreadRadius checks the CSS spread rule for radii.
func TestShadowSpreadRadius(t *testing.T) {
	for _, c := range []struct{ r, e, want float32 }{
		{10, 5, 15},       // r >= spread: plain sum
		{0, 5, 0},         // square corners stay square
		{5, 10, 13.75},    // r/e = 0.5: e * (1 - 0.125) = 8.75
		{10, -4, 6},       // shrinking
		{3, -4, 0},        // floored
		{2, 0, 2},         // no spread
		{9.99, 10, 19.99}, // continuous at r = e
	} {
		got := spreadRadius(c.r, c.e)
		if math.Abs(float64(got-c.want)) > 1e-3 {
			t.Errorf("spreadRadius(%v, %v) = %v, want %v", c.r, c.e, got, c.want)
		}
	}
}
