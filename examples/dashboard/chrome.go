package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Nav is an item of the sidebar.
type Nav struct {
	Label string
	Icon  tela.Icon
}

var navItems = []Nav{
	{"Overview", icons.LayoutDashboard}, {"Analytics", icons.ChartColumn}, {"Customers", icons.Users},
	{"Products", icons.Package}, {"Messages", icons.MessageSquare}, {"Settings", icons.SlidersHorizontal},
}

// Sidebar is the navigation column: sticky, as tall as the window, so it
// stays while the page scrolls. It is glass too, over the same backdrop.
func Sidebar() tela.Node { return tela.C(sidebar) }

func sidebar() tela.Node {
	active, setActive := tela.UseState("Overview")
	return Div(Class("sticky top-0 hidden h-screen flex-col gap-6 self-start border-r border-border bg-white/40 p-5",
		"lg:flex dark:bg-slate-950/30"),
		Div(Class("flex items-center gap-3 px-1 pt-1"),
			Div(Class("flex size-10 items-center justify-center rounded-xl bg-linear-to-br from-indigo-500 via-fuchsia-500 to-pink-500 shadow-lg shadow-fuchsia-500/30"),
				Div(Class("size-4 rotate-45 rounded-[5px] border-2 border-white"))),
			Div(
				Div(Class("text-lg leading-5 font-semibold tracking-tight text-foreground"), Text("Aurora")),
				Div(Class("text-xs text-muted-foreground"), Text("Analytics")),
			),
		),
		Div(Class("flex flex-col gap-1"),
			Div(Class("px-3 pb-1 text-[11px] font-semibold tracking-widest text-muted-foreground uppercase"), Text("Menu")),
			MapKeyed(navItems, func(n Nav) string { return n.Label }, func(n Nav) tela.Component {
				return NavItem{Nav: n, Active: n.Label == active, Select: setActive}
			}),
		),
		Div(Class("mt-auto flex flex-col gap-4"),
			Div(Class("overflow-hidden rounded-2xl bg-linear-to-br from-indigo-500 to-fuchsia-500 p-4 text-white shadow-lg shadow-indigo-500/30"),
				Div(Class("text-sm font-semibold"), Text("Pro plan")),
				Div(Class("mt-0.5 text-xs text-white/80"), Text("7 of 10 seats in use")),
				kit.Progress{Value: 70, AriaLabel: "Seats in use", Class: "mt-3 h-1.5 bg-white/25", IndicatorClass: "bg-white"},
				kit.Button{Variant: "tint", Size: kit.Sm, Class: "mt-4 w-full backdrop-blur", Children: Text("Add seats")},
			),
			Div(Class("flex items-center gap-3 px-1"),
				kit.Avatar{Fallback: "OM", FallbackClass: "from-sky-400 to-indigo-500"},
				Div(Class("min-w-0"),
					Div(Class("truncate text-sm font-medium"), Text("Olivia Martin")),
					Div(Class("truncate text-xs text-muted-foreground"), Text("olivia@aurora.dev")),
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
	return Fragment(kit.Button{Variant: "nav", Size: "nav", Class: "relative " + when(p.Active, "text-foreground"),
		OnClick:  func() { p.Select.Set(p.Nav.Label) },
		Children: navLabel{Nav: p.Nav, Active: p.Active}})
}

// navLabel is what a nav button holds: the pill when the entry is the
// active one, the icon, the label and, on Messages, the count.
type navLabel struct {
	Nav    Nav
	Active bool
}

func (p navLabel) Build() tela.Node {
	return Fragment(
		If(p.Active, Div(Key("pill"), LayoutID("nav-pill"),
			Class("absolute inset-0 rounded-xl bg-accent shadow-sm ring-1 ring-white dark:ring-white/10"))),
		Div(Class("relative"), Icon(p.Nav.Icon, Class("size-4"))),
		Div(Class("relative"), Text(p.Nav.Label)),
		If(p.Nav.Label == "Messages", kit.Badge{Variant: kit.Destructive, Class: "relative ml-auto px-1.5 text-[11px] font-semibold", Children: Text("3")}),
	)
}

// when returns class if cond is true, else no class.
func when(cond bool, class string) string {
	if cond {
		return class
	}
	return ""
}

// Header is the glass bar over the content: sticky, so the page scrolls
// under it and its backdrop-blur frosts whatever passes.
func Header() tela.Node { return tela.C(header) }

func header() tela.Node {
	return Div(Class("sticky top-0 z-30 flex h-16 shrink-0 items-center gap-4 border-b px-6 md:px-8", bar),
		Div(Class("hidden items-center gap-2 text-sm sm:flex"),
			Span(Class("text-muted-foreground"), Text("Dashboard")),
			Span(Class("text-slate-400 dark:text-slate-600"), Text("/")),
			Span(Class("font-medium"), Text("Overview")),
		),
		Div(Class("relative ml-auto hidden w-64 lg:block"),
			Icon(icons.Search, Class("pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground")),
			kit.Input{Type: "search", Placeholder: "Search reports", Class: "pl-9", AriaLabel: "Search"},
		),
		RangeControl(),
		ThemeSwitch(),
		kit.Button{Variant: "glass", Size: kit.Icon, Icon: icons.Bell, AriaLabel: "Notifications", Class: "relative",
			Children: Div(Class("absolute top-2 right-2.5 size-2 rounded-full bg-rose-500 ring-2 ring-white/80 dark:ring-slate-900"))},
	)
}

// Segment is the content of an item of a segmented control. The pill under
// the selected item is a child of the item that has it, and every pill of a
// control shares a LayoutID, so when the selection moves the new pill grows
// from where the old one was and slides.
type Segment struct {
	Label string
	Pill  string // the LayoutID
	On    bool
}

func (s Segment) Build() tela.Node {
	return Fragment(
		If(s.On, Div(Key("pill"), LayoutID(s.Pill), Class("absolute inset-0 rounded-lg bg-white shadow-sm dark:bg-white/15"))),
		Div(Class("relative"), Text(s.Label)),
	)
}

// RangeControl is the segmented control that picks the period: a
// kit.ToggleGroup whose pill slides between the segments, and the whole
// page follows the choice through AppCtx.
func RangeControl() tela.Node { return tela.C(rangeControl) }

func rangeControl() tela.Node {
	app := tela.UseContext(AppCtx)
	items := make([]kit.ToggleGroupItem[int], len(periods))
	for i, p := range periods {
		items[i] = kit.ToggleGroupItem[int]{Value: i, Children: Segment{Label: p.Label, Pill: "range-pill", On: i == app.Range}}
	}
	return Fragment(kit.ToggleGroup[int]{Items: items, Value: &app.Range, OnValueChange: app.SetRange.Set,
		DisallowEmptySelection: true, AriaLabel: "Period", Class: "ml-auto lg:ml-0"})
}

// ThemeSwitch toggles the color scheme. tela.UseColorScheme is the scheme
// in effect (the system's, until the app picks one) and tela.SetColorScheme
// changes it from here; the knob slides with a transition.
func ThemeSwitch() tela.Node { return tela.C(themeSwitch) }

func themeSwitch() tela.Node {
	dark := tela.UseColorScheme() == tela.Dark
	return Div(Class("flex h-10 items-center gap-2.5 rounded-xl border border-border bg-secondary px-3 text-muted-foreground"),
		Icon(icons.Sun, Class("size-4")),
		kit.Switch{Checked: &dark, AriaLabel: "Dark mode", OnCheckedChange: func(on bool) {
			if on {
				tela.SetColorScheme(tela.Dark)
			} else {
				tela.SetColorScheme(tela.Light)
			}
		}},
		Icon(icons.Moon, Class("size-4")),
	)
}

// Hero is the greeting, with a gradient headline painted through the
// glyphs (bg-clip-text), the sync state and the two actions. Dialog starts
// the new-report dialog open.
type Hero struct{ Dialog bool }

func (p Hero) Build() tela.Node {
	toast := kit.UseToast()
	open, setOpen := tela.UseState(p.Dialog)
	return Div(Class("flex flex-wrap items-end justify-between gap-4"),
		Div(
			H1(Class("bg-linear-to-r from-indigo-600 via-fuchsia-500 to-sky-500 bg-clip-text pb-1 text-4xl font-extrabold tracking-tight text-transparent",
				"dark:from-indigo-300 dark:via-fuchsia-300 dark:to-sky-300"),
				Text("Good evening, Olivia")),
			Div(Class("mt-1 flex items-center gap-3 text-sm text-muted-foreground"),
				Span(Text("Here is how Aurora is doing.")),
				LiveBadge(),
			),
		),
		Div(Class("flex items-center gap-3"),
			kit.Button{Variant: "glass", Children: Text("Export"),
				OnClick: func() { toast.Success("Export started: revenue.csv") }},
			kit.Dialog{
				Trigger:      kit.Button{Icon: icons.Plus, Children: Text("New report")},
				Open:         &open,
				OnOpenChange: setOpen.Set,
				Title:        Text("New report"),
				Description:  Text("Pick what to include and how to deliver it."),
				Children:     ReportForm{Close: setOpen},
			},
		),
	)
}

// LiveBadge says the data is live, and syncing for a moment after the
// period changes: a spinner while it does, then neither. The spinner is an
// infinite animation, so the badge only has it while it syncs; an idle page
// requests no frames. The end of the sync is a tween on the frame clock
// (tela.Animate with OnDone), not a timer.
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
		return Fragment(kit.Badge{Variant: "syncing", Children: Fragment(
			Icon(icons.LoaderCircle, Class("size-3 animate-spin text-amber-500")), Text("Syncing"))})
	}
	return Fragment(kit.Badge{Variant: "live", Children: Fragment(
		Span(Class("size-2 rounded-full bg-emerald-500")), Text("Live"))})
}

// HelpButton is the round button fixed to the corner of the window: it
// stays there while the page scrolls under it.
func HelpButton() tela.Node { return tela.C(helpButton) }

func helpButton() tela.Node {
	toast := kit.UseToast()
	return Fragment(kit.Button{Variant: "fab", Size: "fab", Icon: icons.CircleHelp, AriaLabel: "Help",
		Class:   "fixed right-6 bottom-6 z-40 rounded-full",
		OnClick: func() { toast.Show("Support will reply within the hour") }})
}
