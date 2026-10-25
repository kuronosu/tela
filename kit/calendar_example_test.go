package kit_test

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func ExampleCalendar() {
	t := &exampleT{}
	defer t.close()
	var picked aria.Date
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Calendar{
			// Today is injected so the example does not depend on the clock;
			// leave it out in an app.
			Today:         aria.Date{Year: 2026, Month: time.October, Day: 25},
			Min:           aria.Date{Year: 2026, Month: time.October, Day: 8},
			OnValueChange: func(day aria.Date) { picked = day },
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 400))
	// Tab skips the previous button (disabled at Min) and reaches today's
	// date; PageDown moves a month and Enter chooses.
	app.Press("Tab")
	app.Press("Tab")
	app.Press("PageDown")
	app.Press("Enter")
	fmt.Println(picked)
	// Output: 2026-11-25
}

func ExampleRangeCalendar() {
	t := &exampleT{}
	defer t.close()
	var stay aria.DateRange
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.RangeCalendar{
			Today:         aria.Date{Year: 2026, Month: time.October, Day: 25},
			VisibleMonths: 2,
			OnValueChange: func(r aria.DateRange) { stay = r },
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(800, 400))
	// The first press anchors the range, the second completes it.
	app.Click(telatest.ByRole("gridcell", "Tuesday, October 27, 2026"))
	app.Click(telatest.ByRole("gridcell", "Thursday, November 5, 2026"))
	fmt.Println(stay.Start, stay.End)
	// Output: 2026-10-27 2026-11-05
}

func ExampleDatePicker() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.DatePicker{
			AriaLabel: "Birthday",
			Today:     aria.Date{Year: 2026, Month: time.October, Day: 25},
			OnValueChange: func(day aria.Date) {
				fmt.Println("chose", day)
			},
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 480))
	app.Click(telatest.ByRole("button", "Birthday"))
	app.Click(telatest.ByRole("gridcell", "Friday, October 9, 2026"))
	// The popover closed and the button shows the date.
	app.ExpectText("October 9, 2026")
	fmt.Println("open:", app.Count(telatest.ByRole("application", "")) == 1)
	// Output:
	// chose 2026-10-09
	// open: false
}

func ExampleDateRangePicker() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.DateRangePicker{
			AriaLabel: "Stay",
			Today:     aria.Date{Year: 2026, Month: time.October, Day: 25},
			OnValueChange: func(r aria.DateRange) {
				fmt.Println("stay", r.Start, r.End)
			},
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(800, 480))
	app.Click(telatest.ByRole("button", "Stay"))
	app.Click(telatest.ByRole("gridcell", "Tuesday, October 27, 2026"))
	app.Click(telatest.ByRole("gridcell", "Saturday, October 31, 2026"))
	// Output: stay 2026-10-27 2026-10-31
}
