package telatest

import (
	"slices"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/internal/platform"
)

// StartLaunch gives the app its own start, as the process that started it
// would see it: every desktop.UseLaunch mounted with the first one gets l
// once, with Source LaunchStart and AtStart set, and l's Args (files and
// links) and Dir. With l.AtLogin it plays the desktop starting the app
// because the user logged in (desktop.SetStartAtLogin): Launch.AtLogin
// reports it, so an app that starts in its tray opens no window. Without
// this option there is no start launch. It applies to one app, so parallel
// tests do not interfere; [App.InjectLaunch] plays the launches that come
// later.
//
//	app := telatest.MountApp(t, App, telatest.StartLaunch(desktop.Launch{AtLogin: true}))
//	app.ExpectCount(telatest.ByRole("heading", "Notes"), 0) // starts in the tray
func StartLaunch(l desktop.Launch) Option {
	return func(c *config) {
		c.start = &platform.Launch{Args: slices.Clone(l.Args), Dir: l.Dir}
		c.startAtLogin = l.AtLogin
	}
}

// AskForFileDefaults plays Windows' rule for file types: while it is on,
// desktop.MakeDefault of a file type only opens the system's Default apps
// settings and returns desktop.DefaultAsked, leaving the default to the
// user (play their choice with [App.SetDefaultHandler]); a URL scheme is
// still set at once. [App.DefaultsAsked] lists what the app asked for.
// Off by default.
//
//	app.AskForFileDefaults(true)
//	r, _ := desktop.MakeDefault(desktop.FileType(".note", "application/x-notes-note", "Notes"))
//	// r == desktop.DefaultAsked
func (a *App) AskForFileDefaults(ask bool) {
	a.tb.Helper()
	a.backend.Associations().SetAskForFileDefaults(ask)
}

// DefaultsAsked returns the file types desktop.MakeDefault asked the user
// to choose a default app for ([App.AskForFileDefaults]), such as ".note",
// in the order asked, after settling the app.
func (a *App) DefaultsAsked() []string {
	a.tb.Helper()
	a.settleAfter()
	var out []string
	for _, as := range a.backend.Associations().Asked() {
		if as.Extension != "" {
			out = append(out, as.Extension)
		} else {
			out = append(out, as.Scheme+":")
		}
	}
	return out
}
