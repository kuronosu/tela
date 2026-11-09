package main

import (
	"fmt"

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

// Header is the title of the form, with a line under it when it has a
// Description.
type Header struct {
	Title, Description string
}

func (p Header) Build() tela.Node {
	return Div(Class("flex flex-col gap-1.5"),
		H1(Class("text-2xl font-bold tracking-tight"), Text(p.Title)),
		If(p.Description != "", P(Class("text-sm text-muted-foreground"), Text(p.Description))),
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

// EmailField is a Field around an e-mail Input. Its Error marks the input
// invalid (a red border and ring from the recipe, `[invalid]` for assistive
// technologies) and shows the message below it. Sign in sets it once the
// user tried to send the form; Create account, whose button is disabled
// until the form is valid and so cannot be tried, once the field was left
// (SetTouched, from OnBlur).
type EmailField struct {
	Email string
	Set   tela.Setter[string]
	Error string
	// AutoFocus takes the focus when the field mounts.
	AutoFocus bool
	// SetTouched, when set, is called when the field loses the focus.
	SetTouched tela.Setter[bool]
}

func (p EmailField) Build() tela.Node {
	email := p.Email
	in := kit.Input{
		Name: "email", Type: "email", Placeholder: "you@example.com", AutoFocus: p.AutoFocus,
		Value: &email, OnValueChange: p.Set.Set,
		Props: AutoComplete("email"),
	}
	if !p.SetTouched.IsZero() {
		in.Props = OnBlur(func() { p.SetTouched.Set(true) })
	}
	return kit.Field{Label: "E-mail", Error: p.Error, Children: in}
}

// NameField is a plain controlled field. Input takes input-method text
// (Japanese, Chinese, Korean) like any browser field.
type NameField struct {
	Name string
	Set  tela.Setter[string]
}

func (p NameField) Build() tela.Node {
	name := p.Name
	return kit.Field{
		Label: "Display name",
		Children: kit.Input{
			Name: "name", Placeholder: "Ana García", AutoFocus: true,
			Value: &name, OnValueChange: p.Set.Set,
			Props: AutoComplete("name"),
		},
	}
}

// PasswordField has a show/hide button inside the end of the field
// (kit.Input's EndContent): a ghost icon kit.Button with AriaPressed and a
// kit.Tooltip that names what the next press does. Type is just a prop, so
// toggling it swaps the dots for the text without remounting the input. For
// a new account (New) it adds a strength meter under the field.
type PasswordField struct {
	Password string
	Set      tela.Setter[string]
	Error    string
	Show     bool
	SetShow  tela.Setter[bool]
	// New is the password of an account being created: the browser may
	// suggest one, and the meter rates it.
	New bool
}

func (p PasswordField) Build() tela.Node {
	password := p.Password
	typ, icon, tip := "password", icons.Eye, "Show password"
	if p.Show {
		typ, icon, tip = "text", icons.EyeOff, "Hide password"
	}
	placeholder, complete := "Your password", "current-password"
	if p.New {
		placeholder, complete = "At least 8 characters", "new-password"
	}
	field := kit.Input{
		Name: "password", Type: typ, Placeholder: placeholder,
		Value: &password, OnValueChange: p.Set.Set,
		Props: AutoComplete(complete),
		EndContent: kit.Tooltip{
			Trigger: kit.Button{
				Type: "button", Variant: kit.Ghost, Size: kit.IconXs, Icon: icon,
				AriaLabel: "Show password", Class: "text-muted-foreground", IconClass: "size-4",
				Props:   AriaPressed(p.Show),
				OnClick: tela.Toggle(p.SetShow),
			},
			Content: Text(tip),
		},
	}
	if !p.New {
		return kit.Field{Label: "Password", Error: p.Error, Children: field}
	}
	return kit.Field{
		Label: "Password", Error: p.Error,
		Children: Div(Class("flex flex-col gap-2.5"), field, StrengthMeter{Password: password}),
	}
}

// StrengthMeter is a kit.Progress from 0 to 4 and a word, colored by the
// score.
type StrengthMeter struct {
	Password string
}

var (
	strengthWords  = []string{"Empty", "Weak", "Fair", "Good", "Strong"}
	strengthColors = []string{"", "bg-red-500", "bg-amber-500", "bg-lime-500", "bg-emerald-500"}
)

func (m StrengthMeter) Build() tela.Node {
	score := Strength(m.Password)
	word := strengthWords[score]
	if score == 0 {
		word = ""
	}
	return Div(Class("flex items-center gap-3"),
		kit.Progress{
			Value: float64(score), Max: 4, AriaLabel: "Password strength",
			FormatValue:    func(v float64) string { return strengthWords[int(v)] },
			Class:          "h-1.5 flex-1 bg-muted",
			IndicatorClass: strengthColors[score],
		},
		Span(Class("w-11 text-right text-xs font-medium text-muted-foreground"), Text(word)),
	)
}

// BioField is a Textarea that grows with its text between a minimum and a
// maximum height (the recipe's field-sizing-content with min-h and max-h;
// past the maximum it scrolls). It is bound to a tela.Value with BindValue,
// and its counter is a TextFn that reads that Value: a keystroke repaints
// the counter and nothing builds. The counter sits on the label's line, at
// the other end (a Field has no slot beside its label, so it is a Span placed
// with absolute in the relative field).
type BioField struct {
	Bio *tela.Value[string]
}

func (p BioField) Build() tela.Node {
	bio := p.Bio
	return kit.Field{
		Label: "Short bio (optional)",
		Class: "relative",
		Children: Fragment(
			kit.Textarea{
				Name: "bio", Rows: 3, MaxLength: bioLimit, Placeholder: "Tell us what you build…",
				Class: "max-h-40 min-h-16 leading-5",
				Props: BindValue(bio),
			},
			Span(Class("absolute top-0 right-0 text-xs text-muted-foreground tabular-nums"),
				TextFn(func() string { return fmt.Sprintf("%d/%d", units(bio.Get()), bioLimit) })),
		),
	}
}

// termsLabel is the accessible name of the terms checkbox: the text of its
// Label.
const termsLabel = "Accept terms and conditions"

// TermsField is a horizontal Field with a kit.Checkbox: the label sits after
// the box, and clicking it toggles the box.
type TermsField struct {
	On  bool
	Set func(bool)
}

func (p TermsField) Build() tela.Node {
	on := p.On
	return kit.Field{
		Orientation: kit.Horizontal,
		Label:       termsLabel,
		Children:    kit.Checkbox{Checked: &on, OnCheckedChange: p.Set, Class: "mt-px"},
	}
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
	Label, LoadingLabel string
	Loading             bool
	// Disabled keeps it off until the form is valid (Create account).
	Disabled bool
}

func (p SubmitButton) Build() tela.Node {
	label := Fragment(Text(p.Label))
	if p.Loading {
		label = Fragment(Icon(icons.LoaderCircle, Class("size-4 animate-spin")), Text(p.LoadingLabel))
	}
	return kit.Button{Type: "submit", Disabled: p.Loading || p.Disabled, Class: "w-full", Children: label}
}

// Alternatives are the other ways in, under a separator with a caption. The
// buttons are outline kit.Buttons with generic Lucide icons: no brand
// logos.
type Alternatives struct {
	Disabled bool
}

func (p Alternatives) Build() tela.Node {
	option := func(icon tela.Icon, label string) tela.Node {
		return kit.Button{Type: "button", Variant: kit.Outline, Icon: icon, Disabled: p.Disabled, Children: Text(label)}
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

// ModeSwitch is the footer of each form: the link button that switches the
// card to the other mode. Its AriaLabel says where it goes ("Sign up for an
// account") and contains the visible text, so it is clear out of the
// sentence it sits in.
type ModeSwitch struct {
	Prompt, Label, AriaLabel string
	OnClick                  func()
}

func (p ModeSwitch) Build() tela.Node {
	return P(Class("text-center text-sm text-muted-foreground"),
		Text(p.Prompt),
		kit.Button{Type: "button", Variant: kit.Link, Class: "h-auto p-0 text-foreground",
			AriaLabel: p.AriaLabel, OnClick: p.OnClick, Children: Text(p.Label)})
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

// AccountCreated replaces the Create account form once the request
// succeeded; it is a status region, like Welcome.
type AccountCreated struct {
	Name, Email string
	SignIn      func()
}

func (p AccountCreated) Build() tela.Node {
	return Div(Role("status"), Class("flex flex-col items-center gap-5 py-6 text-center"),
		Div(Class("flex size-14 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"),
			Icon(icons.CircleCheck, Class("size-7"))),
		Div(Class("flex flex-col gap-1.5"),
			H1(Class("text-2xl font-bold tracking-tight"), Text("Account created")),
			P(Class("text-sm text-muted-foreground"), Textf("Welcome, %s. We sent a confirmation link to %s.", p.Name, p.Email)),
		),
		kit.Button{Variant: kit.Outline, OnClick: p.SignIn, Children: Text("Continue to sign in")},
	)
}

// Panel is the brand side of the card, hidden below md:. It is decoration,
// so it keeps its own colors in both schemes: a gradient with a soft grid,
// two glows, and a glass card with a quote. Its headline follows the mode.
type Panel struct {
	Mode Mode
}

func (p Panel) Build() tela.Node {
	headline := "Desktop apps, woven from one codebase."
	if p.Mode == CreateAccount {
		headline = "Start weaving in minutes, free."
	}
	return Div(Class("relative hidden flex-col justify-between overflow-hidden bg-linear-to-br from-teal-500 via-emerald-600 to-cyan-900 p-10 text-white md:flex"),
		Div(Class("absolute inset-0 bg-[linear-gradient(to_right,rgba(255,255,255,0.08)_1px,transparent_1px),linear-gradient(to_bottom,rgba(255,255,255,0.08)_1px,transparent_1px)] bg-[size:32px_32px]")),
		Div(Class("absolute -top-24 -right-24 size-80 rounded-full bg-white/20 blur-3xl")),
		Div(Class("absolute -bottom-32 -left-24 size-96 rounded-full bg-cyan-300/25 blur-3xl")),
		Div(Class("relative flex flex-col items-start gap-5"),
			Div(Class("rounded-full border border-white/25 bg-white/10 px-3 py-1 text-xs font-medium backdrop-blur"),
				Text("Pure Go. One binary.")),
			H2(Class("max-w-xs text-4xl leading-tight font-semibold tracking-tight text-balance"),
				Text(headline)),
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
