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
// it: core draws its own message box, desktop reports the tray as
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
	// ErrUnsupported (the portal has no message box; core draws one).
	MessageBox(opts MessageBoxOptions, done func(answer MessageAnswer, err error)) error

	// Notify shows a desktop notification and returns its ID, which
	// NotificationEvents carry. Backends without actions (Win32 balloon
	// notifications) show it without its actions.
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

// MenuItemID identifies an item of a tray menu; chosen by the app and
// reported by TrayEvents.
type MenuItemID uint32

// MenuItem is an item of a tray icon's menu.
type MenuItem struct {
	// ID is reported when the item is activated. Items that can be
	// activated (neither separators nor submenus) need a non-zero ID,
	// unique in the whole menu.
	ID MenuItemID
	// Label is plain text: backends escape their mnemonic characters
	// ("&" on Win32, "_" in dbusmenu).
	Label string
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
	seen := map[MenuItemID]bool{}
	var walk func(items []MenuItem, path string)
	walk = func(items []MenuItem, path string) {
		for i, it := range items {
			at := fmt.Sprintf("%s[%d]", path, i)
			switch {
			case it.Separator:
			case len(it.Submenu) > 0:
				walk(it.Submenu, at+".Submenu")
			case it.ID == 0:
				panicf("platform: TrayOptions.%s (%q) has ID 0; items that can be activated need a non-zero ID", at, it.Label)
			case seen[it.ID]:
				panicf("platform: TrayOptions.%s (%q) reuses ID %d; item IDs must be unique in the whole menu", at, it.Label, it.ID)
			default:
				seen[it.ID] = true
			}
		}
	}
	walk(opts.Menu, "Menu")
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
// every platform) and returns ErrUnsupported. Backends without services
// return it, and partial implementations can embed it.
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
