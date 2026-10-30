package main

import (
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
)

// The command palette and the tooltips are built directly on aria hooks,
// with no kit component: they are the page's own controls, and show what
// the hooks give (focus, dismissal, the combobox's virtual focus,
// positioning) when the look is all yours.

// Command is an entry of the palette.
type Command struct {
	Key, Label, Group string
	Icon              tela.Icon
	Hint              string // muted text at the end, such as "8 open"
	Shortcut          string // keys shown at the end, separated by spaces
}

// Palette is a command palette: an overlay (UseOverlay: Escape and a press
// outside close it; modal, so the page is inert) holding a dialog
// (UseDialog: its name, the focus moved in and given back when it closes)
// around a combobox (UseCombobox) whose list is always open and filters the
// commands while typing; Escape reaches the overlay because that list cannot
// close, and Enter runs the first match (AutoFocusFirst). The input keeps
// the focus while the arrows move a virtual focus through the list
// (aria-activedescendant). Render it only
// while it is open, so it starts empty each time.
type Palette struct {
	Commands []Command
	Query    string // the text it starts with (a preview's seed)
	Close    func()
	Run      func(key string)
}

func (p Palette) Build() tela.Node {
	ov := aria.UseOverlay(aria.OverlayOptions{Open: true, OnClose: p.Close, Dismissable: true, Modal: true})
	dlg := aria.UseDialog(aria.DialogOptions{Modal: true, AriaLabel: "Command palette"})

	open := true
	byKey := map[string]Command{}
	keys := make([]string, len(p.Commands))
	for i, c := range p.Commands {
		byKey[c.Key] = c
		keys[i] = c.Key
	}
	cb := aria.UseCombobox(aria.ComboboxOptions[string]{
		Items:             keys,
		TextValue:         func(k string) string { return byKey[k].Label },
		DefaultInputValue: p.Query,
		Open:              &open,
		OnSelectionChange: func(sel aria.Selection[string]) {
			if k, ok := sel.First(); ok {
				p.Run(k)
			}
		},
		AllowsEmptyCollection: true,
		AllowsCustomValue:     true,
		AutoFocusFirst:        true,
		AriaLabel:             "Search commands",
	})

	var rows []tela.Child
	group := ""
	for _, k := range cb.Keys {
		c := byKey[k]
		if c.Group != group {
			group = c.Group
			rows = append(rows, Div(Key("g-"+group), Role("presentation"),
				Class("px-3 pt-3 pb-1", eyebrow), Text(group)))
		}
		active := cb.ListBox.IsFocused(k)
		rows = append(rows, Div(cb.ListBox.ItemProps(k), Key(k),
			Class("flex cursor-default items-center gap-3 rounded-lg px-3 py-2 text-[13px] outline-none"),
			If(active, Class("bg-accent text-foreground")),
			Icon(c.Icon, Class("size-4 shrink-0 text-muted-foreground")),
			Span(Class("flex-1 truncate"), Text(c.Label)),
			If(c.Hint != "", Span(Class("text-xs text-muted-foreground"), Text(c.Hint))),
			If(c.Shortcut != "", Span(Class("flex gap-1"), AriaHidden(true),
				Map(strings.Fields(c.Shortcut), func(s string) tela.Node {
					return Span(Key(s), Class(kbd), If(active, Class("border-white/15 text-foreground")), Text(s))
				}))),
		))
	}
	// The scrim is a plain translucent veil: a blur over the whole window
	// would be recomputed for every pixel the palette repaints under it
	// (see the glass budget). The panel itself is glass, and appears with
	// a short scale and fade; nothing has an exit, so it closes at once.
	return Div(Class("fixed inset-0 z-50"),
		Div(Class("absolute inset-0 bg-black/50")),
		Div(Class("absolute inset-0 flex items-start justify-center px-6 pt-[14vh]"),
			Div(ov.Props, dlg.Props,
				Class("flex w-full max-w-[38rem] flex-col overflow-hidden rounded-2xl border border-pane-border bg-popover text-popover-foreground shadow-2xl shadow-black/50 backdrop-blur-3xl",
					"transition duration-150 ease-out starting:scale-[0.97] starting:opacity-0"),
				Div(Class("flex items-center gap-3 border-b border-border px-4"),
					Icon(icons.Search, Class("size-[18px] text-muted-foreground")),
					Input(cb.InputProps, AutoFocus(true), Placeholder("Type a command or search…"),
						Class("h-14 flex-1 bg-transparent text-[15px] outline-none placeholder:text-muted-foreground")),
					Span(Class(kbd), AriaHidden(true), Text("esc")),
				),
				Div(cb.ListBox.Props, Class("max-h-[26rem] overflow-y-auto p-2 outline-none"),
					Fragment(rows...),
					If(len(cb.Keys) == 0, Div(Role("status"), Class("px-3 py-8 text-center text-sm text-muted-foreground"), Text("No command matches."))),
				),
				Div(Class("flex items-center gap-4 border-t border-border px-4 py-2.5 text-xs text-muted-foreground"), AriaHidden(true),
					Span(Class("flex items-center gap-1.5"), Span(Class(kbd), Icon(icons.CornerDownLeft, Class("size-3"))), Text("Run")),
					Span(Class("flex items-center gap-1.5"), Span(Class(kbd), Icon(icons.ArrowUp, Class("size-3"))),
						Span(Class(kbd), Icon(icons.ArrowDown, Class("size-3"))), Text("Navigate")),
					Span(Class("ml-auto flex items-center gap-1.5"), Span(Class(kbd), Text("esc")), Text("Close")),
				),
			),
		),
	)
}

// kbd is a key cap.
const kbd = "flex h-5 min-w-5 items-center justify-center rounded border border-border bg-white/[0.04] px-1 text-[10px] font-medium text-muted-foreground"

// Tip is a tooltip on aria.UseTooltipTrigger and aria.UseFloating: it opens
// when the pointer rests on the trigger (after 400 ms) or the keyboard
// focuses it (at once), closes with Escape, and describes the trigger
// (aria-describedby). Render receives the props for the trigger element.
type Tip struct {
	Text      string
	Placement string // "bottom" by default
	Open      bool   // starts open: a preview's seed
	Render    func(trigger tela.Prop) tela.Node
}

func (t Tip) Build() tela.Node {
	tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{DefaultOpen: t.Open, Delay: 400 * time.Millisecond})
	pl := t.Placement
	if pl == "" {
		pl = "bottom"
	}
	fl := aria.UseFloating(tip.Ref, aria.Placement(pl), aria.Offset(8), aria.Flip(), aria.Shift(8))
	return Fragment(
		t.Render(tip.TriggerProps),
		Portal(Presence(If(tip.Open,
			Div(tip.TooltipProps, fl.Props, Exit("scale-95 opacity-0 duration-100"),
				Class("pointer-events-none z-50 transition duration-150 starting:scale-95 starting:opacity-0", tipClass),
				Text(t.Text))))),
	)
}
