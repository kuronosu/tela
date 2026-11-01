//go:build linux

package wayland

import "github.com/kuronosu/tela/internal/platform"

// Phase-13 window requests (ADR-209, ADR-210) that are not drag and drop
// (dnd.go) or the clipboard (clipboard.go). Linux has no native menus:
// SetMenu and PopupMenu stay ErrUnsupported, and the layers above draw
// their menus.

// ActivateWithToken activates the window with an activation token another
// process of the app got from the compositor (XDG_ACTIVATION_TOKEN, sent
// by the single-instance launch): xdg_activation_v1.activate with it, at
// once when the window is mapped, else when it maps (like Activate). ""
// is Activate, which asks for a token of its own. Without
// xdg_activation_v1, or for a hidden window, nothing happens.
func (w *Window) ActivateWithToken(token string) {
	if token == "" {
		w.Activate()
		return
	}
	b := w.b
	if w.Closed() || b.activation == nil || w.ctl.hidden {
		return
	}
	if !w.mapped {
		w.ctl.activationToken = token // sent by onMapped
		return
	}
	b.activation.Activate(token, w.surface)
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
