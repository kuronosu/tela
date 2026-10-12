// Package auto selects the platform backend for the current environment.
//
// New is the single entry point used by applications and tools: it returns
// the native backend of the OS (Wayland or X11 on Linux), or the headless
// backend when the user asks for it with TELA_BACKEND=headless.
//
// # Linux
//
// TELA_BACKEND selects a backend explicitly: "wayland", "x11" or "headless".
// When it is set, exactly that backend is used and its error, if any, is
// returned as is. When it is not set, Wayland is tried first and X11 second;
// a candidate is skipped only if its constructor reports
// [platform.ErrUnavailable] (for example WAYLAND_DISPLAY is not set).
// Any other error is a real failure and is returned without trying the next
// candidate.
//
// # Other systems
//
// Only TELA_BACKEND=headless works until the native backend of the OS is
// implemented.
package auto

import (
	"fmt"
	"os"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// EnvBackend is the environment variable that overrides backend selection.
const EnvBackend = "TELA_BACKEND"

// New returns the backend for the current environment. See the package
// documentation for the selection rules.
func New() (platform.Backend, error) {
	name := strings.TrimSpace(os.Getenv(EnvBackend))
	switch name {
	case "":
		return native()
	case "headless":
		// A wall-clock headless backend, so that Run works like on a real
		// backend.
		return headless.New(headless.Options{RealTime: true}), nil
	}
	return named(name)
}

// errUnknown is the error for an invalid TELA_BACKEND value.
func errUnknown(name string, valid []string) error {
	return fmt.Errorf("auto: unknown %s=%q; valid values are %s", EnvBackend, name, strings.Join(valid, ", "))
}

var _ platform.Backend = (*headless.Backend)(nil)
