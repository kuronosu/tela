package aria_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

func ExampleUseCalendar() {
	t := &exampleT{}
	defer t.close()
	var picked aria.Date
	page := func() tela.Node {
		cal := aria.UseCalendar(aria.CalendarOptions{
			AriaLabel: "Appointment", Locale: "en-US",
			Today:         aria.Date{Year: 2026, Month: time.October, Day: 25},
			OnValueChange: func(day aria.Date) { picked = day },
		})
		return Div(cal.Props, Class("w-64 p-3"),
			Div(Class("flex items-center justify-between"),
				Button(cal.PrevProps(), Text("<")),
				H2(cal.HeadingProps(0), Text(cal.Heading(0))),
				Button(cal.NextProps(), Text(">"))),
			Div(cal.GridProps(0),
				Div(cal.RowProps(), Class("flex"),
					MapIndex(cal.WeekdayNames(i18n.NameShort), func(i int, name string) tela.Node {
						return Div(cal.ColumnHeaderProps(i), Key(name), Class("w-8"), Text(name))
					})),
				Map(cal.Weeks(0), func(week []aria.Date) tela.Node {
					return Div(cal.RowProps(), Key(week[0].String()), Class("flex"),
						Map(week, func(day aria.Date) tela.Node {
							return Button(cal.CellProps(0, day), Key(day.String()),
								Class("size-8 aria-selected:bg-primary aria-disabled:opacity-40"),
								If(cal.IsOutsideMonth(0, day), Class("text-muted-foreground")),
								Text(cal.DayText(day)))
						}))
				})))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	// Tab reaches the grid at today's cell; the arrows move over the months,
	// PageDown a month, Enter selects.
	app.Focus(telatest.ByRole("button", "Next"))
	app.Press("Tab")
	app.Press("ArrowRight")
	app.Press("PageDown")
	app.Press("Enter")
	title, _, _ := strings.Cut(app.AriaSnapshot(), "\n")
	fmt.Println(picked, title)
	// Output:
	// 2026-11-26 - application "Appointment, November 2026":
}

func ExampleUseRangeCalendar() {
	t := &exampleT{}
	defer t.close()
	var stay aria.DateRange
	page := func() tela.Node {
		cal := aria.UseRangeCalendar(aria.RangeCalendarOptions{
			AriaLabel: "Stay", Locale: "en-US",
			Today:         aria.Date{Year: 2026, Month: time.October, Day: 25},
			OnValueChange: func(r aria.DateRange) { stay = r },
		})
		return Div(cal.Props,
			Div(cal.GridProps(0),
				Map(cal.Weeks(0), func(week []aria.Date) tela.Node {
					return Div(cal.RowProps(), Key(week[0].String()), Class("flex"),
						Map(week, func(day aria.Date) tela.Node {
							return Button(cal.CellProps(0, day), Key(day.String()),
								Class("size-8 aria-selected:bg-primary"), Text(cal.DayText(day)))
						}))
				})))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	// The first press anchors, moving the focus previews, the second press
	// completes the range.
	app.Focus(telatest.ByRole("gridcell", "Sunday, October 25, 2026"))
	app.Press("Enter")
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.Press("Enter")
	fmt.Println(stay.Start, stay.End)
	// Output:
	// 2026-10-25 2026-11-02
}

func ExampleDate() {
	oct := aria.Date{Year: 2026, Month: time.October, Day: 31}
	fmt.Println(oct.AddMonths(1), oct.AddDays(1), oct.Weekday())
	fmt.Println(aria.DateOf(time.Date(2026, 1, 31, 23, 0, 0, 0, time.UTC)).AddMonths(1))
	// Output:
	// 2026-11-30 2026-11-01 Saturday
	// 2026-02-28
}
