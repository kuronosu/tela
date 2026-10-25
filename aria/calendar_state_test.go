package aria_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func TestCalendarClickSelectsAndFocuses(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		OnValueChange: func(v aria.Date) { ev.add(v.String()) },
	}), telatest.Size(400, 360))
	app.Click(cell(d(2026, time.October, 14)))
	app.Expect(cell(d(2026, time.October, 14)), telatest.Selected(true))
	app.ExpectFocused(cell(d(2026, time.October, 14)))
	// A second click on it changes nothing.
	app.Click(cell(d(2026, time.October, 14)))
	if got := ev.take(); got != "2026-10-14" {
		t.Errorf("OnValueChange saw %q", got)
	}
	// A day of the next month, shown in the last row, selects it and moves
	// the month, keeping the focus on the date.
	app.Click(cell(d(2026, time.October, 31)))
	app.Click(cell(d(2026, time.October, 31)))
	_ = ev.take()
	app.Click(cell(d(2026, time.September, 28)))
	app.ExpectRole("heading", "September 2026")
	app.ExpectFocused(cell(d(2026, time.September, 28)))
	app.Expect(cell(d(2026, time.September, 28)), telatest.Selected(true))
	if got := ev.take(); got != "2026-09-28" {
		t.Errorf("OnValueChange saw %q", got)
	}
	app.Press("ArrowRight")
	app.ExpectFocused(cell(d(2026, time.September, 29)))
}

func TestCalendarMonthButtonsMoveTheFocusedDate(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultValue:  d(2026, time.January, 31),
		OnFocusChange: func(v aria.Date) { ev.add(v.String()) },
	}), telatest.Size(400, 360))
	app.Click(telatest.ByRole("button", "Next"))
	app.ExpectRole("heading", "February 2026")
	app.Click(telatest.ByRole("button", "Next"))
	app.ExpectRole("heading", "March 2026")
	app.Click(telatest.ByRole("button", "Previous"))
	app.Click(telatest.ByRole("button", "Previous"))
	app.Click(telatest.ByRole("button", "Previous"))
	app.ExpectRole("heading", "December 2025")
	if got := ev.take(); got != "2026-02-28 2026-03-28 2026-02-28 2026-01-28 2025-12-28" {
		// The day is kept from the focused date, not from the selected one,
		// so the 31st became the 28th when February was shorter and stayed.
		t.Errorf("focused dates = %q", got)
	}
	// The selected date is still January 31, and Tab goes to the focused one.
	app.Focus(telatest.ByRole("button", "Next"))
	app.Press("Tab")
	app.ExpectFocused(cell(d(2025, time.December, 28)))
}

func TestCalendarControlledValue(t *testing.T) {
	var asked events
	refuse := true
	var set tela.Setter[aria.Date]
	app := telatest.MountFunc(t, func() tela.Node {
		day, setDay := tela.UseState(d(2026, time.October, 5))
		set = setDay
		cal := aria.UseCalendar(aria.CalendarOptions{
			Locale: "en-US", Today: calToday, Value: &day,
			OnValueChange: func(v aria.Date) {
				asked.add(v.String())
				if !refuse {
					setDay.Set(v)
				}
			},
		})
		return renderCalendar(cal)
	}, telatest.Size(400, 360))
	app.Click(cell(d(2026, time.October, 9)))
	// The calendar shows what the caller keeps; a change it does not store
	// is refused.
	app.Expect(cell(d(2026, time.October, 9)), telatest.Selected(false))
	app.Expect(cell(d(2026, time.October, 5)), telatest.Selected(true))
	refuse = false
	app.Click(cell(d(2026, time.October, 9)))
	app.Expect(cell(d(2026, time.October, 9)), telatest.Selected(true))
	app.Expect(cell(d(2026, time.October, 5)), telatest.Selected(false))
	if got := asked.take(); got != "2026-10-09 2026-10-09" {
		t.Errorf("asked %q", got)
	}
	// A value set from outside while the focus is elsewhere moves the
	// month on display to it.
	app.Focus(telatest.ByRole("button", "Next"))
	set.Set(d(2027, time.March, 3))
	app.ExpectRole("heading", "March 2027")
	app.Expect(cell(d(2027, time.March, 3)), telatest.Selected(true))
}

func TestCalendarControlledFocusedDate(t *testing.T) {
	var asked events
	app := telatest.MountFunc(t, func() tela.Node {
		focus, setFocus := tela.UseState(d(2026, time.October, 5))
		cal := aria.UseCalendar(aria.CalendarOptions{
			Locale: "en-US", Today: calToday, FocusedDate: &focus,
			OnFocusChange: func(v aria.Date) { asked.add(v.String()); setFocus.Set(v) },
		})
		return Div(
			Button(TestID("jump"), OnClick(func() { setFocus.Set(d(2027, time.June, 1)) }), Text("Jump")),
			renderCalendar(cal))
	}, telatest.Size(400, 400))
	app.Click(cell(d(2026, time.October, 9)))
	app.Press("ArrowDown")
	app.ExpectFocused(cell(d(2026, time.October, 16)))
	if got := asked.take(); got != "2026-10-09 2026-10-16" {
		t.Errorf("asked %q", got)
	}
	app.Click(telatest.ByTestID("jump"))
	app.ExpectRole("heading", "June 2027")
}

func TestCalendarFocusChangeIsRefusedWhenControlledAndNotStored(t *testing.T) {
	focus := d(2026, time.October, 5)
	app := telatest.MountFunc(t, func() tela.Node {
		return renderCalendar(aria.UseCalendar(aria.CalendarOptions{
			Locale: "en-US", Today: calToday, FocusedDate: &focus,
			OnFocusChange: func(aria.Date) {}, // asked, never stored
		}))
	}, telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 5)))
	app.Press("ArrowRight")
	app.Press("PageDown")
	app.ExpectFocused(cell(d(2026, time.October, 5)))
	app.ExpectRole("heading", "October 2026")
}

func TestCalendarMarksTodayAndOutsideDays(t *testing.T) {
	var res aria.CalendarResult
	telatest.MountFunc(t, func() tela.Node {
		res = aria.UseCalendar(aria.CalendarOptions{Locale: "en-US", Today: calToday, Min: d(2026, time.October, 3)})
		return renderCalendar(res)
	}, telatest.Size(400, 360))
	if !res.IsToday(calToday) || res.IsToday(calToday.AddDays(1)) {
		t.Error("IsToday")
	}
	if !res.IsOutsideMonth(0, d(2026, time.September, 30)) || res.IsOutsideMonth(0, d(2026, time.October, 1)) {
		t.Error("IsOutsideMonth")
	}
	if !res.IsDisabled(d(2026, time.October, 2)) || res.IsDisabled(d(2026, time.October, 3)) || res.IsUnavailable(d(2026, time.October, 2)) {
		t.Error("IsDisabled and IsUnavailable are different things")
	}
	if res.FocusedDate != calToday {
		t.Errorf("FocusedDate = %v", res.FocusedDate)
	}
	if res.PrevDisabled() != true || res.NextDisabled() != false {
		t.Errorf("PrevDisabled = %v, NextDisabled = %v", res.PrevDisabled(), res.NextDisabled())
	}
	// Today is the real current day by default.
	var def aria.CalendarResult
	telatest.MountFunc(t, func() tela.Node {
		def = aria.UseCalendar(aria.CalendarOptions{Locale: "en-US"})
		return renderCalendar(def)
	}, telatest.Size(400, 360))
	if !def.IsToday(aria.Today()) {
		t.Error("the default Today is aria.Today()")
	}
}

func TestCalendarFocusStateQuery(t *testing.T) {
	var res aria.CalendarResult
	app := telatest.MountFunc(t, func() tela.Node {
		res = aria.UseCalendar(aria.CalendarOptions{Locale: "en-US", Today: calToday})
		return renderCalendar(res)
	}, telatest.Size(400, 360))
	if res.IsFocused(calToday) {
		t.Error("focused before the focus is inside")
	}
	app.Focus(cell(calToday))
	app.WaitFor("IsFocused", func() bool { return res.IsFocused(calToday) })
	app.Press("ArrowRight")
	app.WaitFor("IsFocused moves", func() bool { return res.IsFocused(calToday.AddDays(1)) && !res.IsFocused(calToday) })
	app.Focus(telatest.ByRole("button", "Next"))
	app.WaitFor("IsFocused leaves", func() bool { return !res.IsFocused(calToday.AddDays(1)) })
}

func TestCalendarMistakesPanicInDevMode(t *testing.T) {
	for name, tt := range map[string]struct {
		o    aria.CalendarOptions
		want string
	}{
		"Min after Max":     {aria.CalendarOptions{Min: d(2026, 10, 10), Max: d(2026, 10, 1)}, "Min (2026-10-10) is after Max (2026-10-01)"},
		"too many months":   {aria.CalendarOptions{VisibleMonths: 13}, "VisibleMonths must be between 1 and 12"},
		"negative months":   {aria.CalendarOptions{VisibleMonths: -1}, "VisibleMonths must be between 1 and 12"},
		"an invalid limit":  {aria.CalendarOptions{Min: d(2026, 13, 1)}, "must be valid dates"},
		"a typo in the dir": {aria.CalendarOptions{Direction: "rlt"}, `Direction "rlt": unknown value; use one of "ltr", "rtl"`},
	} {
		func() {
			defer func() {
				r := recover()
				msg, _ := r.(string)
				if r == nil || !strings.Contains(msg, tt.want) {
					t.Errorf("%s: panic %v, want one containing %q", name, r, tt.want)
				}
			}()
			telatest.MountFunc(t, calendarPage(tt.o), telatest.Size(300, 300))
		}()
	}
}

func TestCalendarAccessibleNames(t *testing.T) {
	// The calendar is named by its label and the range on show; without a
	// label by the range alone.
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{}), telatest.Size(400, 360))
	app.ExpectRole("application", "October 2026")
	app = telatest.MountFunc(t, func() tela.Node {
		label := tela.UseElementRef()
		cal := aria.UseCalendar(aria.CalendarOptions{Locale: "en-US", Today: calToday, AriaLabelledBy: label})
		return Div(Span(Ref(label), Text("Check-in")), renderCalendar(cal))
	}, telatest.Size(400, 400))
	app.ExpectRole("application", "Check-in")
}
