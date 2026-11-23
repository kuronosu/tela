package lint

// Migrations are the rewrites of the changes that broke the public API,
// newest last (ADR-305). A release that breaks the API adds its entry here
// in the same change, and cites it from the "Breaking" entry of
// CHANGELOG.md as migrate: ID. A break with nothing mechanical to rewrite
// (a changed default, a behavior) says so there instead: migrate: none --
// reason.
//
// Rewrites exist from v0.14.0 on. Breaking changes of earlier releases are
// not here: the project had no outside users yet (ADR-208).
var Migrations = []Migration{
	{
		ID:     "desktop-launch-forwarded",
		Since:  "v0.14.0",
		Note:   "desktop.Launch.Forwarded was removed in v0.14.0: Launch.Source tells a forwarded launch (LaunchForwarded) from the app's own start and from one the OS opened",
		Custom: launchForwarded,
		Example: Example{
			Old: `package example

import "github.com/kuronosu/tela/desktop"

func forwarded(l desktop.Launch) bool { return l.Forwarded }

func fresh(l desktop.Launch) bool { return !l.Forwarded && len(l.Args) > 0 }

func both(a, b desktop.Launch) bool { return a.Forwarded == b.Forwarded }

func build() desktop.Launch { return desktop.Launch{Dir: "/tmp", Forwarded: true} }
`,
			New: `package example

import "github.com/kuronosu/tela/desktop"

func forwarded(l desktop.Launch) bool { return l.Source == desktop.LaunchForwarded }

func fresh(l desktop.Launch) bool { return l.Source != desktop.LaunchForwarded && len(l.Args) > 0 }

func both(a, b desktop.Launch) bool { return (a.Source == desktop.LaunchForwarded) == (b.Source == desktop.LaunchForwarded) }

func build() desktop.Launch { return desktop.Launch{Dir: "/tmp", Source: desktop.LaunchForwarded} }
`,
		},
	},
	{
		ID:     "telatest-app-move",
		Since:  "v0.14.1",
		Note:   "telatest.App.Move was renamed MoveWindow in v0.14.1 (Move moves the window; MouseMove moves the pointer)",
		Member: &MemberRename{Pkg: "telatest", Type: "App", Old: "Move", New: "MoveWindow"},
		Example: Example{
			Old: `package example

import "github.com/kuronosu/tela/telatest"

func move(a *telatest.App) {
	a.Move(10, 20)
	f := a.Move
	f(30, 40)
}
`,
			New: `package example

import "github.com/kuronosu/tela/telatest"

func move(a *telatest.App) {
	a.MoveWindow(10, 20)
	f := a.MoveWindow
	f(30, 40)
}
`,
		},
	},
}
