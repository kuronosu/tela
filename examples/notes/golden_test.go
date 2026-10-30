package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The goldens use the fixed clock of sandbox and the GNOME title bar: they
// do not depend on the day or on the OS they are made on.

// TestGolden is the main window with the welcome note open, light and dark,
// at scales 1 and 2 (testdata/notes_x1.png, notes_dark_x2.png...).
func TestGolden(t *testing.T) {
	sandbox(t)
	mount(t).GoldenScales("notes", 1, 2)
	mount(t, telatest.Dark()).GoldenScales("notes_dark", 1, 2)
}

// TestGoldenOverview is the pane with no note open: the grid of cards of a
// filter (testdata/notes_overview.png, and the dark one at scale 2).
func TestGoldenOverview(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(railItem("All notes"))
	a.MouseMove(windowW-60, windowH-60) // away from the cards and the buttons
	a.Golden("notes_overview.png")

	d := mount(t, telatest.Dark(), telatest.Scale(2))
	d.Click(railItem("Getting started"))
	d.MouseMove(windowW-60, windowH-60)
	d.Golden("notes_overview_dark_x2.png")
}

// TestGoldenEdited is a second note being typed: the dot of the unsaved
// changes in the header and the card, and the new note first in the list.
func TestGoldenEdited(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "Groceries\n- milk\n- eggs\n- coffee #errands")
	a.MouseMove(windowW-60, windowH-120) // the pointer rests inside the editor, away from the buttons
	a.Golden("notes_edited.png")
}

// TestGoldenFolderMenu is the menu of the folders, open on the header.
func TestGoldenFolderMenu(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(folderMenu("Getting started"))
	a.Golden("notes_folder_menu.png")
}

// TestGoldenPreferences is the second window, which is as big as it asks:
// a golden of a window other than the main one. The first is the
// Appearance section; the second is the dark scheme on the Editor section.
func TestGoldenPreferences(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a)
	prefs.MouseMove(300, 400)
	prefs.Golden("preferences.png")

	prefs.Click(telatest.ByRole("button", "Dark"))
	prefs.Click(telatest.ByRole("button", "Editor"))
	prefs.MouseMove(300, 400)
	prefs.Golden("preferences_dark.png")
}

// TestGoldenDeleteDialog is the question before deleting a note that is
// tied to a file, and the context menu of a card.
func TestGoldenDeleteDialog(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"), telatest.Right())
	a.Golden("notes_card_menu.png")
	a.Press("Escape")
	typeNote(a, "Groceries\n- milk")
	a.Click(deleteBtn)
	a.Golden("notes_delete_dialog.png")
}
