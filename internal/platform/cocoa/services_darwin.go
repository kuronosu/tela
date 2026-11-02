//go:build darwin

package cocoa

import "github.com/kuronosu/tela/internal/platform"

// Desktop returns the desktop services: none yet (macOS desktop
// integration). The clipboard is in clipboard_darwin.go and the system
// settings in settings_darwin.go.
func (b *Backend) Desktop() platform.Desktop { return platform.UnsupportedDesktop{} }
