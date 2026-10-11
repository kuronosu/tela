package gfx

import (
	"image"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func build(f func(b *Builder)) *DisplayList {
	b := NewBuilder()
	f(b)
	return b.Finish()
}

func mustPanic(t *testing.T, name string, wantSubs []string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Errorf("%s: expected a panic", name)
			return
		}
		msg, _ := r.(string)
		for _, sub := range wantSubs {
			if !strings.Contains(msg, sub) {
				t.Errorf("%s: panic message %q does not contain %q", name, msg, sub)
			}
		}
	}()
	f()
}

func TestBuilderNestingPanics(t *testing.T) {
	mustPanic(t, "pop with nothing open", []string{"PopClip", "no open block", "PushClip"}, func() {
		NewBuilder().PopClip()
	})
	mustPanic(t, "wrong pop kind", []string{"PopLayer", "innermost open block is PushTransform", "call #2", "PopTransform"}, func() {
		b := NewBuilder()
		b.PushClipRect(Rect{0, 0, 10, 10})
		b.PushTransform(Translate(1, 1))
		b.PopLayer()
	})
	mustPanic(t, "interleaved", []string{"PopClip", "PushTransform"}, func() {
		b := NewBuilder()
		b.PushClipRect(Rect{0, 0, 10, 10})
		b.PushTransform(Translate(1, 1))
		b.PopClip()
	})
	mustPanic(t, "finish with open blocks", []string{"2 open block", "PushLayer (Builder call #2)", "PushClipRect (Builder call #1)", "PopLayer, PopClip"}, func() {
		b := NewBuilder()
		b.PushClipRect(Rect{0, 0, 10, 10})
		b.PushLayer(0.5, SrcOver)
		b.Finish()
	})
	mustPanic(t, "nil paint", []string{"FillRect", "paint is nil"}, func() {
		NewBuilder().FillRect(Rect{0, 0, 1, 1}, nil)
	})
	mustPanic(t, "nil image", []string{"DrawImage", "image is nil"}, func() {
		NewBuilder().DrawImage(nil, Rect{}, Rect{0, 0, 1, 1}, SampleLinear)
	})
	mustPanic(t, "bad blend", []string{"BlendMode(200)"}, func() {
		NewBuilder().PushLayer(1, BlendMode(200))
	})
	// Dropped blocks are still validated.
	mustPanic(t, "dropped block pop mismatch", []string{"PopTransform", "PushLayer"}, func() {
		b := NewBuilder()
		b.PushLayer(0, SrcOver) // dropped (opacity 0)
		b.PopTransform()
	})
	mustPanic(t, "dropped block unfinished", []string{"PushTransform"}, func() {
		b := NewBuilder()
		b.PushTransform(Affine{float32(math.NaN()), 0, 0, 1, 0, 0})
		b.Finish()
	})
}

func TestBuilderReuseAfterFinish(t *testing.T) {
	b := NewBuilder()
	b.PushTransform(Translate(100, 100))
	b.FillRect(Rect{0, 0, 10, 10}, Black)
	b.PopTransform()
	first := b.Finish()
	b.FillRect(Rect{0, 0, 10, 10}, White)
	second := b.Finish()
	if first.Len() != 3 || second.Len() != 1 {
		t.Fatalf("lens = %d, %d", first.Len(), second.Len())
	}
	if second.BoundsAt(0) != (Rect{0, 0, 10, 10}) {
		t.Errorf("transform leaked into the reused builder: %v", second.BoundsAt(0))
	}
	if first.BoundsAt(0) != (Rect{100, 100, 110, 110}) {
		t.Errorf("first list changed: %v", first.BoundsAt(0))
	}
	// A zero Builder is also usable.
	var z Builder
	z.FillRect(Rect{0, 0, 1, 1}, Black)
	if z.Finish().Len() != 1 {
		t.Error("zero Builder unusable")
	}
	if got := NewBuilder().Finish(); got.Len() != 0 || !got.Bounds().Empty() || got.String() != "" {
		t.Errorf("empty list: len %d bounds %v %q", got.Len(), got.Bounds(), got.String())
	}
}

func TestBuilderBoundsTransforms(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *Builder)
		want  Rect
	}{
		{"plain", func(b *Builder) { b.FillRect(Rect{1, 2, 11, 22}, Black) }, Rect{1, 2, 11, 22}},
		{"inverted rect is made upright", func(b *Builder) { b.FillRect(Rect{11, 22, 1, 2}, Black) }, Rect{1, 2, 11, 22}},
		{"translate", func(b *Builder) {
			b.PushTransform(Translate(10, 20))
			b.FillRect(Rect{0, 0, 5, 5}, Black)
			b.PopTransform()
		}, Rect{10, 20, 15, 25}},
		{"nested: inner applies first", func(b *Builder) {
			b.PushTransform(Translate(100, 0))
			b.PushTransform(Scale(2, 2))
			b.FillRect(Rect{0, 0, 5, 5}, Black)
			b.PopTransform()
			b.PopTransform()
		}, Rect{100, 0, 110, 10}},
		{"nested reversed", func(b *Builder) {
			b.PushTransform(Scale(2, 2))
			b.PushTransform(Translate(100, 0))
			b.FillRect(Rect{0, 0, 5, 5}, Black)
			b.PopTransform()
			b.PopTransform()
		}, Rect{200, 0, 210, 10}},
		{"state restored after pop", func(b *Builder) {
			b.PushTransform(Translate(1000, 1000))
			b.PopTransform()
			b.FillRect(Rect{0, 0, 5, 5}, Black)
		}, Rect{0, 0, 5, 5}},
		{"rotation grows to the bounding box", func(b *Builder) {
			b.PushTransform(Rotate(math.Pi / 4))
			b.FillRect(Rect{0, 0, 10, 10}, Black)
			b.PopTransform()
		}, Rect{-7.0710678, 0, 7.0710678, 14.142136}},
		{"fill path uses control bounds", func(b *Builder) {
			var p Path
			p.MoveTo(Pt(0, 0))
			p.CubicTo(Pt(0, 100), Pt(100, 100), Pt(100, 0))
			p.Close()
			b.FillPath(p, NonZero, Black)
		}, Rect{0, 0, 100, 100}},
		{"fill rrect", func(b *Builder) { b.FillRRect(NewRRect(Rect{5, 5, 50, 50}, 9), Black) }, Rect{5, 5, 50, 50}},
		{"image uses dst", func(b *Builder) {
			b.DrawImage(NewImage(image.NewRGBA(image.Rect(0, 0, 4, 4))), Rect{}, Rect{10, 10, 30, 50}, SampleLinear)
		}, Rect{10, 10, 30, 50}},
	}
	for _, tt := range tests {
		dl := build(tt.build)
		if dl.Len() == 0 {
			t.Errorf("%s: nothing recorded", tt.name)
			continue
		}
		if got := dl.Bounds(); !rectNear(got, tt.want, 1e-4) {
			t.Errorf("%s: Bounds = %v want %v", tt.name, got, tt.want)
		}
	}
}

func TestBuilderBoundsStroke(t *testing.T) {
	line := func() Path {
		var p Path
		p.MoveTo(Pt(0, 10))
		p.LineTo(Pt(100, 10))
		return p
	}
	tests := []struct {
		name  string
		style StrokeStyle
		want  Rect
	}{
		// Width 4: half 2. Miter joins can reach MiterLimit*half = 8.
		{"default miter", NewStrokeStyle(4), Rect{-8, 2, 108, 18}},
		{"zero miter limit means default", StrokeStyle{Width: 4}, Rect{-8, 2, 108, 18}},
		{"bevel join, butt cap", StrokeStyle{Width: 4, Join: JoinBevel}, Rect{-2, 8, 102, 12}},
		{"round join", StrokeStyle{Width: 4, Join: JoinRound}, Rect{-2, 8, 102, 12}},
		{"square cap", StrokeStyle{Width: 4, Join: JoinBevel, Cap: CapSquare}, Rect{-2.828427, 7.171573, 102.828427, 12.828427}},
		{"miter limit 1", StrokeStyle{Width: 4, MiterLimit: 1}, Rect{-2, 8, 102, 12}},
		{"miter limit below 1 raised", StrokeStyle{Width: 4, MiterLimit: 0.2}, Rect{-2, 8, 102, 12}},
		{"miter 10", StrokeStyle{Width: 2, MiterLimit: 10}, Rect{-10, 0, 110, 20}},
	}
	for _, tt := range tests {
		dl := build(func(b *Builder) { b.StrokePath(line(), tt.style, Black) })
		if dl.Len() != 1 {
			t.Errorf("%s: %d commands", tt.name, dl.Len())
			continue
		}
		if got := dl.BoundsAt(0); !rectNear(got, tt.want, 1e-3) {
			t.Errorf("%s: bounds = %v want %v", tt.name, got, tt.want)
		}
	}
	// Stroke bounds under non-uniform scale: the outset is applied locally.
	dl := build(func(b *Builder) {
		b.PushTransform(Scale(1, 3))
		b.StrokePath(line(), StrokeStyle{Width: 4, Join: JoinBevel}, Black)
		b.PopTransform()
	})
	if got := dl.Bounds(); !rectNear(got, Rect{-2, 24, 102, 36}, 1e-3) {
		t.Errorf("scaled stroke bounds = %v", got)
	}
	// A zero-length path (a dot) still has area with round caps.
	var dot Path
	dot.MoveTo(Pt(5, 5))
	dot.Close()
	dl = build(func(b *Builder) { b.StrokePath(dot, StrokeStyle{Width: 6, Cap: CapRound}, Black) })
	if dl.Len() != 1 || dl.BoundsAt(0).Empty() {
		t.Errorf("dot stroke dropped or empty: %d %v", dl.Len(), dl.Bounds())
	}
}

func TestBuilderStrokeSanitize(t *testing.T) {
	line := RectPath(Rect{0, 0, 10, 10})
	nan := float32(math.NaN())
	for _, tt := range []struct {
		name  string
		style StrokeStyle
		keep  bool
	}{
		{"zero width", StrokeStyle{}, false},
		{"negative width", StrokeStyle{Width: -1}, false},
		{"nan width", StrokeStyle{Width: nan}, false},
		{"nan miter", StrokeStyle{Width: 1, MiterLimit: nan}, false},
		{"ok", NewStrokeStyle(1), true},
	} {
		dl := build(func(b *Builder) { b.StrokePath(line, tt.style, Black) })
		if (dl.Len() == 1) != tt.keep {
			t.Errorf("%s: kept = %v", tt.name, dl.Len() == 1)
		}
	}
	dashTests := []struct {
		name string
		in   []float32
		want []float32
	}{
		{"nil", nil, nil},
		{"even", []float32{4, 2}, []float32{4, 2}},
		{"odd repeats", []float32{3}, []float32{3, 3}},
		{"odd three", []float32{1, 2, 3}, []float32{1, 2, 3, 1, 2, 3}},
		{"negative dropped", []float32{4, -2}, nil},
		{"nan dropped", []float32{nan, 2}, nil},
		{"all zero dropped", []float32{0, 0}, nil},
		{"zero gap ok", []float32{4, 0}, []float32{4, 0}},
	}
	for _, tt := range dashTests {
		style := NewStrokeStyle(2)
		style.Dashes = tt.in
		style.DashOffset = 3
		dl := build(func(b *Builder) { b.StrokePath(line, style, Black) })
		got := dl.At(0).(StrokePath).Style
		if len(got.Dashes) != len(tt.want) {
			t.Errorf("%s: dashes = %v want %v", tt.name, got.Dashes, tt.want)
			continue
		}
		for i := range tt.want {
			if got.Dashes[i] != tt.want[i] {
				t.Errorf("%s: dashes = %v want %v", tt.name, got.Dashes, tt.want)
			}
		}
		if tt.want == nil && got.DashOffset != 0 {
			t.Errorf("%s: dash offset kept for solid line", tt.name)
		}
	}
	dl := build(func(b *Builder) { b.StrokePath(line, StrokeStyle{Width: 1}, Black) })
	if m := dl.At(0).(StrokePath).Style.MiterLimit; m != DefaultMiterLimit {
		t.Errorf("miter limit not defaulted: %v", m)
	}
}

func TestBuilderBoundsClip(t *testing.T) {
	dl := build(func(b *Builder) {
		b.PushClipRect(Rect{0, 0, 50, 50})
		b.FillRect(Rect{40, 40, 100, 100}, Black) // clipped to [40,50]
		b.FillRect(Rect{60, 60, 70, 70}, Black)   // outside: dropped
		b.PushClipRRect(NewRRect(Rect{10, 10, 30, 30}, 4))
		b.FillRect(Rect{0, 0, 100, 100}, Black) // clipped to [10,30]
		b.PopClip()
		b.FillRect(Rect{0, 0, 5, 5}, Black) // clip restored
		b.PopClip()
		b.FillRect(Rect{200, 200, 210, 210}, Black) // no clip any more
	})
	want := []struct {
		name string
		rect Rect
	}{
		{"PushClipRect", Rect{0, 0, 50, 50}},
		{"FillRect", Rect{40, 40, 50, 50}},
		{"PushClipRRect", Rect{10, 10, 30, 30}},
		{"FillRect", Rect{10, 10, 30, 30}},
		{"PopClip", Rect{10, 10, 30, 30}},
		{"FillRect", Rect{0, 0, 5, 5}},
		{"PopClip", Rect{0, 0, 50, 50}},
		{"FillRect", Rect{200, 200, 210, 210}},
	}
	if dl.Len() != len(want) {
		t.Fatalf("len = %d\n%s", dl.Len(), dl)
	}
	for i, w := range want {
		if dl.At(i).Name() != w.name || dl.BoundsAt(i) != w.rect {
			t.Errorf("#%d: %s %v, want %s %v", i, dl.At(i).Name(), dl.BoundsAt(i), w.name, w.rect)
		}
	}
	if dl.Bounds() != (Rect{0, 0, 210, 210}) {
		t.Errorf("total bounds = %v", dl.Bounds())
	}
	// Clip under transforms: the clip is given in the transformed space.
	dl = build(func(b *Builder) {
		b.PushTransform(Translate(100, 100))
		b.PushClipRect(Rect{0, 0, 10, 10})
		b.PopClip()
		b.PushClipPath(RectPath(Rect{0, 0, 10, 10}), EvenOdd)
		b.FillRect(Rect{-50, -50, 50, 50}, Black)
		b.PopClip()
		b.PopTransform()
	})
	if dl.Bounds() != (Rect{100, 100, 110, 110}) {
		t.Errorf("clip in transformed space: bounds = %v\n%s", dl.Bounds(), dl)
	}
}

func TestBuilderDropsInvisibleBlocks(t *testing.T) {
	// A clip whose content is entirely outside is removed with the content.
	dl := build(func(b *Builder) {
		b.PushClipRect(Rect{0, 0, 10, 10})
		b.FillRect(Rect{20, 20, 30, 30}, Black)
		b.PopClip()
	})
	if dl.Len() != 0 {
		t.Errorf("empty clip block was kept:\n%s", dl)
	}
	// Nested empty blocks collapse recursively.
	dl = build(func(b *Builder) {
		b.PushTransform(Translate(1, 1))
		b.PushLayer(0.5, SrcOver, Blur{3})
		b.PopLayer()
		b.PopTransform()
	})
	if dl.Len() != 0 {
		t.Errorf("empty layer block was kept:\n%s", dl)
	}
	// An empty clip drops everything inside, including nested blocks.
	dl = build(func(b *Builder) {
		b.PushClipRect(Rect{5, 5, 5, 10}) // zero width
		b.PushTransform(Translate(1, 1))
		b.FillRect(Rect{0, 0, 100, 100}, Black)
		b.PopTransform()
		b.PopClip()
		b.FillRect(Rect{0, 0, 1, 1}, Black)
	})
	if dl.Len() != 1 {
		t.Errorf("zero-area clip content kept:\n%s", dl)
	}
	// Intersecting disjoint clips clips everything.
	dl = build(func(b *Builder) {
		b.PushClipRect(Rect{0, 0, 10, 10})
		b.PushClipRect(Rect{20, 20, 30, 30})
		b.FillRect(Rect{0, 0, 100, 100}, Black)
		b.PopClip()
		b.PopClip()
	})
	if dl.Len() != 0 {
		t.Errorf("disjoint clips kept content:\n%s", dl)
	}
	// Singular transforms and zero opacity drop the block.
	dl = build(func(b *Builder) {
		b.PushTransform(Scale(0, 1))
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PopTransform()
		b.PushLayer(0, SrcOver)
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PopLayer()
	})
	if dl.Len() != 0 {
		t.Errorf("invisible blocks kept:\n%s", dl)
	}
	// No-op draws.
	dl = build(func(b *Builder) {
		b.FillRect(Rect{}, Black)
		b.FillRect(Rect{0, 0, 10, 10}, Transparent)
		b.FillRect(Rect{0, 0, 10, 10}, LinearGradient{End: Pt(1, 0)}) // no stops
		b.FillPath(Path{}, NonZero, Black)
		b.FillRRect(RRect{}, Black)
		b.DrawShadow(DrawShadow{RRect: NewRRect(Rect{0, 0, 10, 10}, 2), Color: Transparent})
		b.BackdropFilter(NewRRect(Rect{0, 0, 10, 10}, 2)) // no filters
		b.BackdropFilter(NewRRect(Rect{0, 0, 10, 10}, 2), Blur{0})
		var line Path
		line.MoveTo(Pt(0, 0))
		line.LineTo(Pt(10, 0))
		b.FillPath(line, NonZero, Black) // zero area
	})
	if dl.Len() != 0 {
		t.Errorf("no-op commands kept:\n%s", dl)
	}
}

func TestBuilderNonFinite(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	bad := Rect{0, 0, nan, 10}
	var badPath Path
	badPath.MoveTo(Pt(0, 0))
	badPath.LineTo(Pt(inf, 5))
	badStops := []GradientStop{{0, Black}, {1, White}}
	dl := build(func(b *Builder) {
		b.FillRect(bad, Black)
		b.FillRect(Rect{0, 0, inf, 10}, Black)
		b.FillRect(Rect{0, 0, 10, 10}, Color{nan, 0, 0, 1})
		b.FillRect(Rect{0, 0, 10, 10}, LinearGradient{Start: Pt(nan, 0), Stops: badStops})
		b.FillRRect(RRect{Rect: bad}, Black)
		b.FillPath(badPath, NonZero, Black)
		b.StrokePath(badPath, NewStrokeStyle(1), Black)
		b.DrawShadow(DrawShadow{RRect: NewRRect(Rect{0, 0, 10, 10}, 2), Blur: nan, Color: Black})
		b.DrawShadow(DrawShadow{RRect: NewRRect(Rect{0, 0, 10, 10}, 2), Offset: Pt(inf, 0), Color: Black})
		b.DrawImage(NewImage(image.NewRGBA(image.Rect(0, 0, 2, 2))), Rect{}, bad, SampleLinear)
		b.BackdropFilter(NewRRect(Rect{0, 0, 10, 10}, 2), Blur{nan})
		// Pushes with bad values drop the whole block, nested pushes included.
		b.PushTransform(Affine{nan, 0, 0, 1, 0, 0})
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PushClipRect(Rect{0, 0, 5, 5})
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PopClip()
		b.PopTransform()
		b.PushClipRect(bad)
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PopClip()
		b.PushClipPath(badPath, NonZero)
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PopClip()
		b.PushClipRRect(RRect{Rect: Rect{0, 0, 10, inf}})
		b.PopClip()
		b.PushLayer(nan, SrcOver)
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PopLayer()
		b.PushLayer(1, SrcOver, Blur{inf})
		b.FillRect(Rect{0, 0, 10, 10}, Black)
		b.PopLayer()
		// The builder state is intact afterwards.
		b.FillRect(Rect{1, 1, 2, 2}, Black)
	})
	if dl.Len() != 1 || dl.Bounds() != (Rect{1, 1, 2, 2}) {
		t.Errorf("non-finite input leaked into the list:\n%s", dl)
	}
}

func TestBuilderBoundsShadow(t *testing.T) {
	box := NewRRect(Rect{10, 10, 110, 60}, 8)
	tests := []struct {
		name string
		s    DrawShadow
		want Rect
	}{
		// sigma = Blur/2 = 5: 3 sigma = 15, plus spread 2 and offset (0, 4).
		{"outer", DrawShadow{RRect: box, Offset: Pt(0, 4), Blur: 10, Spread: 2, Color: Black}, Rect{-7, -3, 127, 81}},
		{"outer, no blur", DrawShadow{RRect: box, Offset: Pt(3, 3), Color: Black}, Rect{13, 13, 113, 63}},
		{"outer, negative spread never shrinks the bounds", DrawShadow{RRect: box, Blur: 4, Spread: -5, Color: Black}, Rect{4, 4, 116, 66}},
		{"negative blur counts as 0", DrawShadow{RRect: box, Blur: -4, Color: Black}, Rect{10, 10, 110, 60}},
		{"inset is painted only inside the box", DrawShadow{RRect: box, Offset: Pt(50, 50), Blur: 40, Spread: 9, Inset: true, Color: Black}, Rect{10, 10, 110, 60}},
	}
	for _, tt := range tests {
		dl := build(func(b *Builder) { b.DrawShadow(tt.s) })
		if dl.Len() != 1 {
			t.Errorf("%s: %d commands", tt.name, dl.Len())
			continue
		}
		if got := dl.BoundsAt(0); !rectNear(got, tt.want, 1e-4) {
			t.Errorf("%s: bounds = %v want %v", tt.name, got, tt.want)
		}
	}
}

func TestBuilderBoundsLayers(t *testing.T) {
	// Blur sigma 4: reach 12.
	dl := build(func(b *Builder) {
		b.PushLayer(0.5, SrcOver, Blur{4})
		b.FillRect(Rect{10, 10, 20, 20}, Black)
		b.PopLayer()
	})
	if dl.Len() != 3 {
		t.Fatalf("len = %d", dl.Len())
	}
	if got := dl.BoundsAt(0); got != (Rect{-2, -2, 32, 32}) {
		t.Errorf("layer bounds = %v", got)
	}
	if got := dl.BoundsAt(1); got != (Rect{10, 10, 20, 20}) {
		t.Errorf("content bounds = %v (children keep their own bounds)", got)
	}
	if dl.BoundsAt(2) != dl.BoundsAt(0) || dl.Bounds() != dl.BoundsAt(0) {
		t.Error("pop and total bounds should equal the layer's")
	}
	if dl.LayerReach(0) != 12 || dl.LayerReach(1) != 0 {
		t.Errorf("reach = %v %v", dl.LayerReach(0), dl.LayerReach(1))
	}
	// Under a scale of 2 the blur is twice as large in root space.
	dl = build(func(b *Builder) {
		b.PushTransform(Scale(2, 2))
		b.PushLayer(1, Multiply, Blur{4}, DropShadow{Offset: Pt(3, -5), StdDev: 1, Color: Black})
		b.FillRect(Rect{10, 10, 20, 20}, Black)
		b.PopLayer()
		b.PopTransform()
	})
	// Filters outset: 12 + (5 + 3) = 20 local, 40 root; content [20,20,40,40].
	if got := dl.BoundsAt(1); !rectNear(got, Rect{-20, -20, 80, 80}, 1e-4) {
		t.Errorf("scaled layer bounds = %v", got)
	}
	// The layer is limited by the clip active at PushLayer time, but its
	// content is limited by clips inside.
	dl = build(func(b *Builder) {
		b.PushClipRect(Rect{0, 0, 30, 30})
		b.PushLayer(1, SrcOver, Blur{4})
		b.FillRect(Rect{10, 10, 100, 100}, Black)
		b.PopLayer()
		b.PopClip()
	})
	if got := dl.BoundsAt(1); !rectNear(got, Rect{0, 0, 30, 30}, 1e-4) {
		t.Errorf("clipped layer bounds = %v", got)
	}
	if got := dl.BoundsAt(2); got != (Rect{10, 10, 30, 30}) {
		t.Errorf("clipped content bounds = %v", got)
	}
	// Opacity is clamped; identity blurs disappear from the stored filters.
	dl = build(func(b *Builder) {
		b.PushLayer(7, SrcOver, Blur{0})
		b.FillRect(Rect{0, 0, 1, 1}, Black)
		b.PopLayer()
	})
	l := dl.At(0).(PushLayer)
	if l.Opacity != 1 || len(l.Filters) != 0 {
		t.Errorf("layer = %+v", l)
	}
	// Backdrop filter: bounds are the shape; the reach is reported separately.
	dl = build(func(b *Builder) {
		b.PushTransform(Scale(2, 2))
		b.BackdropFilter(NewRRect(Rect{10, 10, 60, 40}, 8), Blur{5}, Saturate(1.5))
		b.PopTransform()
	})
	if got := dl.BoundsAt(1); got != (Rect{20, 20, 120, 80}) {
		t.Errorf("backdrop bounds = %v", got)
	}
	if dl.BackdropReach() != 30 {
		t.Errorf("BackdropReach = %v want 30", dl.BackdropReach())
	}
}

func TestMatchAndVisit(t *testing.T) {
	dl := build(func(b *Builder) {
		b.FillRect(Rect{0, 0, 10, 10}, Black) // 0: tile A
		b.PushTransform(Translate(100, 0))    // 1
		b.FillRect(Rect{0, 0, 10, 10}, Black) // 2: x 100..110
		b.PushClipRect(Rect{0, 0, 5, 5})      // 3
		b.FillRect(Rect{0, 0, 50, 50}, Black) // 4: x 100..105
		b.PopClip()                           // 5
		b.PopTransform()                      // 6
		b.PushLayer(1, SrcOver, Blur{2})      // 7: reach 6
		b.FillRect(Rect{200, 0, 210, 10}, Black)
		b.PopLayer() // 9
	})
	if dl.Len() != 10 {
		t.Fatalf("len = %d\n%s", dl.Len(), dl)
	}
	wantMatch := []int{-1, 6, -1, 5, -1, 3, 1, 9, -1, 7}
	for i, w := range wantMatch {
		if dl.Match(i) != w {
			t.Errorf("Match(%d) = %d want %d", i, dl.Match(i), w)
		}
	}
	visit := func(r Rect) []int {
		var got []int
		dl.Visit(r, func(i int, c Command) {
			if dl.At(i).Name() != c.Name() {
				t.Errorf("callback command mismatch at %d", i)
			}
			got = append(got, i)
		})
		return got
	}
	eq := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	tests := []struct {
		name   string
		region Rect
		want   []int
	}{
		{"everything", Rect{-1000, -1000, 1000, 1000}, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{"only the first", Rect{0, 0, 50, 50}, []int{0}},
		{"transform block", Rect{101, 1, 104, 4}, []int{1, 2, 3, 4, 5, 6}},
		// The clip block (3..5) is skipped as a whole: it only covers x 100..105.
		{"transform block, outside the clip", Rect{106, 1, 108, 4}, []int{1, 2, 6}},
		// The layer's blur reaches 6 pixels, so content at x=200 matters for
		// the region [195,199] even though they do not touch.
		{"layer reach pulls in the content", Rect{195, 0, 199, 10}, []int{7, 8, 9}},
		{"nothing", Rect{500, 500, 600, 600}, nil},
	}
	for _, tt := range tests {
		if got := visit(tt.region); !eq(got, tt.want) {
			t.Errorf("%s: visited %v want %v", tt.name, got, tt.want)
		}
	}
	// Properly nested output for every tile of a grid.
	for x := float32(-10); x < 230; x += 7 {
		var depth int
		dl.Visit(Rect{x, 0, x + 7, 50}, func(i int, c Command) {
			if _, ok := pushKind(c); ok {
				depth++
			}
			if _, ok := popKind(c); ok {
				depth--
			}
			if depth < 0 {
				t.Fatalf("unbalanced pops in tile at x=%v", x)
			}
		})
		if depth != 0 {
			t.Errorf("tile x=%v ends at depth %d", x, depth)
		}
	}
	// All iterates everything in order, and stops early.
	n := 0
	for i, c := range dl.All() {
		if i != n || c.Name() != dl.At(i).Name() {
			t.Errorf("All yielded %d at position %d", i, n)
		}
		n++
		if n == 3 {
			break
		}
	}
	if n != 3 {
		t.Error("All did not stop")
	}
}

func TestBuilderImmutability(t *testing.T) {
	path := RectPath(Rect{0, 0, 10, 10})
	stops := []GradientStop{{0, Black}, {1, White}}
	dashes := []float32{4, 2}
	filters := []Filter{Blur{3}, DropShadow{StdDev: 1, Color: Black}}
	clipPath := RectPath(Rect{0, 0, 50, 50})

	b := NewBuilder()
	b.FillPath(path, NonZero, LinearGradient{End: Pt(10, 0), Stops: stops})
	style := NewStrokeStyle(2)
	style.Dashes = dashes
	b.StrokePath(path, style, Black)
	b.PushClipPath(clipPath, NonZero)
	b.PushLayer(1, SrcOver, filters...)
	b.FillRect(Rect{0, 0, 5, 5}, Black)
	b.PopLayer()
	b.PopClip()
	b.BackdropFilter(NewRRect(Rect{0, 0, 5, 5}, 1), filters...)
	dl := b.Finish()
	before := dl.String()

	// Scribble over every input the caller still owns.
	path.Points()[0] = Pt(-99, -99)
	path.Points()[2] = Pt(99, 99)
	clipPath.Points()[1] = Pt(77, 77)
	stops[0] = GradientStop{0.5, White}
	dashes[0] = 100
	filters[0] = Blur{100}
	filters[1] = Blur{100}

	if after := dl.String(); after != before {
		t.Errorf("the DisplayList changed after the inputs were modified:\n--- before\n%s--- after\n%s", before, after)
	}
}

// TestBuilderRandomSequences drives the Builder with random command streams,
// including NaN, infinite and huge values, and checks the structural
// invariants of the result: balanced nesting, matching indices, finite bounds
// and children inside their block's bounds.
func TestBuilderRandomSequences(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	special := []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)), 1e30, -1e30, 0}
	num := func() float32 {
		if rng.Intn(25) == 0 {
			return special[rng.Intn(len(special))]
		}
		return float32(rng.Intn(400) - 100)
	}
	rect := func() Rect { return Rect{num(), num(), num(), num()} }
	rrect := func() RRect {
		return RRect{rect(), [4]Point{{num(), num()}, {num(), num()}, {num(), num()}, {num(), num()}}}
	}
	path := func() Path {
		var p Path
		p.MoveTo(Pt(num(), num()))
		for i := rng.Intn(4); i >= 0; i-- {
			switch rng.Intn(3) {
			case 0:
				p.LineTo(Pt(num(), num()))
			case 1:
				p.QuadTo(Pt(num(), num()), Pt(num(), num()))
			default:
				p.CubicTo(Pt(num(), num()), Pt(num(), num()), Pt(num(), num()))
			}
		}
		return p
	}
	paints := func() Paint {
		switch rng.Intn(4) {
		case 0:
			return Color{rng.Float32(), rng.Float32(), rng.Float32(), rng.Float32()}
		case 1:
			return LinearGradient{Start: Pt(num(), num()), End: Pt(num(), num()), Stops: []GradientStop{AutoStop(Black), AutoStop(White)}}
		case 2:
			return RadialGradient{Center: Pt(num(), num()), Radius: Pt(num(), num()), Stops: []GradientStop{AutoStop(Black), AutoStop(White)}}
		}
		return ConicGradient{Center: Pt(num(), num()), StartAngle: num(), Stops: []GradientStop{AutoStop(Black), AutoStop(White)}}
	}
	filters := func() []Filter {
		switch rng.Intn(4) {
		case 0:
			return []Filter{Blur{num()}}
		case 1:
			return []Filter{DropShadow{Offset: Pt(num(), num()), StdDev: num(), Color: Black}, Brightness(num())}
		}
		return nil
	}
	img := NewImage(image.NewRGBA(image.Rect(0, 0, 3, 3)))
	for iter := 0; iter < 300; iter++ {
		b := NewBuilder()
		var open []stackKind
		for n := rng.Intn(60); n > 0; n-- {
			switch k := rng.Intn(16); {
			case k == 0:
				b.FillRect(rect(), paints())
			case k == 1:
				b.FillRRect(rrect(), paints())
			case k == 2:
				b.FillPath(path(), FillRule(rng.Intn(2)), paints())
			case k == 3:
				b.StrokePath(path(), StrokeStyle{Width: num(), MiterLimit: num(), Dashes: []float32{num(), num(), num()}}, paints())
			case k == 4:
				b.DrawImage(img, rect(), rect(), SampleLinear)
			case k == 5:
				b.DrawShadow(DrawShadow{RRect: rrect(), Offset: Pt(num(), num()), Blur: num(), Spread: num(), Color: Black, Inset: rng.Intn(2) == 0})
			case k == 6:
				b.BackdropFilter(rrect(), Blur{num()})
			case k == 7:
				b.PushClipRect(rect())
				open = append(open, kindClip)
			case k == 8:
				b.PushClipRRect(rrect())
				open = append(open, kindClip)
			case k == 9:
				b.PushClipPath(path(), NonZero)
				open = append(open, kindClip)
			case k == 10:
				b.PushTransform(Affine{num(), num(), num(), num(), num(), num()})
				open = append(open, kindTransform)
			case k == 11:
				b.PushLayer(rng.Float32()*1.5-0.2, BlendMode(rng.Intn(17)), filters()...)
				open = append(open, kindLayer)
			default:
				if len(open) == 0 {
					continue
				}
				switch open[len(open)-1] {
				case kindClip:
					b.PopClip()
				case kindTransform:
					b.PopTransform()
				default:
					b.PopLayer()
				}
				open = open[:len(open)-1]
			}
		}
		for len(open) > 0 {
			switch open[len(open)-1] {
			case kindClip:
				b.PopClip()
			case kindTransform:
				b.PopTransform()
			default:
				b.PopLayer()
			}
			open = open[:len(open)-1]
		}
		dl := b.Finish()
		var stack []int
		for i := 0; i < dl.Len(); i++ {
			c := dl.At(i)
			bb := dl.BoundsAt(i)
			if !bb.IsFinite() || bb.Empty() {
				t.Fatalf("iter %d: command %d (%s) has bounds %v\n%s", iter, i, c.Name(), bb, dl)
			}
			if !dl.Bounds().ContainsRect(bb) {
				t.Fatalf("iter %d: command %d bounds %v outside total %v", iter, i, bb, dl.Bounds())
			}
			if _, ok := pushKind(c); ok {
				stack = append(stack, i)
				if m := dl.Match(i); m <= i || m >= dl.Len() {
					t.Fatalf("iter %d: push %d has match %d", iter, i, m)
				}
			} else if _, ok := popKind(c); ok {
				if len(stack) == 0 {
					t.Fatalf("iter %d: unbalanced pop at %d", iter, i)
				}
				push := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if dl.Match(i) != push || dl.Match(push) != i || dl.BoundsAt(push) != bb {
					t.Fatalf("iter %d: pop %d does not match push %d", iter, i, push)
				}
			} else if dl.Match(i) != -1 {
				t.Fatalf("iter %d: draw command %d has a match", iter, i)
			}
			for _, p := range stack {
				if !dl.BoundsAt(p).ContainsRect(bb) && p != i {
					t.Fatalf("iter %d: command %d bounds %v not inside block %d bounds %v\n%s", iter, i, bb, p, dl.BoundsAt(p), dl)
				}
			}
			// Everything recorded must be finite.
			switch c := c.(type) {
			case FillRect:
				if !c.Rect.IsFinite() {
					t.Fatalf("non-finite FillRect")
				}
			case FillPath:
				if !c.Path.IsFinite() {
					t.Fatalf("non-finite FillPath")
				}
			case PushTransform:
				if !c.Affine.IsFinite() || c.Affine.Det() == 0 {
					t.Fatalf("bad PushTransform %v", c.Affine)
				}
			}
		}
		if len(stack) != 0 {
			t.Fatalf("iter %d: %d blocks left open", iter, len(stack))
		}
		_ = dl.String()
		// Visit with the full bounds yields everything.
		n := 0
		dl.Visit(dl.Bounds().Outset(1), func(int, Command) { n++ })
		if n != dl.Len() {
			t.Fatalf("iter %d: Visit(bounds) yielded %d of %d commands", iter, n, dl.Len())
		}
	}
}

func TestDisplayListConcurrentReaders(t *testing.T) {
	dl := build(func(b *Builder) {
		for i := 0; i < 50; i++ {
			b.PushTransform(Translate(float32(i), 0))
			b.FillRRect(NewRRect(Rect{0, 0, 10, 10}, 3), RGB8(uint8(i), 0, 0))
			b.PopTransform()
		}
	})
	want := dl.String()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 20; k++ {
				if dl.String() != want {
					t.Error("concurrent read mismatch")
				}
				dl.Visit(Rect{0, 0, 30, 30}, func(int, Command) {})
			}
		}()
	}
	wg.Wait()
}

// blobPath is a closed path with integer coordinates and all curve kinds, so
// that the golden text does not depend on trigonometry or on the CPU's
// floating-point fusing.
func blobPath() Path {
	var p Path
	p.MoveTo(Pt(35, 200))
	p.CubicTo(Pt(35, 180), Pt(50, 175), Pt(60, 175))
	p.QuadTo(Pt(85, 175), Pt(85, 200))
	p.LineTo(Pt(80, 220))
	p.CubicTo(Pt(70, 230), Pt(50, 230), Pt(40, 220))
	p.Close()
	return p
}

// goldenList exercises every command and paint.
func goldenList() *DisplayList {
	b := NewBuilder()
	card := NewRRect(Rect{20, 20, 220, 140}, 12)
	b.DrawShadow(DrawShadow{RRect: card, Offset: Pt(0, 8), Blur: 24, Spread: -4, Color: Black.WithAlpha(0.25)})
	b.FillRRect(card, LinearGradient{
		Start: Pt(20, 20), End: Pt(220, 140),
		Stops: []GradientStop{AutoStop(OKLCH(0.585, 0.233, 277.117, 1)), AutoStop(OKLCH(0.591, 0.293, 322.896, 1))},
	})
	b.DrawShadow(DrawShadow{RRect: card, Offset: Pt(0, 2), Blur: 6, Color: White.WithAlpha(0.5), Inset: true})
	b.PushClipRRect(card)
	b.FillRect(Rect{20, 100, 220, 140}, RadialGradient{
		Center: Pt(120, 140), Radius: Pt(100, 40), Interp: InterpSRGB,
		Stops: []GradientStop{{0, White.WithAlpha(0.6)}, {1, Transparent}},
	})
	b.PushTransform(Affine{0.5, 0.25, -0.25, 0.5, 120, 80}) // exact in binary: golden stays stable across CPUs
	b.FillRect(Rect{-30, -10, 30, 10}, ConicGradient{
		Center: Pt(0, 0), StartAngle: math.Pi / 2,
		Stops: []GradientStop{{0, RGB8(255, 0, 0)}, {0.5, RGB8(0, 255, 0)}, {0.5, RGB8(0, 0, 255)}, {1, RGB8(255, 0, 0)}},
	})
	b.PopTransform()
	b.PopClip()

	var star Path
	star.MoveTo(Pt(240, 30))
	star.LineTo(Pt(250, 60))
	star.QuadTo(Pt(270, 50), Pt(280, 30))
	star.CubicTo(Pt(285, 70), Pt(250, 90), Pt(240, 30))
	star.Close()
	b.FillPath(star, EvenOdd, RGB8(250, 204, 21))
	style := StrokeStyle{Width: 2.5, Join: JoinRound, Cap: CapRound, Dashes: []float32{6, 3, 2}, DashOffset: 1}
	b.StrokePath(star, style, MustParseHex("#1e1e2e"))

	img := NewImagePremul(2, 2, []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255})
	b.DrawImage(img, Rect{}, Rect{240, 100, 280, 140}, SampleNearest)
	b.DrawImage(img, Rect{0, 0, 1, 2}, Rect{240, 150, 260, 190}, SampleLinear)

	b.PushLayer(0.8, Multiply, Blur{3}, Brightness(1.2), DropShadow{Offset: Pt(2, 3), StdDev: 4, Color: Black.WithAlpha(0.4)})
	b.FillPath(blobPath(), NonZero, Color{0.2, 0.4, 0.8, 1})
	b.PopLayer()

	b.PushClipPath(blobPath().Transform(Translate(90, 0)), NonZero)
	b.BackdropFilter(RRectCorners(Rect{110, 180, 190, 220}, Pt(8, 8), Pt(8, 8), Pt(0, 0), Pt(20, 10)), Blur{6}, Contrast(1.5))
	b.PopClip()
	return b.Finish()
}

func TestDisplayListGolden(t *testing.T) {
	got := goldenList().String()
	path := filepath.Join("testdata", "displaylist.golden.txt")
	if os.Getenv("TELA_UPDATE") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (generate it with TELA_UPDATE=1 go test ./internal/gfx/)", err)
	}
	if got != string(want) {
		t.Errorf("DisplayList dump differs from %s (rerun with TELA_UPDATE=1 to regenerate and review the diff).\n--- got\n%s--- want\n%s", path, got, want)
	}
}

func TestDumpFormat(t *testing.T) {
	dl := build(func(b *Builder) {
		b.PushClipRect(Rect{0, 0, 100, 50})
		b.FillRect(Rect{0, 0, 100.5, 0.1}, MustParseHex("#ff0000"))
		b.PopClip()
	})
	want := "PushClipRect rect=[0,0,100,50] bounds=[0,0,100,0.1]\n" +
		"  FillRect rect=[0,0,100.5,0.1] paint=#ff0000 bounds=[0,0,100,0.1]\n" +
		"PopClip\n"
	if got := dl.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// Dump is deterministic across calls and writes through io.Writer.
	var sb strings.Builder
	if err := dl.Dump(&sb); err != nil || sb.String() != want {
		t.Errorf("Dump = %q, %v", sb.String(), err)
	}
	if fmtF(float32(math.Copysign(0, -1))) != "0" || fmtF(1.0/3) != "0.33333334" || fmtF(100) != "100" || fmtF(-2.5) != "-2.5" {
		t.Error("fmtF not compact")
	}
}
