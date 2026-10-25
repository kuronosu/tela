package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// KPI is the definition of a stat card: its title, how it reads a Period
// and how it prints the (animated) number.
type KPI struct {
	Title  string
	Icon   tela.Icon
	Tone   string // gradient of the icon chip
	Value  func(Period) float32
	Format func(float32) string
	Deltas [3]string // by period
	Up     [3]bool
}

var kpis = []KPI{
	{"Revenue", icons.DollarSign, "from-indigo-500 to-violet-500", func(p Period) float32 { return p.Revenue }, dollars,
		[3]string{"+8.2%", "+20.1%", "+31.4%"}, [3]bool{true, true, true}},
	{"Subscribers", icons.Users, "from-fuchsia-500 to-pink-500", func(p Period) float32 { return p.Subs }, count,
		[3]string{"+2.4%", "+18.0%", "-1.3%"}, [3]bool{true, true, false}},
	{"Orders", icons.Package, "from-sky-500 to-cyan-500", func(p Period) float32 { return p.Orders }, count,
		[3]string{"-3.1%", "+19.0%", "+12.6%"}, [3]bool{false, true, true}},
	{"Monthly goal", icons.Target, "from-amber-500 to-orange-500", func(p Period) float32 { return p.Goal },
		func(v float32) string { return fmt.Sprintf("%.0f%%", v) },
		[3]string{"+4 pts", "+12 pts", "+9 pts"}, [3]bool{true, true, true}},
}

// lift is the hover reaction of an interactive panel. Only transform and
// shadow transition: the fills do not, so switching the theme repaints the
// page once instead of fading every card.
const lift = "transition-[transform,box-shadow] duration-300 ease-out hover:-translate-y-0.5 hover:shadow-xl hover:shadow-indigo-950/10"

// StatCard is one number: which card, and the period. Both fields are small
// comparable values, so a card that is not affected skips its build. The
// number follows its target through a spring (tela.UseSpring) and is
// printed by a deferred text (h.TextFn): while it counts, the frames only
// lay out the paragraph, and the component builds once, when the period
// changes. The last card shows its value as a meter whose width and color
// are springs too (WidthFn, BgFn).
//
// The card is a kit.Card (its Action slot holds the icon chip); the delta
// is a kit.Badge with the "up" or "down" variant of the glass style.
type StatCard struct{ Index, Range int }

func (p StatCard) Build() tela.Node {
	k := kpis[p.Index]
	target := k.Value(periods[p.Range])
	v := tela.UseSpring(target, tela.Initial(float32(0)), tela.Stiffness(90), tela.Damping(18))

	delta := kit.Badge{Variant: "up", Icon: icons.ArrowUp, Children: Text(k.Deltas[p.Range])}
	if !k.Up[p.Range] {
		delta = kit.Badge{Variant: "down", Icon: icons.ArrowDown, Children: Text(k.Deltas[p.Range])}
	}
	return Fragment(kit.Card{
		Class:       lift + " gap-4 py-5",
		HeaderClass: "px-5", ContentClass: "px-5",
		TitleLevel: 3,
		Title:      Text(k.Title),
		TitleClass: "text-sm font-medium tracking-normal text-muted-foreground",
		Action: Div(Class("flex size-9 items-center justify-center rounded-xl bg-linear-to-br text-white shadow-md", k.Tone),
			Icon(k.Icon, Class("size-4"))),
		Children: Fragment(
			Div(Class("text-3xl font-bold tracking-tight text-foreground tabular-nums"),
				TextFn(func() string { return k.Format(v.Get()) })),
			Div(Class("mt-4 flex items-end justify-between gap-3"),
				Div(Class("flex flex-col gap-1.5"),
					delta,
					Span(Class("text-xs text-muted-foreground"), Text("vs last period")),
				),
				IfElse(p.Index == len(kpis)-1, goalMeter(v), spark(p)),
			)),
	})
}

// spark is the card's mini bar chart: twelve bars that spring to the new
// data when the period changes.
func spark(p StatCard) tela.Node {
	data := series(Metric{seed: float64(p.Index)*1.3 + 0.2}, p.Range)
	return Div(Class("flex h-9 items-end gap-[3px]"),
		MapKeyed(upto(len(data)), func(i int) int { return i }, func(i int) tela.Component {
			return SparkBar{Pct: data[i]}
		}),
	)
}

// SparkBar is one bar of a sparkline: Pct is its height as a fraction of
// the sparkline. Its height is a spring read by a layout-phase closure: the
// 12 bars of 4 cards animate without building.
type SparkBar struct{ Pct float32 }

func (p SparkBar) Build() tela.Node {
	h := tela.UseSpring(p.Pct, tela.Initial(float32(0.08)), tela.Stiffness(170), tela.Damping(20))
	return Div(Class("w-1 rounded-full bg-linear-to-t from-indigo-500/70 to-fuchsia-400/70 dark:from-indigo-400/70 dark:to-fuchsia-300/70"),
		Style(tela.HeightFn(Scale(h, 36))))
}

// goalMeter is a 112px track whose fill follows the animated percentage:
// its width is a layout closure, and its color a paint closure that goes
// from rose through amber to emerald as the value rises (BgFn). It is
// drawn here, not with kit.Progress: a Progress is built from a value that
// changes between builds, and this meter's value changes every frame
// without building.
func goalMeter(v *tela.Value[float32]) tela.Node {
	return Div(Class("h-2.5 w-28 shrink-0 overflow-hidden rounded-full bg-black/10 dark:bg-white/10"), AriaHidden(true),
		Div(Class("h-full rounded-full"),
			Style(
				tela.WidthFn(func() float32 { return 112 * min(v.Get(), 100) / 100 }),
				tela.BgFn(func() tela.Color { return goalColor(v.Get()) }),
			)),
	)
}

// goalColor is rose below 40, amber below 75 and emerald above.
func goalColor(pct float32) tela.Color {
	switch {
	case pct < 40:
		return tela.Hex("#f43f5e")
	case pct < 75:
		return tela.Hex("#f59e0b")
	}
	return tela.Hex("#10b981")
}

// RevenueCard is the bar chart with its metric tabs. The tabs are a
// kit.Tabs with a sliding underline (its Indicator slot has a shared
// LayoutID); the chart is the example's own drawing. The bars spring to the
// data of the metric and the period (Range indexes periods).
type RevenueCard struct{ Range int }

func (p RevenueCard) Build() tela.Node {
	tab, setTab := tela.UseState(metrics[0].ID)
	var m Metric
	for _, mm := range metrics {
		if mm.ID == tab {
			m = mm
		}
	}
	data := series(m, p.Range)
	period := periods[p.Range]
	tabs := make([]kit.Tab[string], len(metrics))
	for i, mm := range metrics {
		tabs[i] = kit.Tab[string]{Value: mm.ID, Label: mm.Label}
	}
	return Fragment(kit.Card{
		Class:      "min-w-0 flex-1",
		TitleLevel: 2, Title: Text("Performance"), Description: Textf("%s, last %s", m.Label, period.Caption),
		// The content of the metric is the chart below, outside the tab list:
		// the tabs are a tab list with no panels.
		Action: kit.Tabs[string]{Tabs: tabs, Variant: kit.TabsLine, Value: &tab, OnValueChange: setTab.Set, AriaLabel: "Metric",
			Indicator: Div(Key("underline"), LayoutID("tab-underline"),
				Class("absolute inset-x-1 -bottom-px h-0.5 rounded-full bg-linear-to-r from-indigo-500 to-fuchsia-500"))},
		Children: Div(Class("mt-2 flex gap-3"),
			Div(Class("flex h-80 flex-col justify-between text-right text-xs leading-none text-muted-foreground tabular-nums"),
				Map([]float32{1, 0.75, 0.5, 0.25, 0}, func(f float32) tela.Node {
					return Span(Key(f), Text(m.Unit+compact(m.Scale*f)))
				}),
			),
			Div(Class("flex min-w-0 flex-1 flex-col"),
				Div(Class("relative h-80"),
					Div(Class("absolute inset-0 flex flex-col justify-between"),
						Map([]int{0, 1, 2, 3, 4}, func(i int) tela.Node {
							return Div(Key(i), Class("h-px bg-black/10 dark:bg-white/10"))
						}),
					),
					Div(Class("absolute inset-0 flex items-end gap-2"),
						MapKeyed(upto(len(data)), func(i int) int { return i }, func(i int) tela.Component {
							return Bar{Pct: data[i], Label: m.Unit + compact(m.Scale*data[i])}
						}),
					),
				),
				Div(Class("mt-2 flex gap-2 text-xs text-muted-foreground"),
					MapIndex(period.XLabels[:], func(i int, l string) tela.Node {
						return Span(Key(i), Class("flex-1 text-center"), Text(l))
					}),
				),
			),
		),
	})
}

// Bar is one column: Pct is its height as a fraction of the plot and Label
// the value its tooltip shows. Its height is a spring read in a layout
// closure, so switching the tab or the period moves the 12 bars without
// building them; hovering the column brightens the bar and shows its value
// (group-hover:), also without building.
type Bar struct {
	Pct   float32
	Label string
}

func (p Bar) Build() tela.Node {
	h := tela.UseSpring(p.Pct, tela.Initial(float32(0)), tela.Stiffness(160), tela.Damping(18))
	return Div(Class("group relative flex h-full flex-1 items-end"),
		Div(Class("relative w-full rounded-t-lg bg-linear-to-t from-indigo-500 to-fuchsia-400"),
			Style(tela.HeightFn(Scale(h, 320))),
			Div(Class("absolute inset-0 rounded-t-lg bg-white/0 transition duration-200 group-hover:bg-white/25")),
			Div(Class("pointer-events-none absolute bottom-full left-1/2 z-10 mb-2 -translate-x-1/2 rounded-lg bg-slate-900/90 px-2 py-1",
				"text-xs font-medium whitespace-nowrap text-white opacity-0 transition duration-200 group-hover:opacity-100"),
				Text(p.Label)),
		),
	)
}

// TrafficCard is the donut of traffic sources: a conic-gradient circle with
// a glass disc over its middle, and a legend.
func TrafficCard() tela.Node { return tela.C(trafficCard) }

func trafficCard() tela.Node {
	return Fragment(kit.Card{
		TitleLevel: 2, Title: Text("Traffic sources"), Description: Text("Where visitors come from"),
		Children: Fragment(
			Div(Class("relative mx-auto my-6 size-44 shrink-0 rounded-full", donut()),
				Div(Class("absolute inset-6 flex flex-col items-center justify-center rounded-full border border-border bg-popover backdrop-blur-md"),
					Div(Class("text-2xl font-bold tracking-tight tabular-nums"), Text("48.2k")),
					Div(Class("text-xs text-muted-foreground"), Text("visitors")),
				),
			),
			Div(Class("flex flex-col gap-1"),
				Map(sources, func(s Source) tela.Node {
					return Div(Key(s.Name), Class("flex items-center gap-3 rounded-lg px-2 py-1.5 text-sm transition-colors duration-200 hover:bg-accent/60"),
						Span(Class("size-2.5 rounded-full", s.Color)),
						Span(Class("flex-1"), Text(s.Name)),
						Span(Class("font-medium tabular-nums"), Textf("%d%%", s.Pct)),
					)
				}),
			),
		),
	})
}

// compact prints v as 9k or 6.8k, or the number itself below 1000.
func compact(v float32) string {
	if v >= 1000 {
		if r := v / 1000; r == float32(int(r)) {
			return fmt.Sprintf("%dk", int(r))
		} else {
			return fmt.Sprintf("%.1fk", r)
		}
	}
	return fmt.Sprintf("%d", int(v))
}
