package tela

import (
	"github.com/kuronosu/tela/internal/canvas"
	"github.com/kuronosu/tela/internal/gfx"
)

// Canvas is what the function given to h.Canvas draws with: the 2D context
// of a browser's canvas, with the same methods over float64, and its
// properties as fields. The function records a drawing, which Tela keeps
// and paints at any scale: every draw starts from an empty canvas, and
// nothing is drawn again until something changes.
//
//	Canvas(func(c *tela.Canvas) {
//	    r := min(c.Width, c.Height)/2 - 8
//	    c.LineWidth, c.LineCap = 8, tela.CapRound
//	    c.BeginPath()
//	    c.Arc(c.Width/2, c.Height/2, r, 0, 2*math.Pi*progress)
//	    c.Stroke()
//	}, Class("size-32 stroke-primary"), AriaLabel("Progress"))
//
// # When it draws
//
// The function runs on the UI thread, while a frame is painted and after its
// layout, when something it draws with changed: at the mount and at every
// build of the component that made the element, for a new size, display
// scale, style (the color, the fill, the stroke), theme or color scheme, when
// a [Value] it read with Get changes (no component builds for that), and for
// [Canvas.RequestFrame]. A canvas that nothing shows waits until something
// does. A drawing equal to the last one paints nothing.
//
// It must not call hooks, set state (a [Setter], a [Value], [Animate],
// [Post]) or keep the Canvas: in dev mode those panic. What a simulation
// moves from a draw to the next one is kept in a [UseRef].
//
// # Colors
//
// FillStyle and StrokeStyle start as the fill and the stroke of the element
// (fill-*, stroke-*), and as its text color without those classes, so a
// canvas follows the theme as an icon does. Color reads any other color of
// the theme:
//
//	c.FillStyle = c.Color("chart-1")
//
// # Animation
//
// A draw that calls RequestFrame is followed by another one at the window's
// next frame. Time and Delta tell how long the canvas has been moving; they
// stand still while it rests, and while it is off screen or in a window that
// gets no frames, where it waits. Reduced motion changes nothing: see
// [UseReducedMotion].
//
//	ball := tela.UseRef(Ball{})
//	return Canvas(func(c *tela.Canvas) {
//	    ball.Current.Move(c.Delta)
//	    c.BeginPath()
//	    c.Arc(ball.Current.X, ball.Current.Y, 12, 0, 2*math.Pi)
//	    c.Fill()
//	    c.RequestFrame()
//	}, Class("w-full h-64 fill-sky-500"))
//
// # Text
//
// Font starts as the element's font (font-*, text-*, italic, tracking-*, and
// what the theme and the desktop make of them), and a canvas draws again when
// that changes. A field of it set to zero is the element's again:
//
//	c.Font.Size, c.Font.Weight = 12, 600 // the element's family
//	c.Font = tela.CanvasFont{Family: "ui-monospace, monospace"}
//	c.TextAlign, c.TextBaseline = tela.TextAlignCenter, tela.BaselineMiddle
//	c.FillText("42 %", c.Width/2, c.Height/2)
//
// The direction and the language are the element's (h.Dir, h.Lang), as are
// the features of its font (tabular-nums). Text is shaped once, however many
// draws write it.
//
// # Compositing
//
// A draw that uses a GlobalCompositeOperation other than the first one, or
// clears a part of the canvas with ClearRect, makes the canvas a layer of its
// own, so that what is behind it is not erased. The others cost no layer.
//
// # From the browser's API
//
// Methods have the browser's names, with a capital, and its properties are
// fields, with these differences:
//
//	browser                              Tela
//	getContext, width, height            the draw function's argument, Width, Height
//	requestAnimationFrame(draw)          c.RequestFrame(), Time and Delta
//	fillStyle = "#4f46e5"                FillStyle = tela.Hex("#4f46e5"), or Color("primary")
//	fillStyle = gradient                 FillStyle = gradient, a CanvasGradient
//	gradient.addColorStop(0, "red")      AddColorStop(0, color)
//	createPattern(img, "repeat-x")       CreatePattern(img, tela.RepeatX)
//	pattern.setTransform(matrix)         SetTransform(m11, m12, m21, m22, dx, dy)
//	fill("evenodd"), clip("evenodd")     FillRule = tela.EvenOdd, then Fill or Clip
//	fill(path), stroke(path), clip(path) FillPath(path), StrokePath(path), ClipPath(path)
//	new Path2D("M0 0L9 9"), new Path2D() tela.ParsePath("M0 0L9 9"), new(tela.Path2D)
//	isPointInPath(path, x, y)            path.Contains(x, y), in the path's coordinates, also outside a draw
//	roundRect(x, y, w, h, [8, 4])        RoundRect(x, y, w, h, 8, 4)
//	setLineDash([4, 2]), getLineDash()   SetLineDash(4, 2), LineDash()
//	lineCap = "round", lineJoin = "bevel" LineCap = tela.CapRound, LineJoin = tela.JoinBevel
//	font = "600 12px Inter"              Font = tela.CanvasFont{Family: "Inter", Size: 12, Weight: 600}
//	textAlign = "center"                 TextAlign = tela.TextAlignCenter
//	textBaseline = "middle"              TextBaseline = tela.BaselineMiddle
//	letterSpacing = "2px"                LetterSpacing = 2
//	direction, lang, fontVariantNumeric  h.Dir, h.Lang and classes on the element
//	drawImage(img, x, y)                 DrawImage(img, x, y), of an image.Image
//	drawImage(img, x, y, w, h)           DrawImageSize(img, x, y, w, h)
//	drawImage(img, sx, ..., dh)          DrawImageRegion(img, sx, ..., dh)
//	imageSmoothingEnabled = false        ImageSmoothing = false
//	globalCompositeOperation = "xor"     GlobalCompositeOperation = tela.CompositeXor
//	shadowColor = "rgb(0 0 0 / 50%)"     ShadowColor = tela.Black.WithAlpha(0.5)
//	filter, getImageData, putImageData   none (pixels: UseTexture)
//
// FillStyle, StrokeStyle and Font start as the element's, where the browser
// starts black and in 10px sans-serif. Invalid values never stop a draw: what
// the browser ignores is ignored when it draws, and what it answers with an
// error (a negative radius, an offset of a stop above 1) does nothing.
type Canvas = canvas.Canvas

// CanvasStyle is what the FillStyle and the StrokeStyle of a [Canvas] take: a
// [Color], a [*CanvasGradient] or a [*CanvasPattern]. No type outside Tela
// implements it.
type CanvasStyle = canvas.Style

// CanvasGradient is a gradient that a [Canvas] paints with, made by its
// CreateLinearGradient, CreateRadialGradient and CreateConicGradient. Its
// AddColorStop adds a color at an offset from 0 to 1:
//
//	sky := c.CreateLinearGradient(0, 0, 0, c.Height)
//	sky.AddColorStop(0, c.Color("sky-400"))
//	sky.AddColorStop(1, c.Color("sky-100"))
//	c.FillStyle = sky
//	c.FillRect(0, 0, c.Width, c.Height)
//
// It is in the coordinates of the transform current when it paints, whatever
// it paints. Colors interpolate in sRGB, as in a browser's canvas, not in
// Oklab as those of the bg-linear-* classes. It may be kept, and used by
// other draws and other canvases, on the UI thread.
type CanvasGradient = canvas.CanvasGradient

// CanvasPattern is an image that a [Canvas] paints with, repeated, made by
// its CreatePattern. Its SetTransform places, turns and scales the image. It
// is in the coordinates of the transform current when it paints, from 0, 0 on.
// It may be kept, and used by other draws and other canvases, on the UI
// thread.
type CanvasPattern = canvas.CanvasPattern

// PatternRepeat tells in which directions a [CanvasPattern] repeats.
type PatternRepeat = canvas.PatternRepeat

// The repetitions of a pattern.
const (
	// Repeat repeats the image in both directions.
	Repeat PatternRepeat = gfx.RepeatBoth
	// RepeatX repeats the image to the sides.
	RepeatX PatternRepeat = gfx.RepeatX
	// RepeatY repeats the image up and down.
	RepeatY PatternRepeat = gfx.RepeatY
	// NoRepeat paints the image once, and nothing around it.
	NoRepeat PatternRepeat = gfx.RepeatNone
)

// Path2D is a path that is kept: built once with the methods of the path of a
// [Canvas] (MoveTo, LineTo, Arc, RoundRect...) in coordinates of its own, and
// drawn by the FillPath, StrokePath and ClipPath of any canvas, with the
// transform current then. The zero Path2D is an empty path, ready to use.
//
// Its Contains tells whether a point is inside, without a canvas, which is
// how a click finds what it hit:
//
//	slice := new(tela.Path2D)
//	slice.MoveTo(50, 50)
//	slice.Arc(50, 50, 40, 0, math.Pi/2)
//	slice.ClosePath()
//	return Canvas(func(c *tela.Canvas) { c.FillPath(slice) },
//	    Class("size-25"),
//	    OnClick(func(e tela.MouseEvent) {
//	        if slice.Contains(float64(e.LocalX), float64(e.LocalY)) {
//	            open()
//	        }
//	    }))
//
// Several goroutines may read a path, and one that changes it must be alone.
type Path2D = canvas.Path2D

// ParsePath returns the path that SVG path data describes (the d attribute of
// an SVG path element, the argument of a browser's new Path2D), or an error
// that tells where the data is wrong. A browser draws such data up to there.
func ParsePath(d string) (*Path2D, error) { return canvas.ParsePath(d) }

// MustPath is [ParsePath] for data known to be valid, such as a package-level
// shape: it panics on an error.
//
//	var pin = tela.MustPath("M12 21s7-6.4 7-11a7 7 0 1 0-14 0c0 4.6 7 11 7 11z")
func MustPath(d string) *Path2D { return canvas.MustPath(d) }

// CanvasFont is the font of the text of a [Canvas]: a family or a list of
// them as in CSS ("Inter", "ui-monospace, monospace"), a size in logical
// pixels, a weight from 1 to 1000 and whether it is italic. A zero field is
// the element's: on an element with the class italic, so is Italic: false.
type CanvasFont = canvas.CanvasFont

// TextMetrics are the sizes of a text as a [Canvas] writes it, which its
// MeasureText returns.
type TextMetrics = canvas.TextMetrics

// TextAlign is where in a text of a [Canvas] its x is.
type TextAlign = canvas.TextAlign

// The alignments of the text of a canvas.
const (
	// TextAlignStart: x is the start of the text: its left end, or its right
	// one in a right-to-left element (h.Dir).
	TextAlignStart TextAlign = canvas.TextAlignStart
	// TextAlignEnd: x is the end of the text.
	TextAlignEnd TextAlign = canvas.TextAlignEnd
	// TextAlignLeft: x is the left end of the text.
	TextAlignLeft TextAlign = canvas.TextAlignLeft
	// TextAlignRight: x is the right end of the text.
	TextAlignRight TextAlign = canvas.TextAlignRight
	// TextAlignCenter: x is the middle of the text.
	TextAlignCenter TextAlign = canvas.TextAlignCenter
)

// TextBaseline is where in a text of a [Canvas] its y is.
type TextBaseline = canvas.TextBaseline

// The baselines of the text of a canvas.
const (
	// BaselineAlphabetic: y is the line that most letters stand on.
	BaselineAlphabetic TextBaseline = canvas.BaselineAlphabetic
	// BaselineTop: y is the top of the em square.
	BaselineTop TextBaseline = canvas.BaselineTop
	// BaselineHanging: y is the line that letters hang from in scripts such
	// as Devanagari.
	BaselineHanging TextBaseline = canvas.BaselineHanging
	// BaselineMiddle: y is the middle of the em square.
	BaselineMiddle TextBaseline = canvas.BaselineMiddle
	// BaselineIdeographic: y is the line below the letters that go down.
	BaselineIdeographic TextBaseline = canvas.BaselineIdeographic
	// BaselineBottom: y is the bottom of the em square.
	BaselineBottom TextBaseline = canvas.BaselineBottom
)

// CompositeOperation is how what a [Canvas] draws, the source, meets what it
// has, the destination: its GlobalCompositeOperation.
type CompositeOperation = canvas.CompositeOperation

// The operations of Porter and Duff. Those that keep the destination only
// where the source is (CompositeSourceIn, CompositeSourceOut,
// CompositeDestinationIn, CompositeDestinationAtop and CompositeCopy) change
// the whole canvas, inside its clip.
const (
	// CompositeSourceOver draws on top of what there is. It is the zero value.
	CompositeSourceOver CompositeOperation = canvas.SourceOver
	// CompositeSourceIn keeps the source where there was something.
	CompositeSourceIn CompositeOperation = canvas.SourceIn
	// CompositeSourceOut keeps the source where there was nothing.
	CompositeSourceOut CompositeOperation = canvas.SourceOut
	// CompositeSourceAtop draws only over what there is.
	CompositeSourceAtop CompositeOperation = canvas.SourceAtop
	// CompositeDestinationOver draws behind what there is.
	CompositeDestinationOver CompositeOperation = canvas.DestinationOver
	// CompositeDestinationIn keeps what there is where the source is.
	CompositeDestinationIn CompositeOperation = canvas.DestinationIn
	// CompositeDestinationOut erases what there is where the source is.
	CompositeDestinationOut CompositeOperation = canvas.DestinationOut
	// CompositeDestinationAtop keeps what there is where the source is, over
	// the source.
	CompositeDestinationAtop CompositeOperation = canvas.DestinationAtop
	// CompositeLighter adds the colors.
	CompositeLighter CompositeOperation = canvas.Lighter
	// CompositeCopy leaves the source alone.
	CompositeCopy CompositeOperation = canvas.Copy
	// CompositeXor leaves the source and what there is where the other is
	// not.
	CompositeXor CompositeOperation = canvas.Xor
)

// The blend modes, which are those of the mix-blend-* classes.
const (
	// CompositeMultiply multiplies the colors, which darkens.
	CompositeMultiply CompositeOperation = canvas.Multiply
	// CompositeScreen is the opposite of multiply, and lightens.
	CompositeScreen CompositeOperation = canvas.Screen
	// CompositeOverlay multiplies dark colors and screens light ones.
	CompositeOverlay CompositeOperation = canvas.Overlay
	// CompositeDarken keeps the darker of each component.
	CompositeDarken CompositeOperation = canvas.Darken
	// CompositeLighten keeps the lighter of each component.
	CompositeLighten CompositeOperation = canvas.Lighten
	// CompositeColorDodge lightens what there is to reflect the source.
	CompositeColorDodge CompositeOperation = canvas.ColorDodge
	// CompositeColorBurn darkens what there is to reflect the source.
	CompositeColorBurn CompositeOperation = canvas.ColorBurn
	// CompositeHardLight is overlay with the two sides exchanged.
	CompositeHardLight CompositeOperation = canvas.HardLight
	// CompositeSoftLight is a softer hard light.
	CompositeSoftLight CompositeOperation = canvas.SoftLight
	// CompositeDifference subtracts the darker from the lighter.
	CompositeDifference CompositeOperation = canvas.Difference
	// CompositeExclusion is a difference of lower contrast.
	CompositeExclusion CompositeOperation = canvas.Exclusion
	// CompositeHue keeps the saturation and the luminosity of what there is.
	CompositeHue CompositeOperation = canvas.Hue
	// CompositeSaturation keeps the hue and the luminosity of what there is.
	CompositeSaturation CompositeOperation = canvas.Saturation
	// CompositeColor keeps the luminosity of what there is.
	CompositeColor CompositeOperation = canvas.ColorOp
	// CompositeLuminosity keeps the hue and the saturation of what there is.
	CompositeLuminosity CompositeOperation = canvas.Luminosity
)

// LineCap is the shape of the ends of the lines of a [Canvas].
type LineCap = gfx.LineCap

// The line caps.
const (
	// CapButt ends a line at its last point.
	CapButt LineCap = gfx.CapButt
	// CapRound ends a line with half a circle around its last point.
	CapRound LineCap = gfx.CapRound
	// CapSquare ends a line half its width beyond its last point.
	CapSquare LineCap = gfx.CapSquare
)

// LineJoin is the shape of the corners of the lines of a [Canvas].
type LineJoin = gfx.LineJoin

// The line joins.
const (
	// JoinMiter extends the sides of a corner until they meet, up to the
	// canvas's MiterLimit, beyond which the corner is cut as JoinBevel.
	JoinMiter LineJoin = gfx.JoinMiter
	// JoinRound rounds a corner.
	JoinRound LineJoin = gfx.JoinRound
	// JoinBevel cuts a corner.
	JoinBevel LineJoin = gfx.JoinBevel
)

// FillRule tells which points are inside a path of a [Canvas] that crosses
// itself, or has one subpath inside another.
type FillRule = gfx.FillRule

// The fill rules.
const (
	// NonZero: the points that the path turns around.
	NonZero FillRule = gfx.NonZero
	// EvenOdd: the points that a line from far away reaches after crossing
	// the path an odd number of times, so a subpath inside another one is a
	// hole.
	EvenOdd FillRule = gfx.EvenOdd
)
