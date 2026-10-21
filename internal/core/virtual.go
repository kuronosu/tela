package core

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
)

// Virtualization (ARCHITECTURE section 9, after TanStack Virtual v3 and
// Flutter's slivers): VirtualList builds, lays out and paints only the
// items of a long list that intersect what the window shows, plus a few
// more (the overscan).
//
// # The nodes
//
// VirtualList returns a component node (virtualList) whose Element keeps
// the list's state (a vlist, in its only hook slot). Its build returns
// the scroll container, a Div with the user's props (overflow-y-auto
// unless the props set another overflow), holding one internal content
// Div that holds one wrapper Div per built item, keyed by the item's index
// (so an item component's state follows its index). The content box is as
// tall as the whole list: the sum of the sizes of all items, measured or
// estimated, so the scrollbar, ScrollHeight and the semantics of the
// container reflect every item. The wrappers are in block flow: the first
// wrapper of each run of consecutive indices gets a top margin that skips
// the items before it that are not built, so each item is laid out at its
// offset; the others follow their predecessor. The wrappers and the
// content box have no roles: the semantics tree and the aria snapshot see
// the list's items as its children.
//
// # Sizes and offsets
//
// Every item has a size: its estimate (estimateSize, called for every
// index when the list is created, when count grows or when estimateSize is
// another function) until it is built and laid out, then its measured
// height (the wrapper's border box, which holds the item's margins). The
// sizes and their prefix sums live in a Fenwick tree (sizeIndex), so
// offsets, the item at an offset and size updates take O(log n): a list
// of a million items costs about 12 MB and no per-frame O(n) work.
// Measurements are kept per index: count changes keep the sizes of the
// indices that remain (TanStack keys its size cache by index too), a
// width change re-measures the built items after the next layout and
// keeps the last measurement of the others until they are built again,
// and only a new estimateSize function (another function, not the same
// function literal with other captured values) resets them all.
//
// # The window
//
// The visible part of the list is its scrollport intersected with the
// scrollports of the scroll containers around it and with the window
// (a list with no height of its own is still bounded by what the window
// shows). The built range is the visible items plus overscan items on
// each side (5 by default), plus the item holding the focus and its two
// neighbors, which stay mounted while it is scrolled away (TanStack's
// "keep mounted" range), so the focus survives scrolling and Tab moves
// on from it. A scroll that keeps the visible items and one more on each
// side inside the built range changes nothing but the offset: that frame
// only paints. A scroll past it builds the list again: the items that
// stay keep their nodes (the same node is not built again, so their
// components are not rebuilt), new items are rendered and mounted, the
// items that left are unmounted. This hysteresis is why the overscan is
// larger than TanStack's 1: Tela builds the new window synchronously
// before painting, so the overscan is not there to hide blank areas but
// to make most scroll frames paint-only (a wheel notch is about one row);
// it also keeps the next item built for Tab and the arrow keys. A range
// that is more than the overscan larger than wanted on a side (built from
// estimates that were too small, or for a list that got shorter) is
// trimmed; steady scrolling never gets there.
//
// Before its first layout a list builds its first item and the overscan
// only (TanStack's empty initial rect); the pass after the layout builds
// the rest of the window in the same frame. A list that is not displayed
// (display: none on it or an ancestor) builds nothing, and keeps its items
// and their state while it is hidden.
//
// Tab moves the focus along the built items in index order; the window
// follows the focused item when the frame after the key reveals it, so a
// Tab pressed before that frame runs leaves the list after its last built
// item.
//
// Who triggers what: any scroll offset change (setScroll: wheel, keys,
// scrollbars, touch, animation frames, ScrollTo, ScrollToIndex) checks
// every list's window in O(log n) and marks the list for building when
// the visible items left the built range; the build, style, structure and
// layout phases then run for the list's subtree only. Without that mark
// the frame is paint-only (Tree.Frame skips the other phases).
//
// # Measuring and the stable position
//
// In the scroll phase, after layout and before paint (virtualPass), every
// built item's height is compared with its size. A different size updates
// the index; when the item starts above the top of the scrollport the
// scroll offset moves by the difference in the same frame (TanStack's rule
// item.start < scrollOffset), so the items at and below the top of the
// scrollport keep their place on screen: an item measured while scrolling
// up, partly visible at the top, grows upwards. The content box and the
// run margins are then laid out again, the window checked again with the
// new sizes and the new offset, and the loop repeats (at most
// maxVirtualPasses times per frame; the next frame goes on if it did not
// settle). Anchoring moves only the list's own offset: a list without a
// height of its own that scrolls with the page is not anchored.
//
// # ScrollToIndex
//
// ElementRef.ScrollToIndex follows TanStack's scrollToIndex: AlignStart,
// AlignCenter and AlignEnd put the item's start, center or end at the
// scrollport's; AlignAuto scrolls only when the item is not fully
// visible, to the nearer edge (end when the item reaches past the bottom,
// start when it starts above the top). The offset is computed from the
// sizes known then, so the list keeps the item aligned while the items it
// builds are measured (TanStack's scroll reconciliation): each pass
// computes the offset again and jumps (or retargets a smooth scroll) when
// it moved, until a pass finds it in place, the user scrolls, or
// alignTimeout passes. It jumps or animates like ScrollTo (scroll-smooth).
// Home and End on a list keep the first or last item aligned the same way,
// so End reaches the end of a list whose last items are taller than their
// estimates.

// defaultOverscan is the number of items built beyond each side of the
// visible ones (see the comment at the top of virtual.go).
const defaultOverscan = 5

// maxVirtualPasses bounds the measuring passes of one frame (each may
// build, lay out and measure again).
const maxVirtualPasses = 6

// maxWindow bounds the visible items of one window, so that thousands of
// zero-height items cannot all be built at once.
const maxWindow = 4096

// alignTimeout ends the alignment of a ScrollToIndex that never settles
// (TanStack's MAX_RECONCILE_MS).
const alignTimeout = 5 * time.Second

// ScrollAlign is where ElementRef.ScrollToIndex puts an item in the
// scrollport (TanStack Virtual's align).
type ScrollAlign uint8

const (
	// AlignAuto scrolls as little as possible: nothing when the item is
	// fully visible, else to align its end (when it reaches past the
	// bottom) or its start.
	AlignAuto ScrollAlign = iota
	// AlignStart puts the item's top at the top of the scrollport.
	AlignStart
	// AlignCenter centers the item in the scrollport.
	AlignCenter
	// AlignEnd puts the item's bottom at the bottom of the scrollport.
	AlignEnd
)

func (a ScrollAlign) String() string {
	switch a {
	case AlignAuto:
		return "AlignAuto"
	case AlignStart:
		return "AlignStart"
	case AlignCenter:
		return "AlignCenter"
	case AlignEnd:
		return "AlignEnd"
	}
	return fmt.Sprintf("ScrollAlign(%d)", uint8(a))
}

// virtRole is the part of a virtual list a node is.
type virtRole uint8

const (
	virtList    virtRole = iota + 1 // the scroll container
	virtContent                     // the box as tall as the list
	virtItem                        // the wrapper of one item
)

// virtNode marks the primitive nodes a VirtualList builds (desc.virt).
type virtNode struct {
	vl   *vlist
	role virtRole
	// idx is an item wrapper's index.
	idx int
	// overscan is the Overscan prop, in the list's props template only.
	overscan int
}

// virtMark returns the virtual-list mark of box b's node, or nil.
func (b *Box) virtMark() *virtNode {
	if b.el == nil {
		return nil
	}
	return b.el.node.d.virt
}

// vprops are the props of one VirtualList node.
type vprops struct {
	count    int
	estimate func(int) float32
	estFn    uintptr // the code pointer of estimate
	render   func(int) Node
	// outer is the scroll container's node without children: the props
	// applied once.
	outer    *desc
	overscan int
}

// virtState is the virtualization state of a Tree (UI thread).
type virtState struct {
	// lists are the mounted virtual lists.
	lists []*vlist
	// again reports that a frame ended before its lists settled: the next
	// frame goes on.
	again bool
}

// vlist is the state of one mounted VirtualList: the hook slot of its
// component Element.
type vlist struct {
	tree *Tree
	el   *Element
	// props are the props of the latest node, estFn the code pointer of
	// the estimate function the sizes come from.
	props    *vprops
	estFn    uintptr
	overscan int
	sizes    sizeIndex
	// lo and hi are the built range, built every built index (the range
	// and the items kept mounted), ascending. guessed reports a range
	// built before the list was laid out, which the next pass replaces.
	lo, hi  int
	built   []int
	guessed bool
	// nodes are the wrapper nodes of the built items, reused by the
	// builds that only move the window.
	nodes map[int]Node
	// The marks of the scroll container and of the content box.
	listMark, contentMark *virtNode
	// req is a ScrollToIndex waiting for a current layout; align keeps an
	// item aligned while measurements move it.
	req   *alignReq
	align *alignState
	// sticky are the indices of the items seen sticky when they were last
	// built, ascending (noteSticky).
	sticky []int
}

// noteSticky records whether built item i is sticky (its box is
// position: sticky).
func (vl *vlist) noteSticky(i int, sticky bool) {
	k, found := slices.BinarySearch(vl.sticky, i)
	switch {
	case sticky && !found:
		vl.sticky = slices.Insert(vl.sticky, k, i)
	case !sticky && found:
		vl.sticky = slices.Delete(vl.sticky, k, k+1)
	}
}

// stickyBefore returns the last item before index lo that was sticky when
// it was last built, or -1: kept mounted, since it may be stuck over the
// items of the window. An item never built is not known to be sticky.
func (vl *vlist) stickyBefore(lo int) int {
	k, _ := slices.BinarySearch(vl.sticky, min(lo, vl.sizes.len()))
	if k == 0 {
		return -1
	}
	return vl.sticky[k-1]
}

type alignReq struct {
	idx   int
	align ScrollAlign
}

// alignState is an item kept aligned: where the list was sent (last) and
// since when.
type alignState struct {
	idx   int
	align ScrollAlign // AlignStart, AlignCenter or AlignEnd
	last  float32
	since time.Duration
}

// virtualTyp identifies the component of VirtualList nodes.
var virtualTyp = reflect.ValueOf(virtualList).Pointer()

// VirtualList returns a vertical list of count items of which only the
// visible ones, plus overscan, are built (see the comment at the top of
// virtual.go; h.VirtualList). estimateSize(i) estimates the height of item
// i in logical pixels before it is measured; renderItem(i) returns its
// node, which runs inside the list's build and must not call hooks (return
// a component node, tela.C or tela.CP, for items with state). The node is
// the scroll container itself: props style it like a Div's (Class, Style,
// Ref, handlers, Key, the Aria props), with overflow-y-auto before the
// classes, and Overscan sets the number of extra items.
func VirtualList(count int, estimateSize func(i int) float32, renderItem func(i int) Node, props ...Prop) Node {
	switch {
	case count < 0:
		panic(fmt.Sprintf("h: VirtualList(%d, ...): count is the number of items, 0 or more", count))
	case estimateSize == nil:
		panic("h: VirtualList(n, nil, ...): pass estimateSize, a func(i int) float32 that estimates the height of item i in px, such as func(int) float32 { return 40 }")
	case renderItem == nil:
		panic("h: VirtualList(n, estimate, nil): pass renderItem, a func(i int) tela.Node that returns item i, such as func(i int) tela.Node { return tela.CP(Row, i) }")
	}
	d := &desc{kind: nodeElement, elem: style.KindDiv, classes: []string{"overflow-y-auto"},
		virt: &virtNode{role: virtList, overscan: defaultOverscan}}
	for _, p := range props {
		if p.apply != nil {
			p.apply(d)
		}
	}
	switch {
	case d.src != nil || d.img != nil:
		panic("h: Src on VirtualList: only Img takes an image source")
	case d.href != "":
		panic("h: Href on VirtualList: only A takes a destination")
	case d.aria != nil && d.aria.hasAlt:
		panic("h: Alt on VirtualList: only Img takes an alternative text; name the list with AriaLabel")
	}
	p := &vprops{
		count: count, estimate: estimateSize, estFn: reflect.ValueOf(estimateSize).Pointer(),
		render: renderItem, outer: d, overscan: d.virt.overscan,
	}
	n := &desc{kind: nodeComponent, comp: &compDesc{
		typ: virtualTyp, fn: virtualList, build: func() Node { return virtualList(p) }, props: p,
	}}
	if d.keyed {
		// The key identifies the list among its siblings: it belongs to
		// the component node.
		n.key, n.keyed = d.key, true
		d.key, d.keyed = nil, false
	}
	return Node{n}
}

// Overscan sets the number of items a VirtualList builds beyond each side
// of the visible ones (5 by default; see the comment at the top of
// virtual.go). It panics on other elements and for negative n.
func Overscan(n int) Prop {
	if n < 0 {
		panic(fmt.Sprintf("h: Overscan(%d): pass the number of extra items, 0 or more", n))
	}
	return Prop{name: "Overscan", apply: func(d *desc) {
		if d.virt == nil || d.virt.role != virtList {
			panic("h: Overscan is a prop of VirtualList; other elements build all their children")
		}
		d.virt.overscan = n
	}}
}

// virtualList is the component of VirtualList nodes.
func virtualList(p *vprops) Node {
	el, i := nextSlot(hookVirtual)
	var vl *vlist
	if i < len(el.hooks) {
		v, ok := el.hooks[i].(*vlist)
		if !ok {
			el.tree.slotMismatch(el, i, "VirtualList")
		}
		vl = v
	} else {
		vl = &vlist{tree: el.tree, el: el, hi: -1, nodes: map[int]Node{}}
		vl.listMark = &virtNode{vl: vl, role: virtList}
		vl.contentMark = &virtNode{vl: vl, role: virtContent}
		el.hooks = append(el.hooks, vl)
		el.tree.virt.lists = append(el.tree.virt.lists, vl)
	}
	return vl.render(p)
}

// unmount removes the list from its tree's lists.
func (vl *vlist) unmount(t *Tree) {
	l := t.virt.lists
	for i, x := range l {
		if x == vl {
			copy(l[i:], l[i+1:])
			l[len(l)-1] = nil
			t.virt.lists = l[:len(l)-1]
			break
		}
	}
	vl.nodes, vl.built, vl.req, vl.align = nil, nil, nil, nil
}

// virtualListOf returns the list whose scroll container is el, or nil.
func virtualListOf(el *Element) *vlist {
	if el == nil || el.kind != elemPrimitive {
		return nil
	}
	if v := el.node.d.virt; v != nil && v.role == virtList && v.vl != nil && v.vl.el.mounted {
		return v.vl
	}
	return nil
}

// ---------------------------------------------------------------------
// Building.

// sync takes the props of a new node from the parent's build: the sizes
// follow count and estimateSize, and every item is rendered again
// (renderItem may capture new data).
func (vl *vlist) sync(p *vprops) {
	reset := vl.props == nil || p.estFn != vl.estFn
	if reset || p.count != vl.sizes.len() {
		vl.sizes.resize(p.count, p.estimate, reset)
	}
	vl.props, vl.estFn, vl.overscan = p, p.estFn, p.overscan
	clear(vl.nodes)
	if vl.req != nil && vl.req.idx >= p.count {
		vl.req.idx = p.count - 1
		if p.count == 0 {
			vl.req = nil
		}
	}
	if vl.align != nil && vl.align.idx >= p.count {
		vl.align = nil
	}
}

// render builds the list's node for the current window.
func (vl *vlist) render(p *vprops) Node {
	if p != vl.props {
		vl.sync(p)
	} else if o := vl.el.vobs; o != nil && len(o.subs) > 0 {
		// The last build read Values (in renderItem): render every item
		// again, since one of them may have changed.
		clear(vl.nodes)
	}
	lo, hi, guess := vl.wantRange()
	vl.lo, vl.hi, vl.guessed = lo, hi, guess
	// The focused item and its neighbors stay mounted outside the range.
	keep := vl.keepIndex()
	if keep >= lo && keep <= hi {
		keep = -1
	}
	n := vl.sizes.len()
	built := vl.built[:0]
	// The last sticky item before the range stays mounted: it may be
	// stuck over the visible items (sticky.go).
	if sk := vl.stickyBefore(lo); sk >= 0 && (keep < 0 || sk < keep-1) {
		built = append(built, sk)
	}
	for k := max(keep-1, 0); keep >= 0 && k <= keep+1 && k < lo; k++ {
		built = append(built, k)
	}
	if sk := vl.stickyBefore(lo); sk >= 0 && keep >= 0 && sk >= keep-1 && sk > keep+1 {
		built = append(built, sk)
	}
	for i := lo; i <= hi; i++ {
		built = append(built, i)
	}
	for k := max(keep-1, hi+1); keep >= 0 && k <= keep+1 && k < n; k++ {
		built = append(built, k)
	}
	vl.built = built
	for i := range vl.nodes {
		if _, ok := slices.BinarySearch(built, i); !ok {
			delete(vl.nodes, i)
		}
	}
	kids := make([]Node, len(built))
	for k, i := range built {
		node, ok := vl.nodes[i]
		if !ok {
			node = vl.item(i, p)
			vl.nodes[i] = node
		}
		kids[k] = node
	}
	content := &desc{kind: nodeElement, elem: style.KindDiv, children: kids, virt: vl.contentMark}
	outer := *p.outer
	outer.virt = vl.listMark
	outer.children = []Node{{content}}
	return Node{&outer}
}

// item returns the wrapper node of item i.
func (vl *vlist) item(i int, p *vprops) Node {
	el := vl.el
	before := el.hookIdx
	child := p.render(i)
	if el.hookIdx != before {
		panic(fmt.Sprintf("tela: the renderItem function of a VirtualList called a hook while rendering item %d; it runs inside the list's build, so items with state must be components: return tela.C(...) or tela.CP(Row, i) from renderItem", i))
	}
	return Node{&desc{kind: nodeElement, elem: style.KindDiv, key: i, keyed: true,
		children: []Node{child}, virt: &virtNode{vl: vl, role: virtItem, idx: i}}}
}

// keepIndex returns the index of the built item that holds the focus, or
// -1.
func (vl *vlist) keepIndex() int {
	for e := vl.tree.foc.el; e != nil && e != vl.el; e = e.parent {
		if e.kind != elemPrimitive {
			continue
		}
		if v := e.node.d.virt; v != nil && v.role == virtItem && v.vl == vl {
			if e.mounted && v.idx < vl.sizes.len() {
				return v.idx
			}
			return -1
		}
	}
	return -1
}

// ---------------------------------------------------------------------
// Geometry.

// boxes returns the list's scroll container and content boxes, or nil
// before the first build.
func (vl *vlist) boxes() (outer, content *Box) {
	if len(vl.el.children) == 0 {
		return nil, nil
	}
	oe := vl.el.children[0]
	if oe.kind != elemPrimitive {
		return nil, nil
	}
	if len(oe.children) == 0 || oe.children[0].kind != elemPrimitive {
		return oe.box, nil
	}
	return oe.box, oe.children[0].box
}

// viewKind says what view knows of the list's visible part.
type viewKind uint8

const (
	viewUnknown viewKind = iota // not laid out yet
	viewHidden                  // not displayed: nothing is visible
	viewKnown
)

// view returns the visible part of the list in the coordinates of its
// content box: its scrollport, cut by the scrollports of the scroll
// containers around it and by the window. A list that is laid out but off
// screen gets the first screenful of its scrollport, so that it has items
// (for tests and for scrolling into view) without building many.
func (vl *vlist) view() (top, bot float32, kind viewKind) {
	t := vl.tree
	outer, content := vl.boxes()
	if outer == nil || content == nil {
		return 0, 0, viewUnknown
	}
	// display: none on the list or an ancestor hides it; an element
	// without a style is new.
	for e := outer.el; e != nil; e = e.styleParent() {
		switch cs := e.box.cs; {
		case cs == nil:
			return 0, 0, viewUnknown
		case cs.Display == style.DisplayNone:
			return 0, 0, viewHidden
		}
	}
	if outer.parent == nil || content.parent != outer {
		return 0, 0, viewUnknown
	}
	if !t.inTree(outer) {
		return 0, 0, viewHidden
	}
	l := outer.ld.Layout()
	pos := float32(0)
	clientH := max(l.Size.H-l.Border.Top-l.Border.Bottom-l.ScrollbarSize.H, 0)
	if s := outer.scr; s != nil {
		pos, clientH = s.pos.Y, s.client.H
	}
	base := content.ld.Layout().Location.Y - l.Border.Top
	portTop := t.liveOrigin(outer).Y + l.Border.Top
	vt, vb := max(portTop, 0), min(portTop+clientH, t.viewport.H)
	for cb := containingBlockOf(outer); cb != nil && vt < vb; cb = containingBlockOf(cb) {
		if s := cb.scr; s != nil {
			cl := cb.ld.Layout()
			ct := t.liveOrigin(cb).Y + cl.Border.Top // perf: O(depth) per ancestor
			vt, vb = max(vt, ct), min(vb, ct+s.client.H)
		}
	}
	contentTop := portTop + base - pos
	if !(vt < vb) {
		h := min(clientH, t.viewport.H)
		return pos - base, pos - base + h, viewKnown
	}
	return vt - contentTop, vb - contentTop, viewKnown
}

// liveOrigin returns the window position of bx's border box with the
// scroll offsets as they are now (paint snaps them; this does not): the
// layout positions minus the offsets of the scroll containers on its
// containing-block chain. Transforms are ignored.
func (t *Tree) liveOrigin(bx *Box) gfx.Point {
	var o gfx.Point
	for b := bx; b != nil; b = b.parent {
		o = o.Add(b.ld.Layout().Location)
	}
	for b := containingBlockOf(bx); b != nil; b = containingBlockOf(b) {
		if b.scr != nil {
			o = o.Sub(b.scr.flip(b.scr.pos))
		}
	}
	return o
}

// visibleRange returns the indices of the first and last visible items,
// or ok false when nothing is visible (no items, or the list is hidden);
// guess reports that the list is not laid out yet.
func (vl *vlist) visibleRange() (v0, v1 int, ok, guess bool) {
	n := vl.sizes.len()
	if n == 0 {
		return 0, -1, false, false
	}
	top, bot, kind := vl.view()
	switch kind {
	case viewHidden:
		return 0, -1, false, false
	case viewUnknown:
		guess = true
		// Before the first layout the first item only (TanStack's empty
		// initial rect): the pass after the layout builds the rest in the
		// same frame.
		top, bot = 0, 0
	}
	// The first item that ends below the top, the last that starts above
	// the bottom.
	v0 = min(vl.sizes.upTo(float64(top)), n-1)
	v1 = min(vl.sizes.before(float64(bot)), n-1)
	v1 = min(max(v1, v0), v0+maxWindow-1)
	return v0, v1, true, guess
}

// wantRange returns the range to build: the visible items and the
// overscan; guess reports that the list is not laid out yet.
func (vl *vlist) wantRange() (lo, hi int, guess bool) {
	v0, v1, ok, guess := vl.visibleRange()
	if !ok {
		return 0, -1, false
	}
	return max(v0-vl.overscan, 0), min(v1+vl.overscan, vl.sizes.len()-1), guess
}

// covered reports whether the built range holds the visible items and,
// with an overscan, one more on each side; a range guessed before the
// list was laid out is replaced by the exact one.
func (vl *vlist) covered() bool {
	v0, v1, ok, guess := vl.visibleRange()
	switch {
	case !ok:
		return true
	case vl.guessed && !guess:
		lo, hi, _ := vl.wantRange()
		return lo == vl.lo && hi == vl.hi
	}
	n, o := vl.sizes.len(), vl.overscan
	m := min(o, 1)
	if vl.lo > max(v0-m, 0) || vl.hi < min(v1+m, n-1) {
		return false
	}
	// A range more than the overscan larger than wanted on a side (built
	// from estimates that were too small, or for a taller list) is
	// trimmed. Scrolling never gets there first: the far edge reaches the
	// visible items before the near one is that far.
	return vl.lo >= max(v0-o, 0)-o && vl.hi <= min(v1+o, n-1)+o
}

// base returns the offset of the content box in the scroll coordinates of
// the list (its top padding).
func (vl *vlist) base(outer, content *Box) float32 {
	return content.ld.Layout().Location.Y - outer.ld.Layout().Border.Top
}

// virtualScrolled checks the window of every list after a scroll offset
// changed (setScroll): a list whose visible items left its built range is
// marked for building.
func (t *Tree) virtualScrolled() {
	for _, vl := range t.virt.lists {
		if vl.el.mounted && !vl.el.needsBuild && vl.props != nil && !vl.covered() {
			t.markBuild(vl.el)
		}
	}
}

// ---------------------------------------------------------------------
// Layout.

// virtualLayout applies the layout style of the boxes of a virtual list
// (mapLayoutStyle): the content box is as tall as the list, and the first
// wrapper of a run of built items has a top margin that skips the items
// before it.
func (v *virtNode) layoutStyle(ls *layout.Style) {
	switch v.role {
	case virtContent:
		*ls = layout0
		ls.Width = layout.Percent(1)
		ls.Height = layout.Px(float32(v.vl.sizes.total()))
		ls.FlexShrink = 0
	case virtItem:
		*ls = layout0
		ls.Margin.Top = layout.Px(v.vl.gapBefore(v.idx))
	}
}

// gapBefore returns the space before the wrapper of built item i: the
// sizes of the items between it and the previous built item.
func (vl *vlist) gapBefore(i int) float32 {
	k, ok := slices.BinarySearch(vl.built, i)
	if !ok || i >= vl.sizes.len() {
		return 0
	}
	from := 0
	if k > 0 {
		from = vl.built[k-1] + 1
	}
	if from == i {
		return 0
	}
	return float32(vl.sizes.offset(i) - vl.sizes.offset(from))
}

// ---------------------------------------------------------------------
// The scroll phase.

// virtualPass measures the built items of every list, keeps the
// positions stable, keeps aligned items aligned and checks the windows
// (see the comment at the top of virtual.go). It reports whether a list
// must be built or laid out again before painting.
func (t *Tree) virtualPass() bool {
	work := false
	for _, vl := range t.virt.lists {
		if vl.pass() {
			work = true
		}
	}
	return work
}

// pass is virtualPass for one list.
func (vl *vlist) pass() bool {
	t := vl.tree
	if !vl.el.mounted || vl.props == nil {
		return false
	}
	outer, content := vl.boxes()
	if outer == nil || content == nil || outer.cs == nil || content.parent != outer || !t.inTree(outer) {
		// Not laid out: a hidden list keeps its items (and their state),
		// except a guess made before it was styled.
		if vl.guessed && !vl.el.needsBuild {
			if _, _, kind := vl.view(); kind == viewHidden {
				t.markBuild(vl.el)
			}
		}
		return vl.el.needsBuild
	}
	s := outer.scr
	if s == nil {
		vl.req, vl.align = nil, nil
	}
	work := false
	if vl.measure(outer, content, s) {
		// The content's height and the run margins follow the new sizes;
		// alignment waits for the layout (the extents may grow).
		content.queueRemap()
		work = true
	} else if s != nil {
		if vl.req != nil {
			vl.applyRequest(s)
		}
		if vl.align != nil {
			vl.reconcile(s)
		}
	}
	if !vl.el.needsBuild && !vl.covered() {
		t.markBuild(vl.el)
	}
	return work || vl.el.needsBuild
}

// measure compares the heights of the built items with their sizes and
// updates the sizes; size changes of items that start above the top of
// the scrollport move the scroll offset by the change. It reports whether
// a size changed.
func (vl *vlist) measure(outer, content *Box, s *scroller) bool {
	n := vl.sizes.len()
	top := math.Inf(-1)
	if s != nil {
		top = float64(s.pos.Y - vl.base(outer, content))
	}
	changed := false
	var adjust float64
	for _, k := range content.kids {
		v := k.virtMark()
		if v == nil || v.role != virtItem || v.vl != vl || v.idx >= n {
			continue
		}
		vl.noteSticky(v.idx, len(k.kids) > 0 && k.kids[0].cs != nil && k.kids[0].cs.Position == style.PositionSticky)
		h := clampSize(k.ld.Layout().Size.H)
		old := vl.sizes.at(v.idx)
		if h == old {
			continue
		}
		if vl.sizes.offset(v.idx) < top+adjust {
			adjust += float64(h) - float64(old)
		}
		vl.sizes.set(v.idx, h)
		changed = true
	}
	if adjust != 0 {
		vl.tree.shiftScroll(s, float32(adjust))
		if vl.align != nil {
			vl.align.last = s.target().Y
		}
	}
	return changed
}

// shiftScroll moves the vertical offset of s by dy without clamping (the
// layout that follows clamps it), with its animation and fling, so that
// content that moved by dy stays in place on screen. It runs in the scroll
// phase.
func (t *Tree) shiftScroll(s *scroller, dy float32) {
	s.pos.Y = max(s.pos.Y+dy, 0)
	if a := &s.anim[1]; a.active {
		a.from += dy
		a.to = max(a.to+dy, 0)
	}
	if f := &s.fling[1]; f.active {
		f.from += dy
	}
	t.markPaint(s.box)
	t.pendEvent(s)
	if !s.syncPending {
		s.syncPending = true
		t.scr.pending = append(t.scr.pending, s)
	}
}

// virtualRelayout runs the phases a pass asked for: the builds of the
// lists whose window moved, then style, structure and layout, and the
// layout effects of the items mounted, as Frame does.
func (t *Tree) virtualRelayout() {
	// A new pass: the per-frame marks of the structure phase (boxFrame,
	// inlineFrame, sibFrame) must not take the boxes of the previous pass
	// for current.
	t.frame++
	t.buildPhase()
	t.stylePhase()
	if t.focusPending() {
		t.focusWork()
		t.stylePhase()
	}
	t.boxPhase()
	t.layoutPhase()
	t.commitLayout()
	if len(t.styleQ) > 0 || len(t.boxQ) > 0 {
		// A layout effect moved the focus.
		t.stylePhase()
		t.fixFocus()
		t.stylePhase()
		t.boxPhase()
		t.layoutPhase()
	}
}

// virtualAfterFrame asks for the next frame when this one ended before the
// lists settled.
func (t *Tree) virtualAfterFrame() {
	if t.virt.again {
		t.virt.again = false
		t.dirty()
	}
}

// ---------------------------------------------------------------------
// ScrollToIndex.

// ScrollToIndex scrolls a VirtualList so that item i is in view, placed
// as align says (TanStack Virtual's scrollToIndex; see the comment at the
// top of virtual.go): at once, or animated when the list has
// scroll-smooth, like ScrollTo. i is clamped to the items. It does
// nothing when the element is not a mounted VirtualList, has no items or
// does not scroll (overflow-visible).
func (r ElementRef) ScrollToIndex(i int, align ScrollAlign) {
	vl := virtualListOf(r.element())
	if vl == nil {
		return
	}
	if align > AlignEnd {
		align = AlignAuto
	}
	t := vl.tree
	current := t.layoutCurrent()
	if current && vl.sizes.len() == 0 {
		return
	}
	// The index is clamped when the request applies: a count set in the
	// same handler is not built yet.
	vl.req = &alignReq{idx: max(i, 0), align: align}
	vl.align = nil
	if outer, content := vl.boxes(); outer != nil && content != nil && outer.scr != nil &&
		current && content.parent == outer && t.inTree(outer) {
		vl.applyRequest(outer.scr)
		return
	}
	// The layout is stale: after the next one (virtualPass).
	t.requestRepaint()
}

// applyRequest scrolls to the pending ScrollToIndex and keeps the item
// aligned.
func (vl *vlist) applyRequest(s *scroller) {
	r := vl.req
	vl.req = nil
	if vl.sizes.len() == 0 {
		return
	}
	r.idx = min(r.idx, vl.sizes.len()-1)
	align := r.align
	if align == AlignAuto {
		start, end := vl.itemSpan(r.idx, s)
		switch {
		case end >= s.pos.Y+s.client.H:
			align = AlignEnd
		case start <= s.pos.Y:
			align = AlignStart
		default:
			return // fully visible
		}
	}
	vl.tree.scrollScripted(s, gfx.Point{X: s.pos.X, Y: vl.offsetFor(r.idx, align, s)})
	vl.align = &alignState{idx: r.idx, align: align, last: s.target().Y, since: vl.tree.scr.now}
}

// itemSpan returns the start and end of item i in the scroll coordinates
// of the list.
func (vl *vlist) itemSpan(i int, s *scroller) (start, end float32) {
	outer, content := vl.boxes()
	start = vl.base(outer, content) + float32(vl.sizes.offset(i))
	return start, start + vl.sizes.at(i)
}

// offsetFor returns the scroll offset that aligns item i as align says,
// clamped to the extents.
func (vl *vlist) offsetFor(i int, align ScrollAlign, s *scroller) float32 {
	start, end := vl.itemSpan(i, s)
	h := s.client.H
	var to float32
	switch {
	case align == AlignCenter:
		to = start + (end-start-h)/2
	case align == AlignEnd && i == vl.sizes.len()-1:
		to = s.max.Y // the list's end padding too (TanStack)
	case align == AlignEnd:
		to = end - h
	case i == 0:
		to = 0 // the list's start padding too
	default:
		to = start
	}
	return min(max(to, 0), s.max.Y)
}

// reconcile keeps the aligned item aligned (TanStack's
// reconcileScroll): when its offset moved, the list goes there (a running
// animation is retargeted); a pass that finds it in place and not
// animating ends it, as do a scroll by someone else and alignTimeout.
func (vl *vlist) reconcile(s *scroller) {
	a := vl.align
	t := vl.tree
	cur := s.target().Y
	// Layouts clamp the offset and the animation's target to the extents.
	expect := min(max(a.last, 0), s.max.Y)
	if a.idx >= vl.sizes.len() || absf(cur-expect) > 0.5 || t.scr.now-a.since > alignTimeout {
		vl.align = nil // the user scrolled, or it never settled
		return
	}
	to := vl.offsetFor(a.idx, a.align, s)
	if absf(to-cur) <= 0.5 {
		if !s.animating() {
			vl.align = nil
		}
		return
	}
	if an := &s.anim[1]; an.active {
		s.startAnim(1, to, max(an.start+an.dur-t.scr.now, 16*time.Millisecond), t.scr.now)
		t.requestRepaint()
	} else {
		t.setScroll(s, gfx.Point{X: s.pos.X, Y: to})
	}
	a.last = s.target().Y
}

// virtualEdge keeps the first or the last item of a list aligned after
// Home or End (dir < 0 or > 0) on its scroller s.
func (t *Tree) virtualEdge(s *scroller, dir float32) {
	vl := virtualListOf(s.box.el)
	if vl == nil || vl.sizes.len() == 0 {
		return
	}
	a := &alignState{idx: 0, align: AlignStart, last: s.target().Y, since: t.scr.now}
	if dir > 0 {
		a.idx, a.align = vl.sizes.len()-1, AlignEnd
	}
	vl.req, vl.align = nil, a
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
