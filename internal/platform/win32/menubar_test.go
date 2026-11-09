package win32

import (
	"slices"
	"testing"
	"unicode"

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
		{ID: 6, Label: "ファイル"},                // no character of its own: a Latin key appended, the first free
		{ID: 7, Label: "…2go"},                // a digit is a key
		{ID: 8, Label: "Édition"},             // an accented capital
		{ID: 9, Label: "ee"},                  // e and é are different keys, but e is taken
		{ID: 10, Label: "Bold"},               // its own B, although the appended key comes first
	}
	at := func(off int) mnemonic { return mnemonic{at: off} }
	none := mnemonic{at: -1}
	want := []mnemonic{at(0), at(0), at(1), none, none, none, {at: -1, key: 'A'}, at(len("…")), at(0), none, at(0)}
	got := autoMnemonics(items)
	if !slices.Equal(got, want) {
		t.Fatalf("autoMnemonics = %v, want %v", got, want)
	}
	if autoMnemonics(nil) != nil {
		t.Errorf("an empty menu has no mnemonics")
	}
}

// Labels in other scripts follow Windows' convention for them (ADR-252):
// scripts typed directly mark a letter of their own, those typed through
// an IME get a Latin key in parentheses (the role's English one when
// free), so every top-level menu opens with Alt and a key.
func TestMnemonicScripts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		items  []platform.MenuItem
		labels []string
		keys   string
	}{
		{"Russian", []platform.MenuItem{{Label: "Файл"}, {Label: "Правка"}, {Label: "Вид"}, {Label: "Выход"}},
			[]string{"&Файл", "&Правка", "&Вид", "В&ыход"}, "фпвы"},
		{"Greek", []platform.MenuItem{{Label: "Αρχείο"}, {Label: "Επεξεργασία"}},
			[]string{"&Αρχείο", "&Επεξεργασία"}, "αε"},
		{"Arabic", []platform.MenuItem{{Label: "ملف"}, {Label: "تحرير"}, {Label: "عرض"}},
			[]string{"&ملف", "&تحرير", "&عرض"}, "متع"},
		{"Hebrew", []platform.MenuItem{{Label: "קובץ"}, {Label: "עריכה"}},
			[]string{"&קובץ", "&עריכה"}, "קע"},
		{"Japanese", []platform.MenuItem{{Label: "ファイル"}, {Label: "編集", Role: platform.RoleEditMenu}, {Label: "表示"}, {Label: "ヘルプ", Role: platform.RoleHelpMenu}},
			[]string{"ファイル(&A)", "編集(&E)", "表示(&B)", "ヘルプ(&H)"}, "aebh"},
		{"Chinese, a role's letter taken", []platform.MenuItem{{Label: "Excel"}, {Label: "编辑", Role: platform.RoleEditMenu}},
			[]string{"&Excel", "编辑(&A)"}, "ea"},
		{"Korean with a digit of its own", []platform.MenuItem{{Label: "파일"}, {Label: "보기 2"}},
			[]string{"파일(&A)", "보기 &2"}, "a2"},
		{"symbols only", []platform.MenuItem{{Label: "★"}, {Label: "&"}},
			[]string{"★(&A)", "&&(&B)"}, "ab"},
		{"fullwidth Latin is typed through the IME", []platform.MenuItem{{Label: "ＡＢＣ"}},
			[]string{"ＡＢＣ(&A)"}, "a"},
	} {
		entries, _, keys := buildBarMenu(tc.items)
		var labels []string
		for _, e := range entries {
			labels = append(labels, e.label)
		}
		if !slices.Equal(labels, tc.labels) {
			t.Errorf("%s: labels %q, want %q", tc.name, labels, tc.labels)
		}
		if string(keys) != tc.keys {
			t.Errorf("%s: top-level keys %q, want %q", tc.name, string(keys), tc.keys)
		}
		// Alt and each key opens its menu (WM_SYSCHAR, in either case).
		for _, k := range keys {
			if !hasMnemonic(keys, uintptr(unicode.ToUpper(k))) {
				t.Errorf("%s: Alt+%q opens nothing", tc.name, k)
			}
		}
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
		{"Alt with Shift held (the layout switch)", []key{{vk: vkShift, shift: true}, {vk: vkMenu, shift: true}}, vkMenu, false},
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
	for _, msg := range []uint32{wmLButtonDown, wmRButtonUp, wmMButtonDown, wmXButtonDown, wmXButtonUp, wmMouseWheel, wmMouseHWheel,
		wmNCLButtonDown, wmNCRButtonDown, wmNCMButtonUp, wmNCXButtonDown, wmNCXButtonDblClk} {
		if !mouseDisarmsMenu(msg) {
			t.Errorf("message %#x does not disarm the bar", msg)
		}
	}
	for _, msg := range []uint32{wmNCMouseMove, 0x0200 /* WM_MOUSEMOVE */, wmKeyDown, wmSysKeyUp, wmNCMouseLeave} {
		if mouseDisarmsMenu(msg) {
			t.Errorf("message %#x disarms the bar", msg)
		}
	}
}
