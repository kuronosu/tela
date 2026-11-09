package desktop

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// Registration of URL schemes and file types (ADR-209, ADR-213, ADR-228).
// The owner's conditions hold on every OS: per user, without
// administrator rights; nothing is written when nothing would change;
// another application's association is never overwritten without
// [Replace]; registering never makes the app the default ([MakeDefault]
// does, when asked).

// Association is a URL scheme or a file type the app opens: build it with
// [URLScheme] or [FileType].
type Association struct {
	scheme, ext, mime, desc string
	role                    HandlerRole
}

// HandlerRole is what the app does with the files of a type, or the links
// of a scheme, it registers for ([Association.WithRole]): the role of
// macOS's CFBundleTypeRole, Tauri's and electron-builder's file
// associations.
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
	// HandlerNone: the app declares the file type, so the desktop knows
	// its name and icon (and, on Linux, detects its files), without
	// opening it: no "Open with" entry, never the default. For file types
	// only; a scheme with it panics, and so does MakeDefault.
	HandlerNone
)

func (r HandlerRole) String() string { return platform.HandlerRole(r).String() }

// WithRole returns a copy of a with the role r. macOS writes it in the
// bundle's Info.plist (CFBundleTypeRole; HandlerNone also with
// LSHandlerRank None). Linux and Windows have no roles, so Editor, Viewer
// and Shell register the same handler there; HandlerNone registers the
// type's MIME package on Linux and its ProgID (description, icon) on
// Windows, without the entries that open files with the app.
//
//	// Notes edits .note files, opens .notes-backup ones read-only, and
//	// only gives .notes-index files (which it writes) a name and an icon.
//	desktop.Register(desktop.FileType(".note", "application/x-notes-note", "Notes document"))
//	desktop.Register(desktop.FileType(".notes-backup", "application/x-notes-backup", "Notes backup").
//	    WithRole(desktop.HandlerViewer))
//	desktop.Register(desktop.FileType(".notes-index", "application/x-notes-index", "Notes index").
//	    WithRole(desktop.HandlerNone))
//
// The role is not part of what a registration is: Register with another
// role changes the registration that is there (changed true), and
// Unregister and RegistrationStatus address the scheme or extension
// whatever the role. RegistrationStatus with HandlerNone reports whether
// the type is declared; with another role, whether the app opens it.
func (a Association) WithRole(r HandlerRole) Association {
	a.role = r
	return a
}

// Role returns a's role: HandlerDefault unless WithRole set another.
func (a Association) Role() HandlerRole { return a.role }

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
	return platform.Association{Scheme: a.scheme, Extension: a.ext, MIMEType: a.mime, Description: a.desc,
		Role: platform.HandlerRole(a.role)}
}

// HandlerApp describes the application registered as a handler: what the
// OS runs to open a link or a file. Its zero fields are the running
// program's (see [RegisterOption] ForApp).
type HandlerApp struct {
	// ID is the app ID, which names the app's registration (its .desktop
	// file on Linux, its ProgIDs on Windows; on macOS it must be the
	// bundle's CFBundleIdentifier, which `tela build` takes from
	// App.AppID). Default: [AppID].
	ID string
	// Name is shown in the desktop's "Open with" lists. Default: the
	// last part of the ID, capitalized ("com.example.notes" is "Notes").
	Name string
	// Exec is the absolute path of the program to run, with Args and then
	// the link or the files. Default: os.Executable. Under tela dev the
	// executable is a temporary dev build, so Register and MakeDefault
	// there need Exec given (ErrDevExecutable otherwise); under go run it
	// is a temporary build too: register from the installed program, or
	// give its path. On macOS it must be the program inside an app bundle
	// (<Name>.app/Contents/MacOS/), as `tela build` and `tela run` make
	// it: the registration is written in the bundle's Info.plist, and
	// every call outside a bundle returns an error wrapping
	// [ErrUnsupported].
	Exec string
	// Args are arguments put before the link or the files. Unused on
	// macOS, where the link or the files reach the app as a launch, not
	// as arguments.
	Args []string
	// Icon is the absolute path of the app's icon (PNG or SVG on Linux,
	// .ico on Windows; macOS shows the bundle's own icon). Optional.
	Icon string
}

// RegisterOption configures [Register], [Unregister], [MakeDefault],
// [RegistrationStatus], [RegisterApp], [UnregisterApp] and [AppStatus]:
// [Replace] and [ForApp].
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

// ErrDevExecutable is wrapped by the error of [Register] and
// [MakeDefault] under tela dev when no Exec was given (ForApp): the
// running program is a temporary dev build, which the OS must not be told
// to run. Nothing is written. Give the installed program's path, or
// register from a normal build.
var ErrDevExecutable = errors.New("under tela dev the program is a temporary build; give the installed program's path with desktop.ForApp(desktop.HandlerApp{Exec: ...}) " +
	"(on macOS, the program inside the .app that `tela build` writes), or register from a program started with `tela run`")

// Registration is what [RegistrationStatus] reports.
type Registration struct {
	// Registered: the app is a candidate handler (offered in "Open with",
	// or for the scheme), in any role but HandlerNone. Asked about an
	// Association with HandlerNone, it reports whether the file type is
	// declared (on Linux its MIME type is known, whoever installed it; on
	// Windows the app's ProgID exists; on macOS the Info.plist declares it
	// and Launch Services knows the bundle).
	Registered bool
	// Default: the app is the default handler; always false asked with
	// HandlerNone.
	Default bool
	// Other names what holds the association instead of the app (the
	// default handler, the owner of a Windows scheme key, the MIME type
	// that claims an extension on Linux, the default app's bundle ID on
	// macOS); "" when nothing does.
	Other string
}

// DefaultResult is what [MakeDefault] did.
type DefaultResult uint8

const (
	// DefaultNone: nothing was done; MakeDefault returns it with every
	// error (the zero value: a failure never reads as DefaultSet).
	DefaultNone DefaultResult = iota
	// DefaultSet: the app is now the default handler.
	DefaultSet
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
// registered already (changed false). The app registers as an editor of a
// file type and a viewer of a scheme, unless [Association.WithRole] gives
// another role; [HandlerNone] declares a file type without opening it.
//
//	if _, err := desktop.Register(desktop.URLScheme("notes")); err != nil {
//	    log.Print(err)
//	}
//	desktop.Register(desktop.FileType(".note", "application/x-notes-note", "Notes document"))
//
// Linux writes the app's own .desktop file in ~/.local/share/applications
// (and a MIME type for a new extension); Windows writes
// HKEY_CURRENT_USER\Software\Classes; macOS adds the scheme or the
// document type (and a type declaration for a new extension) to the app
// bundle's Info.plist and registers the bundle with Launch Services, so
// the program must be in a bundle (`tela build`, `tela run`; ErrUnsupported
// otherwise). Launch Services makes the only app that handles a scheme
// or a type its handler by itself. It works without a running app (an
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
// Where the OS lets an app set it (Linux; Windows schemes; macOS, through
// Launch Services, without a prompt) it is set at once (DefaultSet). For Windows file types only the user can choose:
// MakeDefault opens the system's Default apps settings and returns
// DefaultAsked. On an error it returns DefaultNone. Call it only when the
// user asked ("Make Notes the default app for .note files").
func MakeDefault(a Association, opts ...RegisterOption) (DefaultResult, error) {
	_, app, err := regPrepare("MakeDefault", a, opts, true)
	if err != nil {
		return DefaultNone, err
	}
	r, err := associations().MakeDefault(app, a.platform())
	if err != nil {
		return DefaultNone, wrap(fmt.Sprintf("MakeDefault(%v)", a), err)
	}
	return DefaultResult(r), nil
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
	if a == (Association{}) {
		panic(fmt.Sprintf("desktop.%s: an empty Association; use desktop.URLScheme(\"notes\") or desktop.FileType(\".note\", \"application/x-notes-note\", \"Notes document\")", what))
	}
	if what == "MakeDefault" {
		platform.CheckMakeDefault("desktop."+what, a.platform())
	} else {
		platform.CheckAssociation("desktop."+what, a.platform())
	}
	return appPrepare(what+"("+a.String()+")", what, opts, needExec)
}

// appPrepare applies the options and fills the handler's defaults (Exec
// only when needExec); call names the call in errors.
func appPrepare(call, what string, opts []RegisterOption, needExec bool) (regConfig, platform.HandlerApp, error) {
	var c regConfig
	for _, o := range opts {
		if o.apply == nil {
			panic(fmt.Sprintf("desktop.%s: a zero RegisterOption; use desktop.Replace() or desktop.ForApp(...)", what))
		}
		o.apply(&c)
	}
	app := platform.HandlerApp{ID: c.app.ID, Name: c.app.Name, Exec: c.app.Exec, Args: c.app.Args, Icon: c.app.Icon}
	if app.ID == "" {
		app.ID = serviceID(AppID())
	}
	if app.Name == "" {
		app.Name = nameOf(app.ID)
	}
	if app.Exec == "" && needExec {
		if core.DevChild() {
			// tela dev runs a temporary build that the next save replaces:
			// registering it would leave links pointing at a deleted file.
			return c, app, fmt.Errorf("desktop: %s: %w", call, ErrDevExecutable)
		}
		exe, err := os.Executable()
		if err != nil {
			return c, app, fmt.Errorf("desktop: %s: the program's path: %w (give it with desktop.ForApp)", call, err)
		}
		app.Exec = exe
	}
	platform.CheckHandlerApp("desktop."+what, app, needExec)
	return c, app, nil
}

// AppRegistration is what [AppStatus] reports.
type AppRegistration struct {
	// Registered: the desktop knows the app's ID (on Linux an entry
	// <ID>.desktop exists, whoever wrote it; always on Windows, which
	// needs none; on macOS Launch Services knows a bundle of the ID).
	Registered bool
	// Requested: RegisterApp wrote the entry, and UnregisterApp would
	// remove it.
	Requested bool
	// Other is the path of an entry of the app's ID that Tela did not
	// write (a package's, an installer's), "" when there is none.
	Other string
}

// RegisterApp makes the desktop know the app, without any URL scheme or
// file type (ADR-217). On Linux it writes the app's own desktop entry,
// ~/.local/share/applications/<AppID>.desktop, per user, with
// NoDisplay=true (no launcher in the menus) and no file types: the Linux
// global shortcuts portal refuses an app without one ([GlobalShortcut]).
// Scheme and file-type registration ([Register]) share that file. It looks
// first and writes nothing when the entry is up to date or another
// program's entry of the ID exists (a package's: never taken over
// without [Replace]). On Windows, which needs no entry, it writes nothing.
// On macOS the app bundle is the registration: RegisterApp registers it
// with Launch Services when Launch Services does not know it yet, and
// UnregisterApp writes nothing.
//
//	func main() {
//	    desktop.SetAppID("com.example.notes")
//	    if _, err := desktop.RegisterApp(); err != nil {
//	        log.Print(err)
//	    }
//	    tela.Main(App)
//	}
//
// Call it from the installed program, at start: the entry runs
// os.Executable ([ForApp] gives another path, as an installer does).
// Under tela dev it returns ErrDevExecutable without Exec; under go run
// the program is a temporary build, so skip it there (on macOS it is not
// in a bundle either: ErrUnsupported, use `tela run`). It does file I/O
// and is safe from any goroutine; without a running app it uses the OS
// directly, and while an app runs (telatest included) its backend, which
// tests inspect with telatest's App.RegisteredApps.
func RegisterApp(opts ...RegisterOption) (changed bool, err error) {
	c, app, err := appPrepare("RegisterApp", "RegisterApp", opts, true)
	if err != nil {
		return false, err
	}
	changed, err = associations().RegisterApp(app, platform.RegisterOptions{Replace: c.replace})
	return changed, wrap("RegisterApp", err)
}

// UnregisterApp removes the entry RegisterApp wrote, unless schemes or
// file types registered with [Register] still need it: they keep it, and
// the [Unregister] of the last one removes it. An entry Tela did not
// write is never touched. Nothing to remove is not an error.
func UnregisterApp(opts ...RegisterOption) (changed bool, err error) {
	_, app, err := appPrepare("UnregisterApp", "UnregisterApp", opts, false)
	if err != nil {
		return false, err
	}
	changed, err = associations().UnregisterApp(app)
	return changed, wrap("UnregisterApp", err)
}

// AppStatus reports whether the desktop knows the app, and whether
// RegisterApp wrote its entry.
func AppStatus(opts ...RegisterOption) (AppRegistration, error) {
	_, app, err := appPrepare("AppStatus", "AppStatus", opts, false)
	if err != nil {
		return AppRegistration{}, err
	}
	s, err := associations().AppStatus(app)
	return AppRegistration{Registered: s.Registered, Requested: s.Requested, Other: s.Other}, wrap("AppStatus", err)
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
