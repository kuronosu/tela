package main

import (
	"slices"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
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

// TasksCard is the priorities list: a kit.Table of tasks with a segmented
// control (a kit.ToggleGroup) that sorts it; Sort is the order it starts
// in. Every cell has AnimateLayout: choosing another order, or finishing a
// task (it sinks to the bottom), lays the table out once and each cell is
// drawn sliding from its old place to its new one on a spring, a transform
// that only composites. The table shows its rows in the order given: the
// order is the page's (finished tasks last, then by priority, due date or
// name), not a column's.
type TasksCard struct{ Sort string }

func (p TasksCard) Build() tela.Node {
	sortBy, setSort := tela.UseState(cmpOr(p.Sort, "priority"))
	done, setDone := tela.UseState(map[string]bool{})
	toggle := func(id string) {
		next := maps2(done)
		next[id] = !next[id]
		setDone.Set(next)
	}
	chips := make([]kit.ToggleGroupItem[string], len(sorts))
	for i, s := range sorts {
		chips[i] = kit.ToggleGroupItem[string]{Value: s.ID, Label: s.Label}
	}
	slide := func(Task) tela.Child { return AnimateLayout() }
	return Fragment(kit.Card{
		Title: H2(Text("Priorities")),
		Action: kit.ToggleGroup[string]{Items: chips, Value: &sortBy, OnValueChange: setSort.Set, DisallowEmptySelection: true,
			Size: kit.Sm, AriaLabel: "Sort by", Class: "rounded-lg border-0 bg-muted p-0.5",
			ItemClass: "checked:bg-white checked:text-foreground checked:shadow-sm dark:checked:bg-white/15"},
		Children: kit.Table[Task]{
			Rows: order(sortBy, done), RowKey: func(t Task) any { return t.ID }, AriaLabel: "Priorities",
			Columns: []kit.Column[Task]{
				{ID: "done", Header: "Done", HeaderContent: Span(Class("sr-only"), Text("Done")), CellProps: slide,
					Cell: func(t Task) tela.Node {
						d := done[t.ID]
						return Fragment(kit.Checkbox{Checked: &d, OnCheckedChange: func(bool) { toggle(t.ID) }, AriaLabel: t.Title})
					}},
				{ID: "task", Header: "Task", Width: "minmax(0,1fr)", CellProps: slide,
					Cell: func(t Task) tela.Node {
						return Div(Class("min-w-0 flex-1"),
							Div(Class("truncate text-sm font-medium transition-colors duration-200", strike(done[t.ID])), Text(t.Title)),
							Div(Class("mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground"),
								Span(Class("size-1.5 rounded-full", t.Dot)), Text(t.Tag)),
						)
					}},
				{ID: "due", Header: "Due", Align: kit.AlignRight, CellProps: slide,
					Cell: func(t Task) tela.Node {
						return Fragment(kit.Badge{Variant: "soft", Children: Text(t.DueLabel)})
					}},
			},
		},
	})
}

func strike(done bool) string {
	if done {
		return "text-slate-400 line-through dark:text-slate-500"
	}
	return "text-foreground"
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
// grows. It is a kit.Card with the gradient as its Class.
func InsightCard() tela.Node { return tela.C(insightCard) }

func insightCard() tela.Node {
	open, setOpen := tela.UseState(false)
	label := "Show details"
	if open {
		label = "Hide details"
	}
	return Fragment(kit.Card{
		Class: "relative isolate overflow-hidden border-0 bg-linear-to-br from-indigo-500 via-violet-500 to-fuchsia-500 text-white shadow-lg shadow-indigo-500/30",
		Props: AnimateLayout(),
		Children: Fragment(
			// A highlight that blends with the gradient under it (mix-blend-overlay).
			Div(Class("pointer-events-none absolute -top-24 -right-24 size-72 bg-[radial-gradient(closest-side,rgb(255_255_255/0.7),transparent)] mix-blend-overlay"), AriaHidden(true)),
			Div(AnimateLayout(), Class("relative flex flex-col gap-3"),
				kit.Badge{Variant: "tint", Class: "px-2.5", Children: Text("Insight")},
				H2(Class("text-xl leading-snug font-semibold"), Text("Revenue grew 18% over last quarter")),
				P(Class("text-sm text-white/80"), Text("Annual plans drove most of it, and churn fell to its lowest point of the year.")),
				kit.Button{Variant: "tint", Class: "h-9 w-fit", OnClick: tela.Toggle(setOpen), Children: Text(label)},
			),
			If(open, Div(Key("details"), AnimateLayout(),
				Class("mt-5 grid grid-cols-3 gap-3 transition-opacity duration-300 starting:opacity-0"),
				Insight{"Annual plans", "+34%"}, Insight{"Churn", "1.8%"}, Insight{"ARPU", "$62"}),
			),
		),
	})
}

// Insight is one figure of the insight card's details.
type Insight struct{ Label, Value string }

func (i Insight) Build() tela.Node {
	return Div(Class("rounded-xl bg-white/15 p-3"),
		Div(Class("text-xs text-white/75"), Text(i.Label)),
		Div(Class("mt-0.5 text-lg font-semibold tabular-nums"), Text(i.Value)))
}

// ActivityCard lists the latest sales in a kit.Table.
func ActivityCard() tela.Node { return tela.C(activityCard) }

func activityCard() tela.Node {
	return Fragment(kit.Card{
		Title:  H2(Text("Recent sales")),
		Action: kit.Button{Variant: kit.Link, Class: "h-auto px-1", Children: Text("View all")},
		Children: kit.Table[Sale]{
			Rows: sales, RowKey: func(s Sale) any { return s.Name }, AriaLabel: "Recent sales",
			Columns: []kit.Column[Sale]{
				{ID: "customer", Header: "Customer", Width: "minmax(0,1fr)",
					Cell: func(s Sale) tela.Node {
						return Div(Class("flex min-w-0 items-center gap-3"),
							kit.Avatar{Size: kit.Lg, Fallback: s.Initials, FallbackClass: s.Tint},
							Div(Class("min-w-0"),
								Div(Class("truncate text-sm font-medium"), Text(s.Name)),
								Div(Class("truncate text-xs text-muted-foreground"), Text(s.Item))))
					}},
				{ID: "amount", Header: "Amount", Align: kit.AlignRight,
					Cell: func(s Sale) tela.Node {
						return Div(Class("text-sm font-semibold text-emerald-700 tabular-nums dark:text-emerald-300"), Text(s.Amount))
					}},
			},
		},
	})
}
