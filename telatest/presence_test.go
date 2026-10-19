package telatest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Enter and exit animations (h.Presence, starting:, h.Exit) under
// auto-wait and NoSettle.

// dialogPage opens a dialog that fades and scales in over 200 ms and out
// over 200 ms.
func dialogPage() tela.Node {
	open, setOpen := tela.UseState(false)
	return Div(Class("relative h-full p-4"),
		Button(Class("rounded bg-slate-900 px-3 py-1 text-white"), OnClick(func(tela.MouseEvent) { setOpen.Set(true) }), Text("Open")),
		Presence(If(open, Div(Key("dialog"), Role("dialog"), AriaLabel("Saved"),
			Class("absolute left-10 top-14 w-40 rounded-xl bg-indigo-600 p-4 text-white transition duration-200 ease-linear "+
				"opacity-100 scale-100 starting:opacity-0 starting:scale-50"),
			Exit("opacity-0 scale-50"),
			P(Text("All changes saved")),
			Button(Class("mt-2 rounded bg-white px-2 text-indigo-600"), OnClick(func(tela.MouseEvent) { setOpen.Set(false) }), Text("Close")),
		))),
	)
}

func TestPresenceAutoWait(t *testing.T) {
	app := telatest.Mount(t, tela.C(dialogPage), telatest.Size(240, 200))
	before := app.Now()
	app.Click(telatest.ByRole("button", "Open"))
	if d := app.Now() - before; d < 200*time.Millisecond {
		t.Errorf("Click returned after %v: the entrance was not waited for", d)
	}
	app.ExpectRole("dialog", "Saved")
	before = app.Now()
	app.Click(telatest.ByRole("button", "Close"))
	if d := app.Now() - before; d < 200*time.Millisecond {
		t.Errorf("Click returned after %v: the exit was not waited for", d)
	}
	// Settled: the dialog is gone, from the semantics tree and the pixels.
	if n := app.Count(telatest.ByRole("dialog", "")); n != 0 {
		t.Errorf("%d dialogs after the exit", n)
	}
	if px := app.Screenshot().RGBAAt(60, 80); px.R != 255 || px.G != 255 || px.B != 255 {
		t.Errorf("the dialog is still drawn: %v", px)
	}
}

func TestNoSettleCapturesAnimations(t *testing.T) {
	app := telatest.Mount(t, tela.C(dialogPage), telatest.Size(240, 200), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Open"))
	start := app.Now()
	// The click's frame mounted the dialog in its starting style: nothing
	// shows yet.
	if px := app.Screenshot().RGBAAt(120, 90); px.R != 255 || px.G != 255 {
		t.Errorf("the dialog shows before its entrance ran: %v", px)
	}
	app.Advance(100 * time.Millisecond)
	if app.Now() != start+100*time.Millisecond {
		t.Errorf("the screenshot moved the clock to %v", app.Now()-start)
	}
	app.Golden("dialog_enter_100ms.png")
	mid := app.Screenshot().RGBAAt(120, 90)
	if mid.R == 255 || mid.B == mid.R { // half-transparent indigo over white
		t.Errorf("halfway in: %v", mid)
	}
	// The exit: Golden shows it halfway; AriaSnapshot settles the app (it
	// keeps its auto-wait), so the dialog is gone by then.
	app.Advance(200 * time.Millisecond)
	app.Click(telatest.ByRole("button", "Close"))
	app.Advance(100 * time.Millisecond)
	app.Golden("dialog_exit_100ms.png")
	if snap := app.AriaSnapshot(); strings.Contains(snap, "dialog") {
		t.Errorf("the exiting dialog is in the aria snapshot:\n%s", snap)
	}
	if n := app.Count(telatest.ByRole("dialog", "")); n != 0 {
		t.Errorf("%d dialogs", n)
	}
}

func TestNoSettleSpring(t *testing.T) {
	var x *tela.Value[float32]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		target, setTarget := tela.UseState(float32(0))
		x = tela.UseSpring(float32(0), target, tela.Stiffness(200), tela.Damping(20))
		return Div(Class("p-4"),
			Button(OnClick(func(tela.MouseEvent) { setTarget.Set(100) }), Text("Go")),
			Div(Class("size-8 bg-black"), Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 }))))
	}), telatest.Size(200, 100), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Go"))
	app.Advance(50 * time.Millisecond)
	a := x.Get()
	_ = app.Screenshot()
	if x.Get() != a || !(a > 0 && a < 100) {
		t.Errorf("the screenshot moved the spring: %v then %v", a, x.Get())
	}
	app.ExpectText("Go") // expectations still settle
	if x.Get() != 100 {
		t.Errorf("after an expectation: %v", x.Get())
	}
}
