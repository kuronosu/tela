package remote

import "github.com/kuronosu/tela/internal/platform"

// Desktop returns the desktop services. Until they are forwarded to the
// host over the control channel, every service is unsupported.
func (b *Backend) Desktop() platform.Desktop { return platform.UnsupportedDesktop{} }
