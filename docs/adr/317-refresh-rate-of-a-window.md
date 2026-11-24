# ADR-317: The refresh rate of a window

- **Date:** 2026-11-23
- **Status:** Accepted. The platform half of ADR-315's pacing.

## Decision

`platform.FrameEvent.Refresh` is the refresh interval of the output the window is on, so an application that renders its own frames learns the pace: the presentation after the one `Time` estimates is at `Time+Refresh`. 0 is unknown.

- **Which output:** the one showing most of the window (X11's RandR CRTC, the Win32 monitor, the `NSScreen`). A Wayland surface does not know that: the fastest `wl_output` it entered, as its scale is the largest. 0 on none.
- **Variable refresh:** the interval at the mode's rate, the highest.
- **Looked up at each FrameEvent** (the shared loop's `Refresh` hook) from what the backend keeps: no event and no frame when it changes, nothing runs while idle.
- **Pacing is unchanged:** it describes the display. X11's frames stay a 60 Hz timer's, Win32's `DwmFlush`'s.
- Headless reports `Options.FrameInterval` (60 Hz); `Window.SetRefresh` changes only what is reported.

Not covered: uncapped frame rates, variable refresh, tearing and direct scanout (Tela composes the frame), and when a frame was really shown: no `wp_presentation` or presentation feedback.

## Why

`Time` alone does not tell 60 from 144 Hz before a run of frames was measured.

## Changes

Platform contract v18, remote protocol 20. ARCHITECTURE §15. `internal/platform/refresh.go` and the backends' `refresh*.go`.
