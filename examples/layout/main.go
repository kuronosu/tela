// Command layout shows Tela's layout animations (ARCHITECTURE section 12,
// ADR-081): a task list whose rows slide to their new places when it is
// reordered (h.AnimateLayout), a tab bar whose underline slides from the
// tab that loses it to the one that gets it (h.LayoutID), and a card that
// grows when it is expanded while its header keeps its size and the note
// below it moves down.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Task is a row of the list.
type Task struct {
	ID, Title, Tag, Dot string
}

var tasks = []Task{
	{"design", "Review the new onboarding flow", "Design", "bg-fuchsia-500"},
	{"api", "Ship the billing API", "Backend", "bg-sky-500"},
	{"bug", "Fix the flaky export test", "Bug", "bg-rose-500"},
	{"docs", "Write the migration guide", "Docs", "bg-amber-500"},
	{"perf", "Profile the dashboard", "Performance", "bg-emerald-500"},
}

// Layout is the example's screen.
func Layout() tela.Node {
	return Div(Class("flex h-screen gap-6 bg-slate-100 p-8 font-sans"),
		tela.C(TaskList),
		Div(Class("flex flex-1 flex-col gap-6"),
			tela.C(Tabs),
			tela.C(ExpandingCard),
			// The note follows the card's bottom edge.
			P(AnimateLayout(), Class("px-1 text-xs text-slate-400"),
				Text("Rows, underline and card animate with FLIP transforms: layout runs once per change.")),
		),
	)
}

// TaskList reorders its rows: each row is laid out at its new place at
// once and drawn sliding there on a spring.
func TaskList() tela.Node {
	order, setOrder := tela.UseState(tasks)
	rotate := func(tela.MouseEvent) {
		// The last task moves to the top: every row moves.
		n := len(order)
		next := append([]Task{order[n-1]}, order[:n-1]...)
		setOrder.Set(next)
	}
	reverse := func(tela.MouseEvent) {
		next := make([]Task, len(order))
		for i, t := range order {
			next[len(order)-1-i] = t
		}
		setOrder.Set(next)
	}
	return Div(Class("flex w-80 flex-col gap-3 rounded-2xl bg-white p-5 shadow-sm ring-1 ring-slate-200"),
		Div(Class("flex items-center justify-between"),
			H2(Class("text-base font-semibold text-slate-900"), Text("Tasks")),
			Div(Class("flex gap-2"),
				Button(Class("rounded-md bg-slate-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-slate-700"),
					OnClick(rotate), Text("Rotate")),
				Button(Class("rounded-md bg-white px-3 py-1.5 text-xs font-medium text-slate-700 ring-1 ring-slate-200 hover:bg-slate-50"),
					OnClick(reverse), Text("Reverse")))),
		Map(order, func(t Task) tela.Node {
			return Div(Key(t.ID), AnimateLayout(),
				Class("flex items-center gap-3 rounded-xl bg-slate-50 px-3 py-3 ring-1 ring-slate-200"),
				Div(Class("size-2.5 shrink-0 rounded-full "+t.Dot)),
				Div(Class("flex-1 text-sm text-slate-700"), Text(t.Title)),
				Div(Class("rounded-full bg-white px-2 py-0.5 text-[11px] font-medium text-slate-500 ring-1 ring-slate-200"),
					Text(t.Tag)))
		}),
	)
}

// Tabs slides its underline between tabs: each tab renders an underline
// when it is selected, and their shared LayoutID moves the new one from
// where the old one was.
func Tabs() tela.Node {
	active, setActive := tela.UseState(0)
	labels := []string{"Overview", "Activity", "Settings"}
	bodies := []string{
		"Three projects are on track and one needs a review this week.",
		"Twelve changes were merged since Monday, four of them by you.",
		"Notifications go to your email; the weekly digest is on.",
	}
	return Div(Class("rounded-2xl bg-white p-5 shadow-sm ring-1 ring-slate-200"),
		Div(Class("flex gap-1 border-b border-slate-200"), MapIndex(labels, func(i int, label string) tela.Node {
			color := "text-slate-500 hover:text-slate-800"
			if i == active {
				color = "text-indigo-600"
			}
			return Button(Key(label), Class("relative px-3 pb-3 pt-1 text-sm font-medium "+color),
				OnClick(func(tela.MouseEvent) { setActive.Set(i) }),
				Text(label),
				If(i == active, Div(Key("underline"), LayoutID("tab-underline"),
					Class("absolute inset-x-1 -bottom-px h-0.5 rounded-full bg-indigo-500"))))
		})),
		P(Class("pt-4 text-sm text-slate-600"), Text(bodies[active])),
	)
}

// ExpandingCard grows when it is opened. The card scales toward its new
// size (its corners and shadow are corrected); its header and body have
// AnimateLayout too, so they keep their own size instead of stretching.
func ExpandingCard() tela.Node {
	open, setOpen := tela.UseState(false)
	label := "Show details"
	if open {
		label = "Hide details"
	}
	return Div(AnimateLayout(), Class("overflow-hidden rounded-2xl bg-linear-to-br from-indigo-500 to-violet-600 p-5 text-white shadow-lg"),
		Div(AnimateLayout(), Class("flex items-center justify-between"),
			Div(
				H2(Class("text-base font-semibold"), Text("Quarterly report")),
				P(Class("text-sm text-indigo-100"), Text("Revenue grew 18% over the last quarter."))),
			Button(Class("rounded-lg bg-white/15 px-3 py-1.5 text-xs font-medium hover:bg-white/25"),
				OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text(label))),
		If(open, Div(Key("details"), AnimateLayout(),
			Class("mt-4 grid grid-cols-3 gap-3 transition-opacity duration-300 starting:opacity-0"),
			stat("Revenue", "$1.2M"), stat("Customers", "4,210"), stat("Churn", "1.8%"))),
	)
}

func stat(label, value string) tela.Node {
	return Div(Class("rounded-xl bg-white/10 p-3"),
		P(Class("text-xs text-indigo-100"), Text(label)),
		P(Class("text-lg font-semibold"), Text(value)))
}

func main() {
	tela.Run(tela.App{Title: "Layout animations", Width: 820, Height: 560, Root: Layout})
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return Layout() }
