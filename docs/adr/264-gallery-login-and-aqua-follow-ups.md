# ADR-264: Gallery, login and Aqua follow-ups

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-197, ADR-255, ADR-257, ADR-258.

## Decision

- **A demo window** (owner): the gallery's Title Bar and Menubar pages have a button that opens a second window (`tela.Window`, frameless, transparent) with a resizable card (`h.WindowResize`), a `kit.TitleBar` and a drawn `kit.Menubar{AppMenu: true, Native: kit.NativeOff}`, so F10, a lone Alt and the resize handles can be tried live (the gallery's own window is framed). Its texts follow the gallery's language and direction; the Mac flavor has no F10 or Alt, by design (ADR-255).
- **The `Count` presets** are named by the catalog (`count`), as the rest of the page.
- **The DateRangePicker keeps its order.** Under right to left the button reads "2026/11/05 - 2026/10/20" left to right, the start (Oct 20) at the right: the reading order of an RTL line. Chromium lays out the same string, and Intl's `formatRange` (CLDR's interval pattern, start first, an en dash), identically for `ar`, `ar-EG` and `he` (checked with Chromium 155, headless); isolating each date would change nothing. A test pins the start at the reading side in both directions.
- **`examples/login`** builds its alert with `kit.Alert` (destructive, with the tinted `Class` it had), the bio counter as the Field's `LabelEnd` and "Or continue with" as `Separator.Caption`; the look is unchanged within a few pixels.
- **`looks/aqua` keeps macOS's own accent.** White on Apple's blue is 3.2:1 to 4.2:1 (Default) and the Link's blue 2.8:1 (Tahoe light, `#0088ff`) to 4.6:1: the look reproduces the platform, so the colors stay and `TestAccentTextContrast` pins floors. The Select trigger's text in Tahoe's dark is 8.6:1 (14:1 in light): no change.

## Why

A look is a copy of the platform, not a fix of it; a live demo needs a window the app draws; an unchanged order is the correct one, and a test says so.
