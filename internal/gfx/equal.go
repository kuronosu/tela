package gfx

import "slices"

// Equal reports whether two commands are the same command with the same
// values: drawn in the same state, they paint the same pixels. Slices
// (paths, stops, dashes, filters, glyphs) are compared element by element,
// after a fast path for shared ones; images and fonts by identity.
func Equal(a, b Command) bool {
	return a != nil && a.equal(b)
}

func (a FillRect) equal(c Command) bool {
	b, ok := c.(FillRect)
	return ok && a.Rect == b.Rect && PaintEqual(a.Paint, b.Paint)
}

func (a FillRRect) equal(c Command) bool {
	b, ok := c.(FillRRect)
	return ok && a.RRect == b.RRect && PaintEqual(a.Paint, b.Paint)
}

func (a FillPath) equal(c Command) bool {
	b, ok := c.(FillPath)
	return ok && a.Rule == b.Rule && pathEqual(a.Path, b.Path) && PaintEqual(a.Paint, b.Paint)
}

func (a StrokePath) equal(c Command) bool {
	b, ok := c.(StrokePath)
	return ok && strokeEqual(&a.Style, &b.Style) && pathEqual(a.Path, b.Path) && PaintEqual(a.Paint, b.Paint)
}

func (a DrawImage) equal(c Command) bool {
	b, ok := c.(DrawImage)
	return ok && a == b
}

func (a DrawShadow) equal(c Command) bool {
	b, ok := c.(DrawShadow)
	return ok && a == b
}

func (a DrawGlyphRun) equal(c Command) bool {
	b, ok := c.(DrawGlyphRun)
	return ok && a.Font.FontKey() == b.Font.FontKey() && a.Size == b.Size && a.Origin == b.Origin &&
		sameSlice(a.Glyphs, b.Glyphs) && PaintEqual(a.Paint, b.Paint)
}

func (a PushClipRect) equal(c Command) bool {
	b, ok := c.(PushClipRect)
	return ok && a == b
}

func (a PushClipRRect) equal(c Command) bool {
	b, ok := c.(PushClipRRect)
	return ok && a == b
}

func (a PushClipPath) equal(c Command) bool {
	b, ok := c.(PushClipPath)
	return ok && a.Rule == b.Rule && pathEqual(a.Path, b.Path)
}

func (a PushTransform) equal(c Command) bool {
	b, ok := c.(PushTransform)
	return ok && a == b
}

func (a PushLayer) equal(c Command) bool {
	b, ok := c.(PushLayer)
	return ok && a.Opacity == b.Opacity && a.Blend == b.Blend && sameSlice(a.Filters, b.Filters) &&
		a.Mask.Mode == b.Mask.Mode && listEqual(a.Mask.List, b.Mask.List)
}

func (a BackdropFilter) equal(c Command) bool {
	b, ok := c.(BackdropFilter)
	return ok && a.Shape == b.Shape && a.Opacity == b.Opacity && sameSlice(a.Filters, b.Filters)
}

func (PopClip) equal(c Command) bool {
	_, ok := c.(PopClip)
	return ok
}

func (PopTransform) equal(c Command) bool {
	_, ok := c.(PopTransform)
	return ok
}

func (PopLayer) equal(c Command) bool {
	_, ok := c.(PopLayer)
	return ok
}

func (a Marker) equal(c Command) bool {
	b, ok := c.(Marker)
	return ok && a == b
}

// PaintEqual reports whether two paints are the same paint.
func PaintEqual(a, b Paint) bool {
	switch a := a.(type) {
	case Color:
		b, ok := b.(Color)
		return ok && a == b
	case LinearGradient:
		b, ok := b.(LinearGradient)
		return ok && a.Start == b.Start && a.End == b.End && a.Interp == b.Interp && a.Spread == b.Spread && sameSlice(a.Stops, b.Stops)
	case RadialGradient:
		b, ok := b.(RadialGradient)
		return ok && a.Center == b.Center && a.Radius == b.Radius && a.Interp == b.Interp && a.Spread == b.Spread && sameSlice(a.Stops, b.Stops)
	case ConicGradient:
		b, ok := b.(ConicGradient)
		return ok && a.Center == b.Center && a.StartAngle == b.StartAngle && a.Interp == b.Interp && sameSlice(a.Stops, b.Stops)
	case FocalGradient:
		b, ok := b.(FocalGradient)
		return ok && a.Focal == b.Focal && a.FocalRadius == b.FocalRadius && a.Center == b.Center &&
			a.Radius == b.Radius && a.Interp == b.Interp && a.Spread == b.Spread && sameSlice(a.Stops, b.Stops)
	}
	return false
}

// listEqual reports whether two lists hold equal commands with equal
// bounds (the same pointer, or nil for both, is equal).
func listEqual(a, b *DisplayList) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || len(a.cmds) != len(b.cmds) {
		return false
	}
	for i := range a.cmds {
		if a.bounds[i] != b.bounds[i] || !Equal(a.cmds[i], b.cmds[i]) {
			return false
		}
	}
	return true
}

// sameSlice compares two slices of comparable values, element by element
// after checking whether they share their backing array.
func sameSlice[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 || &a[0] == &b[0] {
		return true
	}
	return slices.Equal(a, b)
}

func pathEqual(a, b Path) bool {
	return a.start == b.start && a.needMove == b.needMove && sameSlice(a.verbs, b.verbs) && sameSlice(a.pts, b.pts)
}

func strokeEqual(a, b *StrokeStyle) bool {
	return a.Width == b.Width && a.MiterLimit == b.MiterLimit && a.Join == b.Join && a.Cap == b.Cap &&
		a.DashOffset == b.DashOffset && sameSlice(a.Dashes, b.Dashes)
}
