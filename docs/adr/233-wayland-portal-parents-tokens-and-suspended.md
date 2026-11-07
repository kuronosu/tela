# ADR-233: Wayland portal parents, activation tokens and suspended windows

- **Date:** 2026-11-07
- **Status:** Accepted. Amends ADR-179 and ADR-185 for Wayland.

## Decision

Details in `internal/platform/wayland/foreign.go` and `doc.go`.

- **Parents:** portal requests pass `wayland:<handle>` from `zxdg_exporter_v2` (BindShortcuts: the focused window's). One export per toplevel, made at its first request and kept until the toplevel is destroyed (hide, close); hidden or unmapped windows have none.
- **Waiting:** a request waits for the handle through the loop, as `gtk_window_export_handle` does, bounded by a `wl_display.sync` round trip, then goes without a parent.
- **Tokens:** OpenURI and OpenFile pass an `xdg_activation_v1` token as `activation_token` (xdg-open: `XDG_ACTIVATION_TOKEN`); `Activate` uses a notification's or tray's token (at most 10 s old) instead of its own.
- **Suspended** (xdg-shell 6) reports `WindowState.Minimized` and pauses frames like hiding, so core commits without frames (ADR-185).

## Why

GTK unexports per dialog; Tela cannot tell when every portal dialog ends. Minimized is what core pauses on; hidden-workspace and locked-screen windows get no frames either.
