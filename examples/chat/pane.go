package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/chat/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Header is the top of the conversation: who it is with, whether they are
// around (or typing), and the call buttons, each with a tooltip, and a menu.
type Header struct {
	Person Person
	Muted  bool
	Typing bool
	Do     tela.Callback[func(Action)]
}

func (hd Header) Build() tela.Node {
	p := hd.Person
	mute := "Mute notifications"
	if hd.Muted {
		mute = "Unmute notifications"
	}
	menu := []kit.MenuItem[string]{
		{Key: "mute", Label: mute, Icon: icons.BellOff, OnSelect: func() { hd.Do.Fn()(Action{Kind: ToggleMute, ID: p.ID}) }},
		{Kind: kit.MenuSeparator},
		{Key: "clear", Label: "Clear messages", Icon: icons.Trash2, Variant: kit.Destructive, OnSelect: func() { hd.Do.Fn()(Action{Kind: Clear, ID: p.ID}) }},
	}

	status := Div(Class("flex items-center gap-1.5 text-xs text-muted-foreground"),
		Span(Class("size-1.5 rounded-full", statusDot[p.Status])), Text(string(p.Status)))
	if hd.Typing {
		status = Div(Class("text-xs font-medium", accent), Text("typing…"))
	}

	return Div(Class("flex h-16 shrink-0 items-center gap-3 border-b border-border px-5"),
		Face{Person: p, Size: kit.Lg},
		Div(Class("flex min-w-0 flex-1 flex-col gap-0.5"),
			H2(Class("truncate text-sm leading-none font-semibold"), Text(p.Name)),
			status,
		),
		Div(Class("flex items-center gap-1 text-muted-foreground"),
			tip("Voice call", kit.Button{Variant: kit.Ghost, Size: kit.Icon, Icon: icons.Phone, AriaLabel: "Voice call"}),
			tip("Video call", kit.Button{Variant: kit.Ghost, Size: kit.Icon, Icon: icons.Video, AriaLabel: "Video call"}),
			// A tooltip and a menu on one button: a button takes one overlay's
			// props, so the tooltip goes on a span around the menu's button.
			tip("More", kit.TriggerFunc(func(p tela.Prop) tela.Node {
				return Span(p, Class("flex"), kit.DropdownMenu[string]{
					Align: "end", AriaLabel: "More actions", Items: menu,
					Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, Icon: icons.Ellipsis, AriaLabel: "More actions"},
				})
			})),
		),
	)
}

// tip puts a tooltip with a text on a trigger.
func tip(text string, trigger kit.Trigger) tela.Node {
	return Fragment(kit.Tooltip{Trigger: trigger, Content: Text(text), Placement: "bottom"})
}

// Messages is the scrolling list of a conversation: its rows, and the
// typing indicator while a reply is coming. It is a VirtualList, as a chat
// history should be (a long one builds only the rows in view), and it scrolls
// to its last row when it opens and whenever a row is added.
type Messages struct {
	Person Person
	Thread *Thread
	Typing bool
	Today  time.Time
}

func (m Messages) Build() tela.Node {
	list := tela.UseElementRef()
	rows := m.Thread.Items(m.Person.Name, m.Today)
	count := len(rows)
	if m.Typing {
		count++ // the indicator is a row at the end
	}

	// A layout effect runs after layout and before paint, so the first frame
	// of a message is already scrolled to it: no flash at the top. It runs
	// again when a row comes or the indicator toggles (the count alone would
	// miss the indicator turning into the reply). ScrollToIndex keeps the row
	// aligned while the rows around it are measured, which a list that has
	// not measured them yet needs.
	tela.UseLayoutEffect(func() func() {
		list.ScrollToIndex(count-1, tela.AlignEnd)
		return nil
	}, len(rows), m.Typing)

	if count == 0 {
		return Empty{Name: m.Person.Name}.Build()
	}
	return VirtualList(count, func(int) float32 { return 56 },
		func(i int) tela.Component {
			if i < len(rows) {
				return rows[i]
			}
			return Typing{Name: m.Person.Name}
		},
		ItemKey(func(i int) string {
			if i < len(rows) {
				return rows[i].Key
			}
			return "typing"
		}),
		Ref(list), Class("flex-1 py-5"), Role("log"), AriaLabel("Conversation with "+m.Person.Name),
	)
}

// column is the classes of a row of the list: the messages sit in a column of
// at most 768 px, centered in a wider window.
const column = "mx-auto w-full max-w-3xl px-6"

// Empty is what a conversation without messages says.
type Empty struct{ Name string }

func (e Empty) Build() tela.Node {
	return Div(Class("flex flex-col items-center gap-2 py-24 text-center"),
		Div(Class("flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground"),
			Icon(icons.MessageSquare, Class("size-5"))),
		P(Class("text-sm font-medium"), Text("No messages yet")),
		P(Class("text-sm text-muted-foreground"), Textf("Say hello to %s.", e.Name)),
	)
}

// Build draws a row: a day separator, or a message bubble. The bubbles of
// one side that follow each other are a group: tight spacing, the corners
// that touch squared off, and the time and read state only under the last.
func (i Item) Build() tela.Node {
	if i.Day != "" {
		return Div(Class(column, "flex items-center gap-3 py-4 text-xs font-medium text-muted-foreground"),
			kit.Separator{Class: "flex-1"}, Text(i.Day), kit.Separator{Class: "flex-1"})
	}
	m := i.Msg
	// Your bubbles are on the right in the primary color and theirs on the
	// left in a muted one; the corner on the sender's side is squared where
	// the bubble touches the next or the previous one, and has a small tail
	// on the last.
	sender, side, bubble := i.From, "items-start", "rounded-2xl bg-muted text-foreground"
	top, bottom, tail := "rounded-tl-md", "rounded-bl-md", "rounded-bl-sm"
	if m.Mine {
		sender, side, bubble = "You", "items-end", "rounded-2xl bg-primary text-primary-foreground"
		top, bottom, tail = "rounded-tr-md", "rounded-br-md", "rounded-br-sm"
	}
	return Div(Class(column, "flex flex-col", side), IfElse(i.First, Class("pt-3"), Class("pt-0.5")),
		Role("article"), AriaLabel(sender+", "+m.At.Format("3:04 PM")),
		Div(Class("max-w-[75%] px-3.5 py-2 text-sm leading-relaxed whitespace-pre-wrap", bubble),
			If(!i.First, Class(top)), IfElse(i.Last, Class(tail), Class(bottom)),
			Text(m.Text)),
		If(i.Last, Div(Class("mt-1 flex items-center gap-1 px-1 text-[11px] text-muted-foreground"),
			Text(m.At.Format("3:04 PM")),
			If(m.Mine, Receipt{Read: m.Read}))),
	)
}

// Receipt is the tick after the time of a message of yours: one when it
// was sent, two in the primary color when they have read it.
type Receipt struct{ Read bool }

func (r Receipt) Build() tela.Node {
	if r.Read {
		return Icon(icons.CheckCheck, Class("size-3.5", accent), AriaLabel("Read"), Role("img"))
	}
	return Icon(icons.Check, Class("size-3.5"), AriaLabel("Sent"), Role("img"))
}

// Typing is the bubble of three bouncing dots that stands for the reply.
// The dots are the same animate-bounce keyframes, started a fraction of a
// second apart by the delay in the animation shorthand.
type Typing struct{ Name string }

func (t Typing) Build() tela.Node {
	dot := func(animation string) tela.Node {
		return Span(Class("size-1.5 rounded-full bg-muted-foreground/70", animation))
	}
	return Div(Class(column, "pt-3"),
		Div(Class("flex w-fit items-center gap-1 rounded-2xl rounded-bl-md bg-muted px-4 py-3.5"),
			Role("status"), AriaLabel(t.Name+" is typing"),
			dot("animate-[bounce_1s_0ms_infinite]"), dot("animate-[bounce_1s_150ms_infinite]"), dot("animate-[bounce_1s_300ms_infinite]")))
}
