# ADR-168: Focus after the focused element of a containing scope unmounts

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

(v0.10.4; engine only, no API change; amends ADR-160 and ADR-056.) When the focused element unmounts, or starts a Presence exit, inside the active containing scope, the focus work after the commit focuses the scope's root if it is focusable (Radix FocusScope `trapped`: `aria.UseDialog`'s `TabIndex(-1)` panel), else its first tabbable element (React Aria `contain`), else its first focusable one, else nothing. It runs once nothing else of the commit can focus (Radix's MutationObserver runs after React's layout effects): after AutoFocus and scope restoration, so a closing dialog still restores the focus and an AutoFocus element mounted in the same commit wins, and after the commit's layout effects, so a layout effect that focuses (a wizard's next step) makes the only focus change. That is at once after a discrete event whose commit has no layout effect pending, else in the frame after the layout effects and before paint; a keydown that comes before that frame runs it first, so Escape reaches the dialog. Passive effects are not waited for. Any focus change meanwhile cancels it; a press on nothing focusable does not (the scope holds the focus against it, ADR-160). The root, or the fallback element, is focused as a script focus without scrolling (Radix uses preventScroll), so `focus-visible` follows the last interaction; no OnBlur (React). Scopes without Contain, and the fix-up blur of an element that stops being focusable, still leave nothing focused (the browser's outcome).

## Rationale

A modal dialog's Escape, keyboard shortcuts and Tab order need a focused element inside it; after removing an item from inside a dialog the keyboard user was left with nothing focused until the next Tab. Radix is kit's reference, and focusing the panel does not jump to an arbitrary control.
