package svg

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// PaintKind is the kind of a root paint override (Root.Fill, Root.Stroke).
type PaintKind uint8

const (
	// PaintUnset leaves the document's own paint. It is the zero value.
	PaintUnset PaintKind = iota
	// PaintNone paints nothing (fill-none, stroke-none).
	PaintNone
	// PaintColor paints Paint.Color (fill-red-500).
	PaintColor
	// PaintCurrentColor paints the element's currentColor (fill-current).
	PaintCurrentColor
)

// Paint is a root paint override.
type Paint struct {
	Kind  PaintKind
	Color gfx.Color
}

// Root holds declarations for the root svg element that come from author
// CSS on it (in Tela, classes on the element that shows the document):
// they beat the root's presentation attributes and style attribute, and
// are inherited by every element that does not set the property itself,
// as CSS would do. Zero values leave the document unchanged.
type Root struct {
	Fill, Stroke Paint
	// StrokeWidth, when HasStrokeWidth, is the stroke width in user units
	// (stroke-[1.5]).
	StrokeWidth    float32
	HasStrokeWidth bool
	// Color, when HasColor, is the root's color property, which
	// currentColor takes (text-red-500 on the element).
	Color    gfx.Color
	HasColor bool
}

// Options are the per-draw inputs of a document. Changing them does not
// need a new Parse. Options is comparable, so it can key a cache of
// recorded lists (Doc.Record).
type Options struct {
	// CurrentColor is the color the root inherits from its context (the
	// CSS color of the element that shows the document): currentColor
	// unless the root sets color itself or Root.HasColor is set. The zero
	// value is transparent: pass the context's color.
	CurrentColor gfx.Color
	// Root overrides inherited properties on the root element.
	Root Root
}

// drawer emits a render tree into a Builder.
type drawer struct {
	b    *gfx.Builder
	opts *Options
	rc   gfx.Color // the root's color: currentColor where it is inherited
	// defaultRC is the root color the mask lists were recorded with.
	defaultRC gfx.Color
}

// record draws n into a new list, with the root color rc. defaultRC is
// the root color the precomputed mask lists were recorded with.
func record(n *node, opts *Options, rc, defaultRC gfx.Color) *gfx.DisplayList {
	b := gfx.NewBuilder()
	dr := drawer{b: b, opts: opts, rc: rc, defaultRC: defaultRC}
	dr.node(n)
	return b.Finish()
}

// shapePaint is a paint after the overrides, ready to draw.
type shapePaint struct {
	kind  paintKind // paintNone, paintColor (color) or paintURL (grad)
	color gfx.Color
	grad  *gradRes
}

// effective returns the paint p of sh after the root override ov.
func (dr *drawer) effective(sh *shape, p *paintRes, rootBit uint8, ov Paint) shapePaint {
	kind, col := p.kind, p.color
	if sh.fromRoot&rootBit != 0 && ov.Kind != PaintUnset {
		switch ov.Kind {
		case PaintNone:
			kind = paintNone
		case PaintColor:
			kind, col = paintColor, ov.Color
		default:
			kind = paintCurrent
		}
	}
	switch kind {
	case paintColor:
		col.A *= p.alpha
		if !(col.A > 0) {
			return shapePaint{}
		}
		return shapePaint{kind: paintColor, color: col}
	case paintCurrent:
		col = sh.color
		if sh.fromRoot&fromRootColor != 0 {
			col = dr.rc
		}
		col.A *= p.alpha
		if !(col.A > 0) {
			return shapePaint{}
		}
		return shapePaint{kind: paintColor, color: col}
	case paintURL:
		if p.grad != nil {
			return shapePaint{kind: paintURL, grad: p.grad}
		}
	}
	return shapePaint{}
}

// gradPaint returns the paint of g, with its stops rebuilt when they use
// the root's currentColor.
func (dr *drawer) gradPaint(g *gradRes) gfx.Paint {
	if g.tmpl == nil || dr.rc == dr.defaultRC {
		return g.paint
	}
	stops := finishStops(g.tmpl, dr.rc)
	switch p := g.paint.(type) {
	case gfx.LinearGradient:
		p.Stops = stops
		return p
	case gfx.RadialGradient:
		p.Stops = stops
		return p
	case gfx.FocalGradient:
		p.Stops = stops
		return p
	}
	return g.paint
}

func (dr *drawer) node(n *node) {
	b := dr.b
	var fill, stroke shapePaint
	var st gfx.StrokeStyle
	if n.kind == kindShape {
		sh := n.sh
		if sh.hidden {
			return
		}
		fill = dr.effective(sh, &sh.fill, fromRootFill, dr.opts.Root.Fill)
		stroke = dr.effective(sh, &sh.stroke, fromRootStroke, dr.opts.Root.Stroke)
		st = sh.stroke0
		if sh.fromRoot&fromRootStrokeWidth != 0 && dr.opts.Root.HasStrokeWidth {
			st.Width = dr.opts.Root.StrokeWidth
		}
		if !(st.Width > 0) {
			stroke = shapePaint{}
		}
		if fill.kind == paintNone && stroke.kind == paintNone {
			return
		}
	}
	// Order (CSS): transform, viewport clip, clip-path, then mask and
	// opacity in one layer.
	if n.hasT {
		b.PushTransform(n.t)
	}
	if n.hasClipRect {
		b.PushClipRect(n.clipRect)
	}
	pushClip(b, n.clip)
	opacity := n.opacity
	layer := false
	if n.mask != nil {
		b.PushMaskLayer(gfx.Mask{List: dr.maskList(n.mask), Mode: n.mask.mode}, opacity, gfx.SrcOver)
		layer, opacity = true, 1
	} else if opacity < 1 {
		// A shape with one solid paint folds its opacity into the color
		// (the same pixels, without a layer); anything else is a group.
		fold := n.kind == kindShape &&
			((fill.kind == paintColor && stroke.kind == paintNone) || (stroke.kind == paintColor && fill.kind == paintNone))
		if !fold {
			b.PushLayer(opacity, gfx.SrcOver)
			layer, opacity = true, 1
		}
	}
	if n.kind == kindShape {
		dr.shape(n.sh, fill, stroke, st, opacity)
	} else {
		for i := range n.children {
			dr.node(&n.children[i])
		}
	}
	if layer {
		b.PopLayer()
	}
	popClip(b, n.clip)
	if n.hasClipRect {
		b.PopClip()
	}
	if n.hasT {
		b.PopTransform()
	}
}

// shape draws the fill and the stroke of sh; opacity multiplies solid
// paints (the caller folded the node's opacity into them).
func (dr *drawer) shape(sh *shape, fill, stroke shapePaint, st gfx.StrokeStyle, opacity float32) {
	b := dr.b
	switch fill.kind {
	case paintColor:
		b.FillPath(sh.path, sh.rule, fill.color.MulAlpha(opacity))
	case paintURL:
		g := fill.grad
		if g.hasSpace {
			b.PushTransform(g.space)
			b.FillPath(g.fillPath, sh.rule, dr.gradPaint(g))
			b.PopTransform()
		} else {
			b.FillPath(sh.path, sh.rule, dr.gradPaint(g))
		}
	}
	switch stroke.kind {
	case paintColor:
		b.StrokePath(sh.path, st, stroke.color.MulAlpha(opacity))
	case paintURL:
		g := stroke.grad
		if !g.hasSpace {
			b.StrokePath(sh.path, st, dr.gradPaint(g))
			return
		}
		// The gradient lives in a space the stroke must not be distorted
		// by: paint it over the stroke's bounds, masked by the stroke.
		mb := gfx.NewBuilder()
		mb.StrokePath(sh.path, st, gfx.White)
		mask := mb.Finish()
		inv, ok := g.space.Invert()
		if !ok || mask.Len() == 0 {
			return
		}
		k := float32(math.Max(float64(st.MiterLimit), math.Sqrt2))
		local := sh.path.ControlBounds().Outset(st.Width / 2 * k)
		b.PushMaskLayer(gfx.Mask{List: mask, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
		b.PushTransform(g.space)
		b.FillRect(inv.TransformRect(local), dr.gradPaint(g))
		b.PopTransform()
		b.PopLayer()
	}
}

// maskList returns the recorded list of m, recording it again when the
// content depends on root properties that these Options change.
func (dr *drawer) maskList(m *maskRes) *gfx.DisplayList {
	r := &dr.opts.Root
	if (m.deps&fromRootFill != 0 && r.Fill.Kind != PaintUnset) ||
		(m.deps&fromRootStroke != 0 && r.Stroke.Kind != PaintUnset) ||
		(m.deps&fromRootStrokeWidth != 0 && r.HasStrokeWidth) ||
		(m.deps&fromRootColor != 0 && dr.rc != dr.defaultRC) {
		return record(&m.content, dr.opts, dr.rc, dr.defaultRC)
	}
	return m.list
}
