package gfx

import "testing"

// TestEqualDeep checks that Equal compares the values of commands, not
// their slices' identity, and tells different values apart.
func TestEqualDeep(t *testing.T) {
	var p Path
	p.MoveTo(Pt(0, 0))
	p.CubicTo(Pt(10, 0), Pt(10, 10), Pt(0, 10))
	p.Close()
	grad := LinearGradient{Start: Pt(0, 0), End: Pt(10, 0), Stops: []GradientStop{{0, White}, {1, Black}}}
	font := &boxFont{key: 1}
	cmds := func() []Command {
		g := grad
		g.Stops = append([]GradientStop(nil), grad.Stops...)
		return []Command{
			FillRect{RectXYWH(1, 2, 3, 4), g},
			FillRRect{NewRRect(RectXYWH(0, 0, 9, 9), 2), White},
			FillPath{p.Clone(), EvenOdd, White},
			StrokePath{p.Clone(), StrokeStyle{Width: 2, MiterLimit: 4, Dashes: []float32{1, 2}}, Black},
			DrawShadow{RRect: NewRRect(RectXYWH(0, 0, 9, 9), 2), Blur: 4, Color: Black},
			DrawGlyphRun{Font: font, Size: 12, Origin: Pt(3, 4), Glyphs: []Glyph{{1, 0, 0}, {2, 5, 0}}, Paint: White},
			PushClipPath{p.Clone(), NonZero},
			PushTransform{Rotate(0.5)},
			PushLayer{Opacity: 0.5, Filters: []Filter{Blur{2}, Brightness(1.2)}},
			BackdropFilter{NewRRect(RectXYWH(0, 0, 9, 9), 2), []Filter{Blur{3}}, 1},
			Marker{3},
			PopLayer{},
		}
	}
	a, b := cmds(), cmds()
	for i := range a {
		if !Equal(a[i], b[i]) {
			t.Errorf("%s: copies differ", a[i].Name())
		}
		for j := range b {
			if i != j && Equal(a[i], b[j]) {
				t.Errorf("%s equals %s", a[i].Name(), b[j].Name())
			}
		}
	}
	changed := cmds()
	changed[0].(FillRect).Paint.(LinearGradient).Stops[1].Offset = 0.5
	changed[5].(DrawGlyphRun).Glyphs[1].X = 6
	changed[3] = StrokePath{p.Clone(), StrokeStyle{Width: 2, MiterLimit: 4, Dashes: []float32{1, 3}}, Black}
	for _, i := range []int{0, 3, 5} {
		if Equal(a[i], changed[i]) {
			t.Errorf("%s: a changed value compares equal", a[i].Name())
		}
	}
}

// TestMarkerAndTags checks markers (bounds of their clip, dropped under an
// empty clip), tags (pops carry their push's) and the backdrop table.
func TestMarkerAndTags(t *testing.T) {
	b := NewBuilder()
	b.SetTag(4)
	b.PushClipRect(RectXYWH(10, 10, 20, 20))
	b.Marker(1)
	b.PopClip()
	b.PushClipRect(Rect{}) // empty: everything inside is dropped
	b.Marker(2)
	b.PopClip()
	b.SetTag(5)
	b.PushTransform(Scale(2, 2))
	b.BackdropFilter(NewRRect(RectXYWH(0, 0, 10, 10), 2), Blur{1})
	b.PopTransform()
	dl := b.Finish()
	if dl.Len() != 6 {
		t.Fatalf("%d commands:\n%s", dl.Len(), dl)
	}
	if m, ok := dl.At(1).(Marker); !ok || m.ID != 1 || dl.BoundsAt(1) != RectXYWH(10, 10, 20, 20) {
		t.Errorf("marker %#v bounds %v", dl.At(1), dl.BoundsAt(1))
	}
	for i, want := range []uint32{4, 4, 4, 5, 5, 5} {
		if dl.Tag(i) != want || dl.Source(i) != -1 {
			t.Errorf("command %d: tag %d source %d, want %d and -1", i, dl.Tag(i), dl.Source(i), want)
		}
	}
	if dl.NumBackdrops() != 1 {
		t.Fatalf("%d backdrops", dl.NumBackdrops())
	}
	i, read, reach, inFilter := dl.Backdrop(0)
	if i != 4 || read != RectXYWH(0, 0, 20, 20) || reach != 6 || inFilter {
		t.Errorf("backdrop %d read %v reach %v in filter %v", i, read, reach, inFilter)
	}
}
