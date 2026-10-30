package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
)

// Rail is the first pane: the New note button, the views, the folders and
// the tags, and Preferences at the bottom. It shares the title bar's color,
// so the window's chrome is one frame around the notes. Choosing an item
// shows the overview of that filter (OnFilter), also when it is the one
// that is already chosen: that is the way back from a note.
type Rail struct {
	Notes    []Note
	Filter   filter
	OnFilter func(filter)
	OnNew    func()
	OnPrefs  func()
}

func (r Rail) Build() tela.Node {
	count := func(f filter) int { return len(visible(r.Notes, f, "")) }
	item := func(icon tela.Icon, label string, f filter) tela.Node {
		return tela.WithKey(RailItem{Icon: icon, Label: label, Count: count(f), Active: r.Filter == f,
			OnClick: func() { r.OnFilter(f) }}, f.Kind+":"+f.Name)
	}
	return Div(Class("flex w-44 shrink-0 flex-col gap-5 bg-sidebar px-2 pb-2 text-sm"),
		Button(OnClick(r.OnNew),
			Class("flex h-9 w-full items-center justify-center gap-2 rounded-lg bg-primary text-[13px] font-semibold text-primary-foreground",
				"outline-none transition hover:bg-primary/90 focus-visible:ring-[3px] focus-visible:ring-ring/40 active:scale-[0.98]"),
			Icon(icons.Plus, Class("size-4")), Text("New note")),
		Div(Class("flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto"), Role("navigation"), AriaLabel("Library"),
			Div(Class("flex flex-col gap-px"),
				item(icons.Library, "All notes", viewAll),
				item(icons.Pin, "Pinned", viewPinned),
				item(icons.CircleDashed, "Unsaved", viewUnsaved),
			),
			If(len(folders(r.Notes)) > 0, Div(Class("flex flex-col gap-px"),
				Span(Class("px-2 pb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase"), Text("Folders")),
				Map(folders(r.Notes), func(f string) tela.Node { return item(icons.Folder, f, filter{kindFolder, f}) }),
			)),
			If(len(tagsOf(r.Notes)) > 0, Div(Class("flex flex-col gap-1.5"),
				Span(Class("px-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase"), Text("Tags")),
				Div(Class("flex flex-wrap gap-1 px-1"),
					Map(tagsOf(r.Notes), func(t string) tela.Node {
						f := filter{kindTag, t}
						return tela.WithKey(TagChip{Tag: t, Active: r.Filter == f, OnClick: func() { r.OnFilter(f) }}, t)
					}),
				),
			)),
		),
		Button(OnClick(r.OnPrefs),
			Class("flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-start text-[13px] text-foreground/70 outline-none transition-colors",
				"hover:bg-accent/60 hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40"),
			Icon(icons.Settings, Class("size-4 shrink-0 text-muted-foreground")), Text("Preferences")),
	)
}

// RailItem is a row of the rail: an icon, a name and a count. The chosen one
// is aria-current. It is named by its label alone, without the count.
type RailItem struct {
	Icon    tela.Icon
	Label   string
	Count   int
	Active  bool
	OnClick func()
}

func (r RailItem) Build() tela.Node {
	cur := ""
	if r.Active {
		cur = "page"
	}
	return Button(AriaLabel(r.Label), AriaCurrent(cur), OnClick(r.OnClick),
		Class("group flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-start text-[13px] text-foreground/70 outline-none transition-colors",
			"hover:bg-accent/60 hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40",
			"aria-[current=page]:bg-accent aria-[current=page]:font-medium aria-[current=page]:text-foreground"),
		Icon(r.Icon, Class("size-4 shrink-0 text-muted-foreground group-aria-[current=page]:text-gold")),
		Span(Class("min-w-0 flex-1 truncate"), Text(r.Label)),
		If(r.Count > 0, Span(Class("text-[11px] text-muted-foreground tabular-nums"), Textf("%d", r.Count))),
	)
}

// TagChip is a #tag of the rail that filters the notes.
type TagChip struct {
	Tag     string
	Active  bool
	OnClick func()
}

func (t TagChip) Build() tela.Node {
	cur := ""
	if t.Active {
		cur = "page"
	}
	return Button(AriaCurrent(cur), OnClick(t.OnClick),
		Class("rounded-md border border-border px-1.5 py-0.5 text-[12px] text-muted-foreground outline-none transition-colors",
			"hover:border-input hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40",
			"aria-[current=page]:border-gold/50 aria-[current=page]:bg-gold/10 aria-[current=page]:text-gold"),
		Text("#"+t.Tag))
}
