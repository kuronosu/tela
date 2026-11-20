package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The window's unhandled input (windowdefault.go): the listeners run for
// the keys and side buttons nothing handled, innermost first, and the first
// that uses the input ends the dispatch.

type unhandledLog struct {
	keys    []string
	buttons []MouseButton
}

// listener is a component that listens, answering used for what it sees.
func listener(name string, log *unhandledLog, used bool) Node {
	return Component(func() Node {
		UseUnhandledKeyDown(func(e KeyboardEvent) bool {
			log.keys = append(log.keys, name+":"+string(e.Key))
			return used
		})
		UseUnhandledMouseButton(func(b MouseButton) bool {
			log.buttons = append(log.buttons, b)
			log.keys = append(log.keys, name+":"+b.String())
			return used
		})
		return nil
	})
}

func TestUnhandledKeysReachTheWindow(t *testing.T) {
	var log unhandledLog
	root := div(Class("h-full"),
		div(Key("box"), Class("h-10"), TabIndex(0)),
		listener("outer", &log, false),
	)
	x := newKeys(t, root, 200, 100)
	x.tree().focusElement(keyed(x.tree(), "box"))
	x.press("x", platform.CodeKeyX, platform.ModAlt)
	if got := log.keys; len(got) != 1 || got[0] != "outer:x" {
		t.Fatalf("listeners saw %v, want one key", got)
	}
}

func TestUnhandledKeysYieldToHandlers(t *testing.T) {
	for _, tc := range []struct {
		name string
		prop Prop
	}{
		{"prevented", On("keydown", func(e KeyboardEvent) { e.PreventDefault() })},
		{"stopped", On("keydown", func(e KeyboardEvent) { e.StopPropagation() })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log unhandledLog
			root := div(Class("h-full"), div(Key("box"), Class("h-10"), TabIndex(0), tc.prop), listener("w", &log, false))
			x := newKeys(t, root, 200, 100)
			x.tree().focusElement(keyed(x.tree(), "box"))
			x.press("x", platform.CodeKeyX, 0)
			if len(log.keys) != 0 {
				t.Errorf("listeners saw %v after a handler took the key", log.keys)
			}
		})
	}
}

func TestUnhandledKeysAreLeftOutWhenATextFieldTakesThem(t *testing.T) {
	var log unhandledLog
	root := div(Class("h-full"), input(Key("f"), Class("h-8")), listener("w", &log, false))
	x := newKeys(t, root, 200, 100)
	x.tree().focusElement(keyed(x.tree(), "f"))
	x.press("a", platform.CodeKeyA, 0)                        // typed: the field's
	x.press(platform.KeyArrowLeft, platform.CodeArrowLeft, 0) // moves its caret
	if len(log.keys) != 0 {
		t.Errorf("listeners saw %v for keys the field uses", log.keys)
	}
	// Alt+Left is nothing to the field on the Linux flavor.
	x.press(platform.KeyArrowLeft, platform.CodeArrowLeft, platform.ModAlt)
	if len(log.keys) != 1 {
		t.Errorf("listeners saw %v for a key the field leaves", log.keys)
	}
}

func TestUnhandledListenersRunInnermostFirstUntilOneUsesTheInput(t *testing.T) {
	for _, innerUses := range []bool{true, false} {
		var log unhandledLog
		outer := Component(func() Node {
			UseUnhandledKeyDown(func(e KeyboardEvent) bool { log.keys = append(log.keys, "outer"); return true })
			return div(listener("inner", &log, innerUses))
		})
		x := newKeys(t, div(Class("h-full"), outer), 200, 100)
		x.press("x", platform.CodeKeyX, 0)
		want := []string{"inner:x", "outer"}
		if innerUses {
			want = want[:1]
		}
		if len(log.keys) != len(want) || log.keys[0] != want[0] || len(want) > 1 && log.keys[1] != want[1] {
			t.Errorf("inner uses the key %v: listeners ran %v, want %v", innerUses, log.keys, want)
		}
	}
}

func TestUnhandledSideButtons(t *testing.T) {
	var log unhandledLog
	root := div(Class("h-full"), div(Key("box"), Class("h-10")), listener("w", &log, true))
	x := newKeys(t, root, 200, 100)
	press := func(b platform.PointerButton) {
		x.send(move(20, 5), ptr(platform.PointerDown, 20, 5, b, b.Mask()), ptr(platform.PointerUp, 20, 5, b, 0))
	}
	press(platform.ButtonBack)
	press(platform.ButtonForward)
	press(platform.ButtonPrimary)   // not a side button
	press(platform.ButtonSecondary) // neither
	if want := []MouseButton{MouseButtonBack, MouseButtonForward}; len(log.buttons) != 2 || log.buttons[0] != want[0] || log.buttons[1] != want[1] {
		t.Errorf("listeners saw %v, want %v", log.buttons, want)
	}
}

func TestUnhandledSideButtonsYieldToHandlers(t *testing.T) {
	for _, ev := range []string{"pointerdown", "mousedown", "pointerup", "mouseup"} {
		for _, how := range []string{"prevent", "stop"} {
			t.Run(ev+" "+how, func(t *testing.T) {
				var log unhandledLog
				h := func(e MouseEvent) {
					if how == "prevent" {
						e.PreventDefault()
					} else {
						e.StopPropagation()
					}
				}
				var prop Prop
				if ev[:5] == "point" {
					prop = On(ev, func(e PointerEvent) {
						if how == "prevent" {
							e.PreventDefault()
						} else {
							e.StopPropagation()
						}
					})
				} else {
					prop = On(ev, h)
				}
				root := div(Class("h-full"), div(Key("box"), Class("h-10"), prop), listener("w", &log, true))
				x := newKeys(t, root, 200, 100)
				b := platform.ButtonBack
				x.send(move(20, 5), ptr(platform.PointerDown, 20, 5, b, b.Mask()), ptr(platform.PointerUp, 20, 5, b, 0))
				if len(log.buttons) != 0 {
					t.Errorf("listeners saw %v after %s was %sed", log.buttons, ev, how)
				}
				// Below the box nothing handles it.
				x.send(move(20, 60), ptr(platform.PointerDown, 20, 60, b, b.Mask()), ptr(platform.PointerUp, 20, 60, b, 0))
				if len(log.buttons) != 1 {
					t.Errorf("listeners saw %v for an unhandled press", log.buttons)
				}
			})
		}
	}
}

func TestUnhandledListenerIsRemovedWithItsComponent(t *testing.T) {
	var log unhandledLog
	var setShown func(bool)
	root := Component(func() Node {
		shown, set := UseState(true)
		setShown = set.Set
		return div(Class("h-full"), If(shown, listener("w", &log, false)))
	})
	x := newKeys(t, root, 200, 100)
	x.press("x", platform.CodeKeyX, 0)
	setShown(false)
	x.frame()
	x.press("y", platform.CodeKeyY, 0)
	if len(log.keys) != 1 {
		t.Errorf("listeners saw %v: the unmounted one must not run", log.keys)
	}
}
