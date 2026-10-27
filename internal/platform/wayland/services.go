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
// Portal dialogs get no parent window: "wayland:<handle>" needs a toplevel
// exported with xdg-foreign (zxdg_exporter_v2), whose handle arrives
// asynchronously; the dialogs are then not modal to the window.
func (b *Backend) Desktop() platform.Desktop {
	if b.desktop == nil {
		b.desktop = freedesktop.New(freedesktop.Options{
			AppID: b.appID,
			Wake:  b.signal,
			Post:  b.loop.Post,
			Event: func(e platform.Event) { b.loop.Enqueue(nil, e) },
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
