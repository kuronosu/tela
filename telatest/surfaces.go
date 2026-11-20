package telatest

import (
	"slices"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/internal/platform"
)

// The desktop surfaces in tests (ADR-298): start at login, the app's
// tasks and its recent documents, which the headless desktop records for
// the app's ID instead of writing them to the system.

// surfaceAppID returns the app ID the surfaces are kept under.
func surfaceAppID() string { return platform.ServiceAppID(desktop.AppID()) }

// StartsAtLogin reports whether the app asked the desktop to start it at
// login (desktop.SetStartAtLogin), after settling the app; also what
// SetStartsAtLogin set.
//
//	app.Click(telatest.ByRole("switch", "Open at login"))
//	if !app.StartsAtLogin() {
//	    t.Error("the switch did not turn start at login on")
//	}
func (a *App) StartsAtLogin() bool {
	a.tb.Helper()
	a.settleAfter()
	return a.backend.StartAtLogin(surfaceAppID())
}

// SetStartsAtLogin plays the user switching the app's start at login in
// the OS's own settings (Task Manager, System Settings › Login Items):
// desktop.StartsAtLogin reports on from now on.
func (a *App) SetStartsAtLogin(on bool) {
	a.tb.Helper()
	a.mustRun()
	a.backend.SetStartsAtLogin(surfaceAppID(), on)
}

// Tasks returns the app's tasks (desktop.SetTasks), in order, after
// settling the app; nil without any.
//
//	if got := app.Tasks(); len(got) != 2 || got[0].Title != "New note" {
//	    t.Errorf("tasks %+v", got)
//	}
func (a *App) Tasks() []desktop.Task {
	a.tb.Helper()
	a.settleAfter()
	return tasksOf(a.backend.Tasks(surfaceAppID()))
}

func tasksOf(pt []platform.Task) []desktop.Task {
	if len(pt) == 0 {
		return nil
	}
	out := make([]desktop.Task, len(pt))
	for i, t := range pt {
		out[i] = desktop.Task{Title: t.Title, Args: slices.Clone(t.Args), Icon: slices.Clone(t.Icon), Description: t.Description}
	}
	return out
}

// RunTask plays the user choosing the app's task titled title in its jump
// list, Dock menu or launcher: the app gets a launch with the task's Args
// through desktop.UseLaunch, Source LaunchForwarded, as on every system
// (Windows and Linux start a second process, which desktop.SingleInstance
// forwards; macOS's Dock menu hands it over directly), and the app
// settles. It fails the test when the app has no such task.
//
//	app.RunTask("New note")
//	app.ExpectText("Untitled note")
func (a *App) RunTask(title string) {
	a.tb.Helper()
	a.mustRun()
	a.settleAfter()
	for _, t := range a.backend.Tasks(surfaceAppID()) {
		if t.Title == title {
			a.InjectLaunch(desktop.Launch{Args: t.Args})
			return
		}
	}
	var titles []string
	for _, t := range a.backend.Tasks(surfaceAppID()) {
		titles = append(titles, t.Title)
	}
	a.fatalf("RunTask(%q): the app has no such task (its tasks: %q; set them with desktop.SetTasks)", title, titles)
}

// RecentDocuments returns the app's recent documents
// (desktop.AddRecentDocument), most recent first, as absolute paths, after
// settling the app; ClearRecentDocuments empties it.
//
//	app.Click(telatest.ByRole("button", "Open"))
//	if got := app.RecentDocuments(); len(got) == 0 || got[0] != path {
//	    t.Errorf("recent %v", got)
//	}
func (a *App) RecentDocuments() []string {
	a.tb.Helper()
	a.settleAfter()
	return a.backend.RecentDocuments(surfaceAppID())
}

// FailSurfaces makes every call of the desktop surfaces
// (desktop.SetStartAtLogin, StartsAtLogin, SetTasks, AddRecentDocument,
// ClearRecentDocuments) fail with err from now on, until
// FailSurfaces(nil), and nothing is recorded: a registry or a folder the
// user may not write. (A desktop without one of them is
// App.Unsupported(telatest.StartAtLogin), Tasks or RecentDocuments.)
func (a *App) FailSurfaces(err error) {
	a.tb.Helper()
	a.backend.SetSurfaceError(err)
}
