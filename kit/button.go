package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// ButtonStyle is the recipe of [Button] (shadcn/ui's buttonVariants).
type ButtonStyle struct {
	// Root is what every button has: layout, text, focus ring, disabled.
	Root string
	// Variants are the looks: Default, Secondary, Outline, Ghost,
	// Destructive and Link.
	Variants map[Variant]string
	// Sizes are the dimensions: the default, Xs, Sm, Lg, Icon, IconXs,
	// IconSm and IconLg.
	Sizes map[Size]string
	// HasIcon are the classes a size adds when the button has an Icon or
	// an IconEnd (shadcn's has-[>svg]: padding), after Sizes.
	HasIcon map[Size]string
	// Icon is what the icon slots have, before IconSizes.
	Icon string
	// IconSizes are the icon's size per button size; a size the map does
	// not define uses the default's.
	IconSizes map[Size]string
}

func init() {
	defaultStyle.Button = ButtonStyle{
		Root: "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-all outline-none " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50",
		Variants: map[Variant]string{
			Default:     "bg-primary text-primary-foreground hover:bg-primary/90",
			Destructive: "bg-destructive text-white hover:bg-destructive/90 focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40",
			Outline:     "border border-border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50",
			Secondary:   "bg-secondary text-secondary-foreground hover:bg-secondary/80",
			Ghost:       "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50",
			Link:        "text-primary underline-offset-4 hover:underline",
		},
		Sizes: map[Size]string{
			"":     "h-9 px-4 py-2",
			Xs:     "h-6 gap-1 rounded-md px-2 text-xs",
			Sm:     "h-8 gap-1.5 rounded-md px-3",
			Lg:     "h-10 rounded-md px-6",
			Icon:   "size-9",
			IconXs: "size-6 rounded-md",
			IconSm: "size-8",
			IconLg: "size-10",
		},
		HasIcon: map[Size]string{
			"": "px-3",
			Xs: "px-1.5",
			Sm: "px-2.5",
			Lg: "px-4",
		},
		Icon: "pointer-events-none shrink-0",
		IconSizes: map[Size]string{
			"":     "size-4",
			Xs:     "size-3",
			IconXs: "size-3",
		},
	}
}

// Button is a button, shadcn/ui's Button: the rounded, medium-weight control
// in six variants and eight sizes. It is an h.Button, so it takes focus
// with Tab, is activated with Enter and Space, shows hover and active
// states, and inside an h.Form submits it (Type "submit" is the HTML
// default there).
//
//	kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: save, Children: h.Text("Save")}
//
//	// An icon before the label (your own tela.Icon), and an icon-only button.
//	kit.Button{Icon: lucide.Mail, Children: h.Text("Login with email")}
//	kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: lucide.ChevronRight, AriaLabel: "Next"}
//
// A Variant or Size the active [Style] does not define panics in dev mode
// with the closest one.
type Button struct {
	// Variant is the look: Default, Secondary, Outline, Ghost, Destructive
	// or Link.
	Variant Variant
	// Size is Default, Xs, Sm, Lg, Icon, IconXs, IconSm or IconLg.
	Size Size
	// Type is "button", "submit" or "reset" (h.Type). Empty leaves the
	// element's default: "submit" inside an h.Form, else "button".
	Type string
	// Disabled makes the button inert (h.Disabled) and dims it.
	Disabled bool
	// OnClick is called when the button is clicked or activated with the
	// keyboard.
	OnClick func()
	// AriaLabel names a button whose content is not text, such as an icon
	// button.
	AriaLabel string
	// Icon is drawn before the children, IconEnd after them. Their size
	// and shape come from the recipe's Icon and IconSizes.
	Icon, IconEnd tela.Icon
	// Children is the label.
	Children tela.Component
	// Class is added after the recipe's classes, so it wins over them.
	Class string
	// Props are more props for the button element: a Key, a TestID, a
	// handler.
	Props tela.Child
}

// Build returns the button element.
func (b Button) Build() tela.Node {
	s := &UseStyle().Button
	hasIcon := b.Icon != (tela.Icon{}) || b.IconEnd != (tela.Icon{})
	iconSize, ok := s.IconSizes[b.Size]
	if !ok {
		iconSize = s.IconSizes[""]
	}
	kids := []tela.Child{h.Class(s.Root, pick("Button", "Variant", s.Variants, b.Variant),
		pick("Button", "Size", s.Sizes, b.Size), when(hasIcon, s.HasIcon[b.Size]), b.Class)}
	if b.Type != "" {
		kids = append(kids, h.Type(b.Type))
	}
	if b.Disabled {
		kids = append(kids, h.Disabled(true))
	}
	if b.OnClick != nil {
		kids = append(kids, h.OnClick(b.OnClick))
	}
	if b.AriaLabel != "" {
		kids = append(kids, h.AriaLabel(b.AriaLabel))
	}
	kids = append(kids, b.Props, icon(b.Icon, s.Icon, iconSize), b.Children, icon(b.IconEnd, s.Icon, iconSize))
	return h.Button(kids...)
}
