package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 960, 640

// sandbox points the app's folders (settings, library, window placements)
// at a temporary directory for the test, so no test touches the real ones.
// A test that mounts the app twice calls it once: the second run finds what
// the first one stored.
func sandbox(t *testing.T) {
	t.Helper()
	telatest.TempDesktopDirs(t)
	desktop.SetAppID(App.AppID)
}

// appFor is the app as main runs it, except for the look of the title bar,
// which is named so that the goldens match on every OS. Like main it goes
// through desktop.PersistApp, so the window opens where it was left.
func appFor() tela.App {
	app := App
	app.Root = Notes{TitleBar: kit.GNOME}
	return desktop.PersistApp(app)
}

// mount opens the app in dev mode, where any dev message fails the test.
func mount(t *testing.T, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(windowW, windowH)}, opts...)
	return telatest.MountApp(t, appFor(), opts...)
}

// The parts of the main window.
var (
	editor   = telatest.ByRole("textbox", "Note text")
	newNote  = telatest.ByRole("button", "New note")
	openBtn  = telatest.ByRole("button", "Open")
	saveBtn  = telatest.ByRole("button", "Save")
	prefsBtn = telatest.ByRole("button", "Preferences")
	unsaved  = telatest.ByRole("img", "Unsaved changes")
)

// write makes a file in a temporary folder.
func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// read returns the content of a file.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// typeNote adds a note and types its text.
func typeNote(a *telatest.App, text string) {
	a.Click(newNote)
	a.Click(editor)
	a.Type(text)
}

// openPreferences clicks the Preferences button and returns the window it
// opens.
func openPreferences(a *telatest.App) *telatest.App {
	return a.WaitForWindow(func() { a.Click(prefsBtn) })
}

// setSetting stores a setting before the app starts.
func setSetting(t *testing.T, key string, value any) {
	t.Helper()
	if err := desktop.Settings().Set(key, value); err != nil {
		t.Fatal(err)
	}
}

func TestNoDevMessages(t *testing.T) {
	sandbox(t)
	for _, s := range [][2]float32{{windowW, windowH}, {700, 420}, {1400, 900}} {
		mount(t, telatest.Size(s[0], s[1]))
	}
	mount(t, telatest.Dark())
}

// A frameless window: the title bar is the app's, with drag regions and
// the window controls.
func TestFramelessTitleBar(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectWindowState(telatest.ClientDecorations(true))
	a.ExpectRole("group", "Window controls")
	if n := a.CaptionDrags(); n == 0 {
		t.Errorf("the title bar has no drag region")
	}
	// The close button of the title bar asks to close the window like the
	// OS's own: the app is not quit by the click itself.
	a.Click(telatest.ByRole("button", "Close"))
	a.ExpectQuit() // a clean library, no tray setting: closing quits
}

func TestNewNoteAndTyping(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectRole("button", "Welcome to Notes")
	a.ExpectCount(unsaved, 0)

	a.Click(newNote)
	a.ExpectRole("button", "Untitled note")
	a.Click(editor)
	a.Type("Groceries\nmilk and eggs")
	a.ExpectValue(editor, "Groceries\nmilk and eggs")

	// The note is named by its first line and is dirty: it has text that
	// is not in a file.
	a.ExpectRole("button", "Groceries, unsaved changes")
	a.ExpectRole("img", "Unsaved changes") // the dot in the title bar
	a.ExpectText("— Groceries")
	a.ExpectText("4 words")

	// Selecting the welcome note shows its text; the new one is still there.
	a.Click(telatest.ByRole("button", "Welcome to Notes"))
	a.ExpectValue(editor, welcomeText)
	a.ExpectCount(unsaved, 0)
	a.Click(telatest.ByRole("button", "Groceries, unsaved changes"))
	a.ExpectValue(editor, "Groceries\nmilk and eggs")
}

// The library is a JSON file in the data folder, and the next run finds it.
func TestLibraryIsPersisted(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "Groceries\nmilk")
	a.Close()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if data := read(t, filepath.Join(dir, "notes.json")); !strings.Contains(data, "milk") {
		t.Errorf("notes.json does not have the note: %s", data)
	}

	b := mount(t)
	b.ExpectRole("button", "Groceries, unsaved changes")
	b.ExpectValue(editor, "Groceries\nmilk") // the selected note is kept too
}

// A library that cannot be parsed starts the app with the welcome note and
// a toast; the file is kept next to the new one.
func TestCorruptLibrary(t *testing.T) {
	sandbox(t)
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "notes.json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := mount(t)
	a.ExpectRole("button", "Welcome to Notes")
	a.ExpectText("Your notes could not be read")

	typeNote(a, "Fresh start")
	a.Close()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, file+".corrupt"); got != "{not json" {
		t.Errorf("notes.json.corrupt = %q, want the unreadable file", got)
	}
	if got := read(t, file); !strings.Contains(got, "Fresh start") {
		t.Errorf("notes.json = %s, want the new library", got)
	}
}

func TestOpenFile(t *testing.T) {
	sandbox(t)
	a := mount(t)
	path := write(t, "todo.md", "Todo\r\n- ship it\r\n")

	a.Click(openBtn)
	dialogs := a.FileDialogs()
	if len(dialogs) != 1 || dialogs[0].Kind != telatest.OpenFileDialog {
		t.Fatalf("dialogs after Open = %+v, want one open-file dialog", dialogs)
	}
	if f := dialogs[0].Filters; len(f) != 1 || f[0].Name != "Text" || !slices.Equal(f[0].Patterns, []string{"*.txt", "*.md"}) {
		t.Errorf("filters = %+v, want Text (*.txt, *.md)", f)
	}
	a.AnswerFileDialog(path)

	// A new note named by the file, with its text (line endings made "\n")
	// and nothing unsaved.
	a.ExpectRole("button", "todo.md")
	a.ExpectValue(editor, "Todo\n- ship it\n")
	a.ExpectText(path)
	a.ExpectCount(unsaved, 0)

	// Opening the same file again selects its note instead of adding one.
	a.Click(telatest.ByRole("button", "Welcome to Notes"))
	a.Click(openBtn)
	a.AnswerFileDialog(path)
	a.ExpectValue(editor, "Todo\n- ship it\n")
	a.ExpectCount(telatest.ByRole("button", "todo.md"), 1)
}

func TestOpenFileCanceledOrUnreadable(t *testing.T) {
	sandbox(t)
	a := mount(t)
	buttons := a.Count(telatest.ByRole("button", ""))

	a.Click(openBtn)
	a.AnswerFileDialog() // canceled: nothing happens
	a.ExpectNoText("Could not")
	if n := a.Count(telatest.ByRole("button", "")); n != buttons {
		t.Errorf("a canceled dialog changed the screen: %d buttons, want %d", n, buttons)
	}
	a.ExpectValue(editor, welcomeText)

	// A file that is not text is an error toast, not a note.
	bad := write(t, "photo.txt", "\xff\xfe\x00binary")
	a.Click(openBtn)
	a.AnswerFileDialog(bad)
	a.ExpectText("Could not open photo.txt")
	a.ExpectValue(editor, welcomeText)
}

func TestSaveNewNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "Ideas\nbuild a notes app")
	a.ExpectRole("img", "Unsaved changes")

	a.Click(saveBtn)
	dialogs := a.FileDialogs()
	if len(dialogs) != 1 || dialogs[0].Kind != telatest.SaveFileDialog || dialogs[0].DefaultName != "Ideas.txt" {
		t.Fatalf("dialogs after Save = %+v, want a save dialog suggesting Ideas.txt", dialogs)
	}
	target := filepath.Join(t.TempDir(), "ideas.txt")
	a.AnswerFileDialog(target)

	if got := read(t, target); got != "Ideas\nbuild a notes app" {
		t.Errorf("the file has %q", got)
	}
	// The note is the file's now, and clean.
	a.ExpectRole("button", "ideas.txt")
	a.ExpectCount(unsaved, 0)
	a.ExpectText(target)

	// Saving again writes to the same file without asking.
	a.Click(editor)
	a.Type("!")
	a.ExpectRole("img", "Unsaved changes")
	a.Click(saveBtn)
	if d := a.FileDialogs(); len(d) != 0 {
		t.Fatalf("Save on a note with a file opened a dialog: %+v", d)
	}
	a.ExpectCount(unsaved, 0)
	if got := read(t, target); got != "Ideas\nbuild a notes app!" {
		t.Errorf("the file has %q after the second save", got)
	}
}

func TestSaveCanceled(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "Ideas")
	a.Click(saveBtn)
	a.AnswerFileDialog() // canceled
	a.ExpectRole("img", "Unsaved changes")
	a.ExpectText("Not saved to a file")
	if n := a.Notifications(); len(n) != 0 {
		t.Errorf("a canceled save notified: %+v", n)
	}
}

func TestSaveError(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "Ideas")
	a.Click(saveBtn)
	// A folder that does not exist: the dialog would not allow it, but a
	// disk can fail anyway.
	a.AnswerFileDialog(filepath.Join(t.TempDir(), "missing", "ideas.txt"))
	a.ExpectText("Could not save ideas.txt")
	a.ExpectRole("img", "Unsaved changes")
	if n := a.Notifications(); len(n) != 0 {
		t.Errorf("a failed save notified: %+v", n)
	}
}

func TestKeyboardShortcuts(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(editor) // the shortcuts are keys of the window: focus is in it
	a.Press("Control+n")
	a.ExpectRole("button", "Untitled note")
	a.Click(editor)
	a.Type("Shortcuts")
	a.Press("Control+s")
	if d := a.FileDialogs(); len(d) != 1 || d[0].Kind != telatest.SaveFileDialog {
		t.Fatalf("Ctrl+S opened %+v, want a save dialog", d)
	}
	a.AnswerFileDialog() // cancel
	a.Press("Control+o")
	if d := a.FileDialogs(); len(d) != 1 || d[0].Kind != telatest.OpenFileDialog {
		t.Fatalf("Ctrl+O opened %+v, want an open dialog", d)
	}
	a.AnswerFileDialog()
}

func TestSaveNotification(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "Ideas")
	a.Click(saveBtn)
	target := filepath.Join(t.TempDir(), "ideas.txt")
	a.AnswerFileDialog(target)

	n := a.Notifications()
	if len(n) != 1 || n[0].Title != "Saved ideas.txt" || n[0].Body != target {
		t.Fatalf("notifications = %+v, want one: Saved ideas.txt", n)
	}
}

func TestSaveNotificationCanBeTurnedOff(t *testing.T) {
	sandbox(t)
	setSetting(t, keyNotifyOnSave, false)
	a := mount(t)
	typeNote(a, "Ideas")
	a.Click(saveBtn)
	a.AnswerFileDialog(filepath.Join(t.TempDir(), "ideas.txt"))
	a.ExpectCount(unsaved, 0) // saved
	if n := a.Notifications(); len(n) != 0 {
		t.Errorf("notifications = %+v, want none: the setting is off", n)
	}
}

// Without notifications on the desktop the save is told in the window.
func TestSaveWithoutNotifications(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.Notifications)
	typeNote(a, "Ideas")
	a.Click(saveBtn)
	a.AnswerFileDialog(filepath.Join(t.TempDir(), "ideas.txt"))
	a.ExpectText("Saved ideas.txt")
}

func TestPreferencesWindow(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectWindowCount(1)
	prefs := openPreferences(a)
	a.ExpectWindowCount(2)
	prefs.ExpectTitle("Preferences")
	wrap := telatest.ByRole("switch", "Word wrap")
	prefs.Expect(wrap, telatest.Checked(true))
	prefs.Expect(telatest.ByRole("radio", "Medium"), telatest.Checked(true))

	// A setting changed in one window is stored, and the other window,
	// which reads the same key, follows.
	prefs.Click(wrap)
	prefs.Expect(wrap, telatest.Checked(false))
	a.ExpectCount(editor.Filter(telatest.HasClass("text-nowrap")), 1)
	var stored bool
	if !desktop.Settings().Get(keyWordWrap, &stored) || stored {
		t.Errorf("wordWrap in the settings store = %v, want false", stored)
	}
	prefs.Click(telatest.ByRole("radio", "Large"))
	a.ExpectCount(editor.Filter(telatest.HasClass("text-lg")), 1)

	// Closing the window is the app's state: OnClose sets it to false.
	prefs.RequestClose()
	prefs.ExpectClosed()
	a.ExpectWindowCount(1)

	// The button opens it again, and what was set is still set.
	prefs = openPreferences(a)
	prefs.Expect(wrap, telatest.Checked(false))
	prefs.Expect(telatest.ByRole("radio", "Large"), telatest.Checked(true))
}

func TestSettingsSurviveARestart(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a)
	prefs.Click(telatest.ByRole("switch", "Keep running in the tray when the window is closed"))
	prefs.Click(telatest.ByRole("switch", "Notify when a file is saved"))
	prefs.Click(telatest.ByRole("radio", "Small"))
	a.Close()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	dir, err := desktop.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if data := read(t, filepath.Join(dir, "settings.json")); !strings.Contains(data, `"keepInTray": true`) || !strings.Contains(data, `"fontSize": "small"`) {
		t.Errorf("settings.json = %s, want the three settings", data)
	}

	b := mount(t)
	prefs = openPreferences(b)
	prefs.Expect(telatest.ByRole("switch", "Keep running in the tray when the window is closed"), telatest.Checked(true))
	prefs.Expect(telatest.ByRole("switch", "Notify when a file is saved"), telatest.Checked(false))
	prefs.Expect(telatest.ByRole("radio", "Small"), telatest.Checked(true))
	b.ExpectCount(editor.Filter(telatest.HasClass("text-xs")), 1)
}

// The Preferences window opens where it was closed.
func TestPreferencesPlacementIsRemembered(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a)
	prefs.Move(120, 90)
	prefs.Resize(500, 460)
	prefs.RequestClose()
	prefs.ExpectClosed()

	saved := desktop.LoadPlacement("preferences")
	if !saved.HasPosition || saved.X != 120 || saved.Y != 90 || saved.Width != 500 || saved.Height != 460 {
		t.Fatalf("saved placement = %+v, want 500x460 at 120,90", saved)
	}
	prefs = openPreferences(a)
	if p := prefs.Placement(); !p.HasPosition || p.X != 120 || p.Y != 90 {
		t.Errorf("the window reopened at %+v, want 120,90", p)
	}
	if w, h := prefs.WindowSize(); w != 500 || h != 460 {
		t.Errorf("the window reopened %vx%v, want 500x460", w, h)
	}
}

// The main window opens where the user left it.
func TestMainPlacementIsRemembered(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Move(40, 60)
	a.Resize(800, 560)
	a.ClickTrayItem("Quit")
	a.ExpectQuit()

	saved := desktop.LoadPlacement(desktop.MainWindowKey)
	if !saved.HasPosition || saved.X != 40 || saved.Y != 60 || saved.Width != 800 || saved.Height != 560 {
		t.Fatalf("saved placement = %+v, want 800x560 at 40,60", saved)
	}

	b := mount(t)
	if p := b.Placement(); !p.HasPosition || p.X != 40 || p.Y != 60 {
		t.Errorf("the window reopened at %+v, want 40,60", p)
	}
	if w, h := b.WindowSize(); w != 800 || h != 560 {
		t.Errorf("the window reopened %vx%v, want 800x560", w, h)
	}
}

func TestTrayMenu(t *testing.T) {
	sandbox(t)
	a := mount(t)
	tray := a.Tray()
	if tray.Tooltip != "Notes" {
		t.Errorf("tray tooltip = %q, want Notes", tray.Tooltip)
	}
	var labels []string
	for _, it := range tray.Menu {
		if it.Separator {
			labels = append(labels, "-")
		} else {
			labels = append(labels, it.Label)
		}
	}
	if want := []string{"New note", "Show Notes", "Preferences…", "-", "Quit"}; !slices.Equal(labels, want) {
		t.Errorf("tray menu = %q, want %q", labels, want)
	}

	a.ClickTrayItem("New note")
	a.ExpectRole("button", "Untitled note")
	prefs := a.WaitForWindow(func() { a.ClickTrayItem("Preferences…") })
	prefs.ExpectTitle("Preferences")
}

// Closing the window with "keep running in the tray" hides it; the tray
// brings it back.
func TestCloseKeepsRunningInTheTray(t *testing.T) {
	sandbox(t)
	setSetting(t, keyKeepInTray, true)
	a := mount(t)
	typeNote(a, "Still here")

	a.RequestClose()
	a.ExpectWindowState(telatest.Hidden(true))
	a.ExpectRunning()
	if d := a.MessageBoxes(); len(d) != 0 {
		t.Fatalf("hiding to the tray asked a question: %+v", d)
	}

	a.ClickTrayItem("Show Notes")
	a.ExpectWindowState(telatest.Hidden(false))
	a.ExpectValue(editor, "Still here")

	// A click on the icon toggles the window.
	a.ClickTray()
	a.ExpectWindowState(telatest.Hidden(true))
	a.ClickTray()
	a.ExpectWindowState(telatest.Hidden(false))

	// The tray menu works on a hidden window too.
	a.RequestClose()
	a.ExpectWindowState(telatest.Hidden(true))
	a.ClickTrayItem("New note")
	a.ExpectWindowState(telatest.Hidden(false))
	a.ExpectRole("button", "Untitled note")
}

func TestCloseQuitsWhenNothingIsUnsaved(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.RequestClose()
	a.ExpectQuit()
}

// With the tray off and a note not saved to a file, closing asks first.
func TestCloseAsksAboutUnsavedNotes(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "draft")

	a.RequestClose()
	boxes := a.MessageBoxes()
	if len(boxes) != 1 {
		t.Fatalf("message boxes = %+v, want one question", boxes)
	}
	if b := boxes[0]; b.Kind != desktop.MessageQuestion || b.Buttons != desktop.ButtonsYesNoCancel || !strings.Contains(b.Text, `"draft"`) {
		t.Errorf("message box = %+v, want a Yes/No/Cancel question about draft", b)
	}

	// Cancel keeps the app and its window.
	a.AnswerMessage(desktop.AnswerCancel)
	a.ExpectRunning()
	a.ExpectWindowState(telatest.Hidden(false))
	if b := a.MessageBoxes(); len(b) != 0 {
		t.Fatalf("the box is still open: %+v", b)
	}

	// No quits without saving: the draft stays in the library.
	a.RequestClose()
	a.AnswerMessage(desktop.AnswerNo)
	a.ExpectQuit()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	b := mount(t)
	b.ExpectRole("button", "draft, unsaved changes")
}

func TestCloseSavesThenQuits(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "draft")

	a.RequestClose()
	a.AnswerMessage(desktop.AnswerYes)
	// The note has no file: the save dialog follows. Canceling it keeps the
	// app open, so no work is thrown away by accident.
	if d := a.FileDialogs(); len(d) != 1 || d[0].Kind != telatest.SaveFileDialog {
		t.Fatalf("dialogs = %+v, want a save dialog", d)
	}
	a.AnswerFileDialog()
	a.ExpectRunning()

	a.RequestClose()
	a.AnswerMessage(desktop.AnswerYes)
	target := filepath.Join(t.TempDir(), "draft.txt")
	a.AnswerFileDialog(target)
	a.ExpectQuit()
	if got := read(t, target); got != "draft" {
		t.Errorf("the file has %q, want draft", got)
	}
}

// Quit in the tray menu quits at once.
func TestTrayQuit(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, "draft")
	a.ClickTrayItem("Quit")
	a.ExpectQuit()
}
