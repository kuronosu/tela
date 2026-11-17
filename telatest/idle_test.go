package telatest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The idle check (App.Pending, App.ExpectIdle; ADR-291).

func idleCounter() tela.Node {
	n, setN := tela.UseState(0)
	return Div(Class("p-4"),
		Button(Class("rounded bg-stone-800 px-3 py-1 text-white transition hover:bg-stone-700"),
			OnClick(func() { setN.Set(n + 1) }), Textf("Count %d", n)))
}

// silentTicker runs an interval that does nothing: no state changes, so
// nothing shows in Stats until the clock reaches it.
func silentTicker() tela.Node {
	tela.UseInterval(time.Second, func() {})
	return P(Text("tick"))
}

func pendingTimeout() tela.Node {
	shown, setShown := tela.UseState(true)
	tela.UseTimeout(3*time.Second, func() { setShown.Set(false) })
	return Div(If(shown, P(Text("Saved"))))
}

func spinner() tela.Node {
	return Div(Class("size-8 animate-spin rounded-full border-4 border-stone-300 border-t-stone-800"), AriaLabel("Loading"), Role("status"))
}

func focusedField() tela.Node {
	v, setV := tela.UseState("")
	return Input(Bind(v, setV), AutoFocus(true), AriaLabel("Name"))
}

func TestExpectIdlePasses(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, idleCounter, telatest.Size(300, 200))
	app.ExpectIdle()
	// After a hover and a click their transitions settle, and nothing is
	// left.
	app.Click(telatest.ByRole("button", "Count 0"))
	app.ExpectText("Count 1")
	app.ExpectIdle()
	if p := app.Pending(); len(p) != 0 {
		t.Errorf("Pending() = %q, want none", p)
	}
}

func TestExpectIdleNamesWhatIsPending(t *testing.T) {
	t.Parallel()
	t.Run("UseInterval", func(t *testing.T) {
		mustFail(t, func(tb testing.TB) {
			telatest.MountFunc(tb, silentTicker, telatest.Size(300, 200)).ExpectIdle()
		}, "ExpectIdle: the app is not idle", "a UseInterval every 1s in <", "telatest_test.silentTicker> (called at", "(idle_test.go:", "next in 1s")
	})
	t.Run("UseTimeout", func(t *testing.T) {
		mustFail(t, func(tb testing.TB) {
			telatest.MountFunc(tb, pendingTimeout, telatest.Size(300, 200)).ExpectIdle()
		}, "a UseTimeout of 3s in <", "telatest_test.pendingTimeout>", "next in 3s")
		// Once it fired, nothing is left.
		app := telatest.MountFunc(t, pendingTimeout, telatest.Size(300, 200))
		app.Advance(3 * time.Second)
		app.ExpectNoText("Saved")
		app.ExpectIdle()
	})
	t.Run("endless animation", func(t *testing.T) {
		mustFail(t, func(tb testing.TB) {
			telatest.MountFunc(tb, spinner, telatest.Size(300, 200)).ExpectIdle()
		}, "a frame request", "an endless animation")
	})
	t.Run("running transition", func(t *testing.T) {
		// Under NoSettle the hover's transition is still running.
		mustFail(t, func(tb testing.TB) {
			app := telatest.MountFunc(tb, idleCounter, telatest.Size(300, 200), telatest.NoSettle())
			app.Advance(time.Second)
			app.Hover(telatest.ByRole("button", "Count 0"))
			app.ExpectIdle()
		}, "a running transition or animation")
	})
	t.Run("caret blink", func(t *testing.T) {
		mustFail(t, func(tb testing.TB) {
			telatest.MountFunc(tb, focusedField, telatest.Size(300, 200)).ExpectIdle()
		}, "the caret of the focused text field, which blinks next in")
		// The caret stops blinking 10 s after the last input.
		app := telatest.MountFunc(t, focusedField, telatest.Size(300, 200))
		app.Advance(11 * time.Second)
		app.ExpectIdle()
	})
}

// TestPendingDoesNotMoveTheClock: reading what is pending settles the app
// but never moves the clock to a deadline.
func TestPendingDoesNotMoveTheClock(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, pendingTimeout, telatest.Size(300, 200))
	before := app.Now()
	for range 3 {
		p := app.Pending()
		if len(p) != 1 || !strings.Contains(p[0], "UseTimeout") {
			t.Fatalf("Pending() = %q, want the UseTimeout alone", p)
		}
	}
	if app.Now() != before {
		t.Errorf("Pending moved the clock from %v to %v", before, app.Now())
	}
}
