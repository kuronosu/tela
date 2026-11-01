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
//   - Global shortcuts: implemented in shortcuts.go.
//   - Secrets: the Secret Service (org.freedesktop.secrets) with an
//     encrypted session ("dh-ietf1024-sha256-aes128-cbc-pkcs7": DH group
//     2 from math/big, HKDF-SHA256, AES-128-CBC, all in the standard
//     library), items in the default collection with the attributes
//     xdg:schema "dev.telaui.Secret", app-id and name; Unlock prompts
//     (org.freedesktop.Secret.Prompt) are waited for, since these calls
//     block and run off the UI thread.
//   - Single instance: implemented in instance.go.
//   - System settings: implemented in settings.go.
//   - Associations: package assoc (Native).

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

// Associations returns the registration of the session's desktop
// (package assoc).
func (d *Desktop) Associations() platform.Associations { return assoc.Native() }
