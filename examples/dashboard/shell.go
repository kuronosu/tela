package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Wallpaper is what the glass is over: a fixed layer behind the panes. The
// picture is made for the scheme (tela.UseColorScheme) and covers the
// window; img/gen.go draws both.
func Wallpaper() tela.Node { return tela.C(wallpaper) }

func wallpaper() tela.Node {
	img := "bg-[url(img/dunes-light.jpg)]"
	if tela.UseColorScheme() == tela.Dark {
		img = "bg-[url(img/dunes-dark.jpg)]"
	}
	return Div(Class("pointer-events-none fixed inset-0 -z-10 bg-cover bg-bottom", img), AriaHidden(true))
}

// View is a page of the rail.
type View struct {
	ID, Label string
	Icon      tela.Icon
}

var views = []View{
	{"overview", "Overview", icons.LayoutDashboard},
	{"tasks", "Tasks", icons.ListChecks},
	{"orders", "Orders", icons.Receipt},
}

// Rail is the pane of icons on the left: one button per view, each with a
// tooltip (Tip, on aria hooks), and an amber bar by the active one.
type Rail struct {
	View    string
	SetView tela.Setter[string]
}

func (p Rail) Build() tela.Node {
	return Div(Role("navigation"), AriaLabel("Views"), Class("flex w-16 shrink-0 flex-col items-center gap-2 py-4", pane),
		Div(Class("mb-3 flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground"), AriaHidden(true),
			Icon(icons.Grid3x3, Class("size-5"))),
		MapKeyed(views, func(v View) string { return v.ID }, func(v View) tela.Component {
			return RailButton{View: v, Active: v.ID == p.View, Set: p.SetView}
		}),
		Div(Class("mt-auto"), kit.Avatar{Fallback: "OM", Alt: "Olivia Martin", Class: "size-8"}),
	)
}

// RailButton is one view of the rail. Its fields are comparable, so a
// button builds only when it gains or loses the selection.
type RailButton struct {
	View   View
	Active bool
	Set    tela.Setter[string]
}

func (p RailButton) Build() tela.Node {
	return Fragment(Tip{Text: p.View.Label, Placement: "right", Render: func(tp tela.Prop) tela.Node {
		return Button(tp, AriaLabel(p.View.Label), AriaCurrent(when(p.Active, "page")), OnClick(func() { p.Set.Set(p.View.ID) }),
			Class("relative flex size-10 items-center justify-center rounded-xl text-muted-foreground transition-colors outline-none",
				"hover:bg-accent hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring aria-[current=page]:bg-accent aria-[current=page]:text-foreground"),
			If(p.Active, Span(Key("bar"), LayoutID("rail-bar"), Class("absolute top-1/2 -left-[13px] h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-primary"))),
			Icon(p.View.Icon, Class("size-[18px]")))
	}})
}

// when returns s if c is true, else "".
func when(c bool, s string) string {
	if c {
		return s
	}
	return ""
}

// TopBar sits at the top of the main pane: the title of the view, the
// view's own control (Right), the search button that opens the palette,
// the theme button and the notifications.
type TopBar struct {
	Title, Sub string
	Right      tela.Component
	OpenSearch tela.Setter[bool]
}

func (p TopBar) Build() tela.Node {
	return Div(Class("flex h-14 shrink-0 items-center gap-3 border-b border-border px-6"),
		Div(Class("flex items-baseline gap-2.5"),
			H1(Class("text-[15px] font-semibold tracking-tight"), Text(p.Title)),
			Span(Class("text-sm text-muted-foreground"), Text(p.Sub))),
		Div(Class("ml-auto flex items-center gap-2"),
			If(p.Right != nil, p.Right),
			Button(Class("flex h-8 w-60 items-center gap-2 rounded-lg border border-border bg-white/[0.03] px-2.5 text-[13px] text-muted-foreground transition-colors outline-none",
				"hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring"),
				AriaLabel("Search"), OnClick(func() { p.OpenSearch.Set(true) }),
				Icon(icons.Search, Class("size-3.5")),
				Span(Class("flex-1 text-left"), Text("Search…")),
				Span(Class("rounded border border-border px-1 py-px text-[10px] font-medium"), AriaHidden(true), Text("Ctrl K"))),
			ThemeButton(),
			Bell(),
		),
	)
}

// ThemeButton switches the color scheme: tela.UseColorScheme is the scheme
// in effect and tela.SetColorScheme changes it.
func ThemeButton() tela.Node { return tela.C(themeButton) }

func themeButton() tela.Node {
	dark := tela.UseColorScheme() == tela.Dark
	return Fragment(Tip{Text: "Switch theme", Render: func(tp tela.Prop) tela.Node {
		return Button(tp, AriaLabel("Switch theme"), Class(iconBtn), OnClick(toggleTheme(dark)),
			IfElse(dark, Icon(icons.Sun, Class("size-4")), Icon(icons.Moon, Class("size-4"))))
	}})
}

// toggleTheme returns the action that leaves the scheme dark is in.
func toggleTheme(dark bool) func() {
	return func() {
		if dark {
			tela.SetColorScheme(tela.Light)
		} else {
			tela.SetColorScheme(tela.Dark)
		}
	}
}

// Notice is an entry of the notifications.
type Notice struct{ Title, Detail string }

var notices = []Notice{
	{"Invoice #4881 paid", "Isabella Nguyen, $299.00"},
	{"TEL-101 is due today", "Fix failed invoice retries"},
	{"Weekly report sent", "To 4 people, Monday 9:00"},
}

// Bell is the notifications: a kit.Popover with the latest notices, and an
// amber dot while some are unread; Mark all as read takes the dot away.
func Bell() tela.Node { return tela.C(bell) }

func bell() tela.Node {
	unread, setUnread := tela.UseState(true)
	label := "Notifications"
	if unread {
		label = "Notifications, 3 unread"
	}
	return Fragment(kit.Popover{
		Align: "end", Offset: 8, AriaLabel: "Notifications",
		Trigger: kit.TriggerFunc(func(tp tela.Prop) tela.Node {
			return Button(tp, AriaLabel(label), Class(iconBtn, "relative"),
				Icon(icons.Bell, Class("size-4")),
				If(unread, Span(Class("absolute top-1.5 right-1.5 size-1.5 rounded-full bg-primary"))))
		}),
		Children: Div(Class("flex flex-col"),
			Div(Class("flex items-center justify-between border-b border-border px-4 py-3"),
				Span(Class("text-sm font-semibold"), Text("Notifications")),
				kit.Button{Variant: kit.Ghost, Size: kit.Sm, Icon: icons.CheckCheck, Disabled: !unread,
					OnClick: func() { setUnread.Set(false) }, Children: Text("Mark all as read")}),
			Div(Class("flex flex-col p-1.5"),
				Map(notices, func(n Notice) tela.Node {
					return Div(Key(n.Title), Class("flex items-start gap-3 rounded-lg px-2.5 py-2"),
						Span(Class("mt-1.5 size-1.5 shrink-0 rounded-full", when(unread, "bg-primary"), when(!unread, "bg-border"))),
						Div(Class("min-w-0"),
							Div(Class("text-[13px] font-medium"), Text(n.Title)),
							Div(Class("text-xs text-muted-foreground"), Text(n.Detail))))
				})),
		),
	})
}

// Segment is the content of an item of a segmented control: the pill under
// the selected item is its child, and every pill of a control shares a
// LayoutID, so when the selection moves the new pill grows from where the
// old one was and slides.
type Segment struct {
	Label string
	Icon  tela.Icon
	Pill  string // the LayoutID
	On    bool
}

func (s Segment) Build() tela.Node {
	return Fragment(
		If(s.On, Div(Key("pill"), LayoutID(s.Pill),
			Class("absolute inset-0 rounded-md bg-white shadow-sm ring-1 ring-black/5 dark:bg-white/10 dark:ring-white/10"))),
		If(s.Icon.SVG != "", Icon(s.Icon, Class("relative size-3.5"))),
		Span(Class("relative"), Text(s.Label)))
}

// segmented is a kit.ToggleGroup of Segments that always has a selection.
func segmented[K comparable](label, pill string, value K, set tela.Setter[K], keys []K, text func(K) string, icon func(K) tela.Icon) tela.Component {
	items := make([]kit.ToggleGroupItem[K], len(keys))
	for i, k := range keys {
		var ic tela.Icon
		if icon != nil {
			ic = icon(k)
		}
		items[i] = kit.ToggleGroupItem[K]{Value: k, Children: Segment{Label: text(k), Icon: ic, Pill: pill, On: k == value}}
	}
	return kit.ToggleGroup[K]{Items: items, Value: &value, OnValueChange: set.Set, DisallowEmptySelection: true, AriaLabel: label}
}

// Periods is the 7D / 30D / 90D switch of the overview.
type Periods struct {
	Range int
	Set   tela.Setter[int]
}

func (p Periods) Build() tela.Node {
	return Fragment(segmented("Period", "period-pill", p.Range, p.Set, upto(len(periods)),
		func(i int) string { return periods[i].Label }, nil))
}

// Modes is the Table / Board switch of the tasks.
type Modes struct {
	Mode string
	Set  tela.Setter[string]
}

func (p Modes) Build() tela.Node {
	return Fragment(segmented("Layout", "mode-pill", p.Mode, p.Set, []string{"table", "board"},
		func(m string) string { return map[string]string{"table": "Table", "board": "Board"}[m] },
		func(m string) tela.Icon {
			if m == "board" {
				return icons.SquareKanban
			}
			return icons.Table
		}))
}

// Statuses is the filter of the orders view.
type Statuses struct {
	Status string
	Set    tela.Setter[string]
}

func (p Statuses) Build() tela.Node {
	return Fragment(segmented("Status", "status-pill", p.Status, p.Set, statuses,
		func(s string) string { return s }, nil))
}
