package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/board/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Lane is one column of the board and a drop target. Its fields are
// comparable (the Column value, an array of IDs, included), so it is built
// again only when its cards change or something it reads does: while a card
// is dragged it reads Over, so it rebuilds as the pointer crosses cards and
// columns to show a ring and an insertion line (an absolutely positioned
// bar, so that nothing shifts under the pointer).
type Lane struct {
	Index    int
	Cards    Column
	Tasks    Tasks
	Full     [3]bool // which columns cannot take another card
	D        *DragState
	Set      tela.Setter[Board]
	SetTasks tela.Setter[Tasks]
}

func (p Lane) Build() tela.Node {
	dragging, over := p.D.Dragging.Get(), p.D.Over.Get()
	here := dragging != 0 && over.Valid && over.Col == p.Index

	surface := "bg-muted"
	if here {
		surface = "bg-accent ring-2 ring-primary/50"
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
			kit.Badge{Variant: kit.Outline, Class: "bg-background tabular-nums", Children: Textf("%d", len(ids))},
		),
		Div(Class("flex flex-1 flex-col gap-2"),
			If(len(ids) == 0, Div(Class("flex h-24 items-center justify-center rounded-xl border-2 border-dashed border-border text-sm text-muted-foreground"),
				Text("Drop cards here"))),
			MapKeyed(ids, func(id int) int { return id }, func(id int) tela.Component {
				line := LineNone
				switch {
				case here && dragging != id && over.Before == id:
					line = LineBefore
				case here && over.Before == 0 && id == ids[len(ids)-1] && dragging != id:
					line = LineAfter
				}
				return TaskCard{
					ID: id, Col: p.Index, Task: p.Tasks[id], Full: p.Full, Dragging: dragging == id, Line: line,
					D: p.D, Set: p.Set, SetTasks: p.SetTasks,
				}
			}),
		),
	)
}

// Line says where a card shows the insertion bar of the dragged card.
type Line uint8

const (
	LineNone   Line = iota
	LineBefore      // above the card
	LineAfter       // below the card
)

// insertion is the bar that shows where a dropped card will go.
func insertion(pos string) tela.Node {
	return Div(Class("pointer-events-none absolute inset-x-1 z-10 h-[3px] rounded-full bg-primary", pos))
}

// TaskCard is a draggable card: a kit.Card inside the element that carries
// the drag. Dragging it moves that element with TranslateFn reading Offset,
// so pointer moves repaint the card only. While it is dragged it is lifted
// (shadow, ring, a slight rotation, z-index) and pointer-events-none, so
// the pointer reaches the column and the card under it: those decide where
// it would land.
//
// The card's action is a DropdownMenu and its area a ContextMenu, which
// share the items of cardItems; Edit and Delete open the dialogs of
// actions.go, whose state lives here.
type TaskCard struct {
	ID, Col  int
	Task     Task
	Full     [3]bool
	Dragging bool
	Line     Line
	D        *DragState
	Set      tela.Setter[Board]
	SetTasks tela.Setter[Tasks]
}

func (p TaskCard) Build() tela.Node {
	task := p.Task
	toast := kit.UseToast()
	editing, setEditing := tela.UseState(false)
	deleting, setDeleting := tela.UseState(false)
	draft, setDraft := tela.UseState(task)

	items := p.items(toast, func() { setDraft.Set(task); setEditing.Set(true) }, func() { setDeleting.Set(true) })

	wrapper := []tela.Child{
		TestID("card"), Class("select-none"),
		If(p.Dragging, Class("pointer-events-none relative z-50 rotate-2")),
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
		wrapper = append(wrapper, Style(tela.TranslateFn(func() (x, y float32) {
			o := p.D.Offset.Get()
			return o.X, o.Y
		})))
	}

	look := "cursor-grab hover:border-ring/60 active:cursor-grabbing"
	if p.Dragging {
		look = "border-primary shadow-2xl ring-2 ring-ring/50"
	}
	wrapper = append(wrapper, kit.Card{
		Class:            "gap-3 py-4 " + look,
		HeaderClass:      "gap-1 px-4",
		TitleClass:       "text-sm leading-5 font-medium",
		DescriptionClass: "line-clamp-2 text-xs leading-4",
		ContentClass:     "px-4",
		Title:            H3(Text(task.Title)),
		Description:      Text(task.Note),
		Action: kit.DropdownMenu[string]{
			Trigger: kit.Button{
				Variant: kit.Ghost, Size: kit.IconSm, Icon: icons.Ellipsis,
				AriaLabel: "Actions for " + task.Title, Class: "-mt-1 -mr-2 size-7 text-muted-foreground",
			},
			Align: "end", Items: items,
		},
		Children: Div(Class("flex items-center justify-between"),
			Div(Class("flex items-center gap-3"),
				kit.Badge{Variant: kit.Outline, Children: Fragment(Span(Class("size-1.5 rounded-full", tagDots[task.Tag])), Text(task.Tag))},
				If(task.Comments > 0, Div(Class("flex items-center gap-1 text-xs text-muted-foreground"),
					Icon(icons.MessageSquare, Class("size-3.5")), Textf("%d", task.Comments))),
			),
			kit.Avatar{Size: kit.Sm, Fallback: task.Assignee, Alt: task.Assignee},
		),
	})

	return Fragment(kit.ContextMenu[string]{
		Class: "relative", AriaLabel: "Actions for " + task.Title, Items: items,
		Children: Fragment(
			If(p.Line == LineBefore, insertion("-top-[7px]")),
			Div(wrapper...),
			If(p.Line == LineAfter, insertion("-bottom-[7px]")),
			EditDialog{Open: editing, SetOpen: setEditing, Draft: draft, SetDraft: setDraft, ID: p.ID, SetTasks: p.SetTasks, Toast: toast},
			DeleteDialog{Open: deleting, SetOpen: setDeleting, ID: p.ID, Title: task.Title, Set: p.Set, Toast: toast},
		),
	})
}
