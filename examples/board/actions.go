package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/board/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// items are the entries of the card's menu, the same for the dropdown of its
// action and for its context menu: Move to every other column (a full
// column is disabled), Edit and Delete (destructive). Moving works from
// the keyboard too, which dragging does not.
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
		kit.MenuItem[string]{Key: "edit", Label: "Edit card", Icon: icons.Pencil, OnSelect: edit},
		kit.MenuItem[string]{Key: "delete", Label: "Delete card", Icon: icons.Trash, Variant: kit.Destructive, OnSelect: remove},
	)
}

// EditDialog edits the title, the note and the tag of a card in a dialog
// opened by state (the menu opens it, so it has no trigger). The form is a
// Draft kept by the card, which starts from the card's data each time the
// dialog opens; Save writes it into Tasks. A card with no title cannot be
// saved.
type EditDialog struct {
	Open     bool
	SetOpen  tela.Setter[bool]
	Draft    Task
	SetDraft tela.Setter[Task]
	ID       int
	SetTasks tela.Setter[Tasks]
	Toast    kit.ToastHandle
}

func (p EditDialog) Build() tela.Node {
	open, draft := p.Open, p.Draft
	title, note, tag := draft.Title, draft.Note, draft.Tag
	edit := func(f func(*Task, string)) func(string) {
		return func(v string) { p.SetDraft.Update(func(t Task) Task { f(&t, v); return t }) }
	}
	invalid := strings.TrimSpace(title) == ""
	errText := ""
	if invalid {
		errText = "Give the card a title."
	}
	save := func() {
		draft.Title = strings.TrimSpace(draft.Title)
		p.SetTasks.Update(func(ts Tasks) Tasks { ts[p.ID] = draft; return ts })
		p.SetOpen.Set(false)
		p.Toast.Success("Card updated", kit.ToastDescription(draft.Title))
	}
	items := make([]kit.SelectItem[string], len(tags))
	for i, t := range tags {
		items[i] = kit.SelectItem[string]{Value: t, Label: t}
	}
	return Fragment(kit.Dialog{
		Open: &open, OnOpenChange: p.SetOpen.Set,
		Title:       Text("Edit card"),
		Description: Text("Change what the card says. Dragging or the Move to menu changes its column."),
		Children: Div(Class("flex flex-col gap-4"),
			kit.Field{Label: "Title", Error: errText, Children: kit.Input{
				Value: &title, OnValueChange: edit(func(t *Task, v string) { t.Title = v }), AutoFocus: true,
			}},
			kit.Field{Label: "Note", Children: kit.Textarea{
				Value: &note, OnValueChange: edit(func(t *Task, v string) { t.Note = v }), Class: "min-h-20",
			}},
			kit.Field{Label: "Tag", Children: kit.Select[string]{
				Items: items, Value: &tag, OnValueChange: edit(func(t *Task, v string) { t.Tag = v }), Class: "w-full",
			}},
		),
		Footer: Fragment(
			kit.Button{Variant: kit.Outline, OnClick: func() { p.SetOpen.Set(false) }, Children: Text("Cancel")},
			kit.Button{Disabled: invalid, OnClick: save, Children: Text("Save changes")},
		),
	})
}

// DeleteDialog asks before a card is removed from the board.
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
	return Fragment(kit.AlertDialog{
		Open: &open, OnOpenChange: p.SetOpen.Set,
		Title:       Text("Delete this card?"),
		Description: Textf("%q will be removed from the board. This cannot be undone.", p.Title),
		Action:      Text("Delete card"),
		Destructive: true,
		OnAction: func() {
			p.Set.Update(func(b Board) Board { return b.Remove(p.ID) })
			p.Toast.Success("Card deleted", kit.ToastDescription(p.Title))
		},
	})
}
