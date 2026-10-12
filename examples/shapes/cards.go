package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// ProfileCard: shadow-sm, a circular avatar (a clipped landscape with a
// ring), a name, a bio and two buttons.
func ProfileCard() tela.Node {
	return card("shadow-sm", "justify-between",
		Div(Class("flex items-center gap-3"),
			// The avatar clips its landscape with overflow-hidden.
			Div(Class("relative size-14 shrink-0 overflow-hidden rounded-full bg-linear-to-b from-sky-300 to-amber-200 ring-[3px] ring-indigo-500"),
				Div(Class("absolute top-2.5 right-3 size-4 rounded-full bg-amber-300")),
				Div(Class("absolute -bottom-4 -left-3 h-9 w-12 rounded-full bg-emerald-400")),
				Div(Class("absolute -right-3 -bottom-5 h-9 w-12 rounded-full bg-emerald-600")),
			),
			title("Ada Lovelace", "Engineer"),
		),
		P(Class("text-xs leading-4 text-slate-500"), Text("Builds tools that make complex things feel simple.")),
		Div(Class("flex gap-2"),
			Div(Class("rounded-full bg-indigo-500 px-4 py-1 text-xs leading-4 font-medium text-white"), Text("Follow")),
			Div(Class("rounded-full bg-slate-100 px-3 py-1 text-xs leading-4 font-medium text-slate-500"), Text("Message")),
		),
	)
}

// ChartCard: shadow-md, a title, a badge and a bar chart made of flex items
// whose heights are percentages.
func ChartCard() tela.Node {
	heights := []string{"h-[34%]", "h-[52%]", "h-[44%]", "h-[72%]", "h-[58%]", "h-full", "h-[66%]"}
	return card("shadow-md", "gap-3",
		Div(Class("flex items-start justify-between"),
			title("Revenue", "Last 7 days"),
			Div(Class("flex items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-xs leading-4 font-medium text-emerald-600"),
				Div(Class("size-1.5 rounded-full bg-emerald-500")),
				Text("+12%"),
			),
		),
		Div(Class("flex flex-1 items-end gap-2"),
			Map(heights, func(h string) tela.Node {
				cls := "flex-1 rounded-t-md bg-indigo-100 " + h
				if h == "h-full" {
					cls = "flex-1 rounded-t-md bg-linear-to-b from-indigo-400 to-indigo-600 h-full"
				}
				return Div(Key(h), Class(cls))
			}),
		),
	)
}

// PhotoCard: shadow-lg, a picture drawn with gradients (sky, sun and two
// waves, clipped by its rounded corners) and a round action button.
func PhotoCard() tela.Node {
	return Div(Class(cell+"flex flex-col justify-between rounded-2xl bg-white p-2.5 shadow-lg"),
		Div(Class("relative h-[88px] overflow-hidden rounded-xl bg-linear-to-b from-violet-400 to-pink-300"),
			Div(Class("absolute top-3 right-12 size-7 rounded-full bg-amber-200")),
			Div(Class("absolute -bottom-7 -left-6 h-14 w-[120%] rounded-[50%] bg-indigo-400")),
			Div(Class("absolute -right-8 -bottom-9 h-14 w-[120%] rounded-[50%] bg-indigo-600")),
		),
		Div(Class("flex items-end justify-between px-1.5 pb-1"),
			title("Aurora", "Photo, 24 MB"),
			arrowButton("size-9 bg-indigo-500 text-white"),
		),
	)
}

// arrowButton is a round button with an arrow.
func arrowButton(class string) tela.Node {
	return Div(Class("flex shrink-0 items-center justify-center rounded-full text-base font-semibold "+class), Text("→"))
}

// TogglesCard: shadow-xl, three switches: a rounded track with a white
// knob that has its own shadow.
func TogglesCard() tela.Node {
	return card("shadow-xl", "justify-around",
		toggle(true, "Notifications", "Push alerts"),
		toggle(false, "Dark mode", "Follow system"),
		toggle(true, "Autosave", "Every minute"),
	)
}

func toggle(on bool, label, hint string) tela.Node {
	track := "bg-slate-300 justify-start"
	if on {
		track = "bg-indigo-500 justify-end"
	}
	return Div(Class("flex items-center gap-3"),
		Div(Class("flex h-[26px] w-[46px] shrink-0 items-center rounded-full p-0.5 "+track),
			Div(Class("size-[22px] rounded-full bg-white shadow-md")),
		),
		title(label, hint),
	)
}

// ProgressCard: shadow-2xl, three labeled progress bars whose fills are
// gradients.
func ProgressCard() tela.Node {
	return card("shadow-2xl", "justify-around",
		progress("Design", "78%", "w-[78%]", "from-indigo-500 to-fuchsia-500"),
		progress("Research", "46%", "w-[46%]", "from-sky-400 to-emerald-400"),
		progress("Launch", "90%", "w-[90%]", "from-amber-400 to-rose-500"),
	)
}

func progress(label, value, width, colors string) tela.Node {
	return Div(Class("flex flex-col gap-1.5"),
		Div(Class("flex justify-between text-xs leading-4"),
			Span(Class("font-medium text-slate-700"), Text(label)),
			Span(Class("text-slate-400"), Text(value)),
		),
		Div(Class("h-2.5 rounded-full bg-slate-100"),
			Div(Class("h-full rounded-full bg-linear-to-r "+colors+" "+width)),
		),
	)
}

// SearchCard: an inset shadow and ring (a well) holding a search box and
// raised chips.
func SearchCard() tela.Node {
	type chip struct{ label, dot string }
	chips := []chip{{"UI", "bg-indigo-500"}, {"Go", "bg-emerald-500"}, {"Ops", "bg-rose-400"}, {"Docs", "bg-amber-400"}, {"AI", "bg-sky-500"}}
	return Div(Class(cell+"flex flex-col gap-3 rounded-2xl bg-slate-100 p-3.5 inset-shadow-sm inset-ring inset-ring-slate-200"),
		Div(Class("flex items-center gap-2 rounded-full bg-white px-3 py-2 shadow-xs"),
			magnifier(),
			Span(Class("text-xs leading-4 text-slate-400"), Text("Search")),
		),
		Div(Class("flex flex-wrap gap-2"),
			Map(chips, func(c chip) tela.Node {
				return Div(Key(c.label), Class("flex items-center gap-1.5 rounded-full bg-white py-1 pr-2.5 pl-2 text-xs leading-4 font-medium text-slate-600 shadow-sm"),
					Div(Class("size-2.5 rounded-full "+c.dot)),
					Text(c.label),
				)
			}),
		),
		P(Class("mt-auto text-xs leading-4 text-slate-400"), Text("5 filters")),
	)
}

// magnifier is a search icon made of a ring and a rotated handle.
func magnifier() tela.Node {
	return Div(Class("relative size-4 shrink-0"),
		Div(Class("absolute top-0 left-0 size-3 rounded-full border-2 border-slate-400")),
		Div(Class("absolute right-0 bottom-0.5 h-[7px] w-0.5 -rotate-45 rounded-full bg-slate-400")),
	)
}
