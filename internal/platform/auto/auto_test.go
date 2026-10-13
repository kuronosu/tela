package auto

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestHeadless(t *testing.T) {
	t.Setenv(EnvBackend, "headless")
	b, err := New()
	if err != nil {
		t.Fatalf("New() with %s=headless: %v", EnvBackend, err)
	}
	if b.Name() != "headless" {
		t.Fatalf("Name() = %q, want headless", b.Name())
	}
}

func TestUnknownBackend(t *testing.T) {
	t.Setenv(EnvBackend, "gdi")
	b, err := New()
	if err == nil {
		t.Fatalf("New() = %v, nil; want an error for an unknown backend", b)
	}
	for _, want := range []string{`"gdi"`, "headless"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
	if errors.Is(err, platform.ErrUnavailable) {
		t.Errorf("an unknown value must not look like an unavailable backend: %v", err)
	}
}

func TestNothingAvailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the win32 backend needs no display server")
	}
	if runtime.GOOS == "linux" {
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("DISPLAY", "")
	}
	t.Setenv(EnvBackend, "")
	b, err := New()
	if err == nil {
		t.Fatalf("New() = %v, nil; want an error without a display server", b)
	}
	if !errors.Is(err, platform.ErrUnavailable) {
		t.Errorf("error %v does not wrap platform.ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), EnvBackend+"=headless") {
		t.Errorf("error %q does not suggest %s=headless", err, EnvBackend)
	}
}

func TestExplicitBackendReturnsItsError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("wayland and x11 only exist on Linux")
	}
	for _, name := range []string{"wayland", "x11"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("WAYLAND_DISPLAY", "")
			t.Setenv("DISPLAY", "")
			t.Setenv(EnvBackend, name)
			b, err := New()
			if err == nil {
				t.Fatalf("New() = %v, nil; want the %s backend's error", b, name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not come from the %s backend", err, name)
			}
		})
	}
}
