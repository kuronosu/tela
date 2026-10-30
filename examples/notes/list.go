package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// searchBox is the flat search field of the list and the overview: it
// filters the notes by their title and text.
func searchBox(query string, set func(string), class string) tela.Node {
	return Label(Class("relative flex", class),
		Icon(icons.Search, Class("pointer-events-none absolute start-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground")),
		Input(Type("search"), AriaLabel("Search notes"), Placeholder("Search"), Bind(query, set),
			Class("block h-9 w-full rounded-lg border border-border bg-card ps-9 pe-3 text-[13px] outline-none placeholder:text-muted-foreground",
				"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/25")))
}

// List is the second pane, next to an open note: the search field and the
// cards of the notes of the chosen filter in a column, the open one marked
// with aria-current. Escape in it clears the search and then goes back to
// the overview (OnBack).
type List struct {
	Notes    []Note
	Current  int
	Query    string
	Previews bool
	OnQuery  func(string)
	OnSelect func(id int)
	OnBack   func()
	// OnPin and OnDelete are the actions of a card's context menu.
	OnPin, OnDelete func(id int)
}

func (l List) Build() tela.Node {
	escape := OnKeyDown(func(e tela.KeyboardEvent) {
		if e.Key != "Escape" || e.Ctrl || e.Alt || e.Meta || e.Shift {
			return
		}
		e.PreventDefault()
		if l.Query != "" {
			l.OnQuery("")
			return
		}
		l.OnBack()
	})
	return Div(Class("flex w-64 shrink-0 flex-col gap-3 border-e border-border pt-3"), escape,
		searchBox(l.Query, l.OnQuery, "mx-3"),
		Div(Class("flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-3 pb-3"), Role("navigation"), AriaLabel("Notes"),
			Map(l.Notes, func(n Note) tela.Node {
				return tela.WithKey(Card{Note: n, Active: n.ID == l.Current, Preview: l.Previews, OnSelect: l.OnSelect, OnPin: l.OnPin, OnDelete: l.OnDelete}, n.ID)
			}),
			If(len(l.Notes) == 0, Span(Class("px-1 py-6 text-center text-[13px] text-muted-foreground"), Text(nothingText(l.Query)))),
		),
	)
}

// nothingText says why a list is empty.
func nothingText(query string) string {
	if query != "" {
		return fmt.Sprintf("Nothing matches %q.", query)
	}
	return "No notes here yet."
}

// Card is a note as a card: its title, a few lines of text, its folder and
// its date, a dot while it has unsaved changes and a pin while it is
// pinned. It is a button named by the heading of the note, and aria-current
// while it is the open one. Grid is the card of the overview, which shows
// more text than the one of the list; Preview, for the list, turns the text
// off.
type Card struct {
	Note     Note
	Active   bool
	Grid     bool
	Preview  bool
	OnSelect func(id int)
	// OnPin and OnDelete are the items of the card's context menu (a right
	// click, or Shift+F10 with the card focused).
	OnPin, OnDelete func(id int)
}

func (c Card) Build() tela.Node {
	n := c.Note
	cur := ""
	if c.Active {
		cur = "page"
	}
	name := n.Heading()
	if n.Dirty() {
		name += ", unsaved changes"
	}
	lines, limit := "line-clamp-2", 140
	if c.Grid {
		lines, limit = "line-clamp-4", 280
	}
	pin := "Pin"
	if n.Pinned {
		pin = "Unpin"
	}
	return Fragment(kit.ContextMenu[string]{
		AriaLabel: "Note actions", Class: "w-full",
		Items: []kit.MenuItem[string]{
			{Key: "pin", Label: pin, Icon: icons.Pin, OnSelect: func() { c.OnPin(n.ID) }},
			{Key: "delete", Label: "Delete", Icon: icons.Trash, Variant: kit.Destructive, OnSelect: func() { c.OnDelete(n.ID) }},
		},
		Children: c.button(name, cur, lines, limit),
	})
}

// button is the card itself.
func (c Card) button(name, cur, lines string, limit int) tela.Node {
	n := c.Note
	return Button(AriaLabel(name), AriaCurrent(cur), OnClick(func() { c.OnSelect(n.ID) }),
		Class("flex w-full flex-col gap-2 rounded-xl border border-border bg-card p-3.5 text-start outline-none transition-colors",
			"hover:border-input focus-visible:ring-[3px] focus-visible:ring-ring/40",
			"aria-[current=page]:border-gold/60 aria-[current=page]:ring-1 aria-[current=page]:ring-gold/25"),
		Span(Class("flex items-center gap-2"),
			Span(Class("min-w-0 flex-1 truncate text-[14px] font-semibold tracking-tight"), Text(n.Heading())),
			If(n.Dirty(), Span(Class("size-1.5 shrink-0 rounded-full bg-gold"), AriaHidden(true))),
			If(n.Pinned, Icon(icons.Pin, Class("size-3 shrink-0 text-gold"), AriaHidden(true))),
		),
		If(c.Grid || c.Preview, Span(Class("text-start text-[13px] leading-5 text-muted-foreground", lines), Text(n.Snippet(limit)))),
		Span(Class("flex items-center gap-1.5 text-[11px] text-muted-foreground"),
			If(n.Folder != "", Span(Class("flex min-w-0 items-center gap-1"),
				Icon(icons.Folder, Class("size-3 shrink-0")), Span(Class("truncate"), Text(n.Folder)))),
			Span(Class("ms-auto shrink-0 tabular-nums"), Text(n.When())),
		),
	)
}

// Overview is what the notes pane shows with no note open: the title of the
// filter with the number of notes, the search field and a grid of cards.
type Overview struct {
	Notes    []Note
	Filter   filter
	Query    string
	OnQuery  func(string)
	OnSelect func(id int)
	// OnPin and OnDelete are the actions of a card's context menu.
	OnPin, OnDelete func(id int)
}

func (o Overview) Build() tela.Node {
	return Div(Class("flex min-w-0 flex-1 flex-col gap-4 overflow-y-auto px-7 pt-6 pb-6"),
		Div(Class("flex items-center gap-4"),
			Div(Class("flex min-w-0 flex-1 items-baseline gap-3"),
				H1(Class("truncate text-2xl font-semibold tracking-tight"), Text(o.Filter.Title())),
				Span(Class("text-sm text-muted-foreground tabular-nums"), Textf("%d", len(o.Notes))),
			),
			searchBox(o.Query, o.OnQuery, "w-56 shrink-0"),
		),
		Div(Class("grid grid-cols-2 items-start gap-3 min-[880px]:grid-cols-3"),
			Map(o.Notes, func(n Note) tela.Node {
				return tela.WithKey(Card{Note: n, Grid: true, OnSelect: o.OnSelect, OnPin: o.OnPin, OnDelete: o.OnDelete}, n.ID)
			}),
		),
		If(len(o.Notes) == 0, Span(Class("py-10 text-center text-sm text-muted-foreground"), Text(nothingText(o.Query)))),
	)
}
