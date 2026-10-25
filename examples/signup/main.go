// Command signup is a "Create your account" screen: a brand panel on the
// left and, on the right, a form built on package kit in its default style,
// with every kind of text field Tela has.
//
// A display name (any language), an e-mail with a helper message and an
// error, a password with a show/hide button and a strength meter, a bio that
// grows with its text and counts its characters, a terms checkbox and a
// submit button that stays disabled until the form is valid. Submitting it,
// with the button or with Enter in a field, shows a confirmation card.
//
// Each field is a kit.Field around a kit.Input or kit.Textarea, so the label
// names the field and the error or the help text describes it. The name,
// e-mail and password are controlled (the validation and the strength meter
// read them). The bio uses h.BindValue, so typing in it never rebuilds the
// form: only its counter, an h.TextFn that reads the same tela.Value,
// repaints.
//
// `tela snapshot ./examples/signup` renders it without a window; pick a
// state with --preview ("1 Empty", "2 Filled", "3 Invalid", "4 Created")
// and add --dark for the dark theme.
package main

import (
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
	// Touched means the e-mail field was left once, so its errors show.
	Touched bool
	// Created shows the confirmation card.
	Created bool
}

func (p Page) Build() tela.Node {
	name, setName := tela.UseState(p.Initial.Name)
	email, setEmail := tela.UseState(p.Initial.Email)
	password, setPassword := tela.UseState(p.Initial.Password)
	terms, setTerms := tela.UseState(p.Initial.Terms)
	show, setShow := tela.UseState(p.ShowPassword)
	touched, setTouched := tela.UseState(p.Touched)
	created, setCreated := tela.UseState(p.Created)

	// The bio is a tela.Value, not a state: see the file comment.
	bio := tela.UseValue(p.Initial.Bio)

	values := Values{Name: name, Email: email, Password: password, Terms: terms}
	valid := values.Valid()

	// Editing anything after a submit takes the confirmation away.
	// OnChange bubbles, so one handler on the Form sees every field; the
	// terms checkbox is not a text field, so it calls dismiss itself.
	dismiss := func() {
		if created {
			setCreated.Set(false)
		}
	}

	return Div(Class("flex min-h-screen"),
		Brand(),
		Div(Class("flex flex-1 items-center justify-center px-6 py-8 md:px-10"),
			Form(Class("flex w-full max-w-sm flex-col gap-5"),
				OnSubmit(func() {
					// Enter in a field and the submit button both land here; check again
					// because a handler is the last line of defense, not the disabled
					// button.
					values.Bio = bio.Latest()
					if values.Valid() && !created {
						setCreated.Set(true)
					}
				}),
				OnChange(dismiss),

				Header(),
				If(created, Banner{Name: values.Name, Email: values.Email}),

				NameField{Name: name, Set: setName},
				EmailField{Email: email, Set: setEmail, Touched: touched, SetTouched: setTouched},
				PasswordField{Password: password, Set: setPassword, Show: show, SetShow: setShow},
				BioField{Bio: bio},
				TermsField{On: terms, Set: func(on bool) { setTerms.Set(on); dismiss() }},

				Div(Class("flex flex-col gap-3"),
					kit.Button{Type: "submit", Disabled: !valid || created, Class: "w-full", Children: Text("Create account")},
					If(!valid && !created, P(Class("text-center text-xs text-muted-foreground"),
						Text("Fill in your name, e-mail and password, and accept the terms."))),
				),
				P(Class("text-center text-sm text-muted-foreground"),
					Text("Already have an account? "),
					kit.Button{Type: "button", Variant: kit.Link, Class: "h-auto p-0 text-foreground", Children: Text("Sign in")}),
			),
		),
	)
}

var theme = signupTheme()

// signupTheme is kit's theme with the brand's indigo as the primary color
// (buttons, the checkbox, selections) and as the focus ring.
func signupTheme() tela.Theme {
	t := kit.Theme()
	t.Colors["primary"] = tela.SchemeColor{Light: tela.Hex("#4f46e5"), Dark: tela.Hex("#6366f1")}
	t.Colors["primary-foreground"] = tela.SchemeColor{Light: tela.Hex("#ffffff"), Dark: tela.Hex("#ffffff")}
	t.Colors["ring"] = tela.SchemeColor{Light: tela.Hex("#6366f1"), Dark: tela.Hex("#818cf8")}
	return t
}

// App is the window. `tela snapshot` applies its theme and root class to
// the Previews below, and the tests mount the same.
var App = tela.App{
	Title:     "Tela — create your account",
	AppID:     "dev.telaui.signup",
	Width:     1100,
	Height:    860,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Page{},
}

// Previews are the states the README and the goldens show.
var Previews = tela.Previews{
	"1 Empty":  {Root: Page{}},
	"2 Filled": {Root: Page{Initial: filled}},
	"3 Invalid": {Root: Page{Touched: true, ShowPassword: true, Initial: Values{
		Name: "Ana García", Email: "ana@", Password: "hunter2", Bio: "Writes Go by day.",
	}}},
	"4 Created": {Root: Page{Initial: filled, Created: true}},
}

// filled is a form ready to submit.
var filled = Values{
	Name:     "Mei Tanaka",
	Email:    "taro@example.com",
	Password: "Tela-weaves-2026",
	Bio:      "Building small desktop tools in Go. Coffee, ramen and tidy layouts.",
	Terms:    true,
}

func main() { tela.Main(App) }
