package remote

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Desktop integration (phase 13, ADR-209, ADR-210), protocol 5 (ADR-214).
//
// Drag and drop, target side. The host forwards a real window's
// DragEvents to its active child window, as input (held while input is
// frozen); each child window's view keeps the session valid (a DragOver
// or DragDrop without the DragEnter it missed gets one first). The
// child's answer, Window.SetDropEffect, is a message to the host, which
// applies it to the real window: the answer is sticky, so the host's
// backend reports to the OS the latest answer the host has applied. That
// answer lags the event it answers by one round trip to the child: Win32
// returns it from inside IDropTarget::DragEnter and DragOver, which call
// the host's handler synchronously while the child has not seen the
// event yet, so a drag entering a window is refused until the child's
// answer arrives, and then the answer reaches Windows at its next
// DragOver (called on a timer too); X11 sends it with the XdndStatus of
// the next XdndPosition (the next pointer motion); Wayland right after the
// host applies it. A release before the answer arrived is a refused drop
// (DragLeave). Answers carry the child's count of DragEnters, so a late
// answer never applies to a later session. A drop's data is read by the
// host's backend, for the types of the latest answer, and travels with
// the DragDrop. The final effect the source sees is the answer the child
// gave before the drop: the child's SetDropEffect while it handles the
// DragDrop arrives after the host's backend finished the drop, so it is
// not sent (a moving source deletes its data even when the child's OnDrop
// refuses it, under tela dev only).
//
// Drag and drop, source side. Window.StartDrag is a round trip: the host
// starts the drag from the real window, under the pointer press its
// backend saw (ErrNoGrab once that press ended), and the real window's
// DragDoneEvent goes back to the child window that started it (dropped if
// that child was replaced meanwhile). The image is sent as is, in the
// child's physical pixels.
//
// Menus. SetMenu is a round trip: the active child's menu bar is set on
// the real window and its result returned; a pending child's is kept and
// set at its promotion, like the other window state requests, and it is
// answered with whether the real window has menus (learned from the
// window's last SetMenu, or by setting its current menu again).
// MenuEvents go to the active child; PopupMenu works like a dialog,
// answered later on the child's UI thread.
//
// Global shortcuts are held by the host like trays (ADR-184): each child
// claims the IDs it registers, and a child that registers an ID the host
// holds with the same accelerator and description takes it over without
// a new registration (the Wayland portal may ask the user at each one),
// getting the latest ShortcutStatusEvent at once. A pending child's
// changed shortcut is registered at its promotion; an ID no child claims
// is unregistered when a child is promoted, or at once while a child is
// active; with no active child (the app crashed) the shortcuts stay for
// the next build. Status changes go to every child holding the ID with
// the accelerator registered; presses go to the active child if it holds
// the ID (held while input is frozen).
//
// Single instance. The host is the long-lived process, so it is the one a
// second process must find: a child's ClaimInstance makes the host claim
// the app ID on its backend (with the child's launch), and once the host
// holds it every later claim of that ID is answered "primary" at once (a
// reload). LaunchEvents go to the active child (held while input is
// frozen). When another process holds the ID (another tela dev of the same
// app, or the app itself running), the child's launch is forwarded to it
// and the child is told it is a second instance, so it exits with status
// 0 and that tela dev ends, as a second instance of the app would
// (HostOptions.OnSecondInstance lets the supervisor say why). The claim
// lasts as long as the host.
//
// The app ID (ADR-217): a child's Desktop.SetAppID goes to the host's
// backend, so the host registers the shortcuts it holds, and shows
// notifications and trays, under the child app's ID rather than its own
// (tela's executable); the portal needs the app's desktop entry.
//
// Secrets, associations and the trash (ADR-216) need no window, but are
// forwarded to the host too, rather than served in the child: the child
// links only the remote backend, and the host's backend is the one under
// test (headless fakes, never the developer's keyring, registry or
// trash). They block and are safe from any goroutine on both sides: the
// host serves them on a goroutine of their own, not on its UI thread, and
// secret and trash calls wait without a timeout (the OS may show an
// unlock prompt; the shell may take long over a big folder).
//
// Clipboard data: ReadTypes and Read are asynchronous like the dialogs,
// Write a round trip. ActivateWithToken is forwarded with its token (an
// action: the active child's only).

// --- child side: windows ---

// dragTracker wraps the app's handler on the child's UI thread to follow
// the drag sessions each window delivered (Window.SetDropEffect).
type dragTracker struct {
	b *Backend
	h platform.Handler
}

func (t dragTracker) HandleEvent(w platform.Window, e platform.Event) {
	if de, ok := e.(platform.DragEvent); ok {
		if cw, ok := w.(*Window); ok {
			t.b.mu.Lock()
			switch de.Action {
			case platform.DragEnter:
				cw.dropSession++
				cw.dragOver, cw.dropSent = true, false
			case platform.DragLeave, platform.DragDrop:
				cw.dragOver, cw.dropSent = false, false
			}
			t.b.mu.Unlock()
		}
	}
	t.h.HandleEvent(w, e)
}

// SetDropEffect sends the answer to the drag over the window to the host
// (equal answers within a session are not resent). Without a drag over
// the window, and while it handles the drop, it does nothing: the host's
// backend finished that drop already (see the top of phase13.go).
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
	b := w.b
	b.mu.Lock()
	if w.Closed() || !w.dragOver || (w.dropSent && w.dropEffect == effect && slices.Equal(w.dropTypes, types)) {
		b.mu.Unlock()
		return
	}
	w.dropSent, w.dropEffect, w.dropTypes = true, effect, slices.Clone(types)
	session := w.dropSession
	b.mu.Unlock()
	w.send(newMsg(msgSetDropEffect), func(e *enc) {
		e.u32(session)
		e.u8(uint8(effect))
		e.strs(types)
	})
}

// StartDrag asks the host to start an OS drag from its window, under the
// pointer press the host's backend holds (ErrNoGrab when there is none).
// A DragDoneEvent follows when it returns nil.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	if !w.open() {
		return platform.ErrNoGrab
	}
	_, err := childDesktop{w.b}.call(dStartDrag, 0, func(e *enc) { e.u32(w.id); e.dragOptions(opts) })
	return err
}

// SetMenu asks the host to set its window's menu bar (see the top of
// phase13.go); ErrUnsupported where the real window has none.
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	if !w.open() {
		return nil
	}
	_, err := childDesktop{w.b}.call(dSetMenu, 0, func(e *enc) { e.u32(w.id); e.menu(menu) })
	return err
}

// PopupMenu asks the host to show a native context menu over its window;
// done runs later on the UI thread.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	if !w.open() {
		return errors.New("remote: PopupMenu on a closed window")
	}
	return childDesktop{w.b}.dialog(dPopupMenu, func(e *enc) { e.u32(w.id); e.point(at); e.menu(menu) }, func(r *dec, err error) {
		if err != nil {
			done(0, err)
			return
		}
		item := platform.MenuItemID(r.u32())
		if err := r.end(); err != nil {
			done(0, err)
			return
		}
		done(item, nil)
	})
}

// ActivateWithToken asks the host to activate its window with the token.
func (w *Window) ActivateWithToken(token string) {
	w.ctl(ctlActivateToken, func(e *enc) { e.str(token) })
}

// --- child side: clipboard ---

func (c clipboard) ReadTypes(done func([]string, error)) error {
	if done == nil {
		panic("remote: Clipboard.ReadTypes called with a nil callback")
	}
	return childDesktop(c).dialog(dClipReadTypes, nil, func(r *dec, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		types := r.strs()
		if err := r.end(); err != nil {
			done(nil, err)
			return
		}
		done(types, nil)
	})
}

func (c clipboard) Read(mime string, done func([]byte, error)) error {
	platform.CheckClipboardRead(mime, done != nil)
	return childDesktop(c).dialog(dClipRead, func(e *enc) { e.str(mime) }, func(r *dec, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		p := r.optBytes()
		if err := r.end(); err != nil {
			done(nil, err)
			return
		}
		done(p, nil)
	})
}

func (c clipboard) Write(data platform.Data) error {
	platform.CheckData("Clipboard.Write", data)
	_, err := childDesktop(c).call(dClipWrite, 0, func(e *enc) { e.data(data) })
	return err
}

// --- child side: desktop ---

func (d childDesktop) RegisterShortcut(s platform.Shortcut) error {
	platform.CheckShortcut(s)
	_, err := d.call(dRegisterShortcut, 0, func(e *enc) { e.shortcut(s) })
	return err
}

func (d childDesktop) UnregisterShortcut(id string) error {
	platform.CheckShortcutID("UnregisterShortcut", id)
	_, err := d.call(dUnregisterShortcut, 0, func(e *enc) { e.str(id) })
	return err
}

// SetAppID gives the host's backend the app's ID (ADR-217): under tela
// dev the host registers the global shortcuts, and shows the
// notifications and trays, for the child app. A host that does not know
// the call is ignored.
func (d childDesktop) SetAppID(id string) {
	platform.CheckAppID("SetAppID", id)
	_, _ = d.call(dSetAppID, 0, func(e *enc) { e.str(id) })
}

// SetStrings gives the host's backend the app's texts (ADR-251): under
// tela dev the host's native message boxes and menus are the child app's.
// A host that does not know the call is ignored.
func (d childDesktop) SetStrings(strings platform.Strings) {
	_, _ = d.call(dSetStrings, 0, func(e *enc) { e.stringMap(strings) })
}

func (d childDesktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	platform.CheckAppID("ClaimInstance", appID)
	r, err := d.call(dClaimInstance, 0, func(e *enc) { e.str(appID); e.launch(launch) })
	if err != nil {
		return false, err
	}
	primary := r.boolean()
	return primary, r.end()
}

// SetSecret stores a secret through the host's backend. Safe from any
// goroutine; it waits without a timeout (an unlock prompt), until the
// host answers or the connection ends.
func (d childDesktop) SetSecret(appID, name string, secret []byte) error {
	if err := platform.CheckSecret(appID, name, secret); err != nil {
		return err
	}
	_, err := d.callFor(dSetSecret, 0, secretTimeout, func(e *enc) { e.str(appID); e.str(name); e.blob(secret) })
	return err
}

// Secret reads a secret through the host's backend (see SetSecret).
func (d childDesktop) Secret(appID, name string) ([]byte, error) {
	platform.CheckSecretName("Secret", appID, name)
	r, err := d.callFor(dSecret, 0, secretTimeout, func(e *enc) { e.str(appID); e.str(name) })
	if err != nil {
		return nil, err
	}
	p := r.optBytes()
	if err := r.end(); err != nil {
		return nil, err
	}
	if p == nil {
		p = []byte{}
	}
	return p, nil
}

// DeleteSecret removes a secret through the host's backend (see
// SetSecret).
func (d childDesktop) DeleteSecret(appID, name string) error {
	platform.CheckSecretName("DeleteSecret", appID, name)
	_, err := d.callFor(dDeleteSecret, 0, secretTimeout, func(e *enc) { e.str(appID); e.str(name) })
	return err
}

// MoveToTrash moves a file to the trash through the host's backend (the
// same machine and file system). Safe from any goroutine; like the
// secrets it waits without a timeout (Windows' shell may take long over a
// big folder), until the host answers or the connection ends.
func (d childDesktop) MoveToTrash(path string) error {
	clean, err := platform.CheckTrashPath(path)
	if err != nil {
		return err
	}
	_, err = d.callFor(dMoveToTrash, 0, secretTimeout, func(e *enc) { e.str(clean) })
	return err
}

// Associations returns the host backend's registration of schemes and
// file types; its methods block and are safe from any goroutine.
func (d childDesktop) Associations() platform.Associations { return childAssoc(d) }

type childAssoc struct{ b *Backend }

var _ platform.Associations = childAssoc{}

func (a childAssoc) call(op uint8, app platform.HandlerApp, as platform.Association, fill func(*enc)) (*dec, error) {
	return childDesktop(a).call(op, 0, func(e *enc) {
		e.handlerApp(app)
		e.association(as)
		if fill != nil {
			fill(e)
		}
	})
}

func (a childAssoc) Status(app platform.HandlerApp, as platform.Association) (platform.AssociationStatus, error) {
	platform.CheckHandlerApp("Associations.Status", app, false)
	platform.CheckAssociation("Associations.Status", as)
	r, err := a.call(dAssocStatus, app, as, nil)
	if err != nil {
		return platform.AssociationStatus{}, err
	}
	st := platform.AssociationStatus{Registered: r.boolean(), Default: r.boolean(), Other: r.str(), Role: platform.HandlerRole(r.u8())}
	return st, r.end()
}

func (a childAssoc) Register(app platform.HandlerApp, as platform.Association, opts platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.Register", app, true)
	platform.CheckAssociation("Associations.Register", as)
	r, err := a.call(dAssocRegister, app, as, func(e *enc) { e.boolean(opts.Replace) })
	if err != nil {
		return false, err
	}
	changed := r.boolean()
	return changed, r.end()
}

func (a childAssoc) Unregister(app platform.HandlerApp, as platform.Association) (bool, error) {
	platform.CheckHandlerApp("Associations.Unregister", app, false)
	platform.CheckAssociation("Associations.Unregister", as)
	r, err := a.call(dAssocUnregister, app, as, nil)
	if err != nil {
		return false, err
	}
	changed := r.boolean()
	return changed, r.end()
}

func (a childAssoc) RegisterApp(app platform.HandlerApp, opts platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.RegisterApp", app, true)
	r, err := childDesktop(a).call(dAppRegister, 0, func(e *enc) { e.handlerApp(app); e.boolean(opts.Replace) })
	if err != nil {
		return false, err
	}
	changed := r.boolean()
	return changed, r.end()
}

func (a childAssoc) UnregisterApp(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("Associations.UnregisterApp", app, false)
	r, err := childDesktop(a).call(dAppUnregister, 0, func(e *enc) { e.handlerApp(app) })
	if err != nil {
		return false, err
	}
	changed := r.boolean()
	return changed, r.end()
}

func (a childAssoc) AppStatus(app platform.HandlerApp) (platform.AppStatus, error) {
	platform.CheckHandlerApp("Associations.AppStatus", app, false)
	r, err := childDesktop(a).call(dAppStatus, 0, func(e *enc) { e.handlerApp(app) })
	if err != nil {
		return platform.AppStatus{}, err
	}
	st := platform.AppStatus{Registered: r.boolean(), Requested: r.boolean(), Other: r.str()}
	return st, r.end()
}

func (a childAssoc) MakeDefault(app platform.HandlerApp, as platform.Association) (platform.DefaultResult, error) {
	platform.CheckHandlerApp("Associations.MakeDefault", app, true)
	platform.CheckMakeDefault("Associations.MakeDefault", as)
	r, err := a.call(dAssocMakeDefault, app, as, nil)
	if err != nil {
		return platform.DefaultNone, err
	}
	res := platform.DefaultResult(r.u8())
	if err := r.end(); err != nil {
		return platform.DefaultNone, err
	}
	return res, nil
}

// --- host side: windows ---

// windowOf returns c's open window id and its real window, or nil.
func (h *Host) windowOf(c *Child, id uint32) (*childWin, *realWin) {
	cw := c.win(id)
	if cw == nil || !cw.live() || cw.rw == nil || cw.rw.closed {
		return nil, nil
	}
	return cw, cw.rw
}

// setDropEffect applies a child's answer to the drag over its window
// (msgSetDropEffect) if it answers the session the window is in.
func (c *Child) setDropEffect(d *dec) error {
	id, session, effect, types := d.u32(), d.u32(), platform.DropEffect(d.u8()), d.strs()
	if d.err == nil {
		if err := try(func() { platform.CheckSetDropEffect(effect, types) }); err != nil {
			return err
		}
	}
	return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
		if active && cw.dragging && cw.dragSession == session {
			rw.w.SetDropEffect(effect, types...)
		}
	})
}

// dragView passes a DragEvent through cw's view of the window: a session
// cw did not see start gets a DragEnter first, and a DragLeave of a
// session it never saw is dropped. It reports whether e is delivered.
func (h *Host) dragView(cw *childWin, e platform.DragEvent) bool {
	switch e.Action {
	case platform.DragEnter:
		if cw.dragging {
			leave := e
			leave.Action = platform.DragLeave
			cw.c.sendEvent(cw.id, leave)
		}
		cw.dragging = true
		cw.dragSession++
	case platform.DragOver, platform.DragDrop:
		if !cw.dragging {
			enter := e
			enter.Action, enter.Data = platform.DragEnter, nil
			cw.c.sendEvent(cw.id, enter)
			cw.dragSession++
		}
		cw.dragging = e.Action == platform.DragOver
	case platform.DragLeave:
		if !cw.dragging {
			return false
		}
		cw.dragging = false
	}
	return true
}

// startDrag starts an OS drag from the real window of c's window id, for
// its active child.
func (h *Host) startDrag(c *Child, id uint32, opts platform.DragOptions, call func(func() error) error) error {
	cw, rw := h.windowOf(c, id)
	if cw == nil || !cw.isActive() || c.state != stateActive {
		return platform.ErrNoGrab // a window that is not shown holds no press
	}
	if err := call(func() error { return rw.w.StartDrag(opts) }); err != nil {
		return err
	}
	rw.dragSrc = cw
	return nil
}

// setMenu sets the menu bar of c's window id (see the top of phase13.go).
func (h *Host) setMenu(c *Child, id uint32, menu []platform.MenuItem, call func(func() error) error) error {
	cw, rw := h.windowOf(c, id)
	if cw == nil {
		return nil
	}
	cw.menu = menu
	if cw.isActive() {
		return h.applyMenu(rw, menu, call)
	}
	if !rw.menuKnown {
		// Learn whether the real window has menus without changing it.
		return h.applyMenu(rw, rw.menu, call)
	}
	return rw.menuErr
}

// applyMenu sets rw's menu bar and remembers whether it has one.
func (h *Host) applyMenu(rw *realWin, menu []platform.MenuItem, call func(func() error) error) error {
	err := call(func() error { return rw.w.SetMenu(menu) })
	if err == nil || errors.Is(err, errors.ErrUnsupported) {
		rw.menuKnown, rw.menuErr = true, err
	}
	if err == nil {
		rw.menu = menu
	}
	return err
}

// promoteMenu gives rw the menu bar of cw, its new active child window
// (none if it never set one).
func (h *Host) promoteMenu(rw *realWin, cw *childWin) {
	if reflect.DeepEqual(rw.menu, cw.menu) || (rw.menuKnown && rw.menuErr != nil) {
		return
	}
	if err := h.applyMenu(rw, cw.menu, callSafe); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		h.reportError(fmt.Errorf("remote: setting the menu bar of window %d: %w", rw.id, err))
	}
}

// popupMenu shows a context menu over c's window id; answer sends the
// choice back.
func (h *Host) popupMenu(c *Child, id uint32, at platform.Point, menu []platform.MenuItem, call func(func() error) error, answer func(error, func(*enc))) error {
	if c.state == stateDetached {
		return errReplaced
	}
	cw, rw := h.windowOf(c, id)
	if cw == nil || !cw.isActive() {
		return errors.New("remote: PopupMenu on a window that is not shown yet (a starting build)")
	}
	return call(func() error {
		return rw.w.PopupMenu(at, menu, func(item platform.MenuItemID, err error) {
			answer(err, func(e *enc) { e.u32(uint32(item)) })
		})
	})
}

// callSafe runs f and turns a panic into an error.
func callSafe(f func() error) (err error) {
	if perr := try(func() { err = f() }); perr != nil {
		return perr
	}
	return err
}

// --- host side: global shortcuts ---

// hostShortcut is a global shortcut registered on the host's backend and
// the children that hold it, each with the shortcut it asked for.
type hostShortcut struct {
	reg    platform.Shortcut             // what the backend was last asked for
	status *platform.ShortcutStatusEvent // the backend's latest answer for reg
	claims map[*Child]platform.Shortcut  // by child
}

// registerShortcut serves a child's RegisterShortcut.
func (h *Host) registerShortcut(c *Child, s platform.Shortcut, call func(func() error) error) error {
	if c.state == stateDetached {
		return errReplaced
	}
	if hs := h.shortcuts[s.ID]; hs != nil {
		hs.claims[c] = s
		switch {
		case s == hs.reg:
			if hs.status != nil {
				c.sendEvent(0, *hs.status) // the outcome it is owed, at once
			}
			return nil
		case c.state != stateActive:
			return nil // changed by a starting build: registered at its promotion
		}
		return h.reregister(hs, s, call)
	}
	if err := call(func() error { return h.b.Desktop().RegisterShortcut(s) }); err != nil {
		return err
	}
	h.shortcuts[s.ID] = &hostShortcut{reg: s, claims: map[*Child]platform.Shortcut{c: s}}
	return nil
}

// reregister asks the backend for s in place of hs.reg. A failed request
// leaves the earlier registration in place.
func (h *Host) reregister(hs *hostShortcut, s platform.Shortcut, call func(func() error) error) error {
	old, status := hs.reg, hs.status
	hs.reg, hs.status = s, nil
	err := call(func() error { return h.b.Desktop().RegisterShortcut(s) })
	if err != nil {
		hs.reg, hs.status = old, status
	}
	return err
}

// unregisterShortcut serves a child's UnregisterShortcut: it drops the
// child's claim (a replaced child's no longer counts).
func (h *Host) unregisterShortcut(c *Child, id string, call func(func() error) error) error {
	hs := h.shortcuts[id]
	if hs == nil {
		return nil
	}
	delete(hs.claims, c)
	return h.pruneShortcut(id, hs, call)
}

// pruneShortcut unregisters id when no live child claims it and a child
// is active (without one, the next build may take it over).
func (h *Host) pruneShortcut(id string, hs *hostShortcut, call func(func() error) error) error {
	if h.active == nil {
		return nil
	}
	for c := range hs.claims {
		if c.state != stateDetached {
			return nil
		}
	}
	delete(h.shortcuts, id)
	return call(func() error { return h.b.Desktop().UnregisterShortcut(id) })
}

// shortcutStatus records the backend's status of a shortcut and sends it
// to the children holding it with the shortcut registered.
func (h *Host) shortcutStatus(e platform.ShortcutStatusEvent) {
	hs := h.shortcuts[e.ID]
	if hs == nil {
		return
	}
	hs.status = &e
	for _, c := range h.children {
		if s, held := hs.claims[c]; held && s == hs.reg && c.state != stateDetached {
			c.sendEvent(0, e)
		}
	}
}

// promoteShortcuts registers the shortcuts c, the new active child,
// changed while it was pending, and unregisters those no child holds.
func (h *Host) promoteShortcuts(c *Child) {
	for _, id := range sortedKeys(h.shortcuts) {
		hs := h.shortcuts[id]
		if s, held := hs.claims[c]; held && s != hs.reg {
			if err := h.reregister(hs, s, callSafe); err != nil {
				// c's RegisterShortcut returned nil: it is owed a status.
				c.sendEvent(0, platform.ShortcutStatusEvent{ID: id, Status: platform.ShortcutFailed, Reason: err.Error()})
			}
		}
		h.dropShortcut(id, hs)
	}
}

// dropShortcut prunes id, reporting a failure.
func (h *Host) dropShortcut(id string, hs *hostShortcut) {
	if err := h.pruneShortcut(id, hs, callSafe); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		h.reportError(fmt.Errorf("remote: unregistering shortcut %q: %w", id, err))
	}
}

// dropShortcutClaims drops the claims of c, which was detached or lost.
func (h *Host) dropShortcutClaims(c *Child) {
	for _, id := range sortedKeys(h.shortcuts) {
		hs := h.shortcuts[id]
		if _, held := hs.claims[c]; held {
			delete(hs.claims, c)
			h.dropShortcut(id, hs)
		}
	}
}

// holdsShortcut reports whether c holds the shortcut id.
func (h *Host) holdsShortcut(c *Child, id string) bool {
	hs := h.shortcuts[id]
	if hs == nil {
		return false
	}
	_, held := hs.claims[c]
	return held
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// --- host side: app ID ---

// setAppID serves a child's SetAppID: the host's backend registers the
// shortcuts it holds under the child app's ID, not the host's (the
// executable of tela dev). A replaced child's is dropped.
func (h *Host) setAppID(c *Child, id string, call func(func() error) error) error {
	if c.state == stateDetached {
		return nil
	}
	if err := try(func() { platform.CheckAppID("SetAppID", id) }); err != nil {
		return err
	}
	return call(func() error { h.b.Desktop().SetAppID(id); return nil })
}

// setStrings serves a child's SetStrings: the host's backend shows the
// child app's texts. A replaced child's are dropped.
func (h *Host) setStrings(c *Child, strs platform.Strings, call func(func() error) error) error {
	if c.state == stateDetached {
		return nil
	}
	return call(func() error { h.b.Desktop().SetStrings(strs); return nil })
}

// --- host side: single instance ---

// claimInstance serves a child's ClaimInstance (see the top of
// phase13.go).
func (h *Host) claimInstance(c *Child, appID string, launch platform.Launch, call func(func() error) error) (bool, error) {
	if h.instances[appID] {
		return true, nil
	}
	if c.state == stateDetached {
		return false, errReplaced
	}
	var primary bool
	if err := call(func() (err error) { primary, err = h.b.Desktop().ClaimInstance(appID, launch); return err }); err != nil {
		return false, err
	}
	if primary {
		h.instances[appID] = true
	} else if fn := h.opts.OnSecondInstance; fn != nil {
		fn(c, appID)
	}
	return primary, nil
}

// --- host side: blocking services (secrets, associations) ---

// blockingDesktop reports whether a desktop request is one the host
// serves off its UI thread: the secrets, associations and the trash, which
// block on the OS and are safe from any goroutine.
func blockingDesktop(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	switch payload[0] {
	case dSetSecret, dSecret, dDeleteSecret, dAssocStatus, dAssocRegister, dAssocUnregister, dAssocMakeDefault, dMoveToTrash,
		dAppRegister, dAppUnregister, dAppStatus:
		return true
	}
	return false
}

// blockingRequest serves a secret, association or trash call on a
// goroutine of its own and replies.
func (h *Host) blockingRequest(c *Child, in incoming) {
	d := &dec{b: in.payload}
	op, _ := d.u8(), d.u32()
	out := &enc{}
	var result func(*enc)
	err := callSafe(func() error { return h.serveBlocking(op, d, &result) })
	encodeOutcome(out, err)
	if err == nil && result != nil {
		result(out)
	}
	c.ctl.reply(in.id, out.b, nil)
}

func (h *Host) serveBlocking(op uint8, d *dec, result *func(*enc)) error {
	desk := h.desk
	switch op {
	case dMoveToTrash:
		path := d.str()
		if err := d.end(); err != nil {
			return err
		}
		return desk.MoveToTrash(path)
	case dSetSecret, dSecret, dDeleteSecret:
		appID, name := d.str(), d.str()
		var secret []byte
		if op == dSetSecret {
			secret = d.blob()
		}
		if err := d.end(); err != nil {
			return err
		}
		switch op {
		case dSetSecret:
			return desk.SetSecret(appID, name, secret)
		case dDeleteSecret:
			return desk.DeleteSecret(appID, name)
		}
		v, err := desk.Secret(appID, name)
		if v == nil {
			v = []byte{}
		}
		*result = func(e *enc) { e.optBytes(v) }
		return err
	case dAssocStatus, dAssocRegister, dAssocUnregister, dAssocMakeDefault:
		app, as := d.handlerApp(), d.association()
		var replace bool
		if op == dAssocRegister {
			replace = d.boolean()
		}
		if err := d.end(); err != nil {
			return err
		}
		a := desk.Associations()
		switch op {
		case dAssocStatus:
			st, err := a.Status(app, as)
			*result = func(e *enc) { e.boolean(st.Registered); e.boolean(st.Default); e.str(st.Other); e.u8(uint8(st.Role)) } // Role: version 15
			return err
		case dAssocRegister, dAssocUnregister:
			var changed bool
			var err error
			if op == dAssocRegister {
				changed, err = a.Register(app, as, platform.RegisterOptions{Replace: replace})
			} else {
				changed, err = a.Unregister(app, as)
			}
			*result = func(e *enc) { e.boolean(changed) }
			return err
		}
		res, err := a.MakeDefault(app, as)
		*result = func(e *enc) { e.u8(uint8(res)) }
		return err
	case dAppRegister, dAppUnregister, dAppStatus:
		app := d.handlerApp()
		var replace bool
		if op == dAppRegister {
			replace = d.boolean()
		}
		if err := d.end(); err != nil {
			return err
		}
		a := desk.Associations()
		switch op {
		case dAppStatus:
			st, err := a.AppStatus(app)
			*result = func(e *enc) { e.boolean(st.Registered); e.boolean(st.Requested); e.str(st.Other) }
			return err
		case dAppRegister:
			changed, err := a.RegisterApp(app, platform.RegisterOptions{Replace: replace})
			*result = func(e *enc) { e.boolean(changed) }
			return err
		}
		changed, err := a.UnregisterApp(app)
		*result = func(e *enc) { e.boolean(changed) }
		return err
	}
	return fmt.Errorf("remote: unknown desktop operation %d", op)
}

// --- host side: clipboard ---

// clipWriteData writes a child's data to the real clipboard; a replaced
// child's write is dropped.
func (h *Host) clipWriteData(c *Child, data platform.Data, call func(func() error) error) error {
	if c.state == stateDetached {
		return nil
	}
	return call(func() error { return h.b.Clipboard().Write(data) })
}

// --- shared ---

// hostError is an error the host reported, keeping its text and the
// platform sentinel it wraps across the connection.
type hostError struct {
	text     string
	sentinel error
}

func (e *hostError) Error() string { return e.text }
func (e *hostError) Unwrap() error { return e.sentinel }

// secretTimeout is how long a child waits for a secret call: no limit
// (the OS may show an unlock prompt), only the end of the connection.
const secretTimeout time.Duration = 0
