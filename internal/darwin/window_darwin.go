//go:build darwin

package darwin

// AppKit constants of window control (NSWindow.h, NSView.h,
// NSVisualEffectView.h): style masks, levels, collection behaviors,
// ordering, the standard buttons and the visual effect materials.
const (
	// NSWindowStyleMaskFullScreen is set while the window is in native
	// full screen (also during the transitions).
	NSWindowStyleMaskFullScreen = 1 << 14
	// NSWindowStyleMaskFullSizeContentView extends the content view under
	// the title bar, to the whole frame.
	NSWindowStyleMaskFullSizeContentView = 1 << 15

	// Window levels (CGWindowLevel keys): normal windows and the floating
	// level of palettes and always-on-top windows.
	NSNormalWindowLevel   = 0
	NSFloatingWindowLevel = 3

	// NSWindowCollectionBehavior bits: a window that can be the primary
	// window of a full-screen space, or one shown with another window in
	// its full-screen space.
	NSWindowCollectionBehaviorFullScreenPrimary   = 1 << 7
	NSWindowCollectionBehaviorFullScreenAuxiliary = 1 << 8

	// NSWindowOrderingMode values (addChildWindow:ordered:,
	// addSubview:positioned:relativeTo:).
	NSWindowAbove = 1
	NSWindowBelow = -1

	// NSWindowButton values (standardWindowButton:).
	NSWindowCloseButton       = 0
	NSWindowMiniaturizeButton = 1
	NSWindowZoomButton        = 2

	// NSWindowTitleHidden is the titleVisibility that draws no title.
	NSWindowTitleHidden = 1
	// NSTitlebarSeparatorStyleNone draws no line under the title bar
	// (macOS 11+).
	NSTitlebarSeparatorStyleNone = 1

	// NSAutoresizingMaskOptions that keep a subview the size of its
	// superview.
	NSViewWidthSizable  = 1 << 1
	NSViewHeightSizable = 1 << 4

	// NSVisualEffectView: blending with what is behind the window, and a
	// material that looks active or inactive with its window.
	NSVisualEffectBlendingModeBehindWindow      = 0
	NSVisualEffectStateFollowsWindowActiveState = 0

	// NSVisualEffectMaterial values.
	NSVisualEffectMaterialTitlebar              = 3
	NSVisualEffectMaterialMenu                  = 5
	NSVisualEffectMaterialPopover               = 6
	NSVisualEffectMaterialSidebar               = 7
	NSVisualEffectMaterialHeaderView            = 10
	NSVisualEffectMaterialSheet                 = 11
	NSVisualEffectMaterialWindowBackground      = 12
	NSVisualEffectMaterialHUDWindow             = 13
	NSVisualEffectMaterialUnderWindowBackground = 21
)

// SendSize sends sel and returns a CGSize (NSSize) result:
// -[NSWindow contentMinSize], -[NSWindow minSize]. A CGSize is two doubles
// like a CGPoint, so it comes back in the same registers on both
// architectures (a homogeneous floating-point aggregate on arm64, xmm0
// and xmm1 on amd64) through the same objc_msgSend view.
func SendSize(id ID, sel SEL, args ...any) CGSize {
	p := sendPoint(id, sel, args...)
	return CGSize{Width: p.X, Height: p.Y}
}
