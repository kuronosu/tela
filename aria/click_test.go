package aria_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// An uncontrolled toggle tells OnChange about every change, and the press
// callbacks of the options run.
func TestToggleUncontrolledNotifiesAndReportsPresses(t *testing.T) {
	var ev events
	page := checkboxPage(func() aria.ToggleOptions {
		return aria.ToggleOptions{
			DefaultSelected: true,
			OnChange:        func(on bool) { ev.add("change:" + map[bool]string{true: "on", false: "off"}[on]) },
			OnPress:         func(e aria.PressEvent) { ev.add("press:" + string(e.PointerType)) },
			OnPressStart:    func(aria.PressEvent) { ev.add("start") },
			OnPressUp:       func(aria.PressEvent) { ev.add("up") },
			OnPressEnd:      func(aria.PressEvent) { ev.add("end") },
			OnPressChange:   func(p bool) { ev.add(map[bool]string{true: "pressed", false: "released"}[p]) },
		}
	}, aria.UseCheckbox)
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	box := telatest.ByRole("checkbox", "Notifications")
	app.Expect(box, telatest.Checked(true))
	app.Click(box)
	app.Expect(box, telatest.Checked(false))
	if got := ev.take(); got != "start pressed up end released press:mouse change:off" {
		t.Errorf("events %q", got)
	}
}

// Selected nil is uncontrolled, whatever the other options; pointing to a
// value controls it, and the control follows the caller.
func TestToggleSelectedPointer(t *testing.T) {
	var on bool
	var asked []bool
	var setOn tela.Setter[bool]
	page := func() tela.Node {
		v, set := tela.UseState(true)
		on, setOn = v, set
		return useCheckboxPage(func() aria.ToggleOptions {
			return aria.ToggleOptions{Selected: &v, OnChange: func(next bool) { asked = append(asked, next) }}
		}, aria.UseSwitch)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	sw := telatest.ByRole("switch", "Notifications")
	app.Expect(sw, telatest.Checked(true))
	app.Click(sw) // OnChange is asked, the caller stores nothing
	app.Expect(sw, telatest.Checked(true))
	setOn.Set(false)
	app.Expect(sw, telatest.Checked(false))
	if !on && (len(asked) != 1 || asked[0]) {
		t.Errorf("OnChange got %v, want [false]", asked)
	}
}

// formButtonsPage has a form with a field and two buttons made with
// UseButton on Divs, a submit and a reset.
func formButtonsPage(ev *events) func() tela.Node {
	return func() tela.Node {
		send := aria.UseButton(aria.ButtonOptions{Type: "submit", OnPress: func(aria.PressEvent) { ev.add("press") }})
		clear := aria.UseButton(aria.ButtonOptions{Type: "reset"})
		plain := aria.UseButton(aria.ButtonOptions{Type: "button"})
		return Form(OnSubmit(func(e tela.SubmitEvent) { ev.add("submit") }),
			Input(TestID("field"), DefaultValue("draft"), Class("h-8 w-40")),
			Div(send.Props, TestID("send"), Class("h-8 w-20"), Text("Send")),
			Div(clear.Props, TestID("clear"), Class("h-8 w-20"), Text("Clear")),
			Div(plain.Props, Class("h-8 w-20"), Text("Plain")))
	}
}

// A button made by UseButton with Type submits or resets its Form when it
// is pressed, with the mouse or the keyboard, whatever the element is.
func TestUseButtonType(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, formButtonsPage(&ev), telatest.Size(300, 200))
	app.Click(telatest.ByRole("button", "Send"))
	if got := ev.take(); got != "press submit" {
		t.Errorf("click: %q", got)
	}
	app.Press("Enter") // the focus is on the button
	if got := ev.take(); got != "press submit" {
		t.Errorf("Enter: %q", got)
	}
	app.Click(telatest.ByRole("button", "Plain"))
	if got := ev.take(); got != "" {
		t.Errorf("a plain button: %q", got)
	}
	field := telatest.ByTestID("field")
	app.Fill(field, "changed")
	app.ExpectValue(field, "changed")
	app.Click(telatest.ByRole("button", "Clear"))
	app.ExpectValue(field, "draft")
	if got := ev.take(); got != "" {
		t.Errorf("reset: %q", got)
	}
}

// Enter in a text field presses the Form's default button: the first Button
// of type submit. A Button that a press hook drives (a switch, a select's
// trigger, a menu button, a plain UseButton) is type "button" until its
// hook says "submit", so it is not that default: Enter used to open the
// select and flip the switch that came first in the form instead of
// submitting it. An explicit Type still wins.
func TestImplicitSubmissionSkipsHookButtons(t *testing.T) {
	var ev events
	page := func() tela.Node {
		sw := aria.UseSwitch(aria.ToggleOptions{OnChange: func(bool) { ev.add("switch") }})
		sel := aria.UseSelect(aria.SelectOptions[string]{Keys: []string{"a", "b"}, OnOpenChange: func(bool) { ev.add("select") }})
		plain := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { ev.add("plain") }})
		send := aria.UseButton(aria.ButtonOptions{Type: "submit", OnPress: func(aria.PressEvent) { ev.add("send") }})
		return Form(OnSubmit(func() { ev.add("submit") }),
			Button(sw.Props, AriaLabel("Notify"), Class("h-6 w-11")),
			Button(sel.TriggerProps, Class("h-8 w-24"), Span(sel.ValueProps, Text("Pick"))),
			Button(plain.Props, Class("h-8 w-20"), Text("Plain")),
			Input(TestID("field"), Class("h-8 w-40")),
			Button(send.Props, Class("h-8 w-20"), Text("Send")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Focus(telatest.ByTestID("field"))
	app.Press("Enter")
	if got := ev.take(); got != "send submit" {
		t.Errorf("Enter in the field: %q, want only the submit button's press and the submit", got)
	}
}

// A hook-driven Button with an explicit Type "submit" is the default button
// again, before or after the hook's props.
func TestImplicitSubmissionExplicitType(t *testing.T) {
	var ev events
	for name, props := range map[string]func(p tela.Prop) []tela.Child{
		"before": func(p tela.Prop) []tela.Child { return []tela.Child{Type("submit"), p} },
		"after":  func(p tela.Prop) []tela.Child { return []tela.Child{p, Type("submit")} },
	} {
		t.Run(name, func(t *testing.T) {
			page := func() tela.Node {
				send := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { ev.add("press") }})
				return Form(OnSubmit(func() { ev.add("submit") }),
					Input(TestID("field"), Class("h-8 w-40")),
					Button(append(props(send.Props), Class("h-8 w-20"))...))
			}
			app := telatest.MountFunc(t, page, telatest.Size(300, 100))
			app.Focus(telatest.ByTestID("field"))
			app.Press("Enter")
			if got := ev.take(); got != "press" {
				t.Errorf("%s: %q, want the button's press (the hook did not ask to submit)", name, got)
			}
		})
	}
}

func TestUseButtonTypePanics(t *testing.T) {
	msg := recoverMount(t, func() tela.Node {
		aria.UseButton(aria.ButtonOptions{Type: "sumbit"})
		return Div()
	})
	if !strings.Contains(msg, `did you mean "submit"`) {
		t.Errorf("panic message %q", msg)
	}
}

// ElementRef.Click is element.click(): a handler or an effect clicks a
// button, a hook-made button submits through it, a Label forwards to its
// control; a disabled or unmounted element ignores it.
func TestElementRefClick(t *testing.T) {
	var ev events
	page := func() tela.Node {
		send := aria.UseButton(aria.ButtonOptions{Type: "submit"})
		box := aria.UseCheckbox(aria.ToggleOptions{})
		labelRef, plainRef, disabledRef, goneRef := tela.UseElementRef(), tela.UseElementRef(), tela.UseElementRef(), tela.UseElementRef()
		shown, setShown := tela.UseState(true)
		tela.UseMount(func() func() { plainRef.Click(); return nil }) // from an effect
		return Div(
			Form(OnSubmit(func() { ev.add("submit") }),
				Div(send.Props, Class("h-8 w-20"), Text("Send"))),
			Button(Ref(plainRef), OnClick(func(e tela.MouseEvent) {
				ev.add("click:" + map[bool]string{true: "centered", false: "origin"}[e.X > 0])
			}),
				Class("h-8 w-20"), Text("Plain")),
			Button(Ref(disabledRef), Disabled(true), OnClick(func() { ev.add("disabled") }), Class("h-8 w-20"), Text("Off")),
			If(shown, Button(Ref(goneRef), OnClick(func() { ev.add("gone") }), Text("Gone"))),
			Button(box.Props, Class("h-4 w-4 checked:w-8")),
			Label(Ref(labelRef), For(box.ID), Text("Agree")),
			Button(TestID("go"), Class("h-8 w-20"), OnClick(func() {
				send.Ref.Click()
				labelRef.Click()
				disabledRef.Click()
				setShown.Set(false)
			}), Text("Go")),
			Button(TestID("late"), Class("h-8 w-20"), OnClick(func() { goneRef.Click() }), Text("Late")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	if got := ev.take(); got != "click:centered" {
		t.Errorf("Click from an effect: %q", got)
	}
	app.Click(telatest.ByTestID("go"))
	if got := ev.take(); got != "submit" {
		t.Errorf("Click from a handler: %q (a disabled button must not click)", got)
	}
	app.Expect(telatest.ByRole("checkbox", "Agree"), telatest.Checked(true))
	app.Click(telatest.ByTestID("late"))
	if got := ev.take(); got != "" {
		t.Errorf("clicking an unmounted element: %q", got)
	}
}

// Two different refs on one element are a mistake in dev mode, and the
// message says how to share one; outside dev mode the last one wins.
func TestTwoRefsPanicInDev(t *testing.T) {
	page := func() tela.Node {
		btn := aria.UseButton(aria.ButtonOptions{})
		ring := aria.UseFocusRing(aria.FocusRingOptions{}) // its own ref
		return Button(btn.Props, ring.Props, Text("Go"))
	}
	msg := recoverMount(t, page)
	t.Log(msg)
	for _, want := range []string{"two different refs", "aria.PressOptions{Ref: ref}", "tela.UseElementRef", "click_test.go"} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic message lacks %q:\n%s", want, msg)
		}
	}
	// A hand-written h.Ref next to a hook's props is the same mistake.
	msg = recoverMount(t, func() tela.Node {
		btn := aria.UseButton(aria.ButtonOptions{})
		other := tela.UseElementRef()
		return Button(btn.Props, Ref(other), Text("Go"))
	})
	if !strings.Contains(msg, "two different refs") {
		t.Errorf("h.Ref next to a hook: panic message %q", msg)
	}
	// The same ref twice is fine, outside dev mode the last wins.
	telatest.MountFunc(t, func() tela.Node {
		ref := tela.UseElementRef()
		btn := aria.UseButton(aria.ButtonOptions{Ref: ref})
		ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
		return Button(btn.Props, ring.Props, Ref(ref), Text("Go"))
	}, telatest.Size(200, 100))
	telatest.MountFunc(t, page, telatest.Size(200, 100), telatest.NoDev())
}
