# ADR-232: X11 touchpad fling and drag image

- **Date:** 2026-11-07
- **Status:** Accepted

## Decision

Amends ADR-057 (X11 had no scroll phases) and ADR-054.

- **Fling:** the zero-delta XInput2 scroll event is the scroll stop, as in GTK 3 (`is_stop`, since 3.20) and Firefox. A touchpad whose driver is known to post it reports `ScrollPhaseUpdate`, and the stop becomes a `ScrollPhaseEnd` for the gesture's window. Known means the device has the `libinput Send Events Modes Available` property (xf86-input-libinput posts libinput's value-0 axis event), or a stop was already seen from it. Wheels, Synaptics (it coasts by itself) and Xwayland keep `ScrollPhaseNone`. Xwayland ≥ 23.1 posts the stop for `axis_stop` but drops `axis_source`, so its finger scrolling cannot be told from a wheel. There is no timeout heuristic. Scroll times come from the server's monotonic timestamps.
- **Drag image:** an override-redirect window with `_NET_WM_WINDOW_TYPE_DND`, the image as its background pixmap, and an empty SHAPE input region, so `TranslateCoordinates` (the XDND target walk) sees through it, as Qt's `WindowTransparentForInput` does. It is ARGB when a compositing manager owns `_NET_WM_CM_S<n>` and a 32-bit visual exists. Otherwise it uses the root visual, shaped to the pixels that are at least half opaque. Without SHAPE 1.1 it shows no image.
- **Ask:** with more than one allowed action, `XdndActionList` (default first) and `XdndActionDescription` are set. `XdndActionAsk` is requested on a secondary or middle button drag or with Alt alone (GTK's rules).

## Rationale

These follow the exact signals the drivers and toolkits use, with no guessing. A phase without an end would latch core's scroll container forever.
