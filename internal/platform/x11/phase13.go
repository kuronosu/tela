//go:build linux

package x11

import "github.com/kuronosu/tela/internal/platform"

// Phase-13 window and clipboard requests (ADR-209, ADR-210). Stubs until
// step B, which implements drag and drop with XDND version 5 (XdndAware,
// XdndSelection; see "Drag and drop and the clipboard" in package
// platform), clipboard types on the CLIPBOARD selection (TARGETS, INCR),
// and ActivateWithToken with the startup notification ID. Linux has no
// native menus: SetMenu and PopupMenu stay ErrUnsupported, and the layers
// above draw their menus.

// ActivateWithToken activates the window; the token is not used yet
// (step B: its _TIME timestamp as the user time of _NET_ACTIVE_WINDOW, and
// a _NET_STARTUP_INFO "remove" message).
func (w *Window) ActivateWithToken(token string) { w.Activate() }

// SetDropEffect checks its arguments; no drag reaches the window yet.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
}

// StartDrag checks its arguments and returns ErrUnsupported (step B: XDND
// as the source).
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	return platform.ErrUnsupported
}

// SetMenu returns ErrUnsupported: X11 has no native menu bar.
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	return platform.ErrUnsupported
}

// PopupMenu returns ErrUnsupported: X11 has no native popup menu.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	return platform.ErrUnsupported
}

// ReadTypes returns ErrUnsupported until step B.
func (c clipboard) ReadTypes(done func([]string, error)) error {
	if done == nil {
		panic("x11: Clipboard.ReadTypes called with a nil callback")
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
