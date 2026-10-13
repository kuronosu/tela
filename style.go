package tela

import "github.com/kuronosu/tela/internal/style"

// StyleProp is one typed style declaration, built by the functions below
// and passed to h.Style. Typed props apply after the element's classes, so
// they win over a class that sets the same property; use them for values
// that come from code (a measured width, a color from state). Lengths are
// logical pixels.
type StyleProp = style.Op

// Shadow is one box shadow layer: OffsetX, OffsetY, Blur and Spread in
// logical pixels, its Color, and Inset for an inner shadow.
type Shadow = style.Shadow

// Gradient is a background gradient; build one with [LinearGradient].
type Gradient = style.Gradient

// GradientStop is a color stop of a gradient; build one with [Stop] or
// [StopAt].
type GradientStop = style.ColorStop

// Stop returns a gradient stop whose position is spread automatically
// between its neighbors, as in CSS.
func Stop(c Color) GradientStop { return GradientStop{Color: c} }

// StopAt returns a gradient stop at a percentage (0 to 100) of the gradient
// line.
func StopAt(c Color, percent float32) GradientStop {
	return GradientStop{Color: c, Pos: style.StopPos{Set: true, LP: style.PctLP(percent)}}
}

// LinearGradient returns a linear gradient in the direction of angleDeg: 0
// points up, 90 to the right and 180 (CSS's default) down. Colors
// interpolate in Oklab, like Tailwind v4.
func LinearGradient(angleDeg float32, stops ...GradientStop) Gradient {
	if len(stops) < 2 {
		panic("tela: LinearGradient needs at least two stops, for example tela.LinearGradient(135, tela.Stop(from), tela.Stop(to))")
	}
	return Gradient{
		Kind: style.GradientLinear, Angle: angleDeg,
		CenterX: style.PctLP(50), CenterY: style.PctLP(50),
		Stops: append([]GradientStop(nil), stops...),
	}
}

// Width sets the width in pixels.
func Width(px float32) StyleProp { return style.Width(px) }

// Height sets the height in pixels.
func Height(px float32) StyleProp { return style.Height(px) }

// MinWidth sets the minimum width in pixels.
func MinWidth(px float32) StyleProp { return style.MinWidth(px) }

// MinHeight sets the minimum height in pixels.
func MinHeight(px float32) StyleProp { return style.MinHeight(px) }

// MaxWidth sets the maximum width in pixels.
func MaxWidth(px float32) StyleProp { return style.MaxWidth(px) }

// MaxHeight sets the maximum height in pixels.
func MaxHeight(px float32) StyleProp { return style.MaxHeight(px) }

// Padding sets the padding of the four sides in pixels.
func Padding(px float32) StyleProp { return style.Padding(px) }

// PaddingXY sets the horizontal and vertical padding in pixels.
func PaddingXY(x, y float32) StyleProp { return style.PaddingXY(x, y) }

// Margin sets the margin of the four sides in pixels.
func Margin(px float32) StyleProp { return style.Margin(px) }

// Gap sets the row and column gap of a flex container in pixels.
func Gap(px float32) StyleProp { return style.Gap(px) }

// Left sets the left inset of a positioned element in pixels.
func Left(px float32) StyleProp { return style.Left(px) }

// Top sets the top inset of a positioned element in pixels.
func Top(px float32) StyleProp { return style.Top(px) }

// Right sets the right inset of a positioned element in pixels.
func Right(px float32) StyleProp { return style.Right(px) }

// Bottom sets the bottom inset of a positioned element in pixels.
func Bottom(px float32) StyleProp { return style.Bottom(px) }

// Bg sets the background color.
func Bg(c Color) StyleProp { return style.Bg(c) }

// BackgroundGradient sets a gradient as the background image, over the
// background color.
func BackgroundGradient(g Gradient) StyleProp { return style.BackgroundGradient(g) }

// TextColor sets the text color, which the element's descendants inherit.
func TextColor(c Color) StyleProp { return style.Color(c) }

// BorderColor sets the color of the four borders.
func BorderColor(c Color) StyleProp { return style.BorderColor(c) }

// BorderWidth sets the width of the four borders in pixels.
func BorderWidth(px float32) StyleProp { return style.BorderWidth(px) }

// Radius sets the radius of the four corners in pixels.
func Radius(px float32) StyleProp { return style.Radius(px) }

// Opacity sets the opacity from 0 (transparent) to 1.
func Opacity(v float32) StyleProp { return style.Opacity(v) }

// ZIndex sets an explicit z-index (it applies to positioned elements and
// flex items).
func ZIndex(z int) StyleProp { return style.ZIndexOf(int32(z)) }

// Translate moves the element by x and y pixels without affecting layout.
func Translate(x, y float32) StyleProp { return style.Translate(x, y) }

// Scale scales the element by x and y (1 is no change) without affecting
// layout.
func Scale(x, y float32) StyleProp { return style.Scale(x, y) }

// Rotate rotates the element by deg degrees, clockwise.
func Rotate(deg float32) StyleProp { return style.Rotate(deg) }

// FontSize sets the font size in pixels.
func FontSize(px float32) StyleProp { return style.FontSize(px) }

// FontWeight sets the font weight, from 100 to 900.
func FontWeight(w float32) StyleProp { return style.FontWeight(w) }

// BoxShadow sets the box shadows, replacing those of shadow-* classes.
func BoxShadow(layers ...Shadow) StyleProp { return style.BoxShadow(layers...) }

// Phase-deferred props (ARCHITECTURE section 7). fn is called when the
// phase that needs the value runs, not when the node is built: when the
// element's style or structure changes, and when a [Value] that fn read
// the last time changes. Read a [UseValue] inside fn to drive drags,
// scroll effects and progress bars without building components: a change
// redoes only that element's layout (WidthFn, HeightFn) or paint
// (TranslateFn, OpacityFn). fn runs on the UI thread and should only
// compute the value.

// TranslateFn defers Translate to the paint phase: a change of the Values
// fn reads repaints the element, without layout.
//
//	Style(tela.TranslateFn(func() (x, y float32) { return float32(pos.Get()), 0 }))
func TranslateFn(fn func() (x, y float32)) StyleProp { return style.TranslateFn(fn) }

// OpacityFn defers Opacity to the paint phase: a change of the Values fn
// reads repaints the element, without layout.
func OpacityFn(fn func() float32) StyleProp { return style.OpacityFn(fn) }

// WidthFn defers Width, in pixels, to the layout phase: a change of the
// Values fn reads lays the element out again, without building it.
func WidthFn(fn func() float32) StyleProp { return style.WidthFn(fn) }

// HeightFn defers Height, in pixels, to the layout phase: a change of the
// Values fn reads lays the element out again, without building it.
func HeightFn(fn func() float32) StyleProp { return style.HeightFn(fn) }
