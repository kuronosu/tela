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
	// entry on Windows.
	ID string
	// Name is the name shown in "Open with" lists ("Notes").
	Name string
	// Exec is the absolute path of the executable. The OS runs it with
	// Args and then the URI or file paths as further arguments. Needed by
	// Register; Status, Unregister and MakeDefault ignore it.
	Exec string
	// Args are arguments put before the URI or paths (optional).
	Args []string
	// Icon is the absolute path of an icon file (optional): PNG or SVG on
	// Linux, an .ico or "exe,index" on Windows.
	Icon string
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
	// offered for the scheme).
	Registered bool
	// Default: the app is the default handler.
	Default bool
	// Other names another application that holds the association: the
	// default handler (a .desktop file name, a ProgID) or, on Windows, the
	// owner of the scheme's key; "" when none does.
	Other string
}

// RegisterOptions configures Associations.Register.
type RegisterOptions struct {
	// Replace lets Register take over an association another application
	// holds (ErrAssociationTaken otherwise): a Windows scheme key whose
	// command is not the app's, a Linux .desktop file of the app's ID that
	// Tela did not write, an extension already mapped to another MIME type
	// in the user's MIME database. It never makes the app the default
	// handler of a file type: that is MakeDefault.
	Replace bool
}

// DefaultResult is what Associations.MakeDefault did.
type DefaultResult uint8

const (
	// DefaultSet: the app is now the default handler.
	DefaultSet DefaultResult = iota
	// DefaultAsked: the OS does not let apps take the default, so the
	// backend opened the system's page for it (Windows "Default apps"
	// settings for file types); Status says later whether the user chose
	// the app.
	DefaultAsked
)

func (r DefaultResult) String() string {
	switch r {
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
// arguments first (CheckHandlerApp, CheckAssociation).
type Associations interface {
	// Status reports whether the app is registered for a, whether it is
	// the default, and which other application holds a.
	Status(app HandlerApp, a Association) (AssociationStatus, error)

	// Register makes the app a candidate handler of a, not the default
	// (unless nothing else handles a Windows scheme, where the scheme key
	// is the handler). It reads what exists first and writes nothing when
	// nothing would change (changed false). It never overwrites another
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
	// lets apps set it (Linux mimeapps.list, Windows schemes) it is set at
	// once (DefaultSet); Windows file types open the Default apps settings
	// page instead (DefaultAsked).
	MakeDefault(app HandlerApp, a Association) (DefaultResult, error)
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
// valid scheme nor a valid file type. what names the call.
func CheckAssociation(what string, a Association) {
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
	CheckAssociation("Associations.MakeDefault", a)
	return DefaultSet, ErrUnsupported
}

// TakenError returns an error wrapping ErrAssociationTaken that names the
// other application, for the implementations of Register.
func TakenError(a Association, other string) error {
	return fmt.Errorf("%w: %v is held by %q (set RegisterOptions.Replace to take it over)", ErrAssociationTaken, a, other)
}
