package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

// Locking a note tied to a file detaches it and offers to move the file to
// the Trash (lock.go, ADR-216). The headless trash of telatest moves the
// file into a temporary folder, never the user's trash.

var (
	moveToTrash = telatest.ByRole("button", "Move to Trash")
	keepFile    = telatest.ByRole("button", "Keep file")
)

// openFileNote opens a new text file as a note and returns its path.
func openFileNote(t *testing.T, a *telatest.App, name, text string) string {
	t.Helper()
	path := write(t, name, text)
	a.Click(openBtn)
	a.AnswerFileDialog(path)
	a.ExpectValue(editor, text)
	a.ExpectText(path)
	return path
}

// lockFileNote opens a file as a note and locks it; the trash dialog is
// then open, and the rest of the window is inert behind it.
func lockFileNote(t *testing.T, a *telatest.App, name, text string) string {
	t.Helper()
	path := openFileNote(t, a, name, text)
	a.Click(lockNote)
	return path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// The library forgets the file of a locked note: notes.json has neither
// the text nor the file's name, and the dialog says where the file is.
func TestLockDetachesTheFile(t *testing.T) {
	sandbox(t)
	a := mount(t)
	path := lockFileNote(t, a, "secret-plans.md", "Plans for the windmill.")

	a.ExpectText("Move the file to the Trash?")
	a.ExpectText(`"secret-plans" is locked in your library and no longer tied to a file.`)
	a.ExpectText("The file secret-plans.md, in " + filepath.Dir(path) + ", still holds its text unencrypted.")
	a.ExpectRole("button", "Keep file")
	a.ExpectRole("button", "Move to Trash")

	noPlaintext(t, "secret-plans", "windmill", filepath.Base(filepath.Dir(filepath.Dir(path)))) // the file, its text, its folder
	locked := 0
	for _, n := range storedNotes(t) {
		if n.Locked {
			locked++
			if n.Path != "" || n.Title != "" || n.Text != "" || n.Saved != "" || !strings.HasPrefix(n.Sealed, "v1:") {
				t.Errorf("the locked note = %+v, want it sealed with no file", n)
			}
		}
	}
	if locked != 1 {
		t.Errorf("notes.json has %d locked notes, want 1", locked)
	}

	// Keep file closes the dialog and leaves the file alone.
	a.Click(keepFile)
	a.ExpectNoText("Move the file to the Trash?")
	if !exists(path) || len(a.Trashed()) != 0 {
		t.Errorf("Keep file moved the file (exists %v, trashed %v)", exists(path), a.Trashed())
	}
	a.ExpectText("Encrypted in your library")
}

func TestLockThenMoveToTrash(t *testing.T) {
	sandbox(t)
	a := mount(t)
	path := lockFileNote(t, a, "secret-plans.md", "Plans for the windmill.")
	a.Click(moveToTrash)
	a.ExpectText("Moved secret-plans.md to the Trash")
	a.ExpectNoText("Move the file to the Trash?")
	if exists(path) {
		t.Errorf("the file is still in its place")
	}
	if got := a.Trashed(); len(got) != 1 || got[0] != path {
		t.Errorf("trashed %v, want [%s]", got, path)
	}
	// The note is still there, locked.
	a.ExpectText("This note is locked")
	a.ExpectRole("button", "Locked note, locked")
}

// On a desktop that cannot move the file, nothing is offered that does
// not work: the dialog says where the file is, and so does the next one.
func TestLockTrashUnsupported(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.Trash)
	path := lockFileNote(t, a, "secret-plans.md", "Plans for the windmill.")
	a.Click(moveToTrash)
	a.ExpectText("The file still holds the text")
	a.ExpectText("this desktop cannot move its file to the Trash. The file secret-plans.md, in " + filepath.Dir(path) + ", still holds its text unencrypted")
	a.ExpectCount(moveToTrash, 0)
	if !exists(path) {
		t.Errorf("the file left its place")
	}
	a.Click(telatest.ByRole("button", "OK"))
	a.ExpectNoText("The file still holds the text")

	// The next lock knows: no button that does not work.
	other := lockFileNote(t, a, "diary.txt", "Dear diary")
	a.ExpectText("The file still holds the text")
	a.ExpectText("The file diary.txt, in " + filepath.Dir(other))
	a.ExpectCount(moveToTrash, 0)
	a.Press("Escape")
	a.ExpectNoText("The file still holds the text")
	if len(a.Trashed()) != 0 {
		t.Errorf("trashed %v", a.Trashed())
	}
}

func TestLockTrashFails(t *testing.T) {
	sandbox(t)
	a := mount(t)
	path := lockFileNote(t, a, "secret-plans.md", "Plans for the windmill.")
	a.FailTrash(errors.New("the file is in use"))
	a.Click(moveToTrash)
	a.ExpectText("Could not move secret-plans.md to the Trash")
	a.ExpectText("the file is in use. The file is still in " + filepath.Dir(path) + ".")
	if !exists(path) {
		t.Errorf("the file left its place")
	}

	// A file that went away meanwhile is not an error.
	a.FailTrash(nil)
	gone := lockFileNote(t, a, "gone.md", "x")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	a.Click(moveToTrash)
	a.ExpectText("gone.md was already gone")
}

// Removing the lock keeps the note detached: Save asks where.
func TestRemoveLockStaysDetached(t *testing.T) {
	sandbox(t)
	a := mount(t)
	path := lockFileNote(t, a, "secret-plans.md", "Plans for the windmill.")
	a.Click(keepFile)
	a.Click(unlockBtn)
	a.ExpectValue(editor, "Plans for the windmill.")
	a.ExpectText("In your library")
	a.Click(telatest.ByRole("button", "secret-plans, locked"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Remove lock"))
	a.ExpectRole("button", "secret-plans")
	a.ExpectText("In your library")
	a.ExpectNoText(path)

	a.Click(saveBtn)
	if d := a.FileDialogs(); len(d) != 1 || d[0].Kind != telatest.SaveFileDialog {
		t.Fatalf("dialogs = %+v, want a save dialog: the note has no file", d)
	}
	a.AnswerFileDialog()
	if got := read(t, path); got != "Plans for the windmill." {
		t.Errorf("the old file changed: %q", got)
	}
}

// A locked note is never written to a file: Save is off with a tooltip,
// and Ctrl+S says why, unlocked or not.
func TestLockedNoteCannotBeSaved(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	a.Expect(saveBtn, telatest.Disabled(true))
	a.MouseMove(windowW-60, windowH-120)
	b := a.Bounds(saveBtn)
	a.MouseMove(b.X+b.Width/2, b.Y+b.Height/2)
	a.ExpectText("Locked notes stay in your library. Remove the lock to save to a file.")

	a.Click(unlockBtn)
	a.ExpectValue(editor, groceriesText)
	a.Expect(saveBtn, telatest.Disabled(true))
	a.Click(editor)
	a.Press("ControlOrMeta+s")
	a.ExpectText("Locked notes stay in your library.")
	a.ExpectText("Remove the lock to save to a file.")
	if d := a.FileDialogs(); len(d) != 0 {
		t.Errorf("Ctrl+S of a locked note opened %+v", d)
	}

	// Any other note saves as before.
	a.Click(card("Keyboard shortcuts"))
	a.Expect(saveBtn, telatest.Disabled(false))
}

// Quitting does not ask about a locked note, even with edits: it is never
// saved to a file, so it has nothing unsaved.
func TestQuitWithLockedNotes(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	a.Click(unlockBtn)
	a.Click(editor)
	a.Type(" Plus saffron.")
	a.ExpectCount(unsaved, 0)
	a.ExpectRole("button", "Groceries, locked")
	a.RequestClose()
	a.ExpectQuit() // without a question: a message box would keep it running
}

// A library of an earlier build of locked notes may hold the path of a
// locked note: it goes when the library is opened.
func TestOldLockedNoteLosesItsPath(t *testing.T) {
	sandbox(t)
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	old := `{"notes":[
		{"id":1,"title":"Plain","text":"a","saved":"a","path":"/home/ana/plain.md"},
		{"id":2,"title":"","text":"","saved":"","path":"/home/ana/secret-plans.md","locked":true,"sealed":"v1:AAAA"}],
		"selected":1}`
	if err := os.WriteFile(filepath.Join(dir, "notes.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	a := mount(t)
	a.ExpectRole("button", "Locked note, locked")
	a.ExpectValue(editor, "a")
	file := libraryFile(t)
	if strings.Contains(file, "secret-plans") {
		t.Errorf("notes.json keeps the path of a locked note:\n%s", file)
	}
	if !strings.Contains(file, "/home/ana/plain.md") {
		t.Errorf("notes.json lost the path of a note that is not locked:\n%s", file)
	}
}

// seedLibrary writes the welcome library with Groceries tied to a file
// whose path is the same on every OS (it does not exist: locking does not
// read it), for the goldens of the trash dialog.
func seedLibrary(t *testing.T, path string) {
	t.Helper()
	ns := welcomeNotes(fixedNow, "Ctrl")
	ns[2].Path = path
	data, err := json.Marshal(map[string]any{"notes": ns, "selected": grocery})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestGoldenTrashDialog is the question after locking a note that had a
// file (testdata/notes_trash_dialog.png and the dark one), and the notice
// of a desktop without a trash.
func TestGoldenTrashDialog(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []telatest.Option
	}{{"notes_trash_dialog.png", nil}, {"notes_trash_dialog_dark.png", []telatest.Option{telatest.Dark()}}} {
		t.Run(c.name, func(t *testing.T) {
			sandbox(t)
			seedLibrary(t, "/home/ana/Notes/Groceries.md")
			a := mount(t, c.opts...)
			a.ExpectValue(editor, groceriesText)
			a.Click(lockNote)
			a.ExpectText("Move the file to the Trash?")
			a.MouseMove(windowW-60, windowH-60)
			a.Golden(c.name)
		})
	}
	t.Run("notes_trash_notice.png", func(t *testing.T) {
		sandbox(t)
		seedLibrary(t, "/home/ana/Notes/Groceries.md")
		a := mount(t)
		a.Unsupported(telatest.Trash)
		a.Click(lockNote)
		a.Click(moveToTrash)
		a.ExpectText("The file still holds the text")
		a.MouseMove(windowW-60, windowH-60)
		a.Golden("notes_trash_notice.png")
	})
}

// TestGoldenSaveLocked is the Save button while a locked note is open: off,
// with the tooltip that says why.
func TestGoldenSaveLocked(t *testing.T) {
	sandbox(t)
	a := mount(t)
	lockGroceries(t, a)
	a.MouseMove(windowW-60, windowH-120)
	b := a.Bounds(saveBtn)
	a.MouseMove(b.X+b.Width/2, b.Y+b.Height/2)
	a.Advance(time.Second)
	a.ExpectText("Locked notes stay in your library. Remove the lock to save to a file.")
	a.Golden("notes_save_locked.png")
}

// The trash dialog's text splits a path where it is written, so a path
// shows the same on every OS.
func TestSplitPath(t *testing.T) {
	cases := []struct{ path, name, folder string }{
		{"/home/ana/Notes/a.md", "a.md", "/home/ana/Notes"},
		{`C:\Users\ana\a.md`, "a.md", `C:\Users\ana`},
		{"/a.md", "a.md", "/"},
		{`C:\a.md`, "a.md", `C:\`},
		{"a.md", "a.md", "."},
	}
	for _, c := range cases {
		if name, folder := splitPath(c.path); name != c.name || folder != c.folder {
			t.Errorf("splitPath(%q) = %q, %q; want %q, %q", c.path, name, folder, c.name, c.folder)
		}
	}
}
