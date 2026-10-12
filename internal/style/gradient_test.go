package style

import (
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// position returns where p falls along the line of a linear gradient: 0 at
// Start and 1 at End.
func position(g gfx.LinearGradient, p gfx.Point) float32 {
	d := g.End.Sub(g.Start)
	return p.Sub(g.Start).Dot(d) / d.Len2()
}

func TestLinearGradientCorners(t *testing.T) {
	// CSS Images 3, "magic corners": for "to bottom right" the corners
	// top-right and bottom-left are on the 50% line, the other two at 0%
	// and 100%, whatever the aspect ratio.
	for _, size := range [][2]float32{{200, 100}, {100, 200}, {100, 100}, {640, 48}} {
		w, h := size[0], size[1]
		box := gfx.RectXYWH(30, 20, w, h)
		tl, tr := gfx.Pt(box.X0, box.Y0), gfx.Pt(box.X1, box.Y0)
		bl, br := gfx.Pt(box.X0, box.Y1), gfx.Pt(box.X1, box.Y1)
		for _, tc := range []struct {
			corner     Corner
			zero, one  gfx.Point
			mid1, mid2 gfx.Point
		}{
			{CornerBottomRight, tl, br, tr, bl},
			{CornerBottomLeft, tr, bl, tl, br},
			{CornerTopRight, bl, tr, tl, br},
			{CornerTopLeft, br, tl, tr, bl},
		} {
			g := Gradient{Kind: GradientLinear, Corner: tc.corner, Stops: []ColorStop{{Color: gfx.White}, {Color: gfx.Black}}}
			lg := g.Paint(box).(gfx.LinearGradient)
			for _, c := range []struct {
				name string
				p    gfx.Point
				want float32
			}{{"start corner", tc.zero, 0}, {"end corner", tc.one, 1}, {"mid1", tc.mid1, 0.5}, {"mid2", tc.mid2, 0.5}} {
				if got := position(lg, c.p); !near(got, c.want, 1e-4) {
					t.Errorf("%vx%v corner %d %s: position %v, want %v", w, h, tc.corner, c.name, got, c.want)
				}
			}
			if ctr := lg.Start.Lerp(lg.End, 0.5); !near(ctr.X, box.Center().X, 1e-3) || !near(ctr.Y, box.Center().Y, 1e-3) {
				t.Errorf("the gradient line misses the center: %v", ctr)
			}
		}
	}
}

func TestLinearGradientAngles(t *testing.T) {
	box := gfx.RectXYWH(0, 0, 200, 100)
	tests := []struct {
		angle      float32
		start, end gfx.Point
	}{
		// 0deg points up: the line is as long as the height.
		{0, gfx.Pt(100, 100), gfx.Pt(100, 0)},
		{90, gfx.Pt(0, 50), gfx.Pt(200, 50)},
		{180, gfx.Pt(100, 0), gfx.Pt(100, 100)},
		{270, gfx.Pt(200, 50), gfx.Pt(0, 50)},
		// 45deg: |200 sin 45| + |100 cos 45| = 212.13 long.
		{45, gfx.Pt(25, 125), gfx.Pt(175, -25)},
	}
	for _, tt := range tests {
		g := Gradient{Kind: GradientLinear, Angle: tt.angle, Stops: []ColorStop{{Color: gfx.White}, {Color: gfx.Black}}}
		lg := g.Paint(box).(gfx.LinearGradient)
		if !near(lg.Start.X, tt.start.X, 1e-3) || !near(lg.Start.Y, tt.start.Y, 1e-3) ||
			!near(lg.End.X, tt.end.X, 1e-3) || !near(lg.End.Y, tt.end.Y, 1e-3) {
			t.Errorf("%vdeg: line %v -> %v, want %v -> %v", tt.angle, lg.Start, lg.End, tt.start, tt.end)
		}
	}
	// The corner formula and the angle formula agree for a square at 45deg.
	sq := gfx.RectXYWH(0, 0, 100, 100)
	a := (&Gradient{Kind: GradientLinear, Angle: 135, Stops: []ColorStop{{Color: gfx.White}}}).Paint(sq).(gfx.LinearGradient)
	c := (&Gradient{Kind: GradientLinear, Corner: CornerBottomRight, Stops: []ColorStop{{Color: gfx.White}}}).Paint(sq).(gfx.LinearGradient)
	if !near(a.Start.X, c.Start.X, 1e-3) || !near(a.End.Y, c.End.Y, 1e-3) {
		t.Errorf("135deg %v and to bottom right %v differ on a square", a, c)
	}
	// An offset box moves the line.
	off := (&Gradient{Kind: GradientLinear, Angle: 90, Stops: []ColorStop{{Color: gfx.White}}}).Paint(gfx.RectXYWH(50, 10, 200, 100)).(gfx.LinearGradient)
	if off.Start != gfx.Pt(50, 60) || off.End != gfx.Pt(250, 60) {
		t.Errorf("offset line = %v -> %v", off.Start, off.End)
	}
}

func TestGradientStopsAndInterp(t *testing.T) {
	g := Gradient{
		Kind: GradientLinear, Angle: 90, Interp: gfx.InterpSRGB,
		Stops: []ColorStop{
			{Color: gfx.White},
			{Color: gfx.Black, Pos: StopPos{Set: true, LP: LP{Px: 20}}},
			{Color: gfx.White, Pos: StopPos{Set: true, LP: LP{Pct: 50, Px: 10}}},
		},
	}
	lg := g.Paint(gfx.RectXYWH(0, 0, 200, 100)).(gfx.LinearGradient)
	eq(t, "interp", lg.Interp, gfx.InterpSRGB)
	if !(lg.Stops[0].Offset != lg.Stops[0].Offset) {
		t.Errorf("auto stop offset = %v, want NaN", lg.Stops[0].Offset)
	}
	eq(t, "px stop", lg.Stops[1].Offset, 0.1) // 20 px on a 200 px line
	eq(t, "mixed stop", lg.Stops[2].Offset, 0.55)
	// NormalizeStops accepts the result.
	if n := gfx.NormalizeStops(lg.Stops); n[0].Offset != 0 {
		t.Errorf("normalized first offset = %v", n[0].Offset)
	}
	// Default interpolation is Oklab (ADR-023).
	d := (&Gradient{Kind: GradientLinear}).Paint(gfx.RectXYWH(0, 0, 10, 10)).(gfx.LinearGradient)
	eq(t, "default interp", d.Interp, gfx.InterpOklab)
}

func TestRadialGradient(t *testing.T) {
	box := gfx.RectXYWH(10, 20, 200, 100)
	g := newGradient(GradientRadial)
	rg := g.Paint(box).(gfx.RadialGradient)
	eq(t, "center", rg.Center, gfx.Pt(110, 70))
	// farthest-corner ellipse: the closest-side ratio scaled by sqrt 2.
	if !near(rg.Radius.X, float32(100*math.Sqrt2), 1e-3) || !near(rg.Radius.Y, float32(50*math.Sqrt2), 1e-3) {
		t.Errorf("farthest-corner ellipse = %v", rg.Radius)
	}

	cases := []struct {
		name   string
		shape  RadialShape
		size   RadialSize
		center [2]LP
		rx, ry float32
	}{
		{"ellipse closest-side", ShapeEllipse, SizeClosestSide, [2]LP{PctLP(50), PctLP(50)}, 100, 50},
		{"ellipse farthest-side", ShapeEllipse, SizeFarthestSide, [2]LP{PctLP(25), PctLP(50)}, 150, 50},
		{"ellipse closest-corner", ShapeEllipse, SizeClosestCorner, [2]LP{PctLP(25), PctLP(50)}, float32(50 * math.Sqrt2), float32(50 * math.Sqrt2)},
		{"circle closest-side", ShapeCircle, SizeClosestSide, [2]LP{PctLP(50), PctLP(50)}, 50, 50},
		{"circle farthest-side", ShapeCircle, SizeFarthestSide, [2]LP{PctLP(50), PctLP(50)}, 100, 100},
		{"circle closest-corner", ShapeCircle, SizeClosestCorner, [2]LP{PctLP(50), PctLP(50)}, 111.80340, 111.80340},
		{"circle farthest-corner", ShapeCircle, SizeFarthestCorner, [2]LP{PctLP(0), PctLP(0)}, 223.60680, 223.60680},
	}
	for _, c := range cases {
		g := newGradient(GradientRadial)
		g.Shape, g.Size, g.CenterX, g.CenterY = c.shape, c.size, c.center[0], c.center[1]
		rg := g.Paint(box).(gfx.RadialGradient)
		if !near(rg.Radius.X, c.rx, 1e-2) || !near(rg.Radius.Y, c.ry, 1e-2) {
			t.Errorf("%s: radii %v, want (%v, %v)", c.name, rg.Radius, c.rx, c.ry)
		}
	}
	// Explicit radii.
	e := newGradient(GradientRadial)
	e.Size = SizeExplicit
	e.Radius = [2]LP{PxLP(30), PctLP(40)}
	er := e.Paint(box).(gfx.RadialGradient)
	eq(t, "explicit", er.Radius, gfx.Pt(30, 40))
}

func TestConicGradient(t *testing.T) {
	g := newGradient(GradientConic)
	g.Angle = 90
	g.CenterX, g.CenterY = PctLP(25), PxLP(10)
	cg := g.Paint(gfx.RectXYWH(0, 0, 200, 100)).(gfx.ConicGradient)
	eq(t, "center", cg.Center, gfx.Pt(50, 10))
	if !near(cg.StartAngle, float32(math.Pi/2), 1e-6) {
		t.Errorf("StartAngle = %v", cg.StartAngle)
	}
}

func TestGradientFromClassesPaints(t *testing.T) {
	s := style(t, "bg-linear-to-br from-indigo-500 to-fuchsia-600")
	p := s.Backgrounds[0].Gradient.Paint(gfx.RectXYWH(0, 0, 300, 150))
	lg, ok := p.(gfx.LinearGradient)
	if !ok {
		t.Fatalf("Paint = %T", p)
	}
	th := DefaultTheme()
	if lg.Stops[0].Color != th.Colors["indigo-500"] || lg.Stops[1].Color != th.Colors["fuchsia-600"] {
		t.Errorf("stops = %+v", lg.Stops)
	}
	if lg.Stops[0].Offset != 0 || lg.Stops[1].Offset != 1 {
		t.Errorf("offsets = %v, %v", lg.Stops[0].Offset, lg.Stops[1].Offset)
	}
	// The gfx builder accepts the paint.
	b := gfx.NewBuilder()
	b.FillRect(gfx.RectXYWH(0, 0, 300, 150), p)
	if dl := b.Finish(); dl == nil || dl.Len() == 0 {
		t.Error("the gfx builder dropped the gradient fill")
	}
}
