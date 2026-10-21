package telatest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// ADR-097: under NoSettle the queries handle only the work due, like the
// events of an action; they never run the clock for animations.

// springPage runs a spring from 0 to 100 when "Go" is clicked and shows
// whether it arrived, through a TextFn, which a Value change updates
// without rebuilding.
func springPage(x **tela.Value[float32]) func() tela.Node {
	return func() tela.Node {
		target, setTarget := tela.UseState(float32(0))
		v := tela.UseSpring(target, tela.Stiffness(200), tela.Damping(20))
		*x = v
		return Div(Class("p-4"),
			Button(OnClick(func() { setTarget.Set(100) }), Text("Go")),
			Span(TextFn(func() string {
				if v.Get() >= 99.5 {
					return "arrived"
				}
				return "moving"
			})),
			Div(Class("size-8 bg-black"), Style(tela.TranslateFn(func() (float32, float32) { return v.Get(), 0 }))))
	}
}

func TestNoSettleQueriesDoNotAdvance(t *testing.T) {
	var x *tela.Value[float32]
	app := telatest.Mount(t, tela.C(springPage(&x)), telatest.Size(300, 100), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Go"))
	app.Advance(50 * time.Millisecond)
	pos, now := x.Get(), app.Now()
	if !(pos > 0 && pos < 99) {
		t.Fatalf("the spring is at %v after 50 ms, want it running", pos)
	}
	still := func(what string) {
		t.Helper()
		if x.Get() != pos || app.Now() != now {
			t.Errorf("%s moved the clock or the spring: %v at %v, was %v at %v", what, x.Get(), app.Now(), pos, now)
		}
	}

	if n := app.Count(telatest.ByRole("button", "Go")); n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}
	still("Count")
	if snap := app.AriaSnapshot(); !strings.Contains(snap, `button "Go"`) || !strings.Contains(snap, "moving") {
		t.Errorf("AriaSnapshot = %q, want the button and the text \"moving\"", snap)
	}
	still("AriaSnapshot")
	_ = app.Stats()
	still("Stats")
	_ = app.Cursor()
	still("Cursor")
	_ = app.Bounds(telatest.ByRole("button", "Go"))
	still("Bounds")
	app.ExpectText("moving")
	still("ExpectText")
	app.ExpectNoText("arrived")
	still("ExpectNoText")
	app.ExpectCount(telatest.ByRole("button", "Go"), 1)
	still("ExpectCount")
	app.ExpectRole("button", "Go")
	still("ExpectRole")
	app.Expect(telatest.ByRole("button", "Go"), telatest.Disabled(false))
	still("Expect")
	app.ExpectAria("- button \"Go\"\n- text: moving")
	still("ExpectAria")
	_ = app.Screenshot()
	still("Screenshot")
}

// An expectation that fails still polls: the clock moves a frame at a time
// between tries until the expectation holds.
func TestNoSettleExpectationsKeepPolling(t *testing.T) {
	var x *tela.Value[float32]
	app := telatest.Mount(t, tela.C(springPage(&x)), telatest.Size(300, 100), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Go"))
	app.Advance(50 * time.Millisecond)
	now := app.Now()
	app.ExpectText("arrived")
	if app.Now() <= now || x.Get() < 99.5 {
		t.Errorf("after ExpectText(\"arrived\"): clock %v (was %v), spring %v; polling should have run the spring to its end", app.Now(), now, x.Get())
	}
	app.WaitFor("the spring at rest", func() bool { return x.Get() == 100 })
}

// The wait before an action still settles the app, as ADR-080 says.
func TestNoSettleActionsStillSettleBeforeActing(t *testing.T) {
	var x *tela.Value[float32]
	app := telatest.Mount(t, tela.C(springPage(&x)), telatest.Size(300, 100), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Go"))
	app.Advance(50 * time.Millisecond)
	app.Hover(telatest.ByRole("button", "Go"))
	if x.Get() != 100 {
		t.Errorf("the spring is at %v after an action's wait, want it settled at 100", x.Get())
	}
}

// Without NoSettle the queries settle the app, as before.
func TestSettleQueriesSettleWithoutNoSettle(t *testing.T) {
	var x *tela.Value[float32]
	app := telatest.Mount(t, tela.C(springPage(&x)), telatest.Size(300, 100))
	app.Click(telatest.ByRole("button", "Go"))
	if got := app.Count(telatest.ByRole("button", "Go")); got != 1 || x.Get() != 100 {
		t.Errorf("Count = %d, spring %v, want 1 and 100", got, x.Get())
	}
}
