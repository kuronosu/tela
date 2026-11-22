package telatest_test

import (
	"context"
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// A wait for what a goroutine brings (a UseGo result, a tela.Post) lets
// real time pass, not the app's clock, when nothing on that clock is due:
// the caret of a focused field is in the same blink phase after the wait
// however long the goroutine took, so a golden taken after it does not
// depend on the machine's speed. notes' TestGoldenUpdateNotice caught the
// caret in its off phase under load: a try that failed moved the clock to
// the caret's next blink.
func TestWaitForAGoroutineKeepsTheCaretPhase(t *testing.T) {
	release := make(chan struct{})
	app := telatest.MountFunc(t, func() tela.Node {
		msg, setMsg := tela.UseState("")
		tela.UseGo(func(ctx context.Context) {
			select {
			case <-release:
				setMsg.Set("Loaded")
			case <-ctx.Done():
			}
		}, tela.Once)
		return Div(Class("flex flex-col gap-2 p-2"),
			Input(AriaLabel("Name"), AutoFocus(true), Class("w-40 h-8 border")),
			If(msg != "", P(Text(msg))),
		)
	}, telatest.Size(200, 100))
	field := app.Bounds(telatest.ByRole("textbox", "Name"))
	before := app.Screenshot()
	start := app.Now()
	go func() {
		time.Sleep(30 * time.Millisecond)
		close(release)
	}()
	app.ExpectText("Loaded")
	if d := app.Now() - start; d > 100*time.Millisecond {
		t.Errorf("waiting for the goroutine moved the app's clock %v: only the frames of its result should run", d)
	}
	after := app.Screenshot()
	r := image.Rect(int(field.X), int(field.Y), int(field.X+field.Width), int(field.Y+field.Height))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if b, a := before.RGBAAt(x, y), after.RGBAAt(x, y); b != a {
				t.Fatalf("the field changed during the wait at (%d, %d): %v, then %v (the caret blinked)", x, y, b, a)
			}
		}
	}
}

// A wait for a long timer jumps to the timer's deadline even while a
// focused field's caret blinks: the caret's blinks are not what a check
// waits for, and they run on the way.
func TestWaitJumpsPastTheCaretBlinks(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		done, setDone := tela.UseState(false)
		tela.UseTimeout(5*time.Second, func() { setDone.Set(true) })
		return Div(Input(AriaLabel("Name"), AutoFocus(true)), If(done, P(Text("Done"))))
	}, telatest.Size(200, 100))
	app.Expect(telatest.ByRole("textbox", "Name"), telatest.Focused(true))
	tries := 0
	app.WaitFor("the timer", func() bool {
		tries++
		return app.Count(telatest.ByText("Done")) > 0
	})
	if tries > 3 {
		t.Errorf("the wait for a 5 s timer took %d tries; want a jump to its deadline, not to each blink of the caret", tries)
	}
}
