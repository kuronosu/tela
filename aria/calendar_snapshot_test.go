package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/telatest"
)

// The roles and states of the calendars print as Playwright prints the
// equivalent HTML: testdata/calendar_*.aria.txt are Playwright's own
// output (ariaSnapshot() of Chromium, Playwright 1.64.1) for a page that
// has the same roles, names and attributes as the props of the hooks (a
// div role=application with the month buttons and a heading, a div
// role=grid labelled by the heading, rows, columnheaders named by the
// weekday and button role=gridcell cells with aria-selected, aria-label,
// disabled and aria-disabled). The test fails if Tela prints anything
// else.
func TestCalendarAriaSnapshot(t *testing.T) {
	// February 2026 with Sunday first has four weeks; the 10th is selected
	// and has the tab stop, the 12th is unavailable (aria-disabled only),
	// the 1st and 2nd are before Min (disabled), and so is the previous
	// month button.
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		AriaLabel: "Appointment", DefaultValue: d(2026, time.February, 10),
		Min: d(2026, time.February, 3), IsDateUnavailable: func(x aria.Date) bool { return x.Day == 12 },
		DefaultFocusedDate: d(2026, time.February, 10),
	}), telatest.Size(400, 360))
	app.AriaGolden("calendar_single.aria.txt")
}

func TestRangeCalendarAriaSnapshot(t *testing.T) {
	// A read-only range of three days: the grid is aria-readonly and
	// aria-multiselectable, and the cells of the range are selected.
	app := telatest.MountFunc(t, rangePage(aria.RangeCalendarOptions{
		AriaLabel: "Stay", ReadOnly: true,
		DefaultValue:       aria.DateRange{Start: d(2026, time.February, 10), End: d(2026, time.February, 12)},
		DefaultFocusedDate: d(2026, time.February, 10),
	}), telatest.Size(400, 360))
	app.AriaGolden("calendar_range.aria.txt")
}
