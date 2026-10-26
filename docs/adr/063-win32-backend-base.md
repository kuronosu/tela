# ADR-063: Win32 backend base

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Win32 backend base (phase 5; implements §15). `win32.New` locks the calling goroutine to its OS thread until `Run` returns (window affinity does not depend on `tela.Run`). One `WndProc` (created once with `syscall.NewCallback`) serves all windows and a message-only window; after each message that queued events it runs the shared loop's delivery pass itself, so modal move/resize loops are serviced, and `Run` ends by `PostQuitMessage` (drained from the thread queue at shutdown). A handler panic is recovered in `WndProc` and re-raised by `Run`. `Wake`/`Quit` post one coalesced private message to the message-only window. Pacing is a goroutine that blocks in `DwmFlush` (sleeping out the refresh interval if it returns at once) and posts a private message that becomes the loop's display signal; no timers, and the goroutine exists only while a frame waits. Present converts damage into a retained top-down BGRA slice, blits with `SetDIBitsToDevice`, shows the window at the first Present and answers `WM_PAINT` from the copy. Pointer input uses mouse messages with `SetCapture` while a button is held; a leave during the capture is deferred to the release, a stolen capture is a `PointerCancel` (pure state machine in `pointer.go`). `WM_POINTER`, keyboard, IME, clipboard, system settings and `WM_GETDPISCALEDSIZE` are left for later

## Rationale

Owner-approved doc.go design plus the details it left open; pure logic lives in files without a build tag so it is tested on every OS, and the Windows files run the shared `platformtest` suite under Wine
