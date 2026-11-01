//go:build linux

package wayland

import "github.com/kuronosu/tela/internal/platform"

// Phase-13 window and clipboard requests (ADR-209, ADR-210). Stubs until
// step B, which implements them on wl_data_device_manager (drag and drop,
// clipboard types: see "Drag and drop and the clipboard" in package
// platform) and xdg_activation_v1 (ActivateWithToken). Linux has no native
// menus: SetMenu and PopupMenu stay ErrUnsupported, and the layers above
// draw their menus.

// ActivateWithToken activates the window; the token is not used yet
// (step B: xdg_activation_v1.activate with it).
func (w *Window) ActivateWithToken(token string) { w.Activate() }

// SetDropEffect checks its arguments; no drag reaches the window yet.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
}

// StartDrag checks its arguments and returns ErrUnsupported (step B:
// wl_data_device.start_drag).
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	return platform.ErrUnsupported
}

// SetMenu returns ErrUnsupported: Wayland has no native menu bar.
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	return platform.ErrUnsupported
}

// PopupMenu returns ErrUnsupported: Wayland has no native popup menu.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	return platform.ErrUnsupported
}

// ReadTypes returns ErrUnsupported until step B.
func (c clipboard) ReadTypes(done func([]string, error)) error {
	if done == nil {
		panic("wayland: Clipboard.ReadTypes called with a nil callback")
	}
	return platform.ErrUnsupported
}

// Read returns ErrUnsupported until step B.
func (c clipboard) Read(mime string, done func([]byte, error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	return platform.ErrUnsupported
}

// Write returns ErrUnsupported until step B.
func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	return platform.ErrUnsupported
}
