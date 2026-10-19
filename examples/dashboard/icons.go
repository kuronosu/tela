package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// The embedded fonts have no icon glyphs, so the icons are drawn with a few
// boxes on a 16px canvas, in the current text color (bg-current,
// border-current): they follow the color of the button they sit in.

// icon draws the icon called name, 16px square.
func icon(name string) tela.Node {
	switch name {
	case "grid":
		return Div(Class("grid size-4 shrink-0 grid-cols-2 gap-[2px]"),
			Div(Class("rounded-[3px] bg-current")), Div(Class("rounded-[3px] bg-current opacity-60")),
			Div(Class("rounded-[3px] bg-current opacity-60")), Div(Class("rounded-[3px] bg-current")))
	case "chart":
		return Div(Class("flex size-4 shrink-0 items-end gap-[2px]"),
			Div(Class("h-2 w-[3px] rounded-full bg-current")), Div(Class("h-4 w-[3px] rounded-full bg-current")),
			Div(Class("h-3 w-[3px] rounded-full bg-current")), Div(Class("h-[10px] w-[3px] rounded-full bg-current")))
	case "users":
		return Div(Class("relative size-4 shrink-0"),
			Div(Class("absolute top-0 left-1 size-2 rounded-full bg-current")),
			Div(Class("absolute bottom-0 left-0 h-[6px] w-4 rounded-t-full bg-current opacity-70")))
	case "box":
		return Div(Class("relative size-4 shrink-0 rounded-[4px] border-[1.5px] border-current"),
			Div(Class("absolute inset-x-0 top-[3px] h-[1.5px] bg-current")),
			Div(Class("absolute top-[3px] left-[5px] h-[8px] w-[1.5px] bg-current")))
	case "chat":
		return Div(Class("relative size-4 shrink-0 rounded-[5px] rounded-bl-[1px] border-[1.5px] border-current"),
			Div(Class("absolute top-[3px] left-[2px] h-[1.5px] w-[7px] rounded-full bg-current")),
			Div(Class("absolute top-[7px] left-[2px] h-[1.5px] w-[4px] rounded-full bg-current")))
	case "sliders":
		return Div(Class("flex size-4 shrink-0 flex-col justify-between py-[1px]"),
			Div(Class("relative h-[1.5px] bg-current"), Div(Class("absolute -top-[2px] left-[3px] size-[5px] rounded-full bg-current"))),
			Div(Class("relative h-[1.5px] bg-current"), Div(Class("absolute -top-[2px] right-[3px] size-[5px] rounded-full bg-current"))),
			Div(Class("relative h-[1.5px] bg-current"), Div(Class("absolute -top-[2px] left-[7px] size-[5px] rounded-full bg-current"))))
	case "bell":
		return Div(Class("relative size-4 shrink-0"),
			Div(Class("absolute top-[1px] left-[2px] h-[11px] w-3 rounded-t-full rounded-b-[2px] border-[1.5px] border-current")),
			Div(Class("absolute bottom-0 left-[6px] h-[3px] w-1 rounded-b-full bg-current")))
	case "search":
		return Div(Class("relative size-4 shrink-0"),
			Div(Class("absolute top-0 left-0 size-3 rounded-full border-[1.5px] border-current")),
			Div(Class("absolute right-0 bottom-[1px] h-[7px] w-[1.5px] -rotate-45 rounded-full bg-current")))
	case "plus":
		return Div(Class("relative size-4 shrink-0"),
			Div(Class("absolute inset-x-[2px] top-[7px] h-[2px] rounded-full bg-current")),
			Div(Class("absolute inset-y-[2px] left-[7px] w-[2px] rounded-full bg-current")))
	case "close":
		return Div(Class("relative size-4 shrink-0"),
			Div(Class("absolute inset-x-[2px] top-[7px] h-[2px] rotate-45 rounded-full bg-current")),
			Div(Class("absolute inset-x-[2px] top-[7px] h-[2px] -rotate-45 rounded-full bg-current")))
	case "arrow-up":
		return Div(Class("relative size-3 shrink-0"),
			Div(Class("absolute top-[1px] left-[5px] h-[10px] w-[2px] rounded-full bg-current")),
			Div(Class("absolute top-[1px] left-[2px] h-[6px] w-[6px] rotate-45 border-t-2 border-l-2 border-current")))
	case "arrow-down":
		return Div(Class("relative size-3 shrink-0"),
			Div(Class("absolute top-[1px] left-[5px] h-[10px] w-[2px] rounded-full bg-current")),
			Div(Class("absolute bottom-[2px] left-[3px] h-[6px] w-[6px] rotate-45 border-r-2 border-b-2 border-current")))
	case "check":
		return Div(Class("relative size-4 shrink-0"),
			Div(Class("absolute top-[3px] left-[5px] h-[9px] w-[5px] rotate-45 border-r-2 border-b-2 border-current")))
	}
	panic("dashboard: no icon called " + name)
}
