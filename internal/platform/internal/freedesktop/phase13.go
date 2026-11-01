//go:build linux

package freedesktop

import (
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// Phase-13 services (ADR-209, ADR-210), shared by Wayland and X11: global
// shortcuts (shortcuts.go), secrets (secret.go), single instance
// (instance.go) and system settings (settings.go). Registration of schemes
// and file types needs no bus: package assoc.

// Associations returns the registration of the session's desktop
// (package assoc).
func (d *Desktop) Associations() platform.Associations { return assoc.Native() }
