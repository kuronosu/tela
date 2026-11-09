package platform

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// HandlerApp describes the application that registers as a handler of URL
// schemes and file types ([Associations]).
type HandlerApp struct {
	// ID is the app ID (CheckAppID): it names the app's .desktop file on
	// Linux (<ID>.desktop, the file Wayland compositors match with
	// xdg_toplevel.app_id) and its ProgIDs and registered application
	// entry on Windows; on macOS it must be the bundle's
	// CFBundleIdentifier (compared without regard to case).
	ID string
	// Name is the name shown in "Open with" lists ("Notes").
	Name string
	// Exec is the absolute path of the executable. The OS runs it with
	// Args and then the URI or file paths as further arguments (on Linux
	// an app that registers both schemes and file types gets %U, so files
	// may arrive as file: URIs; the layer that parses a launch turns them
	// into paths). Needed by Register and MakeDefault, which registers
	// first; Status and Unregister ignore it, except on macOS, where Exec
	// (else the running executable) names the app bundle that every
	// method reads: it must be inside <Name>.app/Contents/MacOS/.
	Exec string
	// Args are arguments put before the URI or paths (optional). Unused on
	// macOS, where Launch Services starts the bundle and hands it the URLs
	// and files in an open event, not as arguments.
	Args []string
	// Icon is the absolute path of an icon file (optional): PNG or SVG on
	// Linux, an .ico or "exe,index" on Windows; unused on macOS (the
	// bundle's icon).
	Icon string
}

// HandlerRole is what an app does with the files of a type or the links of
// a scheme it registers for ([Association].Role): Info.plist's
// CFBundleTypeRole, and the role of Tauri's and electron-builder's file
// associations. The role is not part of an association's identity: Status,
// Unregister and a Register with another role address the same scheme or
// extension, and that Register updates what Tela wrote (changed true).
//
// Per OS:
//
//   - macOS writes it as the CFBundleTypeRole of the CFBundleDocumentTypes
//     or CFBundleURLTypes entry; HandlerNone also sets LSHandlerRank None
//     (Launch Services never uses the app to open the type).
//   - Linux and Windows have no roles: HandlerEditor, HandlerViewer and
//     HandlerShell write the same handler (the MimeType of the desktop
//     entry; the ProgID's open verb). HandlerNone declares the file type
//     without making the app a handler: on Linux the MIME package (glob,
//     description) and no MimeType entry; on Windows the ProgID's
//     description and icon (and the extension's default value, with
//     Register's rule), with no shell\open\command, no OpenWithProgids
//     entry and no Capabilities entry. Changing a registration to
//     HandlerNone removes what made the app a handler, defaults naming it
//     included, as Unregister does.
//
// HandlerNone is for file types only: a scheme with it panics
// (CheckAssociation), and so does MakeDefault with it (CheckMakeDefault).
type HandlerRole uint8

const (
	// HandlerDefault (the zero value) is HandlerEditor for a file type and
	// HandlerViewer for a scheme.
	HandlerDefault HandlerRole = iota
	// HandlerEditor: the app opens, edits and saves files of the type.
	HandlerEditor
	// HandlerViewer: the app opens files of the type, or links of the
	// scheme, without editing them.
	HandlerViewer
	// HandlerShell: the app provides runtime services for the type
	// (macOS's Shell role; elsewhere the same as HandlerEditor).
	HandlerShell
	// HandlerNone: the app declares the file type (its description and
	// icon) but does not open it.
	HandlerNone
)

func (r HandlerRole) String() string {
	switch r {
	case HandlerDefault:
		return "default"
	case HandlerEditor:
		return "editor"
	case HandlerViewer:
		return "viewer"
	case HandlerShell:
		return "shell"
	case HandlerNone:
		return "none"
	}
	return "HandlerRole(invalid)"
}

// ResolvedRole returns the role a registers with: its Role, with
// HandlerDefault replaced by HandlerViewer for a scheme and HandlerEditor
// for a file type.
func (a Association) ResolvedRole() HandlerRole {
	switch {
	case a.Role != HandlerDefault:
		return a.Role
	case a.Scheme != "":
		return HandlerViewer
	}
	return HandlerEditor
}

// Association is one URL scheme or one file extension an app handles. Set
// Scheme, or Extension (with MIMEType and Description), not both.
// Association values are comparable.
type Association struct {
	// Scheme is a URL scheme without ':' ("notes" for notes:… links):
	// a letter, then letters, digits, '+', '-' or '.' (RFC 3986), compared
	// without regard to case.
	Scheme string
	// Extension is a file extension with its dot (".note"), compared
	// without regard to case.
	Extension string
	// MIMEType is the file type's MIME type ("application/x-notes-note"),
	// required with Extension: Linux associates apps with MIME types, and
	// registers this one for the extension when no other type claims it.
	MIMEType string
	// Description names the file type ("Notes document"), required with
	// Extension.
	Description string
	// Role is what the app does with the type or the links (see
	// [HandlerRole]); the zero value, HandlerDefault, is the usual one.
	Role HandlerRole
}

func (a Association) String() string {
	if a.Scheme != "" {
		return a.Scheme + ":"
	}
	return a.Extension + " (" + a.MIMEType + ")"
}

// AssociationStatus is what Associations.Status reports.
type AssociationStatus struct {
	// Registered: the app is a candidate handler (listed in "Open with",
	// offered for the scheme); false for a type the app only declares
	// (HandlerNone). HandlerEditor, HandlerViewer and HandlerShell are not
	// told apart. For an Association with HandlerNone it reports instead
	// whether the type's declaration exists, whatever the role it was
	// registered with: on Linux the extension maps to the MIME type
	// (Tela's MIME package, or one already installed); on Windows the
	// ProgID Classes\<ID>.<ext> exists; on macOS the Info.plist declares
	// the type and Launch Services knows the bundle.
	Registered bool
	// Default: the app is the default handler; always false for an
	// Association with HandlerNone.
	Default bool
	// Other names what holds the association instead of the app: the
	// default handler (a .desktop file name, a ProgID), on Windows the
	// owner of the scheme's key, and on Linux also a desktop entry of the
	// app's ID that Tela did not write (its path) or the MIME type that
	// already claims an extension; on macOS the bundle identifier of the
	// default handler, else of another handler; "" when nothing does.
	Other string
}

// AppStatus is what Associations.AppStatus reports: whether the desktop
// knows the app's ID.
type AppStatus struct {
	// Registered: the desktop has an entry for the app's ID, whoever wrote
	// it (Linux: a desktop entry <ID>.desktop in the data directories that
	// is not Hidden; the GlobalShortcuts portal needs one). Always true on
	// Windows, which identifies a desktop app by its executable and needs
	// no entry. On macOS: Launch Services knows a bundle of the ID.
	Registered bool
	// Requested: RegisterApp wrote the entry and UnregisterApp would
	// remove it (Linux; never on macOS, where the bundle is the entry).
	Requested bool
	// Other is the path of an entry of the app's ID that Tela did not
	// write (a package's, an installer's, or the user's hidden one), which
	// RegisterApp does not take over without RegisterOptions.Replace; on
	// macOS a bundle of the ID at another path (an installed copy); ""
	// when there is none.
	Other string
}

// RegisterOptions configures Associations.Register and RegisterApp.
type RegisterOptions struct {
	// Replace lets Register take over an association another application
	// holds (ErrAssociationTaken otherwise): a Windows scheme key whose
	// command is not the app's, a Linux .desktop file of the app's ID that
	// Tela did not write, an extension already mapped to another MIME type
	// in the user's MIME database. It never makes the app the default
	// handler of a file type: that is MakeDefault. macOS has nothing to
	// take over (Launch Services keeps every app's claims), so it changes
	// nothing there.
	Replace bool
}

// DefaultResult is what Associations.MakeDefault did.
type DefaultResult uint8

const (
	// DefaultNone: nothing was done, the result of every error (the zero
	// value, so a caller that only checks the result never reads a
	// failure as success).
	DefaultNone DefaultResult = iota
	// DefaultSet: the app is now the default handler.
	DefaultSet
	// DefaultAsked: the OS does not let apps take the default, so the
	// backend opened the system's page for it (Windows "Default apps"
	// settings for file types); Status says later whether the user chose
	// the app.
	DefaultAsked
)

func (r DefaultResult) String() string {
	switch r {
	case DefaultNone:
		return "none"
	case DefaultSet:
		return "set"
	case DefaultAsked:
		return "asked"
	}
	return "DefaultResult(invalid)"
}

// ErrAssociationTaken is returned (wrapped, naming the other application)
// by Associations.Register when another application holds the association
// and RegisterOptions.Replace is not set; nothing was written.
var ErrAssociationTaken = errors.New("platform: another application holds the association")

// Associations registers an app as a handler of URL schemes and file types
// for the current user (see "Registration of schemes and file types" in
// the package documentation). Unlike the rest of Desktop, its methods do
// file or registry I/O at once, block, and are safe from any goroutine; a
// running app is not needed (package internal/platform/assoc gives the
// OS's implementation directly, for installers). Every method checks its
// arguments first (CheckHandlerApp, CheckAssociation; CheckMakeDefault for
// MakeDefault).
type Associations interface {
	// Status reports whether the app is registered for a, whether it is
	// the default, and which other application holds a.
	Status(app HandlerApp, a Association) (AssociationStatus, error)

	// Register makes the app a candidate handler of a, not the default
	// (unless nothing else handles a Windows scheme, where the scheme key
	// is the handler, or nothing else handles a on macOS, where Launch
	// Services uses the only candidate), in a's role (HandlerNone only
	// declares a file type; a registration of another role changes to
	// a's). It reads what exists first and writes nothing when nothing
	// would change (changed false). It never overwrites another
	// application's association without opts.Replace
	// (ErrAssociationTaken).
	Register(app HandlerApp, a Association, opts RegisterOptions) (changed bool, err error)

	// Unregister removes what Register and MakeDefault wrote for a: the
	// app stops being a candidate, and a default naming the app is
	// removed. Another application's entries are never touched. Nothing to
	// remove is not an error (changed false).
	Unregister(app HandlerApp, a Association) (changed bool, err error)

	// MakeDefault asks for the app to become the default handler of a,
	// registering it first if needed (with Register's rules). Where the OS
	// lets apps set it (Linux mimeapps.list, Windows schemes, macOS Launch
	// Services) it is set at once (DefaultSet); Windows file types open
	// the Default apps settings
	// page instead (DefaultAsked). On Linux a desktop-specific
	// <desktop>-mimeapps.list that names another default wins over the
	// user's mimeapps.list and belongs to the desktop environment, so
	// MakeDefault returns an error naming it instead of editing it. Every
	// error comes with DefaultNone.
	MakeDefault(app HandlerApp, a Association) (DefaultResult, error)

	// RegisterApp makes the desktop know the app's ID, with no scheme or
	// file type (ADR-217): on Linux the app's own desktop entry
	// $XDG_DATA_HOME/applications/<ID>.desktop, the file Register shares,
	// with NoDisplay=true and no MimeType, marked as asked for by
	// RegisterApp. It looks first and writes nothing when the entry is
	// up to date, or when an entry of the ID that Tela did not write
	// already names the app (changed false); a hidden one is taken over
	// only with opts.Replace (ErrAssociationTaken). On Windows it writes
	// nothing (see AppStatus.Registered). On macOS the bundle is the
	// registration: it is registered with Launch Services unless Launch
	// Services lists its path for the ID already.
	RegisterApp(app HandlerApp, opts RegisterOptions) (changed bool, err error)

	// UnregisterApp withdraws RegisterApp's request: the entry is removed
	// unless associations registered with Register still need it (they
	// keep it, and Unregister of the last one removes it). An entry Tela
	// did not write is never touched. Nothing to remove is not an error
	// (changed false). On macOS it writes nothing.
	UnregisterApp(app HandlerApp) (changed bool, err error)

	// AppStatus reports whether the desktop knows the app's ID, and
	// whether RegisterApp asked for its entry.
	AppStatus(app HandlerApp) (AppStatus, error)
}

// CheckHandlerApp panics with an actionable message if app has an invalid
// ID (CheckAppID), a Name with control characters, or, when needExec is
// set (Register, MakeDefault), no Name or an Exec that is not an absolute
// path. Exec and Icon may use Unix or Windows syntax on every OS, so the
// Windows logic can be tested anywhere. what names the call.
func CheckHandlerApp(what string, app HandlerApp, needExec bool) {
	CheckAppID(what, app.ID)
	if strings.IndexFunc(app.Name, unicode.IsControl) >= 0 {
		panicf("platform: %s: HandlerApp.Name %q has control characters", what, app.Name)
	}
	if !needExec {
		return
	}
	if strings.TrimSpace(app.Name) == "" {
		panicf("platform: %s: HandlerApp.Name is empty; it is shown in \"Open with\" lists", what)
	}
	if !absAnyOS(app.Exec) {
		panicf("platform: %s: HandlerApp.Exec %q is not an absolute path; give the executable's full path (os.Executable)", what, app.Exec)
	}
	if app.Icon != "" && !absAnyOS(strings.TrimRight(app.Icon, ",-0123456789")) {
		panicf("platform: %s: HandlerApp.Icon %q is not an absolute path", what, app.Icon)
	}
	for i, a := range app.Args {
		if strings.ContainsRune(a, 0) {
			panicf("platform: %s: HandlerApp.Args[%d] holds a NUL byte", what, i)
		}
	}
}

// absAnyOS reports whether p is an absolute Unix path, or a Windows path
// with a drive ("C:\…") or a UNC path ("\\server\…").
func absAnyOS(p string) bool {
	if strings.ContainsRune(p, 0) {
		return false
	}
	switch {
	case strings.HasPrefix(p, "/"), strings.HasPrefix(p, `\\`):
		return true
	case len(p) >= 3 && isDriveLetter(p[0]) && p[1] == ':' && (p[2] == '\\' || p[2] == '/'):
		return true
	}
	return false
}

// CheckAssociation panics with an actionable message if a is neither a
// valid scheme nor a valid file type, or has an invalid Role (HandlerNone
// with a scheme among them). what names the call.
func CheckAssociation(what string, a Association) {
	if a.Role > HandlerNone {
		panicf("platform: %s: invalid HandlerRole %d; use HandlerDefault, HandlerEditor, HandlerViewer, HandlerShell or HandlerNone", what, a.Role)
	}
	if a.Scheme != "" && a.Extension == "" && a.Role == HandlerNone {
		panicf("platform: %s: scheme %q has HandlerNone, but the handler of a scheme opens its links; register it with HandlerViewer (the default), or do not register it", what, a.Scheme)
	}
	switch {
	case a.Scheme != "" && a.Extension != "":
		panicf("platform: %s: Association has both Scheme %q and Extension %q; register them separately", what, a.Scheme, a.Extension)
	case a.Scheme != "":
		if !validScheme(a.Scheme) {
			panicf("platform: %s: invalid scheme %q; a scheme is a letter followed by letters, digits, '+', '-' or '.', without ':' (\"notes\")", what, a.Scheme)
		}
		switch strings.ToLower(a.Scheme) {
		case "http", "https", "file", "mailto", "ftp", "data", "javascript", "about":
			panicf("platform: %s: %q is a standard scheme other applications own; register a scheme of the app's own", what, a.Scheme)
		}
	case a.Extension != "":
		ext := a.Extension
		if len(ext) < 2 || len(ext) > 64 || ext[0] != '.' || strings.IndexFunc(ext[1:], func(r rune) bool {
			return !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'))
		}) >= 0 {
			panicf("platform: %s: invalid extension %q; give it with its dot, ASCII letters, digits, '-' or '_' (\".note\")", what, ext)
		}
		if !validMIME(a.MIMEType) || strings.Contains(a.MIMEType, ";") {
			panicf("platform: %s: extension %q needs a MIMEType such as \"application/x-myapp-doc\" (no parameters); got %q", what, ext, a.MIMEType)
		}
		if strings.TrimSpace(a.Description) == "" || strings.IndexFunc(a.Description, unicode.IsControl) >= 0 {
			panicf("platform: %s: extension %q needs a Description such as \"Notes document\" (no control characters)", what, ext)
		}
	default:
		panicf("platform: %s: empty Association; set Scheme or Extension", what)
	}
}

// CheckMakeDefault is CheckAssociation for MakeDefault, which also panics
// for HandlerNone: an app that does not open a type cannot be its default
// handler.
func CheckMakeDefault(what string, a Association) {
	CheckAssociation(what, a)
	if a.Role == HandlerNone {
		panicf("platform: %s: %v has HandlerNone: the app does not open it, so it cannot be its default handler; give it another role", what, a)
	}
}

func validScheme(s string) bool {
	if len(s) > 64 || s == "" || !isDriveLetter(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(isDriveLetter(c) || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// UnsupportedAssociations is an [Associations] without an OS behind it:
// every method checks its arguments and returns ErrUnsupported.
type UnsupportedAssociations struct{}

var _ Associations = UnsupportedAssociations{}

// Status checks its arguments and returns ErrUnsupported.
func (UnsupportedAssociations) Status(app HandlerApp, a Association) (AssociationStatus, error) {
	CheckHandlerApp("Associations.Status", app, false)
	CheckAssociation("Associations.Status", a)
	return AssociationStatus{}, ErrUnsupported
}

// Register checks its arguments and returns ErrUnsupported.
func (UnsupportedAssociations) Register(app HandlerApp, a Association, _ RegisterOptions) (bool, error) {
	CheckHandlerApp("Associations.Register", app, true)
	CheckAssociation("Associations.Register", a)
	return false, ErrUnsupported
}

// Unregister checks its arguments and returns ErrUnsupported.
func (UnsupportedAssociations) Unregister(app HandlerApp, a Association) (bool, error) {
	CheckHandlerApp("Associations.Unregister", app, false)
	CheckAssociation("Associations.Unregister", a)
	return false, ErrUnsupported
}

// MakeDefault checks its arguments and returns ErrUnsupported.
func (UnsupportedAssociations) MakeDefault(app HandlerApp, a Association) (DefaultResult, error) {
	CheckHandlerApp("Associations.MakeDefault", app, true)
	CheckMakeDefault("Associations.MakeDefault", a)
	return DefaultNone, ErrUnsupported
}

// RegisterApp checks its arguments and returns ErrUnsupported.
func (UnsupportedAssociations) RegisterApp(app HandlerApp, _ RegisterOptions) (bool, error) {
	CheckHandlerApp("Associations.RegisterApp", app, true)
	return false, ErrUnsupported
}

// UnregisterApp checks its arguments and returns ErrUnsupported.
func (UnsupportedAssociations) UnregisterApp(app HandlerApp) (bool, error) {
	CheckHandlerApp("Associations.UnregisterApp", app, false)
	return false, ErrUnsupported
}

// AppStatus checks its arguments and returns ErrUnsupported.
func (UnsupportedAssociations) AppStatus(app HandlerApp) (AppStatus, error) {
	CheckHandlerApp("Associations.AppStatus", app, false)
	return AppStatus{}, ErrUnsupported
}

// TakenError returns an error wrapping ErrAssociationTaken that names the
// other application, for the implementations of Register.
func TakenError(a Association, other string) error {
	return fmt.Errorf("%w: %v is held by %q (set RegisterOptions.Replace to take it over)", ErrAssociationTaken, a, other)
}
