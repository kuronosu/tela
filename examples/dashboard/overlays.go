package main

import (
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Scrim dims the page behind the dialog. It appears and goes at once, with
// no fade: a fade over the whole window repaints the whole page on every
// frame of it, and the panel's own entrance is what the eye follows.
func Scrim(close func()) tela.Node {
	return Div(Class("fixed inset-0 bg-slate-900/30 dark:bg-black/50"), AriaHidden(true),
		OnClick(close))
}

// ReportDialog is the new-report dialog. It renders into an h.Portal (see
// Page), so no overflow or z-index of the page can clip or cover it, and
// its panel enters and leaves with a transition: the starting: classes are
// its first style, the Exit classes its last, and h.Presence keeps the
// dialog mounted until the transition ends.
//
// The shell is a plain element, not a component: Presence looks for the
// Exit classes of the child it holds. The form inside is a component, with
// its own state. The shell lets the pointer through to the scrim around the
// panel (pointer-events-none), which closes the dialog.
type ReportDialog struct {
	Close  tela.Setter[bool]
	Notify tela.Setter[string]
}

func (p ReportDialog) Build() tela.Node {
	return Div(Key("report-dialog"), Class("pointer-events-none fixed inset-0 flex items-center justify-center p-6"),
		OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == "Escape" {
				p.Close.Set(false)
			}
		}),
		Div(Role("dialog"), AriaLabel("New report"),
			Class("pointer-events-auto relative w-full max-w-md rounded-3xl border text-slate-900 dark:text-slate-100 border-white/60 bg-white/65 p-6 shadow-2xl shadow-indigo-950/20 backdrop-blur-xl",
				"transition duration-200 ease-out starting:translate-y-4 starting:scale-95 starting:opacity-0",
				"dark:border-white/10 dark:bg-slate-900/60"),
			Exit("translate-y-4 scale-95 opacity-0"),
			ReportForm(p), // the form takes the same fields
		),
	)
}

var formats = []string{"PDF", "CSV", "XLSX"}

// ReportForm is the content of the dialog.
type ReportForm struct {
	Close  tela.Setter[bool]
	Notify tela.Setter[string]
}

func (p ReportForm) Build() tela.Node {
	name, setName := tela.UseState("Q3 revenue summary")
	format, setFormat := tela.UseState("PDF")
	weekly, setWeekly := tela.UseState(true)
	submit := func() {
		p.Close.Set(false)
		p.Notify.Set("Report queued: " + name)
	}
	return Form(Class("flex flex-col gap-5"), OnSubmit(submit),
		Div(
			H2(Class("text-xl", heading), Text("New report")),
			P(Class("mt-1 text-sm", muted), Text("Pick what to include and how to deliver it.")),
		),
		Div(Class("flex flex-col gap-1.5"),
			Label(For("report-name"), Class("text-sm font-medium"), Text("Name")),
			Input(ID("report-name"), Bind(name, setName), AutoFocus(true), Placeholder("Report name"),
				Class("h-10 rounded-xl border border-white/70 bg-white/60 px-3 text-sm text-slate-900 outline-none transition duration-200",
					"placeholder:text-slate-500 focus:bg-white/90 focus:ring-2 focus:ring-indigo-500/50",
					"dark:border-white/10 dark:bg-white/5 dark:text-white dark:focus:bg-white/10")),
		),
		Div(Class("flex flex-col gap-1.5"),
			Span(Class("text-sm font-medium"), Text("Format")),
			Div(Role("radiogroup"), Class("flex rounded-xl border border-white/60 bg-white/40 p-1 dark:border-white/10 dark:bg-white/5"),
				Map(formats, func(f string) tela.Node {
					color := "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white"
					if f == format {
						color = "text-slate-900 dark:text-white"
					}
					return Button(Key(f), Type("button"), Role("radio"), AriaChecked(f == format),
						Class("relative flex flex-1 items-center justify-center rounded-lg py-1.5 text-sm font-medium transition-colors duration-200", color),
						OnClick(func() { setFormat.Set(f) }),
						If(f == format, Div(Key("pill"), LayoutID("format-pill"),
							Class("absolute inset-0 rounded-lg bg-white shadow-sm dark:bg-white/15"))),
						Div(Class("relative"), Text(f)),
					)
				}),
			),
		),
		Button(Type("button"), Role("switch"), AriaChecked(weekly), AriaLabel("Send every week"),
			Class("flex items-center justify-between rounded-xl px-1 py-1 text-left"),
			OnClick(tela.Toggle(setWeekly)),
			Span(Class("text-sm font-medium"), Text("Send every Monday")),
			toggle(weekly),
		),
		Div(Class("flex justify-end gap-3 pt-1"),
			Button(Type("button"), Class("flex h-10 items-center rounded-xl border border-white/60 bg-white/50 px-4 text-sm font-medium transition duration-200 hover:bg-white/80 active:scale-95 dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/10"),
				OnClick(func() { p.Close.Set(false) }), Text("Cancel")),
			Button(Type("submit"), Class("flex h-10 items-center rounded-xl bg-linear-to-r from-indigo-500 to-fuchsia-500 px-5 text-sm font-semibold text-white shadow-lg shadow-fuchsia-500/30 transition duration-200 hover:-translate-y-0.5 active:scale-95"),
				Text("Create report")),
		),
	)
}

// toggle is the switch drawn inside a button: the knob slides with a
// transition.
func toggle(on bool) tela.Node {
	track, knob := "bg-slate-300 dark:bg-slate-600", "translate-x-0"
	if on {
		track, knob = "bg-linear-to-r from-indigo-500 to-fuchsia-500", "translate-x-5"
	}
	return Div(Class("relative h-6 w-11 shrink-0 rounded-full shadow-inner transition-colors duration-200", track),
		Div(Class("absolute top-0.5 left-0.5 size-5 rounded-full bg-white shadow transition duration-200 ease-out", knob)))
}

// Toast slides up when it mounts and runs its own exit: UsePresenceTween
// tweens it in and, when its Presence child is leaving, out, and tells
// Presence when it arrives. A second tween, on the frame clock too, empties
// its countdown bar in 4 s and dismisses it (OnDone). Nothing builds while
// either tween runs: the values are read by paint closures.
type Toast struct {
	Message string
	Dismiss tela.Setter[string]
	// Pinned keeps the toast until it is dismissed by hand: no countdown.
	Pinned bool
}

func (p Toast) Build() tela.Node {
	shown := tela.UsePresenceTween(tela.Duration(250*time.Millisecond), tela.Ease(tela.EaseOut))
	countdown := float32(0)
	if p.Pinned {
		countdown = 1
	}
	left := tela.UseTween(countdown, tela.Initial(float32(1)), tela.Duration(4*time.Second), tela.Ease(tela.EaseLinear),
		tela.OnDone(func() {
			if !p.Pinned {
				p.Dismiss.Set("")
			}
		}))
	return Div(Class("fixed bottom-8 left-1/2 flex w-0 justify-center"),
		Div(Role("status"), Class("relative flex w-[360px] shrink-0 items-center gap-3 overflow-hidden rounded-2xl border text-slate-900 dark:text-slate-100 border-white/60 bg-white/70 py-3 pr-3 pl-4 text-sm font-medium",
			"whitespace-nowrap shadow-2xl shadow-indigo-950/20 backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/70"),
			Style(
				tela.TranslateFn(func() (float32, float32) { return 0, 24 * (1 - shown.Get()) }),
				tela.OpacityFn(shown.Get),
			),
			Div(Class("flex size-6 shrink-0 items-center justify-center rounded-full bg-emerald-500 text-white"), icon("check")),
			Span(Class("min-w-0 flex-1 truncate"), Text(p.Message)),
			Button(AriaLabel("Dismiss"), Class("flex size-7 shrink-0 items-center justify-center rounded-lg", muted, "transition-colors duration-200 hover:bg-black/5 dark:hover:bg-white/10"),
				OnClick(func() { p.Dismiss.Set("") }), icon("close")),
			Div(Class("absolute inset-x-0 bottom-0 h-0.5 bg-linear-to-r from-indigo-500 to-fuchsia-500"),
				Style(tela.WidthFn(Scale(left, 360)))),
		),
	)
}
