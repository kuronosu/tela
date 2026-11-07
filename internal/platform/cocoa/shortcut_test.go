package cocoa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

func TestHotkeySignature(t *testing.T) {
	if hotkeySignature != 'T'<<24|'E'<<16|'L'<<8|'A' {
		t.Errorf("hotkeySignature = %#x, want the four-character code 'TELA'", hotkeySignature)
	}
}

func TestHotkeyModifiers(t *testing.T) {
	for _, c := range []struct {
		m    platform.Modifiers
		want uint32
	}{
		{0, 0},
		{platform.ModSuper, carbonCmdKey},
		{platform.ModShift, carbonShiftKey},
		{platform.ModAlt, carbonOptionKey},
		{platform.ModCtrl, carbonControlKey},
		{platform.ModCtrl | platform.ModAlt | platform.ModShift | platform.ModSuper, 0x1B00},
	} {
		if got := hotkeyModifiers(c.m); got != c.want {
			t.Errorf("hotkeyModifiers(%v) = %#x, want %#x", c.m, got, c.want)
		}
	}
}

// fakeLayout returns a layout whose plain and shifted keys are given by
// code.
func fakeLayout(plain, shifted map[uint16]string) *keyLayout {
	l := &keyLayout{id: "test"}
	for c, s := range plain {
		l.plain[c] = s
	}
	for c, s := range shifted {
		l.shifted[c] = s
	}
	return l
}

// usLayout is the ANSI layout's plain keys, plus '+' shifted on '='.
func usLayout() *keyLayout {
	plain := map[uint16]string{}
	for r, c := range ansiKeyCodes {
		plain[c] = string(r)
	}
	plain[0x31] = " "
	plain[0x24] = "\r"
	plain[0x7A] = "\x10" // F1 types a control character
	plain[0x53] = "1"    // keypad 1
	return fakeLayout(plain, map[uint16]string{0x18: "+", 0x12: "!"})
}

func TestHotkeyKeyCode(t *testing.T) {
	us := usLayout()
	// AZERTY: the A key types q, the Q key a, the digit row types
	// symbols and digits with Shift; the keypad types digits.
	azerty := fakeLayout(
		map[uint16]string{0x00: "q", 0x0C: "a", 0x12: "&", 0x13: "é", 0x53: "1"},
		map[uint16]string{0x00: "Q", 0x0C: "A", 0x12: "1", 0x13: "2"},
	)
	// Cyrillic: the K key types л.
	cyrillic := fakeLayout(map[uint16]string{0x28: "л", 0x0B: "и"}, nil)
	// A layout whose plain letters are capitals, and one with a function
	// key character on a key.
	odd := fakeLayout(map[uint16]string{0x05: "G", 0x10: ""}, nil)
	for _, c := range []struct {
		key    platform.Key
		layout *keyLayout
		want   uint16
	}{
		{platform.KeyF1, us, 0x7A},
		{platform.KeyF13, nil, 0x69},
		{platform.KeyF20, nil, 0x5A},
		{platform.KeyArrowUp, nil, 0x7E},
		{platform.KeyArrowLeft, nil, 0x7B},
		{platform.KeyEscape, nil, 0x35},
		{platform.KeyEnter, nil, 0x24},
		{platform.KeyTab, nil, 0x30},
		{platform.KeyBackspace, nil, 0x33},
		{platform.KeyDelete, nil, 0x75},
		{platform.KeyInsert, nil, 0x72},
		{platform.KeyPageDown, nil, 0x79},
		{" ", nil, 0x31},
		{" ", azerty, 0x31},
		{"k", nil, 0x28},
		{"k", us, 0x28},
		{"1", us, 0x12},     // the digit row, not the keypad
		{"+", us, 0x18},     // shifted '=': the key, without Shift
		{"a", azerty, 0x0C}, // where the layout types it
		{"q", azerty, 0x00},
		{"1", azerty, 0x12}, // shifted on the digit row, not the keypad
		{"é", azerty, 0x13},
		{"k", cyrillic, 0x28}, // not on the layout: the ANSI position
		{"л", cyrillic, 0x28},
		{"b", cyrillic, 0x0B},
		{"g", odd, 0x05},
		{"`", nil, 0x32},
	} {
		got, err := hotkeyKeyCode(c.key, c.layout)
		if err != nil || got != c.want {
			t.Errorf("hotkeyKeyCode(%q, %s) = %#x, %v; want %#x", c.key, layoutName(c.layout, us, azerty, cyrillic), got, err, c.want)
		}
	}
	for _, c := range []struct {
		key    platform.Key
		layout *keyLayout
	}{
		{platform.KeyMediaPlayPause, nil},
		{platform.KeyPrintScreen, us},
		{"é", nil},
		{"л", us},
		{"", odd}, // a function key character names no key
	} {
		if got, err := hotkeyKeyCode(c.key, c.layout); err == nil {
			t.Errorf("hotkeyKeyCode(%q) = %#x, nil; want an error", c.key, got)
		} else {
			t.Logf("hotkeyKeyCode(%q): %v", c.key, err)
		}
	}
}

func layoutName(l *keyLayout, us, azerty, cyrillic *keyLayout) string {
	switch l {
	case nil:
		return "no layout"
	case us:
		return "US"
	case azerty:
		return "AZERTY"
	case cyrillic:
		return "Cyrillic"
	}
	return "odd"
}

// TestNamedKeyCodes: every fixed code is the physical key the macOS
// keyboard translation reports for the named key (keyboard.MacKeyCode),
// whose W3C code name is the key's name but for the listed keys.
func TestNamedKeyCodes(t *testing.T) {
	codeOf := map[platform.Key]platform.KeyCode{
		platform.KeyInsert:   platform.CodeInsert,
		platform.KeyHelp:     platform.CodeInsert, // kVK_Help is at the Insert position
		platform.KeyClear:    platform.CodeNumLock,
		platform.KeyEisu:     platform.CodeLang2,
		platform.KeyKanaMode: platform.CodeLang1,
	}
	for k, c := range namedKeyCodes {
		if !k.IsNamed() {
			t.Errorf("namedKeyCodes has %q, which is not a named key", k)
		}
		got := keyboard.MacKeyCode(c, false)
		want, ok := codeOf[k]
		if ok && got != want || !ok && got.String() != string(k) {
			t.Errorf("namedKeyCodes[%q] = %#x, which is the physical key %v", k, c, got)
		}
	}
	for r, c := range ansiKeyCodes {
		if code := keyboard.MacKeyCode(c, false); !code.Identified() {
			t.Errorf("ansiKeyCodes[%q] = %#x, an unknown key", r, c)
		}
	}
	for n := 1; n <= 20; n++ {
		if _, ok := namedKeyCodes[platform.Key("F"+itoa(n))]; !ok {
			t.Errorf("F%d has no key code", n)
		}
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func TestMacTrigger(t *testing.T) {
	for _, c := range []struct {
		a    platform.Accelerator
		want string
	}{
		{platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt | platform.ModSuper, Key: "k"}, "⌃⌥⌘K"},
		{platform.Accelerator{Mods: platform.ModSuper | platform.ModShift, Key: platform.KeyF5}, "⇧⌘F5"},
		{platform.Accelerator{Mods: platform.ModSuper, Key: " "}, "⌘Space"},
		{platform.Accelerator{Mods: platform.ModCtrl, Key: platform.KeyArrowUp}, "⌃↑"},
		{platform.Accelerator{Mods: platform.ModAlt, Key: platform.KeyEscape}, "⌥⎋"},
		{platform.Accelerator{Mods: platform.ModSuper, Key: "+"}, "⌘+"},
		{platform.Accelerator{Key: platform.KeyF1}, "F1"},
		{platform.Accelerator{Mods: platform.ModSuper, Key: "é"}, "⌘É"},
	} {
		if got := macTrigger(c.a); got != c.want {
			t.Errorf("macTrigger(%v) = %q, want %q", c.a, got, c.want)
		}
	}
	// CmdOrCtrl is Command under the Mac flavor: what ParseAccelerator
	// gives on a Mac reaches the backend as ModSuper.
	if a, err := platform.ParseAccelerator("CmdOrCtrl+Shift+K"); err == nil && platform.HostFlavor() == platform.FlavorMac {
		if got := hotkeyModifiers(a.Mods); got != carbonCmdKey|carbonShiftKey {
			t.Errorf("CmdOrCtrl+Shift+K on a Mac: Carbon modifiers %#x, want Command and Shift", got)
		}
		if got := macTrigger(a); !strings.HasPrefix(got, "⇧⌘") {
			t.Errorf("CmdOrCtrl+Shift+K on a Mac: trigger %q", got)
		}
	}
}

// TestRelayoutPlan: switching from US to AZERTY moves the hot keys of the
// characters the layouts type on other keys (a and q trade places), keeps
// named keys and characters on the same key, and lets a character that
// had no key find one.
func TestRelayoutPlan(t *testing.T) {
	us := usLayout()
	azerty := fakeLayout(
		map[uint16]string{0x00: "q", 0x0C: "a", 0x28: "k", 0x13: "é"},
		map[uint16]string{0x00: "Q", 0x0C: "A"},
	)
	keys := []platform.Key{"a", "q", "k", platform.KeyF5, "é", platform.KeyMediaPlayPause}
	targets := func(l *keyLayout) []hotkeyTarget {
		var out []hotkeyTarget
		for _, k := range keys {
			out = append(out, targetOf(k, l))
		}
		return out
	}
	old, now := targets(us), targets(azerty)
	if old[4].err == "" || now[4].err != "" || now[4].code != 0x13 {
		t.Errorf("é: US %+v, AZERTY %+v; want an error, then the key 0x13", old[4], now[4])
	}
	if old[5].err == "" || old[5] != now[5] {
		t.Errorf("a media key: %+v, %+v; want the same error under both layouts", old[5], now[5])
	}
	if got, want := relayoutPlan(old, now), []int{0, 1, 4}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("relayoutPlan(US → AZERTY) = %v, want %v (a, q and é move)", got, want)
	}
	if now[0].code != 0x0C || now[1].code != 0x00 {
		t.Errorf("AZERTY targets of a and q: %+v, %+v; want 0x0C and 0x00", now[0], now[1])
	}
	if got := relayoutPlan(old, old); len(got) != 0 {
		t.Errorf("relayoutPlan with the same layout = %v, want none", got)
	}
	if got := relayoutPlan(now, old); fmt.Sprint(got) != "[0 1 4]" {
		t.Errorf("relayoutPlan(AZERTY → US) = %v, want [0 1 4]", got)
	}
}
