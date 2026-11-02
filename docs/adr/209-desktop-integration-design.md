# ADR-209: Desktop integration design (phase 13)

- **Date:** 2026-11-01
- **Status:** Accepted

## Decision

The owner approved this shape for phase 13 (amends §2, §10, §15; details in ADR-210 to 217). As in ADR-178, what lives while mounted is a component, and imperative calls answer on the UI thread.

- **Drag and drop is HTML's model:**
  - It works within a window, between the app's windows, and with other apps, through the OS.
  - Targets use `h.OnDragEnter`, `OnDragOver`, `OnDragLeave` and `OnDrop`, and accept a drop with `PreventDefault`.
  - A source is an `h.Draggable` element that fills its data in `OnDragStart`; the gesture props are unchanged for every other element.
  - Drags and the clipboard share one data type (text, HTML, PNG, files, custom types).
- **Native menus are opt-in.** Kit menus stay drawn. A `Native` setting per menu, or a `tela.App` field for the whole app, uses the OS menu where one exists; the menu's own setting wins.
- **Global shortcuts** are a declarative `desktop` component.
- **Single instance** is opt-in. A second process forwards its launch to the first and exits.
- **Registration of links and file types:**
  - It works at run time, without a running app, so phase 17's installers can reuse it.
  - Per user, never with administrator rights.
  - It looks before writing and writes nothing when nothing changes.
  - It never overwrites another app's association without an explicit option.
  - It can register and unregister both URL schemes and file types.
  - Registering makes the app a candidate handler. Only a separate, explicit request makes it the default; on Windows, for file types, that request opens the Default apps settings.
- **Settings v2:** the accent color, high contrast (`contrast-more:`) and the text scale. The text scale works like a browser's default font size: 1rem = the theme's rem × the scale. It is on by default and an app can turn it off.
- **Secrets:** Credential Manager on Windows; on Linux, the Secret Service with an encrypted session from the standard library.
- **§2** adds `advapi32`.
- **Done check:** `examples/notes` accepts a dropped file, runs a global shortcut, reopens in its running instance from a `notes:` link, and keeps locked notes encrypted with a key in secure storage.

## Rationale

HTML's names are what an LLM writes. Drawn menus keep one look everywhere, with native menus one field away. Rem scaling is the browser's semantics, which Tailwind sizes are designed for. Registration never takes a choice that belongs to the user.
