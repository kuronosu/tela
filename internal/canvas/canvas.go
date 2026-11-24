package canvas

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// Style is what [Canvas.FillStyle] and [Canvas.StrokeStyle] take: a Color.
// The interface is closed: only Tela's types implement it.
//
// It is closed by the method of gfx.Paint, which gfx.Color has and which a
// type outside gfx gets only by embedding: a gradient or a pattern of this
// package embeds a Style (nil) to be one, and paint tells them apart.
type Style interface{ gfx.Paint }

// The kinds of line ends and corners, and the fill rules.
type (
	// LineCap is the shape of the ends of a line: [Canvas.LineCap].
	LineCap = gfx.LineCap
	// LineJoin is the shape of the corners of a line: [Canvas.LineJoin].
	LineJoin = gfx.LineJoin
	// FillRule tells the inside of a path that crosses itself:
	// [Canvas.FillRule].
	FillRule = gfx.FillRule
)

// DefaultMiterLimit is the initial [Canvas.MiterLimit], the browser's.
const DefaultMiterLimit = 10

// Canvas is what the function given to h.Canvas draws with: the browser's
// CanvasRenderingContext2D, with the same method names over float64 and its
// properties as fields. It is valid only while that function runs, on the
// UI thread: do not keep it.
type Canvas struct {
	// Width and Height are the size of the canvas in logical pixels: the
	// content box of its element, snapped to device pixels as it is painted.
	// (0, 0) is its top left corner. Nothing is drawn outside.
	Width, Height float64
	// Time is the canvas's own clock: how long it has been running, the sum
	// of the Deltas. It is 0 at the first draw and moves with the frame clock
	// from a draw that called [Canvas.RequestFrame] to the next draw; it
	// stands still while the canvas rests, and while it waits off screen or
	// in a window that gets no frames, so what moves with it goes on from
	// where it was.
	Time time.Duration
	// Delta is how far Time moved since the canvas's previous draw. It is 0
	// when the clock did not run: at the first draw, at a second one within
	// the same frame, and at the first one after a rest (the previous draw
	// did not call RequestFrame) or a wait (off screen, hidden, or more than
	// a second without frames).
	Delta time.Duration

	// FillStyle is what Fill and FillRect paint with. It starts as the
	// element's fill (a fill-* class, its own or inherited), or its text
	// color without one; nil and transparent paint nothing, as fill-none.
	FillStyle Style
	// StrokeStyle is what Stroke and StrokeRect paint with. It starts as the
	// element's stroke (a stroke-* class, its own or inherited), or its text
	// color without one; nil and transparent paint nothing, as stroke-none.
	StrokeStyle Style
	// LineWidth is the width of lines. It starts as the element's
	// stroke-width (stroke-2), or 1. Lines are drawn only for a finite width
	// above 0.
	LineWidth float64
	// LineCap is the shape of the ends of lines, CapButt at first.
	LineCap LineCap
	// LineJoin is the shape of the corners of lines, JoinMiter at first.
	LineJoin LineJoin
	// MiterLimit is the longest miter a JoinMiter corner may have, as a
	// multiple of LineWidth, 10 at first; a longer one is cut as JoinBevel.
	// A limit that is not above 0 counts as 10.
	MiterLimit float64
	// GlobalAlpha multiplies the alpha of everything drawn, 1 at first. It
	// is cut to 0 to 1; NaN draws nothing.
	GlobalAlpha float64
	// FillRule is the rule of Fill and Clip, NonZero at first: the argument
	// of the browser's fill and clip.
	FillRule FillRule

	// rec records what the canvas draws, nil before and after its draw.
	rec atomic.Pointer[Recorder]
	// dev: using the canvas after its draw panics (set before rec is, and
	// not written again).
	dev bool
}

// Frame is what a draw starts from.
type Frame struct {
	// Width and Height are the size of the canvas, in logical pixels.
	Width, Height float64
	Time, Delta   time.Duration
	// Fill, Stroke and LineWidth are the initial values of the fields.
	Fill, Stroke Style
	LineWidth    float64
	// Env answers what the canvas asks of its element.
	Env Env
	// Dev is dev mode.
	Dev bool
}

// Env is what a canvas asks of its element.
type Env interface {
	// Color returns the color of a theme token or of the palette.
	Color(token string) gfx.Color
}

// state is what Save keeps and Restore brings back.
type state struct {
	m                                matrix
	clips                            int
	fill, stroke                     Style
	lineWidth, miterLimit, globAlpha float64
	lineCap                          LineCap
	lineJoin                         LineJoin
	fillRule                         FillRule
}

func (c *Canvas) state(r *Recorder) state {
	return state{
		m: r.m, clips: r.clips, fill: c.FillStyle, stroke: c.StrokeStyle,
		lineWidth: c.LineWidth, miterLimit: c.MiterLimit, globAlpha: c.GlobalAlpha,
		lineCap: c.LineCap, lineJoin: c.LineJoin, fillRule: c.FillRule,
	}
}

func (c *Canvas) setState(r *Recorder, s *state) {
	r.m = s.m
	r.unclip(s.clips)
	c.FillStyle, c.StrokeStyle = s.fill, s.stroke
	c.LineWidth, c.MiterLimit, c.GlobalAlpha = s.lineWidth, s.miterLimit, s.globAlpha
	c.LineCap, c.LineJoin, c.FillRule = s.lineCap, s.lineJoin, s.fillRule
}

func newCanvas(f *Frame) *Canvas {
	return &Canvas{
		Width: f.Width, Height: f.Height, Time: f.Time, Delta: f.Delta,
		FillStyle: f.Fill, StrokeStyle: f.Stroke,
		LineWidth: f.LineWidth, MiterLimit: DefaultMiterLimit, GlobalAlpha: 1,
		dev: f.Dev,
	}
}

// use returns the recorder of the draw in progress, nil when there is none:
// the canvas was kept after its draw. In dev mode that panics, naming
// method. Any goroutine.
func (c *Canvas) use(method string) *Recorder {
	r := c.rec.Load()
	if r == nil && c.dev {
		panic(fmt.Sprintf("tela: Canvas.%s called after the draw function returned: a *tela.Canvas is valid only while the function given to h.Canvas runs. Draw there; to draw again, set state, set a tela.Value that the function reads, or call RequestFrame inside it", method))
	}
	return r
}

// Save puts the state of the canvas on a stack: the transform, the clip and
// the fields from FillStyle to FillRule. The path is not part of it. What is
// still saved when the draw function returns is restored.
func (c *Canvas) Save() {
	if r := c.use("Save"); r != nil {
		r.states = append(r.states, c.state(r))
	}
}

// Restore brings back the state of the last Save that was not restored. It
// does nothing without one.
func (c *Canvas) Restore() {
	r := c.use("Restore")
	if r == nil || len(r.states) == 0 {
		return
	}
	n := len(r.states) - 1
	c.setState(r, &r.states[n])
	r.states[n] = state{} // its styles
	r.states = r.states[:n]
}

// ---------------------------------------------------------------------
// The transform.

// Translate moves what is drawn from now on by x, y.
func (c *Canvas) Translate(x, y float64) {
	if r := c.use("Translate"); r != nil && finite(x, y) {
		r.m = r.m.mul(matrix{a: 1, d: 1, e: x, f: y})
	}
}

// Rotate turns what is drawn from now on by angle radians around the origin,
// clockwise.
func (c *Canvas) Rotate(angle float64) {
	if r := c.use("Rotate"); r != nil && finite(angle) {
		r.m = r.m.mul(rotation(angle))
	}
}

// Scale scales what is drawn from now on from the origin, by x and y.
func (c *Canvas) Scale(x, y float64) {
	if r := c.use("Scale"); r != nil && finite(x, y) {
		r.m = r.m.mul(matrix{a: x, d: y})
	}
}

// Transform multiplies the current transform by the matrix
//
//	m11 m21 dx
//	m12 m22 dy
//	0   0   1
//
// whose numbers it takes in the browser's order (a, b, c, d, e, f).
func (c *Canvas) Transform(m11, m12, m21, m22, dx, dy float64) {
	if r := c.use("Transform"); r != nil && finite(m11, m12, m21, m22, dx, dy) {
		r.m = r.m.mul(matrix{m11, m12, m21, m22, dx, dy})
	}
}

// SetTransform makes the matrix of [Canvas.Transform] the current transform,
// from the canvas's logical pixels.
func (c *Canvas) SetTransform(m11, m12, m21, m22, dx, dy float64) {
	if r := c.use("SetTransform"); r != nil && finite(m11, m12, m21, m22, dx, dy) {
		r.m = matrix{m11, m12, m21, m22, dx, dy}
	}
}

// ResetTransform makes the current transform the identity.
func (c *Canvas) ResetTransform() {
	if r := c.use("ResetTransform"); r != nil {
		r.m = identity
	}
}

// ---------------------------------------------------------------------
// The frame.

// RequestFrame asks for one more draw, at the window's next frame: call it
// in the draw function for as long as something moves, as a browser's
// animation calls requestAnimationFrame. Without it the canvas rests, and the
// window sleeps.
//
// Off screen (scrolled out of view, invisible), or in a window that gets no
// frames (hidden, minimized), the request waits, and asks for no frames;
// the canvas goes on when it is shown.
func (c *Canvas) RequestFrame() {
	if r := c.use("RequestFrame"); r != nil {
		r.again = true
	}
}

// Color returns the color of a theme token ("primary") or of the palette
// ("violet-500") in the element's color scheme, as ThemeValues.Color does.
// An unknown name is transparent; in dev mode it panics with the closest
// token. A change of the theme or of the scheme draws the canvas again.
func (c *Canvas) Color(token string) gfx.Color {
	r := c.use("Color")
	if r == nil || r.env == nil {
		return gfx.Color{}
	}
	return r.env.Color(token)
}

// ---------------------------------------------------------------------
// Recording.

// Recorder records the draws of canvases, one at a time. The zero Recorder
// is ready to use. It belongs to the UI thread.
type Recorder struct {
	b *gfx.Builder
	m matrix
	// path is the current path, in the canvas's coordinates (path.go), and
	// line reports that its last subpath has a segment. rect and user are
	// scratch: the path of a rectangle, and one in the coordinates of the
	// transform.
	path       gfx.Path
	line       bool
	rect, user gfx.Path
	// states is the stack of Save, and clips how many clips are open in b
	// besides the canvas's own.
	states []state
	clips  int
	// drawn reports that something was recorded.
	drawn bool
	again bool
	env   Env
	// size is the canvas.
	size gfx.Rect
	// paints are the last paints made of a fill and a stroke style.
	paints [2]paintMemo
}

// paintMemo is the last paint made of a color, to make it once for the many
// commands that use it.
type paintMemo struct {
	color gfx.Color
	alpha float32
	paint gfx.Paint
}

// Record runs draw with a new Canvas of f and returns the list it recorded,
// in the canvas's coordinates and clipped to its size (an empty list when it
// drew nothing), and whether it called RequestFrame. When draw panics, the
// panic goes on and the recorder is ready for another draw.
func (r *Recorder) Record(f *Frame, draw func(*Canvas)) (list *gfx.DisplayList, again bool) {
	c := newCanvas(f)
	r.begin(f)
	c.rec.Store(r)
	defer func() {
		c.rec.Store(nil)
		if list == nil { // draw panicked, or ended its goroutine
			r.finish()
		}
	}()
	draw(c)
	c.rec.Store(nil)
	again = r.again
	return r.finish(), again
}

// begin starts a draw.
func (r *Recorder) begin(f *Frame) {
	if r.b == nil {
		r.b = gfx.NewBuilder()
	}
	r.m, r.env = identity, f.Env
	r.size = gfx.Rect{X1: float32(f.Width), Y1: float32(f.Height)}
	r.path.Reset()
	r.line, r.drawn, r.again = false, false, false
	r.b.PushClipRect(r.size)
}

// finish closes what the draw left open and returns what it recorded.
func (r *Recorder) finish() *gfx.DisplayList {
	r.unclip(0)
	r.b.PopClip()
	list := r.b.Finish()
	clear(r.states)
	r.states = r.states[:0]
	r.env = nil
	r.paints = [2]paintMemo{}
	return list
}

// unclip closes the clips opened after the first n.
func (r *Recorder) unclip(n int) {
	for ; r.clips > n; r.clips-- {
		r.b.PopClip()
	}
}

// Which paint a style is.
const (
	paintFill = iota
	paintStroke
)

// paint returns the paint of style s at opacity alpha for a fill or a stroke
// (which), and false when it paints nothing.
func (r *Recorder) paint(which int, s Style, alpha float64) (gfx.Paint, bool) {
	a := float32(1)
	if !(alpha >= 1) {
		a = float32(alpha)
		if !(a > 0) {
			return nil, false
		}
	}
	switch s := s.(type) {
	case nil:
		return nil, false
	case gfx.Color:
		m := &r.paints[which]
		if m.paint == nil || m.color != s || m.alpha != a {
			c := s.Clamp() // as the Builder would, for each command
			c.A = float32(c.A * a)
			*m = paintMemo{s, a, c}
		}
		return m.paint, s.A > 0
	}
	panic(fmt.Sprintf("tela: a Canvas cannot paint with a %T: FillStyle and StrokeStyle take a tela.Color", s))
}
