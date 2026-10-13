package h_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
)

// focus gives the headless window keyboard focus.
func (a *app) focus() {
	a.w.SetFocused(true)
	a.b.Drain(a.h)
}

// press presses and releases a key, with mods held.
func (a *app) press(k platform.Key, c platform.KeyCode, mods platform.Modifiers) {
	e := platform.KeyEvent{Action: platform.KeyPress, Code: c, Key: k, Location: c.Location(), Mods: mods}
	if !k.IsNamed() {
		e.Text = string(k)
	}
	a.w.Inject(e)
	a.w.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: c, Key: k, Location: c.Location(), Mods: mods})
	a.b.Drain(a.h)
}

// TestCounterByKeyboard operates the Counter of ARCHITECTURE section 1
// with the keyboard: Tab focuses the button, Enter and Space click it.
func TestCounterByKeyboard(t *testing.T) {
	a := open(t, Counter)
	a.focus()
	a.press(platform.KeyTab, platform.CodeTab, 0)
	a.press(platform.KeyEnter, platform.CodeEnter, 0)
	a.frame()
	if got := a.heading(); got != "Count: 1" {
		t.Errorf("after Tab and Enter: %q", got)
	}
	a.press(" ", platform.CodeSpace, 0)
	a.frame()
	if got := a.heading(); got != "Count: 2" {
		t.Errorf("after Space: %q", got)
	}
}

func TestKeyboardAndFocusProps(t *testing.T) {
	var log []string
	add := func(s string) { log = append(log, s) }
	var input tela.ElementRef
	root := func() tela.Node {
		input = tela.UseElementRef()
		tela.UseEffect(func() func() {
			input.Focus()
			return nil
		}, nil)
		return Div(Class("flex flex-col gap-2 p-2"),
			OnKeyDown(func(e tela.KeyboardEvent) {
				if e.Key == "Escape" {
					add("close")
				}
				if e.Ctrl && e.Code == tela.CodeKeyS {
					add("save")
				}
			}),
			OnKeyUp(func(e tela.KeyboardEvent) {
				if e.Key == tela.KeyShift {
					add("shift-up")
				}
			}),
			OnFocus(func(tela.FocusEvent) { add("focus") }),
			OnBlur(func(tela.FocusEvent) { add("blur") }),
			Div(Ref(input), TabIndex(0), Class("h-8 focus:bg-indigo-500"), Text("input")),
			Button(Key("ok"), AutoFocus(false), Text("OK")),
			Div(Key("first"), TabIndex(1), Text("first")),
		)
	}
	a := open(t, root)
	a.focus()
	if !input.Focused() {
		t.Fatal("Focus in an effect did not focus the element")
	}
	a.press(platform.KeyEscape, platform.CodeEscape, 0)
	a.press("s", platform.CodeKeyS, platform.ModCtrl)
	a.press(platform.KeyShift, platform.CodeShiftLeft, platform.ModShift)
	a.press(platform.KeyTab, platform.CodeTab, 0) // to OK (after the input in tree order)
	if input.Focused() {
		t.Error("Tab did not move the focus")
	}
	input.Blur() // not focused: nothing
	a.press(platform.KeyTab, platform.CodeTab, platform.ModShift)
	input.Blur()
	if got, want := strings.Join(log, " "), "focus close save shift-up blur focus blur focus blur"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestFocusPropsPanic(t *testing.T) {
	for name, tc := range map[string]struct {
		f    func()
		want string
	}{
		"OnKeyDown": {func() { OnKeyDown(nil) }, "OnKeyDown(nil)"},
		"OnKeyUp":   {func() { OnKeyUp(nil) }, "OnKeyUp(nil)"},
		"OnFocus":   {func() { OnFocus(nil) }, "OnFocus(nil)"},
		"OnBlur":    {func() { OnBlur(nil) }, "OnBlur(nil)"},
		"Ref":       {func() { Ref(tela.ElementRef{}) }, "UseElementRef"},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(r.(string), tc.want) {
					t.Errorf("%s: panic %v", name, r)
				}
			}()
			tc.f()
		}()
	}
}
