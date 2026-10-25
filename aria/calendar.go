// Portions of this file follow React Aria's useCalendar, useRangeCalendar,
// useCalendarGrid and useCalendarCell and react-stately's
// useCalendarState and useRangeCalendarState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/calendar
// and packages/@react-stately/calendar), Copyright 2020 Adobe, licensed
// under the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/internal/core"
)

// CalendarOptions configures [UseCalendar]. Dates are [Date] values, the
// zero Date meaning none.
//
// The selected date is uncontrolled when Value is nil (the hook keeps it,
// starting at DefaultValue, or with nothing selected) and controlled when
// Value points to the caller's state: the calendar shows it and only asks
// OnValueChange for the next one, so a change the caller does not store is
// refused. OnValueChange is also called for an uncontrolled calendar. The
// focused date, which decides the month on show and the tab stop, follows
// the same rule with FocusedDate, DefaultFocusedDate and OnFocusChange.
//
//	// Uncontrolled.
//	aria.CalendarOptions{AriaLabel: "Appointment date"}
//	// Controlled.
//	day, setDay := tela.UseState(aria.Date{})
//	aria.CalendarOptions{Value: &day, OnValueChange: setDay.Set, Min: aria.Today()}
type CalendarOptions struct {
	// Value points to the selected date of a controlled calendar; nil
	// leaves it to the hook.
	Value *Date
	// DefaultValue is the selected date of an uncontrolled calendar at the
	// start; the zero Date selects nothing.
	DefaultValue Date
	// OnValueChange is called with the date the user selects: how a
	// controlled calendar asks for the change, a notification for an
	// uncontrolled one. It is not called for the date that already is
	// selected.
	OnValueChange func(Date)
	// Min and Max bound the dates that can be focused or selected (both
	// included); the zero Date is no limit. Dates outside them are shown
	// disabled, and the buttons of PrevProps and NextProps are disabled
	// when no month is left in that direction. Min after Max panics in
	// dev mode.
	Min, Max Date
	// IsDateUnavailable marks dates that are shown but can not be
	// selected (booked days, holidays). As in React Aria they can still
	// be focused: the keys move over them and they have aria-disabled.
	IsDateUnavailable func(Date) bool
	// Disabled disables every date: none can be focused or selected.
	Disabled bool
	// ReadOnly keeps the dates focusable but not selectable (aria-readonly
	// on the grid).
	ReadOnly bool
	// FocusedDate points to the focused date of a controlled calendar,
	// the one with the tab stop; the month that shows it is on display.
	// nil leaves it to the hook.
	FocusedDate *Date
	// DefaultFocusedDate is the focused date of an uncontrolled calendar at
	// the start: else the selected date, else Today. It is limited to Min
	// and Max.
	DefaultFocusedDate Date
	// OnFocusChange is called with the date that becomes the focused one:
	// by keys, by focus, by a month button, by a pointer that previews a
	// range. For a controlled calendar it is how the change is asked for.
	OnFocusChange func(Date)
	// VisibleMonths is the number of months shown at once: 1 (the
	// default) to 12. The month buttons move by that many.
	VisibleMonths int
	// FirstDayOfWeek is the first column of the grid; nil takes the
	// locale's (i18n.FirstDayOfWeek: Sunday for en-US, Monday for de,
	// Saturday for ar-EG).
	FirstDayOfWeek *time.Weekday
	// FixedWeeks always shows six weeks per month, so the grid keeps its
	// height from month to month; else a month has the weeks it needs.
	FixedWeeks bool
	// Today is the date [CalendarResult.IsToday] reports; the zero Date is
	// the current day (aria.Today). Set it to make a test deterministic.
	Today Date
	// Locale is the BCP 47 tag of the names, the headings and the first
	// weekday; empty is the component's locale (i18n.UseFormatter).
	Locale string
	// Direction is "ltr" or "rtl": it swaps ArrowLeft and ArrowRight. Empty
	// is the locale's.
	Direction string
	// AriaLabel names the calendar ("Appointment date"); the visible range
	// ("October 2026") is appended, as React Aria does. Else AriaLabelledBy
	// names it.
	AriaLabel      string
	AriaLabelledBy tela.ElementRef
	// PreviousLabel and NextLabel are the accessible names of the month
	// buttons, "Previous" and "Next" by default. They are English unless
	// the caller translates them (with i18n.T).
	PreviousLabel, NextLabel string
	// Ref is the calendar element's ref, for a hook that is used together
	// with others on one element; the zero value makes the hook create
	// its own.
	Ref tela.ElementRef
}

// RangeCalendarOptions configures [UseRangeCalendar]. The fields shared
// with [CalendarOptions] mean the same; the value is a [DateRange].
type RangeCalendarOptions struct {
	// Value points to the selected range of a controlled calendar; nil
	// leaves it to the hook. DefaultValue starts an uncontrolled one,
	// and OnValueChange is called with the range the user completes (the
	// earlier date first). The zero DateRange selects nothing.
	Value         *DateRange
	DefaultValue  DateRange
	OnValueChange func(DateRange)
	// AllowsNonContiguousRanges lets a range cross unavailable dates.
	// Without it, once the first date is chosen the dates beyond the
	// nearest unavailable one on each side are unavailable too, so a
	// range never contains one (React Aria's rule). The nearest one is
	// searched three years either way.
	AllowsNonContiguousRanges bool

	Min, Max           Date
	IsDateUnavailable  func(Date) bool
	Disabled           bool
	ReadOnly           bool
	FocusedDate        *Date
	DefaultFocusedDate Date
	OnFocusChange      func(Date)
	VisibleMonths      int
	FirstDayOfWeek     *time.Weekday
	FixedWeeks         bool
	Today              Date
	Locale             string
	Direction          string
	AriaLabel          string
	AriaLabelledBy     tela.ElementRef
	PreviousLabel      string
	NextLabel          string
	Ref                tela.ElementRef
}

// calSpec is what the two option structs have in common.
type calSpec struct {
	min, max       Date
	unavailable    func(Date) bool
	disabled       bool
	readOnly       bool
	focusedDate    *Date
	defaultFocused Date
	onFocusChange  func(Date)
	visible        int
	firstDay       *time.Weekday
	fixedWeeks     bool
	today          Date
	locale         string
	direction      string
	label          string
	labelledBy     tela.ElementRef
	prevLabel      string
	nextLabel      string
	ref            tela.ElementRef
	nonContiguous  bool
}

// CalendarResult is what [UseCalendar] and [UseRangeCalendar] return. Its
// methods give the props and the data of each part of the calendar: the
// heading and the month buttons, the weekday headers, and one grid of
// weeks of cells per visible month (see [UseCalendar] for how they fit
// together).
type CalendarResult struct {
	// Props go to the calendar's element: the "application" role, its name
	// and the ref (React Aria's calendarProps).
	Props tela.Prop
	// Title is the heading of the whole visible range, month and year in
	// the locale's order ("October 2026", "October 2026 – November 2026"
	// with two months): what an app shows above a single grid, or announces.
	Title string
	// Value is the selected date of a single-date calendar, the zero Date
	// for none. Range is the selected range of a range calendar (while the
	// first date of a new one is chosen it is still the previous range).
	Value Date
	Range DateRange
	// FocusedDate is the date with the tab stop.
	FocusedDate Date
	// Ref is the ref attached to the calendar by Props.
	Ref tela.ElementRef

	c *cal
}

// cal is the state and the configuration of one calendar. Like the
// collections it lives in a UseOnce: the handlers write it directly (a
// second event before the next build sees the first), the build reads
// it, and a counter asks for the build.
type cal struct {
	bump tela.Setter[int]

	// The configuration of this build.
	f        i18n.Formatter
	s        calSpec
	rng      bool
	val      calValue
	foc      *choice[Date]
	defFocus Date
	today    Date
	firstDay time.Weekday
	rtl      bool
	visible  int
	min, max Date

	heads map[Date]tela.ElementRef
	refs  map[cellKey]tela.ElementRef

	// start is the first day of the first month on display.
	start Date
	// anchor is the first date of a range being selected; winLo and winHi
	// bound the dates that may complete it (zero: no bound).
	anchor       Date
	hasAnchor    bool
	winLo, winHi Date
	// pending is the date whose cell must take the DOM focus as soon as it
	// is rendered: the month changed under the focus.
	pending    Date
	hasPending bool
	// hasFocus is whether the DOM focus is inside the calendar.
	hasFocus bool
	mounted  bool
}

// cellKey identifies a cell: the date and the month of its grid (an
// outside date is a cell of two grids when two months are shown).
type cellKey struct{ month, date Date }

// calValue is the selected value: a date (start == end) or a range.
type calValue interface {
	get() DateRange
	set(DateRange)
}

type singleValue struct{ c *choice[Date] }

func (v singleValue) get() DateRange {
	d, _ := v.c.value()
	return DateRange{d, d}
}
func (v singleValue) set(r DateRange) { v.c.request(r.Start) }

type rangeValue struct{ c *choice[DateRange] }

func (v rangeValue) get() DateRange {
	r, _ := v.c.value()
	return r
}
func (v rangeValue) set(r DateRange) { v.c.request(r) }

// UseCalendar holds the state of a calendar that selects one date and
// returns the props and data of its parts (React Aria's useCalendar,
// useCalendarGrid and useCalendarCell with react-stately's
// useCalendarState). The calendar is the Gregorian one; a date is a
// [Date], and the names, the headings and the first day of the week come
// from package i18n.
//
// The caller renders the calendar. Each part takes its props from the
// result. The cells are keyed by their date and each grid by its month, so
// that a month on display is a new grid whose cell for the focused date
// takes the focus as it mounts (a calendar without those keys still works:
// the hook focuses the cell after the build):
//
//	cal := aria.UseCalendar(aria.CalendarOptions{AriaLabel: "Birthday"})
//	return Div(cal.Props, Class("w-64 p-3"),
//	    Div(Class("flex items-center justify-between"),
//	        Button(cal.PrevProps(), Text("<")),
//	        H2(cal.HeadingProps(0), Text(cal.Heading(0))),
//	        Button(cal.NextProps(), Text(">"))),
//	    Div(cal.GridProps(0), Key(cal.Month(0).String()),
//	        Div(cal.RowProps(), Class("grid grid-cols-7"),
//	            MapIndex(cal.WeekdayNames(i18n.NameShort), func(i int, name string) tela.Node {
//	                return Div(cal.ColumnHeaderProps(i), Key(name), Text(name))
//	            })),
//	        Map(cal.Weeks(0), func(week []aria.Date) tela.Node {
//	            return Div(cal.RowProps(), Key(week[0].String()), Class("grid grid-cols-7"),
//	                Map(week, func(day aria.Date) tela.Node {
//	                    return Button(cal.CellProps(0, day), Key(day.String()),
//	                        Class("aria-selected:bg-primary aria-selected:text-primary-foreground"),
//	                        If(cal.IsOutsideMonth(0, day), Class("opacity-50")),
//	                        Text(cal.DayText(day)))
//	                }))
//	        })))
//
// A cell is one element, a focusable gridcell (the ARIA grid pattern): use
// a Button, which gives it a focus ring, and style the states from the
// `aria-selected:`, `aria-disabled:` and `focus-visible:` variants or from
// the queries of the result ([CalendarResult.IsToday],
// [CalendarResult.IsUnavailable]).
//
// Keys, on a cell: ArrowLeft and ArrowRight move the focus a day (swapped
// by a right-to-left Direction), ArrowUp and ArrowDown a week, PageUp and
// PageDown a month and with Shift a year (the day of the month is kept, or
// the last one), Home and End the first and last day of the week, Enter and
// Space select the date. The month on display follows the focus. Dates
// outside Min and Max can not be reached (the focus stops at the limit);
// unavailable dates can, and are not selectable.
func UseCalendar(o CalendarOptions) CalendarResult {
	f := i18n.UseFormatter()
	var def *Date
	if !o.DefaultValue.IsZero() {
		def = &o.DefaultValue
	}
	val := useChoice(o.Value, def, o.OnValueChange)
	return useCalendar("UseCalendar", calSpec{
		min: o.Min, max: o.Max, unavailable: o.IsDateUnavailable, disabled: o.Disabled, readOnly: o.ReadOnly,
		focusedDate: o.FocusedDate, defaultFocused: o.DefaultFocusedDate, onFocusChange: o.OnFocusChange,
		visible: o.VisibleMonths, firstDay: o.FirstDayOfWeek, fixedWeeks: o.FixedWeeks, today: o.Today,
		locale: o.Locale, direction: o.Direction, label: o.AriaLabel, labelledBy: o.AriaLabelledBy,
		prevLabel: o.PreviousLabel, nextLabel: o.NextLabel, ref: o.Ref,
	}, f, false, singleValue{val})
}

// UseRangeCalendar is [UseCalendar] for a range of dates (React Aria's
// useRangeCalendar with useRangeCalendarState). Selecting works in two
// steps: a press or Enter on a date anchors the range, and while it is
// anchored the pointer over a date, or the focus on it, previews the range
// between the anchor and that date (see [CalendarResult.IsInRange]); a
// second press completes the range, whichever end comes first, and calls
// OnValueChange. Escape cancels an anchored range. While anchored, dates
// beyond the nearest unavailable one are not selectable, unless
// AllowsNonContiguousRanges. The result is the same as a single calendar's;
// its Range is the selected range, and [CalendarResult.Anchor] the first
// date of a range being chosen.
func UseRangeCalendar(o RangeCalendarOptions) CalendarResult {
	f := i18n.UseFormatter()
	var def *DateRange
	if !o.DefaultValue.IsZero() {
		def = &o.DefaultValue
	}
	val := useChoice(o.Value, def, o.OnValueChange)
	return useCalendar("UseRangeCalendar", calSpec{
		min: o.Min, max: o.Max, unavailable: o.IsDateUnavailable, disabled: o.Disabled, readOnly: o.ReadOnly,
		focusedDate: o.FocusedDate, defaultFocused: o.DefaultFocusedDate, onFocusChange: o.OnFocusChange,
		visible: o.VisibleMonths, firstDay: o.FirstDayOfWeek, fixedWeeks: o.FixedWeeks, today: o.Today,
		locale: o.Locale, direction: o.Direction, label: o.AriaLabel, labelledBy: o.AriaLabelledBy,
		prevLabel: o.PreviousLabel, nextLabel: o.NextLabel, ref: o.Ref, nonContiguous: o.AllowsNonContiguousRanges,
	}, f, true, rangeValue{val})
}

func useCalendar(hook string, s calSpec, cur i18n.Formatter, rng bool, val calValue) CalendarResult {
	_, bump := tela.UseState(0)
	c := tela.UseOnce(func() *cal {
		return &cal{refs: map[cellKey]tela.ElementRef{}, heads: map[Date]tela.ElementRef{}}
	})
	own := tela.UseElementRef()
	c.bump = bump
	c.configure(hook, s, cur, rng, val)

	// The focused date starts at the one the caller names, else the
	// selected date, else today; the choice keeps it from there.
	selected := val.get().Start
	def := s.defaultFocused
	if def.IsZero() {
		def = selected
	}
	if def.IsZero() {
		def = c.today
	}
	def = c.clamp(def)
	c.defFocus = def
	c.foc = useChoice(s.focusedDate, &def, s.onFocusChange)

	// A selection made from outside moves the focus to it, unless the
	// calendar has the focus.
	tela.UseEffect(func() func() {
		if !c.mounted {
			c.mounted = true
			return nil
		}
		if !c.hasFocus && !selected.IsZero() {
			c.foc.request(c.clamp(selected))
		}
		return nil
	}, selected)

	c.sync()
	c.prune()

	// The cell that must take the focus is a new element (AutoFocus) or
	// an old one that now stands for another date (its ref changed): after
	// the build the second can be focused.
	tela.UseEffect(func() func() {
		if c.hasPending {
			ref := c.cellRef(c.pending.firstOfMonth(), c.pending)
			ref.Focus()
			if ref.Focused() {
				c.hasPending = false
			}
		}
		return nil
	})

	ref := s.ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	title := c.title()
	ps := []tela.Prop{h.Ref(ref), h.Role("application")}
	switch {
	case s.label != "":
		ps = append(ps, h.AriaLabel(s.label+", "+title))
	case s.labelledBy != (tela.ElementRef{}):
		ps = append(ps, h.AriaLabelledBy(s.labelledBy))
	default:
		ps = append(ps, h.AriaLabel(title))
	}
	ps = append(ps,
		h.OnKeyDown(func(e tela.KeyboardEvent) {
			// Escape cancels a range being chosen, and is that
			// key's only use here (an overlay around the calendar
			// does not see it).
			if e.Key == tela.KeyEscape && c.hasAnchor {
				e.PreventDefault()
				e.StopPropagation()
				c.hasAnchor = false
				c.invalidate()
			}
		}),
	)
	r := val.get()
	res := CalendarResult{
		Props: core.MergeProps(ps...), Title: title, FocusedDate: c.focused(), Ref: ref, c: c,
	}
	if rng {
		res.Range = r
	} else {
		res.Value = r.Start
	}
	return res
}

// configure resolves the options of this build.
func (c *cal) configure(hook string, s calSpec, cur i18n.Formatter, rng bool, val calValue) {
	const where = ": CalendarOptions."
	c.s, c.rng, c.val = s, rng, val
	c.f = cur
	if s.locale != "" {
		c.f = i18n.For(s.locale)
	}
	c.today = s.today
	if c.today.IsZero() {
		c.today = Today()
	}
	c.firstDay = c.f.FirstDayOfWeek()
	if s.firstDay != nil {
		c.firstDay = *s.firstDay
	}
	dir := checkChoice(hook+where, "Direction", s.direction, "", "ltr", "rtl")
	c.rtl = dir == "rtl" || dir == "" && c.f.Dir() == i18n.RTL
	c.visible = s.visible
	if c.visible == 0 {
		c.visible = 1
	}
	c.min, c.max = s.min, s.max
	if core.DevNow() {
		switch {
		case c.visible < 0 || c.visible > 12:
			panic("aria: " + hook + where + "VisibleMonths must be between 1 and 12")
		case !c.min.IsZero() && !c.max.IsZero() && c.max.Before(c.min):
			panic("aria: " + hook + where + "Min (" + c.min.String() + ") is after Max (" + c.max.String() + "); swap them")
		case !c.min.IsZero() && !c.min.IsValid(), !c.max.IsZero() && !c.max.IsValid():
			panic("aria: " + hook + where + "Min and Max must be valid dates (year 1 to 9999) or the zero Date")
		}
	}
	c.visible = max(1, min(c.visible, 12))
}

func (c *cal) invalidate() { c.bump.Update(func(n int) int { return n + 1 }) }

// clamp limits d to Min and Max.
func (c *cal) clamp(d Date) Date {
	if !c.min.IsZero() && d.Before(c.min) {
		return c.min
	}
	if !c.max.IsZero() && d.After(c.max) {
		return c.max
	}
	return d
}

// focused is the focused date of this build.
func (c *cal) focused() Date {
	d, has := c.foc.value()
	if !has || d.IsZero() {
		d = c.defFocus
	}
	return c.clamp(d)
}

// sync keeps the focused date in the months on display, moving the window
// to the focus (React Aria's alignStart) when it left.
func (c *cal) sync() {
	f := c.focused()
	if c.start.IsZero() {
		c.start = c.alignStart(f)
		return
	}
	if i := c.start.monthsUntil(f); i < 0 || i >= c.visible {
		c.start = c.alignStart(f)
	}
}

// alignStart is the first month of the window that shows f, kept inside
// Min and Max where there are enough months.
func (c *cal) alignStart(f Date) Date {
	s := f.firstOfMonth()
	if !c.max.IsZero() {
		if hi := c.max.firstOfMonth().AddMonths(1 - c.visible); s.After(hi) {
			s = hi
		}
	}
	if !c.min.IsZero() {
		if lo := c.min.firstOfMonth(); s.Before(lo) {
			s = lo
		}
	}
	return s
}

// prune drops the refs of months that are not on display.
func (c *cal) prune() {
	end := c.start.AddMonths(c.visible)
	for k := range c.refs {
		if k.month.Before(c.start) || !k.month.Before(end) {
			delete(c.refs, k)
		}
	}
	for m := range c.heads {
		if m.Before(c.start) || !m.Before(end) {
			delete(c.heads, m)
		}
	}
	if c.hasPending && (c.pending != c.focused() || c.start.monthsUntil(c.pending) >= c.visible) {
		c.hasPending = false // refused, or gone
	}
}

func (c *cal) month(i int) Date { return c.start.AddMonths(i) }

func (c *cal) cellRef(month, d Date) tela.ElementRef {
	k := cellKey{month, d}
	r, ok := c.refs[k]
	if !ok {
		r = core.NewElementRef()
		c.refs[k] = r
	}
	return r
}

func (c *cal) headRef(month Date) tela.ElementRef {
	r, ok := c.heads[month]
	if !ok {
		r = core.NewElementRef()
		c.heads[month] = r
	}
	return r
}

// utc is d as the time the formatter reads.
func utc(d Date) time.Time { return d.Time(time.UTC) }

func (c *cal) title() string {
	first := c.f.MonthYear(utc(c.start))
	if c.visible == 1 {
		return first
	}
	return first + " – " + c.f.MonthYear(utc(c.start.AddMonths(c.visible-1)))
}

// --- state queries

func (c *cal) isDisabled(d Date) bool {
	return c.s.disabled || !c.min.IsZero() && d.Before(c.min) || !c.max.IsZero() && d.After(c.max)
}

func (c *cal) isUnavailable(d Date) bool {
	if c.s.unavailable != nil && c.s.unavailable(d) {
		return true
	}
	if c.hasAnchor {
		return !c.winLo.IsZero() && d.Before(c.winLo) || !c.winHi.IsZero() && d.After(c.winHi)
	}
	return false
}

// highlight is the range on show: the one being chosen (the anchor to the
// focused date) or the value; a single date is a range of one day.
func (c *cal) highlight() DateRange {
	if c.rng && c.hasAnchor {
		return orderedRange(c.anchor, c.focused())
	}
	r := c.val.get()
	if r.Start.IsZero() {
		return DateRange{}
	}
	return r
}

// --- moves

// setFocused makes d the focused date. When the focus is inside the
// calendar it stays with the date: the cell of d may be a new element (the
// months on display changed) that takes the DOM focus once it is rendered.
// A controlled calendar learns the date at its next build, so the cell
// waits for it there (and nothing happens if the caller refuses the date).
func (c *cal) setFocused(d Date) {
	d = c.clamp(d)
	if d == c.focused() {
		return
	}
	c.foc.request(d)
	if c.focused() == d {
		c.sync()
	}
	if c.hasFocus {
		c.pending, c.hasPending = d, true
	}
	c.invalidate()
}

// moveTo moves the focused date and the DOM focus with it (the keys).
func (c *cal) moveTo(t Date) {
	t = c.clamp(t)
	c.foc.request(t)
	if c.focused() == t {
		c.sync()
		ref := c.cellRef(t.firstOfMonth(), t)
		ref.Focus()
		if ref.Focused() {
			c.hasPending = false
			c.invalidate()
			return
		}
	}
	// Not rendered yet, or the date is not known until the caller
	// stores it: the cell takes the focus when the build has it.
	c.pending, c.hasPending = t, true
	c.invalidate()
}

// page moves the months on display by dir pages of VisibleMonths and the
// focused date with them (the month buttons).
func (c *cal) page(dir int) {
	if dir < 0 && c.prevDisabled() || dir > 0 && c.nextDisabled() {
		return
	}
	ns := c.start.AddMonths(dir * c.visible)
	t := c.focused().AddMonths(dir * c.visible)
	if end := ns.AddMonths(c.visible).AddDays(-1); t.Before(ns) {
		t = ns
	} else if t.After(end) {
		t = end
	}
	t = c.clamp(t)
	c.start = ns
	c.foc.request(t)
	c.invalidate()
}

func (c *cal) prevDisabled() bool {
	return c.s.disabled || !c.min.IsZero() && c.start.AddDays(-1).Before(c.min)
}

func (c *cal) nextDisabled() bool {
	return c.s.disabled || !c.max.IsZero() && c.start.AddMonths(c.visible).After(c.max)
}

// weekStart is the first day of the week that holds d.
func (c *cal) weekStart(d Date) Date {
	return d.AddDays(-((int(d.Weekday()) - int(c.firstDay) + 7) % 7))
}

// press is a press, a tap or Enter on the cell of d: the date takes the
// focus, and is selected unless it can not be.
func (c *cal) press(d Date) {
	if c.isDisabled(d) {
		return
	}
	c.setFocused(d)
	if c.s.readOnly || c.isUnavailable(d) {
		return
	}
	c.selectDate(d)
}

func (c *cal) selectDate(d Date) {
	if !c.rng {
		c.val.set(DateRange{d, d})
		return
	}
	if !c.hasAnchor {
		c.anchor, c.hasAnchor = d, true
		c.computeWindow()
		c.invalidate()
		return
	}
	r := orderedRange(c.anchor, d)
	c.hasAnchor = false
	c.invalidate()
	c.val.set(r)
}

// scanDays is how far from the anchor the nearest unavailable date is
// searched.
const scanDays = 366 * 3

// computeWindow finds the dates that may complete a range from the
// anchor: those before the nearest unavailable date on each side
// (react-stately's nextUnavailableDate and previousAvailableDate).
func (c *cal) computeWindow() {
	c.winLo, c.winHi = Date{}, Date{}
	if c.s.unavailable == nil || c.s.nonContiguous {
		return
	}
	scan := func(step int) Date {
		for i := 1; i <= scanDays; i++ {
			d := c.anchor.AddDays(step * i)
			if step < 0 && !c.min.IsZero() && d.Before(c.min) || step > 0 && !c.max.IsZero() && d.After(c.max) {
				return Date{}
			}
			if c.s.unavailable(d) {
				return d.AddDays(-step)
			}
		}
		return c.anchor.AddDays(step * scanDays)
	}
	c.winLo, c.winHi = scan(-1), scan(1)
}

func (c *cal) onKeyDown(month, d Date, e tela.KeyboardEvent) {
	if !c.cellRef(month, d).Focused() || e.Ctrl || e.Meta || e.Alt {
		return
	}
	step := 0
	switch e.Key {
	case tela.KeyArrowRight:
		step = 1
	case tela.KeyArrowLeft:
		step = -1
	}
	if c.rtl {
		step = -step
	}
	switch {
	case step != 0:
		e.PreventDefault()
		e.StopPropagation()
		c.moveTo(d.AddDays(step))
	case e.Key == tela.KeyArrowDown:
		e.PreventDefault()
		e.StopPropagation()
		c.moveTo(d.AddDays(7))
	case e.Key == tela.KeyArrowUp:
		e.PreventDefault()
		e.StopPropagation()
		c.moveTo(d.AddDays(-7))
	case e.Key == tela.KeyPageDown, e.Key == tela.KeyPageUp:
		e.PreventDefault()
		e.StopPropagation()
		n := 1
		if e.Key == tela.KeyPageUp {
			n = -1
		}
		if e.Shift {
			c.moveTo(d.AddYears(n))
		} else {
			c.moveTo(d.AddMonths(n))
		}
	case e.Key == tela.KeyHome:
		e.PreventDefault()
		e.StopPropagation()
		c.moveTo(c.weekStart(d))
	case e.Key == tela.KeyEnd:
		e.PreventDefault()
		e.StopPropagation()
		c.moveTo(c.weekStart(d).AddDays(6))
	case e.Key == tela.KeyEnter, e.Key == " ":
		// A Button would click on Enter and Space too: the default is
		// the key's, so it is prevented.
		e.PreventDefault()
		e.StopPropagation()
		if !e.Repeat {
			c.press(d)
		}
	}
}

// --- the result

// Months returns the number of months on display.
func (r CalendarResult) Months() int { return r.c.visible }

// Month returns the first day of month i (0 to Months()-1) on display.
func (r CalendarResult) Month(i int) Date { return r.c.month(i) }

// Heading returns the title of month i, month and year in the locale's
// order and form ("October 2026", "octubre de 2026", "2026年10月").
func (r CalendarResult) Heading(i int) string { return r.c.f.MonthYear(utc(r.c.month(i))) }

// HeadingProps go to the element that shows [CalendarResult.Heading] of
// month i (an H2): the grid of that month is named by it.
func (r CalendarResult) HeadingProps(i int) tela.Prop {
	return h.Ref(r.c.headRef(r.c.month(i)))
}

// GridProps returns the props of the grid of month i: the "grid" role,
// named by its heading, aria-readonly and, for a range calendar,
// aria-multiselectable.
func (r CalendarResult) GridProps(i int) tela.Prop {
	c := r.c
	ps := []tela.Prop{h.Role("grid"), h.AriaLabelledBy(c.headRef(c.month(i)))}
	if c.s.readOnly {
		ps = append(ps, h.AriaReadOnly(true))
	}
	if c.s.disabled {
		ps = append(ps, h.AriaDisabled(true))
	}
	if c.rng {
		ps = append(ps, h.AriaMultiSelectable(true))
	}
	return core.MergeProps(ps...)
}

// RowProps returns the props of a row of the grid: the weekday headers
// and each week.
func (r CalendarResult) RowProps() tela.Prop { return h.Role("row") }

// WeekdayNames returns the names of the seven weekdays in the locale at
// the given width (i18n.NameShort: "Mo", i18n.NameNarrow: "M"), in the
// order of the columns: the first day of the week first.
func (r CalendarResult) WeekdayNames(width i18n.NameWidth) []string {
	names := make([]string, 7)
	for i := range names {
		names[i] = r.c.f.WeekdayName(time.Weekday((int(r.c.firstDay)+i)%7), width)
	}
	return names
}

// ColumnHeaderProps returns the props of the header of column i (0 to 6):
// the "columnheader" role, named by the full weekday ("Monday") whatever
// the short text it shows.
func (r CalendarResult) ColumnHeaderProps(i int) tela.Prop {
	day := time.Weekday((int(r.c.firstDay) + i) % 7)
	return core.MergeProps(h.Role("columnheader"), h.AriaLabel(r.c.f.WeekdayName(day, i18n.NameWide)))
}

// FirstDayOfWeek returns the first column's weekday.
func (r CalendarResult) FirstDayOfWeek() time.Weekday { return r.c.firstDay }

// Weeks returns the weeks of month i: rows of seven dates starting on the
// first day of the week, as many rows as the month needs (four to six), or
// six with FixedWeeks. The first and last rows hold dates of the
// neighbouring months, which [CalendarResult.IsOutsideMonth] reports.
func (r CalendarResult) Weeks(i int) [][]Date {
	c := r.c
	first := c.month(i)
	offset := (int(first.Weekday()) - int(c.firstDay) + 7) % 7
	rows := (offset + daysIn(first.Year, first.Month) + 6) / 7
	if c.s.fixedWeeks {
		rows = 6
	}
	day := first.AddDays(-offset)
	weeks := make([][]Date, rows)
	for w := range weeks {
		weeks[w] = make([]Date, 7)
		for d := range weeks[w] {
			weeks[w][d] = day
			day = day.AddDays(1)
		}
	}
	return weeks
}

// DayText returns the day of the month of d as the locale writes numbers
// ("5", "٥").
func (r CalendarResult) DayText(d Date) string { return r.c.f.Number(float64(d.Day)) }

// CellProps returns the props of the cell of date d in the grid of month i
// (React Aria's useCalendarCell): the "gridcell" role, a name that is the
// full localized date ("Monday, October 5, 2026"), aria-selected, the
// disabled state, the ref, the tab stop (the focused date's cell in its
// own month only) and the handlers: a click or tap selects, the keys of
// [UseCalendar] move the focus, and a pointer over a cell previews the
// range being chosen. A date outside Min and Max or a disabled calendar
// has the Disabled prop (it can not be focused); an unavailable one,
// aria-disabled only. Render the cell with a Button and give it
// Key(d.String()): the cell of the focused date is a new element when the
// month changes and takes the focus when it mounts.
func (r CalendarResult) CellProps(i int, d Date) tela.Prop {
	c := r.c
	month := c.month(i)
	ref := c.cellRef(month, d)
	outside := d.Year != month.Year || d.Month != month.Month
	ps := []tela.Prop{
		h.Ref(ref), h.Role("gridcell"), h.AriaLabel(c.f.Date(utc(d), i18n.Full)),
		h.AriaSelected(c.highlight().Contains(d)),
		// A cell in a Form is not its default button.
		core.DefaultButtonType("button"),
	}
	if c.isDisabled(d) {
		return core.MergeProps(append(ps, h.Disabled(true), h.AriaDisabled(true))...)
	}
	if c.isUnavailable(d) {
		ps = append(ps, h.AriaDisabled(true))
	}
	if !outside && d == c.focused() {
		ps = append(ps, h.TabIndex(0))
		if c.hasPending && c.pending == d {
			ps = append(ps, h.AutoFocus(true))
		}
	} else {
		ps = append(ps, h.TabIndex(-1))
	}
	ps = append(ps,
		h.OnClick(func(e tela.MouseEvent) {
			if e.Button == tela.MouseButtonLeft {
				c.press(d)
			}
		}),
		h.OnKeyDown(func(e tela.KeyboardEvent) { c.onKeyDown(month, d, e) }),
		h.OnFocus(func() {
			if !c.hasFocus {
				c.hasFocus = true
				c.invalidate()
			}
			if ref.Focused() && c.hasPending && c.pending == d {
				c.hasPending = false
			}
			// A date of a neighbouring month does not move the months
			// on display until it is pressed: the press that focuses it
			// would otherwise replace the cell under the pointer before
			// its click.
			if !outside {
				c.setFocused(d)
			}
		}),
		h.OnBlur(func() {
			if c.hasFocus {
				c.hasFocus = false
				c.invalidate()
			}
		}),
		h.OnPointerEnter(func() {
			// Previewing a range: the pointer over a date of the month
			// moves the end (the month on display does not change).
			if c.hasAnchor && !outside && !c.isDisabled(d) && c.focused() != d {
				c.foc.request(d)
			}
		}),
	)
	return core.MergeProps(ps...)
}

// IsSelected reports whether d is selected: the selected date, or a date
// of the selected range (while a range is being chosen, of the range
// between its anchor and the focused date).
func (r CalendarResult) IsSelected(d Date) bool { return r.c.highlight().Contains(d) }

// IsInRange is [CalendarResult.IsSelected] for a range calendar: whether d
// is in the range, its ends included; it is false for a single-date
// calendar. Style the ends with IsRangeStart and IsRangeEnd.
func (r CalendarResult) IsInRange(d Date) bool { return r.c.rng && r.c.highlight().Contains(d) }

// IsRangeStart reports whether d is the first date of the range.
func (r CalendarResult) IsRangeStart(d Date) bool {
	return r.c.rng && !r.c.highlight().IsZero() && r.c.highlight().Start == d
}

// IsRangeEnd reports whether d is the last date of the range.
func (r CalendarResult) IsRangeEnd(d Date) bool {
	return r.c.rng && !r.c.highlight().IsZero() && r.c.highlight().End == d
}

// Anchor returns the first date of a range being chosen, and whether
// there is one.
func (r CalendarResult) Anchor() (Date, bool) { return r.c.anchor, r.c.hasAnchor }

// IsToday reports whether d is today (the Today option, else the current
// day).
func (r CalendarResult) IsToday(d Date) bool { return d == r.c.today }

// IsOutsideMonth reports whether d is not a day of month i on display: a
// date of the neighbouring months that fills the first or last week.
func (r CalendarResult) IsOutsideMonth(i int, d Date) bool {
	m := r.c.month(i)
	return d.Year != m.Year || d.Month != m.Month
}

// IsUnavailable reports whether d can not be selected: the caller's
// IsDateUnavailable, or, while a range is being chosen, a date beyond the
// nearest unavailable one.
func (r CalendarResult) IsUnavailable(d Date) bool { return r.c.isUnavailable(d) }

// IsDisabled reports whether d can not be focused: the calendar is
// Disabled, or d is before Min or after Max.
func (r CalendarResult) IsDisabled(d Date) bool { return r.c.isDisabled(d) }

// IsFocused reports whether d is the focused date and the focus is inside
// the calendar.
func (r CalendarResult) IsFocused(d Date) bool { return r.c.hasFocus && r.c.focused() == d }

// PrevProps returns the props of the button that shows the previous
// month, or the previous VisibleMonths months (aria-label "Previous"): a
// click moves the months and the focused date back. It is disabled at
// Min. Give it to a Button.
func (r CalendarResult) PrevProps() tela.Prop { return r.navProps(-1) }

// NextProps is [CalendarResult.PrevProps] for the next month, disabled at
// Max (aria-label "Next").
func (r CalendarResult) NextProps() tela.Prop { return r.navProps(1) }

// PrevDisabled reports whether the previous button is disabled.
func (r CalendarResult) PrevDisabled() bool { return r.c.prevDisabled() }

// NextDisabled reports whether the next button is disabled.
func (r CalendarResult) NextDisabled() bool { return r.c.nextDisabled() }

func (r CalendarResult) navProps(dir int) tela.Prop {
	c := r.c
	label, disabled := c.s.prevLabel, c.prevDisabled()
	if label == "" {
		label = "Previous"
	}
	if dir > 0 {
		label, disabled = c.s.nextLabel, c.nextDisabled()
		if label == "" {
			label = "Next"
		}
	}
	return core.MergeProps(h.AriaLabel(strings.TrimSpace(label)), h.Disabled(disabled),
		core.DefaultButtonType("button"),
		h.OnClick(func(e tela.MouseEvent) {
			if e.Button == tela.MouseButtonLeft {
				c.page(dir)
			}
		}))
}
