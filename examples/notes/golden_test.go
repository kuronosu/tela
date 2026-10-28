package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestGolden is the main window with the welcome note, light and dark, at
// scales 1 and 2 (testdata/notes_x1.png, notes_dark_x2.png...). The title
// bar is GNOME's: the goldens do not depend on the OS they are made on.
func TestGolden(t *testing.T) {
	sandbox(t)
	mount(t).GoldenScales("notes", 1, 2)
	mount(t, telatest.Dark()).GoldenScales("notes_dark", 1, 2)
}

// TestGoldenEdited is a second note being typed: the dot of the unsaved
// changes in the title bar and the sidebar.
func TestGoldenEdited(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "Groceries\n- milk\n- eggs\n- coffee")
	a.MouseMove(windowW-60, windowH-120) // the pointer rests inside the editor, away from the buttons
	a.Golden("notes_edited.png")
}

// TestGoldenPreferences is the second window, which is as big as it asks:
// a golden of a window other than the main one.
func TestGoldenPreferences(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a)
	prefs.Golden("preferences.png")
}
