package gfx

import (
	"slices"
	"sync/atomic"
)

// GlyphID identifies a glyph inside a font (an OpenType glyph index).
type GlyphID uint32

// Font is a font face instance, variation coordinates included, that can
// produce the shapes of its glyphs. gfx defines it so that the DisplayList
// and the rasterizers do not depend on a font library: package text
// implements it on top of go-text/typesetting.
//
// Glyph geometry is in em space: 1 em is 1 unit, y grows downward and the
// origin is the pen position on the baseline, so one outline serves every
// size (DrawGlyphRun scales it by Size).
//
// Implementations must be safe for concurrent use: raster workers call them
// from several goroutines at once. The results for a given glyph must never
// change: rasterizers cache them, keyed by FontKey.
type Font interface {
	// FontKey identifies this face instance in rasterizer caches. Two Fonts
	// with equal keys must produce identical glyphs; NewFontKey returns
	// process-unique keys.
	FontKey() uint64
	// FontName is a human-readable name, used by Dump (for example
	// "Inter Bold").
	FontName() string
	// GlyphOutline returns the outline of glyph g in em space, filled with
	// the NonZero rule. ok is false for glyphs without an outline (a space)
	// and for missing glyphs.
	GlyphOutline(g GlyphID) (p Path, ok bool)
	// GlyphBounds returns conservative em-space bounds of everything glyph g
	// can paint (its outline or its color glyph). An empty Rect means the
	// glyph paints nothing. It is called for every glyph of every run the
	// Builder records and by rasterizers to cull glyphs, so it must be cheap.
	GlyphBounds(g GlyphID) Rect
	// ColorGlyph returns, for a color glyph (COLR, bitmap or SVG glyphs
	// converted to drawing commands), a DisplayList in em space that paints
	// it, with fg as the foreground color (CSS currentColor). It returns nil
	// for outline glyphs; whether it returns nil must not depend on fg. It is
	// called whenever a rasterizer meets a glyph that it has not cached, so
	// it must be cheap for outline glyphs.
	ColorGlyph(g GlyphID, fg Color) *DisplayList
}

var nextFontKey atomic.Uint64

// NewFontKey returns a process-unique, non-zero font key for a new Font.
func NewFontKey() uint64 { return nextFontKey.Add(1) }

// Glyph is one positioned glyph of a run.
type Glyph struct {
	ID GlyphID
	// X, Y is the pen position of the glyph relative to the run's Origin,
	// in the run's (local) coordinate space.
	X, Y float32
}

// DrawGlyphRun draws glyphs of one font at one size. Glyph i is drawn with
// its em-space shape scaled by Size and its pen position at Origin +
// (Glyphs[i].X, Glyphs[i].Y).
//
// Outline glyphs are filled (NonZero) with Paint, which is in the local
// coordinate space like any other paint. Color glyphs paint themselves:
// their DisplayList (Font.ColorGlyph) is drawn with Paint's color as the
// foreground when Paint is a Color, and with a transparent foreground when it
// is a gradient (as in CSS, where gradient text is drawn with a transparent
// color, so foreground layers vanish).
//
// Rasterizers may snap the pen position to their device grid (see package
// raster: x to a quarter of a device pixel and y to whole device pixels
// under scale/translate transforms).
type DrawGlyphRun struct {
	Font   Font
	Size   float32 // em size in local units
	Origin Point   // baseline origin in the local coordinate space
	Glyphs []Glyph
	Paint  Paint
}

func (DrawGlyphRun) isCommand()   {}
func (DrawGlyphRun) Name() string { return "DrawGlyphRun" }

// DrawGlyphRun records a run of glyphs; see the DrawGlyphRun command. The
// glyph slice is copied. A nil Font or Paint panics. The run is dropped if
// Size is not positive, a position is not finite, the paint is not finite,
// or no glyph has non-empty bounds (Font.GlyphBounds). A fully transparent
// Paint is kept: color glyphs paint their own colors.
func (b *Builder) DrawGlyphRun(run DrawGlyphRun) {
	b.begin()
	if run.Font == nil {
		panic("gfx.Builder.DrawGlyphRun: Font is nil; pass a font face (package text)")
	}
	checkPaint("DrawGlyphRun", run.Paint)
	if b.suppressed() {
		return
	}
	p, ok := sanitizeGlyphPaint(run.Paint)
	if !ok || !finite(run.Size) || !(run.Size > 0) || !run.Origin.IsFinite() {
		return
	}
	var local Rect
	for _, g := range run.Glyphs {
		if !finite(g.X) || !finite(g.Y) {
			return
		}
		gb := run.Font.GlyphBounds(g.ID)
		if gb.Empty() || !gb.IsFinite() {
			continue
		}
		// Explicit float32 conversions keep the products from being fused
		// into the additions (ADR-027).
		x, y := run.Origin.X+g.X, run.Origin.Y+g.Y
		local = local.Union(Rect{
			X0: x + float32(gb.X0*run.Size), Y0: y + float32(gb.Y0*run.Size),
			X1: x + float32(gb.X1*run.Size), Y1: y + float32(gb.Y1*run.Size),
		})
	}
	if local.Empty() {
		return
	}
	if bb, ok := b.drawBounds(local); ok {
		run.Paint = p
		run.Glyphs = slices.Clone(run.Glyphs)
		b.record(run, bb)
	}
}

// sanitizeGlyphPaint is sanitizePaint without the visibility test: a
// transparent text color still lets color glyphs paint.
func sanitizeGlyphPaint(p Paint) (Paint, bool) {
	if c, ok := p.(Color); ok {
		if !c.IsFinite() {
			return nil, false
		}
		return c.Clamp(), true
	}
	if s, ok := sanitizePaint(p); ok {
		return s, true
	}
	// A valid gradient whose stops are all transparent.
	var stops []GradientStop
	switch g := p.(type) {
	case LinearGradient:
		if !g.Start.IsFinite() || !g.End.IsFinite() {
			return nil, false
		}
		stops = g.Stops
	case RadialGradient:
		if !g.Center.IsFinite() || !g.Radius.IsFinite() {
			return nil, false
		}
		stops = g.Stops
	case ConicGradient:
		if !g.Center.IsFinite() || !finite(g.StartAngle) {
			return nil, false
		}
		stops = g.Stops
	case FocalGradient:
		if !g.Focal.IsFinite() || !g.Center.IsFinite() || !finite(g.FocalRadius) || !finite(g.Radius) {
			return nil, false
		}
		stops = g.Stops
	default:
		return nil, false
	}
	if len(stops) == 0 {
		return nil, false
	}
	for _, s := range stops {
		if !s.Color.IsFinite() {
			return nil, false
		}
	}
	return Transparent, true
}
