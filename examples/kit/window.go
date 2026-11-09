package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
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
	tr := trFor(i18n.UseFormatter())
	notes := tr("titlebar.notes", "Notes")
	notesInactive := tr("titlebar.notesInactive", "Notes - inactive")
	notesMaximized := tr("titlebar.notesMaximized", "Notes - maximized")
	content := tr("titlebar.content", "Window content")
	settings := ghostIcon(icons.Settings, tr("titlebar.settings", "Settings"))
	newNote := ghostIcon(icons.Plus, tr("titlebar.newNote", "New note"))
	share := ghostIcon(icons.Share2, tr("titlebar.share", "Share"))
	bell := ghostIcon(icons.Bell, tr("titlebar.notifications", "Notifications"))
	return grid(
		demo("Windows", "The title at the start, the content (here a search field) in the room that is left, and flat buttons that fill the bar. Close turns red.", col,
			mockWindow(kit.TitleBar{
				Variant: kit.Windows, State: stateActive,
				Leading: ic(icons.LayoutGrid, "size-4 text-primary"),
				Title:   Text(notes),
				Children: kit.Input{Type: "search", Placeholder: tr("titlebar.search", "Search notes"), AriaLabel: tr("titlebar.search", "Search notes"),
					Class: "h-6 w-48 px-2 py-0 text-xs md:text-xs"},
				Trailing: settings,
			}, content)),
		demo("GNOME", "A tall header bar with the title centered between its start and end, and round buttons.", col,
			mockWindow(kit.TitleBar{
				Variant: kit.GNOME, State: stateActive,
				Leading:  newNote,
				Title:    Text(notes),
				Trailing: share,
			}, content)),
		demo("macOS", "The dots at the start, glyphs on hover, and the title centered.", col,
			mockWindow(kit.TitleBar{
				Variant: kit.MacOS, State: stateActive,
				Title:    Text(notes),
				Trailing: bell,
			}, content)),
		demo("Plain", "For an app with a design of its own: the platform's arrangement (here Windows, GNOME and macOS, by Look) with no background, border or outline, so the surface under the bar shows, and ghost window controls in the theme's colors.", col,
			mockWindow(kit.TitleBar{
				Variant: kit.Plain, Look: kit.Windows, State: stateActive,
				Leading:  ic(icons.LayoutGrid, "size-4 text-primary"),
				Title:    Text(notes),
				Trailing: settings,
			}, "Windows"),
			mockWindow(kit.TitleBar{
				Variant: kit.Plain, Look: kit.GNOME, State: stateActive,
				Leading:  newNote,
				Title:    Text(notes),
				Trailing: share,
			}, "GNOME"),
			mockWindow(kit.TitleBar{
				Variant: kit.Plain, Look: kit.MacOS, State: stateActive,
				Title:    Text(notes),
				Trailing: bell,
			}, "macOS"),
			mockWindow(kit.TitleBar{Variant: kit.Plain, Look: kit.GNOME, State: stateInactive, Title: Text(notesInactive)}, tr("titlebar.inactive", "Inactive"))),
		demo("Inactive", "While the window is not focused (State().Focused is false) the bar and its buttons dim.", col,
			mockWindow(kit.TitleBar{Variant: kit.Windows, State: stateInactive, Title: Text(notesInactive)}, "Windows"),
			mockWindow(kit.TitleBar{Variant: kit.GNOME, State: stateInactive, Title: Text(notesInactive)}, "GNOME"),
			mockWindow(kit.TitleBar{Variant: kit.MacOS, State: stateInactive, Title: Text(notesInactive)}, "macOS")),
		demo("Maximized", "The maximize button becomes Restore, and the bar loses the outline a floating window has.", col,
			mockWindow(kit.TitleBar{Variant: kit.Windows, State: stateMaximized, Title: Text(notesMaximized)}, "Windows"),
			mockWindow(kit.TitleBar{Variant: kit.GNOME, State: stateMaximized, Title: Text(notesMaximized)}, "GNOME")),
		demo("Window controls alone", "WindowControls in each look; HideMinimize and HideMaximize leave a button out. Each is a real button that acts on click.", col,
			Div(Class("flex flex-wrap items-center gap-x-8 gap-y-4"),
				kit.WindowControls{Variant: kit.Windows, State: stateActive},
				kit.WindowControls{Variant: kit.GNOME, State: stateActive},
				kit.WindowControls{Variant: kit.MacOS, State: stateActive}),
			Div(Class("flex flex-wrap items-center gap-x-8 gap-y-4"),
				kit.WindowControls{Variant: kit.Windows, State: stateMaximized},
				kit.WindowControls{Variant: kit.GNOME, State: stateMaximized, HideMinimize: true},
				kit.WindowControls{Variant: kit.Windows, State: stateInactive, HideMinimize: true, HideMaximize: true}),
			Div(Class("flex flex-wrap items-center gap-x-8 gap-y-4"),
				kit.WindowControls{Variant: kit.Plain, Look: kit.Windows, State: stateActive},
				kit.WindowControls{Variant: kit.Plain, Look: kit.GNOME, State: stateActive},
				kit.WindowControls{Variant: kit.Plain, Look: kit.MacOS, State: stateActive})),
		demo("Resizing a frameless window", "h.WindowResize() on the card makes the inside of its edges the window's resize handles: a band 5 px wide (tela.ResizeBand changes it) on every side, corners 16 px, drawn here in color. It is for windows whose OS draws no resize border: a transparent one (tela.App.Transparent), whose edges are the margin around the card, and any frameless one. The gallery's own window is framed, so the band would do nothing here; a snippet shows where it goes.", col,
			Div(Class("relative w-full overflow-hidden rounded-lg"),
				mockWindow(kit.TitleBar{Variant: kit.Windows, State: stateActive, Title: Text(notes)}, content),
				Div(AriaHidden(true), Class("pointer-events-none absolute inset-0 rounded-lg border-[5px] border-primary/30"))),
			snippet("Div(Class(\"absolute inset-4 rounded-2xl bg-card\"),\n\tWindowResize(),\n\tkit.TitleBar{Variant: kit.Plain},\n\t...)")),
		demo("Right to left", "Under h.Dir(\"rtl\") the bar mirrors: the controls move to the other side, and so do the start and the end.", col,
			Div(Dir("rtl"), Class("flex w-full flex-col gap-4"),
				mockWindow(kit.TitleBar{
					Variant: kit.Windows, State: stateActive,
					Leading: ic(icons.LayoutGrid, "size-4 text-primary"), Title: Text(notes),
					Trailing: settings,
				}, content),
				mockWindow(kit.TitleBar{
					Variant: kit.GNOME, State: stateActive,
					Leading: newNote, Title: Text(notes),
					Trailing: share,
				}, content))),
	)
}
