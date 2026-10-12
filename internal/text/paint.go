package text

import "github.com/kuronosu/tela/internal/gfx"

// Paint draws the visible lines with origin as the top-left corner of the
// layout box: one DrawGlyphRun per run of glyphs with the same font, size
// and paint, underlines and overlines below the text and line-throughs
// above it (CSS Text Decoration 3, painting order), as rectangles merged
// across adjacent clusters of equal decoration. Inline boxes are not
// painted: their positions come from BoxRect.
func (l *Layout) Paint(b *gfx.Builder, origin gfx.Point) {
	var gs []gfx.Glyph
	for li := range l.lines {
		l.paintDecorations(b, origin, li, DecorationUnderline|DecorationOverline)
		gs = l.paintGlyphs(b, origin, li, gs)
		l.paintDecorations(b, origin, li, DecorationLineThrough)
	}
}

// samePaint reports whether two paints are certainly equal (solid colors
// are compared, gradients only by style).
func samePaint(a, b gfx.Paint) bool {
	ca, ok1 := a.(gfx.Color)
	cb, ok2 := b.(gfx.Color)
	return ok1 && ok2 && ca == cb
}

func (l *Layout) paintGlyphs(b *gfx.Builder, origin gfx.Point, li int, gs []gfx.Glyph) []gfx.Glyph {
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
			b.DrawGlyphRun(gfx.DrawGlyphRun{Font: face, Size: size, Origin: gfx.Pt(origin.X+x0, by), Glyphs: gs, Paint: l.sinfo[style].paint})
		}
		gs = gs[:0]
	}
	start := func(f *Face, sz float32, s int, x float32) {
		if f == face && sz == size && (s == style || samePaint(l.sinfo[s].paint, l.sinfo[style].paint)) {
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

// paintDecorations draws the decoration lines in mask of line li.
func (l *Layout) paintDecorations(b *gfx.Builder, origin gfx.Point, li int, mask Decoration) {
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
				case DecorationOverline:
					y, h = si.overline, si.underlineThick
				default:
					y, h = si.strike, si.strikeThick
				}
				s := &cur[kind]
				if open[kind] && s.y == y && s.h == h && (s.style == cl.style || samePaint(s.paint, si.decoPaint)) &&
					absf(s.x1-x0) < 0.01 {
					s.x1 = x1
					continue
				}
				emit(kind)
				cur[kind] = seg{x0, x1, y, h, si.decoPaint, cl.style}
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
