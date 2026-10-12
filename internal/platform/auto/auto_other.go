//go:build !linux

package auto

import (
	"fmt"
	"runtime"

	"github.com/kuronosu/tela/internal/platform"
)

var validNames = []string{"headless"}

// named constructs the backend called name (TELA_BACKEND is set). New handles
// "headless" before calling it, so only errors remain on this OS.
func named(name string) (platform.Backend, error) {
	switch name {
	case "wayland", "x11":
		return nil, fmt.Errorf("auto: %s=%q is only available on Linux (GOOS=%s): %w",
			EnvBackend, name, runtime.GOOS, platform.ErrUnavailable)
	}
	return nil, errUnknown(name, validNames)
}

// native reports that the native backend of this OS does not exist yet.
func native() (platform.Backend, error) {
	return nil, fmt.Errorf("auto: the native backend for GOOS=%s is not implemented yet; "+
		"set %s=headless to run without a window: %w", runtime.GOOS, EnvBackend, platform.ErrUnavailable)
}
