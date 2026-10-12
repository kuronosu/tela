package core

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
)

// Paint phase (ARCHITECTURE sections 8 and 13). One display list holds the
// whole tree for now; layers and repaint boundaries arrive in phase 4.
//
// The painter walks the box tree in tree order. For each box in the tree
// that is not display: none, paintBox computes its border box in absolute
// logical pixels from the unrounded layout and calls, in order:
//
//  1. paintDecorations: what is drawn under the content, from the box's
//     paint-phase style (ComputedStyle.Resolved(style.PhasePaint)). Phase 3
//     paints the background color only; the root's background is the
//     canvas (painted first, over the whole viewport).
//  2. paintContent: the content of a leaf, in its content box: a
//     paragraph's text (broken and aligned at the content width first), or
//     an image (stretched to the content box).
//  3. the children, recursively, with this box's border-box origin.
//
// Boxes with visibility: hidden or collapse skip 1 and 2 but still paint
// their children, which may be visible.
//
// T4b plugs in here: borders, radii, box shadows, gradients and image
// layers in paintDecorations; opacity, transforms, filters, backdrop
// filters, overflow clipping, z-order and stacking contexts as pushes
// around 1-3 in paintBox (the painter is where the state they need
// lives); outlines after 3; and pixel snapping of the border box (the
// rect computed in paintBox) for crisp edges.

// painter is the state of one paint pass.
type painter struct {
	t *Tree
	b *gfx.Builder
}

// paintPhase paints the tree into a new display list.
func (t *Tree) paintPhase() *gfx.DisplayList {
	t.stats.Paints++
	b := t.builder
	if t.rootBox != nil && t.rootBox.cs != nil {
		b.FillRect(gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}, t.CanvasColor())
		p := painter{t: t, b: b}
		p.paintBox(t.rootBox, gfx.Point{})
	}
	return b.Finish()
}

// paintBox paints box bx and its subtree; origin is the absolute position
// of the border box of bx's layout parent (zero for the root).
func (p *painter) paintBox(bx *Box, origin gfx.Point) {
	if bx.ls.Display == layout.DisplayNone {
		return
	}
	l := bx.ld.Layout()
	r := gfx.RectFromOriginSize(origin.Add(l.Location), l.Size)
	cs := bx.cs
	if bx.deferred {
		cs = cs.Resolved(style.PhasePaint)
	}
	if cs.Visibility == style.VisibilityVisible {
		p.paintDecorations(bx, cs, r)
		p.paintContent(bx, cs, r, l)
	}
	o := r.Origin()
	for _, k := range bx.kids {
		p.paintBox(k, o)
	}
}

// paintDecorations paints what lies under the content of box bx, whose
// border box is r: in phase 3, the background color (the root's is the
// canvas).
func (p *painter) paintDecorations(bx *Box, cs *style.ComputedStyle, r gfx.Rect) {
	if bx == p.t.rootBox {
		return
	}
	if c := cs.BackgroundColor; c.A > 0 {
		p.b.FillRect(r, c)
	}
}

// paintContent paints the content of a leaf box in its content box.
func (p *painter) paintContent(bx *Box, cs *style.ComputedStyle, r gfx.Rect, l *layout.Layout) {
	switch bx.mode {
	case modeParagraph:
		para := bx.para
		if para == nil || para.lay == nil {
			return
		}
		c := contentRect(r, l)
		para.prepare(p.t, max(c.Width(), 0))
		para.lay.Paint(p.b, c.Origin())
	case modeImage:
		if bx.img != nil {
			p.b.DrawImage(bx.img, gfx.Rect{}, contentRect(r, l), gfx.SampleLinear)
		}
	}
}
