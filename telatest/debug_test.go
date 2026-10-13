package telatest_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// debugScene is a small settings card: boxes with margins, borders and
// padding, a button that changes a label, a scroll container and a few
// roles.
func debugScene() tela.Node {
	on, setOn := tela.UseState(false)
	rows := make([]string, 8)
	for i := range rows {
		rows[i] = fmt.Sprintf("Item %d", i+1)
	}
	label := "Notifications off"
	if on {
		label = "Notifications on"
	}
	return Div(Class("flex flex-col gap-3 p-4 bg-slate-50 text-slate-900"),
		H1(Class("text-xl font-semibold"), Text("Settings")),
		Div(Class("flex items-center gap-3 rounded-lg border border-slate-300 bg-white p-3 m-1"),
			Button(Class("rounded bg-indigo-500 px-3 py-1 text-white"),
				OnClick(func(tela.MouseEvent) { setOn.Set(!on) }), Text("Save")),
			Span(Text(label))),
		Div(Class("h-24 overflow-y-auto rounded border border-slate-300 bg-white"),
			Map(rows, func(s string) tela.Node {
				return Div(Key(s), Class("px-3 py-1 text-sm"), Text(s))
			})),
	)
}

// The goldens of the overlays at scales 1 and 2 (testdata/debug_*.png).
func TestDebugGoldens(t *testing.T) {
	// A clock that makes frames take 2, 20 and 40 ms in turn: a green, an
	// amber and a red bar.
	scripted := func() func() time.Duration {
		var calls int
		var now time.Duration
		durs := []time.Duration{2 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 8 * time.Millisecond}
		return func() time.Duration {
			calls++
			if calls%2 == 0 {
				now += durs[(calls/2)%len(durs)]
			}
			return now
		}
	}
	cases := []struct {
		flag string
		run  func(app *telatest.App)
	}{
		{"layout", func(app *telatest.App) {
			app.Hover(telatest.ByRole("button", "Save"))
		}},
		{"layers", func(app *telatest.App) {
			// A scroll is a paint-only frame: only the scrollbars are
			// recorded again, so only that layer is marked "rec".
			app.Scroll(telatest.ByClass("overflow-y-auto"), 0, 30)
			app.Advance(100 * time.Millisecond)
		}},
		{"semantics", func(app *telatest.App) {}},
		{"fps", func(app *telatest.App) {
			for range 3 {
				app.Click(telatest.ByRole("button", "Save"))
				app.Advance(60 * time.Millisecond)
			}
			app.Advance(250 * time.Millisecond)
		}},
		{"paint-flash", func(app *telatest.App) {
			app.Advance(500 * time.Millisecond) // the mount's flash ends
			app.Click(telatest.ByRole("button", "Save"))
			app.Advance(100 * time.Millisecond) // the label's flash, a third gone
		}},
	}
	for _, c := range cases {
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("%s_x%g", c.flag, scale), func(t *testing.T) {
				app := telatest.Mount(t, tela.C(debugScene), telatest.Size(320, 240), telatest.Scale(scale),
					telatest.Debug(c.flag), telatest.DebugClock(scripted()), telatest.InstantScroll())
				c.run(app)
				app.Golden(fmt.Sprintf("debug_%s_x%g.png", c.flag, scale))
			})
		}
	}
}

// TELA_DEBUG is ignored unless the test opts in, so goldens never depend on
// the environment.
func TestDebugIgnoresEnv(t *testing.T) {
	t.Setenv("TELA_DEBUG", "all")
	opts := []telatest.Option{telatest.Size(320, 240), telatest.Scale(2)}
	plain := func() []byte {
		return telatest.Mount(t, tela.C(debugScene), opts...).Screenshot().Pix
	}
	rendered, err := telatest.Render(tela.C(debugScene), opts...)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain(), rendered.Pix) {
		t.Error("with TELA_DEBUG=all, Mount drew overlays")
	}
	with := func() []byte {
		o := append([]telatest.Option{telatest.Debug("layers")}, opts...)
		return telatest.Mount(t, tela.C(debugScene), o...).Screenshot().Pix
	}
	envOn := with()
	t.Setenv("TELA_DEBUG", "")
	if !bytes.Equal(envOn, with()) {
		t.Error("an explicit Debug option depends on TELA_DEBUG")
	}
	if bytes.Equal(envOn, rendered.Pix) {
		t.Error("Debug(\"layers\") drew nothing")
	}
}

func TestDebugOptionErrors(t *testing.T) {
	_, err := telatest.Render(tela.C(debugScene), telatest.Debug("layot"))
	if err == nil {
		t.Fatal("an unknown overlay was accepted")
	}
	for _, want := range []string{`unknown debug flag "layot"`, `probably "layout"`, "valid flags: layout, paint-flash, layers, fps, semantics"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	// "all" and a list are accepted; Render draws them.
	plain, err := telatest.Render(tela.C(debugScene), telatest.Size(200, 150))
	if err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{{"all"}, {"layout", "fps"}, {"semantics,layers"}} {
		img, err := telatest.Render(tela.C(debugScene), telatest.Size(200, 150), telatest.Debug(flags...))
		if err != nil {
			t.Fatalf("Debug(%v): %v", flags, err)
		}
		if bytes.Equal(img.Pix, plain.Pix) {
			t.Errorf("Render with Debug(%v) drew no overlay", flags)
		}
	}
}

// The fps meter and a fading flash are not work an action waits for:
// actions return as soon as the app is quiet, the app is not reported as
// busy, and the clock does not run away.
func TestDebugDoesNotKeepActionsWaiting(t *testing.T) {
	for _, flag := range []string{"fps", "paint-flash", "all"} {
		app := telatest.Mount(t, tela.C(debugScene), telatest.Size(320, 240), telatest.Debug(flag))
		before := app.Now()
		for range 5 {
			app.Click(telatest.ByRole("button", "Save"))
		}
		app.ExpectText("Notifications on")
		if d := app.Now() - before; d > time.Second {
			t.Errorf("%s: five clicks took %v of app time", flag, d)
		}
		// The meter keeps drawing while time passes.
		if flag == "fps" {
			a := app.Screenshot()
			app.Advance(100 * time.Millisecond)
			if bytes.Equal(a.Pix, app.Screenshot().Pix) {
				t.Error("the fps meter did not change in 100 ms")
			}
		}
	}
}

// CheckDamage compares the window with a full rendering of the app alone,
// so it leaves overlays out: an app with overlays and CheckDamage still
// runs.
func TestDebugWithCheckDamage(t *testing.T) {
	app := telatest.Mount(t, tela.C(debugScene), telatest.Size(320, 240), telatest.Debug("all"), telatest.CheckDamage())
	app.Click(telatest.ByRole("button", "Save"))
	app.Scroll(telatest.ByClass("overflow-y-auto"), 0, 30)
	app.Advance(time.Second)
}
