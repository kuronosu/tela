package h_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Frames of the layout animations (AnimateLayout, LayoutID), captured with
// telatest.NoSettle: the clock moves only with Advance, so each golden is
// the picture at a known time after the change. The numbers (where every
// box is, at which frame) are checked in internal/core; these goldens check
// what the FLIP transforms look like: rows sliding on a spring, a reversal
// that starts from where the rows are drawn, an underline that slides and
// stretches between tabs, and a card that scales toward its new size with
// its corners and shadow corrected while its children keep their own size.
// Regenerate them with TELA_UPDATE=1 and look at them.

type frameTask struct{ ID, Title, Dot string }

var frameTasks = []frameTask{
	{"design", "Review the onboarding flow", "bg-fuchsia-500"},
	{"api", "Ship the billing API", "bg-sky-500"},
	{"bug", "Fix the flaky export test", "bg-rose-500"},
	{"docs", "Write the migration guide", "bg-amber-500"},
}

// reorderPage is a list of keyed rows with AnimateLayout; Rotate moves the
// last one to the top and Reverse reverses the list.
func reorderPage() tela.Node {
	order, setOrder := tela.UseState(frameTasks)
	rotate := func() {
		n := len(order)
		setOrder.Set(append([]frameTask{order[n-1]}, order[:n-1]...))
	}
	reverse := func() {
		next := make([]frameTask, len(order))
		for i, t := range order {
			next[len(order)-1-i] = t
		}
		setOrder.Set(next)
	}
	return Div(Class("flex h-full flex-col gap-3 bg-slate-100 p-4"),
		Div(Class("flex gap-2"),
			Button(Class("rounded-md bg-slate-900 px-3 py-1.5 text-xs font-medium text-white"), OnClick(rotate), Text("Rotate")),
			Button(Class("rounded-md bg-white px-3 py-1.5 text-xs font-medium text-slate-700 ring-1 ring-slate-200"), OnClick(reverse), Text("Reverse"))),
		MapKeyed(order, func(t frameTask) string { return t.ID }, func(t frameTask) tela.Component {
			return frameRow{t}
		}),
	)
}

type frameRow struct{ Task frameTask }

func (r frameRow) Build() tela.Node {
	return Div(AnimateLayout(), Class("flex items-center gap-3 rounded-xl bg-white px-3 py-3 shadow-sm ring-1 ring-slate-200"),
		Div(Class("size-2.5 shrink-0 rounded-full", r.Task.Dot)),
		Div(Class("flex-1 text-sm text-slate-700"), Text(r.Task.Title)))
}

func TestLayoutFramesReorder(t *testing.T) {
	app := telatest.Mount(t, tela.C(reorderPage), telatest.Size(320, 260), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Rotate"))
	app.Advance(50 * time.Millisecond)
	app.Golden("layoutanim_rotate_50ms.png")
	app.Advance(50 * time.Millisecond)
	app.Golden("layoutanim_rotate_100ms.png")
	// The reversal interrupts the rotation: the rows start from where they
	// are drawn, at their speed.
	app.Click(telatest.ByRole("button", "Reverse"))
	app.Advance(80 * time.Millisecond)
	app.Golden("layoutanim_reverse_80ms.png")
	app.Advance(time.Second) // the rows come to rest
	app.ExpectText("Ship the billing API")
	app.Golden("layoutanim_reversed.png")
}

// underlineTabs renders the underline in the selected tab, with the same
// LayoutID in all of them: the new underline starts from the old one's box.
func underlineTabs() tela.Node {
	active, setActive := tela.UseState(0)
	labels := []string{"Overview", "Activity", "Settings"}
	return Div(Class("h-full bg-white p-4"),
		Div(Class("flex gap-1 border-b border-slate-200"), MapIndex(labels, func(i int, label string) tela.Node {
			color := "text-slate-500"
			if i == active {
				color = "text-indigo-600"
			}
			return Button(Key(label), Class("relative px-3 pb-3 pt-1 text-sm font-medium", color),
				OnClick(func() { setActive.Set(i) }),
				Text(label),
				If(i == active, Div(Key("underline"), LayoutID("tab-underline"),
					Class("absolute inset-x-1 -bottom-px h-0.5 rounded-full bg-indigo-500"))))
		})),
		P(Class("pt-4 text-sm text-slate-600"), Textf("Tab %d of %d", active+1, len(labels))),
	)
}

func TestLayoutFramesTabUnderline(t *testing.T) {
	app := telatest.Mount(t, tela.C(underlineTabs), telatest.Size(320, 110), telatest.NoSettle())
	underline := telatest.ByClass("bg-indigo-500")
	app.Click(telatest.ByRole("button", "Settings"))
	app.Advance(40 * time.Millisecond)
	app.Golden("layoutanim_tabs_40ms.png")
	app.Advance(50 * time.Millisecond)
	app.Golden("layoutanim_tabs_90ms.png")
	app.Advance(time.Second) // the underline comes to rest
	app.ExpectText("Tab 3 of 3")
	app.Golden("layoutanim_tabs_settings.png")
	if x, w := app.Bounds(underline).X, app.Bounds(telatest.ByRole("button", "Settings")).X; x < w {
		t.Errorf("the underline rests at x=%v, left of the Settings tab at %v", x, w)
	}
}

// expandPage is a gradient card with AnimateLayout that grows when its
// details open; its header and details have AnimateLayout too, so they keep
// their own size instead of stretching, and the note below follows its
// bottom edge.
func expandPage() tela.Node {
	open, setOpen := tela.UseState(false)
	label := "Show details"
	if open {
		label = "Hide details"
	}
	return Div(Class("flex h-full flex-col gap-4 bg-slate-100 p-4"),
		Div(AnimateLayout(), Class("overflow-hidden rounded-2xl bg-linear-to-br from-indigo-500 to-violet-600 p-5 text-white shadow-lg"),
			Div(AnimateLayout(), Class("flex items-center justify-between"),
				Div(
					H2(Class("text-base font-semibold"), Text("Quarterly report")),
					P(Class("text-sm text-indigo-100"), Text("Revenue grew 18%."))),
				Button(Class("rounded-lg bg-white/15 px-3 py-1.5 text-xs font-medium"), OnClick(tela.Toggle(setOpen)), Text(label))),
			If(open, Div(Key("details"), AnimateLayout(),
				Class("mt-4 grid grid-cols-3 gap-3 transition-opacity duration-300 starting:opacity-0"),
				frameStat{"Revenue", "$1.2M"}, frameStat{"Customers", "4,210"}, frameStat{"Churn", "1.8%"})),
		),
		P(AnimateLayout(), Class("px-1 text-xs text-slate-400"), Text("The note follows the card's bottom edge.")),
	)
}

type frameStat struct{ Label, Value string }

func (s frameStat) Build() tela.Node {
	return Div(Class("rounded-xl bg-white/10 p-3"),
		P(Class("text-xs text-indigo-100"), Text(s.Label)),
		P(Class("text-lg font-semibold"), Text(s.Value)))
}

func TestLayoutFramesExpand(t *testing.T) {
	app := telatest.Mount(t, tela.C(expandPage), telatest.Size(360, 280), telatest.NoSettle())
	note := telatest.ByText("The note follows")
	app.Click(telatest.ByRole("button", "Show details"))
	app.Advance(30 * time.Millisecond)
	app.Golden("layoutanim_expand_30ms.png")
	app.Advance(50 * time.Millisecond)
	app.Golden("layoutanim_expand_80ms.png")
	app.Advance(time.Second) // the card comes to rest
	app.ExpectText("Hide details")
	app.Golden("layoutanim_expanded.png")
	below := app.Bounds(note).Y
	app.Click(telatest.ByRole("button", "Hide details"))
	app.Advance(40 * time.Millisecond)
	app.Golden("layoutanim_collapse_40ms.png")
	app.Advance(time.Second)
	if y := app.Bounds(note).Y; y >= below {
		t.Errorf("the note is at y=%v after the card collapsed, it was at %v while open", y, below)
	}
}
