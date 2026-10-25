package aria_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// d is a date of the tests.
func d(y int, m time.Month, day int) aria.Date { return aria.Date{Year: y, Month: m, Day: day} }

// calToday is the "today" of every calendar test: a Sunday.
var calToday = d(2026, time.October, 25)

// calendarPage renders the calendar the way the package documentation
// shows: month buttons, headings, weekday headers and a grid per month.
func calendarPage(o aria.CalendarOptions) func() tela.Node {
	if o.Locale == "" {
		o.Locale = "en-US"
	}
	if o.Today.IsZero() {
		o.Today = calToday
	}
	return func() tela.Node { return renderCalendar(aria.UseCalendar(o)) }
}

func rangePage(o aria.RangeCalendarOptions) func() tela.Node {
	if o.Locale == "" {
		o.Locale = "en-US"
	}
	if o.Today.IsZero() {
		o.Today = calToday
	}
	return func() tela.Node { return renderCalendar(aria.UseRangeCalendar(o)) }
}

// loose renders the grids keyed by their position, not their month: a
// month change then updates elements instead of mounting new ones.
var loose bool

func renderCalendar(cal aria.CalendarResult) tela.Node {
	return Div(cal.Props, Class("flex flex-col gap-2 p-3"),
		Div(Class("flex items-center justify-between"),
			Button(cal.PrevProps(), Class("size-8"), Text("<")),
			Span(Class("text-sm"), Text(cal.Title)),
			Button(cal.NextProps(), Class("size-8"), Text(">"))),
		Div(Class("flex gap-4"),
			Range(cal.Months(), func(i int) tela.Node {
				key := cal.Month(i).String()
				if loose {
					key = fmt.Sprint(i)
				}
				return Div(Key(key), Class("flex flex-col gap-1"),
					H2(cal.HeadingProps(i), Class("text-sm font-medium"), Text(cal.Heading(i))),
					Div(cal.GridProps(i), Class("flex flex-col gap-1"),
						Div(cal.RowProps(), Class("flex"),
							MapIndex(cal.WeekdayNames(i18n.NameShort), func(c int, name string) tela.Node {
								return Div(cal.ColumnHeaderProps(c), Key(name), Class("w-8 text-xs"), Text(name))
							})),
						Map(cal.Weeks(i), func(week []aria.Date) tela.Node {
							return Div(cal.RowProps(), Key(week[0].String()), Class("flex"),
								Map(week, func(day aria.Date) tela.Node {
									return Button(cal.CellProps(i, day), Key(day.String()),
										Class("h-8 w-8 text-sm aria-selected:bg-blue-600 aria-selected:text-white aria-disabled:opacity-40"),
										If(cal.IsToday(day), Class("font-bold")),
										If(cal.IsOutsideMonth(i, day), Class("text-gray-400")),
										Text(cal.DayText(day)))
								}))
						})))
			})))
}

// enDate is the accessible name of a cell in English.
func enDate(day aria.Date) string {
	return i18n.For("en-US").Date(day.Time(time.UTC), i18n.Full)
}

func cell(day aria.Date) telatest.Locator { return telatest.ByRole("gridcell", enDate(day)) }

func TestCalendarRendersTheMonth(t *testing.T) {
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{
		DefaultValue: d(2026, time.October, 5), AriaLabel: "Appointment",
	}), telatest.Size(400, 360))
	// Sunday first for en-US; October 2026 starts on a Thursday and has
	// five weeks, filled with the neighbouring months.
	app.Expect(cell(d(2026, time.October, 5)), telatest.Selected(true))
	app.Expect(cell(d(2026, time.October, 6)), telatest.Selected(false))
	app.ExpectRole("application", "Appointment, October 2026")
	app.ExpectRole("grid", "October 2026")
	app.ExpectCount(telatest.ByRole("gridcell", ""), 35)
	app.ExpectCount(telatest.ByRole("row", ""), 6)
	app.ExpectCount(telatest.ByRole("columnheader", ""), 7)
	app.ExpectRole("columnheader", "Sunday")
	app.ExpectRole("gridcell", "Sunday, September 27, 2026")
	app.ExpectRole("gridcell", "Saturday, October 31, 2026")
}

// Grids keyed by position keep their elements across months: the cell of
// the focused date is an old element that now stands for another date, and
// takes the focus after the build.
func TestCalendarWithGridsKeyedByPosition(t *testing.T) {
	loose = true
	t.Cleanup(func() { loose = false })
	app := telatest.MountFunc(t, calendarPage(aria.CalendarOptions{DefaultFocusedDate: d(2026, time.October, 1)}), telatest.Size(400, 360))
	app.Focus(cell(d(2026, time.October, 1)))
	expectKeys(t, app,
		"ArrowLeft", d(2026, time.September, 30),
		"ArrowDown", d(2026, time.October, 7),
		"PageDown", d(2026, time.November, 7),
		"Shift+PageDown", d(2027, time.November, 7),
		"Home", d(2027, time.November, 7),
	)
	app.Click(cell(d(2027, time.December, 4)))
	app.ExpectRole("heading", "December 2027")
	app.ExpectFocused(cell(d(2027, time.December, 4)))
}
