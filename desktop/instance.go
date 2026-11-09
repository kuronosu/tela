package desktop

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Launch is a request to open something: the command-line arguments and
// the working directory of a process of the app, or the links and files
// the OS opened with it. [UseLaunch] gives the app its own start's, with
// [SingleInstance] those of every later start that another process
// forwarded (a file opened from a file manager, a deep link such as
// "notes:open?id=3" clicked in a browser), and on macOS the links and
// files Launch Services hands the app. Source says which.
type Launch struct {
	// Args are the command-line arguments after the program's name, or
	// the links and files opened (files as paths).
	Args []string
	// Dir is the working directory of the process that was started, for
	// relative paths in Args.
	Dir string
	// Source says where the launch comes from: this process's own start,
	// another process of the app, or the OS.
	Source LaunchSource
	// Forwarded is true for every launch but the start's (Source is not
	// LaunchStart).
	//
	// Deprecated: use Source, which tells a forwarded launch from one the
	// OS opened (ADR-253). Forwarded is removed in a later minor version.
	Forwarded bool
}

// LaunchSource says where a [Launch] comes from.
type LaunchSource uint8

const (
	// LaunchStart is this process's own start: its command line.
	LaunchStart LaunchSource = iota
	// LaunchForwarded is another process of the app, started while this
	// one runs, which [SingleInstance] forwarded to this one before it
	// exited: its command line.
	LaunchForwarded
	// LaunchOpened is the OS handing the running app links or files to
	// open, without another process of the app (Electron's open-url and
	// open-file events): on macOS a link clicked in another app or a file
	// opened from the Finder, the Dock or Open With, including the one
	// that started the app, which arrives just after the start (whose Args
	// are then empty); on Linux the desktop's launchers that call the
	// running instance over D-Bus (org.freedesktop.Application).
	LaunchOpened
)

func (s LaunchSource) String() string {
	switch s {
	case LaunchStart:
		return "start"
	case LaunchForwarded:
		return "forwarded"
	case LaunchOpened:
		return "opened"
	}
	return "LaunchSource(invalid)"
}

// URLs returns the arguments that are URLs, in order: any "scheme:…"
// with a scheme of at least two characters ("notes:open?id=3",
// "https://example.com"), except file: URIs, which are files. Arguments
// that start with '-' are flags, neither URLs nor files (after a "--",
// every argument counts).
func (l Launch) URLs() []string {
	var out []string
	for _, a := range l.operands() {
		if isURL(a) {
			out = append(out, a)
		}
	}
	return out
}

// Files returns the arguments that are files or folders, as absolute
// paths, in order: relative paths are made absolute against Dir, and
// file: URIs (which Linux launchers pass for a .desktop file's %U) are
// turned into paths. The files may not exist. Flags and URLs are left
// out (see URLs).
func (l Launch) Files() []string {
	var out []string
	for _, a := range l.operands() {
		switch {
		case hasScheme(a, "file"):
			if p, err := platform.FilePath(a); err == nil {
				out = append(out, p)
			}
		case isURL(a):
		case filepath.IsAbs(a):
			out = append(out, filepath.Clean(a))
		case l.Dir != "":
			out = append(out, filepath.Join(l.Dir, a))
		default:
			if p, err := filepath.Abs(a); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}

// operands returns the arguments that are not flags.
func (l Launch) operands() []string {
	var out []string
	dashes := false
	for _, a := range l.Args {
		switch {
		case dashes:
			out = append(out, a)
		case a == "--":
			dashes = true
		case a == "" || strings.HasPrefix(a, "-"):
		default:
			out = append(out, a)
		}
	}
	return out
}

// isURL reports whether a starts with a URI scheme of two characters or
// more (RFC 3986: a letter, then letters, digits, '+', '-', '.'), so a
// Windows drive ("C:\notes") is not one.
func isURL(a string) bool {
	i := strings.IndexByte(a, ':')
	if i < 2 {
		return false
	}
	for j := 0; j < i; j++ {
		c := a[j]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !(letter || j > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return false
		}
	}
	return !hasScheme(a, "file")
}

func hasScheme(a, scheme string) bool {
	return len(a) > len(scheme) && a[len(scheme)] == ':' && strings.EqualFold(a[:len(scheme)], scheme)
}

func launchOf(l platform.Launch, src LaunchSource) Launch {
	return Launch{Args: slices.Clone(l.Args), Dir: l.Dir, Source: src, Forwarded: src != LaunchStart}
}

// launchSource returns the Source of a platform LaunchEvent.
func launchSource(e platform.LaunchEvent) LaunchSource {
	if e.Source == platform.LaunchOpened {
		return LaunchOpened
	}
	return LaunchForwarded
}

// The process-wide state of the start of Run (runStart).
var instance struct {
	mu sync.Mutex
	// on: SingleInstance was called; appID is its App's AppID.
	on    bool
	appID string
	// start is this process's own launch, set when Run starts (not under
	// telatest, nor in a reload of tela dev); nil otherwise.
	start *platform.Launch
}

func init() { core.OnRunStart(runStart) }

// SingleInstance returns app made a single-instance app: when the app is
// started while it already runs, the new process hands its launch (its
// arguments and working directory) to the running one and exits with
// status 0 before opening any window (tela.Run returns nil at once). The
// running app brings its main window to the front (showing it if it was
// hidden in the tray) and gives the launch to its [UseLaunch] handlers:
//
//	func main() {
//	    tela.Main(desktop.SingleInstance(desktop.PersistApp(tela.App{
//	        AppID: "com.example.Notes",
//	        Title: "Notes",
//	        Root:  Notes{},
//	    })))
//	}
//
//	func (n Notes) Build() tela.Node {
//	    desktop.UseLaunch(func(l desktop.Launch) {
//	        for _, p := range l.Files() { openNote(p) }
//	        for _, u := range l.URLs() { openLink(u) } // notes:open?id=3
//	    })
//	    // ...
//	}
//
// Instances are told apart by the app ID ([AppID]: [SetAppID]'s, else
// app.AppID, else the executable's name), per user session: give the app
// an AppID. When the desktop cannot tell (no D-Bus session on Linux), the
// app runs on its own and a dev warning says why. Under tela dev the
// supervisor holds the claim across reloads (a second tela dev of the
// same app exits as a second instance). telatest claims nothing:
// App.InjectLaunch plays a second start.
func SingleInstance(app tela.App) tela.App {
	instance.mu.Lock()
	instance.on, instance.appID = true, app.AppID
	instance.mu.Unlock()
	if app.Root != nil {
		app.Root = instanceRoot{Root: app.Root}
	}
	return app
}

// runStart runs when tela.Run starts, before any window opens: it gives
// the OS services the app's ID (ADR-217: the portal registers global
// shortcuts under it, and under tela dev the host takes it), keeps the
// process's launch for UseLaunch and claims the single instance.
func runStart(rs core.RunStart) bool {
	rs.Backend.Desktop().SetAppID(instanceAppID(rs.AppID))
	launch := platform.CurrentLaunch()
	instance.mu.Lock()
	on, id := instance.on, instance.appID
	if !rs.Reload {
		l := launch.Clone()
		instance.start = &l
	}
	instance.mu.Unlock()
	if !on {
		return true
	}
	id = instanceAppID(id)
	primary, err := rs.Backend.Desktop().ClaimInstance(id, launch)
	if err != nil {
		// Under tela dev the host claims for the child (remote protocol
		// 5); a host without it answers ErrUnsupported, and the app runs
		// as before, without a warning on every reload.
		if rs.Dev && !(rs.DevChild && errors.Is(err, errors.ErrUnsupported)) {
			fmt.Fprintf(rs.Log, "tela: dev warning: desktop.SingleInstance: %v; the app runs on its own (another start opens another window)\n", err)
		}
		return true
	}
	return primary
}

// instanceAppID returns the app ID the single instance is claimed under:
// SetAppID's, else the App's AppID, else the executable's name, as a
// valid OS service name.
func instanceAppID(appAppID string) string {
	appID.mu.Lock()
	id := appID.set
	appID.mu.Unlock()
	if id == "" {
		id = appAppID
	}
	if id == "" {
		id = platform.DefaultAppID()
	}
	return serviceID(id)
}

// serviceID returns id as the OS services take it: spaces and characters
// that are not printable become '-' (platform.ServiceAppID).
func serviceID(id string) string { return platform.ServiceAppID(id) }

// startLaunch returns the process's own launch, if Run kept one.
func startLaunch() (platform.Launch, bool) {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	if instance.start == nil {
		return platform.Launch{}, false
	}
	return instance.start.Clone(), true
}

// instanceRoot is the root of a SingleInstance app: it raises the main
// window for each forwarded launch.
type instanceRoot struct{ Root tela.Component }

func (r instanceRoot) Build() tela.Node {
	w := tela.UseWindow()
	tela.UseMount(func() func() {
		d, ok := core.DesktopOf(w)
		if !ok {
			return nil
		}
		return d.Listen(func(e platform.Event) {
			if le, ok := e.(platform.LaunchEvent); ok {
				d.Raise(d.MainWindow(), le.Launch.Token)
			}
		})
	})
	return r.Root
}

// UseLaunch calls f, on the UI thread, with each launch of the app: once
// with the process's own start, after the component mounts
// (LaunchStart), with every later start that [SingleInstance] forwarded
// to this process (LaunchForwarded), and with the links and files the OS
// opens with the running app (LaunchOpened: macOS Launch Services, also
// the file that started the app, just after the start). Launches that
// arrive before the app's first frame wait for it, so the UseLaunch calls
// of the first frame get them. Use Launch.Files and Launch.URLs to open
// what the user asked for:
//
//	desktop.UseLaunch(func(l desktop.Launch) {
//	    for _, p := range l.Files() {
//	        openNote(p)
//	    }
//	})
//
// The process's own start is given to the UseLaunch calls mounted when
// the first of them mounts (normally the app's first frame), not to those
// that mount later, so a screen that mounts again does not reopen the
// files. telatest gives no start launch (App.InjectLaunch gives later
// ones), and neither does a reload of tela dev.
func UseLaunch(f func(Launch)) {
	if f == nil {
		panic("desktop.UseLaunch(nil): pass the function that opens what the launch names, such as desktop.UseLaunch(func(l desktop.Launch) { open(l.Files()) })")
	}
	w := tela.UseWindow()
	cur := tela.UseRef[func(Launch)](nil)
	cur.Current = f
	tela.UseMount(func() func() {
		d, ok := core.DesktopOf(w)
		if !ok {
			return nil
		}
		st := d.Local(startStateKey{}, func() any { return &startState{} }).(*startState)
		if l, ok := startLaunch(); ok && !st.given {
			cur.Current(launchOf(l, LaunchStart))
			if !st.closing {
				// Every UseLaunch mounted in this commit gets it: their
				// effects run before posted functions.
				st.closing = true
				tela.Post(func() { st.given = true })
			}
		}
		return d.Listen(func(e platform.Event) {
			if le, ok := e.(platform.LaunchEvent); ok {
				cur.Current(launchOf(le.Launch, launchSource(le)))
			}
		})
	})
}

type startStateKey struct{}

// startState is whether the app gave its start launch to UseLaunch.
type startState struct{ given, closing bool }
