// Command pricing is a pricing page: three plans with a highlighted one,
// feature lists with check marks and a monthly/yearly switch that changes
// the prices.
//
// The switch is a component built from divs: a track and a knob that move
// with justify-start/justify-end when the state changes. The plan cards
// are struct components whose fields are an ID and the billing period:
// plain comparable values, so a rebuild of the page that changes neither
// skips them.
//
// `tela snapshot ./examples/pricing` renders it without a window; add
// --dark for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Pricing is the page. The billing period lives here and goes down as a
// bool plus its Setter.
func Pricing() tela.Node {
	yearly, setYearly := tela.UseState(false)
	return Div(Class("flex min-h-screen flex-col items-center gap-8 overflow-hidden bg-zinc-50 px-6 pt-12 text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50"),
		Div(Class("flex flex-col items-center gap-3 text-center"),
			Div(Class("rounded-full border border-zinc-200 bg-white px-3 py-1 text-xs font-medium text-zinc-600 shadow-xs dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-400"),
				Text("Pricing")),
			H1(Class("text-4xl font-bold tracking-tight md:text-5xl"), Text("Simple, transparent pricing")),
			P(Class("text-base text-zinc-500 dark:text-zinc-400"), Text("Start free. Upgrade when your team grows. Cancel anytime.")),
		),
		BillingToggle{Yearly: yearly, Set: setYearly},
		Div(Class("flex flex-wrap items-stretch justify-center gap-6"),
			MapKeyed(plans, func(p Plan) string { return p.ID }, func(p Plan) tela.Component {
				return PlanCard{ID: p.ID, Yearly: yearly}
			}),
		),
	)
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return tela.C(Pricing) }

func main() {
	tela.Main(tela.App{
		Title:  "Tela — pricing",
		AppID:  "dev.telaui.pricing",
		Width:  1280,
		Height: 800,
		Root:   Pricing,
	})
}
