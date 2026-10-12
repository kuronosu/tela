// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (round_layout in src/compute/mod.rs); see
// THIRD_PARTY.md.

package layout

import "github.com/kuronosu/tela/internal/gfx"

// Round snaps the layout of the tree rooted at root to whole logical
// pixels, into each node's RoundedLayout (the unrounded layout is kept).
// Edges are rounded at their cumulative position from the root and sizes
// taken as differences of rounded edges, so adjacent boxes stay adjacent
// and no gaps appear (Yoga's and Taffy's scheme). Values round half away
// from zero.
//
// Chromium computes layout in LayoutUnits (1/64 px fixed point) and snaps
// boxes to device pixels when painting in the same way: the left and top
// edges round to the nearest pixel and the size follows from the rounded
// right and bottom edges. core may paint either the unrounded layout
// (anti-aliased edges) or the rounded one (crisp edges at scale 1).
//
// Unlike Taffy, which rounds each location relative to its parent, the
// rounded location is the difference of the rounded cumulative positions
// of the node and its parent, so a node's rounded edges, summed from the
// root, are exactly its rounded cumulative edges.
//
// perf: Round visits the whole tree (about 70 ns per node); rounding only
// the subtrees laid out since the last run would make it incremental.
func Round(root Node) {
	roundNode(root, 0, 0)
}

func roundNode(n Node, px, py float32) {
	d := n.LayoutData()
	u := &d.unrounded
	r := *u
	cx := px + u.Location.X
	cy := py + u.Location.Y

	r.Location = gfx.Point{X: round(cx) - round(px), Y: round(cy) - round(py)}
	r.Size = gfx.Size{W: round(cx+u.Size.W) - round(cx), H: round(cy+u.Size.H) - round(cy)}
	r.ScrollbarSize = gfx.Size{W: round(u.ScrollbarSize.W), H: round(u.ScrollbarSize.H)}
	r.Border = roundInsets(u.Border, u.Size, cx, cy)
	r.Padding = roundInsets(u.Padding, u.Size, cx, cy)
	r.ScrollableOverflow = gfx.Rect{
		X0: round(cx+u.ScrollableOverflow.X0) - round(cx),
		X1: round(cx+u.ScrollableOverflow.X1) - round(cx),
		Y0: round(cy+u.ScrollableOverflow.Y0) - round(cy),
		Y1: round(cy+u.ScrollableOverflow.Y1) - round(cy),
	}
	if u.HasBaseline {
		r.FirstBaseline = round(cy+u.FirstBaseline) - round(cy)
	}
	d.rounded = r

	for i, c := 0, n.LayoutChildCount(); i < c; i++ {
		roundNode(n.LayoutChild(i), cx, cy)
	}
}

// roundInsets rounds the edges of a box at their cumulative positions.
func roundInsets(in Insets, sz gfx.Size, cx, cy float32) Insets {
	return Insets{
		Left:   round(cx+in.Left) - round(cx),
		Right:  round(cx+sz.W) - round(cx+sz.W-in.Right),
		Top:    round(cy+in.Top) - round(cy),
		Bottom: round(cy+sz.H) - round(cy+sz.H-in.Bottom),
	}
}
