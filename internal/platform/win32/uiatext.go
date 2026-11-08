// Copyright 2022 The AccessKit Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 or the MIT license, at
// your option.
//
// The text range moves (Move, MoveEndpointByUnit, ExpandToEnclosingUnit)
// port the logic of AccessKit's platforms/windows/src/text.rs onto Tela's
// flat text (THIRD_PARTY.md).

package win32

import (
	"unicode"
	unicode16 "unicode/utf16"
	"unicode/utf8"
)

// Text ranges of the UIA Text pattern (ADR-246), pure Go.
//
// A range is a pair of offsets in UTF-16 code units (UIA's unit) into
// the text of one node: a text field's Value or a text node's Text. The
// flat tree has no glyph positions and no line layout, so the units are
// approximate:
//
//   - Character: a code point with the marks, variation selectors, emoji
//     modifiers and zero-width-joiner sequences after it, and regional
//     indicator pairs (most grapheme clusters; not every UAX #29 rule).
//   - Word: a run of letters and digits, or of other non-space
//     characters, with the spaces after it (a word starts where a
//     non-space run starts, as in Windows' edit controls); a line break
//     starts a word.
//   - Line and Paragraph: hard line breaks only (LF, CR, CRLF, U+2028,
//     U+2029); a line ends after its break. Soft wraps are not known.
//   - Format, Page and Document: the whole text.

// TextUnit values.
const (
	uiaUnitCharacter = 0
	uiaUnitFormat    = 1
	uiaUnitWord      = 2
	uiaUnitLine      = 3
	uiaUnitParagraph = 4
	uiaUnitPage      = 5
	uiaUnitDocument  = 6
)

// uiaUnitStarts returns the offsets where a unit starts in text, in order,
// beginning with 0 and ending with len(text) (the document end, which is
// the end of the last unit); for an empty text, just 0.
func uiaUnitStarts(text []uint16, unit int32) []int {
	n := len(text)
	starts := []int{0}
	if n == 0 {
		return starts
	}
	switch unit {
	case uiaUnitCharacter:
		starts = uiaCharStarts(text)
	case uiaUnitWord:
		starts = uiaWordStarts(text)
	case uiaUnitLine, uiaUnitParagraph:
		for i := 0; i < n; i++ {
			switch text[i] {
			case '\r':
				if i+1 < n && text[i+1] == '\n' {
					i++
				}
				fallthrough
			case '\n', 0x2028, 0x2029:
				if i+1 < n {
					starts = append(starts, i+1)
				}
			}
		}
	}
	if starts[len(starts)-1] != n {
		starts = append(starts, n)
	}
	return starts
}

// uiaRunes decodes text into code points with their UTF-16 offsets.
func uiaRunes(text []uint16) (runes []rune, offs []int) {
	for i := 0; i < len(text); {
		r, size := rune(text[i]), 1
		if unicode16.IsSurrogate(r) && i+1 < len(text) {
			if d := unicode16.DecodeRune(r, rune(text[i+1])); d != utf8.RuneError {
				r, size = d, 2
			}
		}
		runes, offs = append(runes, r), append(offs, i)
		i += size
	}
	return runes, offs
}

// uiaExtends reports whether r continues the character before it.
func uiaExtends(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Mc, r) ||
		(r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF) || // variation selectors
		(r >= 0x1F3FB && r <= 0x1F3FF) || // emoji skin tone modifiers
		(r >= 0xE0020 && r <= 0xE007F) || // emoji tag sequences
		r == 0x200D // zero width joiner
}

// uiaCharStarts returns the starts of the characters of text.
func uiaCharStarts(text []uint16) []int {
	runes, offs := uiaRunes(text)
	var starts []int
	regional := 0 // regional indicators in the current run
	for i, r := range runes {
		isRI := r >= 0x1F1E6 && r <= 0x1F1FF
		switch {
		case i == 0:
		case uiaExtends(r):
			continue
		case runes[i-1] == 0x200D:
			continue // the character after a joiner joins the sequence
		case runes[i-1] == '\r' && r == '\n':
			continue
		case isRI && regional%2 == 1:
			regional++
			continue // the second of a flag pair
		}
		if isRI {
			regional = 1
		} else {
			regional = 0
		}
		starts = append(starts, offs[i])
	}
	return starts
}

// uiaWordClass returns the class of r for word breaks: 0 space, 1 line
// break, 2 letters and digits, 3 anything else.
func uiaWordClass(r rune) int {
	switch {
	case r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029:
		return 1
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || uiaExtends(r):
		return 2
	}
	return 3
}

// uiaWordStarts returns the starts of the words of text.
func uiaWordStarts(text []uint16) []int {
	runes, offs := uiaRunes(text)
	starts := []int{0}
	for i := 1; i < len(runes); i++ {
		c, p := uiaWordClass(runes[i]), uiaWordClass(runes[i-1])
		var start bool
		switch c {
		case 0: // spaces join the word before them, but after a break they start one
			start = p == 1
		case 1: // a line break is a word of its own (CRLF one)
			start = !(runes[i-1] == '\r' && runes[i] == '\n')
		default:
			start = p != c
		}
		if start {
			starts = append(starts, offs[i])
		}
	}
	return starts
}

// uiaTextRange is a range of a node's text: start <= end, both in UTF-16
// code units.
type uiaTextRange struct{ start, end int }

// clamp fits the range into a text of n code units (the text may have
// changed since the range was made).
func (r uiaTextRange) clamp(n int) uiaTextRange {
	r.start, r.end = min(max(r.start, 0), n), min(max(r.end, 0), n)
	if r.end < r.start {
		r.end = r.start
	}
	return r
}

// uiaUnitText is a text with the unit boundaries of its last query.
type uiaUnitText struct {
	text   []uint16
	unit   int32
	starts []int
}

func (t *uiaUnitText) bounds(unit int32) []int {
	if t.starts == nil || t.unit != unit {
		t.unit, t.starts = unit, uiaUnitStarts(t.text, unit)
	}
	return t.starts
}

// isStart reports whether pos starts a unit (or is the document end).
func (t *uiaUnitText) isStart(pos int, unit int32) bool {
	b := t.bounds(unit)
	for _, s := range b {
		if s == pos {
			return true
		}
		if s > pos {
			break
		}
	}
	return false
}

// backToStart returns the start of the unit that holds pos.
func (t *uiaUnitText) backToStart(pos int, unit int32) int {
	b := t.bounds(unit)
	best := 0
	for _, s := range b {
		if s > pos {
			break
		}
		best = s
	}
	if best == len(t.text) && best > 0 && pos == best {
		// The document end starts no unit of its own.
		for i := len(b) - 1; i >= 0; i-- {
			if b[i] < best {
				return b[i]
			}
		}
	}
	return best
}

// forward returns the first unit boundary after pos (the document end at
// most): the start of the next unit, which is also the end of the one at
// pos.
func (t *uiaUnitText) forward(pos int, unit int32) int {
	for _, s := range t.bounds(unit) {
		if s > pos {
			return s
		}
	}
	return len(t.text)
}

// backward returns the last unit start before pos (0 at least).
func (t *uiaUnitText) backward(pos int, unit int32) int {
	b := t.bounds(unit)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < pos {
			return b[i]
		}
	}
	return 0
}

// movePosition moves pos by count units (forward when positive) and
// returns it with the number of units moved, as AccessKit's
// move_position does (every unit's end is the next one's start here).
func (t *uiaUnitText) movePosition(pos int, unit int32, count int32) (int, int32) {
	var moved int32
	for ; count > 0 && pos < len(t.text); count-- {
		pos = t.forward(pos, unit)
		moved++
	}
	for ; count < 0 && pos > 0; count++ {
		pos = t.backward(pos, unit)
		moved--
	}
	return pos, moved
}

// normUnit maps the units without data of their own to Document.
func normUnit(unit int32) int32 {
	switch unit {
	case uiaUnitFormat, uiaUnitPage:
		return uiaUnitDocument
	}
	return unit
}

// expand implements ITextRangeProvider::ExpandToEnclosingUnit.
func (t *uiaUnitText) expand(r uiaTextRange, unit int32) uiaTextRange {
	unit = normUnit(unit)
	n := len(t.text)
	r = r.clamp(n)
	if unit == uiaUnitDocument {
		return uiaTextRange{0, n}
	}
	if unit == uiaUnitCharacter && r.start == n {
		// Windows ATs expect a degenerate range at the document end to
		// stay (AccessKit).
		return r
	}
	start := t.backToStart(r.start, unit)
	if r.start < n && t.isStart(r.start, unit) {
		start = r.start
	}
	end := start
	if start < n {
		end = t.forward(start, unit)
	}
	return uiaTextRange{start, end}
}

// move implements ITextRangeProvider::Move.
func (t *uiaUnitText) move(r uiaTextRange, unit int32, count int32) (uiaTextRange, int32) {
	unit = normUnit(unit)
	n := len(t.text)
	r = r.clamp(n)
	degenerate := r.start == r.end
	start := r.start
	if !degenerate && !t.isStart(start, unit) {
		start = t.backToStart(start, unit)
	}
	if unit == uiaUnitDocument {
		// One document: moving forward reaches its end, backward its start.
		switch {
		case count > 0 && start < n:
			return uiaTextRange{n, n}, 1
		case count < 0 && start > 0:
			if degenerate {
				return uiaTextRange{0, 0}, -1
			}
			return uiaTextRange{0, n}, -1
		}
		return r, 0
	}
	start, moved := t.movePosition(start, unit, count)
	if moved == 0 {
		return r, 0
	}
	end := start
	if !degenerate && start < n {
		end = t.forward(start, unit)
	}
	return uiaTextRange{start, end}, moved
}

// moveEndpoint implements ITextRangeProvider::MoveEndpointByUnit: the
// endpoint moves, and the other one follows when they would cross.
func (t *uiaUnitText) moveEndpoint(r uiaTextRange, endEP bool, unit int32, count int32) (uiaTextRange, int32) {
	unit = normUnit(unit)
	n := len(t.text)
	r = r.clamp(n)
	pos := r.start
	if endEP {
		pos = r.end
	}
	var moved int32
	if unit == uiaUnitDocument {
		switch {
		case count > 0 && pos < n:
			pos, moved = n, 1
		case count < 0 && pos > 0:
			pos, moved = 0, -1
		}
	} else {
		pos, moved = t.movePosition(pos, unit, count)
	}
	return r.setEndpoint(endEP, pos), moved
}

// setEndpoint sets one endpoint, moving the other when they would cross
// (UIA's rule).
func (r uiaTextRange) setEndpoint(endEP bool, pos int) uiaTextRange {
	if endEP {
		r.end = pos
		if r.start > pos {
			r.start = pos
		}
	} else {
		r.start = pos
		if r.end < pos {
			r.end = pos
		}
	}
	return r
}

// uiaUTF16ToCodePoints converts an offset in UTF-16 code units of the
// text u to code points.
func uiaUTF16ToCodePoints(u []uint16, off int) int {
	off = min(max(off, 0), len(u))
	_, offs := uiaRunes(u)
	cp := 0
	for _, o := range offs {
		if o >= off {
			break
		}
		cp++
	}
	return cp
}

// uiaFind returns the first (or, backward, last) occurrence of needle in
// the range r of text, as a range, or false.
func uiaFind(text []uint16, r uiaTextRange, needle string, backward, ignoreCase bool) (uiaTextRange, bool) {
	r = r.clamp(len(text))
	hay, offs := uiaRunes(text[r.start:r.end])
	want := []rune(needle)
	if len(want) == 0 || len(want) > len(hay) {
		return uiaTextRange{}, false
	}
	fold := func(c rune) rune {
		if ignoreCase {
			return unicode.ToLower(c)
		}
		return c
	}
	match := func(i int) bool {
		for j, c := range want {
			if fold(hay[i+j]) != fold(c) {
				return false
			}
		}
		return true
	}
	last := len(hay) - len(want)
	for k := 0; k <= last; k++ {
		i := k
		if backward {
			i = last - k
		}
		if match(i) {
			end := r.end
			if i+len(want) < len(offs) {
				end = r.start + offs[i+len(want)]
			}
			return uiaTextRange{r.start + offs[i], end}, true
		}
	}
	return uiaTextRange{}, false
}

// uiaCaretPosition returns the CaretPosition text attribute of a
// degenerate range at pos.
func (t *uiaUnitText) caretPosition(pos int) int32 {
	switch {
	case t.isStart(pos, uiaUnitLine) && pos < len(t.text):
		return uiaCaretPositionBeginningOfLine
	case pos == 0:
		return uiaCaretPositionBeginningOfLine
	case pos == len(t.text) || uiaWordClass(rune(t.text[pos])) == 1:
		return uiaCaretPositionEndOfLine
	}
	return uiaCaretPositionUnknown
}
