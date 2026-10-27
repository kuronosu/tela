//go:build windows

package win32

import (
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// Desktop services (phase 12, ADR-178): ShellExecuteW for OpenURI and
// OpenPath (shell_windows.go), the common item dialogs over COM and
// MessageBoxW (dialog_windows.go), Shell_NotifyIconW for tray icons and
// balloon notifications (tray_windows.go).
//
// Dialogs and message boxes run a modal message loop. The app asks for
// them from its handler, inside a delivery pass, where a modal loop would
// block delivery until the dialog closed. So a request is queued and the
// message-only window is posted a private message; the dialog starts when
// that message is dispatched, outside the pass (by Run's loop, or by the
// modal loop of a dialog already open: dialogs are shown one at a time, in
// request order). While it is open, its modal loop dispatches the
// backend's messages, and WndProc runs the delivery passes. The answer is
// posted to the loop and delivered by the pass that follows.
//
// Tray icons, notifications and their menus belong to a hidden top-level
// window of the backend (the tray window): TaskbarCreated, which asks the
// icons to be added again after Explorer restarts, is broadcast to
// top-level windows only, and TrackPopupMenuEx needs a window that can be
// the foreground window, which a message-only window cannot.

// Private messages of the desktop services, apart from the backend's own
// (WM_APP + 1 and 2) so that both can grow.
const (
	wmTelaModal    = 0x8000 + 0x40 // to the message-only window: start the next dialog
	wmTelaTray     = 0x8000 + 0x41 // to the tray window: callback of a notification area icon
	wmTelaTrayMenu = 0x8000 + 0x42 // to the tray window: a tray menu deferred out of a pass
)

// desktopState is the state of the desktop services, on the UI thread.
type desktopState struct {
	// COM (com_windows.go).
	comDone bool  // initCOM ran
	comInit bool  // CoInitializeEx succeeded and needs CoUninitialize
	comErr  error // why COM is unusable

	// Dialogs and message boxes, shown one at a time.
	modal        []func() // waiting, in request order
	modalPosted  bool     // a wmTelaModal is in the queue
	modalRunning bool     // a dialog is open
	modalLater   bool     // wmTelaModal arrived during a pass: post it again after
	dialog       comObject
	// later holds the tray menus that were asked for during a pass, posted
	// again when it ends.
	later []postedMessage

	// Tray icons and notifications (tray_windows.go).
	hwnd           uintptr // the tray window, created on first use
	taskbarCreated uint32  // the registered TaskbarCreated message
	ids            iconIDs
	icons          map[uint16]*shellIcon // by icon ID
	lastTray       platform.TrayID
	lastNote       platform.NotificationID
	appIcon        uintptr // the executable's small icon, for notifications
	appIconOwned   bool    // appIcon was extracted (DestroyIcon), not a shared system icon
}

// postedMessage is a message to post later.
type postedMessage struct {
	hwnd           uintptr
	msg            uint32
	wparam, lparam uintptr
}

// Desktop returns the desktop services; see the package documentation of
// platform and the comment at the top of this file.
func (b *Backend) Desktop() platform.Desktop { return desktop{b} }

// desktop implements platform.Desktop for a Backend.
type desktop struct{ b *Backend }

var _ platform.Desktop = desktop{}

func (d desktop) OpenURI(parent platform.Window, uri string) error {
	if err := checkShellURI(uri); err != nil {
		return err
	}
	if d.b.closed {
		return errClosed
	}
	return d.b.shellOpen(parent, uri)
}

func (d desktop) OpenPath(parent platform.Window, path string) error {
	if path == "" || strings.ContainsRune(path, 0) {
		return fmt.Errorf("win32: OpenPath(%q): not a path", path)
	}
	if d.b.closed {
		return errClosed
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("win32: OpenPath(%q): %w", path, err)
	}
	return d.b.shellOpen(parent, abs)
}

func (d desktop) OpenFiles(opts platform.FileDialogOptions, done func([]string, error)) error {
	if done == nil {
		panic("win32: OpenFiles called with a nil callback; pass a func(paths []string, err error)")
	}
	platform.CheckFileDialogOptions("OpenFiles", opts)
	return d.b.requestFileDialog(dialogOpenFiles, opts, func(paths []string, err error) { done(paths, err) })
}

func (d desktop) SaveFile(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("win32: SaveFile called with a nil callback; pass a func(path string, err error)")
	}
	platform.CheckFileDialogOptions("SaveFile", opts)
	return d.b.requestFileDialog(dialogSaveFile, opts, func(paths []string, err error) { done(firstPath(paths), err) })
}

func (d desktop) PickFolder(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("win32: PickFolder called with a nil callback; pass a func(path string, err error)")
	}
	platform.CheckFileDialogOptions("PickFolder", opts)
	return d.b.requestFileDialog(dialogPickFolder, opts, func(paths []string, err error) { done(firstPath(paths), err) })
}

func firstPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func (d desktop) MessageBox(opts platform.MessageBoxOptions, done func(platform.MessageAnswer, error)) error {
	if done == nil {
		panic("win32: MessageBox called with a nil callback; pass a func(answer platform.MessageAnswer, err error)")
	}
	platform.CheckMessageBoxOptions(opts)
	b := d.b
	if b.closed {
		return errClosed
	}
	b.queueModal(func() {
		answer, err := b.showMessageBox(opts)
		b.answer(func() { done(answer, err) })
	})
	return nil
}

func (d desktop) Notify(n platform.Notification) (platform.NotificationID, error) {
	platform.CheckNotification(n)
	if d.b.closed {
		return 0, errClosed
	}
	return d.b.showNotification(n)
}

func (d desktop) CloseNotification(id platform.NotificationID) error {
	if ic := d.b.findIcon(func(ic *shellIcon) bool { return ic.note == id && id != 0 }); ic != nil {
		d.b.deleteIcon(ic)
	}
	return nil
}

func (d desktop) NewTray(opts platform.TrayOptions) (platform.TrayID, error) {
	platform.CheckTrayOptions(opts)
	if d.b.closed {
		return 0, errClosed
	}
	return d.b.newTray(opts)
}

func (d desktop) UpdateTray(id platform.TrayID, opts platform.TrayOptions) error {
	platform.CheckTrayOptions(opts)
	ic := d.b.findIcon(func(ic *shellIcon) bool { return ic.tray == id && id != 0 })
	if ic == nil {
		return fmt.Errorf("win32: UpdateTray: tray %d does not exist (it was removed, or Run returned)", id)
	}
	return d.b.updateTray(ic, opts)
}

func (d desktop) RemoveTray(id platform.TrayID) error {
	if ic := d.b.findIcon(func(ic *shellIcon) bool { return ic.tray == id && id != 0 }); ic != nil {
		d.b.deleteIcon(ic)
	}
	return nil
}

// requestFileDialog queues a common item dialog; done gets its paths
// (nil when canceled).
func (b *Backend) requestFileDialog(kind fileDialogKind, opts platform.FileDialogOptions, done func([]string, error)) error {
	if b.closed {
		return errClosed
	}
	// COM is joined now, so that a thread that cannot have the dialogs
	// fails the request instead of answering it with an error later.
	if err := b.desk.initCOM(); err != nil {
		return err
	}
	opts.Filters = slices.Clone(opts.Filters)
	for i := range opts.Filters {
		opts.Filters[i].Patterns = slices.Clone(opts.Filters[i].Patterns)
	}
	b.queueModal(func() {
		paths, err := b.showFileDialog(kind, opts)
		b.answer(func() { done(paths, err) })
	})
	return nil
}

// answer posts a dialog's answer to the loop; the pass that runs after the
// dialog closes delivers it.
func (b *Backend) answer(fn func()) {
	b.dirty = true
	b.loop.Post(fn)
}

// queueModal queues a dialog or message box and asks for it to be started
// outside the current delivery pass.
func (b *Backend) queueModal(show func()) {
	b.desk.modal = append(b.desk.modal, show)
	b.postModal()
}

// postModal posts the message that starts the next dialog, once.
func (b *Backend) postModal() {
	d := &b.desk
	if !d.modalPosted && !b.stopped.Load() {
		d.modalPosted = postMessage(b.msgHwnd, wmTelaModal, 0, 0)
	}
}

// runModal shows the next queued dialog, unless one is open (it starts the
// next when it closes) or a delivery pass is running, which a modal loop
// would block until the dialog closed (a pass cannot be running here
// unless something inside it ran a message loop; the message is then
// posted again when that pass ends).
func (b *Backend) runModal() {
	d := &b.desk
	d.modalPosted = false
	switch {
	case d.modalRunning || len(d.modal) == 0:
		return
	case b.pumping:
		d.modalLater = true
		return
	}
	show := d.modal[0]
	d.modal = slices.Delete(d.modal, 0, 1)
	func() {
		d.modalRunning = true
		defer func() { d.modalRunning = false }()
		show()
	}()
	if len(d.modal) > 0 {
		b.postModal()
	}
	b.pump() // the answer
}

// desktopPassEnded runs at the end of every delivery pass: it posts again
// the work that had to wait for the pass to end, and closes an open file
// dialog when the app quits (its modal loop would otherwise keep Run from
// returning until the user closed it; MessageBoxW and menus end on the
// WM_QUIT that Quit posts).
func (b *Backend) desktopPassEnded() {
	d := &b.desk
	if b.loop.Quitting() && d.dialog != 0 {
		d.dialog.call(vtFileDialogClose, 0x800704C7) // the HRESULT that Show returns: hrCanceled
	}
	if d.modalLater {
		d.modalLater = false
		b.postModal()
	}
	for _, m := range d.later {
		postMessage(m.hwnd, m.msg, m.wparam, m.lparam)
	}
	d.later = d.later[:0]
}

// desktopMessage handles the messages of the desktop services that reach
// the backend's own windows (the message-only window and the tray window);
// ok is false for any other.
func (b *Backend) desktopMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) (ret uintptr, ok bool) {
	d := &b.desk
	switch {
	case hwnd == b.msgHwnd && msg == wmTelaModal:
		b.runModal()
	case hwnd != d.hwnd || hwnd == 0:
		return 0, false
	case msg == wmTelaTray:
		b.onTrayCallback(wparam, lparam)
	case msg == wmTelaTrayMenu:
		if ic := d.icons[uint16(wparam)]; ic != nil {
			x, y := pointFromLParam(lparam)
			b.trayMenu(ic, x, y)
		}
	case msg == d.taskbarCreated && msg != 0:
		b.taskbarRestarted()
	case msg == wmClose:
		// The tray window lives until Run returns.
	default:
		return 0, false
	}
	return 0, true
}

// desktopShutdown releases the desktop services when Run returns: the
// icons leave the notification area and waiting dialogs are dropped.
func (b *Backend) desktopShutdown() {
	d := &b.desk
	d.modal, d.later = nil, nil
	for _, ic := range b.sortedIcons() {
		b.deleteIcon(ic)
	}
	if d.hwnd != 0 {
		registry.mu.Lock()
		delete(registry.backends, d.hwnd)
		registry.mu.Unlock()
		procDestroyWindow.Call(d.hwnd)
		d.hwnd = 0
	}
	if d.appIconOwned {
		procDestroyIcon.Call(d.appIcon)
	}
	d.appIcon, d.appIconOwned = 0, false
	d.closeCOM()
}

// ownerOf returns the handle of the window a request names as its parent,
// or 0 for none (nil, closed, or a window of another backend).
func ownerOf(w platform.Window) uintptr {
	if win, ok := w.(*Window); ok && win != nil && !win.Closed() {
		return win.hwnd
	}
	return 0
}

// errNoNotificationArea is returned when Explorer does not take an icon.
var errNoNotificationArea = errors.New("win32: the notification area did not accept the icon (is Explorer running?)")

// shellIcon is an icon of the notification area: a tray icon (tray set) or
// the icon that carries a notification's balloon (note set).
type shellIcon struct {
	uid  uint16
	tray platform.TrayID
	note platform.NotificationID

	tip    string
	images []image.Image       // tray: the icon at its sizes (a copy)
	menu   []platform.MenuItem // tray: the menu (a copy)
	hicon  uintptr             // the icon shown
	owned  bool                // hicon was created for it (DestroyIcon)
	big    uintptr             // note: the balloon's large icon, or 0
	added  bool                // the notification area has it
}
