package desktop

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// The desktop surfaces (ADR-298): start at login, the app's tasks and its
// recent documents. Each call blocks briefly (a file, the registry, a
// D-Bus or COM call) and is safe from any goroutine; each needs a running
// app (tela.Run, or telatest in tests, whose headless desktop records them:
// App.StartsAtLogin, App.Tasks, App.RunTask, App.RecentDocuments).

// SetStartAtLogin makes the desktop start the app when the user logs in
// (on), or stops it (off), as a settings screen's "Open at login" switch
// does: on Windows the Run key of the user's registry, on Linux an XDG
// autostart entry ($XDG_CONFIG_HOME/autostart/<AppID>.desktop; in a
// Flatpak sandbox the Background portal, which may ask the user), on
// macOS 13 and later the app's login item (SMAppService), which needs the
// app to be a bundle (tela build, tela run) and which the user may have
// to approve in System Settings › Login Items. A start at login is told
// by [Launch].AtLogin. Nothing is written when nothing changes.
//
//	kit.Switch{Label: "Open at login", Checked: atLogin, OnChange: func(on bool) {
//	    if err := desktop.SetStartAtLogin(on); err != nil {
//	        toast.Error("Could not change it", kit.ToastDescription(err.Error()))
//	        return
//	    }
//	    setAtLogin.Set(on)
//	}}
//
// Where the desktop has no such service (a macOS program outside a
// bundle) the error wraps [ErrUnsupported]. Under tela dev the program is
// a temporary build, which must not start at login: turning it on returns
// an error wrapping [ErrDevExecutable].
func SetStartAtLogin(on bool) error {
	svc, app, err := surfaceApp("SetStartAtLogin", on)
	if err != nil {
		return err
	}
	return wrap(fmt.Sprintf("SetStartAtLogin(%v)", on), svc.SetStartAtLogin(app, on))
}

// StartsAtLogin reports whether the desktop starts the app at login: what
// [SetStartAtLogin] set, unless the user turned it off since in the OS's
// own settings (Task Manager's Startup apps, the desktop's autostart
// settings, System Settings › Login Items, where an item that still waits
// for the user's approval counts as off). Read it when a settings screen
// opens, so its switch shows the truth.
func StartsAtLogin() (bool, error) {
	svc, app, err := surfaceApp("StartsAtLogin", false)
	if err != nil {
		return false, err
	}
	on, err := svc.StartsAtLogin(app)
	return on, wrap("StartsAtLogin", err)
}

// Task is one of the app's tasks ([SetTasks]): an entry of the app's
// jump list on the Windows taskbar, of its Dock menu on macOS, of its
// launcher's menu on Linux. Choosing it starts the app with Args, which
// reach the running app through [UseLaunch] (see SetTasks).
type Task struct {
	// Title is the entry's label, one line ("New note").
	Title string
	// Args are the command-line arguments the task starts the app with,
	// as [Launch].Args gives them back ("--new-note", "notes:new").
	Args []string
	// Icon is the entry's picture, the same picture at one or more sizes
	// (16 and 32 px); none shows no picture (on Windows the app's icon).
	Icon []image.Image
	// Description is a longer text, shown as the entry's tooltip on
	// Windows and macOS.
	Description string
}

// SetTasks replaces the app's tasks; nil removes them. A task is an
// entry the OS shows beside the app: in its taskbar jump list on Windows
// (also while the app is not running, once pinned), in its Dock menu on
// macOS (while it runs), in its launcher's menu on Linux (the actions of
// the desktop entry that [RegisterApp] wrote). Choosing one gives the app
// a [Launch] with the task's Args through [UseLaunch], the same way on
// every system: Windows and Linux start a process of the app with them,
// which [SingleInstance] forwards to the running app (Source
// LaunchForwarded; without SingleInstance it is a new instance of the
// app, whose own start has them); the Dock menu hands them to the running
// app directly, also LaunchForwarded.
//
//	desktop.SetTasks([]desktop.Task{
//	    {Title: "New note", Args: []string{"notes:new"}},
//	    {Title: "Search notes", Args: []string{"notes:search"}},
//	})
//
//	desktop.UseLaunch(func(l desktop.Launch) {
//	    for _, u := range l.URLs() {
//	        open(u) // notes:new, notes:search
//	    }
//	})
//
// On Linux the app needs the entry [RegisterApp] writes: without it, or
// when an installer's entry has the app's ID, the error wraps
// [ErrAppNotRegistered] and nothing is written. Under tela dev the tasks
// start the current temporary build. Invalid tasks (no Title, a Title of
// several lines) panic.
func SetTasks(tasks []Task) error {
	pt := make([]platform.Task, len(tasks))
	for i, t := range tasks {
		pt[i] = platform.Task{Title: t.Title, Description: t.Description, Args: slices.Clone(t.Args), Icon: slices.Clone(t.Icon)}
	}
	func() {
		defer func() {
			if p := recover(); p != nil {
				panic(strings.Replace(fmt.Sprint(p), "platform: SetTasks: tasks", "desktop.SetTasks: tasks", 1))
			}
		}()
		platform.CheckTasks("SetTasks", pt)
	}()
	svc, app, err := surfaceApp("SetTasks", true)
	if err != nil {
		return err
	}
	if len(pt) == 0 {
		pt = nil
	}
	return wrap("SetTasks", svc.SetTasks(app, pt))
}

// AddRecentDocument adds the file at path (relative paths are taken from
// the working directory) to the app's recent documents, as an editor does
// when the user opens or saves a file: the "Recent" category of its jump
// list on Windows, its Dock menu and the File › Open Recent menu on
// macOS, recently-used.xbel on Linux (the desktop's and the file
// choosers' Recent lists). The OS shows a file only for a type the app is
// registered for on Windows ([Register]) and declares on macOS (its
// bundle's document types). A file that does not exist is an error
// wrapping fs.ErrNotExist.
//
//	desktop.AddRecentDocument(path) // after opening it
func AddRecentDocument(path string) error {
	abs, err := filepath.Abs(path)
	if path == "" || err != nil {
		abs = path // the platform's check reports it
	}
	svc, app, err := surfaceApp("AddRecentDocument", true)
	if err != nil {
		return err
	}
	return wrap(fmt.Sprintf("AddRecentDocument(%q)", path), svc.AddRecentDocument(app, abs))
}

// ClearRecentDocuments removes the app's recent documents, as a "Clear
// Recent" menu item does. Only the app's go: the lists other applications
// keep are not touched.
func ClearRecentDocuments() error {
	svc, app, err := surfaceApp("ClearRecentDocuments", false)
	if err != nil {
		return err
	}
	return wrap("ClearRecentDocuments", svc.ClearRecentDocuments(app))
}

// surfaceApp returns the running app's services and the app as the
// desktop knows it: its ID, a name and, when needExec, its executable. A
// start at login of a tela dev build is refused (ErrDevExecutable).
func surfaceApp(what string, needExec bool) (platform.Desktop, platform.HandlerApp, error) {
	svc, _, _, ok := core.RunningServices()
	if !ok {
		return nil, platform.HandlerApp{}, fmt.Errorf("desktop.%s needs a running app (tela.Run, or telatest in tests)", what)
	}
	app := platform.HandlerApp{ID: serviceID(AppID())}
	app.Name = nameOf(app.ID)
	if needExec {
		if what == "SetStartAtLogin" && core.DevChild() {
			return nil, app, fmt.Errorf("desktop: SetStartAtLogin(true): %w", errDevStartAtLogin)
		}
		exe, err := os.Executable()
		if err != nil {
			return nil, app, fmt.Errorf("desktop: %s: the program's path: %w", what, err)
		}
		app.Exec = exe
	}
	return svc, app, nil
}

// errDevStartAtLogin wraps ErrDevExecutable for SetStartAtLogin(true)
// under tela dev.
var errDevStartAtLogin = fmt.Errorf("%w (start at login is tried from a program started with `tela run`)", ErrDevExecutable)
