package core

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
)

func (x *inputHarness) escape() { x.press(platform.KeyEscape, platform.CodeEscape, 0) }

// TestSeveralHandlersRun: several props handling the same event on one
// element all run, in the order they were given (package aria merges its
// props with the user's, ADR-096).
func TestSeveralHandlersRun(t *testing.T) {
	var r recorder
	root := div(Key("box"), Class("size-20"),
		r.mouse("click", "1"), r.pointer("pointerdown", "1"),
		r.mouse("click", "2"), r.pointer("pointerdown", "2"),
		r.mouse("click", "3"))
	x := newInput(t, root, 200, 200)
	x.click(10, 10)
	if g := r.String(); g != "pointerdown:1 pointerdown:2 click:1 click:2 click:3" {
		t.Errorf("handlers: %s", g)
	}
}

// outsideApp is a page with a button, and a popover in a Portal that
// listens to presses outside it; a submenu rendered from inside the
// popover, in a Portal of its own, is inside it.
func outsideApp(r *recorder, opts func() OutsideOptions) Node {
	return div(Key("page"), Class("h-full"),
		button(Key("btn"), Class("block h-10 w-20"), r.mouse("click", "btn"), r.pointer("pointerdown", "btn"), Text("b")),
		Portal(div(Key("pop"), InteractOutside(opts()), Class("fixed top-[100px] left-[200px] size-20"),
			Portal(div(Key("sub"), Class("fixed top-[100px] left-[300px] size-10"))))))
}

func TestInteractOutside(t *testing.T) {
	var r recorder
	stop, prevent := false, false
	opts := func() OutsideOptions {
		return OutsideOptions{
			OnStart: func(e PointerEvent) {
				r.add("start")
				if stop {
					e.StopPropagation()
				}
			},
			OnEnd: func(e PointerEvent) {
				r.add("end")
				if prevent {
					e.PreventDefault()
				}
			},
		}
	}
	x := newInput(t, outsideApp(&r, opts), 400, 300)
	press := func(px, py float32) string {
		r.reset()
		x.click(px, py)
		return r.String()
	}
	if g := press(210, 110); g != "" {
		t.Errorf("press inside the popover: %q", g)
	}
	if g := press(310, 110); g != "" {
		t.Errorf("press on the submenu, a portal rendered from inside: %q", g)
	}
	if g := press(10, 10); g != "start pointerdown:btn end click:btn" {
		t.Errorf("press outside: %q", g)
	}
	if g := press(100, 200); g != "start end" {
		t.Errorf("press on the page: %q", g)
	}
	// A press that starts outside and ends inside ends nothing.
	r.reset()
	x.send(move(10, 10), down(10, 10), moveHeld(210, 110), up(210, 110))
	if g := r.String(); g != "start pointerdown:btn" {
		t.Errorf("drag from outside to inside: %q", g)
	}
	// The secondary and middle buttons are presses too (Radix's
	// DismissableLayer); back and forward are not.
	for _, b := range []platform.PointerButton{platform.ButtonSecondary, platform.ButtonMiddle} {
		r.reset()
		x.send(move(10, 10), ptr(platform.PointerDown, 10, 10, b, b.Mask()), ptr(platform.PointerUp, 10, 10, b, 0))
		if g := r.String(); g != "start pointerdown:btn end" {
			t.Errorf("button %v: %q", b, g)
		}
	}
	r.reset()
	x.send(move(10, 10), ptr(platform.PointerDown, 10, 10, platform.ButtonBack, platform.ButtonBack.Mask()),
		ptr(platform.PointerUp, 10, 10, platform.ButtonBack, 0))
	if g := r.String(); g != "pointerdown:btn" {
		t.Errorf("back button: %q", g)
	}
	// A pen's barrel button is not a press; its tip is.
	pen := func(a platform.PointerAction, b platform.PointerButton, held platform.Buttons) platform.PointerEvent {
		return platform.PointerEvent{Action: a, Kind: platform.PointerPen, ID: 1, Pos: platform.Point{X: 10, Y: 10}, Button: b, Buttons: held}
	}
	r.reset()
	x.send(pen(platform.PointerDown, platform.ButtonSecondary, platform.ButtonSecondary.Mask()), pen(platform.PointerUp, platform.ButtonSecondary, 0))
	if g := r.String(); g != "pointerdown:btn" {
		t.Errorf("pen barrel button: %q", g)
	}
	r.reset()
	x.b.Advance(time.Second)
	x.send(pen(platform.PointerDown, platform.ButtonPrimary, platform.ButtonPrimary.Mask()), pen(platform.PointerUp, platform.ButtonPrimary, 0))
	if g := r.String(); !strings.HasPrefix(g, "start pointerdown:btn end") {
		t.Errorf("pen tip: %q", g)
	}
	x.send(pen(platform.PointerLeave, platform.ButtonNone, 0))
	// A primary button pressed while the secondary one is held is not a
	// new press (chorded buttons): the press started at the first button.
	r.reset()
	x.b.Advance(time.Second)
	x.send(move(10, 10),
		ptr(platform.PointerDown, 10, 10, platform.ButtonSecondary, platform.ButtonSecondary.Mask()),
		ptr(platform.PointerDown, 10, 10, platform.ButtonPrimary, platform.ButtonSecondary.Mask()|platform.ButtonPrimary.Mask()),
		ptr(platform.PointerUp, 10, 10, platform.ButtonPrimary, platform.ButtonSecondary.Mask()),
		ptr(platform.PointerUp, 10, 10, platform.ButtonSecondary, 0))
	if g := r.String(); !strings.HasPrefix(g, "start pointerdown:btn") || strings.Count(g, "start") != 1 || !strings.HasSuffix(g, "end") {
		t.Errorf("chorded buttons: %q", g)
	}
	// Touch presses count.
	r.reset()
	x.send(platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerTouch, ID: 1, Pos: platform.Point{X: 10, Y: 10}, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()},
		platform.PointerEvent{Action: platform.PointerUp, Kind: platform.PointerTouch, ID: 1, Pos: platform.Point{X: 10, Y: 10}, Button: platform.ButtonPrimary})
	if g := r.String(); !strings.HasPrefix(g, "start pointerdown:btn end") {
		t.Errorf("touch press: %q", g)
	}
	// StopPropagation at the start keeps the pointerdown from the target;
	// PreventDefault at the end cancels the click.
	stop, prevent = true, true
	x.frame() // the props of the new closures
	if g := press(10, 10); g != "start end" {
		t.Errorf("stopped and prevented: %q", g)
	}
}

// overlayApp: a dialog (an overlay) with a button that opens a popover
// (an overlay above it, in a Portal rendered from the dialog).
type overlayProbe struct {
	r                 recorder
	setDialog, setPop Setter[bool]
	dialog, pop       ElementRef
	modal, popModal   bool
}

func (o *overlayProbe) app() Node {
	comp := func() Node {
		dlg, sd := UseState(true)
		pop, sp := UseState(false)
		o.setDialog, o.setPop = sd, sp
		o.dialog, o.pop = UseElementRef(), UseElementRef()
		overlay := func(name string, close Setter[bool]) Prop {
			return Overlay(OverlayOptions{
				Modal:                  o.modal && (name == "dialog" || o.popModal),
				OnEscape:               func(KeyboardEvent) { o.r.add("escape:" + name); close.Set(false) },
				OnInteractOutsideStart: func(PointerEvent) { o.r.add("start:" + name) },
				OnInteractOutside:      func(PointerEvent) { o.r.add("outside:" + name); close.Set(false) },
			})
		}
		return div(Key("page"), Class("h-full"),
			button(Key("a"), Class("block h-10 w-20"), o.r.mouse("click", "a"), Text("A")),
			button(Key("b"), Class("block h-10 w-20"), Text("B")),
			If(dlg, Portal(div(Key("dialog"), AttachRef(o.dialog), overlay("dialog", sd), Class("fixed top-[100px] left-[100px] h-30 w-60"),
				button(Key("d1"), Class("block h-10 w-20"), On("click", func(MouseEvent) { sp.Set(true) }), Text("Open")),
				button(Key("d2"), Class("block h-10 w-20"), Text("Two")),
				If(pop, Portal(div(Key("pop"), AttachRef(o.pop), overlay("pop", sp), Class("fixed top-[100px] left-[400px] size-20"),
					button(Key("p1"), Class("block h-10 w-20"), Text("P")))))))))
	}
	return Component(comp)
}

func TestOverlayStack(t *testing.T) {
	o := &overlayProbe{}
	x := newKeys(t, o.app(), 600, 400)
	tr := x.tree()
	if !IsTopOverlay(o.dialog) {
		t.Fatal("the dialog is not the top overlay")
	}
	x.click(110, 110) // d1 opens the popover
	x.frame()
	if !IsTopOverlay(o.pop) || IsTopOverlay(o.dialog) {
		t.Fatalf("top: dialog %v, popover %v", IsTopOverlay(o.dialog), IsTopOverlay(o.pop))
	}
	// Escape reaches only the top overlay.
	o.r.reset()
	x.escape()
	x.frame()
	if g := o.r.String(); g != "escape:pop" || keyed(tr, "pop") != nil || keyed(tr, "dialog") == nil {
		t.Errorf("Escape with a popover over the dialog: %q", g)
	}
	// So does a press outside both.
	x.click(110, 110)
	x.frame()
	o.r.reset()
	x.click(500, 350)
	x.frame()
	if g := o.r.String(); g != "start:pop outside:pop" || keyed(tr, "dialog") == nil {
		t.Errorf("outside press with a popover over the dialog: %q", g)
	}
	// A press inside the dialog is outside the popover only... which is
	// closed now: the dialog is top, and a press in it is inside it.
	o.r.reset()
	x.click(110, 160)
	if g := o.r.String(); g != "" {
		t.Errorf("press inside the top overlay: %q", g)
	}
	// Escape on the dialog, top again.
	o.r.reset()
	x.escape()
	x.frame()
	if g := o.r.String(); g != "escape:dialog" || keyed(tr, "dialog") != nil {
		t.Errorf("Escape on the dialog: %q", g)
	}
	// Without overlays, Escape does nothing special and a press is a press.
	o.r.reset()
	x.click(10, 10)
	if g := o.r.String(); g != "click:a" {
		t.Errorf("without overlays: %q", g)
	}
}

// TestEscapePrevented: a handler that prevents the keydown keeps the top
// overlay open (a combobox closing its own list).
func TestEscapePrevented(t *testing.T) {
	var r recorder
	root := div(Key("page"), On("keydown", func(e KeyboardEvent) {
		if e.Key == "Escape" {
			r.add("handled")
			e.PreventDefault()
		}
	}),
		button(Key("btn"), Text("b")),
		Portal(div(Key("pop"), Overlay(OverlayOptions{OnEscape: func(KeyboardEvent) { r.add("escape") }}), Class("fixed size-10"))))
	x := newKeys(t, root, 200, 200)
	x.tab()
	x.escape()
	if g := r.String(); g != "handled" {
		t.Errorf("prevented Escape: %q", g)
	}
}

// TestModalInert: a modal dialog makes the rest of the window inert:
// not hit, not focusable, out of the Tab order and hidden in the
// semantics tree; an overlay above it stays alive.
func TestModalInert(t *testing.T) {
	o := &overlayProbe{modal: true}
	x := newKeys(t, o.app(), 600, 400)
	tr := x.tree()
	snap := tr.AriaSnapshot()
	if strings.Contains(snap, `"A"`) || !strings.Contains(snap, `button "Open"`) {
		t.Errorf("snapshot with a modal dialog:\n%s", snap)
	}
	n := tr.Semantics().Find(semantics.NodeID(keyed(tr, "a").semID))
	if n == nil || !n.Has(semantics.StateHidden) || n.Has(semantics.StateFocusable) {
		t.Errorf("the page's button A in the semantics tree: %+v", n)
	}
	// The pointer: a click on A hits the root (a press outside the
	// dialog).
	o.r.reset()
	if g := hitName(tr, 10, 10); g != "root" {
		t.Errorf("hit on A behind the modal: %s, want root", g)
	}
	x.click(10, 10)
	x.frame()
	if g := o.r.String(); g != "start:dialog outside:dialog" {
		t.Errorf("click on A behind the modal: %q", g)
	}
	if keyed(tr, "dialog") != nil {
		t.Fatal("an outside press did not close the dialog")
	}
	// Closed: A works again.
	o.r.reset()
	x.click(10, 10)
	if g := o.r.String(); g != "click:a" {
		t.Errorf("click on A after the dialog closed: %q", g)
	}
	if tr.Focused() == nil || focusedName(tr) != "a" {
		t.Errorf("focus after the click: %s", focusedName(tr))
	}
	// Opening it again blurs A; Tab stays in the dialog, then reaches the
	// popover above it.
	o.setDialog.Set(true)
	x.frame()
	if tr.Focused() != nil {
		t.Errorf("the focus stayed on %s behind the modal", focusedName(tr))
	}
	var got []string
	for range 3 {
		x.tab()
		got = append(got, focusedName(tr))
	}
	if g := strings.Join(got, " "); g != "d1 d2 d1" {
		t.Errorf("Tab with a modal dialog: %s", g)
	}
	x.enter() // opens the popover
	x.frame()
	got = got[:0]
	for range 4 {
		x.tab()
		got = append(got, focusedName(tr))
	}
	if g := strings.Join(got, " "); g != "d2 p1 d1 d2" {
		t.Errorf("Tab with a popover above the modal dialog: %s", g)
	}
	if g := hitName(tr, 410, 110); g != "p1" {
		t.Errorf("hit in the popover above the modal: %s", g)
	}
	if snap := tr.AriaSnapshot(); !strings.Contains(snap, `button "P"`) || strings.Contains(snap, `"B"`) {
		t.Errorf("snapshot with a popover over the modal dialog:\n%s", snap)
	}
}

// TestNestedModal: the topmost modal overlay wins: a modal popover over a
// modal dialog makes the dialog inert too, until it closes.
func TestNestedModal(t *testing.T) {
	o := &overlayProbe{modal: true, popModal: true}
	x := newKeys(t, o.app(), 600, 400)
	tr := x.tree()
	x.click(110, 110) // d1 opens the popover
	x.frame()
	if g := hitName(tr, 110, 150); g != "root" {
		t.Errorf("hit on d2 under a modal popover: %s, want root", g)
	}
	if g := hitName(tr, 410, 110); g != "p1" {
		t.Errorf("hit in the modal popover: %s", g)
	}
	if snap := tr.AriaSnapshot(); snap != `- button "P"` {
		t.Errorf("snapshot with a modal popover:\n%s", snap)
	}
	x.escape()
	x.frame()
	if g := hitName(tr, 110, 150); g != "d2" {
		t.Errorf("hit on d2 after the popover closed: %s", g)
	}
}

// TestInertProp: Inert makes a subtree inert, and back.
func TestInertProp(t *testing.T) {
	var set Setter[bool]
	var r recorder
	comp := func() Node {
		inert, s := UseState(false)
		set = s
		return div(Key("page"),
			button(Key("a"), Class("block h-10 w-20"), Text("A")),
			div(Key("zone"), Inert(inert),
				button(Key("b"), Class("block h-10 w-20"), r.mouse("click", "b"), Text("B")),
				Portal(button(Key("c"), Class("fixed top-[100px] left-[100px] h-10 w-20"), r.mouse("click", "c"), Text("C")))))
	}
	x := newKeys(t, Component(comp), 300, 300)
	tr := x.tree()
	x.tab()
	x.tab()
	if focusedName(tr) != "b" {
		t.Fatalf("focus %s", focusedName(tr))
	}
	set.Set(true)
	x.frame()
	if tr.Focused() != nil {
		t.Errorf("the focus stayed on %s, now inert", focusedName(tr))
	}
	x.click(10, 50)
	x.click(110, 110)
	if g := r.String(); g != "" {
		t.Errorf("clicks on inert elements: %q", g)
	}
	var got []string
	for range 2 {
		x.tab()
		got = append(got, focusedName(tr))
	}
	if g := strings.Join(got, " "); g != "a a" {
		t.Errorf("Tab with an inert subtree: %s", g)
	}
	if snap := tr.AriaSnapshot(); snap != `- button "A"` {
		t.Errorf("snapshot with an inert subtree:\n%s", snap)
	}
	set.Set(false)
	x.frame()
	x.click(10, 50)
	x.click(110, 110)
	if g := r.String(); g != "click:b click:c" {
		t.Errorf("clicks after Inert(false): %q", g)
	}
}

// TestFocusManager: first, last, next and previous within a subtree, with
// and without wrap, tabbable or not, from the focus or another element.
func TestFocusManager(t *testing.T) {
	var scope, other ElementRef
	comp := func() Node {
		scope, other = UseElementRef(), UseElementRef()
		return div(
			button(Key("out"), AttachRef(other), Text("out")),
			div(Key("menu"), AttachRef(scope),
				button(Key("m1"), Text("1")),
				div(Key("plain"), Text("p")),
				div(Key("m2"), TabIndex(-1), Text("2")),
				button(Key("m3"), Disabled(true), Text("3")),
				Portal(button(Key("m4"), Class("fixed size-4"), Text("4")))))
	}
	x := newKeys(t, Component(comp), 300, 300)
	tr := x.tree()
	step := func(name string, f func(ElementRef, FocusOptions) bool, o FocusOptions) string {
		f(scope, o)
		return focusedName(tr)
	}
	if g := step("first", FocusFirst, FocusOptions{}); g != "m1" {
		t.Errorf("FocusFirst: %s", g)
	}
	var got []string
	for range 4 {
		got = append(got, step("next", FocusNext, FocusOptions{Wrap: true}))
	}
	if g := strings.Join(got, " "); g != "m2 m4 m1 m2" {
		t.Errorf("FocusNext with wrap: %s", g)
	}
	got = got[:0]
	for range 3 {
		got = append(got, step("prev", FocusPrevious, FocusOptions{Tabbable: true, Wrap: true}))
	}
	if g := strings.Join(got, " "); g != "m1 m4 m1" {
		t.Errorf("FocusPrevious, tabbable, with wrap: %s", g)
	}
	if ok := FocusNext(scope, FocusOptions{Tabbable: true}); !ok || focusedName(tr) != "m4" {
		t.Errorf("FocusNext tabbable from m1: %v, %s", ok, focusedName(tr))
	}
	if ok := FocusNext(scope, FocusOptions{}); ok || focusedName(tr) != "m4" {
		t.Errorf("FocusNext without wrap at the end moved to %s", focusedName(tr))
	}
	if g := step("last", FocusLast, FocusOptions{Tabbable: true}); g != "m4" {
		t.Errorf("FocusLast: %s", g)
	}
	if g := step("from", FocusNext, FocusOptions{From: other}); g != "m1" {
		t.Errorf("FocusNext from outside the subtree: %s", g)
	}
	if !ContainsFocus(scope) || ContainsFocus(other) {
		t.Error("ContainsFocus")
	}
	tr.focusElement(keyed(tr, "out"))
	if g := step("prev", FocusPrevious, FocusOptions{}); g != "m4" {
		t.Errorf("FocusPrevious from outside: %s", g)
	}
}

// TestScopeAutoFocus: a scope with AutoFocus focuses its first tabbable
// element when it mounts, the first focusable one when none is tabbable,
// and nothing when the focus is already inside.
func TestScopeAutoFocus(t *testing.T) {
	for _, c := range []struct {
		name string
		kids func() []Child
		want string
	}{
		{"tabbable", func() []Child {
			return []Child{div(Key("x"), TabIndex(-1), Text("x")), button(Key("first"), Text("1")), button(Key("second"), Text("2"))}
		}, "first"},
		{"focusable", func() []Child { return []Child{div(Key("x"), TabIndex(-1), Text("x"))} }, "x"},
		{"autofocus inside", func() []Child {
			return []Child{button(Key("first"), Text("1")), button(Key("second"), AutoFocus(true), Text("2"))}
		}, "second"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var set Setter[bool]
			comp := func() Node {
				open, s := UseState(false)
				set = s
				kids := append([]Child{Key("scope"), Scope(ScopeOptions{Contain: true, Restore: true, AutoFocus: true})}, c.kids()...)
				return div(button(Key("trigger"), Text("t")), If(open, div(kids...)))
			}
			x := newKeys(t, Component(comp), 300, 300)
			tr := x.tree()
			x.tab()
			set.Set(true)
			x.frame()
			if g := focusedName(tr); g != c.want {
				t.Errorf("focus %s, want %s", g, c.want)
			}
			set.Set(false)
			x.frame()
			if g := focusedName(tr); g != "trigger" {
				t.Errorf("restored to %s", g)
			}
		})
	}
}
