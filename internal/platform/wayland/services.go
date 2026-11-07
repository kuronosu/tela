//go:build linux

package wayland

import (
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/freedesktop"
)

// Desktop returns the desktop services: D-Bus (XDG Desktop Portal,
// org.freedesktop.Notifications, StatusNotifierItem), shared with the X11
// backend in internal/platform/internal/freedesktop. Nothing touches D-Bus
// until a service is used. UI thread.
//
// Portal requests get their parent window as "wayland:<handle>" of the
// toplevel exported with xdg-foreign (zxdg_exporter_v2), and OpenURI and
// OpenPath an xdg_activation_v1 token for the application they start;
// both arrive as events, which the request waits for (foreign.go). The
// shortcut portal's dialog is parented to the focused window.
func (b *Backend) Desktop() platform.Desktop {
	if b.desktop == nil {
		b.desktop = freedesktop.New(freedesktop.Options{
			AppID:   b.appID,
			Wake:    b.signal,
			Post:    b.loop.Post,
			Event:   func(e platform.Event) { b.loop.Enqueue(nil, e) },
			Prepare: b.preparePortal,
			Focused: b.focusedWindow,
			Now:     b.Now,
		})
	}
	return b.desktop
}

// dispatchDesktop moves the desktop services' answers and events to the
// loop; called before every delivery pass.
func (b *Backend) dispatchDesktop() {
	if b.desktop != nil {
		b.desktop.Dispatch()
	}
}

// closeDesktop removes the tray icons and closes the D-Bus connections
// when Run returns.
func (b *Backend) closeDesktop() {
	if b.desktop != nil {
		b.desktop.Close()
	}
}
