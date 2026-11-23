package telatest_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// A wait for something a long timer of the app's clock brings advances the
// clock to the timer's deadline when nothing else is due, instead of one
// frame per try: the number of tries, and so the time it takes, does not
// depend on the machine's speed.
func TestWaitJumpsToTheNextDeadline(t *testing.T) {
	later := func() tela.Node {
		done, setDone := tela.UseState(false)
		tela.UseTimeout(10*time.Second, func() { setDone.Set(true) })
		return Div(If(done, P(Text("Done"))), If(!done, P(Text("Waiting"))))
	}
	for _, c := range []struct {
		name string
		opts []telatest.Option
	}{{"settle", nil}, {"no settle", []telatest.Option{telatest.NoSettle()}}} {
		t.Run(c.name, func(t *testing.T) {
			app := telatest.MountFunc(t, later, append(c.opts, telatest.Size(200, 100))...)
			tries := 0
			start := app.Now()
			app.WaitFor("the timer", func() bool {
				tries++
				return app.Count(telatest.ByText("Done")) > 0
			})
			if tries > 5 {
				t.Errorf("the wait for a 10 s timer took %d tries; want a jump to its deadline", tries)
			}
			if d := app.Now() - start; d < 10*time.Second || d > 11*time.Second {
				t.Errorf("the clock moved %v, want about 10 s", d)
			}
			app.ExpectText("Done")
		})
	}
}

// A frame owed before the deadline still runs at its own time: an
// animation running toward the timer is not skipped.
func TestWaitJumpKeepsFramesOnTheWay(t *testing.T) {
	var frames int
	app := telatest.MountFunc(t, func() tela.Node {
		done, setDone := tela.UseState(false)
		tela.UseTimeout(2*time.Second, func() { setDone.Set(true) })
		frames++
		return Div(If(done, P(Text("Done"))))
	}, telatest.Size(200, 100))
	app.ExpectText("Done")
	if frames != 2 {
		t.Errorf("the component built %d times, want 2 (mount and the timer)", frames)
	}
}
