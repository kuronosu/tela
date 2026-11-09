# ADR-247: Submenus, menu roles and the trigger gaps

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-101, ADR-104, ADR-133, ADR-141, ADR-215 and ADR-221.

## Decision

- **Submenus (aria):** `MenuOptions.SubmenuKeys` and `Submenu`, `MenuResult.SubmenuTrigger(k) SubmenuTriggerResult{MenuProps, Open, FocusStrategy, Close, CloseAll, Ref, MenuRef, Direction}` with `PopoverOptions()`, `SubmenuDelay` (200 ms). React Aria's rules: ArrowRight (ArrowLeft under rtl), Enter, Space open on the first item; the back key and Escape close one level; hover opens after the delay with useSafelyMouseToSubmenu's grace; focus on another item closes; an outside press closes the chain.
- **kit:** one new kind, `MenuSub` (Key, Label, Icon, Items), not shadcn's three parts: it fits the one-struct model of ADR-141. Recipe fields `SubTrigger`, `SubIcon`, `SubContent`; native menus get OS submenus. `MenuItem.Role` (item roles of ADR-226): native, the OS or desktop does it; drawn, `desktop.PerformRole` after close, in the element focused before the trigger's press.
- **Triggers:** Toggle, Checkbox, Switch, Slider and the overlays implement `Trigger`, so overlays nest; the trigger's own `Disabled` wins.
- **Context menu:** `ContextMenuTriggerResult.Opening`; the keys open below the focused element; areas get no tab stop.
- **Collections:** a context click on an unselected list box option selects it first; a selected one keeps the selection (Finder).
- **Shortcuts:** glyphs and CmdOrCtrl give `aria-keyshortcuts`; CmdOrCtrl follows the flavor (`tela.UsePlatformKeys`). Also `WindowHandle.ActiveElement`, `SliderOptions.TrackRef`.

## Rationale

Phase 20's gaps; names follow React Aria, Radix and the DOM.
