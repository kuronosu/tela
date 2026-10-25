// Portions of this file follow shadcn/ui, separator.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// SeparatorStyle is the recipe of [Separator] (shadcn/ui's Separator).
type SeparatorStyle struct {
	Root string
	// Orientations are a 1 px line across the available width
	// (Horizontal) and one down the available height (Vertical).
	Orientations map[Orientation]string
}

func init() {
	defaultStyle.Separator = SeparatorStyle{
		Root: "shrink-0 bg-border",
		Orientations: map[Orientation]string{
			Horizontal: "h-px w-full",
			Vertical:   "h-full w-px",
		},
	}
}

// Separator is a thin line that divides content, shadcn/ui's Separator.
// By default it is decorative and hidden from assistive technologies, like
// Radix's decorative separator; Semantic gives it the separator role.
//
//	kit.Separator{}
//	h.Div(cls("flex h-5 items-center gap-4"), h.Text("Blog"), kit.Separator{Orientation: kit.Vertical}, h.Text("Docs"))
type Separator struct {
	// Orientation is Horizontal (the default) or Vertical.
	Orientation Orientation
	// Semantic makes the separator part of the accessibility tree, with
	// the separator role and its orientation, for a line that divides
	// sections of the content and not just decorates it.
	Semantic bool
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the separator element.
	Props tela.Child
}

// Build returns the separator element.
func (s Separator) Build() tela.Node {
	r := &UseStyle().Separator
	o := orientationOr(s.Orientation, Horizontal)
	kids := []tela.Child{cls(r.Root, pick("Separator", "Orientation", r.Orientations, o), s.Class), s.Props}
	if s.Semantic {
		kids = append(kids, h.Role("separator"), h.AriaOrientation(string(o)))
	} else {
		kids = append(kids, h.AriaHidden(true))
	}
	return h.Div(kids...)
}
