package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/aria/icons"
	. "github.com/kuronosu/tela/h"
)

// Notify is the card with the list box of people to notify. UseListBox holds
// a multiple selection: a click or Space toggles a person, Shift+click and
// Shift+arrows select a range, Ctrl+A selects all, Escape clears, and the
// arrow keys, Home, End and typeahead move the focus. The selection is
// controlled here to show its count.
type Notify struct {
	// Initial are the people selected when the card appears.
	Initial []string
}

func (n Notify) Build() tela.Node {
	sel, setSel := tela.UseState(aria.SelectionOf(n.Initial...))
	lb := aria.UseListBox(aria.ListBoxOptions[string]{
		Keys: memberNames, TextValue: func(name string) string { return name },
		SelectionMode: "multiple", AriaLabel: "People to notify",
		Selected: sel, OnSelectionChange: setSel.Set,
	})

	return Div(Class(card, "flex w-80 shrink-0 flex-col"),
		Div(Class("flex flex-col gap-1 px-4 pt-4 pb-3"),
			H3(Class("text-sm leading-none font-semibold"), Text("Release notes")),
			P(Class("text-sm text-muted-foreground"), Text("Choose who is notified when a deployment finishes."))),
		Div(lb.Props, Class("flex flex-col gap-1 px-2 pb-2 outline-none"),
			Range(len(Members), func(i int) tela.Node {
				m := Members[i]
				return Div(lb.ItemProps(m.Name), Key(m.Name),
					Class("flex cursor-default items-center gap-3 rounded-lg border border-transparent px-2 py-2 text-sm transition-colors",
						"hover:bg-accent/60 aria-selected:border-border aria-selected:bg-accent", focusRing),
					avatar(m.Name, "size-8 text-xs"),
					Div(Class("flex min-w-0 flex-1 flex-col"),
						Span(Class("truncate font-medium"), Text(m.Name)),
						Span(Class("truncate text-xs text-muted-foreground"), Text(m.Role))),
					Span(Class("flex size-5 items-center justify-center rounded-full border border-input text-primary-foreground transition-colors"),
						If(lb.IsSelected(m.Name), Class("border-primary bg-primary")),
						If(lb.IsSelected(m.Name), ic(icons.Check, "size-3.5"))),
				)
			})),
		Div(Class("flex items-center justify-between border-t border-border px-4 py-3 text-xs text-muted-foreground"),
			Textf("%d of %d selected", len(sel.In(memberNames)), len(Members)),
			Span(Text("Space toggles, Ctrl+A selects all"))),
	)
}
