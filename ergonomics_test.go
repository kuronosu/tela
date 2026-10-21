package tela_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Tests of the phase-7 ergonomics of package tela (ADR-091): Toggle,
// keyed C, CP and Memo, Easing, CubicBezier, UsePresenceTween, MapValue.

func recovered(f func()) (msg string) {
	defer func() { msg = fmt.Sprint(recover()) }()
	f()
	return "no panic"
}

func TestToggleCountsTwoClicksInOneFrame(t *testing.T) {
	var (
		toggle func()
		set    tela.Setter[bool]
	)
	page := func() tela.Node {
		open, setOpen := tela.UseState(false)
		set = setOpen
		toggle = tela.Toggle(setOpen)
		return Div(Button(OnClick(toggle), Text("Menu")), If(open, P(Text("open"))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(200, 100))
	// Two events in one frame: Update chains them, so the state is back
	// where it started.
	toggle()
	toggle()
	app.Advance(32 * time.Millisecond)
	app.ExpectNoText("open")
	// Three of them leave it open; so does a click.
	toggle()
	toggle()
	toggle()
	app.ExpectText("open")
	app.Click(telatest.ByRole("button", "Menu"))
	app.ExpectNoText("open")
	// What Set(!open) would have done with the same two events.
	open := false
	set.Set(!open)
	set.Set(!open)
	app.ExpectText("open")
	if msg := recovered(func() { tela.Toggle(tela.Setter[bool]{}) }); !strings.Contains(msg, "Toggle(zero Setter)") {
		t.Errorf("Toggle(zero Setter): %q", msg)
	}
}

type rowProps struct {
	ID   int
	Name string
}

func countingRow(p rowProps) tela.Node {
	n, setN := tela.UseState(0)
	return Button(Textf("%s:%d", p.Name, n), OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }))
}

func TestKeyedComponents(t *testing.T) {
	for name, mk := range map[string]func(p rowProps) tela.Node{
		"CP": func(p rowProps) tela.Node { return tela.CP(countingRow, p, Key(p.ID)) },
		"Memo": func(p rowProps) tela.Node {
			return tela.Memo(countingRow, p, func(a, b rowProps) bool { return a == b }, Key(p.ID))
		},
		"C": func(p rowProps) tela.Node {
			return tela.C(func() tela.Node {
				n, setN := tela.UseState(0)
				return Button(Textf("%s:%d", p.Name, n), OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }))
			}, Key(p.ID))
		},
	} {
		t.Run(name, func(t *testing.T) {
			var set tela.Setter[[]rowProps]
			page := func() tela.Node {
				rows, s := tela.UseState([]rowProps{{1, "a"}, {2, "b"}, {3, "c"}})
				set = s
				nodes := make([]tela.Node, len(rows))
				for i, r := range rows {
					nodes[i] = mk(r)
				}
				return Div(Nodes(nodes))
			}
			app := telatest.MountFunc(t, page, telatest.Size(200, 200))
			app.Click(telatest.ByText("c:0"))
			app.Click(telatest.ByText("c:1"))
			app.Click(telatest.ByText("a:0"))
			set.Set([]rowProps{{3, "c"}, {1, "a"}, {2, "b"}})
			app.ExpectAria(`
				- button "c:2"
				- button "a:1"
				- button "b:0"`)
		})
	}
}

func TestKeyedComponentsTakeOnlyKey(t *testing.T) {
	f := func() tela.Node { return Div() }
	n := tela.C(f, Key("k"))
	if k, ok := n.Key(); !ok || k != "k" {
		t.Errorf("C(f, Key(k)) has key %v, %v", k, ok)
	}
	if k, ok := tela.CP(countingRow, rowProps{}, If(true, Key(7))).Key(); !ok || k != 7 {
		t.Errorf("CP with If(true, Key(7)) has key %v, %v", k, ok)
	}
	if _, ok := tela.CP(countingRow, rowProps{}, If(false, Key(7))).Key(); ok {
		t.Error("CP with If(false, Key(7)) has a key")
	}
	if _, ok := tela.C(f).Key(); ok {
		t.Error("C(f) has a key")
	}
	for name, f := range map[string]func(){
		"C":    func() { tela.C(f, Class("x")) },
		"CP":   func() { tela.CP(countingRow, rowProps{}, Class("x")) },
		"Memo": func() { tela.Memo(countingRow, rowProps{}, func(a, b rowProps) bool { return true }, Class("x")) },
	} {
		if msg := recovered(f); !strings.Contains(msg, name+" takes only Key as its last argument, not Class") {
			t.Errorf("%s with Class: %q", name, msg)
		}
	}
}

func TestEasing(t *testing.T) {
	if tela.EaseLinear != "linear" || tela.EaseIn != "ease-in" || tela.EaseOut != "ease-out" || tela.EaseInOut != "ease-in-out" {
		t.Errorf("easing constants: %q %q %q %q", tela.EaseLinear, tela.EaseIn, tela.EaseOut, tela.EaseInOut)
	}
	for _, c := range []struct {
		got  tela.Easing
		want string
	}{
		{tela.CubicBezier(0, 0, 0.2, 1), "cubic-bezier(0, 0, 0.2, 1)"},
		{tela.CubicBezier(0.25, 0.1, 0.25, 1), "cubic-bezier(0.25, 0.1, 0.25, 1)"},
		{tela.CubicBezier(0.34, 1.56, 0.64, 1), "cubic-bezier(0.34, 1.56, 0.64, 1)"},
		{tela.CubicBezier(0.4, 0, 0.2, 1), "cubic-bezier(0.4, 0, 0.2, 1)"},
	} {
		if string(c.got) != c.want {
			t.Errorf("CubicBezier: %q, want %q", c.got, c.want)
		}
	}
	// String literals, variables of type Easing and CubicBezier all drive a
	// tween, in dev mode (an invalid one panics there).
	var half = func(e tela.Easing) float32 {
		var v *tela.Value[float32]
		app := telatest.MountFunc(t, func() tela.Node {
			v = tela.UseValue[float32](0)
			return Div(Class("size-4"), Style(tela.OpacityFn(v.Get)))
		}, telatest.NoSettle())
		tela.Animate(v, 100, tela.Duration(100*time.Millisecond), tela.Ease(e))
		app.Advance(83 * time.Millisecond) // the tween starts at the next frame: about half of it
		return v.Get()
	}
	linear, literal := half(tela.EaseLinear), half("linear")
	if linear != literal || linear < 40 || linear > 60 {
		t.Errorf("linear tween at about half: %v and %v", linear, literal)
	}
	if in, out := half(tela.EaseIn), half(tela.EaseOut); !(in < linear && linear < out) {
		t.Errorf("ease-in %v, linear %v, ease-out %v: want them increasing", in, linear, out)
	}
	if got := half(tela.CubicBezier(0, 0, 1, 1)); got != linear {
		t.Errorf("cubic-bezier(0, 0, 1, 1) at %v, linear at %v", got, linear)
	}
	if got := half(tela.Easing("cubic-bezier(0, 0, 1, 1)")); got != linear {
		t.Errorf("the same curve as a string: %v", got)
	}
}

func TestEaseRejectsBadCubicBezier(t *testing.T) {
	msg := recovered(func() {
		telatest.MountFunc(t, func() tela.Node {
			tela.UseTween(float32(1), tela.Ease(tela.CubicBezier(2, 0, 0.2, 1)))
			return Div()
		}, telatest.AllowDev("cubic-bezier"))
	})
	if !strings.Contains(msg, "Ease(") || !strings.Contains(msg, "cubic-bezier(2, 0, 0.2, 1)") {
		t.Errorf("an out-of-range cubic-bezier: %q", msg)
	}
}

func TestUsePresenceTween(t *testing.T) {
	var shown *tela.Value[float32]
	var log []string
	page := func() tela.Node {
		on, setOn := tela.UseState(false)
		return Div(
			Button(OnClick(tela.Toggle(setOn)), Text("Toggle")),
			Presence(If(on, tela.C(func() tela.Node {
				shown = tela.UsePresenceTween(tela.Duration(200*time.Millisecond), tela.Ease(tela.EaseLinear),
					tela.OnDone(func() { log = append(log, fmt.Sprintf("done at %.0f", shown.Get())) }))
				return Div(Role("status"), Class("h-4 w-10 bg-black"), Style(tela.OpacityFn(shown.Get)), Text("Saved"))
			}))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(200, 100), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Toggle"))
	if shown.Get() != 0 {
		t.Errorf("the entrance starts at %v, want 0", shown.Get())
	}
	app.Advance(116 * time.Millisecond) // 100 ms of the tween
	if v := shown.Get(); v < 0.4 || v > 0.6 {
		t.Errorf("halfway in: %v", v)
	}
	app.Advance(200 * time.Millisecond)
	if shown.Get() != 1 || strings.Join(log, ",") != "done at 1" {
		t.Errorf("entered: value %v, OnDone %v", shown.Get(), log)
	}
	if app.Count(telatest.ByRole("status", "")) != 1 {
		t.Fatal("the toast is not shown")
	}
	app.Click(telatest.ByRole("button", "Toggle")) // exit
	app.Advance(116 * time.Millisecond)
	if v := shown.Get(); v < 0.4 || v > 0.6 {
		t.Errorf("halfway out: %v", v)
	}
	if shown.Get() == 0 {
		t.Error("the toast unmounted before its exit ended")
	}
	app.Advance(300 * time.Millisecond)
	if n := app.Count(telatest.ByRole("status", "")); n != 0 {
		t.Errorf("%d toasts after the exit: done was not called", n)
	}
	if got := strings.Join(log, ","); got != "done at 1,done at 0" {
		t.Errorf("OnDone ran as %q, want after each motion", got)
	}
}

func TestUsePresenceTweenSpring(t *testing.T) {
	var shown *tela.Value[float32]
	page := func() tela.Node {
		on, setOn := tela.UseState(false)
		return Div(
			Button(OnClick(tela.Toggle(setOn)), Text("Toggle")),
			Presence(If(on, tela.C(func() tela.Node {
				shown = tela.UsePresenceTween(tela.Stiffness(300), tela.Damping(30))
				return Div(Role("status"), Class("h-4 w-10 bg-black"), Style(tela.OpacityFn(shown.Get)), Text("Saved"))
			}))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(200, 100))
	app.Click(telatest.ByRole("button", "Toggle")) // settles: the spring ends
	if shown.Get() != 1 {
		t.Errorf("the spring ended at %v, want 1", shown.Get())
	}
	app.Click(telatest.ByRole("button", "Toggle"))
	if n := app.Count(telatest.ByRole("status", "")); n != 0 {
		t.Errorf("%d toasts after the spring exit", n)
	}
}

func TestMapValueChecksItsArguments(t *testing.T) {
	if msg := recovered(func() { tela.MapValue[float32, float32](nil, func(x float32) float32 { return x }) }); !strings.Contains(msg, "MapValue(nil") {
		t.Errorf("MapValue(nil): %q", msg)
	}
	var v *tela.Value[float32]
	telatest.MountFunc(t, func() tela.Node { v = tela.UseValue(float32(2)); return Div() })
	if msg := recovered(func() { tela.MapValue[float32, float32](v, nil) }); !strings.Contains(msg, "MapValue(v, nil") {
		t.Errorf("MapValue(v, nil): %q", msg)
	}
	if got := tela.MapValue(v, func(x float32) string { return fmt.Sprint(x * 2) })(); got != "4" {
		t.Errorf("MapValue = %q, want 4", got)
	}
}
