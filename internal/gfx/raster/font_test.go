package raster

import (
	"sync/atomic"

	"github.com/kuronosu/tela/internal/gfx"
)

// A synthetic font for the text tests: letter-like glyphs built from paths
// (curves, holes, a stroked outline), color glyphs and an empty glyph. Every
// coordinate is a float32 constant or an explicitly rounded product, so the
// glyphs are the same on every architecture (ADR-027).

// Glyph IDs of testFont.
const (
	gSpace   gfx.GlyphID = 0 // no outline, empty bounds
	gO       gfx.GlyphID = 1 // ring: two ellipses, the inner one reversed
	gA       gfx.GlyphID = 2 // triangle with a triangular hole
	gS       gfx.GlyphID = 3 // stroked S curve (self-overlapping outline)
	gL       gfx.GlyphID = 4 // stem with a rounded (quadratic) top
	gI       gfx.GlyphID = 5 // stem and a round dot
	gBadge   gfx.GlyphID = 6 // color: gradient disc, fg star, translucent layer
	gLoop    gfx.GlyphID = 7 // color: draws itself (nesting limit)
	gMissing gfx.GlyphID = 8 // missing from the font
)

// testRunes maps the characters of the test strings to glyphs.
var testRunes = map[rune]gfx.GlyphID{
	' ': gSpace, 'o': gO, 'A': gA, 'S': gS, 'l': gL, 'i': gI, '*': gBadge, '@': gLoop, '?': gMissing,
}

type testGlyph struct {
	path   gfx.Path
	bounds gfx.Rect
	adv    float32
	color  func(f *testFont, fg gfx.Color) *gfx.DisplayList
}

// testFont implements gfx.Font. It counts the calls that a cache saves.
type testFont struct {
	key     uint64
	name    string
	glyphs  map[gfx.GlyphID]*testGlyph
	outline atomic.Int64 // GlyphOutline calls
	colors  atomic.Int64 // ColorGlyph calls returning a list
}

func (f *testFont) FontKey() uint64  { return f.key }
func (f *testFont) FontName() string { return f.name }

func (f *testFont) GlyphOutline(g gfx.GlyphID) (gfx.Path, bool) {
	f.outline.Add(1)
	tg := f.glyphs[g]
	if tg == nil || tg.path.IsEmpty() {
		return gfx.Path{}, false
	}
	return tg.path, true
}

func (f *testFont) GlyphBounds(g gfx.GlyphID) gfx.Rect {
	if tg := f.glyphs[g]; tg != nil {
		return tg.bounds
	}
	return gfx.Rect{}
}

func (f *testFont) ColorGlyph(g gfx.GlyphID, fg gfx.Color) *gfx.DisplayList {
	tg := f.glyphs[g]
	if tg == nil || tg.color == nil {
		return nil
	}
	f.colors.Add(1)
	return tg.color(f, fg)
}

func (f *testFont) advance(g gfx.GlyphID) float32 {
	if tg := f.glyphs[g]; tg != nil {
		return tg.adv
	}
	return 0.5
}

// shape lays out s on one line at the given size: each character's glyph,
// advanced by its em advance times size.
func (f *testFont) shape(s string, size float32) []gfx.Glyph {
	var gs []gfx.Glyph
	var x float32
	for _, ch := range s {
		id := testRunes[ch]
		gs = append(gs, gfx.Glyph{ID: id, X: x})
		x += float32(f.advance(id) * size)
	}
	return gs
}

// run returns a DrawGlyphRun of s.
func (f *testFont) run(s string, size, x, y float32, p gfx.Paint) gfx.DrawGlyphRun {
	return gfx.DrawGlyphRun{Font: f, Size: size, Origin: gfx.Pt(x, y), Glyphs: f.shape(s, size), Paint: p}
}

// addEllipse appends an ellipse of four cubics, clockwise on screen, or
// counterclockwise if reverse.
func addEllipse(p *gfx.Path, cx, cy, rx, ry float32, reverse bool) {
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

func polygon(p *gfx.Path, pts ...gfx.Point) {
	p.MoveTo(pts[0])
	for _, q := range pts[1:] {
		p.LineTo(q)
	}
	p.Close()
}

// badgeList is the color glyph gBadge: a disc with a linear gradient, a
// star in the foreground color, and a translucent highlight in a layer.
func badgeList(_ *testFont, fg gfx.Color) *gfx.DisplayList {
	b := gfx.NewBuilder()
	var disc gfx.Path
	addEllipse(&disc, 0.4, -0.35, 0.36, 0.36, false)
	b.FillPath(disc, gfx.NonZero, gfx.LinearGradient{
		Start: gfx.Pt(0.04, -0.71), End: gfx.Pt(0.76, 0.01),
		Stops: stops(amber, rose, fuchsia),
	})
	b.FillPath(starPath(0.4, -0.33, 0.24), gfx.NonZero, fg)
	b.PushLayer(0.5, gfx.SrcOver)
	var hl gfx.Path
	addEllipse(&hl, 0.3, -0.56, 0.14, 0.06, false)
	b.FillPath(hl, gfx.NonZero, gfx.White)
	b.PopLayer()
	return b.Finish()
}

// loopList is the color glyph gLoop: a ring in fg and, inside it, the same
// glyph at half size, which draws itself again until the nesting limit.
func loopList(f *testFont, fg gfx.Color) *gfx.DisplayList {
	b := gfx.NewBuilder()
	var ring gfx.Path
	addEllipse(&ring, 0.4, -0.4, 0.38, 0.38, false)
	addEllipse(&ring, 0.4, -0.4, 0.3, 0.3, true)
	b.FillPath(ring, gfx.NonZero, fg)
	b.DrawGlyphRun(gfx.DrawGlyphRun{Font: f, Size: 0.5, Origin: gfx.Pt(0.2, -0.2), Glyphs: []gfx.Glyph{{ID: gLoop}}, Paint: sky})
	return b.Finish()
}

func newTestFont(name string) *testFont {
	f := &testFont{key: gfx.NewFontKey(), name: name, glyphs: map[gfx.GlyphID]*testGlyph{}}
	add := func(id gfx.GlyphID, p gfx.Path, adv float32) {
		f.glyphs[id] = &testGlyph{path: p, bounds: p.ControlBounds(), adv: adv}
	}
	f.glyphs[gSpace] = &testGlyph{adv: 0.3}

	var o gfx.Path
	addEllipse(&o, 0.3, -0.26, 0.25, 0.27, false)
	addEllipse(&o, 0.3, -0.26, 0.14, 0.17, true)
	add(gO, o, 0.6)

	var a gfx.Path
	polygon(&a, gfx.Pt(0.02, 0), gfx.Pt(0.33, -0.72), gfx.Pt(0.64, 0), gfx.Pt(0.5, 0), gfx.Pt(0.33, -0.4), gfx.Pt(0.16, 0))
	polygon(&a, gfx.Pt(0.2, -0.16), gfx.Pt(0.46, -0.16), gfx.Pt(0.43, -0.24), gfx.Pt(0.23, -0.24))
	add(gA, a, 0.66)

	var sc gfx.Path
	sc.MoveTo(gfx.Pt(0.47, -0.62))
	sc.CubicTo(gfx.Pt(0.38, -0.74), gfx.Pt(0.06, -0.72), gfx.Pt(0.08, -0.54))
	sc.CubicTo(gfx.Pt(0.1, -0.36), gfx.Pt(0.48, -0.4), gfx.Pt(0.48, -0.2))
	sc.CubicTo(gfx.Pt(0.48, 0), gfx.Pt(0.12, 0.02), gfx.Pt(0.04, -0.12))
	st := gfx.NewStrokeStyle(0.1)
	st.Cap, st.Join = gfx.CapRound, gfx.JoinRound
	add(gS, sc.Stroke(st, 0.002), 0.56)

	var l gfx.Path
	l.MoveTo(gfx.Pt(0.08, 0))
	l.LineTo(gfx.Pt(0.08, -0.68))
	l.QuadTo(gfx.Pt(0.14, -0.8), gfx.Pt(0.2, -0.68))
	l.LineTo(gfx.Pt(0.2, 0))
	l.Close()
	add(gL, l, 0.28)

	var i gfx.Path
	polygon(&i, gfx.Pt(0.08, 0), gfx.Pt(0.08, -0.5), gfx.Pt(0.2, -0.5), gfx.Pt(0.2, 0))
	addEllipse(&i, 0.14, -0.64, 0.075, 0.075, false)
	add(gI, i, 0.28)

	f.glyphs[gBadge] = &testGlyph{bounds: badgeList(f, gfx.Black).Bounds(), adv: 0.8, color: badgeList}
	f.glyphs[gLoop] = &testGlyph{bounds: gfx.Rect{X0: 0, Y0: -0.8, X1: 0.8, Y1: 0}, adv: 0.84, color: loopList}
	return f
}

// testFontA is the font of the text scenes.
var testFontA = newTestFont("Synthetic Sans")
