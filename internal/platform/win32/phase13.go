//go:build windows

package win32

import (
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// Phase-13 requests and services (ADR-209, ADR-210). Stubs until step B,
// which implements them as "Mapping onto each OS" in package platform
// says: OLE drag and drop (OleInitialize on the UI thread,
// RegisterDragDrop with an IDropTarget per window, DoDragDrop started
// outside the handler like the dialogs), the clipboard formats (CF_UNICODETEXT,
// "HTML Format", "PNG" with CF_DIBV5/CF_DIB, CF_HDROP, registered custom
// formats), menu bars (CreateMenu, SetMenu, WM_COMMAND; framed windows
// only) and popup menus (TrackPopupMenuEx outside the handler),
// RegisterHotKey and WM_HOTKEY, Credential Manager (CredWriteW, CredReadW,
// CredDeleteW in advapi32), and single instance with a named mutex and a
// WM_COPYDATA message to the first instance's message-only window.

// ActivateWithToken activates the window. Windows has no tokens: the
// second process allowed this one to take the foreground
// (AllowSetForegroundWindow).
func (w *Window) ActivateWithToken(string) { w.Activate() }

// SetDropEffect checks its arguments; no drag reaches the window yet.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
}

// StartDrag checks its arguments and returns ErrUnsupported until step B.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	return platform.ErrUnsupported
}

// SetMenu checks its arguments and returns ErrUnsupported until step B.
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	return platform.ErrUnsupported
}

// PopupMenu checks its arguments and returns ErrUnsupported until step B.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	return platform.ErrUnsupported
}

// ReadTypes returns ErrUnsupported until step B.
func (c clipboard) ReadTypes(done func([]string, error)) error {
	if done == nil {
		panic("win32: Clipboard.ReadTypes called with a nil callback")
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

func (d desktop) RegisterShortcut(s platform.Shortcut) error {
	return platform.UnsupportedDesktop{}.RegisterShortcut(s)
}

func (d desktop) UnregisterShortcut(id string) error {
	return platform.UnsupportedDesktop{}.UnregisterShortcut(id)
}

func (d desktop) SetSecret(appID, name string, secret []byte) error {
	return platform.UnsupportedDesktop{}.SetSecret(appID, name, secret)
}

func (d desktop) Secret(appID, name string) ([]byte, error) {
	return platform.UnsupportedDesktop{}.Secret(appID, name)
}

func (d desktop) DeleteSecret(appID, name string) error {
	return platform.UnsupportedDesktop{}.DeleteSecret(appID, name)
}

func (d desktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	return platform.UnsupportedDesktop{}.ClaimInstance(appID, launch)
}

// Associations returns the current user's registration (package assoc).
func (d desktop) Associations() platform.Associations { return assoc.Native() }
