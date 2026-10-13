package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// barW is the width of the progress bar in pixels: the card's content
// width. The seek handlers divide the pointer position by it.
const barW = 332

// TransportProps are the props of the components that show or drive the
// player. *Transport is a pointer: comparable and stable.
type TransportProps struct{ Tr *Transport }

// NowPlaying is the player card: cover, title, progress and controls. It
// reads Track in its body, so a new track rebuilds it; the progress bar
// below it does not.
func NowPlaying(p TransportProps) tela.Node {
	track := p.Tr.Current()
	return Div(Class("flex w-[380px] shrink-0 flex-col gap-5 rounded-3xl border border-white/10 bg-zinc-900/80 p-6 shadow-2xl"),
		Cover(track, false, "mx-auto size-[clamp(96px,calc(100vh-310px),332px)] rounded-2xl shadow-lg"),
		Div(Class("flex flex-col gap-1"),
			Div(Class("text-xs font-medium tracking-widest text-violet-300 uppercase"), Text("Now playing")),
			H2(Class("truncate text-2xl font-semibold tracking-tight"), Text(track.Title)),
			P(Class("truncate text-sm text-zinc-400"), Text(track.Artist)),
		),
		tela.CP(Progress, TransportProps{Tr: p.Tr}),
		tela.CP(Controls, TransportProps{Tr: p.Tr}),
	)
}

// Cover is the cover art: a gradient with a few shapes (only one in the
// small covers of the queue). class sets its size.
func Cover(t Track, small bool, class string) tela.Node {
	box := "relative aspect-square shrink-0 overflow-hidden bg-linear-to-br " + t.From + " " + t.To + " " + class
	if small {
		return Div(Class(box),
			Div(Class("absolute -top-3 -right-3 size-8 rounded-full bg-white/25")),
			Div(Class("absolute -bottom-4 -left-2 size-9 rounded-full bg-white/10")),
		)
	}
	return Div(Class(box),
		Div(Class("absolute -top-12 -right-10 size-48 rounded-full bg-white/20")),
		Div(Class("absolute right-10 bottom-10 size-24 rounded-full bg-black/20")),
		Div(Class("absolute -bottom-16 -left-8 size-56 rounded-full bg-white/10")),
		Div(Class("absolute bottom-6 left-6 size-12 rounded-full border-4 border-white/70")),
	)
}

// Progress is the seek bar with the elapsed and total time. The fill and
// the thumb follow Elapsed through WidthFn and TranslateFn, so a tick lays
// out the fill and repaints the thumb without building anything; only
// TimeLabel, which reads Secs, builds (once per second).
func Progress(p TransportProps) tela.Node {
	tr := p.Tr
	total := float64(tr.Current().Seconds)
	seek := func(x float32) { tr.Seek(float64(x) / barW) }
	frac := func() float64 { return tr.Elapsed.Get() / total }

	return Div(Class("flex flex-col gap-2"),
		// The bar is taller than it looks, so it is easy to hit.
		Div(Class("group flex h-4 w-[332px] cursor-pointer items-center"),
			OnClick(func(e tela.MouseEvent) { seek(e.LocalX) }),
			OnDragStart(func(e tela.DragEvent) { seek(e.LocalX) }),
			OnDrag(func(e tela.DragEvent) { seek(e.LocalX) }),
			Div(Class("relative h-1.5 w-full rounded-full bg-white/15 group-hover:h-2"),
				Div(Class("h-full rounded-full bg-white group-hover:bg-violet-300"),
					Style(tela.WidthFn(func() float32 { return float32(frac() * barW) }))),
				Div(Class("absolute top-1/2 left-0 -mt-[7px] size-3.5 rounded-full bg-white opacity-0 shadow-md group-hover:opacity-100"),
					Style(tela.TranslateFn(func() (x, y float32) { return float32(frac()*barW) - 7, 0 }))),
			),
		),
		Div(Class("flex justify-between text-xs text-zinc-400 tabular-nums"),
			tela.CP(TimeLabel, TransportProps{Tr: tr}),
			Span(Text(clock(int(total)))),
		),
	)
}

// TimeLabel shows the elapsed time. It reads Secs in its body, so it
// builds when the second changes.
func TimeLabel(p TransportProps) tela.Node {
	return Span(Text(clock(p.Tr.Secs.Get())))
}

// clock formats seconds as m:ss.
func clock(s int) string { return fmt.Sprintf("%d:%02d", s/60, s%60) }

// Controls are the previous, play/pause and next buttons. It reads
// Playing, so a toggle rebuilds only the controls.
func Controls(p TransportProps) tela.Node {
	tr := p.Tr
	playing := tr.Playing.Get()
	return Div(Class("flex items-center justify-center gap-6"),
		Button(Class("flex size-12 items-center justify-center rounded-full text-zinc-300 hover:bg-white/10 hover:text-white active:bg-white/20"),
			OnClick(func(tela.MouseEvent) { tr.Prev() }),
			Div(Class("flex items-center gap-px"), Div(Class("h-4 w-[3px] rounded-full bg-current")), triangle(16, true)),
		),
		Button(Class("flex size-16 items-center justify-center rounded-full bg-white text-zinc-900 shadow-lg hover:scale-105 hover:bg-zinc-100 active:scale-95"),
			OnClick(func(tela.MouseEvent) { tr.Toggle() }),
			If(playing, Div(Class("flex gap-1.5"),
				Div(Class("h-5 w-1.5 rounded-sm bg-current")),
				Div(Class("h-5 w-1.5 rounded-sm bg-current")))),
			If(!playing, Div(Class("ml-1"), triangle(22, false))),
		),
		Button(Class("flex size-12 items-center justify-center rounded-full text-zinc-300 hover:bg-white/10 hover:text-white active:bg-white/20"),
			OnClick(func(tela.MouseEvent) { tr.Next() }),
			Div(Class("flex items-center gap-px"), triangle(16, false), Div(Class("h-4 w-[3px] rounded-full bg-current"))),
		),
	)
}

// triangle is a play triangle of height h pointing right, or left: half of
// a square rotated by 45 degrees, centered on one edge of a box that clips
// the other half. (A zero-size box with colored side borders would draw a
// square: borders have no mitered joins yet.)
func triangle(h float32, left bool) tela.Node {
	d := h / 2          // width of the clip box and half of the diagonal
	s := d * 1.41421356 // side of the square
	pos := fmt.Sprintf("-left-[%.2fpx]", s/2)
	if left {
		pos = fmt.Sprintf("-right-[%.2fpx]", s/2)
	}
	return Div(Class(fmt.Sprintf("relative h-[%gpx] w-[%gpx] shrink-0 overflow-hidden", h, d)),
		Div(Class(fmt.Sprintf("absolute top-[%.2fpx] size-[%.2fpx] rotate-45 rounded-[2px] bg-current %s", d-s/2, s, pos))),
	)
}

// QueueProps are the props of Queue.
type QueueProps struct{ Tr *Transport }

// Queue is the list of tracks. It reads Track and Playing in its body to
// mark the current row.
func Queue(p QueueProps) tela.Node {
	current, playing := p.Tr.Track.Get(), p.Tr.Playing.Get()
	return Div(Class("hidden w-[420px] shrink-0 flex-col gap-1 rounded-3xl border border-white/10 bg-zinc-900/60 p-3 lg:flex"),
		Div(Class("flex items-baseline justify-between px-3 pt-2 pb-3"),
			H2(Class("text-lg font-semibold tracking-tight"), Text("Up next")),
			Span(Class("text-xs text-zinc-500"), Textf("%d tracks", len(tracks))),
		),
		Map(tracks, func(t Track) tela.Node {
			i := indexOf(t)
			return tela.WithKey(tela.CP(QueueRow, RowProps{Tr: p.Tr, Index: i, Current: i == current, Playing: playing && i == current}), t.Title)
		}),
	)
}

func indexOf(t Track) int {
	for i, c := range tracks {
		if c == t {
			return i
		}
	}
	return -1
}

// RowProps are the props of QueueRow.
type RowProps struct {
	Tr               *Transport
	Index            int
	Current, Playing bool
}

// QueueRow is one track: a small cover, title, artist and length. The
// current row is highlighted; clicking a row plays it.
func QueueRow(p RowProps) tela.Node {
	t := tracks[p.Index]
	cls := "hover:bg-white/5 active:bg-white/10"
	if p.Current {
		cls = "bg-white/10"
	}
	return Button(Class("flex items-center gap-3 rounded-2xl p-2 pr-4 text-left transition-colors "+cls),
		OnClick(func(tela.MouseEvent) { p.Tr.Select(p.Index) }),
		Div(Class("flex w-5 shrink-0 justify-center text-sm text-zinc-500 tabular-nums"),
			If(!p.Playing, Textf("%d", p.Index+1)),
			If(p.Playing, equalizer()),
		),
		Cover(t, true, "size-11 rounded-xl"),
		Div(Class("flex min-w-0 flex-1 flex-col"),
			Span(Class("truncate text-sm font-medium"), Text(t.Title)),
			Span(Class("truncate text-sm text-zinc-400"), Text(t.Artist)),
		),
		Span(Class("text-sm text-zinc-500 tabular-nums"), Text(clock(t.Seconds))),
	)
}

// equalizer is three bars of different heights, the "playing" marker.
func equalizer() tela.Node {
	return Div(Class("flex h-4 items-end gap-[2px] text-violet-400"),
		Div(Class("h-2 w-[3px] rounded-full bg-current")),
		Div(Class("h-4 w-[3px] rounded-full bg-current")),
		Div(Class("h-3 w-[3px] rounded-full bg-current")),
	)
}
