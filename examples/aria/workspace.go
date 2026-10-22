package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/aria/icons"
	. "github.com/kuronosu/tela/h"
)

// card is the surface of a section of the page.
const card = "rounded-xl border border-border bg-card text-card-foreground shadow-sm"

// tabs are the keys of the workspace's tab list, with their labels.
var tabKeys = []string{"overview", "activity", "alerts"}

var tabLabels = map[string]string{"overview": "Overview", "activity": "Activity", "alerts": "Alerts"}

// Workspace is the card with the tabs. UseTabs holds the selected tab and the
// keys that move between tabs (arrows, Home, End; the selection follows the
// focus); the card renders the tab list and the panel of the selected tab.
type Workspace struct {
	Rows []Deployment
	// Columns are the optional columns that are shown: "branch", "duration"
	// and "author".
	Columns aria.Selection[string]
	// Project is the project the page is about: its rows are marked.
	Project string
	// Tab is the tab the card starts on.
	Tab string
}

func (w Workspace) Build() tela.Node {
	first := w.Tab
	if first == "" {
		first = "overview"
	}
	tabs := aria.UseTabs(aria.TabsOptions[string]{
		Keys: tabKeys, DefaultSelected: aria.SelectionOf(first), AriaLabel: "Sections",
	})
	current, _ := tabs.Selected.First()

	var panel tela.Node
	switch current {
	case "activity":
		panel = activityPanel()
	case "alerts":
		panel = alertsPanel()
	default:
		panel = overviewPanel(w)
	}

	return Div(Class(card, "flex min-w-0 flex-1 flex-col"),
		Div(Class("flex items-center justify-between border-b border-border px-4 py-3"),
			Div(tabs.Props, Class("inline-flex h-9 items-center rounded-lg bg-muted p-[3px] text-muted-foreground"),
				Range(len(tabKeys), func(i int) tela.Node {
					k := tabKeys[i]
					return Div(tabs.TabProps(k), Key(k),
						Class("inline-flex h-7 cursor-default items-center justify-center rounded-md px-3 text-sm font-medium whitespace-nowrap transition-colors",
							"hover:text-foreground aria-selected:bg-background aria-selected:text-foreground aria-selected:shadow-sm", focusRing),
						Text(tabLabels[k]))
				})),
			Div(Class("flex items-center gap-2 text-xs text-muted-foreground"),
				ic(icons.Clock, "size-3.5"), Text("Updated just now")),
		),
		Div(tabs.PanelProps(current), Class("flex flex-col gap-4 p-4 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 rounded-b-xl"), panel),
	)
}

// stat is one of the three figures above the list.
func stat(label, value, hint string) tela.Node {
	return Div(Class("flex flex-col gap-1 rounded-lg border border-border p-3"),
		Span(Class("text-xs text-muted-foreground"), Text(label)),
		Span(Class("text-2xl font-semibold tracking-tight tabular-nums"), Text(value)),
		Span(Class("text-xs text-muted-foreground"), Text(hint)))
}

func overviewPanel(w Workspace) tela.Node {
	return Fragment(
		Div(Class("grid grid-cols-3 gap-3"),
			stat("Deployments", "128", "last 30 days"),
			stat("Success rate", "97.4%", "+1.2% from last month"),
			stat("Median time", "3m 12s", "-18s from last month")),
		Div(Class("flex flex-col"),
			H3(Class("pb-2 text-sm font-medium"), Text("Recent deployments")),
			Div(Role("list"), AriaLabel("Recent deployments"),
				Class("overflow-hidden rounded-lg border border-border"),
				Range(len(w.Rows), func(i int) tela.Node {
					d := w.Rows[i]
					return deploymentRow(d, w.Columns, d.Project == w.Project)
				})),
		),
	)
}

// statusIcon is the icon and the color of a status.
func statusIcon(s Status) tela.Node {
	switch s {
	case Succeeded:
		return ic(icons.CircleCheck, "size-4 text-emerald-600 dark:text-emerald-500")
	case Failed:
		return ic(icons.CircleX, "size-4 text-destructive")
	case Running:
		return ic(icons.LoaderCircle, "size-4 text-blue-600 dark:text-blue-400")
	default:
		return ic(icons.Clock, "size-4 text-muted-foreground")
	}
}

func deploymentRow(d Deployment, cols aria.Selection[string], current bool) tela.Node {
	return Div(Role("listitem"), Key(d.Project+" "+d.Version),
		Class("flex items-center gap-3 border-t border-border px-4 py-3 text-sm first:border-t-0"),
		If(current, Class("bg-accent/50")),
		Span(AriaLabel(string(d.Status)), Role("img"), statusIcon(d.Status)),
		Div(Class("flex min-w-0 flex-1 flex-col gap-0.5"),
			Div(Class("flex items-center gap-2"),
				Span(Class("truncate font-medium"), Text(d.Project)),
				Span(Class("rounded-md border border-border px-1.5 py-px text-[11px] leading-4 text-muted-foreground"), Text(EnvLabel(d.Env)))),
			Span(Class("text-xs text-muted-foreground tabular-nums"), Text(d.Version))),
		If(cols.Has("branch"),
			Div(Class("flex w-40 items-center gap-1.5 text-xs text-muted-foreground"),
				ic(icons.GitBranch, "size-3.5"), Span(Class("truncate"), Text(d.Branch)))),
		If(cols.Has("duration"),
			Span(Class("w-16 text-right text-xs text-muted-foreground tabular-nums"), Text(d.Duration))),
		If(cols.Has("author"),
			Span(Class("w-28 truncate text-xs text-muted-foreground"), Text(d.Author))),
		Span(Class("w-20 text-right text-xs text-muted-foreground"), Text(d.Age)),
	)
}

func activityPanel() tela.Node {
	return Div(Role("list"), AriaLabel("Activity"), Class("flex flex-col"),
		Range(len(ActivityLog), func(i int) tela.Node {
			a := ActivityLog[i]
			return Div(Role("listitem"), Class("flex items-start gap-3 border-t border-border py-3 text-sm first:border-t-0 first:pt-0"),
				avatar(a.Who, "size-8 text-xs"),
				Div(Class("flex min-w-0 flex-1 flex-col gap-0.5"),
					P(Strong(Class("font-medium"), Text(a.Who)), Text(" "+a.What+" "),
						Strong(Class("font-medium"), Text(a.Where))),
					Span(Class("text-xs text-muted-foreground"), Text(a.When))))
		}))
}

func alertsPanel() tela.Node {
	return Div(Class("flex flex-col gap-3"),
		Range(len(Alerts), func(i int) tela.Node {
			a := Alerts[i]
			tone := "text-amber-600 dark:text-amber-500"
			if a.Severe {
				tone = "text-destructive"
			}
			return Div(Class("flex items-start gap-3 rounded-lg border border-border p-3"),
				ic(icons.TriangleAlert, "mt-0.5 size-4", tone),
				Div(Class("flex flex-col gap-1"),
					Span(Class("text-sm font-medium"), Text(a.Title)),
					Span(Class("text-sm text-muted-foreground"), Text(a.Detail))))
		}))
}

// avatar is a circle with initials, in the chart color of the member.
func avatar(name string, size string) tela.Node {
	chart := 1
	for _, m := range Members {
		if m.Name == name {
			chart = m.Chart
		}
	}
	tint := fmt.Sprintf("bg-chart-%d", chart)
	return Span(Class("flex shrink-0 items-center justify-center rounded-full font-medium text-white", tint, size),
		AriaHidden(true), Text(initials(name)))
}
