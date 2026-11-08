//go:build linux

package wayland

import (
	"image"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/atspi"
)

// Accessibility (contract v5, ADR-246): the AT-SPI2 bridge shared with
// the X11 backend (internal/platform/internal/atspi). The first window
// starts it; it reports AccessibilityEvent and AccessActionEvent through
// the loop (dispatchAccess before every delivery pass) and is closed when
// Run returns. Wayland does not tell clients where their windows are, so
// the screen extents the bridge reports are window ones (as AccessKit's
// on Wayland).

// accessBridge returns the process's bridge, made on first use. UI thread.
func (b *Backend) accessBridge() *atspi.Bridge {
	if b.access == nil {
		b.access = atspi.New(atspi.Options{
			AppName: b.appID,
			Wake:    b.signal,
			Event:   func(w platform.Window, e platform.Event) { b.loop.Enqueue(w.(*Window), e) },
		})
	}
	return b.access
}

// dispatchAccess moves the bridge's events to the loop; called before
// every delivery pass.
func (b *Backend) dispatchAccess() {
	if b.access != nil {
		b.access.Dispatch()
	}
}

// closeAccess unregisters the app from the accessibility registry when
// Run returns.
func (b *Backend) closeAccess() {
	if b.access != nil {
		b.access.Close()
	}
}

// SetAccessibility implements platform.Window: the update goes to the
// window's AT-SPI bridge, which drops it unless an assistive technology
// is active.
func (w *Window) SetAccessibility(update platform.AccessUpdate) {
	platform.CheckAccessUpdate(&update)
	if w.acc == nil || w.Closed() {
		return
	}
	w.acc.SetGeometry(w.Metrics().Scale, image.Point{}, false)
	w.acc.Update(update)
}
