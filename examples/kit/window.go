package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The page of the title bar and the window controls. A real title bar is
// the top of a frameless window, so the demos draw each one at the top of a
// small mock window, with the window state given explicitly (State): the
// gallery's own window is not frameless and has no state to show. The looks
// are named too, since Default is the one of the platform the gallery runs
// on and the page must look the same everywhere.

// The window states the demos draw. They are package variables so the
// components get the same pointer on every build and can skip it.
var (
	stateActive    = &tela.WindowState{Focused: true}
	stateInactive  = &tela.WindowState{}
	stateMaximized = &tela.WindowState{Focused: true, Maximized: true}
)

// mockWindow is a small window: the title bar over a body.
func mockWindow(bar tela.Component, body string) tela.Node {
	return Div(Class("w-full overflow-hidden rounded-lg border border-border bg-card shadow-sm"),
		bar,
		Div(Class("flex h-20 items-center justify-center text-sm text-muted-foreground"), Text(body)))
}

// ghostIcon is the icon button of a title bar.
func ghostIcon(icon tela.Icon, label string) tela.Component {
	return kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icon, AriaLabel: label}
}

func titleBarPage() tela.Node {
	return grid(
		demo("Windows", "The title at the start, the content (here a search field) in the room that is left, and flat buttons that fill the bar. Close turns red.", col,
			mockWindow(kit.TitleBar{
				Variant: kit.Windows, State: stateActive,
				Leading: ic(icons.LayoutGrid, "size-4 text-primary"),
				Title:   Text("Notes"),
				Children: kit.Input{Type: "search", Placeholder: "Search notes", AriaLabel: "Search notes",
					Class: "h-6 w-48 px-2 py-0 text-xs md:text-xs"},
				Trailing: ghostIcon(icons.Settings, "Settings"),
			}, "Window content")),
		demo("GNOME", "A tall header bar with the title centered between its start and end, and round buttons.", col,
			mockWindow(kit.TitleBar{
				Variant: kit.GNOME, State: stateActive,
				Leading:  ghostIcon(icons.Plus, "New note"),
				Title:    Text("Notes"),
				Trailing: ghostIcon(icons.Share2, "Share"),
			}, "Window content")),
		demo("macOS", "The dots at the start, glyphs on hover, and the title centered.", col,
			mockWindow(kit.TitleBar{
				Variant: kit.MacOS, State: stateActive,
				Title:    Text("Notes"),
				Trailing: ghostIcon(icons.Bell, "Notifications"),
			}, "Window content")),
		demo("Inactive", "While the window is not focused (State().Focused is false) the bar and its buttons dim.", col,
			mockWindow(kit.TitleBar{Variant: kit.Windows, State: stateInactive, Title: Text("Notes - inactive")}, "Windows"),
			mockWindow(kit.TitleBar{Variant: kit.GNOME, State: stateInactive, Title: Text("Notes - inactive")}, "GNOME"),
			mockWindow(kit.TitleBar{Variant: kit.MacOS, State: stateInactive, Title: Text("Notes - inactive")}, "macOS")),
		demo("Maximized", "The maximize button becomes Restore, and the bar loses the outline a floating window has.", col,
			mockWindow(kit.TitleBar{Variant: kit.Windows, State: stateMaximized, Title: Text("Notes - maximized")}, "Windows"),
			mockWindow(kit.TitleBar{Variant: kit.GNOME, State: stateMaximized, Title: Text("Notes - maximized")}, "GNOME")),
		demo("Window controls alone", "WindowControls in each look; HideMinimize and HideMaximize leave a button out. Each is a real button that acts on click.", col,
			Div(Class("flex flex-wrap items-center gap-x-8 gap-y-4"),
				kit.WindowControls{Variant: kit.Windows, State: stateActive},
				kit.WindowControls{Variant: kit.GNOME, State: stateActive},
				kit.WindowControls{Variant: kit.MacOS, State: stateActive}),
			Div(Class("flex flex-wrap items-center gap-x-8 gap-y-4"),
				kit.WindowControls{Variant: kit.Windows, State: stateMaximized},
				kit.WindowControls{Variant: kit.GNOME, State: stateMaximized, HideMinimize: true},
				kit.WindowControls{Variant: kit.Windows, State: stateInactive, HideMinimize: true, HideMaximize: true})),
		demo("Right to left", "Under h.Dir(\"rtl\") the bar mirrors: the controls move to the other side, and so do the start and the end.", col,
			Div(Dir("rtl"), Class("flex w-full flex-col gap-4"),
				mockWindow(kit.TitleBar{
					Variant: kit.Windows, State: stateActive,
					Leading: ic(icons.LayoutGrid, "size-4 text-primary"), Title: Text("Notes"),
					Trailing: ghostIcon(icons.Settings, "Settings"),
				}, "Window content"),
				mockWindow(kit.TitleBar{
					Variant: kit.GNOME, State: stateActive,
					Leading: ghostIcon(icons.Plus, "New note"), Title: Text("Notes"),
					Trailing: ghostIcon(icons.Share2, "Share"),
				}, "Window content"))),
	)
}
