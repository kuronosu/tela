package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// VirtualList is a vertical scrolling list of count items that builds,
// lays out and paints only the items in view, plus a few more on each
// side (see [Overscan]), so a list of 100,000 rows costs about as much as
// one screenful of them (TanStack Virtual's model). The node is the
// scrolling element itself: props style it like a Div (Class, Style,
// Ref, handlers, Key, Role and the Aria props), and it is overflow-y-auto
// unless a class sets another overflow. Give it a height (h-96, flex-1 in
// a column, h-screen): the items it builds are bounded by what the window
// shows, but only a list that scrolls by itself keeps its position stable
// and supports ScrollToIndex.
//
// estimateSize(i) estimates the height of item i in logical pixels, before
// it is built; renderItem(i) returns it. Each item is measured once it is
// laid out, and the measurement replaces the estimate (per index; a new
// estimateSize function resets them), so items may have any height and the
// scrollbar, OnScroll's ScrollHeight and the aria tree reflect the whole
// list. When a measurement changes the size of an item above the top of
// the list's viewport, the scroll offset moves by the difference in the
// same frame: the content on screen does not jump.
//
// Items are keyed by their index unless ItemKey says otherwise: an item
// component's state follows its index, so give the list [ItemKey] when
// items are inserted, removed or reordered, and the state must follow the
// item. For rows of one height, [ItemHeight] sets it, skips measuring and
// makes estimateSize unnecessary (it may be nil). The list has the role
// "list" and its items "listitem", for the semantics tree and the aria
// snapshot; Role on the list replaces its role (the items are then not
// listitems), and an item's own role goes on the element renderItem
// returns. renderItem runs inside the list's build, so it must not call
// hooks; return a component node (tela.C, tela.CP) for items with state,
// and prefer tela.CP so that a list rebuilt by its parent skips the items
// whose props did not change. Scrolling never rebuilds the items that stay
// in view: a scroll inside the built items only repaints, and a scroll
// past them builds the entering items and unmounts the leaving ones. The
// focused item stays mounted while it is scrolled away, so Tab goes on
// from it.
//
// Scroll to an item with the ref's ScrollToIndex (tela.AlignAuto,
// AlignStart, AlignCenter, AlignEnd), which keeps the item aligned while
// the items around it are measured, and which jumps or animates like
// ScrollTo (scroll-smooth):
//
//	list := tela.UseElementRef()
//	return Div(Class("flex h-screen flex-col"),
//	    Button(OnClick(func() { list.ScrollToIndex(5000, tela.AlignCenter) }),
//	        Text("Go to 5000")),
//	    VirtualList(100_000,
//	        func(int) float32 { return 40 },
//	        func(i int) tela.Node { return tela.CP(Row, i) },
//	        Ref(list), Class("flex-1 rounded-lg border")),
//	)
//
// Lists are vertical; horizontal lists and grids are not virtualized yet.
func VirtualList(count int, estimateSize func(i int) float32, renderItem func(i int) tela.Node, props ...tela.Prop) tela.Node {
	return core.VirtualList(count, estimateSize, renderItem, props...)
}

// Overscan sets how many items a [VirtualList] builds beyond each side of
// the visible ones: 5 by default. Tela builds a new window before painting
// the frame, so the overscan is not needed to hide blank areas while
// scrolling fast; it makes most scroll frames paint-only (the window moves
// only when the visible items reach its edge) and keeps the next items
// ready for Tab and the arrow keys. More items cost more layout and paint
// per frame; 0 rebuilds the list on every scroll that shows a new item.
//
//	VirtualList(len(rows), estimate, render, Overscan(10))
func Overscan(n int) tela.Prop { return core.Overscan(n) }

// ItemKey keys the items of a [VirtualList] by key(i), the identity of
// item i, instead of by its index, so that the state of an item component
// follows the item when others are inserted or removed before it
// (Key's rule for lists, and TanStack Virtual's getItemKey). K is any
// comparable type, such as an ID; a nil key or one that cannot be
// compared panics when the list builds. Give the key function with the
// data the list shows: it runs for the items that are built, with the
// indices of the list's current items.
//
// The heights measured so far stay with the indices, not with the items:
// after an insertion the items that moved are measured again when they are
// laid out.
//
//	VirtualList(len(rows), estimate,
//	    func(i int) tela.Node { return tela.CP(Row, rows[i]) },
//	    ItemKey(func(i int) int { return rows[i].ID }))
func ItemKey[K comparable](key func(i int) K) tela.Prop { return core.ItemKey(key) }

// ItemHeight gives every item of a [VirtualList] the same height in
// logical pixels, 1 or more. The items are not measured, so the list
// does no per-item measuring work, the scroll positions are exact from the
// first frame and the scrollbar does not change while scrolling; the
// estimateSize argument is not needed (pass nil). An item taller than the
// height spills over the next one (it is not clipped).
//
//	VirtualList(len(rows), nil, func(i int) tela.Node { return tela.CP(Row, rows[i]) },
//	    ItemHeight(40), Class("h-96"))
func ItemHeight(px float32) tela.Prop { return core.ItemHeight(px) }
