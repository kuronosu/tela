//go:build linux

package freedesktop

import (
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// Phase-13 services (ADR-209, ADR-210). Stubs until step B: they check
// their arguments like every backend and return platform.ErrUnsupported.
// Step B implements them here, shared by Wayland and X11:
//
//   - Global shortcuts: org.freedesktop.portal.GlobalShortcuts
//     (CreateSession, BindShortcuts with each shortcut's description and
//     preferred_trigger in the XDG shortcuts format, "CTRL+SHIFT+k" with
//     an xkb keysym name; the Response's trigger_description; the
//     Activated, Deactivated and ShortcutsChanged signals). Without the
//     portal the X11 backend grabs keys itself (XGrabKey); Wayland has no
//     other way (ErrUnsupported).
//   - Secrets: implemented in secret.go.
//   - Single instance: the well-known bus name of the app ID (or one
//     derived from it when the ID is not a valid bus name), requested
//     with DBUS_NAME_FLAG_DO_NOT_QUEUE; the primary exports a Launch
//     method (arguments, directory, token) on it, which a second process
//     calls before exiting.
//   - Associations: package assoc (Native).

// RegisterShortcut is not implemented yet: ErrUnsupported.
func (d *Desktop) RegisterShortcut(s platform.Shortcut) error {
	return platform.UnsupportedDesktop{}.RegisterShortcut(s)
}

// UnregisterShortcut is not implemented yet: ErrUnsupported.
func (d *Desktop) UnregisterShortcut(id string) error {
	return platform.UnsupportedDesktop{}.UnregisterShortcut(id)
}

// ClaimInstance is not implemented yet: ErrUnsupported.
func (d *Desktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	return platform.UnsupportedDesktop{}.ClaimInstance(appID, launch)
}

// Associations returns the registration of the session's desktop
// (package assoc).
func (d *Desktop) Associations() platform.Associations { return assoc.Native() }
