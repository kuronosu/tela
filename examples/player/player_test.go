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

// barW is the width of the seek bar the layout gives: the card's content
// width: 380 less the padding (2 * 24) and the border (2 * 1). Only the
// tests know it.
const barW = 330

// knobW is the width of the slider's knob, which stays inside the bar: its
// center runs from half of it to barW minus half.
const knobW = 16

// mount opens the screen in the window of main (kit's theme, always dark);
// dev mode is on, so any dev message fails the test. The clock goroutine
// runs in real time, so the waits are generous for slow machines.
func mount(t *testing.T, w, h float32, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = root
	opts = append([]telatest.Option{telatest.Size(w, h), telatest.Timeout(30 * time.Second)}, opts...)
	return telatest.MountApp(t, app, opts...)
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
		mount(t, s[0], s[1], Player{})
	}
}

// TestGolden is Preview (a minute and 24 seconds in), at scale 1 and 2.
func TestGolden(t *testing.T) {
	slow(t)
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, Preview(), telatest.Scale(scale)).Golden(fmt.Sprintf("player_x%g.png", scale))
		})
	}
}

var (
	seekBar  = telatest.ByTestID("seek")
	seekKnob = telatest.ByRole("slider", "Seek")
)

// fillWidth returns how far along the bar the progress is, in pixels of the
// bar: the knob's center, from the bar's left edge, without the half knob
// the bar keeps inside.
func fillWidth(a *telatest.App) float32 {
	k, bar := a.Bounds(seekKnob), a.Bounds(seekBar)
	return (k.X + k.Width/2 - bar.X - knobW/2) * barW / (barW - knobW)
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
	a := mount(t, windowW, windowH, tela.C(func() tela.Node {
		tr = useTransport(0)
		return page(tr)
	}))
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
	// The knob and the range are repainted by a Value: no build for them.
	// The seek bar builds once a second (Beat, for aria-valuenow): three
	// times here, each of the bar's few components.
	if builds > 3*8 {
		t.Errorf("%d builds for 3 seconds of music over %d frames, want about one build of the seek bar per second", builds, frames)
	}
	if frames < 25 {
		t.Errorf("only %d frames for 30 ticks: the bar should be painted on (nearly) every tick", frames)
	}
	// Only the label's paragraph is laid out, once a second: moving the
	// knob and the range is a paint.
	if got := after.Layouts - before.Layouts; got > frames/2 {
		t.Errorf("%d layouts in %d frames: the knob and the range move by paint, want a layout only once a second", got, frames)
	}
	if got := after.Styles - before.Styles; got > 10 {
		t.Errorf("%d style computations: ticks should not restyle", got)
	}
}

// TestSecondsOnlyBuild checks that the clock builds nothing between the
// whole seconds: nine ticks of a tenth of a second move the knob, and
// nothing builds until the second boundary.
func TestSecondsOnlyBuild(t *testing.T) {
	slow(t)
	var tr *Transport
	a := mount(t, windowW, windowH, tela.C(func() tela.Node {
		tr = useTransport(0.05)
		return page(tr)
	}))
	st := a.Measure(func() {
		for range 9 { // 0.05 to 0.95: still the first second
			tela.Post(func() { tr.advance(0.1) })
			a.Advance(16 * time.Millisecond)
		}
	})
	if st.Builds != 0 {
		t.Errorf("%d builds inside one second of music, want none", st.Builds)
	}
	if w := fillWidth(a); w <= 0 {
		t.Errorf("the knob did not move: %v px", w)
	}
}

func TestPauseStopsTheClock(t *testing.T) {
	fast(t, 5*time.Millisecond, 10)
	a := mount(t, windowW, windowH, Player{})
	a.WaitFor("the bar to start moving", func() bool { return fillWidth(a) > 3 })
	a.ExpectCount(telatest.ByRole("button", "Pause"), 1) // the pause button while playing

	a.Click(telatest.ByRole("button", "Pause"))
	a.ExpectCount(telatest.ByRole("button", "Pause"), 0) // the play button now
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
	a := mount(t, windowW, windowH, Player{})
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
	// The current row shows the equalizer icon in place of its number, so
	// its name has no number; the others keep theirs.
	a.ExpectRole("button", "Slow Tide Marlowe 4:15")
	a.ExpectRole("button", "1 Midnight Drive Neon Harbor 3:42")
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
	a := mount(t, windowW, windowH, Player{})
	a.ExpectCount(seekBar, 1)
	bar := a.Bounds(seekBar)
	bx := bar.X
	if bar.Width != barW {
		t.Fatalf("the seek bar is %v px wide, the test assumes %v", bar.Width, barW)
	}
	y := bar.Y + bar.Height/2

	clickAt(a, bx+barW/2, y)
	a.ExpectText("1:51") // half of 3:42
	if w := fillWidth(a); w < barW/2-2 || w > barW/2+2 {
		t.Errorf("fill is %v px after seeking to the middle", w)
	}
	a.Expect(seekKnob, telatest.HasValue("1:51"))

	// Dragging: press at a quarter, release at four fifths. The slider
	// rounds to a whole second.
	dragBetween(a, bx+barW/4, y, bx+barW*4/5, y, 8)
	a.ExpectText("2:58") // four fifths of 222 s, 177.6 rounded
	// Dragging beyond the ends clamps.
	dragBetween(a, bx+100, y, bx-200, y, 6)
	a.ExpectText("0:00")
	dragBetween(a, bx+100, y, bx+barW+200, y, 6)
	a.ExpectText("3:41") // a seek never lands on the end of the track
	// The pointer cursor shows over the bar.
	a.MouseMove(bx+10, y)
	if got := a.Cursor(); got == "default" {
		t.Errorf("cursor over the bar is %v", got)
	}
}

// TestSeekWithTheKeyboard checks the knob as a slider: its value is the
// elapsed seconds, the arrows move it by one second and Home goes to the
// start.
func TestSeekWithTheKeyboard(t *testing.T) {
	slow(t)
	a := mount(t, windowW, windowH, Preview())
	a.Expect(seekKnob, telatest.HasValue("1:24"))
	a.Focus(seekKnob)
	a.Press("ArrowRight")
	a.ExpectText("1:25")
	a.Press("Home")
	a.ExpectText("0:00")
	a.Expect(seekKnob, telatest.HasValue("0:00"))
}

// TestSeekDoesNotBuild measures a drag of the seek bar: moving the pointer
// builds nothing, as the slider's knob and the range follow Elapsed by
// paint and the time label is a TextFn.
func TestSeekDoesNotBuild(t *testing.T) {
	slow(t)
	a := mount(t, windowW, windowH, Player{})
	bar := a.Bounds(seekBar)
	y := bar.Y + bar.Height/2
	a.MouseMove(bar.X+20, y)
	a.MouseDown() // builds once: the slider's Dragging state
	st := a.Measure(func() {
		for i := 1; i <= 20; i++ {
			a.MouseMove(bar.X+20+float32(i)*10, y)
		}
	})
	a.MouseUp()
	t.Logf("20 moves of a seek drag: %d builds, %d layouts, %d paints (%d frames)", st.Builds, st.Layouts, st.Paints, st.Frames)
	if st.Builds != 0 {
		t.Errorf("%d builds while dragging the seek bar, want none", st.Builds)
	}
	// The label's text changes with each second: a layout of its paragraph
	// at most per move.
	if st.Layouts > 20 {
		t.Errorf("%d layouts for 20 moves", st.Layouts)
	}
}

// TestVolume checks the mute toggle and the volume slider.
func TestVolume(t *testing.T) {
	slow(t)
	a := mount(t, windowW, windowH, Player{})
	vol, mute := telatest.ByRole("slider", "Volume"), telatest.ByRole("button", "Mute")
	a.Expect(vol, telatest.HasValue("70%"))
	a.Focus(vol)
	a.Press("ArrowRight")
	a.Expect(vol, telatest.HasValue("75%"))

	a.Expect(mute, telatest.Pressed(false))
	a.Click(mute)
	a.Expect(mute, telatest.Pressed(true))
	a.Expect(vol, telatest.Disabled(true))
	a.Click(mute)
	a.Expect(vol, telatest.Disabled(false))
}

// TestButtonTooltips checks that a transport button says what it does.
func TestButtonTooltips(t *testing.T) {
	slow(t)
	a := mount(t, windowW, windowH, Player{})
	a.Hover(telatest.ByRole("button", "Next track"))
	a.ExpectText("Next")
}

// TestAriaSnapshot pins what `tela inspect --aria` prints for the preview:
// the buttons by name, the seek bar and the volume as sliders.
func TestAriaSnapshot(t *testing.T) {
	slow(t)
	mount(t, windowW, windowH, Preview()).AriaGolden("player.aria.yml")
}

func TestTrackEndsAndPlaysNext(t *testing.T) {
	slow(t) // the test ticks the clock: 2 s of music per tick
	var tr *Transport
	a := mount(t, windowW, windowH, tela.C(func() tela.Node {
		tr = useTransport(0)
		return page(tr)
	}))
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
	a := mount(t, windowW, windowH, Player{}) // mounting runs the passive effects
	// The goroutine is spawned by the effect and may not be scheduled yet
	// on a loaded runner, so wait for it as for its exit below.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && clocks() <= before {
		time.Sleep(5 * time.Millisecond)
	}
	if clocks() <= before {
		t.Fatal("the clock goroutine did not start")
	}
	a.Close()
	deadline = time.Now().Add(2 * time.Second)
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
