// Portions of this file follow React Aria's useTypeSelect
// (https://github.com/adobe/react-spectrum, packages/@react-aria/selection),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0, and
// Radix UI's typeahead (MIT; no code copied). See THIRD_PARTY.md.

package aria

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// The typeahead of the collections (UseListBox, UseMenu, UseSelect...) and
// of a menu bar's triggers: one search buffer and one matching function,
// with React Aria's rule for collections and Radix's cycling for a bar.

// typeBuffer is the search of a typeahead: the characters typed within a
// second of each other (useTypeSelect's).
type typeBuffer struct {
	search string
	t      core.Timeout
}

// add adds the character that e types to the search and returns it. ok is
// false for a key that types no character, or a space that would start a
// search; space reports a space inside a search, which is a character of
// it and not a press (the caller stops and prevents it).
func (b *typeBuffer) add(e tela.KeyboardEvent) (search string, space, ok bool) {
	s := string(e.Key)
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || n != len(s) || !unicode.IsPrint(r) || (b.search == "" && s == " ") {
		return "", false, false
	}
	space = s == " " && strings.TrimSpace(b.search) != ""
	b.search += s
	b.t.Start(time.Second, b.clear)
	return b.search, space, true
}

// clear empties the search.
func (b *typeBuffer) clear() { b.search = "" }

// typeMatch returns the first usable key whose text starts with search,
// ignoring case, looking from keys[start] around the end. With cycle
// (Radix's typeahead, a menu bar's) a search of one character repeated
// ("ff") is that character, and a one-character search leaves keys[start]
// out, so that typing a letter again moves on to the next match; without
// it (React Aria's getKeyForSearch, from the focused item and then from
// the top) keys[start] counts.
func typeMatch[K comparable](keys []K, start int, search string, text func(K) string, usable func(K) bool, cycle bool) (K, bool) {
	var zero K
	n := len(keys)
	if n == 0 {
		return zero, false
	}
	if cycle {
		first, _ := utf8.DecodeRuneInString(search)
		repeated := true
		for _, c := range search {
			if unicode.ToLower(c) != unicode.ToLower(first) {
				repeated = false
				break
			}
		}
		if repeated {
			search = string(first)
		}
	}
	skip := cycle && utf8.RuneCountInString(search) == 1
	start = min(max(start, 0), n-1)
	for step := range n {
		k := keys[(start+step)%n]
		if (step == 0 && skip) || !usable(k) {
			continue
		}
		if hasPrefixFold(text(k), search) {
			return k, true
		}
	}
	return zero, false
}

// hasPrefixFold reports whether s starts with prefix, ignoring case (the
// base sensitivity of React Aria's collator, for the letters that have a
// simple case).
func hasPrefixFold(s, prefix string) bool {
	for _, p := range prefix {
		if s == "" {
			return false
		}
		r, n := utf8.DecodeRuneInString(s)
		if unicode.ToLower(r) != unicode.ToLower(p) {
			return false
		}
		s = s[n:]
	}
	return true
}
