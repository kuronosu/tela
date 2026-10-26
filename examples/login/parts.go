package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/login/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Logo is the brand mark: a gradient tile with an icon, and the name.
func Logo() tela.Node {
	return Div(Class("flex items-center gap-2.5 text-lg font-semibold tracking-tight"),
		Div(Class("flex size-8 items-center justify-center rounded-lg bg-linear-to-br from-teal-400 to-emerald-600 text-white shadow-sm"),
			Icon(icons.Layers, Class("size-4"))),
		Text("Tela"),
	)
}

// Header is the title of the form.
func Header() tela.Node {
	return Div(Class("flex flex-col gap-1.5"),
		H1(Class("text-2xl font-bold tracking-tight"), Text("Welcome back")),
		P(Class("text-sm text-muted-foreground"), Text("Sign in to your account to continue.")),
	)
}

// Alert is a message above the form. It has the alert role, so a screen
// reader announces it when it appears (kit has no Alert component).
type Alert struct {
	Title, Description string
}

func (a Alert) Build() tela.Node {
	return Div(Role("alert"),
		Class("flex items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive dark:bg-destructive/10"),
		Icon(icons.CircleAlert, Class("mt-0.5 size-4 shrink-0")),
		Div(Class("flex flex-col gap-0.5"),
			Div(Class("font-medium"), Text(a.Title)),
			Div(Class("text-destructive/80"), Text(a.Description)),
		),
	)
}

// EmailField is a Field around an e-mail Input. Its Error, set once the
// user tried to sign in, marks the input invalid (a red border and ring
// from the recipe) and shows the message below it.
type EmailField struct {
	Email string
	Set   tela.Setter[string]
	Error string
}

func (p EmailField) Build() tela.Node {
	email := p.Email
	return Fragment(kit.Field{
		Label: "E-mail",
		Error: p.Error,
		Children: kit.Input{
			Name: "email", Type: "email", Placeholder: "you@example.com", AutoFocus: true,
			Value: &email, OnValueChange: p.Set.Set,
			Props: AutoComplete("email"),
		},
	})
}

// PasswordField has a show/hide button over the end of the field: a ghost
// icon kit.Button with AriaPressed. Type is just a prop, so toggling it
// swaps the dots for the text without remounting the input.
type PasswordField struct {
	Password string
	Set      tela.Setter[string]
	Error    string
	Show     bool
	SetShow  tela.Setter[bool]
}

func (p PasswordField) Build() tela.Node {
	password := p.Password
	typ, icon := "password", icons.Eye
	if p.Show {
		typ, icon = "text", icons.EyeOff
	}
	return Fragment(kit.Field{
		Label: "Password",
		Error: p.Error,
		Children: Div(Class("relative"),
			kit.Input{
				Name: "password", Type: typ, Placeholder: "Your password",
				Value: &password, OnValueChange: p.Set.Set,
				Class: "pr-10", Props: AutoComplete("current-password"),
			},
			kit.Button{
				Type: "button", Variant: kit.Ghost, Size: kit.IconSm, Icon: icon,
				AriaLabel: "Show password", Class: "absolute top-0.5 right-0.5 text-muted-foreground",
				Props:   AriaPressed(p.Show),
				OnClick: tela.Toggle(p.SetShow),
			},
		),
	})
}

// RememberRow has the "Remember me" checkbox, a horizontal Field, and the
// link to reset the password at the other end.
type RememberRow struct {
	On  bool
	Set func(bool)
}

func (p RememberRow) Build() tela.Node {
	on := p.On
	return Div(Class("flex items-center justify-between gap-4"),
		kit.Field{
			Orientation: kit.Horizontal,
			Label:       "Remember me",
			Class:       "w-auto items-center gap-2",
			Children:    kit.Checkbox{Checked: &on, OnCheckedChange: p.Set},
		},
		kit.Button{Type: "button", Variant: kit.Link, Class: "h-auto p-0 font-normal text-muted-foreground hover:text-foreground",
			Children: Text("Forgot password?")},
	)
}

// SubmitButton is the primary action. While the request is in flight it is
// disabled and shows a spinner: an icon with animate-spin.
type SubmitButton struct {
	Loading bool
}

func (p SubmitButton) Build() tela.Node {
	label := Fragment(Text("Sign in"))
	if p.Loading {
		label = Fragment(Icon(icons.LoaderCircle, Class("size-4 animate-spin")), Text("Signing in…"))
	}
	return Fragment(kit.Button{Type: "submit", Disabled: p.Loading, Class: "w-full", Children: label})
}

// Alternatives are the other ways in, under a separator with a caption. The
// buttons are outline kit.Buttons with generic Lucide icons: no brand
// logos.
type Alternatives struct {
	Disabled bool
}

func (p Alternatives) Build() tela.Node {
	option := func(icon tela.Icon, label string) tela.Node {
		return Fragment(kit.Button{Type: "button", Variant: kit.Outline, Icon: icon, Disabled: p.Disabled, Children: Text(label)})
	}
	return Div(Class("flex flex-col gap-5"),
		Div(Class("flex items-center gap-3 text-sm text-muted-foreground"),
			kit.Separator{Class: "flex-1"}, Text("Or continue with"), kit.Separator{Class: "flex-1"}),
		Div(Class("grid grid-cols-3 gap-3"),
			option(icons.KeyRound, "Passkey"),
			option(icons.Mail, "Magic link"),
			option(icons.Building, "SSO"),
		),
	)
}

// SignUp is the footer of the form.
func SignUp() tela.Node {
	return P(Class("text-center text-sm text-muted-foreground"),
		Text("Don't have an account? "),
		kit.Button{Type: "button", Variant: kit.Link, Class: "h-auto p-0 text-foreground", Children: Text("Sign up")})
}

// Welcome replaces the form once the sign-in succeeded.
type Welcome struct {
	Email   string
	SignOut func()
}

func (p Welcome) Build() tela.Node {
	return Div(Role("status"), Class("flex flex-col items-center gap-5 py-6 text-center"),
		Div(Class("flex size-14 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"),
			Icon(icons.CircleCheck, Class("size-7"))),
		Div(Class("flex flex-col gap-1.5"),
			H1(Class("text-2xl font-bold tracking-tight"), Text("You're signed in")),
			P(Class("text-sm text-muted-foreground"), Textf("Welcome back, %s.", p.Email)),
		),
		kit.Button{Variant: kit.Outline, OnClick: p.SignOut, Children: Text("Sign out")},
	)
}

// Panel is the brand side of the card, hidden below md:. It is decoration,
// so it keeps its own colors in both schemes: a gradient with a soft grid,
// two glows, and a glass card with a quote.
func Panel() tela.Node {
	return Div(Class("relative hidden flex-col justify-between overflow-hidden bg-linear-to-br from-teal-500 via-emerald-600 to-cyan-900 p-10 text-white md:flex"),
		Div(Class("absolute inset-0 bg-[linear-gradient(to_right,rgba(255,255,255,0.08)_1px,transparent_1px),linear-gradient(to_bottom,rgba(255,255,255,0.08)_1px,transparent_1px)] bg-[size:32px_32px]")),
		Div(Class("absolute -top-24 -right-24 size-80 rounded-full bg-white/20 blur-3xl")),
		Div(Class("absolute -bottom-32 -left-24 size-96 rounded-full bg-cyan-300/25 blur-3xl")),
		Div(Class("relative flex flex-col items-start gap-5"),
			Div(Class("rounded-full border border-white/25 bg-white/10 px-3 py-1 text-xs font-medium backdrop-blur"),
				Text("Pure Go. One binary.")),
			H2(Class("max-w-xs text-4xl leading-tight font-semibold tracking-tight text-balance"),
				Text("Desktop apps, woven from one codebase.")),
		),
		Div(Class("relative flex flex-col gap-5 rounded-2xl border border-white/25 bg-white/10 p-6 shadow-xl backdrop-blur-md"),
			P(Class("text-lg leading-snug font-medium tracking-tight text-balance"),
				Text("“We replaced three desktop codebases with one Go program, and it looks better than the web app did.”")),
			Div(Class("flex items-center gap-3"),
				kit.Avatar{Size: kit.Lg, Fallback: "PR", Class: "ring-2 ring-white/40", FallbackClass: "bg-white/25 text-sm font-medium text-white"},
				Div(Class("flex flex-col text-sm"),
					Span(Class("font-medium"), Text("Priya Raman")),
					Span(Class("text-white/70"), Text("Staff engineer, Northwind")),
				),
			),
		),
	)
}
