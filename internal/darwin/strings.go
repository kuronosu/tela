package darwin

import (
	"strings"
	"unicode/utf8"
)

// CString returns s as valid UTF-8 without NUL characters, ready to be
// passed to a function taking a C string (purego appends the terminator):
// invalid bytes and NUL characters become U+FFFD.
func CString(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return strings.ReplaceAll(s, "\x00", "�")
}
