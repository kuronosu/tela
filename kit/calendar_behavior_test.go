package kit_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The behavior of kit.Calendar and kit.RangeCalendar: the hooks of package
// aria are tested on their own, so these check what the kit adds: that the
// parts are wired (cells, buttons, headings), the value rule, the locale and
// the look of each state.

// calCell is the cell of a date, found by its full English name.
func calCell(day aria.Date) telatest.Locator {
	return telatest.ByRole("gridcell", day.Time(nil).Format("Monday, January 2, 2006"))
}

// pressDisabled presses where an element is, as a user would: telatest's
// Click refuses an element that is disabled.
func pressDisabled(app *telatest.App, l telatest.Locator) {
	b := app.Bounds(l)
	app.ClickAt(b.X+b.Width/2, b.Y+b.Height/2)
}

func calPage(c tela.Component) func() tela.Node {
	return func() tela.Node { return h.Div(h.Class("p-6"), c) }
}

func calMount(t *testing.T, c tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.MountFunc(t, calPage(c), append([]telatest.Option{telatest.Size(800, 460), telatest.Theme(kit.Theme())}, opts...)...)
}

func TestCalendarSelectsByClickAndKeyboard(t *testing.T) {
	var got []aria.Date
	app := calMount(t, kit.Calendar{Today: calToday, AriaLabel: "Birthday", OnValueChange: func(day aria.Date) { got = append(got, day) }})
	app.ExpectRole("application", "Birthday, October 2026")
	app.Click(calCell(calDate(2026, time.October, 14)))
	app.Expect(calCell(calDate(2026, time.October, 14)), telatest.Selected(true))
	// A date of the neighbouring month is a cell too: pressing it selects it
	// and shows its month.
	app.Click(calCell(calDate(2026, time.September, 30)))
	app.ExpectRole("heading", "September 2026")
	app.Expect(calCell(calDate(2026, time.September, 30)), telatest.Selected(true))
	app.ExpectCount(calCell(calDate(2026, time.October, 14)), 0)
	// The keyboard: the arrows move the focus (and the month), Enter and
	// Space select.
	app.Press("ArrowRight")
	app.ExpectRole("heading", "October 2026")
	app.Press("Enter")
	app.Press("ArrowDown")
	app.Press("Space")
	want := []aria.Date{calDate(2026, 10, 14), calDate(2026, 9, 30), calDate(2026, 10, 1), calDate(2026, 10, 8)}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("OnValueChange got %v, want %v", got, want)
	}
}

func TestCalendarMonthsByKeyboardAndButtons(t *testing.T) {
	app := calMount(t, kit.Calendar{Today: calToday, DefaultValue: calDate(2026, time.October, 31)})
	app.Focus(calCell(calDate(2026, time.October, 31)))
	app.Press("PageDown")
	app.ExpectRole("heading", "November 2026")
	app.ExpectFocused(calCell(calDate(2026, time.November, 30))) // the day is clamped
	app.Press("Shift+PageUp")
	app.ExpectRole("heading", "November 2025")
	app.ExpectFocused(calCell(calDate(2025, time.November, 30)))
	app.Click(telatest.ByRole("button", "Next"))
	app.ExpectRole("heading", "December 2025")
	app.Click(telatest.ByRole("button", "Previous"))
	app.Click(telatest.ByRole("button", "Previous"))
	app.ExpectRole("heading", "October 2025")
	// Home and End are the first and last day of the week (Sunday first).
	app.Focus(calCell(calDate(2025, time.October, 30)))
	app.Press("Home")
	app.ExpectFocused(calCell(calDate(2025, time.October, 26)))
	app.Press("End")
	app.ExpectFocused(calCell(calDate(2025, time.November, 1)))
}

func TestCalendarLimitsAndUnavailableDates(t *testing.T) {
	var got []aria.Date
	app := calMount(t, kit.Calendar{
		Today: calToday, DefaultValue: calDate(2026, time.October, 20),
		Min: calDate(2026, time.October, 8), Max: calDate(2026, time.November, 12),
		IsDateUnavailable: func(day aria.Date) bool { return day == calDate(2026, time.October, 15) },
		OnValueChange:     func(day aria.Date) { got = append(got, day) },
	})
	// Min ends the way back: the previous button is disabled and the dates
	// before Min can not be pressed.
	app.Expect(telatest.ByRole("button", "Previous"), telatest.Disabled(true))
	app.Expect(telatest.ByRole("button", "Next"), telatest.Disabled(false))
	app.Expect(calCell(calDate(2026, time.October, 7)), telatest.Disabled(true))
	// An unavailable date is shown dimmed, can be focused but not chosen.
	app.Focus(calCell(calDate(2026, time.October, 15)))
	app.ExpectFocused(calCell(calDate(2026, time.October, 15)))
	app.Press("Enter")
	pressDisabled(app, calCell(calDate(2026, time.October, 15)))
	if len(got) != 0 {
		t.Fatalf("an unavailable date was chosen: %v", got)
	}
	app.Click(calCell(calDate(2026, time.October, 16)))
	if fmt.Sprint(got) != "[2026-10-16]" {
		t.Errorf("OnValueChange got %v, want [2026-10-16]", got)
	}
	// Max ends the way forward.
	app.Click(telatest.ByRole("button", "Next"))
	app.Expect(telatest.ByRole("button", "Next"), telatest.Disabled(true))
	app.Expect(calCell(calDate(2026, time.November, 13)), telatest.Disabled(true))
	// Dates of a month the calendar can not show are not drawn at all.
	app.ExpectCount(telatest.ByRole("gridcell", "Sunday, December 6, 2026"), 0)
}

func TestCalendarControlledValueRule(t *testing.T) {
	// A controlled calendar shows what it is given and asks for changes: a
	// change the parent does not store is refused.
	chosen := calDate(2026, time.October, 3)
	var asked []aria.Date
	app := calMount(t, kit.Calendar{Today: calToday, Value: &chosen, OnValueChange: func(day aria.Date) { asked = append(asked, day) }})
	app.Expect(calCell(chosen), telatest.Selected(true))
	app.Click(calCell(calDate(2026, time.October, 9)))
	app.Expect(calCell(chosen), telatest.Selected(true))
	app.Expect(calCell(calDate(2026, time.October, 9)), telatest.Selected(false))
	if fmt.Sprint(asked) != "[2026-10-09]" {
		t.Errorf("OnValueChange got %v, want [2026-10-09]", asked)
	}
}

func TestCalendarDisabledAndReadOnly(t *testing.T) {
	var got []aria.Date
	app := calMount(t, kit.Calendar{Today: calToday, ReadOnly: true, OnValueChange: func(day aria.Date) { got = append(got, day) }})
	app.Click(calCell(calDate(2026, time.October, 9)))
	app.ExpectFocused(calCell(calDate(2026, time.October, 9)))
	if len(got) != 0 {
		t.Errorf("a read-only calendar chose %v", got)
	}
	app = calMount(t, kit.Calendar{Today: calToday, Disabled: true})
	app.Expect(calCell(calDate(2026, time.October, 9)), telatest.Disabled(true))
}

func TestCalendarVisibleMonthsHideRepeatedOutsideDays(t *testing.T) {
	app := calMount(t, kit.Calendar{Today: calToday, VisibleMonths: 2})
	app.ExpectCount(telatest.ByRole("grid", ""), 2)
	// Oct 31 (Saturday) is the last day of a row of the first month; the
	// first week of November starts on the 1st (a Sunday): its cells are
	// the month's own, and the dates of the other month are not repeated.
	app.ExpectCount(calCell(calDate(2026, time.October, 31)), 1)
	app.ExpectCount(calCell(calDate(2026, time.November, 1)), 1)
	app.ExpectRole("application", "October 2026 – November 2026")
}

func TestCalendarFirstWeekdayAndHideOutsideDays(t *testing.T) {
	mon := time.Monday
	app := calMount(t, kit.Calendar{Today: calToday, FirstDayOfWeek: &mon, HideOutsideDays: true})
	snap := app.AriaSnapshot()
	if i, j := strings.Index(snap, `columnheader "Monday"`), strings.Index(snap, `columnheader "Sunday"`); i < 0 || j < i {
		t.Errorf("the first column is not Monday:\n%s", snap)
	}
	app.ExpectCount(calCell(calDate(2026, time.September, 28)), 0)
	app.ExpectCount(calCell(calDate(2026, time.October, 1)), 1)
	app.ExpectCount(calCell(calDate(2026, time.November, 1)), 0)
}

func TestCalendarFixedWeeks(t *testing.T) {
	feb := calDate(2026, time.February, 10)
	four := calMount(t, kit.Calendar{Today: calToday, DefaultFocusedDate: feb})
	six := calMount(t, kit.Calendar{Today: calToday, DefaultFocusedDate: feb, FixedWeeks: true})
	hFour := four.Bounds(telatest.ByRole("application", "")).Height
	hSix := six.Bounds(telatest.ByRole("application", "")).Height
	if hSix-hFour != 2*40 {
		t.Errorf("fixed weeks add %v px of height, want 80 (two rows of 32 and a margin of 8)", hSix-hFour)
	}
}

func TestCalendarLocales(t *testing.T) {
	cases := []struct {
		locale, heading, first, digit string
		rtl                           bool
	}{
		{"es", "octubre de 2026", "lunes", "14", false},
		{"ja", "2026年10月", "日曜日", "14", false},
		{"ar-EG", "أكتوبر 2026", "السبت", "14", true},
		{"ar-EG-u-nu-arab", "أكتوبر ٢٠٢٦", "السبت", "١٤", true},
	}
	for _, c := range cases {
		t.Run(c.locale, func(t *testing.T) {
			app := calMount(t, kit.Calendar{Today: calToday, Locale: c.locale, DefaultValue: calDate(2026, time.October, 14)})
			app.ExpectRole("heading", c.heading)
			snap := app.AriaSnapshot()
			if i := strings.Index(snap, "columnheader"); !strings.HasPrefix(snap[i:], `columnheader "`+c.first+`"`) {
				t.Errorf("the first weekday is not %s:\n%s", c.first, snap[i:i+80])
			}
			if !strings.Contains(snap, `: "`+c.digit+`"`) && !strings.Contains(snap, ": "+c.digit+"\n") {
				t.Errorf("no cell with the day %q written in the locale:\n%s", c.digit, snap)
			}
			prev, next := app.Bounds(telatest.ByRole("button", "Previous")), app.Bounds(telatest.ByRole("button", "Next"))
			if c.rtl == (prev.X < next.X) {
				t.Errorf("the previous button is at %v and the next at %v: rtl is %v", prev.X, next.X, c.rtl)
			}
		})
	}
}

func TestCalendarRTLArrowKeys(t *testing.T) {
	// In a right-to-left calendar ArrowLeft moves forward, and the layout
	// is mirrored.
	app := calMount(t, kit.Calendar{Today: calToday, Direction: "rtl", DefaultValue: calDate(2026, time.October, 14)})
	app.Focus(calCell(calDate(2026, time.October, 14)))
	app.Press("ArrowLeft")
	app.ExpectFocused(calCell(calDate(2026, time.October, 15)))
	a, b := app.Bounds(calCell(calDate(2026, time.October, 14))), app.Bounds(calCell(calDate(2026, time.October, 15)))
	if b.X >= a.X {
		t.Errorf("the 15th is at x=%v and the 14th at x=%v: the row is not mirrored", b.X, a.X)
	}
}

func TestCalendarCustomLabelsAndIcons(t *testing.T) {
	app := calMount(t, kit.Calendar{Today: calToday, PreviousLabel: "Mes anterior", NextLabel: "Mes siguiente", AriaLabel: "Fecha"})
	app.ExpectRole("button", "Mes anterior")
	app.ExpectRole("button", "Mes siguiente")
	app.ExpectRole("application", "Fecha, October 2026")
}

func TestCalendarStyleAndClasses(t *testing.T) {
	s := kit.DefaultStyle()
	s.Calendar.Today = "ring-2 ring-primary"
	s.Calendar.DayButtonWithSelected = "bg-emerald-600 text-white"
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: tela.C(calPage(kit.Calendar{
			Today: calToday, DefaultValue: calDate(2026, time.October, 14), Class: "shadow-lg", DayButtonClass: "font-semibold",
		}))})
	}, telatest.Size(400, 360), telatest.Theme(kit.Theme()))
	for class, want := range map[string]int{"ring-primary": 1, "bg-emerald-600": 1, "shadow-lg": 1, "font-semibold": 35} {
		if n := app.Count(telatest.ByClass(class)); n != want {
			t.Errorf("%d elements have %s, want %d", n, class, want)
		}
	}
}

func TestRangeCalendarSelectsInTwoPresses(t *testing.T) {
	var got []aria.DateRange
	app := calMount(t, kit.RangeCalendar{Today: calToday, OnValueChange: func(r aria.DateRange) { got = append(got, r) }})
	app.Click(calCell(calDate(2026, time.October, 20)))
	if len(got) != 0 {
		t.Fatalf("the first press completed a range: %v", got)
	}
	// The pointer over a date previews the range.
	app.Hover(calCell(calDate(2026, time.October, 23)))
	for _, day := range []int{20, 21, 22, 23} {
		app.Expect(calCell(calDate(2026, time.October, day)), telatest.Selected(true))
	}
	app.Expect(calCell(calDate(2026, time.October, 24)), telatest.Selected(false))
	// The second press completes it, the earlier date first.
	app.Click(calCell(calDate(2026, time.October, 12)))
	if fmt.Sprint(got) != "[{2026-10-12 2026-10-20}]" {
		t.Errorf("OnValueChange got %v, want [{2026-10-12 2026-10-20}]", got)
	}
	app.Expect(calCell(calDate(2026, time.October, 16)), telatest.Selected(true))
	app.Expect(calCell(calDate(2026, time.October, 21)), telatest.Selected(false))
}

func TestRangeCalendarEscapeCancelsTheAnchor(t *testing.T) {
	var got []aria.DateRange
	app := calMount(t, kit.RangeCalendar{Today: calToday, OnValueChange: func(r aria.DateRange) { got = append(got, r) }})
	app.Focus(calCell(calDate(2026, time.October, 20)))
	app.Press("Enter")
	app.Press("ArrowRight")
	app.Expect(calCell(calDate(2026, time.October, 21)), telatest.Selected(true))
	app.Press("Escape")
	app.Expect(calCell(calDate(2026, time.October, 20)), telatest.Selected(false))
	app.Press("Enter") // a new first press
	app.Press("ArrowDown")
	app.Press("Enter")
	if fmt.Sprint(got) != "[{2026-10-21 2026-10-28}]" {
		t.Errorf("OnValueChange got %v, want [{2026-10-21 2026-10-28}]", got)
	}
}

func TestRangeCalendarUnavailableDatesLimitTheRange(t *testing.T) {
	var got []aria.DateRange
	booked := calDate(2026, time.October, 22)
	app := calMount(t, kit.RangeCalendar{Today: calToday, IsDateUnavailable: func(day aria.Date) bool { return day == booked },
		OnValueChange: func(r aria.DateRange) { got = append(got, r) }})
	app.Click(calCell(calDate(2026, time.October, 20)))
	// Beyond the booked date nothing can end the range...
	pressDisabled(app, calCell(calDate(2026, time.October, 25)))
	if len(got) != 0 {
		t.Fatalf("a range crossed an unavailable date: %v", got)
	}
	// ...before it, a date can.
	app.Click(calCell(calDate(2026, time.October, 21)))
	if fmt.Sprint(got) != "[{2026-10-20 2026-10-21}]" {
		t.Errorf("OnValueChange got %v, want [{2026-10-20 2026-10-21}]", got)
	}
	// With AllowsNonContiguousRanges it can cross.
	got = nil
	app = calMount(t, kit.RangeCalendar{Today: calToday, AllowsNonContiguousRanges: true,
		IsDateUnavailable: func(day aria.Date) bool { return day == booked },
		OnValueChange:     func(r aria.DateRange) { got = append(got, r) }})
	app.Click(calCell(calDate(2026, time.October, 20)))
	app.Click(calCell(calDate(2026, time.October, 25)))
	if fmt.Sprint(got) != "[{2026-10-20 2026-10-25}]" {
		t.Errorf("OnValueChange got %v, want the range across the booked date", got)
	}
}

func TestRangeCalendarControlled(t *testing.T) {
	r := aria.DateRange{Start: calDate(2026, time.October, 5), End: calDate(2026, time.October, 7)}
	app := calMount(t, kit.RangeCalendar{Today: calToday, Value: &r})
	for day := 5; day <= 7; day++ {
		app.Expect(calCell(calDate(2026, time.October, day)), telatest.Selected(true))
	}
	app.Expect(calCell(calDate(2026, time.October, 8)), telatest.Selected(false))
}

func TestCalendarIdle(t *testing.T) {
	expectIdle(t, calPage(kit.Calendar{Today: calToday, DefaultValue: calDate(2026, time.October, 14)}), nil)
	expectIdle(t, calPage(kit.RangeCalendar{Today: calToday, VisibleMonths: 2}), nil)
}
