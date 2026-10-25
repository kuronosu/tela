// Portions of this file follow shadcn/ui, card.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// CardStyle is the recipe of [Card] (shadcn/ui's Card, CardHeader,
// CardTitle, CardDescription, CardAction, CardContent and CardFooter).
type CardStyle struct {
	Root string
	// Header holds the title, the description and the action.
	Header string
	// HeaderWithAction is added to Header when the card has an Action: the
	// header becomes two columns (shadcn's has-data-[slot=card-action]:).
	HeaderWithAction string
	Title            string
	Description      string
	Action           string
	Content          string
	Footer           string
}

func init() {
	defaultStyle.Card = CardStyle{
		Root:             "flex flex-col gap-6 rounded-xl border border-border bg-card py-6 text-card-foreground shadow-sm",
		Header:           "grid auto-rows-min grid-rows-[auto_auto] items-start gap-2 px-6",
		HeaderWithAction: "grid-cols-[1fr_auto]",
		Title:            "leading-none font-semibold",
		Description:      "text-sm text-muted-foreground",
		Action:           "col-start-2 row-span-2 row-start-1 self-start justify-self-end",
		Content:          "px-6",
		Footer:           "flex items-center px-6",
	}
}

// Card is a surface with a header, content and a footer, shadcn/ui's Card.
// Its parts are slots; an absent slot (nil) draws nothing, and the header
// is drawn when the card has a Title, a Description or an Action.
//
//	kit.Card{
//	    Title:       h.Text("Create project"),
//	    Description: h.Text("Deploy your new project in one click."),
//	    Action:      kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icons.X, AriaLabel: "Close"},
//	    Children:    form,
//	    Footer:      h.Fragment(kit.Button{Variant: kit.Outline, Children: h.Text("Cancel")}, ...),
//	}
//
// The recipe's Header, Title, Description, Action, Content and Footer
// classes style the parts; the fields ending in Class add to them for one
// card.
//
// shadcn's [.border-b]:pb-6 and [.border-t]:pt-6 (a header or footer that
// has a border brings its own padding) are not in the recipe: add pb-6 or
// pt-6 with HeaderClass and FooterClass when you add the border.
type Card struct {
	// Title and Description are the header's heading and its muted line.
	Title, Description tela.Component
	// Action sits at the header's right end: a button, a menu.
	Action tela.Component
	// Children is the content.
	Children tela.Component
	// Footer is the card's last row.
	Footer tela.Component

	// Class is added after the recipe's Root classes. The other Class
	// fields do the same for their part.
	Class            string
	HeaderClass      string
	TitleClass       string
	DescriptionClass string
	ActionClass      string
	ContentClass     string
	FooterClass      string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the card element.
func (c Card) Build() tela.Node {
	s := &UseStyle().Card
	var parts []tela.Child
	parts = append(parts, cls(s.Root, c.Class), c.Props)
	if c.Title != nil || c.Description != nil || c.Action != nil {
		header := []tela.Child{cls(s.Header, when(c.Action != nil, s.HeaderWithAction), c.HeaderClass)}
		if c.Title != nil {
			header = append(header, h.Div(cls(s.Title, c.TitleClass), c.Title))
		}
		if c.Description != nil {
			header = append(header, h.Div(cls(s.Description, c.DescriptionClass), c.Description))
		}
		if c.Action != nil {
			header = append(header, h.Div(cls(s.Action, c.ActionClass), c.Action))
		}
		parts = append(parts, h.Div(header...))
	}
	if c.Children != nil {
		parts = append(parts, h.Div(cls(s.Content, c.ContentClass), c.Children))
	}
	if c.Footer != nil {
		parts = append(parts, h.Div(cls(s.Footer, c.FooterClass), c.Footer))
	}
	return h.Div(parts...)
}
