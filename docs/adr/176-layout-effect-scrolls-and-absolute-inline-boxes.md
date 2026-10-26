# ADR-176: Layout-effect scrolls and absolute inline boxes

- **Date:** 2026-10-26
- **Status:** Accepted

## Decision

Two engine fixes found by `examples/chat` (amends ADR-066 and §9's defaults). **Scrolls from layout effects:** the scroll phase reads the extents after the layout effects, so `ElementRef.ScrollTo`, `ScrollBy` and `ScrollIntoView` called between a layout of the frame and that phase are queued, in call order, and run once the extents are read: the frame is painted scrolled, on mount and when content grew (they clamped to the previous frame's extents). The scroll chain takes boxes laid out but not painted yet, so an element mounted in the frame (or focused there) is scrolled into view. `ScrollToIndex` is unchanged. **Out-of-flow inline boxes:** `style.Compute` blockifies an absolute or fixed box (CSS 2.1 §9.7): inline and inline-block become block, inline-flex flex, inline-grid grid, so an absolute `Span` is laid out like an absolute `Div`. Out-of-flow boxes split no inline content: the block keeps them as children around one anonymous paragraph (before it when they precede its content), hoisting those inside inline elements; a paragraph that holds all the in-flow content takes the block's text-overflow and line-clamp.

## Rationale

`UseLayoutEffect`'s documented idiom must hold; blockification and unbroken lines are CSS and Chromium. Known gaps: with auto insets, a box in the middle of a paragraph has its static position below it (Chromium: its place on the line); inline elements are not containing blocks.
