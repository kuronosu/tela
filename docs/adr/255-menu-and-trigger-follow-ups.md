# ADR-255: Menu and trigger follow-ups

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-133, ADR-141, ADR-183, ADR-247 and ADR-251.

## Decision

- **Submenus in a bar:** `MenuOptions.ShouldCloseOnInteractOutside`, the root popover's rule, which its submenus follow: a press on another bar button with a submenu open switches menus.
- **Origin of roles:** `MenuBarResult.FocusedBefore()` and `MenuTriggerResult.FocusedBefore` (the focus that came from outside by Tab, a press or F10; cleared when it leaves); kit's drawn role items act there, keyboard or pointer. Engine: `core.RelatedTarget`, `core.Contains` (internal).
- **F10 / lone Alt:** `MenuBarOptions.ActivationKeys` (`core.MenuBarKeys`, first live bar in tree order): focus the first trigger; again, or Escape, gives the focus back. Not under the Mac flavor. kit sets it only on a drawn `AppMenu` bar, the window's bar.
- `MenuItem.ContentClass` styles a `MenuSub`'s panel.
- Under RTL a shortcut is its own LTR paragraph (macOS, Windows). A trailing ellipsis stays bidi's: Chromium shows `…Print` too.
- Keyboard context menus open at the focused element's center, or a text field's caret (Chromium's ShowNonLocatedContextMenu).
- `ActiveElement` returns the element's own ref, else one stable per element.
- `kit.LoadingTrigger(t, loading)`: the overlay's hook runs before a `TriggerFunc`, whose props can land anywhere, so the Button's `Loading` cannot reach it.
- A controlled context menu set open by the app is an opening.
- Role labels follow `i18n.Provider`.
- No `w-fit` on `Menubar`: shadcn has none and Chromium stretches it too.
- `telatest.App.Move` is `MoveWindow` (beside `MouseMove`); the old name is removed, no deprecation.

## Why

Phase 20's leftovers, following native menus, Chromium and shadcn.
