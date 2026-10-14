// Package utf16x converts text offsets between UTF-16 code units and UTF-8
// bytes for the backends whose OS text APIs count UTF-16 (Win32 IMM32 and
// macOS NSTextInputClient). The platform contract uses UTF-8 byte offsets
// everywhere (see "Text input and IME" in package platform); these helpers
// are where a backend converts. They match internal/text/edit's
// UTF16ToByte and ByteToUTF16, which platform cannot import.
package utf16x

import "unicode/utf8"

// units is the number of UTF-16 code units of r: 2 for characters outside
// the Basic Multilingual Plane, 1 otherwise (U+FFFD for invalid bytes).
func units(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// Len returns the length of s in UTF-16 code units.
func Len(s string) int {
	n := 0
	for _, r := range s {
		n += units(r)
	}
	return n
}

// ToByte returns the byte offset in s of the UTF-16 offset u. An offset
// inside a surrogate pair maps to the start of its character; negative
// offsets map to 0 and offsets past the end to len(s).
func ToByte(s string, u int) int {
	n := 0
	for i, r := range s {
		w := units(r)
		if n+w > u {
			return i
		}
		n += w
	}
	return len(s)
}

// FromByte returns the UTF-16 offset of the byte offset b in s. An offset
// inside a UTF-8 sequence counts the characters before it; offsets are
// clamped to s.
func FromByte(s string, b int) int {
	b = max(0, min(b, len(s)))
	for b > 0 && b < len(s) && !utf8.RuneStart(s[b]) {
		b--
	}
	return Len(s[:b])
}
