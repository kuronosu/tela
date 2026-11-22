# ADR-308: `Pending` kinds and the gaps of `tela mcp`

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-174, ADR-291, ADR-299, ADR-300, ADR-303, ADR-307.

## Decision

- **`telatest.App.Pending() []telatest.Pending`** (was `[]string`): each item has a `Kind` (`PendingKind`: `PendingFrame`, `PendingUpdate`, `PendingAnimation`, `PendingEndless`, `PendingScrollbar`, `PendingCaret`, `PendingTimer`, `PendingAsync`, `PendingGo`...), the `Window` it belongs to when not the App's, `What` (the old text) and `In`, the distance of its deadline; `String()` gives the old line. `core.Tree.Pending` returns kinds too. A scrollbar fade is its own kind, apart from a scroll animation.
- **`UseGo`** goroutines that have not returned are `PendingGo`, named with their component and call site. `ExpectIdle` and the idle budgets do not count them (a goroutine that waits costs nothing) and nothing waits for them (one may live as long as its component).
- **`App.Scrollables() []uint64`**: the identities of the scroll containers a user can scroll, largest visible area first. **`App.NodeOf(l Locator) uint64`**: the identity of the one element `l` finds, waiting as an action does, so `tela mcp`'s `state` takes any element, not only a ref.
- **`tela mcp`** reads the kinds: a miss fails at once only when all that is pending only redraws (endless, scrollbar, caret, debug, and the frame request beside them); a load on a goroutine waits. `wait` for a text fails at once the same way. `scroll` without an element or point scrolls the largest container that moves, and every scroll says when nothing moved. A module on an older Tela keeps the text matching and the window's center. `open` no longer takes `dark`: `prefer` switches it live.
- **Film and session tests** (amends ADR-303): `tela snapshot --film` takes `--click`, `--hover` (Playwright's `role=`, `text=`, `testid=` selectors) or `--press`. Under `--attach`, `film` takes the window's frames in real time and `test` writes the recording of the app's first window since its last build as a test of `tela dev`'s package. A recorded ref is `ByTestID` when its `h.TestID` is unique (`App.TestIDOf`); a locale is `i18n.SetLocale`, a direction an `h.Dir`.
- **Tools socket** (amends ADR-299, compatible): `reveal` adds a `point` where the target is painted (`core.Tree.ActionPoint`), a text line's element is its text's, and `info` adds `cwd` and `preview`.

## Why

Matching the text of `Pending` broke whenever a message was reworded, and a load on `UseGo` looked settled, so a click on what it shows failed before the load ended. The window's center is often a sidebar, and a scroll that moved nothing answered as if it had.
