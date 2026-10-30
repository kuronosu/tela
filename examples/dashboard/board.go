package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The board view of the tasks: three columns of cards that can be dragged
// to another column, dropped between two cards or reordered in their
// column, and moved, edited or deleted from a menu (the card's ellipsis, a
// kit.DropdownMenu, and the card's area, a kit.ContextMenu), which works
// from the keyboard too.
//
// Dragging uses OnDragStart, OnDrag and OnDragEnd. The dragged card
// follows the pointer with tela.TranslateFn reading a tela.UseValue, so a
// pointer move only repaints that card; the column under the pointer and
// the card it hovers set the drop target (DragState.Over) from
// OnMouseEnter and OnMouseLeave, and the board, a comparable array of task
// IDs, changes when the pointer is released.

// Offset is how far the dragged card is from where it started.
type Offset struct{ X, Y float32 }

// DragState is the drag in progress, made of Values so that each piece
// costs only what reads it: Offset changes on every pointer move and is
// read only in the TranslateFn of the dragged card (a move repaints that
// card and builds nothing); Dragging (the card's ID, 0 when idle) and Over
// (where it would land) are read by the columns, which rebuild when they
// change: at the start and the end of the drag, and when the pointer
// crosses a card. It is a pointer made once per board, so it is comparable
// and sits in the props of the columns and cards.
type DragState struct {
	Dragging *tela.Value[int]
	Offset   *tela.Value[Offset]
	Over     *tela.Value[Target]
}

// useDrag is a custom hook: call it from a component body.
func useDrag() *DragState {
	d := tela.UseOnce(func() *DragState { return &DragState{} })
	d.Dragging = tela.UseValue(0)
	d.Offset = tela.UseValue(Offset{})
	d.Over = tela.UseValue(Target{})
	return d
}

// Active reports whether a card is being dragged.
func (d *DragState) Active() bool { return d.Dragging.Get() != 0 }

// Start begins dragging card id of column col: until the pointer reaches
// another card or column, the target is the card itself (no change).
func (d *DragState) Start(id, col int) {
	d.Offset.Set(Offset{})
	d.Over.Set(Target{Col: col, Before: id, Valid: true})
	d.Dragging.Set(id)
}

// Drop ends the drag: the card goes to the last target. It runs through
// tela.Post, which shares one queue with Value updates, so it sees the Over
// set by the last pointer move even if no frame ran in between.
func (d *DragState) Drop(id int, set tela.Setter[Board]) {
	tela.Post(func() {
		target := d.Over.Get()
		d.Dragging.Set(0)
		d.Offset.Set(Offset{})
		d.Over.Set(Target{})
		set.Update(func(b Board) Board { return b.Move(id, target) })
	})
}

// BoardView is the three columns.
type BoardView struct {
	Board    Board
	Tasks    Tasks
	Set      tela.Setter[Board]
	SetTasks tela.Setter[Tasks]
	Compose  tela.Setter[int] // opens the new-task dialog for a column
}

func (p BoardView) Build() tela.Node {
	d := useDrag()
	full := p.Board.Full()
	return Div(Class("grid flex-1 grid-cols-3 gap-5"),
		MapKeyed(upto(3), func(i int) int { return i }, func(i int) tela.Component {
			return Lane{Index: i, Cards: p.Board[i], Tasks: p.Tasks, Full: full, D: d, Set: p.Set, SetTasks: p.SetTasks, Compose: p.Compose}
		}))
}

var laneDot = [3]string{"bg-foreground/40", "bg-primary", "bg-foreground/80"}

// Lane is one column and a drop target. Its fields are comparable, so it
// builds again only when its cards change or the drop target does.
type Lane struct {
	Index    int
	Cards    Column
	Tasks    Tasks
	Full     [3]bool
	D        *DragState
	Set      tela.Setter[Board]
	SetTasks tela.Setter[Tasks]
	Compose  tela.Setter[int]
}

func (p Lane) Build() tela.Node {
	dragging, over := p.D.Dragging.Get(), p.D.Over.Get()
	here := dragging != 0 && over.Valid && over.Col == p.Index
	ids := p.Cards.IDs()
	name := columnNames[p.Index]
	return Div(TestID("lane"), Role("group"), AriaLabel(name),
		Class("flex min-h-24 flex-col gap-3 rounded-2xl p-2", when(here, "bg-white/[0.05] ring-1 ring-primary/50")),
		// Entering the column or leaving it keeps Over up to date; the
		// cards handle their own boundaries.
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
		Div(Class("flex items-center gap-2.5 px-1 pb-1"),
			Span(Class("size-2 rounded-full", laneDot[p.Index])),
			H2(Class("text-[13px] font-medium"), Text(name)),
			Span(Class("text-xs text-muted-foreground tabular-nums"), Textf("%d", len(ids))),
			kit.Button{Variant: kit.Ghost, Size: kit.Icon, Icon: icons.Plus, AriaLabel: "New task in " + name, Disabled: p.Full[p.Index],
				Class: "ml-auto size-7", OnClick: func() { p.Compose.Set(p.Index) }}),
		Div(Class("flex flex-col gap-2.5"),
			If(len(ids) == 0, Div(Class("flex h-24 items-center justify-center rounded-xl border border-dashed border-border text-[13px] text-muted-foreground"),
				Text("Drop cards here"))),
			MapKeyed(ids, func(id int) int { return id }, func(id int) tela.Component {
				line := lineNone
				switch {
				case here && dragging != id && over.Before == id:
					line = lineBefore
				case here && over.Before == 0 && id == ids[len(ids)-1] && dragging != id:
					line = lineAfter
				}
				return TaskCard{ID: id, Col: p.Index, Task: p.Tasks[id], Full: p.Full, Dragging: dragging == id, Line: line,
					D: p.D, Set: p.Set, SetTasks: p.SetTasks}
			})),
	)
}

// line says where a card shows the insertion bar of the dragged card.
type line uint8

const (
	lineNone   line = iota
	lineBefore      // above the card
	lineAfter       // below the card
)

// insertion is the bar that shows where a dropped card goes: absolutely
// positioned, so nothing shifts under the pointer.
func insertion(pos string) tela.Node {
	return Div(Class("pointer-events-none absolute inset-x-1 z-10 h-[3px] rounded-full bg-primary", pos))
}

var priorityIcon = [5]tela.Icon{{}, icons.SignalHigh, icons.SignalHigh, icons.SignalMedium, icons.SignalLow}

// TaskCard is a draggable card. While it is dragged it is lifted (the
// glass of a popover, a ring, a slight rotation, z-50) and
// pointer-events-none, so the pointer reaches the column and the card
// under it: those decide where it lands. Its menu and its context menu
// share the items of cardItems; Edit and Delete open the dialogs of
// actions.go, whose state lives here.
type TaskCard struct {
	ID, Col  int
	Task     Task
	Full     [3]bool
	Dragging bool
	Line     line
	D        *DragState
	Set      tela.Setter[Board]
	SetTasks tela.Setter[Tasks]
}

func (p TaskCard) Build() tela.Node {
	t := p.Task
	toast := kit.UseToast()
	editing, setEditing := tela.UseState(false)
	deleting, setDeleting := tela.UseState(false)
	draft, setDraft := tela.UseState(t)
	items := p.items(toast, func() { setDraft.Set(t); setEditing.Set(true) }, func() { setDeleting.Set(true) })

	look := "cursor-grab hover:border-foreground/20 hover:bg-white/[0.07] active:cursor-grabbing"
	if p.Dragging {
		look = "border-primary/60 bg-menu shadow-xl shadow-black/40 ring-1 ring-primary/40"
	}
	props := []tela.Child{
		TestID("card"), Class("relative select-none"),
		If(p.Dragging, Class("pointer-events-none z-50 rotate-2")),
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
		If(p.Dragging, Style(tela.TranslateFn(func() (x, y float32) {
			o := p.D.Offset.Get()
			return o.X, o.Y
		}))),
		Div(Class("flex flex-col gap-3 rounded-xl border border-border bg-card p-3.5", look),
			Div(Class("flex items-center justify-between"),
				Span(Class("flex items-center gap-2 font-mono text-[11px] text-muted-foreground"),
					Icon(priorityIcon[t.Priority], AriaLabel(priorityNames[t.Priority]), Class("size-3.5", when(t.Priority == 1, "text-primary"))),
					Text(t.Key)),
				kit.DropdownMenu[string]{Align: "end", Items: items, Trigger: kit.Button{
					Variant: kit.Ghost, Size: kit.Icon, Icon: icons.Ellipsis, AriaLabel: "Actions for " + t.Title,
					Class: "-my-1 -mr-1.5 size-6 text-muted-foreground"}}),
			Div(
				H3(Class("text-[13px] leading-snug font-medium"), Text(t.Title)),
				P(Class("mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground"), Text(t.Note))),
			Div(Class("flex items-center justify-between"),
				Div(Class("flex items-center gap-3"),
					Span(Class("rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground"), Text(t.Tag)),
					If(t.Comments > 0, Span(Class("flex items-center gap-1 text-xs text-muted-foreground"),
						Icon(icons.MessageSquare, Class("size-3")), Textf("%d", t.Comments)))),
				Div(Class("flex items-center gap-2"),
					Span(Class("text-xs text-muted-foreground"), Text(t.Due)),
					kit.Avatar{Fallback: t.Assignee, Alt: t.Assignee, Class: "size-5"}))),
	}
	return Fragment(kit.ContextMenu[string]{
		Class: "relative", AriaLabel: "Actions for " + t.Title, Items: items,
		Children: Fragment(
			If(p.Line == lineBefore, insertion("-top-[7px]")),
			Div(props...),
			If(p.Line == lineAfter, insertion("-bottom-[7px]")),
			TaskDialog{Open: editing, SetOpen: setEditing.Set, Draft: draft, SetDraft: setDraft,
				Title: "Edit task", Description: "Change what the card says. Dragging or the Move to menu changes its column.",
				Submit: "Save changes",
				OnSave: func(t Task) {
					p.SetTasks.Update(func(ts Tasks) Tasks { ts[p.ID] = t; return ts })
					toast.Success("Task updated", kit.ToastDescription(t.Title))
				}},
			DeleteDialog{Open: deleting, SetOpen: setDeleting, ID: p.ID, Title: t.Title, Set: p.Set, Toast: toast},
		),
	})
}
