package win32

import (
	"strings"
	utf16pkg "unicode/utf16"
)

// Windows text uses CR LF as the line ending. The platform layer, like
// browsers and the other backends, speaks LF: the clipboard converts at the
// boundary (browsers on Windows do the same in navigator.clipboard).

// lfFromClipboard converts the text of a CF_UNICODETEXT clipboard to LF line
// endings: CR LF and a lone CR (old Mac text, some terminals) both become LF.
func lfFromClipboard(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// clipboardFromLF converts text to what a Windows clipboard holds: every
// line break (LF, CR LF or a lone CR) is CR LF, and the NUL characters that
// would end the string early are dropped.
func clipboardFromLF(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = lfFromClipboard(s)
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// utf16Clipboard encodes text for CF_UNICODETEXT: UTF-16 with a terminating
// NUL. Invalid UTF-8 becomes U+FFFD.
func utf16Clipboard(s string) []uint16 {
	u := utf16pkg.Encode([]rune(strings.ToValidUTF8(s, "\uFFFD")))
	return append(u, 0)
}

// fromUTF16Clipboard decodes CF_UNICODETEXT data up to the first NUL (or the
// end of the data). Unpaired surrogates become U+FFFD.
func fromUTF16Clipboard(u []uint16) string {
	for i, c := range u {
		if c == 0 {
			u = u[:i]
			break
		}
	}
	return string(utf16pkg.Decode(u))
}
