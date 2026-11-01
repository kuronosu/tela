package main

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const overlayText = "Drop to import"

// Files dropped anywhere on the window become notes, as Open makes them,
// and the last one opens.
func TestDropFilesMakeNotes(t *testing.T) {
	sandbox(t)
	a := mount(t)
	todo := write(t, "todo.md", "Todo\r\n- ship it\r\n")
	log := write(t, "log.txt", "Monday: started")

	if got := a.DropFiles(editor, todo, log); got != tela.DropCopy {
		t.Fatalf("drop = %v, want a copy", got)
	}
	a.ExpectText("Opened 2 notes")
	a.ExpectRole("button", "todo")
	a.ExpectRole("button", "log")
	a.ExpectValue(titleBox, "log") // the last one is open
	a.ExpectValue(editor, "Monday: started")
	a.ExpectText(log)
	a.ExpectCount(unsaved, 0) // a file is saved by definition

	// The text has its line ends fixed, as Open does.
	a.Click(card("todo"))
	a.ExpectValue(editor, "Todo\n- ship it\n")
	a.ExpectNoText(overlayText)
}

// A file that a note is tied to selects the note; so does the same file
// twice in a drop.
func TestDropAnOpenFileSelectsItsNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	todo := write(t, "todo.md", "Todo")
	a.DropFiles(telatest.ByRole("button", "New note"), todo)
	a.ExpectValue(editor, "Todo")
	a.Click(card("Welcome to Notes"))

	a.DropFiles(editor, todo)
	a.ExpectValue(titleBox, "todo")
	a.ExpectCount(card("todo"), 1)

	a.DropFiles(editor, todo, todo)
	a.ExpectCount(card("todo"), 1)
}

// A drop goes to where the user is: the folder the rail chose.
func TestDropGoesToTheChosenFolder(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(railItem("Ideas"))
	a.DropFiles(railItem("Ideas"), write(t, "plan.md", "Plan"))
	a.Click(railItem("Ideas"))
	a.ExpectCount(card("plan"), 1)
	a.ExpectCount(card("Groceries"), 0)
}

// Text dropped on the window is a note, like one that was typed: it is not
// in a file, so it is unsaved.
func TestDropTextMakesANote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	if got := a.DropData(editor, telatest.DragText("Buy stamps\nand envelopes")); got != tela.DropCopy {
		t.Fatalf("drop = %v, want a copy", got)
	}
	a.ExpectRole("button", "Buy stamps, unsaved changes")
	a.ExpectValue(editor, "Buy stamps\nand envelopes")
	a.ExpectValue(titleBox, "")
	a.ExpectRole("img", "Unsaved changes")

	// Files win over the text that comes with them (a file manager also
	// offers the names).
	todo := write(t, "todo.md", "Todo")
	a.DropData(editor, telatest.DragFiles(todo).And(telatest.DragText(todo)))
	a.ExpectValue(titleBox, "todo")
}

// Other drags are refused: no overlay while they are over the window, and
// nothing happens when they are dropped.
func TestDropRefusesOtherTypes(t *testing.T) {
	sandbox(t)
	a := mount(t)
	notes := a.Count(telatest.ByRole("button", ""))

	png := telatest.DragType("image/png", "not really a png")
	a.HoverDrag(editor, telatest.Carrying(png))
	a.ExpectNoText(overlayText)
	if got := a.DropOn(editor); got != tela.DropNone {
		t.Errorf("an image was accepted: %v", got)
	}

	if got := a.DropData(editor, telatest.DragType("application/x-card", "7")); got != tela.DropNone {
		t.Errorf("a custom type was accepted: %v", got)
	}
	if got := a.DropData(editor, telatest.DragHTML("<b>hi</b>")); got != tela.DropNone {
		t.Errorf("html without text was accepted: %v", got)
	}
	if n := a.Count(telatest.ByRole("button", "")); n != notes {
		t.Errorf("a refused drop changed the screen: %d buttons, want %d", n, notes)
	}
	a.ExpectNoText(overlayText)
}

// A file that is not a note (a photo, a binary named .txt) is named in a
// toast and leaves the others alone.
func TestDropFilesThatCannotBeOpened(t *testing.T) {
	sandbox(t)
	a := mount(t)
	photo := write(t, "photo.png", "\x89PNG")
	bad := write(t, "binary.txt", "\xff\xfe\x00binary")
	good := write(t, "ok.md", "fine")

	a.DropFiles(editor, photo, bad, good)
	a.ExpectText("Could not open photo.png")
	a.ExpectText("Notes opens .txt and .md files.")
	a.ExpectText("1 file more could not be opened either.")
	a.ExpectValue(titleBox, "ok")
	a.ExpectCount(card("photo.png"), 0)
	a.ExpectCount(card("binary.txt"), 0)

	// Only what cannot be used: nothing opens, the open note stays.
	a.DropFiles(editor, photo)
	a.ExpectValue(titleBox, "ok")
}

// The overlay is there while an acceptable drag is over the window, wherever
// in it, and goes with the drag: leaving, dropping or canceling.
func TestDropOverlay(t *testing.T) {
	sandbox(t)
	a := mount(t)
	todo := write(t, "todo.md", "Todo")
	files := telatest.Carrying(telatest.DragFiles(todo))

	a.HoverDrag(editor, files)
	a.ExpectText(overlayText)
	a.ExpectRole("status", overlayText)

	// Moving between elements (the enter of the new one comes before the
	// leave of the old one) keeps it.
	a.HoverDrag(telatest.ByRole("button", "New note"))
	a.ExpectText(overlayText)
	a.HoverDrag(searchField)
	a.HoverDrag(titleBox)
	a.ExpectText(overlayText)

	a.LeaveDrag()
	a.ExpectNoText(overlayText)

	a.HoverDrag(editor)
	a.ExpectText(overlayText)
	a.CancelDrag()
	a.ExpectNoText(overlayText)

	a.HoverDrag(editor, telatest.Carrying(telatest.DragText("some text")))
	a.ExpectText(overlayText)
	a.DropOn(editor)
	a.ExpectNoText(overlayText)
	a.ExpectRole("button", "some text, unsaved changes")
}

// The overlay does not take the drag from what is under it: a drop on a
// card works too.
func TestDropOnACard(t *testing.T) {
	sandbox(t)
	a := mount(t)
	todo := write(t, "todo.md", "Todo")
	a.HoverDrag(card("Groceries"), telatest.Carrying(telatest.DragFiles(todo)))
	a.ExpectText(overlayText)
	if got := a.DropOn(card("Groceries")); got != tela.DropCopy {
		t.Errorf("a drop on a card = %v", got)
	}
	a.ExpectValue(titleBox, "todo")
}

// Preferences is not a drop target.
func TestDropOnPreferencesIsRefused(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a)
	todo := write(t, "todo.md", "Todo")
	if got := prefs.DropFiles(telatest.ByRole("button", "Appearance"), todo); got != tela.DropNone {
		t.Errorf("Preferences took a drop: %v", got)
	}
	a.ExpectCount(card("todo"), 0)
}

// The overlay, in both schemes (testdata/notes_drop.png and the dark one).
func TestGoldenDropOverlay(t *testing.T) {
	sandbox(t)
	todo := write(t, "todo.md", "Todo")
	a := mount(t)
	a.HoverDrag(editor, telatest.Carrying(telatest.DragFiles(todo)))
	a.ExpectText(overlayText)
	a.Golden("notes_drop.png")
	a.CancelDrag()

	d := mount(t, telatest.Dark())
	d.Click(railItem("All notes"))
	d.HoverDrag(card("Groceries"), telatest.Carrying(telatest.DragText("a dropped thought")))
	d.ExpectText(overlayText)
	d.Golden("notes_drop_dark.png")
}
