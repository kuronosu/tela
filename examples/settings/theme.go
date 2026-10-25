package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// themeCard draws one option of the theme RadioGroup: a card with a
// miniature of the theme above the group's own radio mark and the label.
// The group keeps the behavior (arrow keys, Space, the radio role and
// aria-checked); st.Props goes on the one button the user presses.
func themeCard(o kit.RadioItem[string], st kit.RadioState) tela.Node {
	return Button(Key(o.Value), st.Props,
		Class("flex min-w-0 flex-col gap-2.5 rounded-lg border border-border bg-card p-2.5 text-left text-sm font-medium text-card-foreground shadow-xs transition-colors outline-none",
			"hover:bg-accent focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
			"aria-checked:border-primary aria-checked:bg-accent/50 aria-checked:ring-1 aria-checked:ring-primary"),
		themePreview(o.Value),
		Div(Class("flex items-center gap-2 px-0.5"), st.Indicator, Text(o.Label)),
	)
}

// themePreview draws a tiny window: a light one, a dark one, or both halves
// for "system". Its colors are fixed: it is a miniature of each theme, not
// of the app's tokens.
func themePreview(theme string) tela.Node {
	pane := func(bg, bar, card string) tela.Node {
		return Div(Class("flex flex-1 flex-col gap-1.5 p-2", bg),
			Div(Class("h-1.5 w-2/3 rounded-full", bar)),
			Div(Class("flex flex-1 flex-col gap-1 rounded-md p-1.5", card),
				Div(Class("h-1 w-full rounded-full", bar)),
				Div(Class("h-1 w-3/4 rounded-full", bar)),
			),
		)
	}
	light := func() tela.Node { return pane("bg-zinc-100", "bg-zinc-300", "bg-white") }
	dark := func() tela.Node { return pane("bg-zinc-800", "bg-zinc-600", "bg-zinc-900") }
	return Div(Class("flex h-[72px] overflow-hidden rounded-md border border-border"),
		If(theme != "dark", light()),
		If(theme != "light", dark()),
	)
}
