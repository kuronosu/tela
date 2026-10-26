# ADR-056: Keyboard dispatch and focus follow the DOM and HTML

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Keyboard dispatch and focus follow the DOM and HTML (amends §14: the public event is `KeyboardEvent`, with `Key`/`Code` string types whose constants are generated from `internal/platform`; no text field, text input arrives with phase 5). Key events target the focused element, or the outermost content element when nothing is focused, and bubble. Defaults: Tab/Shift+Tab sequential navigation (positive tab indexes first, wrapping in the window or the active containing focus scope), Enter on keydown and Space on keyup activate a focused `Button`. Focusable means `Button` or `TabIndex`, minus disabled, `display: none`, invisible or `display: contents` elements. A primary press focuses the nearest focusable ancestor unless pointerdown or mousedown is prevented. A focused element that stops being focusable is blurred with `OnBlur` (Chromium); unmounting it fires nothing (React). `OnFocus`/`OnBlur` bubble (React). `focus-visible` uses Chromium's heuristic. While the window is inactive the element stays focused without focus states, and window focus changes fire `OnBlur`/`OnFocus`. `AutoFocus` applies after the style phase of the commit that mounts it, unless an element gained the focus since the mount. Programmatic focus goes through `tela.UseElementRef` + `h.Ref`. `core.FocusScope` (contain, restore) is reserved for `aria.UseFocusScope`

## Rationale

Owner's API decisions (React event name, string key types, element handle, no public trap yet); DOM/HTML semantics are what users and LLMs expect; the AutoFocus rule lets dialogs opened by a click autofocus; generated constants cannot drift from the backends
