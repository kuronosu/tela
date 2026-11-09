package core

import (
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
// records with style b: every field is equal, slices and pointers by
// identity (copies of a style share them, and styles are not changed
// after they are made, so a shared array is the same value), except, for
// a deferred layer (layer), its opacity and transform, which the
// compositor applies (layer.go) and its list does not read. The
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
	return a.Display == b.Display && a.Position == b.Position && a.Inset == b.Inset &&
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
		sameSlice(a.GridAutoColumns, b.GridAutoColumns) && sameSlice(a.GridAutoRows, b.GridAutoRows) &&
		a.GridAutoFlow == b.GridAutoFlow && a.GridColumn == b.GridColumn && a.GridRow == b.GridRow &&
		a.BackgroundColor == b.BackgroundColor && sameSlice(a.Backgrounds, b.Backgrounds) &&
		a.BackgroundClip == b.BackgroundClip && a.BackgroundBlend == b.BackgroundBlend &&
		a.Border == b.Border && a.Radius == b.Radius && a.Outline == b.Outline && a.Divide == b.Divide &&
		sameSlice(a.BoxShadows, b.BoxShadows) && a.MixBlendMode == b.MixBlendMode &&
		sameFilters(&a.Filter, &b.Filter) && sameFilters(&a.BackdropFilter, &b.BackdropFilter) &&
		sameClipPath(&a.ClipPath, &b.ClipPath) && sameMask(&a.Mask, &b.Mask) &&
		a.Fill == b.Fill && a.Stroke == b.Stroke && a.StrokeWidth == b.StrokeWidth &&
		a.StrokeWidthSet == b.StrokeWidthSet &&
		a.ObjectFit == b.ObjectFit && a.ObjectPosX == b.ObjectPosX && a.ObjectPosY == b.ObjectPosY &&
		a.Transition == b.Transition && a.Animation == b.Animation &&
		a.AnimationDelay == b.AnimationDelay && a.AnimationDelaySet == b.AnimationDelaySet &&
		a.Cursor == b.Cursor && a.PointerEvents == b.PointerEvents && a.UserSelect == b.UserSelect &&
		sameText(&a.Text, &b.Text) && a.TextOverflow == b.TextOverflow && a.LineClamp == b.LineClamp &&
		a.VerticalAlign == b.VerticalAlign &&
		a.Caret == b.Caret && a.FieldSizing == b.FieldSizing &&
		a.Placeholder == b.Placeholder && a.Selection == b.Selection &&
		sameSlice(a.Deferred, b.Deferred) &&
		a.TextColorSet == b.TextColorSet && a.FontSizeSet == b.FontSizeSet &&
		a.CurrentColors == b.CurrentColors && a.DecorationInherited == b.DecorationInherited &&
		a.Scheme == b.Scheme
}

// sameSlice reports whether a and b are the same slice: the same length
// and, when not empty, the same array.
func sameSlice[T any](a, b []T) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func sameTracks(a, b *style.TrackList) bool {
	return sameSlice(a.Entries, b.Entries) && sameSlice(a.LineNames, b.LineNames)
}

func sameFilters(a, b *style.FilterSet) bool {
	return a.Set == b.Set && a.Blur == b.Blur && a.Brightness == b.Brightness && a.Contrast == b.Contrast &&
		a.Grayscale == b.Grayscale && a.HueRotate == b.HueRotate && a.Invert == b.Invert &&
		a.Saturate == b.Saturate && a.Sepia == b.Sepia && sameSlice(a.DropShadow, b.DropShadow)
}

func sameClipPath(a, b *style.ClipPath) bool {
	return a.Shape == b.Shape && sameSlice(a.Values, b.Values) && a.Radius == b.Radius && a.Rule == b.Rule
}

func sameMask(a, b *style.Mask) bool {
	return sameSlice(a.Layers, b.Layers) && a.Mode == b.Mode && a.Composite == b.Composite &&
		a.Clip == b.Clip && a.NoClip == b.NoClip
}

func sameText(a, b *style.TextProps) bool {
	return a.Color == b.Color && sameSlice(a.FontFamily, b.FontFamily) && a.FontSize == b.FontSize &&
		a.FontWeight == b.FontWeight && a.FontStyle == b.FontStyle && a.FontStretch == b.FontStretch &&
		a.LineHeight == b.LineHeight && a.LetterSpacing == b.LetterSpacing && a.WordSpacing == b.WordSpacing &&
		a.TextAlign == b.TextAlign && a.TextTransform == b.TextTransform && a.Decoration == b.Decoration &&
		a.WhiteSpace == b.WhiteSpace && a.WrapStyle == b.WrapStyle && a.WordBreak == b.WordBreak &&
		a.OverflowWrap == b.OverflowWrap && a.TabSize == b.TabSize && a.Numeric == b.Numeric &&
		sameSlice(a.Shadow, b.Shadow) && a.Direction == b.Direction
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
