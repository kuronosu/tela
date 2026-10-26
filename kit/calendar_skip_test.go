package kit_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The day cells of Calendar and RangeCalendar are struct components with
// comparable fields, skipped while their state does not change, and a
// date whose text only changes color is recolored, not shaped and laid
// out again (ADR-161). These tests pin the work of the interactions that
// were slow, as counts: wall time is not a test.

func TestRangeCalendarPreviewBuildsOnlyChangedCells(t *testing.T) {
	app := calMount(t, kit.RangeCalendar{Today: calToday, VisibleMonths: 2, AriaLabel: "Stay"})
	app.Click(calCell(calDate(2026, time.October, 5)))
	app.Hover(calCell(calDate(2026, time.October, 6)))
	for _, day := range []int{7, 8, 9, 14, 21} {
		st := app.Measure(func() { app.Hover(calCell(calDate(2026, time.October, day))) })
		// The calendar builds again for the new end of the preview; of
		// its cells only those whose part of the range or tab
		// stop changed build again (the old end, the new end and the
		// dates between them), the others are skipped.
		if st.Builds < 1 || st.Builds > 1+day-5 {
			t.Errorf("preview to October %d: %d builds, want the calendar and at most the %d changed cells", day, st.Builds, day-5)
		}
		if st.Skips < 50 {
			t.Errorf("preview to October %d: %d builds skipped, want the unchanged cells (at least 50)", day, st.Skips)
		}
		// The dates whose colors change keep their text layout.
		if st.Shapes != 0 || st.Breaks != 0 {
			t.Errorf("preview to October %d: %d paragraphs shaped and %d broken, want 0 and 0", day, st.Shapes, st.Breaks)
		}
	}
	for _, day := range []int{5, 6, 21} {
		app.Expect(calCell(calDate(2026, time.October, day)), telatest.Selected(true))
	}
	app.Expect(calCell(calDate(2026, time.October, 22)), telatest.Selected(false))
}

func TestCalendarPressBuildsOnlyChangedCells(t *testing.T) {
	app := calMount(t, kit.Calendar{Today: calToday, DefaultValue: calDate(2026, time.October, 12), AriaLabel: "Date"})
	app.Hover(calCell(calDate(2026, time.October, 14)))
	st := app.Measure(func() { app.Click(calCell(calDate(2026, time.October, 14))) })
	// The press focuses the date and the click selects it: two builds of
	// the calendar, each building again only the cells of the old and the
	// new date.
	if st.Builds < 1 || st.Builds > 8 {
		t.Errorf("a press: %d builds, want the calendar and the cells of the two dates (at most 8)", st.Builds)
	}
	if st.Skips < 50 {
		t.Errorf("a press: %d builds skipped, want the unchanged cells of both builds (at least 50)", st.Skips)
	}
	if st.Shapes != 0 || st.Breaks != 0 {
		t.Errorf("a press: %d paragraphs shaped and %d broken, want 0 and 0", st.Shapes, st.Breaks)
	}
	app.Expect(calCell(calDate(2026, time.October, 14)), telatest.Selected(true))
	app.Expect(calCell(calDate(2026, time.October, 12)), telatest.Selected(false))
}

// TestCalendarCellsFollowTheirState checks that a skipped cell still
// follows every change of what it shows when the calendar builds again:
// limits, unavailable dates and the selection set from outside.
func TestCalendarCellsFollowTheirState(t *testing.T) {
	type state struct {
		min    aria.Date
		booked bool
		day    aria.Date
	}
	var set func(state)
	page := func() tela.Node {
		s, setS := tela.UseState(state{day: calDate(2026, time.October, 12)})
		set = setS.Set
		unavailable := func(d aria.Date) bool { return s.booked && d.Day == 20 }
		return h.Div(h.Class("p-6"), kit.Calendar{Today: calToday, Min: s.min, IsDateUnavailable: unavailable,
			Value: &s.day, OnValueChange: func(d aria.Date) { setS.Update(func(s state) state { s.day = d; return s }) }})
	}
	app := telatest.MountFunc(t, page, telatest.Size(800, 460), telatest.Theme(kit.Theme()))
	app.Expect(calCell(calDate(2026, time.October, 8)), telatest.Disabled(false))
	app.Expect(calCell(calDate(2026, time.October, 12)), telatest.Selected(true))

	set(state{min: calDate(2026, time.October, 10), booked: true, day: calDate(2026, time.October, 15)})
	app.Expect(calCell(calDate(2026, time.October, 8)), telatest.Disabled(true))
	app.Expect(calCell(calDate(2026, time.October, 11)), telatest.Disabled(false))
	app.Expect(calCell(calDate(2026, time.October, 20)), telatest.Disabled(true)) // aria-disabled: unavailable
	app.Expect(calCell(calDate(2026, time.October, 12)), telatest.Selected(false))
	app.Expect(calCell(calDate(2026, time.October, 15)), telatest.Selected(true))

	set(state{day: calDate(2026, time.October, 15)})
	app.Expect(calCell(calDate(2026, time.October, 8)), telatest.Disabled(false))
	app.Expect(calCell(calDate(2026, time.October, 20)), telatest.Disabled(false))
}
