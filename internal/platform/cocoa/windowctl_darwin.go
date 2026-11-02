//go:build darwin

package cocoa

import (
	"image"

	"github.com/kuronosu/tela/internal/platform"
)

// The window control, styling and integration requests below are validated
// like on every backend and otherwise do nothing in the base backend: they
// are the macOS desktop integration phase (window control, menus, drag and
// drop). Requests are allowed to be
// refused by the platform contract, and no WindowStateEvent is sent, so
// State stays the zero WindowState.

// Minimize does nothing yet (macOS desktop integration).
func (w *Window) Minimize() {}

// SetMaximized does nothing yet (macOS desktop integration: zoom).
func (w *Window) SetMaximized(bool) {}

// SetFullscreen does nothing yet (macOS desktop integration).
func (w *Window) SetFullscreen(bool) {}

// SetAlwaysOnTop does nothing yet (macOS desktop integration).
func (w *Window) SetAlwaysOnTop(bool) {}

// SetPosition does nothing yet (macOS desktop integration); new windows
// are centered.
func (w *Window) SetPosition(image.Point) {}

// Activate does nothing yet (macOS desktop integration).
func (w *Window) Activate() {}

// ActivateWithToken is Activate: macOS has no activation tokens.
func (w *Window) ActivateWithToken(string) { w.Activate() }

// SetIcon validates the images and does nothing: macOS windows have no
// icon (the app's icon is in the Dock, from its bundle).
func (w *Window) SetIcon(images []image.Image) { platform.CheckIcon("SetIcon", images) }

// SetTitleBarColors does nothing: only Windows 11 draws colored title bars.
func (w *Window) SetTitleBarColors(platform.TitleBarColors) {}

// SetBackdrop validates the backdrop and does nothing yet (a vibrancy
// backdrop is macOS desktop integration).
func (w *Window) SetBackdrop(backdrop platform.Backdrop) {
	platform.CheckBackdrop("SetBackdrop backdrop", backdrop)
}

// SetHitRegions validates the regions and does nothing yet (a custom title
// bar is macOS desktop integration).
func (w *Window) SetHitRegions(regions []platform.HitRegion) { platform.CheckHitRegions(regions) }

// RestoreToken returns "": macOS restores windows by position.
func (w *Window) RestoreToken() string { return "" }

// SetDropEffect validates the answer and does nothing: no drag reaches the
// window yet (macOS desktop integration).
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
}

// StartDrag validates the options and returns ErrUnsupported.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	return platform.ErrUnsupported
}

// SetMenu validates the menu and returns ErrUnsupported: the app's menu
// bar is macOS desktop integration.
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	return platform.ErrUnsupported
}

// PopupMenu validates the request and returns ErrUnsupported.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	return platform.ErrUnsupported
}
