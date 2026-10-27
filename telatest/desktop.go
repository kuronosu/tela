package telatest

import (
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/semantics"
)

// The desktop services of package desktop in tests (ADR-181). The headless
// backend shows nothing: it records the dialogs, notifications and tray
// icons the app asks for, and these methods answer and click them as the
// user would. Nothing is answered by itself: a dialog stays open until the
// test answers it.

// TempDesktopDirs points package desktop's folders (ConfigDir, the
// settings store, saved window placements...) at a new temporary directory
// for the rest of the test, through the desktop.DirEnv variable, and
// returns it. Call it before the app reads a setting. Like t.Setenv, it
// cannot be used in parallel tests.
func TempDesktopDirs(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	tb.Setenv(desktop.DirEnv, dir)
	// Write what the stores hold before the folder is removed (cleanups
	// run last first).
	tb.Cleanup(func() { _ = desktop.FlushAll() })
	return dir
}

// DesktopService is a group of desktop services, for [App.Unsupported].
type DesktopService uint8

const (
	// FileDialogs: desktop.OpenFile, OpenFiles, SaveFile, PickFolder.
	FileDialogs DesktopService = iota + 1
	// MessageBoxes: the native message box of desktop.ShowMessage; without
	// it the box is drawn in a window, as on Linux.
	MessageBoxes
	// Notifications: desktop.Notify.
	Notifications
	// TrayIcons: desktop.Tray.
	TrayIcons
	// OpenURLs: tela.OpenURL and tela.OpenPath.
	OpenURLs
)

// Unsupported makes the headless desktop lack the given services from now
// on, as a desktop without them does (Linux without a portal, GNOME
// without a tray): the calls return errors wrapping
// desktop.ErrUnsupported, and ShowMessage draws its box. Called with
// nothing, every service is supported again.
//
//	app := telatest.Mount(t, Editor{})
//	app.Unsupported(telatest.FileDialogs)
//	app.Click(telatest.ByRole("button", "Open…"))
//	app.ExpectText("Type the path of the file")
func (a *App) Unsupported(services ...DesktopService) {
	a.tb.Helper()
	a.mustBeOpen()
	var hs []headless.Service
	for _, s := range services {
		switch s {
		case FileDialogs:
			hs = append(hs, headless.ServiceFileDialogs)
		case MessageBoxes:
			hs = append(hs, headless.ServiceMessageBox)
		case Notifications:
			hs = append(hs, headless.ServiceNotifications)
		case TrayIcons:
			hs = append(hs, headless.ServiceTray)
		case OpenURLs:
			hs = append(hs, headless.ServiceOpen)
		default:
			a.fatalf("Unsupported(%d): not a DesktopService; use telatest.FileDialogs, MessageBoxes, Notifications, TrayIcons or OpenURLs", s)
		}
	}
	a.backend.SetUnsupported(hs...)
}

// ---------------------------------------------------------------------
// File dialogs.

// FileDialogKind is the kind of a [FileDialog].
type FileDialogKind string

const (
	OpenFileDialog   FileDialogKind = "OpenFile"   // desktop.OpenFile
	OpenFilesDialog  FileDialogKind = "OpenFiles"  // desktop.OpenFiles (several files)
	SaveFileDialog   FileDialogKind = "SaveFile"   // desktop.SaveFile
	PickFolderDialog FileDialogKind = "PickFolder" // desktop.PickFolder
)

// FileDialog is a file dialog the app opened and the test has not answered.
type FileDialog struct {
	Kind FileDialogKind
	// Title, Directory and DefaultName are the options the app gave
	// (desktop.Title, Directory, DefaultName).
	Title, Directory, DefaultName string
	// Filters are the desktop.Filter entries.
	Filters []desktop.FileFilter
}

// FileDialogs returns the file dialogs open now, oldest first, after
// settling the app.
func (a *App) FileDialogs() []FileDialog {
	a.tb.Helper()
	a.settleAfter()
	var out []FileDialog
	for _, d := range a.backend.OpenDialogs() {
		if d.Kind != headless.DialogMessageBox {
			out = append(out, fileDialogOf(d))
		}
	}
	return out
}

func fileDialogOf(d headless.Dialog) FileDialog {
	kind := map[headless.DialogKind]FileDialogKind{
		headless.DialogOpenFiles: OpenFileDialog, headless.DialogSaveFile: SaveFileDialog, headless.DialogPickFolder: PickFolderDialog,
	}[d.Kind]
	if d.Kind == headless.DialogOpenFiles && d.File.Multiple {
		kind = OpenFilesDialog
	}
	fd := FileDialog{Kind: kind, Title: d.File.Title, Directory: d.File.Directory, DefaultName: d.File.Name}
	for _, f := range d.File.Filters {
		fd.Filters = append(fd.Filters, desktop.FileFilter{Name: f.Name, Patterns: slices.Clone(f.Patterns)})
	}
	return fd
}

// AnswerFileDialog answers the oldest open file dialog as the user would:
// with the given paths, or, without paths, by canceling it. The app's
// callback runs and the app settles. It fails the test when no file
// dialog is open, or when a dialog that takes one path gets several.
//
//	app.Click(telatest.ByRole("button", "Open…"))
//	app.AnswerFileDialog("/home/ana/notes/groceries.md")
//	app.ExpectText("groceries.md")
func (a *App) AnswerFileDialog(paths ...string) {
	a.tb.Helper()
	a.settleAfter()
	for _, d := range a.backend.OpenDialogs() {
		if d.Kind == headless.DialogMessageBox {
			continue
		}
		if len(paths) > 1 && !(d.Kind == headless.DialogOpenFiles && d.File.Multiple) {
			a.fatalf("AnswerFileDialog: the %s dialog takes one path, got %d", fileDialogOf(d).Kind, len(paths))
		}
		if slices.Contains(paths, "") {
			a.fatalf("AnswerFileDialog: an empty path; call AnswerFileDialog() without paths to cancel")
		}
		a.backend.AnswerDialog(d.ID, headless.DialogResponse{Paths: slices.Clone(paths)})
		a.settleAfter()
		return
	}
	a.fatalf("AnswerFileDialog: no file dialog is open (the app's desktop.OpenFile, SaveFile or PickFolder was not called, or was answered already)")
}

// ---------------------------------------------------------------------
// Message boxes.

// MessageBox is a message box the app shows (desktop.ShowMessage) and the
// test has not answered.
type MessageBox struct {
	Kind    desktop.MessageKind
	Title   string
	Text    string
	Buttons desktop.Buttons
	// Drawn: Tela draws the box in a window (the desktop has no native
	// message box, as on Linux; see [App.Unsupported]).
	Drawn bool
}

// MessageBoxes returns the message boxes open now, oldest first (native
// ones before drawn ones), after settling the app.
func (a *App) MessageBoxes() []MessageBox {
	a.tb.Helper()
	a.settleWindows()
	var out []MessageBox
	for _, d := range a.backend.OpenDialogs() {
		if d.Kind == headless.DialogMessageBox {
			m := d.Message
			out = append(out, MessageBox{Kind: desktop.MessageKind(m.Kind), Title: m.Title, Text: m.Text, Buttons: desktop.Buttons(m.Buttons)})
		}
	}
	for _, b := range a.drawnBoxes() {
		out = append(out, b.box)
	}
	return out
}

// AnswerMessage answers the oldest open message box with the button
// answer, as the user would (a drawn box gets a real click on its
// button); the app's callback runs and the app settles. It fails the test
// when no box is open or the box has no such button.
//
//	app.Click(telatest.ByRole("button", "Close"))
//	app.AnswerMessage(desktop.AnswerNo) // "Save the changes?" — No
func (a *App) AnswerMessage(answer desktop.Answer) {
	a.tb.Helper()
	a.settleWindows()
	for _, d := range a.backend.OpenDialogs() {
		if d.Kind != headless.DialogMessageBox {
			continue
		}
		if !desktop.Buttons(d.Message.Buttons).Has(answer) {
			a.fatalf("AnswerMessage(%v): the message box %q has no such button", answer, d.Message.Title)
		}
		a.backend.AnswerDialog(d.ID, headless.DialogResponse{Answer: platform.MessageAnswer(answer)})
		a.settleAfter()
		return
	}
	boxes := a.drawnBoxes()
	if len(boxes) == 0 {
		a.fatalf("AnswerMessage(%v): no message box is open (desktop.ShowMessage was not called, or was answered already)", answer)
	}
	b := boxes[0]
	label := map[desktop.Answer]string{desktop.AnswerOK: "OK", desktop.AnswerCancel: "Cancel", desktop.AnswerYes: "Yes", desktop.AnswerNo: "No"}[answer]
	btn, ok := b.buttons[label]
	if !ok {
		a.fatalf("AnswerMessage(%v): the message box %q has no such button", answer, b.box.Title)
	}
	p := platform.Point{X: (btn.X0 + btn.X1) / 2, Y: (btn.Y0 + btn.Y1) / 2}
	for _, act := range []platform.PointerAction{platform.PointerMove, platform.PointerDown, platform.PointerUp, platform.PointerLeave} {
		e := platform.PointerEvent{Action: act, Pos: p}
		switch act {
		case platform.PointerDown:
			e.Button, e.Buttons = platform.ButtonPrimary, platform.ButtonPrimary.Mask()
		case platform.PointerUp:
			e.Button = platform.ButtonPrimary
		}
		b.win.Inject(e)
		a.settleWindows()
	}
}

// settleWindows settles the app and runs the frames the other windows of
// the app wait for (a drawn message box lays out in its own window).
func (a *App) settleWindows() {
	a.tb.Helper()
	for range 100 {
		a.settleAfter()
		waiting := false
		for _, w := range a.backend.Windows() {
			if w != a.win && !w.Closed() && w.FrameRequested() {
				waiting = true
			}
		}
		if !waiting {
			return
		}
		if next, ok := a.backend.NextFrameTime(); ok {
			if now := a.backend.Now(); next > now {
				a.backend.Advance(next - now)
			}
		}
		a.backend.Wake()
		a.deliver()
	}
}

// drawnBox is a drawn message box found in a window of the app.
type drawnBox struct {
	box     MessageBox
	win     *headless.Window
	buttons map[string]semanticsRect
}

type semanticsRect struct{ X0, Y0, X1, Y1 float32 }

// drawnBoxes finds the drawn message boxes in the windows the app opened
// (an alertdialog the desktop package draws), in the order they opened.
func (a *App) drawnBoxes() []drawnBox {
	var out []drawnBox
	for _, w := range a.host.Windows() {
		hw, ok := w.(*headless.Window)
		if !ok || hw == a.win {
			continue
		}
		t := a.host.TreeOf(w)
		if t == nil {
			continue
		}
		var walk func(n *semantics.Node) bool
		walk = func(n *semantics.Node) bool {
			if n.Role == "alertdialog" {
				out = append(out, drawnBoxOf(n, hw))
				return true
			}
			for _, c := range n.Children {
				if walk(c) {
					return true
				}
			}
			return false
		}
		walk(t.Semantics())
	}
	return out
}

func drawnBoxOf(n *semantics.Node, w *headless.Window) drawnBox {
	b := drawnBox{box: MessageBox{Title: n.Name, Text: n.Description, Drawn: true}, win: w, buttons: map[string]semanticsRect{}}
	var labels []string
	var walk func(n *semantics.Node)
	walk = func(n *semantics.Node) {
		switch n.Role {
		case "button":
			r := n.Bounds
			b.buttons[n.Name] = semanticsRect{r.X0, r.Y0, r.X1, r.Y1}
			labels = append(labels, n.Name)
		case "img":
			switch n.Name {
			case "Warning":
				b.box.Kind = desktop.MessageWarning
			case "Error":
				b.box.Kind = desktop.MessageError
			case "Question":
				b.box.Kind = desktop.MessageQuestion
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	switch strings.Join(labels, ",") {
	case "Cancel,OK":
		b.box.Buttons = desktop.ButtonsOKCancel
	case "No,Yes":
		b.box.Buttons = desktop.ButtonsYesNo
	case "Cancel,No,Yes":
		b.box.Buttons = desktop.ButtonsYesNoCancel
	}
	return b
}

// ---------------------------------------------------------------------
// Notifications.

// Notification is a notification the app showed (desktop.Notify).
type Notification struct {
	Title, Body string
	// Actions are the labels of its buttons.
	Actions []string
	// Closed: the app closed it, or the user clicked or dismissed it.
	Closed bool
}

// Notifications returns the notifications the app showed, oldest first,
// closed ones included, after settling the app.
func (a *App) Notifications() []Notification {
	a.tb.Helper()
	a.settleAfter()
	var out []Notification
	for _, r := range a.backend.Notifications() {
		n := Notification{Title: r.Notification.Title, Body: r.Notification.Body, Closed: r.Closed}
		for _, act := range r.Notification.Actions {
			n.Actions = append(n.Actions, act.Label)
		}
		out = append(out, n)
	}
	return out
}

// openNotification returns the newest open notification with the title,
// failing the test without one.
func (a *App) openNotification(what, title string) headless.NotificationRecord {
	a.tb.Helper()
	a.settleAfter()
	rs := a.backend.Notifications()
	for i := len(rs) - 1; i >= 0; i-- {
		if r := rs[i]; !r.Closed && r.Notification.Title == title {
			return r
		}
	}
	var titles []string
	for _, r := range rs {
		if !r.Closed {
			titles = append(titles, r.Notification.Title)
		}
	}
	a.fatalf("%s(%q): no open notification has that title (open: %q)", what, title, titles)
	return headless.NotificationRecord{}
}

// ClickNotification clicks the open notification with the given title (the
// newest, if several): its OnClick runs and it closes.
func (a *App) ClickNotification(title string) {
	a.tb.Helper()
	r := a.openNotification("ClickNotification", title)
	a.backend.InjectApp(platform.NotificationEvent{ID: r.ID, Kind: platform.NotificationActivated})
	a.backend.InjectApp(platform.NotificationEvent{ID: r.ID, Kind: platform.NotificationClosed})
	a.settleAfter()
}

// ClickNotificationAction clicks the button labeled action of the open
// notification with the given title: the action's OnClick runs and the
// notification closes.
func (a *App) ClickNotificationAction(title, action string) {
	a.tb.Helper()
	r := a.openNotification("ClickNotificationAction", title)
	i := slices.IndexFunc(r.Notification.Actions, func(x platform.NotificationAction) bool { return x.Label == action })
	if i < 0 {
		var labels []string
		for _, x := range r.Notification.Actions {
			labels = append(labels, x.Label)
		}
		a.fatalf("ClickNotificationAction(%q, %q): the notification has no such action (actions: %q)", title, action, labels)
	}
	a.backend.InjectApp(platform.NotificationEvent{ID: r.ID, Kind: platform.NotificationActionInvoked, Action: r.Notification.Actions[i].ID})
	a.backend.InjectApp(platform.NotificationEvent{ID: r.ID, Kind: platform.NotificationClosed})
	a.settleAfter()
}

// DismissNotification closes the open notification with the given title
// as the user does with its close button (or as it expires): its OnClose
// runs.
func (a *App) DismissNotification(title string) {
	a.tb.Helper()
	r := a.openNotification("DismissNotification", title)
	a.backend.InjectApp(platform.NotificationEvent{ID: r.ID, Kind: platform.NotificationClosed})
	a.settleAfter()
}

// ---------------------------------------------------------------------
// The tray.

// TrayIcon is a tray icon the app shows (a mounted desktop.Tray).
type TrayIcon struct {
	Tooltip string
	Menu    []TrayItem
	// Updates counts the changes of the icon since it was added.
	Updates int
}

// TrayItem is an item of a tray icon's menu.
type TrayItem struct {
	Label                        string
	Separator, Checkbox, Checked bool
	Disabled                     bool
	Submenu                      []TrayItem
}

// Trays returns the tray icons the app shows now, in the order they were
// added, after settling the app.
func (a *App) Trays() []TrayIcon {
	a.tb.Helper()
	a.settleAfter()
	var out []TrayIcon
	for _, r := range a.backend.Trays() {
		out = append(out, TrayIcon{Tooltip: r.Options.Tooltip, Menu: trayItems(r.Options.Menu), Updates: r.Updates})
	}
	return out
}

func trayItems(items []platform.MenuItem) []TrayItem {
	var out []TrayItem
	for _, it := range items {
		out = append(out, TrayItem{Label: it.Label, Separator: it.Separator, Checkbox: it.Checkable, Checked: it.Checked,
			Disabled: it.Disabled, Submenu: trayItems(it.Submenu)})
	}
	return out
}

// Tray returns the app's tray icon; it fails the test unless the app shows
// exactly one.
func (a *App) Tray() TrayIcon {
	a.tb.Helper()
	ts := a.Trays()
	if len(ts) != 1 {
		a.fatalf("Tray: the app shows %d tray icons; Tray needs exactly one (use Trays)", len(ts))
	}
	return ts[0]
}

// oneTray returns the record of the app's only tray icon.
func (a *App) oneTray(what string) headless.TrayRecord {
	a.tb.Helper()
	a.settleAfter()
	rs := a.backend.Trays()
	if len(rs) != 1 {
		a.fatalf("%s: the app shows %d tray icons; it needs exactly one", what, len(rs))
	}
	return rs[0]
}

// ClickTray clicks the app's tray icon (its OnClick runs). It fails the
// test unless the app shows exactly one.
func (a *App) ClickTray() {
	a.tb.Helper()
	r := a.oneTray("ClickTray")
	a.backend.InjectApp(platform.TrayEvent{Tray: r.ID, Kind: platform.TrayActivated})
	a.settleAfter()
}

// ClickTrayItem chooses an item of the tray icon's menu (its OnSelect
// runs), named by its label, and for an item in a submenu by the labels of
// the submenus first:
//
//	app.ClickTrayItem("Quit")
//	app.ClickTrayItem("Theme", "Dark")
//
// It fails the test when the app does not show exactly one tray icon, or
// the item is missing, disabled, or opens a submenu.
func (a *App) ClickTrayItem(path ...string) {
	a.tb.Helper()
	if len(path) == 0 {
		a.fatalf("ClickTrayItem(): give the label of the item, such as ClickTrayItem(\"Quit\")")
	}
	r := a.oneTray("ClickTrayItem")
	items := r.Options.Menu
	for i, label := range path {
		j := slices.IndexFunc(items, func(it platform.MenuItem) bool { return !it.Separator && it.Label == label })
		if j < 0 {
			var labels []string
			for _, it := range items {
				if !it.Separator {
					labels = append(labels, it.Label)
				}
			}
			a.fatalf("ClickTrayItem(%q): no item %q in the menu (items: %q)", path, label, labels)
		}
		it := items[j]
		if it.Disabled {
			a.fatalf("ClickTrayItem(%q): %q is disabled", path, label)
		}
		if i < len(path)-1 {
			if len(it.Submenu) == 0 {
				a.fatalf("ClickTrayItem(%q): %q has no submenu", path, label)
			}
			items = it.Submenu
			continue
		}
		if len(it.Submenu) > 0 {
			a.fatalf("ClickTrayItem(%q): %q opens a submenu; name an item in it too", path, label)
		}
		a.backend.InjectApp(platform.TrayEvent{Tray: r.ID, Kind: platform.TrayMenuItem, Item: it.ID})
		a.settleAfter()
	}
}
