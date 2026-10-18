package gfx

import (
	"strings"
	"testing"
)

func maskOf(r Rect) *DisplayList {
	return build(func(b *Builder) { b.FillRect(r, Black) })
}

// TestPushMaskLayer checks the bounds and the dropping rules of mask
// layers.
func TestPushMaskLayer(t *testing.T) {
	m := maskOf(RectXYWH(10, 10, 20, 20))
	dl := build(func(b *Builder) {
		b.PushMaskLayer(Mask{List: m, Mode: MaskLuminance}, 0.5, Multiply)
		b.FillRect(RectXYWH(0, 0, 100, 100), White)
		b.PopLayer()
	})
	if dl.Len() != 3 {
		t.Fatalf("%d commands, want 3:\n%s", dl.Len(), dl)
	}
	c := dl.At(0).(PushLayer)
	if c.Mask.List != m || c.Mask.Mode != MaskLuminance || c.Opacity != 0.5 || c.Blend != Multiply {
		t.Errorf("push %+v", c)
	}
	// The layer shows only inside the mask, and its content is cut there.
	if got, want := dl.BoundsAt(0), RectXYWH(10, 10, 20, 20); got != want {
		t.Errorf("layer bounds %v, want the mask's %v", got, want)
	}
	if got, want := dl.BoundsAt(1), RectXYWH(10, 10, 20, 20); got != want {
		t.Errorf("content bounds %v, want %v", got, want)
	}
	// Under a transform the mask is in the local space.
	dl = build(func(b *Builder) {
		b.PushTransform(Translate(100, 0))
		b.PushMaskLayer(Mask{List: m}, 1, SrcOver)
		b.FillRect(RectXYWH(0, 0, 100, 100), White)
		b.PopLayer()
		b.PopTransform()
	})
	if got, want := dl.BoundsAt(0), RectXYWH(110, 10, 20, 20); got != want {
		t.Errorf("transformed layer bounds %v, want %v", got, want)
	}
	// With filters, the content is kept within their reach of the mask.
	dl = build(func(b *Builder) {
		b.PushMaskLayer(Mask{List: m}, 1, SrcOver, Blur{StdDev: 2})
		b.FillRect(RectXYWH(0, 0, 100, 100), White)
		b.PopLayer()
	})
	if got, want := dl.BoundsAt(1), RectXYWH(4, 4, 32, 32); got != want {
		t.Errorf("filtered content bounds %v, want the mask's grown by 3 sigma %v", got, want)
	}
	// Nil, empty and invisible masks drop the block.
	for name, draw := range map[string]func(b *Builder){
		"nil":     func(b *Builder) { b.PushMaskLayer(Mask{}, 1, SrcOver) },
		"empty":   func(b *Builder) { b.PushMaskLayer(Mask{List: build(func(*Builder) {})}, 1, SrcOver) },
		"clipped": func(b *Builder) { b.PushClipRect(RectXYWH(50, 50, 10, 10)); b.PushMaskLayer(Mask{List: m}, 1, SrcOver) },
		"opacity": func(b *Builder) { b.PushMaskLayer(Mask{List: m}, 0, SrcOver) },
	} {
		dl := build(func(b *Builder) {
			draw(b)
			b.FillRect(RectXYWH(0, 0, 100, 100), White)
			b.PopLayer()
			if name == "clipped" {
				b.PopClip()
			}
		})
		if dl.Len() != 0 {
			t.Errorf("%s mask: %d commands, want the block dropped:\n%s", name, dl.Len(), dl)
		}
	}
	mustPanic(t, "bad mask mode", []string{"MaskMode(7)"}, func() {
		NewBuilder().PushMaskLayer(Mask{List: m, Mode: 7}, 1, SrcOver)
	})
}

// TestMaskEqualAppendDump checks that masks take part in Equal, survive
// AppendList and show in Dump.
func TestMaskEqualAppendDump(t *testing.T) {
	a := PushLayer{Opacity: 1, Mask: Mask{List: maskOf(RectXYWH(0, 0, 5, 5))}}
	b := PushLayer{Opacity: 1, Mask: Mask{List: maskOf(RectXYWH(0, 0, 5, 5))}}
	c := PushLayer{Opacity: 1, Mask: Mask{List: maskOf(RectXYWH(0, 0, 6, 5))}}
	d := PushLayer{Opacity: 1, Mask: Mask{List: b.Mask.List, Mode: MaskLuminance}}
	if !Equal(a, b) || Equal(a, c) || Equal(b, d) || Equal(a, PushLayer{Opacity: 1}) {
		t.Error("Equal on masks: equal lists must be equal, other lists or modes not")
	}
	src := build(func(bb *Builder) {
		bb.PushMaskLayer(a.Mask, 1, Screen)
		bb.FillRect(RectXYWH(0, 0, 10, 10), White)
		bb.PopLayer()
	})
	out := build(func(bb *Builder) {
		bb.PushTransform(Translate(20, 0))
		bb.AppendList(src, nil)
		bb.PopTransform()
	})
	if out.Len() != 5 || !Equal(out.At(1), src.At(0)) || out.BoundsAt(1) != RectXYWH(20, 0, 5, 5) {
		t.Errorf("appended mask layer: %d commands, bounds %v:\n%s", out.Len(), out.BoundsAt(1), out)
	}
	if s := src.String(); !strings.Contains(s, "blend=screen mask=alpha(1 cmds [0,0,5,5])") {
		t.Errorf("dump:\n%s", s)
	}
}

// TestFocalGradientSanitize checks the Builder's handling of focal
// gradients.
func TestFocalGradientSanitize(t *testing.T) {
	st := []GradientStop{{Offset: 0, Color: White}, {Offset: 1, Color: Black}}
	dl := build(func(b *Builder) {
		b.FillRect(RectXYWH(0, 0, 10, 10), FocalGradient{Focal: Pt(1, 1), FocalRadius: -3, Center: Pt(5, 5), Radius: 4, Stops: st})
		// Equal circles paint nothing.
		b.FillRect(RectXYWH(0, 0, 10, 10), FocalGradient{Focal: Pt(5, 5), FocalRadius: 2, Center: Pt(5, 5), Radius: 2, Stops: st})
		b.FillRect(RectXYWH(0, 0, 10, 10), FocalGradient{Focal: Pt(5, 5), Center: Pt(nan32(), 5), Radius: 2, Stops: st})
	})
	if dl.Len() != 1 {
		t.Fatalf("%d commands, want 1:\n%s", dl.Len(), dl)
	}
	g := dl.At(0).(FillRect).Paint.(FocalGradient)
	if g.FocalRadius != 0 || g.Radius != 4 {
		t.Errorf("radii %v %v, want the negative one clamped to 0", g.FocalRadius, g.Radius)
	}
	if !PaintEqual(g, g) || PaintEqual(g, FocalGradient{Focal: g.Focal, Center: g.Center, Radius: 5, Stops: g.Stops}) {
		t.Error("PaintEqual on focal gradients")
	}
	if s := dl.String(); !strings.Contains(s, "focal(f=(1,1) fr=0 c=(5,5) r=4 oklab") {
		t.Errorf("dump:\n%s", s)
	}
}

func nan32() float32 {
	var z float32
	return z / z
}
