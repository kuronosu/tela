// Command i18n is a translated screen: an order summary in English,
// Spanish and Arabic that switches language while it runs, and lays
// itself out right to left in Arabic.
//
// The messages live in locales/*.json (ICU MessageFormat: plurals with
// Arabic's six categories, an ordinal, arguments), embedded and loaded at
// startup. Store reads the locale with i18n.UseLocale and sets the
// direction of the whole screen with Dir(i18n.Dir(locale)): under Arabic
// the rows, the progress bar, the accent border (border-s-4), the logical
// spacing (ms-*, ps-*) and the text alignment mirror, and the back arrow
// turns with rtl:rotate-180. Numbers, dates, times and lists come from
// i18n.UseFormatter, so they follow the locale too (Arabic-Indic digits,
// Arabic month names). The buttons at the top call i18n.SetLocale; only
// the components that read the locale build again.
//
// `tela snapshot ./examples/i18n` renders the Arabic preview without a
// window (the previews are "ar", "en" and "es").
package main

import (
	"embed"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
)

//go:embed locales/*.json
var locales embed.FS

func init() {
	if err := i18n.Load(locales, "locales/*.json"); err != nil {
		panic(err)
	}
}

// Languages are the locales of the switcher, in its order.
var Languages = []string{"en", "es", "ar"}

// The order's fixed facts, so that every run shows the same screen.
var (
	updated  = time.Date(2026, time.March, 14, 15, 30, 0, 0, time.UTC)
	delivery = time.Date(2026, time.March, 17, 9, 0, 0, 0, time.UTC)
	subtotal = 1234.5
	discount = 0.15
	steps    = 4
)

// Store is the screen.
func Store() tela.Node {
	f := i18n.UseFormatter() // the locale's messages and formats; rebuilds on SetLocale
	t := f.T
	messages, setMessages := tela.UseState(3)
	step := 3

	return Div(Dir(f.Dir().String()), Class("min-h-screen bg-slate-100 p-6 text-slate-900"),
		Div(Class("mx-auto flex max-w-xl flex-col gap-4"),
			// Header: the avatar at the inline start, the switcher at the end.
			Div(Class("flex items-center gap-3"),
				Div(Class("flex size-11 items-center justify-center rounded-xl bg-indigo-600 text-lg font-bold text-white"), Text("T")),
				Div(Class("flex-1"),
					H1(Class("text-xl font-bold tracking-tight"), Text(t("title"))),
					P(Class("text-sm text-slate-500"), Text(t("greeting", "name", "Ana"))),
				),
				tela.C(Switcher),
			),

			// Messages: a plural with a counter, and a progress bar that
			// fills from the inline start.
			Div(Class("flex flex-col gap-3 rounded-2xl border-s-4 border-indigo-500 bg-white p-5 shadow-sm"),
				Div(Class("flex items-center gap-3"),
					P(Class("flex-1 font-medium"), TestID("messages"), Text(t("messages", "count", messages))),
					Button(Class("rounded-lg bg-slate-100 px-3 py-1 text-sm hover:bg-slate-200"),
						Disabled(messages == 0),
						OnClick(func(tela.MouseEvent) { setMessages.Update(func(n int) int { return max(n-1, 0) }) }),
						Text(t("remove"))),
					Button(Class("rounded-lg bg-indigo-600 px-3 py-1 text-sm text-white hover:bg-indigo-500"),
						OnClick(func(tela.MouseEvent) { setMessages.Update(func(n int) int { return n + 1 }) }),
						Text(t("add"))),
				),
				Div(Class("relative h-2 overflow-hidden rounded-full bg-slate-100"),
					Div(TestID("progress"), Class("absolute inset-y-0 start-0 w-3/4 rounded-full bg-indigo-500"))),
				Div(Class("flex justify-between text-sm text-slate-500"),
					Span(Text(t("step", "step", f.Number(float64(step)), "total", f.Number(float64(steps))))),
					Span(Text(t("rank", "n", 2))),
				),
			),

			// The summary: label at the start, value at the end of each row.
			Div(Class("flex flex-col rounded-2xl bg-white px-5 py-2 shadow-sm"),
				Row(t("items", "count", 4), f.Number(subtotal, i18n.MinFractionDigits(2))),
				Row(t("summary.discount"), f.Number(discount, i18n.Percent())),
				Row(t("summary.delivery"), f.Date(delivery, i18n.Full)),
				Row(t("summary.updated"), f.Time(updated, i18n.Short)),
				Row(t("summary.contents"), f.List([]string{t("fruit.apples"), t("fruit.pears"), t("fruit.figs")})),
			),

			// Footer: Back at the start, its arrow pointing back in either
			// direction, and Checkout pushed to the end.
			Div(Class("flex items-center"),
				Button(TestID("back"), Class("flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-600 hover:bg-white"),
					Div(Class("rtl:rotate-180"), Text("←")),
					Span(Text(t("back")))),
				Button(TestID("checkout"), Class("ms-auto rounded-lg bg-slate-900 px-5 py-2 text-sm font-medium text-white hover:bg-slate-700"),
					Text(t("checkout"))),
			),
		),
	)
}

// Row is a line of the summary.
func Row(label, value string) tela.Node {
	return Div(Class("flex items-baseline gap-4 border-b border-slate-100 py-3 text-sm last:border-b-0"),
		Span(Class("text-slate-500"), Text(label)),
		Span(Class("ms-auto text-end font-medium"), Text(value)),
	)
}

// Switcher picks the language: each button is named in its own language
// and pressed for the current one.
func Switcher() tela.Node {
	current := i18n.UseLocale()
	return Div(Class("flex gap-1 rounded-xl bg-white p-1 shadow-sm"),
		Map(Languages, func(tag string) tela.Node {
			on := current == tag
			return Button(Key(tag), AriaPressed(on),
				Class("rounded-lg px-3 py-1 text-sm text-slate-600 hover:bg-slate-100"),
				If(on, Class("bg-slate-900 text-white hover:bg-slate-900")),
				OnClick(func(tela.MouseEvent) { i18n.SetLocale(tag) }),
				Text(i18n.For(tag).T("language")))
		}),
	)
}

// Previews are the screens of `tela snapshot` and `tela inspect`, one per
// language, each with its locale fixed by i18n.Provider.
var Previews = tela.Previews{
	"ar": func() tela.Node { return i18n.Provider("ar", tela.C(Store)) },
	"en": func() tela.Node { return i18n.Provider("en", tela.C(Store)) },
	"es": func() tela.Node { return i18n.Provider("es", tela.C(Store)) },
}

func main() {
	tela.Main(tela.App{
		Title:  "Tela — i18n",
		AppID:  "dev.telaui.i18n",
		Width:  720,
		Height: 720,
		Root:   Store,
	})
}
