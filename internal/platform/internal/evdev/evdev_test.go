package evdev

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestKnownKeys(t *testing.T) {
	for _, tc := range []struct {
		evdev uint32
		code  string
	}{
		{1, "Escape"}, {2, "Digit1"}, {11, "Digit0"}, {16, "KeyQ"}, {30, "KeyA"}, {44, "KeyZ"},
		{28, "Enter"}, {29, "ControlLeft"}, {42, "ShiftLeft"}, {54, "ShiftRight"}, {56, "AltLeft"},
		{100, "AltRight"}, {125, "MetaLeft"}, {126, "MetaRight"}, {57, "Space"}, {15, "Tab"},
		{14, "Backspace"}, {41, "Backquote"}, {43, "Backslash"}, {86, "IntlBackslash"}, {89, "IntlRo"},
		{124, "IntlYen"}, {103, "ArrowUp"}, {105, "ArrowLeft"}, {106, "ArrowRight"}, {108, "ArrowDown"},
		{82, "Numpad0"}, {79, "Numpad1"}, {96, "NumpadEnter"}, {98, "NumpadDivide"}, {55, "NumpadMultiply"},
		{83, "NumpadDecimal"}, {117, "NumpadEqual"}, {121, "NumpadComma"}, {69, "NumLock"},
		{59, "F1"}, {68, "F10"}, {87, "F11"}, {88, "F12"}, {183, "F13"}, {194, "F24"},
		{99, "PrintScreen"}, {70, "ScrollLock"}, {119, "Pause"}, {127, "ContextMenu"},
		{113, "AudioVolumeMute"}, {114, "AudioVolumeDown"}, {115, "AudioVolumeUp"},
		{163, "MediaTrackNext"}, {164, "MediaPlayPause"}, {165, "MediaTrackPrevious"}, {166, "MediaStop"},
		{158, "BrowserBack"}, {159, "BrowserForward"}, {172, "BrowserHome"}, {217, "BrowserSearch"},
		{122, "Lang1"}, {123, "Lang2"}, {92, "Convert"}, {94, "NonConvert"}, {93, "KanaMode"},
		{464, "Fn"},
	} {
		c := KeyCode(tc.evdev)
		if c.String() != tc.code || !c.Identified() {
			t.Errorf("KeyCode(%d) = %v, want %s", tc.evdev, c, tc.code)
		}
		if got := X11KeyCode(tc.evdev + X11Offset); got != c {
			t.Errorf("X11KeyCode(%d) = %v, want %v", tc.evdev+X11Offset, got, c)
		}
		if back, ok := Code(c); !ok || back != tc.evdev {
			t.Errorf("Code(%v) = %d, %v; want %d", c, back, ok, tc.evdev)
		}
	}
}

func TestUnknownKeysKeepTheirCode(t *testing.T) {
	for _, code := range []uint32{0, 84, 95, 112, 118, 240, 248, 465, 0x2ff, 100000} {
		c := KeyCode(code)
		if c.Identified() {
			t.Errorf("KeyCode(%d) = %v, want unidentified", code, c)
			continue
		}
		if n, ok := c.Native(); !ok || n != code {
			t.Errorf("KeyCode(%d).Native() = %d, %v", code, n, ok)
		}
		if c.String() != "Unidentified" {
			t.Errorf("KeyCode(%d).String() = %q", code, c)
		}
		if back, ok := Code(c); !ok || back != code {
			t.Errorf("Code(KeyCode(%d)) = %d, %v", code, back, ok)
		}
	}
	if KeyCode(95) == KeyCode(118) {
		t.Errorf("two unknown keys compare equal")
	}
	if c := X11KeyCode(3); c != platform.CodeUnidentified {
		t.Errorf("X11KeyCode(3) = %#v, want CodeUnidentified", c)
	}
	if _, ok := Code(platform.CodeUnidentified); ok {
		t.Errorf("CodeUnidentified has an evdev code")
	}
	if _, ok := Code(platform.CodeNumpadHash); ok {
		t.Errorf("NumpadHash has an evdev code")
	}
}

// TestTableIsInjective: two evdev codes never map to the same physical key,
// so Code is a true inverse.
func TestTableIsInjective(t *testing.T) {
	seen := map[platform.KeyCode]int{}
	for code, c := range codes {
		if c == platform.CodeUnidentified {
			continue
		}
		if !c.Identified() {
			t.Errorf("codes[%d] = %#v is not an identified code", code, c)
		}
		if prev, dup := seen[c]; dup {
			t.Errorf("%v is mapped from evdev %d and %d", c, prev, code)
		}
		seen[c] = code
	}
	if len(seen) < 150 {
		t.Errorf("only %d keys mapped", len(seen))
	}
}

// TestTableMatchesKernelHeader checks the numbers of the table against the
// KEY_* names in its comments, using the kernel header when it is installed.
func TestTableMatchesKernelHeader(t *testing.T) {
	header, err := os.ReadFile("/usr/include/linux/input-event-codes.h")
	if err != nil {
		t.Skip("linux/input-event-codes.h is not installed")
	}
	defs := map[string]uint32{}
	for _, m := range regexp.MustCompile(`(?m)^#define\s+(KEY_\w+)\s+(0x[0-9a-fA-F]+|\d+)\b`).FindAllSubmatch(header, -1) {
		v, err := strconv.ParseUint(string(m[2]), 0, 32)
		if err == nil {
			defs[string(m[1])] = uint32(v)
		}
	}
	src, err := os.ReadFile("evdev.go")
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile(`(?m)^\s*(\d+):\s+platform\.Code\w+,\s*// (KEY_\w+)`).FindAllSubmatch(src, -1)
	if len(rows) < 150 {
		t.Fatalf("parsed only %d table rows", len(rows))
	}
	for _, m := range rows {
		n, _ := strconv.ParseUint(string(m[1]), 10, 32)
		name := string(m[2])
		want, ok := defs[name]
		if !ok {
			t.Errorf("%s is not defined by the kernel header", name)
			continue
		}
		if uint32(n) != want {
			t.Errorf("table row %d is commented %s, which is %d", n, name, want)
		}
	}
}
