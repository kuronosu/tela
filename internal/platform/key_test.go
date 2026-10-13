package platform

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestKeyCodeNames(t *testing.T) {
	seen := map[string]KeyCode{}
	for c := CodeUnidentified + 1; c < numKeyCodes; c++ {
		name := keyCodeNames[c]
		if name == "" {
			t.Errorf("KeyCode %d has no name", c)
			continue
		}
		if prev, dup := seen[name]; dup {
			t.Errorf("%q names %d and %d", name, prev, c)
		}
		seen[name] = c
		if got, ok := ParseKeyCode(name); !ok || got != c {
			t.Errorf("ParseKeyCode(%q) = %v, %v", name, got, ok)
		}
		if !c.Identified() {
			t.Errorf("%v is not Identified", c)
		}
		if _, ok := c.Native(); ok {
			t.Errorf("%v has a native code", c)
		}
		if gs := fmt.Sprintf("%#v", c); gs != "platform.Code"+name {
			t.Errorf("GoString = %q", gs)
		}
	}
	// A sample of W3C values, spelled as the specification does.
	for c, want := range map[KeyCode]string{
		CodeKeyA: "KeyA", CodeDigit1: "Digit1", CodeEnter: "Enter", CodeShiftLeft: "ShiftLeft",
		CodeArrowUp: "ArrowUp", CodeNumpad0: "Numpad0", CodeF1: "F1", CodeF24: "F24",
		CodeIntlBackslash: "IntlBackslash", CodeMetaLeft: "MetaLeft", CodeBackquote: "Backquote",
		CodeMediaPlayPause: "MediaPlayPause", CodeAudioVolumeMute: "AudioVolumeMute",
		CodeContextMenu: "ContextMenu", CodePrintScreen: "PrintScreen",
	} {
		if c.String() != want {
			t.Errorf("%d.String() = %q, want %q", c, c, want)
		}
	}
	for _, bad := range []string{"", "Unidentified", "keyA", "SuperLeft", "OSLeft"} {
		if c, ok := ParseKeyCode(bad); ok {
			t.Errorf("ParseKeyCode(%q) = %v", bad, c)
		}
	}
}

func TestUnidentifiedKeyCode(t *testing.T) {
	if CodeUnidentified.Identified() || CodeUnidentified.String() != "Unidentified" {
		t.Errorf("CodeUnidentified: identified %v, %q", CodeUnidentified.Identified(), CodeUnidentified)
	}
	if _, ok := CodeUnidentified.Native(); ok {
		t.Errorf("CodeUnidentified carries a native code")
	}
	var zero KeyEvent
	if zero.Code != CodeUnidentified {
		t.Errorf("the zero KeyEvent has Code %v", zero.Code)
	}
	for _, n := range []uint32{0, 1, 0xE05B, 1<<31 - 1} {
		c := UnidentifiedKeyCode(n)
		if c.Identified() || c.String() != "Unidentified" {
			t.Errorf("UnidentifiedKeyCode(%#x) = %v, identified %v", n, c, c.Identified())
		}
		if got, ok := c.Native(); !ok || got != n {
			t.Errorf("UnidentifiedKeyCode(%#x).Native() = %#x, %v", n, got, ok)
		}
		if c == CodeUnidentified {
			t.Errorf("UnidentifiedKeyCode(%#x) == CodeUnidentified", n)
		}
		if gs := fmt.Sprintf("%#v", c); gs != fmt.Sprintf("platform.UnidentifiedKeyCode(%#x)", n) {
			t.Errorf("GoString = %q", gs)
		}
	}
	first, second := UnidentifiedKeyCode(5), UnidentifiedKeyCode(5)
	if first == UnidentifiedKeyCode(6) || first != second {
		t.Errorf("unidentified codes do not compare by native code")
	}
	if s := KeyCode(numKeyCodes + 3).String(); !strings.HasPrefix(s, "KeyCode(") {
		t.Errorf("invalid KeyCode prints %q", s)
	}
	defer func() {
		if recover() == nil {
			t.Errorf("UnidentifiedKeyCode(1<<31) did not panic")
		}
	}()
	UnidentifiedKeyCode(1 << 31)
}

func TestKeyCodeLocation(t *testing.T) {
	for c, want := range map[KeyCode]KeyLocation{
		CodeShiftLeft: KeyLocationLeft, CodeControlLeft: KeyLocationLeft, CodeAltLeft: KeyLocationLeft,
		CodeMetaLeft: KeyLocationLeft, CodeShiftRight: KeyLocationRight, CodeControlRight: KeyLocationRight,
		CodeAltRight: KeyLocationRight, CodeMetaRight: KeyLocationRight,
		CodeNumpad0: KeyLocationNumpad, CodeNumpad9: KeyLocationNumpad, CodeNumpadEnter: KeyLocationNumpad,
		CodeNumpadSubtract: KeyLocationNumpad, CodeNumpadParenLeft: KeyLocationNumpad,
		CodeNumLock: KeyLocationStandard, CodeEnter: KeyLocationStandard, CodeKeyA: KeyLocationStandard,
		CodeDigit1: KeyLocationStandard, CodeEscape: KeyLocationStandard, CodeUnidentified: KeyLocationStandard,
		UnidentifiedKeyCode(9): KeyLocationStandard,
	} {
		if got := c.Location(); got != want {
			t.Errorf("%v.Location() = %v, want %v", c, got, want)
		}
	}
}

func TestNamedKeys(t *testing.T) {
	seen := map[Key]bool{}
	for _, k := range allNamedKeys {
		if seen[k] {
			t.Errorf("named key %q listed twice", k)
		}
		seen[k] = true
		if !k.IsNamed() {
			t.Errorf("%q.IsNamed() = false", k)
		}
		// W3C named values are ASCII words of at least two letters, which
		// is what keeps them apart from character values.
		if len(k) < 2 || utf8.RuneCountInString(string(k)) != len(k) {
			t.Errorf("named key %q is not an ASCII word", k)
		}
	}
	for _, k := range []Key{"a", "A", "é", " ", "1", "€", "ß", "", "Space", "Win", "OS"} {
		if k.IsNamed() {
			t.Errorf("%q.IsNamed() = true", k)
		}
	}
}

func TestKeyEnumStrings(t *testing.T) {
	if KeyPress.String() != "press" || KeyRelease.String() != "release" || KeyAction(9).String() != "KeyAction(invalid)" {
		t.Errorf("KeyAction strings: %v %v %v", KeyPress, KeyRelease, KeyAction(9))
	}
	for l, want := range map[KeyLocation]string{
		KeyLocationStandard: "standard", KeyLocationLeft: "left", KeyLocationRight: "right",
		KeyLocationNumpad: "numpad", 9: "KeyLocation(invalid)",
	} {
		if l.String() != want {
			t.Errorf("KeyLocation(%d) = %q, want %q", l, l, want)
		}
	}
	// The DOM's KeyboardEvent.location values.
	if KeyLocationStandard != 0 || KeyLocationLeft != 1 || KeyLocationRight != 2 || KeyLocationNumpad != 3 {
		t.Errorf("KeyLocation values differ from DOM_KEY_LOCATION_*")
	}
	var e Event = KeyEvent{}
	_ = e
}
