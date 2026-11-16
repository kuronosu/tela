package platform

import "image"

// Presenting through a swapchain on a Win32 window (contract v14,
// ADR-289). See "Presenting through a swapchain" in the package
// documentation.

// Win32Surface describes a Win32 window to a renderer that presents its
// frames through a swapchain on it (vkCreateWin32SurfaceKHR, or DXGI), and
// what that swapchain must follow right now.
type Win32Surface struct {
	// Instance is the HINSTANCE of the module that registered the
	// window's class and Window its HWND. They stay the same while the
	// window is open, and are handles of the OS, not memory.
	Instance, Window uintptr
	// Size is the client area in physical pixels now: what a swapchain's
	// extent must be. It is Metrics().Physical() once the ConfigureEvent of
	// a change is delivered (a resize, a DPI change that resized the
	// window) and empty while the window is minimized, when nothing may be
	// presented. During a resize it may run ahead of the metrics of the
	// frame being drawn: that frame is not presented through the
	// swapchain, and the next one, at the new size, is.
	Size image.Point
	// Full reports that the next swapchain present must be whole and must
	// happen even when nothing changed: the window lost what it showed (a
	// repaint the OS asked for, a reconnected session, a display or theme
	// change, a resume), or it was shown again by the app, or its last
	// frame was a Present. The window asks for a frame when it sets it,
	// and a present with nil damage clears it.
	Full bool
}

// Win32SurfacePresenter is implemented by Win32 windows: a renderer may
// present their frames through a swapchain on the window instead of
// Present. It is optional twice over, like [DmabufPresenter]: a backend's
// windows may not implement it (found by a type assertion), and a window
// that does may report that it cannot right now. The caller then presents
// with Window.Present, which always works. See "Presenting through a
// swapchain" in the package documentation for the full contract.
type Win32SurfacePresenter interface {
	// Win32Surface returns the window's handles and what its swapchain
	// must follow. ok is false when the window cannot be presented
	// through a swapchain: it is Transparent (a layered window, whose
	// input mask needs the pixels), it has a client-area backdrop (Mica,
	// Acrylic: DWM shows the frame's alpha, which a swapchain does not
	// carry), it is a TitleBarOverlay window (DWM's caption buttons show
	// through transparent pixels), or it is closed. The answer may change
	// while the window is open (SetBackdrop); the caller asks once a
	// frame and, when it is no, destroys its swapchain before it calls
	// Present.
	Win32Surface() (s Win32Surface, ok bool)

	// SurfacePresented tells the window that the caller presented a frame
	// through its swapchain on it (vkQueuePresentKHR returned), with the
	// damage since the previous present of either kind (buffer pixels,
	// nil meaning the whole frame). It follows Present's rules where they
	// apply: the first one shows the window (not before, so that it does
	// not flash), one to a hidden window shows nothing, and a nil damage
	// clears Win32Surface().Full. From then on the window draws nothing
	// itself (WM_PAINT asks for a frame instead of painting its retained
	// copy, which is stale) until the next Present, which must be whole
	// and comes after the caller destroyed its swapchain.
	SurfacePresented(damage []image.Rectangle)
}
