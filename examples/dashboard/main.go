// Command dashboard is an analytics dashboard in the "Frosted graphite"
// style: dark first, a calm dune wallpaper and three fixed panes of real
// glass over it (the rail, the main pane and the Pulse pane), with one
// amber accent. Only the content inside a pane scrolls, so what each blur
// samples never moves. It is the target of the GPU backend (which draws
// it at 60 fps at scale 2; the CPU rasterizer does not), and its
// benchmarks (bench_test.go) measure the CPU path.
//
//   - Overview: a headline with metric tabs, a number and a spectrum of 48
//     bars, four figures with sparklines (all of them springs read by
//     deferred props, tela.UseSpring with h.TextFn and tela.HeightFn), the
//     latest orders, and the Pulse pane (the monthly goal, traffic,
//     activity and the weekly report).
//   - Tasks: one model shown as a sortable table (a finished task sinks,
//     with AnimateLayout) or as a board whose cards are dragged between
//     columns, with menus, a context menu and edit, new and delete dialogs.
//   - Orders: 240 orders in a table filtered by status.
//   - A command palette (Ctrl+K) and the tooltips, built directly on aria
//     hooks; everything else is package kit in a recipe of its own
//     (style.go). LayoutID pills slide in the segmented controls, the rail
//     and the tabs; the theme button switches the color scheme.
//
// `tela snapshot ./examples/dashboard` renders it without a window; add
// --dark for the dark scheme and --preview to pick another state.
package main

import (
	"embed"
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

//go:embed img/*.jpg
var images embed.FS

func init() { tela.Assets(images) }

// rootClass styles the box that fills the window, under the wallpaper.
const rootClass = "bg-background text-foreground"

// Page is the page with its state. Its fields start it in any state: the
// previews use them, and the app starts from Dashboard.
type Page struct {
	View    string // "overview", "tasks" or "orders"
	Mode    string // the tasks' layout: "table" or "board"
	Range   int    // index into periods
	Status  string // the orders' filter
	Palette bool   // the command palette is open
	Query   string // the palette's text
	Report  string // the report dialog is open: "new" or "weekly"
	Toast   string // the page starts with this toast, pinned
	TipOpen bool   // the peak bar's tooltip is open
}

// Dashboard is the root of the app: the page as the user opens it.
func Dashboard() tela.Node {
	return Page{View: "overview", Mode: "table", Range: 1, Status: "All"}
}

// Build puts the page under the style and the toaster.
func (p Page) Build() tela.Node {
	return kit.StyleProvider{Style: &style, Children: kit.Toaster{
		Position: kit.ToastBottomCenter, Children: body{p},
	}}
}

type body struct{ Page }

func (p body) Build() tela.Node {
	toast := kit.UseToast()
	view, setView := tela.UseState(p.View)
	mode, setMode := tela.UseState(p.Mode)
	rng, setRange := tela.UseState(p.Range)
	status, setStatus := tela.UseState(p.Status)
	palette, setPalette := tela.UseState(p.Palette)
	report, setReport := tela.UseState(p.Report)
	weekly, setWeekly := tela.UseState(true)
	board, setBoard := tela.UseState(initialBoard)
	tasks, setTasks := tela.UseState(initialTasks)
	compose, setCompose := tela.UseState(-1) // the column of a new task, -1 while closed
	dark := tela.UseColorScheme() == tela.Dark

	open := board.Count() - board[done].Len()
	theme := "Dark"
	if dark {
		theme = "Light"
	}
	commands := []Command{
		{"go-overview", "Go to Overview", "Navigate", icons.LayoutDashboard, "", "Ctrl 1"},
		{"go-tasks", "Go to Tasks", "Navigate", icons.ListChecks, fmt.Sprintf("%d open", open), "Ctrl 2"},
		{"go-orders", "Go to Orders", "Navigate", icons.Receipt, "", "Ctrl 3"},
		{"new-report", "New report", "Actions", icons.FileText, "", ""},
		{"new-task", "New task", "Actions", icons.Plus, "", ""},
		{"export", "Export orders as CSV", "Actions", icons.Download, "", ""},
		{"theme", "Switch theme", "Actions", icons.Palette, theme, ""},
	}
	run := func(key string) {
		setPalette.Set(false)
		switch key {
		case "go-overview":
			setView.Set("overview")
		case "go-tasks":
			setView.Set("tasks")
		case "go-orders":
			setView.Set("orders")
		case "new-report":
			setReport.Set("new")
		case "new-task":
			setView.Set("tasks")
			setCompose.Set(todo)
		case "export":
			toast.Success("Export started", kit.ToastDescription(fmt.Sprintf("orders.csv, %d rows", len(orders))))
		case "theme":
			toggleTheme(dark)()
		}
	}
	keys := func(e tela.KeyboardEvent) {
		if !(e.Ctrl || e.Meta) || e.Alt || e.Shift {
			return
		}
		switch e.Key {
		case "k":
			setPalette.Set(true)
		case "1", "2", "3":
			setView.Set(views[e.Key[0]-'1'].ID)
		default:
			return
		}
		e.PreventDefault()
	}

	// The orders table scrolls itself; the other views scroll as a whole.
	scroll := "overflow-y-auto"
	var title, sub string
	var right tela.Component
	var content tela.Child
	switch view {
	case "tasks":
		title, sub = "Tasks", fmt.Sprintf("%d tasks", board.Count())
		right = Modes{Mode: mode, Set: setMode}
		content = TasksView{Mode: mode, Board: board, Tasks: tasks, Set: setBoard, SetTasks: setTasks, Compose: setCompose}
	case "orders":
		title, sub = "Orders", fmt.Sprintf("%d orders", len(ordersWith(status)))
		right = Statuses{Status: status, Set: setStatus}
		content = OrdersTable{Status: status, Fill: true}
		scroll = "overflow-hidden"
	default:
		title, sub = "Overview", "Acme workspace"
		right = Periods{Range: rng, Set: setRange}
		content = Fragment(
			Hero{Range: rng, TipOpen: p.TipOpen},
			Div(Class("grid grid-cols-4"),
				MapKeyed(upto(len(stats)), func(i int) int { return i }, func(i int) tela.Component { return StatCell{Index: i, Range: rng} })),
			LatestOrders{SetView: setView},
		)
	}
	return Div(Class("relative isolate flex h-screen gap-3 overflow-hidden p-3"), OnKeyDown(keys),
		Wallpaper(),
		Rail{View: view, SetView: setView},
		Div(Role("main"), Class("flex min-w-0 flex-1 flex-col overflow-hidden", pane),
			TopBar{Title: title, Sub: sub, Right: right, OpenSearch: setPalette},
			Div(Key(view), TestID("content"), Class("flex min-h-0 flex-1 flex-col gap-6 px-6 py-6", scroll), content)),
		If(view == "overview", Pulse{Weekly: weekly, OpenReport: setReport}),
		Portal(If(palette, Palette{Commands: commands, Query: p.Query, Close: func() { setPalette.Set(false) }, Run: run})),
		ReportDialog{Mode: report, SetMode: setReport, Weekly: weekly, SetWeekly: setWeekly},
		NewTask{Col: compose, Close: setCompose, Tasks: tasks, SetBoard: setBoard, SetTasks: setTasks},
		If(p.Toast != "", ShowToast{Message: p.Toast, Pinned: true}),
	)
}

// App is the window: main runs it, the tests mount it (telatest.MountApp)
// and `tela snapshot` renders it at the same size and with the same root
// class and theme.
var App = tela.App{
	Title:     "Tela - dashboard",
	AppID:     "dev.telaui.dashboard",
	Width:     1440,
	Height:    900,
	Root:      Dashboard(),
	RootClass: rootClass,
	Theme:     &theme,
}

// Previews are the states the README and the goldens show, each in App's
// window.
var Previews = tela.Previews{
	"1 Overview":        {Root: Page{View: "overview", Mode: "table", Range: 1, Status: "All", TipOpen: true}},
	"2 Board":           {Root: Page{View: "tasks", Mode: "board", Range: 1, Status: "All"}},
	"3 Tasks":           {Root: Page{View: "tasks", Mode: "table", Range: 1, Status: "All"}},
	"4 Command palette": {Root: Page{View: "overview", Mode: "table", Range: 1, Status: "All", Palette: true}},
	"5 Report dialog":   {Root: Page{View: "overview", Mode: "table", Range: 1, Status: "All", Report: "new"}},
	"6 Toast":           {Root: Page{View: "overview", Mode: "table", Range: 2, Status: "All", Toast: "Report queued: Q3 revenue summary"}},
	"7 Orders":          {Root: Page{View: "orders", Mode: "table", Range: 1, Status: "All"}},
}

func main() { tela.Main(App) }
