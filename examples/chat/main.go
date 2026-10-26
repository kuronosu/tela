// Command chat is a messaging app screen built on package kit in its default
// style: a sidebar with a search box and the list of conversations, and next
// to it the open conversation with its header, its messages and a composer.
// Sending a message shows a typing indicator for a moment, then the other
// side answers.
//
// What it shows:
//
//   - State: one Store in tela.UseReducer (model.go). A draft, a selection
//     and a reply are actions on it, the reducer is pure, and the root
//     stamps the time on the actions that make a message. The conversations, a Person and an
//     immutable *Thread are comparable, so the components that get them as
//     props skip their build until their piece changes: typing in the
//     composer rebuilds the composer and the sidebar row of its draft, and
//     leaves the message list alone.
//   - The reply: tela.UseTimeout on the frame clock, so there is no
//     goroutine and the tests move it with App.Advance. The typing dots are
//     animate-[bounce] classes.
//   - Scrolling: the message list is a VirtualList that scrolls itself to its
//     last row, in a layout effect that runs after layout and before paint,
//     whenever a row comes or the typing dots toggle (pane.go).
//   - The current conversation: the sidebar row has h.AriaCurrent("page"),
//     styled with the aria-[current=page]: variant and reported to
//     assistive technologies; the rows are buttons, reached with Tab.
//   - kit: Input, Textarea, Button, Tooltip, DropdownMenu, Avatar, Badge and
//     Separator, with theme tokens only (bg-muted, text-muted-foreground),
//     so the screen follows the color scheme with no dark: classes of its
//     own beyond the avatar tints.
//
// `tela snapshot ./examples/chat` renders it without a window; add --dark
// for the dark theme.
package main

import (
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// replyDelay is how long the other side "types" before the reply arrives.
const replyDelay = 1800 * time.Millisecond

// Chat is the whole screen. Clock says what time it is for the messages you
// send and for the seed data; nil is time.Now, and the tests pass their own.
type Chat struct{ Clock func() time.Time }

func (c Chat) Build() tela.Node {
	now := c.Clock
	if now == nil {
		now = time.Now
	}
	store, dispatch := tela.UseReducer(reduce, tela.UseOnce(func() Store { return Seed(now()) }))
	do := tela.UseCallback(func(a Action) {
		if a.Kind == Send || a.Kind == Reply { // the actions that make a message
			a.At = now()
		}
		dispatch.Dispatch(a)
	})

	// While someone is typing, a reply is due: UseTimeout waits for it on the
	// frame clock, and starts over if the pending reply changes (you wrote
	// again). A zero duration cancels it.
	due, wait := store.Pending(), time.Duration(0)
	if due.ID != "" {
		wait = replyDelay
	}
	tela.UseTimeout(wait, func() { do.Fn()(Action{Kind: Reply, ID: due.ID}) }, due)

	open := store.Find(store.Active)
	return Div(Class("flex h-screen overflow-hidden"),
		Sidebar{Convs: store.Convs, Active: store.Active, Today: store.Today, Unread: store.Unread(), Do: do},
		// Keyed by conversation: each one opens with a fresh list, scrolled to
		// its end, and a fresh composer.
		Div(Key(open.ID), Class("flex min-w-0 flex-1 flex-col"),
			Header{Person: open.Person, Muted: open.Muted, Typing: open.Typing, Do: do},
			Messages{Person: open.Person, Thread: open.Thread, Typing: open.Typing, Today: store.Today},
			Composer{ID: open.ID, Name: open.Name, Draft: open.Draft, Do: do},
		),
	)
}

// theme is kit's theme with a violet primary color, for the bubbles you send
// and the focus ring.
var theme = chatTheme()

func chatTheme() tela.Theme {
	t := kit.Theme()
	t.Colors["primary"] = tela.SchemeColor{Light: tela.Hex("#7c3aed"), Dark: tela.Hex("#7c3aed")}
	t.Colors["primary-foreground"] = tela.SchemeColor{Light: tela.Hex("#ffffff"), Dark: tela.Hex("#ffffff")}
	t.Colors["ring"] = tela.SchemeColor{Light: tela.Hex("#8b5cf6"), Dark: tela.Hex("#a78bfa")}
	return t
}

// App is the window; `tela snapshot`, `tela mcp` and the tests read it too.
var App = tela.App{
	Title:     "Tela — chat",
	AppID:     "dev.telaui.chat",
	Width:     1100,
	Height:    720,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Chat{},
}

func main() { tela.Main(App) }
