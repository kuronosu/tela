//go:build darwin

package auto

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/cocoa"
)

var validNames = []string{"cocoa", "headless"}

// named constructs the backend called name (TELA_BACKEND is set). New handles
// "headless" before calling it.
func named(name string) (platform.Backend, error) {
	switch name {
	case "cocoa":
		return native()
	case "wayland", "x11":
		return nil, fmt.Errorf("auto: %s=%q is only available on Linux (GOOS=darwin): %w",
			EnvBackend, name, platform.ErrUnavailable)
	case "win32":
		return nil, fmt.Errorf("auto: %s=%q is only available on Windows (GOOS=darwin): %w",
			EnvBackend, name, platform.ErrUnavailable)
	}
	return nil, errUnknown(name, validNames)
}

// native returns the Cocoa backend. The caller must be on the main thread
// (tela.Run is): cocoa.New refuses any other, and locks the goroutine to it.
// Without a window server session (an SSH login) the error wraps
// platform.ErrUnavailable.
func native() (platform.Backend, error) {
	b, err := cocoa.New()
	if err != nil {
		return nil, err
	}
	return b, nil
}

// prefetch does nothing: the Cocoa backend's settings are local reads.
func prefetch(string) {}
