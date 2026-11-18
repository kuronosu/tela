# ADR-297: Logs, crash reports and the toast activator

- **Date:** 2026-11-17
- **Status:** Accepted. Amends ADR-127 and ADR-235.

## Decision

- **Structured logging:** `desktop.Logger()` is a `log/slog` logger writing JSON lines to `LogDir()/<name>.log` (rotated by size, a few files kept), also text to standard error when the program has one; `desktop.SetLogLevel`. Tela's own warnings go there too.
- **Crash reports:** a panic on the UI thread or a `UseGo` goroutine, and fatal runtime errors (`runtime/debug.SetCrashOutput`), are written to `LogDir()/crash-<time>.txt` with the version, OS and stack. `desktop.OnCrash(func(desktop.Crash))` runs on the next start for each report not yet handled, so the app can send it where it wants; Tela sends nothing.
- **Toast activator (Windows):** a COM local server whose CLSID is derived from the app ID (a name-based UUID), registered with the AUMID (`CustomActivator`, `LocalServer32` with the executable and a flag) and declared in the MSIX manifest (`ToastActivatorCLSID`). A toast clicked after the app exited starts it, and the click arrives as the `NotificationEvent` it would have been.

## Why

A GUI program has no standard output (ADR-127); reports handed to the app keep Tela free of a crash service.
