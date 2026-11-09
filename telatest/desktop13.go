package telatest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// The desktop services of phase 13 in tests (ADR-213): global shortcuts,
// launches, secrets, the clipboard, native menus and the registration of
// schemes and file types, all through the headless backend's fakes. As
// for dialogs, nothing happens by itself: the test presses the shortcut,
// starts the app again, chooses the menu item.

// isFileDialog reports whether d is a file dialog (not a message box or a
// popup menu).
func isFileDialog(d headless.Dialog) bool {
	return d.Kind != headless.DialogMessageBox && d.Kind != headless.DialogPopupMenu
}

// ---------------------------------------------------------------------
// Global shortcuts.

// GlobalShortcut is a global shortcut the app registered (a mounted
// desktop.GlobalShortcut).
type GlobalShortcut struct {
	ID, Description string
	// Keys is the combination asked for, written the usual way
	// ("Ctrl+Alt+N").
	Keys string
	// Status and Trigger are the latest outcome the app was told
	// (ShortcutStatusEvent).
	Status  desktop.ShortcutStatus
	Trigger string
}

// GlobalShortcuts returns the global shortcuts the app has registered, in
// registration order, after settling the app.
func (a *App) GlobalShortcuts() []GlobalShortcut {
	a.tb.Helper()
	a.settleAfter()
	var out []GlobalShortcut
	for _, r := range a.backend.Shortcuts() {
		out = append(out, GlobalShortcut{ID: r.Shortcut.ID, Description: r.Shortcut.Description,
			Keys: r.Shortcut.Accelerator.String(), Status: desktop.ShortcutStatus(r.Status), Trigger: r.Trigger})
	}
	return out
}

// shortcut returns the registered shortcut id, failing the test without
// one, or when it is not bound and bound is set.
func (a *App) shortcut(what, id string, bound bool) headless.ShortcutRecord {
	a.tb.Helper()
	a.mustRun()
	a.settleAfter()
	rs := a.backend.Shortcuts()
	i := slices.IndexFunc(rs, func(r headless.ShortcutRecord) bool { return r.Shortcut.ID == id })
	if i < 0 {
		var ids []string
		for _, r := range rs {
			ids = append(ids, r.Shortcut.ID)
		}
		a.fatalf("%s(%q): no global shortcut has that ID (registered: %q); is its desktop.GlobalShortcut mounted?", what, id, ids)
	}
	if bound && rs[i].Status != platform.ShortcutBound {
		a.fatalf("%s(%q): the shortcut is %v, not bound, so the user cannot press it", what, id, rs[i].Status)
	}
	return rs[i]
}

// PressShortcut presses the global shortcut id as the user does, from any
// application: its OnPress runs and the app settles. It fails the test
// when no mounted desktop.GlobalShortcut has the ID or it is not bound.
//
//	app.PressShortcut("new-note")
//	app.ExpectText("Untitled note")
func (a *App) PressShortcut(id string) {
	a.tb.Helper()
	a.shortcut("PressShortcut", id, true)
	a.backend.InjectApp(platform.ShortcutEvent{ID: id, Action: platform.ShortcutPressed, Time: a.backend.Now()})
	a.settleAfter()
}

// ReleaseShortcut releases the global shortcut id (its OnRelease runs),
// as Linux reports; Windows reports no releases.
func (a *App) ReleaseShortcut(id string) {
	a.tb.Helper()
	a.shortcut("ReleaseShortcut", id, true)
	a.backend.InjectApp(platform.ShortcutEvent{ID: id, Action: platform.ShortcutReleased, Time: a.backend.Now()})
	a.settleAfter()
}

// SetShortcutStatus plays the OS changing the state of the registered
// shortcut id: another application took its keys (ShortcutTaken), the
// user declined it (ShortcutRefused) or removed it (ShortcutRevoked), or
// bound it to other keys (ShortcutBound with trigger). Its OnStatus runs
// and the app settles. trigger is ignored unless the status is
// ShortcutBound; "" then means the keys asked for.
//
//	app.SetShortcutStatus("new-note", desktop.ShortcutTaken, "")
//	app.ExpectText("Ctrl+Alt+N is taken by another application")
func (a *App) SetShortcutStatus(id string, status desktop.ShortcutStatus, trigger string) {
	a.tb.Helper()
	r := a.shortcut("SetShortcutStatus", id, false)
	if status > desktop.ShortcutFailed {
		a.fatalf("SetShortcutStatus(%q, %d): not a desktop.ShortcutStatus", id, status)
	}
	e := platform.ShortcutStatusEvent{ID: id, Status: platform.ShortcutStatus(status)}
	if status == desktop.ShortcutBound {
		e.Trigger = trigger
		if e.Trigger == "" {
			e.Trigger = r.Shortcut.Accelerator.String()
		}
	}
	a.backend.InjectApp(e)
	a.settleAfter()
}

// SetShortcutUnassigned plays a desktop that registers the shortcut id
// without assigning it keys, as Hyprland does (the user binds it in
// hyprland.conf): its OnStatus gets ShortcutBound with an empty Trigger,
// and the app settles. PressShortcut still works, as once the user bound
// it.
//
//	app.SetShortcutUnassigned("new-note")
//	app.ExpectText("bind = CTRL ALT, N, global, com.example.notes:new-note")
func (a *App) SetShortcutUnassigned(id string) {
	a.tb.Helper()
	a.shortcut("SetShortcutUnassigned", id, false)
	a.backend.InjectApp(platform.ShortcutStatusEvent{ID: id, Status: platform.ShortcutBound})
	a.settleAfter()
}

// FailShortcuts makes every global shortcut the app registers from now on
// fail with err, until FailShortcuts(nil); shortcuts registered already
// keep their state. An error wrapping desktop.ErrAppNotRegistered plays a
// Linux desktop without the app's entry (desktop.RegisterApp). OnStatus
// gets ShortcutFailed with an error wrapping err.
//
//	app.FailShortcuts(desktop.ErrAppNotRegistered)
//	app.Click(telatest.ByRole("switch", "Quick note")) // registers again
func (a *App) FailShortcuts(err error) {
	a.tb.Helper()
	a.backend.SetShortcutError(err)
}

// ---------------------------------------------------------------------
// Launches.

// InjectLaunch plays the user starting the app again while it runs, as
// a single-instance app sees it (desktop.SingleInstance): the new process
// forwards l (its Args and Dir) and exits, the main window comes to the
// front (shown if hidden), and every desktop.UseLaunch handler gets the
// launch, with Source LaunchForwarded. With l.Source LaunchOpened it plays
// the OS opening links or files with the running app instead (macOS
// Launch Services: a link clicked, a file opened from the Finder), which
// needs no SingleInstance; with l.AtStart too, the file that started the
// app, as macOS hands it just after the start. The app settles.
//
//	app := telatest.MountApp(t, desktop.SingleInstance(App))
//	app.InjectLaunch(desktop.Launch{Args: []string{"notes:open?id=3"}})
//	app.ExpectText("Groceries")
func (a *App) InjectLaunch(l desktop.Launch) {
	a.tb.Helper()
	a.mustRun()
	if l.Source == desktop.LaunchOpened {
		a.backend.InjectApp(platform.LaunchEvent{Launch: platform.Launch{Args: slices.Clone(l.Args), Dir: l.Dir}, Source: platform.LaunchOpened, AtStart: l.AtStart})
		a.settleAfter()
		return
	}
	if len(a.backend.Instances()) == 0 {
		// The single instance the app would have claimed when Run started
		// (telatest does not call Run). The fake's registry is shared by
		// the process: a name of this backend's own keeps tests apart.
		id := fmt.Sprintf("telatest.instance-%p", a.backend)
		if _, err := a.backend.Desktop().ClaimInstance(id, platform.Launch{}); err != nil {
			a.fatalf("InjectLaunch: %v", err)
		}
		a.tb.Cleanup(a.backend.ReleaseInstances)
	}
	a.backend.InjectApp(platform.LaunchEvent{Launch: platform.Launch{Args: slices.Clone(l.Args), Dir: l.Dir, Token: "telatest-token"}})
	a.settleAfter()
}

// ---------------------------------------------------------------------
// Secrets.

// secretAppID returns the app ID desktop's secrets are kept under.
func secretAppID() string { return platform.ServiceAppID(desktop.AppID()) }

// StoredSecret returns the secret the app stored under name
// (desktop.SetSecret), and whether there is one. It does not fail while
// FailSecrets is in effect.
func (a *App) StoredSecret(name string) ([]byte, bool) {
	a.tb.Helper()
	a.settleAfter()
	return a.backend.StoredSecret(secretAppID(), name)
}

// SetStoredSecret stores a secret under name, as an earlier run of the
// app would have, before the app reads it with desktop.Secret.
func (a *App) SetStoredSecret(name string, secret []byte) {
	a.tb.Helper()
	a.mustRun()
	a.backend.SetSecretError(nil)
	if err := a.backend.Desktop().SetSecret(secretAppID(), name, secret); err != nil {
		a.fatalf("SetStoredSecret(%q): %v", name, err)
	}
}

// FailSecrets makes every secret call of the app fail with err from now
// on, until FailSecrets(nil): desktop.ErrSecretCanceled plays the user
// dismissing the keyring's unlock prompt. (A desktop without secure
// storage is App.Unsupported(telatest.Secrets).)
//
//	app.FailSecrets(desktop.ErrSecretCanceled)
//	app.Click(telatest.ByRole("button", "Unlock"))
//	app.ExpectText("The keyring stayed locked")
func (a *App) FailSecrets(err error) {
	a.tb.Helper()
	a.backend.SetSecretError(err)
}

// ---------------------------------------------------------------------
// The trash.

// Trashed returns the paths the app moved to the trash with
// desktop.MoveToTrash, in order, after settling the app. The headless
// trash moves each into a temporary folder of the test, so the file is
// gone from its place, as with a real trash; failed calls are not listed.
//
//	app.Click(telatest.ByRole("button", "Move to Trash"))
//	app.ExpectText("Moved todo.md to the Trash")
//	if got := app.Trashed(); len(got) != 1 || got[0] != path {
//	    t.Errorf("trashed %v", got)
//	}
func (a *App) Trashed() []string {
	a.tb.Helper()
	a.settleAfter()
	var out []string
	for _, r := range a.backend.Trashed() {
		out = append(out, r.Path)
	}
	return out
}

// FailTrash makes every desktop.MoveToTrash of the app fail with err from
// now on, until FailTrash(nil); the file stays in place: a file in use, a
// folder the user may not change. (A desktop without a trash, or a file it
// cannot take, is App.Unsupported(telatest.Trash).)
//
//	app.FailTrash(errors.New("the file is in use"))
func (a *App) FailTrash(err error) {
	a.tb.Helper()
	a.backend.SetTrashError(err)
}

// ---------------------------------------------------------------------
// The clipboard.

// Clipboard returns a copy of the clipboard's content, after settling the
// app: what the app wrote (desktop.WriteClipboard, a text field's copy),
// or what SetClipboard put there.
//
//	app.Click(telatest.ByRole("button", "Copy link"))
//	if got := app.Clipboard().Text(); got != "notes:open?id=3" {
//	    t.Errorf("clipboard = %q", got)
//	}
func (a *App) Clipboard() *tela.DataTransfer {
	a.tb.Helper()
	a.settleAfter()
	return core.NewDataTransfer(a.backend.ClipboardData())
}

// SetClipboard puts dt's content on the clipboard, as another application
// copying it would (nil empties it), for the app to paste.
//
//	var dt tela.DataTransfer
//	dt.SetFiles("/home/ana/photo.png")
//	app.SetClipboard(&dt)
//	app.Press("Control+V")
func (a *App) SetClipboard(dt *tela.DataTransfer) {
	a.tb.Helper()
	a.mustRun()
	a.backend.SetClipboardData(core.TransferData(dt))
}

// ---------------------------------------------------------------------
// Native menus.

// MenuItem is an item of a native menu: a popup menu
// (desktop.PopupMenu), a window's menu bar (desktop.MenuBar) or the app's
// (desktop.AppMenuBar).
type MenuItem struct {
	Label, Accelerator           string
	Separator, Checkbox, Checked bool
	Disabled                     bool
	// Role is the item's (or submenu's) role, as the app gave it
	// (desktop.MenuItem.Role).
	Role    desktop.Role
	Submenu []MenuItem
}

func menuItems(items []platform.MenuItem) []MenuItem {
	var out []MenuItem
	for _, it := range items {
		out = append(out, MenuItem{Label: it.Label, Accelerator: it.Accelerator, Separator: it.Separator,
			Checkbox: it.Checkable, Checked: it.Checked, Disabled: it.Disabled, Role: desktop.Role(it.Role), Submenu: menuItems(it.Submenu)})
	}
	return out
}

// PopupMenu is a native context menu the app shows (desktop.PopupMenu)
// and the test has not answered.
type PopupMenu struct {
	// At is the menu's top-left corner in its window, logical pixels.
	At    tela.Point
	Items []MenuItem
}

// PopupMenus returns the native context menus open in this App's window,
// oldest first, after settling the app.
func (a *App) PopupMenus() []PopupMenu {
	a.tb.Helper()
	a.settleAfter()
	var out []PopupMenu
	for _, d := range a.backend.OpenDialogs() {
		if d.Kind == headless.DialogPopupMenu && d.Popup.Window == a.win {
			out = append(out, PopupMenu{At: tela.Point{X: d.Popup.At.X, Y: d.Popup.At.Y}, Items: menuItems(d.Popup.Menu)})
		}
	}
	return out
}

// openPopup returns the oldest native context menu open in this App's
// window, failing the test without one.
func (a *App) openPopup(what string) headless.Dialog {
	a.tb.Helper()
	a.mustRun()
	a.settleAfter()
	for _, d := range a.backend.OpenDialogs() {
		if d.Kind == headless.DialogPopupMenu && d.Popup.Window == a.win {
			return d
		}
	}
	a.fatalf("%s: no native context menu is open in the window (desktop.PopupMenu was not called, returned an error, or was answered already)", what)
	return headless.Dialog{}
}

// ChoosePopupItem chooses an item of the open native context menu (the
// oldest, if several), named by its label, and for an item in a submenu by
// the labels of the submenus first, as ClickTrayItem does: its OnSelect
// runs, then the menu's done, and the app settles. It fails the test when
// no menu is open, or the item is missing, disabled, or opens a submenu.
//
//	app.RightClick(telatest.ByText("groceries.md"))
//	app.ChoosePopupItem("Rename…")
func (a *App) ChoosePopupItem(path ...string) {
	a.tb.Helper()
	d := a.openPopup("ChoosePopupItem")
	it := a.findMenuItem("ChoosePopupItem", d.Popup.Menu, path)
	a.backend.AnswerDialog(d.ID, headless.DialogResponse{Item: it.ID})
	a.settleAfter()
}

// DismissPopupMenu closes the open native context menu without choosing
// (Escape, a click elsewhere): the menu's done runs with chosen false.
func (a *App) DismissPopupMenu() {
	a.tb.Helper()
	d := a.openPopup("DismissPopupMenu")
	a.backend.AnswerDialog(d.ID, headless.DialogResponse{})
	a.settleAfter()
}

// MenuBar returns the native menu bar of this App's window (a mounted
// desktop.MenuBar), nil without one, after settling the app.
func (a *App) MenuBar() []MenuItem {
	a.tb.Helper()
	a.settleAfter()
	return menuItems(a.win.Menu())
}

// ChooseMenuItem chooses an item of the window's native menu bar, named
// by the labels from the top-level entry down, as the user does with the
// mouse or Alt: its OnSelect runs and the app settles. It fails the test
// when the window has no menu bar, or the item is missing, disabled or
// opens a submenu.
//
//	app.ChooseMenuItem("File", "New")
func (a *App) ChooseMenuItem(path ...string) {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleAfter()
	menu := a.win.Menu()
	if len(menu) == 0 {
		a.fatalf("ChooseMenuItem(%q): the window has no native menu bar (no desktop.MenuBar is mounted, or the window has none: Unsupported(telatest.NativeMenus), a frameless window)", path)
	}
	it := a.findMenuItem("ChooseMenuItem", menu, path)
	a.win.Inject(platform.MenuEvent{Item: it.ID})
	a.settleAfter()
}

// findMenuItem returns the item of menu named by path, failing the test
// as ClickTrayItem does.
func (a *App) findMenuItem(what string, items []platform.MenuItem, path []string) platform.MenuItem {
	a.tb.Helper()
	if len(path) == 0 {
		a.fatalf("%s(): give the label of the item, such as %s(\"Copy\")", what, what)
	}
	for i, label := range path {
		j := slices.IndexFunc(items, func(it platform.MenuItem) bool { return !it.Separator && it.Label == label })
		if j < 0 {
			var labels []string
			for _, it := range items {
				if !it.Separator {
					labels = append(labels, it.Label)
				}
			}
			a.fatalf("%s(%q): no item %q in the menu (items: %q)", what, path, label, labels)
		}
		it := items[j]
		if it.Disabled {
			a.fatalf("%s(%q): %q is disabled", what, path, label)
		}
		if i < len(path)-1 {
			if len(it.Submenu) == 0 {
				a.fatalf("%s(%q): %q has no submenu", what, path, label)
			}
			items = it.Submenu
			continue
		}
		if len(it.Submenu) > 0 {
			a.fatalf("%s(%q): %q opens a submenu; name an item in it too", what, path, label)
		}
		return it
	}
	return platform.MenuItem{}
}

// ---------------------------------------------------------------------
// Registration of schemes and file types.

// Registration is a URL scheme or a file type the app registered
// (desktop.Register, MakeDefault), as the headless desktop recorded it.
type Registration struct {
	// Association is "notes:" for a scheme, ".note" for a file type.
	Association string
	// App is what was registered as the handler.
	App desktop.HandlerApp
	// Role is the role registered (desktop.Association.WithRole),
	// resolved: desktop.HandlerDefault reads as HandlerEditor for a file
	// type and HandlerViewer for a scheme.
	Role desktop.HandlerRole
	// Default: the app is the default handler.
	Default bool
}

// Registrations returns the schemes and file types registered by the app
// (and by others the test played), sorted by association, after settling
// the app.
//
//	app.Click(telatest.ByRole("button", "Open notes: links with Notes"))
//	regs := app.Registrations() // [{notes: {com.example.Notes Notes /path/notes …} viewer true}]
func (a *App) Registrations() []Registration {
	a.tb.Helper()
	a.settleAfter()
	f := a.backend.Associations()
	var out []Registration
	for _, r := range f.Registrations() {
		s, err := f.Status(platform.HandlerApp{ID: r.App.ID}, fakeAssociation(r.Association))
		out = append(out, Registration{
			Association: r.Association,
			App:         desktop.HandlerApp{ID: r.App.ID, Name: r.App.Name, Exec: r.App.Exec, Args: slices.Clone(r.App.Args), Icon: r.App.Icon},
			Role:        desktop.HandlerRole(r.Role),
			Default:     err == nil && s.Default,
		})
	}
	return out
}

// HoldAssociation plays another application, named other, holding a: the
// app's desktop.Register of it fails with desktop.ErrAssociationTaken
// unless it passes desktop.Replace. "" releases it.
func (a *App) HoldAssociation(as desktop.Association, other string) {
	a.tb.Helper()
	a.backend.Associations().SetConflict(fakeAssociation(as.String()), other)
}

// SetDefaultHandler plays the user (or another application) making the
// application holder the default handler of as; "" removes the explicit
// default.
func (a *App) SetDefaultHandler(as desktop.Association, holder string) {
	a.tb.Helper()
	a.backend.Associations().SetDefault(fakeAssociation(as.String()), holder)
}

// RegisteredApps returns the apps registered with desktop.RegisterApp and
// not unregistered, sorted by ID, after settling the app.
//
//	desktop.RegisterApp(desktop.ForApp(desktop.HandlerApp{Exec: "/opt/notes/notes"}))
//	apps := app.RegisteredApps() // [{com.example.Notes Notes /opt/notes/notes …}]
func (a *App) RegisteredApps() []desktop.HandlerApp {
	a.tb.Helper()
	a.settleAfter()
	var out []desktop.HandlerApp
	for _, r := range a.backend.Associations().Apps() {
		out = append(out, desktop.HandlerApp{ID: r.ID, Name: r.Name, Exec: r.Exec, Args: slices.Clone(r.Args), Icon: r.Icon})
	}
	return out
}

// HoldAppEntry plays another program's desktop entry of the app ID id
// (a package's), at path: desktop.RegisterApp then writes nothing unless
// it passes desktop.Replace, and desktop.AppStatus reports it. ""
// removes it.
func (a *App) HoldAppEntry(id, path string) {
	a.tb.Helper()
	a.backend.Associations().SetAppEntry(id, path)
}

// fakeAssociation returns a platform association with the key of the
// headless fake ("notes:" or ".note").
func fakeAssociation(key string) platform.Association {
	if s, ok := strings.CutSuffix(key, ":"); ok {
		return platform.Association{Scheme: s}
	}
	return platform.Association{Extension: key, MIMEType: "application/x-telatest", Description: "telatest"}
}
