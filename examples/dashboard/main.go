// Command dashboard is an analytics dashboard in the glassmorphism style:
// frosted panels over a colorful backdrop, in light and dark. It shows what
// Tela's phase-6 vocabulary does together on one screen, and doubles as the
// 60 fps target of that phase (bench_test.go drives it at 1920x1080).
//
//   - Layout: CSS grid for the page (a sidebar column and the content) and
//     for the card rows, a sticky sidebar and header, a fixed button.
//   - Glass: translucent panels over an image registered with tela.Assets,
//     backdrop-blur where there is detail to frost (the header, the dialog,
//     the toast, the donut's disc), a bg-clip-text gradient headline, a
//     conic-gradient donut and a mix-blend-overlay highlight. backdrop.go
//     explains what a blur costs and where it is spent.
//   - Motion: transitions on hover, a spinner and a pulse while the page
//     syncs, springs for the numbers and the bars (UseSpring with HeightFn,
//     WidthFn and BgFn, so nothing builds while they move), AnimateLayout for
//     the sortable list and the expanding card, LayoutID for the sliding
//     nav, range and tab highlights.
//   - Overlays: a dialog and a toast in an h.Portal, entering with starting:
//     and leaving with h.Exit or tela.UsePresence.
//   - Environment: dark: classes, a theme switch with tela.SetColorScheme
//     and tela.UseColorScheme (the backdrop picks its image).
//
// `tela snapshot ./examples/dashboard` renders it without a window; add
// --dark for the dark theme and --preview to pick another state.
package main

import (
	"embed"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

//go:embed img/*.jpg
var images embed.FS

func init() { tela.Assets(images) }

// rootClass styles the box that fills the window: the canvas under Backdrop.
const rootClass = "bg-slate-50 dark:bg-slate-950"

// AppState is what the whole page shares through AppCtx: the selected
// period and the setters that the far corners of the page need. It is
// comparable, so a component that reads the context only rebuilds when the
// period changes.
type AppState struct {
	Range     int
	SetRange  tela.Setter[int]
	SetDialog tela.Setter[bool]
	SetToast  tela.Setter[string]
}

// AppCtx carries the AppState.
var AppCtx = tela.CreateContext(AppState{})

// Props starts the page in any state; the zero Props is the real app. The
// Previews use it to show the dialog and the toast.
type Props struct {
	Range  int    // index into periods
	Dialog bool   // the new-report dialog is open
	Toast  string // the toast's message, empty for none
	Pinned bool   // the toast stays until dismissed (a snapshot has no clock to count down)
	Sort   string // the task order: "priority" (default), "due" or "name"
}

// Dashboard is the page.
func Dashboard() tela.Node { return tela.CP(Page, Props{Range: 1}) }

// Page is the page with its state.
func Page(p Props) tela.Node {
	rng, setRange := tela.UseState(p.Range)
	dialog, setDialog := tela.UseState(p.Dialog)
	toast, setToast := tela.UseState(p.Toast)

	app := AppState{Range: rng, SetRange: setRange, SetDialog: setDialog, SetToast: setToast}
	return AppCtx.Provider(app,
		Div(Class("relative isolate min-h-screen text-slate-900 dark:text-slate-100"),
			tela.C(Backdrop),
			Div(Class("grid min-h-screen grid-cols-1 lg:grid-cols-[16rem_1fr]"),
				tela.C(Sidebar),
				Div(Class("flex min-w-0 flex-col"),
					tela.C(Header),
					Div(Class("flex flex-col gap-6 px-6 pt-8 pb-24 md:px-8"),
						tela.C(Hero),
						Div(Class("grid grid-cols-2 gap-4 xl:grid-cols-4"),
							MapIndex(kpis, func(i int, k KPI) tela.Node {
								return tela.WithKey(tela.CP(StatCard, StatProps{Index: i, Range: rng}), k.Title)
							}),
						),
						Div(Class("grid grid-cols-1 gap-4 xl:grid-cols-3"),
							Div(Class("flex xl:col-span-2"), tela.CP(RevenueCard, RevenueProps{Range: rng})),
							tela.C(TrafficCard),
						),
						Div(Class("grid grid-cols-1 items-start gap-4 xl:grid-cols-3"),
							tela.CP(TasksCard, TasksProps{Sort: p.Sort}),
							tela.C(InsightCard),
							tela.C(ActivityCard),
						),
					),
				),
			),
			tela.C(HelpButton),
		),
		Portal(
			If(dialog, Scrim(func() { setDialog.Set(false) })),
			Presence(If(dialog, tela.CP(ReportDialog, DialogProps{Close: setDialog, Notify: setToast}))),
			Presence(If(toast != "", tela.CP(Toast, ToastProps{Message: toast, Dismiss: setToast, Pinned: p.Pinned}))),
		),
	)
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return Dashboard() }

// Previews are the states the README and the goldens show.
var Previews = tela.Previews{
	"1 Overview": {Root: tela.C(Dashboard)},
	"2 Dialog":   {Root: tela.CP(Page, Props{Range: 1, Dialog: true})},
	"3 Toast": {Root: tela.CP(Page, Props{
		Range: 2, Sort: "due", Toast: "Report queued: Q3 revenue summary", Pinned: true,
	})},
}

func main() {
	tela.Run(tela.App{
		Title:     "Tela - dashboard",
		AppID:     "dev.telaui.dashboard",
		Width:     1440,
		Height:    900,
		Root:      Dashboard,
		RootClass: rootClass,
	})
}
