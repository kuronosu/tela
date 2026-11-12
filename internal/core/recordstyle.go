package core

import (
	"image"
	"reflect"
	"slices"

	"github.com/kuronosu/tela/internal/style"
)

// The style a box's list reads (record.go): paintIn holds the box's style
// by identity, and two styles of different identity are compared by
// content here. It is the one place that decides whether a new style
// changes what a box records: a restyle that computes an equal style, the
// steps of an animation (each a new copy of the target, anim.go), a
// layout animation's corrected copy (layoutanim.go).

// sameStyle reports whether a box painted with style a records what it
// records with style b: every field is equal, except, for a deferred
// layer (layer), its opacity and transform, which the compositor applies
// (layer.go) and its list does not read. Slices and pointers are equal
// when they are the same (copies of a style share them, and styles are
// not changed after they are made, so a shared array is the same value);
// those that a style computed again makes anew (the background and mask
// layers, the shadows, the font families, a clip path's values, the
// placeholder and selection colors) are compared by content, and the
// deferred props by what they are (sameDeferred), so that an element
// built again with equal classes and …Fn props records nothing. The
// comparison covers every field of style.ComputedStyle (a new field must
// be added here: TestSameStyleCoversEveryField).
func sameStyle(a, b *style.ComputedStyle, layer bool) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !layer && (a.Opacity != b.Opacity || a.Transform != b.Transform) {
		return false
	}
	return a.Display == b.Display && a.Position == b.Position && a.InlineOrigin == b.InlineOrigin && a.Inset == b.Inset &&
		a.ZIndex == b.ZIndex && a.Isolate == b.Isolate &&
		a.OverflowX == b.OverflowX && a.OverflowY == b.OverflowY && a.Visibility == b.Visibility &&
		a.ScrollBehavior == b.ScrollBehavior && a.Scrollbar == b.Scrollbar &&
		a.OverscrollX == b.OverscrollX && a.OverscrollY == b.OverscrollY &&
		a.Width == b.Width && a.Height == b.Height && a.MinWidth == b.MinWidth &&
		a.MinHeight == b.MinHeight && a.MaxWidth == b.MaxWidth && a.MaxHeight == b.MaxHeight &&
		a.AspectRatio == b.AspectRatio &&
		a.Margin == b.Margin && a.Padding == b.Padding && a.RowGap == b.RowGap &&
		a.ColumnGap == b.ColumnGap && a.SpaceX == b.SpaceX && a.SpaceY == b.SpaceY &&
		a.FlexDirection == b.FlexDirection && a.FlexWrap == b.FlexWrap && a.FlexGrow == b.FlexGrow &&
		a.FlexShrink == b.FlexShrink && a.FlexBasis == b.FlexBasis && a.Order == b.Order &&
		a.AlignItems == b.AlignItems && a.AlignSelf == b.AlignSelf && a.AlignContent == b.AlignContent &&
		a.JustifyContent == b.JustifyContent && a.JustifyItems == b.JustifyItems &&
		a.JustifySelf == b.JustifySelf &&
		sameTracks(&a.GridTemplateColumns, &b.GridTemplateColumns) &&
		sameTracks(&a.GridTemplateRows, &b.GridTemplateRows) &&
		sameAreas(&a.GridTemplateAreas, &b.GridTemplateAreas) &&
		slices.Equal(a.GridAutoColumns, b.GridAutoColumns) && slices.Equal(a.GridAutoRows, b.GridAutoRows) &&
		a.GridAutoFlow == b.GridAutoFlow && a.GridColumn == b.GridColumn && a.GridRow == b.GridRow &&
		a.BackgroundColor == b.BackgroundColor && sameLayers(a.Backgrounds, b.Backgrounds) &&
		a.BackgroundClip == b.BackgroundClip && a.BackgroundBlend == b.BackgroundBlend &&
		a.Border == b.Border && a.Radius == b.Radius && a.Outline == b.Outline && a.Divide == b.Divide &&
		slices.Equal(a.BoxShadows, b.BoxShadows) && a.MixBlendMode == b.MixBlendMode &&
		sameFilters(&a.Filter, &b.Filter) && sameFilters(&a.BackdropFilter, &b.BackdropFilter) &&
		sameClipPath(&a.ClipPath, &b.ClipPath) && sameMask(&a.Mask, &b.Mask) &&
		a.Fill == b.Fill && a.Stroke == b.Stroke && a.StrokeWidth == b.StrokeWidth &&
		a.StrokeWidthSet == b.StrokeWidthSet &&
		a.ObjectFit == b.ObjectFit && a.ObjectPosX == b.ObjectPosX && a.ObjectPosY == b.ObjectPosY &&
		a.Transition == b.Transition && a.Animation == b.Animation &&
		a.AnimationDelay == b.AnimationDelay && a.AnimationDelaySet == b.AnimationDelaySet && a.AnimProps == b.AnimProps &&
		a.Cursor == b.Cursor && a.PointerEvents == b.PointerEvents && a.UserSelect == b.UserSelect &&
		sameText(&a.Text, &b.Text) && a.TextOverflow == b.TextOverflow && a.LineClamp == b.LineClamp &&
		a.VerticalAlign == b.VerticalAlign &&
		a.Caret == b.Caret && a.FieldSizing == b.FieldSizing &&
		samePtr(a.Placeholder, b.Placeholder, sameText) && samePtr(a.Selection, b.Selection, sameSelection) &&
		sameDeferred(a.Deferred, b.Deferred) &&
		a.TextColorSet == b.TextColorSet && a.FontSizeSet == b.FontSizeSet &&
		a.CurrentColors == b.CurrentColors && a.DecorationInherited == b.DecorationInherited &&
		a.Scheme == b.Scheme
}

// sameSlice reports whether a and b are the same slice: the same length
// and, when not empty, the same array.
func sameSlice[T any](a, b []T) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// samePtr reports whether a and b are both nil, or point to values that
// same reports equal.
func samePtr[T any](a, b *T, same func(a, b *T) bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a == b || same(a, b)
}

func sameSelection(a, b *style.SelectionColors) bool { return *a == *b }

// sameDeferred reports whether two lists of deferred props are the same
// props: their closures differ whenever an element builds again, and what
// they give is compared where it is read (paintIn: the resolved colors and
// gradient; the layout; the compositor's translation and opacity), so
// only their phases and kinds count here.
func sameDeferred(a, b []style.Deferred) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := &a[i], &b[i]
		if x.Phase != y.Phase || x.TextColor != y.TextColor || (x.Apply == nil) != (y.Apply == nil) {
			return false
		}
	}
	return true
}

// sameLayers reports whether two lists of background (or mask) layers
// paint the same: equal fields, gradients by content (sameGradient), and
// decoded images the same image.
func sameLayers(a, b []style.BackgroundLayer) bool {
	if sameSlice(a, b) {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := &a[i], &b[i]
		if x.URL != y.URL || x.Size != y.Size || x.PosX != y.PosX || x.PosY != y.PosY || x.Repeat != y.Repeat ||
			x.Origin != y.Origin || !sameGradient(x.Gradient, y.Gradient) || !sameImage(x.Image, y.Image) {
			return false
		}
	}
	return true
}

// sameImage reports whether a and b are the same image (nil for none). An
// image whose dynamic type is not comparable is only the same as itself
// when both are nil: it counts as a change.
func sameImage(a, b image.Image) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	return ta == tb && ta.Comparable() && a == b
}

func sameTracks(a, b *style.TrackList) bool {
	return sameSlice(a.Entries, b.Entries) && sameSlice(a.LineNames, b.LineNames)
}

func sameAreas(a, b *style.GridAreas) bool {
	return sameSlice(a.Areas, b.Areas) && sameSlice(a.Template, b.Template) && a.Rows == b.Rows && a.Columns == b.Columns
}

func sameFilters(a, b *style.FilterSet) bool {
	return a.Set == b.Set && a.Blur == b.Blur && a.Brightness == b.Brightness && a.Contrast == b.Contrast &&
		a.Grayscale == b.Grayscale && a.HueRotate == b.HueRotate && a.Invert == b.Invert &&
		a.Saturate == b.Saturate && a.Sepia == b.Sepia && slices.Equal(a.DropShadow, b.DropShadow)
}

func sameClipPath(a, b *style.ClipPath) bool {
	return a.Shape == b.Shape && slices.Equal(a.Values, b.Values) && a.Radius == b.Radius && a.Rule == b.Rule
}

func sameMask(a, b *style.Mask) bool {
	return sameLayers(a.Layers, b.Layers) && a.Mode == b.Mode && a.Composite == b.Composite &&
		a.Clip == b.Clip && a.NoClip == b.NoClip
}

func sameText(a, b *style.TextProps) bool {
	return a.Color == b.Color && slices.Equal(a.FontFamily, b.FontFamily) && a.FontSize == b.FontSize &&
		a.FontWeight == b.FontWeight && a.FontStyle == b.FontStyle && a.FontStretch == b.FontStretch &&
		a.LineHeight == b.LineHeight && a.LetterSpacing == b.LetterSpacing && a.WordSpacing == b.WordSpacing &&
		a.TextAlign == b.TextAlign && a.TextTransform == b.TextTransform && a.Decoration == b.Decoration &&
		a.WhiteSpace == b.WhiteSpace && a.WrapStyle == b.WrapStyle && a.WordBreak == b.WordBreak &&
		a.OverflowWrap == b.OverflowWrap && a.TabSize == b.TabSize && a.Numeric == b.Numeric &&
		slices.Equal(a.Shadow, b.Shadow) && a.Direction == b.Direction && a.Language == b.Language &&
		a.Hyphens == b.Hyphens
}

// same reports whether in and o read the same (the comparison of the
// tracker, record.go): equal, but for a style of another identity, which
// is compared by sameStyle.
func (in *paintIn) same(o *paintIn) bool {
	a, b := *in, *o
	if a.grad != b.grad {
		// A BackgroundGradientFn makes a gradient on every paint of its
		// box: an equal one records the same.
		if !sameGradient(a.grad, b.grad) {
			return false
		}
		a.grad, b.grad = nil, nil
	}
	if a.cs == b.cs {
		return a == b
	}
	a.cs, b.cs = nil, nil
	return a == b && sameStyle(in.cs, o.cs, in.flags&inDeferred != 0)
}

// sameGradient reports whether gradients a and b (nil for none) paint the
// same: equal fields and stops.
func sameGradient(a, b *style.Gradient) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Kind == b.Kind && a.Angle == b.Angle && a.Corner == b.Corner && a.Shape == b.Shape &&
		a.Size == b.Size && a.Radius == b.Radius && a.CenterX == b.CenterX && a.CenterY == b.CenterY &&
		a.Interp == b.Interp && slices.Equal(a.Stops, b.Stops)
}
