package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// items are the entries of a card's menu, the same for the dropdown of its
// ellipsis and for its context menu: Move to every other column (a full
// column is disabled), Edit and Delete (destructive). Moving works from the
// keyboard, which dragging does not.
func (p TaskCard) items(toast kit.ToastHandle, edit, remove func()) []kit.MenuItem[string] {
	items := []kit.MenuItem[string]{{Kind: kit.MenuLabel, Label: "Move to"}}
	for col, name := range columnNames {
		if col == p.Col {
			continue
		}
		items = append(items, kit.MenuItem[string]{
			Key: "move-" + name, Label: name, Icon: icons.ArrowRight, Disabled: p.Full[col],
			OnSelect: func() {
				p.Set.Update(func(b Board) Board { return b.Move(p.ID, Target{Col: col, Valid: true}) })
				toast.Success("Moved to "+name, kit.ToastDescription(p.Task.Title))
			},
		})
	}
	return append(items,
		kit.MenuItem[string]{Kind: kit.MenuSeparator},
		kit.MenuItem[string]{Key: "edit", Label: "Edit task", Icon: icons.Pencil, OnSelect: edit},
		kit.MenuItem[string]{Key: "delete", Label: "Delete task", Icon: icons.Trash, Variant: kit.Destructive, OnSelect: remove},
	)
}

// TaskDialog edits the title, the note and the team of a task in a
// kit.Dialog opened by state (a menu or a button opens it, so it has no
// trigger). The form is a Draft its owner keeps, set from the task each
// time the dialog opens; OnSave gets it. A task with no title cannot be
// saved. The same dialog writes a new task.
type TaskDialog struct {
	Open                       bool
	SetOpen                    func(bool)
	Draft                      Task
	SetDraft                   tela.Setter[Task]
	Title, Description, Submit string
	OnSave                     func(Task)
}

func (p TaskDialog) Build() tela.Node {
	open, draft := p.Open, p.Draft
	title, note, tag := draft.Title, draft.Note, draft.Tag
	edit := func(f func(*Task, string)) func(string) {
		return func(v string) { p.SetDraft.Update(func(t Task) Task { f(&t, v); return t }) }
	}
	invalid := strings.TrimSpace(title) == ""
	errText := ""
	if invalid {
		errText = "Give the task a title."
	}
	save := func() {
		draft.Title = strings.TrimSpace(draft.Title)
		p.SetOpen(false)
		p.OnSave(draft)
	}
	items := make([]kit.SelectItem[string], len(tags))
	for i, t := range tags {
		items[i] = kit.SelectItem[string]{Value: t, Label: t}
	}
	return kit.Dialog{
		Open: &open, OnOpenChange: p.SetOpen,
		Title:       Text(p.Title),
		Description: Text(p.Description),
		Children: Div(Class("flex flex-col gap-4"),
			kit.Field{Label: "Title", Error: errText, Children: kit.Input{
				Value: &title, OnValueChange: edit(func(t *Task, v string) { t.Title = v }), AutoFocus: true,
			}},
			kit.Field{Label: "Note", Children: kit.Textarea{
				Value: &note, OnValueChange: edit(func(t *Task, v string) { t.Note = v }), Class: "min-h-20",
			}},
			kit.Field{Label: "Team", Children: kit.Select[string]{
				Items: items, Value: &tag, OnValueChange: edit(func(t *Task, v string) { t.Tag = v }), Class: "w-full",
			}},
		),
		Footer: Fragment(
			kit.Button{Variant: kit.Outline, OnClick: func() { p.SetOpen(false) }, Children: Text("Cancel")},
			kit.Button{Disabled: invalid, OnClick: save, Children: Text(p.Submit)},
		),
	}
}

// DeleteDialog asks before a task is removed from the board.
type DeleteDialog struct {
	Open    bool
	SetOpen tela.Setter[bool]
	ID      int
	Title   string
	Set     tela.Setter[Board]
	Toast   kit.ToastHandle
}

func (p DeleteDialog) Build() tela.Node {
	open := p.Open
	return kit.AlertDialog{
		Open: &open, OnOpenChange: p.SetOpen.Set,
		Title:       Text("Delete this task?"),
		Description: Textf("%q will be removed from the board. This cannot be undone.", p.Title),
		Action:      Text("Delete task"),
		Destructive: true,
		OnAction: func() {
			p.Set.Update(func(b Board) Board { return b.Remove(p.ID) })
			p.Toast.Success("Task deleted", kit.ToastDescription(p.Title))
		},
	}
}

// NewTask is the dialog that writes a new task into a column (Col, or -1
// while closed). It keeps its own draft, which starts empty each time it
// opens.
type NewTask struct {
	Col      int
	Close    tela.Setter[int]
	Tasks    Tasks
	SetBoard tela.Setter[Board]
	SetTasks tela.Setter[Tasks]
}

func (p NewTask) Build() tela.Node {
	toast := kit.UseToast()
	draft, setDraft := tela.UseState(Task{Tag: "Product"})
	// A new opening starts from an empty draft, before it is painted.
	tela.UseLayoutEffect(func() func() {
		if p.Col >= 0 {
			setDraft.Set(Task{Tag: "Product"})
		}
		return nil
	}, p.Col >= 0)
	col := max(p.Col, 0)
	return TaskDialog{
		Open: p.Col >= 0, Draft: draft, SetDraft: setDraft,
		SetOpen: func(open bool) {
			if !open {
				p.Close.Set(-1)
			}
		},
		Title: "New task", Description: "It goes to the end of " + columnNames[col] + ".", Submit: "Create task",
		OnSave: func(t Task) {
			id := p.Tasks.freeID()
			if id == 0 {
				toast.Error("The board is full")
				return
			}
			t.Key, t.Assignee, t.Priority, t.Due, t.DueIn = p.Tasks.nextKey(), "OM", 3, "Nov 8", 9
			p.SetTasks.Update(func(ts Tasks) Tasks { ts[id] = t; return ts })
			p.SetBoard.Update(func(b Board) Board { return b.Add(id, col) })
			toast.Success("Task created", kit.ToastDescription(t.Title))
		},
	}
}
