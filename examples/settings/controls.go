package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// SwitchProps are the props of Switch. Set is the form's Setter: it is
// comparable, so a Switch skips its build unless its own On or Locked
// changed. Label names it for assistive technologies and tests.
type SwitchProps struct {
	Field  Field
	Label  string
	On     bool
	Locked bool
	Set    tela.Setter[Prefs]
}

// Switch is shadcn's Switch built from divs: a track and a knob that sits
// at one end (justify-start / justify-end). It is a button, so it shows the
// pointer cursor, reacts with hover: and active:, and a Locked switch is
// Disabled and picks up the disabled: classes. Its role is switch, checked
// when on, named by its row's title.
func Switch(p SwitchProps) tela.Node {
	track := "justify-start bg-zinc-200 hover:bg-zinc-300 active:bg-zinc-400 dark:bg-zinc-700 dark:hover:bg-zinc-600 dark:active:bg-zinc-500"
	knob := "bg-white dark:bg-zinc-100"
	if p.On {
		track = "justify-end bg-zinc-900 hover:bg-zinc-700 active:bg-zinc-600 dark:bg-zinc-50 dark:hover:bg-zinc-200 dark:active:bg-zinc-300"
		knob = "bg-white dark:bg-zinc-900"
	}
	return Button(Class("flex h-[22px] w-10 shrink-0 items-center rounded-full p-0.5 shadow-xs transition-colors "+
		"disabled:cursor-not-allowed disabled:opacity-50 "+track),
		Role("switch"), AriaChecked(p.On), AriaLabel(p.Label), Disabled(p.Locked),
		OnClick(func(tela.MouseEvent) { p.Set.Update(func(x Prefs) Prefs { return x.Toggle(p.Field) }) }),
		Div(Class("size-[18px] rounded-full shadow-sm "+knob)),
	)
}

// RadioProps are the props of the radio-like option cards.
type RadioProps struct {
	Field    Field
	Value    string
	Selected bool
	Set      tela.Setter[Prefs]
}

// selectable is the frame of an option card: a bordered button whose ring
// shows the selection. The cards have the radio role, checked when
// selected, and are named by their content.
func selectable(p RadioProps) string {
	if p.Selected {
		return "border-zinc-900 bg-zinc-50 ring-1 ring-zinc-900 dark:border-zinc-50 dark:bg-zinc-800/60 dark:ring-zinc-50"
	}
	return "border-zinc-200 bg-white hover:border-zinc-300 hover:bg-zinc-50 active:bg-zinc-100 " +
		"dark:border-zinc-800 dark:bg-zinc-900 dark:hover:border-zinc-700 dark:hover:bg-zinc-800/60"
}

func choose(p RadioProps) tela.Prop {
	return OnClick(func(tela.MouseEvent) { p.Set.Update(func(x Prefs) Prefs { return x.Choose(p.Field, p.Value) }) })
}

// Radio is the round indicator of a radio card.
func Radio(selected bool) tela.Node {
	if selected {
		return Div(Class("flex size-4 shrink-0 items-center justify-center rounded-full border border-zinc-900 dark:border-zinc-50"),
			Div(Class("size-2 rounded-full bg-zinc-900 dark:bg-zinc-50")))
	}
	return Div(Class("size-4 shrink-0 rounded-full border border-zinc-300 dark:border-zinc-600"))
}

// ThemeOption is a card with a miniature of the theme above its label.
func ThemeOption(p RadioProps) tela.Node {
	var o Option
	for _, t := range themes {
		if t.Value == p.Value {
			o = t
		}
	}
	return Button(Class("flex min-w-0 flex-1 flex-col gap-2.5 rounded-lg border p-2.5 text-left transition-colors "+selectable(p)),
		choose(p), Role("radio"), AriaChecked(p.Selected),
		themePreview(p.Value),
		Div(Class("flex items-center gap-2 px-0.5 text-sm font-medium"),
			Radio(p.Selected),
			Text(o.Title),
		),
	)
}

// themePreview draws a tiny window: a light one, a dark one, or both halves
// for "system".
func themePreview(theme string) tela.Node {
	pane := func(bg, bar, card string) tela.Node {
		return Div(Class("flex flex-1 flex-col gap-1.5 p-2 "+bg),
			Div(Class("h-1.5 w-2/3 rounded-full "+bar)),
			Div(Class("flex flex-1 flex-col gap-1 rounded-md p-1.5 "+card),
				Div(Class("h-1 w-full rounded-full "+bar)),
				Div(Class("h-1 w-3/4 rounded-full "+bar)),
			),
		)
	}
	light := func() tela.Node { return pane("bg-zinc-100", "bg-zinc-300", "bg-white") }
	dark := func() tela.Node { return pane("bg-zinc-800", "bg-zinc-600", "bg-zinc-900") }
	return Div(Class("flex h-[72px] overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-700"),
		If(theme != "dark", light()),
		If(theme != "light", dark()),
	)
}

// DensityOption is a full-width radio row with a title and a hint.
func DensityOption(p RadioProps) tela.Node {
	var o Option
	for _, d := range densities {
		if d.Value == p.Value {
			o = d
		}
	}
	return Button(Class("flex items-start gap-3 rounded-lg border p-3 text-left transition-colors "+selectable(p)),
		choose(p), Role("radio"), AriaChecked(p.Selected),
		Div(Class("mt-0.5"), Radio(p.Selected)),
		Div(Class("flex flex-col gap-0.5"),
			Span(Class("text-sm leading-5 font-medium"), Text(o.Title)),
			Span(Class("text-sm leading-5 text-zinc-500 dark:text-zinc-400"), Text(o.Hint)),
		),
	)
}
