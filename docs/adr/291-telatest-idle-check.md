# ADR-291: telatest sees what keeps an app awake

- **Date:** 2026-11-17
- **Status:** Accepted. Implements part of ADR-290 (counted budgets).

## Decision

- `telatest.App.Pending() []string` names what keeps the app from being idle once settled (the work due handled; with `NoSettle` only that): events to deliver, a frame request or a frame owed, a paced commit of a hidden window, and in each window's tree queued updates, changed elements, running animations (endless ones too), scroll animations, debug overlays asking for frames, and every deadline of the tree, each with its distance: a `UseInterval` or `UseTimeout` (its component and, in dev mode, its call site), a caret blink, a long press, a scrollbar fade, a drag's autoscroll. It never moves the clock to a deadline.
- `App.ExpectIdle()` fails the test with that list when it is not empty.
- Both read `core.Tree.Pending(now)`, which `internal/testutil/idle`'s host check also uses. The host's own timer (the trims of ADR-241), presents and damage stay visible only on `core.Host`, so the idle budgets keep their second mount for them.

## Why

`Stats` counts work done, not work waiting: a no-op `UseInterval`, a toast's timer or a mounted spinner leaves no trace until the clock reaches it, so an app's test could not check "0% CPU when idle" (§3.5), and the examples' idle budgets had to mount the screen a second time on `core.Host` to see it.

## What it changes

`telatest` (two methods), `internal/core` (`Tree.Pending`), `internal/testutil/idle`, §17.
