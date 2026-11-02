package keyboard

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

func TestMacKeyCodeTable(t *testing.T) {
	seen := map[platform.KeyCode]uint16{}
	for kvk := uint16(0); kvk < 0x80; kvk++ {
		mapped := macKeyCodes[kvk] != platform.CodeUnidentified
		unmapped := slices.Contains(macUnmapped[:], kvk)
		if mapped == unmapped {
			t.Errorf("kVK %#02x: mapped %v, listed as unmapped %v (exactly one must hold)", kvk, mapped, unmapped)
			continue
		}
		got := MacKeyCode(kvk, false)
		if unmapped {
			if n, ok := got.Native(); !ok || n != uint32(kvk) {
				t.Errorf("unmapped kVK %#02x: got %#v, want UnidentifiedKeyCode(%#x)", kvk, got, kvk)
			}
			continue
		}
		if !got.Identified() {
			t.Errorf("kVK %#02x: %#v is not an identified code", kvk, got)
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("kVK %#02x and %#02x both map to %v", prev, kvk, got)
		}
		seen[got] = kvk
	}
	for _, kvk := range []uint16{0x80, 0xB3, 0xFFFF} {
		if n, ok := MacKeyCode(kvk, false).Native(); !ok || n != uint32(kvk) {
			t.Errorf("kVK %#x: want an unidentified code carrying it", kvk)
		}
	}
}

func TestMacKeyCodes(t *testing.T) {
	for _, c := range []struct {
		kvk  uint16
		iso  bool
		want platform.KeyCode
	}{
		{0x00, false, platform.CodeKeyA},
		{0x06, false, platform.CodeKeyZ},
		{0x0A, false, platform.CodeIntlBackslash}, // kVK_ISO_Section
		{0x32, false, platform.CodeBackquote},     // kVK_ANSI_Grave
		{0x0A, true, platform.CodeBackquote},      // ISO: the § key left of 1
		{0x32, true, platform.CodeIntlBackslash},  // ISO: the key next to the left Shift
		{0x00, true, platform.CodeKeyA},
		{0x5D, false, platform.CodeIntlYen},
		{0x5E, false, platform.CodeIntlRo},
		{0x5F, false, platform.CodeNumpadComma},
		{0x66, false, platform.CodeLang2},
		{0x68, false, platform.CodeLang1},
		{0x24, false, platform.CodeEnter},
		{0x4C, false, platform.CodeNumpadEnter},
		{0x33, false, platform.CodeBackspace},
		{0x75, false, platform.CodeDelete},
		{0x72, false, platform.CodeInsert},
		{0x47, false, platform.CodeNumLock},
		{0x6E, false, platform.CodeContextMenu},
		{0x3F, false, platform.CodeFn},
		{0x37, false, platform.CodeMetaLeft},
		{0x36, false, platform.CodeMetaRight},
		{0x3A, false, platform.CodeAltLeft},
		{0x3D, false, platform.CodeAltRight},
		{0x3B, false, platform.CodeControlLeft},
		{0x3E, false, platform.CodeControlRight},
		{0x38, false, platform.CodeShiftLeft},
		{0x3C, false, platform.CodeShiftRight},
		{0x39, false, platform.CodeCapsLock},
		{0x48, false, platform.CodeAudioVolumeUp},
		{0x49, false, platform.CodeAudioVolumeDown},
		{0x4A, false, platform.CodeAudioVolumeMute},
	} {
		if got := MacKeyCode(c.kvk, c.iso); got != c.want {
			t.Errorf("MacKeyCode(%#02x, iso %v) = %v, want %v", c.kvk, c.iso, got, c.want)
		}
	}
}

// macFKeyCodes are the virtual key codes of F1 to F20.
var macFKeyCodes = [20]uint16{
	0x7A, 0x78, 0x63, 0x76, 0x60, 0x61, 0x62, 0x64, 0x65, 0x6D,
	0x67, 0x6F, 0x69, 0x6B, 0x71, 0x6A, 0x40, 0x4F, 0x50, 0x5A,
}

func TestMacFunctionKeys(t *testing.T) {
	for i, kvk := range macFKeyCodes {
		n := i + 1
		wantCode, _ := platform.ParseKeyCode(fmt.Sprintf("F%d", n))
		wantKey := platform.Key(fmt.Sprintf("F%d", n))
		fn := string(rune(nsF1FunctionKey + i))
		for _, chars := range []string{fn, ""} { // characters, or none (the code names it)
			ev, ok := MacKeyDown(MacKey{KeyCode: kvk, Flags: MacFlagFunction, Characters: chars, CharactersIgnoringModifiers: chars})
			if !ok || ev.Code != wantCode || ev.Key != wantKey || ev.Text != "" || ev.Mods != 0 {
				t.Errorf("F%d (chars %q): got ok %v %v %q text %q mods %v", n, chars, ok, ev.Code, ev.Key, ev.Text, ev.Mods)
			}
		}
	}
	// F21 to F24 have function-key characters but no Mac key code; F25
	// and beyond have no W3C name.
	if k, _ := macNamedChar(string(rune(nsF1FunctionKey + 23))); k != platform.KeyF24 {
		t.Errorf("NSF24FunctionKey: %q", k)
	}
	if k, ok := macNamedChar(string(rune(nsF1FunctionKey + 24))); !ok || k != platform.KeyUnidentified {
		t.Errorf("NSF25FunctionKey: %q %v", k, ok)
	}
}

func TestMacModifiers(t *testing.T) {
	for _, c := range []struct {
		flags uint64
		want  platform.Modifiers
	}{
		{0, 0},
		{MacFlagShift | MacDeviceLeftShift, platform.ModShift},
		{MacFlagControl, platform.ModCtrl},
		{MacFlagOption, platform.ModAlt},
		{MacFlagCommand, platform.ModSuper},
		{MacFlagShift | MacFlagControl | MacFlagOption | MacFlagCommand, platform.ModShift | platform.ModCtrl | platform.ModAlt | platform.ModSuper},
		{MacFlagCapsLock | MacFlagNumericPad | MacFlagHelp | MacFlagFunction | 0x100, 0},
	} {
		if got := MacModifiers(c.flags); got != c.want {
			t.Errorf("MacModifiers(%#x) = %v, want %v", c.flags, got, c.want)
		}
	}
}

const (
	lShift = MacFlagShift | MacDeviceLeftShift
	lCmd   = MacFlagCommand | MacDeviceLeftCommand
	lCtrl  = MacFlagControl | MacDeviceLeftControl
	lOpt   = MacFlagOption | MacDeviceLeftOption
	arrow  = MacFlagFunction | MacFlagNumericPad // what AppKit sets for the arrows
	nav    = MacFlagFunction                     // Home, End, Page Up/Down, forward Delete
	pad    = MacFlagNumericPad
)

func TestMacKeyDown(t *testing.T) {
	type want struct {
		code platform.KeyCode
		key  platform.Key
		text string
		mods platform.Modifiers
		loc  platform.KeyLocation
	}
	std := platform.KeyLocationStandard
	num := platform.KeyLocationNumpad
	for _, c := range []struct {
		name  string
		in    MacKey
		want  want
		check string // an extra note for failures
	}{
		// Letters on the US layout.
		{"a", MacKey{KeyCode: 0x00, Characters: "a", CharactersIgnoringModifiers: "a"}, want{platform.CodeKeyA, "a", "a", 0, std}, ""},
		{"Shift+a", MacKey{KeyCode: 0x00, Flags: lShift, Characters: "A", CharactersIgnoringModifiers: "A"}, want{platform.CodeKeyA, "A", "A", platform.ModShift, std}, ""},
		{"CapsLock a", MacKey{KeyCode: 0x00, Flags: MacFlagCapsLock, Characters: "A", CharactersIgnoringModifiers: "a"}, want{platform.CodeKeyA, "A", "A", 0, std}, "Caps Lock is reflected, not a modifier"},
		{"CapsLock Shift+a", MacKey{KeyCode: 0x00, Flags: MacFlagCapsLock | lShift, Characters: "A", CharactersIgnoringModifiers: "A"}, want{platform.CodeKeyA, "A", "A", platform.ModShift, std}, ""},
		{"Shift+1", MacKey{KeyCode: 0x12, Flags: lShift, Characters: "!", CharactersIgnoringModifiers: "!"}, want{platform.CodeDigit1, "!", "!", platform.ModShift, std}, ""},
		{"space", MacKey{KeyCode: 0x31, Characters: " ", CharactersIgnoringModifiers: " "}, want{platform.CodeSpace, " ", " ", 0, std}, ""},

		// Command and Control are not reflected in Key; Text is Key's character.
		{"Cmd+S", MacKey{KeyCode: 0x01, Flags: lCmd, Characters: "s", CharactersIgnoringModifiers: "s"}, want{platform.CodeKeyS, "s", "s", platform.ModSuper, std}, ""},
		{"Cmd+Shift+S", MacKey{KeyCode: 0x01, Flags: lCmd | lShift, Characters: "S", CharactersIgnoringModifiers: "S"}, want{platform.CodeKeyS, "S", "S", platform.ModSuper | platform.ModShift, std}, ""},
		{"Cmd+Shift+S lowercase", MacKey{KeyCode: 0x01, Flags: lCmd | lShift, Characters: "s", CharactersIgnoringModifiers: "s"}, want{platform.CodeKeyS, "S", "S", platform.ModSuper | platform.ModShift, std}, "AppKit dropping Shift"},
		{"Cmd+CapsLock s", MacKey{KeyCode: 0x01, Flags: lCmd | MacFlagCapsLock, Characters: "s", CharactersIgnoringModifiers: "s"}, want{platform.CodeKeyS, "S", "S", platform.ModSuper, std}, ""},
		{"Cmd+Option+S", MacKey{KeyCode: 0x01, Flags: lCmd | lOpt, Characters: "ß", CharactersIgnoringModifiers: "s"}, want{platform.CodeKeyS, "s", "s", platform.ModSuper | platform.ModAlt, std}, "Option ignored with Command"},
		{"Cmd+Shift+2", MacKey{KeyCode: 0x13, Flags: lCmd | lShift, Characters: "@", CharactersIgnoringModifiers: "@"}, want{platform.CodeDigit2, "@", "@", platform.ModSuper | platform.ModShift, std}, ""},
		{"Ctrl+A", MacKey{KeyCode: 0x00, Flags: lCtrl, Characters: "\x01", CharactersIgnoringModifiers: "a"}, want{platform.CodeKeyA, "a", "a", platform.ModCtrl, std}, "no control character"},
		{"Ctrl+H", MacKey{KeyCode: 0x04, Flags: lCtrl, Characters: "\b", CharactersIgnoringModifiers: "h"}, want{platform.CodeKeyH, "h", "h", platform.ModCtrl, std}, "not Backspace"},
		{"Ctrl+I", MacKey{KeyCode: 0x22, Flags: lCtrl, Characters: "\t", CharactersIgnoringModifiers: "i"}, want{platform.CodeKeyI, "i", "i", platform.ModCtrl, std}, "not Tab"},
		{"Ctrl+[", MacKey{KeyCode: 0x21, Flags: lCtrl, Characters: "\x1b", CharactersIgnoringModifiers: "["}, want{platform.CodeBracketLeft, "[", "[", platform.ModCtrl, std}, "not Escape"},
		{"Ctrl+Shift+A", MacKey{KeyCode: 0x00, Flags: lCtrl | lShift, Characters: "\x01", CharactersIgnoringModifiers: "A"}, want{platform.CodeKeyA, "A", "A", platform.ModCtrl | platform.ModShift, std}, ""},

		// Option types characters.
		{"Option+A", MacKey{KeyCode: 0x00, Flags: lOpt, Characters: "å", CharactersIgnoringModifiers: "a"}, want{platform.CodeKeyA, "å", "å", platform.ModAlt, std}, ""},
		{"Option+Shift+K", MacKey{KeyCode: 0x28, Flags: lOpt | lShift, Characters: "", CharactersIgnoringModifiers: "K"}, want{platform.CodeKeyK, "", "", platform.ModAlt | platform.ModShift, std}, "the Apple logo is text"},
		{"Option+Space", MacKey{KeyCode: 0x31, Flags: lOpt, Characters: " ", CharactersIgnoringModifiers: " "}, want{platform.CodeSpace, " ", " ", platform.ModAlt, std}, ""},
		{"right Option+A", MacKey{KeyCode: 0x00, Flags: MacFlagOption | MacDeviceRightOption, Characters: "å", CharactersIgnoringModifiers: "a"}, want{platform.CodeKeyA, "å", "å", platform.ModAlt, std}, ""},

		// Dead keys.
		{"Option+E dead", MacKey{KeyCode: 0x0E, Flags: lOpt, Characters: "", CharactersIgnoringModifiers: "e"}, want{platform.CodeKeyE, platform.KeyDead, "", platform.ModAlt, std}, ""},
		{"unmodified dead key", MacKey{KeyCode: 0x27, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeQuote, platform.KeyDead, "", 0, std}, ""},
		{"e after dead", MacKey{KeyCode: 0x0E, Characters: "é", CharactersIgnoringModifiers: "e"}, want{platform.CodeKeyE, "é", "é", 0, std}, ""},
		{"x after dead", MacKey{KeyCode: 0x07, Characters: "´x", CharactersIgnoringModifiers: "x"}, want{platform.CodeKeyX, "x", "´x", 0, std}, ""},
		{"Return after dead", MacKey{KeyCode: 0x24, Characters: "´\r", CharactersIgnoringModifiers: "\r"}, want{platform.CodeEnter, platform.KeyEnter, "´", 0, std}, "the accent is typed"},
		{"x after dead, no fallback", MacKey{KeyCode: 0x07, Characters: "´x"}, want{platform.CodeKeyX, "x", "´x", 0, std}, "the last character"},
		{"Cmd+dead key", MacKey{KeyCode: 0x27, Flags: lCmd}, want{platform.CodeQuote, platform.KeyDead, "", platform.ModSuper, std}, ""},
		{"Cmd+unknown key", MacKey{KeyCode: 0x90, Flags: lCmd}, want{platform.UnidentifiedKeyCode(0x90), platform.KeyUnidentified, "", platform.ModSuper, std}, ""},
		{"Cmd+control character", MacKey{KeyCode: 0x2A, Flags: lCmd, Characters: "\x1c", CharactersIgnoringModifiers: "\x1c"}, want{platform.CodeBackslash, platform.KeyUnidentified, "", platform.ModSuper, std}, ""},
		{"unnamed control character", MacKey{KeyCode: 0x2A, Characters: "\x1c", CharactersIgnoringModifiers: "\\"}, want{platform.CodeBackslash, "\\", "", 0, std}, "never text"},
		{"only control characters", MacKey{KeyCode: 0x2A, Characters: "\x1c", CharactersIgnoringModifiers: "\x1c"}, want{platform.CodeBackslash, platform.KeyUnidentified, "", 0, std}, ""},
		{"emoji sequence", MacKey{KeyCode: 0x00, Characters: "👩‍💻", CharactersIgnoringModifiers: "a"}, want{platform.CodeKeyA, "👩‍💻", "👩‍💻", 0, std}, "one cluster"},
		{"Cmd+CapsLock é", MacKey{KeyCode: 0x13, Flags: lCmd | MacFlagCapsLock, Characters: "é", CharactersIgnoringModifiers: "é"}, want{platform.CodeDigit2, "é", "é", platform.ModSuper, std}, "only ASCII letters are upper-cased"},
		{"e after dead decomposed", MacKey{KeyCode: 0x0E, Characters: "é", CharactersIgnoringModifiers: "e"}, want{platform.CodeKeyE, "é", "é", 0, std}, ""},

		// Named keys from function-key characters.
		{"ArrowUp", MacKey{KeyCode: 0x7E, Flags: arrow, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeArrowUp, platform.KeyArrowUp, "", 0, std}, "NumericPad does not make it numpad"},
		{"ArrowDown", MacKey{KeyCode: 0x7D, Flags: arrow, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeArrowDown, platform.KeyArrowDown, "", 0, std}, ""},
		{"ArrowLeft", MacKey{KeyCode: 0x7B, Flags: arrow, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeArrowLeft, platform.KeyArrowLeft, "", 0, std}, ""},
		{"ArrowRight", MacKey{KeyCode: 0x7C, Flags: arrow, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeArrowRight, platform.KeyArrowRight, "", 0, std}, ""},
		{"Shift+ArrowLeft", MacKey{KeyCode: 0x7B, Flags: arrow | lShift, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeArrowLeft, platform.KeyArrowLeft, "", platform.ModShift, std}, ""},
		{"Option+ArrowLeft", MacKey{KeyCode: 0x7B, Flags: arrow | lOpt, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeArrowLeft, platform.KeyArrowLeft, "", platform.ModAlt, std}, ""},
		{"Cmd+ArrowRight", MacKey{KeyCode: 0x7C, Flags: arrow | lCmd, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeArrowRight, platform.KeyArrowRight, "", platform.ModSuper, std}, ""},
		{"Home", MacKey{KeyCode: 0x73, Flags: nav, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeHome, platform.KeyHome, "", 0, std}, ""},
		{"End", MacKey{KeyCode: 0x77, Flags: nav, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeEnd, platform.KeyEnd, "", 0, std}, ""},
		{"PageUp", MacKey{KeyCode: 0x74, Flags: nav, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodePageUp, platform.KeyPageUp, "", 0, std}, ""},
		{"PageDown", MacKey{KeyCode: 0x79, Flags: nav, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodePageDown, platform.KeyPageDown, "", 0, std}, ""},
		{"forward Delete", MacKey{KeyCode: 0x75, Flags: nav, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeDelete, platform.KeyDelete, "", 0, std}, ""},
		{"Help", MacKey{KeyCode: 0x72, Flags: nav, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeInsert, platform.KeyHelp, "", 0, std}, ""},
		{"Help without characters", MacKey{KeyCode: 0x72, Flags: nav}, want{platform.CodeInsert, platform.KeyHelp, "", 0, std}, ""},
		{"ContextMenu", MacKey{KeyCode: 0x6E, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeContextMenu, platform.KeyContextMenu, "", 0, std}, ""},

		// Named keys from control characters.
		{"Backspace", MacKey{KeyCode: 0x33, Characters: "\x7f", CharactersIgnoringModifiers: "\x7f"}, want{platform.CodeBackspace, platform.KeyBackspace, "", 0, std}, ""},
		{"Option+Backspace", MacKey{KeyCode: 0x33, Flags: lOpt, Characters: "\x7f", CharactersIgnoringModifiers: "\x7f"}, want{platform.CodeBackspace, platform.KeyBackspace, "", platform.ModAlt, std}, ""},
		{"Escape", MacKey{KeyCode: 0x35, Characters: "\x1b", CharactersIgnoringModifiers: "\x1b"}, want{platform.CodeEscape, platform.KeyEscape, "", 0, std}, ""},
		{"Tab", MacKey{KeyCode: 0x30, Characters: "\t", CharactersIgnoringModifiers: "\t"}, want{platform.CodeTab, platform.KeyTab, "", 0, std}, ""},
		{"Shift+Tab", MacKey{KeyCode: 0x30, Flags: lShift, Characters: "\x19", CharactersIgnoringModifiers: "\x19"}, want{platform.CodeTab, platform.KeyTab, "", platform.ModShift, std}, ""},
		{"Ctrl+Tab", MacKey{KeyCode: 0x30, Flags: lCtrl, Characters: "\t", CharactersIgnoringModifiers: "\t"}, want{platform.CodeTab, platform.KeyTab, "", platform.ModCtrl, std}, ""},
		{"Enter", MacKey{KeyCode: 0x24, Characters: "\r", CharactersIgnoringModifiers: "\r"}, want{platform.CodeEnter, platform.KeyEnter, "", 0, std}, ""},
		{"Cmd+Enter", MacKey{KeyCode: 0x24, Flags: lCmd, Characters: "\r", CharactersIgnoringModifiers: "\r"}, want{platform.CodeEnter, platform.KeyEnter, "", platform.ModSuper, std}, ""},
		{"keypad Enter", MacKey{KeyCode: 0x4C, Flags: pad, Characters: "\x03", CharactersIgnoringModifiers: "\x03"}, want{platform.CodeNumpadEnter, platform.KeyEnter, "", 0, num}, ""},

		// Keypad.
		{"keypad 1", MacKey{KeyCode: 0x53, Flags: pad, Characters: "1", CharactersIgnoringModifiers: "1"}, want{platform.CodeNumpad1, "1", "1", 0, num}, ""},
		{"keypad 0", MacKey{KeyCode: 0x52, Flags: pad, Characters: "0", CharactersIgnoringModifiers: "0"}, want{platform.CodeNumpad0, "0", "0", 0, num}, ""},
		{"keypad 9", MacKey{KeyCode: 0x5C, Flags: pad, Characters: "9", CharactersIgnoringModifiers: "9"}, want{platform.CodeNumpad9, "9", "9", 0, num}, ""},
		{"keypad *", MacKey{KeyCode: 0x43, Flags: pad, Characters: "*", CharactersIgnoringModifiers: "*"}, want{platform.CodeNumpadMultiply, "*", "*", 0, num}, ""},
		{"keypad +", MacKey{KeyCode: 0x45, Flags: pad, Characters: "+", CharactersIgnoringModifiers: "+"}, want{platform.CodeNumpadAdd, "+", "+", 0, num}, ""},
		{"keypad -", MacKey{KeyCode: 0x4E, Flags: pad, Characters: "-", CharactersIgnoringModifiers: "-"}, want{platform.CodeNumpadSubtract, "-", "-", 0, num}, ""},
		{"keypad /", MacKey{KeyCode: 0x4B, Flags: pad, Characters: "/", CharactersIgnoringModifiers: "/"}, want{platform.CodeNumpadDivide, "/", "/", 0, num}, ""},
		{"keypad =", MacKey{KeyCode: 0x51, Flags: pad, Characters: "=", CharactersIgnoringModifiers: "="}, want{platform.CodeNumpadEqual, "=", "=", 0, num}, ""},
		{"keypad .", MacKey{KeyCode: 0x41, Flags: pad, Characters: ".", CharactersIgnoringModifiers: "."}, want{platform.CodeNumpadDecimal, ".", ".", 0, num}, ""},
		{"keypad decimal comma", MacKey{KeyCode: 0x41, Flags: pad, Characters: ",", CharactersIgnoringModifiers: ","}, want{platform.CodeNumpadDecimal, ",", ",", 0, num}, "German layout"},
		{"keypad Clear", MacKey{KeyCode: 0x47, Flags: pad, Characters: "", CharactersIgnoringModifiers: ""}, want{platform.CodeNumLock, platform.KeyClear, "", 0, std}, ""},
		{"Cmd+keypad 1", MacKey{KeyCode: 0x53, Flags: pad | lCmd, Characters: "1", CharactersIgnoringModifiers: "1"}, want{platform.CodeNumpad1, "1", "1", platform.ModSuper, num}, ""},

		// ISO and JIS keys.
		{"ISO section on ANSI table", MacKey{KeyCode: 0x0A, Characters: "§", CharactersIgnoringModifiers: "§"}, want{platform.CodeIntlBackslash, "§", "§", 0, std}, ""},
		{"ISO section on ISO keyboard", MacKey{KeyCode: 0x0A, ISO: true, Characters: "§", CharactersIgnoringModifiers: "§"}, want{platform.CodeBackquote, "§", "§", 0, std}, ""},
		{"ISO < key", MacKey{KeyCode: 0x32, ISO: true, Characters: "<", CharactersIgnoringModifiers: "<"}, want{platform.CodeIntlBackslash, "<", "<", 0, std}, ""},
		{"JIS Yen", MacKey{KeyCode: 0x5D, Characters: "¥", CharactersIgnoringModifiers: "¥"}, want{platform.CodeIntlYen, "¥", "¥", 0, std}, ""},
		{"JIS Underscore", MacKey{KeyCode: 0x5E, Characters: "_", CharactersIgnoringModifiers: "_"}, want{platform.CodeIntlRo, "_", "_", 0, std}, ""},
		{"JIS keypad comma", MacKey{KeyCode: 0x5F, Flags: pad, Characters: ",", CharactersIgnoringModifiers: ","}, want{platform.CodeNumpadComma, ",", ",", 0, num}, ""},
		{"JIS Eisu", MacKey{KeyCode: 0x66}, want{platform.CodeLang2, platform.KeyEisu, "", 0, std}, ""},
		{"JIS Kana", MacKey{KeyCode: 0x68}, want{platform.CodeLang1, platform.KeyKanaMode, "", 0, std}, ""},

		// Media keys macOS reports as keys, and unknown keys.
		{"VolumeUp", MacKey{KeyCode: 0x48}, want{platform.CodeAudioVolumeUp, platform.KeyAudioVolumeUp, "", 0, std}, ""},
		{"VolumeDown", MacKey{KeyCode: 0x49}, want{platform.CodeAudioVolumeDown, platform.KeyAudioVolumeDown, "", 0, std}, ""},
		{"Mute", MacKey{KeyCode: 0x4A}, want{platform.CodeAudioVolumeMute, platform.KeyAudioVolumeMute, "", 0, std}, ""},
		{"unknown key", MacKey{KeyCode: 0x34}, want{platform.UnidentifiedKeyCode(0x34), platform.KeyUnidentified, "", 0, std}, ""},
		{"unknown key with text", MacKey{KeyCode: 0x90, Characters: "q", CharactersIgnoringModifiers: "q"}, want{platform.UnidentifiedKeyCode(0x90), "q", "q", 0, std}, ""},
	} {
		c.in.Time = 42 * time.Millisecond
		ev, ok := MacKeyDown(c.in)
		got := want{ev.Code, ev.Key, ev.Text, ev.Mods, ev.Location}
		if !ok || got != c.want || ev.Action != platform.KeyPress || ev.Repeat || ev.Synthetic || ev.Time != c.in.Time {
			t.Errorf("%s (%s): got ok %v %+v (action %v repeat %v time %v), want %+v", c.name, c.check, ok, got, ev.Action, ev.Repeat, ev.Time, c.want)
		}
		if ev.Location != ev.Code.Location() {
			t.Errorf("%s: Location %v, Code says %v", c.name, ev.Location, ev.Code.Location())
		}
		// The release has the same Code, Key, Mods and Location, no Text.
		up, ok := MacKeyUp(c.in)
		gotUp := want{up.Code, up.Key, up.Text, up.Mods, up.Location}
		wantUp := c.want
		wantUp.text = ""
		if !ok || gotUp != wantUp || up.Action != platform.KeyRelease {
			t.Errorf("%s up: got ok %v %+v, want %+v", c.name, ok, gotUp, wantUp)
		}
	}
}

func TestMacKeyRepeat(t *testing.T) {
	in := MacKey{KeyCode: 0x00, Characters: "a", CharactersIgnoringModifiers: "a", Repeat: true}
	ev, ok := MacKeyDown(in)
	if !ok || !ev.Repeat || ev.Key != "a" || ev.Text != "a" {
		t.Errorf("repeat: %+v %v", ev, ok)
	}
	if up, _ := MacKeyUp(in); up.Repeat {
		t.Error("a release is never a repeat")
	}
	// Holding "a" and pressing Shift: the next repeat reports "A".
	in.Flags, in.Characters, in.CharactersIgnoringModifiers = lShift, "A", "A"
	if ev, _ := MacKeyDown(in); ev.Key != "A" || ev.Text != "A" || ev.Mods != platform.ModShift {
		t.Errorf("repeat with Shift: %+v", ev)
	}
}

func TestMacKeyDownModifierKeys(t *testing.T) {
	// macOS reports modifiers through flagsChanged: only; a keyDown: for one
	// is dropped.
	for kvk := range macModKeys {
		if _, ok := MacKeyDown(MacKey{KeyCode: kvk}); ok {
			t.Errorf("keyDown of modifier %#02x accepted", kvk)
		}
		if _, ok := MacKeyUp(MacKey{KeyCode: kvk}); ok {
			t.Errorf("keyUp of modifier %#02x accepted", kvk)
		}
	}
	if _, ok := MacKeyDown(MacKey{KeyCode: kvkCapsLock}); ok {
		t.Error("keyDown of Caps Lock accepted")
	}
}

// macFlags simulates a window that receives flagsChanged: events: it keeps
// the previous flags and the Held set, as the backend does, and records
// what is delivered.
type macFlags struct {
	held Held
	prev uint64
}

func (s *macFlags) change(kvk uint16, flags uint64) []string {
	var out []string
	for _, e := range MacFlagsChanged(&s.held, kvk, s.prev, flags, time.Second) {
		if e.Location != e.Code.Location() || e.Text != "" || e.Repeat || e.Time != time.Second {
			out = append(out, fmt.Sprintf("bad event %+v", e))
		}
		if s.held.Accept(e) {
			out = append(out, fmt.Sprintf("%v %v %v %v", e.Action, e.Code, e.Key, e.Mods))
		}
	}
	s.prev = flags
	return out
}

func TestMacFlagsChangedEachModifier(t *testing.T) {
	for _, c := range []struct {
		kvk   uint16
		flags uint64
		code  platform.KeyCode
		key   platform.Key
		mods  platform.Modifiers
	}{
		{kvkShift, MacFlagShift | MacDeviceLeftShift, platform.CodeShiftLeft, platform.KeyShift, platform.ModShift},
		{kvkRightShift, MacFlagShift | MacDeviceRightShift, platform.CodeShiftRight, platform.KeyShift, platform.ModShift},
		{kvkControl, MacFlagControl | MacDeviceLeftControl, platform.CodeControlLeft, platform.KeyControl, platform.ModCtrl},
		{kvkRightControl, MacFlagControl | MacDeviceRightControl, platform.CodeControlRight, platform.KeyControl, platform.ModCtrl},
		{kvkOption, MacFlagOption | MacDeviceLeftOption, platform.CodeAltLeft, platform.KeyAlt, platform.ModAlt},
		{kvkRightOption, MacFlagOption | MacDeviceRightOption, platform.CodeAltRight, platform.KeyAlt, platform.ModAlt},
		{kvkCommand, MacFlagCommand | MacDeviceLeftCommand, platform.CodeMetaLeft, platform.KeyMeta, platform.ModSuper},
		{kvkRightCommand, MacFlagCommand | MacDeviceRightCommand, platform.CodeMetaRight, platform.KeyMeta, platform.ModSuper},
		{kvkFunction, MacFlagFunction, platform.CodeFn, platform.KeyFn, 0},
	} {
		for _, noDevice := range []bool{false, true} {
			flags := c.flags
			if noDevice {
				flags &= 0xFFFF0000
			}
			// NX_NONCOALSESCEDMASK (0x100) is set on real events; it changes nothing.
			flags |= 0x100
			var s macFlags
			s.prev = 0x100
			press := s.change(c.kvk, flags)
			release := s.change(c.kvk, 0x100)
			wantPress := fmt.Sprintf("press %v %v %v", c.code, c.key, c.mods)
			wantRelease := fmt.Sprintf("release %v %v %v", c.code, c.key, platform.Modifiers(0))
			if !slices.Equal(press, []string{wantPress}) || !slices.Equal(release, []string{wantRelease}) {
				t.Errorf("%v (no device bits %v): press %q release %q, want %q and %q", c.code, noDevice, press, release, wantPress, wantRelease)
			}
			if s.held.Len() != 0 {
				t.Errorf("%v: still held", c.code)
			}
		}
	}
}

func TestMacFlagsChangedBothSides(t *testing.T) {
	type step struct {
		kvk   uint16
		flags uint64
		want  []string
	}
	const (
		shift = MacFlagShift
		ls    = MacDeviceLeftShift
		rs    = MacDeviceRightShift
		cmd   = MacFlagCommand
		lc    = MacDeviceLeftCommand
		rc    = MacDeviceRightCommand
	)
	for _, c := range []struct {
		name  string
		steps []step
	}{
		{"left, right, release left, release right", []step{
			{kvkShift, shift | ls, []string{"press ShiftLeft Shift 1"}},
			{kvkRightShift, shift | ls | rs, []string{"press ShiftRight Shift 1"}},
			{kvkShift, shift | rs, []string{"release ShiftLeft Shift 1"}},
			{kvkRightShift, 0, []string{"release ShiftRight Shift 0"}},
		}},
		{"left, right, release right, release left", []step{
			{kvkCommand, cmd | lc, []string{"press MetaLeft Meta 8"}},
			{kvkRightCommand, cmd | lc | rc, []string{"press MetaRight Meta 8"}},
			{kvkRightCommand, cmd | lc, []string{"release MetaRight Meta 8"}},
			{kvkCommand, 0, []string{"release MetaLeft Meta 0"}},
		}},
		{"without device bits", []step{
			{kvkShift, shift, []string{"press ShiftLeft Shift 1"}},
			{kvkRightShift, shift, []string{"press ShiftRight Shift 1"}},
			{kvkShift, shift, []string{"release ShiftLeft Shift 1"}},
			{kvkRightShift, 0, []string{"release ShiftRight Shift 0"}},
		}},
		{"both released in one event", []step{
			{kvkShift, shift | ls, []string{"press ShiftLeft Shift 1"}},
			{kvkRightShift, shift | ls | rs, []string{"press ShiftRight Shift 1"}},
			{kvkShift, 0, []string{"release ShiftLeft Shift 0", "release ShiftRight Shift 0"}},
		}},
		{"a repeated event changes nothing", []step{
			{kvkShift, shift | ls, []string{"press ShiftLeft Shift 1"}},
			{kvkShift, shift | ls, nil},
			{kvkShift, 0, []string{"release ShiftLeft Shift 0"}},
			{kvkShift, 0, nil},
		}},
		{"held before focus: release dropped", []step{
			{kvkShift, 0, nil},
		}},
		{"Shift with Command held", []step{
			{kvkCommand, cmd | lc, []string{"press MetaLeft Meta 8"}},
			{kvkShift, cmd | lc | shift | ls, []string{"press ShiftLeft Shift 9"}},
			{kvkCommand, shift | ls, []string{"release MetaLeft Meta 1"}},
			{kvkShift, 0, []string{"release ShiftLeft Shift 0"}},
		}},
		{"other key codes", []step{
			{0, shift | ls, nil}, // keyCode 0 carries no key in flagsChanged:
			{0x7E, shift | ls, nil},
		}},
	} {
		var s macFlags
		for i, st := range c.steps {
			if got := s.change(st.kvk, st.flags); !slices.Equal(got, st.want) {
				t.Errorf("%s, step %d: got %q, want %q", c.name, i, got, st.want)
			}
		}
	}
}

func TestMacFlagsChangedCapsLock(t *testing.T) {
	var s macFlags
	on := s.change(kvkCapsLock, MacFlagCapsLock)
	want := []string{"press CapsLock CapsLock 0", "release CapsLock CapsLock 0"}
	if !slices.Equal(on, want) {
		t.Errorf("Caps Lock on: %q, want %q", on, want)
	}
	if got := s.change(kvkCapsLock, MacFlagCapsLock); got != nil {
		t.Errorf("no change: %q", got)
	}
	if off := s.change(kvkCapsLock, 0); !slices.Equal(off, want) {
		t.Errorf("Caps Lock off: %q, want %q", off, want)
	}
	// With Shift held, the Mods of the events say so.
	s.prev = lShift
	s.held.Accept(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeShiftLeft})
	got := s.change(kvkCapsLock, lShift|MacFlagCapsLock)
	if want := []string{"press CapsLock CapsLock 1", "release CapsLock CapsLock 1"}; !slices.Equal(got, want) {
		t.Errorf("Caps Lock with Shift: %q, want %q", got, want)
	}
	if s.held.IsHeld(platform.CodeCapsLock) {
		t.Error("Caps Lock is never held")
	}
}

func TestMacFlagsChangedNilHeld(t *testing.T) {
	got := MacFlagsChanged(nil, kvkShift, 0, lShift, 0)
	if len(got) != 1 || got[0].Action != platform.KeyPress || got[0].Code != platform.CodeShiftLeft {
		t.Errorf("press with nil Held: %+v", got)
	}
	if got := MacFlagsChanged(nil, kvkShift, lShift, 0, 0); got != nil {
		t.Errorf("release with nil (empty) Held: %+v", got)
	}
}

func TestMacFocusLossReleasesModifiers(t *testing.T) {
	var s macFlags
	s.change(kvkCommand, lCmd)
	ev, _ := MacKeyDown(MacKey{KeyCode: 0x01, Flags: lCmd, Characters: "s", CharactersIgnoringModifiers: "s"})
	s.held.Accept(ev)
	rel := s.held.ReleaseAll(time.Second)
	if len(rel) != 2 || rel[0].Code != platform.CodeMetaLeft || rel[1].Code != platform.CodeKeyS || !rel[0].Synthetic || rel[1].Key != "s" {
		t.Errorf("synthetic releases: %+v", rel)
	}
}
