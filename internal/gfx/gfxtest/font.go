package gfxtest

import (
	"sync/atomic"

	"github.com/kuronosu/tela/internal/gfx"
)

// A synthetic font for the text tests: letter-like glyphs built from paths
// (curves, holes, a stroked outline), color glyphs and an empty glyph. Every
// coordinate is a float32 constant or an explicitly rounded product, so the
// glyphs are the same on every architecture (ADR-027).

// Glyph IDs of a TestFont.
const (
	GlyphSpace   gfx.GlyphID = 0 // no outline, empty bounds
	GlyphO       gfx.GlyphID = 1 // ring: two ellipses, the inner one reversed
	GlyphA       gfx.GlyphID = 2 // triangle with a triangular hole
	GlyphS       gfx.GlyphID = 3 // stroked S curve (self-overlapping outline)
	GlyphL       gfx.GlyphID = 4 // stem with a rounded (quadratic) top
	GlyphI       gfx.GlyphID = 5 // stem and a round dot
	GlyphBadge   gfx.GlyphID = 6 // color: gradient disc, fg star, translucent layer
	GlyphLoop    gfx.GlyphID = 7 // color: draws itself (nesting limit)
	GlyphMissing gfx.GlyphID = 8 // missing from the font
	GlyphMedal   gfx.GlyphID = 9 // color: gradient disc and highlight, no fg
)

// TestRunes maps the characters of the test strings to glyphs.
var TestRunes = map[rune]gfx.GlyphID{
	' ': GlyphSpace, 'o': GlyphO, 'A': GlyphA, 'S': GlyphS, 'l': GlyphL, 'i': GlyphI, '*': GlyphBadge, '@': GlyphLoop, '?': GlyphMissing, '%': GlyphMedal,
}

// TestGlyph is one glyph of a TestFont.
type TestGlyph struct {
	Path   gfx.Path
	Bounds gfx.Rect
	Adv    float32 // advance, in em
	// Color, when not nil, makes the glyph a color glyph: it returns its
	// display list in em space.
	Color func(f *TestFont, fg gfx.Color) *gfx.DisplayList
	NoFG  bool // the color glyph does not paint with fg
}

// TestFont implements gfx.Font (and gfx.ForegroundColorFont). It counts the
// calls that a cache saves.
type TestFont struct {
	key      uint64
	name     string
	Glyphs   map[gfx.GlyphID]*TestGlyph
	Outlines atomic.Int64 // GlyphOutline calls
	Colors   atomic.Int64 // ColorGlyph calls returning a list
}

func (f *TestFont) FontKey() uint64  { return f.key }
func (f *TestFont) FontName() string { return f.name }

func (f *TestFont) GlyphOutline(g gfx.GlyphID) (gfx.Path, bool) {
	f.Outlines.Add(1)
	tg := f.Glyphs[g]
	if tg == nil || tg.Path.IsEmpty() {
		return gfx.Path{}, false
	}
	return tg.Path, true
}

func (f *TestFont) GlyphBounds(g gfx.GlyphID) gfx.Rect {
	if tg := f.Glyphs[g]; tg != nil {
		return tg.Bounds
	}
	return gfx.Rect{}
}

func (f *TestFont) ColorGlyph(g gfx.GlyphID, fg gfx.Color) *gfx.DisplayList {
	tg := f.Glyphs[g]
	if tg == nil || tg.Color == nil {
		return nil
	}
	f.Colors.Add(1)
	return tg.Color(f, fg)
}

// ColorGlyphUsesForeground implements gfx.ForegroundColorFont.
func (f *TestFont) ColorGlyphUsesForeground(g gfx.GlyphID) bool {
	tg := f.Glyphs[g]
	return tg != nil && tg.Color != nil && !tg.NoFG
}

func (f *TestFont) advance(g gfx.GlyphID) float32 {
	if tg := f.Glyphs[g]; tg != nil {
		return tg.Adv
	}
	return 0.5
}

// Shape lays out s on one line at the given size: each character's glyph,
// advanced by its em advance times size.
func (f *TestFont) Shape(s string, size float32) []gfx.Glyph {
	var gs []gfx.Glyph
	var x float32
	for _, ch := range s {
		id := TestRunes[ch]
		gs = append(gs, gfx.Glyph{ID: id, X: x})
		x += float32(f.advance(id) * size)
	}
	return gs
}

// Run returns a DrawGlyphRun of s.
func (f *TestFont) Run(s string, size, x, y float32, p gfx.Paint) gfx.DrawGlyphRun {
	return gfx.DrawGlyphRun{Font: f, Size: size, Origin: gfx.Pt(x, y), Glyphs: f.Shape(s, size), Paint: p}
}

// AddEllipse appends an ellipse of four cubics, clockwise on screen, or
// counterclockwise if reverse.
func AddEllipse(p *gfx.Path, cx, cy, rx, ry float32, reverse bool) {
	const k = 0.5522848
	kx, ky := float32(rx*k), float32(ry*k)
	if !reverse {
		p.MoveTo(gfx.Pt(cx+rx, cy))
		p.CubicTo(gfx.Pt(cx+rx, cy+ky), gfx.Pt(cx+kx, cy+ry), gfx.Pt(cx, cy+ry))
		p.CubicTo(gfx.Pt(cx-kx, cy+ry), gfx.Pt(cx-rx, cy+ky), gfx.Pt(cx-rx, cy))
		p.CubicTo(gfx.Pt(cx-rx, cy-ky), gfx.Pt(cx-kx, cy-ry), gfx.Pt(cx, cy-ry))
		p.CubicTo(gfx.Pt(cx+kx, cy-ry), gfx.Pt(cx+rx, cy-ky), gfx.Pt(cx+rx, cy))
	} else {
		p.MoveTo(gfx.Pt(cx+rx, cy))
		p.CubicTo(gfx.Pt(cx+rx, cy-ky), gfx.Pt(cx+kx, cy-ry), gfx.Pt(cx, cy-ry))
		p.CubicTo(gfx.Pt(cx-kx, cy-ry), gfx.Pt(cx-rx, cy-ky), gfx.Pt(cx-rx, cy))
		p.CubicTo(gfx.Pt(cx-rx, cy+ky), gfx.Pt(cx-kx, cy+ry), gfx.Pt(cx, cy+ry))
		p.CubicTo(gfx.Pt(cx+kx, cy+ry), gfx.Pt(cx+rx, cy+ky), gfx.Pt(cx+rx, cy))
	}
	p.Close()
}

// Polygon appends a closed polygon.
func Polygon(p *gfx.Path, pts ...gfx.Point) {
	p.MoveTo(pts[0])
	for _, q := range pts[1:] {
		p.LineTo(q)
	}
	p.Close()
}

// BadgeList is the color glyph GlyphBadge: a disc with a linear gradient, a
// star in the foreground color, and a translucent highlight in a layer.
func BadgeList(_ *TestFont, fg gfx.Color) *gfx.DisplayList {
	b := gfx.NewBuilder()
	var disc gfx.Path
	AddEllipse(&disc, 0.4, -0.35, 0.36, 0.36, false)
	b.FillPath(disc, gfx.NonZero, gfx.LinearGradient{
		Start: gfx.Pt(0.04, -0.71), End: gfx.Pt(0.76, 0.01),
		Stops: Stops(Amber, Rose, Fuchsia),
	})
	b.FillPath(StarPath(0.4, -0.33, 0.24), gfx.NonZero, fg)
	b.PushLayer(0.5, gfx.SrcOver)
	var hl gfx.Path
	AddEllipse(&hl, 0.3, -0.56, 0.14, 0.06, false)
	b.FillPath(hl, gfx.NonZero, gfx.White)
	b.PopLayer()
	return b.Finish()
}

// MedalList is the color glyph GlyphMedal: GlyphBadge without its star, so it
// does not paint with the foreground color.
func MedalList(_ *TestFont, _ gfx.Color) *gfx.DisplayList {
	b := gfx.NewBuilder()
	var disc gfx.Path
	AddEllipse(&disc, 0.4, -0.35, 0.36, 0.36, false)
	b.FillPath(disc, gfx.NonZero, gfx.LinearGradient{
		Start: gfx.Pt(0.04, -0.71), End: gfx.Pt(0.76, 0.01),
		Stops: Stops(Amber, Rose, Fuchsia),
	})
	b.PushLayer(0.5, gfx.SrcOver)
	var hl gfx.Path
	AddEllipse(&hl, 0.3, -0.56, 0.14, 0.06, false)
	b.FillPath(hl, gfx.NonZero, gfx.White)
	b.PopLayer()
	return b.Finish()
}

// LoopList is the color glyph GlyphLoop: a ring in fg and, inside it, the same
// glyph at half size, which draws itself again until the nesting limit.
func LoopList(f *TestFont, fg gfx.Color) *gfx.DisplayList {
	b := gfx.NewBuilder()
	var ring gfx.Path
	AddEllipse(&ring, 0.4, -0.4, 0.38, 0.38, false)
	AddEllipse(&ring, 0.4, -0.4, 0.3, 0.3, true)
	b.FillPath(ring, gfx.NonZero, fg)
	b.DrawGlyphRun(gfx.DrawGlyphRun{Font: f, Size: 0.5, Origin: gfx.Pt(0.2, -0.2), Glyphs: []gfx.Glyph{{ID: GlyphLoop}}, Paint: Sky})
	return b.Finish()
}

// NewTestFont returns a new font of the synthetic glyphs, with its own
// font key.
func NewTestFont(name string) *TestFont {
	f := &TestFont{key: gfx.NewFontKey(), name: name, Glyphs: map[gfx.GlyphID]*TestGlyph{}}
	add := func(id gfx.GlyphID, p gfx.Path, adv float32) {
		f.Glyphs[id] = &TestGlyph{Path: p, Bounds: p.ControlBounds(), Adv: adv}
	}
	f.Glyphs[GlyphSpace] = &TestGlyph{Adv: 0.3}

	var o gfx.Path
	AddEllipse(&o, 0.3, -0.26, 0.25, 0.27, false)
	AddEllipse(&o, 0.3, -0.26, 0.14, 0.17, true)
	add(GlyphO, o, 0.6)

	var a gfx.Path
	Polygon(&a, gfx.Pt(0.02, 0), gfx.Pt(0.33, -0.72), gfx.Pt(0.64, 0), gfx.Pt(0.5, 0), gfx.Pt(0.33, -0.4), gfx.Pt(0.16, 0))
	Polygon(&a, gfx.Pt(0.2, -0.16), gfx.Pt(0.46, -0.16), gfx.Pt(0.43, -0.24), gfx.Pt(0.23, -0.24))
	add(GlyphA, a, 0.66)

	var sc gfx.Path
	sc.MoveTo(gfx.Pt(0.47, -0.62))
	sc.CubicTo(gfx.Pt(0.38, -0.74), gfx.Pt(0.06, -0.72), gfx.Pt(0.08, -0.54))
	sc.CubicTo(gfx.Pt(0.1, -0.36), gfx.Pt(0.48, -0.4), gfx.Pt(0.48, -0.2))
	sc.CubicTo(gfx.Pt(0.48, 0), gfx.Pt(0.12, 0.02), gfx.Pt(0.04, -0.12))
	st := gfx.NewStrokeStyle(0.1)
	st.Cap, st.Join = gfx.CapRound, gfx.JoinRound
	add(GlyphS, sc.Stroke(st, 0.002), 0.56)

	var l gfx.Path
	l.MoveTo(gfx.Pt(0.08, 0))
	l.LineTo(gfx.Pt(0.08, -0.68))
	l.QuadTo(gfx.Pt(0.14, -0.8), gfx.Pt(0.2, -0.68))
	l.LineTo(gfx.Pt(0.2, 0))
	l.Close()
	add(GlyphL, l, 0.28)

	var i gfx.Path
	Polygon(&i, gfx.Pt(0.08, 0), gfx.Pt(0.08, -0.5), gfx.Pt(0.2, -0.5), gfx.Pt(0.2, 0))
	AddEllipse(&i, 0.14, -0.64, 0.075, 0.075, false)
	add(GlyphI, i, 0.28)

	f.Glyphs[GlyphBadge] = &TestGlyph{Bounds: BadgeList(f, gfx.Black).Bounds(), Adv: 0.8, Color: BadgeList}
	f.Glyphs[GlyphMedal] = &TestGlyph{Bounds: MedalList(f, gfx.Black).Bounds(), Adv: 0.8, Color: MedalList, NoFG: true}
	f.Glyphs[GlyphLoop] = &TestGlyph{Bounds: gfx.Rect{X0: 0, Y0: -0.8, X1: 0.8, Y1: 0}, Adv: 0.84, Color: LoopList}
	return f
}

// FontA is the font of the text scenes.
var FontA = NewTestFont("Synthetic Sans")
