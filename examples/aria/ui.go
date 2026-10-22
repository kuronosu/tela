package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// The look is shadcn/ui's, written with the theme tokens (bg-background,
// text-muted-foreground, ring-ring...), so that one set of classes serves the
// light and the dark scheme. The behavior is not here: it comes from the
// aria hooks, which put their props next to these classes.

// focusRing is the ring of focus-visible:, which shows after the keyboard
// moves the focus and not after a click.
const focusRing = "outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"

// button is what every button shares; outline, primary and icon complete it.
const button = "inline-flex shrink-0 cursor-default items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-all disabled:pointer-events-none disabled:opacity-50"

const (
	outline = "h-9 border border-input bg-background px-4 shadow-xs hover:bg-accent hover:text-accent-foreground aria-expanded:bg-accent"
	primary = "h-9 bg-primary px-4 text-primary-foreground shadow-xs hover:bg-primary/90 active:bg-primary/80"
	iconBtn = "size-9 border border-input bg-background shadow-xs hover:bg-accent hover:text-accent-foreground"
)

// field is the look of a text field.
const field = "h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs transition-[color,box-shadow] placeholder:text-muted-foreground"

// surface is a floating surface: a menu, a list, a popover.
const surface = "z-50 overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md"

// pop is the entrance of a floating surface, which Exit(exitPop) undoes
// while it leaves: Presence keeps the element mounted until that ends.
const pop = "transition duration-150 ease-out starting:scale-95 starting:opacity-0"

// exitPop are the Exit classes of a floating surface.
const exitPop = "scale-95 opacity-0 duration-100"

// ic is an icon of the given size class, in the text color.
func ic(icon tela.Icon, class ...string) tela.Node {
	return Icon(icon, Class("shrink-0"), Class(class...))
}

// initials are the first letters of a name's words: "Mei Tanaka" is "MT".
func initials(name string) string {
	var out []rune
	start := true
	for _, r := range name {
		if r == ' ' {
			start = true
			continue
		}
		if start {
			out = append(out, r)
			start = false
		}
	}
	return string(out)
}
