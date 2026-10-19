package core

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Tests of Presence, Exit, UsePresence (presence.go) and the starting:
// variant (anim.go's beforeChange) on the frame clock of a headless window.

// fadeBox enters from opacity 0 and exits to it, linearly in 200 ms.
const fadeBox = "h-10 bg-sky-500 transition duration-200 ease-linear opacity-100 starting:opacity-0"

// listProbe renders keyed boxes inside a Presence.
type listProbe struct {
	set Setter[[]string]
}

func (l *listProbe) root(initial ...string) Node {
	return Component(func() Node {
		items, set := UseState(initial)
		l.set = set
		return div(Class("flex flex-col"), Presence(Map(items, func(k string) Node {
			return div(Key(k), Class(fadeBox), Exit("opacity-0"), Text(k))
		})))
	})
}

func (x *inputHarness) opacity(k string) float32 {
	x.t.Helper()
	el := keyed(x.tree(), k)
	if el == nil {
		x.t.Fatalf("%q is not mounted", k)
	}
	return el.box.cs.Opacity
}

// transStart returns the start of the first transition of the box with key k.
func (x *inputHarness) transStart(k string) time.Duration {
	x.t.Helper()
	ba := keyed(x.tree(), k).box.anim
	if ba == nil || len(ba.trans) == 0 {
		x.t.Fatalf("%q has no transition", k)
	}
	return ba.trans[0].Start
}

func TestStartingEnter(t *testing.T) {
	var l listProbe
	x := newHarness(t, l.root(), false)
	l.set.Set([]string{"a"})
	x.frame() // mounts and styles "a": its transition starts now
	if op := x.opacity("a"); op != 0 {
		t.Fatalf("the first frame shows opacity %v, want the starting 0", op)
	}
	start := x.transStart("a")
	x.advanceTo(start + 100*time.Millisecond)
	if op := x.opacity("a"); !near32(op, 0.5, 1e-5) {
		t.Errorf("halfway: %v", op)
	}
	x.settle()
	if op := x.opacity("a"); op != 1 || keyed(x.tree(), "a").box.anim != nil || !x.quiet() {
		t.Errorf("after the entrance: %v", op)
	}
}

// TestStartingWithoutPresence: starting: is CSS's @starting-style, on any
// box that starts being rendered.
func TestStartingWithoutPresence(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		show, s := UseState(false)
		set = s
		return div(If(show, div(Key("b"), Class(fadeBox+" starting:translate-y-4"))))
	})
	x := newHarness(t, root, false)
	set.Set(true)
	x.frame()
	b := keyed(x.tree(), "b").box
	if b.cs.Opacity != 0 || b.cs.Transform.TranslateY.Px != 16 {
		t.Fatalf("starting style not applied: opacity %v, translate %v", b.cs.Opacity, b.cs.Transform.TranslateY)
	}
	x.settle()
	if b.cs.Opacity != 1 || b.cs.Transform.TranslateY.Px != 0 {
		t.Errorf("end: %v %v", b.cs.Opacity, b.cs.Transform.TranslateY)
	}
	// The starting style only applies when the box starts being
	// rendered: a later restyle (hover, a class change) does not use it.
	set.Set(false)
	x.settle()
	if keyed(x.tree(), "b") != nil {
		t.Error("without a Presence the box unmounts at once")
	}
}

// TestStartingAfterDisplayNone: a box shown again after display: none
// transitions from its starting style, and without one it does not
// transition at all (it has no before-change style; CSS Transitions).
func TestStartingAfterDisplayNone(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		show, s := UseState(true)
		set = s
		hidden := ""
		if !show {
			hidden = " hidden opacity-50"
		}
		return div(
			div(Key("s"), Class(fadeBox+hidden)),
			div(Key("n"), Class("h-10 bg-red-500 transition duration-200 opacity-100"+hidden)))
	})
	x := newHarness(t, root, false)
	set.Set(false)
	x.settle()
	set.Set(true)
	x.frame()
	if op := x.opacity("s"); op != 0 {
		t.Errorf("shown again with starting: opacity %v, want 0", op)
	}
	if op := x.opacity("n"); op != 1 || keyed(x.tree(), "n").box.anim != nil {
		t.Errorf("shown again without starting: opacity %v (animating %v), want 1 at once", op, keyed(x.tree(), "n").box.anim != nil)
	}
	x.settle()
}

func TestPresenceExit(t *testing.T) {
	var l listProbe
	x := newHarness(t, l.root("a"), false)
	el := keyed(x.tree(), "a")
	u0 := x.tree().Stats().Unmounts
	l.set.Set(nil)
	x.frame() // the exit starts
	if keyed(x.tree(), "a") != el || !el.exiting() {
		t.Fatal("the removed child is not kept while it exits")
	}
	start := x.transStart("a")
	x.advanceTo(start + 100*time.Millisecond)
	if op := x.opacity("a"); !near32(op, 0.5, 1e-5) {
		t.Errorf("halfway out: %v", op)
	}
	x.advanceTo(start + 200*time.Millisecond)
	if keyed(x.tree(), "a") != nil || el.mounted {
		t.Fatal("the child is still mounted when its exit ended")
	}
	if d := x.tree().Stats().Unmounts - u0; d != 2 { // the div and its text
		t.Errorf("%d unmounts", d)
	}
	if !x.quiet() || len(x.tree().pres.exits) != 0 {
		t.Error("not idle after the exit")
	}
}

func TestPresenceReAdd(t *testing.T) {
	var l listProbe
	x := newHarness(t, l.root("a"), false)
	el := keyed(x.tree(), "a")
	l.set.Set(nil)
	x.frame()
	start := x.transStart("a")
	x.advanceTo(start + 100*time.Millisecond)
	l.set.Set([]string{"a"})
	x.frame()
	if keyed(x.tree(), "a") != el || el.exiting() || el.inert || el.exitOn {
		t.Fatal("the child added back is not the same element, out of its exit")
	}
	op := x.opacity("a")
	if op < 0.4 || op > 0.6 {
		t.Errorf("added back at %v: it should go back from about 0.5, not jump", op)
	}
	x.frame()
	if x.opacity("a") <= op {
		t.Errorf("not going back up: %v then %v", op, x.opacity("a"))
	}
	x.settle()
	if x.opacity("a") != 1 || keyed(x.tree(), "a") != el || !el.mounted {
		t.Errorf("end %v", x.opacity("a"))
	}
	if !x.quiet() || len(x.tree().pres.exits) != 0 {
		t.Error("not idle")
	}
}

func TestPresenceSeveral(t *testing.T) {
	var l listProbe
	x := newHarness(t, l.root("a", "b", "c"), false)
	ya, yc := x.boxY("a"), x.boxY("c")
	l.set.Set([]string{"a", "c"})
	x.frame()
	startB := x.transStart("b")
	x.runFor(50 * time.Millisecond)
	l.set.Set([]string{"c", "d"})
	x.frame()
	startA := x.transStart("a")
	// Both exit at their old places; d enters at the end.
	if got := x.order(); !slices.Equal(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("children %v", got)
	}
	if x.boxY("a") != ya || x.boxY("c") != yc {
		t.Errorf("the exiting children do not keep their layout positions")
	}
	x.advanceTo(startB + 200*time.Millisecond)
	if got := x.order(); !slices.Equal(got, []string{"a", "c", "d"}) {
		t.Errorf("after b's exit: %v", got)
	}
	x.advanceTo(startA + 200*time.Millisecond)
	if got := x.order(); !slices.Equal(got, []string{"c", "d"}) {
		t.Errorf("after a's exit: %v", got)
	}
	x.settle()
	if x.opacity("d") != 1 {
		t.Errorf("d entered to %v", x.opacity("d"))
	}
}

// order returns the keys of the mounted keyed primitives in tree order.
func (x *inputHarness) order() []string {
	var out []string
	var walk func(el *Element)
	walk = func(el *Element) {
		if el.kind == elemPrimitive && el.keyed {
			if k, ok := el.key.(string); ok {
				out = append(out, k)
			}
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(x.tree().root)
	return out
}

func (x *inputHarness) boxY(k string) float32 {
	x.t.Helper()
	return x.tree().elemBounds(keyed(x.tree(), k)).Y0
}

// TestPresenceWithoutExitWork: a child with no Exit classes and no
// UsePresence unmounts at once, as outside a Presence.
func TestPresenceWithoutExitWork(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		show, s := UseState(true)
		set = s
		return div(Presence(If(show, div(Key("p"), Class("h-4 transition opacity-100")))))
	})
	x := newHarness(t, root, false)
	set.Set(false)
	x.frame()
	if keyed(x.tree(), "p") != nil {
		t.Error("a child without exit work is kept")
	}
}

func TestPresenceReducedMotion(t *testing.T) {
	var l listProbe
	x := newHarness(t, l.root("a"), true)
	if x.opacity("a") != 1 {
		t.Fatal("the starting style applied with reduced motion")
	}
	l.set.Set(nil)
	x.frame()
	if keyed(x.tree(), "a") != nil {
		t.Error("with reduced motion the exit does not end at once")
	}
	if !x.quiet() {
		t.Error("not idle")
	}
	// A tree without a clock (Render) shows the final styles and no exit.
	_, img := Render(div(Presence(div(Key("r"), Class("size-10 bg-black transition starting:opacity-0")))),
		RenderOptions{Width: 40, Height: 40, Config: Config{Fonts: testFonts}})
	if c := img.RGBAAt(5, 5); c.R != 0 || c.A != 255 {
		t.Errorf("Render shows %v, want the final black", c)
	}
}

// TestUsePresence: a component that runs its own exit; the child unmounts
// when it calls done, and also waits for the Exit transitions.
func TestUsePresence(t *testing.T) {
	var (
		set     Setter[bool]
		present []bool
		done    func()
	)
	child := func() Node {
		p, d := UsePresence()
		present = append(present, p)
		done = d
		return div(Key("m"), Class("h-4 bg-black transition duration-300 ease-linear"), Exit("opacity-0"), Text("m"))
	}
	root := Component(func() Node {
		show, s := UseState(true)
		set = s
		return div(Presence(If(show, Component(child))))
	})
	x := newHarness(t, root, false)
	if !slices.Equal(present, []bool{true}) {
		t.Fatalf("present %v", present)
	}
	done() // while present: ignored
	set.Set(false)
	x.frame()
	if !slices.Equal(present, []bool{true, false}) {
		t.Fatalf("the component did not build with present false: %v", present)
	}
	x.runFor(time.Second) // the transition ended; done not called yet
	if keyed(x.tree(), "m") == nil {
		t.Fatal("unmounted before done")
	}
	if !x.quiet() {
		t.Error("waiting for done should not run frames")
	}
	done()
	x.b.Drain(x.h)
	x.frame()
	if keyed(x.tree(), "m") != nil {
		t.Error("still mounted after done")
	}

	// A done made during an exit that was cancelled before the UI thread
	// applied it does not count for the next exit.
	set.Set(true)
	x.settle()
	set.Set(false)
	x.frame()
	slot := keyed(x.tree(), "m").parent.hooks[0].(*presenceSlot)
	stale := slot.gen.Load()
	set.Set(true) // cancels
	x.frame()
	set.Set(false) // a new exit
	x.frame()
	slot.markDone(stale) // from the cancelled exit: not this one
	x.runFor(500 * time.Millisecond)
	if keyed(x.tree(), "m") == nil {
		t.Fatal("a done of a cancelled exit ended the next one")
	}
	done()
	x.settle()
	if keyed(x.tree(), "m") != nil {
		t.Error("still mounted")
	}
}

// TestUsePresenceWaitsForTransitions: done before the Exit transitions end.
func TestUsePresenceWaitsForTransitions(t *testing.T) {
	var set Setter[bool]
	child := func() Node {
		present, done := UsePresence()
		if !present {
			done()
		}
		return div(Key("m"), Class("h-4 bg-black transition duration-300 ease-linear"), Exit("opacity-0"))
	}
	root := Component(func() Node {
		show, s := UseState(true)
		set = s
		return div(Presence(If(show, Component(child))))
	})
	x := newHarness(t, root, false)
	set.Set(false)
	x.frame()
	start := x.transStart("m")
	x.advanceTo(start + 150*time.Millisecond)
	if keyed(x.tree(), "m") == nil {
		t.Fatal("unmounted before its transition ended")
	}
	x.advanceTo(start + 300*time.Millisecond)
	if keyed(x.tree(), "m") != nil {
		t.Error("still mounted after the transition and done")
	}
}

// TestExitingIsInert: an exiting child is not hit, not focusable and not in
// the semantics tree.
func TestExitingIsInert(t *testing.T) {
	var set Setter[bool]
	clicks := 0
	root := Component(func() Node {
		show, s := UseState(true)
		set = s
		return div(Class("relative h-40 w-40"),
			div(Key("below"), Class("absolute inset-0"), TabIndex(0), On("click", func(MouseEvent) { clicks++ })),
			Presence(If(show, button(Key("btn"), Class("absolute inset-0 size-full bg-white transition duration-500"),
				Exit("opacity-0"), FocusScope(true, true), Text("Close")))))
	})
	x := newKeys(t, root, 200, 200)
	x.settle()
	btn := keyed(x.tree(), "btn")
	x.tree().setFocus(btn, focusKeyboard)
	if f, h := x.tree().Focused(), x.tree().HitTest(80, 80); f != btn || h != btn {
		t.Fatalf("before the exit: focused %s, hit %s; want the button", keyName(f), keyName(h))
	}
	if !strings.Contains(x.tree().AriaSnapshot(), `button "Close"`) {
		t.Fatal(x.tree().AriaSnapshot())
	}
	set.Set(false)
	x.frame()
	if !btn.exiting() || !btn.mounted {
		t.Fatal("not exiting")
	}
	if got := x.tree().HitTest(80, 80); got != keyed(x.tree(), "below") {
		t.Errorf("the exiting button is hit (%v)", keyName(got))
	}
	if x.tree().Focused() == btn || x.tree().focusable(btn) {
		t.Error("the exiting button keeps or can take the focus")
	}
	if len(x.tree().foc.scopes) != 0 {
		t.Error("its focus scope is still active")
	}
	if snap := x.tree().AriaSnapshot(); strings.Contains(snap, "Close") {
		t.Errorf("the exiting button is in the aria snapshot:\n%s", snap)
	}
	x.tab()
	if f := x.tree().Focused(); f == btn {
		t.Error("Tab reached the exiting button")
	}
	x.click(80, 80)
	if clicks != 1 {
		t.Errorf("the click did not reach the element below (%d)", clicks)
	}
	x.settle()
	if keyed(x.tree(), "btn") != nil {
		t.Error("not unmounted")
	}
}

// TestNestedPresence: the children of a nested Presence do not run their
// Exit classes when an outer child exits.
func TestNestedPresence(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		show, s := UseState(true)
		set = s
		return div(Presence(If(show, div(Key("outer"), Class("transition duration-200"), Exit("opacity-0"),
			Presence(div(Key("inner"), Class("transition duration-200"), Exit("opacity-50")))))))
	})
	x := newHarness(t, root, false)
	inner := keyed(x.tree(), "inner")
	set.Set(false)
	x.frame()
	if inner.exitOn || x.opacity("inner") != 1 || !inner.inert {
		t.Errorf("inner: exitOn %v, opacity %v, inert %v", inner.exitOn, x.opacity("inner"), inner.inert)
	}
	x.settle()
	if inner.mounted {
		t.Error("the inner child outlived the outer one")
	}
}

// TestPresenceExitStyle: the Exit classes apply on top of the element's
// classes (last wins) and come off when it is added back.
func TestPresenceExitStyle(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		show, s := UseState(true)
		set = s
		return div(Presence(If(show, div(Key("e"), Class("h-4 bg-white scale-100 transition duration-1000"),
			Exit("bg-black"), Exit("scale-50")))))
	})
	x := newHarness(t, root, false)
	set.Set(false)
	x.frame()
	ba := keyed(x.tree(), "e").box.anim
	if ba == nil || ba.target.BackgroundColor != gfx.Black || ba.target.Transform.ScaleX != 0.5 {
		t.Fatalf("the exit target %+v", ba)
	}
	set.Set(true)
	x.frame()
	if tgt := keyed(x.tree(), "e").box.anim.target; tgt.BackgroundColor != gfx.White || tgt.Transform.ScaleX != 1 {
		t.Errorf("the Exit classes stay after the cancel")
	}
	x.settle()
}

func TestPresenceProps(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Presence does not take Class") {
			t.Errorf("panic %v", r)
		}
	}()
	Presence(Key(1), Prop{}, Class("p-4"))
}

// TestExitClassErrors: unknown Exit classes are reported at mount in dev
// mode.
func TestExitClassErrors(t *testing.T) {
	var logs []string
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { logs = append(logs, m) }})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	tr.Mount(div(Presence(div(Key("x"), Exit("opacity-0 scale-95x")))))
	tr.Frame()
	if len(logs) != 1 || !strings.Contains(logs[0], `"scale-95x"`) {
		t.Errorf("logs %q", logs)
	}
	_ = style.KindDiv
}

// TestExitRestoresFocus: the focus scope of an exiting dialog ends when
// the exit starts, so the focus returns to where it was at once, not when
// the dialog unmounts.
func TestExitRestoresFocus(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		open, s := UseState(false)
		set = s
		return div(button(Key("open"), Text("Open")),
			Presence(If(open, div(Key("dlg"), FocusScope(true, true), Class("transition duration-300"), Exit("opacity-0"),
				button(Key("close"), AutoFocus(true), Text("Close"))))))
	})
	x := newKeys(t, root, 200, 200)
	x.settle()
	x.tree().setFocus(keyed(x.tree(), "open"), focusKeyboard)
	set.Set(true)
	x.settle()
	if got := focusedName(x.tree()); got != "close" {
		t.Fatalf("focused %s, want close", got)
	}
	set.Set(false)
	x.frame()
	if !keyed(x.tree(), "dlg").exiting() {
		t.Fatal("not exiting")
	}
	if got := focusedName(x.tree()); got != "open" {
		t.Errorf("focused %s during the exit, want open (restored)", got)
	}
	x.settle()
}
