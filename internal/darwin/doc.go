// Package darwin holds thin, typed bindings to the macOS system libraries
// that Tela calls without cgo, through github.com/ebitengine/purego: the
// Objective-C runtime (selectors, message sends with typed results, class
// registration for delegates and view subclasses, autorelease pools),
// CoreFoundation (the run loop and its sources, CFData), CoreGraphics
// (color spaces, data providers, images, bitmap contexts, window capture)
// and the constants of QuartzCore and AppKit that the backends need, plus
// Foundation conversions (NSString and NSData to and from bytes, NSArray),
// AppKit's NSString constants by name and the UniformTypeIdentifiers
// framework (foundation_darwin.go), and the text input helpers: NSRange,
// NSString as UTF-16, the secure event input and, for tests, the Text
// Input Sources API (text_darwin.go).
// AppKit and QuartzCore classes themselves are reached through the
// Objective-C runtime (GetClass, Send). The desktop services add blocks
// (completion handlers made from Go functions, CallBlock for the system's
// own, CFRunLoopPerformBlock; block_darwin.go), the Security framework's
// SecItem functions (security_darwin.go, opened on first use), Launch
// Services and its Uniform Type Identifier lookup for the registration of
// schemes and file types (launchservices_darwin.go, opened on first use,
// with CoreFoundation of its own so it needs no Load), the
// UserNotifications framework (usernotifications_darwin.go), file URLs,
// NSError, dictionaries and arrays (foundation_services_darwin.go) and the
// constants of the panels, alerts and status items
// (appkit_services_darwin.go), those of drag and drop (dnd_darwin.go),
// the few constants of the menus and the quit request
// (appkit_menu_darwin.go), and AppKit's NSAccessibility functions with
// NSNumber and NSURL values and, for tests, the AXUIElement client API
// (access_darwin.go, opened on first use).
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
// opened on first use instead: a backend without them still works, and so
// are the Carbon hot keys and the keyboard layout's characters of the
// global shortcuts ([LoadCarbon], carbon_darwin.go). The CGEvent
// constructors build input events for tests, which hand them to AppKit
// in-process; only the global shortcut test posts one to the system
// ([CGEventPost]), since a hot key is matched there.
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
//   - Arguments narrower than 8 bytes must not go on the stack (after the
//     8 integer and pointer registers of arm64, counting self and _cmd of
//     a message send; 6 on amd64). Darwin's arm64 ABI packs such stack
//     arguments at their natural alignment, and purego's message sends
//     through [Send] give each its own 8-byte slot: the keyCode
//     (unsigned short) after isARepeat (BOOL) of +[NSEvent
//     keyEventWithType:...] arrived as 0. Build such objects another way
//     (key events from a CGEvent), or pass only 8-byte values there. The
//     calls that have stack arguments today are safe: otherEventWithType:
//     (data2 is an NSInteger) and CGImageCreate (shouldInterpolate and
//     intent are false and 0, which read the same however they are laid
//     out, since the unused bytes of a slot are zero).
package darwin
