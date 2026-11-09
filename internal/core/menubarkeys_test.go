package core

import (
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
		div(Key("bar"), Class("h-8 w-40"), MenuBarKeys(func(e KeyboardEvent) { *log = append(*log, string(e.Key)) })),
		div(Key("bar2"), Class("h-8 w-40"), MenuBarKeys(func(e KeyboardEvent) { *log = append(*log, "second:"+string(e.Key)) })),
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
	if got := take(); got != "" {
		t.Errorf("Alt with another key, a click or Ctrl: %q", got)
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
		div(Key("bar"), Inert(true), Class("h-8"), MenuBarKeys(func(KeyboardEvent) { log = append(log, "inert") })),
		div(Key("bar2"), Class("h-8"), MenuBarKeys(func(KeyboardEvent) { log = append(log, "live") })),
	)
	x := newKeys(t, root, 300, 200)
	x.press(platform.KeyF10, platform.CodeF10, 0)
	if got := strings.Join(log, " "); got != "live" {
		t.Errorf("F10 reached %q", got)
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
