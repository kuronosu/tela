package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1180, 900

// still is the page with the particles at rest: the one a test can ask to be
// idle.
var still = Gallery{Probe: -1, Still: true}

var (
	chartEl     = telatest.ByTestID("chart")
	particlesEl = telatest.ByTestID("particles")
	sketchEl    = telatest.ByTestID("sketch")

	play     = telatest.ByRole("button", "Play")
	pause    = telatest.ByRole("button", "Pause")
	undo     = telatest.ByRole("button", "Undo")
	wipe     = telatest.ByRole("button", "Clear")
	eraser   = telatest.ByRole("radio", "Eraser")
	pen      = telatest.ByRole("radio", "Pen")
	pink     = telatest.ByRole("radio", "Pink")
	thick    = telatest.ByRole("radio", "Thick")
	signups  = telatest.ByRole("radio", "Sign-ups")
	visitors = telatest.ByRole("radio", "Visitors")
)

// mount opens a root in the window of main (its theme and root class), in
// dev mode, where any dev message fails the test.
func mount(t *testing.T, root tela.Component, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = root
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.MountApp(t, app, opts...)
}

// watch gives the field's draws to a test, the last one's field.
func watch(t *testing.T) **field {
	t.Helper()
	var last *field
	drawn = func(f *field) { last = f }
	t.Cleanup(func() { drawn = nil })
	return &last
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {760, 1300}, {520, 1500}} {
		mount(t, Gallery{Probe: -1}, s[0], s[1])
		mount(t, Previews["Overview"].Root, s[0], s[1], telatest.Dark())
	}
}

// TestGolden is the preview at scale 1 and 2, light and dark, and a narrow
// window, where the two lower cards go one under the other.
func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		for _, dark := range []bool{false, true} {
			name, opts := fmt.Sprintf("canvas_x%g.png", scale), []telatest.Option{telatest.Scale(scale)}
			if dark {
				name, opts = fmt.Sprintf("canvas_dark_x%g.png", scale), append(opts, telatest.Dark())
			}
			t.Run(strings.TrimSuffix(name, ".png"), func(t *testing.T) {
				mount(t, Previews["Overview"].Root, windowW, windowH, opts...).Golden(name)
			})
		}
	}
	t.Run("narrow", func(t *testing.T) {
		mount(t, Previews["Overview"].Root, 640, 1380).Golden("canvas_narrow_x1.png")
	})
}

func TestAriaSnapshot(t *testing.T) {
	mount(t, Previews["Overview"].Root, windowW, windowH).AriaGolden("canvas.aria.yml")
}

// TestChartCursorBuildsNothing: the cursor is a Value that the draw reads,
// so following the pointer paints the canvas and builds no component.
func TestChartCursorBuildsNothing(t *testing.T) {
	app := mount(t, still, windowW, windowH, telatest.Dark())
	r := app.Bounds(chartEl)
	before := app.PixelAt(r.X+r.Width/2, r.Y+r.Height/2)
	st := app.Measure(func() {
		for i := range 10 {
			app.MouseMove(r.X+100+float32(i)*60, r.Y+60)
		}
	})
	if st.Builds != 0 {
		t.Errorf("moving the pointer over the chart built %d components, want none", st.Builds)
	}
	if st.Paints == 0 {
		t.Errorf("moving the pointer over the chart painted nothing: the cursor does not follow it")
	}
	app.MouseMove(r.X+r.Width/2, r.Y+60)
	if after := app.PixelAt(r.X+r.Width/2, r.Y+r.Height/2); after == before {
		t.Errorf("the pixel under the cursor is still %v: the cursor line is not drawn", after)
	}
	app.Golden("chart_hover_dark_x1.png")
	// Out of the chart the cursor goes, and the canvas is as it was.
	app.MouseMove(r.X-30, r.Y-30)
	if after := app.PixelAt(r.X+r.Width/2, r.Y+r.Height/2); after != before {
		t.Errorf("after the pointer left, the pixel is %v, want %v", after, before)
	}
	app.ExpectIdle()
}

// TestChartSeries: another series is on its way with a spring, then the app
// rests, and the chart says its values in text.
func TestChartSeries(t *testing.T) {
	app := mount(t, still, windowW, windowH, telatest.NoSettle())
	app.ExpectText("Visitors by month: Jan 8.2k, Feb 9.1k")
	app.Click(signups)
	app.Advance(40 * time.Millisecond)
	moving := false
	for _, p := range app.Pending() {
		moving = moving || p.Kind == telatest.PendingAnimation
	}
	if !moving {
		t.Errorf("40 ms after another series was chosen nothing is moving: %v", app.Pending())
	}
	app.Advance(5 * time.Second)
	app.ExpectIdle()
	app.ExpectText("Sign-ups by month: Jan 420, Feb 465")
	app.Expect(signups, telatest.Checked(true))
	app.Expect(visitors, telatest.Checked(false))
	app.ExpectText("+167%") // 420 to 1,120
}

// TestChartSeriesWithReducedMotion: the new series is there at once.
func TestChartSeriesWithReducedMotion(t *testing.T) {
	app := mount(t, still, windowW, windowH, telatest.NoSettle(), telatest.ReducedMotion())
	app.Click(signups)
	app.Advance(time.Millisecond)
	app.ExpectIdle()
}

// TestParticlesRunAndRest: a running field asks for a frame at every vsync,
// a paused one for none, and a field that starts paused (reduced motion) does
// not move until it is played.
func TestParticlesRunAndRest(t *testing.T) {
	last := watch(t)
	app := mount(t, Gallery{Probe: -1}, windowW, windowH)
	hasCanvas := func() bool {
		for _, p := range app.Pending() {
			if p.Kind == telatest.PendingCanvas {
				return true
			}
		}
		return false
	}
	if !hasCanvas() {
		t.Fatalf("a running field asks for no frames: %v", app.Pending())
	}
	app.Advance(time.Second)
	moved := (*last).ps[0]

	app.Click(pause)
	app.ExpectIdle() // nothing is pending: the window sleeps
	frames := app.Stats().Frames
	app.Advance(10 * time.Second)
	if got := app.Stats().Frames; got != frames {
		t.Errorf("a paused field ran %d frames in 10 s", got-frames)
	}
	if (*last).ps[0] != moved && hasCanvas() {
		t.Errorf("a paused field still moves")
	}
	app.ExpectText("Paused: no frames, no CPU.")

	app.Click(play)
	if !hasCanvas() {
		t.Errorf("a played field asks for no frames: %v", app.Pending())
	}
	app.Advance(time.Second)
	if (*last).ps[0] == moved {
		t.Errorf("a played field does not move")
	}
	app.ExpectText("Running")

	still := mount(t, Gallery{Probe: -1}, windowW, windowH, telatest.ReducedMotion())
	still.ExpectIdle()
	still.ExpectText("Paused: no frames, no CPU.")
	still.Advance(time.Second)
	still.ExpectIdle()
	still.Click(play)
	if got := still.Pending(); len(got) == 0 {
		t.Errorf("a field played under reduced motion asks for no frames")
	}
}

// TestParticlesFollowThePointer: the dots near the pointer are more once it
// has been over the field for a while, and they let go when it leaves.
func TestParticlesFollowThePointer(t *testing.T) {
	last := watch(t)
	app := mount(t, Gallery{Probe: -1}, windowW, windowH)
	r := app.Bounds(particlesEl)
	near := func(x, y float64) int {
		n := 0
		for _, p := range (*last).ps {
			if math.Hypot(p.x-x, p.y-y) < 120 {
				n++
			}
		}
		return n
	}
	// A point of the field that few dots are near at first.
	px, py := 0.8*float64(r.Width), 0.5*float64(r.Height)
	app.Advance(50 * time.Millisecond)
	before := near(px, py)
	app.MouseMove(r.X+float32(px), r.Y+float32(py))
	app.Advance(3 * time.Second)
	after := near(px, py)
	if after < before+6 {
		t.Errorf("the dots near the pointer went from %d to %d in 3 s: it does not pull", before, after)
	}
}

// TestParticlesGolden is the field a few seconds after the pointer came in,
// pressed.
func TestParticlesGolden(t *testing.T) {
	app := mount(t, Gallery{Probe: -1}, windowW, windowH, telatest.Dark(), telatest.NoSettle())
	r := app.Bounds(particlesEl)
	app.MouseMove(r.X+r.Width*0.6, r.Y+r.Height*0.5)
	app.MouseDown()
	app.Advance(1500 * time.Millisecond)
	app.Golden("particles_pull_dark_x1.png")
}

// TestParticlesFrameCost: a frame of the field costs no layout.
func TestParticlesFrameCost(t *testing.T) {
	app := mount(t, Gallery{Probe: -1}, windowW, windowH)
	st := app.Measure(func() { app.Advance(time.Second) })
	if st.Builds != 0 || st.Layouts != 0 {
		t.Errorf("a second of the field built %d components and laid out %d times, want none", st.Builds, st.Layouts)
	}
}

// indigo is the first color of the pen, in the light scheme.
var indigo = color.RGBA{79, 70, 229, 255}

// TestSketch draws, erases, undoes and clears.
func TestSketch(t *testing.T) {
	app := mount(t, still, windowW, windowH)
	r := app.Bounds(sketchEl)
	at := func(x, y float32) color.RGBA { return app.PixelAt(r.X+x, r.Y+y) }
	empty := at(120, 80)
	app.Expect(undo, telatest.Disabled(true))
	app.Expect(wipe, telatest.Disabled(true))

	line := func(y float32) {
		app.DragFromTo(tela.Point{X: r.X + 40, Y: r.Y + y}, tela.Point{X: r.X + 200, Y: r.Y + y})
	}
	line(80)
	if got := at(120, 80); got != indigo {
		t.Errorf("a line drawn with the pen is %v where it passed, want %v", got, indigo)
	}
	// The stroke is where the pointer was, to the pixel (6 px wide at y = 80).
	if got := at(120, 77.5); got != indigo {
		t.Errorf("the top edge of the line is %v, want %v", got, indigo)
	}
	if got := at(120, 76.5); got != empty {
		t.Errorf("a pixel above the line is %v: the stroke is lower than the pointer was", got)
	}
	if got := at(120, 83.5); got != empty {
		t.Errorf("a pixel below the line is %v: the stroke is higher than the pointer was", got)
	}
	if got := at(120, 40); got != empty && got != at(300, 40) {
		t.Errorf("the pad changed away from the line: %v", got)
	}
	app.Expect(undo, telatest.Disabled(false))
	app.ExpectIdle()

	// A second stroke builds nothing: the pad changes, the page does not.
	st := app.Measure(func() { line(120) })
	if st.Builds != 0 {
		t.Errorf("drawing a stroke built %d components, want none", st.Builds)
	}

	// The eraser takes the line away, and shows the pad behind it.
	app.Click(eraser)
	line(80)
	if got := at(120, 80); got != empty {
		t.Errorf("the eraser left %v where the line was, want the pad's %v", got, empty)
	}
	if got := at(120, 120); got != indigo {
		t.Errorf("the eraser took away the other line too: %v", got)
	}

	// Undo brings back the line; Ctrl+Z does the same.
	app.Click(undo)
	if got := at(120, 80); got != indigo {
		t.Errorf("after undoing the eraser the line is %v, want %v", got, indigo)
	}
	app.Press("ControlOrMeta+z") // the focus is in the card: the toggle that was clicked
	if got := at(120, 120); got == indigo {
		t.Errorf("Ctrl+Z did not take the second line back")
	}

	// Another color and brush.
	app.Click(pink)
	app.Click(thick)
	line(100)
	if got := at(120, 102); got != (color.RGBA{236, 72, 153, 255}) {
		t.Errorf("a thick pink line is %v, want pink-500", got)
	}

	app.Click(wipe)
	if got := at(120, 80); got != empty {
		t.Errorf("after Clear the pad is %v, want %v", got, empty)
	}
	app.Expect(undo, telatest.Disabled(true))
	app.ExpectIdle()
}

// TestSketchPicksThePenWithAColor: choosing a color while erasing goes back
// to drawing.
func TestSketchPicksThePenWithAColor(t *testing.T) {
	app := mount(t, Gallery{Probe: -1}, windowW, windowH)
	app.Click(eraser)
	app.Expect(eraser, telatest.Checked(true))
	app.Click(pink)
	app.Expect(pen, telatest.Checked(true))
}

// TestSketchDot: a click is a dot.
func TestSketchDot(t *testing.T) {
	app := mount(t, Gallery{Probe: -1}, windowW, windowH)
	r := app.Bounds(sketchEl)
	app.ClickAt(r.X+60, r.Y+60)
	if got := app.PixelAt(r.X+60, r.Y+60); got != indigo {
		t.Errorf("a click on the pad drew %v, want a dot of %v", got, indigo)
	}
}

// TestSketchByKeyboard: every control is reached with the keyboard.
func TestSketchByKeyboard(t *testing.T) {
	app := mount(t, Previews["Overview"].Root, windowW, windowH)
	app.Focus(eraser)
	app.Press("ArrowLeft")
	app.Expect(pen, telatest.Checked(true))
	app.Focus(undo)
	app.Press("Enter")
	app.Focus(wipe)
	app.Press("Space")
	app.Expect(undo, telatest.Disabled(true))
}

// TestIdle: the screen asks for no frames until something moves.
func TestIdle(t *testing.T) {
	mount(t, Previews["Overview"].Root, windowW, windowH).ExpectIdle()
	mount(t, still, windowW, windowH).ExpectIdle()
}

func TestAccessible(t *testing.T) {
	mount(t, Previews["Overview"].Root, windowW, windowH).ExpectAccessible()
}
