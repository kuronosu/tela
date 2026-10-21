// Command signup is a "Create your account" screen: a brand panel on the
// left and, on the right, a form with every kind of text field Tela has.
//
// A display name (any language), an e-mail with a helper message, a
// password with a show/hide toggle and a strength meter, a bio that grows
// with its text and counts its characters, a terms checkbox and a submit
// button that stays disabled until the form is valid. Submitting it, with
// the button or with Enter in a field, shows a confirmation banner.
//
// The name, e-mail and password are controlled with h.Bind, which is what
// the validation and the strength meter read. The bio uses h.BindValue, so
// typing in it never rebuilds the form: only its counter, an h.TextFn that
// reads the same tela.Value, repaints.
//
// `tela snapshot ./examples/signup` renders it without a window; pick a
// state with --preview ("1 Empty", "2 Filled", "3 Invalid", "4 Created")
// and add --dark for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Props starts the screen in any state. The zero Props is the real app: an
// empty form.
type Props struct {
	Initial Values
	// ShowPassword reveals the password.
	ShowPassword bool
	// Touched means the e-mail field was left once, so its errors show.
	Touched bool
	// Created shows the confirmation banner.
	Created bool
}

// Signup is the screen.
func Signup() tela.Node { return tela.CP(Page, Props{}) }

// Page is the whole screen, with its state.
func Page(p Props) tela.Node {
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

	// Editing anything after a submit takes the banner away. OnChange bubbles,
	// so one handler on the Form sees every field.
	dismiss := func() {
		if created {
			setCreated.Set(false)
		}
	}

	return Div(Class("flex min-h-screen bg-white text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50"),
		Brand(),
		Div(Class("flex flex-1 items-center justify-center px-6 py-8 md:px-10"),
			Form(Class("flex w-full max-w-sm flex-col gap-5"),
				OnSubmit(func(tela.SubmitEvent) {
					// Enter in a field and the submit button both land here; check again
					// because a handler is the last line of defense, not the disabled
					// button.
					values.Bio = bio.Latest()
					if values.Valid() && !created {
						setCreated.Set(true)
					}
				}),
				OnChange(func(tela.ChangeEvent) { dismiss() }),

				Header(),
				If(created, Banner(values.Name, values.Email)),

				NameField(name, setName),
				EmailField(email, setEmail, touched, setTouched),
				PasswordField(password, setPassword, show, setShow),
				BioField(bio),
				TermsField(terms, func() { setTerms.Set(!terms); dismiss() }),

				Div(Class("flex flex-col gap-3"),
					Button(Type("submit"), Disabled(!valid || created),
						Class("flex h-9 w-full items-center justify-center rounded-md bg-indigo-600 text-sm font-medium text-white shadow-xs transition-colors "+
							"hover:bg-indigo-700 active:bg-indigo-800 focus-visible:ring-[3px] focus-visible:ring-indigo-500/40 "+
							"disabled:cursor-not-allowed disabled:opacity-50"),
						Text("Create account")),
					If(!valid && !created, P(Class("text-center text-xs text-zinc-500 dark:text-zinc-400"),
						Text("Fill in your name, e-mail and password, and accept the terms."))),
				),
				P(Class("text-center text-sm text-zinc-500 dark:text-zinc-400"),
					Text("Already have an account? "),
					Button(Type("button"), Class("font-medium text-zinc-950 underline underline-offset-4 hover:text-indigo-600 dark:text-zinc-50 dark:hover:text-indigo-400"),
						Text("Sign in"))),
			),
		),
	)
}

// Previews are the states the README and the goldens show.
var Previews = tela.Previews{
	"1 Empty":  {Root: tela.CP(Page, Props{})},
	"2 Filled": {Root: tela.CP(Page, Props{Initial: filled})},
	"3 Invalid": {Root: tela.CP(Page, Props{Touched: true, ShowPassword: true, Initial: Values{
		Name: "Ana García", Email: "ana@", Password: "hunter2", Bio: "Writes Go by day.",
	}})},
	"4 Created": {Root: tela.CP(Page, Props{Initial: filled, Created: true})},
}

// filled is a form ready to submit.
var filled = Values{
	Name:     "Mei Tanaka",
	Email:    "taro@example.com",
	Password: "Tela-weaves-2026",
	Bio:      "Building small desktop tools in Go. Coffee, ramen and tidy layouts.",
	Terms:    true,
}

func main() {
	tela.Run(tela.App{
		Title:  "Tela — create your account",
		AppID:  "dev.telaui.signup",
		Width:  1100,
		Height: 860,
		Root:   Signup,
	})
}
