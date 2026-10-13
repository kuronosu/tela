package layout

import "github.com/kuronosu/tela/internal/gfx"

// Node is a node of the tree being laid out. core implements it on its Box
// tree. During Compute the style, the children and the measure results of
// every node must not change; between runs they may, as long as the
// changed nodes are passed to MarkDirty.
type Node interface {
	// LayoutStyle returns the node's style.
	LayoutStyle() *Style
	// LayoutChildCount returns the number of children (display: none and
	// absolutely positioned children included).
	LayoutChildCount() int
	// LayoutChild returns child i.
	LayoutChild(i int) Node
	// LayoutData returns the node's layout state: its results and caches.
	// It must return the same pointer on every call.
	LayoutData() *Data
}

// Measurer is implemented by leaves (nodes without children) that have
// content of their own: text, images. A leaf that is not a Measurer has an
// empty content box.
type Measurer interface {
	// Measure returns the size of the node's content box (and its first
	// baseline) under the given constraints. It must be a pure function of
	// its input and the node's content: layout caches its results until
	// the node is passed to MarkDirty.
	Measure(in MeasureInput) Measurement
}

// MeasureInput is a measurement request for a leaf's content box.
type MeasureInput struct {
	// Width and Height are content-box dimensions that are already known;
	// HasWidth and HasHeight report which. A known dimension must be
	// returned unchanged ("what is the height of this text at this
	// width?").
	Width, Height       float32
	HasWidth, HasHeight bool
	// AvailableWidth and AvailableHeight are the space available to the
	// content box in each axis: a definite amount (wrap text to it, as
	// CSS fit-content: min(max-content, max(min-content, available))), or
	// a min-content or max-content constraint (return the min-content or
	// max-content size). Definite amounts are never negative.
	AvailableWidth, AvailableHeight AvailableSpace
}

// Measurement is the result of Measure.
type Measurement struct {
	// Width and Height are the content-box size.
	Width, Height float32
	// Baseline is the first baseline, measured from the top of the content
	// box, when HasBaseline is set (text). Nodes without one are aligned
	// by their bottom edge in baseline alignment.
	Baseline    float32
	HasBaseline bool
	// LastBaseline is the baseline of the last line (the baseline of an
	// inline-block holding the node), when HasLastBaseline is set; else
	// the first baseline is used.
	LastBaseline    float32
	HasLastBaseline bool
}

// Layout is the computed layout of one node. Coordinates are logical
// pixels.
type Layout struct {
	// Location is the position of the node's border box relative to its
	// parent's border box. It includes relative offsets (insets of
	// position: relative); scroll offsets are core's.
	Location gfx.Point
	// Size is the border-box size.
	Size gfx.Size
	// Border, Padding and Margin are the resolved edges (auto margins
	// resolved to their used values).
	Border, Padding, Margin Insets
	// ScrollbarSize is the space taken by scrollbars: Width by a vertical
	// scrollbar (on the right in LTR, on the left in RTL), Height by a
	// horizontal one (at the bottom).
	ScrollbarSize gfx.Size
	// ScrollableOverflow is the node's scrollable overflow rectangle (the
	// border boxes of its in-flow and positioned descendants plus their
	// own non-clipped overflow), measured from the node's scroll origin:
	// the inline-start, block-start corner of its padding box (top-left in
	// LTR, top-right in RTL, with x growing towards the inline end).
	// X0 and Y0 are <= 0 and X1 and Y1 >= 0.
	ScrollableOverflow gfx.Rect
	// FirstBaseline is the distance from the top of the border box to the
	// node's first baseline, when HasBaseline is set.
	FirstBaseline float32
	HasBaseline   bool
}

// Insets holds four edge lengths.
type Insets struct{ Top, Right, Bottom, Left float32 }

// ScrollWidth returns the maximum horizontal scroll offset: how far the
// content reaches past the padding box, at least 0.
func (l *Layout) ScrollWidth() float32 {
	return fmax(0, l.ScrollableOverflow.X1+fmin(l.ScrollbarSize.W, l.Size.W)-l.Size.W+l.Border.Left+l.Border.Right)
}

// ScrollHeight returns the maximum vertical scroll offset.
func (l *Layout) ScrollHeight() float32 {
	return fmax(0, l.ScrollableOverflow.Y1+fmin(l.ScrollbarSize.H, l.Size.H)-l.Size.H+l.Border.Top+l.Border.Bottom)
}

func insets(r rect) Insets { return Insets{Top: r.t, Right: r.r, Bottom: r.b, Left: r.l} }

// Data is the layout state that layout keeps on each node: the computed
// layout, the pixel-rounded layout and the caches. core embeds one in each
// Box and returns it from LayoutData. The zero Data is ready to use.
type Data struct {
	parent Node // the parent that last laid this node out

	unrounded Layout
	rounded   Layout

	cache cache

	// last is the input of the node's last full layout, out its output:
	// relayout boundaries are re-laid out from them.
	last       input
	lastOut    output
	hasLast    bool
	boundary   bool // the node is a relayout boundary (see MarkDirty)
	needLayout bool // the node is a dirty relayout boundary

	// static is the static position of an absolutely positioned node,
	// relative to its parent's border box, recorded by the parent.
	static staticPos
}

// Layout returns the node's computed layout (unrounded).
func (d *Data) Layout() *Layout { return &d.unrounded }

// RoundedLayout returns the node's layout snapped to whole pixels by Round.
func (d *Data) RoundedLayout() *Layout { return &d.rounded }

// setLayout records the unrounded layout of a node.
func (d *Data) setLayout(loc point, sz size, border, padding, margin rect, scrollbar point, overflow rect, baseline opt) {
	d.unrounded = Layout{
		Location:           gfx.Point{X: loc.x, Y: loc.y},
		Size:               gfx.Size{W: sz.w, H: sz.h},
		Border:             insets(border),
		Padding:            insets(padding),
		Margin:             insets(margin),
		ScrollbarSize:      gfx.Size{W: scrollbar.x, H: scrollbar.y},
		ScrollableOverflow: gfx.Rect{X0: overflow.l, Y0: overflow.t, X1: overflow.r, Y1: overflow.b},
		FirstBaseline:      baseline.v,
		HasBaseline:        baseline.ok,
	}
}

// setHidden records the zero layout of a hidden node.
func (d *Data) setHidden() { d.unrounded = Layout{} }
