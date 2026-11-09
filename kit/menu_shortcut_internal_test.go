package kit

import "testing"

func TestFormatShortcut(t *testing.T) {
	for _, c := range []struct {
		in          string
		mac         bool
		shown, keys string
	}{
		// The glyphs of a Mac are read in any flavor and shown as written.
		{"⌘T", false, "⌘T", "Meta+T"},
		{"⇧⌘Z", false, "⇧⌘Z", "Shift+Meta+Z"},
		{"⌘⇧Z", true, "⌘⇧Z", "Shift+Meta+Z"},
		{"⌃⌥⌘K", false, "⌃⌥⌘K", "Control+Alt+Meta+K"},
		{"⌘↩", false, "⌘↩", "Meta+Enter"},
		{"⌘F5", false, "⌘F5", "Meta+F5"},
		{"⌘[", false, "⌘[", "Meta+["},
		{"⌥␣", false, "⌥␣", "Alt+Space"},
		// The usual spelling, shown as written.
		{"Ctrl+Shift+T", true, "Ctrl+Shift+T", "Control+Shift+T"},
		{"Shift+Cmd+P", false, "Shift+Cmd+P", "Shift+Meta+P"},
		{"Ctrl++", false, "Ctrl++", "Control+Plus"},
		// CmdOrCtrl follows the flavor, shown either way.
		{"CmdOrCtrl+Shift+Z", true, "⇧⌘Z", "Shift+Meta+Z"},
		{"CmdOrCtrl+Shift+Z", false, "Ctrl+Shift+Z", "Control+Shift+Z"},
		{"commandorcontrol+n", true, "⌘N", "Meta+N"},
		{"CmdOrCtrl+Enter", true, "⌘↩", "Meta+Enter"},
		// Not a key combination: shown, no keys.
		{"Soon", false, "Soon", ""},
		{"⌘", false, "⌘", ""},
		{"⌘Tab twice", false, "⌘Tab twice", ""},
	} {
		shown, keys := formatShortcut(c.in, c.mac)
		if shown != c.shown || keys != c.keys {
			t.Errorf("formatShortcut(%q, mac %v) = %q, %q; want %q, %q", c.in, c.mac, shown, keys, c.shown, c.keys)
		}
	}
}

func TestNativeAcceleratorFollowsTheFlavor(t *testing.T) {
	if got := nativeAccelerator("CmdOrCtrl+S", true); got != "Super+S" {
		t.Errorf("mac: %q", got)
	}
	if got := nativeAccelerator("CmdOrCtrl+S", false); got != "Ctrl+S" {
		t.Errorf("not mac: %q", got)
	}
	if got := nativeAccelerator("⌘T", true); got != "⌘T" {
		t.Errorf("glyphs: %q", got)
	}
}
