# ADR-053: `UseValue[T]` returns a comparable `*Value[T]`

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

`UseValue[T]` returns a comparable `*Value[T]` (`Get`, `Set`, `Update`; all safe from any goroutine; `Get` returns the value applied last). Updates share the FIFO queue with state and `Post`; `==` comparable values change nothing. The Tree tracks reads while its UI thread evaluates an observer: a component build (change → build), a box's layout-phase deferred props when its layout style is mapped (change → that box's layout style mapped again and laid out; no build, no style), or its paint-phase props when painted (change → repaint; `Frame` skips build, style, structure and layout). Each evaluation replaces the observer's subscriptions; unmount, detach, losing the props or the owner's unmount remove them. Tracking is per Tree; a `Get` from another goroutine that races with a tracked evaluation may add one spurious subscription. Boxes to repaint are queued per box for layers

## Rationale

Compose's phase-deferred state reads (ADR-015). Go has no goroutine-local storage, so over-subscription (never a missed read) is the safe trade. Per-box marks let layers repaint only what changed
