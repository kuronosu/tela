package raster

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// supersampledRender renders draw at scale k and box-filters it down to w x h, so
// that each alpha value is the coverage measured on a k x k grid.
func supersampledRender(w, h, k int, draw func(b *gfx.Builder)) *image.RGBA {
	big := coverageImage(w*k, h*k, float32(k), draw)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum := 0
			for j := 0; j < k; j++ {
				for i := 0; i < k; i++ {
					sum += int(big.Pix[big.PixOffset(x*k+i, y*k+j)+3])
				}
			}
			out.Pix[out.PixOffset(x, y)+3] = uint8((sum + k*k/2) / (k * k))
		}
	}
	return out
}

// TestStrokeCoverage compares stroke coverage with a 16x16 supersampled
// render of the same strokes.
//
// The stroke outline self-overlaps (inner sides of curves, inner joins), and
// area accumulation with min(1, |winding area|) overestimates a pixel where
// a winding-2 region meets the outside inside that pixel. With the current
// stroker this shows up only at the concave notch where the inner sides of
// a tight curve meet: one pixel of the "tight" case is 72 levels too dark
// (255 instead of 183). Ordinary curves, joins and caps stay within the
// flattening error (a few levels).
func TestStrokeCoverage(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		draw  func(b *gfx.Builder)
	}{
		{"tight", 80, func(b *gfx.Builder) {
			st := gfx.NewStrokeStyle(12)
			st.Join = gfx.JoinRound
			b.StrokePath(tightCurve(10, 10), st, gfx.Black)
		}},
		{"circle", 12, func(b *gfx.Builder) {
			b.StrokePath(gfx.CirclePath(gfx.Pt(32, 32), 20), gfx.NewStrokeStyle(9), gfx.Black)
		}},
		{"joins and caps", 12, func(b *gfx.Builder) {
			for i, j := range []gfx.LineJoin{gfx.JoinMiter, gfx.JoinRound, gfx.JoinBevel} {
				var p gfx.Path
				x := float32(i) * 30.3
				p.MoveTo(gfx.Pt(x+4, 40))
				p.LineTo(gfx.Pt(x+14, 10.3))
				p.LineTo(gfx.Pt(x+24.6, 40))
				p.LineTo(gfx.Pt(x+30, 50))
				st := gfx.NewStrokeStyle(5)
				st.Join = j
				st.Cap = gfx.LineCap(i)
				b.StrokePath(p, st, gfx.Black)
			}
		}},
	}
	for _, c := range cases {
		got := coverageImage(96, 64, 1, c.draw)
		want := supersampledRender(96, 64, 16, c.draw)
		d, sum, at := alphaDiff(got, want)
		t.Logf("%s: max alpha difference %d at %v, mean %.3f", c.name, d, at, float64(sum)/(96*64))
		if testing.Verbose() {
			for y := max(at.Y-4, 0); y <= at.Y+4; y++ {
				line := ""
				for x := max(at.X-6, 0); x <= at.X+6; x++ {
					line += fmt.Sprintf(" %3d/%3d", got.Pix[got.PixOffset(x, y)+3], want.Pix[want.PixOffset(x, y)+3])
				}
				t.Log(line)
			}
		}
		if d > c.limit {
			t.Errorf("%s: stroke coverage differs from the supersampled reference by %d at %v", c.name, d, at)
		}
	}
}
