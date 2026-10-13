package main

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1280, 800

// mount opens the screen in dev mode, where any dev message fails the test.
// The clock goroutine runs in real time, so the waits are generous for slow
// machines.
func mount(t *testing.T, w, h float32, root func() tela.Node, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h), telatest.Dev(), telatest.Timeout(30 * time.Second)}, opts...)
	return telatest.Mount(t, tela.C(root), opts...)
}

// fast makes the progress clock run scale times faster than real time and
// tick every tick, until the test ends.
func fast(t *testing.T, tick time.Duration, scale float64) {
	t.Helper()
	oldTick, oldScale := tickEvery, timeScale
	tickEvery, timeScale = tick, scale
	t.Cleanup(func() { tickEvery, timeScale = oldTick, oldScale })
}

// slow keeps the clock at its real speed but ticks rarely, so tests that
// do not look at the progress are not disturbed by it.
func slow(t *testing.T) { fast(t, time.Hour, 1) }

func TestNoDevMessages(t *testing.T) {
	slow(t)
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		mount(t, s[0], s[1], Player)
	}
}

// TestGolden is Preview (a minute and 24 seconds in), at scale 1 and 2.
func TestGolden(t *testing.T) {
	slow(t)
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, Preview, telatest.Scale(scale)).Golden(fmt.Sprintf("player_x%g.png", scale))
		})
	}
}

// The icon buttons have no accessible names, so the tests find them by
// their classes.
var (
	progressFill = telatest.ByClass("h-full", "rounded-full", "bg-white")
	pauseBars    = telatest.ByClass("h-5", "w-1.5")
	seekBar      = telatest.ByClass("group", "h-4", "cursor-pointer")
)

// fillWidth returns the width of the progress fill.
func fillWidth(a *telatest.App) float32 {
	_, _, w, _ := a.Bounds(progressFill)
	return w
}

// titleIs waits for the current track's title (an h2 in the player card).
func titleIs(a *telatest.App, title string) {
	a.ExpectRole("heading", title)
}

func TestProgressAdvancesWithoutBuilding(t *testing.T) {
	// The clock is stopped and the test ticks it, 0.1 s of music per tick,
	// so the result does not depend on how fast frames run (a real-time
	// clock raced past 0:03 when frames were slow, e.g. under
	// TELA_CHECK_DAMAGE).
	slow(t)
	var tr *Transport
	a := mount(t, windowW, windowH, func() tela.Node {
		tr = useTransport(0)
		return page(tr)
	})
	if w := fillWidth(a); w != 0 {
		t.Fatalf("the bar starts at %v px, want 0", w)
	}
	before := a.Stats()
	for range 30 {
		tela.Post(func() { tr.advance(0.1) })
		a.Advance(16 * time.Millisecond)
	}
	a.ExpectText("0:03")
	after := a.Stats()

	frames, builds := after.Frames-before.Frames, after.Builds-before.Builds
	want := float32(3) / 222 * barW
	if w := fillWidth(a); w < want*0.95 || w > want*1.3 {
		t.Errorf("the bar is %v px wide at 0:03, want about %v", w, want)
	}
	// Three label changes: the label builds three times and nothing else.
	if builds != 3 {
		t.Errorf("%d builds for 3 seconds of music over %d frames, want 3 (the label)", builds, frames)
	}
	if frames < 25 {
		t.Errorf("only %d frames for 30 ticks: the bar should be laid out on (nearly) every tick", frames)
	}
	if got := after.Layouts - before.Layouts; got < frames/2 {
		t.Errorf("%d layouts in %d frames: a WidthFn change should lay out each time", got, frames)
	}
	if got := after.Styles - before.Styles; got > 10 {
		t.Errorf("%d style computations: ticks should not restyle", got)
	}
}

func TestPauseStopsTheClock(t *testing.T) {
	fast(t, 5*time.Millisecond, 10)
	a := mount(t, windowW, windowH, Player)
	a.WaitFor("the bar to start moving", func() bool { return fillWidth(a) > 3 })
	a.ExpectCount(pauseBars, 2) // the pause icon while playing

	a.Click(telatest.ByRole("button", "Pause"))
	a.ExpectCount(pauseBars, 0) // the play triangle
	a.MouseMove(5, 5)
	w, frames := fillWidth(a), a.Stats().Frames
	time.Sleep(100 * time.Millisecond)
	if got := fillWidth(a); got != w {
		t.Errorf("the bar moved while paused: %v -> %v", w, got)
	}
	if got := a.Stats().Frames; got != frames {
		t.Errorf("%d frames while paused, want none: nothing changes, so nothing draws", got-frames)
	}

	a.Click(telatest.ByRole("button", "Play")) // play again
	a.WaitFor("the bar to advance after playing again", func() bool { return fillWidth(a) > w+3 })
}

func TestNextPrevAndQueue(t *testing.T) {
	slow(t)
	a := mount(t, windowW, windowH, Player)
	titleIs(a, "Midnight Drive")
	prev, next := telatest.ByRole("button", "Previous track"), telatest.ByRole("button", "Next track")

	a.Click(next)
	titleIs(a, "Paper Planes")
	a.ExpectText("3:08")
	// Prev within the first seconds goes back a track.
	a.Click(prev)
	titleIs(a, "Midnight Drive")
	// Prev wraps around from the first track.
	a.Click(prev)
	titleIs(a, "Low Orbit")

	// A click on a queue row plays it, from its start.
	a.Click(telatest.ByRole("button", "").Filter(telatest.HasText("Slow Tide")))
	titleIs(a, "Slow Tide")
	a.ExpectText("4:15")
	// One equalizer, on the current row.
	a.ExpectCount(telatest.ByClass("h-4", "text-violet-400", "items-end"), 1)
}

// clickAt clicks at a position of the window, like a user.
func clickAt(a *telatest.App, x, y float32) {
	a.MouseMove(x, y)
	a.MouseDown()
	a.MouseUp()
}

// dragBetween drags the pointer from (x0, y0) to (x1, y1) in steps with the
// button held.
func dragBetween(a *telatest.App, x0, y0, x1, y1 float32, steps int) {
	a.MouseMove(x0, y0)
	a.MouseDown()
	for i := 1; i <= steps; i++ {
		f := float32(i) / float32(steps)
		a.MouseMove(x0+(x1-x0)*f, y0+(y1-y0)*f)
	}
	a.MouseUp()
}

func TestSeek(t *testing.T) {
	slow(t)
	a := mount(t, windowW, windowH, Player)
	a.ExpectCount(seekBar, 1)
	bx, by, bw, bh := a.Bounds(seekBar)
	if bw != barW {
		t.Fatalf("the seek bar is %v px wide, the code assumes %v", bw, barW)
	}
	y := by + bh/2

	clickAt(a, bx+barW/2, y)
	a.ExpectText("1:51") // half of 3:42
	if w := fillWidth(a); w < barW/2-1 || w > barW/2+1 {
		t.Errorf("fill is %v px after seeking to the middle", w)
	}

	// Dragging: press at a quarter, release at three quarters.
	dragBetween(a, bx+barW/4, y, bx+barW*3/4, y, 8)
	a.ExpectText("2:46") // three quarters of 222 s
	// Dragging beyond the ends clamps.
	dragBetween(a, bx+100, y, bx-200, y, 6)
	a.ExpectText("0:00")
	// The thumb is hidden until hover (opacity), the pointer cursor shows.
	a.MouseMove(bx+10, y)
	if got := a.Cursor(); got == "default" {
		t.Errorf("cursor over the bar is %v", got)
	}
}

func TestTrackEndsAndPlaysNext(t *testing.T) {
	slow(t) // the test ticks the clock: 2 s of music per tick
	var tr *Transport
	a := mount(t, windowW, windowH, func() tela.Node {
		tr = useTransport(0)
		return page(tr)
	})
	for range 115 { // 230 s: past the end of the 3:42 first track
		tela.Post(func() { tr.advance(2) })
		a.Advance(16 * time.Millisecond)
	}
	titleIs(a, "Paper Planes")
}

// TestClockStopsOnUnmount checks that the goroutine ends with the player.
// It looks for the clock goroutine by its function in a stack dump rather
// than counting all goroutines, which other tests' goroutines disturb.
func TestClockStopsOnUnmount(t *testing.T) {
	fast(t, 5*time.Millisecond, 1)
	before := clocks()
	a := mount(t, windowW, windowH, Player) // mounting runs the passive effects
	if clocks() <= before {
		t.Fatal("the clock goroutine did not start")
	}
	a.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && clocks() > before {
		time.Sleep(5 * time.Millisecond)
	}
	if got := clocks(); got > before {
		t.Errorf("%d clock goroutines after unmount, was %d before the player", got, before)
	}
}

// clocks counts the running clock goroutines started by useTransport.
func clocks() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	n := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "useTransport.func") {
			n++
		}
	}
	return n
}
