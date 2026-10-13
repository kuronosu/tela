package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// card is the surface every panel shares: shadcn's Card.
const card = "rounded-xl border border-zinc-200 bg-white shadow-xs dark:border-zinc-800 dark:bg-zinc-900"

var navItems = []string{"Overview", "Customers", "Products", "Settings"}

// Header is the top bar: team switcher, navigation, search and user menu.
// The navigation is state; it is hidden below the md breakpoint.
func Header() tela.Node {
	active, setActive := tela.UseState("Overview")
	return Div(Class("flex h-16 shrink-0 items-center gap-6 border-b border-zinc-200 bg-white px-6 md:px-8 dark:border-zinc-800 dark:bg-zinc-900"),
		Div(Class("flex h-9 items-center gap-2 rounded-md border border-zinc-200 pr-3 pl-2 text-sm font-medium shadow-xs dark:border-zinc-800"),
			Div(Class("flex size-5 items-center justify-center rounded-md bg-linear-to-br from-violet-500 to-indigo-600 text-[10px] font-bold text-white"), Text("A")),
			Text("Acme Inc."),
			Span(Class("pl-1 text-[10px] text-zinc-400"), Text("▾")),
		),
		Div(Class("hidden items-center gap-1 md:flex"),
			Map(navItems, func(label string) tela.Node {
				return tela.WithKey(tela.CP(NavLink, NavLinkProps{Label: label, Active: label == active, Select: setActive}), label)
			}),
		),
		Div(Class("ml-auto flex items-center gap-4"),
			Div(Class("hidden h-9 w-64 items-center gap-2 rounded-md border border-zinc-200 bg-white px-3 text-sm text-zinc-400 shadow-xs lg:flex dark:border-zinc-800 dark:bg-zinc-950"),
				tela.C(SearchIcon),
				Text("Search..."),
			),
			Avatar("OM", "size-8 bg-linear-to-br from-sky-400 to-indigo-500 text-xs text-white"),
		),
	)
}

// NavLinkProps are the props of NavLink. Select is the Setter of the
// parent's state: Setters are comparable, so NavLink can skip its build.
type NavLinkProps struct {
	Label  string
	Active bool
	Select tela.Setter[string]
}

// NavLink is one item of the header navigation.
func NavLink(p NavLinkProps) tela.Node {
	cls := "text-zinc-500 hover:bg-zinc-100 hover:text-zinc-950 dark:hover:bg-zinc-800 dark:hover:text-zinc-50"
	if p.Active {
		cls = "text-zinc-950 dark:text-zinc-50"
	}
	return Button(Class("rounded-md px-3 py-2 text-sm font-medium transition-colors "+cls),
		OnClick(func(tela.MouseEvent) { p.Select.Set(p.Label) }),
		Text(p.Label),
	)
}

// SearchIcon is a magnifier drawn with a ring and a rotated handle.
func SearchIcon() tela.Node {
	return Div(Class("relative size-4 shrink-0"),
		Div(Class("absolute top-0 left-0 size-3 rounded-full border-[1.5px] border-zinc-400")),
		Div(Class("absolute right-0 bottom-0.5 h-[7px] w-[1.5px] -rotate-45 rounded-full bg-zinc-400")),
	)
}

// Avatar is a circle with initials; class sets its size and colors.
func Avatar(initials, class string) tela.Node {
	return Div(Class("flex shrink-0 items-center justify-center rounded-full font-medium "+class), Text(initials))
}

// DownloadButton is shadcn's default button.
func DownloadButton() tela.Node {
	return Button(Class("flex h-9 items-center rounded-md bg-zinc-900 px-4 text-sm font-medium text-zinc-50 shadow-sm "+
		"hover:bg-zinc-700 active:bg-zinc-950 dark:bg-zinc-50 dark:text-zinc-900 dark:hover:bg-zinc-200"),
		Text("Download"),
	)
}

var tabNames = []string{"Overview", "Analytics", "Reports", "Notifications"}

// Tabs is shadcn's pill tab list: the active tab is a white raised pill.
func Tabs() tela.Node {
	active, setActive := tela.UseState("Overview")
	return Div(Class("flex"),
		Div(Class("flex flex-wrap gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800"),
			Map(tabNames, func(name string) tela.Node {
				return tela.WithKey(tela.CP(Tab, TabProps{Label: name, Active: name == active, Select: setActive}), name)
			}),
		),
	)
}

// TabProps are the props of Tab.
type TabProps struct {
	Label  string
	Active bool
	Select tela.Setter[string]
}

// Tab is one tab of the list.
func Tab(p TabProps) tela.Node {
	cls := "text-zinc-500 hover:text-zinc-950 dark:text-zinc-400 dark:hover:text-zinc-50"
	if p.Active {
		cls = "bg-white text-zinc-950 shadow-sm dark:bg-zinc-950 dark:text-zinc-50"
	}
	return Button(Class("rounded-md px-3 py-1 text-sm font-medium "+cls),
		OnClick(func(tela.MouseEvent) { p.Select.Set(p.Label) }),
		Text(p.Label),
	)
}
