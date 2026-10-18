package text

import (
	"image"
	"math"
	"testing"

	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

var (
	red  = tables.ColorRecord{Red: 255, Alpha: 255}
	blue = tables.ColorRecord{Blue: 255, Alpha: 255}
)

// newTestPainter returns a painter over Inter's outlines with a two-color
// palette (0: red, 1: blue), an empty COLR table and a cover of [-1, 1]²
// em, recording into a builder that maps em space to a 200x200 image
// centered at (100, 100).
func newTestPainter(t *testing.T) *painter {
	t.Helper()
	f := NewDefaultCollection().Match("Inter", Query{}).Face(Instance{})
	p := &painter{
		f: f, colr: &tables.COLR1{}, cpal: []tables.ColorRecord{red, blue},
		b: gfx.NewBuilder(), fg: gfx.RGB8(0, 200, 0), ctm: gfx.Identity,
		cover: gfx.Rect{X0: -1, Y0: -1, X1: 1, Y1: 1}, active: map[tables.GlyphID]bool{},
	}
	p.b.PushTransform(gfx.Translate(100, 100).Mul(gfx.Scale(100, 100)))
	return p
}

// render finishes the painter's list and rasterizes it.
func (p *painter) render() *image.RGBA {
	p.b.PopTransform()
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, p.b.Finish(), raster.Options{})
	return img
}

// at returns the pixel at font-space polar coordinates (angle in degrees,
// counter-clockwise from +x; radius in em) around the image center.
func at(img *image.RGBA, deg, r float64) (red, blue uint8) {
	s, c := math.Sincos(deg * math.Pi / 180)
	x, y := int(math.Round(100+100*r*c)), int(math.Round(100-100*r*s))
	i := img.PixOffset(x, y)
	return img.Pix[i], img.Pix[i+2]
}

func redBlueLine(ext tables.Extend) tables.ColorLine {
	return tables.ColorLine{Extend: ext, ColorStops: []tables.ColorStop{
		{StopOffset: 0, PaletteIndex: 0, Alpha: 1 << 14},
		{StopOffset: 1 << 14, PaletteIndex: 1, Alpha: 1 << 14},
	}}
}

// TestSweepOrientation checks the angle conventions of PaintSweepGradient:
// counter-clockwise in font space, angles biased by 1.0 (OpenType 1.9.1),
// t computed from angles in [0°, 360°) and padded outside the range.
func TestSweepOrientation(t *testing.T) {
	p := newTestPainter(t)
	// start = (-1 + 1) * 180 = 0°, end = (-0.5 + 1) * 180 = 90°.
	p.paint(tables.PaintSweepGradient{ColorLine: redBlueLine(tables.ExtendPad), StartAngle: -1 << 14, EndAngle: -1 << 13})
	img := p.render()
	for _, c := range []struct {
		deg     float64
		redSide bool
	}{
		{3, true}, {20, true}, {70, false}, {87, false}, // inside the range
		{135, false}, {250, false}, {357, false}, // t > 1: the last color
	} {
		r, b := at(img, c.deg, 0.6)
		if (r > b) != c.redSide {
			t.Errorf("sweep at %g°: red %d, blue %d; want the %s side", c.deg, r, b, map[bool]string{true: "red", false: "blue"}[c.redSide])
		}
	}
	// t = 0.5 at 45°: the premultiplied sRGB midpoint.
	if r, b := at(img, 45, 0.6); absDiff(r, 128) > 8 || absDiff(b, 128) > 8 {
		t.Errorf("sweep at 45°: red %d, blue %d, want about 128 each", r, b)
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestLinearGradientP2 checks the rotation point: with p2 straight above
// p0, the gradient runs horizontally from p0 to the projection of p1.
func TestLinearGradientP2(t *testing.T) {
	p := newTestPainter(t)
	paint := p.simplePaint(tables.PaintLinearGradient{
		ColorLine: redBlueLine(tables.ExtendPad), X0: 0, Y0: 0, X1: 1024, Y1: 1024, X2: 0, Y2: 512,
	}, p.cover)
	g, ok := paint.(gfx.LinearGradient)
	if !ok {
		t.Fatalf("got %T", paint)
	}
	if g.Start != (gfx.Point{}) || g.End != (gfx.Point{X: 0.5}) || g.Interp != gfx.InterpSRGB {
		t.Errorf("gradient line %v -> %v (%v), want (0, 0) -> (0.5, 0) in srgb", g.Start, g.End, g.Interp)
	}
	if len(g.Stops) != 2 || g.Stops[0].Offset != 0 || g.Stops[1].Offset != 1 {
		t.Errorf("stops %v", g.Stops)
	}
	// Degenerate: p0 == p2.
	if paint := p.simplePaint(tables.PaintLinearGradient{ColorLine: redBlueLine(tables.ExtendPad), X1: 100}, p.cover); paint != nil {
		t.Errorf("degenerate gradient painted %v", paint)
	}
}

// TestRadialConcentric checks the mapping of a concentric COLR gradient
// with a non-zero start radius onto gfx's radial gradient.
func TestRadialConcentric(t *testing.T) {
	p := newTestPainter(t)
	paint := p.simplePaint(tables.PaintRadialGradient{
		ColorLine: redBlueLine(tables.ExtendPad), X0: 1024, Y0: 0, Radius0: 256, X1: 1024, Y1: 0, Radius1: 1024,
	}, p.cover)
	g, ok := paint.(gfx.RadialGradient)
	if !ok {
		t.Fatalf("got %T", paint)
	}
	if g.Center != (gfx.Point{X: 0.5}) || g.Radius != (gfx.Point{X: 0.5, Y: 0.5}) {
		t.Errorf("center %v radius %v", g.Center, g.Radius)
	}
	// Offsets are distances over the radius: the red stop sits at r0.
	want := []float32{0, 0.25, 1}
	if len(g.Stops) != 3 || g.Stops[0].Offset != want[0] || g.Stops[1].Offset != want[1] || g.Stops[2].Offset != want[2] {
		t.Errorf("stops %v, want offsets %v", g.Stops, want)
	}
	// Two-point gradients become focal gradients between the circles at
	// the ends of the color line.
	fg, ok := p.simplePaint(tables.PaintRadialGradient{
		ColorLine: redBlueLine(tables.ExtendPad), X0: 100, Radius1: 1024,
	}, p.cover).(gfx.FocalGradient)
	if !ok || fg.Focal != (gfx.Point{X: 100.0 / 2048}) || fg.FocalRadius != 0 || fg.Center != (gfx.Point{}) || fg.Radius != 0.5 {
		t.Errorf("two-point gradient: %#v", fg)
	}
}

// TestTwoPointRanges checks the circles chosen for color lines that reach
// beyond the circles of radius 0, and for repeating lines.
func TestTwoPointRanges(t *testing.T) {
	p := newTestPainter(t)
	line := func(ext tables.Extend, t0, t1 float64) tables.ColorLine {
		return tables.ColorLine{Extend: ext, ColorStops: []tables.ColorStop{
			{StopOffset: tables.Coord(t0 * 16384), PaletteIndex: 0, Alpha: 1 << 14},
			{StopOffset: tables.Coord(t1 * 16384), PaletteIndex: 1, Alpha: 1 << 14},
		}}
	}
	// Growing circles, the line starting at t = -1 where r < 0: the focal
	// circle is moved to the circle of radius 0 (t = -0.5).
	g := p.simplePaint(tables.PaintRadialGradient{ColorLine: line(tables.ExtendPad, -1, 1),
		X0: 0, Radius0: 512, X1: 1024, Radius1: 1536}, p.cover).(gfx.FocalGradient)
	if g.FocalRadius != 0 || g.Focal != (gfx.Point{X: -0.25}) || g.Radius != 0.75 || g.Stops[0].Color.R != 0.75 {
		t.Errorf("negative start radius: %#v", g)
	}
	// Shrinking circles that reach radius 0 before the line's end.
	g = p.simplePaint(tables.PaintRadialGradient{ColorLine: line(tables.ExtendPad, 0, 1.5),
		X0: 0, Radius0: 1024, X1: 512, Radius1: 0}, p.cover).(gfx.FocalGradient)
	if g.Radius != 0 || g.Center != (gfx.Point{X: 0.25}) || g.FocalRadius != 0.5 {
		t.Errorf("shrinking to radius 0: %#v", g)
	}
	// Repeat: unrolled over the range of t of the cover area.
	g = p.simplePaint(tables.PaintRadialGradient{ColorLine: line(tables.ExtendRepeat, 0, 0.25),
		X0: 300, Radius0: 0, X1: 0, Radius1: 1024}, p.cover).(gfx.FocalGradient)
	if len(g.Stops) < 8 {
		t.Errorf("repeat: %d stops, want the line repeated over the cover", len(g.Stops))
	}
}

// TestTwoPointRadial renders a focal gradient: red at the focal point,
// blue on the end circle, and checks colors along the axis.
func TestTwoPointRadial(t *testing.T) {
	p := newTestPainter(t)
	// Focal point at x = -0.5 em, end circle centered at 0 with radius 1 em.
	p.paint(tables.PaintRadialGradient{ColorLine: redBlueLine(tables.ExtendPad), X0: -1024, Radius0: 0, X1: 0, Radius1: 2048})
	img := p.render()
	// On the x axis, a point at x lies on the circle of t where
	// |x - c(t)| = r(t): left of the focus x = -0.5 - 0.5t, right of it
	// x = -0.5 + 1.5t.
	for _, c := range []struct{ x, t float64 }{{-0.45, 1.0 / 30}, {0.25, 0.5}, {0.85, 0.9}, {-0.75, 0.5}} {
		i := img.PixOffset(int(math.Round(100+100*c.x)), 100)
		r, b := img.Pix[i], img.Pix[i+2]
		wantB := 255 * c.t
		if math.Abs(float64(b)-wantB) > 8 || math.Abs(float64(r)-(255-wantB)) > 8 {
			t.Errorf("x = %g: red %d, blue %d, want t = %.2f (blue %.0f)", c.x, r, b, c.t, wantB)
		}
	}
}

func TestSampleColorLine(t *testing.T) {
	r, b := gfx.RGBA8(255, 0, 0, 255), gfx.RGBA8(0, 0, 255, 255)
	line := []cstop{{0, r}, {1, b}}
	offsets := func(s []gfx.GradientStop) (o []float32) {
		for _, st := range s {
			o = append(o, st.Offset)
		}
		return o
	}
	rep := sampleColorLine(line, tables.ExtendRepeat, 0, 2)
	if got := offsets(rep); len(got) != 4 || got[1] != 0.5 || got[2] != 0.5 || rep[1].Color != b || rep[2].Color != r {
		t.Errorf("repeat: %v", rep)
	}
	ref := sampleColorLine(line, tables.ExtendReflect, 0, 2)
	if len(ref) < 3 || ref[1].Offset != 0.5 || ref[1].Color != b || ref[len(ref)-1].Color != r || ref[len(ref)-1].Offset != 1 {
		t.Errorf("reflect: %v", ref)
	}
	pad := sampleColorLine(line, tables.ExtendPad, -1, 1)
	if len(pad) != 3 || pad[0].Color != r || pad[1].Offset != 0.5 {
		t.Errorf("pad: %v", pad)
	}
	// Reversed direction.
	rev := sampleColorLine(line, tables.ExtendPad, 1, 0)
	if rev[0].Color != b || rev[len(rev)-1].Color != r {
		t.Errorf("reversed: %v", rev)
	}
	// Premultiplied interpolation: halfway to transparent keeps the color.
	if c := evalLine([]cstop{{0, r}, {1, gfx.Transparent}}, 0.5); c.R != 1 || c.A != 0.5 {
		t.Errorf("premultiplied midpoint %v", c)
	}
}

func commands(l *gfx.DisplayList) []string {
	var out []string
	for _, c := range l.All() {
		out = append(out, c.Name())
	}
	return out
}

func glyphPaint(t *testing.T, p *painter, r rune, child tables.PaintTable) tables.PaintGlyph {
	g, ok := p.f.GlyphIndex(r)
	if !ok {
		t.Fatalf("no glyph for %q", r)
	}
	return tables.PaintGlyph{GlyphID: uint16(g), Paint: child}
}

// TestPaintStructure checks how paint nodes become commands.
func TestPaintStructure(t *testing.T) {
	solid := func(idx uint16, alpha float64) tables.PaintSolid {
		return tables.PaintSolid{PaletteIndex: idx, Alpha: tables.Coord(alpha * 16384)}
	}
	cases := []struct {
		name  string
		paint func(p *painter) tables.PaintTable
		want  []string
	}{
		{"glyph+solid", func(p *painter) tables.PaintTable { return glyphPaint(t, p, 'O', solid(0, 1)) },
			[]string{"PushTransform", "FillPath", "PopTransform"}},
		{"glyph+transformed gradient", func(p *painter) tables.PaintTable {
			return glyphPaint(t, p, 'O', tables.PaintScale{ScaleX: 1 << 13, ScaleY: 1 << 14, Paint: tables.PaintLinearGradient{
				ColorLine: redBlueLine(tables.ExtendPad), X1: 100, Y2: 100}})
		}, []string{"PushTransform", "PushTransform", "FillPath", "PopTransform", "PopTransform"}},
		{"glyph+layers", func(p *painter) tables.PaintTable {
			return glyphPaint(t, p, 'O', tables.PaintComposite{CompositeMode: tables.CompositeSrcOver, SourcePaint: solid(0, 1), BackdropPaint: solid(1, 1)})
		}, []string{"PushTransform", "PushClipPath", "FillRect", "FillRect", "PopClip", "PopTransform"}},
		{"colrv0", func(p *painter) tables.PaintTable {
			o, _ := p.f.GlyphIndex('O')
			return tables.PaintColrLayersResolved{{GlyphID: tables.GlyphID(o), PaletteIndex: 0}, {GlyphID: tables.GlyphID(o), PaletteIndex: 1}}
		}, []string{"PushTransform", "FillPath", "FillPath", "PopTransform"}},
		{"src-in solid mask", func(p *painter) tables.PaintTable {
			return tables.PaintComposite{CompositeMode: tables.CompositeSrcIn, SourcePaint: glyphPaint(t, p, 'O', solid(0, 1)), BackdropPaint: solid(1, 0.5)}
		}, []string{"PushTransform", "PushLayer", "FillPath", "PopLayer", "PopTransform"}},
		{"src-in glyph mask", func(p *painter) tables.PaintTable {
			return tables.PaintComposite{CompositeMode: tables.CompositeSrcIn, SourcePaint: solid(0, 1), BackdropPaint: glyphPaint(t, p, 'H', solid(1, 1))}
		}, []string{"PushTransform", "PushLayer", "FillRect", "PopLayer", "PopTransform"}},
		{"dest-over", func(p *painter) tables.PaintTable {
			return tables.PaintComposite{CompositeMode: tables.CompositeDestOver, SourcePaint: glyphPaint(t, p, 'O', solid(0, 1)), BackdropPaint: solid(1, 1)}
		}, []string{"PushTransform", "FillPath", "FillRect", "PopTransform"}},
		{"clear", func(p *painter) tables.PaintTable {
			return tables.PaintComposite{CompositeMode: tables.CompositeClear, SourcePaint: solid(0, 1), BackdropPaint: solid(1, 1)}
		}, nil}, // the Builder drops empty transform blocks
		{"singular transform", func(p *painter) tables.PaintTable {
			return tables.PaintScale{ScaleX: 0, ScaleY: 1 << 14, Paint: solid(0, 1)}
		}, nil},
	}
	for _, c := range cases {
		p := newTestPainter(t)
		p.paint(c.paint(p))
		p.b.PopTransform()
		got := commands(p.b.Finish())
		if len(got) != len(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

// TestCompositeBlend checks the layer structure of blend composites: an
// isolated group with the backdrop and a layer with the blend mode.
func TestCompositeBlend(t *testing.T) {
	p := newTestPainter(t)
	p.paint(tables.PaintComposite{CompositeMode: tables.CompositeSoftLight,
		SourcePaint:   tables.PaintSolid{PaletteIndex: 0, Alpha: 1 << 14},
		BackdropPaint: glyphPaint(t, p, 'O', tables.PaintSolid{PaletteIndex: 1, Alpha: 1 << 14})})
	p.b.PopTransform()
	l := p.b.Finish()
	got := commands(l)
	want := []string{"PushTransform", "PushLayer", "FillPath", "PushLayer", "FillRect", "PopLayer", "PopLayer", "PopTransform"}
	if len(got) != len(want) {
		t.Fatalf("%v, want %v", got, want)
	}
	if outer, inner := l.At(1).(gfx.PushLayer), l.At(3).(gfx.PushLayer); outer.Blend != gfx.SrcOver || inner.Blend != gfx.SoftLight {
		t.Errorf("layers %v and %v, want an isolated group and a soft-light layer", outer, inner)
	}
}

// TestCompositeSrcInMask checks that SrcIn with a gradient mask multiplies
// the source by the mask's alpha: a red fill masked by a glyph filled with
// a gradient from opaque to transparent.
func TestCompositeSrcInMask(t *testing.T) {
	p := newTestPainter(t)
	fade := tables.ColorLine{ColorStops: []tables.ColorStop{
		{StopOffset: 0, PaletteIndex: 1, Alpha: 1 << 14},
		{StopOffset: 1 << 14, PaletteIndex: 1, Alpha: 0},
	}}
	o, _ := p.f.GlyphIndex('H')
	p.paint(tables.PaintComposite{CompositeMode: tables.CompositeSrcIn,
		SourcePaint: tables.PaintSolid{PaletteIndex: 0, Alpha: 1 << 14},
		// The mask: 'H' filled from x = 0 (opaque) to x = 2048 units (1 em, transparent).
		BackdropPaint: tables.PaintGlyph{GlyphID: uint16(o), Paint: tables.PaintLinearGradient{ColorLine: fade, X1: 2048, Y2: 2048}}})
	l := p.b
	l.PopTransform()
	dl := l.Finish()
	layer, ok := dl.At(1).(gfx.PushLayer)
	if !ok || layer.Mask.List == nil || layer.Mask.Mode != gfx.MaskAlpha {
		t.Fatalf("command 1 is %v, want a layer with an alpha mask", dl.At(1))
	}
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, dl, raster.Options{})
	// Inside the left stem of 'H' (x about 0.15 em) the red is nearly
	// opaque; outside the glyph nothing is painted.
	var stem, out [4]uint8
	for x := 100; x < 160; x++ {
		i := img.PixOffset(x, 70)
		if img.Pix[i+1] < 100 { // red, not white
			copy(stem[:], img.Pix[i:i+4])
			break
		}
	}
	i := img.PixOffset(195, 195)
	copy(out[:], img.Pix[i:i+4])
	if stem[0] != 255 || stem[1] > 80 || out != [4]uint8{255, 255, 255, 255} {
		t.Errorf("stem %v (want red, mostly opaque), outside %v (want white)", stem, out)
	}
}

// TestForeground checks palette index 0xFFFF and the per-foreground cache.
func TestForeground(t *testing.T) {
	p := newTestPainter(t)
	c := p.paletteColor(0xFFFF, 0.5)
	if !p.usesFG || c != (gfx.Color{G: 200.0 / 255, A: 0.5}) {
		t.Errorf("foreground color %v (usesFG %v)", c, p.usesFG)
	}
	if c := p.paletteColor(7, 1); c != gfx.Transparent {
		t.Errorf("out-of-range palette index: %v", c)
	}

	// The emoji font does not use the foreground: one list for every fg.
	f := NewDefaultCollection().Face(Query{Families: []string{"emoji"}}, 64)
	g, _ := f.GlyphIndex('😀')
	a, b := f.ColorGlyph(g, gfx.Black), f.ColorGlyph(g, gfx.White)
	if a == nil || a != b {
		t.Errorf("ColorGlyph lists for two foregrounds: %p, %p (want one shared list)", a, b)
	}
	if r := f.GlyphBounds(g); r.Empty() || r.X0 > 0.1 || r.X1 < 1 || r.Y0 > -0.8 || r.Y1 < 0.1 {
		t.Errorf("😀 bounds %v: want the clip box around the em square", r)
	}
	if l := a.Bounds(); !r(f.GlyphBounds(g)).ContainsRect(l) {
		t.Errorf("😀 list bounds %v exceed its glyph bounds %v", l, f.GlyphBounds(g))
	}
}

func r(x gfx.Rect) gfx.Rect { return x.Outset(1e-6) }

// TestRotateExact checks that quarter turns are exact and that rotations
// are counter-clockwise in font space (and on screen).
func TestRotateExact(t *testing.T) {
	p := newTestPainter(t)
	if got := p.rotate(0.5); got != (gfx.Affine{0, -1, 1, 0, 0, 0}) {
		t.Errorf("rotate 90° = %v", got)
	}
	if got := p.rotate(-1); got != (gfx.Affine{-1, 0, 0, -1, 0, 0}) && got != (gfx.Affine{-1, -0, -0, -1, 0, 0}) {
		t.Errorf("rotate -180° = %v", got)
	}
	// In em space (y down), font-space +x rotated by 90° points up.
	if q := p.rotate(0.5).Apply(gfx.Pt(1, 0)); q != gfx.Pt(0, -1) {
		t.Errorf("rotate 90° maps +x to %v, want up (0, -1)", q)
	}
}
