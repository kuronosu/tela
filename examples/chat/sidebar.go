package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/chat/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// accent colors the small texts that stand out (a time with unread messages,
// the tick of a read message, "typing…"): the primary violet, a shade lighter
// on the dark background, where the primary of the bubbles is too dim for
// small text.
const accent = "text-violet-600 dark:text-violet-400"

// statusDot is the color of the presence dot per status.
var statusDot = map[Status]string{
	Online:  "bg-emerald-500",
	Away:    "bg-amber-400",
	Offline: "bg-muted-foreground/40",
}

// Face is a person's avatar with the dot of their presence on its corner.
type Face struct {
	Person Person
	Size   kit.Size
}

func (f Face) Build() tela.Node {
	return Div(Class("relative shrink-0"), AriaHidden(true),
		kit.Avatar{Size: f.Size, Fallback: f.Person.Initials, FallbackClass: f.Person.Tone + " font-medium"},
		Div(Class("absolute right-0 bottom-0 size-3 rounded-full border-2 border-background", statusDot[f.Person.Status])),
	)
}

// Sidebar has the search box and the conversations, newest first. The text
// of the search is its own state: it is not the rest of the app's business.
type Sidebar struct {
	Convs  []Conv
	Active string
	Today  time.Time
	Unread int
	Do     tela.Callback[func(Action)]
}

func (s Sidebar) Build() tela.Node {
	query, setQuery := tela.UseState("")
	rows := s.matching(query)
	return Div(Class("flex w-80 shrink-0 flex-col border-r border-border bg-muted/40"),
		Div(Class("flex h-16 shrink-0 items-center justify-between px-5"),
			H1(Class("text-lg font-semibold tracking-tight"), Text("Messages")),
			If(s.Unread > 0, kit.Badge{Children: Textf("%d unread", s.Unread)}),
		),
		Div(Class("relative px-4 pb-3"),
			Icon(icons.Search, Class("pointer-events-none absolute top-2.5 left-7 size-4 text-muted-foreground")),
			kit.Input{
				Type: "search", Placeholder: "Search messages", AriaLabel: "Search conversations", Class: "bg-background pl-9",
				Value: &query, OnValueChange: setQuery.Set,
			},
		),
		Div(Class("flex flex-1 flex-col gap-0.5 overflow-y-auto px-2 pb-3"), Role("navigation"), AriaLabel("Conversations"),
			MapKeyed(rows, func(c Conv) string { return c.ID }, func(c Conv) tela.Component {
				return ConvRow{Conv: c, Active: c.ID == s.Active, Today: s.Today, Do: s.Do}
			}),
			If(len(rows) == 0, P(Class("px-3 py-8 text-center text-sm text-muted-foreground"), Textf("No conversation matches \"%s\".", query))),
		),
	)
}

// matching returns the conversations whose name or last message contains
// the query, the one with the newest message first.
func (s Sidebar) matching(query string) []Conv {
	query = strings.ToLower(strings.TrimSpace(query))
	var rows []Conv
	for _, c := range s.Convs {
		last, _ := c.Thread.Last()
		if strings.Contains(strings.ToLower(c.Name+" "+last.Text), query) {
			rows = append(rows, c)
		}
	}
	slices.SortStableFunc(rows, func(a, b Conv) int {
		la, _ := a.Thread.Last()
		lb, _ := b.Thread.Last()
		return lb.At.Compare(la.At)
	})
	return rows
}

// ConvRow is one conversation of the list: a button, the current one
// marked with aria-current="page" and styled by the aria-[current=page]:
// variant. Its props are comparable, so it builds again only when its
// conversation changes.
type ConvRow struct {
	Conv   Conv
	Active bool
	Today  time.Time
	Do     tela.Callback[func(Action)]
}

func (r ConvRow) Build() tela.Node {
	c := r.Conv
	last, ok := c.Thread.Last()
	current := ""
	if r.Active {
		current = "page"
	}

	// The preview: your draft, if you have one in a conversation that is not
	// open, else the last message.
	var preview tela.Child = Text("No messages yet")
	switch {
	case c.Draft != "" && !r.Active:
		preview = Fragment(Span(Class("text-destructive"), Text("Draft: ")), Text(c.Draft))
	case ok && last.Mine:
		preview = Text("You: " + last.Text)
	case ok:
		preview = Text(last.Text)
	}

	// The name says who and how many are unread; the preview is for the eyes.
	name := c.Name
	if c.Unread > 0 {
		name += fmt.Sprintf(", %d unread", c.Unread)
	}

	return Button(Key(c.ID), AriaLabel(name), AriaCurrent(current), OnClick(func() { r.Do.Fn()(Action{Kind: Select, ID: c.ID}) }),
		Class("flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left outline-none transition-colors",
			"hover:bg-accent/60 focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-[current=page]:bg-accent"),
		Face{Person: c.Person, Size: kit.Lg},
		Div(Class("flex min-w-0 flex-1 flex-col gap-0.5"),
			Div(Class("flex items-baseline justify-between gap-2"),
				Span(Class("truncate text-sm font-medium"), Text(c.Name)),
				If(ok, Span(Class("shrink-0 text-xs text-muted-foreground tabular-nums"), If(c.Unread > 0 && !c.Muted, Class("font-medium", accent)),
					Text(shortTime(last.At, r.Today)))),
			),
			Div(Class("flex items-center gap-2"),
				Span(Class("flex-1 truncate text-sm"), IfElse(c.Unread > 0, Class("text-foreground"), Class("text-muted-foreground")), preview),
				If(c.Muted, Icon(icons.BellOff, Class("size-3.5 shrink-0 text-muted-foreground"))),
				If(c.Unread > 0, kit.Badge{
					Variant: badgeVariant(c.Muted), Class: "h-5 min-w-5 shrink-0 px-1.5 tabular-nums",
					Children: Textf("%d", c.Unread),
				}),
			),
		),
	)
}

// badgeVariant is the look of an unread count: loud, or quiet in a
// conversation you muted.
func badgeVariant(muted bool) kit.Variant {
	if muted {
		return kit.Secondary
	}
	return kit.Default
}
