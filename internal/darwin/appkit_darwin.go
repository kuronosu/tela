//go:build darwin

package darwin

// AppKit constants (NSWindow, NSApplication, NSEvent).
const (
	NSWindowStyleMaskBorderless     = 0
	NSWindowStyleMaskTitled         = 1 << 0
	NSWindowStyleMaskClosable       = 1 << 1
	NSWindowStyleMaskMiniaturizable = 1 << 2
	NSWindowStyleMaskResizable      = 1 << 3

	NSBackingStoreBuffered = 2

	// NSWindowTabbingModeDisallowed keeps AppKit from merging windows into
	// tabs (macOS 10.12+), which Tela does not model.
	NSWindowTabbingModeDisallowed = 2

	// NSApplicationActivationPolicyRegular makes an unbundled process an
	// ordinary app with a Dock icon whose windows can become key.
	NSApplicationActivationPolicyRegular = 0

	// NSEventTypeApplicationDefined is the type of the event posted to
	// make [NSApp run] return after [NSApp stop:].
	NSEventTypeApplicationDefined = 15

	// NSTerminateCancel is the answer of applicationShouldTerminate: that
	// keeps the app running.
	NSTerminateCancel = 0
)

// QuartzCore constants (NSString values), set by Load.
var (
	// CAGravityTopLeft and CAGravityBottomLeft are contentsGravity
	// values. Their vertical meaning follows the layer's
	// contentsAreFlipped (CALayer documentation).
	CAGravityTopLeft    ID
	CAGravityBottomLeft ID
)

func loadConstants(cf, cg, qc uintptr) {
	CFRunLoopCommonModes = symbolValue(cf, "kCFRunLoopCommonModes")
	CGColorSpaceSRGB = symbolValue(cg, "kCGColorSpaceSRGB")
	CAGravityTopLeft = ID(symbolValue(qc, "kCAGravityTopLeft"))
	CAGravityBottomLeft = ID(symbolValue(qc, "kCAGravityBottomLeft"))
}
