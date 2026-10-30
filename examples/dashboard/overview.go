package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Hero is the headline: tabs for the metric (a kit.Tabs whose amber line
// slides between them, a LayoutID), the number and the spectrum of 48 thin
// bars. The number follows its target through a spring (tela.UseSpring)
// printed by a deferred text (h.TextFn), and every bar's height is a spring
// read by a layout closure (tela.HeightFn): switching the period or the
// metric moves all of them without building anything per frame.
type Hero struct {
	Range   int
	TipOpen bool // the peak's tooltip starts open (a preview's seed)
}

func (p Hero) Build() tela.Node {
	tab, setTab := tela.UseState(metrics[0].ID)
	m := metricOf(tab)
	per := periods[p.Range]
	v := tela.UseSpring(m.Value(per), tela.Initial(float32(0)), tela.Stiffness(90), tela.Damping(18))
	data := series(48, m.Seed, p.Range)
	peak := 0
	for i, x := range data {
		if x > data[peak] {
			peak = i
		}
	}
	tabs := make([]kit.Tab[string], len(metrics))
	for i, mm := range metrics {
		tabs[i] = kit.Tab[string]{Value: mm.ID, Label: mm.Label}
	}
	return Div(Class("flex flex-col gap-5"),
		Div(Class("flex items-end justify-between"),
			Div(
				Div(Class("flex items-center gap-4"),
					kit.Tabs[string]{Tabs: tabs, Variant: kit.TabsLine, Value: &tab, OnValueChange: setTab.Set, AriaLabel: "Metric",
						Indicator: Div(Key("line"), LayoutID("metric-line"), Class("absolute inset-x-0 bottom-0 h-0.5 rounded-full bg-primary"))},
					Span(Class("pb-1.5 opacity-70", eyebrow), Textf("· last %s", per.Caption))),
				Div(Class("mt-3 flex items-baseline gap-4"),
					Div(Class("text-7xl leading-none font-extralight tracking-tight tabular-nums"),
						TextFn(func() string { return m.Format(v.Get()) })),
					Span(Class("inline-flex items-center gap-1 rounded-md bg-primary/15 px-1.5 py-0.5 text-xs font-semibold text-accent-text"),
						Icon(icons.ArrowUpRight, Class("size-3.5")), Text(m.Deltas[p.Range])))),
			Div(Class("flex items-center gap-4 pb-1 text-xs text-muted-foreground"), AriaHidden(true),
				Span(Class("flex items-center gap-1.5"), Span(Class("size-2 rounded-sm bg-primary")), Text("Peak day")),
				Span(Class("flex items-center gap-1.5"), Span(Class("size-2 rounded-sm bg-foreground/25")), Text("Daily")))),
		Div(Class("relative mt-6 h-44"),
			Div(Class("absolute inset-0 flex flex-col justify-between"), AriaHidden(true),
				Map([]int{0, 1, 2, 3}, func(i int) tela.Node { return Div(Key(i), Class("h-px border-t border-dashed border-border")) })),
			Div(Class("absolute inset-0 flex items-end gap-[5px]"),
				MapKeyed(upto(len(data)), func(i int) int { return i }, func(i int) tela.Component {
					label := tickLabel(m, data[i])
					if i == peak {
						label += " · peak"
					}
					return Tick{Pct: data[i], Peak: i == peak, Label: label, TipOpen: p.TipOpen && i == peak}
				}))),
		Div(Class("flex justify-between text-xs text-muted-foreground tabular-nums"), AriaHidden(true),
			MapIndex(per.XLabels[:], func(i int, l string) tela.Node { return Span(Key(i), Text(l)) })),
	)
}

// tickLabel is the value of a bar of the spectrum, in the metric's unit.
func tickLabel(m Metric, pct float32) string {
	v := count(pct * m.Scale)
	if m.ID == "revenue" {
		return "$" + v
	}
	return v
}

// Tick is one thin bar of the spectrum, with its value in a tooltip (Tip).
// The peak is amber; the others brighten under the pointer (group-hover:,
// no build).
type Tick struct {
	Pct     float32
	Peak    bool
	Label   string
	TipOpen bool
}

func (p Tick) Build() tela.Node {
	h := tela.UseSpring(p.Pct, tela.Initial(float32(0)), tela.Stiffness(150), tela.Damping(18))
	fill := "bg-foreground/20 transition-colors duration-150 group-hover:bg-foreground/60"
	if p.Peak {
		fill = "bg-primary"
	}
	return Fragment(Tip{Text: p.Label, Placement: "top", Open: p.TipOpen, Render: func(tp tela.Prop) tela.Node {
		return Div(tp, Class("group flex h-full flex-1 items-end"),
			Div(Class("w-full rounded-t-[3px]", fill), Style(tela.HeightFn(Scale(h, 176)))))
	}})
}

// StatCell is one of the four figures under the headline: its number and
// its 24-bar sparkline follow springs as the period changes.
type StatCell struct{ Index, Range int }

func (p StatCell) Build() tela.Node {
	st := stats[p.Index]
	v := tela.UseSpring(st.Value(periods[p.Range]), tela.Initial(float32(0)), tela.Stiffness(90), tela.Damping(18))
	tone, icon := "text-muted-foreground", icons.ArrowUpRight
	if !st.Up[p.Range] {
		tone, icon = "text-negative", icons.ArrowDownRight
	}
	data := series(24, float64(p.Index)*1.7+0.3, p.Range)
	return Div(Class("flex flex-col gap-2 border-l border-border px-5 first:border-l-0 first:pl-0 last:pr-0"),
		H2(Class(eyebrow), Text(st.Label)),
		Div(Class("flex items-baseline gap-2"),
			Span(Class("text-2xl font-light tracking-tight tabular-nums"), TextFn(func() string { return st.Format(v.Get()) })),
			Span(Class("inline-flex items-center text-xs", tone), Icon(icon, Class("size-3")), Text(st.Deltas[p.Range]))),
		Div(Class("flex h-6 items-end gap-[2px]"), AriaHidden(true),
			MapKeyed(upto(len(data)), func(i int) int { return i }, func(i int) tela.Component { return Spark{Pct: data[i]} })))
}

// Spark is one bar of a sparkline.
type Spark struct{ Pct float32 }

func (p Spark) Build() tela.Node {
	h := tela.UseSpring(p.Pct, tela.Initial(float32(0.1)), tela.Stiffness(170), tela.Damping(20))
	return Div(Class("w-[3px] rounded-full bg-foreground/25"), Style(tela.HeightFn(Scale(h, 24))))
}

var statusTone = map[string]string{"Paid": "bg-foreground/70", "Pending": "bg-primary", "Refunded": "bg-foreground/30", "Failed": "bg-negative"}

// OrdersTable is the orders in a kit.Table: a scroller of its own (Class
// gives it a height), with a sticky head.
type OrdersTable struct {
	Status string // the filter: "All" or a status
	Limit  int    // at most this many rows, the newest; 0 for all
	Class  string // of the table, such as its height
	Fill   bool   // the table fills the height of its parent
}

func (p OrdersTable) Build() tela.Node {
	rows := ordersWith(p.Status)
	if p.Limit > 0 {
		rows = rows[:min(p.Limit, len(rows))]
	}
	class := p.Class
	if p.Fill {
		class = "h-full"
	}
	return Div(Class("overflow-hidden rounded-xl border border-border bg-card", when(p.Fill, "min-h-0 flex-1")),
		kit.Table[Order]{
			Rows: rows, RowKey: func(o Order) any { return o.ID }, AriaLabel: "Orders", Class: class,
			Columns: []kit.Column[Order]{
				{ID: "id", Header: "Order", Width: "5.5rem", Cell: func(o Order) tela.Node {
					return Span(Class("font-mono text-xs text-muted-foreground"), Text(o.ID))
				}},
				{ID: "customer", Header: "Customer", Width: "minmax(12rem,1fr)", Cell: func(o Order) tela.Node {
					return Div(Class("flex items-center gap-2.5"), kit.Avatar{Fallback: o.Initials, Class: "size-6"},
						Span(Class("text-[13px] font-medium"), Text(o.Customer)),
						Span(Class("text-xs text-muted-foreground"), Text(o.Email)))
				}},
				{ID: "plan", Header: "Plan", Width: "6.5rem", Cell: func(o Order) tela.Node {
					return Span(Class("text-[13px] text-muted-foreground"), Text(o.Plan))
				}},
				{ID: "status", Header: "Status", Width: "7.5rem", Cell: func(o Order) tela.Node {
					return Span(Class("inline-flex items-center gap-2 text-[13px]"), Span(Class("size-1.5 rounded-full", statusTone[o.Status])), Text(o.Status))
				}},
				{ID: "date", Header: "Date", Width: "5.5rem", Cell: func(o Order) tela.Node {
					return Span(Class("text-[13px] text-muted-foreground"), Text(o.Date))
				}},
				{ID: "amount", Header: "Amount", Width: "7rem", Align: kit.AlignRight, Cell: func(o Order) tela.Node {
					return Span(Class("font-mono text-[13px] tabular-nums"), Text(dollars(o.Amount)))
				}},
			},
		})
}

// LatestOrders is the overview's table: the 30 newest orders in a short
// scroller, and a link to the orders view, which has all of them. (A
// kit.Table builds and lays out every row: 240 rows here would cost every
// frame that lays out the overview, such as those of the springs.)
type LatestOrders struct{ SetView tela.Setter[string] }

func (p LatestOrders) Build() tela.Node {
	return Div(Class("flex flex-col"),
		Div(Class("mb-3 flex items-center justify-between"),
			H2(Class(eyebrow), Text("Latest orders")),
			Div(Class("flex items-center gap-3 text-xs text-muted-foreground tabular-nums"),
				Textf("%d total", len(orders)),
				kit.Button{Variant: kit.Link, Size: kit.Sm, Class: "h-auto px-0 text-xs", OnClick: func() { p.SetView.Set("orders") },
					Children: Text("View all")})),
		OrdersTable{Status: "All", Limit: 30, Class: "max-h-[17rem]"},
	)
}

// Pulse is the right-hand pane: the monthly goal as a ring, the traffic as
// thin meters, the activity feed and the weekly report.
type Pulse struct {
	Weekly     bool                // the report goes out every Monday
	OpenReport tela.Setter[string] // opens the report dialog
}

func (p Pulse) Build() tela.Node {
	goal := tela.UseSpring(float32(72), tela.Initial(float32(0)), tela.Stiffness(60), tela.Damping(16))
	next := "Not scheduled"
	if p.Weekly {
		next = "Next: Monday, 9:00"
	}
	return Div(Role("complementary"), AriaLabel("Pulse"), Class("flex w-80 shrink-0 flex-col gap-6 overflow-hidden p-5", pane),
		Div(
			H2(Class(eyebrow), Text("Monthly goal")),
			Div(Class("mt-4 flex items-center gap-5"),
				Div(Class("relative size-24 shrink-0"), Role("meter"), AriaLabel("Monthly goal"), AriaValueNow(72),
					// The ring is a conic gradient with a fixed amber: a token
					// cannot be used inside an arbitrary value.
					Div(Class("absolute inset-0 rounded-full bg-[conic-gradient(#f6b94a_0%,#f6b94a_72%,rgb(128_128_128/0.25)_72%)]")),
					Div(Class("absolute inset-[7px] flex items-center justify-center rounded-full bg-background/80"),
						Span(Class("text-2xl font-light tabular-nums"), TextFn(func() string { return fmt.Sprintf("%.0f%%", goal.Get()) })))),
				Div(Class("text-sm leading-relaxed text-muted-foreground"),
					Span(Class("font-medium text-foreground"), Text("$32.6k")), Text(" of $45k, with 9 days left."))),
		),
		Div(
			H2(Class(eyebrow), Text("Traffic")),
			Div(Class("mt-3 flex flex-col gap-3"),
				MapIndex(sources, func(i int, s Source) tela.Node {
					return Div(Key(s.Name), Class("flex flex-col gap-1.5"),
						Div(Class("flex justify-between text-[13px]"), Span(Text(s.Name)), Span(Class("text-muted-foreground tabular-nums"), Textf("%d%%", s.Pct))),
						Div(Class("h-1 overflow-hidden rounded-full bg-border"), AriaHidden(true),
							Div(Class("h-full rounded-full", when(i == 0, "bg-primary"), when(i != 0, "bg-foreground/40"), fmt.Sprintf("w-[%d%%]", s.Pct*2)))))
				}))),
		Div(Class("flex min-h-0 flex-1 flex-col"),
			H2(Class(eyebrow), Text("Activity")),
			Div(Class("mt-3 flex flex-col"),
				Map(feed, func(e Event) tela.Node {
					return Div(Key(e.Who), Class("flex items-center gap-3 border-t border-border py-2.5 first:border-t-0"),
						kit.Avatar{Fallback: e.Initials, Class: "size-7"},
						Div(Class("min-w-0 flex-1"),
							Div(Class("truncate text-[13px] font-medium"), Text(e.Who)),
							Div(Class("truncate text-xs text-muted-foreground"), Text(e.What))),
						Span(Class("text-xs text-muted-foreground tabular-nums"), Text(e.Ago)))
				}))),
		Div(Class("mt-auto flex items-center gap-3 rounded-xl border border-primary/25 bg-primary/10 p-3"),
			Div(Class("flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground"), AriaHidden(true),
				Icon(icons.Calendar, Class("size-4"))),
			Div(Class("min-w-0 flex-1"),
				Div(Class("text-[13px] font-medium"), Text("Weekly report")),
				Div(Class("text-xs text-muted-foreground"), Text(next))),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, AriaLabel: "Edit the weekly report", OnClick: func() { p.OpenReport.Set("weekly") },
				Children: Text("Edit")}),
	)
}
