// Command login is a "Welcome back" screen: a card that is a sign-in form
// on one side and a brand panel with a quote on the other, on package kit in
// its default style. Below md: (768 px) the panel hides and the form takes
// the whole card. The link under the form turns the same card into a
// "Create your account" form, and back.
//
// Sign in: an e-mail and a password (with a show/hide button), "Remember
// me", a "Forgot password?" link, a Sign in button and three other ways in
// under "Or continue with". Submitting, with the button or with Enter in a
// field, checks the form: the Field errors show under the controls that need
// attention. A valid form starts a fake request: the button turns into a
// disabled "Signing in…" with a spinner, and 1.5 seconds later the sign-in
// is rejected, with an alert above the form, or accepted, with a welcome.
// The password the fake server knows is "weave-2026".
//
// Create account: a display name, an e-mail, a password with a strength
// meter, a bio that grows with its text and counts its characters, and the
// terms. The button stays disabled until the form is valid; then the same
// fake request ends in a confirmation. The name, e-mail and password are
// controlled (the validation and the meter read them). The bio uses
// h.BindValue, so typing in it never rebuilds the form: only its counter,
// an h.TextFn that reads the same tela.Value, repaints.
//
// The request is tela.UseTimeout: it is on the frame clock, with no
// goroutine, so a test moves it with App.Advance. The state is a handful of
// tela.UseState values; the forms receive the Setters in their fields.
//
// `tela snapshot ./examples/login` renders it without a window; pick a
// state with --preview ("1 Sign in", "2 Invalid", "3 Wrong password",
// "4 Signing in", "5 Signed in", "6 Create account", "7 Create account
// filled", "8 Create account invalid", "9 Account created") and add --dark
// for the dark theme.
package main

import (
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Page is the whole screen, with its state. Its fields seed that state, so
// a preview can start it in any state; the zero Page is the real app: an
// empty Sign in form.
type Page struct {
	Initial Values
	// Mode is the form on the card.
	Mode Mode
	// ShowPassword reveals the password.
	ShowPassword bool
	// Submitted means the user tried to sign in, so the errors show.
	Submitted bool
	// Touched means the e-mail of Create account was left once, so its error
	// shows.
	Touched bool
	// Status is where the request of the mode is.
	Status Status
}

func (p Page) Build() tela.Node {
	email, setEmail := tela.UseState(p.Initial.Email)
	password, setPassword := tela.UseState(p.Initial.Password)
	remember, setRemember := tela.UseState(p.Initial.Remember)
	name, setName := tela.UseState(p.Initial.Name)
	terms, setTerms := tela.UseState(p.Initial.Terms)
	mode, setMode := tela.UseState(p.Mode)
	show, setShow := tela.UseState(p.ShowPassword)
	submitted, setSubmitted := tela.UseState(p.Submitted)
	touched, setTouched := tela.UseState(p.Touched)
	status, setStatus := tela.UseState(p.Status)

	// The bio is a tela.Value, not a state: see the file comment.
	bio := tela.UseValue(p.Initial.Bio)

	values := Values{Email: email, Password: password, Remember: remember, Name: name, Terms: terms}
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
		switch {
		case mode == CreateAccount && values.CanCreate():
			setStatus.Set(Created)
		case mode == CreateAccount:
			setStatus.Set(Idle)
		case values.Accepts():
			setStatus.Set(SignedIn)
		default:
			setStatus.Set(Failed)
		}
	})

	// switchTo changes the form on the card. The e-mail stays; the password
	// and what the user was told do not.
	switchTo := func(m Mode) func() {
		return func() {
			setMode.Set(m)
			setStatus.Set(Idle)
			setPassword.Set("")
			setSubmitted.Set(false)
			setTouched.Set(false)
		}
	}

	set := Setters{
		Email: setEmail, Password: setPassword, Name: setName,
		Remember: setRemember, Terms: setTerms, Show: setShow, Touched: setTouched,
	}

	// Create account is the taller form: a tighter column keeps it in the
	// window. min-w-0 lets the grid shrink the column below the width of a
	// growing Textarea's text (field-sizing-content), which a narrow window
	// needs.
	column := "flex min-w-0 flex-col justify-center gap-5 p-8 md:px-10"
	if mode == CreateAccount {
		column = "flex min-w-0 flex-col justify-center gap-4 px-8 py-5 md:px-10"
	}

	var body tela.Child
	switch {
	case status == SignedIn:
		body = Welcome{Email: email, SignOut: func() {
			setPassword.Set("")
			setSubmitted.Set(false)
			setStatus.Set(Idle)
		}}
	case status == Created:
		body = AccountCreated{Name: name, Email: email, SignIn: switchTo(SignIn)}
	case mode == CreateAccount:
		body = CreateForm{
			Values: values, Bio: bio, Set: set, Show: show, Touched: touched, Loading: loading,
			OnSubmit: func() {
				// Enter in a field and the Create account button both land here;
				// check again because a handler is the last line of defense, not
				// the disabled button.
				if values.CanCreate() && !loading {
					setStatus.Set(Loading)
				}
			},
			SwitchMode: switchTo(SignIn),
		}
	default:
		body = SignInForm{
			Values: values, Set: set, Show: show, Submitted: submitted, Loading: loading, Failed: status == Failed,
			OnSubmit: func() {
				// Enter in a field and the Sign in button both land here.
				setSubmitted.Set(true)
				if values.Valid() && !loading {
					setStatus.Set(Loading)
				}
			},
			OnEdit: func() {
				if status == Failed {
					setStatus.Set(Idle)
				}
			},
			SwitchMode: switchTo(CreateAccount),
		}
	}

	return Div(Class("flex min-h-screen items-center justify-center bg-muted/50 p-6 md:p-8"),
		kit.Card{
			Class:        "w-full max-w-4xl gap-0 overflow-hidden py-0 shadow-xl md:min-h-[640px]",
			ContentClass: "grid flex-1 px-0 md:grid-cols-2",
			Children: Fragment(
				Div(Class(column),
					Logo(),
					body,
				),
				Panel{Mode: mode},
			),
		},
	)
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

	"6 Create account":         {Root: Page{Mode: CreateAccount}},
	"7 Create account filled":  {Root: Page{Mode: CreateAccount, Initial: filled}},
	"8 Create account invalid": {Root: Page{Mode: CreateAccount, Touched: true, ShowPassword: true, Initial: Values{Name: "Ana García", Email: "ana@", Password: "hunter2", Bio: "Writes Go by day."}}},
	"9 Account created":        {Root: Page{Mode: CreateAccount, Status: Created, Initial: filled}},
}

// valid is a sign-in form the fake server accepts.
var valid = Values{Email: "mei@example.com", Password: demoPassword, Remember: true}

// filled is a Create account form ready to submit.
var filled = Values{
	Name:     "Mei Tanaka",
	Email:    "taro@example.com",
	Password: "Tela-weaves-2026",
	Bio:      "Building small desktop tools in Go. Coffee, ramen and tidy layouts.",
	Terms:    true,
}

func main() { tela.Main(App) }
