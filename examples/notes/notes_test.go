package main

import (
	"bytes"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	setClock(t, fixedNow)
	// The test binary is a temporary build, which Notes does not register
	// (registrationBlocked): the registration tests go through the fake
	// desktop, as an installed program would.
	blockRegistration(t, "")
}

// blockRegistration makes registrationBlocked say why for the rest of the
// test ("" is an installed program).
func blockRegistration(t *testing.T, why string) {
	t.Helper()
	f := func() string { return why }
	was := blockedBy.Swap(&f)
	t.Cleanup(func() { blockedBy.Store(was) })
}

// fixedNow is the time of the tests: a Friday morning, so that the welcome
// notes read "Today, 09:41", "Yesterday, 22:03" and "Mon".
var fixedNow = time.Date(2026, time.October, 30, 9, 41, 0, 0, time.UTC)

// setClock makes the app's clock say at for the rest of the test.
func setClock(t *testing.T, at time.Time) {
	t.Helper()
	was := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = was })
}

// appFor is the app as main runs it, except for the platform whose
// conventions it follows (Notes.Look), which is named so that the goldens
// match on every OS: main leaves it to the OS the app runs on. Like main it
// goes through desktop.PersistApp, so the window opens where it was left, and
// desktop.SingleInstance, so a launch raises the window.
func appFor(look kit.Variant) tela.App {
	app := App
	app.Root = Notes{Look: look}
	return desktop.SingleInstance(withAppearance(desktop.PersistApp(app)))
}

// mount opens the app in dev mode, where any dev message fails the test,
// with the GNOME conventions whatever the OS the test runs on.
func mount(t *testing.T, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mountLook(t, kit.GNOME, opts...)
}

// mountLook is mount with the conventions of another platform: the look's
// platform keys too (Cmd on a Mac, Ctrl elsewhere), so the shortcut texts do
// not depend on the OS the test runs on; an option given later wins.
func mountLook(t *testing.T, look kit.Variant, opts ...telatest.Option) *telatest.App {
	t.Helper()
	keys := "linux"
	switch look {
	case kit.MacOS:
		keys = "darwin"
	case kit.Windows:
		keys = "windows"
	}
	opts = append([]telatest.Option{telatest.Size(windowW, windowH), telatest.PlatformKeys(keys)}, opts...)
	return telatest.MountApp(t, appFor(look), opts...)
}

// The parts of the main window.
var (
	editor   = telatest.ByRole("textbox", "Note text")
	titleBox = telatest.ByRole("textbox", "Title")
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
// opens, on the section (Appearance when it is empty).
func openPreferences(a *telatest.App, section ...string) *telatest.App {
	w := a.WaitForWindow(func() { a.Click(prefsBtn) })
	for _, s := range section {
		w.Click(telatest.ByRole("button", s))
	}
	return w
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
	if !slices.ContainsFunc(a.HitRegions(), func(r telatest.HitRegion) bool { return r.Kind == telatest.HitCaption }) {
		t.Errorf("the title bar has no drag region: %v", a.HitRegions())
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
	a.ExpectFocused(titleBox) // a new note has the cursor in its title
	a.Click(editor)
	a.Type("Groceries\nmilk and eggs")
	a.ExpectValue(editor, "Groceries\nmilk and eggs")

	// With no title, the note is named by its first line and is dirty: it
	// has text that is not in a file.
	a.ExpectRole("button", "Groceries, unsaved changes")
	a.ExpectRole("img", "Unsaved changes") // the dot in the editor's header
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
	a.ExpectRole("button", "todo")
	a.ExpectValue(titleBox, "todo")
	a.ExpectValue(editor, "Todo\n- ship it\n")
	a.ExpectText(path)
	a.ExpectCount(unsaved, 0)

	// Opening the same file again selects its note instead of adding one.
	a.Click(telatest.ByRole("button", "Welcome to Notes"))
	a.Click(openBtn)
	a.AnswerFileDialog(path)
	a.ExpectValue(editor, "Todo\n- ship it\n")
	a.ExpectCount(telatest.ByRole("button", "todo"), 1)
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

// A desktop without file dialogs (Linux without the XDG Desktop Portal)
// says what to install.
func TestNoFileDialogs(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.FileDialogs)
	a.Click(openBtn)
	a.ExpectText("This desktop has no file dialogs")
	a.ExpectText("Install xdg-desktop-portal")
	a.ExpectNoText("Could not show")
}

// On a desktop that draws no title bars (GNOME on Wayland), Preferences
// draws its own, and its close button closes the window; where the OS
// draws one, it has none. Tela's fallback title bar (ADR-238) never shows:
// both windows have a bar of their own.
func TestPreferencesTitleBar(t *testing.T) {
	sandbox(t)
	closeBtn := telatest.ByRole("button", "Close")

	a := mount(t)
	prefs := openPreferences(a)
	prefs.ExpectWindowState(telatest.ClientDecorations(false))
	prefs.ExpectCount(closeBtn, 0)

	g := mount(t, telatest.NoServerDecorations())
	prefs = openPreferences(g)
	prefs.ExpectWindowState(telatest.ClientDecorations(true))
	prefs.ExpectRole("group", "Window controls")
	prefs.ExpectCount(telatest.ByRole("button", "Minimize"), 0)
	prefs.ExpectCount(closeBtn, 1) // kit's, not the fallback's too
	g.ExpectCount(closeBtn, 1)
	prefs.Click(closeBtn)
	prefs.ExpectClosed()
	g.ExpectWindowCount(1)
	g.ExpectRunning()
}

// The title bar of Preferences, where the app draws it, and the modifier of
// the shortcuts follow Notes.Look, not the OS the test runs on: Windows has
// the close button at the end and Ctrl, macOS at the start and Cmd.
func TestPreferencesLook(t *testing.T) {
	sandbox(t)
	closeBtn := telatest.ByRole("button", "Close")
	for _, c := range []struct {
		look     kit.Variant
		atEnd    bool
		mod, not string
	}{{kit.Windows, true, "Ctrl S", "Cmd S"}, {kit.MacOS, false, "Cmd S", "Ctrl S"}} {
		prefs := openPreferences(mountLook(t, c.look, telatest.NoServerDecorations()), "Shortcuts")
		if b := prefs.Bounds(closeBtn); (b.X > 270) != c.atEnd {
			t.Errorf("%s: Close at x=%v, want it at the end: %v", c.look, b.X, c.atEnd)
		}
		prefs.ExpectText(c.mod)
		prefs.ExpectNoText(c.not)
	}
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
	a.ExpectText("In your library")
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
	a.Press("ControlOrMeta+n")
	a.ExpectRole("button", "Untitled note")
	a.Click(editor)
	a.Type("Shortcuts")
	a.Press("ControlOrMeta+s")
	if d := a.FileDialogs(); len(d) != 1 || d[0].Kind != telatest.SaveFileDialog {
		t.Fatalf("Ctrl+S opened %+v, want a save dialog", d)
	}
	a.AnswerFileDialog() // cancel
	a.Press("ControlOrMeta+o")
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
	prefs := openPreferences(a, "Editor")
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
	a.ExpectCount(editor.Filter(telatest.HasClass("text-[18px]")), 1)
	prefs.Click(telatest.ByRole("radio", "Loose"))
	a.ExpectCount(editor.Filter(telatest.HasClass("leading-loose")), 1)

	// Closing the window is the app's state: OnClose sets it to false.
	prefs.RequestClose()
	prefs.ExpectClosed()
	a.ExpectWindowCount(1)

	// The button opens it again, and what was set is still set.
	prefs = openPreferences(a, "Editor")
	prefs.Expect(wrap, telatest.Checked(false))
	prefs.Expect(telatest.ByRole("radio", "Large"), telatest.Checked(true))
	prefs.Expect(telatest.ByRole("radio", "Loose"), telatest.Checked(true))
}

func TestSettingsSurviveARestart(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a, "General")
	prefs.Click(telatest.ByRole("switch", "Keep running in the tray"))
	prefs.Click(telatest.ByRole("switch", "Notify when a file is saved"))
	prefs.Click(telatest.ByRole("button", "Editor"))
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
		t.Errorf("settings.json = %s, want the settings", data)
	}

	b := mount(t)
	prefs = openPreferences(b, "General")
	prefs.Expect(telatest.ByRole("switch", "Keep running in the tray"), telatest.Checked(true))
	prefs.Expect(telatest.ByRole("switch", "Notify when a file is saved"), telatest.Checked(false))
	prefs.Click(telatest.ByRole("button", "Editor"))
	prefs.Expect(telatest.ByRole("radio", "Small"), telatest.Checked(true))
	b.ExpectCount(editor.Filter(telatest.HasClass("text-[13px]")), 1)
}

// The Preferences window opens where it was closed.
func TestPreferencesPlacementIsRemembered(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a)
	prefs.MoveWindow(120, 90)
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
	a.MoveWindow(40, 60)
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
	if want := []string{"New note", "Hide Notes", "Preferences…", "-", "Quit"}; !slices.Equal(labels, want) {
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

	// The hidden window still builds: its tray menu follows it.
	if label := a.Tray().Menu[1].Label; label != "Show Notes" {
		t.Errorf("tray item of the hidden window = %q, want Show Notes", label)
	}
	a.ClickTrayItem("Show Notes")
	a.ExpectWindowState(telatest.Hidden(false))
	a.ExpectValue(editor, "Still here")
	if label := a.Tray().Menu[1].Label; label != "Hide Notes" {
		t.Errorf("tray item of the shown window = %q, want Hide Notes", label)
	}

	// A click on the icon toggles the window, and so does the menu item.
	a.ClickTray()
	a.ExpectWindowState(telatest.Hidden(true))
	a.ClickTray()
	a.ExpectWindowState(telatest.Hidden(false))
	a.ClickTrayItem("Hide Notes")
	a.ExpectWindowState(telatest.Hidden(true))
	a.ClickTrayItem("Show Notes")
	a.ExpectWindowState(telatest.Hidden(false))

	// The tray menu works on a hidden window too: Preferences… shows it
	// and opens Preferences, New note shows it with the new note.
	a.RequestClose()
	a.ExpectWindowState(telatest.Hidden(true))
	prefs := a.WaitForWindow(func() { a.ClickTrayItem("Preferences…") })
	prefs.ExpectTitle("Preferences")
	a.ExpectWindowState(telatest.Hidden(false))
	prefs.RequestClose()
	prefs.ExpectClosed()
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

// With word wrap off the editor does not wrap its lines, and a line longer
// than the editor scrolls horizontally: typing keeps the caret in view,
// Home goes back to the start of the line, and the wheel (here a
// touchpad's horizontal delta) scrolls it.
func TestNoWordWrapScrollsHorizontally(t *testing.T) {
	long := strings.Repeat("a line that goes on without wrapping ", 6)
	t.Run("off", func(t *testing.T) {
		sandbox(t)
		setSetting(t, keyWordWrap, false)
		// Home and End go to the line's edges with the Linux and Windows
		// bindings (a Mac uses Command+Left and Right).
		a := mount(t, telatest.PlatformKeys("linux"))
		a.ExpectCount(editor.Filter(telatest.HasClass("text-nowrap")), 1)
		typeNote(a, long)
		a.ExpectValue(editor, long)

		// The first line of the editor, away from its scrollbars.
		b := a.Bounds(editor)
		firstLine := func() *image.RGBA {
			shot := a.Screenshot()
			r := image.Rect(int(b.X), int(b.Y), int(b.Right()), int(b.Y)+60)
			out := image.NewRGBA(r)
			draw.Draw(out, r, shot, r.Min, draw.Src)
			return out
		}
		differ := func(p, q *image.RGBA) int {
			n := 0
			for i := 0; i+3 < len(p.Pix); i += 4 {
				if !bytes.Equal(p.Pix[i:i+4], q.Pix[i:i+4]) {
					n++
				}
			}
			return n
		}
		end := firstLine()
		a.Press("Home")
		start := firstLine()
		// More than the caret moved: the text did.
		if n := differ(end, start); n < 500 {
			t.Fatalf("Home changed %d pixels of the first line: the line did not scroll", n)
		}
		a.Press("End")
		if n := differ(end, firstLine()); n != 0 {
			t.Errorf("End: %d pixels differ from the end of the line after typing", n)
		}
		// Scrolling back to the start shows what Home showed, but for the
		// caret, which stays at the end of the line.
		a.Scroll(editor, -2000, 0)
		if n := differ(start, firstLine()); n > 60 {
			t.Errorf("scrolled to the start: %d pixels differ from the start after Home", n)
		}
	})
	// With word wrap on, the line wraps and there is nothing to scroll.
	t.Run("on", func(t *testing.T) {
		sandbox(t)
		w := mount(t)
		w.ExpectCount(editor.Filter(telatest.HasClass("text-nowrap")), 0)
		typeNote(w, long)
		before := w.Screenshot()
		b := w.Bounds(editor)
		w.Scroll(editor, 300, 0)
		after := w.Screenshot()
		r := image.Rect(int(b.X), int(b.Y), int(b.Right()), int(b.Y)+60)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
					t.Fatalf("a wrapping editor moved at (%d, %d) on a horizontal scroll", x, y)
				}
			}
		}
	})
}
