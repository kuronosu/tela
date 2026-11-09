package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The Internationalization page: what the catalogs in locales/ and package
// i18n give a window, in the language the header's Language menu chose. Each
// row shows the key or the call and what it returns right now, so switching
// the language (or counting) changes the right column only.

// The facts the formats are shown with, fixed so that every run shows the
// same page.
var (
	updated  = time.Date(2026, time.March, 14, 15, 30, 0, 0, time.UTC)
	subtotal = 1234.5
	discount = 0.15
	steps    = 4
)

// counts are the presets of the counter: the first value of each of Arabic's
// six plural categories (zero, one, two, few, many, other).
var counts = []int{0, 1, 2, 3, 11, 100}

// kv is a row: the key or call in code, and its value in the language. The
// value has the test id id when it is not empty.
func kv(key, value string, id ...string) tela.Node {
	props := []tela.Child{Class("ms-auto text-end font-medium"), Text(value)}
	if len(id) > 0 {
		props = append(props, TestID(id[0]))
	}
	return Div(Class("flex items-baseline gap-4 text-sm"),
		Span(Dir("ltr"), Class("font-mono text-xs text-muted-foreground"), Text(key)),
		Span(props...))
}

// rows joins rows with separators.
func rows(rs ...tela.Node) tela.Node {
	var kids []tela.Child
	for i, r := range rs {
		if i > 0 {
			kids = append(kids, kit.Separator{})
		}
		kids = append(kids, r)
	}
	return Div(append([]tela.Child{Class("flex flex-col gap-3")}, kids...)...)
}

// noAnswer is the state of the message box demo before it has been answered.
const noAnswer = desktop.Answer(255)

// digits is a row of the numbering systems demo: the tag, a number and a
// date as that locale writes them. It does not follow the window's language.
func digits(tag string) tela.Node {
	f := i18n.For(tag)
	return Div(Class("flex items-baseline gap-4 text-sm"),
		Span(Dir("ltr"), Class("w-36 shrink-0 font-mono text-xs text-muted-foreground"), Text(tag)),
		Span(Class("ms-auto"),
			Span(Dir(f.Dir().String()), Class("block font-medium tabular-nums"), Text(f.Number(subtotal, i18n.MinFractionDigits(2))+"  ·  "+f.Date(updated, i18n.Medium)))))
}

func internationalizationPage() tela.Node {
	f := i18n.UseFormatter() // the locale's messages and formats; rebuilds when the language changes
	t := f.T
	k := kitTextFor(f)
	count, setCount := tela.UseState(3)
	day, setDay := tela.UseState(aria.Date{})
	answer, setAnswer := tela.UseState(noAnswer)

	presets := make([]kit.ToggleGroupItem[int], len(counts))
	for i, n := range counts {
		presets[i] = kit.ToggleGroupItem[int]{Value: n, Label: f.Number(float64(n))}
	}

	chosen := ""
	if !day.IsZero() {
		chosen = f.Date(day.Time(nil), i18n.Full)
	}

	// The message box is outside the tree of the page, so the texts it
	// draws itself (its buttons and its default title) follow the app's
	// locale (i18n.SetLocale), not the page's Provider: the demo sets it to
	// the window's language while the box is open and puts the previous
	// one back when it is answered.
	showMessage := func() {
		prev := i18n.Locale()
		i18n.SetLocale(f.Locale())
		err := desktop.ShowMessage(desktop.Message{
			Kind: desktop.MessageQuestion, Text: t("builtin.text"), Buttons: desktop.ButtonsYesNoCancel,
		}, func(a desktop.Answer) {
			i18n.SetLocale(prev)
			setAnswer.Set(a)
		})
		if err != nil {
			i18n.SetLocale(prev)
		}
	}
	answerText := ""
	if answer != noAnswer {
		answerText = t("builtin.answer", "answer", f.Label("tela.messagebox."+answer.String()))
	}

	return grid(
		demo("Messages", "Arguments in {braces}, numbers formatted for the language, and nested keys written with a dot: t(\"summary.updated\").", col,
			rows(
				kv(`t("greeting", "name", "Ana")`, t("greeting", "name", "Ana")),
				kv(`t("step", "step", 3, "total", 4)`, t("step", "step", f.Number(3), "total", f.Number(float64(steps)))),
				kv(`t("summary.discount")`, t("summary.discount")),
				kv(`t("summary.updated")`, t("summary.updated")),
			)),
		demo("Plurals", "The language's own rules choose the form: English has one and other, Arabic zero, one, two, few, many and other. Count to see them change.", col,
			Div(Class("flex flex-wrap items-center gap-3"),
				kit.Button{Variant: kit.Outline, Size: kit.IconSm, Icon: icons.Minus, AriaLabel: t("remove"), Disabled: count == 0,
					OnClick: func() { setCount.Update(func(n int) int { return max(n-1, 0) }) }},
				Span(Class("min-w-10 text-center text-lg font-semibold tabular-nums"), TestID("count"), Text(f.Number(float64(count)))),
				kit.Button{Variant: kit.Outline, Size: kit.IconSm, Icon: icons.Plus, AriaLabel: t("add"),
					OnClick: func() { setCount.Update(func(n int) int { return n + 1 }) }},
				kit.ToggleGroup[int]{AriaLabel: "Count", Variant: kit.Outline, Size: kit.Sm, DisallowEmptySelection: true,
					Value: &count, OnValueChange: setCount.Set, Items: presets},
			),
			rows(
				kv(`plural "messages"`, t("messages", "count", count), "plural-messages"),
				kv(`plural "items"`, t("items", "count", count), "plural-items"),
				kv(`selectordinal "rank"`, t("rank", "n", count), "ordinal"),
				kv("the plural category", t("category", "count", count), "category"),
			)),
		demo("Formatting", "i18n.UseFormatter gives numbers, percentages, dates, times and lists the way the language writes them, digits included.", col,
			rows(
				kv("f.Number(1234.5, MinFractionDigits(2))", f.Number(subtotal, i18n.MinFractionDigits(2))),
				kv("f.Number(0.15, Percent())", f.Number(discount, i18n.Percent())),
				kv("f.Date(t, Full)", f.Date(updated, i18n.Full)),
				kv("f.Time(t, Short)", f.Time(updated, i18n.Short)),
				kv("f.List(items)", f.List([]string{t("fruit.apples"), t("fruit.pears"), t("fruit.figs")})),
			)),
		demo("kit follows the language", "A DatePicker writes the date, names the months and weekdays, starts the week and lays the calendar out the way the language does. What kit defaults to English comes from the catalog (kit.* keys); Arabic's short weekday names do not fit a cell, so it asks for the narrow ones.", col,
			kit.Field{Label: t("deliveryDate"), Description: t("deliveryHint"), Class: "max-w-xs",
				Children: kit.DatePicker{
					Value: &day, OnValueChange: setDay.Set, Today: today, Min: today, Class: "w-full", Props: TestID("delivery"),
					Placeholder: k.PickDate, PreviousLabel: k.Previous, NextLabel: k.Next,
					NarrowWeekdays:    k.Narrow,
					IsDateUnavailable: func(d aria.Date) bool { return d.Weekday() == time.Sunday },
				}},
			If(!day.IsZero(), readout(chosen)),
		),
		demo("Tela's own texts", "The texts Tela draws itself have an ID and an English default (tela.Labels lists them): a catalog message with that key translates it, so the title bar, the message boxes, the toasts and the role items of a menu follow the language. Tela ships no translations; an app adds the messages, as locales/*.json does here.", col,
			rows(
				kv(`f.Label("tela.toast.region")`, f.Label("tela.toast.region")),
				kv(`f.Label("tela.toast.close")`, f.Label("tela.toast.close")),
				kv(`f.Label("tela.titlebar.close")`, f.Label("tela.titlebar.close")),
				kv(`f.Label("tela.menu.copy")`, f.Label("tela.menu.copy")),
				kv(`f.Label("tela.menu.quit", "name", "Notes")`, f.Label("tela.menu.quit", "name", "Notes")),
				kv(`f.Label("tela.messagebox.question")`, f.Label("tela.messagebox.question")),
			),
			Div(Class("flex flex-wrap items-center gap-3"),
				kit.Button{Variant: kit.Outline, Icon: icons.MessageSquare, Children: Text(t("builtin.show")), OnClick: showMessage},
				If(answerText != "", readout(answerText)))),
		demo("Digits of a locale", "Numbers and dates use the numbering system CLDR gives the locale by region, script and language: ar-EG writes Arabic-Indic digits, ar-AE and ar-MA Latin ones. A -u-nu- extension in the tag wins.", col,
			rows(digits("ar"), digits("ar-EG"), digits("ar-EG-u-nu-latn"), digits("ar-AE"), digits("es"))),
	)
}
