package main

import (
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

var quickSwitch = telatest.ByRole("switch", "Quick note from any application")

// The keys that Notes asks for parse, and read back as they are written.
func TestQuickNoteKeysParse(t *testing.T) {
	got, err := desktop.ParseKeys(quickNoteKeys)
	if err != nil || got != quickNoteKeys {
		t.Fatalf("ParseKeys(%q) = %q, %v", quickNoteKeys, got, err)
	}
}

func TestQuickNoteIsRegistered(t *testing.T) {
	sandbox(t)
	a := mount(t)
	got := a.GlobalShortcuts()
	if len(got) != 1 {
		t.Fatalf("global shortcuts = %+v, want one", got)
	}
	if s := got[0]; s.ID != "quick-note" || s.Keys != "Ctrl+Alt+N" || s.Status != desktop.ShortcutBound || s.Description == "" {
		t.Errorf("shortcut = %+v, want quick-note on Ctrl+Alt+N, bound, with a description", s)
	}
}

// Pressing it from another application makes a note with the cursor in its
// title, and the window comes forward, also from the tray.
func TestQuickNoteMakesANote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.PressShortcut("quick-note")
	a.ExpectRole("button", "Untitled note")
	a.ExpectFocused(titleBox)
	a.Type("From anywhere")
	a.ExpectRole("button", "From anywhere")

	// From the tray: the hidden window shows and has the new note.
	a.ClickTrayItem("Hide Notes")
	a.ExpectWindowState(telatest.Hidden(true))
	a.PressShortcut("quick-note")
	a.ExpectWindowState(telatest.Hidden(false))
	a.ExpectFocused(titleBox)
	a.ExpectValue(titleBox, "")
	a.ExpectCount(telatest.ByRole("button", "Untitled note"), 1)
	a.Click(railItem("All notes"))
	a.ExpectCount(telatest.ByRole("button", "From anywhere"), 1)

	// And from a minimized window.
	a.Click(telatest.ByRole("button", "Minimize"))
	a.ExpectWindowState(telatest.Minimized(true))
	a.PressShortcut("quick-note")
	a.ExpectWindowState(telatest.Minimized(false))
}

// The note goes where the user is, as New note does.
func TestQuickNoteGoesToTheChosenFolder(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(railItem("Ideas"))
	a.PressShortcut("quick-note")
	a.Type("x")
	a.Click(railItem("Ideas"))
	a.ExpectCount(card("x"), 1)
	a.ExpectCount(card("Groceries"), 0)
}

// The setting turns the shortcut on and off, and it is on by default.
func TestQuickNoteSetting(t *testing.T) {
	sandbox(t)
	setSetting(t, keyQuickNote, false)
	a := mount(t)
	if got := a.GlobalShortcuts(); len(got) != 0 {
		t.Fatalf("the shortcut is registered although the setting is off: %+v", got)
	}

	prefs := openPreferences(a, "Shortcuts")
	prefs.ExpectText("Off.")
	prefs.Click(quickSwitch)
	prefs.Expect(quickSwitch, telatest.Checked(true))
	if got := a.GlobalShortcuts(); len(got) != 1 || got[0].Status != desktop.ShortcutBound {
		t.Fatalf("turning it on did not register it: %+v", got)
	}
	prefs.ExpectText("On. Press Ctrl+Alt+N from any application")
	a.PressShortcut("quick-note")
	a.ExpectRole("button", "Untitled note")

	prefs.Click(quickSwitch)
	prefs.ExpectText("Off.")
	if got := a.GlobalShortcuts(); len(got) != 0 {
		t.Errorf("turning it off left the shortcut registered: %+v", got)
	}
}

// What the OS says about the shortcut shows in Preferences.
func TestQuickNoteStatusInPreferences(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a, "Shortcuts")
	prefs.ExpectText("On. Press Ctrl+Alt+N from any application")

	// The desktop (the Wayland portal) binds other keys than the ones asked.
	a.SetShortcutStatus("quick-note", desktop.ShortcutBound, "Ctrl+Shift+N")
	prefs.ExpectText("On. Press Ctrl+Shift+N from any application")
	prefs.ExpectText("Ctrl+Shift+N")

	a.SetShortcutStatus("quick-note", desktop.ShortcutTaken, "")
	prefs.ExpectText("Taken by another application")
	prefs.ExpectNoText("Ctrl+Shift+N") // the chip goes back to the keys asked for

	a.SetShortcutStatus("quick-note", desktop.ShortcutRefused, "")
	prefs.ExpectText("Refused: the desktop did not bind it")

	a.SetShortcutStatus("quick-note", desktop.ShortcutRevoked, "")
	prefs.ExpectText("Removed in the desktop's shortcut settings")

	a.SetShortcutStatus("quick-note", desktop.ShortcutFailed, "")
	prefs.ExpectText("Could not be registered")

	a.SetShortcutStatus("quick-note", desktop.ShortcutBound, "")
	prefs.ExpectText("On. Press Ctrl+Alt+N from any application")
}

// A desktop with no global shortcuts says so.
func TestQuickNoteNotAvailable(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.GlobalShortcuts)
	prefs := openPreferences(a, "Shortcuts")
	prefs.Click(quickSwitch) // off
	prefs.Click(quickSwitch) // and on again: the desktop is asked again
	prefs.ExpectText("Not available on this desktop")
	prefs.ExpectNoText("On. Press")
}

// Preferences, Shortcuts, with the global shortcut and its state.
func TestGoldenShortcuts(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a, "Shortcuts")
	prefs.MouseMove(300, 420)
	prefs.Golden("preferences_shortcuts.png")

	a.SetShortcutStatus("quick-note", desktop.ShortcutTaken, "")
	prefs.ExpectText("Taken by another application")
	prefs.Golden("preferences_shortcuts_taken.png")
}
