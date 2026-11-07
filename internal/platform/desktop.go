package platform

import (
	"errors"
	"fmt"
	"image"
	"net/url"
	"strings"
)

// ErrUnsupported is returned by a service or operation the backend does not
// provide (a message box on Linux, a tray icon without a
// StatusNotifierWatcher, every service of a backend that has none yet).
// errors.Is(err, errors.ErrUnsupported) also holds. The layers above handle
// it: package desktop draws its own message box and reports the tray as
// unavailable.
var ErrUnsupported = fmt.Errorf("platform: %w", errors.ErrUnsupported)

// Desktop is the set of OS services of a backend ([Backend.Desktop]). See
// "Desktop services" in the package documentation for the common rules:
// UI thread only; asynchronous answers through the loop; invalid options
// panic; a missing service returns [ErrUnsupported].
//
// Asynchronous methods (the dialogs and the message box) return an error
// at once when the request cannot be made (unsupported, no D-Bus session);
// done is then never called. When they return nil, done is called exactly
// once, later, on the UI thread from the event loop (never from inside the
// call), unless Run returns first.
type Desktop interface {
	// OpenURI opens uri (https:, mailto:, any scheme with a handler) in
	// the user's default application. parent, if not nil, is the window
	// the request comes from (the OS uses it for focus and placement). A
	// URI without a scheme is an error; file: URIs are opened with
	// OpenPath. Failures after the request was handed to the OS (no
	// application for the scheme) are not reported.
	OpenURI(parent Window, uri string) error

	// OpenPath opens a local file or folder (an absolute or relative
	// path) in its default application, or a file manager for a folder.
	OpenPath(parent Window, path string) error

	// OpenFiles shows an open-file dialog. done receives the chosen paths
	// (one unless opts.Multiple); nil paths and a nil error mean the user
	// canceled.
	OpenFiles(opts FileDialogOptions, done func(paths []string, err error)) error

	// SaveFile shows a save-file dialog. done receives the chosen path
	// (the OS dialog asks about overwriting); "" and a nil error mean the
	// user canceled.
	SaveFile(opts FileDialogOptions, done func(path string, err error)) error

	// PickFolder shows a folder chooser. done receives the folder; "" and
	// a nil error mean the user canceled.
	PickFolder(opts FileDialogOptions, done func(path string, err error)) error

	// MessageBox shows a native message box, modal to opts.Parent when
	// set. done receives the button the user chose. Linux backends return
	// ErrUnsupported (the portal has no message box; package desktop draws one).
	MessageBox(opts MessageBoxOptions, done func(answer MessageAnswer, err error)) error

	// Notify shows a desktop notification and returns its ID, which
	// NotificationEvents carry. Backends without actions (Win32's balloon
	// fallback, before Windows 10 1809) show it without its actions.
	Notify(n Notification) (NotificationID, error)

	// CloseNotification withdraws a notification. Unknown or already
	// closed IDs are ignored (nil).
	CloseNotification(id NotificationID) error

	// NewTray adds an icon to the system tray (Win32 notification area,
	// StatusNotifierItem on Linux) and returns its ID, which TrayEvents
	// carry. Trays are removed when Run returns.
	NewTray(opts TrayOptions) (TrayID, error)

	// UpdateTray replaces a tray icon's icon, tooltip and menu. An
	// unknown or removed ID is an error.
	UpdateTray(id TrayID, opts TrayOptions) error

	// RemoveTray removes a tray icon. Unknown or already removed IDs are
	// ignored (nil).
	RemoveTray(id TrayID) error

	// --- Phase 13 (ADR-209, ADR-210). ---

	// SetAppID sets the app ID the services identify the app by to the
	// desktop (ADR-217): on Linux the ID the GlobalShortcuts portal
	// registers (org.freedesktop.host.portal.Registry, which needs the
	// desktop entry <id>.desktop), the notifications' app name and
	// desktop-entry hint and the tray items' Id. The layers above call it
	// before the first window opens, with the ID package desktop resolves
	// (desktop.SetAppID, else tela.App.AppID, else the executable's name);
	// until then backends use DefaultAppID. A later, different ID applies
	// to later requests (the portal's shortcuts are bound again under it).
	// Backends that need no such ID (Win32) ignore it. An invalid id
	// panics ([CheckAppID]).
	SetAppID(id string)

	// RegisterShortcut asks the OS for a global shortcut (see "Global
	// shortcuts" in the package documentation). When it returns nil,
	// exactly one ShortcutStatusEvent with the outcome follows through the
	// loop (ShortcutBound, or why not), even where the OS answers at once;
	// the portal may ask the user first. Registering an ID again replaces
	// its binding. It returns an error, and nothing follows, when no
	// request was made: ErrUnsupported (no global shortcuts), or an error
	// wrapping ErrAppNotRegistered (the portal refused the app ID, see
	// SetAppID). An invalid shortcut panics ([CheckShortcut]).
	RegisterShortcut(s Shortcut) error

	// UnregisterShortcut removes a global shortcut. Unknown IDs are
	// ignored (nil). No ShortcutStatusEvent follows.
	UnregisterShortcut(id string) error

	// SetSecret stores secret under name for the app appID in the OS's
	// secure storage (Credential Manager, the Secret Service; see
	// "Secrets" in the package documentation), replacing an earlier value.
	// Unlike the other methods, the secret methods block (the OS may show
	// an unlock prompt) and are safe from any goroutine; call them off the
	// UI thread. A value above MaxSecretSize returns ErrSecretTooLarge;
	// invalid names panic ([CheckSecret]).
	SetSecret(appID, name string, secret []byte) error

	// Secret returns the secret stored under name for appID, or
	// ErrSecretNotFound. Blocking, safe from any goroutine (see
	// SetSecret).
	Secret(appID, name string) ([]byte, error)

	// DeleteSecret removes the secret stored under name for appID; a
	// missing one is not an error. Blocking, safe from any goroutine (see
	// SetSecret).
	DeleteSecret(appID, name string) error

	// ClaimInstance makes this process the single instance of the app
	// appID (see "Single instance and launches" in the package
	// documentation). It works before any window exists, and may block
	// briefly (a D-Bus round trip, a message to the other process). When
	// no other process of the app holds the claim, this one takes it until
	// it exits (or Run returns) and gets the later launches as
	// LaunchEvents: primary is true. When another one holds it, launch is
	// forwarded to it and primary is false: the caller exits without
	// opening a window. An error means nothing was claimed or forwarded
	// (ErrUnsupported: no D-Bus session); the app then runs on its own.
	// Claiming again in the primary process returns true. An invalid appID
	// panics ([CheckAppID]).
	ClaimInstance(appID string, launch Launch) (primary bool, err error)

	// Associations returns the registration of URL schemes and file types
	// for the current user: the OS's (package internal/platform/assoc),
	// or the headless fake. It is never nil, and its methods are safe from
	// any goroutine (see "Registration of schemes and file types" in the
	// package documentation).
	Associations() Associations

	// MoveToTrash moves the file or folder at path (absolute; see
	// [CheckTrashPath]) to the user's trash (the Recycle Bin on Windows),
	// where the user can restore it. It never deletes: what the trash
	// cannot take stays where it is, and the call returns ErrUnsupported
	// (no trash in the session, a file on a file system the trash does
	// not cover; see "Trash" in the package documentation). A path that
	// does not exist is an error wrapping fs.ErrNotExist. Like the secret
	// methods it blocks (file I/O, a D-Bus round trip, the shell's file
	// operation) and is safe from any goroutine; call it off the UI
	// thread. (ADR-216.)
	MoveToTrash(path string) error

	// --- Platform contract v4 (ADR-224, ADR-225). ---

	// SetAppMenu sets the app's menu bar, declared once for the whole app
	// (Electron's Menu.setApplicationMenu): menu is the top-level items,
	// each usually with a Submenu. On macOS it is the menu bar of every
	// window, after the app menu that macOS always shows first; appItems
	// go in that app menu after About (Settings…), and other platforms
	// ignore them. A choice of either is an app-scoped AppMenuEvent;
	// role items may be carried out by the backend instead ([MenuRole]),
	// and accelerators are key equivalents there (MenuItem.Accelerator).
	// nil menu and appItems restore the default bar (macOS: the app,
	// Edit and Window menus). The menus are copied. Backends without an
	// app menu bar return ErrUnsupported, and the layers above draw it:
	// every backend but Cocoa, headless and remote. An invalid menu panics
	// ([CheckAppMenu]). See "Native menus" in the package documentation.
	SetAppMenu(menu, appItems []MenuItem) error
}

// FileFilter is one entry of a file dialog's type list.
type FileFilter struct {
	// Name is the label shown for the entry ("Images").
	Name string
	// Patterns are glob patterns of file names, matched without regard
	// to case where the OS does so: "*.png", "*.jpg", "*". At least one;
	// no pattern may be empty or contain ';' or a NUL byte.
	Patterns []string
}

// FileDialogOptions configures OpenFiles, SaveFile and PickFolder. Fields
// that do not apply to a dialog are ignored.
type FileDialogOptions struct {
	// Parent is the window the dialog is modal to (nil: none).
	Parent Window
	// Title is the dialog title ("" : the OS default).
	Title string
	// Directory is the folder the dialog starts in ("": the OS choice,
	// usually the last one used).
	Directory string
	// Name is the suggested file name (SaveFile).
	Name string
	// Filters is the list of file types (OpenFiles and SaveFile); the
	// first is selected. Empty: all files.
	Filters []FileFilter
	// Multiple allows choosing several files (OpenFiles).
	Multiple bool
}

// MessageKind is the icon and tone of a message box.
type MessageKind uint8

const (
	MessageInfo MessageKind = iota
	MessageWarning
	MessageError
	MessageQuestion
)

// MessageButtons is the set of buttons of a message box. The sets are the
// ones every native message box has (Win32 MessageBoxW).
type MessageButtons uint8

const (
	ButtonsOK MessageButtons = iota
	ButtonsOKCancel
	ButtonsYesNo
	ButtonsYesNoCancel
)

// MessageAnswer is the button chosen in a message box. Closing the box
// without a button (Escape, the close button) answers the set's cancel
// choice: AnswerCancel when the set has Cancel, AnswerOK for ButtonsOK and
// AnswerNo for ButtonsYesNo.
type MessageAnswer uint8

const (
	AnswerOK MessageAnswer = iota
	AnswerCancel
	AnswerYes
	AnswerNo
)

var messageAnswerNames = [...]string{"ok", "cancel", "yes", "no"}

func (a MessageAnswer) String() string {
	if int(a) < len(messageAnswerNames) {
		return messageAnswerNames[a]
	}
	return fmt.Sprintf("MessageAnswer(%d)", uint8(a))
}

// DismissAnswer returns the answer of a message box with buttons b that is
// closed without a button; see [MessageAnswer].
func (b MessageButtons) DismissAnswer() MessageAnswer {
	switch b {
	case ButtonsOK:
		return AnswerOK
	case ButtonsYesNo:
		return AnswerNo
	default:
		return AnswerCancel
	}
}

// Has reports whether the set b has the button a.
func (b MessageButtons) Has(a MessageAnswer) bool {
	switch b {
	case ButtonsOK:
		return a == AnswerOK
	case ButtonsOKCancel:
		return a == AnswerOK || a == AnswerCancel
	case ButtonsYesNo:
		return a == AnswerYes || a == AnswerNo
	case ButtonsYesNoCancel:
		return a == AnswerYes || a == AnswerNo || a == AnswerCancel
	}
	return false
}

// MessageBoxOptions configures a message box.
type MessageBoxOptions struct {
	// Parent is the window the box is modal to (nil: none).
	Parent  Window
	Kind    MessageKind
	Title   string
	Text    string
	Buttons MessageButtons
}

// NotificationID identifies a notification of this process; never 0.
type NotificationID uint32

// NotificationAction is a button of a notification.
type NotificationAction struct {
	// ID is reported by the NotificationEvent of the action. Non-empty,
	// unique in the notification, and not "default" (the freedesktop
	// name of activating the notification itself).
	ID string
	// Label is the button text.
	Label string
}

// Notification is a desktop notification.
type Notification struct {
	Title string
	Body  string
	// Icon is the notification's image at one or more sizes (see
	// [CheckIcon]); empty: the application's icon.
	Icon []image.Image
	// Actions are buttons; backends that have none drop them.
	Actions []NotificationAction
}

// TrayID identifies a tray icon of this process; never 0.
type TrayID uint32

// MenuItemID identifies an item of a menu (a tray menu, a window's menu
// bar, the app's menu bar, a popup menu); chosen by the app and reported
// by TrayEvents, MenuEvents, AppMenuEvents and the popup menu's callback.
type MenuItemID uint32

// MenuItem is an item of a menu: a tray icon's menu, a window's menu bar
// (Window.SetMenu), the app's menu bar (Desktop.SetAppMenu) or a popup
// menu (Window.PopupMenu). See [CheckMenu].
type MenuItem struct {
	// ID is reported when the item is activated. Items that can be
	// activated (neither separators nor submenus) need a non-zero ID,
	// unique in the whole menu, role items included (see [MenuRole]).
	ID MenuItemID
	// Label is plain text: backends escape their mnemonic characters
	// ("&" on Win32, "_" in dbusmenu).
	Label string
	// Accelerator is the text of the item's keyboard shortcut, shown at
	// its right ("Ctrl+S"; Accelerator.String formats one). It is display
	// text, and the app handles the key itself, except in the macOS app
	// menu bar (Desktop.SetAppMenu), where it is the item's key
	// equivalent: the key chooses the item (an AppMenuEvent, or the
	// role's native action) and does not reach the app as a KeyEvent.
	// The backend reads it there as [ParseAccelerator] does with
	// CmdOrCtrl as Command, and an accelerator it cannot parse is not a
	// key equivalent (NSMenuItem shows only key equivalents, so it is not
	// shown either). Win32 menus show it
	// after a tab; tray menus may ignore it. No control characters.
	Accelerator string
	// Role makes the item, or its submenu, a standard one the OS may
	// provide (undo, copy, the Window menu; see [MenuRole]). RoleNone,
	// the zero value, is an item of the app's own.
	Role MenuRole
	// Separator makes the item a separator line; its other fields are
	// ignored.
	Separator bool
	// Checkable items show a check mark when Checked. The backend never
	// toggles it: the app updates the menu (UpdateTray) when it handles
	// the item's TrayEvent.
	Checkable bool
	Checked   bool
	// Disabled items are shown grayed out and cannot be activated.
	Disabled bool
	// Submenu, if not empty, makes the item open a submenu (its ID is
	// then ignored).
	Submenu []MenuItem
}

// TrayOptions configures a tray icon.
type TrayOptions struct {
	// Icon is the tray image at one or more sizes; required.
	Icon []image.Image
	// Tooltip is shown when the pointer rests on the icon.
	Tooltip string
	// Menu is the context menu (secondary click; on some Linux hosts any
	// click). Empty: no menu.
	Menu []MenuItem
	// Template marks Icon as a template image (ADR-237): only its alpha
	// counts, and the OS tints it to match the menu bar (light, dark,
	// highlighted while the menu is open), as macOS status items are
	// drawn. Cocoa sets NSImage.template; the other backends ignore it
	// and show the icon's colors as they are (draw a template icon in a
	// color that reads on both light and dark panels).
	Template bool
}

// CheckFileDialogOptions panics with an actionable message if opts has an
// invalid filter. what names the call in the message.
func CheckFileDialogOptions(what string, opts FileDialogOptions) {
	for i, f := range opts.Filters {
		if len(f.Patterns) == 0 {
			panicf("platform: %s: filter %d (%q) has no patterns; give at least one, such as \"*.png\" or \"*\"", what, i, f.Name)
		}
		for _, p := range f.Patterns {
			if p == "" || strings.ContainsAny(p, ";\x00") {
				panicf("platform: %s: filter %d (%q) has invalid pattern %q; patterns are globs such as \"*.png\", without ';'", what, i, f.Name, p)
			}
		}
	}
}

// CheckMessageBoxOptions panics with an actionable message if opts has an
// invalid kind or button set.
func CheckMessageBoxOptions(opts MessageBoxOptions) {
	if opts.Kind > MessageQuestion {
		panicf("platform: MessageBox: invalid Kind %d; use one of the platform.Message… kinds", opts.Kind)
	}
	if opts.Buttons > ButtonsYesNoCancel {
		panicf("platform: MessageBox: invalid Buttons %d; use one of the platform.Buttons… sets", opts.Buttons)
	}
}

// CheckNotification panics with an actionable message if n has an invalid
// icon or action.
func CheckNotification(n Notification) {
	CheckIcon("Notification.Icon", n.Icon)
	for i, a := range n.Actions {
		if a.ID == "" || a.ID == "default" {
			panicf("platform: Notification.Actions[%d] has ID %q; action IDs must be non-empty and not \"default\" (activating the notification is NotificationActivated)", i, a.ID)
		}
		for _, b := range n.Actions[:i] {
			if b.ID == a.ID {
				panicf("platform: Notification.Actions has the ID %q twice", a.ID)
			}
		}
	}
}

// CheckTrayOptions panics with an actionable message if opts has no icon,
// an invalid one, or an invalid menu (see [MenuItem]).
func CheckTrayOptions(opts TrayOptions) {
	if len(opts.Icon) == 0 {
		panic("platform: TrayOptions.Icon is empty; a tray icon needs an image (give it at 16, 24 and 32 pixels, or more sizes)")
	}
	CheckIcon("TrayOptions.Icon", opts.Icon)
	CheckMenu("TrayOptions.Menu", opts.Menu)
}

// CheckMenu panics with an actionable message if menu has an activatable
// item (neither a separator nor a submenu, role items included) with ID
// 0 or an ID used twice in the whole menu, an Accelerator with control
// characters, an invalid Role, an item role on a submenu, a menu role on
// an item without a Submenu, or a menu role used twice (see [MenuRole]).
// what names the menu in the message.
func CheckMenu(what string, menu []MenuItem) {
	c := menuCheck{seen: map[MenuItemID]bool{}}
	c.walk(menu, what)
}

// FindMenuItem returns the activatable item of menu (submenus included)
// with the given ID.
func FindMenuItem(menu []MenuItem, id MenuItemID) (MenuItem, bool) {
	for _, it := range menu {
		switch {
		case it.Separator:
		case len(it.Submenu) > 0:
			if found, ok := FindMenuItem(it.Submenu, id); ok {
				return found, true
			}
		case it.ID == id:
			return it, true
		}
	}
	return MenuItem{}, false
}

// CheckURI returns an error if uri cannot be given to OpenURI: it does not
// parse, has no scheme, or is a file: URI (open local files with
// OpenPath). Backends call it first, so every backend refuses the same
// URIs.
func CheckURI(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("platform: OpenURI: %w", err)
	}
	switch {
	case u.Scheme == "":
		return fmt.Errorf("platform: OpenURI: %q has no scheme (such as https:)", uri)
	case strings.EqualFold(u.Scheme, "file"):
		return fmt.Errorf("platform: OpenURI: %q is a file URI; open local files with OpenPath", uri)
	}
	return nil
}

func checkDone(what string, done bool) {
	if !done {
		panicf("platform: %s called with a nil callback", what)
	}
}

// UnsupportedDesktop is a [Desktop] without any service: every method
// checks its arguments like a real backend (so programmer errors panic on
// every platform) and returns ErrUnsupported (Associations returns
// UnsupportedAssociations). Backends without services return it, and
// partial implementations can embed it.
type UnsupportedDesktop struct{}

var _ Desktop = UnsupportedDesktop{}

// OpenURI returns ErrUnsupported.
func (UnsupportedDesktop) OpenURI(Window, string) error { return ErrUnsupported }

// OpenPath returns ErrUnsupported.
func (UnsupportedDesktop) OpenPath(Window, string) error { return ErrUnsupported }

// OpenFiles checks its arguments and returns ErrUnsupported.
func (UnsupportedDesktop) OpenFiles(opts FileDialogOptions, done func([]string, error)) error {
	checkDone("OpenFiles", done != nil)
	CheckFileDialogOptions("OpenFiles", opts)
	return ErrUnsupported
}

// SaveFile checks its arguments and returns ErrUnsupported.
func (UnsupportedDesktop) SaveFile(opts FileDialogOptions, done func(string, error)) error {
	checkDone("SaveFile", done != nil)
	CheckFileDialogOptions("SaveFile", opts)
	return ErrUnsupported
}

// PickFolder checks its arguments and returns ErrUnsupported.
func (UnsupportedDesktop) PickFolder(opts FileDialogOptions, done func(string, error)) error {
	checkDone("PickFolder", done != nil)
	CheckFileDialogOptions("PickFolder", opts)
	return ErrUnsupported
}

// MessageBox checks its arguments and returns ErrUnsupported.
func (UnsupportedDesktop) MessageBox(opts MessageBoxOptions, done func(MessageAnswer, error)) error {
	checkDone("MessageBox", done != nil)
	CheckMessageBoxOptions(opts)
	return ErrUnsupported
}

// Notify checks n and returns ErrUnsupported.
func (UnsupportedDesktop) Notify(n Notification) (NotificationID, error) {
	CheckNotification(n)
	return 0, ErrUnsupported
}

// CloseNotification returns ErrUnsupported.
func (UnsupportedDesktop) CloseNotification(NotificationID) error { return ErrUnsupported }

// NewTray checks opts and returns ErrUnsupported.
func (UnsupportedDesktop) NewTray(opts TrayOptions) (TrayID, error) {
	CheckTrayOptions(opts)
	return 0, ErrUnsupported
}

// UpdateTray checks opts and returns ErrUnsupported.
func (UnsupportedDesktop) UpdateTray(_ TrayID, opts TrayOptions) error {
	CheckTrayOptions(opts)
	return ErrUnsupported
}

// RemoveTray returns ErrUnsupported.
func (UnsupportedDesktop) RemoveTray(TrayID) error { return ErrUnsupported }

// SetAppID checks id and does nothing.
func (UnsupportedDesktop) SetAppID(id string) { CheckAppID("SetAppID", id) }

// RegisterShortcut checks s and returns ErrUnsupported.
func (UnsupportedDesktop) RegisterShortcut(s Shortcut) error {
	CheckShortcut(s)
	return ErrUnsupported
}

// UnregisterShortcut returns ErrUnsupported.
func (UnsupportedDesktop) UnregisterShortcut(id string) error {
	CheckShortcutID("UnregisterShortcut", id)
	return ErrUnsupported
}

// SetSecret checks its arguments and returns ErrUnsupported.
func (UnsupportedDesktop) SetSecret(appID, name string, secret []byte) error {
	if err := CheckSecret(appID, name, secret); err != nil {
		return err
	}
	return ErrUnsupported
}

// Secret checks its arguments and returns ErrUnsupported.
func (UnsupportedDesktop) Secret(appID, name string) ([]byte, error) {
	CheckSecretName("Secret", appID, name)
	return nil, ErrUnsupported
}

// DeleteSecret checks its arguments and returns ErrUnsupported.
func (UnsupportedDesktop) DeleteSecret(appID, name string) error {
	CheckSecretName("DeleteSecret", appID, name)
	return ErrUnsupported
}

// ClaimInstance checks appID and returns ErrUnsupported.
func (UnsupportedDesktop) ClaimInstance(appID string, _ Launch) (bool, error) {
	CheckAppID("ClaimInstance", appID)
	return false, ErrUnsupported
}

// Associations returns UnsupportedAssociations.
func (UnsupportedDesktop) Associations() Associations { return UnsupportedAssociations{} }

// MoveToTrash checks path ([CheckTrashPath]) and returns ErrUnsupported,
// without looking at the file.
func (UnsupportedDesktop) MoveToTrash(path string) error {
	if _, err := CheckTrashPath(path); err != nil {
		return err
	}
	return ErrUnsupported
}

// SetAppMenu checks the menus ([CheckAppMenu]) and returns
// ErrUnsupported.
func (UnsupportedDesktop) SetAppMenu(menu, appItems []MenuItem) error {
	CheckAppMenu(menu, appItems)
	return ErrUnsupported
}

// CheckPopupMenu panics with an actionable message if a PopupMenu call has
// a nil callback, an empty menu, an invalid one ([CheckMenu]) or a
// position that is not finite.
func CheckPopupMenu(at Point, menu []MenuItem, done bool) {
	checkDone("PopupMenu", done)
	if len(menu) == 0 {
		panic("platform: PopupMenu with an empty menu; give at least one item")
	}
	CheckMenu("PopupMenu menu", menu)
	if at.X != at.X || at.Y != at.Y || at.X > 1e7 || at.X < -1e7 || at.Y > 1e7 || at.Y < -1e7 {
		panicf("platform: PopupMenu at %v; the position must be finite (logical pixels, window coordinates)", at)
	}
}
