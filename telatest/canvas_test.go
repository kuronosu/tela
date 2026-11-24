package telatest_test

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// spinnerCanvas turns a hand, a turn per second, with a button beside it.
// times are the clock at each draw.
func spinnerCanvas(times *[]time.Duration) tela.Node {
	return tela.C(func() tela.Node {
		n, setN := tela.UseState(0)
		open, setOpen := tela.UseState(false)
		return Div(Class("flex items-center gap-4 p-4"),
			Canvas(func(c *tela.Canvas) {
				*times = append(*times, c.Time)
				c.Translate(c.Width/2, c.Height/2)
				c.LineWidth, c.LineCap = 6, tela.CapRound
				c.BeginPath()
				c.Arc(0, 0, c.Width/2-3, 0, 2*math.Pi)
				c.GlobalAlpha = 0.2
				c.Stroke()
				c.GlobalAlpha = 1
				c.Rotate(c.Time.Seconds() * 2 * math.Pi)
				c.BeginPath()
				c.MoveTo(0, 0)
				c.LineTo(c.Width/2-9, 0)
				c.Stroke()
				c.RequestFrame()
			}, Class("size-24 stroke-indigo-600"), AriaLabel("Clock"), TestID("clock")),
			Button(Class("rounded-md bg-slate-900 px-3 py-1 text-white"), OnClick(func() { setN.Set(n + 1) }), Textf("Clicked %d", n)),
			Button(OnClick(func() { setOpen.Set(true) }), Text("Open")),
			If(open, Div(Class("size-8 bg-rose-500 starting:opacity-0 transition-opacity duration-200"))))
	})
}

// Settling does not wait for a canvas that asks for frames for ever, Advance
// moves it a draw per frame, and what it shows depends on the clock alone.
func TestCanvasLoop(t *testing.T) {
	const frame = time.Second / 60
	var times []time.Duration
	f := mustPass(t, func(tb testing.TB) {
		app := telatest.Mount(tb, spinnerCanvas(&times), telatest.Size(400, 130))
		if len(times) != 1 || times[0] != 0 {
			t.Errorf("draws at the mount: %v, want one at 0", times)
		}
		app.Golden("canvas_loop_0.png")

		times = nil
		app.Advance(125 * time.Millisecond)
		want := []time.Duration{frame, 2 * frame, 3 * frame, 4 * frame, 5 * frame, 6 * frame, 7 * frame}
		if !slices.Equal(times, want) {
			t.Errorf("draws in 125 ms: %v, want %v", times, want)
		}
		app.Golden("canvas_loop_125.png")

		// An action runs the frames its changes need, and no more.
		times = nil
		app.Click(telatest.ByRole("button", "Clicked 0"))
		app.ExpectText("Clicked 1")
		if len(times) == 0 || len(times) > 4 {
			t.Errorf("%d draws for a click", len(times))
		}
		// Settling moves it with the animations it waits for.
		times = nil
		app.Click(telatest.ByRole("button", "Open"))
		if n := len(times); n < 11 || n > 16 {
			t.Errorf("%d draws while a transition of 200 ms settles", n)
		}

		// What keeps the app awake is named.
		var found []string
		for _, p := range app.Pending() {
			if p.Kind == telatest.PendingCanvas {
				found = append(found, p.String())
			}
		}
		if len(found) != 1 || !strings.Contains(found[0], "Canvas.RequestFrame") {
			t.Errorf("Pending = %q, want one canvas item", found)
		}
	})
	for _, l := range f.logs {
		if strings.Contains(l, "requesting frames") || strings.Contains(l, "kept handling work") {
			t.Errorf("settling gave up on the loop: %s", l)
		}
	}
	if got := telatest.PendingCanvas.String(); got != "canvas" {
		t.Errorf("PendingCanvas = %q", got)
	}

	mustFail(t, func(tb testing.TB) {
		telatest.Mount(tb, spinnerCanvas(&times), telatest.Size(400, 130)).ExpectIdle()
	}, "not idle", "Canvas.RequestFrame")
}

// A canvas that asks for nothing is idle, and so is one that is not shown.
func TestCanvasIdle(t *testing.T) {
	var draws int
	app := telatest.Mount(t, Canvas(func(c *tela.Canvas) { draws++; c.FillRect(0, 0, 10, 10) }))
	app.ExpectIdle()
	if st := app.Measure(func() { app.Advance(5 * time.Second) }); st.Frames != 0 || draws != 1 {
		t.Errorf("%d frames and %d draws at rest", st.Frames, draws)
	}

	var times []time.Duration
	hidden := telatest.Mount(t, Div(Class("h-20 overflow-hidden"), Div(Class("h-40")), spinnerCanvas(&times)), telatest.Size(400, 200))
	hidden.ExpectIdle()
	if st := hidden.Measure(func() { hidden.Advance(time.Second) }); st.Frames != 0 || len(times) > 1 {
		t.Errorf("%d frames and %d draws of a canvas that is not shown", st.Frames, len(times))
	}
}

// NoSettle leaves it where it is.
func TestCanvasNoSettle(t *testing.T) {
	var times []time.Duration
	app := telatest.Mount(t, spinnerCanvas(&times), telatest.Size(400, 130), telatest.NoSettle())
	if len(times) != 1 {
		t.Errorf("%d draws at the mount", len(times))
	}
	first := app.Screenshot()
	if again := app.Screenshot(); !slices.Equal(first.Pix, again.Pix) || len(times) != 1 {
		t.Errorf("a screenshot moved it: %d draws", len(times))
	}
	app.Advance(50 * time.Millisecond)
	if len(times) != 4 {
		t.Errorf("draws after 50 ms: %v", times)
	}
	app.ExpectText("Clicked 0")
	if slices.Equal(first.Pix, app.Screenshot().Pix) {
		t.Error("it did not move")
	}
}

// Two runs draw the same pictures.
func TestCanvasDeterministic(t *testing.T) {
	var films [2][]telatest.FilmFrame
	for i := range films {
		var times []time.Duration
		app := telatest.Mount(t, spinnerCanvas(&times), telatest.Size(200, 130))
		films[i] = app.Film(300*time.Millisecond, 100*time.Millisecond, nil)
	}
	for k := range films[0] {
		if !slices.Equal(films[0][k].Image.Pix, films[1][k].Image.Pix) {
			t.Errorf("frame %d differs", k)
		}
		if k > 0 && slices.Equal(films[0][k].Image.Pix, films[0][k-1].Image.Pix) {
			t.Errorf("frame %d is the one before", k)
		}
	}
}

// The headless entry points draw it once, at time 0.
func TestCanvasRender(t *testing.T) {
	var times []time.Duration
	if _, err := telatest.Render(spinnerCanvas(&times), telatest.Size(400, 130)); err != nil {
		t.Fatal(err)
	}
	if len(times) != 1 || times[0] != 0 {
		t.Errorf("draws %v, want one at 0", times)
	}
	// A panic of the function is an error, as any panic of the tree.
	_, err := telatest.Render(Canvas(func(c *tela.Canvas) { c.FillStyle = c.Color("primry") }))
	if err == nil || !strings.Contains(err.Error(), `did you mean "primary"?`) {
		t.Errorf("error %v", err)
	}
}

// The film of a canvas that asks for frames.
func ExampleApp_Advance_canvas() {
	var t0 time.Duration
	clock := Canvas(func(c *tela.Canvas) {
		t0 = c.Time
		c.RequestFrame()
	}, AriaLabel("Clock"))
	app := telatest.Mount(t, clock)
	app.Advance(time.Second / 2)
	fmt.Println(t0.Round(time.Millisecond))
	// Output: 500ms
}
