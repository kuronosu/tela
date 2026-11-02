// Package cocoa is the macOS platform backend. It talks to AppKit,
// QuartzCore, CoreGraphics and CoreFoundation through the Objective-C
// runtime and purego (via the leaf bindings package internal/darwin), with
// no cgo, and implements the platform contract documented in package
// platform ("Mapping onto each OS: Cocoa"). The code that touches the OS is
// built only on darwin; cursor.go holds the pure cursor table and is
// tested on every OS.
//
// # Status (phase 14, ADR-222)
//
// This is the base of the backend: the app and its event loop, windows,
// presentation and HiDPI, plus the clipboard (NSPasteboard with every Data
// type, clipboard_darwin.go over the pure mapping in pasteboard.go) and the
// system settings with their change events (settings_darwin.go).
// Keyboard, pointer and scrolling input, IME (NSTextInputClient) and the
// display link come in the next tasks of phase 14; window control
// (minimize, zoom, fullscreen, positions, child and modal windows enforced
// by the OS), native menus, drag and drop and the desktop services are the
// macOS desktop integration phase. Until then those requests are validated
// like on every backend and do nothing, and the services return
// ErrUnsupported.
//
// # Loop
//
// The UI thread is the process's main thread: [New] must be called there
// (tela.Run is, since package tela locks the main goroutine in an init
// function), and it locks the calling goroutine to it until Run returns.
// Run calls [NSApp run], which owns the loop. The backend is called back
// from AppKit (the window delegate and the content view) and from two
// CoreFoundation objects on the main run loop, both in
// kCFRunLoopCommonModes so that they also fire inside AppKit's tracking
// loops (live resize):
//
//   - a version 0 CFRunLoopSource, which Wake and Quit signal from any
//     goroutine (CFRunLoopSourceSignal + CFRunLoopWakeUp); its perform
//     callback runs a delivery pass of the shared loop
//     (internal/platform/internal/loop);
//   - a CFRunLoopTimer armed at the shared loop's pacing deadline (Pace)
//     while a frame waits, and pushed to the far future otherwise, so an
//     idle app never wakes up. This is the pacing of the base backend: the
//     display link (CVDisplayLink, or NSView.displayLink on macOS 14+)
//     replaces it in the next task, as the platform documentation
//     describes.
//
// A delegate callback that queues events (a resize, a close request, a
// focus change) runs a delivery pass itself, so a live resize delivers its
// ConfigureEvent and the forced FrameEvent, and the app presents, before
// AppKit continues. Quit stops the app with [NSApp stop:] and posts an
// application-defined event so that run returns. A panic in the handler
// cannot unwind through the Objective-C frames above the callbacks: the
// callback recovers it, stops the app and Run panics again with the value.
// Every callback runs inside its own autorelease pool.
//
// # Windows and presentation
//
// A window is an NSWindow (titled, closable, miniaturizable and, unless
// FixedSize, resizable; borderless when Frameless) whose content view is a
// registered NSView subclass that hosts a CALayer (the layer is set before
// wantsLayer, so AppKit never draws into it; the GPU backend will give the
// view a CAMetalLayer instead and nothing else changes). The view is
// flipped, the layer's contentsScale follows backingScaleFactor, and
// its contentsGravity pins the frame to the top-left corner, pixel for
// pixel. Sizes are points (logical) and the scale is backingScaleFactor.
// Present keeps a copy of the frame (damage-only presents update it),
// copies it into a CGImage in the sRGB color space
// (kCGImageAlphaPremultipliedLast, Go's image.RGBA layout) and sets it as
// the layer's contents inside a CATransaction without implicit animations.
// The window is ordered front at its first Present. Closing a window
// releases its objects at the next pass, outside any AppKit method that
// might still use them.
package cocoa
