# ADR-222: Cocoa backend base

- **Date:** 2026-11-02
- **Status:** Accepted

## Decision

Cocoa backend base (phase 14; implements §15). `github.com/ebitengine/purego` v0.11.2 enters `go.mod`, imported only by darwin files, and only by the new leaf package `internal/darwin` (ADR-189): typed bindings to the Objective-C runtime, CoreFoundation, CoreGraphics and the QuartzCore and AppKit constants, so the future Metal backend shares them without breaking the layer rules. With `CGO_ENABLED=0` purego needs no build flag on darwin (its fakecgo). The minimum is macOS 13 Ventura, what Go 1.27's linker targets.

`internal/platform/cocoa`: `New` must run on the main thread (an error otherwise; `ErrUnavailable` without a window server session, from `CGSessionCopyCurrentDictionary`); `Run` is `[NSApp run]`. Wake and Quit signal a `CFRunLoopSource` in the common modes; `Quit` sends `stop:` and posts an application-defined event. Pacing is a `CFRunLoopTimer` set to the shared loop's `Pace` deadline until the display link task. Delegate callbacks that queue events run a pass themselves (live resize repaints). The content view hosts a `CALayer` (set before `wantsLayer`), flipped, `contentsScale` = backing scale, top-left gravity; `Present` makes a `CGImage` (sRGB, premultiplied last) its contents in a `CATransaction` without actions. Closed windows' objects are released at the next run loop pass. Input, IME, clipboard, settings and the display link are later phase-14 tasks; window control and services are the macOS desktop phase and are validated no-ops or `ErrUnsupported` until then.

## Rationale

Owner-approved doc.go design plus what it left open; a hosted layer is the one AppKit never redraws and the one the GPU backend swaps for a `CAMetalLayer`. Tests run only on `macos-latest`.
