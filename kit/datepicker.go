package kit

import (
	"cmp"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
)

// DatePickerStyle is the recipe of [DatePicker] and [DateRangePicker]
// (shadcn/ui's date picker pattern: an outline Button that opens a Popover
// that holds a Calendar). The root element is the trigger button; its look
// is the [ButtonStyle]'s outline variant plus these classes, and the
// calendar and the popover are styled by their own recipes ([CalendarStyle]
// and [PopoverStyle]).
type DatePickerStyle struct {
	// Root is added to the trigger button's classes: the width of the
	// button and the alignment of its icon and text.
	Root string
	// Range is added to Root for a [DateRangePicker]: its text is longer.
	Range string
	// Empty is added while no date is chosen, so the placeholder is muted.
	Empty string
	// Content is added to the popover's panel classes: shadcn's w-auto p-0.
	Content string
	// Calendar is added to the calendar's classes: it has no background of
	// its own inside the popover (shadcn's [[data-slot=popover-content]_&]:).
	Calendar string
}

func init() {
	defaultStyle.DatePicker = DatePickerStyle{
		Root:     "w-[240px] justify-start text-left font-normal",
		Range:    "w-[300px]",
		Empty:    "text-muted-foreground",
		Content:  "w-auto p-0",
		Calendar: "bg-transparent",
	}
}

// DatePicker is a button that shows a date and opens a calendar to choose
// it, shadcn/ui's date picker pattern: an outline [Button] with a calendar
// icon and the date, or a placeholder, that toggles a [Popover] holding a
// [Calendar]. Choosing a date closes the popover and puts the focus back on
// the button; Escape and a press outside close it without choosing.
//
//	day, setDay := tela.UseState(aria.Date{})
//	kit.DatePicker{Value: &day, OnValueChange: setDay.Set, Min: aria.Today()}
//
// The date follows the kit's value rule with the aria.Date type (the zero
// Date is none): Value (a pointer to your state) with OnValueChange controls
// it, DefaultValue starts an uncontrolled picker. The button shows the date
// as the component's locale writes it in its long form ("October 25, 2026");
// Format changes that. The focus moves into the calendar, onto the date,
// when the popover opens, and the calendar follows the picker's locale for
// its names, digits, first weekday and direction (Locale).
//
// Inside a [Field] the button takes the field's id, so the label points to
// it and names it, the field's description or error describes it, and an
// error marks it invalid.
//
// Limits (Min, Max), IsDateUnavailable, FirstDayOfWeek, FixedWeeks,
// HideOutsideDays and Today are those of [Calendar]; for several months or
// a range see [DateRangePicker]. There is no time of day and no typed date:
// the date is chosen in the calendar.
type DatePicker struct {
	// Value points to the chosen date of a controlled picker; nil leaves it
	// to the picker, which starts at DefaultValue. OnValueChange is called
	// with the date the user chooses.
	Value         *aria.Date
	DefaultValue  aria.Date
	OnValueChange func(aria.Date)
	// Placeholder is shown while no date is chosen; the default is "Pick a
	// date".
	Placeholder string
	// Format writes the chosen date on the button; nil is the formatter's
	// long date ("October 25, 2026").
	Format func(f i18n.Formatter, d aria.Date) string

	// Min, Max, IsDateUnavailable, VisibleMonths, FirstDayOfWeek,
	// FixedWeeks, HideOutsideDays, Today, Locale, Direction, PreviousLabel
	// and NextLabel are those of [Calendar].
	Min, Max                 aria.Date
	IsDateUnavailable        func(aria.Date) bool
	VisibleMonths            int
	FirstDayOfWeek           *time.Weekday
	FixedWeeks               bool
	HideOutsideDays          bool
	Today                    aria.Date
	Locale, Direction        string
	PreviousLabel, NextLabel string

	// Disabled makes the picker inert and dims it.
	Disabled bool
	// Invalid marks the picker as wrong: assistive technologies announce it
	// and the button's aria-invalid: classes (a red border and ring) apply.
	// A [Field] with an Error sets it.
	Invalid bool
	// ID is the button's id, for a [Label]'s For. Inside a [Field] it
	// defaults to the field's.
	ID string
	// AriaLabel names the picker, and the popover; inside a Field with a
	// Label that label is used. The chosen date is appended to the button's
	// name.
	AriaLabel string
	// Icon replaces the calendar icon of the button.
	Icon tela.Icon
	// Align is where the popover sits along the button: "start" (the
	// default), "center" or "end".
	Align string
	// KeepOpen leaves the popover open after a date is chosen.
	KeepOpen bool
	// Open points to the open state of a controlled popover; nil leaves it
	// to the picker, which starts at DefaultOpen. OnOpenChange is called with
	// the next state.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)

	// Class is added after the recipe's Root classes, to the button; the
	// other Class fields do the same for the popover's panel and the
	// calendar.
	Class         string
	PopoverClass  string
	CalendarClass string
	// Props are more props for the button element.
	Props tela.Child
}

// Build returns the button and its popover.
func (p DatePicker) Build() tela.Node {
	f := i18n.UseFormatter()
	if p.Locale != "" {
		f = i18n.For(p.Locale)
	}
	own, setOwn := tela.UseState(p.DefaultValue)
	value := p.Value
	if value == nil {
		value = &own
	}
	text := p.Placeholder
	if text == "" {
		text = "Pick a date"
	}
	if !value.IsZero() {
		text = formatDate(p.Format, f, *value)
	}
	return usePicker(pickerFrame{
		text: text, empty: value.IsZero(), invalid: p.Invalid, disabled: p.Disabled, id: p.ID, ariaLabel: p.AriaLabel,
		icon: p.Icon, align: p.Align, keepOpen: p.KeepOpen, open: p.Open, defaultOpen: p.DefaultOpen, onOpenChange: p.OnOpenChange,
		class: p.Class, popoverClass: p.PopoverClass, props: p.Props,
	}, func(done func(), label string) tela.Component {
		return Calendar{
			Value: value,
			OnValueChange: func(d aria.Date) {
				if p.Value == nil {
					setOwn.Set(d)
				}
				if p.OnValueChange != nil {
					p.OnValueChange(d)
				}
				done()
			},
			Min: p.Min, Max: p.Max, IsDateUnavailable: p.IsDateUnavailable, VisibleMonths: p.VisibleMonths,
			FirstDayOfWeek: p.FirstDayOfWeek, FixedWeeks: p.FixedWeeks, HideOutsideDays: p.HideOutsideDays, Today: p.Today,
			Locale: p.Locale, Direction: p.Direction, PreviousLabel: p.PreviousLabel, NextLabel: p.NextLabel,
			AriaLabel: label, Class: p.CalendarClass, autoFocus: true,
		}
	}, false)
}

// DateRangePicker is a [DatePicker] for a span of dates, shadcn/ui's date
// range picker: the button shows the two ends ("Oct 20, 2026 - Nov 5,
// 2026") and the popover holds a [RangeCalendar] of two months. The popover
// closes when the second date completes the range (or stays open with
// KeepOpen); the first Escape in the calendar cancels an anchored range, and
// the next closes the popover.
//
//	var stay aria.DateRange
//	kit.DateRangePicker{OnValueChange: func(r aria.DateRange) { stay = r }}
//
// The value follows the kit's rule with aria.DateRange. VisibleMonths
// defaults to 2 here, as in shadcn's demo; the other fields are those of
// [DatePicker] and [RangeCalendar].
type DateRangePicker struct {
	// Value points to the chosen range of a controlled picker; nil leaves it
	// to the picker, which starts at DefaultValue. OnValueChange is called
	// with the range the user completes.
	Value         *aria.DateRange
	DefaultValue  aria.DateRange
	OnValueChange func(aria.DateRange)
	// Placeholder is shown while no range is chosen; the default is "Pick a
	// date range".
	Placeholder string
	// Format writes the chosen range on the button; nil writes both ends in
	// the formatter's medium form joined by " - ".
	Format func(f i18n.Formatter, r aria.DateRange) string
	// AllowsNonContiguousRanges lets a range cross unavailable dates.
	AllowsNonContiguousRanges bool

	Min, Max                 aria.Date
	IsDateUnavailable        func(aria.Date) bool
	VisibleMonths            int
	FirstDayOfWeek           *time.Weekday
	FixedWeeks               bool
	HideOutsideDays          bool
	Today                    aria.Date
	Locale, Direction        string
	PreviousLabel, NextLabel string

	Disabled      bool
	Invalid       bool
	ID            string
	AriaLabel     string
	Icon          tela.Icon
	Align         string
	KeepOpen      bool
	Open          *bool
	DefaultOpen   bool
	OnOpenChange  func(open bool)
	Class         string
	PopoverClass  string
	CalendarClass string
	Props         tela.Child
}

// Build returns the button and its popover.
func (p DateRangePicker) Build() tela.Node {
	f := i18n.UseFormatter()
	if p.Locale != "" {
		f = i18n.For(p.Locale)
	}
	own, setOwn := tela.UseState(p.DefaultValue)
	value := p.Value
	if value == nil {
		value = &own
	}
	text := p.Placeholder
	if text == "" {
		text = "Pick a date range"
	}
	if !value.IsZero() {
		if p.Format != nil {
			text = p.Format(f, *value)
		} else {
			text = f.Date(value.Start.Time(nil), i18n.Medium) + " - " + f.Date(value.End.Time(nil), i18n.Medium)
		}
	}
	months := p.VisibleMonths
	if months == 0 {
		months = 2
	}
	return usePicker(pickerFrame{
		text: text, empty: value.IsZero(), invalid: p.Invalid, disabled: p.Disabled, id: p.ID, ariaLabel: p.AriaLabel,
		icon: p.Icon, align: p.Align, keepOpen: p.KeepOpen, open: p.Open, defaultOpen: p.DefaultOpen, onOpenChange: p.OnOpenChange,
		class: p.Class, popoverClass: p.PopoverClass, props: p.Props,
	}, func(done func(), label string) tela.Component {
		return RangeCalendar{
			Value: value,
			OnValueChange: func(r aria.DateRange) {
				if p.Value == nil {
					setOwn.Set(r)
				}
				if p.OnValueChange != nil {
					p.OnValueChange(r)
				}
				done()
			},
			AllowsNonContiguousRanges: p.AllowsNonContiguousRanges,
			Min:                       p.Min, Max: p.Max, IsDateUnavailable: p.IsDateUnavailable, VisibleMonths: months,
			FirstDayOfWeek: p.FirstDayOfWeek, FixedWeeks: p.FixedWeeks, HideOutsideDays: p.HideOutsideDays, Today: p.Today,
			Locale: p.Locale, Direction: p.Direction, PreviousLabel: p.PreviousLabel, NextLabel: p.NextLabel,
			AriaLabel: label, Class: p.CalendarClass, autoFocus: true,
		}
	}, true)
}

// formatDate writes d with format, or as the formatter's long date.
func formatDate(format func(i18n.Formatter, aria.Date) string, f i18n.Formatter, d aria.Date) string {
	if format != nil {
		return format(f, d)
	}
	return f.Date(d.Time(nil), i18n.Long)
}

// pickerFrame is what DatePicker and DateRangePicker tell their shared
// frame: the button's text and state, and the popover's.
type pickerFrame struct {
	text                     string
	empty, invalid, disabled bool
	id, ariaLabel            string
	icon                     tela.Icon
	align                    string
	keepOpen                 bool
	open                     *bool
	defaultOpen              bool
	onOpenChange             func(bool)
	class, popoverClass      string
	props                    tela.Child
}

// usePicker draws the trigger button and the popover of a date picker.
// calendar makes the calendar for the popover, given what closes the
// popover when a date is chosen and the name of the calendar. It is a hook.
func usePicker(c pickerFrame, calendar func(done func(), label string) tela.Component, rng bool) tela.Node {
	st := UseStyle()
	s, btn := &st.DatePicker, &st.Button
	fc := tela.UseContext(fieldCtx)

	own, setOwn := tela.UseState(c.defaultOpen)
	open := c.open
	if open == nil {
		open = &own
	}
	setOpen := func(v bool) {
		if c.open == nil {
			setOwn.Set(v)
		}
		if c.onOpenChange != nil {
			c.onOpenChange(v)
		}
	}
	done := func() {
		if !c.keepOpen {
			setOpen(false)
		}
	}

	label := cmp.Or(c.ariaLabel, fc.label)
	id := cmp.Or(c.id, fc.id)
	trigger := TriggerFunc(func(p tela.Prop) tela.Node {
		kids := []tela.Child{p, h.Type("button"),
			cls(btn.Root, pick("Button", "Variant", btn.Variants, Outline), pick("Button", "Size", btn.Sizes, Size("")), btn.HasIcon[""],
				s.Root, when(rng, s.Range), when(c.empty, s.Empty), c.class),
			c.props,
			icon(iconOr(c.icon, calendarIcon), btn.Icon, btn.IconSizes[""]),
			h.Span(h.Text(c.text)),
		}
		if label != "" {
			name := label
			if !c.empty {
				name += ", " + c.text
			}
			kids = append(kids, h.AriaLabel(name))
		}
		if id != "" {
			kids = append(kids, h.ID(id))
		}
		if fc.describedBy != (tela.ElementRef{}) {
			kids = append(kids, h.AriaDescribedBy(fc.describedBy))
		}
		if c.invalid || fc.invalid {
			kids = append(kids, h.AriaInvalid(true))
		}
		if c.disabled {
			kids = append(kids, h.Disabled(true))
		}
		return h.Button(kids...)
	})
	return h.Fragment(Popover{
		Trigger: trigger, Children: calendar(done, cmp.Or(label, "Choose date")),
		Align: cmp.Or(c.align, "start"), AriaLabel: cmp.Or(label, "Choose date"),
		Open: open, OnOpenChange: setOpen, Class: joinClass(s.Content, c.popoverClass),
	})
}
