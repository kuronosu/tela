package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/chat/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Composer is the box at the bottom: an attach button, a text area that
// grows with what you type and a send button that stays disabled while
// there is nothing to send. Enter sends and Shift+Enter starts a new line.
//
// The draft is the app's (Draft and a SetDraft action), so it survives
// switching to another conversation and shows in the sidebar.
type Composer struct {
	ID, Name, Draft string
	Do              tela.Callback[func(Action)]
}

func (c Composer) Build() tela.Node {
	field := tela.UseElementRef()
	draft := c.Draft

	// send reads the text from the field itself, not from Draft: Draft is a
	// frame behind when Enter follows a key fast.
	send := func() {
		if text := strings.TrimSpace(field.Value()); text != "" {
			c.Do.Fn()(Action{Kind: Send, ID: c.ID, Text: text})
			field.Focus()
		}
	}
	onKey := OnKeyDown(func(e tela.KeyboardEvent) {
		if e.Key == tela.KeyEnter && !e.Shift {
			e.PreventDefault() // no new line: Shift+Enter is for that
			send()
		}
	})

	return Div(Class("shrink-0 px-5 pt-2 pb-5"),
		Div(Class("mx-auto flex max-w-3xl items-end gap-1 rounded-2xl border border-input bg-background p-1.5 shadow-xs transition-shadow",
			"focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50"),
			tip("Attach a file", kit.Button{Variant: kit.Ghost, Size: kit.Icon, Icon: icons.Paperclip, AriaLabel: "Attach a file", Class: "rounded-xl text-muted-foreground"}),
			kit.Textarea{
				Rows: 1, Placeholder: "Message " + firstName(c.Name), AriaLabel: "Message",
				Value: &draft, OnValueChange: func(s string) { c.Do.Fn()(Action{Kind: SetDraft, ID: c.ID, Text: s}) },
				// The area grows to its text up to 10 lines, then it scrolls. Its own
				// border and ring give way to the box around it.
				Class: "max-h-40 min-h-9 flex-1 resize-none border-0 bg-transparent px-2 py-2 shadow-none focus-visible:ring-0 dark:bg-transparent",
				Props: tela.MergeProps(Ref(field), onKey),
			},
			kit.Button{Size: kit.Icon, Icon: icons.SendHorizontal, AriaLabel: "Send message", Class: "rounded-xl",
				Disabled: strings.TrimSpace(c.Draft) == "", OnClick: send},
		),
	)
}

// firstName is the first word of a name.
func firstName(name string) string {
	first, _, _ := strings.Cut(name, " ")
	return first
}
