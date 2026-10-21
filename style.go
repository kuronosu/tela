package tela

import (
	"image"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/style"
)

// prop wraps one declaration of package style as a StyleProp.
func prop(op style.Op) StyleProp { return core.StyleOps(op) }

// StyleProp is one typed style declaration, built by the functions below
// and passed to h.Style. Typed props apply after the element's classes, so
// they win over a class that sets the same property; use them for values
// that come from code (a measured width, a color from state). Lengths are
// logical pixels.
//
// A StyleProp is a [Child]: besides h.Style(tela.Width(10)), an element
// takes it directly, Div(tela.Width(10), tela.Bg(c)), which is the same
// (and h.If(cond, tela.Opacity(0.5)) works). [Box] and [WH] return a
// StyleProp of several declarations.
type StyleProp = core.StyleProp

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
func Width(px float32) StyleProp { return prop(style.Width(px)) }

// Height sets the height in pixels.
func Height(px float32) StyleProp { return prop(style.Height(px)) }

// WH sets the width and the height in pixels: WH(w, h) is Width(w) and
// Height(h) in one StyleProp. It is named for its two arguments because
// Size is the type of the animatable sizes.
//
//	Div(tela.WH(240, 160), Class("rounded-xl bg-white"))
func WH(w, h float32) StyleProp {
	return core.StyleOps(style.Width(w), style.Height(h))
}

// Box places and sizes a positioned element in pixels: its Left and Top
// insets and its Width and Height, in one StyleProp (the typed form of
// "left-[x] top-[y] w-[w] h-[h]"; the element needs a position class such
// as absolute or relative, and a rectangle from [ElementRef].Bounds or
// [UseBounds] fits it).
//
//	Div(Class("absolute rounded-md ring-2 ring-indigo-500"), tela.Box(r.X, r.Y, r.Width, r.Height))
func Box(x, y, w, h float32) StyleProp {
	return core.StyleOps(style.Left(x), style.Top(y), style.Width(w), style.Height(h))
}

// MinWidth sets the minimum width in pixels.
func MinWidth(px float32) StyleProp { return prop(style.MinWidth(px)) }

// MinHeight sets the minimum height in pixels.
func MinHeight(px float32) StyleProp { return prop(style.MinHeight(px)) }

// MaxWidth sets the maximum width in pixels.
func MaxWidth(px float32) StyleProp { return prop(style.MaxWidth(px)) }

// MaxHeight sets the maximum height in pixels.
func MaxHeight(px float32) StyleProp { return prop(style.MaxHeight(px)) }

// Padding sets the padding of the four sides in pixels.
func Padding(px float32) StyleProp { return prop(style.Padding(px)) }

// PaddingXY sets the horizontal and vertical padding in pixels.
func PaddingXY(x, y float32) StyleProp { return prop(style.PaddingXY(x, y)) }

// Margin sets the margin of the four sides in pixels.
func Margin(px float32) StyleProp { return prop(style.Margin(px)) }

// Gap sets the row and column gap of a flex container in pixels.
func Gap(px float32) StyleProp { return prop(style.Gap(px)) }

// Left sets the left inset of a positioned element in pixels.
func Left(px float32) StyleProp { return prop(style.Left(px)) }

// Top sets the top inset of a positioned element in pixels.
func Top(px float32) StyleProp { return prop(style.Top(px)) }

// Right sets the right inset of a positioned element in pixels.
func Right(px float32) StyleProp { return prop(style.Right(px)) }

// Bottom sets the bottom inset of a positioned element in pixels.
func Bottom(px float32) StyleProp { return prop(style.Bottom(px)) }

// Bg sets the background color.
func Bg(c Color) StyleProp { return prop(style.Bg(c)) }

// BackgroundGradient sets a gradient as the background image, over the
// background color.
func BackgroundGradient(g Gradient) StyleProp { return prop(style.BackgroundGradient(g)) }

// BgImage sets a decoded image as the background image, over the
// background color. The background classes apply to it as to a
// bg-[url(...)] image: bg-cover, bg-contain, bg-center, bg-no-repeat,
// bg-origin-*, bg-clip-* and the others. Each image.Image value is
// converted once and cached (the cache keeps the most recently used
// images, up to 256 MiB of pixels), so pass the same value on every build
// and do not modify its pixels afterwards: give a new image instead. For
// images stored as files, register them with [Assets] and use a class.
func BgImage(img image.Image) StyleProp {
	if img == nil {
		panic("tela: BgImage(nil): pass a decoded image.Image, for example from image.Decode")
	}
	return prop(style.BgImage(img))
}

// TextColor sets the text color, which the element's descendants inherit.
func TextColor(c Color) StyleProp { return prop(style.Color(c)) }

// BorderColor sets the color of the four borders.
func BorderColor(c Color) StyleProp { return prop(style.BorderColor(c)) }

// BorderWidth sets the width of the four borders in pixels.
func BorderWidth(px float32) StyleProp { return prop(style.BorderWidth(px)) }

// Radius sets the radius of the four corners in pixels.
func Radius(px float32) StyleProp { return prop(style.Radius(px)) }

// Opacity sets the opacity from 0 (transparent) to 1.
func Opacity(v float32) StyleProp { return prop(style.Opacity(v)) }

// ZIndex sets an explicit z-index (it applies to positioned elements and
// flex items).
func ZIndex(z int) StyleProp { return prop(style.ZIndexOf(int32(z))) }

// Translate moves the element by x and y pixels without affecting layout.
func Translate(x, y float32) StyleProp { return prop(style.Translate(x, y)) }

// Scale scales the element by x and y (1 is no change) without affecting
// layout.
func Scale(x, y float32) StyleProp { return prop(style.Scale(x, y)) }

// Rotate rotates the element by deg degrees, clockwise.
func Rotate(deg float32) StyleProp { return prop(style.Rotate(deg)) }

// FontSize sets the font size in pixels.
func FontSize(px float32) StyleProp { return prop(style.FontSize(px)) }

// FontWeight sets the font weight, from 100 to 900.
func FontWeight(w float32) StyleProp { return prop(style.FontWeight(w)) }

// BoxShadow sets the box shadows, replacing those of shadow-* classes.
func BoxShadow(layers ...Shadow) StyleProp { return prop(style.BoxShadow(layers...)) }

// Phase-deferred props (ARCHITECTURE section 7). fn is called when the
// phase that needs the value runs, not when the node is built: when the
// element's style or structure changes, and when a [Value] that fn read
// the last time changes. Read a [UseValue] inside fn to drive drags,
// scroll effects and progress bars without building components: a change
// redoes only that element's layout (WidthFn, HeightFn) or paint
// (TranslateFn, OpacityFn, BgFn, TextColorFn). fn runs on the UI thread
// and should only compute the value.

// TranslateFn defers Translate to the paint phase: a change of the Values
// fn reads repaints the element, without layout.
//
//	Style(tela.TranslateFn(func() (x, y float32) { return float32(pos.Get()), 0 }))
func TranslateFn(fn func() (x, y float32)) StyleProp { return prop(style.TranslateFn(fn)) }

// OpacityFn defers Opacity to the paint phase: a change of the Values fn
// reads repaints the element, without layout.
func OpacityFn(fn func() float32) StyleProp { return prop(style.OpacityFn(fn)) }

// BgFn defers [Bg] to the paint phase: the element's background color is
// fn's, and a change of the Values fn reads repaints the element without
// building, computing styles or laying out. Like the other Fn props it
// wins over the background classes, and over their transitions. With a
// [UseSpring] or [Animate] of a [Color] it animates a background without
// rebuilding:
//
//	bg := tela.UseSpring(target) // target is a tela.Color
//	Div(Class("rounded-lg p-4"), Style(tela.BgFn(bg.Get)))
func BgFn(fn func() Color) StyleProp { return prop(style.BgFn(fn)) }

// TextColorFn defers [TextColor] to the paint phase: a change of the
// Values fn reads repaints the text without building, computing styles or
// laying out. The element's text takes fn's color, and so does the text
// of its descendants that inherits the element's color: those whose
// classes (and the classes of the elements between) set no text color of
// their own, as CSS inheritance would give it. Underlines and other
// decorations without a color of their own follow the text. Colors that
// the classes derive from the text color when the style is computed
// (currentColor borders, outlines, ring colors) and the text of
// h.Input and h.Textarea keep the color their classes give. It wins over
// the text color classes and their transitions.
//
//	color := tela.UseTween(target, tela.Duration(200*time.Millisecond)) // a tela.Color
//	P(Style(tela.TextColorFn(color.Get)), Text("Status: "), Strong(Text("online")))
func TextColorFn(fn func() Color) StyleProp { return prop(style.TextColorFn(fn)) }

// WidthFn defers Width, in pixels, to the layout phase: a change of the
// Values fn reads lays the element out again, without building it.
func WidthFn(fn func() float32) StyleProp { return prop(style.WidthFn(fn)) }

// HeightFn defers Height, in pixels, to the layout phase: a change of the
// Values fn reads lays the element out again, without building it.
func HeightFn(fn func() float32) StyleProp { return prop(style.HeightFn(fn)) }
