package main

import (
	"slices"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

var sorts = []struct{ ID, Label string }{{"priority", "Priority"}, {"due", "Due"}, {"name", "A-Z"}}

// order returns the tasks in the order of sortBy, finished ones last.
func order(sortBy string, done map[string]bool) []Task {
	out := slices.Clone(tasks)
	slices.SortStableFunc(out, func(a, b Task) int {
		if done[a.ID] != done[b.ID] {
			if done[a.ID] {
				return 1
			}
			return -1
		}
		switch sortBy {
		case "due":
			return a.Due - b.Due
		case "name":
			return strings.Compare(a.Title, b.Title)
		}
		return a.Priority - b.Priority
	})
	return out
}

// TasksCard is a sortable list; Sort is the order it starts in. Every row
// has AnimateLayout: choosing another order, or finishing a task (it sinks
// to the bottom), lays the list out once and each row is drawn sliding from
// its old place to its new one on a spring, a transform that only
// composites.
type TasksCard struct{ Sort string }

func (p TasksCard) Build() tela.Node {
	sortBy, setSort := tela.UseState(cmpOr(p.Sort, "priority"))
	done, setDone := tela.UseState(map[string]bool{})
	toggle := func(id string) {
		next := maps2(done)
		next[id] = !next[id]
		setDone.Set(next)
	}
	return Div(Class(glass, "flex flex-col gap-4 p-6"),
		Div(Class("flex items-center justify-between"),
			H2(Class("text-lg", heading), Text("Priorities")),
			Div(Class("flex gap-1 rounded-lg bg-black/5 p-0.5 dark:bg-white/10"),
				Map(sorts, func(s struct{ ID, Label string }) tela.Node {
					cls := "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white"
					if s.ID == sortBy {
						cls = "bg-white text-slate-900 shadow-sm dark:bg-white/15 dark:text-white"
					}
					return Button(Key(s.ID), Class("rounded-md px-2.5 py-1 text-xs font-medium transition duration-200", cls),
						OnClick(func() { setSort.Set(s.ID) }), Text(s.Label))
				}),
			),
		),
		Div(Class("flex flex-col gap-2"),
			Map(order(sortBy, done), func(t Task) tela.Node {
				return Div(Key(t.ID), AnimateLayout(),
					Class("flex items-center gap-3 rounded-xl border border-white/60 bg-white/50 px-3 py-2.5 transition-colors duration-200 hover:bg-white/80 dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/10"),
					checkbox(t, done[t.ID], toggle),
					Div(Class("min-w-0 flex-1"),
						Div(Class("truncate text-sm font-medium transition-colors duration-200", strike(done[t.ID])), Text(t.Title)),
						Div(Class("mt-0.5 flex items-center gap-1.5 text-xs", muted),
							Span(Class("size-1.5 rounded-full", t.Dot)), Text(t.Tag)),
					),
					Span(Class("rounded-full bg-black/5 px-2 py-0.5 text-xs font-medium dark:bg-white/10", muted), Text(t.DueLabel)),
				)
			}),
		),
	)
}

func strike(done bool) string {
	if done {
		return "text-slate-400 line-through dark:text-slate-500"
	}
	return "text-slate-900 dark:text-white"
}

// checkbox is the round button that finishes a task.
func checkbox(t Task, done bool, toggle func(string)) tela.Node {
	cls := "border-slate-300 bg-white/60 text-transparent hover:border-indigo-400 dark:border-slate-600 dark:bg-transparent"
	if done {
		cls = "border-transparent bg-linear-to-br from-indigo-500 to-fuchsia-500 text-white"
	}
	return Button(Role("checkbox"), AriaChecked(done), AriaLabel(t.Title),
		Class("flex size-6 shrink-0 items-center justify-center rounded-full border transition duration-200 active:scale-90", cls),
		OnClick(func() { toggle(t.ID) }),
		icon("check"))
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func maps2(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// InsightCard is a card that expands. It has AnimateLayout and so do its
// header and its details: the card grows toward its new size (corners and
// shadow stay as they are) while the header keeps its size and the details
// fade in (starting:). overflow-hidden reveals the new content as the card
// grows.
func InsightCard() tela.Node {
	open, setOpen := tela.UseState(false)
	label := "Show details"
	if open {
		label = "Hide details"
	}
	return Div(AnimateLayout(),
		Class("relative isolate overflow-hidden rounded-2xl bg-linear-to-br from-indigo-500 via-violet-500 to-fuchsia-500 p-6 text-white shadow-lg shadow-indigo-500/30"),
		// A highlight that blends with the gradient under it (mix-blend-overlay).
		Div(Class("pointer-events-none absolute -top-24 -right-24 size-72 bg-[radial-gradient(closest-side,rgb(255_255_255/0.7),transparent)] mix-blend-overlay"), AriaHidden(true)),
		Div(AnimateLayout(), Class("relative flex flex-col gap-3"),
			Span(Class("w-fit rounded-full bg-white/20 px-2.5 py-0.5 text-xs font-semibold"), Text("Insight")),
			H2(Class("text-xl leading-snug font-semibold"), Text("Revenue grew 18% over last quarter")),
			P(Class("text-sm text-white/80"), Text("Annual plans drove most of it, and churn fell to its lowest point of the year.")),
			Button(Class("w-fit rounded-lg bg-white/20 px-3.5 py-1.5 text-sm font-medium transition duration-200 hover:bg-white/30 active:scale-95"),
				OnClick(tela.Toggle(setOpen)), Text(label)),
		),
		If(open, Div(Key("details"), AnimateLayout(),
			Class("mt-5 grid grid-cols-3 gap-3 transition-opacity duration-300 starting:opacity-0"),
			Insight{"Annual plans", "+34%"}, Insight{"Churn", "1.8%"}, Insight{"ARPU", "$62"}),
		),
	)
}

// Insight is one figure of the insight card's details.
type Insight struct{ Label, Value string }

func (i Insight) Build() tela.Node {
	return Div(Class("rounded-xl bg-white/15 p-3"),
		Div(Class("text-xs text-white/75"), Text(i.Label)),
		Div(Class("mt-0.5 text-lg font-semibold tabular-nums"), Text(i.Value)))
}

// ActivityCard lists the latest sales.
func ActivityCard() tela.Node {
	return Div(Class(glass, "flex flex-col gap-3 p-6"),
		Div(Class("flex items-center justify-between"),
			H2(Class("text-lg", heading), Text("Recent sales")),
			Button(Class("text-sm font-medium text-indigo-600 transition-colors hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200"), Text("View all")),
		),
		Map(sales, func(s Sale) tela.Node {
			return Div(Key(s.Name), Class("flex items-center gap-3 rounded-xl px-2 py-2 transition-colors duration-200 hover:bg-white/50 dark:hover:bg-white/10"),
				Avatar{Initials: s.Initials, Size: "size-10", Tint: s.Tint},
				Div(Class("min-w-0 flex-1"),
					Div(Class("truncate text-sm font-medium"), Text(s.Name)),
					Div(Class("truncate text-xs", muted), Text(s.Item))),
				Div(Class("text-sm font-semibold tabular-nums text-emerald-700 dark:text-emerald-300"), Text(s.Amount)),
			)
		}),
	)
}
