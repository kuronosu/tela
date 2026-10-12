// Package layout is Tela's layout engine (ARCHITECTURE section 9): block
// layout without margin collapsing, flexbox, relative and absolute
// positioning, intrinsic sizes, leaf measurement by callback, caching and
// relayout boundaries. The algorithms are ported from Taffy v0.14.0 and
// checked against Taffy's fixtures with expected boxes measured by headless
// Chromium (ADR-039). Grid, fixed and sticky positioning and scrolling
// come in later phases.
//
// The package depends only on the standard library and internal/gfx (for
// result geometry). It knows nothing of styles, text or the Box tree:
// core implements Node on its Boxes and maps ComputedStyle onto Style.
//
// # Nodes and styles
//
// A Node gives its Style, its children and its Data, the per-node state
// layout keeps (results and caches; core embeds one in each Box). A leaf
// (a node without children) with content implements Measurer: text boxes
// measure with text.Layout, images return their intrinsic size.
//
// Style is a Taffy-like subset of CSS: display (block, flex, none),
// position (static, relative, absolute), direction, overflow, sizes with
// min and max bounds, aspect-ratio, insets, margins (auto allowed),
// padding, borders, the flex container and item properties, gaps and
// order. Values are Dims: lengths in logical px, percentages, auto and the
// sizing keywords (min-content, max-content, fit-content, stretch). Box
// sizing is always border-box. Style holds computed values: core resolves
// inheritance (direction) and CSS's computed-value rules (overflow: visible
// next to a scrolling axis becomes auto).
//
// # Protocol
//
// Section 9 describes Flutter's protocol: constraints go down, sizes go
// up, the parent sets positions, intrinsic sizes are separate cached
// queries, and relayout boundaries stop invalidation. CSS needs richer
// constraints than Flutter's min/max box, so layout uses Taffy's: a request
// (input) carries known dimensions (sizes the node must take), available
// space per axis (a definite amount, or a min-content or max-content
// constraint), the parent size (the percentage basis) and whether the
// node's own size styles apply; it either computes only a size (an
// intrinsic or hypothetical size query, which section 9 calls an intrinsic
// size) or lays the subtree out fully (which positions the children). The
// mapping to section 9:
//
//   - Flutter's Box.Layout(constraints) is a full layout request; tight
//     constraints are known dimensions in both axes; loose ones are
//     available space.
//   - Intrinsic sizes are size requests under min-content or max-content
//     constraints, cached like any other request.
//   - Each node caches its last full layout and nine size results, keyed by
//     the whole request (Taffy's cache, with an exact key).
//   - A parent sets each child's position (Data.Layout().Location, relative
//     to the parent's border box).
//
// Engine.Compute(root, width, height) lays out a tree into the given space
// (a window passes its size as definite space). Results are read from each
// node's Data: Layout returns the unrounded layout (location, size,
// border, padding, margin, scrollbar size, scrollable overflow, first
// baseline); RoundedLayout returns it snapped to pixels after Round.
//
// # Incremental layout
//
// Engine.MarkDirty(n) records that n's style, content or children changed:
// it clears the caches of n and its ancestors up to the nearest relayout
// boundary. The next Compute runs the algorithms of the dirty nodes only;
// clean subtrees are reused from their caches (a second Compute with
// nothing dirty does no work).
//
// A relayout boundary is a node whose size cannot depend on its content: a
// fixed width and height in px and overflow other than visible or clip in
// both axes (section 9's "explicit size and overflow other than visible";
// the overflow matters because a flex item's automatic minimum size comes
// from its content unless it is a scroll container). Section 9's other
// boundary, tight constraints, is not one here: a flex item laid out with
// tight constraints was sized from its content's intrinsic sizes, which a
// change inside it invalidates (Flutter escalates in the same case when
// intrinsic sizes were queried). A change inside a boundary lays out only
// the boundary's subtree, from the boundary's previous request; if the
// boundary's first baseline changed (baseline alignment uses it) or it
// hoists absolutely positioned descendants to an ancestor, Compute marks
// its parent dirty and continues upwards.
//
// # Containing blocks
//
// Taffy places an absolutely positioned box against its parent. Tela, like
// CSS, places it against its nearest positioned ancestor (relative or
// absolute) or, without one, against the root: a static parent records the
// box's static position (where it would be in flow) and reports it
// upwards; the containing block lays such boxes out after its own children.
// Absolutely positioned boxes are laid out by CSS 2's rules (10.3.7,
// 10.6.4): the available space and auto margins come from the insets that
// are set, percentages from the containing block's padding box; an auto
// width is fit-content, and an auto height is not definite for the box's
// content.
//
// # Rounding
//
// Layout computes in float32 without rounding. Round snaps a tree to whole
// logical pixels like Taffy and Yoga: edges are rounded at their
// cumulative position from the root and sizes are differences of rounded
// edges, so adjacent boxes stay adjacent. Chromium computes in LayoutUnits
// (1/64 px fixed point, so most of its sizes are multiples of 1/64) and
// snaps boxes to device pixels the same way when painting. core may paint
// either.
//
// # Determinism
//
// Layout output feeds the rasterizer, so it is bit-identical on every
// architecture (ADR-027): every product that feeds a sum is rounded with
// an explicit float32 conversion (the compiler may otherwise fuse it into
// a multiply-add on arm64 or amd64 v3), and no map iteration order affects
// results. TestLayoutDeterministic checks a digest of many layouts.
//
// # Conformance and deviations from Taffy
//
// TestTaffyFixtures lays out Taffy v0.14.0's block, flex, blockflex and
// leaf fixtures and compares every box with Chromium 152 within 0.5 px;
// testdata/taffy/excluded.txt lists the fixtures left out and why (margin
// collapsing, content-box, flex-wrap: balance, grid, float, vertical
// writing modes, legacy text-align). TestTelaCases does the same for
// Tela's own cases (testdata/cases). Where Chromium and Taffy disagree,
// layout follows Chromium:
//
//   - A flex container sized under definite available space is
//     fit-content sized (Taffy sums the flex bases).
//   - The flex base size alone clamps the intrinsic contributions of items
//     that do not grow or shrink (Taffy also takes the preferred size).
//   - An item with an aspect ratio and a definite cross size has the
//     transferred size as its content size suggestion.
//   - Percentage margins count as zero in a block's intrinsic width.
//   - Absolutely positioned boxes: see Containing blocks; Taffy gives them
//     the whole containing block as available space, has a flex variant
//     whose auto margins ignore the insets, and treats an auto height as
//     definite.
//   - Rounded locations are differences of rounded cumulative positions
//     (Taffy rounds each location relative to its parent).
//   - order is supported; flex-wrap: balance is not.
//
// Two fixes to Taffy's machinery: Measure gets content-box known
// dimensions (Taffy passes the border box, so text with padding wrapped at
// the wrong width in size queries); and a size query that lays children
// out fully (for flex baselines) drops the cached full layout of the nodes
// being measured, so a later cache hit never keeps the children positions
// of the query.
package layout
