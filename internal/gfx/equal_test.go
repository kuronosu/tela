package gfx

import "testing"

// benchCommands builds a display list's worth of mixed commands, weighted
// toward the frequent ones. Every call returns fresh slices, so two lists
// compare element by element rather than through the shared-slice fast path.
func benchCommands() []Command {
	var p Path
	p.MoveTo(Pt(0, 0))
	p.CubicTo(Pt(10, 0), Pt(10, 10), Pt(0, 10))
	p.Close()
	font := &boxFont{key: 7}
	stops := func() []GradientStop { return []GradientStop{{0, White}, {1, Black}} }
	var cmds []Command
	for i := 0; i < 8; i++ {
		f := float32(i)
		cmds = append(cmds,
			PushTransform{Translate(f, 2*f)},
			PushClipRect{RectXYWH(0, 0, 100, 40)},
			FillRect{RectXYWH(f, 0, 50, 20), Color{R: 0.2, G: 0.4, B: 0.6, A: 1}},
			FillRRect{NewRRect(RectXYWH(0, f, 80, 30), 4), White},
			DrawShadow{RRect: NewRRect(RectXYWH(0, 0, 80, 30), 4), Offset: Pt(0, 2), Blur: 6, Color: Black},
			DrawGlyphRun{Font: font, Size: 14, Origin: Pt(4, 16+f), Glyphs: []Glyph{{1, 0, 0}, {2, 8, 0}, {3, 16, 0}, {4, 24, 0}}, Paint: Black},
			DrawGlyphRun{Font: font, Size: 14, Origin: Pt(4, 32+f), Glyphs: []Glyph{{5, 0, 0}, {6, 8, 0}}, Paint: White},
			FillRect{RectXYWH(0, f, 10, 10), LinearGradient{Start: Pt(0, 0), End: Pt(10, 0), Stops: stops()}},
			FillPath{p.Clone(), NonZero, White},
			PopClip{},
			PopTransform{},
		)
	}
	cmds = append(cmds,
		StrokePath{p.Clone(), StrokeStyle{Width: 2, MiterLimit: 4, Dashes: []float32{1, 2}}, Black},
		PushClipRRect{NewRRect(RectXYWH(0, 0, 9, 9), 2)}, PopClip{},
		PushClipPath{p.Clone(), EvenOdd}, PopClip{},
		PushLayer{Opacity: 0.5, Filters: []Filter{Blur{2}}}, PopLayer{},
		BackdropFilter{NewRRect(RectXYWH(0, 0, 9, 9), 2), []Filter{Blur{3}}, 1},
		Marker{3},
	)
	return cmds
}

// BenchmarkEqual compares two equal lists of mixed commands, then a pair
// that differs only in its last command.
func BenchmarkEqual(b *testing.B) {
	x, y := benchCommands(), benchCommands()
	z := benchCommands()
	z[len(z)-1] = Marker{4}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range x {
			if !Equal(x[j], y[j]) {
				b.Fatalf("%s: equal commands compare different", x[j].Name())
			}
		}
		for j := range x {
			if !Equal(x[j], z[j]) {
				if j != len(x)-1 {
					b.Fatalf("%s: unexpected difference", x[j].Name())
				}
				break
			}
		}
	}
}
