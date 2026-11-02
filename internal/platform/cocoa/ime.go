package cocoa

import (
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/utf16x"
)

// The IME through NSTextInputClient: the OS-independent half, tested on
// every OS. ime_darwin.go receives the protocol calls of TelaView, converts
// their NSStrings and NSRanges, and hands them to imeInput, which keeps the
// composition and a copy of the field's text and returns the
// platform.IMEEvents to queue. See "IME" in doc.go.
//
// The text the input method sees is the field's document as AppKit
// models it: the committed text (TextInputState.Surrounding) with the
// marked text (the preedit) inserted at the caret. Ranges are UTF-16 code
// units of that document. Without surrounding text the document is the
// marked text alone, and selectedRange is NSNotFound outside a composition
// (as in winit).

// range16 is an NSRange in UTF-16 code units of the document; a negative
// loc is NSNotFound.
type range16 struct{ loc, n int }

// noRange is {NSNotFound, 0}.
var noRange = range16{loc: -1}

func (r range16) valid() bool { return r.loc >= 0 }
func (r range16) end() int    { return r.loc + r.n }

// imeKey is what the protocol calls made of one key event handed to
// interpretKeyEvents:.
type imeKey struct {
	text    string // the KeyEvent's Text
	pending string // text inserted without a composition, held back
	ime     bool   // the IME composed, committed marked text or replaced text
	command bool   // doCommandBySelector: the IME passed the key on
}

// imeInput is the composition of one window as the app has been told of
// it, and the document the input method reads.
type imeInput struct {
	// doc is the field's committed text and selection: the app's latest
	// SurroundingText with the IMEEvents queued since applied to it, so
	// that several calls in one key see each other's edits before the app
	// sends its next state. nil: unknown.
	doc *platform.SurroundingText
	// marked and cursor are the preedit and its cursor of the last
	// IMEEvent; sel is the selection inside marked as setMarkedText gave
	// it (UTF-16).
	marked string
	cursor [2]int
	sel    range16
	// key is the key event being interpreted (nil outside
	// interpretKeyEvents:).
	key *imeKey
}

// setDoc takes the app's surrounding text (a copy).
func (m *imeInput) setDoc(st *platform.SurroundingText) {
	m.doc = nil
	if st != nil {
		c := *st
		m.doc = &c
	}
}

// clear forgets the composition without an event: the session ended or
// the app reset it.
func (m *imeInput) clear() {
	m.marked, m.cursor, m.sel = "", [2]int{}, range16{}
}

// composing reports whether the app has a preedit from the IME.
func (m *imeInput) composing() bool { return m.marked != "" }

// selBytes returns the committed selection, ordered, in bytes (0, 0
// without a document).
func (m *imeInput) selBytes() (int, int) {
	if m.doc == nil {
		return 0, 0
	}
	return min(m.doc.Cursor, m.doc.Anchor), max(m.doc.Cursor, m.doc.Anchor)
}

// markedRange answers markedRange: the preedit in the document.
func (m *imeInput) markedRange() range16 {
	if !m.composing() {
		return noRange
	}
	base := 0
	if m.doc != nil {
		s, _ := m.selBytes()
		base = utf16x.FromByte(m.doc.Text, s)
	}
	return range16{base, utf16x.Len(m.marked)}
}

// selectedRange answers selectedRange: the selection inside the preedit
// during a composition, else the field's selection.
func (m *imeInput) selectedRange() range16 {
	if m.composing() {
		return range16{m.markedRange().loc + m.sel.loc, m.sel.n}
	}
	if m.doc == nil {
		return noRange
	}
	s, e := m.selBytes()
	s16 := utf16x.FromByte(m.doc.Text, s)
	return range16{s16, utf16x.FromByte(m.doc.Text, e) - s16}
}

// document returns the text the input method reads, and false when it is
// unknown.
func (m *imeInput) document() (string, bool) {
	if m.doc == nil {
		return m.marked, m.composing()
	}
	if !m.composing() {
		return m.doc.Text, true
	}
	s, e := m.selBytes()
	return m.doc.Text[:s] + m.marked + m.doc.Text[e:], true
}

// substring answers attributedSubstringForProposedRange:actualRange:: the
// part of the document in r, clamped to it, and the range it covers.
func (m *imeInput) substring(r range16) (string, range16, bool) {
	d, ok := m.document()
	if !ok || !r.valid() {
		return "", noRange, false
	}
	n := utf16x.Len(d)
	lo := min(r.loc, n)
	hi := n
	if r.n < n-lo {
		hi = lo + r.n
	}
	b0, b1 := utf16x.ToByte(d, lo), utf16x.ToByte(d, hi)
	lo16 := utf16x.FromByte(d, b0)
	return d[b0:b1], range16{lo16, utf16x.FromByte(d, b1) - lo16}, true
}

// deletion turns a replacement range into the committed text to delete
// around the protected range (the preedit during a composition, else the
// selection), in bytes. A replacement range is only expressible as
// IMEEvent deletions when it covers the protected range (the usual cases:
// the marked range itself, the selection, a reconversion that takes
// characters before the caret); other ranges, and every range without a
// document, delete nothing and the text replaces the protected range, as
// if the range were NSNotFound.
func (m *imeInput) deletion(r range16) (before, after int) {
	if !r.valid() || m.doc == nil {
		return 0, 0
	}
	p := m.selectedRange()
	if m.composing() {
		p = m.markedRange()
	}
	if r.loc > p.loc || r.end() < p.end() {
		return 0, 0
	}
	text := m.doc.Text
	s, e := m.selBytes()
	// Document offsets after the protected range are committed offsets
	// shifted by the preedit (whose committed range is empty).
	r1 := r.end() - (p.end() - utf16x.FromByte(text, e))
	before = s - utf16x.ToByte(text, r.loc)
	after = utf16x.ToByte(text, r1) - e
	return max(before, 0), max(after, 0)
}

// setMarked handles setMarkedText:selectedRange:replacementRange:: the new
// preedit with the selection as its cursor (a highlighted range when it is
// not empty). An empty text ends the composition: a cancel, unless the
// text was committed in the same event.
func (m *imeInput) setMarked(text string, sel, repl range16) []platform.IMEEvent {
	text = platform.SanitizeIMEText(text)
	var out []platform.IMEEvent
	if k := m.key; k != nil {
		k.ime = true
		out = m.flush(out)
	}
	ev := platform.IMEEvent{Preedit: text}
	ev.DeleteBefore, ev.DeleteAfter = m.deletion(repl)
	m.sel = range16{}
	if text != "" {
		n := utf16x.Len(text)
		lo, hi := n, n
		if sel.valid() {
			lo = min(sel.loc, n)
			hi = lo + min(sel.n, n-lo)
		}
		ev.CursorStart, ev.CursorEnd = platform.SanitizePreeditCursor(text, utf16x.ToByte(text, lo), utf16x.ToByte(text, hi))
		s16 := utf16x.FromByte(text, ev.CursorStart)
		m.sel = range16{s16, utf16x.FromByte(text, ev.CursorEnd) - s16}
	}
	return m.emit(out, ev)
}

// insert handles insertText:replacementRange:: a commit, which replaces
// and ends a composition. Inside a key event, text inserted without a
// composition is held back: if it is the KeyEvent's own text, the
// KeyEvent carries it (endKey). Text of control characters only is a key
// the IME did not handle, never a commit.
func (m *imeInput) insert(text string, repl range16) []platform.IMEEvent {
	text = platform.SanitizeIMEText(text)
	if !hasPrintable(text) {
		if m.key != nil && text != "" {
			m.key.command = true
		}
		return nil
	}
	before, after := m.deletion(repl)
	k := m.key
	if k != nil && !k.ime && !m.composing() && before == 0 && after == 0 {
		k.pending += text
		m.applyDoc(platform.IMEEvent{Commit: text})
		return nil
	}
	var out []platform.IMEEvent
	if k != nil {
		k.ime = true
		out = m.flush(out)
	}
	m.sel = range16{}
	return m.emit(out, platform.IMEEvent{DeleteBefore: before, DeleteAfter: after, Commit: text})
}

// unmark handles unmarkText: the marked text stays as committed text
// (NSTextView's behavior).
func (m *imeInput) unmark() []platform.IMEEvent {
	if !m.composing() {
		return nil
	}
	var out []platform.IMEEvent
	if k := m.key; k != nil {
		k.ime = true
		out = m.flush(out)
	}
	commit := m.marked
	m.sel = range16{}
	return m.emit(out, platform.IMEEvent{Commit: commit})
}

// command handles doCommandBySelector:: the input method passed the key
// on (an editing command of the key bindings), so the KeyEvent is
// delivered.
func (m *imeInput) command() {
	if m.key != nil {
		m.key.command = true
	}
}

// beginKey starts interpreting a key event whose KeyEvent has text.
func (m *imeInput) beginKey(text string) {
	m.key = &imeKey{text: text, ime: m.composing()}
}

// endKey ends the key event and reports whether its KeyEvent is delivered,
// with the IMEEvents to queue before it:
//
//   - the IME passed the key on (doCommandBySelector:): the KeyEvent, after
//     what the IME did first (a commit, as Korean IMEs do before a
//     Return);
//   - the IME composed, or a composition was in progress: no KeyEvent (the
//     IME consumed the key);
//   - the key inserted its own text, or nothing happened (a key the input
//     method ignores): the KeyEvent alone, which carries the text;
//   - the key inserted other text (an input source that maps keys itself):
//     that text as a commit, and no KeyEvent.
func (m *imeInput) endKey() (deliver bool, out []platform.IMEEvent) {
	k := m.key
	if k == nil {
		return true, nil
	}
	deliver = k.command || !k.ime && (k.pending == "" || k.pending == k.text)
	if deliver && k.pending == k.text {
		k.pending = "" // the KeyEvent carries it
	}
	out = m.flush(nil)
	m.key = nil
	return deliver, out
}

// flush queues the text held back during the key event as a commit (it is
// in the document already).
func (m *imeInput) flush(out []platform.IMEEvent) []platform.IMEEvent {
	if k := m.key; k != nil && k.pending != "" {
		out = append(out, platform.IMEEvent{Commit: k.pending})
		k.pending = ""
	}
	return out
}

// emit appends ev to out if it changes something, applying it to the
// document and recording its preedit.
func (m *imeInput) emit(out []platform.IMEEvent, ev platform.IMEEvent) []platform.IMEEvent {
	cursor := [2]int{ev.CursorStart, ev.CursorEnd}
	if ev.DeleteBefore == 0 && ev.DeleteAfter == 0 && ev.Commit == "" && ev.Preedit == m.marked && cursor == m.cursor {
		return out
	}
	m.applyDoc(ev)
	m.marked, m.cursor = ev.Preedit, cursor
	return append(out, ev)
}

// applyDoc applies ev to the document as the app applies it (IMEEvent's
// order: deletions, commit, preedit); a new preedit replaces the
// selection, and during a composition the caret is where the preedit is.
func (m *imeInput) applyDoc(ev platform.IMEEvent) {
	d := m.doc
	if d == nil {
		return
	}
	t := d.Text
	s, e := m.selBytes()
	backward := d.Cursor < d.Anchor
	ds := runeStart(t, max(s-ev.DeleteBefore, 0))
	de := e + ev.DeleteAfter
	if de >= len(t) {
		de = len(t)
	} else {
		de = runeStart(t, de)
	}
	t = t[:ds] + t[s:e] + t[de:]
	s, e = ds, ds+(e-s)
	switch {
	case ev.Commit != "":
		t = t[:s] + ev.Commit + t[e:]
		s += len(ev.Commit)
		e = s
	case ev.Preedit != "" && !m.composing():
		t = t[:s] + t[e:]
		e = s
	}
	d.Text = t
	if backward {
		d.Cursor, d.Anchor = s, e
	} else {
		d.Cursor, d.Anchor = e, s
	}
}

// runeStart moves the byte offset o of s back to the start of its
// character.
func runeStart(s string, o int) int {
	for o > 0 && o < len(s) && !utf8.RuneStart(s[o]) {
		o--
	}
	return o
}

// hasPrintable reports whether s has a character that is not a control
// character.
func hasPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// imeCursorRect returns the rectangle firstRectForCharacterRange: reports,
// in view points (top-left origin): the field's cursor area, at least one
// point high so that a caret is a line, or nil when it is unknown.
func imeCursorRect(st *platform.TextInputState) (platform.Rect, bool) {
	if st == nil || st.CursorArea == (platform.Rect{}) {
		return platform.Rect{}, false
	}
	r := st.CursorArea
	r.H = max(r.H, 1)
	return r, true
}
