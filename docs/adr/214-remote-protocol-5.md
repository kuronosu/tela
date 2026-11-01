# ADR-214: `tela dev` with phase-13 desktop integration (remote protocol 5)

- **Date:** 2026-11-01
- **Status:** Accepted

## Decision

Amends ADR-184 and ADR-210; remote protocol version 5 (details in `internal/platform/remote/phase13.go`).

- **Forwarded:** settings v2, clipboard `Data`, `ActivateWithToken`, menus (accelerators included; a pending child's menu bar is set at its promotion), `PopupMenu` like a dialog.
- **Drag and drop:** the host forwards drag sessions to the active child, and the drop's data read by its backend. `SetDropEffect` is a message, so the OS gets the latest answer the host holds, one round trip late: Win32 refuses a drag that just entered until the next `DragOver`, X11 waits for the next motion, and a SetDropEffect while handling the drop does not reach the source. `StartDrag` runs on the real window under the press the host saw; `DragDoneEvent` returns to the starting child.
- **Shortcuts** are held like trays: an equal re-registration is not sent again (the portal would ask each time); changes apply at promotion; unclaimed ones go at promotion, and stay after a crash.
- **Single instance:** the host claims the app ID and keeps it across reloads. A second `tela dev` of the app is a second instance: its launch goes to the first, it exits, and the supervisor says why.
- **Secrets and associations** go to the host's backend, served off its UI thread.

## Rationale

The host is the long-lived process with the real windows; one implementation, the backend's, serves every service, so tests use the headless fakes.
