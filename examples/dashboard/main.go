// Command dashboard is an admin dashboard in the style of shadcn/ui's
// dashboard example: a header with navigation, four stat cards, a bar chart
// built from divs and a list of recent sales with avatar initials.
//
// The tabs under the title are state (UseState) and the chart bars reveal
// their value on hover (group-hover:). The page fits a 1280x800 window and
// degrades to small windows with wrapping and the md:/lg: breakpoints; a
// window too short for the content scrolls like a page (the outer element
// is min-h-screen, not h-screen).
//
// `tela snapshot ./examples/dashboard` renders it without a window; add
// --dark for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Dashboard is the page. Its colors come in pairs (light, dark:), the way
// shadcn's zinc theme is written in Tailwind.
func Dashboard() tela.Node {
	return Div(Class("flex min-h-screen flex-col overflow-hidden bg-zinc-50 text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50"),
		tela.C(Header),
		Div(Class("flex min-h-0 flex-1 flex-col gap-4 px-6 pt-6 pb-8 md:px-8"),
			Div(Class("flex flex-wrap items-center justify-between gap-3"),
				H2(Class("text-3xl font-bold tracking-tight"), Text("Dashboard")),
				Div(Class("flex items-center gap-2"),
					Div(Class("flex h-9 items-center rounded-md border border-zinc-200 bg-white px-3 text-sm font-medium shadow-xs dark:border-zinc-800 dark:bg-zinc-900"),
						Text("Jan 20, 2025 – Feb 09, 2025")),
					tela.C(DownloadButton),
				),
			),
			tela.C(Tabs),
			Div(Class("flex flex-wrap gap-4"),
				Map(stats, func(s Stat) tela.Node { return tela.WithKey(tela.CP(StatCard, s), s.Title) }),
			),
			Div(Class("flex min-h-0 flex-1 flex-wrap gap-4"),
				tela.C(OverviewCard),
				tela.C(RecentSalesCard),
			),
		),
	)
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return Dashboard() }

func main() {
	tela.Run(tela.App{
		Title:  "Tela — dashboard",
		AppID:  "dev.tela.dashboard",
		Width:  1280,
		Height: 800,
		Root:   Dashboard,
	})
}
