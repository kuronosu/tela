# ADR-179: Platform contract v2 (window control and desktop services)

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

Details of ADR-178 in `internal/platform` (amends §15; full contract in `doc.go`).

- **Window control** is requests on `Window` (`Minimize`, `SetMaximized`, `SetFullscreen`, `SetVisible`, `SetAlwaysOnTop`, `SetMin/MaxSize`, `SetPosition`, `Activate`, `SetIcon`, `SetHitRegions`) plus `WindowOptions` fields; the outcome is a new `WindowStateEvent` (maximized, fullscreen, minimized where known, client-side decorations, position where known) and the usual `ConfigureEvent`. `internal/loop` coalesces state like configures. Positions are physical desktop pixels of the content area's top-left. `WindowOptions` stays comparable: the icon is set only by `SetIcon` (several sizes, `PickIcon`).
- **Hit regions:** last region wins, `HitClient` punches holes; a press in a caption region and the rest of its sequence are the OS's; button regions stay app input. Client-side-decorated windows get resize edges from the backend.
- **Close closes children first** on every backend (Win32 forces it). Modality is enforced by the OS where possible and always by core.
- **Session restore:** one opaque per-window `RestoreToken` (Wayland encodes session id and toplevel name); "" where unneeded.
- **Desktop services:** one `Backend.Desktop()` interface; ID-based notifications and trays; async answers through the loop, cancel is nil result and nil error; `ErrUnsupported` wraps `errors.ErrUnsupported`; message boxes use fixed button sets (MessageBoxW's). `UnsupportedDesktop` checks arguments.
- The headless backend is a deterministic fake window manager and desktop; the remote protocol becomes version 2.

## Rationale

Requests plus reported state is the only model that holds on tiling and Wayland compositors; IDs and plain-data events cross the remote protocol; fixed button sets map to every native box.
