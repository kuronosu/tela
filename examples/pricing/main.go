// Command pricing is a pricing page built on package kit in its default
// style: three plans as cards with a highlighted one, feature lists with
// check icons and a monthly/yearly segmented control that changes the
// prices.
//
// The segmented control is a kit.ToggleGroup whose value is the billing
// period. The plan cards are struct components whose fields are an ID and
// the billing period: plain comparable values, so a rebuild of the page
// that changes neither skips them. Colors are theme tokens (bg-primary,
// text-muted-foreground), so the page follows the color scheme with no
// dark: classes except in two accents.
//
// `tela snapshot ./examples/pricing` renders it without a window; add
// --dark for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Pricing is the page. The billing period lives here and goes down as a
// bool plus its Setter.
func Pricing() tela.Node { return tela.C(pricing) }

func pricing() tela.Node {
	yearly, setYearly := tela.UseState(false)
	return Div(Class("flex min-h-screen flex-col items-center gap-8 overflow-hidden px-6 pt-12"),
		Div(Class("flex flex-col items-center gap-3 text-center"),
			kit.Badge{Variant: kit.Outline, Class: "px-3 py-1", Children: Text("Pricing")},
			H1(Class("text-4xl font-bold tracking-tight md:text-5xl"), Text("Simple, transparent pricing")),
			P(Class("text-base text-muted-foreground"), Text("Start free. Upgrade when your team grows. Cancel anytime.")),
		),
		BillingToggle{Yearly: yearly, Set: setYearly},
		Div(Class("flex flex-wrap items-stretch justify-center gap-6"),
			MapKeyed(plans, func(p Plan) string { return p.ID }, func(p Plan) tela.Component {
				return PlanCard{ID: p.ID, Yearly: yearly}
			}),
		),
	)
}

var theme = kit.Theme()

// App is the window; `tela snapshot` and the tests read it too.
var App = tela.App{
	Title:     "Tela — pricing",
	AppID:     "dev.telaui.pricing",
	Width:     1280,
	Height:    800,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Pricing(),
}

func main() { tela.Main(App) }
