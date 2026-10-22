package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Enter in a text field presses the Form's default button even when a
// press hook made it from a Div (UseButton with Type "submit"): it is the
// first element of the Form, in tree order, whose type is submit, as an
// HTML submit button, so a form with several fields submits through it.
// A hook-made Div of type "button" before it is not the default button.
func TestImplicitSubmissionHookDiv(t *testing.T) {
	var ev events
	var setDisabled tela.Setter[bool]
	page := func() tela.Node {
		disabled, set := tela.UseState(false)
		setDisabled = set
		plain := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { ev.add("plain") }})
		send := aria.UseButton(aria.ButtonOptions{Type: "submit", Disabled: disabled, OnPress: func(aria.PressEvent) { ev.add("send") }})
		return Form(OnSubmit(func() { ev.add("submit") }),
			Input(TestID("name"), Class("h-8 w-40")),
			Div(plain.Props, Class("h-8 w-20"), Text("Plain")),
			Input(TestID("email"), Class("h-8 w-40")),
			Div(send.Props, Class("h-8 w-20"), Text("Send")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	for _, field := range []string{"name", "email"} {
		app.Focus(telatest.ByTestID(field))
		app.Press("Enter")
		if got := ev.take(); got != "send submit" {
			t.Errorf("Enter in %s: %q, want the Div's press and the submit", field, got)
		}
	}
	// A disabled default button blocks implicit submission (HTML): the
	// form is not submitted, and no later button is pressed instead.
	setDisabled.Set(true)
	app.Focus(telatest.ByTestID("email"))
	app.Press("Enter")
	if got := ev.take(); got != "" {
		t.Errorf("Enter with the default button disabled: %q, want nothing", got)
	}
}
