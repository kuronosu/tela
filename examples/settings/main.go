// Command settings is a preferences page: a notifications card with
// switches, an appearance card with radio-like option cards, and a footer
// with Reset and Save buttons that stay disabled until something changed.
//
// The whole form is one comparable struct in UseState; `prefs != saved`
// says whether it is dirty. The switches and option cards are struct
// components that receive the form's Setter in their fields, and the footer
// receives two Callback handles made with UseCallback, so they rebuild only
// when what they show changes. Switches and cards are buttons with hover:,
// active: and disabled: classes (Security alerts is a locked, disabled
// switch).
//
// `tela snapshot ./examples/settings` renders it without a window; add
// --dark for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Settings is the page.
func Settings() tela.Node { return tela.C(settings) }

func settings() tela.Node {
	prefs, setPrefs := tela.UseState(Default)
	saved, setSaved := tela.UseState(Default)

	save := tela.UseCallback(func() { setSaved.Set(prefs) })
	reset := tela.UseCallback(func() { setPrefs.Set(saved) })

	return Div(Class("flex min-h-screen flex-col overflow-hidden bg-zinc-50 text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50"),
		Div(Class("mx-auto flex w-full max-w-5xl flex-1 flex-col gap-6 px-6 py-8 md:px-8"),
			Div(Class("flex flex-col gap-1"),
				H1(Class("text-2xl font-bold tracking-tight"), Text("Settings")),
				P(Class("text-sm text-zinc-500 dark:text-zinc-400"), Text("Manage how Tela reaches you and how it looks on your screen.")),
			),
			Div(Class("flex flex-wrap items-stretch gap-6"),
				NotificationsCard{Prefs: prefs, Set: setPrefs},
				AppearanceCard{Prefs: prefs, Set: setPrefs},
			),
			Footer{Dirty: prefs != saved, Save: save, Reset: reset},
		),
	)
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return Settings() }

func main() {
	tela.Main(tela.App{
		Title:  "Tela — settings",
		AppID:  "dev.telaui.settings",
		Width:  1280,
		Height: 800,
		Root:   Settings(),
	})
}
