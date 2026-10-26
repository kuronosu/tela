// Command login is a "Welcome back" screen: a card that is a sign-in form
// on one side and a brand panel with a quote on the other, on package kit in
// its default style. Below md: (768 px) the panel hides and the form takes
// the whole card.
//
// An e-mail and a password (with a show/hide button), "Remember me", a
// "Forgot password?" link, a Sign in button and three other ways in under
// "Or continue with". Submitting, with the button or with Enter in a field,
// checks the form: the Field errors show under the controls that need
// attention. A valid form starts a fake request: the button turns into a
// disabled "Signing in…" with a spinner, and 1.5 seconds later the sign-in
// is rejected, with an alert above the form, or accepted, with a welcome.
// The password the fake server knows is "weave-2026".
//
// The request is tela.UseTimeout: it is on the frame clock, with no
// goroutine, so a test moves it with App.Advance. The state is a handful of
// tela.UseState values; the field components receive the Setters in their
// fields.
//
// `tela snapshot ./examples/login` renders it without a window; pick a
// state with --preview ("1 Sign in", "2 Invalid", "3 Wrong password",
// "4 Signing in", "5 Signed in") and add --dark for the dark theme.
package main

import (
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Page is the whole screen, with its state. Its fields seed that state, so
// a preview can start it in any state; the zero Page is the real app: an
// empty form.
type Page struct {
	Initial Values
	// ShowPassword reveals the password.
	ShowPassword bool
	// Submitted means the user tried to sign in, so the errors show.
	Submitted bool
	// Status is where the sign-in is.
	Status Status
}

func (p Page) Build() tela.Node {
	email, setEmail := tela.UseState(p.Initial.Email)
	password, setPassword := tela.UseState(p.Initial.Password)
	remember, setRemember := tela.UseState(p.Initial.Remember)
	show, setShow := tela.UseState(p.ShowPassword)
	submitted, setSubmitted := tela.UseState(p.Submitted)
	status, setStatus := tela.UseState(p.Status)

	values := Values{Email: email, Password: password, Remember: remember}
	loading := status == Loading

	// The fake request: while Loading, answer after signInDelay. A delay of
	// 0 cancels the timeout, and a change of delay restarts it, so each
	// attempt gets its own; the function is the latest build's, so it judges
	// what is typed at that moment.
	delay := time.Duration(0)
	if loading {
		delay = signInDelay
	}
	tela.UseTimeout(delay, func() {
		if values.Accepts() {
			setStatus.Set(SignedIn)
		} else {
			setStatus.Set(Failed)
		}
	})

	return Div(Class("flex min-h-screen items-center justify-center bg-muted/50 p-6 md:p-8"),
		kit.Card{
			Class:        "w-full max-w-4xl gap-0 overflow-hidden py-0 shadow-xl md:min-h-[640px]",
			ContentClass: "grid flex-1 px-0 md:grid-cols-2",
			Children: Fragment(
				Div(Class("flex flex-col justify-center gap-5 p-8 md:px-10"),
					Logo(),
					If(status == SignedIn, Welcome{Email: email, SignOut: func() {
						setPassword.Set("")
						setSubmitted.Set(false)
						setStatus.Set(Idle)
					}}),
					If(status != SignedIn, Fragment(
						Header(),
						If(status == Failed, Alert{
							Title:       "Incorrect e-mail or password",
							Description: "Check your details and try again. The demo password is " + demoPassword + ".",
						}),
						Form(Class("flex flex-col gap-5"),
							OnSubmit(func() {
								// Enter in a field and the Sign in button both land here.
								// The button is disabled while Loading, but a handler is
								// the last line of defense, not the button.
								setSubmitted.Set(true)
								if values.Valid() && !loading {
									setStatus.Set(Loading)
								}
							}),
							// OnChange bubbles, so one handler sees every text field:
							// editing after a rejection takes the alert away.
							OnChange(func() {
								if status == Failed {
									setStatus.Set(Idle)
								}
							}),

							EmailField{Email: email, Set: setEmail, Error: shown(submitted, values.EmailError())},
							PasswordField{Password: password, Set: setPassword, Error: shown(submitted, values.PasswordError()), Show: show, SetShow: setShow},
							RememberRow{On: remember, Set: setRemember.Set},
							SubmitButton{Loading: loading},
						),
						Alternatives{Disabled: loading},
						SignUp(),
					)),
				),
				Panel(),
			),
		},
	)
}

// shown is the error message of a field once the user tried to sign in:
// nobody is scolded for a form they have not sent yet.
func shown(submitted bool, message string) string {
	if submitted {
		return message
	}
	return ""
}

var theme = kit.Theme()

// App is the window. `tela snapshot` applies its theme and root class to
// the Previews below, and the tests mount the same.
var App = tela.App{
	Title:     "Tela — sign in",
	AppID:     "dev.telaui.login",
	Width:     1000,
	Height:    760,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Page{},
}

// Previews are the states the README and the goldens show.
var Previews = tela.Previews{
	"1 Sign in":        {Root: Page{}},
	"2 Invalid":        {Root: Page{Submitted: true, Initial: Values{Email: "mei@"}}},
	"3 Wrong password": {Root: Page{Status: Failed, Initial: Values{Email: "mei@example.com", Password: "hunter2"}}},
	"4 Signing in":     {Root: Page{Status: Loading, Initial: valid}},
	"5 Signed in":      {Root: Page{Status: SignedIn, Initial: valid}},
}

// valid is a form the fake server accepts.
var valid = Values{Email: "mei@example.com", Password: demoPassword, Remember: true}

func main() { tela.Main(App) }
