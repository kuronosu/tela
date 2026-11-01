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
//   - Secrets: the Secret Service (org.freedesktop.secrets) with an
//     encrypted session ("dh-ietf1024-sha256-aes128-cbc-pkcs7": DH group
//     2 from math/big, HKDF-SHA256, AES-128-CBC, all in the standard
//     library), items in the default collection with the attributes
//     xdg:schema "dev.telaui.Secret", app-id and name; Unlock prompts
//     (org.freedesktop.Secret.Prompt) are waited for, since these calls
//     block and run off the UI thread.
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

// SetSecret is not implemented yet: ErrUnsupported.
func (d *Desktop) SetSecret(appID, name string, secret []byte) error {
	return platform.UnsupportedDesktop{}.SetSecret(appID, name, secret)
}

// Secret is not implemented yet: ErrUnsupported.
func (d *Desktop) Secret(appID, name string) ([]byte, error) {
	return platform.UnsupportedDesktop{}.Secret(appID, name)
}

// DeleteSecret is not implemented yet: ErrUnsupported.
func (d *Desktop) DeleteSecret(appID, name string) error {
	return platform.UnsupportedDesktop{}.DeleteSecret(appID, name)
}

// ClaimInstance is not implemented yet: ErrUnsupported.
func (d *Desktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	return platform.UnsupportedDesktop{}.ClaimInstance(appID, launch)
}

// Associations returns the registration of the session's desktop
// (package assoc).
func (d *Desktop) Associations() platform.Associations { return assoc.Native() }
