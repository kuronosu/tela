//go:build windows

package auto

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/win32"
)

var validNames = []string{"win32", "headless"}

// named constructs the backend called name (TELA_BACKEND is set). New handles
// "headless" before calling it.
func named(name string) (platform.Backend, error) {
	switch name {
	case "win32":
		return native()
	case "wayland", "x11":
		return nil, fmt.Errorf("auto: %s=%q is only available on Linux (GOOS=windows): %w",
			EnvBackend, name, platform.ErrUnavailable)
	}
	return nil, errUnknown(name, validNames)
}

// native returns the Win32 backend. Like on Linux, the caller must be the
// goroutine that will run the event loop: win32.New locks it to its OS
// thread.
func native() (platform.Backend, error) {
	b, err := win32.New()
	if err != nil {
		return nil, err
	}
	return b, nil
}
