package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The goldens use the fixed clock of sandbox and the GNOME conventions
// (Notes.Look): they do not depend on the day or on the OS they are made on.

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

// TestGoldenPreferencesWindows is Preferences with the Windows conventions,
// drawn on any OS: the surfaces keep 60% of their background so that the
// window's Mica backdrop shows through. The headless backend draws no Mica,
// so the window's clear color shows instead (the pixels of a Windows run of
// TestGoldenPreferences before Notes.Look was named in the tests).
func TestGoldenPreferencesWindows(t *testing.T) {
	sandbox(t)
	a := mountLook(t, kit.Windows)
	prefs := openPreferences(a)
	prefs.MouseMove(300, 400)
	prefs.Golden("preferences_windows.png")
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

// TestGoldenLocked is a locked note: its card with the lock and no text,
// and the editor's placeholder with the Unlock button (testdata/notes_locked.png
// and the dark one), and the note while it is unlocked, with the lock in the
// accent color in the header.
func TestGoldenLocked(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	a.MouseMove(windowW-60, windowH-120)
	a.Golden("notes_locked.png")

	a.Click(unlockBtn)
	a.ExpectValue(editor, groceriesText)
	a.MouseMove(windowW-60, windowH-120)
	a.Golden("notes_unlocked.png")

	d := mount(t, telatest.Dark())
	d.Click(lockedCard) // the library of the first run, which is still locked
	d.ExpectText("This note is locked")
	d.MouseMove(windowW-60, windowH-120)
	d.Golden("notes_locked_dark.png")
}

// TestGoldenNoKeyring is the lock button of a desktop with no keyring: it is
// off, and its tooltip says why.
func TestGoldenNoKeyring(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.Secrets)
	a.Click(card("Groceries"))
	a.Click(lockNote)
	a.ExpectText("This desktop has no system keyring")
	a.Advance(10 * time.Second) // the toast goes away
	a.MouseMove(windowW-60, windowH-120)
	b := a.Bounds(lockNote)
	a.MouseMove(b.X+b.Width/2, b.Y+b.Height/2)
	a.Advance(time.Second)
	a.ExpectText("Locked notes need a system keyring")
	a.Golden("notes_no_keyring.png")
}

// TestGoldenAccount is the rail with an account, light and dark: signed out
// (the card that offers the sign-in), and signed in and synced (the account
// with its sync state).
func TestGoldenAccount(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := mountSynced(t, d)
	a.MouseMove(windowW-60, windowH-60)
	a.Golden("notes_signed_out.png")

	b := signedIn(t, d)
	b.MouseMove(windowW-60, windowH-60)
	b.Golden("notes_signed_in.png")

	// The dark scheme, signed in with a change waiting for the server.
	d2 := demo(t)
	c := signedIn(t, d2, telatest.Dark())
	d2.SetDown(true)
	setClock(t, fixedNow.Add(time.Hour))
	typeNote(c, "Offline for now\n- [ ] sync later")
	c.Advance(syncDelay + time.Second)
	c.ExpectText("Offline")
	c.MouseMove(windowW-60, windowH-120)
	c.Golden("notes_offline_dark.png")
}

// TestGoldenPreview is a note rendered as Markdown, with the segmented
// control of the header: the preview of the welcome note, a note with tasks,
// a table and code in the dark scheme, and the split view in a wide window.
func TestGoldenPreview(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(previewTab)
	a.MouseMove(windowW-60, windowH-120)
	a.Golden("notes_preview.png")

	sandbox(t) // new folders: the window opens at the size asked for, in edit view
	b := mount(t, telatest.Dark(), telatest.Size(1400, 800))
	b.Click(newNote)
	b.Type("Release plan") // the cursor starts in the title
	b.Press("Enter")
	b.Type(markdownDemo)
	b.Click(splitTab)
	b.MouseMove(1400-60, 800-60)
	b.Golden("notes_split_dark.png")
	b.Click(previewTab)
	b.MouseMove(1400-60, 800-60)
	b.Golden("notes_preview_dark.png")
}

// markdownDemo is a note that shows what the preview draws.
const markdownDemo = "# Plan for the week\n\nA **short** list, with `code`, a [link to the groceries](notes:3) and ~~old~~ news.\n\n" +
	"- [x] Write the draft\n- [ ] Review it with Ana\n- [ ] Ship on Friday\n\n" +
	"| Day | Task |\n|---|---|\n| Mon | Draft |\n| Fri | Ship |\n\n" +
	"> Good notes are short.\n\n```go\nfmt.Println(\"hello\")\n```\n"
