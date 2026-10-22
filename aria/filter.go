// Portions of this file follow React Aria's useFilter
// (https://github.com/adobe/react-spectrum, packages/@react-aria/i18n),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Filter decides whether the text of an item matches what the user typed:
// the filter of [ComboboxOptions]. Contains, StartsWith and EndsWith are
// ready to use.
type Filter func(text, query string) bool

// Contains reports whether text contains query anywhere, ignoring case
// and diacritics ("cafe" matches "Café"; React Aria's useFilter with the
// "base" sensitivity of its collator). An empty query matches everything.
func Contains(text, query string) bool {
	return strings.Contains(fold(text), fold(query))
}

// StartsWith reports whether text starts with query, ignoring case and
// diacritics. An empty query matches everything.
func StartsWith(text, query string) bool {
	return strings.HasPrefix(fold(text), fold(query))
}

// EndsWith reports whether text ends with query, ignoring case and
// diacritics. An empty query matches everything.
func EndsWith(text, query string) bool {
	return strings.HasSuffix(fold(text), fold(query))
}

// fold maps a text to what compares equal for the "base" sensitivity of a
// collator: decomposed (NFD), without combining marks, in lower case.
// "Çé" folds to "ce".
func fold(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
