package main

import (
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/idle"
	"github.com/kuronosu/tela/telatest"
)

// The counted budgets of the dashboard (ADR-290): the page at rest does
// nothing, and an interaction costs a known amount of work and of
// allocations per frame. Each budget is the value measured on the date
// it names. A count over its budget fails; one under it fails too, asking
// for the constant to be lowered to it. A budget only goes up with an ADR
// that names what was given up for it.
//
// Every count runs on the app's clock with fixed inputs, so it is the
// same on every machine and in every run. The allocations are counted
// with GOMAXPROCS at 1, so the rasterizer renders on the calling
// goroutine whatever the machine's cores, and by tests that are not
// parallel: the runtime counts the allocations of the whole process, and
// Go runs the parallel tests only once the others have ended.

// Work per interaction, measured 2026-11-17 at 1440x900 (scrolling at
// 1440x720, where the overview's content overflows its pane).
const (
	// The pointer moves from one row of the latest orders to the next and
	// the rows' hover transitions run to their end.
	hoverFrames  = 11
	hoverBuilds  = 0
	hoverStyles  = 12
	hoverLaidOut = 0
	hoverPaints  = 11
	hoverPlaced  = 180

	// One character typed in the report dialog's name field.
	typeFrames  = 1
	typeBuilds  = 9
	typeStyles  = 0
	typeLaidOut = 0
	typePaints  = 1
	typePlaced  = 168

	// One wheel step over the overview's main pane, once its scrollbar
	// shows: one frame that paints the moved content.
	scrollFrames  = 1
	scrollBuilds  = 0
	scrollStyles  = 0
	scrollLaidOut = 0
	scrollPaints  = 1
	scrollPlaced  = 0
)

// Allocations per steady frame, measured 2026-11-17 at 1440x900 (the
// scroll at 1440x720), rasterization of the damage included, in the
// bench rig (bench_test.go), which runs a frame as core.Host does.
const (
	// A sweep of the pointer up and down four rows of the latest orders.
	hoverAllocsPerFrame = 1276
	// Four wheel steps of 40 px down the overview's main pane and four
	// back up.
	scrollAllocsPerFrame = 484
)

// Row names of the latest orders: the first two of the table.
const (
	firstRow  = "#4891 WL William Lee william.lee@example.com Pro Paid Oct 30 $99.00"
	secondRow = "#4890 LS Lucas Silva lucas.silva@example.com Enterprise Pending Oct 29 $2,998.00"
)

// budgeted is one counted value and its budget, the constant named name.
type budgeted struct {
	name        string
	got, budget int
}

// checkBudgets fails tb for every count over or under its budget.
func checkBudgets(tb testing.TB, what string, counts ...budgeted) {
	tb.Helper()
	for _, c := range counts {
		switch {
		case c.got > c.budget:
			tb.Errorf("%s: counted %d for %s, over its budget of %d (budget_test.go). Find the extra work; a budget goes up only with an ADR that names what was given up (ADR-290)",
				what, c.got, c.name, c.budget)
		case c.got < c.budget:
			tb.Errorf("%s: counted %d for %s, under its budget of %d: lower %s to %d in budget_test.go, with today's date (budgets only go down)",
				what, c.got, c.name, c.budget, c.name, c.got)
		}
	}
}

// workCounts pairs the work w of an interaction with its budgets, the
// constants prefix+"Frames" and so on.
func workCounts(prefix string, w telatest.Stats, frames, builds, styles, laidOut, paints, placed int) []budgeted {
	return []budgeted{
		{prefix + "Frames", w.Frames, frames},
		{prefix + "Builds", w.Builds, builds},
		{prefix + "Styles", w.Styles, styles},
		{prefix + "LaidOut", w.LaidOut, laidOut},
		{prefix + "Paints", w.Paints, paints},
		{prefix + "Placed", w.Placed, placed},
	}
}

// TestIdleBudget: once the page has settled, nobody touching it, it does
// nothing (ARCHITECTURE §3.5, "no change, no frame"): no frame, build,
// style, layout or paint over a second of frames and five seconds more,
// nothing pending in the app, and on the host itself no frame request,
// timer or present.
func TestIdleBudget(t *testing.T) {
	t.Parallel()
	idle.Zero(t, "the dashboard", idle.Watch(mount(t, windowW, windowH)))
	cfg := core.Config{RootClass: rootClass, Theme: (*core.Theme)(&theme)}
	idle.Mount(t, Dashboard(), windowW, windowH, cfg).Settle().Idle("the dashboard")
}

// TestHoverBudget: moving from one order row to the next restyles and
// repaints the two rows and builds and lays out nothing; the page is idle
// again once their transitions end.
func TestHoverBudget(t *testing.T) {
	t.Parallel()
	a := mount(t, windowW, windowH)
	a.Hover(telatest.ByRole("row", firstRow))
	w := a.Measure(func() { a.Hover(telatest.ByRole("row", secondRow)) })
	checkBudgets(t, "hovering the next order row", workCounts("hover", w,
		hoverFrames, hoverBuilds, hoverStyles, hoverLaidOut, hoverPaints, hoverPlaced)...)
	a.ExpectIdle()
}

// TestTypeBudget: a character typed in the report dialog's name field.
// The caret blinks until 10 s after the last key; then the page is idle.
func TestTypeBudget(t *testing.T) {
	t.Parallel()
	a := mountPreview(t, "5 Report dialog")
	name := telatest.ByRole("textbox", "Name")
	a.ExpectFocused(name)
	a.Type("s") // the first key of a session may do one-time work
	w := a.Measure(func() { a.Type("s") })
	checkBudgets(t, "typing a key in the report's name field", workCounts("type", w,
		typeFrames, typeBuilds, typeStyles, typeLaidOut, typePaints, typePlaced)...)
	a.ExpectValue(name, "ssQ3 revenue summary") // autofocus puts the caret at the start
	a.Advance(11 * time.Second)
	a.ExpectIdle()
}

// TestScrollBudget: one wheel step over the main pane is one frame that
// lays out and places nothing. The overlay scrollbar fades a second
// after the last step, and then the page is idle.
func TestScrollBudget(t *testing.T) {
	t.Parallel()
	a := mount(t, windowW, 720, telatest.NoSettle())
	a.Advance(3 * time.Second) // the springs of the numbers come to rest
	a.ExpectIdle()
	pane := telatest.ByTestID("content")
	a.Scroll(pane, 0, 40) // shows the scrollbar
	heading := telatest.ByRole("heading", "Latest orders")
	y := a.Bounds(heading).Y
	w := a.Measure(func() { a.Scroll(pane, 0, 40) })
	if moved := y - a.Bounds(heading).Y; moved != 40 {
		t.Fatalf("the wheel step moved the main pane by %g px, want 40: does the overview still overflow at 1440x720?", moved)
	}
	checkBudgets(t, "one wheel step over the main pane", workCounts("scroll", w,
		scrollFrames, scrollBuilds, scrollStyles, scrollLaidOut, scrollPaints, scrollPlaced)...)
	a.Advance(2 * time.Second)
	a.ExpectIdle()
}

// TestHoverAllocsBudget: the allocations of a frame of a hover sweep over
// the order rows.
func TestHoverAllocsBudget(t *testing.T) {
	r := newRig(t, windowW, windowH, 1, false)
	r.settle(600)
	first, second := center(r.find("row", firstRow)), center(r.find("row", secondRow))
	bottom := platform.Point{X: first.X, Y: first.Y + 3*(second.Y-first.Y)}
	sweep := []platform.Point{first, bottom, first}
	per := allocsPerFrame(t, r, 50, func(i int) []func() { return []func(){r.move(path(sweep, 8, i))} })
	checkBudgets(t, "allocations per frame of a hover sweep over the order rows", budgeted{"hoverAllocsPerFrame", per, hoverAllocsPerFrame})
}

// TestScrollAllocsBudget: the allocations of a frame of wheel steps over
// the main pane.
func TestScrollAllocsBudget(t *testing.T) {
	r := newRig(t, windowW, 720, 1, false)
	r.settle(600)
	r.tick(nil, r.move(center(r.find("heading", "Subscribers"))))
	per := allocsPerFrame(t, r, 8, func(i int) []func() {
		dy := float32(40)
		if i >= 4 {
			dy = -dy
		}
		return []func(){r.wheel(dy, platform.ScrollPhaseNone)}
	})
	checkBudgets(t, "allocations per frame of wheel steps over the main pane", budgeted{"scrollAllocsPerFrame", per, scrollAllocsPerFrame})
}

// allocsPerFrame plays steps ticks of input on r three times in a row
// and returns the allocations per frame of the last time, rounded: the
// first two warm the caches and the rasterizer's scratch memory up
// (ADR-241), so the last is the steady cost. It runs
// with GOMAXPROCS at 1, as testing.AllocsPerRun does, so the count does
// not depend on the machine's cores.
func allocsPerFrame(tb testing.TB, r *rig, steps int, input func(i int) []func()) int {
	tb.Helper()
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	// Two passes warm up; each of the three after them is counted, and the
	// smallest count is the frames' own: the work is the same in every
	// pass, and what another goroutine allocates meanwhile (the runtime, one
	// an earlier test left) only adds to a count (the macOS runner once
	// counted 1276.5 for 1276.4).
	var before, after runtime.MemStats
	frames := 0
	best := -1.0
	for pass := range 5 {
		if pass >= 2 {
			runtime.ReadMemStats(&before)
		}
		frames = 0
		for i := range steps {
			if r.tick(nil, input(i)...) {
				frames++
			}
		}
		if pass >= 2 {
			runtime.ReadMemStats(&after)
			if frames == 0 {
				tb.Fatal("the input ran no frame: nothing to count")
			}
			if per := float64(after.Mallocs-before.Mallocs) / float64(frames); best < 0 || per < best {
				best = per
			}
		}
	}
	tb.Logf("%d frames, %s allocations per frame", frames, fmt.Sprintf("%.1f", best))
	return int(math.Round(best))
}
