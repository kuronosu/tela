package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// GradientCard: bg-linear-to-br with a colored shadow, two translucent
// decorative circles clipped by the card, a badge and a round button.
func GradientCard() tela.Node {
	return Div(Class(cell+"relative flex flex-col justify-between overflow-hidden rounded-3xl bg-linear-to-br from-indigo-500 to-fuchsia-600 p-5 shadow-lg shadow-indigo-500/50"),
		Div(Class("absolute -top-14 left-[55%] size-32 rounded-full bg-white/12")),
		Div(Class("absolute -bottom-12 -left-10 size-36 rounded-full bg-white/10")),
		Div(Class("relative flex w-fit items-center gap-1.5 rounded-full border border-white/35 bg-white/15 py-1 pr-2.5 pl-2 text-xs leading-4 font-medium text-white"),
			Div(Class("size-1.5 rounded-full bg-white")),
			Text("New"),
		),
		Div(Class("relative flex items-end justify-between"),
			Div(
				P(Class("text-lg leading-6 font-bold text-white"), Text("Ship faster")),
				P(Class("text-xs text-white/70"), Text("Deploy in one click")),
			),
			arrowButton("size-9 bg-white/95 text-indigo-600"),
		),
	)
}

// SunsetCard: a radial gradient with a bright spot, and a frosted pill at
// the bottom that blurs what is behind it (backdrop-blur).
func SunsetCard() tela.Node {
	return Div(Class(cell+"relative flex flex-col justify-end overflow-hidden rounded-2xl bg-radial-[at_30%_28%] from-amber-200 via-rose-400 via-48% to-violet-600 p-3.5 shadow-xl shadow-rose-500/35"),
		Div(Class("absolute top-0 left-3 size-24 bg-radial-[circle_closest-side] from-white/85 to-white/0")),
		Div(Class("relative flex items-center gap-2.5 rounded-full border border-white/40 bg-white/20 px-2.5 py-2 backdrop-blur"),
			Div(Class("size-4 shrink-0 rounded-full bg-white")),
			Div(
				P(Class("text-xs leading-4 font-semibold text-white"), Text("Golden hour")),
				P(Class("text-[10px] leading-3 text-white/70"), Text("6:42 PM")),
			),
		),
	)
}

// ConicCard: a ring painted with a conic gradient (bg-conic on a circle with
// a white circle inside), with a knob at its start.
func ConicCard() tela.Node {
	return card("shadow-md", "items-center justify-center",
		Div(Class("relative flex size-28 items-center justify-center rounded-full bg-[conic-gradient(#f43f5e,#fbbf24,#34d399,#0ea5e9,#6366f1,#d946ef,#f43f5e)]"),
			Div(Class("flex size-[72px] flex-col items-center justify-center rounded-full bg-white"),
				Span(Class("text-lg leading-6 font-bold text-slate-800"), Text("72%")),
				Span(Class("text-[10px] leading-3 text-slate-400"), Text("of goal")),
			),
			Div(Class("absolute -top-1 left-1/2 flex size-[18px] -translate-x-1/2 items-center justify-center rounded-full bg-white shadow-md"),
				Div(Class("size-2 rounded-full bg-rose-500")),
			),
		),
	)
}

// GlassCard: blurred color discs on a dark card, and a pane of frosted glass
// (backdrop-blur, translucent fill, hairline border) over them.
func GlassCard() tela.Node {
	return Div(Class(cell+"relative overflow-hidden rounded-[20px] bg-linear-to-br from-slate-900 to-slate-950 shadow-xl"),
		Div(Class("absolute top-2 left-2 size-24 rounded-full bg-indigo-500 blur-xl")),
		Div(Class("absolute top-0 right-3 size-20 rounded-full bg-fuchsia-500 blur-xl")),
		Div(Class("absolute -bottom-5 left-14 size-24 rounded-full bg-sky-500 blur-xl")),
		Div(Class("absolute bottom-2 left-0 size-12 rounded-full bg-amber-400 blur-xl")),
		Div(Class("absolute inset-x-[18px] top-[34px] bottom-[34px] flex flex-col gap-2.5 rounded-2xl border border-white/30 bg-white/15 p-3.5 shadow-lg backdrop-blur-xl"),
			Div(Class("flex items-center gap-2.5"),
				Div(Class("size-6 shrink-0 rounded-full bg-white/90")),
				Div(
					P(Class("text-xs leading-4 font-semibold text-white"), Text("Frosted glass")),
					P(Class("text-[10px] leading-3 text-white/60"), Text("backdrop-blur-xl")),
				),
			),
			P(Class("text-[11px] leading-4 text-white/70"), Text("Blurred color behind a hairline border.")),
		),
	)
}

// ShapesCard: shapes that need no paths. A burst (two rounded squares, one
// rotated) with a colored drop shadow, and a blob (a different radius on
// each corner) with a colored shadow.
func ShapesCard() tela.Node {
	return card("shadow-md", "justify-between",
		Div(Class("flex flex-1 items-center justify-around"),
			Div(Class("relative size-[76px] drop-shadow-lg drop-shadow-orange-400/60"),
				Div(Class("absolute inset-[9px] rounded-lg bg-linear-to-br from-amber-300 to-orange-400")),
				Div(Class("absolute inset-[9px] rotate-45 rounded-lg bg-linear-to-br from-amber-300 to-orange-400")),
			),
			Div(Class("size-[76px] rounded-tl-[70%] rounded-tr-[40%] rounded-br-[65%] rounded-bl-[45%] bg-linear-to-br from-emerald-400 to-sky-500 shadow-lg shadow-sky-500/50")),
		),
		Div(Class("flex justify-between text-xs leading-4 text-slate-400"),
			Span(Text("Burst")),
			Span(Text("Blob")),
		),
	)
}

// DropCard: a dashed border around a drop zone. The two wavy strokes of the
// phase-1 scene become a waveform of rounded bars and a zigzag of rotated
// ones (pills of one width, tilted alternately, whose round ends meet).
func DropCard() tela.Node {
	type bar struct {
		i        int
		h, color string
	}
	wave := []bar{
		{0, "h-3", "bg-sky-400"}, {1, "h-5", "bg-sky-400"}, {2, "h-8", "bg-sky-500"}, {3, "h-10", "bg-blue-500"},
		{4, "h-8", "bg-blue-500"}, {5, "h-5", "bg-indigo-500"}, {6, "h-3", "bg-indigo-500"}, {7, "h-5", "bg-violet-500"},
		{8, "h-8", "bg-violet-500"}, {9, "h-10", "bg-purple-500"}, {10, "h-8", "bg-purple-500"}, {11, "h-5", "bg-fuchsia-500"},
		{12, "h-3", "bg-fuchsia-500"},
	}
	type stroke struct {
		i    int
		tilt string
	}
	var zigzag []stroke
	for i := range 10 {
		tilt := "rotate-35"
		if i%2 == 1 {
			tilt = "-rotate-35"
		}
		zigzag = append(zigzag, stroke{i, tilt})
	}
	return Div(Class(cell+"flex flex-col justify-around rounded-2xl border-2 border-dashed border-indigo-300 bg-white/60 px-5 py-4"),
		Div(Class("flex items-center justify-between"),
			Map(wave, func(b bar) tela.Node {
				return Div(Key(b.i), Class("w-1.5 rounded-full "+b.h+" "+b.color))
			}),
		),
		Div(Class("flex items-center justify-center"),
			Map(zigzag, func(s stroke) tela.Node {
				return Div(Key(s.i), Class("flex w-[15px] justify-center"),
					Div(Class("h-[26px] w-[5px] rounded-full bg-emerald-500 "+s.tilt)))
			}),
		),
		P(Class("text-center text-xs leading-4 text-indigo-400"), Text("Drop files here")),
	)
}
