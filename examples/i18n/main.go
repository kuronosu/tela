// Command i18n is a translated screen built on package kit: an order
// summary in English, Spanish and Arabic that switches language while it
// runs, and lays itself out right to left in Arabic.
//
// The messages live in locales/*.json (ICU MessageFormat: plurals with
// Arabic's six categories, an ordinal, arguments), embedded and loaded at
// startup. The screen reads the locale with i18n.UseFormatter and sets the
// direction of the whole screen with Dir(f.Dir().String()); the kit
// components below it follow: the language Select, the cards, the Progress
// bar, the buttons and the DatePicker, whose calendar shows the locale's
// month names, weekday order and digits, mirrored under Arabic. The text of
// the kit's own labels that default to English (the DatePicker's "Pick a
// date", the calendar's month buttons) is set from the catalog. Numbers,
// dates, times and lists come from i18n.UseFormatter, so they follow the
// locale too. The Select calls i18n.SetLocale; only the components that
// read the locale build again.
//
// The look is the kit's default style and its theme tokens (bg-background,
// text-muted-foreground), so there is no palette and no dark: class.
//
// `tela snapshot ./examples/i18n` renders the first preview without a
// window (the previews are "ar", "en" and "es").
package main

import (
	"embed"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/i18n/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
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
	subtotal = 1234.5
	discount = 0.15
	steps    = 4
)

// today is the calendar's clock: the day of the order's last update, so the
// calendar shows the same month on every run.
var today = aria.DateOf(updated)

// Store is the screen.
func Store() tela.Node { return tela.C(store) }

func store() tela.Node {
	f := i18n.UseFormatter()                  // the locale's messages and formats; rebuilds on SetLocale
	day, setDay := tela.UseState(aria.Date{}) // none: the picker shows its placeholder

	return Div(Dir(f.Dir().String()), Class("min-h-screen p-6"),
		Div(Class("mx-auto flex max-w-xl flex-col gap-4"),
			Header(),
			MessagesCard{},
			SummaryCard{Delivery: day},
			DeliveryCard{Day: day, Set: setDay},
			Footer(),
		),
	)
}

// Header is the avatar, the title and the language Select at the end.
func Header() tela.Node { return tela.C(header) }

func header() tela.Node {
	t := i18n.UseFormatter().T
	return Div(Class("flex items-center gap-3"),
		kit.Avatar{Size: kit.Lg, Fallback: "T"},
		Div(Class("flex-1"),
			H1(Class("text-xl font-bold tracking-tight"), Text(t("title"))),
			P(Class("text-sm text-muted-foreground"), Text(t("greeting", "name", "Ana"))),
		),
		Switcher(),
	)
}

// MessagesCard counts messages with a plural and shows the order's progress
// on a bar that fills from the inline start.
type MessagesCard struct{}

func (MessagesCard) Build() tela.Node {
	f := i18n.UseFormatter()
	t := f.T
	messages, setMessages := tela.UseState(3)
	step := 3
	stepText := t("step", "step", f.Number(float64(step)), "total", f.Number(float64(steps)))

	return Fragment(kit.Card{
		Title: Span(TestID("messages"), Text(t("messages", "count", messages))),
		Action: Div(Class("flex gap-2"),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Icon: icons.Minus, Disabled: messages == 0,
				OnClick:  func() { setMessages.Update(func(n int) int { return max(n-1, 0) }) },
				Children: Text(t("remove"))},
			kit.Button{Size: kit.Sm, Icon: icons.Plus,
				OnClick:  func() { setMessages.Update(func(n int) int { return n + 1 }) },
				Children: Text(t("add"))},
		),
		Children: Div(Class("flex flex-col gap-3"),
			kit.Progress{Value: float64(step) / float64(steps) * 100, AriaLabel: stepText, Props: TestID("progress")},
			Div(Class("flex justify-between text-sm text-muted-foreground"),
				Span(Text(stepText)),
				Span(Text(t("rank", "n", 2))),
			),
		),
	})
}

// SummaryCard is the order's facts: label at the start, value at the end of
// each row. The delivery row shows the date chosen in DeliveryCard, once
// there is one.
type SummaryCard struct {
	Delivery aria.Date
}

func (c SummaryCard) Build() tela.Node {
	f := i18n.UseFormatter()
	t := f.T
	delivery := t("summary.pending")
	if !c.Delivery.IsZero() {
		delivery = f.Date(c.Delivery.Time(nil), i18n.Full)
	}
	rows := []Row{
		{t("items", "count", 4), f.Number(subtotal, i18n.MinFractionDigits(2))},
		{t("summary.discount"), f.Number(discount, i18n.Percent())},
		{t("summary.delivery"), delivery},
		{t("summary.updated"), f.Time(updated, i18n.Short)},
		{t("summary.contents"), f.List([]string{t("fruit.apples"), t("fruit.pears"), t("fruit.figs")})},
	}
	var body []tela.Child
	for i, r := range rows {
		if i > 0 {
			body = append(body, kit.Separator{})
		}
		body = append(body, r)
	}
	return Fragment(kit.Card{
		Title:    H2(Text(t("summaryTitle"))),
		Children: Div(append([]tela.Child{Class("flex flex-col gap-3")}, body...)...),
	})
}

// Row is a line of the summary.
type Row struct {
	Label, Value string
}

func (r Row) Build() tela.Node {
	return Div(Class("flex items-baseline gap-4 text-sm"),
		Span(Class("text-muted-foreground"), Text(r.Label)),
		Span(Class("ms-auto text-end font-medium"), Text(r.Value)),
	)
}

// DeliveryCard picks the delivery date with a kit.DatePicker. Its calendar
// follows the locale (month and weekday names, first weekday, digits,
// direction); the labels that default to English come from the catalog.
type DeliveryCard struct {
	Day aria.Date
	Set tela.Setter[aria.Date]
}

func (c DeliveryCard) Build() tela.Node {
	f := i18n.UseFormatter()
	t := f.T
	return Fragment(kit.Card{
		Children: kit.Field{
			Label:       t("deliveryDate"),
			Description: t("deliveryHint"),
			Children: kit.DatePicker{
				Value: &c.Day, OnValueChange: c.Set.Set,
				Placeholder: t("pickDate"), PreviousLabel: t("previousMonth"), NextLabel: t("nextMonth"),
				Min: today, Today: today,
				// Arabic's short weekday names are as long as words and do
				// not fit the 32 px cells: use its one-letter ones.
				NarrowWeekdays:    f.Locale() == "ar",
				IsDateUnavailable: func(d aria.Date) bool { return d.Weekday() == time.Sunday },
				Class:             "w-full", Props: TestID("delivery"),
			},
		},
	})
}

// Footer has Back at the start, its arrow pointing back in either
// direction, and Checkout pushed to the end.
func Footer() tela.Node { return tela.C(footer) }

func footer() tela.Node {
	f := i18n.UseFormatter()
	back := icons.ArrowLeft
	if f.Dir() == i18n.RTL {
		back = icons.ArrowRight
	}
	return Div(Class("flex items-center"),
		kit.Button{Variant: kit.Ghost, Icon: back, Props: TestID("back"), Children: Text(f.T("back"))},
		kit.Button{Icon: icons.ShoppingCart, Class: "ms-auto", Props: TestID("checkout"), Children: Text(f.T("checkout"))},
	)
}

// Switcher picks the language: each option is named in its own language
// and the trigger shows the current one.
func Switcher() tela.Node { return tela.C(switcher) }

func switcher() tela.Node {
	current := i18n.UseLocale()
	items := make([]kit.SelectItem[string], len(Languages))
	for i, tag := range Languages {
		items[i] = kit.SelectItem[string]{Value: tag, Label: i18n.For(tag).T("language")}
	}
	return Fragment(kit.Select[string]{
		Items: items, Value: &current, OnValueChange: i18n.SetLocale,
		AriaLabel: i18n.For(current).T("languageLabel"), Class: "w-36", Props: TestID("language"),
	})
}

// Previews are the screens of `tela snapshot` and `tela inspect`, one per
// language, each with its locale fixed by i18n.Provider.
var Previews = tela.Previews{
	"ar": {Root: Store(), Wrap: inLocale("ar")},
	"en": {Root: Store(), Wrap: inLocale("en")},
	"es": {Root: Store(), Wrap: inLocale("es")},
}

// inLocale wraps a preview in the provider that fixes its locale.
func inLocale(tag string) func(tela.Node) tela.Node {
	return func(n tela.Node) tela.Node { return i18n.Provider(tag, n) }
}

var theme = kit.Theme()

// App is the window; `tela snapshot`, `tela inspect` and the tests read it
// too.
var App = tela.App{
	Title:     "Tela — i18n",
	AppID:     "dev.telaui.i18n",
	Width:     720,
	Height:    800,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Store(),
}

func main() { tela.Main(App) }
