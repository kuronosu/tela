//go:build linux

package freedesktop

import "github.com/kuronosu/tela/internal/platform"

// The app ID (ADR-217). The backends create the Desktop with the
// executable's name (platform.DefaultAppID); the layers above call
// SetAppID with the app's own ID before the first window, and under tela
// dev the host's backend gets the child app's ID this way (remote
// protocol 5). The ID names the desktop entry the GlobalShortcuts portal
// looks for, so it must be the app's, not the executable's.

// SetAppID sets the app ID of the services: the portal registration of
// the global shortcuts (a changed ID binds the registered shortcuts again
// under it, on a new connection), the notifications' app name and
// desktop-entry hint, and the Id and Title of trays added later. UI
// thread.
func (d *Desktop) SetAppID(id string) {
	platform.CheckAppID("SetAppID", id)
	d.idMu.Lock()
	changed := d.id != id
	d.id = id
	d.idMu.Unlock()
	if !changed {
		return
	}
	sc := &d.shortcuts
	sc.mu.Lock()
	started := sc.started && !sc.closed
	sc.mu.Unlock()
	if started {
		d.shortcutOp(shortcutOp{rebind: true})
	}
}

// appID returns the app ID. Safe from any goroutine.
func (d *Desktop) appID() string {
	d.idMu.Lock()
	defer d.idMu.Unlock()
	return d.id
}
