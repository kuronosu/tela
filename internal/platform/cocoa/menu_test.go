package cocoa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// modsText writes NSEventModifierFlags as the menu shows them.
func modsText(m uint64) string {
	var b strings.Builder
	for _, f := range []struct {
		bit uint64
		s   string
	}{{keyboard.MacFlagControl, "⌃"}, {keyboard.MacFlagOption, "⌥"}, {keyboard.MacFlagShift, "⇧"}, {keyboard.MacFlagCommand, "⌘"}} {
		if m&f.bit != 0 {
			b.WriteString(f.s)
		}
	}
	return b.String()
}

// planLines writes a plan one entry per line, indented by depth.
func planLines(p []menuEntry, depth int, out *[]string) {
	for _, e := range p {
		ind := strings.Repeat("  ", depth)
		if e.Separator {
			*out = append(*out, ind+"---")
			continue
		}
		s := ind + e.Title
		if e.Action != "" {
			s += " [" + e.Action
			if e.App {
				s += fmt.Sprintf(" #%d", e.Tag)
			}
			s += "]"
		}
		if e.Key != "" {
			s += fmt.Sprintf(" %s%q", modsText(e.Mods), e.Key)
		}
		if e.Checkable {
			s += fmt.Sprintf(" checked=%v", e.Checked)
		}
		if e.Special != specialNone {
			s += fmt.Sprintf(" (special %d)", e.Special)
		}
		*out = append(*out, s)
		planLines(e.Submenu, depth+1, out)
	}
}

func comparePlan(t *testing.T, got []menuEntry, want string) {
	t.Helper()
	var lines []string
	planLines(got, 0, &lines)
	g := strings.Join(lines, "\n")
	w := strings.TrimSpace(want)
	if g != w {
		t.Errorf("plan:\n%s\nwant:\n%s", g, w)
	}
}

func TestDefaultMenuBarPlan(t *testing.T) {
	comparePlan(t, defaultMenuBarPlan("Demo"), `
Demo (special 1)
  About Demo [orderFrontStandardAboutPanel:]
  ---
  Services (special 2)
  ---
  Hide Demo [hide:] ⌘"h"
  Hide Others [hideOtherApplications:] ⌥⌘"h"
  Show All [unhideAllApplications:]
  ---
  Quit Demo [terminate:] ⌘"q"
Edit (special 3)
  Undo [undo:] ⌘"z"
  Redo [redo:] ⇧⌘"z"
  ---
  Cut [cut:] ⌘"x"
  Copy [copy:] ⌘"c"
  Paste [paste:] ⌘"v"
  Select All [selectAll:] ⌘"a"
Window (special 4)
  Minimize [performMiniaturize:] ⌘"m"
  Zoom [performZoom:]
  ---
  Bring All to Front [arrangeInFront:]`)
}

func TestAppMenuBarPlan(t *testing.T) {
	menu := []platform.MenuItem{
		{Label: "File", Submenu: []platform.MenuItem{
			{ID: 1, Label: "New", Accelerator: "CmdOrCtrl+N"},
			{ID: 2, Label: "Save As…", Accelerator: "CommandOrControl+Shift+S"},
			{Separator: true},
			{ID: 3, Label: "Autosave", Checkable: true, Checked: true},
			{ID: 4, Label: "Print", Disabled: true, Accelerator: "Cmd+P"},
			{ID: 5, Role: platform.RoleClose},
			{Label: "Recent", Disabled: true, Submenu: []platform.MenuItem{{ID: 6, Label: "a.txt"}}},
		}},
		{Label: "Edit", Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{
			{ID: 10, Role: platform.RoleUndo},
			{ID: 11, Label: "Rehacer", Role: platform.RoleRedo, Accelerator: "Ctrl+Y"},
			{ID: 12, Role: platform.RoleCopy, Disabled: true},
			{ID: 13, Label: "Find", Accelerator: "F3"},
			{ID: 14, Label: "Bad", Accelerator: "Ctrl+Nope"},
			{ID: 15, Label: "Media", Accelerator: "MediaPlayPause"},
		}},
		{Label: "View", Role: platform.RoleWindowMenu, Submenu: []platform.MenuItem{{ID: 20, Role: platform.RoleMinimize}, {ID: 21, Role: platform.RoleZoom}}},
		{Label: "Help", Role: platform.RoleHelpMenu, Submenu: []platform.MenuItem{{ID: 30, Label: "Docs"}}},
		{ID: 40, Label: "Loose"},
	}
	appItems := []platform.MenuItem{{ID: 50, Label: "Settings…", Accelerator: "CmdOrCtrl+,"}}
	platform.CheckAppMenu(menu, appItems)
	comparePlan(t, appMenuBarPlan("Demo", menu, appItems), `
Demo (special 1)
  About Demo [orderFrontStandardAboutPanel:]
  ---
  Settings… [appMenuItem: #50] ⌘","
  ---
  Services (special 2)
  ---
  Hide Demo [hide:] ⌘"h"
  Hide Others [hideOtherApplications:] ⌥⌘"h"
  Show All [unhideAllApplications:]
  ---
  Quit Demo [terminate:] ⌘"q"
File
  New [appMenuItem: #1] ⌘"n"
  Save As… [appMenuItem: #2] ⇧⌘"s"
  ---
  Autosave [appMenuItem: #3] checked=true
  Print ⌘"p"
  Close Window [performClose:] ⌘"w"
  Recent
    a.txt
Edit (special 3)
  Undo [undo:] ⌘"z"
  Rehacer [redo:] ⌃"y"
  Copy ⌘"c"
  Find [appMenuItem: #13] "\uf706"
  Bad [appMenuItem: #14]
  Media [appMenuItem: #15]
View (special 4)
  Minimize [performMiniaturize:] ⌘"m"
  Zoom [performZoom:]
Help (special 5)
  Docs [appMenuItem: #30]
Loose
  Loose [appMenuItem: #40]`)
}

func TestPopupPlan(t *testing.T) {
	comparePlan(t, popupPlan([]platform.MenuItem{
		{ID: 1, Label: "Open", Accelerator: "CmdOrCtrl+O"},
		{ID: 2, Role: platform.RoleCopy},
		{Separator: true},
		{Label: "More", Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{{ID: 3, Label: "Off", Disabled: true}, {ID: 4, Label: "On", Checkable: true}}},
	}), `
Open [popupMenuItem: #1] ⌘"o"
Copy [popupMenuItem: #2]
---
More
  Off
  On [popupMenuItem: #4] checked=false`)
}

func TestKeyEquivalent(t *testing.T) {
	for _, c := range []struct {
		accel string
		key   string
		mods  uint64
	}{
		{"CmdOrCtrl+S", "s", keyboard.MacFlagCommand},
		{"cmdorctrl+shift+s", "s", keyboard.MacFlagCommand | keyboard.MacFlagShift},
		{"Ctrl+Alt+Super+K", "k", keyboard.MacFlagCommand | keyboard.MacFlagControl | keyboard.MacFlagOption},
		{"Cmd++", "+", keyboard.MacFlagCommand},
		{"Cmd+Plus", "+", keyboard.MacFlagCommand},
		{"Cmd+Space", " ", keyboard.MacFlagCommand},
		{"F5", "\uf708", 0},
		{"Shift+F24", "\uf71b", keyboard.MacFlagShift},
		{"Cmd+Up", "\uf700", keyboard.MacFlagCommand},
		{"Cmd+PageDown", "\uf72d", keyboard.MacFlagCommand},
		{"Cmd+Backspace", "\b", keyboard.MacFlagCommand},
		{"Cmd+Delete", "\uf728", keyboard.MacFlagCommand},
		{"Cmd+Return", "\r", keyboard.MacFlagCommand},
		{"Escape", "\x1b", 0},
		{"Cmd+ñ", "ñ", keyboard.MacFlagCommand},
		{"MediaPlayPause", "", 0},
		{"Ctrl+Nope", "", 0},
		{"", "", 0},
		{"Ctrl+", "", 0},
	} {
		key, mods := accelKeyEquivalent(c.accel)
		if key != c.key || mods != c.mods {
			t.Errorf("accelKeyEquivalent(%q) = %q %s, want %q %s", c.accel, key, modsText(mods), c.key, modsText(c.mods))
		}
	}
}

func TestEditKeys(t *testing.T) {
	want := map[platform.MenuRole]string{
		platform.RoleUndo:      "KeyZ z z Super",
		platform.RoleRedo:      "KeyZ Z Z Shift|Super",
		platform.RoleCut:       "KeyX x x Super",
		platform.RoleCopy:      "KeyC c c Super",
		platform.RolePaste:     "KeyV v v Super",
		platform.RoleSelectAll: "KeyA a a Super",
	}
	for r := platform.RoleUndo; r <= platform.RoleSelectAll; r++ {
		e, ok := keyboard.MacKeyDown(editKeys[r])
		if !ok {
			t.Errorf("%s: no key press", r)
			continue
		}
		var mods []string
		if e.Mods&platform.ModShift != 0 {
			mods = append(mods, "Shift")
		}
		if e.Mods&platform.ModSuper != 0 {
			mods = append(mods, "Super")
		}
		got := fmt.Sprintf("%v %s %s %s", e.Code, e.Key, e.Text, strings.Join(mods, "|"))
		if got != want[r] {
			t.Errorf("%s: standard key %s, want %s", r, got, want[r])
		}
		if !platform.FlavorMac.CmdOrCtrl(e.Mods&platform.ModCtrl != 0, e.Mods&platform.ModSuper != 0) {
			t.Errorf("%s: the standard key is not CmdOrCtrl under the Mac flavor", r)
		}
		if !isStandardKey(r, e) {
			t.Errorf("%s: isStandardKey of its own key is false", r)
		}
		if sel, ok := editRoleOf(roleActions[r]); !ok || sel != r {
			t.Errorf("editRoleOf(%q) = %v, %v", roleActions[r], sel, ok)
		}
	}
	other, _ := keyboard.MacKeyDown(keyboard.MacKey{KeyCode: 0x08, Flags: keyboard.MacFlagCommand | keyboard.MacFlagShift, Characters: "C", CharactersIgnoringModifiers: "C"})
	if isStandardKey(platform.RoleCopy, other) {
		t.Errorf("Shift+Cmd+C counts as copy's standard key")
	}
}

func TestKeyEquivalentMatches(t *testing.T) {
	cmd, shift, opt := keyboard.MacFlagCommand, keyboard.MacFlagShift, keyboard.MacFlagOption
	caps, fn := keyboard.MacFlagCapsLock, uint64(1<<23)
	for _, c := range []struct {
		key   string
		mask  uint64
		chars string
		flags uint64
		want  bool
	}{
		{"k", cmd, "k", cmd, true},
		{"k", cmd, "k", cmd | 0x100 | 0x8, true}, // device bits
		{"k", cmd, "K", cmd | caps, true},        // Caps Lock
		{"k", cmd, "k", cmd | shift, false},
		{"z", cmd | shift, "Z", cmd | shift, true}, // AppKit's Shift+Cmd+Z
		{"z", cmd, "Z", cmd | shift, false},
		{"Z", cmd, "Z", cmd | shift, true}, // an upper-case key implies Shift
		{"h", cmd | opt, "h", cmd | opt, true},
		{"h", cmd, "h", cmd | opt, false},
		{"", 0, "", fn, true}, // F5: Fn does not count
		{",", cmd, ",", cmd, true},
		{"", cmd, "k", cmd, false},
		{"k", cmd, "", cmd, false},
		{"k", cmd, "j", cmd, false},
	} {
		if got := keyEquivalentMatches(c.key, c.mask, c.chars, c.flags); got != c.want {
			t.Errorf("keyEquivalentMatches(%q, %s, %q, %#x) = %v, want %v", c.key, modsText(c.mask), c.chars, c.flags, got, c.want)
		}
	}
}

// macAccelerator turns CmdOrCtrl (any case, either spelling) into Command,
// also after a rune whose lower case is longer (U+0130), which once moved
// the splice.
func TestMacAccelerator(t *testing.T) {
	for in, want := range map[string]platform.Modifiers{
		"CmdOrCtrl+K":            platform.ModSuper,
		"cmdorctrl+shift+k":      platform.ModSuper | platform.ModShift,
		"CommandOrControl+Alt+K": platform.ModSuper | platform.ModAlt,
		"Ctrl+K":                 platform.ModCtrl,
	} {
		a, err := macAccelerator(in)
		if err != nil || a.Mods != want {
			t.Errorf("macAccelerator(%q) = %+v, %v; want mods %v", in, a, err, want)
		}
	}
	// The splice lands on the right bytes after a multi-byte rune: the
	// result names Cmd and keeps the rest intact.
	if a, err := macAccelerator("İ+CmdOrCtrl+K"); err == nil && a.Mods&platform.ModSuper == 0 {
		t.Errorf("macAccelerator(İ+CmdOrCtrl+K) = %+v: CmdOrCtrl was not turned into Command", a)
	}
}
