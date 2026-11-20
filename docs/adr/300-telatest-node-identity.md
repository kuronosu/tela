# ADR-300: `telatest` node identity and `ScrollAt`

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-174.

## Decision

`telatest.App` gains `AriaSnapshotNodes() (yaml string, nodes []uint64)` (the aria snapshot with the semantics node ID of each line, 0 for text and `/url` lines), `NodeLocator(id) Locator` (matches that element while it is mounted and visible; prints as `getByNodeId(N)`) and `ScrollAt(x, y, dx, dy)` (a wheel at a window point). `tela mcp` refs are node IDs: they follow their element across rebuilds and renames, a ref whose element unmounted is an error, and after a remount refs are matched again by role, name and position. The instance reaches the methods through interface assertions, so a module on an older Tela keeps the old refs. A locator that matches nothing fails at once when nothing is pending; `notifications/cancelled` stops the call with no response.

## Why

Role, name and position refs pointed at the wrong element when a sibling was removed or a name changed. Node IDs already live as long as their element; exposing them as methods keeps tests on `ByRole` and lets tools built against older modules ask for them.
