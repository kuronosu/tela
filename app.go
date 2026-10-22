package tela

import (
	"fmt"
	"io"
	"os"

	"github.com/kuronosu/tela/internal/core"
)

// App describes an application window for [Run].
//
//	tela.Run(tela.App{
//	    Title:             "Files",
//	    Root:              Files{},          // a struct component; tela.C(Files) for a function
//	    ClassicScrollbars: true, // always-visible classic bars (a Windows look)
//	    InstantScroll:     true, // no scroll animation, no inertia
//	})
type App struct {
	// Title is the window title.
	Title string
	// AppID identifies the application to the desktop environment (the
	// Wayland app_id, the X11 WM_CLASS). Use a reverse-DNS name such as
	// "com.example.notes". Empty lets the platform pick one.
	AppID string
	// Width and Height are the requested size of the window in logical
	// pixels. Zero means the platform's default size.
	Width, Height float32
	// RootClass are Tailwind classes for the root, which fills the
	// window. Use it to give the window a background ("bg-slate-900") or
	// a text style; without it the window is white and the text black.
	// The root is the viewport: content taller or wider than the window
	// scrolls, unless RootClass sets an overflow ("overflow-hidden" keeps
	// the window from scrolling; CSS's overflow propagation).
	RootClass string
	// Root is the root component: a struct component value (Root: Notes{})
	// or a function component made with [C] (Root: tela.C(Notes)). It is a
	// component of its own, so it may use hooks. A [Node] works too, but a
	// root that uses hooks must be a component.
	Root Component
	// ClassicScrollbars draws always-visible classic scrollbars (a
	// Windows look) that take space next to the content, instead of the
	// default thin overlay bars that appear while scrolling. The classes
	// scrollbar-classic and scrollbar-overlay choose per element.
	ClassicScrollbars bool
	// InstantScroll makes wheel, keyboard and scrollbar scrolling jump
	// instead of animating, and touchpad and touch scrolling stop when
	// the fingers lift instead of flinging. Elements with scroll-smooth
	// still animate; the class scroll-instant does the same for one
	// element.
	InstantScroll bool
	// ColorScheme is the app's color scheme: Light or Dark override the
	// system's preference (the dark: classes, UseColorScheme); the zero
	// value, System, follows it. SetColorScheme changes it while the app
	// runs.
	ColorScheme ColorScheme
	// Theme holds the color tokens (bg-primary, text-muted-foreground),
	// the base radius and the fonts of the app; nil means DefaultTheme,
	// so the shadcn/ui tokens always resolve. SetTheme changes it while
	// the app runs. See [Theme].
	Theme *Theme
	// ClassAliases are class names that stand for class strings, for the
	// combinations an app repeats:
	//
	//	ClassAliases: map[string]string{
	//	    "text-muted": "text-sm text-muted-foreground",
	//	    "card-title": "text-lg font-semibold dark:text-white",
	//	}
	//
	// An alias expands where it appears, so later classes still win over
	// its classes and its classes over earlier ones. Its variants apply
	// to every class of the expansion: "hover:card-title" is "hover:text-lg
	// hover:font-semibold hover:dark:text-white". An alias may use other
	// aliases; a cycle is reported like an unknown class. An alias wins
	// over a class of the same name ("text-muted" is also the muted token
	// as a text color). A key is one class name without variants, spaces
	// or brackets; Run panics otherwise. Aliases are also suggested for
	// misspelled classes.
	ClassAliases map[string]string
	// Dev turns dev mode on or off. Dev mode reports the mistakes that
	// compile: unknown classes are printed on standard error; unknown
	// roles, input types and easings panic with the closest valid name;
	// hooks called in another order or outside their component panic
	// with the fix; likely mistakes (lists without keys, nil
	// dependencies, a Value read while changes are queued...) are printed
	// as warnings. It costs some speed.
	//
	// Nil, the default, means on unless the program is a release build:
	// go run and go build in a source checkout give development builds,
	// while a binary built with -trimpath, or installed with go install
	// pkg@version, is a release build. The environment variable
	// TELA_DEV=1 or TELA_DEV=0 overrides both Dev and the default.
	//
	//	off := false
	//	tela.Main(tela.App{Title: "Notes", Root: tela.C(Notes), Dev: &off})
	Dev *bool
	// DevState keeps state that lives outside the component tree across
	// the reloads of tela dev: package variables, controllers, the
	// settings of a connection. State in hooks, text fields, scroll
	// offsets and focus is kept without it. See [DevState]; the field is
	// ignored outside tela dev.
	DevState DevState
}

// DevState is the state of an app that lives outside its component tree,
// to keep across the reloads of tela dev: package variables, a controller
// object, the settings of a connection. tela dev rebuilds the app on every
// save and runs the new build as a new process; the state of hooks, text
// fields, scroll offsets and focus is carried over by Tela itself (it is
// in the tree), and DevState carries the rest.
//
// Before it stops the old process, tela dev calls Save and keeps the bytes
// (encode them with encoding/gob, JSON or anything else). In the new
// process it calls Restore with those bytes before the root mounts, so
// the first build already sees the restored values. A Save or Restore that
// returns an error or panics is reported as a warning and that state starts
// fresh; Restore is not called when Save returned no bytes, nor on the
// first start or after a restart with fresh state (the R key,
// --no-state). Both run on the UI thread, so they need no locking against
// the components.
//
// Outside tela dev (tela.Main, tela snapshot, tests, release builds)
// DevState is ignored and neither function is called.
//
// To keep the save and restore code, and encoding/gob, out of release
// builds, give the field a variable that only a file built with the
// teladev tag assigns:
//
//	// app.go
//	var devState tela.DevState
//
//	func main() {
//	    tela.Main(tela.App{Title: "Notes", Root: Notes{}, DevState: devState})
//	}
//
//	// devstate_dev.go
//	//go:build teladev
//
//	func init() {
//	    devState = tela.DevState{
//	        Save:    func() ([]byte, error) { return json.Marshal(current) },
//	        Restore: func(b []byte) error { return json.Unmarshal(b, &current) },
//	    }
//	}
//
// tela dev builds with that tag; go build and go install do not.
type DevState struct {
	// Save returns the state to keep. Nil keeps nothing.
	Save func() ([]byte, error)
	// Restore receives what Save returned in the previous process. Nil
	// ignores it.
	Restore func([]byte) error
}

// Run opens a window, shows app.Root and runs the event loop until the
// window is closed. It must be called from the main goroutine, normally
// from main. The window is drawn by Tela's own renderer on the native
// backend: Wayland or X11 on Linux, Win32 and Cocoa elsewhere.
//
// Run returns an error when the platform cannot open a window (for
// example, no display is available). It panics when app.Root is nil.
// [Main] runs it and handles the error.
func Run(app App) error {
	if app.Root == nil {
		panic("tela: Run(App{...}) needs a Root component; for example tela.Run(tela.App{Title: \"Demo\", Root: tela.C(Counter)}) where Counter is a func() tela.Node, or Root: Counter{} for a struct component with a Build method")
	}
	return core.Run(app.options())
}

// Main runs app with [Run] and, when Run returns an error (no display,
// for example), prints it on standard error and exits with status 1. It
// is the usual body of main:
//
//	func main() {
//	    tela.Main(tela.App{Title: "Demo", Width: 800, Height: 600, Root: tela.C(Counter)})
//	}
//
// Main returns when the window is closed. Like Run, it must be called
// from the main goroutine, and it panics when app.Root is nil.
func Main(app App) {
	if err := mainRun(app); err != nil {
		fmt.Fprintf(mainStderr, "tela: %v\n", err)
		mainExit(1)
	}
}

// What Main calls, replaced by its tests.
var (
	mainRun              = Run
	mainExit             = os.Exit
	mainStderr io.Writer = os.Stderr
)

// options returns the core options of the app.
func (app App) options() core.Options {
	return core.Options{
		Config: core.Config{
			RootClass:         app.RootClass,
			ClassicScrollbars: app.ClassicScrollbars,
			InstantScroll:     app.InstantScroll,
			ColorScheme:       app.ColorScheme,
			Theme:             (*core.Theme)(app.Theme),
			ClassAliases:      app.ClassAliases,
			Dev:               core.ResolveDev(app.Dev),
		},
		Title:      app.Title,
		AppID:      app.AppID,
		Width:      app.Width,
		Height:     app.Height,
		Root:       core.NodeOf(app.Root),
		DevSave:    app.DevState.Save,
		DevRestore: app.DevState.Restore,
	}
}
