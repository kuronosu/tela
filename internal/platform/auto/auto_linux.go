//go:build linux

package auto

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland"
	"github.com/kuronosu/tela/internal/platform/x11"
)

var validNames = []string{"wayland", "x11", "headless"}

// named constructs the backend called name (TELA_BACKEND is set). New handles
// "headless" before calling it.
func named(name string) (platform.Backend, error) {
	switch name {
	case "wayland":
		b, err := wayland.New()
		if err != nil {
			return nil, err
		}
		return b, nil
	case "x11":
		b, err := x11.New()
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return nil, errUnknown(name, validNames)
}

// native tries Wayland, then X11, falling through only on ErrUnavailable.
func native() (platform.Backend, error) {
	wl, wlErr := wayland.New()
	if wlErr == nil {
		return wl, nil
	}
	if !errors.Is(wlErr, platform.ErrUnavailable) {
		return nil, wlErr
	}
	x, xErr := x11.New()
	if xErr == nil {
		return x, nil
	}
	if !errors.Is(xErr, platform.ErrUnavailable) {
		return nil, xErr
	}
	return nil, fmt.Errorf("auto: no display server available (wayland: %w; x11: %w); "+
		"run inside a Wayland or X11 session, or set %s=headless to run without a window",
		wlErr, xErr, EnvBackend)
}
