package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Lane is one column of the board and a drop target. Its fields are
// comparable (the Column value, an array of IDs, included), so it is built
// again only when its cards change or something it reads does: while a card
// is dragged it reads Over, so it rebuilds as the pointer crosses cards and
// columns to show a ring and an insertion line (an absolutely positioned
// bar, so that nothing shifts under the pointer).
type Lane struct {
	Index int
	Cards Column
	D     *DragState
	Set   tela.Setter[Board]
}

func (p Lane) Build() tela.Node {
	dragging, over := p.D.Dragging.Get(), p.D.Over.Get()
	here := dragging != 0 && over.Valid && over.Col == p.Index

	surface := "bg-zinc-100 dark:bg-zinc-900"
	if here {
		surface = "bg-indigo-50 ring-2 ring-indigo-400/70 dark:bg-indigo-500/10 dark:ring-indigo-400/60"
	}
	ids := p.Cards.IDs()
	return Div(Class("flex min-w-0 flex-1 flex-col gap-3 rounded-2xl p-3", surface),
		// Entering the column (from outside) or leaving it keeps Over up to
		// date; the cards handle their own boundaries.
		OnMouseEnter(func() {
			if p.D.Active() {
				p.D.Over.Set(Target{Col: p.Index, Valid: true})
			}
		}),
		OnMouseLeave(func() {
			p.D.Over.Update(func(t Target) Target {
				if t.Col == p.Index {
					return Target{}
				}
				return t
			})
		}),
		Div(Class("flex items-center gap-2 px-1 pt-1 pb-1"),
			Div(Class("size-2 rounded-full", dots[p.Index])),
			H2(Class("text-sm font-semibold"), Text(columnNames[p.Index])),
			Div(Class("rounded-full bg-zinc-200 px-2 py-0.5 text-xs font-medium text-zinc-600 tabular-nums dark:bg-zinc-800 dark:text-zinc-400"),
				Textf("%d", len(ids))),
		),
		Div(Class("flex flex-1 flex-col gap-2"),
			If(len(ids) == 0, Div(Class("flex h-24 items-center justify-center rounded-xl border-2 border-dashed border-zinc-300 text-sm text-zinc-400 dark:border-zinc-700"),
				Text("Drop cards here"))),
			Map(ids, func(id int) tela.Node {
				line := here && dragging != id && over.Before == id
				end := here && over.Before == 0 && id == ids[len(ids)-1] && dragging != id
				return Div(Key(id), Class("relative"),
					If(line, insertion("-top-[7px]")),
					TaskCard{ID: id, Col: p.Index, Dragging: dragging == id, D: p.D, Set: p.Set},
					If(end, insertion("-bottom-[7px]")),
				)
			}),
		),
	)
}

// insertion is the bar that shows where a dropped card will go.
func insertion(pos string) tela.Node {
	return Div(Class("pointer-events-none absolute inset-x-1 z-10 h-[3px] rounded-full bg-indigo-500", pos))
}

// TaskCard is a draggable card. Dragging it moves it with TranslateFn
// reading Offset, so pointer moves repaint the card only. While it is
// dragged it is lifted (shadow, ring, a slight rotation, z-index) and
// pointer-events-none, so the pointer reaches the column and the card
// under it: those decide where it would land.
type TaskCard struct {
	ID, Col  int
	Dragging bool
	D        *DragState
	Set      tela.Setter[Board]
}

func (p TaskCard) Build() tela.Node {
	task := tasks[p.ID]
	props := []tela.Child{
		Class("flex flex-col gap-3 rounded-xl border p-3.5 select-none"),
		If(p.Dragging, Class("pointer-events-none relative z-50 rotate-2 border-indigo-400 bg-white shadow-2xl ring-2 ring-indigo-500/40 dark:border-indigo-400 dark:bg-zinc-900")),
		If(!p.Dragging, Class("cursor-grab border-zinc-200 bg-white shadow-xs hover:border-zinc-300 hover:shadow-sm active:cursor-grabbing",
			"dark:border-zinc-800 dark:bg-zinc-950 dark:hover:border-zinc-700")),
		OnDragStart(func() { p.D.Start(p.ID, p.Col) }),
		OnDrag(func(e tela.DragEvent) { p.D.Offset.Set(Offset{e.OffsetX, e.OffsetY}) }),
		OnDragEnd(func() { p.D.Drop(p.ID, p.Set) }),
		// Moving between cards while dragging retargets the drop.
		OnMouseEnter(func() {
			if p.D.Active() {
				p.D.Over.Set(Target{Col: p.Col, Before: p.ID, Valid: true})
			}
		}),
		OnMouseLeave(func() {
			if p.D.Active() {
				p.D.Over.Set(Target{Col: p.Col, Valid: true})
			}
		}),
	}
	if p.Dragging {
		props = append(props, Style(tela.TranslateFn(func() (x, y float32) {
			o := p.D.Offset.Get()
			return o.X, o.Y
		})))
	}
	props = append(props,
		Div(Class("flex flex-col gap-1"),
			P(Class("text-sm leading-5 font-medium"), Text(task.Title)),
			P(Class("line-clamp-2 text-xs leading-4 text-zinc-500 dark:text-zinc-400"), Text(task.Note)),
		),
		Div(Class("flex items-center justify-between"),
			Div(Class("flex items-center gap-3"),
				Span(Class("rounded-md px-2 py-0.5 text-xs font-medium", task.Colors), Text(task.Tag)),
				If(task.Comments > 0, comments(task.Comments)),
			),
			Div(Class("flex size-6 items-center justify-center rounded-full bg-zinc-100 text-[10px] font-semibold text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300"),
				Text(task.Assignee)),
		),
	)
	return Div(props...)
}

// comments is a speech bubble (a rounded box with a tail) and a count.
func comments(n int) tela.Node {
	return Div(Class("flex items-center gap-1.5 text-xs text-zinc-400"),
		Div(Class("relative h-3 w-3.5 rounded-[4px] border-[1.5px] border-current"),
			Div(Class("absolute -bottom-[3.5px] left-[2px] size-1.5 rotate-45 border-r-[1.5px] border-b-[1.5px] border-current bg-white dark:bg-zinc-950"))),
		Textf("%d", n),
	)
}
