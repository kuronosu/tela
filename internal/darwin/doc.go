// Package darwin holds thin, typed bindings to the macOS system libraries
// that Tela calls without cgo, through github.com/ebitengine/purego: the
// Objective-C runtime (selectors, message sends with typed results, class
// registration for delegates and view subclasses, autorelease pools),
// CoreFoundation (the run loop, its sources and timers, CFData), CoreGraphics
// (color spaces, data providers, images, bitmap contexts, window capture)
// and the constants of QuartzCore and AppKit that the backends need, plus
// Foundation conversions (NSString and NSData to and from bytes, NSArray),
// AppKit's NSString constants by name and the UniformTypeIdentifiers
// framework (foundation_darwin.go).
// AppKit and QuartzCore classes themselves are reached through the
// Objective-C runtime (GetClass, Send).
//
// It is a leaf package (ADR-189, ADR-222): it imports only purego and the
// standard library, so every layer may use it. The cocoa platform backend
// (internal/platform/cocoa) is its first user; the future Metal backend on
// the gfx side is meant to share it, which is why it does not live under
// internal/platform.
//
// The bindings exist only on darwin; on other systems the package is empty.
// Call [Load] once before anything else: it opens the frameworks and binds
// every function, and returns an error instead of panicking when one is
// missing. The function variables are nil until then. CoreVideo's display
// link ([LoadCoreVideo]) and HIToolbox's keyboard type ([KeyboardISO]) are
// opened on first use instead: a backend without them still works. The
// CGEvent constructors build input events for tests, which hand them to
// AppKit in-process; nothing here posts events to the system.
//
// Rules for callers, which the bindings do not check:
//
//   - AppKit objects are used on the main thread only.
//   - Objects returned by alloc/init, new, copy and the CF Create/Copy
//     functions are owned by the caller (+1) and released by it; others are
//     autoreleased or borrowed, so code that creates them outside an event
//     callback of AppKit wraps itself in [PoolPush]/[PoolPop].
//   - Go memory passed to a function is only read during the call (CFData
//     copies it); nothing here keeps Go pointers in C memory.
//   - Callbacks ([NewCallback], [RegisterClass] methods) are a limited
//     resource of purego (about 2000 per process): create them once.
package darwin
