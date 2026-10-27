package remote

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Desktop services (phase 12, ADR-179, ADR-184).
//
// A child's Desktop forwards every call to the host's real Desktop over
// the control channel, with two kinds this package reserves:
// kindDesktop, a request (op, token, arguments) answered at once with the
// outcome of the call (an error code: ErrUnsupported crosses the
// connection as itself), and kindDesktopDone, a notification from the
// host with the answer of a dialog or message box, which runs its done
// callback on the child's UI thread through the loop. Arguments are
// checked in the child first, so programmer errors panic in the app as on
// every backend. Dialogs are answered to the child that asked.
//
// Tray icons and notifications are the host's, held for its children:
// their IDs are the real backend's, the same in every child, so a build
// that keeps a tray ID in its state can go on updating it. Each child
// holds claims on them:
//
//   - A child that is not active yet (a reload starting) takes over the
//     active child's tray icons instead of adding new ones: its k-th
//     NewTray claims the active child's first tray it has not claimed,
//     in creation order, and returns its ID. An UpdateTray of a tray it
//     does not hold (an ID kept in its state) claims it too. Its options
//     reach the real tray when it is promoted; the active child's trays
//     it did not claim are removed then. A tray no live child claims is
//     removed, so a reload neither duplicates nor drops an icon, and a
//     build that no longer adds a tray removes it.
//   - Likewise its Notify returns the active child's live notification
//     with the same title and body, if it has one it has not claimed,
//     instead of showing it again (an app that notifies when it starts
//     does not notify at every reload). Notifications are never closed
//     by a swap.
//   - A detached child (replaced, being stopped) can add nothing, and
//     its RemoveTray and CloseNotification no longer affect what the
//     new build holds (its effects' cleanups run at its quit).
//
// NotificationEvent and TrayEvent go to the active child (held while
// input is frozen, like input).

// Control kinds reserved by this package.
const (
	kindDesktop     = "tela.remote.desktop"
	kindDesktopDone = "tela.remote.desktop.done"
)

// Desktop operations.
const (
	dOpenURI uint8 = iota + 1
	dOpenPath
	dOpenFiles
	dSaveFile
	dPickFolder
	dMessageBox
	dNotify
	dCloseNotification
	dNewTray
	dUpdateTray
	dRemoveTray
)

// Outcome codes of a desktop call.
const (
	codeOK uint8 = iota
	codeUnsupported
	codeError
)

// desktopTimeout bounds the wait for the host's answer to a synchronous
// desktop call (the host answers from its UI thread).
const desktopTimeout = 10 * time.Second

// errReplaced is the error of a service a detached child asks for.
var errReplaced = errors.New("remote: this build was replaced by a newer one")

// --- child side ---

// Desktop returns the host's desktop services (see the comment at the top
// of services.go).
func (b *Backend) Desktop() platform.Desktop { return childDesktop{b} }

type childDesktop struct{ b *Backend }

var _ platform.Desktop = childDesktop{}

// windowID returns the ID of w if it is an open window of this backend,
// else 0 (no parent).
func (d childDesktop) windowID(w platform.Window) uint32 {
	cw, ok := w.(*Window)
	if !ok || cw == nil || cw.b != d.b || !cw.open() {
		return 0
	}
	return cw.id
}

// call sends a desktop request and waits for its outcome; the returned
// decoder holds the results that follow it.
func (d childDesktop) call(op uint8, token uint32, fill func(*enc)) (*dec, error) {
	e := &enc{}
	e.u8(op)
	e.u32(token)
	if fill != nil {
		fill(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), desktopTimeout)
	defer cancel()
	p, err := d.b.ctl.request(ctx, kindDesktop, e.b)
	if err != nil {
		return nil, err
	}
	r := &dec{b: p}
	if err := outcome(r); err != nil {
		return nil, err
	}
	return r, nil
}

// outcome reads an outcome code and text.
func outcome(r *dec) error {
	code, text := r.u8(), r.str()
	if r.err != nil {
		return r.err
	}
	switch code {
	case codeOK:
		return nil
	case codeUnsupported:
		return platform.ErrUnsupported
	}
	return errors.New(text)
}

// dialog starts an asynchronous call: answer decodes the host's final
// answer and runs on the UI thread.
func (d childDesktop) dialog(op uint8, fill func(*enc), answer func(r *dec, err error)) error {
	b := d.b
	b.mu.Lock()
	b.nextDialog++
	token := b.nextDialog
	b.dialogs[token] = answer
	b.mu.Unlock()
	if _, err := d.call(op, token, fill); err != nil {
		b.mu.Lock()
		delete(b.dialogs, token)
		b.mu.Unlock()
		return err
	}
	return nil
}

// desktopDone runs the done callback of a dialog the host answered (UI
// thread).
func (b *Backend) desktopDone(payload []byte) {
	r := &dec{b: payload}
	token := r.u32()
	err := outcome(r)
	b.mu.Lock()
	fn := b.dialogs[token]
	delete(b.dialogs, token)
	b.mu.Unlock()
	if fn != nil {
		fn(r, err)
	}
}

func (d childDesktop) OpenURI(parent platform.Window, uri string) error {
	if err := platform.CheckURI(uri); err != nil {
		return err
	}
	id := d.windowID(parent)
	_, err := d.call(dOpenURI, 0, func(e *enc) { e.u32(id); e.str(uri) })
	return err
}

func (d childDesktop) OpenPath(parent platform.Window, path string) error {
	if path == "" {
		return errors.New("remote: OpenPath with an empty path")
	}
	id := d.windowID(parent)
	_, err := d.call(dOpenPath, 0, func(e *enc) { e.u32(id); e.str(path) })
	return err
}

func (d childDesktop) OpenFiles(opts platform.FileDialogOptions, done func([]string, error)) error {
	if done == nil {
		panic("remote: OpenFiles called with a nil callback")
	}
	platform.CheckFileDialogOptions("OpenFiles", opts)
	id := d.windowID(opts.Parent)
	return d.dialog(dOpenFiles, func(e *enc) { e.fileDialog(opts, id) }, func(r *dec, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		paths := r.strs()
		if err := r.end(); err != nil {
			done(nil, err)
			return
		}
		done(paths, nil)
	})
}

func (d childDesktop) SaveFile(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("remote: SaveFile called with a nil callback")
	}
	platform.CheckFileDialogOptions("SaveFile", opts)
	id := d.windowID(opts.Parent)
	return d.dialog(dSaveFile, func(e *enc) { e.fileDialog(opts, id) }, pathAnswer(done))
}

func (d childDesktop) PickFolder(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("remote: PickFolder called with a nil callback")
	}
	platform.CheckFileDialogOptions("PickFolder", opts)
	id := d.windowID(opts.Parent)
	return d.dialog(dPickFolder, func(e *enc) { e.fileDialog(opts, id) }, pathAnswer(done))
}

func pathAnswer(done func(string, error)) func(*dec, error) {
	return func(r *dec, err error) {
		if err != nil {
			done("", err)
			return
		}
		path := r.str()
		if err := r.end(); err != nil {
			done("", err)
			return
		}
		done(path, nil)
	}
}

func (d childDesktop) MessageBox(opts platform.MessageBoxOptions, done func(platform.MessageAnswer, error)) error {
	if done == nil {
		panic("remote: MessageBox called with a nil callback")
	}
	platform.CheckMessageBoxOptions(opts)
	id := d.windowID(opts.Parent)
	return d.dialog(dMessageBox, func(e *enc) { e.messageBox(opts, id) }, func(r *dec, err error) {
		if err != nil {
			done(opts.Buttons.DismissAnswer(), err)
			return
		}
		a := platform.MessageAnswer(r.u8())
		if err := r.end(); err != nil {
			done(opts.Buttons.DismissAnswer(), err)
			return
		}
		done(a, nil)
	})
}

func (d childDesktop) Notify(n platform.Notification) (platform.NotificationID, error) {
	platform.CheckNotification(n)
	r, err := d.call(dNotify, 0, func(e *enc) { e.notification(n) })
	if err != nil {
		return 0, err
	}
	id := platform.NotificationID(r.u32())
	return id, r.end()
}

func (d childDesktop) CloseNotification(id platform.NotificationID) error {
	_, err := d.call(dCloseNotification, 0, func(e *enc) { e.u32(uint32(id)) })
	return err
}

func (d childDesktop) NewTray(opts platform.TrayOptions) (platform.TrayID, error) {
	platform.CheckTrayOptions(opts)
	r, err := d.call(dNewTray, 0, func(e *enc) { e.tray(opts) })
	if err != nil {
		return 0, err
	}
	id := platform.TrayID(r.u32())
	return id, r.end()
}

func (d childDesktop) UpdateTray(id platform.TrayID, opts platform.TrayOptions) error {
	platform.CheckTrayOptions(opts)
	_, err := d.call(dUpdateTray, 0, func(e *enc) { e.u32(uint32(id)); e.tray(opts) })
	return err
}

func (d childDesktop) RemoveTray(id platform.TrayID) error {
	_, err := d.call(dRemoveTray, 0, func(e *enc) { e.u32(uint32(id)) })
	return err
}

// --- host side ---

// hostTray is a real tray icon and the children that hold it.
type hostTray struct {
	id     platform.TrayID
	opts   platform.TrayOptions // applied to the real tray
	claims map[*Child]platform.TrayOptions
}

// hostNote is a real notification and the children that hold it.
type hostNote struct {
	id          platform.NotificationID
	title, body string
	claims      map[*Child]bool
}

// encodeOutcome writes err as an outcome code and text.
func encodeOutcome(e *enc, err error) {
	switch {
	case err == nil:
		e.u8(codeOK)
		e.str("")
	case errors.Is(err, errors.ErrUnsupported):
		e.u8(codeUnsupported)
		e.str(err.Error())
	default:
		e.u8(codeError)
		e.str(err.Error())
	}
}

// parentOf returns the real window of c's window id while c's window is
// the one shown there (else nil: no parent).
func (h *Host) parentOf(c *Child, id uint32) platform.Window {
	if id == 0 {
		return nil
	}
	if cw := c.win(id); cw != nil && cw.isActive() {
		return cw.rw.w
	}
	return nil
}

// desktopRequest serves a child's desktop call on the UI thread and
// replies with its outcome.
func (h *Host) desktopRequest(c *Child, in incoming) {
	d := &dec{b: in.payload}
	op, token := d.u8(), d.u32()
	out := &enc{}
	var result func(*enc)
	err := h.serveDesktop(c, op, token, d, &result)
	encodeOutcome(out, err)
	if err == nil && result != nil {
		result(out)
	}
	c.ctl.reply(in.id, out.b, nil)
}

// serveDesktop performs one desktop call; result, if set, writes its
// results after the outcome.
func (h *Host) serveDesktop(c *Child, op uint8, token uint32, d *dec, result *func(*enc)) error {
	desk := h.b.Desktop()
	// The real desktop panics on arguments the child already checked: a
	// panic here is a corrupt peer, answered as an error.
	call := func(f func() error) (err error) {
		if perr := try(func() { err = f() }); perr != nil {
			return perr
		}
		return err
	}
	// answer sends the final answer of a dialog to the child that asked
	// (without waiting for the socket: the UI thread never blocks on a
	// child; a child that is gone waits for nothing).
	answer := func(err error, fill func(e *enc)) {
		e := &enc{}
		e.u32(token)
		encodeOutcome(e, err)
		if err == nil {
			fill(e)
		}
		c.ctl.post(kindDesktopDone, e.b)
	}
	switch op {
	case dOpenURI, dOpenPath:
		parent, target := d.u32(), d.str()
		if err := d.end(); err != nil {
			return err
		}
		if op == dOpenURI {
			return call(func() error { return desk.OpenURI(h.parentOf(c, parent), target) })
		}
		return call(func() error { return desk.OpenPath(h.parentOf(c, parent), target) })
	case dOpenFiles, dSaveFile, dPickFolder:
		opts, parent := d.fileDialog()
		if err := d.end(); err != nil {
			return err
		}
		if c.state == stateDetached {
			return errReplaced
		}
		opts.Parent = h.parentOf(c, parent)
		if op == dOpenFiles {
			return call(func() error {
				return desk.OpenFiles(opts, func(paths []string, err error) {
					answer(err, func(e *enc) { e.strs(paths) })
				})
			})
		}
		pick := desk.SaveFile
		if op == dPickFolder {
			pick = desk.PickFolder
		}
		return call(func() error {
			return pick(opts, func(path string, err error) {
				answer(err, func(e *enc) { e.str(path) })
			})
		})
	case dMessageBox:
		opts, parent := d.messageBox()
		if err := d.end(); err != nil {
			return err
		}
		if c.state == stateDetached {
			return errReplaced
		}
		opts.Parent = h.parentOf(c, parent)
		return call(func() error {
			return desk.MessageBox(opts, func(a platform.MessageAnswer, err error) {
				answer(err, func(e *enc) { e.u8(uint8(a)) })
			})
		})
	case dNotify:
		n := d.notification()
		if err := d.end(); err != nil {
			return err
		}
		id, err := h.notify(c, n, call)
		*result = func(e *enc) { e.u32(uint32(id)) }
		return err
	case dCloseNotification:
		id := platform.NotificationID(d.u32())
		if err := d.end(); err != nil {
			return err
		}
		return h.closeNote(c, id, call)
	case dNewTray:
		opts := d.tray()
		if err := d.end(); err != nil {
			return err
		}
		id, err := h.newTray(c, opts, call)
		*result = func(e *enc) { e.u32(uint32(id)) }
		return err
	case dUpdateTray:
		id := platform.TrayID(d.u32())
		opts := d.tray()
		if err := d.end(); err != nil {
			return err
		}
		return h.updateTray(c, id, opts, call)
	case dRemoveTray:
		id := platform.TrayID(d.u32())
		if err := d.end(); err != nil {
			return err
		}
		return h.removeTray(c, id, call)
	}
	return fmt.Errorf("remote: unknown desktop operation %d", op)
}

// notify shows a notification for c, or gives a starting build the
// active child's equal one.
func (h *Host) notify(c *Child, n platform.Notification, call func(func() error) error) (platform.NotificationID, error) {
	if c.state == stateDetached {
		return 0, errReplaced
	}
	if c.state == statePending && h.active != nil {
		for _, id := range h.active.notes {
			hn := h.notes[id]
			if hn != nil && !hn.claims[c] && hn.title == n.Title && hn.body == n.Body {
				hn.claims[c] = true
				c.notes = append(c.notes, id)
				return id, nil
			}
		}
	}
	var id platform.NotificationID
	if err := call(func() (err error) { id, err = h.b.Desktop().Notify(n); return err }); err != nil {
		return 0, err
	}
	h.notes[id] = &hostNote{id: id, title: n.Title, body: n.Body, claims: map[*Child]bool{c: true}}
	c.notes = append(c.notes, id)
	return id, nil
}

// closeNote withdraws a notification unless a live child other than c
// holds it.
func (h *Host) closeNote(c *Child, id platform.NotificationID, call func(func() error) error) error {
	hn := h.notes[id]
	if hn == nil {
		return nil
	}
	delete(hn.claims, c)
	c.notes = slices.DeleteFunc(c.notes, func(x platform.NotificationID) bool { return x == id })
	for o := range hn.claims {
		if o.state != stateDetached {
			return nil
		}
	}
	delete(h.notes, id)
	return call(func() error { return h.b.Desktop().CloseNotification(id) })
}

// noteClosed forgets a notification the OS closed.
func (h *Host) noteClosed(id platform.NotificationID) {
	delete(h.notes, id)
	for _, c := range h.children {
		c.notes = slices.DeleteFunc(c.notes, func(x platform.NotificationID) bool { return x == id })
	}
}

// newTray adds a tray icon for c, or gives a starting build the active
// child's next tray.
func (h *Host) newTray(c *Child, opts platform.TrayOptions, call func(func() error) error) (platform.TrayID, error) {
	if c.state == stateDetached {
		return 0, errReplaced
	}
	if c.state == statePending && h.active != nil {
		for _, id := range h.active.trays {
			t := h.trays[id]
			if t == nil {
				continue
			}
			if _, held := t.claims[c]; !held {
				t.claims[c] = opts
				c.trays = append(c.trays, id)
				return id, nil
			}
		}
	}
	var id platform.TrayID
	if err := call(func() (err error) { id, err = h.b.Desktop().NewTray(opts); return err }); err != nil {
		return 0, err
	}
	h.trays[id] = &hostTray{id: id, opts: opts, claims: map[*Child]platform.TrayOptions{c: opts}}
	c.trays = append(c.trays, id)
	return id, nil
}

// updateTray records c's options for a tray (claiming it) and applies them
// when c is the active child.
func (h *Host) updateTray(c *Child, id platform.TrayID, opts platform.TrayOptions, call func(func() error) error) error {
	t := h.trays[id]
	if t == nil {
		return fmt.Errorf("remote: UpdateTray: tray %d does not exist", id)
	}
	if c.state == stateDetached {
		return nil
	}
	if _, held := t.claims[c]; !held {
		c.trays = append(c.trays, id)
	}
	t.claims[c] = opts
	if c.state != stateActive || equalTray(t.opts, opts) {
		return nil
	}
	t.opts = opts
	return call(func() error { return h.b.Desktop().UpdateTray(id, opts) })
}

// removeTray drops c's claim on a tray; the tray goes away when no child
// holds it any more.
func (h *Host) removeTray(c *Child, id platform.TrayID, call func(func() error) error) error {
	t := h.trays[id]
	if t == nil {
		return nil
	}
	if _, held := t.claims[c]; !held {
		return nil
	}
	delete(t.claims, c)
	c.trays = slices.DeleteFunc(c.trays, func(x platform.TrayID) bool { return x == id })
	if len(t.claims) > 0 {
		return nil
	}
	delete(h.trays, id)
	return call(func() error { return h.b.Desktop().RemoveTray(id) })
}

// promoteServices gives the real trays the options of c, the new active
// child.
func (h *Host) promoteServices(c *Child) {
	for _, id := range c.trays {
		t := h.trays[id]
		if t == nil {
			continue
		}
		opts, held := t.claims[c]
		if !held || equalTray(t.opts, opts) {
			continue
		}
		t.opts = opts
		if err := try(func() {
			if err := h.b.Desktop().UpdateTray(id, opts); err != nil {
				h.reportError(fmt.Errorf("remote: updating tray %d: %w", id, err))
			}
		}); err != nil {
			h.reportError(err)
		}
	}
}

// dropServices drops the claims of c, which was detached or lost: trays no
// child holds any more are removed (notifications stay on screen).
func (h *Host) dropServices(c *Child) {
	for _, id := range c.trays {
		t := h.trays[id]
		if t == nil {
			continue
		}
		delete(t.claims, c)
		if len(t.claims) == 0 {
			delete(h.trays, id)
			if err := try(func() {
				if err := h.b.Desktop().RemoveTray(id); err != nil {
					h.reportError(fmt.Errorf("remote: removing tray %d: %w", id, err))
				}
			}); err != nil {
				h.reportError(err)
			}
		}
	}
	for _, id := range c.notes {
		if hn := h.notes[id]; hn != nil {
			delete(hn.claims, c)
		}
	}
	c.trays, c.notes = nil, nil
}

// equalTray reports whether two decoded tray options are the same.
func equalTray(a, b platform.TrayOptions) bool {
	return a.Tooltip == b.Tooltip && equalIcons(a.Icon, b.Icon) && reflect.DeepEqual(a.Menu, b.Menu)
}
