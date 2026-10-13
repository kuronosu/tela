//go:build windows

package auto

import (
	"errors"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// release runs the backend's loop once so that Run's shutdown destroys its
// hidden window and unlocks the thread.
func release(t *testing.T, b platform.Backend) {
	t.Helper()
	b.Quit()
	if err := b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {})); err != nil {
		t.Errorf("Run: %v", err)
	}
}

func TestWin32(t *testing.T) {
	for _, env := range []string{"", "win32"} {
		t.Run("TELA_BACKEND="+env, func(t *testing.T) {
			t.Setenv(EnvBackend, env)
			b, err := New()
			if err != nil {
				t.Fatalf("New() with %s=%q: %v", EnvBackend, env, err)
			}
			defer release(t, b)
			if b.Name() != "win32" {
				t.Fatalf("Name() = %q, want win32", b.Name())
			}
		})
	}
}

func TestLinuxBackendsAreUnavailable(t *testing.T) {
	for _, name := range []string{"wayland", "x11"} {
		t.Setenv(EnvBackend, name)
		b, err := New()
		if err == nil {
			t.Fatalf("New() with %s=%s = %v, nil; want an error", EnvBackend, name, b)
		}
		if !errors.Is(err, platform.ErrUnavailable) {
			t.Errorf("error %v does not wrap platform.ErrUnavailable", err)
		}
	}
}
