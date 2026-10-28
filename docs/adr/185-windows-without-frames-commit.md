# ADR-185: Windows without frames still build and run effects

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

Amends §8, ADR-180 and ADR-183 (details in `internal/core/paused.go`).

- A window that gets no frames (hidden by the app, minimized, or the child of a minimized window) still **commits**: after every event, the Host runs its tree's `Frame` (updates, build, style, layout, layout effects, paint into display lists) and its passive effects, without raster or present, when it has components to build or effects to run. It starts after the window's first `ConfigureEvent`. Layout- or paint-only work waits for a frame.
- Its framebuffer is then stale: the frame requested at the commit, delivered once the window shows, rasterizes in full.
- Commits are **paced like frames**: one per window per 1/60 s of the backend clock, through the Host's timer. `Host.PausedWork` reports the next one; telatest settles, flushes and advances to it.
- A `ConfigureEvent` with an empty size (Win32 minimized) keeps the tree's viewport.

## Rationale

Builds and effects ran only in frames, so a tray menu, a store write, a Window opened from a hidden main window or a timer's update waited until it was shown. Committing at the Host holds for every backend and for telatest; pacing keeps a busy hidden tree as cheap as a shown one. Wayland cannot report minimized windows: their work runs at the compositor's next frame callback.
