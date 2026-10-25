package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// ---------------------------------------------------------------------
// Harness: keys injected into a focused headless window.

// newKeys returns an input harness whose window has keyboard focus.
func newKeys(t *testing.T, root Node, w, h float32) *inputHarness {
	t.Helper()
	x := newInput(t, root, w, h)
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	return x
}

func keyEv(a platform.KeyAction, k platform.Key, c platform.KeyCode, mods platform.Modifiers) platform.KeyEvent {
	e := platform.KeyEvent{Action: a, Code: c, Key: k, Location: c.Location(), Mods: mods}
	if a == platform.KeyPress && !k.IsNamed() {
		e.Text = string(k)
	}
	return e
}

// keys injects key events and delivers them.
func (x *inputHarness) keys(es ...platform.KeyEvent) {
	for _, e := range es {
		x.w.Inject(e)
	}
	x.b.Drain(x.h)
}

// press presses and releases a key.
func (x *inputHarness) press(k platform.Key, c platform.KeyCode, mods platform.Modifiers) {
	x.keys(keyEv(platform.KeyPress, k, c, mods), keyEv(platform.KeyRelease, k, c, mods))
}

func (x *inputHarness) tab()      { x.press(platform.KeyTab, platform.CodeTab, 0) }
func (x *inputHarness) shiftTab() { x.press(platform.KeyTab, platform.CodeTab, platform.ModShift) }
func (x *inputHarness) enter()    { x.press(platform.KeyEnter, platform.CodeEnter, 0) }
func (x *inputHarness) space()    { x.press(" ", platform.CodeSpace, 0) }

func focusedName(tr *Tree) string { return keyName(tr.Focused()) }

// center returns the center of the border box of the primitive with key k
// (after a frame).
func (x *inputHarness) center(k any) (float32, float32) {
	x.t.Helper()
	el := keyed(x.tree(), k)
	if el == nil || el.box.pcs == nil {
		x.t.Fatalf("no painted element %v", k)
	}
	c := windowRect(el.box).Center()
	return c.X, c.Y
}

func (r *recorder) key(event, id string) Prop {
	return On(event, func(e KeyboardEvent) { r.add(fmt.Sprintf("%s:%s:%s", event, id, e.Key)) })
}

func (r *recorder) focus(event, id string) Prop {
	return On(event, func(FocusEvent) { r.add(event + ":" + id) })
}

// ---------------------------------------------------------------------
// Key events.

func TestKeyboardEventFields(t *testing.T) {
	var got []KeyboardEvent
	root := div(Key("box"), Class("h-10"), TabIndex(0),
		On("keydown", func(e KeyboardEvent) { got = append(got, e) }),
		On("keyup", func(e KeyboardEvent) { got = append(got, e) }))
	x := newKeys(t, root, 200, 100)
	keyed(x.tree(), "box")
	x.tree().focusElement(keyed(x.tree(), "box"))
	x.keys(keyEv(platform.KeyPress, "s", platform.CodeKeyS, platform.ModCtrl))
	rep := keyEv(platform.KeyPress, "s", platform.CodeKeyS, platform.ModCtrl)
	rep.Repeat = true
	x.keys(rep, keyEv(platform.KeyRelease, "s", platform.CodeKeyS, platform.ModCtrl))
	x.keys(keyEv(platform.KeyPress, platform.KeyShift, platform.CodeShiftRight, platform.ModShift),
		keyEv(platform.KeyRelease, platform.KeyShift, platform.CodeShiftRight, 0))
	x.keys(keyEv(platform.KeyPress, platform.KeyUnidentified, platform.UnidentifiedKeyCode(0x1f0), 0))
	if len(got) != 6 {
		t.Fatalf("%d events, want 6", len(got))
	}
	e := got[0]
	if e.Key != "s" || e.Code != "KeyS" || !e.Ctrl || e.Shift || e.Alt || e.Meta || e.Repeat || e.Location != KeyLocationStandard {
		t.Errorf("Ctrl+S: %+v", e)
	}
	if !got[1].Repeat || got[2].Repeat {
		t.Error("Repeat")
	}
	if got[3].Key != KeyValue(platform.KeyShift) || got[3].Code != "ShiftRight" || !got[3].Shift || got[3].Location != KeyLocationRight {
		t.Errorf("Shift press: %+v", got[3])
	}
	if got[4].Shift {
		t.Error("the Shift release still has Shift (the DOM reports the state after the event)")
	}
	if got[5].Code != "Unidentified" || got[5].Key != "Unidentified" {
		t.Errorf("unidentified key: %+v", got[5])
	}
	if KeyLocationNumpad.String() != "numpad" {
		t.Error("KeyLocation.String")
	}
}

func TestKeyBubblingAndStop(t *testing.T) {
	var r recorder
	stop := false
	root := div(Key("outer"), r.key("keydown", "outer"),
		div(Key("mid"), r.key("keydown", "mid"),
			On("keydown", func(e KeyboardEvent) {
				if stop {
					e.StopPropagation()
				}
			}),
			button(Key("btn"), r.key("keydown", "btn"), r.key("keyup", "btn"), Text("b"))))
	x := newKeys(t, root, 200, 100)
	// Nothing focused: the outermost element of the content.
	x.press("a", platform.CodeKeyA, 0)
	if got := r.String(); got != "keydown:outer:a" {
		t.Errorf("unfocused: %s", got)
	}
	r.reset()
	x.tree().focusElement(keyed(x.tree(), "btn"))
	x.press("a", platform.CodeKeyA, 0)
	if got := r.String(); got != "keydown:btn:a keydown:mid:a keydown:outer:a keyup:btn:a" {
		t.Errorf("bubbling: %s", got)
	}
	r.reset()
	stop = true
	x.press("b", platform.CodeKeyB, 0)
	if got := r.String(); got != "keydown:btn:b keydown:mid:b keyup:btn:b" {
		t.Errorf("stopped: %s", got)
	}
}

// ---------------------------------------------------------------------
// Sequential navigation.

func TestTabOrder(t *testing.T) {
	root := div(Class("flex flex-col"),
		button(Key("b0"), Text("b0")),
		div(Key("t2"), TabIndex(2), Text("t2")),
		div(Key("neg"), TabIndex(-1), Text("neg")),
		div(Key("plain"), Text("plain")),
		button(Key("dis"), Disabled(true), Text("dis")),
		div(Disabled(true), button(Key("indis"), Text("indis"))),
		button(Key("none"), Class("hidden"), Text("none")),
		div(Class("hidden"), button(Key("innone"), Text("innone"))),
		div(Class("invisible"), button(Key("inv"), Text("inv")),
			button(Key("vis"), Class("visible"), Text("vis"))),
		div(Key("d0"), TabIndex(0),
			Component(func() Node { return button(Key("nested"), Text("nested")) })),
		div(Key("t1"), TabIndex(1), Text("t1")),
		div(Class("contents"), Key("cont"), TabIndex(0), button(Key("incont"), Text("c"))),
	)
	x := newKeys(t, root, 300, 400)
	tr := x.tree()
	want := []string{"t1", "t2", "b0", "vis", "d0", "nested", "incont"}
	var got []string
	for range len(want) + 1 {
		x.tab()
		got = append(got, focusedName(tr))
	}
	if g, w := strings.Join(got, " "), strings.Join(append(want, "t1"), " "); g != w {
		t.Errorf("Tab order\n got %s\nwant %s", g, w)
	}
	got = got[:0]
	for range len(want) {
		x.shiftTab()
		got = append(got, focusedName(tr))
	}
	if g := strings.Join(got, " "); g != "incont nested d0 vis b0 t2 t1" {
		t.Errorf("Shift+Tab order: %s", g)
	}
	// From an element out of the order (-1), Tab goes on after it.
	tr.focusElement(keyed(tr, "neg"))
	if focusedName(tr) != "neg" {
		t.Fatal("TabIndex(-1) is not focusable by script")
	}
	x.tab()
	if focusedName(tr) != "vis" {
		t.Errorf("Tab from tabindex -1: %s, want vis", focusedName(tr))
	}
	tr.focusElement(keyed(tr, "neg"))
	x.shiftTab()
	if focusedName(tr) != "b0" {
		t.Errorf("Shift+Tab from tabindex -1: %s, want b0", focusedName(tr))
	}
	// Not focusable at all.
	for _, k := range []string{"plain", "dis", "indis", "none", "innone", "inv", "cont"} {
		if tr.focusable(keyed(tr, k)) {
			t.Errorf("%s is focusable", k)
		}
	}
	// Shift+Tab with nothing focused starts from the end.
	keyed(tr, "t1")
	tr.setFocus(nil, focusScript)
	tr.foc.start = nil
	x.shiftTab()
	if focusedName(tr) != "incont" {
		t.Errorf("Shift+Tab from nothing: %s", focusedName(tr))
	}
}

func TestTabPreventDefaultAndModifiers(t *testing.T) {
	prevent := true
	root := div(Key("page"), On("keydown", func(e KeyboardEvent) {
		if prevent && e.Key == KeyValue(platform.KeyTab) {
			e.PreventDefault()
		}
	}), button(Key("a"), Text("a")), button(Key("b"), Text("b")))
	x := newKeys(t, root, 200, 100)
	tr := x.tree()
	x.tab()
	if tr.Focused() != nil {
		t.Error("a prevented Tab moved the focus")
	}
	prevent = false
	x.tab()
	if focusedName(tr) != "a" {
		t.Fatalf("Tab: %s", focusedName(tr))
	}
	x.press(platform.KeyTab, platform.CodeTab, platform.ModCtrl)
	x.press(platform.KeyTab, platform.CodeTab, platform.ModAlt)
	if focusedName(tr) != "a" {
		t.Error("Ctrl+Tab or Alt+Tab moved the focus")
	}
	// Holding Tab repeats the navigation.
	rep := keyEv(platform.KeyPress, platform.KeyTab, platform.CodeTab, 0)
	x.keys(keyEv(platform.KeyPress, platform.KeyTab, platform.CodeTab, 0))
	rep.Repeat = true
	x.keys(rep, keyEv(platform.KeyRelease, platform.KeyTab, platform.CodeTab, 0))
	if focusedName(tr) != "a" {
		t.Errorf("two Tabs from a with two buttons: %s, want a (wrapped)", focusedName(tr))
	}
}

// ---------------------------------------------------------------------
// Button activation.

func TestEnterAndSpaceActivateButtons(t *testing.T) {
	var r recorder
	preventUp := false
	var clicks []MouseEvent
	root := div(Key("page"), r.mouse("click", "page"),
		On("keyup", func(e KeyboardEvent) {
			if preventUp {
				e.PreventDefault()
			}
		}),
		button(Key("btn"), Class("h-10 w-20 active:bg-black"), r.mouse("click", "btn"),
			On("click", func(e MouseEvent) { clicks = append(clicks, e) }), Text("b")),
		button(Key("other"), Text("o")),
		div(Key("div"), TabIndex(0), r.mouse("click", "div"), Text("d")))
	x := newKeys(t, root, 300, 200)
	tr := x.tree()
	btn := keyed(tr, "btn")
	x.tab()
	// Enter clicks on keydown, and bubbles like a click.
	x.keys(keyEv(platform.KeyPress, platform.KeyEnter, platform.CodeEnter, platform.ModShift))
	if got := r.String(); got != "click:btn click:page" {
		t.Fatalf("Enter: %q", got)
	}
	if e := clicks[0]; e.ClickCount != 0 || e.Button != MouseButtonLeft || !e.Shift {
		t.Errorf("keyboard click event %+v", e)
	}
	if bx := btn.box; !near(clicks[0].X, windowRect(bx).Center().X, 0.01) || !near(clicks[0].LocalX, bx.u.Width()/2, 0.01) {
		t.Errorf("keyboard click at %v,%v (local %v), want the button's center", clicks[0].X, clicks[0].Y, clicks[0].LocalX)
	}
	x.keys(keyEv(platform.KeyRelease, platform.KeyEnter, platform.CodeEnter, 0))
	r.reset()
	// Space: active on keydown, click on keyup.
	x.keys(keyEv(platform.KeyPress, " ", platform.CodeSpace, 0))
	if r.String() != "" {
		t.Errorf("Space clicked on keydown: %s", r)
	}
	if btn.istate&style.StateActive == 0 {
		t.Error("Space did not make the button active")
	}
	x.keys(keyEv(platform.KeyRelease, " ", platform.CodeSpace, 0))
	if got := r.String(); got != "click:btn click:page" {
		t.Errorf("Space keyup: %q", got)
	}
	if btn.istate&style.StateActive != 0 {
		t.Error("active after the Space release")
	}
	r.reset()
	// Space pressed, then the focus moves: no click.
	x.keys(keyEv(platform.KeyPress, " ", platform.CodeSpace, 0))
	x.tab()
	x.keys(keyEv(platform.KeyRelease, " ", platform.CodeSpace, 0))
	if r.String() != "" || btn.istate&style.StateActive != 0 {
		t.Errorf("Space after a focus move: %q, active %v", r, btn.istate&style.StateActive != 0)
	}
	x.shiftTab()
	// A prevented keyup does not click.
	preventUp = true
	x.space()
	preventUp = false
	if r.String() != "" {
		t.Errorf("prevented Space keyup clicked: %s", r)
	}
	// A synthetic release (the window lost focus while Space was held).
	x.keys(keyEv(platform.KeyPress, " ", platform.CodeSpace, 0))
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	if r.String() != "" || btn.istate&style.StateActive != 0 {
		t.Errorf("synthetic Space release: %q", r)
	}
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	// Enter on a focusable div does not click (HTML: only buttons).
	tr.focusElement(keyed(tr, "div"))
	x.enter()
	x.space()
	if r.String() != "" {
		t.Errorf("Enter/Space on a div: %s", r)
	}
}

// ---------------------------------------------------------------------
// Pointer focus.

func TestFocusByClick(t *testing.T) {
	var r recorder
	preventMouse, preventPointer := false, false
	root := div(Key("page"), Class("relative h-[300px]"), r.focus("focus", "page"), r.focus("blur", "page"),
		button(Key("a"), Class("h-10 w-20"), Text("a")),
		div(Key("card"), TabIndex(-1), Class("h-20 w-40"),
			div(Key("label"), Class("h-10"), Text("label"))),
		div(Key("keep"), Class("h-10 w-20"),
			On("mousedown", func(e MouseEvent) {
				if preventMouse {
					e.PreventDefault()
				}
			}),
			On("pointerdown", func(e PointerEvent) {
				if preventPointer {
					e.PreventDefault()
				}
			})),
		button(Key("b"), Class("h-10 w-20"), Text("b")),
	)
	x := newKeys(t, root, 400, 300)
	tr := x.tree()
	x.click(x.center("a"))
	if focusedName(tr) != "a" {
		t.Fatalf("click on a button: focused %s", focusedName(tr))
	}
	// Focus events bubble: the page sees the focus of its descendant.
	if got := r.String(); got != "focus:page" {
		t.Errorf("events: %q", got)
	}
	r.reset()
	// A press on a non-focusable descendant focuses the focusable
	// ancestor (tabindex -1 is focusable by click).
	x.click(x.center("label"))
	if focusedName(tr) != "card" {
		t.Errorf("click on the label: %s, want card", focusedName(tr))
	}
	if got := r.String(); got != "blur:page focus:page" {
		t.Errorf("events: %q", got)
	}
	x.click(x.center("a"))
	// PreventDefault on mousedown or pointerdown keeps the focus.
	preventMouse = true
	x.click(x.center("keep"))
	if focusedName(tr) != "a" {
		t.Errorf("mousedown prevented: focus moved to %s", focusedName(tr))
	}
	preventMouse, preventPointer = false, true
	x.click(x.center("keep"))
	if focusedName(tr) != "a" {
		t.Errorf("pointerdown prevented: focus moved to %s", focusedName(tr))
	}
	preventPointer = false
	// A press elsewhere blurs, and Tab goes on from there.
	x.click(x.center("keep"))
	if tr.Focused() != nil {
		t.Errorf("click on nothing focusable: %s still focused", focusedName(tr))
	}
	x.tab()
	if focusedName(tr) != "b" {
		t.Errorf("Tab after clicking keep: %s, want b (the next after the click)", focusedName(tr))
	}
	// Secondary button presses do not move the focus.
	ax, ay := x.center("a")
	x.send(move(ax, ay), ptr(platform.PointerDown, ax, ay, platform.ButtonSecondary, platform.ButtonSecondary.Mask()),
		ptr(platform.PointerUp, ax, ay, platform.ButtonSecondary, 0))
	if focusedName(tr) != "b" {
		t.Errorf("right click moved the focus to %s", focusedName(tr))
	}
}

// ---------------------------------------------------------------------
// Variants.

func TestFocusVariants(t *testing.T) {
	root := div(Key("page"), Class("flex flex-col gap-2 p-2 bg-white focus-within:bg-slate-100"),
		div(Key("plain"), Class("h-4")),
		div(Key("group"), Class("group p-1"),
			button(Key("a"), Class("h-6 w-20 bg-white focus:bg-sky-500 focus-visible:ring-2"), Text("a")),
			span(Key("hint"), Class("text-black group-focus-within:text-red-500"), Text("hint"))),
		button(Key("b"), Class("peer h-6 w-20 bg-white focus:bg-sky-500 focus-visible:ring-2"), Text("b")),
		div(Key("sib"), Class("h-4 bg-white peer-focus:bg-black peer-focus-visible:border")),
	)
	x := newKeys(t, root, 300, 200)
	tr := x.tree()
	a, b, page, hint, sib := keyed(tr, "a"), keyed(tr, "b"), keyed(tr, "page"), keyed(tr, "hint"), keyed(tr, "sib")
	sky := a.box.cs.BackgroundColor
	styles := func() int { return tr.Stats().Styles }

	s0 := styles()
	x.tab()
	x.frame()
	// a (focus, focus-visible), page (focus-within), hint (group-focus-within).
	if d := styles() - s0; d != 3 {
		t.Errorf("Tab restyled %d elements, want 3", d)
	}
	if a.box.cs.BackgroundColor == sky || len(a.box.cs.BoxShadows) != 1 {
		t.Errorf("focus: or focus-visible: not applied to a: bg %v ring %+v", a.box.cs.BackgroundColor, a.box.cs.BoxShadows)
	}
	if page.box.cs.BackgroundColor == gfx.White {
		t.Error("focus-within: not applied")
	}
	if hint.box.cs.Text.Color == gfx.Black {
		t.Error("group-focus-within: not applied")
	}

	s0 = styles()
	x.tab()
	x.frame()
	// a, hint lose; b and sib gain; page keeps focus-within.
	if d := styles() - s0; d != 4 {
		t.Errorf("Tab to b restyled %d elements, want 4", d)
	}
	if a.box.cs.BackgroundColor != gfx.White || len(a.box.cs.BoxShadows) != 0 {
		t.Error("a keeps its focus styles")
	}
	if sib.box.cs.BackgroundColor != gfx.Black || sib.box.cs.Border.Top.Width != 1 {
		t.Error("peer-focus: or peer-focus-visible: not applied")
	}

	// A click focuses without focus-visible; a key then shows it.
	x.click(x.center("a"))
	x.frame()
	if a.istate&style.StateFocus == 0 || a.istate&style.StateFocusVisible != 0 {
		t.Errorf("click focus: states %b", a.istate)
	}
	if len(a.box.cs.BoxShadows) != 0 || a.box.cs.BackgroundColor == gfx.White {
		t.Error("click focus styles")
	}
	s0 = styles()
	x.press(platform.KeyShift, platform.CodeShiftLeft, platform.ModShift)
	x.press("c", platform.CodeKeyC, platform.ModCtrl)
	if a.istate&style.StateFocusVisible != 0 {
		t.Error("a modifier or a Ctrl shortcut showed focus-visible")
	}
	x.press("x", platform.CodeKeyX, 0)
	x.frame()
	if a.istate&style.StateFocusVisible == 0 || len(a.box.cs.BoxShadows) != 1 {
		t.Error("a key press did not show focus-visible on the clicked element")
	}
	if d := styles() - s0; d != 1 {
		t.Errorf("showing focus-visible restyled %d elements, want 1", d)
	}
	// Programmatic focus follows the last interaction: after a pointer
	// press it does not show focus-visible; after a key it does.
	x.click(x.center("plain"))
	tr.focusElement(b)
	if b.istate&style.StateFocusVisible != 0 {
		t.Error("script focus after a click shows focus-visible")
	}
	x.press("x", platform.CodeKeyX, 0)
	tr.focusElement(a)
	if a.istate&style.StateFocusVisible == 0 {
		t.Error("script focus after a key does not show focus-visible")
	}
}

// ---------------------------------------------------------------------
// Window focus.

func TestWindowFocus(t *testing.T) {
	var r recorder
	ref := ElementRef{}
	app := func() Node {
		ref = UseElementRef()
		return div(Key("page"), r.focus("focus", "page"), r.focus("blur", "page"),
			button(Key("a"), AttachRef(ref), Class("focus:bg-black"), Text("a")))
	}
	x := newKeys(t, Component(app), 200, 100)
	tr := x.tree()
	a := keyed(tr, "a")
	x.tab()
	x.frame()
	r.reset()
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	x.frame()
	if got := r.String(); got != "blur:page" {
		t.Errorf("window blur: %q", got)
	}
	if a.istate&(style.StateFocus|style.StateFocusVisible|style.StateFocusWithin) != 0 || a.box.cs.BackgroundColor == gfx.Black {
		t.Error("focus styles kept while the window is inactive")
	}
	if !ref.Focused() || tr.Focused() != a {
		t.Error("the element stopped being the focused element")
	}
	r.reset()
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	x.frame()
	if got := r.String(); got != "focus:page" {
		t.Errorf("window focus: %q", got)
	}
	if a.box.cs.BackgroundColor != gfx.Black || a.istate&style.StateFocusVisible == 0 {
		t.Error("focus styles not restored")
	}
}

// A Host starts with an unfocused window: focus styles and events wait
// for the backend's FocusEvent.
func TestHostStartsUnfocused(t *testing.T) {
	var r recorder
	root := div(r.focus("focus", "page"), button(Key("a"), AutoFocus(true), Text("a")))
	x := newInput(t, root, 200, 100)
	tr := x.tree()
	if tr.WindowFocused() || focusedName(tr) != "a" || r.String() != "" {
		t.Errorf("window %v, focused %s, events %q", tr.WindowFocused(), focusedName(tr), r)
	}
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	if r.String() != "focus:page" || keyed(tr, "a").istate&style.StateFocus == 0 {
		t.Errorf("after the FocusEvent: %q", r)
	}
}

// ---------------------------------------------------------------------
// Fix-up and unmount.

func TestDisableOrHideFocusedBlurs(t *testing.T) {
	var r recorder
	var setDis, setHide Setter[bool]
	app := func() Node {
		dis, sd := UseState(false)
		hide, sh := UseState(false)
		setDis, setHide = sd, sh
		cls := "h-6"
		if hide {
			cls = "hidden"
		}
		return div(Key("page"), r.focus("blur", "page"),
			button(Key("a"), Disabled(dis), r.focus("blur", "a"), Text("a")),
			button(Key("b"), Class(cls), r.focus("blur", "b"), Text("b")))
	}
	x := newKeys(t, Component(app), 200, 100)
	tr := x.tree()
	x.tab()
	setDis.Set(true)
	x.frame()
	if tr.Focused() != nil {
		t.Error("the disabled element keeps the focus")
	}
	if got := r.String(); got != "blur:a blur:page" {
		t.Errorf("blur on disable: %q", got)
	}
	r.reset()
	x.tab()
	if focusedName(tr) != "b" {
		t.Fatalf("Tab: %s", focusedName(tr))
	}
	setHide.Set(true)
	x.frame()
	if tr.Focused() != nil || r.String() != "blur:b blur:page" {
		t.Errorf("display: none: focused %s, events %q", focusedName(tr), r)
	}
	if tr.Dirty() {
		x.frame()
	}
}

func TestUnmountFocused(t *testing.T) {
	var r recorder
	var set Setter[bool]
	app := func() Node {
		show, s := UseState(true)
		set = s
		return div(Key("page"), Class("focus-within:bg-black"), r.focus("blur", "page"),
			If(show, div(Key("wrap"), button(Key("a"), r.focus("blur", "a"), Text("a")))),
			button(Key("b"), Text("b")))
	}
	x := newKeys(t, Component(app), 200, 100)
	tr := x.tree()
	page := keyed(tr, "page")
	x.tab()
	x.frame()
	if page.box.cs.BackgroundColor != gfx.Black {
		t.Fatal("focus-within not applied")
	}
	set.Set(false)
	x.frame()
	if tr.Focused() != nil || r.String() != "" {
		t.Errorf("unmount: focused %s, events %q (React fires no blur)", focusedName(tr), r)
	}
	if page.istate&style.StateFocusWithin != 0 || page.box.cs.BackgroundColor == gfx.Black {
		t.Error("focus-within kept after the focused element unmounted")
	}
	x.tab()
	if focusedName(tr) != "b" {
		t.Errorf("Tab after unmount: %s", focusedName(tr))
	}
}

// ---------------------------------------------------------------------
// AutoFocus.

func TestAutoFocus(t *testing.T) {
	var setOpen Setter[bool]
	app := func() Node {
		open, s := UseState(false)
		setOpen = s
		return div(Key("page"),
			button(Key("first"), AutoFocus(true), Text("first")),
			button(Key("opener"), Class("h-6 w-10"), On("click", func(MouseEvent) { setOpen.Set(true) }), Text("open")),
			If(open, div(Key("panel"),
				button(Key("x"), Text("x")),
				button(Key("ok"), AutoFocus(true), Text("ok")),
				button(Key("also"), AutoFocus(true), Text("also")))))
	}
	x := newKeys(t, Component(app), 300, 200)
	tr := x.tree()
	if focusedName(tr) != "first" {
		t.Fatalf("AutoFocus at mount: %s", focusedName(tr))
	}
	// The opener is focused by the click; the panel mounts and its first
	// AutoFocus element takes the focus in the next frame.
	x.click(x.center("opener"))
	if focusedName(tr) != "opener" {
		t.Fatalf("click: %s", focusedName(tr))
	}
	x.frame()
	if focusedName(tr) != "ok" {
		t.Errorf("AutoFocus of a panel: %s, want ok", focusedName(tr))
	}
	// Close and open again; this time Tab moves the focus before the
	// frame: AutoFocus gives way.
	setOpen.Set(false)
	x.frame()
	x.click(x.center("opener"))
	if keyed(tr, "ok") == nil {
		t.Fatal("panel not mounted by the click")
	}
	x.tab()
	moved := focusedName(tr)
	x.frame()
	if focusedName(tr) != moved {
		t.Errorf("AutoFocus took the focus from %s after the user moved it", moved)
	}
	_ = setOpen
}

// ---------------------------------------------------------------------
// ElementRef.

func TestElementRef(t *testing.T) {
	var r recorder
	var refA, refB, refC ElementRef
	var setShow Setter[bool]
	effectRan := false
	app := func() Node {
		refA, refB, refC = UseElementRef(), UseElementRef(), UseElementRef()
		show, s := UseState(true)
		setShow = s
		UseEffect(func() func() {
			refB.Focus() // React's pattern: focus on mount
			effectRan = true
			return nil
		}, nil)
		return div(r.focus("focus", "page"),
			div(Key("a"), AttachRef(refA), TabIndex(0), Text("a")),
			If(show, div(Key("b"), AttachRef(refB), TabIndex(-1), Text("b"))),
			div(Key("c"), AttachRef(refC), Text("c")))
	}
	x := newKeys(t, Component(app), 200, 100)
	tr := x.tree()
	if !effectRan || focusedName(tr) != "b" || !refB.Focused() || refA.Focused() {
		t.Fatalf("Focus in an effect: effect %v, focused %s", effectRan, focusedName(tr))
	}
	refA.Focus()
	if !refA.Focused() || refB.Focused() {
		t.Error("Focus")
	}
	refC.Focus() // not focusable: nothing happens
	if !refA.Focused() {
		t.Error("Focus on a non-focusable element moved the focus")
	}
	refB.Blur() // not focused: nothing
	refA.Blur()
	if tr.Focused() != nil || refA.Focused() {
		t.Error("Blur")
	}
	if r.String() != "focus:page focus:page" {
		t.Errorf("events %q", r)
	}
	setShow.Set(false)
	x.frame()
	refB.Focus()
	refB.Blur()
	if refB.Focused() || tr.Focused() != nil {
		t.Error("the ref of an unmounted element acts")
	}
	var zero ElementRef
	zero.Focus()
	zero.Blur()
	if zero.Focused() {
		t.Error("zero ElementRef")
	}
	if zero != (ElementRef{}) || refA == refB {
		t.Error("ElementRef comparisons")
	}
}

// ElementRef.Focus in a layout effect is styled in the same frame.
func TestFocusInLayoutEffect(t *testing.T) {
	app := func() Node {
		ref := UseElementRef()
		UseLayoutEffect(func() func() {
			ref.Focus()
			return nil
		}, nil)
		return div(button(Key("a"), AttachRef(ref), Class("focus:bg-black"), Text("a")))
	}
	tr := newTestTree(t, 200, 100, Component(app), nil)
	tr.Frame()
	a := keyed(tr, "a")
	if tr.Focused() != a || a.box.cs.BackgroundColor != gfx.Black {
		t.Errorf("focused %s, bg %v", focusedName(tr), a.box.cs.BackgroundColor)
	}
	if tr.Dirty() {
		t.Error("work left after the frame")
	}
}

func TestUseElementRefHookOrder(t *testing.T) {
	flip := false
	var set Setter[int]
	app := func() Node {
		_, s := UseState(0)
		set = s
		if flip {
			UseState(0)
		} else {
			UseElementRef()
		}
		return div()
	}
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(string) {}})
	tr.Mount(Component(app))
	flip = true
	set.Set(1)
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "UseElementRef") {
			t.Errorf("panic %q does not name UseElementRef", msg)
		}
	}()
	tr.Frame()
}

// ---------------------------------------------------------------------
// Focus scopes (reserved for aria).

func TestFocusScope(t *testing.T) {
	var setOpen, setContain Setter[bool]
	app := func() Node {
		open, s := UseState(false)
		contain, sc := UseState(true)
		setOpen, setContain = s, sc
		return div(Key("page"),
			button(Key("trigger"), Class("h-6 w-10"), On("click", func(MouseEvent) { setOpen.Set(true) }), Text("t")),
			button(Key("after"), Text("after")),
			If(open, div(Key("dialog"), FocusScope(contain, true),
				button(Key("d1"), AutoFocus(true), Text("1")),
				div(Key("plain"), Text("p")),
				button(Key("d2"), Text("2")),
				button(Key("close"), On("click", func(MouseEvent) { setOpen.Set(false) }), Text("x")))))
	}
	x := newKeys(t, Component(app), 300, 200)
	tr := x.tree()
	x.tab()
	if focusedName(tr) != "trigger" {
		t.Fatal(focusedName(tr))
	}
	x.enter()
	x.frame()
	if focusedName(tr) != "d1" {
		t.Fatalf("AutoFocus in the scope: %s", focusedName(tr))
	}
	var got []string
	for range 4 {
		x.tab()
		got = append(got, focusedName(tr))
	}
	if g := strings.Join(got, " "); g != "d2 close d1 d2" {
		t.Errorf("Tab inside the scope: %s", g)
	}
	x.shiftTab()
	x.shiftTab()
	if focusedName(tr) != "close" {
		t.Errorf("Shift+Tab wraps inside the scope: %s", focusedName(tr))
	}
	// Closing it restores the focus to the trigger.
	x.enter()
	if focusedName(tr) != "trigger" {
		t.Errorf("restore: %s, want trigger", focusedName(tr))
	}
	x.tab()
	if focusedName(tr) != "after" {
		t.Errorf("Tab after the scope closed: %s", focusedName(tr))
	}
	// A scope that unmounts while the focus is elsewhere does not steal it
	// (a containing scope does not let it go elsewhere, ADR-160).
	x.shiftTab()
	x.enter()
	x.frame()
	tr.focusElement(keyed(tr, "after"))
	if focusedName(tr) != "d1" {
		t.Errorf("Focus outside a containing scope moved the focus to %s", focusedName(tr))
	}
	setContain.Set(false)
	x.frame()
	tr.focusElement(keyed(tr, "after"))
	setOpen.Set(false)
	x.frame()
	if focusedName(tr) != "after" {
		t.Errorf("restore stole the focus: %s", focusedName(tr))
	}
}

// ---------------------------------------------------------------------
// Golden: a focus ring after Tab.

func TestFocusRingGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			b := headless.New(headless.Options{Size: platform.Size{W: 240, H: 120}, Scale: scale})
			root := div(Class("flex h-full items-center justify-center gap-4 bg-slate-100"),
				button(Class("rounded-lg bg-indigo-500 px-4 py-2 text-white focus-visible:ring-2 "+
					"focus-visible:ring-indigo-500 focus-visible:ring-offset-2"), Text("Save")),
				button(Class("rounded-lg bg-white px-4 py-2 text-slate-900 focus-visible:ring-2 "+
					"focus-visible:ring-indigo-500 focus-visible:ring-offset-2"), Text("Cancel")))
			host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
			if err != nil {
				t.Fatal(err)
			}
			b.Drain(host)
			x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
			t.Cleanup(host.stopTimer)
			x.w.SetFocused(true)
			x.tab()
			x.frame()
			golden.Image(t, fmt.Sprintf("focus_ring_x%g", scale), x.w.Frame())
		})
	}
}

// ---------------------------------------------------------------------
// The Counter of ARCHITECTURE section 1, by keyboard.

func TestCounterByKeyboard(t *testing.T) {
	x := newKeys(t, goldenCounter(), 400, 300)
	x.tab()
	x.enter()
	x.space()
	x.frame()
	in := x.tree().Inspect()
	if h := findKind(&in, "h1"); h == nil || h.Text != "Count: 2" {
		t.Errorf("heading %+v after Tab, Enter, Space", h)
	}
}
