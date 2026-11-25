package canvas

import (
	"fmt"
	"image"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gfx"
)

// What FillStyle and StrokeStyle paint with.
//
// Gradients and patterns are in the coordinates of the transform current when
// they paint, where the geometry of most drawings is already in the canvas's:
// a style is made a paint (ink) for the matrix from its own coordinates to
// those of the drawing. Linear gradients and patterns take any; the circles
// of a radial gradient and the turn of a conic one do not stay such when they
// are distorted, and the drawing is then recorded under the transform, in the
// coordinates of the gradient.
//
// gfx interpolates premultiplied colors, and Canvas (Chromium) colors that
// are not, in sRGB: the stops are those of gfx.UnpremultipliedStops, as SVG's.

// Style is what [Canvas.FillStyle] and [Canvas.StrokeStyle] take: a Color, a
// *CanvasGradient or a *CanvasPattern. The interface is closed: only Tela's
// types implement it.
//
// It is closed by the method of gfx.Paint, which gfx.Color has and which a
// type outside gfx gets only by embedding: a CanvasGradient and a
// CanvasPattern embed a sealed (nil).
type Style interface{ gfx.Paint }

type sealed interface{ gfx.Paint }

// PatternRepeat selects the directions in which a pattern repeats.
type PatternRepeat = gfx.PatternRepeat

type gradientKind uint8

const (
	linearGradient gradientKind = iota
	radialGradient
	conicGradient
	noGradient // paints nothing
)

// CanvasGradient is a gradient that a Canvas paints with, as a FillStyle or a
// StrokeStyle: one of CreateLinearGradient, CreateRadialGradient and
// CreateConicGradient, with the colors of AddColorStop. It is in the
// coordinates of the transform current when it paints, whatever is drawn. It
// may be kept and used by other draws and by other canvases of the UI thread.
// Between two stops it interpolates in sRGB, as a browser's canvas does, not
// in the color space of the bg-linear-* classes.
type CanvasGradient struct {
	sealed
	kind gradientKind
	// The circles of a radial gradient, the points of a linear one; a conic
	// one is around (x0, y0) from angle r0.
	x0, y0, r0, x1, y1, r1 float64
	stops                  []gfx.GradientStop // in order
	// full are the stops with those that emulate the interpolation, nil when
	// stops changed.
	full []gfx.GradientStop
}

// CreateLinearGradient returns a gradient along the line from x0, y0, at
// offset 0, to x1, y1, at offset 1. The colors at its ends go on beyond them.
// A line of no length paints nothing.
func (c *Canvas) CreateLinearGradient(x0, y0, x1, y1 float64) *CanvasGradient {
	c.use("CreateLinearGradient")
	g := &CanvasGradient{x0: x0, y0: y0, x1: x1, y1: y1}
	if !finite(x0, y0, x1, y1) || x0 == x1 && y0 == y1 {
		g.kind = noGradient
	}
	return g
}

// CreateRadialGradient returns a gradient between two circles: around x0, y0
// with radius r0, at offset 0, and around x1, y1 with radius r1, at offset 1.
// When neither circle is inside the other, the gradient paints a cone and
// nothing outside it. Two equal circles paint nothing, and neither does a
// negative radius, where the browser throws an error.
func (c *Canvas) CreateRadialGradient(x0, y0, r0, x1, y1, r1 float64) *CanvasGradient {
	c.use("CreateRadialGradient")
	g := &CanvasGradient{kind: radialGradient, x0: x0, y0: y0, r0: r0, x1: x1, y1: y1, r1: r1}
	if !finite(x0, y0, r0, x1, y1) || !finite(r1) || r0 < 0 || r1 < 0 || x0 == x1 && y0 == y1 && r0 == r1 {
		g.kind = noGradient
	}
	return g
}

// CreateConicGradient returns a gradient that turns clockwise around x, y,
// from startAngle in radians (0 points right), at offset 0, a whole turn.
func (c *Canvas) CreateConicGradient(startAngle, x, y float64) *CanvasGradient {
	c.use("CreateConicGradient")
	g := &CanvasGradient{kind: conicGradient, x0: x, y0: y, r0: startAngle}
	if !finite(startAngle, x, y) {
		g.kind = noGradient
	}
	return g
}

// AddColorStop adds a color at an offset of the gradient, from 0 to 1. Stops
// may be added in any order; those at the same offset keep theirs, and the
// color changes at once there. Without stops a gradient paints nothing. An
// offset outside 0 to 1 adds nothing, where the browser throws an error.
func (g *CanvasGradient) AddColorStop(offset float64, color gfx.Color) {
	if !(offset >= 0 && offset <= 1) || !color.IsFinite() {
		return
	}
	s := gfx.GradientStop{Offset: float32(offset), Color: color}
	i := len(g.stops)
	for i > 0 && g.stops[i-1].Offset > s.Offset {
		i--
	}
	g.stops = slices.Insert(g.stops, i, s)
	g.full = nil
}

// paint returns the paint of g at opacity alpha, for drawings in the
// coordinates that m maps its own to, or nil when it paints nothing. With
// distorted, there is no such paint: the drawing must be in g's coordinates.
func (g *CanvasGradient) paint(m *matrix, alpha float32) (p gfx.Paint, distorted bool) {
	if g == nil || g.kind == noGradient || len(g.stops) == 0 {
		return nil, false
	}
	if _, ok := m.invert(); !ok {
		return nil, false
	}
	if g.full == nil {
		g.full = gfx.UnpremultipliedStops(g.stops)
	}
	stops := g.full
	if alpha != 1 {
		stops = slices.Clone(stops)
		for i := range stops {
			stops[i].Color = stops[i].Color.MulAlpha(alpha)
		}
	}
	switch g.kind {
	case linearGradient:
		// The end points through the matrix, in float64.
		dx, dy := g.x1-g.x0, g.y1-g.y0
		l2 := float64(dx*dx) + float64(dy*dy)
		kx, ky := dx/l2, dy/l2
		det := float64(m.a*m.d) - float64(m.b*m.c)
		ux := (float64(m.d*kx) - float64(m.b*ky)) / det
		uy := (float64(m.a*ky) - float64(m.c*kx)) / det
		u2 := float64(ux*ux) + float64(uy*uy)
		if !(u2 > 0) || math.IsInf(u2, 0) {
			return nil, false
		}
		s := m.vec(g.x0, g.y0)
		return gfx.LinearGradient{
			Start: m.point(g.x0, g.y0), End: gfx.Point{X: float32(s.x + ux/u2), Y: float32(s.y + uy/u2)},
			Stops: stops, Interp: gfx.InterpSRGB,
		}, false
	case radialGradient:
		if g.x0 == g.x1 && g.y0 == g.y1 && g.r0 == 0 && m.axisAligned() {
			return gfx.RadialGradient{
				Center: m.point(g.x1, g.y1),
				Radius: gfx.Point{X: float32(float64(g.r1 * (math.Abs(m.a) + math.Abs(m.c)))), Y: float32(float64(g.r1 * (math.Abs(m.b) + math.Abs(m.d))))},
				Stops:  stops, Interp: gfx.InterpSRGB,
			}, false
		}
		k, ok := m.similar()
		if !ok {
			return nil, true
		}
		if len(stops) == 1 {
			// A gradient of one stop is its color everywhere, outside the cone
			// of two circles too, where nothing is painted.
			stops = []gfx.GradientStop{{Offset: 0, Color: stops[0].Color}, {Offset: 1, Color: stops[0].Color}}
		}
		return gfx.FocalGradient{
			Focal: m.point(g.x0, g.y0), FocalRadius: float32(float64(g.r0 * k)),
			Center: m.point(g.x1, g.y1), Radius: float32(float64(g.r1 * k)),
			Stops: stops, Interp: gfx.InterpSRGB,
		}, false
	}
	if _, ok := m.similar(); !ok || float64(m.a*m.d)-float64(m.b*m.c) < 0 {
		return nil, true
	}
	angle := g.r0 + math.Pi/2 // gfx's 0 points up
	if m.b != 0 || m.a < 0 {
		angle += fmath.Atan2(m.b, m.a)
	}
	return gfx.ConicGradient{
		Center: m.point(g.x0, g.y0), StartAngle: float32(math.Remainder(angle, 2*math.Pi)),
		Stops: stops, Interp: gfx.InterpSRGB,
	}, false
}

// CanvasPattern is an image that a Canvas paints with, as a FillStyle or a
// StrokeStyle, repeated: one of CreatePattern. It is in the coordinates of
// the transform current when it paints, where its first tile starts at 0, 0
// and an image pixel is one unit. It may be kept and used by other draws and
// by other canvases of the UI thread.
type CanvasPattern struct {
	sealed
	img    *gfx.Image
	repeat PatternRepeat
	m      matrix
}

// CreatePattern returns a pattern of img, repeated in both directions, in one
// of them or in none. The image must not be modified while it is used. An
// image of no size paints nothing, and a nil one panics in dev mode.
func (c *Canvas) CreatePattern(img image.Image, repeat PatternRepeat) *CanvasPattern {
	p := &CanvasPattern{m: identity, repeat: repeat}
	if p.repeat > gfx.RepeatNone {
		p.repeat = gfx.RepeatBoth
	}
	if r := c.use("CreatePattern"); r != nil {
		p.img = r.image(c, "CreatePattern", img)
	}
	return p
}

// SetTransform places the pattern: its image goes through the matrix (as in
// [Canvas.Transform]) and then through the transform current when it paints.
func (p *CanvasPattern) SetTransform(m11, m12, m21, m22, dx, dy float64) {
	if finite(m11, m12, m21, m22, dx, dy) {
		p.m = matrix{m11, m12, m21, m22, dx, dy}
	}
}

// paint is as CanvasGradient.paint, without an opacity.
func (p *CanvasPattern) paint(m *matrix, smooth bool) gfx.Paint {
	if p == nil || p.img == nil || p.img.Width() == 0 || p.img.Height() == 0 {
		return nil
	}
	t := m.mul(p.m)
	if _, ok := t.invert(); !ok {
		return nil
	}
	out := gfx.ImagePattern{Image: p.img, Repeat: p.repeat, Transform: t.affine()}
	if !smooth {
		out.Sampling = gfx.SampleNearest
	}
	if !out.Transform.IsFinite() || out.Transform.Det() == 0 {
		return nil
	}
	return out
}

// ink is what a drawing is painted with.
type ink struct {
	// paint is nil for nothing, and for an image, which has no paint.
	paint gfx.Paint
	// alpha is the opacity of the layer that the drawing is painted in, 1
	// for none: the opacity that paint does not have.
	alpha float32
	// color is paint when solid.
	color gfx.Color
	solid bool
}

// Which style an ink is of.
const (
	inkFill = iota
	inkStroke
)

// inkMemo is the last ink made of a style, to make it once for the many
// commands that use it.
type inkMemo struct {
	style Style
	m     matrix // of a gradient or a pattern
	alpha float32
	stops int
	turn  matrix // of a pattern
	flat  bool   // a pattern without smoothing
	ink   ink
}

// ink returns the ink of style at opacity alpha, from 0 to 1, for the fills
// or the strokes (which), in the coordinates that m maps the canvas's current
// ones to. With distorted, there is none, and the drawing must be in the
// current coordinates.
func (r *Recorder) ink(which int, style Style, alpha float32, m *matrix, smooth bool) (k ink, distorted bool) {
	memo := &r.inks[which]
	switch s := style.(type) {
	case nil:
		return ink{}, false
	case gfx.Color:
		if memo.ink.paint != nil && memo.alpha == alpha {
			if c, ok := memo.style.(gfx.Color); ok && c == s {
				return memo.ink, false
			}
		}
		if !(s.A > 0) || alpha == 0 || !s.IsFinite() {
			return ink{}, false
		}
		c := s.Clamp() // as the Builder would, for each command
		c.A = float32(c.A * alpha)
		*memo = inkMemo{style: style, alpha: alpha, ink: ink{paint: c, alpha: 1, color: c, solid: true}}
		return memo.ink, false
	case *CanvasGradient:
		if s == nil || alpha == 0 {
			return ink{}, false
		}
		if memo.ink.paint != nil && memo.alpha == alpha && memo.m == *m && memo.stops == len(s.stops) {
			if g, ok := memo.style.(*CanvasGradient); ok && g == s {
				return memo.ink, false
			}
		}
		paint, distorted := s.paint(m, alpha)
		if paint == nil {
			return ink{}, distorted
		}
		*memo = inkMemo{style: style, alpha: alpha, m: *m, stops: len(s.stops), ink: ink{paint: paint, alpha: 1}}
		return memo.ink, false
	case *CanvasPattern:
		if s == nil || alpha == 0 {
			return ink{}, false
		}
		if memo.ink.paint != nil && memo.alpha == alpha && memo.m == *m && memo.turn == s.m && memo.flat == !smooth {
			if p, ok := memo.style.(*CanvasPattern); ok && p == s {
				return memo.ink, false
			}
		}
		paint := s.paint(m, smooth)
		if paint == nil {
			return ink{}, false
		}
		*memo = inkMemo{style: style, alpha: alpha, m: *m, turn: s.m, flat: !smooth, ink: ink{paint: paint, alpha: alpha}}
		return memo.ink, false
	}
	panic(fmt.Sprintf("tela: a Canvas cannot paint with a %T: FillStyle and StrokeStyle take a tela.Color, a *tela.CanvasGradient or a *tela.CanvasPattern", style))
}
