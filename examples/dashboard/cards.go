package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Stat is the data of one stat card. It is comparable, so StatCard skips
// its build when nothing about it changed.
type Stat struct {
	Title, Value, Note string
	Icon               string
}

var stats = []Stat{
	{"Total Revenue", "$45,231.89", "+20.1% from last month", "revenue"},
	{"Subscriptions", "+2,350", "+180.1% from last month", "users"},
	{"Sales", "+12,234", "+19% from last month", "sales"},
	{"Active Now", "+573", "+201 since last hour", "activity"},
}

// StatCard shows one number with a caption and an icon.
func StatCard(s Stat) tela.Node {
	// Two cards per row below lg, four from lg up (no grid yet).
	return Div(Class(card+" flex w-[calc(50%-8px)] shrink-0 grow flex-col gap-2 p-6 lg:w-[calc(25%-12px)]"),
		Div(Class("flex items-center justify-between"),
			H3(Class("text-sm font-medium"), Text(s.Title)),
			statIcon(s.Icon),
		),
		Div(Class("text-2xl font-bold tracking-tight"), Text(s.Value)),
		P(Class("text-xs text-zinc-500 dark:text-zinc-400"), Text(s.Note)),
	)
}

// statIcon draws the card icons from divs: the embedded fonts have no icon
// glyphs and SVG arrives later.
func statIcon(name string) tela.Node {
	const ink = "border-zinc-400 dark:border-zinc-500"
	const fill = "bg-zinc-400 dark:bg-zinc-500"
	switch name {
	case "revenue":
		return Div(Class("flex size-4 items-center justify-center text-base leading-none font-semibold text-zinc-400 dark:text-zinc-500"), Text("$"))
	case "users":
		return Div(Class("relative size-4"),
			Div(Class("absolute top-0 left-1 size-2 rounded-full border-[1.5px] "+ink)),
			Div(Class("absolute bottom-0 left-0 h-1.5 w-4 rounded-t-full border-[1.5px] border-b-0 "+ink)),
		)
	case "sales":
		return Div(Class("relative h-3 w-4 rounded-[3px] border-[1.5px] "+ink),
			Div(Class("absolute top-[3px] -right-px -left-px h-[1.5px] "+fill)),
		)
	default: // activity: a small bar graph
		return Div(Class("flex size-4 items-center gap-[2px]"),
			Div(Class("h-1.5 w-[2px] rounded-full "+fill)),
			Div(Class("h-3.5 w-[2px] rounded-full "+fill)),
			Div(Class("h-2.5 w-[2px] rounded-full "+fill)),
			Div(Class("h-4 w-[2px] rounded-full "+fill)),
			Div(Class("h-1 w-[2px] rounded-full "+fill)),
		)
	}
}

// Month is one bar of the chart: its label and its height as a percentage
// of the y axis.
type Month struct {
	Name string
	Pct  int
}

var months = []Month{
	{"Jan", 74}, {"Feb", 66}, {"Mar", 86}, {"Apr", 52}, {"May", 40}, {"Jun", 96},
	{"Jul", 58}, {"Aug", 92}, {"Sep", 48}, {"Oct", 36}, {"Nov", 70}, {"Dec", 82},
}

var yTicks = []string{"$5,000", "$3,750", "$2,500", "$1,250", "$0"}

// OverviewCard is the bar chart: a y axis, dashed gridlines and one flex
// item per month whose height is a percentage of the plot.
func OverviewCard() tela.Node {
	return Div(Class(card+" flex min-h-[300px] min-w-[440px] flex-[4] flex-col p-6"),
		H3(Class("leading-none font-semibold tracking-tight"), Text("Overview")),
		P(Class("mt-1.5 text-sm text-zinc-500 dark:text-zinc-400"), Text("Monthly revenue for the current year.")),
		Div(Class("mt-6 flex min-h-0 flex-1 gap-3"),
			// Y axis labels: centered on the gridlines (both have 8px of padding).
			Div(Class("flex flex-col pb-6"),
				Div(Class("flex flex-1 flex-col justify-between py-0 text-right text-xs leading-4 text-zinc-500 tabular-nums dark:text-zinc-400"),
					Map(yTicks, func(t string) tela.Node { return Span(Key(t), Text(t)) }),
				),
			),
			Div(Class("flex min-w-0 flex-1 flex-col"),
				Div(Class("relative flex-1"),
					Div(Class("absolute inset-0 flex flex-col justify-between py-2"),
						Map(yTicks, func(t string) tela.Node {
							return Div(Key(t), Class("h-px bg-zinc-100 dark:bg-zinc-800"))
						}),
					),
					Div(Class("absolute inset-x-0 top-2 bottom-2 flex items-end gap-2"),
						Map(months, func(m Month) tela.Node { return tela.WithKey(tela.CP(Bar, m), m.Name) }),
					),
				),
				Div(Class("flex h-6 items-end gap-2"),
					Map(months, func(m Month) tela.Node {
						return Div(Key(m.Name), Class("flex-1 text-center text-xs text-zinc-500 dark:text-zinc-400"), Text(m.Name))
					}),
				),
			),
		),
	)
}

// Bar is one column of the chart. The whole column is the hover target; its
// value appears above the bar through group-hover:.
func Bar(m Month) tela.Node {
	return Div(Class("group flex h-full flex-1 cursor-pointer items-end"),
		Div(Class(fmt.Sprintf("relative w-full rounded-t-md bg-zinc-900 group-hover:bg-indigo-500 dark:bg-zinc-100 h-[%d%%]", m.Pct)),
			Div(Class("absolute -top-8 left-1/2 -translate-x-1/2 rounded-md bg-zinc-900 px-2 py-1 text-xs font-medium whitespace-nowrap text-white opacity-0 shadow-md group-hover:opacity-100 dark:bg-zinc-50 dark:text-zinc-900"),
				Textf("$%d,%03d", m.Pct*50/1000, m.Pct*50%1000)),
		),
	)
}

// Sale is one row of the recent sales list.
type Sale struct {
	Name, Email, Amount string
	Initials, Colors    string
}

var sales = []Sale{
	{"Olivia Martin", "olivia.martin@email.com", "+$1,999.00", "OM", "bg-rose-100 text-rose-700 dark:bg-rose-500/20 dark:text-rose-300"},
	{"Jackson Lee", "jackson.lee@email.com", "+$39.00", "JL", "bg-sky-100 text-sky-700 dark:bg-sky-500/20 dark:text-sky-300"},
	{"Isabella Nguyen", "isabella.nguyen@email.com", "+$299.00", "IN", "bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-300"},
	{"William Kim", "will@email.com", "+$99.00", "WK", "bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300"},
	{"Sofia Davis", "sofia.davis@email.com", "+$39.00", "SD", "bg-violet-100 text-violet-700 dark:bg-violet-500/20 dark:text-violet-300"},
}

// RecentSalesCard lists the latest sales.
func RecentSalesCard() tela.Node {
	return Div(Class(card+" flex min-w-[320px] flex-[3] flex-col p-6"),
		H3(Class("leading-none font-semibold tracking-tight"), Text("Recent Sales")),
		P(Class("mt-1.5 text-sm text-zinc-500 dark:text-zinc-400"), Text("You made 265 sales this month.")),
		Div(Class("mt-6 flex flex-1 flex-col justify-between gap-4"),
			Map(sales, func(s Sale) tela.Node { return tela.WithKey(tela.CP(SaleRow, s), s.Email) }),
		),
	)
}

// SaleRow is an avatar, a name with an email and an amount.
func SaleRow(s Sale) tela.Node {
	return Div(Class("flex items-center gap-4"),
		Avatar(s.Initials, "size-9 text-sm "+s.Colors),
		Div(Class("min-w-0 flex-1"),
			P(Class("truncate text-sm leading-none font-medium"), Text(s.Name)),
			P(Class("mt-1.5 truncate text-sm text-zinc-500 dark:text-zinc-400"), Text(s.Email)),
		),
		Div(Class("text-sm font-medium tabular-nums"), Text(s.Amount)),
	)
}
