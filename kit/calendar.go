// Portions of this file follow shadcn/ui, calendar.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// Icons embedded in this file are copied unaltered from Lucide
// (https://lucide.dev, lucide-static 1.61.0), Copyright (c) 2026 Lucide Icons and
// Contributors, licensed under the ISC License (the icons derived from Feather are
// MIT, Copyright (c) 2013-present Cole Bemis).
// See THIRD_PARTY.md.

package kit

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
)

// The icons Calendar and the date pickers draw, from Lucide (ISC, see
// THIRD_PARTY.md); each is replaceable with the PreviousIcon, NextIcon and
// Icon fields.
var (
	calendarChevronLeft  = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>`}
	calendarChevronRight = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`}
	calendarIcon         = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2v3"/><path d="M16 2v3"/><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/></svg>`}
)

// CalendarStyle is the recipe of [Calendar] and [RangeCalendar]
// (shadcn/ui's Calendar, whose parts are react-day-picker's: its
// classNames). The root element is the calendar; a day is a cell (Day) that
// holds a ghost icon button (DayButton), as in react-day-picker's td and
// button.
//
// shadcn sizes the calendar with the CSS variable --cell-size (2rem); the
// engine has no variables, so the recipe writes size-8, h-8, px-8 and
// min-w-8 where shadcn writes size-(--cell-size) and the like: change them
// together to resize the cells.
type CalendarStyle struct {
	// Root is the calendar: padding, background, the width of its content.
	Root string
	// Months holds the nav and the months, side by side from md up.
	Months string
	// Month is one month: its caption and its grid.
	Month string
	// Nav is the row of the previous and next buttons, over the captions.
	Nav string
	// ButtonPrevious and ButtonNext are added to the ghost button's classes
	// (the Button recipe's Root, Ghost variant and default size).
	ButtonPrevious, ButtonNext string
	// Chevron is the arrow icon of the nav buttons; rtl:rotate-180 turns it
	// around under a right-to-left direction.
	Chevron string
	// MonthCaption holds the month's title and CaptionLabel is the title.
	MonthCaption, CaptionLabel string
	// MonthGrid is the grid element of a month, Weekdays the row of weekday
	// names, Weekday one name, and Week one row of days.
	MonthGrid, Weekdays, Weekday, Week string
	// Day is the cell of a date: it sizes the day and carries the
	// background of today and of the ends of a range behind the button.
	Day string
	// Today, Outside, Disabled, Hidden, RangeStart, RangeMiddle and
	// RangeEnd are added to Day for a date in that state (react-day-picker's
	// modifiers). Disabled is a date that can not be selected (before Min,
	// after Max, unavailable); Hidden an outside date that is not shown.
	Today, Outside, Disabled, Hidden, RangeStart, RangeMiddle, RangeEnd string
	// TodayWithSelected is added to Day when today is selected
	// (shadcn's data-[selected=true]:rounded-none on the today modifier).
	TodayWithSelected string
	// DayButton is the button of a date, a ghost icon button resized to
	// fill its cell.
	DayButton string
	// DayButtonWithSelected is added to DayButton for the selected date of
	// a Calendar (shadcn's data-[selected-single=true]:), and
	// DayButtonWithRangeStart, DayButtonWithRangeMiddle and
	// DayButtonWithRangeEnd for the parts of a selected range.
	DayButtonWithSelected, DayButtonWithRangeStart, DayButtonWithRangeMiddle, DayButtonWithRangeEnd string
}

func init() {
	defaultStyle.Calendar = CalendarStyle{
		Root:           "w-fit bg-background p-3",
		Months:         "relative flex flex-col gap-4 md:flex-row",
		Month:          "flex w-full flex-col gap-4",
		Nav:            "absolute inset-x-0 top-0 flex w-full items-center justify-between gap-1",
		ButtonPrevious: "size-8 p-0 select-none aria-disabled:opacity-50",
		ButtonNext:     "size-8 p-0 select-none aria-disabled:opacity-50",
		Chevron:        "size-4 rtl:rotate-180",
		MonthCaption:   "flex h-8 w-full items-center justify-center px-8",
		CaptionLabel:   "text-sm font-medium select-none",
		MonthGrid:      "w-full",
		Weekdays:       "flex",
		Weekday:        "flex-1 rounded-md text-center text-[0.8rem] font-normal text-muted-foreground select-none",
		Week:           "mt-2 flex w-full",
		Day:            "relative h-full w-full p-0 text-center select-none",
		Today:          "rounded-md bg-accent text-accent-foreground",
		Outside:        "text-muted-foreground",
		Disabled:       "text-muted-foreground opacity-50",
		Hidden:         "invisible",
		RangeStart:     "rounded-s-md bg-accent",
		RangeMiddle:    "rounded-none",
		RangeEnd:       "rounded-e-md bg-accent",

		TodayWithSelected: "rounded-none",
		DayButton:         "flex aspect-square size-auto w-full min-w-8 flex-col gap-1 leading-none font-normal focus-visible:relative focus-visible:z-10",
		// A selected button keeps its colors under the pointer: the data-[…]:
		// classes shadcn writes win over the ghost variant's hover: classes
		// in the stylesheet, so here they are restated as hover: ones.
		DayButtonWithSelected:    "bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground dark:hover:bg-primary",
		DayButtonWithRangeStart:  "rounded-md rounded-s-md bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground dark:hover:bg-primary",
		DayButtonWithRangeMiddle: "rounded-none bg-accent text-accent-foreground hover:bg-accent dark:hover:bg-accent",
		DayButtonWithRangeEnd:    "rounded-md rounded-e-md bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground dark:hover:bg-primary",
	}
}

// Calendar is a month grid that selects one date, shadcn/ui's Calendar
// (react-day-picker's look) on aria.UseCalendar: a caption with the month
// and year between the previous and next buttons, the weekday names, and a
// ghost button for each day, with today, the selected date, the dates of the
// neighbouring months and the dates that can not be selected each styled.
//
//	day, setDay := tela.UseState(aria.Date{})
//	kit.Calendar{Value: &day, OnValueChange: setDay.Set, Min: aria.Today()}
//
// It is one tab stop: Tab reaches the focused date and the arrow keys move
// over the days (a week with up and down, swapped by a right-to-left
// direction), PageUp and PageDown over the months (a year with Shift), Home
// and End to the first and last day of the week, and Enter or Space select.
// The month on display follows the focus. Names, digits, the first day of the
// week and the direction come from package i18n: the component's locale, or
// Locale.
//
// The selected date follows the kit's value rule with the Date type of
// package aria, a comparable day with no zone: Value (a pointer to your
// state) with OnValueChange controls it, DefaultValue starts an uncontrolled
// one; the zero Date is none. See [RangeCalendar] for a range and
// [DatePicker] for a calendar in a popover.
//
// Deviations from shadcn: react-day-picker's year and month dropdowns
// (captionLayout), week numbers and its other modes (several dates) are not
// ported; outside dates that belong to another visible month are hidden, as
// there; the date cell's aria-label is the full localized date without
// "Today" or "selected" (aria-selected carries the state).
type Calendar struct {
	// Value points to the selected date of a controlled calendar; nil
	// leaves it to the calendar, which starts at DefaultValue.
	Value        *aria.Date
	DefaultValue aria.Date
	// OnValueChange is called with the date the user selects.
	OnValueChange func(aria.Date)

	// Min and Max bound the dates that can be focused or selected, both
	// included; the zero Date is no limit. Dates outside them are dimmed
	// and the month buttons stop at them.
	Min, Max aria.Date
	// IsDateUnavailable marks dates that are shown dimmed and can not be
	// selected (booked days, holidays).
	IsDateUnavailable func(aria.Date) bool
	// Disabled disables every date; ReadOnly keeps them focusable but not
	// selectable.
	Disabled, ReadOnly bool
	// FocusedDate points to the focused date of a controlled calendar, the
	// one with the tab stop, whose month is on display; nil leaves it to the
	// calendar, which starts at DefaultFocusedDate, else the selected date,
	// else today. OnFocusChange is called when it changes.
	FocusedDate        *aria.Date
	DefaultFocusedDate aria.Date
	OnFocusChange      func(aria.Date)
	// VisibleMonths is the number of months shown side by side (1 to 12,
	// default 1; the layout is a column below the md breakpoint).
	VisibleMonths int
	// FirstDayOfWeek is the first column; nil takes the locale's.
	FirstDayOfWeek *time.Weekday
	// FixedWeeks always shows six weeks per month, so the calendar keeps
	// its height from month to month.
	FixedWeeks bool
	// HideOutsideDays leaves out the dates of the neighbouring months that
	// fill the first and last week (shadcn shows them by default).
	HideOutsideDays bool
	// NarrowWeekdays writes the column headers with the narrowest weekday
	// names (one letter, usually) instead of the short ones ("M" for "Mo"):
	// for a locale whose short names do not fit a 32 px cell, such as
	// Arabic's. The headers keep the full weekday as their name for
	// assistive technologies.
	NarrowWeekdays bool
	// Today is the date drawn as today; the zero Date is the current day.
	// Set it to make a test or a golden deterministic.
	Today aria.Date
	// Locale is the BCP 47 tag of the names, digits, first weekday and
	// direction; empty is the component's locale (i18n.UseLocale).
	// Direction is "ltr" or "rtl"; empty follows the Locale when one is
	// given, else the direction at the calendar's position
	// (tela.UseDirection: the nearest h.Dir above it). Either one given
	// here also sets the direction of the calendar's layout.
	Locale, Direction string
	// AriaLabel names the calendar ("Appointment date"); the visible month
	// is appended. PreviousLabel and NextLabel name the month buttons
	// ("Previous" and "Next" by default: translate them with i18n.T).
	AriaLabel                string
	PreviousLabel, NextLabel string
	// PreviousIcon and NextIcon replace the chevrons of the month buttons.
	PreviousIcon, NextIcon tela.Icon

	// Class is added after the recipe's Root classes; the other Class fields
	// do the same for their part: the months row, a month, the nav row, a
	// caption, a weekday name, a day cell and a day button.
	Class          string
	MonthsClass    string
	MonthClass     string
	NavClass       string
	CaptionClass   string
	WeekdayClass   string
	DayClass       string
	DayButtonClass string
	// Props are more props for the root element.
	Props tela.Child

	// autoFocus makes the focused date's cell take the focus when the
	// calendar mounts: a date picker opening its popover.
	autoFocus bool
}

// Build returns the calendar element.
func (c Calendar) Build() tela.Node {
	keysDir := useCalendarDirection(c.Direction, c.Locale)
	res := aria.UseCalendar(aria.CalendarOptions{
		Value: c.Value, DefaultValue: ptrIfSet(c.DefaultValue), OnValueChange: c.OnValueChange,
		Min: c.Min, Max: c.Max, IsDateUnavailable: c.IsDateUnavailable, Disabled: c.Disabled, ReadOnly: c.ReadOnly,
		FocusedDate: c.FocusedDate, DefaultFocusedDate: c.DefaultFocusedDate, OnFocusChange: c.OnFocusChange,
		VisibleMonths: c.VisibleMonths, FirstDayOfWeek: c.FirstDayOfWeek, FixedWeeks: c.FixedWeeks, Today: c.Today,
		Locale: c.Locale, Direction: keysDir, AriaLabel: c.AriaLabel,
		PreviousLabel: c.PreviousLabel, NextLabel: c.NextLabel,
	})
	return useCalendarView(calendarLook{
		hideOutside: c.HideOutsideDays, narrowWeekdays: c.NarrowWeekdays, min: c.Min, max: c.Max, locale: c.Locale, direction: c.Direction, prev: c.PreviousIcon, next: c.NextIcon,
		class: c.Class, months: c.MonthsClass, month: c.MonthClass, nav: c.NavClass, caption: c.CaptionClass,
		weekday: c.WeekdayClass, day: c.DayClass, dayButton: c.DayButtonClass, props: c.Props, autoFocus: c.autoFocus,
	}, res, false)
}

// RangeCalendar is a [Calendar] that selects a span of dates, shadcn/ui's
// Calendar in range mode on aria.UseRangeCalendar. Selecting takes two
// presses: the first anchors the range, the pointer over a date or the focus
// on it previews the range up to it (the band of accent color between the
// two ends), and the second completes it, the earlier date first. Escape
// cancels an anchored range. While one is anchored, dates beyond the nearest
// unavailable one are not selectable, unless AllowsNonContiguousRanges.
//
//	var stay aria.DateRange
//	kit.RangeCalendar{VisibleMonths: 2, OnValueChange: func(r aria.DateRange) { stay = r }}
//
// The value follows the kit's rule with aria.DateRange: Value (a pointer)
// with OnValueChange controls it, DefaultValue starts an uncontrolled one;
// OnValueChange is called when a range is completed, not when it is
// anchored. The other fields are those of [Calendar].
type RangeCalendar struct {
	// Value points to the selected range of a controlled calendar; nil
	// leaves it to the calendar, which starts at DefaultValue.
	Value        *aria.DateRange
	DefaultValue aria.DateRange
	// OnValueChange is called with the range the user completes.
	OnValueChange func(aria.DateRange)
	// AllowsNonContiguousRanges lets a range cross unavailable dates.
	AllowsNonContiguousRanges bool

	Min, Max                 aria.Date
	IsDateUnavailable        func(aria.Date) bool
	Disabled, ReadOnly       bool
	FocusedDate              *aria.Date
	DefaultFocusedDate       aria.Date
	OnFocusChange            func(aria.Date)
	VisibleMonths            int
	FirstDayOfWeek           *time.Weekday
	FixedWeeks               bool
	HideOutsideDays          bool
	NarrowWeekdays           bool
	Today                    aria.Date
	Locale, Direction        string
	AriaLabel                string
	PreviousLabel, NextLabel string
	PreviousIcon, NextIcon   tela.Icon

	Class          string
	MonthsClass    string
	MonthClass     string
	NavClass       string
	CaptionClass   string
	WeekdayClass   string
	DayClass       string
	DayButtonClass string
	Props          tela.Child

	autoFocus bool
}

// Build returns the calendar element.
func (c RangeCalendar) Build() tela.Node {
	keysDir := useCalendarDirection(c.Direction, c.Locale)
	var def *aria.DateRange
	if !c.DefaultValue.IsZero() {
		def = &c.DefaultValue
	}
	res := aria.UseRangeCalendar(aria.RangeCalendarOptions{
		Value: c.Value, DefaultValue: def, OnValueChange: c.OnValueChange,
		AllowsNonContiguousRanges: c.AllowsNonContiguousRanges,
		Min:                       c.Min, Max: c.Max, IsDateUnavailable: c.IsDateUnavailable, Disabled: c.Disabled, ReadOnly: c.ReadOnly,
		FocusedDate: c.FocusedDate, DefaultFocusedDate: c.DefaultFocusedDate, OnFocusChange: c.OnFocusChange,
		VisibleMonths: c.VisibleMonths, FirstDayOfWeek: c.FirstDayOfWeek, FixedWeeks: c.FixedWeeks, Today: c.Today,
		Locale: c.Locale, Direction: keysDir, AriaLabel: c.AriaLabel,
		PreviousLabel: c.PreviousLabel, NextLabel: c.NextLabel,
	})
	return useCalendarView(calendarLook{
		hideOutside: c.HideOutsideDays, narrowWeekdays: c.NarrowWeekdays, min: c.Min, max: c.Max, locale: c.Locale, direction: c.Direction, prev: c.PreviousIcon, next: c.NextIcon,
		class: c.Class, months: c.MonthsClass, month: c.MonthClass, nav: c.NavClass, caption: c.CaptionClass,
		weekday: c.WeekdayClass, day: c.DayClass, dayButton: c.DayButtonClass, props: c.Props, autoFocus: c.autoFocus,
	}, res, true)
}

// useCalendarDirection is the direction of a calendar's arrow keys: the
// Direction it was given, else the locale's (the aria hook resolves "")
// when it has a Locale, else the direction at its position. It is a hook.
// Only a Direction or a Locale given to the calendar sets h.Dir on its root
// (useCalendarView): the direction at its position is already its own.
func useCalendarDirection(direction, locale string) string {
	at := tela.UseDirection()
	if direction == "" && locale == "" {
		return at
	}
	return direction
}

// calendarLook is what Calendar and RangeCalendar tell the view besides the
// hook's result: the presentation fields of the two structs.
type calendarLook struct {
	hideOutside, narrowWeekdays bool
	min, max                    aria.Date
	locale, direction           string
	prev, next                  tela.Icon
	class, months, month, nav   string
	caption, weekday, day       string
	dayButton                   string
	props                       tela.Child
	autoFocus                   bool
}

// ptrIfSet returns &d, or nil for the zero Date.
func ptrIfSet(d aria.Date) *aria.Date {
	if d.IsZero() {
		return nil
	}
	return &d
}

// useCalendarView draws a calendar from the hook's result; it is the part
// Calendar and RangeCalendar have in common. It is a hook (it reads the
// style and keeps the first-build flag), called from their Build.
func useCalendarView(o calendarLook, res aria.CalendarResult, rng bool) tela.Node {
	st := UseStyle()
	s, btn := &st.Calendar, &st.Button
	first := useFirstBuild() && o.autoFocus

	f := i18n.UseFormatter()
	if o.locale != "" {
		f = i18n.For(o.locale)
	}
	dir := o.direction
	if dir == "" && o.locale != "" {
		dir = f.Dir().String()
	}

	prevIcon, nextIcon := iconOr(o.prev, calendarChevronLeft), iconOr(o.next, calendarChevronRight)
	navButton := func(props tela.Prop, class string, ic tela.Icon) tela.Node {
		return h.Button(props, cls(btn.Root, pick("Button", "Variant", btn.Variants, Ghost),
			pick("Button", "Size", btn.Sizes, Size("")), btn.HasIcon[""], class),
			icon(ic, s.Chevron))
	}

	months := res.Months()
	monthNodes := make([]tela.Child, 0, months+1)
	monthNodes = append(monthNodes, h.Div(cls(s.Nav, o.nav),
		navButton(res.PrevProps(), s.ButtonPrevious, prevIcon),
		navButton(res.NextProps(), s.ButtonNext, nextIcon)))
	width := i18n.NameShort
	if o.narrowWeekdays {
		width = i18n.NameNarrow
	}
	weekdays := res.WeekdayNames(width)
	for m := range months {
		month := res.Month(m)
		weeks := res.Weeks(m)
		rows := make([]tela.Child, 0, len(weeks))
		for _, week := range weeks {
			cells := make([]tela.Child, 0, 7)
			for _, day := range week {
				cells = append(cells, calendarDay(s, btn, o, res, m, day, rng, first))
			}
			rows = append(rows, h.Div(res.RowProps(), h.Key(week[0].String()), cls(s.Week), h.Fragment(cells...)))
		}
		head := make([]tela.Child, 0, 7)
		for i, name := range weekdays {
			head = append(head, h.Div(res.ColumnHeaderProps(i), h.Key(i), cls(s.Weekday, o.weekday), h.Text(name)))
		}
		monthNodes = append(monthNodes, h.Div(h.Key(month.String()), cls(s.Month, o.month),
			h.Div(cls(s.MonthCaption, o.caption),
				h.H2(res.HeadingProps(m), cls(s.CaptionLabel), h.Text(res.Heading(m)))),
			h.Div(append([]tela.Child{res.GridProps(m), cls(s.MonthGrid),
				h.Div(res.RowProps(), cls(s.Weekdays), h.Fragment(head...))}, rows...)...)))
	}

	kids := []tela.Child{res.Props, cls(s.Root, o.class)}
	if dir != "" {
		kids = append(kids, h.Dir(dir))
	}
	kids = append(kids, o.props, h.Div(append([]tela.Child{cls(s.Months, o.months)}, monthNodes...)...))
	return h.Div(kids...)
}

// calendarDay draws the cell of date d in the grid of month m.
func calendarDay(s *CalendarStyle, btn *ButtonStyle, o calendarLook, res aria.CalendarResult, m int, d aria.Date, rng, first bool) tela.Node {
	outside := res.IsOutsideMonth(m, d)
	if outside {
		hidden := o.hideOutside
		// A date that belongs to another visible month is shown there.
		for j := range res.Months() {
			if j != m && !res.IsOutsideMonth(j, d) {
				hidden = true
			}
		}
		// So is one whose month the calendar can not show (before Min's
		// month or after Max's), as react-day-picker hides the days of the
		// months outside startMonth and endMonth.
		if !o.min.IsZero() && monthIndex(d) < monthIndex(o.min) || !o.max.IsZero() && monthIndex(d) > monthIndex(o.max) {
			hidden = true
		}
		if hidden {
			// An empty cell as big as a day button, so the grid keeps its shape.
			return h.Div(h.Key(d.String()), cls(s.Day, s.Hidden, o.day), h.AriaHidden(true), h.Div(cls(s.DayButton)))
		}
	}
	selected := res.IsSelected(d)
	start, end := rng && res.IsRangeStart(d), rng && res.IsRangeEnd(d)
	middle := rng && res.IsInRange(d) && !start && !end
	today := res.IsToday(d)

	day := []string{s.Day}
	switch {
	case start && end:
		// A one-day range is both ends: the button is the primary one and
		// the cell has no band.
	case start:
		day = append(day, s.RangeStart)
	case middle:
		day = append(day, s.RangeMiddle)
	case end:
		day = append(day, s.RangeEnd)
	}
	if today {
		day = append(day, s.Today, when(selected, s.TodayWithSelected))
	}
	day = append(day, when(outside, s.Outside), when(res.IsDisabled(d) || res.IsUnavailable(d), s.Disabled), o.day)

	var button []string
	switch {
	case !rng && selected:
		button = append(button, s.DayButtonWithSelected)
	case start:
		button = append(button, s.DayButtonWithRangeStart)
	case end:
		button = append(button, s.DayButtonWithRangeEnd)
	case middle:
		button = append(button, s.DayButtonWithRangeMiddle)
	}
	kids := []tela.Child{res.CellProps(m, d), cls(btn.Root, pick("Button", "Variant", btn.Variants, Ghost),
		pick("Button", "Size", btn.Sizes, Icon), s.DayButton, joinClass(button...), o.dayButton)}
	if first && !outside && d == res.FocusedDate {
		kids = append(kids, h.AutoFocus(true))
	}
	kids = append(kids, h.Text(res.DayText(d)))
	return h.Div(h.Key(d.String()), cls(day...), h.Button(kids...))
}

// monthIndex numbers the months of the calendar, to compare them.
func monthIndex(d aria.Date) int { return d.Year*12 + int(d.Month) }

// useFirstBuild reports whether this is the component's first build, for
// things that happen as it mounts (the focus). It is a hook.
func useFirstBuild() bool {
	done := tela.UseRef(false)
	first := !done.Current
	tela.UseEffect(func() func() {
		done.Current = true
		return nil
	})
	return first
}
