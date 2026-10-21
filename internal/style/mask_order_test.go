package style

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestMaskCascadeClassOrder checks ADR-103: mask classes that set the same
// Tailwind variable follow the class order (the last wins), variants
// included, not the order of Tailwind's stylesheet.
func TestMaskCascadeClassOrder(t *testing.T) {
	// layer returns gradient layer i, failing unless the mask has n layers.
	layer := func(t *testing.T, s *ComputedStyle, n, i int) *Gradient {
		t.Helper()
		if len(s.Mask.Layers) != n {
			t.Fatalf("%d layers, want %d", len(s.Mask.Layers), n)
		}
		return s.Mask.Layers[i].Gradient
	}
	// Edges: four layers (left, right, bottom, top), then the radial and
	// conic ones. Linear: one layer, then the radial and conic ones.
	const edges, linear = 6, 3
	const left, bottom, top, rad, con = 0, 2, 3, 1, 2
	type check func(t *testing.T, s *ComputedStyle)
	fromPos := func(n, i int, want LP) check {
		return func(t *testing.T, s *ComputedStyle) {
			if g := layer(t, s, n, i); g.Stops[0].Pos.LP != want {
				t.Errorf("from position %v, want %v", g.Stops[0].Pos.LP, want)
			}
		}
	}
	toPos := func(n, i int, want LP) check {
		return func(t *testing.T, s *ComputedStyle) {
			if g := layer(t, s, n, i); g.Stops[1].Pos.LP != want {
				t.Errorf("to position %v, want %v", g.Stops[1].Pos.LP, want)
			}
		}
	}
	fromColor := func(n, i int, want gfx.Color) check {
		return func(t *testing.T, s *ComputedStyle) {
			if c := layer(t, s, n, i).Stops[0].Color; c != want {
				t.Errorf("from color %v, want %v", c, want)
			}
		}
	}
	angle := func(n, i int, want float32) check {
		return func(t *testing.T, s *ComputedStyle) {
			if g := layer(t, s, n, i); g.Angle != want {
				t.Errorf("angle %v, want %v", g.Angle, want)
			}
		}
	}
	noMask := func(t *testing.T, s *ComputedStyle) {
		if !s.Mask.IsNone() {
			t.Errorf("want no mask, got %d layers", len(s.Mask.Layers))
		}
	}
	radial := func(f func(g *Gradient) bool) check {
		return func(t *testing.T, s *ComputedStyle) {
			if g := layer(t, s, linear, rad); g.Kind != GradientRadial || !f(g) {
				t.Errorf("radial %+v", g)
			}
		}
	}
	all := func(cs ...check) check {
		return func(t *testing.T, s *ComputedStyle) {
			for _, c := range cs {
				c(t, s)
			}
		}
	}
	shape := func(want RadialShape) check {
		return radial(func(g *Gradient) bool { return g.Shape == want })
	}
	size := func(want RadialSize) check {
		return radial(func(g *Gradient) bool { return g.Size == want })
	}
	center := func(x, y float32) check {
		return radial(func(g *Gradient) bool { return g.CenterX == PctLP(x) && g.CenterY == PctLP(y) })
	}
	red := DefaultTheme().Colors["red-500"]
	cases := []struct {
		classes string
		hover   bool
		want    check
	}{
		// From and to of the same side, and the sides x and y share.
		{"mask-b-from-50% mask-b-from-20%", false, fromPos(edges, bottom, PctLP(20))},
		{"mask-b-from-20% mask-b-from-50%", false, fromPos(edges, bottom, PctLP(50))},
		{"mask-t-to-80% mask-t-to-60%", false, toPos(edges, top, PctLP(60))},
		{"mask-t-to-60% mask-t-to-80%", false, toPos(edges, top, PctLP(80))},
		{"mask-y-from-10% mask-b-from-30%", false, all(fromPos(edges, bottom, PctLP(30)), fromPos(edges, top, PctLP(10)))},
		{"mask-b-from-30% mask-y-from-10%", false, fromPos(edges, bottom, PctLP(10))},
		{"mask-l-from-red-500 mask-l-from-black", false, fromColor(edges, left, gfx.Black)},
		{"mask-l-from-black mask-l-from-red-500", false, fromColor(edges, left, red)},
		// Two linear angles, and the edges against the linear gradient.
		{"mask-linear-45 mask-linear-90 mask-linear-from-10%", false, angle(linear, 0, 90)},
		{"mask-linear-90 mask-linear-45 mask-linear-from-10%", false, angle(linear, 0, 45)},
		{"mask-linear-from-10% mask-linear-90 -mask-linear-45", false, angle(linear, 0, -45)},
		{"mask-linear-from-10% -mask-linear-45 mask-linear-90", false, angle(linear, 0, 90)},
		{"mask-linear-from-20% mask-b-from-50%", false, fromPos(edges, bottom, PctLP(50))},
		{"mask-b-from-50% mask-linear-from-20%", false, fromPos(linear, 0, PctLP(20))},
		{"mask-linear-45 mask-b-from-50%", false, fromPos(edges, bottom, PctLP(50))},
		{"mask-b-from-50% mask-linear-45", false, noMask},
		{"mask-b-from-50% mask-linear-45 mask-linear-to-80%", false, all(angle(linear, 0, 45), toPos(linear, 0, PctLP(80)))},
		{"mask-linear-to-80% mask-b-from-50% mask-linear-45", false, all(angle(linear, 0, 45), toPos(linear, 0, PctLP(80)))},
		// Radial shape, size and position.
		{"mask-radial-from-10% mask-circle mask-ellipse", false, shape(ShapeEllipse)},
		{"mask-radial-from-10% mask-ellipse mask-circle", false, shape(ShapeCircle)},
		{"mask-radial-from-10% mask-radial-closest-side mask-radial-farthest-side", false, size(SizeFarthestSide)},
		{"mask-radial-from-10% mask-radial-farthest-side mask-radial-closest-side", false, size(SizeClosestSide)},
		{"mask-radial-from-10% mask-radial-[40%_60%] mask-radial-closest-corner", false, size(SizeClosestCorner)},
		{"mask-radial-from-10% mask-radial-closest-corner mask-radial-[40%_60%]", false, radial(func(g *Gradient) bool {
			return g.Size == SizeExplicit && g.Radius[1] == PctLP(60)
		})},
		{"mask-radial-[40%_60%] mask-radial-from-10%", false, size(SizeExplicit)},
		{"mask-radial-from-10% mask-radial-at-top-left mask-radial-at-bottom-right", false, center(100, 100)},
		{"mask-radial-from-10% mask-radial-at-bottom-right mask-radial-at-top-left", false, center(0, 0)},
		{"mask-radial-to-60% mask-radial-to-30%", false, toPos(linear, rad, PctLP(30))},
		{"mask-radial-to-30% mask-radial-to-60%", false, toPos(linear, rad, PctLP(60))},
		// Conic from and angle.
		{"mask-conic-from-25% mask-conic-from-75%", false, fromPos(linear, con, PctLP(75))},
		{"mask-conic-from-75% mask-conic-from-25%", false, fromPos(linear, con, PctLP(25))},
		{"mask-conic-from-75% mask-conic-from-[90deg]", false, fromPos(linear, con, PctLP(25))},
		{"mask-conic-from-[90deg] mask-conic-from-75%", false, fromPos(linear, con, PctLP(75))},
		{"mask-conic-from-25% mask-conic-90 mask-conic-180", false, angle(linear, con, 180)},
		{"mask-conic-from-25% mask-conic-180 mask-conic-90", false, angle(linear, con, 90)},
		// Variants: a matching class later in the string beats an earlier
		// one; an earlier one loses to a later plain class.
		{"mask-b-from-50% hover:mask-b-from-20%", true, fromPos(edges, bottom, PctLP(20))},
		{"mask-b-from-50% hover:mask-b-from-20%", false, fromPos(edges, bottom, PctLP(50))},
		{"hover:mask-b-from-20% mask-b-from-50%", true, fromPos(edges, bottom, PctLP(50))},
		{"mask-b-from-50% hover:mask-linear-from-30%", true, fromPos(linear, 0, PctLP(30))},
		{"mask-b-from-50% hover:mask-linear-from-30%", false, fromPos(edges, bottom, PctLP(50))},
		{"hover:mask-linear-from-30% mask-b-from-50%", true, fromPos(edges, bottom, PctLP(50))},
		{"mask-linear-from-10% mask-linear-90 hover:mask-linear-45", true, angle(linear, 0, 45)},
		{"hover:mask-linear-45 mask-linear-90 mask-linear-from-10%", true, angle(linear, 0, 90)},
		{"mask-radial-from-10% mask-circle hover:mask-ellipse", true, shape(ShapeEllipse)},
		{"hover:mask-ellipse mask-circle mask-radial-from-10%", true, shape(ShapeCircle)},
		{"mask-radial-from-10% mask-radial-closest-side hover:mask-radial-[40%_60%]", true, size(SizeExplicit)},
		{"hover:mask-radial-[40%_60%] mask-radial-closest-side mask-radial-from-10%", true, size(SizeClosestSide)},
		{"mask-radial-from-10% mask-radial-at-top-left hover:mask-radial-at-bottom-right", true, center(100, 100)},
		{"hover:mask-radial-at-bottom-right mask-radial-at-top-left mask-radial-from-10%", true, center(0, 0)},
		{"mask-conic-from-25% hover:mask-conic-from-75%", true, fromPos(linear, con, PctLP(75))},
		{"hover:mask-conic-from-75% mask-conic-from-25%", true, fromPos(linear, con, PctLP(25))},
	}
	for _, c := range cases {
		name := c.classes
		if c.hover {
			name = "hover/" + name
		}
		t.Run(name, func(t *testing.T) {
			in := Input{Kind: KindDiv, Classes: []*Classes{mustCompile(t, c.classes)}}
			if c.hover {
				in.State = StateHover
			}
			c.want(t, Compute(testCx, in))
		})
	}
	// An explicit composite still wins over the gradients' intersect in
	// either order (ADR-098, kept by ADR-103).
	for _, cls := range []string{"mask-add mask-t-from-50%", "mask-t-from-50% mask-add"} {
		if s := style(t, cls); s.Mask.Composite != MaskAdd {
			t.Errorf("%s: composite %v, want add", cls, s.Mask.Composite)
		}
	}
}
