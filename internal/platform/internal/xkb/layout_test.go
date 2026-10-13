package xkb

import (
	"os"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// keycodeOf returns the keycode of a key name in km (test helper).
func keycodeOf(t testing.TB, km *Keymap, name string) uint32 {
	t.Helper()
	for i, k := range km.keys {
		if k.exists && k.name == name {
			return km.minKey + uint32(i)
		}
	}
	t.Fatalf("no key <%s>", name)
	return 0
}

// maskOf resolves "+"-joined modifier names to a mask of km.
func maskOf(t testing.TB, km *Keymap, names string) uint32 {
	t.Helper()
	var m uint32
	for _, n := range strings.Split(names, "+") {
		if n == "" {
			continue
		}
		v, ok := km.ModMask(n)
		if !ok {
			t.Fatalf("no modifier %s", n)
		}
		m |= v
	}
	return m
}

// TestLayoutTables checks Key, Text and Mods of representative keys per
// layout: the logical key and text rules of ADR-050 on top of what
// TestKeymapsAgainstLibxkbcommon checks against libxkbcommon.
func TestLayoutTables(t *testing.T) {
	type tc struct {
		key    string // key name
		mods   string // depressed modifiers
		locked string // locked modifiers
		group  uint32
		want   platform.Key
		text   string
	}
	tables := map[string][]tc{
		"us": {
			{"AC01", "", "", 0, "a", "a"},
			{"AC01", "Shift", "", 0, "A", "A"},
			{"AC01", "", "Lock", 0, "A", "A"},
			{"AC01", "Shift", "Lock", 0, "a", "a"},
			{"AC01", "Control", "", 0, "a", "a"},       // Ctrl ignored
			{"AC01", "Control+Shift", "", 0, "A", "A"}, // Ctrl+Shift+A has Key "A"
			{"AE01", "Control+Shift", "", 0, "!", "!"}, // no control characters
			{"AC01", "Alt", "", 0, "a", "a"},           // Alt does not change the level
			{"AC01", "", "", 1, "a", "a"},              // group out of range wraps
			{"RTRN", "", "", 0, platform.KeyEnter, ""}, // control characters go through Key
			{"TAB", "", "", 0, platform.KeyTab, ""},
			{"TAB", "Shift", "", 0, platform.KeyTab, ""}, // ISO_Left_Tab
			{"ESC", "", "", 0, platform.KeyEscape, ""},
			{"BKSP", "", "", 0, platform.KeyBackspace, ""},
			{"DELE", "", "", 0, platform.KeyDelete, ""},
			{"SPCE", "", "", 0, " ", " "},
			{"LFSH", "", "", 0, platform.KeyShift, ""},
			{"LCTL", "", "", 0, platform.KeyControl, ""},
			{"LALT", "", "", 0, platform.KeyAlt, ""},
			{"RALT", "", "", 0, platform.KeyAlt, ""},
			{"LWIN", "", "", 0, platform.KeyMeta, ""},
			{"CAPS", "", "", 0, platform.KeyCapsLock, ""},
			{"FK01", "", "", 0, platform.KeyF1, ""},
			{"FK01", "Control+Alt", "", 0, platform.KeyF1, ""}, // not XF86Switch_VT_1
			{"LEFT", "", "", 0, platform.KeyArrowLeft, ""},
			{"PGUP", "", "", 0, platform.KeyPageUp, ""},
			{"KP1", "", "", 0, platform.KeyEnd, ""},
			{"KP1", "", "NumLock", 0, "1", "1"},
			{"KP1", "Shift", "NumLock", 0, "1", "1"},  // type KEYPAD ignores Shift
			{"KP5", "", "", 0, platform.KeyClear, ""}, // KP_Begin
			{"KPAD", "", "", 0, "+", "+"},
			{"KPEN", "", "", 0, platform.KeyEnter, ""},
			{"KPDL", "", "", 0, platform.KeyDelete, ""},
			{"KPDL", "", "NumLock", 0, ".", "."},
			{"PRSC", "", "", 0, platform.KeyPrintScreen, ""},
			{"PRSC", "Alt", "", 0, platform.KeyPrintScreen, ""}, // Sys_Req
			{"MUTE", "", "", 0, platform.KeyAudioVolumeMute, ""},
			{"COMP", "", "", 0, platform.KeyContextMenu, ""},
		},
		"us-intl": {
			{"AC11", "", "", 0, platform.KeyDead, ""}, // dead_acute
			{"AC11", "Shift", "", 0, platform.KeyDead, ""},
			{"TLDE", "", "", 0, platform.KeyDead, ""},
			{"AD03", "LevelThree", "", 0, "é", "é"},
		},
		"us-altgr-intl": {
			{"AD03", "LevelThree", "", 0, "é", "é"},
			{"AD03", "LevelThree+Shift", "", 0, "É", "É"},
			{"AD03", "LevelThree", "Lock", 0, "É", "É"},
			{"AE05", "LevelThree", "", 0, "€", "€"},
			{"AC11", "", "", 0, "'", "'"},
		},
		"de": {
			{"AD06", "", "", 0, "z", "z"},
			{"AB01", "", "", 0, "y", "y"},
			{"AE11", "", "", 0, "ß", "ß"},
			{"AE11", "Shift", "", 0, "?", "?"},
			{"AE11", "", "Lock", 0, "ẞ", "ẞ"},
			{"AD01", "LevelThree", "", 0, "@", "@"},
			{"AE08", "LevelThree", "", 0, "[", "["},
			{"AD11", "", "", 0, "ü", "ü"},
			{"AD11", "", "Lock", 0, "Ü", "Ü"},
			{"TLDE", "", "", 0, platform.KeyDead, ""}, // dead_circumflex
			{"AE12", "", "", 0, platform.KeyDead, ""}, // dead_acute
			{"AE03", "LevelThree", "", 0, "³", "³"},
		},
		"fr": {
			{"AD01", "", "", 0, "a", "a"},
			{"AC10", "", "", 0, "m", "m"},
			{"AE01", "", "", 0, "&", "&"},
			{"AE01", "Shift", "", 0, "1", "1"},
			{"AE02", "", "", 0, "é", "é"},
			{"AE10", "LevelThree", "", 0, "@", "@"},
			{"AD11", "", "", 0, platform.KeyDead, ""}, // dead_circumflex
		},
		"es": {
			{"AC10", "", "", 0, "ñ", "ñ"},
			{"AC10", "Shift", "", 0, "Ñ", "Ñ"},
			{"AC10", "", "Lock", 0, "Ñ", "Ñ"},
			{"AD11", "", "", 0, platform.KeyDead, ""}, // dead_grave
			{"AE02", "LevelThree", "", 0, "@", "@"},
		},
		"latam": {
			{"AC10", "", "", 0, "ñ", "ñ"},
			{"AD01", "LevelThree", "", 0, "@", "@"},
			{"AD11", "", "", 0, platform.KeyDead, ""}, // dead_acute
		},
		"gb": {
			{"AE02", "Shift", "", 0, "\"", "\""},
			{"AE03", "Shift", "", 0, "£", "£"},
			{"BKSL", "", "", 0, "#", "#"},
		},
		"ru": {
			{"AD01", "", "", 0, "й", "й"},
			{"AD01", "Shift", "", 0, "Й", "Й"},
			{"AD01", "", "Lock", 0, "Й", "Й"},
			{"AD01", "Control", "", 0, "й", "й"},
			{"TLDE", "", "", 0, "ё", "ё"},
		},
		"il": {
			{"AC01", "", "", 0, "ש", "ש"},
			{"AC01", "Shift", "", 0, "A", "A"},
			{"AD01", "", "", 0, "/", "/"},
		},
		"gr": {
			{"AC01", "", "", 0, "α", "α"},
			{"AC01", "Shift", "", 0, "Α", "Α"},
			{"AC01", "", "Lock", 0, "Α", "Α"},
			{"AD02", "", "", 0, "ς", "ς"},
			{"AD02", "", "Lock", 0, "Σ", "Σ"},
		},
		"jp": {
			{"HENK", "", "", 0, platform.KeyConvert, ""},
			{"MUHE", "", "", 0, platform.KeyNonConvert, ""},
			{"HKTG", "", "", 0, platform.KeyHiraganaKatakana, ""},
			{"AE13", "", "", 0, "\\", "\\"}, // the ¥ key types a backslash in jp
		},
		"us-nocaps": {
			{"CAPS", "", "", 0, platform.KeyControl, ""},
		},
		"us-compose-ralt": {
			{"RALT", "", "", 0, platform.KeyCompose, ""},
		},
		"us-ru-grp": {
			{"AD01", "", "", 0, "q", "q"},
			{"AD01", "", "", 1, "й", "й"},
			{"AD01", "Shift", "", 1, "Й", "Й"},
			{"AD01", "", "Lock", 1, "Й", "Й"},
			{"LFSH", "Alt", "", 0, platform.KeyGroupNext, ""},
			{"RTRN", "", "", 1, platform.KeyEnter, ""},
		},
		"de-neo": {
			{"AC01", "", "", 0, "u", "u"},
			{"AC01", "LevelThree", "", 0, "\\", "\\"},
			{"AC01", "LevelFive", "", 0, platform.KeyHome, ""},
			{"AC01", "LevelThree+LevelFive", "", 0, "⊂", "⊂"},
			{"KP1", "", "", 0, "1", "1"},
			{"KP1", "Shift", "", 0, "♦", "♦"},
		},
	}
	for name, cases := range tables {
		km := loadFixture(t, name)
		st := NewState(km)
		for _, c := range cases {
			st.UpdateMask(maskOf(t, km, c.mods), 0, maskOf(t, km, c.locked), c.group)
			got := st.Lookup(keycodeOf(t, km, c.key))
			if got.Key != c.want || got.Text != c.text {
				t.Errorf("%s <%s> mods %q locked %q group %d: Key %q Text %q, want %q %q",
					name, c.key, c.mods, c.locked, c.group, got.Key, got.Text, c.want, c.text)
			}
		}
	}
}

func TestModifiers(t *testing.T) {
	for _, name := range []string{"us", "de", "us-ru-grp", "de-neo"} {
		km := loadFixture(t, name)
		st := NewState(km)
		for _, c := range []struct {
			dep, lat, lock string
			want           platform.Modifiers
		}{
			{"", "", "", 0},
			{"Shift", "", "", platform.ModShift},
			{"Control", "", "", platform.ModCtrl},
			{"Alt", "", "", platform.ModAlt},
			{"Super", "", "", platform.ModSuper},
			{"Shift+Control+Alt+Super", "", "", platform.ModShift | platform.ModCtrl | platform.ModAlt | platform.ModSuper},
			{"LevelThree", "", "", 0}, // AltGr is not Alt or Ctrl
			{"", "", "Lock", 0},       // lock states are not modifiers
			{"", "", "NumLock", 0},
			{"", "Shift", "", platform.ModShift}, // latched (sticky keys)
			{"", "", "Shift", 0},                 // Shift Lock
			{"Control", "", "Lock+NumLock", platform.ModCtrl},
		} {
			st.UpdateMask(maskOf(t, km, c.dep), maskOf(t, km, c.lat), maskOf(t, km, c.lock), 0)
			if got := st.Modifiers(); got != c.want {
				t.Errorf("%s: depressed %q latched %q locked %q: Modifiers %v, want %v", name, c.dep, c.lat, c.lock, got, c.want)
			}
		}
	}
	// LevelFive is not reported either (Neo maps it to Mod3).
	km := loadFixture(t, "de-neo")
	st := NewState(km)
	st.UpdateMask(maskOf(t, km, "LevelFive"), 0, 0, 0)
	if got := st.Modifiers(); got != 0 {
		t.Errorf("de-neo LevelFive: Modifiers %v, want none", got)
	}
}

func TestHeldModifiers(t *testing.T) {
	for _, c := range []struct {
		layout, key string
		want        platform.Modifiers
	}{
		{"us", "LFSH", platform.ModShift},
		{"us", "RTSH", platform.ModShift},
		{"us", "LCTL", platform.ModCtrl},
		{"us", "LALT", platform.ModAlt},
		{"us", "RALT", platform.ModAlt},
		{"us", "LWIN", platform.ModSuper},
		{"us", "CAPS", 0},
		{"us", "NMLK", 0},
		{"us", "AC01", 0},
		{"us-nocaps", "CAPS", platform.ModCtrl},
		{"us-altgr-intl", "RALT", 0}, // AltGr
		{"us-compose-ralt", "RALT", 0},
	} {
		km := loadFixture(t, c.layout)
		if got := km.HeldModifiers(keycodeOf(t, km, c.key)); got != c.want {
			t.Errorf("%s <%s>: HeldModifiers %v, want %v", c.layout, c.key, got, c.want)
		}
	}
}

func TestDeadKeysAndCompose(t *testing.T) {
	km := loadFixture(t, "us-intl")
	data, err := os.ReadFile("testdata/compose/locale/en_US.UTF-8/Compose")
	if err != nil {
		t.Fatal(err)
	}
	table, err := ParseCompose(data, "Compose", fixtureComposeOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	st := NewState(km)
	cs := NewComposeState(table)
	shift := maskOf(t, km, "Shift")
	type press struct {
		key  string
		mods uint32
		want platform.Key
		text string
	}
	run := func(label string, cs *ComposeState, presses []press) {
		t.Helper()
		for i, p := range presses {
			st.UpdateMask(p.mods, 0, 0, 0)
			got := st.Press(keycodeOf(t, km, p.key), cs)
			if got.Key != p.want || got.Text != p.text {
				t.Errorf("%s: press %d <%s>: Key %q Text %q, want %q %q", label, i, p.key, got.Key, got.Text, p.want, p.text)
			}
		}
	}
	run("acute e", cs, []press{
		{"AC11", 0, platform.KeyDead, ""}, // dead_acute
		{"AD03", 0, "é", "é"},
	})
	run("acute shift-e", cs, []press{
		{"AC11", 0, platform.KeyDead, ""},
		{"LFSH", shift, platform.KeyShift, ""}, // modifiers do not break a sequence
		{"AD03", shift, "É", "É"},
	})
	run("acute space", cs, []press{
		{"AC11", 0, platform.KeyDead, ""},
		{"SPCE", 0, "'", "'"},
	})
	run("acute q (no sequence)", cs, []press{
		{"AC11", 0, platform.KeyDead, ""},
		{"AD01", 0, "q", "q"}, // the sequence is dropped and q types
	})
	run("grave a", cs, []press{
		{"TLDE", 0, platform.KeyDead, ""}, // dead_grave
		{"AC01", 0, "à", "à"},
	})
	run("diaeresis u", cs, []press{
		{"AC11", shift, platform.KeyDead, ""}, // dead_diaeresis
		{"AD07", 0, "ü", "ü"},
	})
	// Repeats never feed compose.
	st.UpdateMask(0, 0, 0, 0)
	if got := st.Repeat(keycodeOf(t, km, "AC11"), cs); got.Key != platform.KeyDead || got.Text != "" {
		t.Errorf("repeat of a dead key: %q %q", got.Key, got.Text)
	}
	if cs.Composing() {
		t.Error("a repeat fed compose")
	}
	// Without a Compose table, dead keys type their spacing character.
	run("no table", nil, []press{
		{"AC11", 0, "´", "´"},
		{"AD03", 0, "e", "e"},
		{"TLDE", 0, "`", "`"},
	})
	if got := st.Repeat(keycodeOf(t, km, "AC11"), nil); got.Text != "´" {
		t.Errorf("repeat of a dead key without a table: %q", got.Text)
	}

	// The Compose key (us with compose:ralt).
	km = loadFixture(t, "us-compose-ralt")
	st = NewState(km)
	cs = NewComposeState(table)
	run("Multi_key a e", cs, []press{
		{"RALT", 0, platform.KeyCompose, ""},
		{"AC01", 0, "a", ""}, // intermediate: no text, keeps its Key
		{"AD03", 0, "æ", "æ"},
	})
}

func TestPressDoesNotAllocate(t *testing.T) {
	km := loadFixture(t, "us-intl")
	data, _ := os.ReadFile("testdata/compose/locale/en_US.UTF-8/Compose")
	table, err := ParseCompose(data, "Compose", fixtureComposeOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	st, cs := NewState(km), NewComposeState(table)
	acute, e := keycodeOf(t, km, "AC11"), keycodeOf(t, km, "AD03")
	allocs := testing.AllocsPerRun(100, func() {
		st.Press(acute, cs)
		st.Press(e, cs)
		st.Repeat(e, cs)
	})
	if allocs != 0 {
		t.Errorf("Press allocates %v times per run", allocs)
	}
}
