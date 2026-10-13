package main

import (
	"runtime"
	"testing"
	"time"

	"github.com/kuronosu/tela/examples/internal/exampletest"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

const windowW, windowH = 1280, 800

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
		a := exampletest.Open(t, Player, s[0], s[1])
		a.CheckLogs()
	}
}

// TestGolden is Preview (a minute and 24 seconds in), at scale 1 and 2.
func TestGolden(t *testing.T) {
	slow(t)
	exampletest.Golden(t, "player", Preview, windowW, windowH)
}

// fill returns the box of the progress fill.
func fill(a *exampletest.App) core.InspectNode {
	a.T.Helper()
	f := a.WithClass("h-full", "rounded-full", "bg-white")
	if len(f) != 1 {
		a.T.Fatalf("%d progress fills, want 1", len(f))
	}
	return f[0]
}

// title returns the title of the current track, in the player card (the
// first h2 that is not the queue's heading).
func title(a *exampletest.App) string {
	n, ok := a.Find(func(n core.InspectNode) bool { return n.Kind == "h2" && n.Text != "Up next" })
	if !ok {
		a.T.Fatal("no title")
	}
	return n.Text
}

// pump runs frames while real time passes, until done reports true or the
// timeout expires.
func pump(a *exampletest.App, timeout time.Duration, done func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
		a.Frame()
		if done() {
			return true
		}
	}
	return false
}

// TestProgressAdvancesWithoutBuilding is the point of the example: the
// clock goroutine moves the bar for a few seconds of music, and the only
// component that builds is the time label, once per second.
func TestProgressAdvancesWithoutBuilding(t *testing.T) {
	fast(t, 5*time.Millisecond, 10) // 20 ticks per second of music
	a := exampletest.Open(t, Player, windowW, windowH)
	if w := fill(a).Rect.Width(); w != 0 {
		t.Fatalf("the bar starts at %v px, want 0", w)
	}
	before := a.Tree().Stats()
	if !pump(a, 5*time.Second, func() bool { return a.HasText("0:03") }) {
		t.Fatal("the clock did not reach 0:03")
	}
	after := a.Tree().Stats()

	frames, builds := after.Frames-before.Frames, after.Builds-before.Builds
	want := float32(3) / 222 * barW
	if w := fill(a).Rect.Width(); w < want*0.95 || w > want*1.3 {
		t.Errorf("the bar is %v px wide at 0:03, want about %v", w, want)
	}
	// Three label changes: the label builds three times and nothing else.
	if builds != 3 {
		t.Errorf("%d builds for 3 seconds of music over %d frames, want 3 (the label)", builds, frames)
	}
	if frames < 30 {
		t.Errorf("only %d frames: the bar should be laid out on every tick", frames)
	}
	if got := after.Layouts - before.Layouts; got < frames/2 {
		t.Errorf("%d layouts in %d frames: a WidthFn change should lay out each time", got, frames)
	}
	if got := after.Styles - before.Styles; got > 10 {
		t.Errorf("%d style computations: ticks should not restyle", got)
	}
	a.CheckLogs()
}

func TestPauseStopsTheClock(t *testing.T) {
	fast(t, 5*time.Millisecond, 10)
	a := exampletest.Open(t, Player, windowW, windowH)
	pump(a, 5*time.Second, func() bool { return fill(a).Rect.Width() > 3 })
	if len(a.WithClass("h-5", "w-1.5")) != 2 {
		t.Fatal("no pause icon while playing")
	}

	c := a.WithClass("size-16", "bg-white")[0].Rect.Center()
	a.Click(c.X, c.Y)
	if n := len(a.WithClass("h-5", "w-1.5")); n != 0 {
		t.Errorf("%d pause bars after pausing, want the play triangle", n)
	}
	a.Move(5, 5)
	w, frames := fill(a).Rect.Width(), a.Tree().Stats().Frames
	time.Sleep(100 * time.Millisecond)
	a.Frame()
	if got := fill(a).Rect.Width(); got != w {
		t.Errorf("the bar moved while paused: %v -> %v", w, got)
	}
	if got := a.Tree().Stats().Frames; got != frames {
		t.Errorf("%d frames while paused, want none: nothing changes, so nothing draws", got-frames)
	}

	a.Click(c.X, c.Y) // play again
	if !pump(a, 5*time.Second, func() bool { return fill(a).Rect.Width() > w+3 }) {
		t.Error("the bar did not advance after playing again")
	}
}

func TestNextPrevAndQueue(t *testing.T) {
	slow(t)
	a := exampletest.Open(t, Player, windowW, windowH)
	if title(a) != "Midnight Drive" {
		t.Fatalf("starts on %q", title(a))
	}
	buttons := a.All(func(n core.InspectNode) bool {
		return n.Kind == "button" && exampletest.HasClass(n, "size-12")
	})
	if len(buttons) != 2 {
		t.Fatalf("%d prev/next buttons", len(buttons))
	}
	prev, next := buttons[0].Rect.Center(), buttons[1].Rect.Center()

	a.Click(next.X, next.Y)
	if title(a) != "Paper Planes" || !a.HasText("3:08") {
		t.Errorf("after Next: %q", title(a))
	}
	// Prev within the first seconds goes back a track.
	a.Click(prev.X, prev.Y)
	if title(a) != "Midnight Drive" {
		t.Errorf("after Prev: %q", title(a))
	}
	// Prev wraps around from the first track.
	a.Click(prev.X, prev.Y)
	if title(a) != "Low Orbit" {
		t.Errorf("Prev from the first track: %q", title(a))
	}

	// A click on a queue row plays it, from its start.
	a.ClickButton("Slow Tide")
	if title(a) != "Slow Tide" || !a.HasText("4:15") {
		t.Errorf("after clicking the queue row: %q", title(a))
	}
	if n := len(a.WithClass("h-4", "text-violet-400", "items-end")); n != 1 {
		t.Errorf("%d equalizers, want one on the current row", n)
	}
	a.CheckLogs()
}

func TestSeek(t *testing.T) {
	slow(t)
	a := exampletest.Open(t, Player, windowW, windowH)
	bar := a.WithClass("group", "h-4", "cursor-pointer")
	if len(bar) != 1 {
		t.Fatalf("%d seek bars", len(bar))
	}
	b := bar[0].Rect
	if b.Width() != barW {
		t.Fatalf("the seek bar is %v px wide, the code assumes %v", b.Width(), barW)
	}
	y := b.Center().Y

	a.Click(b.X0+barW/2, y)
	if !a.HasText("1:51") { // half of 3:42
		t.Error("clicking the middle of the bar did not seek to 1:51")
	}
	if w := fill(a).Rect.Width(); w < barW/2-1 || w > barW/2+1 {
		t.Errorf("fill is %v px after seeking to the middle", w)
	}

	// Dragging: press at a quarter, release at three quarters.
	a.Drag(gfx.Point{X: b.X0 + barW/4, Y: y}, gfx.Point{X: b.X0 + barW*3/4, Y: y}, 8)
	if !a.HasText("2:46") { // three quarters of 222 s
		t.Errorf("dragging to 75%% did not seek to 2:46 (fill %v)", fill(a).Rect.Width())
	}
	// Dragging beyond the ends clamps.
	a.Drag(gfx.Point{X: b.X0 + 100, Y: y}, gfx.Point{X: b.X0 - 200, Y: y}, 6)
	if !a.HasText("0:00") {
		t.Error("dragging past the left end did not clamp to 0:00")
	}
	// The thumb is hidden until hover (opacity), the pointer cursor shows.
	a.Move(b.X0+10, y)
	if got := a.Window.Cursor(); got.String() == "default" {
		t.Errorf("cursor over the bar is %v", got)
	}
}

func TestTrackEndsAndPlaysNext(t *testing.T) {
	fast(t, 5*time.Millisecond, 400) // 2 s of music per tick
	a := exampletest.Open(t, Player, windowW, windowH)
	if !pump(a, 5*time.Second, func() bool { return title(a) == "Paper Planes" }) {
		t.Fatalf("still on %q after the first track should have ended", title(a))
	}
}

// TestClockStopsOnUnmount checks that the goroutine ends with the player.
func TestClockStopsOnUnmount(t *testing.T) {
	fast(t, 5*time.Millisecond, 1)
	before := runtime.NumGoroutine()
	a := exampletest.Open(t, Player, windowW, windowH)
	a.Frame()
	a.Frame() // passive effects run after a frame is presented
	if runtime.NumGoroutine() <= before {
		t.Fatal("the clock goroutine did not start")
	}
	a.Tree().Unmount()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && runtime.NumGoroutine() > before {
		time.Sleep(5 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before {
		t.Errorf("%d goroutines after unmount, was %d before the player", got, before)
	}
}
