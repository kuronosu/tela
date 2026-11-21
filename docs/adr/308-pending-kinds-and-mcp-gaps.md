# ADR-308: `Pending` kinds and the gaps of `tela mcp`

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-174, ADR-291, ADR-300, ADR-307.

## Decision

- **`telatest.App.Pending() []telatest.Pending`** (was `[]string`): each item has a `Kind` (`PendingKind`: `PendingFrame`, `PendingUpdate`, `PendingAnimation`, `PendingEndless`, `PendingScrollbar`, `PendingCaret`, `PendingTimer`, `PendingAsync`, `PendingGo`...), the `Window` it belongs to when not the App's, `What` (the old text) and `In`, the distance of its deadline; `String()` gives the old line. `core.Tree.Pending` returns kinds too. A scrollbar fade is its own kind, apart from a scroll animation.
- **`UseGo`** goroutines that have not returned are `PendingGo`, named with their component and call site. `ExpectIdle` and the idle budgets do not count them (a goroutine that waits costs nothing) and nothing waits for them (one may live as long as its component).
- **`App.Scrollables() []uint64`**: the identities of the scroll containers a user can scroll, largest visible area first.
- **`tela mcp`** reads the kinds: a miss fails at once only when all that is pending only redraws (endless, scrollbar, caret, debug, and the frame request beside them); a load on a goroutine waits. `wait` for a text fails at once the same way. `scroll` without an element or point scrolls the largest container that moves, and every scroll says when nothing moved. A module on an older Tela keeps the text matching and the window's center.

## Why

Matching the text of `Pending` broke whenever a message was reworded, and a load on `UseGo` looked settled, so a click on what it shows failed before the load ended. The window's center is often a sidebar, and a scroll that moved nothing answered as if it had.
