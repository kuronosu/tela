package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/aria/icons"
	. "github.com/kuronosu/tela/h"
)

// ProjectSearch is the combobox that picks the project the page shows:
// UseCombobox holds the text, filters the thirty projects with aria.Contains
// (case and accents ignored), opens the list while typing and keeps the focus
// on the input while the arrow keys highlight an item. The popover is placed
// by UseFloating, as wide as the field, and exits with Presence.
type ProjectSearch struct {
	// Project is the chosen project; OnChoose is told of a new one.
	Project  string
	OnChoose func(project string)

	// Text and Open start the field with that text and its list open: a
	// preview's seed.
	Text string
	Open bool
}

func (s ProjectSearch) Build() tela.Node {
	anchor := tela.UseElementRef()
	cb := aria.UseCombobox(aria.ComboboxOptions[string]{
		Items:             projectNames,
		DefaultSelected:   aria.SelectionOf(s.Project),
		DefaultInputValue: s.Text,
		DefaultOpen:       s.Open,
		OnSelectionChange: func(sel aria.Selection[string]) {
			if k, ok := sel.First(); ok {
				s.OnChoose(k)
			}
		},
		AllowsEmptyCollection: true,
		AnchorRef:             anchor,
		MatchWidth:            true,
		Placement:             "bottom-start",
		Offset:                6,
		AriaLabel:             "Project",
		ButtonLabel:           "Show all projects",
	})
	return Fragment(
		Div(Ref(anchor), Class("relative w-72"),
			Icon(icons.Search, Class("pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground")),
			Input(cb.InputProps, Placeholder("Search projects"), Class(field, focusRing, "pr-9 pl-9")),
			Button(cb.ButtonProps,
				Class("absolute top-1/2 right-1 flex size-7 -translate-y-1/2 items-center justify-center rounded-sm text-muted-foreground",
					"transition-colors hover:bg-accent hover:text-accent-foreground aria-expanded:text-foreground"),
				ic(icons.ChevronsUpDown, "size-4")),
		),
		Portal(Presence(If(cb.Open,
			Div(cb.PopoverProps, Exit(exitPop), Class(surface, pop, "origin-top"),
				Div(cb.ListBox.Props, Class("max-h-72 overflow-y-auto outline-none"),
					Range(len(cb.Keys), func(i int) tela.Node {
						return projectOption(cb, cb.Keys[i])
					})),
				If(len(cb.Keys) == 0,
					Div(Role("status"), Class("px-2 py-6 text-center text-sm text-muted-foreground"),
						Text("No project matches your search.")))),
		))),
	)
}

// projectOption is one suggestion. The input keeps the focus, so the
// highlighted item is the list's virtual focus (IsFocused), not :focus.
func projectOption(cb aria.ComboboxResult[string], name string) tela.Node {
	kind := projectKind(name)
	return Div(cb.ListBox.ItemProps(name), Key(name),
		Class("flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none",
			"aria-disabled:opacity-50"),
		If(cb.ListBox.IsFocused(name), Class("bg-accent text-accent-foreground")),
		ic(kind.Icon, "size-4 text-muted-foreground"),
		Span(Class("flex-1 truncate"), Text(name)),
		Span(Class("text-xs text-muted-foreground"), Text(kind.Name)),
		Span(Class("flex size-4 items-center justify-center"),
			If(cb.ListBox.IsSelected(name), ic(icons.Check, "size-4"))),
	)
}
