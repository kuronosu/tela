package main

import (
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Nav is an item of the sidebar.
type Nav struct{ Label, Icon string }

var navItems = []Nav{
	{"Overview", "grid"}, {"Analytics", "chart"}, {"Customers", "users"},
	{"Products", "box"}, {"Messages", "chat"}, {"Settings", "sliders"},
}

// Sidebar is the navigation column: sticky, as tall as the window, so it
// stays while the page scrolls. It is glass too, over the same backdrop.
func Sidebar() tela.Node { return tela.C(sidebar) }

func sidebar() tela.Node {
	active, setActive := tela.UseState("Overview")
	return Div(Class("sticky top-0 hidden h-screen flex-col gap-6 self-start border-r border-white/50 bg-white/40 p-5",
		"lg:flex dark:border-white/10 dark:bg-slate-950/30"),
		Div(Class("flex items-center gap-3 px-1 pt-1"),
			Div(Class("flex size-10 items-center justify-center rounded-xl bg-linear-to-br from-indigo-500 via-fuchsia-500 to-pink-500 shadow-lg shadow-fuchsia-500/30"),
				Div(Class("size-4 rotate-45 rounded-[5px] border-2 border-white"))),
			Div(
				Div(Class("text-lg leading-5", heading), Text("Aurora")),
				Div(Class("text-xs", muted), Text("Analytics")),
			),
		),
		Div(Class("flex flex-col gap-1"),
			Div(Class("px-3 pb-1 text-[11px] font-semibold tracking-widest uppercase", muted), Text("Menu")),
			MapKeyed(navItems, func(n Nav) string { return n.Label }, func(n Nav) tela.Component {
				return NavItem{Nav: n, Active: n.Label == active, Select: setActive}
			}),
		),
		Div(Class("mt-auto flex flex-col gap-4"),
			Div(Class("overflow-hidden rounded-2xl bg-linear-to-br from-indigo-500 to-fuchsia-500 p-4 text-white shadow-lg shadow-indigo-500/30"),
				Div(Class("text-sm font-semibold"), Text("Pro plan")),
				Div(Class("mt-0.5 text-xs text-white/80"), Text("7 of 10 seats in use")),
				Div(Class("mt-3 h-1.5 rounded-full bg-white/25"), Div(Class("h-full w-[70%] rounded-full bg-white"))),
				Button(Class("mt-4 w-full rounded-lg bg-white/20 py-1.5 text-xs font-medium backdrop-blur transition duration-200 hover:bg-white/30 active:scale-95"),
					Text("Add seats")),
			),
			Div(Class("flex items-center gap-3 px-1"),
				Avatar{Initials: "OM", Size: "size-9", Tint: "from-sky-400 to-indigo-500"},
				Div(Class("min-w-0"),
					Div(Class("truncate text-sm font-medium"), Text("Olivia Martin")),
					Div(Class("truncate text-xs", muted), Text("olivia@aurora.dev")),
				),
			),
		),
	)
}

// NavItem is one entry. The active one renders the highlight, and every
// highlight shares a LayoutID: when the selection moves, the new one grows
// from where the old one was, so the pill slides between entries. Select is
// the parent's Setter, which is comparable, so an item skips its build when
// nothing about it changed.
type NavItem struct {
	Nav    Nav
	Active bool
	Select tela.Setter[string]
}

func (p NavItem) Build() tela.Node {
	color := "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white"
	if p.Active {
		color = "text-slate-900 dark:text-white"
	}
	return Button(Class("relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors duration-200", color),
		OnClick(func() { p.Select.Set(p.Nav.Label) }),
		If(p.Active, Div(Key("pill"), LayoutID("nav-pill"),
			Class("absolute inset-0 rounded-xl bg-white/80 shadow-sm ring-1 ring-white dark:bg-white/10 dark:ring-white/10"))),
		Div(Class("relative"), icon(p.Nav.Icon)),
		Div(Class("relative"), Text(p.Nav.Label)),
		If(p.Nav.Label == "Messages", Div(Class("relative ml-auto rounded-full bg-rose-500 px-1.5 text-[11px] font-semibold text-white"), Text("3"))),
	)
}

// Avatar is a gradient circle with initials; Size and Tint are the classes
// of its size and its gradient.
type Avatar struct {
	Initials, Size, Tint string
}

func (a Avatar) Build() tela.Node {
	return Div(Class("flex shrink-0 items-center justify-center rounded-full bg-linear-to-br text-xs font-semibold text-white ring-2 ring-white/70 dark:ring-white/20", a.Size, a.Tint),
		Text(a.Initials))
}

// Header is the glass bar over the content: sticky, so the page scrolls
// under it and its backdrop-blur frosts whatever passes.
func Header() tela.Node { return tela.C(header) }

func header() tela.Node {
	return Div(Class("sticky top-0 z-30 flex h-16 shrink-0 items-center gap-4 border-b px-6 md:px-8", bar),
		Div(Class("hidden items-center gap-2 text-sm sm:flex"),
			Span(Class(muted), Text("Dashboard")),
			Span(Class("text-slate-400 dark:text-slate-600"), Text("/")),
			Span(Class("font-medium"), Text("Overview")),
		),
		Div(Class("ml-auto hidden h-10 w-64 items-center gap-2 rounded-xl border border-white/60 bg-white/50 px-3 text-slate-500",
			"transition duration-200 focus-within:bg-white/80 focus-within:ring-2 focus-within:ring-indigo-500/50",
			"dark:border-white/10 dark:bg-white/5 dark:text-slate-400 dark:focus-within:bg-white/10 lg:flex"),
			icon("search"),
			Input(Type("search"), AriaLabel("Search"), Placeholder("Search reports"),
				Class("min-w-0 flex-1 bg-transparent text-sm text-slate-900 outline-none placeholder:text-slate-500 dark:text-white dark:placeholder:text-slate-400")),
		),
		RangeControl(),
		ThemeSwitch(),
		Button(AriaLabel("Notifications"), Class("relative flex size-10 items-center justify-center rounded-xl border border-white/60 bg-white/50",
			"text-slate-700 transition duration-200 hover:bg-white/80 active:scale-95 dark:border-white/10 dark:bg-white/5 dark:text-slate-200 dark:hover:bg-white/10"),
			icon("bell"),
			Div(Class("absolute top-2 right-2.5 size-2 rounded-full bg-rose-500 ring-2 ring-white/80 dark:ring-slate-900")),
		),
	)
}

// RangeControl is the segmented control that picks the period. Its pill
// slides between the segments (LayoutID), and the whole page follows the
// choice through AppCtx.
func RangeControl() tela.Node { return tela.C(rangeControl) }

func rangeControl() tela.Node {
	app := tela.UseContext(AppCtx)
	return Div(Role("tablist"), AriaLabel("Period"),
		Class("ml-auto flex rounded-xl border border-white/60 bg-white/40 p-1 lg:ml-0 dark:border-white/10 dark:bg-white/5"),
		MapIndex(periods, func(i int, p Period) tela.Node {
			color := "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white"
			if i == app.Range {
				color = "text-slate-900 dark:text-white"
			}
			return Button(Key(p.Label), Role("tab"), AriaSelected(i == app.Range),
				Class("relative flex items-center justify-center rounded-lg px-3.5 py-1.5 text-sm font-medium transition-colors duration-200", color),
				OnClick(func() { app.SetRange.Set(i) }),
				If(i == app.Range, Div(Key("pill"), LayoutID("range-pill"),
					Class("absolute inset-0 rounded-lg bg-white shadow-sm dark:bg-white/15"))),
				Div(Class("relative"), Text(p.Label)),
			)
		}),
	)
}

// ThemeSwitch toggles the color scheme. tela.UseColorScheme is the scheme
// in effect (the system's, until the app picks one) and tela.SetColorScheme
// changes it from here; the knob slides with a transition.
func ThemeSwitch() tela.Node { return tela.C(themeSwitch) }

func themeSwitch() tela.Node {
	dark := tela.UseColorScheme() == tela.Dark
	next := tela.Dark
	if dark {
		next = tela.Light
	}
	knob, label := "translate-x-0", "Day"
	if dark {
		knob, label = "translate-x-[38px]", "Night"
	}
	return Button(Role("switch"), AriaChecked(dark), AriaLabel("Dark mode"),
		Class("relative h-10 w-[84px] rounded-xl border border-white/60 bg-white/50 text-[11px] font-semibold text-slate-500 transition duration-300",
			"hover:bg-white/80 dark:border-white/10 dark:bg-white/5 dark:text-slate-400 dark:hover:bg-white/10"),
		OnClick(func() { tela.SetColorScheme(next) }),
		Div(Class("absolute top-1/2 left-2.5 -translate-y-1/2"), Text("Day")),
		Div(Class("absolute top-1/2 right-2 -translate-y-1/2"), Text("Night")),
		Div(Class("absolute top-1 left-1 flex h-8 w-[38px] items-center justify-center rounded-[9px] bg-white text-slate-900 shadow-md ring-1 ring-black/5", "transition duration-300 ease-out dark:bg-slate-700 dark:text-white dark:ring-white/10", knob),
			Text(label)),
	)
}

// Hero is the greeting, with a gradient headline painted through the
// glyphs (bg-clip-text), the sync state and the two actions.
func Hero() tela.Node { return tela.C(hero) }

func hero() tela.Node {
	app := tela.UseContext(AppCtx)
	return Div(Class("flex flex-wrap items-end justify-between gap-4"),
		Div(
			H1(Class("bg-linear-to-r from-indigo-600 via-fuchsia-500 to-sky-500 bg-clip-text pb-1 text-4xl font-extrabold tracking-tight text-transparent",
				"dark:from-indigo-300 dark:via-fuchsia-300 dark:to-sky-300"),
				Text("Good evening, Olivia")),
			Div(Class("mt-1 flex items-center gap-3 text-sm", muted),
				Span(Text("Here is how Aurora is doing.")),
				LiveBadge(),
			),
		),
		Div(Class("flex items-center gap-3"),
			Button(Class("flex h-10 items-center gap-2 rounded-xl border border-white/60 bg-white/50 px-4 text-sm font-medium",
				"transition duration-200 hover:bg-white/80 active:scale-95 dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/10"),
				OnClick(func() { app.SetToast.Set("Export started: revenue.csv") }),
				Text("Export")),
			Button(Class("flex h-10 items-center gap-2 rounded-xl bg-linear-to-r from-indigo-500 to-fuchsia-500 px-4 text-sm font-semibold text-white",
				"shadow-lg shadow-fuchsia-500/30 transition duration-200 hover:-translate-y-0.5 hover:shadow-xl hover:shadow-fuchsia-500/40 active:scale-95"),
				OnClick(func() { app.SetDialog.Set(true) }),
				icon("plus"), Text("New report")),
		),
	)
}

// LiveBadge says the data is live, and syncing for a moment after the
// period changes: a spinner and a pulsing dot while it does, then neither.
// Both are infinite animations, so the badge only has them while it syncs;
// an idle page requests no frames. The end of the sync is a tween on the
// frame clock (tela.Animate with OnDone), not a timer.
func LiveBadge() tela.Node { return tela.C(liveBadge) }

func liveBadge() tela.Node {
	app := tela.UseContext(AppCtx)
	syncing, setSyncing := tela.UseState(false)
	clock := tela.UseValue(float32(0))
	// Not on mount: only when the period changes.
	tela.UseUpdateEffect(func() func() {
		setSyncing.Set(true)
		tela.Animate(clock, 1, tela.Duration(1200*time.Millisecond), tela.Ease(tela.EaseLinear),
			tela.OnDone(func() { setSyncing.Set(false); clock.Set(0) }))
		return nil
	}, app.Range)

	if syncing {
		return Span(Class("flex items-center gap-2 rounded-full bg-amber-400/20 px-2.5 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300"),
			Span(Class("size-3 animate-spin rounded-full border-2 border-amber-500 border-t-transparent")),
			Text("Syncing"))
	}
	return Span(Class("flex items-center gap-2 rounded-full bg-emerald-400/20 px-2.5 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-300"),
		Span(Class("size-2 rounded-full bg-emerald-500")),
		Text("Live"))
}

// HelpButton is the round button fixed to the corner of the window: it
// stays there while the page scrolls under it.
func HelpButton() tela.Node { return tela.C(helpButton) }

func helpButton() tela.Node {
	app := tela.UseContext(AppCtx)
	return Button(AriaLabel("Help"),
		Class("fixed right-6 bottom-6 z-40 flex size-12 items-center justify-center rounded-full bg-linear-to-br from-indigo-500 to-fuchsia-500",
			"text-lg font-semibold text-white shadow-xl shadow-fuchsia-500/30 ring-4 ring-white/40 transition duration-200 ease-out",
			"hover:scale-110 hover:shadow-2xl active:scale-95 dark:ring-white/10"),
		OnClick(func() { app.SetToast.Set("Support will reply within the hour") }),
		Text("?"))
}
