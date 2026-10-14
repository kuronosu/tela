package win32

import (
	"slices"
	"testing"
	"time"
	utf16pkg "unicode/utf16"

	"github.com/kuronosu/tela/internal/platform"
)

func TestLineEndings(t *testing.T) {
	for _, tc := range []struct{ name, text, toClipboard, back string }{
		{"empty", "", "", ""},
		{"no breaks", "tela", "tela", "tela"},
		{"lf", "a\nb\n", "a\r\nb\r\n", "a\nb\n"},
		{"crlf", "a\r\nb", "a\r\nb", "a\nb"},
		{"lone cr", "a\rb", "a\r\nb", "a\nb"},
		{"mixed", "a\r\nb\nc\rd", "a\r\nb\r\nc\r\nd", "a\nb\nc\nd"},
		{"blank lines", "\n\n", "\r\n\r\n", "\n\n"},
		{"cr then lf apart", "a\r\r\nb", "a\r\n\r\nb", "a\n\nb"},
		{"nul is dropped", "a\x00b", "ab", "ab"},
	} {
		got := clipboardFromLF(tc.text)
		if got != tc.toClipboard {
			t.Errorf("%s: clipboardFromLF(%q) = %q, want %q", tc.name, tc.text, got, tc.toClipboard)
		}
		if back := lfFromClipboard(got); back != tc.back {
			t.Errorf("%s: lfFromClipboard(%q) = %q, want %q", tc.name, got, back, tc.back)
		}
	}
	// Converting twice changes nothing: CR LF does not become CR CR LF.
	if got := clipboardFromLF(clipboardFromLF("a\nb")); got != "a\r\nb" {
		t.Errorf("clipboardFromLF is not idempotent: %q", got)
	}
}

func TestUTF16Clipboard(t *testing.T) {
	for _, s := range []string{"", "tela clipboard ✓", "😀 pair", "ñandú\r\nλ"} {
		u := utf16Clipboard(s)
		if u[len(u)-1] != 0 {
			t.Errorf("%q: no terminating NUL", s)
		}
		if got := fromUTF16Clipboard(u); got != s {
			t.Errorf("round trip of %q = %q", s, got)
		}
	}
	// Data longer than its text (GlobalSize rounds up) stops at the NUL.
	u := append(utf16Clipboard("ab"), 'x', 'y')
	if got := fromUTF16Clipboard(u); got != "ab" {
		t.Errorf("text after the NUL: %q", got)
	}
	// No NUL at all: the whole data.
	if got := fromUTF16Clipboard(utf16pkg.Encode([]rune("abc"))); got != "abc" {
		t.Errorf("unterminated data: %q", got)
	}
	// Invalid UTF-8 and unpaired surrogates become U+FFFD.
	if got := fromUTF16Clipboard(utf16Clipboard("a\xffb")); got != "a�b" {
		t.Errorf("invalid UTF-8: %q", got)
	}
	if got := fromUTF16Clipboard([]uint16{'a', 0xD800, 'b', 0}); got != "a�b" {
		t.Errorf("unpaired surrogate: %q", got)
	}
	if !slices.Equal(utf16Clipboard("😀"), []uint16{0xD83D, 0xDE00, 0}) {
		t.Errorf("surrogate pair encoding")
	}
}

func TestSettingsFrom(t *testing.T) {
	for _, tc := range []struct {
		name  string
		light uint32
		known bool
		anim  bool
		ms    uint32
		want  platform.SystemSettings
	}{
		{"light", 1, true, true, 500, platform.SystemSettings{DoubleClickInterval: 500 * time.Millisecond}},
		{"dark", 0, true, true, 400, platform.SystemSettings{DarkMode: true, DoubleClickInterval: 400 * time.Millisecond}},
		{"value missing is light", 0, false, true, 500, platform.SystemSettings{DoubleClickInterval: 500 * time.Millisecond}},
		{"animations off is reduced motion", 1, true, false, 500, platform.SystemSettings{ReducedMotion: true, DoubleClickInterval: 500 * time.Millisecond}},
		{"any non-zero is light", 7, true, true, 0, platform.SystemSettings{}},
	} {
		if got := settingsFrom(tc.light, tc.known, tc.anim, tc.ms); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
