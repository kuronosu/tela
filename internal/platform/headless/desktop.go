package headless

import (
	"errors"
	"fmt"
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// Service names a group of desktop services, for Backend.SetUnsupported.
type Service uint8

const (
	ServiceOpen          Service = iota // OpenURI, OpenPath
	ServiceFileDialogs                  // OpenFiles, SaveFile, PickFolder
	ServiceMessageBox                   // MessageBox (unsupported on Linux backends)
	ServiceNotifications                // Notify, CloseNotification
	ServiceTray                         // NewTray, UpdateTray, RemoveTray
	serviceCount
)

// DialogKind is the kind of a Dialog.
type DialogKind uint8

const (
	DialogOpenFiles DialogKind = iota
	DialogSaveFile
	DialogPickFolder
	DialogMessageBox
)

var dialogKindNames = [...]string{"OpenFiles", "SaveFile", "PickFolder", "MessageBox"}

func (k DialogKind) String() string {
	if int(k) < len(dialogKindNames) {
		return dialogKindNames[k]
	}
	return fmt.Sprintf("DialogKind(%d)", uint8(k))
}

// Dialog is a file dialog or message box the app asked for.
type Dialog struct {
	// ID numbers the dialogs of a backend from 1, in request order.
	ID   int
	Kind DialogKind
	// File holds the options of the file dialogs, Message those of a
	// message box.
	File    platform.FileDialogOptions
	Message platform.MessageBoxOptions
}

// DialogResponse is the user's answer to a Dialog.
type DialogResponse struct {
	// Paths are the chosen paths: any number for an OpenFiles dialog with
	// Multiple, at most one otherwise. None means the user canceled.
	Paths []string
	// Answer is the chosen button of a message box; it must be one of its
	// buttons (platform.MessageButtons.Has; DismissAnswer for a box
	// closed without a button).
	Answer platform.MessageAnswer
	// Err, if set, is reported instead (a failed dialog).
	Err error
}

// OpenRequest is a recorded OpenURI or OpenPath call.
type OpenRequest struct {
	Parent platform.Window
	// Target is the URI, or the path when Path is set.
	Target string
	Path   bool
}

// NotificationRecord is a notification shown with Notify.
type NotificationRecord struct {
	ID           platform.NotificationID
	Notification platform.Notification
	// Closed is set by CloseNotification or an injected NotificationClosed.
	Closed bool
}

// TrayRecord is a tray icon added with NewTray and not removed.
type TrayRecord struct {
	ID      platform.TrayID
	Options platform.TrayOptions
	// Updates counts the UpdateTray calls.
	Updates int
}

// desktopState is the headless desktop, guarded by b.mu.
type desktopState struct {
	unsupported   [serviceCount]bool
	responder     func(Dialog) DialogResponse
	openResponder func(OpenRequest) error
	dialogs       []Dialog
	open          map[int]func(DialogResponse) // unanswered dialogs
	opened        []OpenRequest
	notifications []NotificationRecord
	trays         []TrayRecord
	lastTray      platform.TrayID
}

// Desktop returns the headless desktop services: nothing is shown; every
// request is recorded for inspection (Dialogs, Opened, Notifications,
// Trays), dialogs are answered by the dialog responder or AnswerDialog,
// and notification and tray events are injected with InjectApp. Services
// marked with SetUnsupported return platform.ErrUnsupported, which is how
// tests reach the fallbacks of the layers above (core's drawn message
// box). Arguments are checked like on every backend.
func (b *Backend) Desktop() platform.Desktop { return desktop{b} }

// SetUnsupported makes the given services return platform.ErrUnsupported
// (and every other one supported). Safe from any goroutine.
func (b *Backend) SetUnsupported(services ...Service) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.unsupported = [serviceCount]bool{}
	for _, s := range services {
		if s >= serviceCount {
			panic(fmt.Sprintf("headless: SetUnsupported(%d): invalid service", s))
		}
		b.desktop.unsupported[s] = true
	}
}

// SetDialogResponder sets the function that answers the dialogs and
// message boxes requested from then on, as the user would: it is called
// on the UI thread inside the request (without the backend's lock held),
// and its response reaches the app's callback through the loop, never from
// inside the request. nil (the default) leaves dialogs open until
// AnswerDialog. A response that no real dialog could give (two paths from
// a single-file dialog, an answer that is not one of the box's buttons)
// panics. Safe from any goroutine.
func (b *Backend) SetDialogResponder(fn func(Dialog) DialogResponse) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.responder = fn
}

// SetOpenResponder sets the function that answers OpenURI and OpenPath
// (after their arguments were checked), for tests of failures; nil (the
// default) accepts every request. Safe from any goroutine.
func (b *Backend) SetOpenResponder(fn func(OpenRequest) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.openResponder = fn
}

// Dialogs returns every dialog requested so far, in request order.
func (b *Backend) Dialogs() []Dialog {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.dialogs)
}

// OpenDialogs returns the dialogs not answered yet, in request order.
func (b *Backend) OpenDialogs() []Dialog {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Dialog
	for _, d := range b.desktop.dialogs {
		if b.desktop.open[d.ID] != nil {
			out = append(out, d)
		}
	}
	return out
}

// AnswerDialog answers the open dialog id as the user would; the app's
// callback runs on the UI thread from the next loop iteration. It panics
// if the dialog is not open or the response is impossible (see
// SetDialogResponder). Safe from any goroutine.
func (b *Backend) AnswerDialog(id int, r DialogResponse) {
	b.mu.Lock()
	answer := b.desktop.open[id]
	if answer == nil {
		b.mu.Unlock()
		panic(fmt.Sprintf("headless: AnswerDialog(%d): no such open dialog (see OpenDialogs)", id))
	}
	d := b.desktop.dialogs[id-1]
	b.mu.Unlock()
	checkResponse(d, r)
	b.mu.Lock()
	delete(b.desktop.open, id)
	b.loop.Post(func() { answer(r) })
	b.mu.Unlock()
	b.signal()
}

// checkResponse panics if r is not an answer a real dialog d could give.
func checkResponse(d Dialog, r DialogResponse) {
	switch {
	case d.Kind == DialogMessageBox && r.Err == nil && !d.Message.Buttons.Has(r.Answer):
		panic(fmt.Sprintf("headless: dialog %d: answer %v is not a button of the message box (buttons %d)", d.ID, r.Answer, d.Message.Buttons))
	case d.Kind != DialogMessageBox && len(r.Paths) > 1 && !(d.Kind == DialogOpenFiles && d.File.Multiple):
		panic(fmt.Sprintf("headless: dialog %d (%v) answered with %d paths; only an OpenFiles dialog with Multiple returns several", d.ID, d.Kind, len(r.Paths)))
	case slices.Contains(r.Paths, ""):
		panic(fmt.Sprintf("headless: dialog %d answered with an empty path", d.ID))
	}
}

// Opened returns the OpenURI and OpenPath requests accepted so far.
func (b *Backend) Opened() []OpenRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.opened)
}

// Notifications returns every notification shown so far, closed ones
// included, in order.
func (b *Backend) Notifications() []NotificationRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.notifications)
}

// Trays returns the tray icons present, in creation order.
func (b *Backend) Trays() []TrayRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.trays)
}

// InjectApp queues an app-scoped event as if the OS produced it: a
// NotificationEvent (the user clicked a notification or one of its
// actions, or it closed) or a TrayEvent (the user clicked a tray icon or
// chose one of its menu items). The event must be one a real backend could
// deliver: an open notification and an existing action; an existing tray
// and an enabled item without a submenu. A NotificationClosed marks the
// notification closed. Safe from any goroutine.
func (b *Backend) InjectApp(e platform.Event) {
	b.mu.Lock()
	problem := ""
	switch e := e.(type) {
	case platform.NotificationEvent:
		i := slices.IndexFunc(b.desktop.notifications, func(r NotificationRecord) bool { return r.ID == e.ID })
		switch {
		case i < 0 || b.desktop.notifications[i].Closed:
			problem = fmt.Sprintf("notification %d is not open", e.ID)
		case e.Kind > platform.NotificationClosed:
			problem = fmt.Sprintf("invalid Kind %d", e.Kind)
		case e.Kind == platform.NotificationActionInvoked && !slices.ContainsFunc(b.desktop.notifications[i].Notification.Actions,
			func(a platform.NotificationAction) bool { return a.ID == e.Action }):
			problem = fmt.Sprintf("notification %d has no action %q", e.ID, e.Action)
		case e.Kind != platform.NotificationActionInvoked && e.Action != "":
			problem = "Action is set on an event that is not NotificationActionInvoked"
		case e.Kind == platform.NotificationClosed:
			b.desktop.notifications[i].Closed = true
		}
	case platform.TrayEvent:
		i := slices.IndexFunc(b.desktop.trays, func(r TrayRecord) bool { return r.ID == e.Tray })
		switch {
		case i < 0:
			problem = fmt.Sprintf("tray %d does not exist", e.Tray)
		case e.Kind > platform.TrayMenuItem:
			problem = fmt.Sprintf("invalid Kind %d", e.Kind)
		case e.Kind == platform.TrayActivated && e.Item != 0:
			problem = "Item is set on a TrayActivated event"
		case e.Kind == platform.TrayMenuItem:
			it, ok := platform.FindMenuItem(b.desktop.trays[i].Options.Menu, e.Item)
			if !ok {
				problem = fmt.Sprintf("tray %d has no menu item %d", e.Tray, e.Item)
			} else if it.Disabled {
				problem = fmt.Sprintf("menu item %d (%q) is disabled", e.Item, it.Label)
			}
		}
	case nil:
		problem = "nil event"
	default:
		problem = fmt.Sprintf("%T is not an app-scoped desktop event; use Window.Inject for window events", e)
	}
	if problem != "" {
		b.mu.Unlock()
		panic(fmt.Sprintf("headless: InjectApp(%+v): %s", e, problem))
	}
	b.loop.Enqueue(nil, e)
	b.mu.Unlock()
	b.signal()
}

// desktop implements platform.Desktop for a headless Backend.
type desktop struct{ b *Backend }

var _ platform.Desktop = desktop{}

// supported reports whether service s is supported; b.mu must not be held.
func (d desktop) supported(s Service) bool {
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	return !d.b.desktop.unsupported[s]
}

func (d desktop) open(r OpenRequest) error {
	if !d.supported(ServiceOpen) {
		return platform.ErrUnsupported
	}
	d.b.mu.Lock()
	fn := d.b.desktop.openResponder
	d.b.mu.Unlock()
	if fn != nil {
		if err := fn(r); err != nil {
			return err
		}
	}
	d.b.mu.Lock()
	d.b.desktop.opened = append(d.b.desktop.opened, r)
	d.b.mu.Unlock()
	return nil
}

func (d desktop) OpenURI(parent platform.Window, uri string) error {
	if err := platform.CheckURI(uri); err != nil {
		return err
	}
	return d.open(OpenRequest{Parent: parent, Target: uri})
}

func (d desktop) OpenPath(parent platform.Window, path string) error {
	if path == "" {
		return errors.New("headless: OpenPath with an empty path")
	}
	return d.open(OpenRequest{Parent: parent, Target: path, Path: true})
}

// request records a dialog and answers it through the responder, or
// leaves it open.
func (d desktop) request(dlg Dialog, service Service, answer func(DialogResponse)) error {
	if !d.supported(service) {
		return platform.ErrUnsupported
	}
	b := d.b
	b.mu.Lock()
	dlg.ID = len(b.desktop.dialogs) + 1
	b.desktop.dialogs = append(b.desktop.dialogs, dlg)
	if b.desktop.open == nil {
		b.desktop.open = map[int]func(DialogResponse){}
	}
	b.desktop.open[dlg.ID] = answer
	fn := b.desktop.responder
	b.mu.Unlock()
	if fn != nil {
		b.AnswerDialog(dlg.ID, fn(dlg))
	}
	return nil
}

func (d desktop) OpenFiles(opts platform.FileDialogOptions, done func([]string, error)) error {
	if done == nil {
		panic("headless: OpenFiles called with a nil callback")
	}
	platform.CheckFileDialogOptions("OpenFiles", opts)
	return d.request(Dialog{Kind: DialogOpenFiles, File: cloneFileOptions(opts)}, ServiceFileDialogs, func(r DialogResponse) {
		if r.Err != nil {
			done(nil, r.Err)
			return
		}
		done(slices.Clone(r.Paths), nil)
	})
}

func (d desktop) SaveFile(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("headless: SaveFile called with a nil callback")
	}
	platform.CheckFileDialogOptions("SaveFile", opts)
	return d.request(Dialog{Kind: DialogSaveFile, File: cloneFileOptions(opts)}, ServiceFileDialogs, func(r DialogResponse) {
		done(firstPath(r))
	})
}

func (d desktop) PickFolder(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("headless: PickFolder called with a nil callback")
	}
	platform.CheckFileDialogOptions("PickFolder", opts)
	return d.request(Dialog{Kind: DialogPickFolder, File: cloneFileOptions(opts)}, ServiceFileDialogs, func(r DialogResponse) {
		done(firstPath(r))
	})
}

func firstPath(r DialogResponse) (string, error) {
	if r.Err != nil || len(r.Paths) == 0 {
		return "", r.Err
	}
	return r.Paths[0], nil
}

func cloneFileOptions(o platform.FileDialogOptions) platform.FileDialogOptions {
	o.Filters = slices.Clone(o.Filters)
	for i := range o.Filters {
		o.Filters[i].Patterns = slices.Clone(o.Filters[i].Patterns)
	}
	return o
}

func (d desktop) MessageBox(opts platform.MessageBoxOptions, done func(platform.MessageAnswer, error)) error {
	if done == nil {
		panic("headless: MessageBox called with a nil callback")
	}
	platform.CheckMessageBoxOptions(opts)
	return d.request(Dialog{Kind: DialogMessageBox, Message: opts}, ServiceMessageBox, func(r DialogResponse) {
		if r.Err != nil {
			done(opts.Buttons.DismissAnswer(), r.Err)
			return
		}
		done(r.Answer, nil)
	})
}

func (d desktop) Notify(n platform.Notification) (platform.NotificationID, error) {
	platform.CheckNotification(n)
	if !d.supported(ServiceNotifications) {
		return 0, platform.ErrUnsupported
	}
	n.Icon = platform.CopyIcon(n.Icon)
	n.Actions = slices.Clone(n.Actions)
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	id := platform.NotificationID(len(b.desktop.notifications) + 1)
	b.desktop.notifications = append(b.desktop.notifications, NotificationRecord{ID: id, Notification: n})
	return id, nil
}

func (d desktop) CloseNotification(id platform.NotificationID) error {
	if !d.supported(ServiceNotifications) {
		return platform.ErrUnsupported
	}
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.desktop.notifications {
		if b.desktop.notifications[i].ID == id {
			b.desktop.notifications[i].Closed = true
		}
	}
	return nil
}

func cloneTrayOptions(o platform.TrayOptions) platform.TrayOptions {
	o.Icon = platform.CopyIcon(o.Icon)
	o.Menu = cloneMenu(o.Menu)
	return o
}

func cloneMenu(items []platform.MenuItem) []platform.MenuItem {
	items = slices.Clone(items)
	for i := range items {
		items[i].Submenu = cloneMenu(items[i].Submenu)
	}
	return items
}

func (d desktop) NewTray(opts platform.TrayOptions) (platform.TrayID, error) {
	platform.CheckTrayOptions(opts)
	if !d.supported(ServiceTray) {
		return 0, platform.ErrUnsupported
	}
	opts = cloneTrayOptions(opts)
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.lastTray++
	id := b.desktop.lastTray
	b.desktop.trays = append(b.desktop.trays, TrayRecord{ID: id, Options: opts})
	return id, nil
}

func (d desktop) UpdateTray(id platform.TrayID, opts platform.TrayOptions) error {
	platform.CheckTrayOptions(opts)
	if !d.supported(ServiceTray) {
		return platform.ErrUnsupported
	}
	opts = cloneTrayOptions(opts)
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	i := slices.IndexFunc(b.desktop.trays, func(r TrayRecord) bool { return r.ID == id })
	if i < 0 {
		return fmt.Errorf("headless: UpdateTray: tray %d does not exist", id)
	}
	b.desktop.trays[i].Options = opts
	b.desktop.trays[i].Updates++
	return nil
}

func (d desktop) RemoveTray(id platform.TrayID) error {
	if !d.supported(ServiceTray) {
		return platform.ErrUnsupported
	}
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.trays = slices.DeleteFunc(b.desktop.trays, func(r TrayRecord) bool { return r.ID == id })
	return nil
}
