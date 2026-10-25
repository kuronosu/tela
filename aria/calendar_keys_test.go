package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/telatest"
)

// key presses a key and expects the focus on a date.
func expectKeys(t *testing.T, app *telatest.App, steps ...any) {
	t.Helper()
	for i := 0; i < len(steps); i += 2 {
		key, want := steps[i].(string), steps[i+1].(aria.Date)
		app.Press(key)
		app.ExpectFocused(cell(want))
	}
}

// clickDisabled presses where an element is, as a user would: telatest's
// Click refuses an element that is disabled.
func clickDisabled(app *telatest.App, l telatest.Locator) {
	b := app.Bounds(l)
	app.ClickAt(b.X+b.Width/2, b.Y+b.Height/2)
}

func TestCalendarTabStopIsTheFocusedDate(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultValue: new(d(2026, time.October, 5)),
	}), telatest.Size(400, 360))
	// Previous, next, then the grid: one tab stop, on the selected date.
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Previous"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Next"))
	app.Press("Tab")
	app.ExpectFocused(cell(d(2026, time.October, 5)))
	app.Press("Tab")                                                // the grid is one tab stop
	app.ExpectFocused(telatest.ByRole("button", "Previous").Nth(0)) // wraps to the page start
	app.Press("Shift+Tab")
	app.ExpectFocused(cell(d(2026, time.October, 5)))
}

func TestCalendarTabStopWithoutValueIsToday(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{}), telatest.Size(400, 360))
	app.Press("Tab")
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(cell(calToday))
	// DefaultFocusedDate decides when there is no value.
	app = telatest.MountFunc(t, calendarPage(aria.CalendarOptions{DefaultFocusedDate: d(2027, time.March, 9)}), telatest.Size(400, 360))
	app.ExpectRole("heading", "March 2027")
	app.Focus(telatest.ByRole("button", "Next"))
	app.Press("Tab")
	app.ExpectFocused(cell(d(2027, time.March, 9)))
}

func TestCalendarArrowKeys(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultValue: new(d(2026, time.October, 28)),
	}), telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 28)))
	expectKeys(t, app,
		"ArrowRight", d(2026, time.October, 29),
		"ArrowDown", d(2026, time.November, 5), // a week, over the month boundary
		"ArrowLeft", d(2026, time.November, 4),
		"ArrowUp", d(2026, time.October, 28),
		"ArrowLeft", d(2026, time.October, 27),
	)
	app.ExpectRole("heading", "October 2026")
	// The focus can go over the end of the year.
	app.Focus(cell(d(2026, time.October, 27)))
	// (The focus lands in the next frame, so a key waits for it.)
	expectKeys(t, app, "PageDown", d(2026, time.November, 27), "PageDown", d(2026, time.December, 27))
	app.ExpectRole("heading", "December 2026")
	expectKeys(t, app,
		"ArrowDown", d(2027, time.January, 3),
		"ArrowUp", d(2026, time.December, 27),
	)
	app.ExpectRole("heading", "December 2026")
	app.Press("ArrowDown")
	app.ExpectRole("heading", "January 2027")
}

func TestCalendarMonthBoundaryRows(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{DefaultFocusedDate: d(2026, time.October, 1)}), telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 1)))
	// Back over the start of the month: the previous month takes over.
	app.Press("ArrowLeft")
	app.ExpectFocused(cell(d(2026, time.September, 30)))
	app.ExpectRole("heading", "September 2026")
	app.Press("ArrowRight")
	app.ExpectFocused(cell(d(2026, time.October, 1)))
	app.ExpectRole("heading", "October 2026")
	app.Press("ArrowUp")
	app.ExpectFocused(cell(d(2026, time.September, 24)))
	app.ExpectRole("heading", "September 2026")
}

func TestCalendarPageKeys(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultFocusedDate: d(2026, time.October, 31),
	}), telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 31)))
	expectKeys(t, app,
		"PageDown", d(2026, time.November, 30), // the day is clamped to the shorter month
		"PageDown", d(2026, time.December, 30),
		"Shift+PageDown", d(2027, time.December, 30),
		"Shift+PageUp", d(2026, time.December, 30),
		"PageUp", d(2026, time.November, 30),
		"PageUp", d(2026, time.October, 30),
	)
	app.ExpectRole("heading", "October 2026")
	app.Press("Shift+PageUp")
	app.ExpectRole("heading", "October 2025")
}

func TestCalendarLeapYears(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultFocusedDate: d(2028, time.January, 31),
	}), telatest.Size(400, 360))
	app.Focus(cell(d(2028, time.January, 31)))
	expectKeys(t, app,
		"PageDown", d(2028, time.February, 29), // 2028 is a leap year
		"Shift+PageDown", d(2029, time.February, 28), // 2029 is not
		"Shift+PageUp", d(2028, time.February, 28),
		"ArrowRight", d(2028, time.February, 29),
		"ArrowRight", d(2028, time.March, 1),
		"ArrowLeft", d(2028, time.February, 29),
		"PageUp", d(2028, time.January, 29),
	)
	// Feb 29 plus a year is Feb 28; a century year is not a leap year.
	if got := d(2028, time.February, 29).AddYears(1); got != d(2029, time.February, 28) {
		t.Errorf("Feb 29 2028 + 1 year = %v", got)
	}
	if got := d(2100, time.January, 31).AddMonths(1); got != d(2100, time.February, 28) {
		t.Errorf("Jan 31 2100 + 1 month = %v", got)
	}
}

func TestCalendarHomeEnd(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultValue: new(d(2026, time.October, 7)), // a Wednesday
	}), telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 7)))
	// Sunday first: the week of the 7th is the 4th to the 10th.
	expectKeys(t, app,
		"Home", d(2026, time.October, 4),
		"End", d(2026, time.October, 10),
		"Home", d(2026, time.October, 4),
	)
	// The first week of the month starts in the previous one.
	app.Focus(cell(d(2026, time.October, 2)))
	app.Press("Home")
	app.ExpectFocused(cell(d(2026, time.September, 27)))
	app.ExpectRole("heading", "September 2026")
	// Monday first (the option, or de-DE's locale): the week of the 7th is
	// the 5th to the 11th.
	app = telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultValue: new(d(2026, time.October, 7)), FirstDayOfWeek: new(time.Monday),
	}), telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 7)))
	expectKeys(t, app, "Home", d(2026, time.October, 5), "End", d(2026, time.October, 11))
}

func TestCalendarSelectsWithEnterAndSpace(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		OnValueChange: func(v aria.Date) { ev.add(v.String()) },
		OnFocusChange: func(v aria.Date) { ev.add("focus:" + v.String()) },
	}), telatest.Size(400, 360))
	app.Focus(cell(calToday))
	app.Press("ArrowRight")
	app.Press("Enter")
	app.Expect(cell(d(2026, time.October, 26)), telatest.Selected(true))
	app.Press("ArrowRight")
	app.Press("Space")
	app.Expect(cell(d(2026, time.October, 27)), telatest.Selected(true))
	app.Expect(cell(d(2026, time.October, 26)), telatest.Selected(false))
	// Enter on the selected date reports nothing.
	app.Press("Enter")
	got := ev.take()
	want := "focus:2026-10-26 2026-10-26 focus:2026-10-27 2026-10-27"
	if got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestCalendarStopsAtMinAndMax(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		Min: d(2026, time.October, 10), Max: d(2026, time.October, 20),
		DefaultValue: new(d(2026, time.October, 12)),
	}), telatest.Size(400, 360))
	app.Expect(cell(d(2026, time.October, 9)), telatest.Disabled(true))
	app.Expect(cell(d(2026, time.October, 10)), telatest.Disabled(false))
	app.Expect(cell(d(2026, time.October, 20)), telatest.Disabled(false))
	app.Expect(cell(d(2026, time.October, 21)), telatest.Disabled(true))
	app.Expect(telatest.ByRole("button", "Previous"), telatest.Disabled(true))
	app.Expect(telatest.ByRole("button", "Next"), telatest.Disabled(true))
	app.Focus(cell(d(2026, time.October, 12)))
	expectKeys(t, app,
		"ArrowLeft", d(2026, time.October, 11),
		"ArrowLeft", d(2026, time.October, 10),
		"ArrowLeft", d(2026, time.October, 10), // the limit
		"ArrowUp", d(2026, time.October, 10),
		"Home", d(2026, time.October, 10),
		"PageUp", d(2026, time.October, 10),
		"PageDown", d(2026, time.October, 20), // a month on is past Max: the limit
		"ArrowDown", d(2026, time.October, 20),
		"End", d(2026, time.October, 20),
		"Shift+PageUp", d(2026, time.October, 10),
	)
	app.ExpectRole("heading", "October 2026")
	// A disabled cell can not be clicked.
	clickDisabled(app, cell(d(2026, time.October, 21)))
	app.Expect(cell(d(2026, time.October, 21)), telatest.Selected(false))
}

func TestCalendarLimitsAcrossMonths(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		Min: d(2026, time.September, 15), Max: d(2026, time.November, 15),
	}), telatest.Size(400, 360))
	app.Expect(telatest.ByRole("button", "Previous"), telatest.Disabled(false))
	app.Expect(telatest.ByRole("button", "Next"), telatest.Disabled(false))
	app.Click(telatest.ByRole("button", "Next"))
	app.ExpectRole("heading", "November 2026")
	app.Expect(telatest.ByRole("button", "Next"), telatest.Disabled(true))
	app.Click(telatest.ByRole("button", "Previous"))
	app.Click(telatest.ByRole("button", "Previous"))
	app.ExpectRole("heading", "September 2026")
	app.Expect(telatest.ByRole("button", "Previous"), telatest.Disabled(true))
	// The pages move the focused date by a month and the limits clamp it:
	// Nov 25 became the 15th (Max), then Oct 15 and Sep 15 (Min).
	app.Focus(telatest.ByRole("button", "Next"))
	app.Press("Tab")
	app.ExpectFocused(cell(d(2026, time.September, 15)))
}

func TestCalendarUnavailableDatesCanBeFocusedNotSelected(t *testing.T) {
	var ev events
	booked := func(day aria.Date) bool { return day.Day == 13 || day.Weekday() == time.Sunday }
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		IsDateUnavailable: booked, OnValueChange: func(v aria.Date) { ev.add(v.String()) },
		DefaultFocusedDate: d(2026, time.October, 12),
	}), telatest.Size(400, 360))
	app.Expect(cell(d(2026, time.October, 13)), telatest.Disabled(true))
	app.Expect(cell(d(2026, time.October, 11)), telatest.Disabled(true))
	app.Expect(cell(d(2026, time.October, 12)), telatest.Disabled(false))
	app.Focus(cell(d(2026, time.October, 12)))
	app.Press("ArrowRight") // focusable, as in React Aria
	app.ExpectFocused(cell(d(2026, time.October, 13)))
	app.Press("Enter")
	app.Press("Space")
	clickDisabled(app, cell(d(2026, time.October, 13)))
	app.ExpectFocused(cell(d(2026, time.October, 13)))
	app.Press("ArrowRight")
	app.Press("Enter")
	if got := ev.take(); got != "2026-10-14" {
		t.Errorf("OnValueChange saw %q, want only the 14th", got)
	}
	app.Expect(cell(d(2026, time.October, 13)), telatest.Selected(false))
}

func TestCalendarReadOnlyAndDisabled(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		ReadOnly: true, OnValueChange: func(v aria.Date) { ev.add(v.String()) },
	}), telatest.Size(400, 360))
	app.Click(cell(d(2026, time.October, 12)))
	app.ExpectFocused(cell(d(2026, time.October, 12))) // still focusable
	app.Press("Enter")
	app.Press("ArrowRight")
	app.ExpectFocused(cell(d(2026, time.October, 13)))
	if got := ev.take(); got != "" {
		t.Errorf("a read-only calendar reported %q", got)
	}
	app = telatest.MountFunc(t, calendarPage(aria.CalendarOptions{Disabled: true}), telatest.Size(400, 360))
	app.Expect(cell(d(2026, time.October, 12)), telatest.Disabled(true))
	app.Expect(telatest.ByRole("button", "Next"), telatest.Disabled(true))
}
