package edit

import (
	"strings"
	"unicode/utf8"
)

// This file holds the text rules: how inserted and programmatic text is
// cleaned, and maxlength in UTF-16 code units.

// sanitizeInput cleans text the user inserts (typing, paste, IME commit or
// preedit), like Chromium's text controls:
//
//   - invalid UTF-8 becomes U+FFFD;
//   - line breaks (CR LF, CR, LF) become one LF in multi-line mode (HTML's
//     textarea normalization) and one space in single-line mode (what
//     Chromium and Firefox do when pasting into an <input>);
//   - other C0 controls, DEL and C1 controls are removed, except tab.
func sanitizeInput(s string, multiline bool) string {
	if !strings.ContainsFunc(s, needsCleaning) && utf8.ValidString(s) {
		return s
	}
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		switch {
		case r == '\r' || r == '\n':
			if r == '\r' && i < len(s) && s[i] == '\n' {
				i++
			}
			if multiline {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		case isControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeValue cleans a value set by the program (SetText), like HTML's
// value sanitization: invalid UTF-8 becomes U+FFFD, and line breaks are
// removed in single-line mode ("strip newlines") or normalized to LF in
// multi-line mode. Other characters are kept as they are.
func sanitizeValue(s string, multiline bool) string {
	s = strings.ToValidUTF8(s, "�")
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	if multiline {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		return strings.ReplaceAll(s, "\r", "\n")
	}
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s)
}

func needsCleaning(r rune) bool { return r == '\r' || r == '\n' || isControl(r) }

// isControl reports whether r is removed from inserted text: C0 controls
// but tab, DEL and C1 controls (line breaks are handled before).
func isControl(r rune) bool {
	return (r < 0x20 && r != '\t') || (r >= 0x7F && r <= 0x9F)
}

// UTF16Len returns the length of s in UTF-16 code units, the unit of
// HTML's maxlength and of the Windows and macOS text APIs.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Units(r)
	}
	return n
}

func utf16Units(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// UTF16ToByte returns the byte offset in s of the UTF-16 offset u (in code
// units). An offset inside a surrogate pair maps to the start of its
// character; offsets past the end map to len(s).
func UTF16ToByte(s string, u int) int {
	n := 0
	for i, r := range s {
		w := utf16Units(r)
		if n+w > u {
			return i
		}
		n += w
	}
	return len(s)
}

// ByteToUTF16 returns the UTF-16 offset (in code units) of the byte offset
// b in s. An offset inside a UTF-8 sequence counts the characters before
// it.
func ByteToUTF16(s string, b int) int {
	return UTF16Len(s[:clampRuneStart(s, b)])
}

// clampRuneStart clamps b to s and moves it back to the start of a UTF-8
// sequence.
func clampRuneStart(s string, b int) int {
	b = max(0, min(b, len(s)))
	for b > 0 && b < len(s) && !utf8.RuneStart(s[b]) {
		b--
	}
	return b
}

// clampRuneEnd clamps b to s and moves it forward to the end of a UTF-8
// sequence.
func clampRuneEnd(s string, b int) int {
	b = max(0, min(b, len(s)))
	for b < len(s) && !utf8.RuneStart(s[b]) {
		b++
	}
	return b
}

// truncateUTF16 returns the longest prefix of s that ends on one of its
// grapheme cluster boundaries and has at most limit UTF-16 code units
// (Chromium truncates inserted text this way so that maxlength never
// splits a character or a grapheme cluster).
func truncateUTF16(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit || UTF16Len(s) <= limit {
		return s
	}
	cut := 0
	n := 0
	prev := 0
	for _, b := range graphemeBounds(s) {
		n += UTF16Len(s[prev:b])
		if n > limit {
			break
		}
		cut, prev = b, b
	}
	return s[:cut]
}
