package remote

import "github.com/kuronosu/tela/internal/platform"

// Phase-13 requests and services (ADR-209, ADR-210) are not forwarded by
// protocol 4: the child checks the arguments like every backend and
// reports ErrUnsupported, and the host drops the phase-13 events its real
// backend delivers (host.go). Step C forwards them in a new protocol
// version and decides whether secrets and associations are served in the
// child (they need no window) or by the host.

// ActivateWithToken asks the host to activate its window; the token is
// not forwarded yet.
func (w *Window) ActivateWithToken(string) { w.Activate() }

// SetDropEffect checks its arguments; no drag reaches the child yet.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
}

// StartDrag checks its arguments and returns ErrUnsupported.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	return platform.ErrUnsupported
}

// SetMenu checks its arguments and returns ErrUnsupported.
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	return platform.ErrUnsupported
}

// PopupMenu checks its arguments and returns ErrUnsupported.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	return platform.ErrUnsupported
}

// ReadTypes returns ErrUnsupported.
func (c clipboard) ReadTypes(done func([]string, error)) error {
	if done == nil {
		panic("remote: Clipboard.ReadTypes called with a nil callback")
	}
	return platform.ErrUnsupported
}

// Read checks its arguments and returns ErrUnsupported.
func (c clipboard) Read(mime string, done func([]byte, error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	return platform.ErrUnsupported
}

// Write checks its arguments and returns ErrUnsupported.
func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	return platform.ErrUnsupported
}

func (d childDesktop) RegisterShortcut(s platform.Shortcut) error {
	return platform.UnsupportedDesktop{}.RegisterShortcut(s)
}

func (d childDesktop) UnregisterShortcut(id string) error {
	return platform.UnsupportedDesktop{}.UnregisterShortcut(id)
}

func (d childDesktop) SetSecret(appID, name string, secret []byte) error {
	return platform.UnsupportedDesktop{}.SetSecret(appID, name, secret)
}

func (d childDesktop) Secret(appID, name string) ([]byte, error) {
	return platform.UnsupportedDesktop{}.Secret(appID, name)
}

func (d childDesktop) DeleteSecret(appID, name string) error {
	return platform.UnsupportedDesktop{}.DeleteSecret(appID, name)
}

func (d childDesktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	return platform.UnsupportedDesktop{}.ClaimInstance(appID, launch)
}

func (d childDesktop) Associations() platform.Associations {
	return platform.UnsupportedAssociations{}
}

// phase13Event reports whether e is an event protocol 4 cannot carry.
func phase13Event(e platform.Event) bool {
	switch e.(type) {
	case platform.DragEvent, platform.DragDoneEvent, platform.MenuEvent,
		platform.ShortcutEvent, platform.ShortcutStatusEvent, platform.LaunchEvent:
		return true
	}
	return false
}
