package text

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Paint draws the visible lines with origin as the top-left corner of the
// layout box: one DrawGlyphRun per run of glyphs with the same font, size
// and paint, underlines and overlines below the text and line-throughs
// above it (CSS Text Decoration 3, painting order), as rectangles merged
// across adjacent clusters of equal decoration. Inline boxes are not
// painted: their positions come from BoxRect. Neither are inlines: their
// fragments come from InlineFragments.
func (l *Layout) Paint(b *gfx.Builder, origin gfx.Point) { l.PaintScaled(b, origin, 0) }

// PaintScaled is Paint for a display at the given scale (device pixels
// per logical pixel; 0 for none): underlines are then snapped to device
// pixels as Blink draws them (see paintDecorations).
func (l *Layout) PaintScaled(b *gfx.Builder, origin gfx.Point, scale float32) {
	l.PaintWith(b, origin, scale, nil)
}

// PaintWith is PaintScaled with every glyph run and decoration painted
// with paint instead of its style's (nil keeps the styles' paints): the
// selected text of a text field, drawn again in the selection's color
// under a clip. Color glyphs (emoji) keep their own colors.
func (l *Layout) PaintWith(b *gfx.Builder, origin gfx.Point, scale float32, paint gfx.Paint) {
	l.PaintLinesWith(b, origin, 0, len(l.lines), scale, paint)
}

// PaintLinesWith is PaintWith for lines i0 to i1 (exclusive), clamped to
// the layout's: the lines of a text field that its visible area can meet.
func (l *Layout) PaintLinesWith(b *gfx.Builder, origin gfx.Point, i0, i1 int, scale float32, paint gfx.Paint) {
	var gs []gfx.Glyph
	o := paintOver{all: paint}
	for li := max(i0, 0); li < min(i1, len(l.lines)); li++ {
		l.paintDecorations(b, origin, li, DecorationUnderline|DecorationOverline, scale, o)
		gs = l.paintGlyphs(b, origin, li, gs, o)
		l.paintDecorations(b, origin, li, DecorationLineThrough, scale, o)
	}
}

// paintOver replaces paints while painting: all replaces every paint
// (PaintWith); styles[s], when set, replaces the paint of the text of
// style s and of its decorations that have the text's paint
// (PaintLineRecolored).
type paintOver struct {
	all    gfx.Paint
	styles []gfx.Paint
}

// glyph returns the paint of the glyphs of style s.
func (o paintOver) glyph(l *Layout, s int) gfx.Paint {
	if o.all != nil {
		return o.all
	}
	if s < len(o.styles) && o.styles[s] != nil {
		return o.styles[s]
	}
	return l.sinfo[s].paint
}

// deco returns the paint of the decorations of style s: the text's, when
// they have no color of their own (currentColor).
func (o paintOver) deco(l *Layout, s int) gfx.Paint {
	if o.all != nil {
		return o.all
	}
	si := &l.sinfo[s]
	if s < len(o.styles) && o.styles[s] != nil && samePaint(si.decoPaint, si.paint) {
		return o.styles[s]
	}
	return si.decoPaint
}

// PaintLine draws line i as PaintScaled does, for callers that paint
// something between lines (the backgrounds of inline boxes, which CSS
// paints line by line: a later line's backgrounds cover an earlier line's
// text).
func (l *Layout) PaintLine(b *gfx.Builder, origin gfx.Point, i int, scale float32) {
	l.PaintLineWith(b, origin, i, scale, nil)
}

// PaintLineWith is PaintLine with every glyph run and decoration painted
// with paint, as in PaintWith (nil keeps the styles' paints).
func (l *Layout) PaintLineWith(b *gfx.Builder, origin gfx.Point, i int, scale float32, paint gfx.Paint) {
	if i < 0 || i >= len(l.lines) {
		return
	}
	o := paintOver{all: paint}
	l.paintDecorations(b, origin, i, DecorationUnderline|DecorationOverline, scale, o)
	l.paintGlyphs(b, origin, i, nil, o)
	l.paintDecorations(b, origin, i, DecorationLineThrough, scale, o)
}

// PaintRecolored is PaintScaled with the paints of some text styles
// replaced, as PaintLineRecolored does for one line; nil paints is
// PaintScaled.
func (l *Layout) PaintRecolored(b *gfx.Builder, origin gfx.Point, scale float32, paints []gfx.Paint) {
	var gs []gfx.Glyph
	o := paintOver{styles: paints}
	for li := range l.lines {
		l.paintDecorations(b, origin, li, DecorationUnderline|DecorationOverline, scale, o)
		gs = l.paintGlyphs(b, origin, li, gs, o)
		l.paintDecorations(b, origin, li, DecorationLineThrough, scale, o)
	}
}

// PaintLineRecolored is PaintLine with the paints of some text styles
// replaced: when paints[s] is not nil, the glyphs of style s and its
// decorations that have the text's paint (a decoration without a color
// of its own) are painted with it. Styles are numbered as the paragraph
// gives them: 0 is Paragraph.Default, 1 to n its Spans in order, then the
// styles of its Inlines. Color glyphs (emoji) keep their own colors.
// Recoloring changes no geometry: the lines, their ink and the
// decorations stay where they are.
func (l *Layout) PaintLineRecolored(b *gfx.Builder, origin gfx.Point, i int, scale float32, paints []gfx.Paint) {
	if i < 0 || i >= len(l.lines) {
		return
	}
	o := paintOver{styles: paints}
	l.paintDecorations(b, origin, i, DecorationUnderline|DecorationOverline, scale, o)
	l.paintGlyphs(b, origin, i, nil, o)
	l.paintDecorations(b, origin, i, DecorationLineThrough, scale, o)
}

// SetPaint replaces the paint of style s and that of its decorations (nil:
// the text's, currentColor), the styles numbered as in
// PaintLineRecolored: 0 is Paragraph.Default, 1 to n its Spans, then its
// Inlines. Paints only color what is painted: the shaping, the lines and
// every geometry stay as they are, exactly as NewLayout lays out the
// paragraph with the new paints. An s out of range does nothing.
func (l *Layout) SetPaint(s int, paint, decoration gfx.Paint) {
	if s < 0 || s >= len(l.styles) {
		return
	}
	st := &l.styles[s]
	st.Paint, st.Decoration.Paint = paint, decoration
	si := &l.sinfo[s]
	si.paint = st.paint()
	si.decoPaint = decoration
	if si.decoPaint == nil {
		si.decoPaint = si.paint
	}
}

// snappedUnderline returns the top (relative to the baseline at y, in
// absolute logical px) and the thickness of an underline of style s for a
// display scale, as Blink computes them in device pixels: the thickness
// is the style's, or a tenth of the font size (at least 1), floored to
// whole device pixels (at least 1), and the line starts max(1,
// ceil(thickness/2)) device pixels below the baseline rounded to the
// device grid (text-underline-position: auto).
func snappedUnderline(s *TextStyle, size, y, scale float32) (top, thick float32) {
	t := s.Decoration.Thickness
	if !(t > 0) || !finite32(t) {
		t = max(1, size/10)
	}
	td := float64(t) * float64(scale)
	gap := max(1, math.Ceil(td/2))
	th := max(1, math.Floor(td))
	base := math.Round(float64(y) * float64(scale))
	return float32((base+gap)/float64(scale)) - y, float32(th / float64(scale))
}

// samePaint reports whether two paints are certainly equal (solid colors
// are compared, gradients only by style).
func samePaint(a, b gfx.Paint) bool {
	ca, ok1 := a.(gfx.Color)
	cb, ok2 := b.(gfx.Color)
	return ok1 && ok2 && ca == cb
}

func (l *Layout) paintGlyphs(b *gfx.Builder, origin gfx.Point, li int, gs []gfx.Glyph, over paintOver) []gfx.Glyph {
	ln := &l.lines[li]
	by := origin.Y + ln.baseline
	var (
		face  *Face
		size  float32
		style = -1
		x0    float32
	)
	flush := func() {
		if face != nil && len(gs) > 0 {
			p := over.glyph(l, style)
			b.DrawGlyphRun(gfx.DrawGlyphRun{Font: face, Size: size, Origin: gfx.Pt(origin.X+x0, by), Glyphs: gs, Paint: p})
		}
		gs = gs[:0]
	}
	start := func(f *Face, sz float32, s int, x float32) {
		if f == face && sz == size && (s == style || samePaint(over.glyph(l, s), over.glyph(l, style))) {
			return
		}
		flush()
		face, size, style, x0 = f, sz, s, x
	}
	for k := ln.i0; k < ln.i1; k++ {
		it := &l.items[k]
		switch it.kind {
		case itemEllipsis:
			e := &l.ellipses[it.run]
			if e.face == nil {
				continue
			}
			x := ln.x + it.x
			start(e.face, e.size, int(e.style), x)
			ls := l.styles[e.style].LetterSpacing
			for _, g := range e.glyphs {
				gs = append(gs, gfx.Glyph{ID: g.id, X: x - x0})
				x += g.adv + ls
			}
		case itemText:
			r := &l.runs[it.run]
			if r.kind != runText {
				continue
			}
			visit := func(c int32) {
				cl := &l.clusters[c]
				if cl.g0 == cl.g1 {
					return
				}
				x := l.cx[c]
				start(r.face, r.size, int(cl.style), x)
				for g := cl.g0; g < cl.g1; g++ {
					gd := &l.glyphs[g]
					gs = append(gs, gfx.Glyph{ID: gd.id, X: x + gd.dx - x0, Y: gd.dy})
					x += gd.adv
				}
			}
			if it.level%2 == 0 {
				for c := it.c0; c < it.c1; c++ {
					visit(c)
				}
			} else {
				for c := it.c1 - 1; c >= it.c0; c-- {
					visit(c)
				}
			}
		}
	}
	flush()
	return gs
}

// paintDecorations draws the decoration lines in mask of line li; at a
// display scale (not 0), underlines are snapped to its device pixels.
func (l *Layout) paintDecorations(b *gfx.Builder, origin gfx.Point, li int, mask Decoration, scale float32, over paintOver) {
	ln := &l.lines[li]
	by := origin.Y + ln.baseline
	type seg struct {
		x0, x1, y, h float32
		paint        gfx.Paint
		style        uint16
	}
	var cur [3]seg // underline, overline, line-through
	var open [3]bool
	emit := func(k int) {
		if open[k] && cur[k].x1 > cur[k].x0 && cur[k].h > 0 {
			s := cur[k]
			b.FillRect(gfx.Rect{X0: origin.X + s.x0, Y0: by + s.y, X1: origin.X + s.x1, Y1: by + s.y + s.h}, s.paint)
		}
		open[k] = false
	}
	for k := ln.i0; k < ln.i1; k++ {
		it := &l.items[k]
		if it.kind != itemText {
			continue
		}
		visit := func(c int32) {
			cl := &l.clusters[c]
			st := &l.styles[cl.style]
			lines := st.Decoration.Lines & mask
			if c >= ln.trail || cl.flags&cfNewline != 0 {
				lines = 0
			}
			si := &l.sinfo[cl.style]
			dp := over.deco(l, int(cl.style))
			x0, x1 := l.cx[c], l.cx[c]+l.cadv[c]
			for kind := range 3 {
				bit := Decoration(1 << kind)
				if lines&bit == 0 {
					emit(kind)
					continue
				}
				var y, h float32
				switch bit {
				case DecorationUnderline:
					y, h = si.underline, si.underlineThick
					if scale > 0 {
						y, h = snappedUnderline(st, si.size, by, scale)
					}
				case DecorationOverline:
					y, h = si.overline, si.underlineThick
				default:
					y, h = si.strike, si.strikeThick
				}
				s := &cur[kind]
				if open[kind] && s.y == y && s.h == h && (s.style == cl.style || samePaint(s.paint, dp)) &&
					absf(s.x1-x0) < 0.01 {
					s.x1 = x1
					continue
				}
				emit(kind)
				cur[kind] = seg{x0, x1, y, h, dp, cl.style}
				open[kind] = true
			}
		}
		if it.level%2 == 0 {
			for c := it.c0; c < it.c1; c++ {
				visit(c)
			}
		} else {
			for c := it.c1 - 1; c >= it.c0; c-- {
				visit(c)
			}
		}
	}
	for k := range 3 {
		emit(k)
	}
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
