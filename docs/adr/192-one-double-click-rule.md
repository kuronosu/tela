# ADR-192: One double-click rule

- **Date:** 2026-10-29
- **Status:** Accepted

## Decision

Amends ADR-055 (codebase review 2026-10-29, C13). Mouse and pen double clicks follow one rule, defined in `internal/platform`: two presses at most the double-click time apart and at most `platform.DoubleClickSlop` (5) logical pixels apart. The double-click time is `SystemSettings.DoubleClickInterval`, or `platform.DefaultDoubleClickInterval` (500 ms) when the OS reports none (`SystemSettings.DoubleClickTime`).

Content clicks use it: `gesture.ClickCounter.Interval` holds the OS's value, which `core` takes from the backend at start (`Config.DoubleClickInterval`) and on every `SettingsEvent` (`Tree.SetDoubleClickInterval`), like dark mode and reduced motion. Title bars the backend handles itself (`platform.CaptionClicks` on Wayland, X11 and headless) use the same rule. Touch keeps its own limits (300 ms, 100 px).

## Rationale

Content had a fixed 500 ms / 5 px, self-drawn title bars 400 ms / 4 px on Wayland and X11 and 500 ms on headless, and the Windows double-click time that Win32 already read reached nothing. One rule makes a double click on a title bar and on the content behave the same, and honors the Windows setting.

## What it changes

On Windows, content double clicks follow the user's double-click time. On Wayland and X11 the self-drawn title bar's double click goes from 400 ms / 4 px to 500 ms / 5 px (the Linux backends read no setting yet; XSETTINGS or the portal would be a separate job). `gesture.DoubleClickTime`, `gesture.DoubleClickDistance` and the per-backend defaults are gone; `CaptionClicks.Press` takes the `SystemSettings`.
