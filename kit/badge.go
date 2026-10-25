package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// BadgeStyle is the recipe of [Badge] (shadcn/ui's badgeVariants).
type BadgeStyle struct {
	// Root is what every badge has.
	Root string
	// Variants are the looks: Default, Secondary, Destructive, Outline,
	// Ghost and Link.
	Variants map[Variant]string
	// Icon is what the Icon slot has.
	Icon string
}

func init() {
	defaultStyle.Badge = BadgeStyle{
		Root: "inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-full border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap " +
			"transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
		Variants: map[Variant]string{
			Default:     "bg-primary text-primary-foreground",
			Secondary:   "bg-secondary text-secondary-foreground",
			Destructive: "bg-destructive text-white focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40",
			Outline:     "border-border text-foreground",
			Ghost:       "",
			Link:        "text-primary underline-offset-4",
		},
		Icon: "pointer-events-none size-3",
	}
}

// Badge is a small label, shadcn/ui's Badge: a status, a count, a tag.
//
//	kit.Badge{Variant: kit.Secondary, Children: h.Text("Draft")}
//	kit.Badge{Variant: kit.Destructive, Icon: lucide.CircleAlert, Children: h.Text("Failed")}
//
// shadcn's hover styles for a badge that is a link ([a&]:hover:) are not
// part of the recipe: a Badge is not a link here; put it in an h.A or give
// it Props with an OnClick and a Class with hover: classes.
type Badge struct {
	// Variant is Default, Secondary, Destructive, Outline, Ghost or Link.
	Variant Variant
	// Icon is drawn before the children.
	Icon tela.Icon
	// Children is the label.
	Children tela.Component
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the badge element.
	Props tela.Child
}

// Build returns the badge element.
func (b Badge) Build() tela.Node {
	s := &UseStyle().Badge
	return h.Span(h.Class(s.Root, pick("Badge", "Variant", s.Variants, b.Variant), b.Class),
		b.Props, icon(b.Icon, s.Icon), b.Children)
}
