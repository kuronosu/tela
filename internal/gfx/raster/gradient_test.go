package raster

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// gradientRefs are premultiplied RGBA8 colors at t = 0.1, 0.25, 0.5, 0.75
// and 0.9 of two-stop gradients, generated with color.js v0.7.1 (the CSS
// Color 4 reference implementation) under node, outside the repository:
//
//	c = a.mix(b, t, {space, premultiplied: true, outputSpace: "srgb"})
//	pixel = round(clamp01(c.coords[i]) * c.alpha * 255), round(c.alpha * 255)
//
// with a and b created as new Color("srgb", [r/255, g/255, b/255], a/255).
// No color.js code is used by Tela; see THIRD_PARTY.md.
var gradientRefs = []struct {
	name  string
	a, b  [4]uint8 // non-premultiplied RGBA8 stops
	space gfx.ColorSpace
	want  [5][4]uint8
}{
	{"indigo500_fuchsia600_oklab", [4]uint8{99, 102, 241, 255}, [4]uint8{192, 38, 211, 255}, gfx.InterpOklab, [5][4]uint8{{111, 98, 238, 255}, {128, 93, 233, 255}, {151, 80, 226, 255}, {173, 64, 218, 255}, {184, 51, 214, 255}}},
	{"blue_yellow_oklab", [4]uint8{0, 0, 255, 255}, [4]uint8{255, 255, 0, 255}, gfx.InterpOklab, [5][4]uint8{{0, 74, 246, 255}, {29, 119, 231, 255}, {108, 171, 199, 255}, {181, 215, 151, 255}, {225, 239, 101, 255}}},
	{"blue_yellow_srgb", [4]uint8{0, 0, 255, 255}, [4]uint8{255, 255, 0, 255}, gfx.InterpSRGB, [5][4]uint8{{26, 26, 230, 255}, {64, 64, 191, 255}, {128, 128, 128, 255}, {191, 191, 64, 255}, {230, 230, 25, 255}}},
	{"sky500_transparent_oklab", [4]uint8{14, 165, 233, 255}, [4]uint8{0, 0, 0, 0}, gfx.InterpOklab, [5][4]uint8{{13, 149, 210, 230}, {10, 124, 175, 191}, {7, 83, 116, 128}, {3, 41, 58, 64}, {1, 16, 23, 25}}},
	{"emerald_rose40_oklab", [4]uint8{16, 185, 129, 255}, [4]uint8{244, 63, 94, 102}, gfx.InterpOklab, [5][4]uint8{{55, 171, 120, 240}, {79, 149, 106, 217}, {97, 113, 83, 179}, {102, 75, 60, 140}, {101, 48, 47, 117}}},
	{"emerald_rose40_srgb", [4]uint8{16, 185, 129, 255}, [4]uint8{244, 63, 94, 102}, gfx.InterpSRGB, [5][4]uint8{{24, 169, 120, 240}, {36, 145, 106, 217}, {57, 105, 83, 179}, {77, 65, 60, 140}, {89, 41, 47, 117}}},
	{"black_white_oklab", [4]uint8{0, 0, 0, 255}, [4]uint8{255, 255, 255, 255}, gfx.InterpOklab, [5][4]uint8{{3, 3, 3, 255}, {34, 34, 34, 255}, {99, 99, 99, 255}, {174, 174, 174, 255}, {222, 222, 222, 255}}},
}

func near(a, b [4]uint8, tol int) bool {
	for i := range a {
		if d := int(a[i]) - int(b[i]); d > tol || d < -tol {
			return false
		}
	}
	return true
}

func rgba(c color.RGBA) [4]uint8 { return [4]uint8{c.R, c.G, c.B, c.A} }

// renderTransparent renders draw onto a transparent w x h image, so that the
// pixels are the premultiplied paint colors.
func renderTransparent(w, h int, scale float32, draw func(b *gfx.Builder)) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	Render(img, build(draw), Options{Scale: scale})
	return img
}

func TestGradientMatchesColorJS(t *testing.T) {
	ts := []float64{0.1, 0.25, 0.5, 0.75, 0.9}
	for _, c := range gradientRefs {
		// Pixel x has its center at x + 0.5, so t = x/100.
		img := renderTransparent(101, 1, 1, func(b *gfx.Builder) {
			b.FillRect(gfx.Rect{X1: 101, Y1: 1}, gfx.LinearGradient{
				Start: gfx.Pt(0.5, 0), End: gfx.Pt(100.5, 0), Interp: c.space,
				Stops: []gfx.GradientStop{
					{Offset: 0, Color: gfx.RGBA8(c.a[0], c.a[1], c.a[2], c.a[3])},
					{Offset: 1, Color: gfx.RGBA8(c.b[0], c.b[1], c.b[2], c.b[3])},
				}})
		})
		for i, tv := range ts {
			x := int(tv*100 + 0.5)
			if got := rgba(img.RGBAAt(x, 0)); !near(got, c.want[i], 1) {
				t.Errorf("%s at t=%v: got %v, color.js %v", c.name, tv, got, c.want[i])
			}
		}
		// The ends are the stop colors exactly.
		if got, want := rgba(img.RGBAAt(0, 0)), premul8(gfx.RGBA8(c.a[0], c.a[1], c.a[2], c.a[3])); got != want {
			t.Errorf("%s at t=0: got %v, want %v", c.name, got, want)
		}
		if got, want := rgba(img.RGBAAt(100, 0)), premul8(gfx.RGBA8(c.b[0], c.b[1], c.b[2], c.b[3])); got != want {
			t.Errorf("%s at t=1: got %v, want %v", c.name, got, want)
		}
	}
}

// Hard stops are exact: the color changes between two neighboring pixels at
// the stop, wherever it falls, with no blending, and pad colors apply
// outside the gradient line.
func TestGradientHardStops(t *testing.T) {
	red, green, blue := gfx.RGB8(255, 0, 0), gfx.RGB8(0, 255, 0), gfx.RGB8(0, 0, 255)
	for _, o := range []float32{0.3, 1.0 / 3, 0.5, 0.71} {
		img := renderTransparent(300, 1, 1, func(b *gfx.Builder) {
			b.FillRect(gfx.Rect{X1: 300, Y1: 1}, gfx.LinearGradient{
				Start: gfx.Pt(50, 0), End: gfx.Pt(250, 0), Stops: []gfx.GradientStop{
					{Offset: 0, Color: red}, {Offset: o, Color: red}, {Offset: o, Color: green},
					{Offset: o, Color: blue}, {Offset: 1, Color: blue}}})
		})
		stopX := 50 + 200*float64(o) // the boundary, in pixels
		for x := 0; x < 300; x++ {
			want := premul8(blue)
			if float64(x)+0.5 < stopX {
				want = premul8(red)
			}
			if got := rgba(img.RGBAAt(x, 0)); got != want {
				t.Fatalf("hard stop at %v: pixel %d = %v, want %v", o, x, got, want)
			}
		}
	}
}

// The parameter t of radial and conic gradients follows the CSS geometry.
func TestGradientGeometry(t *testing.T) {
	black, white := gfx.RGB8(0, 0, 0), gfx.RGB8(255, 255, 255)
	bw := []gfx.GradientStop{{Offset: 0, Color: black}, {Offset: 1, Color: white}}
	// Radial with radii (40, 20) at (50, 25), in sRGB so that the gray level
	// is 255*t.
	img := renderTransparent(100, 50, 1, func(b *gfx.Builder) {
		b.FillRect(gfx.Rect{X1: 100, Y1: 50}, gfx.RadialGradient{Center: gfx.Pt(50, 25), Radius: gfx.Pt(40, 20), Stops: bw, Interp: gfx.InterpSRGB})
	})
	for _, p := range []image.Point{{60, 30}, {80, 25}, {50, 40}, {20, 10}, {95, 45}} {
		dx, dy := (float64(p.X)+0.5-50)/40, (float64(p.Y)+0.5-25)/20
		tv := math.Min(math.Hypot(dx, dy), 1)
		if got, want := int(img.RGBAAt(p.X, p.Y).R), int(math.Floor(255*tv+0.5)); got < want-1 || got > want+1 {
			t.Errorf("radial at %v: gray %d, want %d", p, got, want)
		}
	}
	// Conic quadrants with hard stops: from up, clockwise.
	q := []gfx.Color{gfx.RGB8(255, 0, 0), gfx.RGB8(0, 255, 0), gfx.RGB8(0, 0, 255), gfx.RGB8(255, 255, 0)}
	var st []gfx.GradientStop
	for i, c := range q {
		st = append(st, gfx.GradientStop{Offset: float32(i) / 4, Color: c}, gfx.GradientStop{Offset: float32(i+1) / 4, Color: c})
	}
	for _, start := range []float32{0, math.Pi / 2} {
		img = renderTransparent(40, 40, 1, func(b *gfx.Builder) {
			b.FillRect(gfx.Rect{X1: 40, Y1: 40}, gfx.ConicGradient{Center: gfx.Pt(20, 20), StartAngle: start, Stops: st})
		})
		shift := 0
		if start != 0 {
			shift = 1 // a quarter turn later: the first quadrant starts at 3 o'clock
		}
		for i, p := range []image.Point{{30, 5}, {30, 30}, {5, 30}, {5, 5}} { // up-right, down-right, down-left, up-left
			want := premul8(q[(i-shift+4)%4])
			if got := rgba(img.RGBAAt(p.X, p.Y)); got != want {
				t.Errorf("conic start %v at %v: %v, want %v", start, p, got, want)
			}
		}
	}
	// A linear gradient under a 90 degree rotation varies along y.
	img = renderTransparent(10, 100, 1, func(b *gfx.Builder) {
		b.PushTransform(gfx.Translate(10, 0).Mul(gfx.Rotate(math.Pi / 2)))
		b.FillRect(gfx.Rect{X1: 100, Y1: 10}, gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(100, 0), Stops: bw, Interp: gfx.InterpSRGB})
		b.PopTransform()
	})
	if a, b2 := img.RGBAAt(5, 10).R, img.RGBAAt(5, 90).R; !(a < 30 && b2 > 225) || img.RGBAAt(0, 50) != img.RGBAAt(9, 50) {
		t.Errorf("rotated gradient: %d at the top, %d at the bottom", a, b2)
	}
}

// Degenerate gradients paint their last stop (gfx's documented behavior);
// a single stop paints that color.
func TestGradientDegenerate(t *testing.T) {
	last := gfx.RGB8(10, 200, 30)
	st := []gfx.GradientStop{{Offset: 0, Color: gfx.RGB8(200, 0, 0)}, {Offset: 1, Color: last}}
	for _, p := range []gfx.Paint{
		gfx.LinearGradient{Start: gfx.Pt(3, 3), End: gfx.Pt(3, 3), Stops: st},
		gfx.RadialGradient{Center: gfx.Pt(3, 3), Radius: gfx.Pt(0, 5), Stops: st},
		gfx.ConicGradient{Center: gfx.Pt(3, 3), Stops: []gfx.GradientStop{{Offset: 0.5, Color: last}}},
	} {
		img := renderTransparent(4, 4, 1, func(b *gfx.Builder) { b.FillRect(gfx.Rect{X1: 4, Y1: 4}, p) })
		for _, c := range []color.RGBA{img.RGBAAt(0, 0), img.RGBAAt(3, 3)} {
			if rgba(c) != premul8(last) {
				t.Errorf("%T: %v, want %v", p, c, premul8(last))
			}
		}
	}
}

// Ramps are cached: drawing the same gradient again does not rebuild it.
func TestGradientRampCache(t *testing.T) {
	r := NewRenderer()
	st := gfx.NormalizeStops([]gfx.GradientStop{{Offset: 0, Color: gfx.Black}, {Offset: 1, Color: gfx.White}})
	a := r.ramps.get(st, gfx.InterpOklab)
	if b := r.ramps.get(append([]gfx.GradientStop(nil), st...), gfx.InterpOklab); a != b {
		t.Error("equal stops built a second ramp")
	}
	if b := r.ramps.get(st, gfx.InterpSRGB); a == b {
		t.Error("the color space is not part of the cache key")
	}
}

// Out-of-gamut stops (Tailwind colors in extended sRGB) are interpolated
// unclipped and each pixel is clipped, in both color spaces. The wanted
// colors are Chromium 152's pixels at x = 32, 96, 160 and 224 of a 256 px
// wide "linear-gradient(to right in <space>, oklch(...), oklch(...))";
// clipping the stops first would be off by up to 47 levels.
func TestGradientExtendedStops(t *testing.T) {
	cyan500 := gfx.OKLCH(0.715, 0.143, 215.221, 1)
	blue500 := gfx.OKLCH(0.623, 0.214, 259.815, 1)
	green500 := gfx.OKLCH(0.723, 0.219, 149.579, 1)
	fuchsia600 := gfx.OKLCH(0.591, 0.293, 322.896, 1)
	tests := []struct {
		name  string
		a, b  gfx.Color
		space gfx.ColorSpace
		want  [4][3]uint8
	}{
		{"cyan-blue srgb", cyan500, blue500, gfx.InterpSRGB, [4][3]uint8{{0, 177, 224}, {0, 163, 235}, {5, 148, 245}, {31, 134, 255}}},
		{"cyan-blue oklab", cyan500, blue500, gfx.InterpOklab, [4][3]uint8{{0, 178, 225}, {0, 165, 236}, {0, 151, 246}, {34, 135, 255}}},
		{"green-fuchsia srgb", green500, fuchsia600, gfx.InterpSRGB, [4][3]uint8{{0, 172, 98}, {44, 114, 134}, {107, 57, 169}, {169, 0, 205}}},
		{"green-fuchsia oklab", green500, fuchsia600, gfx.InterpOklab, [4][3]uint8{{70, 187, 109}, {132, 157, 149}, {166, 120, 181}, {190, 64, 209}}},
	}
	for _, tt := range tests {
		img := image.NewRGBA(image.Rect(0, 0, 256, 1))
		Render(img, build(func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 256, 1), gfx.LinearGradient{
				End: gfx.Pt(256, 0), Interp: tt.space,
				Stops: []gfx.GradientStop{gfx.AutoStop(tt.a), gfx.AutoStop(tt.b)},
			})
		}), Options{Scale: 1})
		for i, x := range []int{32, 96, 160, 224} {
			got := img.RGBAAt(x, 0)
			for k, v := range [3]uint8{got.R, got.G, got.B} {
				if d := int(v) - int(tt.want[i][k]); d > 1 || d < -1 {
					t.Errorf("%s at x=%d: got %v, Chromium %v", tt.name, x, got, tt.want[i])
					break
				}
			}
		}
	}
}
