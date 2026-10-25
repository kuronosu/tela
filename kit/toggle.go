// Portions of this file follow shadcn/ui, toggle.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// ToggleStyle is the recipe of [Toggle] (shadcn/ui's toggleVariants), which
// the items of a [ToggleGroup] use too.
type ToggleStyle struct {
	// Root is what every toggle has: layout, text, hover, focus ring,
	// disabled and invalid. The unconditional classes (bg-transparent, the
	// border color) come before the conditional ones, as Tela's last-wins
	// order needs where CSS has the variant order.
	Root string
	// Variants are the looks: Default and Outline.
	Variants map[Variant]string
	// Sizes are the dimensions: the default, Sm and Lg.
	Sizes map[Size]string
	// On is added while the toggle is on (shadcn's
	// data-[state=on]:), after the variant, so that it wins over hover.
	// The toggle button is aria-pressed and the item of a single-selection
	// group aria-checked, so it styles both.
	On string
	// Icon is what the icon slot has (shadcn's [&_svg]: classes).
	Icon string
}

func init() {
	defaultStyle.Toggle = ToggleStyle{
		Root: "inline-flex items-center justify-center gap-2 rounded-md border-input bg-transparent text-sm font-medium whitespace-nowrap transition-[color,box-shadow] outline-none " +
			"hover:bg-muted hover:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 " +
			"disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
		Variants: map[Variant]string{
			Default: "",
			Outline: "border shadow-xs hover:bg-accent hover:text-accent-foreground",
		},
		Sizes: map[Size]string{
			"": "h-9 min-w-9 px-2",
			Sm: "h-8 min-w-8 px-1.5",
			Lg: "h-10 min-w-10 px-2.5",
		},
		On:   "aria-pressed:bg-accent aria-pressed:text-accent-foreground checked:bg-accent checked:text-accent-foreground",
		Icon: "pointer-events-none shrink-0 size-4",
	}
}

// Toggle is a button that stays pressed, shadcn/ui's Toggle, on
// aria.UseToggleButton: a click, Enter or Space turns it on and off, and
// it has aria-pressed. Its state follows the kit's rule: Pressed and
// OnPressedChange to keep it yourself, DefaultPressed to let the toggle
// keep it.
//
//	bold, setBold := tela.UseState(false)
//	kit.Toggle{Variant: kit.Outline, Pressed: &bold, OnPressedChange: setBold.Set,
//	    Icon: lucide.Bold, AriaLabel: "Bold"}
//
// For a row of toggles that share a choice, see [ToggleGroup]. A Variant or
// Size the active [Style] does not define panics in dev mode with the
// closest one.
type Toggle struct {
	// Variant is Default or Outline.
	Variant Variant
	// Size is Default, Sm or Lg.
	Size Size
	// Pressed points to the state of a controlled toggle; nil leaves it to
	// the toggle, which starts at DefaultPressed.
	Pressed *bool
	// DefaultPressed is the initial state of an uncontrolled toggle.
	DefaultPressed bool
	// OnPressedChange is called with the next state when the user toggles
	// the button. For a controlled toggle it is how the change is asked
	// for.
	OnPressedChange func(bool)
	// Disabled makes the toggle inert and dims it.
	Disabled bool
	// Invalid marks the value as wrong (aria-invalid).
	Invalid bool
	// AriaLabel names a toggle whose content is not text.
	AriaLabel string
	// Icon is drawn before the children.
	Icon tela.Icon
	// Children is the label.
	Children tela.Component
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the button element.
	Props tela.Child
}

// Build returns the toggle element.
func (t Toggle) Build() tela.Node {
	s := &UseStyle().Toggle
	tb := aria.UseToggleButton(aria.ToggleOptions{Selected: t.Pressed, DefaultSelected: t.DefaultPressed,
		OnChange: t.OnPressedChange, Disabled: t.Disabled})
	kids := []tela.Child{cls(s.Root, pick("Toggle", "Variant", s.Variants, t.Variant),
		pick("Toggle", "Size", s.Sizes, t.Size), s.On, t.Class), tb.Props, t.Props}
	if t.Invalid {
		kids = append(kids, h.AriaInvalid(true))
	}
	if t.AriaLabel != "" {
		kids = append(kids, h.AriaLabel(t.AriaLabel))
	}
	kids = append(kids, icon(t.Icon, s.Icon), t.Children)
	return h.Button(kids...)
}
