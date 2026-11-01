# ADR-210: Platform contract v3 (desktop integration)

- **Date:** 2026-11-01
- **Status:** Accepted

## Decision

Details of ADR-209 in `internal/platform` (amends §15; contract and per-OS mapping in `doc.go`).

- **`Data`**, ordered (MIME type, bytes), carries drags and the clipboard; text, HTML, PNG and `text/uri-list` map onto OS formats. `Clipboard` adds `ReadTypes`, `Read`, `Write`.
- **Drag and drop:** a `DragEvent` session per window, answered by the sticky `Window.SetDropEffect(effect, types...)` (Win32 delivers synchronously inside `IDropTarget`). The drop carries the data the app named, read first. `Window.StartDrag` needs a held press, cancels the pointer, ends with `DragDoneEvent`.
- **Menus:** `MenuItem.Accelerator`; `Window.SetMenu` (`MenuEvent`), async `Window.PopupMenu`.
- **Shortcuts:** string IDs; the outcome is always a `ShortcutStatusEvent`.
- **Settings v2:** accent color, high contrast, text scale.
- **Secrets** and **`Associations`** block and are safe from any goroutine.
- **Single instance:** `Desktop.ClaimInstance`, `LaunchEvent`, `Window.ActivateWithToken`.
- **Registration** lives in `internal/platform/assoc`, without a running app.
- Headless fakes all of it; remote protocol 4 carries none yet.

## Rationale

Plain events cross the remote protocol; a sticky answer fits Win32's synchronous and Linux's later replies; reading the drop first gives `OnDrop` its data on every OS.
