package gfx

import "slices"

// Equal reports whether two commands are the same command with the same
// values: drawn in the same state, they paint the same pixels. Slices
// (paths, stops, dashes, filters, glyphs) are compared element by element,
// after a fast path for shared ones; images and fonts by identity.
func Equal(a, b Command) bool {
	switch a := a.(type) {
	case FillRect:
		b, ok := b.(FillRect)
		return ok && a.Rect == b.Rect && PaintEqual(a.Paint, b.Paint)
	case FillRRect:
		b, ok := b.(FillRRect)
		return ok && a.RRect == b.RRect && PaintEqual(a.Paint, b.Paint)
	case FillPath:
		b, ok := b.(FillPath)
		return ok && a.Rule == b.Rule && pathEqual(a.Path, b.Path) && PaintEqual(a.Paint, b.Paint)
	case StrokePath:
		b, ok := b.(StrokePath)
		return ok && strokeEqual(&a.Style, &b.Style) && pathEqual(a.Path, b.Path) && PaintEqual(a.Paint, b.Paint)
	case DrawImage:
		b, ok := b.(DrawImage)
		return ok && a == b
	case DrawShadow:
		b, ok := b.(DrawShadow)
		return ok && a == b
	case DrawGlyphRun:
		b, ok := b.(DrawGlyphRun)
		return ok && a.Font.FontKey() == b.Font.FontKey() && a.Size == b.Size && a.Origin == b.Origin &&
			sameSlice(a.Glyphs, b.Glyphs) && PaintEqual(a.Paint, b.Paint)
	case PushClipRect:
		b, ok := b.(PushClipRect)
		return ok && a == b
	case PushClipRRect:
		b, ok := b.(PushClipRRect)
		return ok && a == b
	case PushClipPath:
		b, ok := b.(PushClipPath)
		return ok && a.Rule == b.Rule && pathEqual(a.Path, b.Path)
	case PushTransform:
		b, ok := b.(PushTransform)
		return ok && a == b
	case PushLayer:
		b, ok := b.(PushLayer)
		return ok && a.Opacity == b.Opacity && a.Blend == b.Blend && sameSlice(a.Filters, b.Filters)
	case BackdropFilter:
		b, ok := b.(BackdropFilter)
		return ok && a.Shape == b.Shape && sameSlice(a.Filters, b.Filters)
	case PopClip:
		_, ok := b.(PopClip)
		return ok
	case PopTransform:
		_, ok := b.(PopTransform)
		return ok
	case PopLayer:
		_, ok := b.(PopLayer)
		return ok
	case Marker:
		b, ok := b.(Marker)
		return ok && a == b
	}
	return false
}

// PaintEqual reports whether two paints are the same paint.
func PaintEqual(a, b Paint) bool {
	switch a := a.(type) {
	case Color:
		b, ok := b.(Color)
		return ok && a == b
	case LinearGradient:
		b, ok := b.(LinearGradient)
		return ok && a.Start == b.Start && a.End == b.End && a.Interp == b.Interp && sameSlice(a.Stops, b.Stops)
	case RadialGradient:
		b, ok := b.(RadialGradient)
		return ok && a.Center == b.Center && a.Radius == b.Radius && a.Interp == b.Interp && sameSlice(a.Stops, b.Stops)
	case ConicGradient:
		b, ok := b.(ConicGradient)
		return ok && a.Center == b.Center && a.StartAngle == b.StartAngle && a.Interp == b.Interp && sameSlice(a.Stops, b.Stops)
	}
	return false
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
