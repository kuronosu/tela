// Portions of this file follow shadcn/ui, button.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

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
	// Loading is added to a button that is Loading, after its variant and
	// size: it ignores the pointer.
	Loading string
	// LoadingIcon is added to the spinner, after Icon and IconSizes.
	LoadingIcon string
	// LoadingOverlay is the root's addition and LoadingSpinner the spinner
	// element's classes for a loading button that has no icon to turn into
	// the spinner: the content stays (LoadingContent hides it, so that the
	// button keeps its width) and the spinner is centered over it.
	LoadingOverlay, LoadingSpinner, LoadingContent string
}

func init() {
	defaultStyle.Button = ButtonStyle{
		Root: "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-all outline-none " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
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
		Loading:        "pointer-events-none",
		LoadingIcon:    "animate-spin",
		LoadingOverlay: "relative",
		LoadingSpinner: "absolute inset-0 flex items-center justify-center",
		LoadingContent: "contents text-transparent",
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
// Loading shows that the button's action is under way, as a button that
// submits a form does while the request is out. The button turns its Icon
// (or, with none, its IconEnd) into a spinner, or when it has neither draws
// a spinner over the content, which stays in place and invisible: either
// way the button keeps its width, so nothing around it moves. A loading
// button is marked aria-busy, ignores presses (the pointer and the keyboard,
// and it does not submit its form) and stays in the tab order, unlike a
// Disabled one, so the focus does not jump away from it.
//
//	kit.Button{Type: "submit", Loading: saving, Icon: lucide.Save, Children: h.Text("Save")}
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
	// Loading shows a spinner and ignores presses (see above).
	Loading bool
	// LoadingIcon replaces the spinner (a Lucide loader-circle that turns
	// with animate-spin).
	LoadingIcon tela.Icon
	// OnClick is called when the button is clicked or activated with the
	// keyboard.
	OnClick func()
	// AriaLabel names a button whose content is not text, such as an icon
	// button.
	AriaLabel string
	// Icon is drawn before the children, IconEnd after them. Their size
	// and shape come from the recipe's Icon and IconSizes.
	Icon, IconEnd tela.Icon
	// IconClass is added to the classes of both icons, after the recipe's.
	IconClass string
	// Children is the label.
	Children tela.Component
	// Class is added after the recipe's classes, so it wins over them.
	Class string
	// Props are more props for the button element: a Key, a TestID, a
	// handler.
	Props tela.Child

	// trigger holds the props an overlay gives a button used as its
	// trigger (WithTriggerProps); triggered says there are some.
	trigger   tela.Prop
	triggered bool
}

// WithTriggerProps makes Button a [Trigger]: the overlay that has it as its
// Trigger slot puts its props on the button element, after the button's
// own; the button's Disabled wins over theirs, so a disabled button opens
// nothing (the overlay's own Disabled does the same).
func (b Button) WithTriggerProps(p tela.Prop) tela.Component {
	b.trigger, b.triggered = p, true
	return b
}

// triggerBusy makes a loading button a busyTrigger: the overlay it opens
// disables its hook, so a press from the keyboard opens nothing either.
func (b Button) triggerBusy() bool { return b.Loading }

// Build returns the button element.
func (b Button) Build() tela.Node {
	s := &UseStyle().Button
	hasIcon := b.Icon != (tela.Icon{}) || b.IconEnd != (tela.Icon{})
	iconSize, ok := s.IconSizes[b.Size]
	if !ok {
		iconSize = s.IconSizes[""]
	}
	// A loading button turns its icon into the spinner; without one the
	// spinner goes over the content.
	lead, trail := icon(b.Icon, s.Icon, iconSize, b.IconClass), icon(b.IconEnd, s.Icon, iconSize, b.IconClass)
	var content tela.Child = b.Children
	var overlay tela.Child
	spinner := func() tela.Child {
		return h.Icon(iconOr(b.LoadingIcon, toastLoader), cls(s.Icon, iconSize, s.LoadingIcon))
	}
	overlayClass := ""
	if b.Loading {
		switch {
		case b.Icon != (tela.Icon{}):
			lead = spinner()
		case b.IconEnd != (tela.Icon{}):
			trail = spinner()
		default:
			overlayClass = s.LoadingOverlay
			content = h.Span(cls(s.LoadingContent), b.Children)
			overlay = h.Span(cls(s.LoadingSpinner), h.AriaHidden(true), spinner())
		}
	}
	kids := []tela.Child{cls(s.Root, pick("Button", "Variant", s.Variants, b.Variant),
		pick("Button", "Size", s.Sizes, b.Size), when(hasIcon, s.HasIcon[b.Size]), when(b.Loading, s.Loading), overlayClass, b.Class)}
	switch {
	case b.Loading:
		// Not a submit button while it is loading: a press does nothing.
		kids = append(kids, h.Type("button"), h.AriaBusy(true))
	case b.Type != "":
		kids = append(kids, h.Type(b.Type))
	}
	if b.OnClick != nil && !b.Loading {
		kids = append(kids, h.OnClick(b.OnClick))
	}
	if b.AriaLabel != "" {
		kids = append(kids, h.AriaLabel(b.AriaLabel))
	}
	kids = append(kids, b.Props, b.trigger)
	switch {
	case b.Disabled:
		// After the trigger props, whose button hook enables the element.
		kids = append(kids, h.Disabled(true))
	case b.Loading && b.triggered:
		// The overlay disabled its hook (triggerBusy), which disables the
		// element: a loading button stays focusable.
		kids = append(kids, h.Disabled(false))
	}
	kids = append(kids, lead, content, trail, overlay)
	return h.Button(kids...)
}
