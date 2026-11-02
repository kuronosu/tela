# ADR-222: Cocoa backend base

- **Date:** 2026-11-02
- **Status:** Accepted

## Decision

The macOS backend (phase 14, ARCHITECTURE §15) is `internal/platform/cocoa` over `github.com/ebitengine/purego` v0.11.2, imported only by darwin files of the new leaf package `internal/darwin` (ADR-189): typed bindings to the Objective-C runtime, CoreFoundation, CoreGraphics, QuartzCore and AppKit, shared with the future Metal backend. No build flag is needed (purego's fakecgo); the minimum is macOS 13, Go 1.27's.

- **Loop:** `[NSApp run]` on the main thread; Wake and Quit signal a `CFRunLoopSource` in the common modes. NSApp is a subclass whose `sendEvent:` forwards the key releases AppKit swallows while Command is held.
- **Pacing:** a display link per window (`NSView.displayLink` on macOS 14+, `CVDisplayLink` on 13), a Go timer otherwise.
- **Present:** a layer-hosting, flipped content view whose `CALayer` shows a `CGImage` of the frame (sRGB, premultiplied); the GPU backend swaps in a `CAMetalLayer`.
- **IME:** the view is an `NSTextInputClient` (ADR-068); key downs go through `interpretKeyEvents:` while a text field is focused.
- **Tests** run on `macos-latest`. purego passes sub-8-byte stack arguments unpacked on arm64, so test key events are built from CGEvents.

## Rationale

The design in `internal/platform/doc.go`, plus what it left open. A hosted layer is the one AppKit never redraws. A `CFRunLoopTimer` first paced the frames and fired only about every 100 ms on the runner.
