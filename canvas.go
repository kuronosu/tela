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
type Canvas = canvas.Canvas

// CanvasStyle is what the FillStyle and the StrokeStyle of a [Canvas] take: a
// [Color]. No type outside Tela implements it.
type CanvasStyle = canvas.Style

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
