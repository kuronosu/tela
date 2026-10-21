package core

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Sticky positioning (ARCHITECTURE section 9, CSS Positioned Layout 3
// section 3.4), after Chromium's StickyPositionScrollingConstraints.
//
// A sticky box is laid out like a relative one without an offset (its
// insets are not a relative offset, stylemap.go). When it is painted, its
// insets keep it inside its sticky view: the scrollport of its nearest
// scroll container ancestor (the window for the root), without the
// container's padding (Chromium), each inset that is not auto pushing the
// box in from that side, while its margin box stays inside its containing
// block (the content box of its parent; of the whole scrollable area when
// the parent is the scroll container itself). The offsets are computed
// like Chromium's: right, then left, then bottom, then top, each from the
// box's position without the offset, so left and top win; percentages of
// the insets resolve against the sticky view's size.
//
// The offset is a paint offset only: a box is moved by its own scroll
// space (Box.stick, a scroller with sticky set), whose offset is the
// negated sticky offset, snapped to device pixels. The box, its in-flow
// descendants and the positioned descendants it contains are painted in
// that space (place sets Box.space), so hit-testing, the compositor and
// the damage see the stuck position through shiftOf like any scroll
// offset, and a scroll recomputes the offsets (painter.updateSticky)
// without recording anything again: the chunk of a sticky space is moved
// by the compositor (layer.go), and is never shifted in the framebuffer
// as a scrolled unit (damage.go), since its pixels do not move with its
// scroll container's.
//
// In a VirtualList, an item's wrapper is transparent: the containing block
// of a sticky item is the list's content box, so section headers stay
// stuck while their items scroll under them (the next header covers the
// previous one, as in a flat HTML list), and the list keeps the last
// sticky item above its window mounted (virtual.go).

// isSticky reports whether bx is positioned sticky (the root never is).
func isSticky(bx *Box) bool {
	return bx.pcs != nil && bx.pcs.Position == style.PositionSticky && bx != bx.tree.rootBox
}

// stickySpace returns the sticky scroll space of bx, created on first use;
// up is the space bx would be painted in without it.
func stickySpace(bx *Box, up *scroller) *scroller {
	k := bx.stick
	if k == nil {
		k = &scroller{box: bx, sticky: true}
		bx.stick = k
	}
	k.up = up
	k.interest = unbounded
	return k
}

// stickyContainer returns the nearest scroll container ancestor of bx.
func stickyContainer(bx *Box) *Box {
	for a := bx.parent; a != nil; a = a.parent {
		if a.scr != nil {
			return a
		}
	}
	return nil
}

// stickyBlock returns the containing block of sticky box bx: its parent,
// or the content box of a VirtualList for an item's wrapper.
func stickyBlock(bx *Box) *Box {
	p := bx.parent
	if v := p.virtMark(); v != nil && v.role == virtItem && p.parent != nil {
		return p.parent
	}
	return p
}

// placeSticky computes the sticky offset of bx, placed already in the
// space its sticky space goes up to, and stores it in that space.
func (p *painter) placeSticky(bx *Box) {
	k := bx.stick
	off := p.stickyOffset(bx, k.up)
	k.soff = gfx.Point{X: -off.X, Y: -off.Y}
}

// updateSticky recomputes the offsets of the sticky boxes after scroll
// offsets changed (a frame that only repaints), in tree order so that an
// outer sticky box moves before the ones inside it.
func (p *painter) updateSticky() {
	t := p.t
	for _, bx := range t.stickies {
		if bx.stick == nil || bx.space != bx.stick || !t.inTree(bx) {
			continue
		}
		p.placeSticky(bx)
	}
}

// stickyOffset returns the offset that keeps sticky box bx in its sticky
// view; up is the scroll space its unstuck position bx.u is in.
func (p *painter) stickyOffset(bx *Box, up *scroller) gfx.Point {
	cs := bx.pcs
	in := &cs.Inset
	if in.Top.Kind != style.DimLength && in.Bottom.Kind != style.DimLength &&
		in.Left.Kind != style.DimLength && in.Right.Kind != style.DimLength {
		return gfx.Point{}
	}
	sb := stickyContainer(bx)
	cbx := stickyBlock(bx)
	if sb == nil || cbx == nil || cbx.pcs == nil {
		return gfx.Point{}
	}
	s := sb.scr
	space := contentSpace(sb)
	sl := sb.ld.Layout()
	// The sticky view: the scrollport without the gutters and the
	// padding, at the current offset, in the coordinates of the
	// container's content.
	pad := paddingRect(sb.u, sl)
	port := gfx.Rect{X0: pad.X0, Y0: pad.Y0, X1: pad.X1 - sl.ScrollbarSize.W, Y1: pad.Y1 - sl.ScrollbarSize.H}
	if s != nil && s.rtl {
		port.X0, port.X1 = port.X0+sl.ScrollbarSize.W, port.X1+sl.ScrollbarSize.W
	}
	view := port.InsetEdges(sl.Padding.Left, sl.Padding.Top, sl.Padding.Right, sl.Padding.Bottom)
	if space == s {
		view = view.Translate(s.soff.X, s.soff.Y)
	}
	// The containing block's content box, without the box's margins.
	var block gfx.Rect
	if cbx == sb {
		// The scroll container: its whole scrollable area.
		area := gfx.Rect{X0: port.X0, Y0: port.Y0, X1: port.X1 + s.max.X, Y1: port.Y1 + s.max.Y}
		if s.rtl {
			area.X0, area.X1 = port.X0-s.max.X, port.X1 // the content overflows to the left
		}
		block = area.InsetEdges(sl.Padding.Left, sl.Padding.Top, sl.Padding.Right, sl.Padding.Bottom)
	} else {
		block = contentRect(cbx.u, cbx.ld.Layout())
		o := toSpace(gfx.Point{}, cbx.space, space)
		block = block.Translate(o.X, o.Y)
	}
	l := bx.ld.Layout()
	block = block.InsetEdges(l.Margin.Left, l.Margin.Top, l.Margin.Right, l.Margin.Bottom)
	o := toSpace(gfx.Point{}, up, space)
	box := bx.u.Translate(o.X, o.Y)

	var off gfx.Point
	w, h := max(view.Width(), 0), max(view.Height(), 0)
	if in.Right.Kind == style.DimLength {
		d := min(0, view.X1-in.Right.LP.Resolve(w)-box.X1)
		if avail := min(0, block.X0-box.X0); d < avail {
			d = avail
		}
		off.X += d
	}
	if in.Left.Kind == style.DimLength {
		d := max(0, view.X0+in.Left.LP.Resolve(w)-box.X0)
		if avail := max(0, block.X1-box.X1); d > avail {
			d = avail
		}
		off.X += d
	}
	if in.Bottom.Kind == style.DimLength {
		d := min(0, view.Y1-in.Bottom.LP.Resolve(h)-box.Y1)
		if avail := min(0, block.Y0-box.Y0); d < avail {
			d = avail
		}
		off.Y += d
	}
	if in.Top.Kind == style.DimLength {
		d := max(0, view.Y0+in.Top.LP.Resolve(h)-box.Y0)
		if avail := max(0, block.Y1-box.Y1); d > avail {
			d = avail
		}
		off.Y += d
	}
	// Whole device pixels, like scroll offsets: the box keeps its
	// snapped edges and its chunk moves by integer translations.
	return gfx.Point{X: p.snap(off.X), Y: p.snap(off.Y)}
}

// stickyShift returns how far bx's own sticky offset moves it (zero for
// a box that is not sticky), from the last paint.
func stickyShift(bx *Box) gfx.Point {
	if k := bx.stick; k != nil && bx.space == k {
		return gfx.Point{X: -k.soff.X, Y: -k.soff.Y}
	}
	return gfx.Point{}
}
