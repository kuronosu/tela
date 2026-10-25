package aria_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/telatest"
)

// selectedDays lists the days of October 2026 that gridcells report as
// selected, by day number.
func selectedDays(t *testing.T, app *telatest.App) string {
	t.Helper()
	snap := app.AriaSnapshot()
	out := ""
	for day := 1; day <= 31; day++ {
		if strings.Contains(snap, fmt.Sprintf("gridcell %q [selected]", enDate(d(2026, time.October, day)))) {
			out += fmt.Sprintf(" %d", day)
		}
	}
	return out
}

func TestRangeCalendarAnchorsPreviewsAndCompletes(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, rangePage(aria.RangeCalendarOptions{
		OnValueChange: func(r aria.DateRange) { ev.add(r.Start.String() + ".." + r.End.String()) },
	}), telatest.Size(400, 360))
	if got := selectedDays(t, app); got != "" {
		t.Fatalf("selected before any click:%s", got)
	}
	// The first click anchors: nothing is reported yet.
	app.Click(cell(d(2026, time.October, 7)))
	if got := selectedDays(t, app); got != " 7" {
		t.Errorf("after the first click:%s, want only the anchor", got)
	}
	if got := ev.take(); got != "" {
		t.Errorf("the first click reported %q", got)
	}
	// Hovering previews the range up to the date under the pointer.
	app.Hover(cell(d(2026, time.October, 10)))
	if got := selectedDays(t, app); got != " 7 8 9 10" {
		t.Errorf("hovering the 10th:%s", got)
	}
	app.Hover(cell(d(2026, time.October, 5)))
	if got := selectedDays(t, app); got != " 5 6 7" {
		t.Errorf("hovering before the anchor:%s", got)
	}
	// The second click completes, earlier end first.
	app.Click(cell(d(2026, time.October, 5)))
	if got := ev.take(); got != "2026-10-05..2026-10-07" {
		t.Errorf("completed range = %q", got)
	}
	if got := selectedDays(t, app); got != " 5 6 7" {
		t.Errorf("after completing:%s", got)
	}
	// The pointer no longer previews: the range is done.
	app.Hover(cell(d(2026, time.October, 20)))
	if got := selectedDays(t, app); got != " 5 6 7" {
		t.Errorf("hovering after completing:%s", got)
	}
	// A new first click starts over, and shows only the anchor.
	app.Click(cell(d(2026, time.October, 15)))
	if got := selectedDays(t, app); got != " 15" {
		t.Errorf("a new anchor:%s", got)
	}
	app.Click(cell(d(2026, time.October, 15)))
	if got := ev.take(); got != "2026-10-15..2026-10-15" {
		t.Errorf("a one-day range = %q", got)
	}
}

func TestRangeCalendarKeyboard(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, rangePage(aria.RangeCalendarOptions{
		DefaultFocusedDate: d(2026, time.October, 12),
		OnValueChange:      func(r aria.DateRange) { ev.add(r.Start.String() + ".." + r.End.String()) },
	}), telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 12)))
	app.Press("Enter") // anchor
	expectKeys(t, app, "ArrowRight", d(2026, time.October, 13), "ArrowDown", d(2026, time.October, 20))
	// The focus previews the range.
	if got := selectedDays(t, app); got != " 12 13 14 15 16 17 18 19 20" {
		t.Errorf("previewing with the keyboard:%s", got)
	}
	// Escape cancels the anchor and the preview.
	app.Press("Escape")
	if got := selectedDays(t, app); got != "" {
		t.Errorf("after Escape:%s", got)
	}
	app.Press("Space") // anchors at the 20th
	expectKeys(t, app, "ArrowLeft", d(2026, time.October, 19), "PageUp", d(2026, time.September, 19))
	app.Press("Enter") // completes across the months
	if got := ev.take(); got != "2026-09-19..2026-10-20" {
		t.Errorf("completed range = %q", got)
	}
	app.ExpectRole("heading", "September 2026")
}

func TestRangeCalendarValueAndStyleQueries(t *testing.T) {
	app := telatest.MountFunc(t, rangePage(aria.RangeCalendarOptions{
		DefaultValue: &aria.DateRange{Start: d(2026, time.October, 8), End: d(2026, time.October, 10)},
	}), telatest.Size(400, 360))
	if got := selectedDays(t, app); got != " 8 9 10" {
		t.Errorf("default range:%s", got)
	}
	app.ExpectRole("grid", "October 2026")
	app.Press("Tab")
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(cell(d(2026, time.October, 8))) // the tab stop is the range's start
}

func TestRangeCalendarQueries(t *testing.T) {
	var res aria.CalendarResult
	telatest.MountFunc(t, func() tela.Node {
		res = aria.UseRangeCalendar(aria.RangeCalendarOptions{
			DefaultValue: &aria.DateRange{Start: d(2026, time.October, 8), End: d(2026, time.October, 10)},
			Locale:       "en-US", Today: calToday,
		})
		return renderCalendar(res)
	}, telatest.Size(400, 360))
	for day, want := range map[int][4]bool{ // selected, in range, start, end
		7: {false, false, false, false}, 8: {true, true, true, false}, 9: {true, true, false, false},
		10: {true, true, false, true}, 11: {false, false, false, false},
	} {
		dd := d(2026, time.October, day)
		got := [4]bool{res.IsSelected(dd), res.IsInRange(dd), res.IsRangeStart(dd), res.IsRangeEnd(dd)}
		if got != want {
			t.Errorf("Oct %d: selected, in range, start, end = %v, want %v", day, got, want)
		}
	}
	if !res.IsToday(calToday) || res.IsToday(d(2026, time.October, 24)) {
		t.Error("IsToday")
	}
	if res.Range.Start != d(2026, time.October, 8) || res.Range.End != d(2026, time.October, 10) || !res.Value.IsZero() {
		t.Errorf("Range = %v, Value = %v", res.Range, res.Value)
	}
}

func TestRangeCalendarUnavailableBoundsTheRange(t *testing.T) {
	var ev events
	booked := func(day aria.Date) bool {
		return day == d(2026, time.October, 12) || day == d(2026, time.October, 20)
	}
	app := telatest.MountFunc(t, rangePage(aria.RangeCalendarOptions{
		IsDateUnavailable: booked,
		OnValueChange:     func(r aria.DateRange) { ev.add(r.Start.String() + ".." + r.End.String()) },
	}), telatest.Size(400, 360))
	app.Expect(cell(d(2026, time.October, 11)), telatest.Disabled(false))
	app.Click(cell(d(2026, time.October, 15)))
	// Past the nearest booked date on each side nothing can end the range.
	app.Expect(cell(d(2026, time.October, 11)), telatest.Disabled(true))
	app.Expect(cell(d(2026, time.October, 13)), telatest.Disabled(false))
	app.Expect(cell(d(2026, time.October, 19)), telatest.Disabled(false))
	app.Expect(cell(d(2026, time.October, 21)), telatest.Disabled(true))
	clickDisabled(app, cell(d(2026, time.October, 21)))
	clickDisabled(app, cell(d(2026, time.October, 12)))
	if got := ev.take(); got != "" {
		t.Fatalf("an unavailable date completed a range: %q", got)
	}
	app.Click(cell(d(2026, time.October, 13)))
	if got := ev.take(); got != "2026-10-13..2026-10-15" {
		t.Errorf("range = %q", got)
	}
	// With the anchor gone every date is as available as before.
	app.Expect(cell(d(2026, time.October, 21)), telatest.Disabled(false))
	app.Expect(cell(d(2026, time.October, 12)), telatest.Disabled(true))
}

func TestRangeCalendarNonContiguous(t *testing.T) {
	var ev events
	booked := func(day aria.Date) bool { return day == d(2026, time.October, 12) }
	app := telatest.MountFunc(t, rangePage(aria.RangeCalendarOptions{
		IsDateUnavailable: booked, AllowsNonContiguousRanges: true,
		OnValueChange: func(r aria.DateRange) { ev.add(r.Start.String() + ".." + r.End.String()) },
	}), telatest.Size(400, 360))
	app.Click(cell(d(2026, time.October, 15)))
	app.Expect(cell(d(2026, time.October, 9)), telatest.Disabled(false))
	app.Click(cell(d(2026, time.October, 9)))
	if got := ev.take(); got != "2026-10-09..2026-10-15" {
		t.Errorf("range = %q", got)
	}
}

func TestRangeCalendarReadOnlyDoesNotAnchor(t *testing.T) {
	app := telatest.MountFunc(t, rangePage(aria.RangeCalendarOptions{ReadOnly: true}), telatest.Size(400, 360))
	app.Click(cell(d(2026, time.October, 7)))
	if got := selectedDays(t, app); got != "" {
		t.Errorf("a read-only range calendar selected:%s", got)
	}
}
