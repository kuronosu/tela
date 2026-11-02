package main

import (
	"os"
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

var (
	deleteBtn = telatest.ByRole("button", "Delete note")
	undoBtn   = telatest.ByRole("button", "Undo")
	confirm   = telatest.ByRole("alertdialog", "")
	keepBtn   = telatest.ByRole("button", "Keep")
	doDelete  = telatest.ByRole("button", "Delete")
)

// A note with nothing to lose is deleted at once, the next note of the list
// opens, and the toast can bring it back.
func TestDeleteCleanNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"))
	a.Click(deleteBtn)
	a.ExpectCount(confirm, 0)
	a.ExpectCount(card("Groceries"), 0)
	a.ExpectText("Deleted Groceries")
	a.ExpectValue(titleBox, "Weekend in the mountains") // the one after it in the list
	a.ExpectCount(railItem("All notes").Filter(telatest.HasText("3")), 1)
	a.ExpectCount(railItem("Home"), 0) // its folder had only that note

	a.Click(undoBtn)
	a.ExpectCount(railItem("All notes").Filter(telatest.HasText("4")), 1)
	a.Expect(card("Groceries"), telatest.Current("page")) // it was the open one: it is again
	a.ExpectValue(editor, groceriesText)
	a.ExpectCount(railItem("Home").Filter(telatest.HasText("1")), 1)
}

// The last note of the list hands the selection to the one before it, and
// the only one leaves the overview.
func TestDeleteSelectsAnotherOrGoesBack(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Weekend in the mountains"))
	a.Click(deleteBtn)
	a.ExpectValue(titleBox, "Groceries") // there is none after it

	a.Click(railItem("Pinned"))
	a.Click(card("Welcome to Notes"))
	a.Click(deleteBtn)
	a.ExpectCount(editor, 0) // nothing left in the list: the overview
	a.ExpectRole("heading", "Pinned")
	a.ExpectText("No notes here yet.")
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("0")), 0)
}

// A note with changes that are not in a file asks first; Keep keeps it.
func TestDeleteUnsavedNoteAsks(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "draft\nwords")
	a.Click(deleteBtn)
	a.ExpectCount(confirm, 1)
	a.ExpectText(`Delete "draft"?`)
	a.ExpectText("not saved to a file")
	a.ExpectCount(telatest.ByText("is not deleted"), 0)

	a.Click(keepBtn)
	a.ExpectCount(confirm, 0)
	a.ExpectRole("button", "draft, unsaved changes")
	a.ExpectValue(editor, "draft\nwords")

	a.Click(deleteBtn)
	a.Click(doDelete)
	a.ExpectCount(confirm, 0)
	a.ExpectCount(card("draft, unsaved changes"), 0)
	a.ExpectText("Deleted draft")

	// Undo brings the note back as it was, with its changes.
	a.Click(undoBtn)
	a.ExpectRole("button", "draft, unsaved changes")
	a.ExpectValue(editor, "draft\nwords")
}

// A note tied to a file asks too, and says the file stays; deleting the note
// does not touch the file.
func TestDeleteNoteWithAFileAsks(t *testing.T) {
	sandbox(t)
	a := mount(t)
	path := write(t, "todo.md", "Todo\n- ship it\n")
	a.Click(openBtn)
	a.AnswerFileDialog(path)
	a.ExpectRole("button", "todo")
	a.Click(deleteBtn)
	a.ExpectCount(confirm, 1)
	a.ExpectText("The file " + path + " is not deleted")
	a.ExpectCount(telatest.ByText("Its changes"), 0) // it has none
	a.Click(doDelete)
	a.ExpectCount(card("todo"), 0)
	if got := read(t, path); got != "Todo\n- ship it\n" {
		t.Errorf("the file has %q after deleting its note", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error(err)
	}
}

// The context menu of a card pins and deletes, in the list and in the
// overview; deleting another note leaves the open one open.
func TestCardContextMenu(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Welcome to Notes"), telatest.Right())
	a.ExpectRole("menuitem", "Unpin")
	a.Click(telatest.ByRole("menuitem", "Unpin"))
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("0")), 0)

	a.Click(card("Groceries"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Pin"))
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("1")), 1)

	// Delete a note that is not the open one.
	a.Click(card("Weekend in the mountains"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Delete"))
	a.ExpectCount(card("Weekend in the mountains"), 0)
	a.ExpectValue(titleBox, "Welcome to Notes")
	a.ExpectText("Deleted Weekend in the mountains")

	// In the overview no note opens.
	a.Click(railItem("All notes"))
	a.Click(card("Keyboard shortcuts"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Delete"))
	a.ExpectCount(card("Keyboard shortcuts"), 0)
	a.ExpectCount(editor, 0)

	// A note with changes asks, from the menu too.
	a.Click(card("Welcome to Notes"))
	a.Click(editor)
	a.Type("!")
	a.Click(railItem("All notes"))
	a.Click(card("Welcome to Notes, unsaved changes"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Delete"))
	a.ExpectCount(confirm, 1)
	a.Click(keepBtn)
	a.ExpectRole("button", "Welcome to Notes, unsaved changes")
}

func TestDeleteShortcut(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(editor)
	a.Press("ControlOrMeta+Shift+d")
	a.ExpectCount(card("Welcome to Notes"), 0)
	a.ExpectValue(titleBox, "Keyboard shortcuts")

	// The keys that edit text do not delete the note.
	a.Click(editor)
	a.Press("Control+End")
	a.Press("Control+Backspace")
	a.Press("Delete")
	a.ExpectRole("button", "Keyboard shortcuts, unsaved changes")
	a.ExpectCount(railItem("All notes").Filter(telatest.HasText("3")), 1)
}

// A deleted note stays deleted at the next start, also the last one: the
// library is empty, not the welcome notes again.
func TestDeleteIsPersisted(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(editor)
	for range 4 {
		a.Press("ControlOrMeta+Shift+d")
	}
	a.ExpectCount(editor, 0)
	a.ExpectText("No notes here yet.")
	a.Close()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}

	b := mount(t)
	b.ExpectCount(editor, 0)
	b.ExpectText("No notes here yet.")
	b.ExpectRole("heading", "All notes")
	b.Click(newNote)
	b.ExpectCount(editor, 1)
	if got := b.Count(telatest.ByRole("button", "Untitled note")); got != 1 {
		t.Errorf("%d Untitled note cards, want 1 (a library with no notes starts with none)", got)
	}
}
