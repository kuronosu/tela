package raster

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestShadowRotated checks the local-space evaluation under rotations: by 90
// degrees against the reference of the rotated shape (the pixel squares are
// then exactly the local squares), and by an arbitrary angle against a
// brute-force average of the rotation-invariant blur over each pixel.
func TestShadowRotated(t *testing.T) {
	c := gfx.DrawShadow{RRect: gfx.RRectCorners(gfx.RectXYWH(10, 15, 50, 36),
		gfx.Pt(14, 14), gfx.Pt(4, 4), gfx.Pt(20, 10), gfx.Pt(0, 0)), Offset: gfx.Pt(2, 3), Blur: 6, Spread: 1, Color: gfx.Black}
	sigma := float64(c.Blur) / 2

	t.Run("90", func(t *testing.T) {
		m := gfx.Translate(100, 0).Mul(gfx.Scale(1.5, 1.5)).Mul(gfx.Rotate(math.Pi / 2))
		shape, _ := shadowShape(c)
		// (x, y) -> (100 - 1.5y, 1.5x); local corners TL, TR, BR, BL become
		// device TR, BR, BL, TL with swapped radii.
		rr, s := shape.Rect, 1.5
		swap := func(p gfx.Point) [2]float64 { return [2]float64{float64(p.Y) * s, float64(p.X) * s} }
		q := refShape{
			x0: 100 - float64(rr.Y1)*s, x1: 100 - float64(rr.Y0)*s,
			y0: float64(rr.X0) * s, y1: float64(rr.X1) * s,
			rad: [4][2]float64{swap(shape.Radii[3]), swap(shape.Radii[0]), swap(shape.Radii[1]), swap(shape.Radii[2])},
		}
		zone := image.Rect(int(q.x0)-12, int(q.y0)-12, int(q.x1)+12, int(q.y1)+12)
		got, method := evalShadow(c, m, zone)
		if method != "*raster.localEval" {
			t.Fatalf("method %s, want the local evaluation", method)
		}
		want := refBlur(q, sigma*s, sigma*s, zone)
		worst := maxDiff(got, want)
		t.Logf("max error %.4f levels", worst*255)
		if worst*255 > 0.5 {
			t.Errorf("max error %.3f levels", worst*255)
		}
	})

	t.Run("angle", func(t *testing.T) {
		const theta = 0.4
		m := gfx.RotateAbout(theta, gfx.Pt(35, 33))
		zone := image.Rect(-5, -5, 75, 75)
		got, method := evalShadow(c, m, zone)
		if method != "*raster.localEval" {
			t.Fatalf("method %s, want the local evaluation", method)
		}
		// Brute force: the blurred shape (in local space, where it is axis
		// aligned) at 3x3 points of each device pixel.
		shape, _ := shadowShape(c)
		var g rrBlur
		g.setLocal(shape, 1, 1)
		q := refShape{g.x0, g.y0, g.x1, g.y1, g.rad}
		type sample struct{ v, dv, a, b float64 }
		var ss []sample
		n := int((q.y1 - q.y0) * 64)
		for k := 0; k < n; k++ {
			v := q.y0 + (float64(k)+0.5)*(q.y1-q.y0)/float64(n)
			a, b := q.span(v)
			ss = append(ss, sample{v, (q.y1 - q.y0) / float64(n), a, b})
		}
		inv, _ := m.Invert()
		point := func(x, y float64) float64 {
			p := inv.Apply(gfx.Pt(float32(x), float32(y)))
			lx, ly := float64(p.X), float64(p.Y)
			var s float64
			for _, smp := range ss {
				d := (ly - smp.v) / sigma
				if math.Abs(d) > 8 || smp.b <= smp.a {
					continue
				}
				gv := math.Exp(-d*d/2) / (sigma * math.Sqrt(2*math.Pi))
				s += gv * smp.dv * (refPhi((lx-smp.a)/sigma) - refPhi((lx-smp.b)/sigma))
			}
			return s
		}
		worst := 0.0
		for y := zone.Min.Y; y < zone.Max.Y; y++ {
			for x := zone.Min.X; x < zone.Max.X; x++ {
				var want float64
				for j := 0; j < 3; j++ {
					for i := 0; i < 3; i++ {
						want += point(float64(x)+(float64(i)+0.5)/3, float64(y)+(float64(j)+0.5)/3) / 9
					}
				}
				worst = math.Max(worst, math.Abs(got[y-zone.Min.Y][x-zone.Min.X]-want))
			}
		}
		t.Logf("max error %.4f levels", worst*255)
		if worst*255 > 1 {
			t.Errorf("max error %.3f levels", worst*255)
		}
	})
}

func maxDiff(a, b [][]float64) float64 {
	worst := 0.0
	for y := range a {
		for x := range a[y] {
			worst = math.Max(worst, math.Abs(a[y][x]-b[y][x]))
		}
	}
	return worst
}

// TestShadowMaskPath measures the mask-and-blur path (skews, extreme
// geometry) against the reference on axis-aligned shapes.
func TestShadowMaskPath(t *testing.T) {
	for _, tc := range shadowCases {
		c := tc.c
		sigma := float64(c.Blur) / 2
		if sigma < sigmaMin {
			continue
		}
		q := deviceShape(c, gfx.Identity)
		zone := image.Rect(int(q.x0-3*sigma)-2, int(q.y0-3*sigma)-2, int(q.x1+3*sigma)+3, int(q.y1+3*sigma)+3)
		r := NewRenderer()
		r.ctm = gfx.Identity
		r.blit.clip = zone
		shape, _ := shadowShape(c)
		kx, ky := r.shadowKernels(sigma, sigma)
		ev, _ := r.maskEval(shape, zone, &kx, &ky)
		got := make([][]float64, zone.Dy())
		want := make([]uint8, zone.Dx())
		for i := range want {
			want[i] = 1
		}
		for y := range got {
			got[y] = make([]float64, zone.Dx())
			ev.row(zone.Min.Y+y, got[y], want)
		}
		worst := maxDiff(got, refBlur(q, sigma, sigma, zone))
		t.Logf("%s: max error %.2f levels", tc.name, worst*255)
		// The three-box blur is about 6% narrower than the Gaussian; below
		// sigma 1, the kernel is truncated at 3 sigma.
		lim := 10.0
		if sigma < 1 {
			lim = 16
		}
		if worst*255 > lim {
			t.Errorf("%s: max error %.2f levels", tc.name, worst*255)
		}
	}
}

// TestShadowClipRules renders black shadows onto a transparent image and
// checks every pixel's alpha against the reference value times the box
// coverage rule: outer shadows only outside the box, inset only inside.
func TestShadowClipRules(t *testing.T) {
	scales := []float32{1, 2}
	if testing.Short() {
		scales = scales[:1]
	}
	for _, tc := range shadowCases {
		for _, scale := range scales {
			c := tc.c
			c.Color = gfx.Black
			m := gfx.Scale(scale, scale)
			sx, sy := deviceSigmas(&m, float64(c.Blur)/2)
			if sx < sigmaMin {
				continue // a hard shadow; see TestShadowHard
			}
			t.Run(fmt.Sprintf("%s_x%g", tc.name, scale), func(t *testing.T) {
				t.Parallel()
				q := deviceShape(c, m)
				w := int(float32(140) * scale)
				zone := image.Rect(0, 0, w, w)
				img := image.NewRGBA(zone)
				Render(img, build(func(b *gfx.Builder) { b.DrawShadow(c) }), Options{Scale: scale})
				box := image.NewRGBA(zone)
				Render(box, build(func(b *gfx.Builder) { b.FillRRect(c.RRect, gfx.Black) }), Options{Scale: scale})
				ref := refBlur(q, sx, sy, zone)
				worst, at := 0, image.Point{}
				for y := 0; y < w; y++ {
					for x := 0; x < w; x++ {
						v := ref[y][x]
						bc := float64(box.Pix[box.PixOffset(x, y)+3])
						var want float64
						if c.Inset {
							want = (1 - v) * bc
						} else {
							want = v * (255 - bc)
							// Beyond 3 sigma from the shape the shadow is cut.
							if float64(x)+1 <= q.x0-3*sx || float64(x) >= q.x1+3*sx || float64(y)+1 <= q.y0-3*sy || float64(y) >= q.y1+3*sy {
								want = 0
							}
						}
						got := int(img.Pix[img.PixOffset(x, y)+3])
						if d := iabs(got - int(math.Round(want))); d > worst {
							worst, at = d, image.Pt(x, y)
						}
					}
				}
				if worst > 1 {
					t.Errorf("alpha differs by %d at %v", worst, at)
				}
			})
		}
	}
}

// TestShadowHard checks that a shadow without blur is the fill of its
// shape, clipped by the box.
func TestShadowHard(t *testing.T) {
	for _, tc := range shadowCases {
		c := tc.c
		c.Blur, c.Color = 0, gfx.Black
		shape, ok := shadowShape(c)
		img := image.NewRGBA(image.Rect(0, 0, 140, 140))
		Render(img, build(func(b *gfx.Builder) { b.DrawShadow(c) }), Options{})
		fill := image.NewRGBA(img.Rect)
		if ok {
			Render(fill, build(func(b *gfx.Builder) { b.FillRRect(shape, gfx.Black) }), Options{})
		}
		box := image.NewRGBA(img.Rect)
		Render(box, build(func(b *gfx.Builder) { b.FillRRect(c.RRect, gfx.Black) }), Options{})
		for i := 3; i < len(img.Pix); i += 4 {
			s, bc := fill.Pix[i], box.Pix[i]
			want := mul8(s, 255-bc)
			if c.Inset {
				want = mul8(255-s, bc)
			}
			if d := iabs(int(img.Pix[i]) - int(want)); d > 1 {
				t.Fatalf("%s: pixel %d alpha %d, want %d", tc.name, i/4, img.Pix[i], want)
			}
		}
	}
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
