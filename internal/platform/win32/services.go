//go:build windows

package win32

import "github.com/kuronosu/tela/internal/platform"

// Desktop returns the desktop services (phase 12, ADR-178). Until the
// Win32 services task hooks them up, every service is unsupported.
func (b *Backend) Desktop() platform.Desktop { return platform.UnsupportedDesktop{} }
