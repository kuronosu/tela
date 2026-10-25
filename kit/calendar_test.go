package kit_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/kit"
)

// The Chromium side of the calendar sheets is the DOM that react-day-picker
// 9.14.0 renders in shadcn/ui's Calendar (new-york-v4/ui/calendar.tsx at
// kit.ShadcnCommit, unmodified), rendered to static markup by a React page
// whose scenarios are in testdata/daypicker/capture.jsx, one file of markup
// per scenario (testdata/daypicker/<scenario>.html; build.sh runs it).
// "Today" is October 25, 2026 on both sides.

func readDayPicker(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/daypicker/" + name + ".html")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// d is a date of the tests.
func calDate(y int, m time.Month, day int) aria.Date { return aria.Date{Year: y, Month: m, Day: day} }

var calToday = calDate(2026, time.October, 25)

// calendarCells are the cells of the single-month sheet.
func calendarCells(t testing.TB) []cell {
	oct := func(day int) aria.Date { return calDate(2026, time.October, day) }
	return []cell{
		{name: "single", html: readDayPicker(t, "single"), tela: kit.Calendar{Today: calToday, DefaultValue: oct(14)}},
		{name: "range", html: readDayPicker(t, "range"), tela: kit.RangeCalendar{Today: calToday, DefaultValue: aria.DateRange{Start: oct(12), End: oct(18)}}},
		{name: "limits", html: readDayPicker(t, "limits"), tela: kit.Calendar{
			Today: calToday, DefaultValue: oct(20), Min: oct(8), Max: oct(28),
			IsDateUnavailable: func(day aria.Date) bool { return day == oct(15) },
		}},
		{name: "fixed", html: readDayPicker(t, "fixed"), tela: kit.Calendar{Today: calToday, DefaultFocusedDate: calDate(2026, time.February, 10), FixedWeeks: true}},
		{name: "nooutside", html: readDayPicker(t, "nooutside"), tela: kit.Calendar{Today: calToday, DefaultValue: oct(14), HideOutsideDays: true}},
		{name: "monday", html: readDayPicker(t, "monday"), tela: kit.Calendar{
			Today: calToday, DefaultValue: oct(25), FirstDayOfWeek: func() *time.Weekday { w := time.Monday; return &w }(),
		}},
	}
}

func calendarSheet(t testing.TB) *sheet {
	cells := calendarCells(t)
	for i := range cells {
		cells[i].probes = nil
	}
	return &sheet{name: "calendar", cells: cells, slot: 280, edge: [2]float64{0.1, 0.5}}
}

func rangeCalendarSheet(t testing.TB) *sheet {
	return &sheet{name: "rangecalendar", slot: 520, cells: []cell{
		{name: "two", html: readDayPicker(t, "two"), tela: kit.RangeCalendar{
			Today: calToday, VisibleMonths: 2, DefaultValue: aria.DateRange{Start: calDate(2026, time.October, 20), End: calDate(2026, time.November, 5)},
		}},
	}, edge: [2]float64{0.1, 0.5}}
}

func TestCalendarGolden(t *testing.T)        { calendarSheet(t).golden(t) }
func TestChromiumCalendar(t *testing.T)      { calendarSheet(t).chromium(t) }
func TestRangeCalendarGolden(t *testing.T)   { rangeCalendarSheet(t).golden(t) }
func TestChromiumRangeCalendar(t *testing.T) { rangeCalendarSheet(t).chromium(t) }

var _ tela.Component = kit.Calendar{}
