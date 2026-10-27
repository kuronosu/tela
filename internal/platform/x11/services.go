//go:build linux

package x11

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/freedesktop"
)

// Desktop returns the desktop services: D-Bus (XDG Desktop Portal,
// org.freedesktop.Notifications, StatusNotifierItem), shared with the
// Wayland backend in internal/platform/internal/freedesktop. Nothing
// touches D-Bus until a service is used. Portal dialogs are parented with
// the "x11:<hex XID>" identifier of their window. UI thread.
func (b *Backend) Desktop() platform.Desktop {
	if b.desktop == nil {
		b.desktop = freedesktop.New(freedesktop.Options{
			AppID:  platform.DefaultAppID(),
			Wake:   b.signal,
			Post:   b.loop.Post,
			Event:  func(e platform.Event) { b.loop.Enqueue(nil, e) },
			Parent: portalParent,
		})
	}
	return b.desktop
}

// portalParent is the XDG Desktop Portal identifier of a window.
func portalParent(w platform.Window) string {
	if xw, ok := w.(*Window); ok && xw != nil {
		return fmt.Sprintf("x11:%x", uint32(xw.id))
	}
	return ""
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
