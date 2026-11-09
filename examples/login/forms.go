package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/login/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Setters are the Setters of the page's state, which the two forms share:
// the e-mail and the password stay when the card switches mode.
type Setters struct {
	Email, Password, Name tela.Setter[string]
	Remember, Terms, Show tela.Setter[bool]
	Touched               tela.Setter[bool]
}

// SignInForm is the "Welcome back" form: an e-mail, a password, "Remember
// me", the Sign in button and the other ways in.
type SignInForm struct {
	Values Values
	Set    Setters
	// Show reveals the password.
	Show bool
	// Submitted means the user tried to sign in, so the errors show.
	Submitted bool
	// Loading is the request in flight; Failed a rejected one.
	Loading, Failed bool
	// OnSubmit is Enter in a field and the Sign in button; OnEdit any change
	// of a text field; SwitchMode the link to Create account.
	OnSubmit, OnEdit, SwitchMode func()
}

func (p SignInForm) Build() tela.Node {
	v := p.Values
	return Fragment(
		Header{Title: "Welcome back", Description: "Sign in to your account to continue."},
		If(p.Failed, kit.Alert{
			Variant: kit.Destructive, Icon: icons.CircleAlert,
			Class:       "border-destructive/30 bg-destructive/5 dark:bg-destructive/10",
			Title:       Text("Incorrect e-mail or password"),
			Description: Text("Check your details and try again. The demo password is " + demoPassword + "."),
		}),
		Form(Class("flex flex-col gap-5"),
			// Enter in a field and the Sign in button both land here. The
			// button is disabled while Loading, but a handler is the last line
			// of defense, not the button.
			OnSubmit(p.OnSubmit),
			// OnChange bubbles, so one handler sees every text field: editing
			// after a rejection takes the alert away.
			OnChange(p.OnEdit),

			EmailField{Email: v.Email, Set: p.Set.Email, Error: shown(p.Submitted, v.EmailError()), AutoFocus: true},
			PasswordField{Password: v.Password, Set: p.Set.Password, Error: shown(p.Submitted, v.PasswordError()), Show: p.Show, SetShow: p.Set.Show},
			RememberRow{On: v.Remember, Set: p.Set.Remember.Set},
			SubmitButton{Label: "Sign in", LoadingLabel: "Signing in…", Loading: p.Loading},
		),
		Alternatives{Disabled: p.Loading},
		ModeSwitch{
			Prompt: "Don't have an account? ", Label: "Sign up", AriaLabel: "Sign up for an account",
			OnClick: p.SwitchMode,
		},
	)
}

// CreateForm is the "Create your account" form: a display name, an e-mail, a
// password with a strength meter, a bio and the terms. The name, e-mail and
// password are controlled (the validation and the meter read them); the bio
// is bound to a tela.Value, so typing in it builds nothing.
type CreateForm struct {
	Values Values
	// Bio is the bio's text: a Value, not a state (see BioField).
	Bio  *tela.Value[string]
	Set  Setters
	Show bool
	// Touched means the e-mail field was left once, so its error shows.
	Touched bool
	Loading bool
	// OnSubmit is Enter in a field and the Create account button; SwitchMode
	// the link back to Sign in.
	OnSubmit, SwitchMode func()
}

func (p CreateForm) Build() tela.Node {
	v := p.Values
	emailError := ""
	if p.Touched && v.Email != "" {
		emailError = v.EmailError()
	}
	canCreate := v.CanCreate()
	return Fragment(
		Header{Title: "Create your account"},
		Form(Class("flex flex-col gap-4"),
			OnSubmit(p.OnSubmit),

			NameField{Name: v.Name, Set: p.Set.Name},
			EmailField{Email: v.Email, Set: p.Set.Email, Error: emailError, SetTouched: p.Set.Touched},
			PasswordField{Password: v.Password, Set: p.Set.Password, Show: p.Show, SetShow: p.Set.Show, New: true},
			BioField{Bio: p.Bio},
			TermsField{On: v.Terms, Set: p.Set.Terms.Set},
			Div(Class("flex flex-col gap-2"),
				SubmitButton{Label: "Create account", LoadingLabel: "Creating account…", Loading: p.Loading, Disabled: !canCreate},
				If(!canCreate && !p.Loading, P(Class("text-center text-xs text-muted-foreground"),
					Text("Fill in every field and accept the terms."))),
			),
		),
		ModeSwitch{
			Prompt: "Already have an account? ", Label: "Sign in", AriaLabel: "Sign in to your account",
			OnClick: p.SwitchMode,
		},
	)
}

// shown is the error message of a field once the user tried to send the
// form: nobody is scolded for a form they have not sent yet.
func shown(tried bool, message string) string {
	if tried {
		return message
	}
	return ""
}
