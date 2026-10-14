package core

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Painting of text fields (ADR-069). A field's content is a layer of its
// own (layerField, layer.go) recorded where the field paints its content:
// a frame that only moves the caret, blinks it, changes the selection or
// edits a field whose size does not depend on its text records that layer
// again and nothing else. In it, in order:
//
//   - the selection highlight (focused field, non-empty selection): the
//     selection: background, or Chromium's (rgb(51, 103, 209) in an
//     active window, rgb(200, 200, 200) in an inactive one);
//   - the text, or the placeholder while the value is empty;
//   - the selected text again in the selection's text color, clipped to
//     the highlight (Chromium's white in an active window, unless
//     selection: classes set colors);
//   - the IME composition's underline: a solid line under the preedit,
//     thicker under the IME's highlighted range;
//   - the caret (caretShown): one device-pixel-snapped logical pixel wide,
//     the line box tall, in the caret-color (the text color by default).
//
// An Input clips its text to its content box horizontally (the inner
// editor of Chromium) and paints it moved by its horizontal scroll, its
// line centered vertically; a Textarea's text is painted in its scroll
// container's content layer, clipped by its overflow clip.

// Chromium's default selection colors (headless Chromium 152 on Linux).
var (
	selectionActiveBg   = gfx.RGB8(51, 103, 209)
	selectionActiveFg   = gfx.White
	selectionInactiveBg = gfx.RGB8(200, 200, 200)
	selectionInactiveFg = gfx.RGB8(50, 50, 50)
)

// paintField records the content of text field bx in its layer.
func (p *painter) paintField(bx *Box) {
	p.openLayer(layerField, bx, p.space)
	p.drawField(bx)
	p.closeLayer()
}

// selectionColors returns the selection's background and text paints for
// a field with style cs; a nil text paint keeps the text's colors.
func (t *Tree) selectionColors(cs *style.ComputedStyle) (bg gfx.Color, fg gfx.Paint) {
	bg, fgc := selectionActiveBg, selectionActiveFg
	if !t.foc.window {
		bg, fgc = selectionInactiveBg, selectionInactiveFg
	}
	fg = fgc
	if sel := cs.Selection; sel != nil {
		fg = nil
		if sel.HasBackground {
			bg = sel.Background
		}
		if sel.HasColor {
			fg = sel.Color
		}
	}
	return bg, fg
}

// drawField draws the content of text field bx (see the comment at the top
// of fieldpaint.go). Both callers check that bx has a field.
func (p *painter) drawField(bx *Box) {
	f := bx.fld
	f.paintDirty = false
	t := p.t
	cs := bx.pcs
	if cs == nil || cs.Visibility != style.VisibilityVisible {
		return
	}
	lay := f.layout()
	if lay == nil {
		return
	}
	l := bx.ld.Layout()
	ox, oy := f.textOrigin()
	origin := gfx.Point{X: bx.u.X0 + ox, Y: bx.u.Y0 + oy}
	scale := float32(p.scale)
	clipped := false
	if !f.multi {
		c, pad := contentRect(bx.u, l), paddingRect(bx.u, l)
		p.b.PushClipRect(gfx.Rect{X0: c.X0, Y0: pad.Y0, X1: c.X1, Y1: pad.Y1})
		clipped = true
	}
	focused := t.foc.el == f.el
	sel := f.ed.Selection()
	var rects []gfx.Rect
	var fg gfx.Paint
	// During a composition the IME's highlighted range is the thick
	// underline, not a selection (Chromium).
	if focused && !sel.IsCollapsed() && !f.ed.Composing() {
		var bg gfx.Color
		bg, fg = t.selectionColors(cs)
		rects = lay.SelectionRects(sel, nil)
		for i, r := range rects {
			rects[i] = r.Translate(origin.X, origin.Y)
			p.b.FillRect(rects[i], bg)
		}
	}
	if f.ed.RawValue() == "" {
		if ph := f.placeholder(cs, f.textWidth()); ph != nil {
			ph.PaintScaled(p.b, origin, scale)
		}
	} else {
		lay.PaintScaled(p.b, origin, scale)
	}
	if fg != nil {
		for _, r := range rects {
			p.b.PushClipRect(r)
			lay.PaintWith(p.b, origin, scale, fg)
			p.b.PopClip()
		}
	}
	if comp, ok := f.ed.Composition(); ok && comp.End > comp.Start {
		p.drawComposition(lay, origin, cs, comp.Start, comp.End, comp.CursorStart, comp.CursorEnd)
	}
	if t.caretShown(f) {
		c := lay.Caret(sel.Focus)
		x := p.snap(origin.X + c.X)
		col := cs.Text.Color
		if cs.Caret.Set {
			col = cs.Caret.Color
		}
		p.b.FillRect(gfx.Rect{X0: x, Y0: origin.Y + c.Top, X1: x + p.snapWidth(caretWidth), Y1: origin.Y + c.Top + c.Height}, col)
	}
	if clipped {
		p.b.PopClip()
	}
}

// drawComposition underlines the preedit [start, end) of lay painted at
// origin: one device pixel under the text, two under the IME's
// highlighted range [hs, he) (when it is not empty), in the text color.
func (p *painter) drawComposition(lay *text.Layout, origin gfx.Point, cs *style.ComputedStyle, start, end, hs, he int) {
	col := cs.Text.Color
	under := func(s, e int, thick float32) {
		rects := lay.SelectionRects(text.NewSelection(text.Cursor{Offset: s}, text.Cursor{Offset: e}), nil)
		for _, r := range rects {
			ln := lay.Line(lineAtTop(lay, r.Y0))
			y := p.snap(origin.Y + ln.Baseline + max(1, ln.Descent/2) - thick/2)
			x0, x1 := origin.X+r.X0, origin.X+r.X1
			if r.Width() > 2 {
				x0, x1 = x0+1, x1-1 // a gap between clauses
			}
			p.b.FillRect(gfx.Rect{X0: x0, Y0: y, X1: x1, Y1: y + thick}, col)
		}
	}
	thin := p.snapWidth(1)
	under(start, end, thin)
	if hs < he && hs >= start && he <= end {
		under(hs, he, p.snapWidth(2))
	}
}

// lineAtTop returns the line of lay whose top is y (the line of a
// selection rectangle).
func lineAtTop(lay *text.Layout, y float32) int {
	best, bestD := 0, float32(1e30)
	for i := range lay.NumLines() {
		d := lay.Line(i).Top - y
		if d < 0 {
			d = -d
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// recordFields records again the layers of the fields marked for
// painting (markPaint), on a frame that records nothing else.
func (t *Tree) recordFields(p *painter) {
	L := &t.lay
	if !L.fieldDirty {
		return
	}
	L.fieldDirty = false
	for _, l := range L.layers {
		if l.kind != layerField || l.box.fld == nil || !l.box.fld.paintDirty || l.box.pcs == nil {
			continue
		}
		p.b, p.layer = L.builder(), l
		p.drawField(l.box)
		l.dl = p.b.Finish()
		L.free = append(L.free, p.b)
		l.paint = L.paints
		L.recorded++
	}
	p.b, p.layer = nil, nil
}
