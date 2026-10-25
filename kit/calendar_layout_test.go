package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// TestCalendarInStretchingColumn: a Calendar (whose root is w-fit) placed
// directly in a flex column is as big as in a column with items-start, as
// in Chromium; its day cells are aspect-square like shadcn's (ADR-157).
// The engine used to measure the calendar's height at the column's width:
// 685 px tall instead of 291 at 800 px.
func TestCalendarInStretchingColumn(t *testing.T) {
	bounds := func(class string) (cal, day tela.Rect) {
		app := telatest.MountFunc(t, func() tela.Node {
			return h.Div(h.Class(class), kit.Calendar{Today: calToday, Props: h.TestID("cal")}, h.P(h.Text("After")))
		}, telatest.Size(800, 600), telatest.Theme(kit.Theme()))
		return app.Bounds(telatest.ByTestID("cal")), app.Bounds(telatest.ByRole("gridcell", "").Nth(10))
	}
	cal, day := bounds("flex flex-col")
	calStart, dayStart := bounds("flex flex-col items-start")
	if cal != calStart || day != dayStart {
		t.Errorf("in a stretching column the calendar is %+v and a day %+v; with items-start %+v and %+v", cal, day, calStart, dayStart)
	}
	if cal.Height > 320 || day.Width != day.Height {
		t.Errorf("calendar %+v, day %+v: want a calendar about 291 px tall with square days", cal, day)
	}
}
