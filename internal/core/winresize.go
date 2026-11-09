package core

import (
	"fmt"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Resize handles (ADR-257): WindowResize marks a band along the inside of
// an element's painted border box as the window's resize handles, sides
// and corners, for windows whose OS draws no resize border of their own
// (Transparent windows, whose edges are a transparent margin, and any
// Frameless one). After the subtree of the element, the walk of
// winregion.go appends the band's rectangles as platform.HitResize…
// regions (on top of the regions inside the element), each cut by the
// overflow clips of the ancestors and by the window; the backends start
// the OS's resize from them and show its cursors, except where the window
// cannot be resized (FixedSize, maximized, fullscreen). The band lies on
// the element's own pixels, so a Transparent window's click-through mask
// (platform.InputMask) keeps it clickable.

// Edge is a set of an element's sides, for ResizeEdges.
type Edge uint8

const (
	EdgeTop Edge = 1 << iota
	EdgeBottom
	EdgeLeft
	EdgeRight

	// EdgeAll is the four sides.
	EdgeAll = EdgeTop | EdgeBottom | EdgeLeft | EdgeRight
)

func (e Edge) String() string {
	if e == 0 || e > EdgeAll {
		return fmt.Sprintf("Edge(%d)", uint8(e))
	}
	return platform.Edge(e).String()
}

const (
	// DefaultResizeBand is the default width of the band in logical
	// pixels: the backends' own band (platform.ResizeEdge).
	DefaultResizeBand = 5
	// resizeCornerLen is how far along each side a corner handle reaches
	// (at least the band's width), the backends' 16.
	resizeCornerLen = 16
)

// resizeSpec is a WindowResize prop: the band's width and its sides. The
// zero value is no prop; it is comparable.
type resizeSpec struct {
	on    bool
	band  float32
	edges Edge
}

// ResizeOption configures WindowResize (ResizeBand, ResizeEdges).
type ResizeOption struct {
	apply func(*resizeSpec)
}

// ResizeBand sets the band's width in logical pixels (DefaultResizeBand
// by default). It must be positive and finite.
func ResizeBand(width float32) ResizeOption {
	if !(width > 0) || math.IsInf(float64(width), 0) {
		panic(fmt.Sprintf("tela: ResizeBand(%v): the band's width must be a positive number of logical pixels, such as 6", width))
	}
	return ResizeOption{apply: func(s *resizeSpec) { s.band = width }}
}

// ResizeEdges sets the sides that resize (EdgeAll by default); a corner
// handle exists where both of its sides do.
func ResizeEdges(edges Edge) ResizeOption {
	if edges == 0 || edges > EdgeAll {
		panic(fmt.Sprintf("tela: ResizeEdges(%d): pass tela.EdgeTop, EdgeBottom, EdgeLeft or EdgeRight, combined with |, or tela.EdgeAll", uint8(edges)))
	}
	return ResizeOption{apply: func(s *resizeSpec) { s.edges = edges }}
}

// WindowResize returns the prop that makes the inside of an element's
// edges the window's resize handles (see the top of winresize.go).
func WindowResize(opts ...ResizeOption) Prop {
	s := resizeSpec{on: true, band: DefaultResizeBand, edges: EdgeAll}
	for _, o := range opts {
		if o.apply == nil {
			panic("h: WindowResize: an option is the zero tela.ResizeOption; use tela.ResizeBand or tela.ResizeEdges")
		}
		o.apply(&s)
	}
	return Prop{name: "WindowResize", apply: func(d *desc) { d.winResize = s }}
}

// resizeChanged records that primitive el's WindowResize prop mounted
// (on) or went.
func (t *Tree) resizeChanged(el *Element, on bool) { t.countRegion(el, on) }

// appendResize appends the resize handles of el (spec s) to out, in el's
// bounds as painted, each cut by the clips above el and by the window.
func (t *Tree) appendResize(out []platform.HitRegion, el *Element, s resizeSpec, vp gfx.Rect) []platform.HitRegion {
	b := t.elemBounds(el)
	if b.Empty() {
		return out
	}
	for _, h := range resizeHandles(b, s) {
		r := t.clipUp(el, h.r).Intersect(vp)
		if r.Empty() {
			continue
		}
		out = append(out, platform.HitRegion{Kind: h.kind, Rect: platform.Rect{X: r.X0, Y: r.Y0, W: r.Width(), H: r.Height()}})
	}
	return out
}

// resizeHandle is one rectangle of a band.
type resizeHandle struct {
	kind platform.HitKind
	r    gfx.Rect
}

// resizeHandles returns the handles of a band along the inside of b: one
// rectangle per side in s.edges, the band wide (at most half of b across
// it, so opposite sides do not overlap), then each corner whose two sides
// are in s.edges as an L over them: max(band, 16) along each side (at
// most half of b), the band wide.
func resizeHandles(b gfx.Rect, s resizeSpec) []resizeHandle {
	w, h := b.Width(), b.Height()
	bx, by := min(s.band, w/2), min(s.band, h/2)
	cl := max(s.band, resizeCornerLen)
	cx, cy := min(cl, w/2), min(cl, h/2)
	var out []resizeHandle
	add := func(e Edge, r gfx.Rect) {
		if s.edges&e == e && !r.Empty() {
			out = append(out, resizeHandle{platform.HitResize(platform.Edge(e)), r})
		}
	}
	add(EdgeTop, gfx.Rect{X0: b.X0, Y0: b.Y0, X1: b.X1, Y1: b.Y0 + by})
	add(EdgeBottom, gfx.Rect{X0: b.X0, Y0: b.Y1 - by, X1: b.X1, Y1: b.Y1})
	add(EdgeLeft, gfx.Rect{X0: b.X0, Y0: b.Y0, X1: b.X0 + bx, Y1: b.Y1})
	add(EdgeRight, gfx.Rect{X0: b.X1 - bx, Y0: b.Y0, X1: b.X1, Y1: b.Y1})
	// x0, y0 is the corner; dx, dy point into b.
	corner := func(e Edge, x0, y0, dx, dy float32) {
		span := func(o, d, n float32) (float32, float32) { return min(o, o+d*n), max(o, o+d*n) }
		ax0, ax1 := span(x0, dx, cx)
		ay0, ay1 := span(y0, dy, by)
		add(e, gfx.Rect{X0: ax0, Y0: ay0, X1: ax1, Y1: ay1})
		bx0, bx1 := span(x0, dx, bx)
		by0, by1 := span(y0, dy, cy)
		add(e, gfx.Rect{X0: bx0, Y0: by0, X1: bx1, Y1: by1})
	}
	corner(EdgeTop|EdgeLeft, b.X0, b.Y0, 1, 1)
	corner(EdgeTop|EdgeRight, b.X1, b.Y0, -1, 1)
	corner(EdgeBottom|EdgeLeft, b.X0, b.Y1, 1, -1)
	corner(EdgeBottom|EdgeRight, b.X1, b.Y1, -1, -1)
	return out
}
