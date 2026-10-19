package anim

import (
	"slices"

	"github.com/kuronosu/tela/internal/style"
)

// Prop is a group of animatable properties of a ComputedStyle that
// transitions treat as one unit. The groups are the bits of
// style.TransitionProps, in the same order, so the transition-property of a
// style maps to a set of Props with Props.
type Prop uint8

// The animatable property groups.
const (
	PropColor           Prop = iota // color (inherited)
	PropBackgroundColor             // background-color
	PropBorderColor                 // border-*-color
	PropOutlineColor                // outline-color
	PropDecorationColor             // text-decoration-color
	PropOpacity                     // opacity
	PropShadow                      // box-shadow
	PropTransform                   // translate, rotate, scale, skew
	PropFilter                      // filter
	PropBackdropFilter              // backdrop-filter
	PropWidth                       // width
	PropHeight                      // height
	PropSpacing                     // padding, margin, gap, space-x/y
	PropInset                       // top, right, bottom, left
	PropBorderRadius                // border-radius
	PropBorderWidth                 // border-width
	PropFontSize                    // font-size (inherited)
	PropClipPath                    // clip-path

	// NumProps is the number of groups.
	NumProps
)

var propNames = [...]string{"color", "background-color", "border-color", "outline-color",
	"text-decoration-color", "opacity", "box-shadow", "transform", "filter", "backdrop-filter",
	"width", "height", "spacing", "inset", "border-radius", "border-width", "font-size", "clip-path"}

// String returns the CSS name of the group.
func (p Prop) String() string {
	if p < NumProps {
		return propNames[p]
	}
	return "unknown"
}

// Mask returns the style.TransitionProps bit of the group.
func (p Prop) Mask() style.TransitionProps { return style.TransitionColor << p }

// Set returns the groups in a transition-property set, in order.
func Set(props style.TransitionProps) []Prop {
	var out []Prop
	for p := Prop(0); p < NumProps; p++ {
		if props&p.Mask() != 0 {
			out = append(out, p)
		}
	}
	return out
}

// Class says what a change of a property invalidates.
type Class uint8

const (
	// ClassLayer properties (opacity, transform) are applied when a layer
	// is composited: a change records nothing.
	ClassLayer Class = iota
	// ClassPaint properties (colors, radius) change what is recorded but
	// not its bounds.
	ClassPaint
	// ClassInk properties (shadows, filters) also change the bounds of
	// what the box paints.
	ClassInk
	// ClassLayout properties (sizes, spacing, insets, border and font
	// sizes) change the layout.
	ClassLayout
)

// Class returns what a change of p invalidates.
func (p Prop) Class() Class {
	switch p {
	case PropOpacity, PropTransform:
		return ClassLayer
	case PropShadow, PropFilter, PropBackdropFilter, PropClipPath:
		// A clip path also makes the box a stacking context or not.
		return ClassInk
	case PropWidth, PropHeight, PropSpacing, PropInset, PropBorderWidth, PropFontSize:
		return ClassLayout
	}
	return ClassPaint
}

// Inherited reports whether the property is inherited, so that children
// must be styled again when it changes.
func (p Prop) Inherited() bool {
	return p == PropColor || p == PropDecorationColor || p == PropFontSize
}

func sides[T any](s *style.Sides[T]) [4]*T {
	return [4]*T{&s.Top, &s.Right, &s.Bottom, &s.Left}
}

func corners[T any](c *style.Corners[T]) [4]*T {
	return [4]*T{&c.TopLeft, &c.TopRight, &c.BottomRight, &c.BottomLeft}
}

// Equal reports whether the property has the same value in a and b.
func Equal(p Prop, a, b *style.ComputedStyle) bool {
	switch p {
	case PropColor:
		return a.Text.Color == b.Text.Color
	case PropBackgroundColor:
		return a.BackgroundColor == b.BackgroundColor
	case PropBorderColor:
		return a.Border.Top.Color == b.Border.Top.Color && a.Border.Right.Color == b.Border.Right.Color &&
			a.Border.Bottom.Color == b.Border.Bottom.Color && a.Border.Left.Color == b.Border.Left.Color
	case PropOutlineColor:
		return a.Outline.Color == b.Outline.Color
	case PropDecorationColor:
		return a.Text.Decoration.Color == b.Text.Decoration.Color
	case PropOpacity:
		return a.Opacity == b.Opacity
	case PropShadow:
		return slices.Equal(a.BoxShadows, b.BoxShadows)
	case PropTransform:
		return a.Transform == b.Transform
	case PropFilter:
		return filtersEqual(&a.Filter, &b.Filter)
	case PropBackdropFilter:
		return filtersEqual(&a.BackdropFilter, &b.BackdropFilter)
	case PropWidth:
		return a.Width == b.Width
	case PropHeight:
		return a.Height == b.Height
	case PropSpacing:
		return a.Padding == b.Padding && a.Margin == b.Margin && a.RowGap == b.RowGap &&
			a.ColumnGap == b.ColumnGap && a.SpaceX == b.SpaceX && a.SpaceY == b.SpaceY
	case PropInset:
		return a.Inset == b.Inset
	case PropBorderRadius:
		return a.Radius == b.Radius
	case PropBorderWidth:
		return a.Border.Top.Width == b.Border.Top.Width && a.Border.Right.Width == b.Border.Right.Width &&
			a.Border.Bottom.Width == b.Border.Bottom.Width && a.Border.Left.Width == b.Border.Left.Width
	case PropFontSize:
		return a.Text.FontSize == b.Text.FontSize
	case PropClipPath:
		return a.ClipPath.Equal(&b.ClipPath)
	}
	return true
}

func filtersEqual(a, b *style.FilterSet) bool {
	return a.Set == b.Set && a.Blur == b.Blur && a.Brightness == b.Brightness && a.Contrast == b.Contrast &&
		a.Grayscale == b.Grayscale && a.HueRotate == b.HueRotate && a.Invert == b.Invert &&
		a.Saturate == b.Saturate && a.Sepia == b.Sepia && slices.Equal(a.DropShadow, b.DropShadow)
}

// CanInterpolate reports whether the values of p in a and b interpolate
// (CSS Transitions: "the before-change style is different from and can be
// interpolated with the after-change style"): a change between auto and a
// length, or between an inset and an outer shadow, does not.
func CanInterpolate(p Prop, a, b *style.ComputedStyle) bool {
	switch p {
	case PropShadow:
		_, _, ok := pairShadows(a.BoxShadows, b.BoxShadows)
		return ok
	case PropWidth:
		return a.Width.Kind == style.DimLength && b.Width.Kind == style.DimLength
	case PropHeight:
		return a.Height.Kind == style.DimLength && b.Height.Kind == style.DimLength
	case PropInset:
		for i, x := range sides(&a.Inset) {
			if x.Kind != style.DimLength || sides(&b.Inset)[i].Kind != style.DimLength {
				return false
			}
		}
	case PropClipPath:
		return a.ClipPath.Interpolable(&b.ClipPath)
	}
	return true
}

// Copy sets the property of dst to its value in src.
func Copy(p Prop, dst, src *style.ComputedStyle) {
	switch p {
	case PropColor:
		dst.Text.Color = src.Text.Color
	case PropBackgroundColor:
		dst.BackgroundColor = src.BackgroundColor
	case PropBorderColor:
		dst.Border.Top.Color, dst.Border.Right.Color = src.Border.Top.Color, src.Border.Right.Color
		dst.Border.Bottom.Color, dst.Border.Left.Color = src.Border.Bottom.Color, src.Border.Left.Color
	case PropOutlineColor:
		dst.Outline.Color = src.Outline.Color
	case PropDecorationColor:
		dst.Text.Decoration.Color = src.Text.Decoration.Color
	case PropOpacity:
		dst.Opacity = src.Opacity
	case PropShadow:
		dst.BoxShadows = src.BoxShadows
	case PropTransform:
		dst.Transform = src.Transform
	case PropFilter:
		dst.Filter = src.Filter
	case PropBackdropFilter:
		dst.BackdropFilter = src.BackdropFilter
	case PropWidth:
		dst.Width = src.Width
	case PropHeight:
		dst.Height = src.Height
	case PropSpacing:
		dst.Padding, dst.Margin = src.Padding, src.Margin
		dst.RowGap, dst.ColumnGap = src.RowGap, src.ColumnGap
		dst.SpaceX, dst.SpaceY = src.SpaceX, src.SpaceY
	case PropInset:
		dst.Inset = src.Inset
	case PropBorderRadius:
		dst.Radius = src.Radius
	case PropBorderWidth:
		dst.Border.Top.Width, dst.Border.Right.Width = src.Border.Top.Width, src.Border.Right.Width
		dst.Border.Bottom.Width, dst.Border.Left.Width = src.Border.Bottom.Width, src.Border.Left.Width
	case PropFontSize:
		dst.Text.FontSize = src.Text.FontSize
	case PropClipPath:
		dst.ClipPath = src.ClipPath
	}
}

// Blend sets the property of dst to its value between from and to, t of
// the way (t = 0 is from, t = 1 is to; the eased progress of a transition,
// which may overshoot [0, 1]). The other properties of dst are untouched.
// Values that do not interpolate switch at t = 0.5.
func Blend(p Prop, dst, from, to *style.ComputedStyle, t float64) {
	switch {
	case t == 0:
		Copy(p, dst, from)
		return
	case t == 1:
		Copy(p, dst, to)
		return
	}
	switch p {
	case PropColor:
		dst.Text.Color = LerpColor(from.Text.Color, to.Text.Color, t)
	case PropBackgroundColor:
		dst.BackgroundColor = LerpColor(from.BackgroundColor, to.BackgroundColor, t)
	case PropBorderColor:
		d, f, g := sides(&dst.Border), sides(&from.Border), sides(&to.Border)
		for i := range d {
			d[i].Color = LerpColor(f[i].Color, g[i].Color, t)
		}
	case PropOutlineColor:
		dst.Outline.Color = LerpColor(from.Outline.Color, to.Outline.Color, t)
	case PropDecorationColor:
		dst.Text.Decoration.Color = LerpColor(from.Text.Decoration.Color, to.Text.Decoration.Color, t)
	case PropOpacity:
		dst.Opacity = min(max(Lerp(from.Opacity, to.Opacity, t), 0), 1)
	case PropShadow:
		dst.BoxShadows, _ = LerpShadows(from.BoxShadows, to.BoxShadows, t)
	case PropTransform:
		dst.Transform = LerpTransform(from.Transform, to.Transform, t)
	case PropFilter:
		dst.Filter = LerpFilters(from.Filter, to.Filter, t)
	case PropBackdropFilter:
		dst.BackdropFilter = LerpFilters(from.BackdropFilter, to.BackdropFilter, t)
	case PropWidth:
		dst.Width, _ = LerpDim(from.Width, to.Width, t)
		dst.Width = nonNegative(dst.Width)
	case PropHeight:
		dst.Height, _ = LerpDim(from.Height, to.Height, t)
		dst.Height = nonNegative(dst.Height)
	case PropSpacing:
		blendSpacing(dst, from, to, t)
	case PropInset:
		d, f, g := sides(&dst.Inset), sides(&from.Inset), sides(&to.Inset)
		for i := range d {
			*d[i], _ = LerpDim(*f[i], *g[i], t)
		}
	case PropBorderRadius:
		d, f, g := corners(&dst.Radius), corners(&from.Radius), corners(&to.Radius)
		for i := range d {
			*d[i] = nonNegativeLP(LerpLP(*f[i], *g[i], t))
		}
	case PropBorderWidth:
		d, f, g := sides(&dst.Border), sides(&from.Border), sides(&to.Border)
		for i := range d {
			d[i].Width = max(Lerp(f[i].Width, g[i].Width, t), 0)
		}
	case PropFontSize:
		dst.Text.FontSize = max(Lerp(from.Text.FontSize, to.Text.FontSize, t), 0)
	case PropClipPath:
		dst.ClipPath = LerpClipPath(from.ClipPath, to.ClipPath, t)
	}
}

// LerpClipPath interpolates two clip paths of the same shape value by
// value (CSS Shapes 1: basic shapes of the same type with the same number
// of values and the same keywords interpolate); other pairs switch at t =
// 0.5.
func LerpClipPath(a, b style.ClipPath, t float64) style.ClipPath {
	if !a.Interpolable(&b) {
		if t < 0.5 {
			return a
		}
		return b
	}
	out := b
	out.Values = make([]style.LP, len(a.Values))
	for i := range out.Values {
		out.Values[i] = LerpLP(a.Values[i], b.Values[i], t)
	}
	return out
}

func blendSpacing(dst, from, to *style.ComputedStyle, t float64) {
	d, f, g := sides(&dst.Padding), sides(&from.Padding), sides(&to.Padding)
	for i := range d {
		*d[i] = nonNegativeLP(LerpLP(*f[i], *g[i], t))
	}
	dm, fm, gm := sides(&dst.Margin), sides(&from.Margin), sides(&to.Margin)
	for i := range dm {
		*dm[i], _ = LerpDim(*fm[i], *gm[i], t)
	}
	dst.RowGap = nonNegativeLP(LerpLP(from.RowGap, to.RowGap, t))
	dst.ColumnGap = nonNegativeLP(LerpLP(from.ColumnGap, to.ColumnGap, t))
	dst.SpaceX = LerpLP(from.SpaceX, to.SpaceX, t)
	dst.SpaceY = LerpLP(from.SpaceY, to.SpaceY, t)
}

// nonNegative and nonNegativeLP clamp what CSS does not allow below zero (a
// curve that overshoots can go there).
func nonNegative(d style.Dim) style.Dim {
	if d.Kind == style.DimLength {
		d.LP = nonNegativeLP(d.LP)
	}
	return d
}

func nonNegativeLP(l style.LP) style.LP {
	return style.LP{Px: max(l.Px, 0), Pct: max(l.Pct, 0)}
}
