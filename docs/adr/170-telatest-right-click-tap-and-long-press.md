# ADR-170: `telatest` right click, tap and long press

- **Date:** 2026-10-26
- **Status:** Accepted

## Decision

(Phase 11, API gap; the API is the owner's decision; amends ADR-086.) **Added:** `telatest.Right() ClickOption` (Playwright's `click({button: 'right'})`) makes `Click` and `ClickAt` press and release the secondary mouse button with real pointer events (`Button` and `Buttons` of the secondary button), so `h.OnContextMenu` runs after the release and `OnClick` does not; it combines with `Delay`. The actionability checks are those of a primary click (visible, enabled, in view, not covered), as in Playwright, and the failure says `cannot right-click`. `(*App).Tap(l Locator)` (Playwright's `tap`) puts a touch pointer's primary contact down at the element's center and lifts it, then the pointer leaves; the element's `OnClick` runs. `(*App).LongPress(l Locator)` presses the same way, advances the app clock past `gesture.LongPressTimeout` (500 ms, plus 100 ms), and lifts it: `h.OnLongPress` runs and a `kit.ContextMenu` or `aria.UseContextMenuTrigger` menu opens (ADR-167: touch and pen only). `Tap` and `LongPress` take no options. All three settle like the other actions (`NoSettle` respected) and fail with messages that name the action (`cannot tap`, `cannot long-press`). A touch contact is its own pointer (kind touch, id 1): it does not move the mouse position of `MouseMove` and `Hover`. Not added: a pen, a middle button, `LongPressAt`/`TapAt`; a test that needs a pen, a move while held or a sequence between the press and the release still builds its pointer events by hand.

## Rationale

Tests of context menus, long presses and touch behavior injected pointer events through a headless window by hand (`kit/menu_test.go`, `aria/helpers_test.go`). Playwright has these actions, and the engine already turned the events into `contextmenu` and long-press dispatch. The tests that were a plain right click, tap or long press now use them.
