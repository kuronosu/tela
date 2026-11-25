package gfx

import (
	"math"
	"strings"
	"testing"
)

// porterDuff are the operators that are not blend modes of CSS.
var porterDuff = []BlendMode{SrcIn, SrcOut, SrcAtop, DstOver, DstIn, DstOut, DstAtop, Copy, Xor}

// TestPorterDuffModes checks the names and the properties of the operators,
// and that the values of the modes before them did not change.
func TestPorterDuffModes(t *testing.T) {
	names := []string{"source-in", "source-out", "source-atop", "destination-over", "destination-in",
		"destination-out", "destination-atop", "copy", "xor"}
	for i, m := range porterDuff {
		if m != PlusLighter+1+BlendMode(i) || m.String() != names[i] || !m.IsSeparable() {
			t.Errorf("%d: %d %q, separable %v", i, m, m, m.IsSeparable())
		}
	}
	if SrcOver != 0 || Multiply != 1 || Luminosity != 15 || PlusLighter != 16 {
		t.Error("the values of the CSS modes changed")
	}
	if Hue.IsSeparable() || Luminosity.IsSeparable() || !PlusLighter.IsSeparable() {
		t.Error("IsSeparable on CSS modes")
	}
	mustPanic(t, "bad mode", []string{"BlendMode(26)"}, func() { NewBuilder().PushLayer(1, Xor+1) })
}

// TestUnboundedLayers checks the bounds and the dropping rules of layers
// whose operator clears the backdrop where they are transparent.
func TestUnboundedLayers(t *testing.T) {
	clip := RectXYWH(10, 20, 200, 100)
	content := RectXYWH(30, 30, 20, 20)
	for _, m := range porterDuff {
		draw := func(opacity float32, body func(b *Builder)) *DisplayList {
			return build(func(b *Builder) {
				b.PushClipRect(clip)
				b.PushLayer(opacity, m)
				body(b)
				b.PopLayer()
				b.PopClip()
			})
		}
		fill := func(b *Builder) { b.FillRect(content, Black) }
		dl := draw(0.5, fill)
		want := content
		if m.Unbounded() {
			want = clip
		}
		if dl.Len() != 5 || dl.BoundsAt(1) != want || dl.BoundsAt(0) != want || dl.Bounds() != want || dl.BoundsAt(2) != content {
			t.Errorf("%s: bounds %v of the layer, want %v:\n%s", m, dl.BoundsAt(1), want, dl)
		}
		// Nothing to show: the layer alone, or nothing.
		for name, dl := range map[string]*DisplayList{
			"empty":            draw(1, func(*Builder) {}),
			"opacity 0":        draw(0, fill),
			"negative opacity": draw(-2, fill),
			"hidden content":   draw(1, func(b *Builder) { b.FillRect(RectXYWH(-500, 0, 10, 10), Black) }),
			"empty mask": build(func(b *Builder) {
				b.PushClipRect(clip)
				b.PushMaskLayer(Mask{List: build(func(*Builder) {})}, 1, m)
				fill(b)
				b.PopLayer()
				b.PopClip()
			}),
			"nil mask": build(func(b *Builder) {
				b.PushClipRect(clip)
				b.PushMaskLayer(Mask{}, 1, m)
				fill(b)
				b.PopLayer()
				b.PopClip()
			}),
			"mask outside the clip": build(func(b *Builder) {
				b.PushClipRect(clip)
				b.PushMaskLayer(Mask{List: maskOf(RectXYWH(500, 500, 10, 10))}, 1, m, Blur{StdDev: 2})
				fill(b)
				b.PopLayer()
				b.PopClip()
			}),
			"nested": draw(0, func(b *Builder) {
				// Nothing in a layer that does not show matters.
				b.PushTransform(Translate(1, 1))
				b.PushLayer(1, m, Blur{StdDev: 3})
				fill(b)
				b.PopLayer()
				b.PopTransform()
				b.Marker(3)
			}),
		} {
			if !m.Unbounded() {
				if dl.Len() != 0 {
					t.Errorf("%s, %s: %d commands, want none:\n%s", m, name, dl.Len(), dl)
				}
				continue
			}
			if dl.Len() != 4 || dl.BoundsAt(1) != clip {
				t.Errorf("%s, %s: want the layer alone over its clip:\n%s", m, name, dl)
				continue
			}
			l := dl.At(1).(PushLayer)
			if l.Blend != m || l.Mask.List != nil || len(l.Filters) != 0 || (l.Opacity != 0) != (name == "empty" || name == "hidden content") {
				t.Errorf("%s, %s: %+v", m, name, l)
			}
			// Appended, the layer keeps clearing the clip it is under.
			app := build(func(b *Builder) {
				b.PushTransform(Translate(5, 0))
				b.AppendList(dl, nil)
				b.PopTransform()
			})
			if app.Len() != 6 || app.BoundsAt(2) != clip.Translate(5, 0) || !Equal(app.At(2), dl.At(1)) {
				t.Errorf("%s, %s: appended:\n%s", m, name, app)
			}
		}
		// Dropped with everything around it.
		for name, dl := range map[string]*DisplayList{
			"NaN opacity":   draw(float32(math.NaN()), fill),
			"NaN filter":    build(func(b *Builder) { b.PushLayer(1, m, Blur{StdDev: float32(math.NaN())}); fill(b); b.PopLayer() }),
			"empty clip":    build(func(b *Builder) { b.PushClipRect(Rect{}); b.PushLayer(1, m); fill(b); b.PopLayer(); b.PopClip() }),
			"hidden parent": build(func(b *Builder) { b.PushLayer(0, SrcOver); b.PushLayer(1, m); fill(b); b.PopLayer(); b.PopLayer() }),
			"singular": build(func(b *Builder) {
				b.PushTransform(Scale(0, 1))
				b.PushLayer(1, m)
				fill(b)
				b.PopLayer()
				b.PopTransform()
			}),
		} {
			if dl.Len() != 0 {
				t.Errorf("%s, %s: %d commands, want none:\n%s", m, name, dl.Len(), dl)
			}
		}
	}
	// Under a mask, the content is cut to it; what is cleared is not.
	dl := build(func(b *Builder) {
		b.PushClipRect(clip)
		b.PushMaskLayer(Mask{List: maskOf(RectXYWH(40, 40, 30, 30))}, 1, DstIn, Blur{StdDev: 2})
		b.FillRect(RectXYWH(0, 0, 300, 300), Black)
		b.PopLayer()
		b.PopClip()
	})
	if dl.BoundsAt(1) != clip || dl.BoundsAt(2) != RectXYWH(34, 34, 42, 42) {
		t.Errorf("masked: layer %v, content %v:\n%s", dl.BoundsAt(1), dl.BoundsAt(2), dl)
	}
	// Without a clip the layer covers every surface, and no tile skips it.
	dl = build(func(b *Builder) {
		b.PushLayer(1, Copy)
		b.PopLayer()
	})
	n := 0
	dl.Visit(RectXYWH(1e5, -1e5, 1, 1), func(int, Command) { n++ })
	if n != 2 || !dl.Bounds().IsFinite() || dl.Bounds().Width() < 1e6 {
		t.Errorf("unclipped: %d commands visited, bounds %v", n, dl.Bounds())
	}
}

// TestImagePattern checks how the Builder takes patterns, and Equal and Dump.
func TestImagePattern(t *testing.T) {
	img := NewImagePremul(3, 2, make([]byte, 24))
	nan := float32(math.NaN())
	p := ImagePattern{Image: img, Repeat: RepeatX, Transform: Affine{2, 0, 0, 2, 5, 6}, Sampling: SampleNearest}
	for _, c := range []struct {
		name string
		p    ImagePattern
		ok   bool
	}{
		{"pattern", p, true},
		{"identity", ImagePattern{Image: img, Transform: Identity}, true},
		{"zero transform", ImagePattern{Image: img}, false},
		{"singular", ImagePattern{Image: img, Transform: Affine{1, 2, 2, 4, 0, 0}}, false},
		{"NaN", ImagePattern{Image: img, Transform: Affine{1, 0, 0, 1, nan, 0}}, false},
		{"infinite", ImagePattern{Image: img, Transform: Affine{1, 0, 0, float32(math.Inf(1)), 0, 0}}, false},
		{"empty image", ImagePattern{Image: NewImagePremul(0, 2, nil), Transform: Identity}, false},
	} {
		dl := build(func(b *Builder) {
			b.FillRect(RectXYWH(0, 0, 10, 10), c.p)
			b.FillRRect(NewRRect(RectXYWH(0, 0, 10, 10), 3), c.p)
			b.FillPath(RectPath(RectXYWH(0, 0, 10, 10)), NonZero, c.p)
			b.StrokePath(RectPath(RectXYWH(0, 0, 10, 10)), NewStrokeStyle(2), c.p)
			b.DrawGlyphRun(DrawGlyphRun{Font: &boxFont{key: 1}, Size: 10, Glyphs: []Glyph{{ID: 1}}, Paint: c.p})
		})
		if want := map[bool]int{true: 5}[c.ok]; dl.Len() != want {
			t.Errorf("%s: %d commands, want %d:\n%s", c.name, dl.Len(), want, dl)
		}
	}
	mustPanic(t, "no image", []string{"image is nil"}, func() { NewBuilder().FillRect(RectXYWH(0, 0, 1, 1), ImagePattern{Transform: Identity}) })
	mustPanic(t, "bad repeat", []string{"PatternRepeat(4)"}, func() {
		NewBuilder().FillRect(RectXYWH(0, 0, 1, 1), ImagePattern{Image: img, Repeat: 4, Transform: Identity})
	})
	for i, want := range []string{"repeat", "repeat-x", "repeat-y", "no-repeat", "PatternRepeat(?)"} {
		if got := PatternRepeat(i).String(); got != want {
			t.Errorf("PatternRepeat(%d) = %q, want %q", i, got, want)
		}
	}
	if !RepeatBoth.X() || !RepeatBoth.Y() || !RepeatX.X() || RepeatX.Y() || RepeatY.X() || !RepeatY.Y() || RepeatNone.X() || RepeatNone.Y() {
		t.Error("the axes of PatternRepeat")
	}
	others := []ImagePattern{p, p, p, p}
	others[0].Image = NewImagePremul(3, 2, make([]byte, 24))
	others[1].Repeat = RepeatY
	others[2].Transform[4] = 7
	others[3].Sampling = SampleLinear
	if !PaintEqual(p, p) || PaintEqual(p, White) || PaintEqual(White, p) {
		t.Error("PaintEqual on patterns")
	}
	for i, o := range others {
		if PaintEqual(p, o) || Equal(FillRect{Paint: p}, FillRect{Paint: o}) {
			t.Errorf("%d: different patterns are equal", i)
		}
	}
	dl := build(func(b *Builder) { b.FillRect(RectXYWH(0, 0, 10, 10), p) })
	if s := dl.String(); !strings.Contains(s, "paint=pattern(3x2 repeat-x nearest [2,0,0,2,5,6])") {
		t.Errorf("dump:\n%s", s)
	}
}
