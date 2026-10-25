package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of the dates: Calendar, RangeCalendar, DatePicker and
// DateRangePicker.

// today is the day the gallery draws as today: a fixed one, so that a
// screenshot of the gallery is the same every day. An app leaves Today
// alone and gets the current day.
var today = aria.Date{Year: 2026, Month: time.October, Day: 25}

// long writes a date as "Sunday, October 25, 2026".
func long(d aria.Date) string {
	if d.IsZero() {
		return "none"
	}
	return d.Time(nil).Format("Monday, January 2, 2006")
}

func short(d aria.Date) string { return d.Time(nil).Format("Jan 2, 2006") }

func weekend(d aria.Date) bool { return d.Weekday() == time.Saturday || d.Weekday() == time.Sunday }

// wide makes a demo take both columns of the grid.
func wide(n tela.Node) tela.Node { return Div(Class("xl:col-span-2"), n) }

func calendarPage() tela.Node {
	day, setDay := tela.UseState(aria.Date{Year: 2026, Month: time.October, Day: 28})
	slot, setSlot := tela.UseState(aria.Date{})
	return grid(
		demo("Single date", "One tab stop; the arrows move over the days, Page Up and Down over the months (a year with Shift), Home and End to the ends of the week.", colStart,
			kit.Calendar{Today: today, Value: &day, OnValueChange: setDay.Set, Class: "rounded-md border border-border", AriaLabel: "Date"},
			readout("Selected: "+long(day))),
		demo("Limits", "Min, Max and IsDateUnavailable: out-of-range and unavailable dates are dimmed and cannot be chosen.", colStart,
			kit.Calendar{Today: today, Min: today, Max: today.AddDays(45), IsDateUnavailable: weekend, Value: &slot, OnValueChange: setSlot.Set,
				Class: "rounded-md border border-border", AriaLabel: "Appointment"},
			readout("Appointment: "+long(slot))),
		wide(demo("Two months", "VisibleMonths 2 with FixedWeeks (six rows each, so the height does not change) and HideOutsideDays.", "",
			kit.Calendar{Today: today, VisibleMonths: 2, FixedWeeks: true, HideOutsideDays: true, DefaultValue: aria.Date{Year: 2026, Month: time.November, Day: 5},
				Class: "rounded-md border border-border", AriaLabel: "Two months"})),
		wide(demo("Locales", "Names, digits, the first weekday and the direction come from the locale: Spanish starts on Monday, Arabic is right to left.", rowTop,
			kit.Calendar{Today: today, Locale: "es", Class: "rounded-md border border-border", AriaLabel: "Fecha"},
			kit.Calendar{Today: today, Locale: "ja", Class: "rounded-md border border-border", AriaLabel: "日付"},
			kit.Calendar{Today: today, Locale: "ar-EG", Class: "rounded-md border border-border", AriaLabel: "التاريخ"})),
		demo("Read-only and disabled", "ReadOnly keeps the dates focusable but not selectable.", rowTop,
			kit.Calendar{Today: today, ReadOnly: true, DefaultValue: today, Class: "rounded-md border border-border", AriaLabel: "Read-only"},
			kit.Calendar{Today: today, Disabled: true, Class: "rounded-md border border-border", AriaLabel: "Disabled"}),
	)
}

func rangeCalendarPage() tela.Node {
	stay, setStay := tela.UseState(aria.DateRange{Start: aria.Date{Year: 2026, Month: time.October, Day: 27}, End: aria.Date{Year: 2026, Month: time.November, Day: 3}})
	preset := func(r aria.DateRange) func() { return func() { setStay.Set(r) } }
	return grid(
		wide(demo("Two months", "The first press anchors the range, the pointer or the focus previews it, the second completes it; Escape cancels an anchored range.", colStart,
			kit.RangeCalendar{Today: today, VisibleMonths: 2, Value: &stay, OnValueChange: setStay.Set, Class: "rounded-md border border-border", AriaLabel: "Stay"},
			readout(rangeText(stay)),
			Div(Class("flex flex-wrap gap-2"),
				kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: preset(aria.DateRange{Start: today, End: today.AddDays(6)}), Children: Text("Next 7 days")},
				kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: preset(aria.DateRange{Start: aria.Date{Year: 2026, Month: time.November, Day: 1}, End: aria.Date{Year: 2026, Month: time.November, Day: 30}}), Children: Text("November")},
				kit.Button{Variant: kit.Ghost, Size: kit.Sm, OnClick: preset(aria.DateRange{}), Children: Text("Clear")}))),
		demo("Limits", "Dates before today and the weekends cannot be chosen, and a range cannot cross an unavailable date.", "",
			kit.RangeCalendar{Today: today, Min: today, IsDateUnavailable: weekend, Class: "rounded-md border border-border", AriaLabel: "Weekdays"}),
		demo("Non-contiguous", "AllowsNonContiguousRanges lets a range cross unavailable dates.", "",
			kit.RangeCalendar{Today: today, Min: today, IsDateUnavailable: weekend, AllowsNonContiguousRanges: true,
				DefaultValue: aria.DateRange{Start: aria.Date{Year: 2026, Month: time.October, Day: 27}, End: aria.Date{Year: 2026, Month: time.October, Day: 30}},
				Class:        "rounded-md border border-border", AriaLabel: "Any days"}),
	)
}

func rangeText(r aria.DateRange) string {
	if r.IsZero() {
		return "No dates chosen."
	}
	return "From " + short(r.Start) + " to " + short(r.End)
}

func datePickerPage() tela.Node {
	day, setDay := tela.UseState(aria.Date{})
	return grid(
		demo("Basic", "A button that shows the date and opens a calendar in a popover; choosing a date closes it and the focus goes back to the button.", col,
			kit.DatePicker{Today: today, Value: &day, OnValueChange: setDay.Set, Class: "w-64", AriaLabel: "Date"},
			readout("Chosen: "+long(day))),
		demo("In a Field", "The field's label names the button and its description describes it.", col,
			kit.Field{Class: "max-w-xs", Label: "Date of birth", Description: "Your age is calculated from it.",
				Children: kit.DatePicker{Today: today, Class: "w-full", Placeholder: "Select your birthday", Max: today}}),
		demo("Appointment", "Min and IsDateUnavailable: a booking window without weekends.", "",
			kit.DatePicker{Today: today, Class: "w-64", Placeholder: "Choose a day", Min: today, Max: today.AddDays(60), IsDateUnavailable: weekend, AriaLabel: "Appointment"}),
		demo("Format", "Format writes the date on the button, here in the medium style of the formatter.", "",
			kit.DatePicker{Today: today, Class: "w-64", AriaLabel: "Deadline", DefaultValue: aria.Date{Year: 2026, Month: time.December, Day: 1},
				Format: func(f i18n.Formatter, d aria.Date) string { return f.Date(d.Time(nil), i18n.Medium) }}),
		demo("States", "Disabled and invalid.", "",
			kit.DatePicker{Today: today, Disabled: true, Class: "w-56", AriaLabel: "Disabled"},
			kit.DatePicker{Today: today, Invalid: true, Class: "w-56", AriaLabel: "Invalid", Placeholder: "Required"}),
	)
}

func dateRangePickerPage() tela.Node {
	trip, setTrip := tela.UseState(aria.DateRange{})
	return grid(
		demo("Basic", "The button shows both ends; the popover holds two months and closes when the range is completed.", col,
			kit.DateRangePicker{Today: today, Value: &trip, OnValueChange: setTrip.Set, Class: "w-72", AriaLabel: "Trip"},
			readout(rangeText(trip))),
		demo("With a value", "DefaultValue starts the picker on a range.", "",
			kit.DateRangePicker{Today: today, Class: "w-72", AriaLabel: "Sprint",
				DefaultValue: aria.DateRange{Start: aria.Date{Year: 2026, Month: time.October, Day: 20}, End: aria.Date{Year: 2026, Month: time.November, Day: 5}}}),
		demo("In a Field", "", col,
			kit.Field{Class: "max-w-xs", Label: "Trip dates", Description: "Choose when you leave and when you come back.",
				Children: kit.DateRangePicker{Today: today, Class: "w-full", Min: today, Placeholder: "Pick your dates"}}),
		demo("Disabled", "", "",
			kit.DateRangePicker{Today: today, Disabled: true, Class: "w-72", AriaLabel: "Disabled"}),
	)
}
