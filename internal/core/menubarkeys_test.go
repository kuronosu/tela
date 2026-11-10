package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// menuBarKeysPage is a focusable field and a bar with MenuBarKeys that logs
// the keys it gets; prevent makes the field prevent its keys' defaults.
func menuBarKeysPage(log *[]string, prevent *bool) Node {
	return div(Class("flex flex-col gap-2 p-2"),
		div(Key("field"), TabIndex(0), Class("h-10 w-40"),
			On("keydown", func(e KeyboardEvent) {
				if *prevent {
					e.PreventDefault()
				}
			}),
			On("keyup", func(e KeyboardEvent) {
				if *prevent {
					e.PreventDefault()
				}
			})),
		div(Key("bar"), Class("h-8 w-40"), MenuBarKeys(func(e KeyboardEvent) bool { *log = append(*log, string(e.Key)); return true })),
		div(Key("bar2"), Class("h-8 w-40"), MenuBarKeys(func(e KeyboardEvent) bool { *log = append(*log, "second:"+string(e.Key)); return true })),
	)
}

func TestMenuBarKeys(t *testing.T) {
	var log []string
	prevent := false
	x := newKeys(t, menuBarKeysPage(&log, &prevent), 300, 200)
	x.tab() // the field
	take := func() string {
		s := strings.Join(log, " ")
		log = nil
		return s
	}
	alt := func(a platform.KeyAction) platform.KeyEvent {
		mods := platform.ModAlt
		if a == platform.KeyRelease {
			mods = 0
		}
		return keyEv(a, platform.KeyAlt, platform.CodeAltLeft, mods)
	}

	// F10 alone, the first bar only.
	x.press(platform.KeyF10, platform.CodeF10, 0)
	if got := take(); got != "F10" {
		t.Errorf("F10: %q", got)
	}
	// Shift+F10 is the context menu's, Ctrl+F10 is not the bar's either.
	x.press(platform.KeyF10, platform.CodeF10, platform.ModShift)
	x.press(platform.KeyF10, platform.CodeF10, platform.ModCtrl)
	if got := take(); got != "" {
		t.Errorf("Shift+F10, Ctrl+F10: %q", got)
	}
	// A lone Alt, at its release.
	x.keys(alt(platform.KeyPress))
	if got := take(); got != "" {
		t.Errorf("Alt down: %q", got)
	}
	x.keys(alt(platform.KeyRelease))
	if got := take(); got != "Alt" {
		t.Errorf("lone Alt: %q", got)
	}
	// Alt with another key in between (Alt+Tab, Alt+F) is no lone Alt.
	x.keys(alt(platform.KeyPress), keyEv(platform.KeyPress, "f", platform.CodeKeyF, platform.ModAlt),
		keyEv(platform.KeyRelease, "f", platform.CodeKeyF, platform.ModAlt), alt(platform.KeyRelease))
	// Nor with a press of the pointer in between (Alt+click).
	x.keys(alt(platform.KeyPress))
	px, py := x.center("bar")
	x.click(px, py)
	x.keys(alt(platform.KeyRelease))
	// Nor with Ctrl held.
	x.keys(keyEv(platform.KeyPress, platform.KeyAlt, platform.CodeAltLeft, platform.ModAlt|platform.ModCtrl),
		keyEv(platform.KeyRelease, platform.KeyAlt, platform.CodeAltLeft, platform.ModCtrl))
	// Nor with a wheel turned in between (Alt+wheel): Win32 counts
	// WM_MOUSEWHEEL as an input that cancels the menu activation.
	x.keys(alt(platform.KeyPress))
	x.wheel(platform.ScrollEvent{Pos: platform.Point{X: px, Y: py}, Steps: platform.Point{Y: 1}, Mods: platform.ModAlt})
	x.keys(alt(platform.KeyRelease))
	if got := take(); got != "" {
		t.Errorf("Alt with another key, a click, Ctrl or a wheel: %q", got)
	}
	// Nor when the window loses the focus in between (Alt+Tab to another
	// app): the backend's synthetic release does not count.
	x.keys(alt(platform.KeyPress))
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	if got := take(); got != "" {
		t.Errorf("Alt across a window switch: %q", got)
	}
	// A handler that prevents the key keeps it from the bar.
	if focusedName(x.tree()) != "field" {
		x.tab()
	}
	if f := focusedName(x.tree()); f != "field" {
		t.Fatalf("focus on %q, want the field", f)
	}
	prevent = true
	x.press(platform.KeyF10, platform.CodeF10, 0)
	x.keys(alt(platform.KeyPress), alt(platform.KeyRelease))
	if got := take(); got != "" {
		t.Errorf("prevented: %q", got)
	}
}

// An inert bar (outside a modal, under Inert) does not take the keys; the
// next live one does.
func TestMenuBarKeysSkipInert(t *testing.T) {
	var log []string
	root := div(
		div(Key("bar"), Inert(true), Class("h-8"), MenuBarKeys(func(KeyboardEvent) bool { log = append(log, "inert"); return true })),
		div(Key("bar2"), Class("h-8"), MenuBarKeys(func(KeyboardEvent) bool { log = append(log, "live"); return true })),
	)
	x := newKeys(t, root, 300, 200)
	x.press(platform.KeyF10, platform.CodeF10, 0)
	if got := strings.Join(log, " "); got != "live" {
		t.Errorf("F10 reached %q", got)
	}
}

// A bar that is not displayed (display:none, or in a subtree that is) does
// not take the keys; the next one does. A bar whose handler takes nothing
// (no trigger to focus) leaves the key unhandled.
func TestMenuBarKeysSkipHidden(t *testing.T) {
	var log []string
	take := true
	root := div(
		div(Class("hidden"), div(Key("bar0"), Class("h-8"), MenuBarKeys(func(KeyboardEvent) bool { log = append(log, "nested"); return true }))),
		div(Key("bar"), Class("hidden h-8"), MenuBarKeys(func(KeyboardEvent) bool { log = append(log, "hidden"); return true })),
		div(Key("bar2"), Class("h-8"), MenuBarKeys(func(KeyboardEvent) bool { log = append(log, "shown"); return take })),
	)
	x := newKeys(t, root, 300, 200)
	x.press(platform.KeyF10, platform.CodeF10, 0)
	if got := strings.Join(log, " "); got != "shown" {
		t.Errorf("F10 reached %q", got)
	}
	take = false
	ke := KeyboardEvent{Key: KeyValue(platform.KeyF10)}
	if x.tree().menuKeyDown(ke, &eventState{}) {
		t.Error("F10 reported handled by a bar that took nothing")
	}
}

// The focus events tell where the focus came from and goes to, as the
// stable refs of ActiveElement: the element's own ref when it has one.
func TestRelatedTargetAndStableActiveElement(t *testing.T) {
	var got []string
	a, b := NewElementRef(), NewElementRef()
	name := func(r ElementRef, ok bool) string {
		switch {
		case !ok:
			return "none"
		case r == a:
			return "a"
		case r == b:
			return "b"
		}
		return "other"
	}
	root := div(
		div(Key("a"), AttachRef(a), TabIndex(0), Class("h-8"),
			On("focus", func(e FocusEvent) { got = append(got, "focus a from "+name(RelatedTarget(e))) }),
			On("blur", func(e FocusEvent) { got = append(got, "blur a to "+name(RelatedTarget(e))) })),
		div(Key("b"), AttachRef(b), TabIndex(0), Class("h-8"),
			On("focus", func(e FocusEvent) { got = append(got, "focus b from "+name(RelatedTarget(e))) })),
		div(Key("c"), TabIndex(0), Class("h-8")),
	)
	x := newKeys(t, root, 300, 200)
	x.tab()
	x.tab()
	if s := strings.Join(got, ", "); s != "focus a from none, blur a to b, focus b from a" {
		t.Errorf("focus events: %s", s)
	}
	w := WindowHandle{t: x.tree()}
	if r, ok := w.ActiveElement(); !ok || r != b {
		t.Errorf("ActiveElement is not b's own ref")
	}
	// An element without a ref gets the same ref on every call.
	x.tab()
	r1, _ := w.ActiveElement()
	r2, _ := w.ActiveElement()
	if r1 != r2 || r1 == a || r1 == b || !r1.Focused() {
		t.Errorf("ActiveElement of c: %v, %v", r1, r2)
	}
	if !Contains(r1, r1) || Contains(a, r1) {
		t.Errorf("Contains")
	}
}

// A lone Alt does not survive what a window manager's Alt+drag leaves the
// app (ADR-260): the pointer leaving the window (the grab) and a resize.
// (A change of the window's focus is TestMenuBarKeys'.)
func TestMenuBarKeysLoneAltGuards(t *testing.T) {
	var log []string
	prevent := false
	x := newKeys(t, menuBarKeysPage(&log, &prevent), 300, 200)
	altDown := keyEv(platform.KeyPress, platform.KeyAlt, platform.CodeAltLeft, platform.ModAlt)
	altUp := keyEv(platform.KeyRelease, platform.KeyAlt, platform.CodeAltLeft, 0)
	px, py := x.center("bar")
	x.send(platform.PointerEvent{Kind: platform.PointerMouse, Action: platform.PointerMove, Pos: platform.Point{X: px, Y: py}})
	x.keys(altDown)
	x.send(platform.PointerEvent{Kind: platform.PointerMouse, Action: platform.PointerLeave, Pos: platform.Point{X: px, Y: py}})
	x.keys(altUp)
	x.keys(altDown)
	x.w.Resize(platform.Size{W: 320, H: 220})
	x.b.Drain(x.h)
	x.keys(altUp)
	if got := strings.Join(log, " "); got != "" {
		t.Errorf("Alt across a pointer leave or a resize: %q", got)
	}
	// And a lone Alt still works after them.
	x.keys(altDown, altUp)
	if got := strings.Join(log, " "); got != "Alt" {
		t.Errorf("lone Alt: %q", got)
	}
}

// MenuBarAccess: Alt and a character is the first live bar's Mnemonic as
// the key's default action, and AltHeld follows Alt held alone.
func TestMenuBarAccess(t *testing.T) {
	var log []string
	take, prevent := true, false
	root := div(Class("flex flex-col gap-2 p-2"),
		div(Key("field"), TabIndex(0), Class("h-10 w-40"),
			On("keydown", func(e KeyboardEvent) {
				log = append(log, "field:"+string(e.Key))
				if prevent {
					e.PreventDefault()
				}
			})),
		div(Key("bar"), Class("h-8 w-40"),
			MenuBarKeys(func(KeyboardEvent) bool { return true }),
			MenuBarAccess(MenuBarAccessOptions{
				Mnemonic: func(e KeyboardEvent) bool { log = append(log, "mnemonic:"+string(e.Key)); return take },
				AltHeld:  func(held bool) { log = append(log, fmt.Sprintf("held:%v", held)) },
			})),
	)
	x := newKeys(t, root, 300, 200)
	x.tab()
	got := func() string {
		s := strings.Join(log, " ")
		log = nil
		return s
	}
	altDown := keyEv(platform.KeyPress, platform.KeyAlt, platform.CodeAltLeft, platform.ModAlt)
	altUp := keyEv(platform.KeyRelease, platform.KeyAlt, platform.CodeAltLeft, 0)
	// tap presses and releases a key (the release is not logged: only
	// keydowns are).
	tap := func(k platform.Key, c platform.KeyCode, mods platform.Modifiers, repeat bool) {
		e := keyEv(platform.KeyPress, k, c, mods)
		x.keys(e)
		if repeat {
			e.Repeat = true
			x.keys(e)
		}
		x.keys(keyEv(platform.KeyRelease, k, c, mods))
	}
	x.keys(altDown)
	tap("f", platform.CodeKeyF, platform.ModAlt, false)
	x.keys(altUp)
	if s := got(); s != "field:Alt held:true field:f mnemonic:f held:false" {
		t.Errorf("Alt+F: %s", s)
	}
	// Not with Ctrl (AltGr on Windows), nor a repeat, nor when the field
	// prevents it; Shift is allowed.
	tap("f", platform.CodeKeyF, platform.ModAlt|platform.ModCtrl, false)
	if s := got(); s != "field:f" {
		t.Errorf("Ctrl+Alt+F: %s", s)
	}
	take = false
	tap("f", platform.CodeKeyF, platform.ModAlt, true)
	if s := got(); s != "field:f mnemonic:f field:f" {
		t.Errorf("Alt+F and its repeat: %s", s)
	}
	take = true
	prevent = true
	tap("f", platform.CodeKeyF, platform.ModAlt, false)
	prevent = false
	if s := got(); s != "field:f" {
		t.Errorf("a prevented Alt+F: %s", s)
	}
	tap("F", platform.CodeKeyF, platform.ModAlt|platform.ModShift, false)
	if s := got(); s != "field:F mnemonic:F" {
		t.Errorf("Alt+Shift+F: %s", s)
	}
	// A key without Alt ends the cues (a release the window missed), and
	// so does the window losing the focus.
	x.keys(altDown)
	tap("a", platform.CodeKeyA, 0, false)
	x.keys(altUp)
	x.keys(altDown)
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	if s := got(); s != "field:Alt held:true field:a held:false field:Alt held:true held:false" {
		t.Errorf("cues: %s", s)
	}
}

// FocusFromNowhere: a focus with nothing focused yet, or after a blur to
// nothing, comes from nowhere; the window gaining the focus back, and the
// focus a scope gives back after its focused element unmounted, do not.
func TestFocusFromNowhere(t *testing.T) {
	var got []string
	var setInner Setter[bool]
	app := func() Node {
		inner, si := UseState(false)
		setInner = si
		return div(Key("page"), Class("h-full"),
			button(Key("a"), Class("block h-10 w-20"), Text("a"),
				On("focus", func(e FocusEvent) { got = append(got, fmt.Sprintf("a:%v", FocusFromNowhere(e))) })),
			If(inner, div(Key("inner"), Scope(ScopeOptions{Restore: true, AutoFocus: true}),
				button(Key("b"), Class("block h-10 w-20"), Text("b")))))
	}
	x := newKeys(t, Component(app), 400, 300)
	tr := x.tree()
	take := func() string {
		s := strings.Join(got, " ")
		got = nil
		return s
	}
	x.tab()
	if s := take(); s != "a:true" {
		t.Errorf("first Tab: %s", s)
	}
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	if s := take(); s != "a:false" {
		t.Errorf("window focus back: %s", s)
	}
	setInner.Set(true)
	x.frame()
	if g := focusedName(tr); g != "b" {
		t.Fatalf("AutoFocus: %s", g)
	}
	setInner.Set(false)
	x.frame()
	if g := focusedName(tr); g != "a" {
		t.Fatalf("restore: %s", g)
	}
	if s := take(); s != "a:false" {
		t.Errorf("restored after the unmount: %s", s)
	}
	x.click(x.center("page"))
	if tr.Focused() != nil {
		t.Fatalf("press on nothing: %s focused", focusedName(tr))
	}
	x.tab()
	if s := take(); s != "a:true" {
		t.Errorf("Tab after a blur to nothing: %s", s)
	}
}

// The bar told that Alt is held hears its release even when it went
// inert or another bar became the live one meanwhile (Alt+Enter opened
// a modal dialog), so its cues do not stay (review of ADR-260).
func TestMenuBarAltHeldReleaseReachesTheToldBar(t *testing.T) {
	var log []string
	var setInert Setter[bool]
	app := func() Node {
		inert, si := UseState(false)
		setInert = si
		return div(Class("flex flex-col gap-2 p-2"),
			div(Key("field"), TabIndex(0), Class("h-10 w-40")),
			div(Key("bar"), Inert(inert), Class("h-8 w-40"),
				MenuBarKeys(func(KeyboardEvent) bool { return true }),
				MenuBarAccess(MenuBarAccessOptions{AltHeld: func(held bool) { log = append(log, fmt.Sprintf("bar:%v", held)) }})),
			div(Key("bar2"), Class("h-8 w-40"),
				MenuBarKeys(func(KeyboardEvent) bool { return true }),
				MenuBarAccess(MenuBarAccessOptions{AltHeld: func(held bool) { log = append(log, fmt.Sprintf("bar2:%v", held)) }})))
	}
	x := newKeys(t, Component(app), 300, 200)
	x.tab()
	x.keys(keyEv(platform.KeyPress, platform.KeyAlt, platform.CodeAltLeft, platform.ModAlt))
	setInert.Set(true)
	x.frame()
	x.keys(keyEv(platform.KeyRelease, platform.KeyAlt, platform.CodeAltLeft, 0))
	if got := strings.Join(log, " "); got != "bar:true bar:false" {
		t.Errorf("AltHeld: %s", got)
	}
}

// The live bar follows mounts, unmounts, inert changes and display
// changes in tree order, with the bars kept in a list (no walk of the
// tree; review of ADR-260).
func TestMenuBarLiveBarFollowsTheTree(t *testing.T) {
	var log []string
	var setFirst, setHidden, setInert Setter[bool]
	bar := func(name string, extra ...Child) Node {
		return div(append([]Child{Key(name), Class("h-8 w-40"),
			MenuBarKeys(func(KeyboardEvent) bool { log = append(log, name); return true })}, extra...)...)
	}
	app := func() Node {
		first, sf := UseState(false)
		hidden, sh := UseState(false)
		inert, si := UseState(false)
		setFirst, setHidden, setInert = sf, sh, si
		hide := "block"
		if hidden {
			hide = "hidden"
		}
		return div(Class("flex flex-col gap-2 p-2"),
			div(Key("field"), TabIndex(0), Class("h-10 w-40")),
			If(first, bar("first")),
			div(Key("wrap"), Class(hide), bar("second", Inert(inert))),
			bar("third"))
	}
	x := newKeys(t, Component(app), 300, 200)
	x.tab()
	f10 := func() string {
		log = nil
		x.press(platform.KeyF10, platform.CodeF10, 0)
		return strings.Join(log, " ")
	}
	if got := f10(); got != "second" {
		t.Errorf("at mount: %s", got)
	}
	setFirst.Set(true)
	x.frame()
	if got := f10(); got != "first" {
		t.Errorf("a bar mounted before: %s", got)
	}
	setFirst.Set(false)
	setHidden.Set(true)
	x.frame()
	if got := f10(); got != "third" {
		t.Errorf("the second hidden by an ancestor: %s", got)
	}
	setHidden.Set(false)
	setInert.Set(true)
	x.frame()
	if got := f10(); got != "third" {
		t.Errorf("the second inert: %s", got)
	}
	setInert.Set(false)
	x.frame()
	if got := f10(); got != "second" {
		t.Errorf("the second live again: %s", got)
	}
}
