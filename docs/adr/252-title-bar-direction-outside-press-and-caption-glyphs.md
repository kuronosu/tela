# ADR-252: Title bar direction, outside presses, caption glyphs and access keys

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-238, ADR-236 and ADR-234 (contract v7, protocol 11).

## Decision

- **Direction.** The fallback title bar follows the outermost `Dir` along the window's wrapper chain: the root's content and, while an element holds one box and no text (through components and fragments; portals do not count), that box. `Dir("auto")` resolves from its text. Without one, left to right.
- **Outside presses.** A primary click in a `HitCaption` region that moved and maximized nothing is reported at its release as `platform.CaptionClickEvent` (contract v7). Core runs the top overlay's `OnInteractOutsideStart` and `OnInteractOutside` for it, nothing else. Wayland and headless send it, remote forwards it; X11, Win32 and Cocoa start the move at the press and cannot tell. A drag is no press.
- **No opt-out:** the owner chose `WindowDrag` and `Frameless` as the only ways to drop the bar.
- **Caption glyphs:** `DWMWA_USE_IMMERSIVE_DARK_MODE` from the bar's given or sampled color (ADR-236) is the right mechanism; `DWMWA_TEXT_COLOR` colors only the caption text.
- **Access keys:** a letter of the label's own script when keyboards type it (Cyrillic, Greek, Arabic, Hebrew…); for IME scripts or symbols, an appended Latin key, "ファイル(&F)": the role's English letter when free, else the first free A–Z, then a digit.

## Why

A wrapper's `Dir` is the app's direction, one on a part of a screen is not. The OS takes caption presses, so only the backend can tell a click from a drag. Each key rule is what native Windows apps in that language do.
