package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

var App = tela.App{
	Title: "Tip", Width: 480, Height: 380,
	Theme: &theme, RootClass: "bg-muted text-foreground",
	Root: Div(Class("flex h-full items-center justify-center"), tela.C(tip)),
}

func tip() tela.Node {
	pct, setPct := tela.UseState(18.0)
	total := tela.UseMemo(func() float64 { return 84.50 * (1 + pct/100) }, pct)

	return kit.Card{
		Class:       "w-96",
		Title:       Text("Tip calculator"),
		Description: Textf("%.0f%% on a $84.50 bill", pct),
		Children: Div(Class("flex flex-col gap-5"),
			kit.Slider{Value: &pct, OnValueChange: setPct.Set, Min: 0, Max: 30, Step: 1, AriaLabel: "Tip"},
			Div(Class("flex items-baseline justify-between"),
				Span(Class("text-sm text-muted-foreground"), Text("Total")),
				Span(Class("text-3xl font-semibold tabular-nums"), Textf("$%.2f", total))),
		),
	}
}

func main() { tela.Main(App) }
