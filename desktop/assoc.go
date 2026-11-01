package desktop

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// Registration of URL schemes and file types (ADR-209, ADR-213). The
// owner's conditions hold on every OS: per user, without administrator
// rights; nothing is written when nothing would change; another
// application's association is never overwritten without [Replace];
// registering never makes the app the default ([MakeDefault] does, when
// asked).

// Association is a URL scheme or a file type the app opens: build it with
// [URLScheme] or [FileType].
type Association struct {
	scheme, ext, mime, desc string
}

// URLScheme returns the association of a URL scheme of the app's own, given
// without ':' ("notes" for notes:open?id=3 links): a letter, then
// letters, digits, '+', '-' or '.'. Standard schemes (http, mailto…)
// belong to other applications and panic.
func URLScheme(name string) Association { return Association{scheme: name} }

// FileType returns the association of a file extension, given with its
// dot (".note"), with its MIME type ("application/x-notes-note") and a
// description of the type in the user's language ("Notes document"), all
// required: Linux associates apps with MIME types, and registers this one
// for the extension when no other type claims it.
func FileType(extension, mimeType, description string) Association {
	return Association{ext: extension, mime: mimeType, desc: description}
}

// String returns "notes:" for a scheme, ".note" for a file type.
func (a Association) String() string {
	if a.scheme != "" {
		return a.scheme + ":"
	}
	return a.ext
}

func (a Association) platform() platform.Association {
	return platform.Association{Scheme: a.scheme, Extension: a.ext, MIMEType: a.mime, Description: a.desc}
}

// HandlerApp describes the application registered as a handler: what the
// OS runs to open a link or a file. Its zero fields are the running
// program's (see [RegisterOption] ForApp).
type HandlerApp struct {
	// ID is the app ID, which names the app's registration (its .desktop
	// file on Linux, its ProgIDs on Windows). Default: [AppID].
	ID string
	// Name is shown in the desktop's "Open with" lists. Default: the
	// last part of the ID, capitalized ("com.example.notes" is "Notes").
	Name string
	// Exec is the absolute path of the program to run, with Args and then
	// the link or the files. Default: os.Executable. Under go run and tela
	// dev it is a temporary build: register from the installed program,
	// or give its path.
	Exec string
	// Args are arguments put before the link or the files.
	Args []string
	// Icon is the absolute path of the app's icon (PNG or SVG on Linux,
	// .ico on Windows). Optional.
	Icon string
}

// RegisterOption configures [Register], [Unregister], [MakeDefault] and
// [RegistrationStatus]: [Replace] and [ForApp].
type RegisterOption struct{ apply func(*regConfig) }

type regConfig struct {
	replace bool
	app     HandlerApp
}

// Replace lets [Register] (and MakeDefault, which registers first) take
// over an association another application holds: a Windows scheme key
// whose command runs another program, a Linux desktop entry of the app's
// ID that Tela did not write, an extension another MIME type claims.
// Without it Register returns an error wrapping [ErrAssociationTaken] and
// writes nothing. Ask the user before passing it.
func Replace() RegisterOption {
	return RegisterOption{func(c *regConfig) { c.replace = true }}
}

// ForApp sets what is registered instead of the running program: an
// installer registers the program it installed. Fields left empty keep
// their defaults (see [HandlerApp]).
//
//	desktop.Register(desktop.URLScheme("notes"), desktop.ForApp(desktop.HandlerApp{
//	    ID:   "com.example.Notes",
//	    Name: "Notes",
//	    Exec: `C:\Program Files\Notes\notes.exe`,
//	    Icon: `C:\Program Files\Notes\notes.ico`,
//	}))
func ForApp(app HandlerApp) RegisterOption {
	app.Args = slices.Clone(app.Args)
	return RegisterOption{func(c *regConfig) { c.app = app }}
}

// ErrAssociationTaken is wrapped by the error of [Register] when another
// application holds the association and [Replace] was not given; nothing
// was written. The error names the other application.
var ErrAssociationTaken = platform.ErrAssociationTaken

// Registration is what [RegistrationStatus] reports.
type Registration struct {
	// Registered: the app is a candidate handler (offered in "Open with",
	// or for the scheme).
	Registered bool
	// Default: the app is the default handler.
	Default bool
	// Other names what holds the association instead of the app (the
	// default handler, the owner of a Windows scheme key, the MIME type
	// that claims an extension on Linux); "" when nothing does.
	Other string
}

// DefaultResult is what [MakeDefault] did.
type DefaultResult uint8

const (
	// DefaultSet: the app is now the default handler.
	DefaultSet DefaultResult = iota
	// DefaultAsked: the OS lets only the user choose (Windows file
	// types), so the system's Default apps settings opened; tell the user
	// to pick the app there, and check later with RegistrationStatus.
	DefaultAsked
)

func (r DefaultResult) String() string { return platform.DefaultResult(r).String() }

// Register makes the app a candidate handler of a URL scheme or a file
// type, for the current user (no administrator rights): the app appears
// in "Open with", and links of its scheme open it when no other
// application is the default. It does not make it the default
// ([MakeDefault]). It looks first, and writes nothing when the app is
// registered already (changed false).
//
//	if _, err := desktop.Register(desktop.URLScheme("notes")); err != nil {
//	    log.Print(err)
//	}
//	desktop.Register(desktop.FileType(".note", "application/x-notes-note", "Notes document"))
//
// Linux writes the app's own .desktop file in ~/.local/share/applications
// (and a MIME type for a new extension); Windows writes
// HKEY_CURRENT_USER\Software\Classes. It works without a running app (an
// installer, a setup command), and is safe from any goroutine; it does
// file or registry I/O, so an app calls it from a goroutine or at start.
// The OS opens the app with the link or the files as arguments: read
// them with [UseLaunch] (with [SingleInstance], in the running app).
// While an app runs (telatest included), the calls go to its backend,
// which tests inspect with telatest's App.Registrations.
func Register(a Association, opts ...RegisterOption) (changed bool, err error) {
	c, app, err := regPrepare("Register", a, opts, true)
	if err != nil {
		return false, err
	}
	changed, err = associations().Register(app, a.platform(), platform.RegisterOptions{Replace: c.replace})
	return changed, wrap(fmt.Sprintf("Register(%v)", a), err)
}

// Unregister removes what Register and MakeDefault wrote for a: the app
// is no longer a candidate, and a default that names it is removed.
// Another application's entries are never touched. Nothing to remove is
// not an error (changed false).
func Unregister(a Association, opts ...RegisterOption) (changed bool, err error) {
	_, app, err := regPrepare("Unregister", a, opts, false)
	if err != nil {
		return false, err
	}
	changed, err = associations().Unregister(app, a.platform())
	return changed, wrap(fmt.Sprintf("Unregister(%v)", a), err)
}

// MakeDefault asks for the app to become the default handler of a,
// registering it first if needed (with Register's rules and options).
// Where the OS lets an app set it (Linux; Windows schemes) it is set at
// once (DefaultSet). For Windows file types only the user can choose:
// MakeDefault opens the system's Default apps settings and returns
// DefaultAsked. Call it only when the user asked ("Make Notes the default
// app for .note files").
func MakeDefault(a Association, opts ...RegisterOption) (DefaultResult, error) {
	_, app, err := regPrepare("MakeDefault", a, opts, true)
	if err != nil {
		return DefaultSet, err
	}
	r, err := associations().MakeDefault(app, a.platform())
	return DefaultResult(r), wrap(fmt.Sprintf("MakeDefault(%v)", a), err)
}

// RegistrationStatus reports whether the app is registered for a,
// whether it is the default, and what else holds a.
func RegistrationStatus(a Association, opts ...RegisterOption) (Registration, error) {
	_, app, err := regPrepare("RegistrationStatus", a, opts, false)
	if err != nil {
		return Registration{}, err
	}
	s, err := associations().Status(app, a.platform())
	return Registration{Registered: s.Registered, Default: s.Default, Other: s.Other},
		wrap(fmt.Sprintf("RegistrationStatus(%v)", a), err)
}

// regPrepare applies the options, checks a and fills the handler's
// defaults (Exec only when needExec).
func regPrepare(what string, a Association, opts []RegisterOption, needExec bool) (regConfig, platform.HandlerApp, error) {
	var c regConfig
	for _, o := range opts {
		if o.apply == nil {
			panic(fmt.Sprintf("desktop.%s: a zero RegisterOption; use desktop.Replace() or desktop.ForApp(...)", what))
		}
		o.apply(&c)
	}
	if a == (Association{}) {
		panic(fmt.Sprintf("desktop.%s: an empty Association; use desktop.URLScheme(\"notes\") or desktop.FileType(\".note\", \"application/x-notes-note\", \"Notes document\")", what))
	}
	platform.CheckAssociation("desktop."+what, a.platform())
	app := platform.HandlerApp{ID: c.app.ID, Name: c.app.Name, Exec: c.app.Exec, Args: c.app.Args, Icon: c.app.Icon}
	if app.ID == "" {
		app.ID = serviceID(AppID())
	}
	if app.Name == "" {
		app.Name = nameOf(app.ID)
	}
	if app.Exec == "" && needExec {
		exe, err := os.Executable()
		if err != nil {
			return c, app, fmt.Errorf("desktop: %s(%v): the program's path: %w (give it with desktop.ForApp)", what, a, err)
		}
		app.Exec = exe
	}
	platform.CheckHandlerApp("desktop."+what, app, needExec)
	return c, app, nil
}

// nameOf returns the name shown for an app ID: its last dot-separated
// part, with its first letter upper-cased ("com.example.notes" is
// "Notes").
func nameOf(id string) string {
	n := id
	if i := strings.LastIndexByte(n, '.'); i >= 0 && i < len(n)-1 {
		n = n[i+1:]
	}
	for i, r := range n {
		return string(unicode.ToUpper(r)) + n[i+len(string(r)):]
	}
	return id
}

// associations returns the running app's registration (the headless fake
// in tests), else the OS's.
func associations() platform.Associations {
	if svc, _, _, ok := core.RunningServices(); ok {
		return svc.Associations()
	}
	return assoc.Native()
}
