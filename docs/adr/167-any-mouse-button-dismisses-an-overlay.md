# ADR-167: Any mouse button dismisses an overlay; a context menu's long press is touch and pen only

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

(v0.10.4; engine only, no API change.) **1.** Interact-outside and the overlay stack count a pointerdown outside the top overlay with the mouse's primary, secondary or middle button; touch and pen count their tip only (not barrel or back/forward buttons). Start at the down, end at the up, StopPropagation/PreventDefault and modality unchanged. The `contextmenu` event still reaches its target after the release, so a right click elsewhere on a context-menu area closes the menu and opens a new one at the new point (kit keys its panel per opening so the new one takes the focus), one in another area opens that area's menu, and one outside every area only closes it. Supersedes the primary-only rule of ADR-096; amends ADR-101 and lifts limit (1) of ADR-133. **2.** The long press of `aria.UseContextMenuTrigger` uses the internal `core.OnTouchLongPress`, whose recognizer joins the gesture arena for touch and pen only, and opens the menu for both (Radix's `whenTouchOrPen`). Public `h.OnLongPress` and `UseMenuTrigger`'s `longPress` trigger still recognize every pointer. Lifts limit (2) of ADR-133.

## Rationale

kit follows shadcn/ui, whose Radix DismissableLayer dismisses on a pointerdown outside with any button; React Aria's primary-only rule left a right click outside a context menu, dropdown or popover open. With `h.OnLongPress` on a context-menu area, a mouse held 500 ms let the long press win the arena, so a slow click on any button inside the area was lost.
