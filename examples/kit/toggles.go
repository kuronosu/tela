package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The page of ToggleGroup. Its stages are written with tr: English here,
// Spanish and Arabic in the catalogs.

// RangePills is a segmented control with a pill that slides from segment to
// segment: the choice is a tela.Value that the group reads through ValueFn,
// so a press rebuilds the group and not this component, and the Indicator's
// LayoutID makes the pill move.
type RangePills struct {
	Label                  string
	Day, Week, Month, Year string
	Readout                func(choice string) string
}

func (r RangePills) Build() tela.Node {
	choice := tela.UseValue("week")
	return Div(Class("flex flex-col gap-3"),
		kit.ToggleGroup[string]{
			AriaLabel: r.Label, DisallowEmptySelection: true, Spacing: 1,
			ValueFn: choice.Get, OnValueChange: choice.Set,
			Class:     "rounded-lg bg-muted p-1",
			ItemClass: "rounded-md text-muted-foreground hover:bg-transparent aria-checked:bg-transparent aria-checked:text-foreground",
			Indicator: Div(Class("absolute inset-0 -z-10 rounded-md bg-background shadow-sm"), LayoutID("range-pill")),
			Items: []kit.ToggleGroupItem[string]{
				{Value: "day", Label: r.Day}, {Value: "week", Label: r.Week}, {Value: "month", Label: r.Month}, {Value: "year", Label: r.Year},
			}},
		P(Class("text-sm text-muted-foreground tabular-nums"), TextFn(func() string { return r.Readout(choice.Get()) })))
}

func toggleGroupPage() tela.Node {
	tr := useTr()
	view, setView := tela.UseState("list")
	align, setAlign := tela.UseState("start")
	return grid(
		demo("Segmented control", "One choice, always one on (DisallowEmptySelection). The group is one tab stop; the arrows move through it.", col,
			kit.ToggleGroup[string]{AriaLabel: tr("tg.view", "View"), Variant: kit.Outline, DisallowEmptySelection: true, Value: &view, OnValueChange: setView.Set,
				Items: []kit.ToggleGroupItem[string]{
					{Value: "list", Label: tr("tg.list", "List"), Icon: icons.List},
					{Value: "grid", Label: tr("tg.grid", "Grid"), Icon: icons.LayoutGrid},
				}},
			readout(tr("tg.viewIs", "View: {view}", "view", map[string]string{"list": tr("tg.list", "List"), "grid": tr("tg.grid", "Grid")}[view]))),
		demo("A sliding pill", "ValueFn (the Get of a tela.Value) and Indicator: a pill drawn in the item that is on, with a LayoutID, slides to the next one. A press rebuilds the group only, not the page.", col,
			RangePills{Label: tr("tg.range", "Range"), Day: tr("tg.day", "Day"), Week: tr("tg.week", "Week"), Month: tr("tg.month", "Month"), Year: tr("tg.year", "Year"),
				Readout: func(c string) string {
					return tr("tg.showing", "Showing the last {range}.", "range", map[string]string{
						"day": tr("tg.lastDay", "day"), "week": tr("tg.lastWeek", "week"), "month": tr("tg.lastMonth", "month"), "year": tr("tg.lastYear", "year")}[c])
				}}),
		demo("Multiple", "Any number on: the aria.Selection of the group is Selected, DefaultSelected and OnSelectionChange.", "",
			kit.ToggleGroup[string]{AriaLabel: tr("tg.formatting", "Text formatting"), Multiple: true, Variant: kit.Outline, DefaultSelected: aria.SelectionOf("bold", "italic"),
				Items: []kit.ToggleGroupItem[string]{
					{Value: "bold", Icon: icons.Bold, AriaLabel: tr("tg.bold", "Toggle bold")},
					{Value: "italic", Icon: icons.Italic, AriaLabel: tr("tg.italic", "Toggle italic")},
					{Value: "underline", Icon: icons.Underline, AriaLabel: tr("tg.underline", "Toggle underline")},
				}}),
		demo("Alignment", "Icons only, default variant.", col,
			kit.ToggleGroup[string]{AriaLabel: tr("tg.alignment", "Text alignment"), DisallowEmptySelection: true, Value: &align, OnValueChange: setAlign.Set,
				Items: []kit.ToggleGroupItem[string]{
					{Value: "start", Icon: icons.TextAlignStart, AriaLabel: tr("tg.alignStart", "Align left")},
					{Value: "center", Icon: icons.TextAlignCenter, AriaLabel: tr("tg.alignCenter", "Align center")},
					{Value: "end", Icon: icons.TextAlignEnd, AriaLabel: tr("tg.alignEnd", "Align right")},
				}},
			P(Class("w-64 text-sm", map[string]string{"start": "text-start", "center": "text-center", "end": "text-end"}[align]),
				Text(tr("tg.fox", "The quick brown fox jumps over the lazy dog.")))),
		demo("Spacing, sizes and disabled", "Spacing 2 separates the items (4 px steps); Sm and Lg; one item disabled.", col,
			kit.ToggleGroup[string]{AriaLabel: tr("tg.spaced", "Spaced"), Variant: kit.Outline, Spacing: 2, DefaultValue: ptr("a"),
				Items: []kit.ToggleGroupItem[string]{{Value: "a", Label: tr("tg.top", "Top")}, {Value: "b", Label: tr("tg.bottom", "Bottom")}, {Value: "c", Label: tr("tg.left", "Left")}}},
			kit.ToggleGroup[string]{AriaLabel: tr("tg.small", "Small"), Variant: kit.Outline, Size: kit.Sm, DefaultValue: ptr("a"),
				Items: []kit.ToggleGroupItem[string]{{Value: "a", Label: tr("tg.day", "Day")}, {Value: "b", Label: tr("tg.week", "Week")}, {Value: "c", Label: tr("tg.month", "Month"), Disabled: true}}},
			kit.ToggleGroup[string]{AriaLabel: tr("tg.large", "Large"), Size: kit.Lg, DefaultValue: ptr("a"),
				Items: []kit.ToggleGroupItem[string]{{Value: "a", Label: tr("tg.one", "One")}, {Value: "b", Label: tr("tg.two", "Two")}}}),
	)
}
