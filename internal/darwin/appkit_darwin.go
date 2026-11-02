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

// NSEventType values ([NSEvent type]) of the input events the backends
// handle (NSEvent.h).
const (
	NSEventTypeLeftMouseDown     = 1
	NSEventTypeLeftMouseUp       = 2
	NSEventTypeRightMouseDown    = 3
	NSEventTypeRightMouseUp      = 4
	NSEventTypeMouseMoved        = 5
	NSEventTypeLeftMouseDragged  = 6
	NSEventTypeRightMouseDragged = 7
	NSEventTypeMouseEntered      = 8
	NSEventTypeMouseExited       = 9
	NSEventTypeKeyDown           = 10
	NSEventTypeKeyUp             = 11
	NSEventTypeFlagsChanged      = 12
	NSEventTypeScrollWheel       = 22
	NSEventTypeOtherMouseDown    = 25
	NSEventTypeOtherMouseUp      = 26
	NSEventTypeOtherMouseDragged = 27
)

// NSEventModifierFlagCommand and NSEventModifierFlagControl are bits of
// [NSEvent modifierFlags] (the keyboard package has the full set).
const (
	NSEventModifierFlagControl = 1 << 18
	NSEventModifierFlagCommand = 1 << 20
)

// NSEventPhase values ([NSEvent phase] and momentumPhase): a bit set.
const (
	NSEventPhaseBegan      = 1 << 0
	NSEventPhaseStationary = 1 << 1
	NSEventPhaseChanged    = 1 << 2
	NSEventPhaseEnded      = 1 << 3
	NSEventPhaseCancelled  = 1 << 4
	NSEventPhaseMayBegin   = 1 << 5
)

// NSEventSubtypeTabletPoint is the subtype ([NSEvent subtype]) of mouse
// events that a tablet pen generates.
const NSEventSubtypeTabletPoint = 1

// NSTrackingAreaOptions (NSTrackingArea.h).
const (
	NSTrackingMouseEnteredAndExited  = 0x01
	NSTrackingMouseMoved             = 0x02
	NSTrackingCursorUpdate           = 0x04
	NSTrackingActiveInKeyWindow      = 0x20
	NSTrackingActiveAlways           = 0x80
	NSTrackingAssumeInside           = 0x100
	NSTrackingInVisibleRect          = 0x200
	NSTrackingEnabledDuringMouseDrag = 0x400
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
