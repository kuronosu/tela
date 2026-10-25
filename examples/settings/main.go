// Command settings is a preferences page built on package kit in its default
// style: a notifications card with switches, an appearance card with a
// radio group of theme cards and a segmented control, and a footer with
// Reset and Save buttons that stay disabled until something changed. Reset
// asks for confirmation with an alert dialog and Save shows a toast.
//
// The whole form is one comparable struct in UseState; `prefs != saved`
// says whether it is dirty. The rows and cards are struct components that
// receive the form's Setter in their fields, and the footer receives two
// Callback handles made with UseCallback, so they rebuild only when what
// they show changes. The look comes from the kit's recipes and the theme
// tokens (bg-background, text-muted-foreground), so the page follows the
// color scheme with no dark: classes.
//
// `tela snapshot ./examples/settings` renders it without a window; add
// --dark for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Settings is the page, inside the Toaster that Save shows its toast in.
func Settings() tela.Component { return kit.Toaster{Children: tela.C(settings)} }

func settings() tela.Node {
	prefs, setPrefs := tela.UseState(Default)
	saved, setSaved := tela.UseState(Default)
	toast := kit.UseToast()

	save := tela.UseCallback(func() {
		setSaved.Set(prefs)
		toast.Success("Settings saved", kit.ToastDescription("Your preferences were updated."))
	})
	reset := tela.UseCallback(func() { setPrefs.Set(saved) })

	return Div(Class("flex min-h-screen flex-col overflow-hidden"),
		Div(Class("mx-auto flex w-full max-w-5xl flex-1 flex-col gap-6 px-6 py-8 md:px-8"),
			Div(Class("flex flex-col gap-1"),
				H1(Class("text-2xl font-bold tracking-tight"), Text("Settings")),
				P(Class("text-sm text-muted-foreground"), Text("Manage how Tela reaches you and how it looks on your screen.")),
			),
			Div(Class("flex flex-wrap items-stretch gap-6"),
				NotificationsCard{Prefs: prefs, Set: setPrefs},
				AppearanceCard{Prefs: prefs, Set: setPrefs},
			),
			Footer{Dirty: prefs != saved, Save: save, Reset: reset},
		),
	)
}

var theme = kit.Theme()

// App is the window; `tela snapshot` and the tests read it too.
var App = tela.App{
	Title:     "Tela — settings",
	AppID:     "dev.telaui.settings",
	Width:     1280,
	Height:    800,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Settings(),
}

func main() { tela.Main(App) }
