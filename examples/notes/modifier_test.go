package main

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The shortcut texts name the primary modifier of the platform whose key
// conventions the window follows: Cmd on a Mac, Ctrl elsewhere. They are the
// Keyboard shortcuts note of the first run and the list in Preferences, and
// both list every key the window answers to (Cmd+W closes the window,
// Cmd+1 to 3 choose a view).
func TestShortcutTextsFollowThePlatformKeys(t *testing.T) {
	for _, c := range []struct {
		goos, mod, other string
	}{
		{"darwin", "Cmd", "Ctrl"},
		{"linux", "Ctrl", "Cmd"},
		{"windows", "Ctrl", "Cmd"},
	} {
		t.Run(c.goos, func(t *testing.T) {
			sandbox(t)
			a := mount(t, telatest.PlatformKeys(c.goos))
			a.Click(card("Keyboard shortcuts"))
			text := a.AriaSnapshot()
			for _, want := range []string{
				c.mod + "+N makes a note", c.mod + "+O opens a file", c.mod + "+S saves", c.mod + "+, opens Preferences",
				c.mod + "+W closes the window", c.mod + "+1, " + c.mod + "+2 and " + c.mod + "+3 show all notes",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("the Keyboard shortcuts note lacks %q:\n%s", want, text)
				}
			}
			if strings.Contains(text, c.other+"+N") {
				t.Errorf("the Keyboard shortcuts note names %s on %s", c.other, c.goos)
			}

			prefs := openPreferences(a, "Shortcuts")
			for _, key := range []string{c.mod + " N", c.mod + " O", c.mod + " S", c.mod + " ,", c.mod + " Shift D", c.mod + " W", c.mod + " 1 / 2 / 3"} {
				prefs.ExpectText(key)
			}
			prefs.ExpectNoText(c.other + " N")
		})
	}
}
