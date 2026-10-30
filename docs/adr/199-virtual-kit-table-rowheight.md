# ADR-199: Virtual `kit.Table` (`RowHeight`), `h.AriaRowCount` and `h.AriaRowIndex`

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

`kit.Table` gains `RowHeight float32` (px, 37 for the default look; 0 builds every row). Set, the body is an `h.VirtualList` with `h.ItemHeight` (and `h.ItemKey` from `RowKey`, so a row's state follows it while it is mounted): only the rows in view plus the overscan exist. Each row is a grid of its own with the header's tracks, inside a flex column (header, list, footer, caption `order-last`); the root scrolls sideways, the list vertically. The table needs a height; `auto` and `Nfr` columns become `minmax(0,...)` so every row has the same tracks; `Sticky` columns are not supported (a dev-mode panic names the fix). Sorting, `Selected`, `RowProps`, `Empty`, footer and caption work as before. New public API: `h.AriaRowCount(n)` and `h.AriaRowIndex(i)` (`semantics.Node.RowCount`/`RowIndex`, not printed by aria snapshots, as Playwright's); a virtual table sets them (header is row 1).

## Why

Measured (`kit/table_perf_test.go`, release): every non-scroll frame costs time proportional to the built rows in build, box, layout and paint: 240 rows 9.8 ms, 2000 rows 77 ms when the parent rebuilds; scrolling is paint-only and cheap. Virtual: 1.4 ms at 240, 2000 and 100,000 rows.

## What it changes

`kit/table.go`, `h/aria.go`, `internal/core/aria.go` and `semantics.go`, `internal/semantics/node.go`, `docs/llms.txt`. A plain table is unchanged. Not done: a ref for `ScrollToIndex`, a11y backend mapping of the two fields.
