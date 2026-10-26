# ADR-127: Windows console window

- **Date:** 2026-10-22
- **Status:** Accepted

## Decision

Windows console window (the owner's decision). Go builds Windows executables for the console subsystem by default, so Windows opens a console window next to a Tela app started from Explorer, a shortcut or the Start menu. **Release builds use `-ldflags=-H=windowsgui`** (GUI subsystem: only the app's window opens), documented in Getting Started ("Ship it") and the README; phase 17's `tela build` sets it for Windows targets by default and, since a GUI program has no standard output or error, writes panics and the app's log to a file in its data directory. **Tela does not hide or detach the console at run time** (`FreeConsole`/`ShowWindow` when the process owns its console): the console would still flash on screen at every start, which users read as malware. Development is unaffected: `go run` and `tela dev` run from a terminal and use it

## Rationale

A console flash at start looks like a virus; the subsystem flag is the standard fix (Fyne, Gio, Wails document the same) and costs nothing at run time; output must not be lost silently in shipped apps, hence the log file in `tela build`
