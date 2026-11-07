//go:build linux

package wayland

import (
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// What portal requests need from the compositor (ADR-233): the parent
// window as "wayland:<handle>" of a toplevel exported with
// zxdg_exporter_v2 (xdg-foreign-unstable-v2), and an xdg_activation_v1
// token for the application OpenURI and OpenPath start. Both arrive as
// events, so the desktop services (freedesktop.Options.Prepare) wait for
// them on the UI thread through the loop, never blocking it.
//
// One export per toplevel. A window is exported at the first request
// that names it as parent, and the export is kept until its toplevel is
// destroyed (hidden or closed; destroyRole), so later requests get the
// handle at once. GTK 4 instead exports anew for every dialog and
// unexports when the dialog ends; Tela cannot tell when every portal
// dialog ends (OpenURI's application chooser outlives its call), and the
// portal backend's import of the handle must stay valid for as long as
// the dialog shows. A handle is useless once the toplevel is gone: a
// window shown again is exported again, with a new handle.
//
// Waiting. Like gtk_window_export_handle, a request waits for the handle
// (the compositor sends it at once, xdg_exported.handle). To bound the
// wait on a compositor that never sends it, a wl_display.sync follows the
// export: events arrive in order, so when its done arrives without a
// handle, the requests go without a parent. Activation tokens get the
// same bound. A window that is hidden, not mapped yet or closed has no
// parent identifier (a dialog over a window not on screen helps nobody),
// and neither has any window without the protocol.

// foreignVersion is the zxdg_exporter_v2 version the backend binds.
const foreignVersion = 1

// exportState is a window's xdg-foreign export.
type exportState struct {
	obj     *protocols.ZxdgExportedV2 // nil when not exported
	handle  string                    // "" until the handle event
	waiters []func(handle string)
}

// exportHandle calls done with the window's exported handle, "" when it
// has none: at once when the handle is known or cannot be had, else when
// it arrives (or when the round trip after the export ends without it).
// UI thread.
func (w *Window) exportHandle(done func(handle string)) {
	b := w.b
	e := &w.export
	switch {
	case w.Closed() || b.exporter == nil || w.toplevel == nil || !w.mapped:
		done("")
		return
	case e.handle != "":
		done(e.handle)
		return
	}
	e.waiters = append(e.waiters, done)
	if e.obj != nil {
		return // exported; its handle is on its way
	}
	obj := b.exporter.ExportToplevel(w.surface)
	e.obj = obj
	obj.SetHandleHandler(func(ev protocols.ZxdgExportedV2HandleEvent) {
		if e.obj != obj {
			return // unexported since
		}
		e.handle = ev.Handle
		w.flushExport(ev.Handle)
	})
	b.afterRoundtrip(func() {
		if e.obj == obj && e.handle == "" {
			w.flushExport("") // no handle came: no parent
		}
	})
}

// flushExport calls the requests waiting for the handle.
func (w *Window) flushExport(handle string) {
	waiters := w.export.waiters
	w.export.waiters = nil
	for _, done := range waiters {
		done(handle)
	}
}

// unexport destroys the export, before the toplevel goes (destroyRole):
// importers lose the relation, and requests still waiting go without a
// parent.
func (w *Window) unexport() {
	e := &w.export
	if e.obj != nil {
		e.obj.Destroy()
	}
	e.obj, e.handle = nil, ""
	w.flushExport("")
}

// afterRoundtrip calls f when the compositor has answered every request
// sent before it (wl_display.sync). UI thread.
func (b *Backend) afterRoundtrip(f func()) {
	cb := b.display.Sync()
	cb.SetDoneHandler(func(protocols.WlCallbackDoneEvent) { f() })
}

// requestToken asks xdg_activation_v1 for a token and calls done with it
// ("" without the protocol, or, when bounded, if the compositor does not
// answer within a round trip: a request waits for it). The token carries
// the latest input serial and its seat (none before any input, as winit
// does), the surface of our focused window, else that of w (if not nil
// and open), and appID if not "". UI thread.
func (b *Backend) requestToken(w *Window, appID string, bounded bool, done func(token string)) {
	if b.activation == nil {
		done("")
		return
	}
	tok := b.activation.GetActivationToken()
	if s := b.lastSeat; s != nil && b.lastSerial != 0 {
		tok.SetSerial(b.lastSerial, s.wl)
	}
	req := w
	if req != nil && req.Closed() {
		req = nil
	}
	for _, o := range b.loop.Windows() {
		if o.focused {
			req = o
			break
		}
	}
	if req != nil {
		tok.SetSurface(req.surface)
	}
	if appID != "" {
		tok.SetAppId(appID)
	}
	answered := false
	tok.SetDoneHandler(func(e protocols.XdgActivationTokenV1DoneEvent) {
		tok.Destroy()
		if !answered {
			answered = true
			done(e.Token)
		}
	})
	tok.Commit()
	if !bounded {
		return
	}
	b.afterRoundtrip(func() {
		if !answered {
			answered = true
			done("")
		}
	})
}

// preparePortal is freedesktop.Options.Prepare: the parent identifier of
// pw and, if token is set, an activation token for the application the
// request starts (no app id: the portal picks the application). done
// runs once both are known.
func (b *Backend) preparePortal(pw platform.Window, token bool, done func(parent, token string)) {
	w, _ := pw.(*Window)
	if w != nil && w.b != b {
		w = nil
	}
	var parent, tok string
	pending := 1
	if token {
		pending++
	}
	finish := func() {
		if pending--; pending == 0 {
			done(parent, tok)
		}
	}
	if w != nil {
		w.exportHandle(func(h string) {
			if h != "" {
				parent = "wayland:" + h
			}
			finish()
		})
	} else {
		finish()
	}
	if token {
		b.requestToken(w, "", true, func(t string) {
			tok = t
			finish()
		})
	}
}

// focusedWindow is freedesktop.Options.Focused: our window with keyboard
// focus or activated, else nil.
func (b *Backend) focusedWindow() platform.Window {
	for _, w := range b.loop.Windows() {
		if w.focused {
			return w
		}
	}
	return nil
}
