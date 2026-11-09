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
	// Captioned is the root of a separator with a Caption: the two lines
	// and the caption between them, in a row (Horizontal) or a column
	// (Vertical). Caption is the caption's part; Line is each of the two
	// lines, by orientation.
	Captioned map[Orientation]string
	Caption   string
	Line      map[Orientation]string
	// LineShort is the line on the side of the caption that a CaptionAlign
	// of CaptionStart or CaptionEnd makes short, by orientation.
	LineShort map[Orientation]string
}

// CaptionAlign is where a [Separator]'s Caption sits on the line. The zero
// value is the middle; start and end follow the direction (the caption of a
// right-to-left page starts at the right), and in a Vertical separator they
// are the top and the bottom.
type CaptionAlign string

const (
	CaptionCenter CaptionAlign = ""
	// CaptionStart puts the caption near the start of the line, after a short
	// line.
	CaptionStart CaptionAlign = "start"
	// CaptionEnd puts it near the end, before a short line.
	CaptionEnd CaptionAlign = "end"
)

func init() {
	defaultStyle.Separator = SeparatorStyle{
		Root: "shrink-0 bg-border",
		Orientations: map[Orientation]string{
			Horizontal: "h-px w-full",
			Vertical:   "h-full w-px",
		},
		Captioned: map[Orientation]string{
			Horizontal: "flex w-full items-center gap-2",
			Vertical:   "flex h-full flex-col items-center gap-2",
		},
		Caption: "shrink-0 text-sm text-muted-foreground",
		Line: map[Orientation]string{
			Horizontal: "h-px flex-1 bg-border",
			Vertical:   "w-px flex-1 bg-border",
		},
		LineShort: map[Orientation]string{
			Horizontal: "h-px w-6 shrink-0 bg-border",
			Vertical:   "h-6 w-px shrink-0 bg-border",
		},
	}
}

// Separator is a thin line that divides content, shadcn/ui's Separator.
// By default it is decorative and hidden from assistive technologies, like
// Radix's decorative separator; Semantic gives it the separator role.
//
//	kit.Separator{}
//	h.Div(cls("flex h-5 items-center gap-4"), h.Text("Blog"), kit.Separator{Orientation: kit.Vertical}, h.Text("Docs"))
//
// A Caption puts a label in the middle of the line, as login forms do:
//
//	kit.Separator{Caption: h.Text("Or continue with")}
//
// CaptionAlign moves the caption to the start or the end of the line, after
// or before a short line, as a section heading does:
//
//	kit.Separator{Caption: h.Text("Yesterday"), CaptionAlign: kit.CaptionStart}
//
// The caption is text for the page, so a captioned separator is not hidden
// from assistive technologies, and has no separator role (a separator's
// children are presentational in ARIA, which would drop the text): the lines
// are decorative and the caption is read as it is.
type Separator struct {
	// Orientation is Horizontal (the default) or Vertical.
	Orientation Orientation
	// Caption is drawn in the middle of the line, between two lines that
	// fill the rest of the width (or the height of a Vertical separator).
	Caption tela.Component
	// CaptionAlign is CaptionCenter (the default), CaptionStart or CaptionEnd.
	CaptionAlign CaptionAlign
	// Semantic makes the separator part of the accessibility tree, with
	// the separator role and its orientation, for a line that divides
	// sections of the content and not just decorates it. It has no effect
	// with a Caption.
	Semantic bool
	// Class is added after the recipe's classes. CaptionClass is added to
	// the caption's.
	Class        string
	CaptionClass string
	// Props are more props for the separator element.
	Props tela.Child
}

// Build returns the separator element.
func (s Separator) Build() tela.Node {
	r := &UseStyle().Separator
	o := orientationOr(s.Orientation, Horizontal)
	if s.Caption != nil {
		line := func(short bool) tela.Child {
			lines := r.Line
			if short {
				lines = r.LineShort
			}
			return h.Div(cls(pick("Separator", "Orientation", lines, o)), h.AriaHidden(true))
		}
		if devMode() && s.CaptionAlign != CaptionCenter && s.CaptionAlign != CaptionStart && s.CaptionAlign != CaptionEnd {
			panic("kit: Separator.CaptionAlign " + string(s.CaptionAlign) + " is not kit.CaptionCenter, kit.CaptionStart or kit.CaptionEnd")
		}
		return h.Div(cls(pick("Separator", "Orientation", r.Captioned, o), s.Class), s.Props,
			line(s.CaptionAlign == CaptionStart), h.Div(cls(r.Caption, s.CaptionClass), s.Caption), line(s.CaptionAlign == CaptionEnd))
	}
	kids := []tela.Child{cls(r.Root, pick("Separator", "Orientation", r.Orientations, o), s.Class), s.Props}
	if s.Semantic {
		kids = append(kids, h.Role("separator"), h.AriaOrientation(string(o)))
	} else {
		kids = append(kids, h.AriaHidden(true))
	}
	return h.Div(kids...)
}
