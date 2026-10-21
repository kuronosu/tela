package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// fieldShell is what Input and Textarea share: shadcn's field. The ring of
// focus: is the focus indicator; text fields match focus-visible: on every
// focus, so focus: and focus-visible: look alike here.
const fieldShell = "block w-full rounded-md border border-zinc-300 bg-white px-3 text-sm text-zinc-950 shadow-xs outline-none " +
	"transition-colors placeholder:text-zinc-400 selection:bg-indigo-200 " +
	"focus:border-indigo-500 focus:ring-[3px] focus:ring-indigo-500/25 " +
	"dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-50 dark:placeholder:text-zinc-500 dark:selection:bg-indigo-800 " +
	"dark:focus:border-indigo-400 dark:focus:ring-indigo-400/25"

// invalidField paints a field red while its value is invalid (for an e-mail
// field: not an address by HTML's syntax). Without the focus: twins, the
// indigo focus classes above would win on a focused field.
const invalidField = "invalid:border-red-500 invalid:ring-[3px] invalid:ring-red-500/20 " +
	"focus:invalid:border-red-500 focus:invalid:ring-red-500/25 " +
	"dark:invalid:border-red-400 dark:focus:invalid:border-red-400"

// FieldLabel is a Label tied to its field by id. hint is the faded
// remainder, such as "(optional)".
func FieldLabel(id, text, hint string) tela.Node {
	return Label(For(id), Class("block text-sm leading-none font-medium"),
		Text(text),
		If(hint != "", Span(Class("font-normal text-zinc-500 dark:text-zinc-400"), Text(" "+hint))),
	)
}

// Hint is the small gray text under a field.
func Hint(text, extra string) tela.Node {
	return P(Class("text-xs text-zinc-500 dark:text-zinc-400 "+extra), Text(text))
}

// Brand is the left panel: a gradient with a tagline, hidden below md:.
func Brand() tela.Node {
	return Div(Class("relative hidden w-[400px] shrink-0 flex-col justify-between overflow-hidden "+
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
		P(Class("text-sm text-zinc-500 dark:text-zinc-400"),
			Text("Start building in minutes. No credit card needed.")),
	)
}

// Banner confirms the submission. It is a status region, so a screen reader
// announces it.
func Banner(name, email string) tela.Node {
	return Div(Role("status"),
		Class("flex items-start gap-3 rounded-lg border border-emerald-200 bg-emerald-50 p-3.5 text-sm text-emerald-950 shadow-xs "+
			"dark:border-emerald-900 dark:bg-emerald-950/60 dark:text-emerald-50"),
		Div(Class("mt-px flex size-5 shrink-0 items-center justify-center rounded-full bg-emerald-600"),
			Div(Class("-mt-px h-2 w-1 rotate-45 border-r-2 border-b-2 border-white"))),
		Div(Class("flex flex-col gap-0.5"),
			Strong(Class("font-semibold"), Textf("Welcome, %s!", name)),
			Span(Class("text-emerald-900/80 dark:text-emerald-100/70"),
				Textf("We sent a confirmation link to %s.", email)),
		),
	)
}

// NameField is a plain controlled field. Its label says "any language" on
// purpose: Input takes input-method text (Japanese, Chinese, Korean)
// like any browser field.
func NameField(name string, set tela.Setter[string]) tela.Node {
	return Div(Class("flex flex-col gap-2"),
		FieldLabel("name", "Display name", "(any language)"),
		Input(ID("name"), Name("name"), AutoComplete("name"), AutoFocus(true),
			Placeholder("Ana García"),
			Bind(name, set),
			Class("h-9 "+fieldShell)),
	)
}

// EmailField shows two ways to react to an invalid value. The border comes
// from the invalid: variant, which h.Type("email") drives; the message
// comes from peer-invalid:, shown only while the field is invalid. Both
// wait until the user left the field once (touched), so nobody is told off
// for the first letter.
func EmailField(email string, set tela.Setter[string], touched bool, setTouched tela.Setter[bool]) tela.Node {
	field := "peer h-9 " + fieldShell
	hint := ""
	if touched {
		field += " " + invalidField
		hint = "peer-invalid:hidden"
	}
	return Div(Class("flex flex-col gap-2"),
		FieldLabel("email", "E-mail", ""),
		Input(ID("email"), Name("email"), Type("email"), AutoComplete("email"),
			Placeholder("you@example.com"),
			Bind(email, set),
			OnBlur(func(tela.FocusEvent) { setTouched.Set(true) }),
			Class(field)),
		Hint("We only use it to confirm your account.", hint),
		If(touched, P(Class("hidden text-xs font-medium text-red-600 peer-invalid:block dark:text-red-400"),
			Text("Enter a valid e-mail address, like you@example.com."))),
	)
}

// PasswordField has a show/hide toggle: a button over the end of the field
// with AriaPressed, and a strength meter. Type is just a prop, so toggling
// it swaps the dots for the text without remounting the field.
func PasswordField(password string, set tela.Setter[string], show bool, setShow tela.Setter[bool]) tela.Node {
	typ := "password"
	if show {
		typ = "text"
	}
	return Div(Class("flex flex-col gap-2"),
		FieldLabel("password", "Password", ""),
		Div(Class("relative"),
			Input(ID("password"), Name("password"), Type(typ), AutoComplete("new-password"),
				Placeholder("At least 8 characters"),
				Bind(password, set),
				Class("h-9 pr-14 "+fieldShell)),
			Button(Type("button"), AriaPressed(show), AriaLabel("Show password"),
				Class("absolute inset-y-0 right-0 flex items-center rounded-r-md px-3 text-xs font-medium text-zinc-500 "+
					"transition-colors hover:text-zinc-950 focus-visible:text-indigo-600 dark:text-zinc-400 dark:hover:text-zinc-50"),
				OnClick(func(tela.MouseEvent) { setShow.Set(!show) }),
				Text(map[bool]string{false: "Show", true: "Hide"}[show])),
		),
		StrengthMeter(password),
	)
}

// StrengthMeter is four bars and a word, with the rule under them.
func StrengthMeter(password string) tela.Node {
	score := Strength(password)
	words := []string{"", "Weak", "Fair", "Good", "Strong"}
	colors := []string{"", "bg-red-500", "bg-amber-500", "bg-lime-500", "bg-emerald-500"}
	bar := func(i int) tela.Node {
		color := "bg-zinc-200 dark:bg-zinc-800"
		if i <= score {
			color = colors[score]
		}
		return Div(Key(i), Class("h-1 flex-1 rounded-full transition-colors "+color))
	}
	return Div(Class("flex flex-col gap-2"),
		Div(Class("flex items-center gap-3"),
			Div(Class("flex flex-1 gap-1"), bar(1), bar(2), bar(3), bar(4)),
			Span(Class("w-11 text-right text-xs font-medium text-zinc-500 dark:text-zinc-400"), Text(words[score])),
		),
		Hint("Use 8 or more characters with letters, numbers and symbols.", ""),
	)
}

// BioField is a Textarea that grows with its text between a minimum and a
// maximum height (field-sizing-content with min-h and max-h; past the
// maximum it scrolls). It is bound to a tela.Value with BindValue, and its
// counter is a TextFn that reads that Value: a keystroke repaints the
// counter and nothing builds.
func BioField(bio *tela.Value[string]) tela.Node {
	return Div(Class("flex flex-col gap-2"),
		FieldLabel("bio", "Short bio", "(optional)"),
		Textarea(ID("bio"), Name("bio"), Rows(3), MaxLength(bioLimit), SpellCheck(false),
			Placeholder("Tell us what you build…"),
			BindValue(bio),
			Class("field-sizing-content max-h-40 min-h-[4.5rem] py-2 leading-5 "+fieldShell)),
		Div(Class("flex justify-between gap-4"),
			Hint("Shown on your public profile.", ""),
			Span(Class("text-xs text-zinc-500 tabular-nums dark:text-zinc-400"),
				TextFn(func() string { return fmt.Sprintf("%d/%d", units(bio.Get()), bioLimit) })),
		),
	)
}

// termsLabel is the accessible name of the terms checkbox: the text of its
// Label.
const termsLabel = "I agree to the Terms of Service and Privacy Policy."

// TermsField is a checkbox built from a button, since a native checkbox is
// not a Tela element yet. The Label's For makes a click on the text click
// the button and gives the button its accessible name.
func TermsField(on bool, toggle func()) tela.Node {
	box := "border-zinc-300 bg-white hover:border-zinc-400 dark:border-zinc-700 dark:bg-zinc-900"
	if on {
		box = "border-indigo-600 bg-indigo-600 hover:bg-indigo-700 dark:border-indigo-500 dark:bg-indigo-500"
	}
	link := "font-medium text-zinc-950 underline underline-offset-4 dark:text-zinc-50"
	return Div(Class("flex items-start gap-3"),
		Button(ID("terms"), Type("button"), Role("checkbox"), AriaChecked(on),
			OnClick(func(tela.MouseEvent) { toggle() }),
			Class("mt-px flex size-4 shrink-0 items-center justify-center rounded-[4px] border shadow-xs transition-colors "+
				"focus-visible:ring-[3px] focus-visible:ring-indigo-500/40 "+box),
			If(on, Div(Class("-mt-0.5 h-2 w-1 rotate-45 border-r-[1.5px] border-b-[1.5px] border-white"))),
		),
		Label(For("terms"), Class("cursor-pointer text-sm leading-snug text-zinc-600 dark:text-zinc-400"),
			Text("I agree to the "), Span(Class(link), Text("Terms of Service")),
			Text(" and "), Span(Class(link), Text("Privacy Policy")), Text(".")),
	)
}
