// Portions of this file follow shadcn/ui, alert.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// AlertStyle is the recipe of [Alert] (shadcn/ui's Alert, AlertTitle and
// AlertDescription).
type AlertStyle struct {
	// Root is what every alert has: a grid with a column for the icon (of
	// no width without one), a border and the card's colors.
	Root string
	// Variants are the looks: Default, Destructive, and the tones Success,
	// Warning and Info (shadcn has the first two).
	Variants map[Variant]string
	// WithIcon is added to Root when the alert has an Icon (shadcn's
	// has-[>svg]:): the icon column gets its width and a gap.
	WithIcon string
	// Icon is what the Icon slot has.
	Icon string
	// WithAction is added to Root when the alert has an Action: room for
	// the action at the end (shadcn's has-data-[slot=alert-action]:pr-18).
	// Action is the box of the Action slot, at the top end corner.
	WithAction, Action string
	// Title and Description are the two texts, in the second column.
	Title, Description string
	// DescriptionVariants are added to Description per alert variant
	// (shadcn's destructive alert softens its description).
	DescriptionVariants map[Variant]string
}

func init() {
	defaultStyle.Alert = AlertStyle{
		Root: "relative grid w-full grid-cols-[0_1fr] items-start gap-y-0.5 rounded-lg border border-border px-4 py-3 text-sm",
		Variants: map[Variant]string{
			Default:     "bg-card text-card-foreground",
			Destructive: "bg-card text-destructive",
			Success:     "border-emerald-500/30 bg-emerald-500/10 text-emerald-800 dark:border-emerald-400/30 dark:text-emerald-300",
			Warning:     "border-amber-500/40 bg-amber-500/10 text-amber-800 dark:border-amber-400/30 dark:text-amber-300",
			Info:        "border-sky-500/30 bg-sky-500/10 text-sky-800 dark:border-sky-400/30 dark:text-sky-300",
		},
		WithIcon:    "grid-cols-[1rem_1fr] gap-x-3",
		WithAction:  "pe-18",
		Action:      "absolute end-3 top-2.5",
		Icon:        "size-4 translate-y-0.5 text-current",
		Title:       "col-start-2 line-clamp-1 min-h-4 font-medium tracking-tight",
		Description: "col-start-2 grid justify-items-start gap-1 text-sm text-muted-foreground",
		DescriptionVariants: map[Variant]string{
			Destructive: "text-destructive/90",
			Success:     "text-current/80",
			Warning:     "text-current/80",
			Info:        "text-current/80",
		},
	}
}

// Alert is a callout that tells the user something, shadcn/ui's Alert: a
// bordered card with an optional icon, a title and a description.
//
//	kit.Alert{Icon: lucide.Terminal, Title: h.Text("Heads up!"),
//	    Description: h.Text("You can add components to your app using the CLI.")}
//
//	kit.Alert{Variant: kit.Destructive, Icon: lucide.CircleAlert, Title: h.Text("Payment failed"),
//	    Description: h.Text("Your card was declined. Update it and try again.")}
//
// The variants are [Default] and [Destructive], as in shadcn, and the tones
// [Success], [Warning] and [Info] that [Badge] has.
//
// Action puts a button (or a few) at the top end corner, shadcn's
// AlertAction: a way to undo, retry or dismiss. The alert keeps 72 px free
// at its end for it; a wider action needs a padding in Class ("pe-32").
//
//	kit.Alert{Title: h.Text("Update available"), Description: h.Text("Restart to install it."),
//	    Action: kit.Button{Size: kit.Xs, Variant: kit.Outline, Children: h.Text("Restart")}}
//
// An alert has the alert role, as shadcn's does: a live region that
// assistive technologies announce at once when it appears in the page, which
// is right for an error that shows after the user did something. An alert
// that is part of the page from the start (a notice at the top of a form)
// is not an interruption: give it Role "status" (a polite live region) or
// "note" (none).
type Alert struct {
	// Variant is Default, Destructive, Success, Warning or Info.
	Variant Variant
	// Icon is drawn in the first column, level with the title.
	Icon tela.Icon
	// Title is the first line, in medium weight; Description follows it
	// in the muted color, and may be several lines or a few paragraphs.
	Title, Description tela.Component
	// Action is drawn at the top end corner of the alert, over the room the
	// alert keeps for it; see above.
	Action tela.Component
	// Role is "alert" (the default), "status" or "note".
	Role string
	// Class is added after the recipe's classes. TitleClass and
	// DescriptionClass do the same for their part.
	Class            string
	TitleClass       string
	DescriptionClass string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the alert element.
func (a Alert) Build() tela.Node {
	s := &UseStyle().Alert
	role := a.Role
	if role == "" {
		role = "alert"
	}
	if devMode() && role != "alert" && role != "status" && role != "note" {
		panic("kit: Alert.Role " + role + " is not one of \"alert\", \"status\" and \"note\"")
	}
	hasIcon := a.Icon != (tela.Icon{})
	var title, desc tela.Child
	if a.Title != nil {
		title = h.Div(cls(s.Title, a.TitleClass), a.Title)
	}
	if a.Description != nil {
		desc = h.Div(cls(s.Description, s.DescriptionVariants[a.Variant], a.DescriptionClass), a.Description)
	}
	var action tela.Child
	if a.Action != nil {
		action = h.Div(cls(s.Action), a.Action)
	}
	return h.Div(cls(s.Root, pick("Alert", "Variant", s.Variants, a.Variant), when(hasIcon, s.WithIcon), when(a.Action != nil, s.WithAction), a.Class),
		h.Role(role), a.Props, icon(a.Icon, s.Icon), title, desc, action)
}
