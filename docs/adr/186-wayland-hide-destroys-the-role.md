# ADR-186: Wayland hides a window by destroying its role objects

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

Amends ADR-179 for the Wayland backend (details in `internal/platform/wayland/windowctl.go`, `unmap` and `SetVisible`).

- `SetVisible(false)` on a mapped window commits a null buffer, then destroys the role objects: decoration, `xdg_dialog_v1` and `xdg_toplevel_session_v1` (destroyed, not removed), then `xdg_toplevel` and `xdg_surface`. The `wl_surface` stays, with its viewport, fractional scale and text input.
- `SetVisible(true)` makes a new `xdg_surface` and `xdg_toplevel` on it, sends everything the toplevel carries (title, app id, size limits, parent, maximized and fullscreen wishes, decoration mode, modality, icon), adds it to the session under a fresh name (removing the old one, so the restore token changes), and makes the initial commit. Its configure maps the window as before; children are re-parented to the new toplevel when it maps.
- An activation granted before the window maps is sent when it maps.
- Every compositor gets this path; none is detected.

## Rationale

xdg-shell allows a new initial commit on a kept toplevel, but Hyprland 0.56 never answers it with a configure, so a hidden window never came back. A new `xdg_surface` is a new window to every compositor; GTK 4 and Qt hide this way.
