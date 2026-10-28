package main

import (
	"fmt"
	"strconv"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Sidebar lists the notes, the selected one marked with aria-current, and
// has the New note button.
type Sidebar struct {
	Notes    []Note
	Current  int
	OnSelect func(id int)
	OnNew    func()
}

func (s Sidebar) Build() tela.Node {
	return Div(Class("flex w-60 shrink-0 flex-col gap-2 border-e border-border bg-muted/40 p-2"),
		kit.Button{
			Variant: kit.Outline, Icon: icons.Plus, OnClick: s.OnNew, Class: "w-full bg-background",
			Children: Text("New note"),
		},
		Div(Class("flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto"), Role("navigation"), AriaLabel("Notes"),
			MapKeyed(s.Notes, func(n Note) int { return n.ID }, func(n Note) tela.Component {
				return NoteRow{Note: n, Active: n.ID == s.Current, OnSelect: s.OnSelect}
			}),
		),
	)
}

// NoteRow is a note of the sidebar: a button named by the title of the
// note, with a dot while the note has changes that were not saved to a file.
type NoteRow struct {
	Note     Note
	Active   bool
	OnSelect func(id int)
}

func (r NoteRow) Build() tela.Node {
	n := r.Note
	current, name := "", n.Title()
	if r.Active {
		current = "page"
	}
	if n.Dirty() {
		name += ", unsaved changes"
	}
	return Button(Key(n.ID), AriaLabel(name), AriaCurrent(current), OnClick(func() { r.OnSelect(n.ID) }),
		Class("flex w-full flex-col gap-0.5 rounded-md px-3 py-2 text-start outline-none transition-colors",
			"hover:bg-accent/60 focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-[current=page]:bg-accent"),
		Span(Class("flex items-center gap-2"),
			Span(Class("truncate text-sm font-medium"), Text(n.Title())),
			If(n.Dirty(), Span(Class("size-1.5 shrink-0 rounded-full bg-amber-500"), AriaHidden(true))),
		),
		Span(Class("truncate text-xs text-muted-foreground"), Text(n.Preview())),
	)
}

// Editor is the text of the open note and a status line under it. The text
// field is controlled: it shows the note's text and reports every edit. It
// is keyed by the note, so each note gets a field of its own (with its own
// caret and undo history).
type Editor struct {
	Note     Note
	Prefs    Prefs
	OnChange func(text string)
}

func (e Editor) Build() tela.Node {
	text := e.Note.Text
	wrap := "text-wrap"
	if !e.Prefs.WordWrap {
		wrap = "text-nowrap overflow-x-auto"
	}
	where := "Not saved to a file"
	if e.Note.Path != "" {
		where = e.Note.Path
	}
	return Div(Class("flex min-w-0 flex-1 flex-col"),
		kit.Textarea{
			AriaLabel: "Note text", Placeholder: "Start typing…", AutoFocus: true,
			Value: &text, OnValueChange: e.OnChange, Props: Key(strconv.Itoa(e.Note.ID)),
			Class: fmt.Sprintf("field-sizing-fixed min-h-0 flex-1 resize-none rounded-none border-0 bg-transparent px-6 py-5 leading-relaxed shadow-none focus-visible:ring-0 dark:bg-transparent %s %s",
				fontClass(e.Prefs.FontSize), wrap),
		},
		Div(Class("flex h-8 shrink-0 items-center justify-between gap-4 border-t border-border px-4 text-xs text-muted-foreground"),
			Span(Class("min-w-0 truncate"), Text(where)),
			Span(Class("shrink-0 tabular-nums"), Textf("%d words · %d characters", e.Note.Words(), e.Note.Characters())),
		),
	)
}
