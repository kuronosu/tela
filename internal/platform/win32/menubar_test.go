package win32

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the keyboard and dark drawing of the menu bar
// (menu.go, keyboard.go, menubar_dark.go).

func TestAutoMnemonics(t *testing.T) {
	items := []platform.MenuItem{
		{ID: 1, Label: "File"},
		{ID: 2, Label: "Edit"},
		{ID: 3, Label: "Exit"}, // E is taken: x
		{Separator: true},
		{ID: 4, Label: "Fe"},                  // F and e are taken: none
		{ID: 5, Label: "Off", Disabled: true}, // disabled: none
		{ID: 6, Label: "ファイル"},                // no Latin character
		{ID: 7, Label: "…2go"},                // a digit is a key
		{ID: 8, Label: "Édition"},             // an accented capital
		{ID: 9, Label: "ee"},                  // e and é are different keys, but e is taken
	}
	want := []int{0, 0, 1, -1, -1, -1, -1, len("…"), 0, -1}
	got := autoMnemonics(items)
	if !slices.Equal(got, want) {
		t.Fatalf("autoMnemonics = %v, want %v", got, want)
	}
	if autoMnemonics(nil) != nil {
		t.Errorf("an empty menu has no mnemonics")
	}
}

func TestBuildBarMenu(t *testing.T) {
	entries, cmds, keys := buildBarMenu([]platform.MenuItem{
		{Label: "File", Submenu: []platform.MenuItem{
			{ID: 10, Label: "Open…", Accelerator: "Ctrl+O"},
			{ID: 11, Label: "Save & close"},
			{ID: 12, Label: "Save as", Accelerator: "Ctrl+Shift+S"},
		}},
		{ID: 20, Label: "Help & about"},
		{ID: 21, Label: "Fun"},
	})
	if !slices.Equal(cmds, []platform.MenuItemID{10, 11, 12, 20, 21}) {
		t.Fatalf("cmds = %v", cmds)
	}
	if !slices.Equal(keys, []rune{'f', 'h', 'u'}) {
		t.Errorf("top-level mnemonics = %q, want fhu", keys)
	}
	if entries[0].label != "&File" || entries[1].label != "&Help && about" || entries[2].label != "F&un" {
		t.Errorf("bar labels = %q, %q, %q", entries[0].label, entries[1].label, entries[2].label)
	}
	// Each level has its own keys.
	sub := entries[0].sub
	if sub[0].label != "&Open…\tCtrl+O" || sub[1].label != "&Save && close" || sub[2].label != "S&ave as\tCtrl+Shift+S" {
		t.Errorf("submenu labels = %q, %q, %q", sub[0].label, sub[1].label, sub[2].label)
	}
	// A tray or popup menu keeps its labels plain.
	plain, _ := buildMenu([]platform.MenuItem{{ID: 1, Label: "File"}})
	if plain[0].label != "File" {
		t.Errorf("buildMenu label = %q", plain[0].label)
	}
}

func TestHasMnemonic(t *testing.T) {
	keys := []rune{'f', 'h'}
	for _, tc := range []struct {
		c    uintptr
		want bool
	}{{'f', true}, {'F', true}, {'h', true}, {'x', false}, {' ', false}, {0, false}} {
		if got := hasMnemonic(keys, tc.c); got != tc.want {
			t.Errorf("hasMnemonic(%q) = %v, want %v", rune(tc.c), got, tc.want)
		}
	}
}

func TestMenuKeys(t *testing.T) {
	type key struct {
		vk                 uint32
		repeat, ext, altGr bool
		ctrl, shift, alt   bool
	}
	alt := key{vk: vkMenu}
	for _, tc := range []struct {
		name string
		down []key
		up   uint32
		want bool
	}{
		{"lone Alt", []key{alt}, vkMenu, true},
		{"Alt held until it repeats", []key{alt, {vk: vkMenu, repeat: true, alt: true}}, vkMenu, true},
		{"right Alt without AltGr", []key{{vk: vkMenu, ext: true}}, vkMenu, true},
		{"AltGr", []key{{vk: vkMenu, ext: true, altGr: true}}, vkMenu, false},
		{"Alt with Ctrl", []key{{vk: vkMenu, ctrl: true}}, vkMenu, false},
		{"Alt and a key", []key{alt, {vk: 'F', alt: true}}, vkMenu, false},
		{"Alt+Shift", []key{alt, {vk: vkShift, alt: true}}, vkMenu, false},
		{"Alt+Tab", []key{alt, {vk: 0x09, alt: true}}, vkMenu, false},
		{"F10", []key{{vk: vkF10}}, vkF10, true},
		{"F10 repeating", []key{{vk: vkF10}, {vk: vkF10, repeat: true}}, vkF10, true},
		{"Shift+F10", []key{{vk: vkF10, shift: true}}, vkF10, false},
		{"Ctrl+F10", []key{{vk: vkF10, ctrl: true}}, vkF10, false},
		{"Alt+F10", []key{alt, {vk: vkF10, alt: true}}, vkF10, false},
		{"another key's release", []key{{vk: vkF10}}, vkMenu, false},
		{"Alt after F10", []key{{vk: vkF10}, alt}, vkMenu, true},
	} {
		var m menuKeys
		for _, k := range tc.down {
			m.down(k.vk, k.repeat, k.ext, k.altGr, k.ctrl, k.shift, k.alt)
		}
		if got := m.up(tc.up); got != tc.want {
			t.Errorf("%s: up = %v, want %v", tc.name, got, tc.want)
		}
		if m.up(tc.up) {
			t.Errorf("%s: a second release opened the bar again", tc.name)
		}
	}
	// A mouse button or a lost focus disarms it.
	var m menuKeys
	m.down(vkMenu, false, false, false, false, false, false)
	m.reset()
	if m.up(vkMenu) {
		t.Errorf("reset did not disarm the bar")
	}
}
