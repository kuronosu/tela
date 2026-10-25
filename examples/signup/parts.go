package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/signup/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Brand is the left panel: a gradient with a tagline, hidden below md:. It
// is decoration, so it keeps its own colors in both schemes.
func Brand() tela.Node {
	return Div(Class("relative hidden w-[400px] shrink-0 flex-col justify-between overflow-hidden",
		"bg-linear-to-br from-indigo-600 via-violet-600 to-fuchsia-700 p-10 text-white md:flex"),
		Div(Class("absolute -top-24 -right-24 size-80 rounded-full bg-white/10")),
		Div(Class("absolute -bottom-32 -left-16 size-96 rounded-full bg-white/10")),
		Div(Class("relative flex items-center gap-2.5 text-lg font-semibold tracking-tight"),
			Div(Class("flex size-8 items-center justify-center rounded-lg bg-white/20 text-base font-bold"), Text("T")),
			Text("Tela"),
		),
		Div(Class("relative flex flex-col gap-4"),
			P(Class("text-2xl leading-snug font-medium tracking-tight text-balance"),
				Text("“It is the form I would have written in React, shipped as one static binary.”")),
			P(Class("text-sm text-white/70"), Text("Mei Tanaka, desktop engineer")),
		),
	)
}

// Header is the title of the form.
func Header() tela.Node {
	return Div(Class("flex flex-col gap-1.5 pb-1"),
		H1(Class("text-2xl font-semibold tracking-tight"), Text("Create your account")),
		P(Class("text-sm text-muted-foreground"),
			Text("Start building in minutes. No credit card needed.")),
	)
}

// Banner confirms the submission: a kit.Card (the kit has no Alert) that is
// a status region, so a screen reader announces it.
type Banner struct {
	Name, Email string
}

func (b Banner) Build() tela.Node {
	return Fragment(kit.Card{
		Props: Role("status"),
		Class: "gap-1 py-4",
		Title: Div(Class("flex items-center gap-2"),
			Icon(icons.CircleCheck, Class("size-4 text-emerald-600 dark:text-emerald-400")),
			Textf("Welcome, %s!", b.Name)),
		Description: Textf("We sent a confirmation link to %s.", b.Email),
		HeaderClass: "px-4",
	})
}

// NameField is a plain controlled field. Its description says "any
// language" on purpose: Input takes input-method text (Japanese, Chinese,
// Korean) like any browser field.
type NameField struct {
	Name string
	Set  tela.Setter[string]
}

func (p NameField) Build() tela.Node {
	name := p.Name
	return Fragment(kit.Field{
		Label:       "Display name",
		Description: "Any language works, input methods included.",
		Children: kit.Input{
			Name: "name", Placeholder: "Ana García", AutoFocus: true,
			Value: &name, OnValueChange: p.Set.Set,
			Props: AutoComplete("name"),
		},
	})
}

// EmailField shows two ways to tell the user a value is wrong: the Field's
// Error, which marks the input invalid (a red border and ring from the
// recipe's aria-invalid: classes, and [invalid] for assistive technologies)
// and shows the message below it in place of the help text. It waits until
// the user left the field once (Touched), so nobody is told off for the
// first letter.
type EmailField struct {
	Email      string
	Set        tela.Setter[string]
	Touched    bool
	SetTouched tela.Setter[bool]
}

func (p EmailField) Build() tela.Node {
	email := p.Email
	f := kit.Field{
		Label: "E-mail",
		Children: kit.Input{
			Name: "email", Type: "email", Placeholder: "you@example.com",
			Value: &email, OnValueChange: p.Set.Set,
			Props: OnBlur(func() { p.SetTouched.Set(true) }),
		},
	}
	if p.Touched && email != "" && !ValidEmail(email) {
		f.Error = "Enter a valid e-mail address, like you@example.com."
	} else {
		f.Description = "We only use it to confirm your account."
	}
	return Fragment(f)
}

// PasswordField has a show/hide button over the end of the field, a
// kit.Button with an icon and a tooltip and AriaPressed, and a strength
// meter. Type is just a prop, so toggling it swaps the dots for the text
// without remounting the field.
type PasswordField struct {
	Password string
	Set      tela.Setter[string]
	Show     bool
	SetShow  tela.Setter[bool]
}

func (p PasswordField) Build() tela.Node {
	password := p.Password
	typ, icon, tip := "password", icons.Eye, "Show password"
	if p.Show {
		typ, icon, tip = "text", icons.EyeOff, "Hide password"
	}
	return Fragment(kit.Field{
		Label:       "Password",
		Description: "Use letters, numbers and symbols.",
		Children: Div(Class("flex flex-col gap-3"),
			Div(Class("relative"),
				kit.Input{
					Name: "password", Type: typ, Placeholder: "At least 8 characters",
					Value: &password, OnValueChange: p.Set.Set,
					Class: "pr-10", Props: AutoComplete("new-password"),
				},
				kit.Tooltip{
					Trigger: kit.Button{
						Type: "button", Variant: kit.Ghost, Size: kit.IconSm, Icon: icon,
						AriaLabel: "Show password", Class: "absolute top-0.5 right-0.5 text-muted-foreground",
						Props:   AriaPressed(p.Show),
						OnClick: tela.Toggle(p.SetShow),
					},
					Content: Text(tip),
				},
			),
			StrengthMeter{Password: password},
		),
	})
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
// the counter and nothing builds. The counter sits on the field's last line,
// across from the help text.
type BioField struct {
	Bio *tela.Value[string]
}

func (p BioField) Build() tela.Node {
	bio := p.Bio
	return Fragment(kit.Field{
		Label:            "Short bio (optional)",
		Description:      "Shown on your public profile.",
		Class:            "relative",
		DescriptionClass: "pr-16",
		Children: Fragment(
			kit.Textarea{
				Name: "bio", Rows: 3, MaxLength: bioLimit, Placeholder: "Tell us what you build…",
				Class: "max-h-40 min-h-[4.5rem] leading-5",
				Props: BindValue(bio),
			},
			Span(Class("absolute right-0 bottom-0 text-xs text-muted-foreground tabular-nums"),
				TextFn(func() string { return fmt.Sprintf("%d/%d", units(bio.Get()), bioLimit) })),
		),
	})
}

// termsLabel is the accessible name of the terms checkbox: the text of its
// Label.
const termsLabel = "Accept terms and conditions"

// TermsField is a horizontal Field with a kit.Checkbox: the label and its
// description sit after the box, and clicking either toggles it.
type TermsField struct {
	On  bool
	Set func(bool)
}

func (p TermsField) Build() tela.Node {
	on := p.On
	return Fragment(kit.Field{
		Orientation: kit.Horizontal,
		Label:       termsLabel,
		Description: "You agree to our Terms of Service and Privacy Policy.",
		Children:    kit.Checkbox{Checked: &on, OnCheckedChange: p.Set, Class: "mt-px"},
	})
}
