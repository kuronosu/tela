package cocoa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func imeEvString(e platform.IMEEvent) string {
	var parts []string
	if e.DeleteBefore != 0 || e.DeleteAfter != 0 {
		parts = append(parts, fmt.Sprintf("delete %d,%d", e.DeleteBefore, e.DeleteAfter))
	}
	if e.Commit != "" {
		parts = append(parts, fmt.Sprintf("commit %q", e.Commit))
	}
	parts = append(parts, fmt.Sprintf("preedit %q %d,%d", e.Preedit, e.CursorStart, e.CursorEnd))
	return strings.Join(parts, " ")
}

func checkIME(t *testing.T, step string, got []platform.IMEEvent, want ...string) {
	t.Helper()
	var g []string
	for _, e := range got {
		if p := platform.CheckIMEEvent(e); p != "" {
			t.Errorf("%s: %s breaks the IMEEvent rules: %s", step, imeEvString(e), p)
		}
		g = append(g, imeEvString(e))
	}
	if strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s:\n got  %q\n want %q", step, g, want)
	}
}

func checkRange(t *testing.T, step string, got range16, loc, n int) {
	t.Helper()
	if got != (range16{loc, n}) {
		t.Errorf("%s: range %v, want {%d %d}", step, got, loc, n)
	}
}

func checkDoc(t *testing.T, step string, m *imeInput, text string, cursor, anchor int) {
	t.Helper()
	if m.doc == nil || m.doc.Text != text || m.doc.Cursor != cursor || m.doc.Anchor != anchor {
		t.Errorf("%s: document %+v, want %q cursor %d anchor %d", step, m.doc, text, cursor, anchor)
	}
}

// TestIMEComposition drives a composition as an input method does: marked
// text with a caret, a highlighted clause, the document the IME reads,
// a commit, unmarkText and a cancel.
func TestIMEComposition(t *testing.T) {
	var m imeInput
	m.setDoc(&platform.SurroundingText{Text: "hello world", Cursor: 5, Anchor: 5})
	checkRange(t, "markedRange before", m.markedRange(), -1, 0)
	checkRange(t, "selectedRange before", m.selectedRange(), 5, 0)

	checkIME(t, "setMarked にほ", m.setMarked("にほ", range16{2, 0}, noRange), `preedit "にほ" 6,6`)
	checkRange(t, "markedRange", m.markedRange(), 5, 2)
	checkRange(t, "selectedRange", m.selectedRange(), 7, 0)
	checkIME(t, "same marked text", m.setMarked("にほ", range16{2, 0}, noRange))

	checkIME(t, "setMarked 日本 selected", m.setMarked("日本", range16{0, 2}, range16{5, 2}), `preedit "日本" 0,6`)
	checkRange(t, "selectedRange clause", m.selectedRange(), 5, 2)
	for _, c := range []struct {
		r    range16
		want string
		got  range16
	}{
		{range16{0, 5}, "hello", range16{0, 5}},
		{range16{5, 2}, "日本", range16{5, 2}},
		{range16{3, 100}, "lo日本 world", range16{3, 10}},
		{range16{50, 2}, "", range16{13, 0}},
	} {
		s, got, ok := m.substring(c.r)
		if !ok || s != c.want || got != c.got {
			t.Errorf("substring %v = %q %v %v, want %q %v", c.r, s, got, ok, c.want, c.got)
		}
	}

	checkIME(t, "insert 日本", m.insert("日本", noRange), `commit "日本" preedit "" 0,0`)
	checkDoc(t, "after the commit", &m, "hello日本 world", 11, 11)
	checkRange(t, "selectedRange after the commit", m.selectedRange(), 7, 0)
	checkRange(t, "markedRange after the commit", m.markedRange(), -1, 0)
	checkIME(t, "unmark without marked text", m.unmark())

	checkIME(t, "setMarked a", m.setMarked("a", range16{1, 0}, noRange), `preedit "a" 1,1`)
	checkIME(t, "unmark", m.unmark(), `commit "a" preedit "" 0,0`)
	checkDoc(t, "after unmark", &m, "hello日本a world", 12, 12)

	// A replacement range before the caret (a reconversion) deletes the
	// committed text it covers.
	checkIME(t, "insert X over lo日本a", m.insert("X", range16{3, 5}), `delete 9,0 commit "X" preedit "" 0,0`)
	checkDoc(t, "after the replacement", &m, "helX world", 4, 4)
	checkIME(t, "setMarked か over X", m.setMarked("か", range16{1, 0}, range16{3, 1}), `delete 1,0 preedit "か" 3,3`)
	checkDoc(t, "composing", &m, "hel world", 3, 3)
	checkRange(t, "markedRange か", m.markedRange(), 3, 1)
	checkIME(t, "cancel", m.setMarked("", range16{0, 0}, noRange), `preedit "" 0,0`)
	checkDoc(t, "after the cancel", &m, "hel world", 3, 3)
	checkIME(t, "cancel again", m.setMarked("", range16{0, 0}, noRange))

	// A range after the caret, and one that does not cover it (ignored).
	checkIME(t, "insert Y over ' w'", m.insert("Y", range16{3, 2}), `delete 0,2 commit "Y" preedit "" 0,0`)
	checkDoc(t, "after", &m, "helYorld", 4, 4)
	checkIME(t, "range away from the caret", m.insert("Z", range16{6, 1}), `commit "Z" preedit "" 0,0`)
	checkDoc(t, "after", &m, "helYZorld", 5, 5)

	// The session ends (or the app resets the IME): the composition is
	// forgotten without an event, and the next preedit starts a new one.
	checkIME(t, "setMarked q", m.setMarked("q", range16{1, 0}, noRange), `preedit "q" 1,1`)
	m.clear()
	checkRange(t, "markedRange after clear", m.markedRange(), -1, 0)
	checkIME(t, "setMarked q again", m.setMarked("q", range16{1, 0}, noRange), `preedit "q" 1,1`)
}

// TestIMESelection: a composition replaces the selection; the selection's
// direction survives deletions.
func TestIMESelection(t *testing.T) {
	var m imeInput
	m.setDoc(&platform.SurroundingText{Text: "abcdef", Cursor: 4, Anchor: 1})
	checkRange(t, "selectedRange", m.selectedRange(), 1, 3)
	if _, _, ok := m.substring(range16{1, 3}); !ok {
		t.Errorf("substring of the selection failed")
	}
	checkIME(t, "insert covering the selection", m.insert("", range16{0, 5}))
	checkIME(t, "setMarked x", m.setMarked("x", range16{1, 0}, noRange), `preedit "x" 1,1`)
	checkDoc(t, "composing", &m, "aef", 1, 1)
	checkRange(t, "markedRange", m.markedRange(), 1, 1)
	if s, _, _ := m.substring(range16{0, 3}); s != "axe" {
		t.Errorf("document %q, want axe", s)
	}

	var b imeInput
	b.setDoc(&platform.SurroundingText{Text: "abcdef", Cursor: 2, Anchor: 4})
	checkIME(t, "deletions around a selection", b.insert("Q", range16{1, 4}), `delete 1,1 commit "Q" preedit "" 0,0`)
	checkDoc(t, "after", &b, "aQf", 2, 2)
}

// TestIMENoDocument: without surrounding text the document is the marked
// text, and replacement ranges delete nothing.
func TestIMENoDocument(t *testing.T) {
	var m imeInput
	checkRange(t, "selectedRange", m.selectedRange(), -1, 0)
	if _, _, ok := m.substring(range16{0, 1}); ok {
		t.Errorf("substring without a document and marked text succeeded")
	}
	checkIME(t, "setMarked", m.setMarked("ab", range16{1, 0}, range16{0, 4}), `preedit "ab" 1,1`)
	checkRange(t, "markedRange", m.markedRange(), 0, 2)
	checkRange(t, "selectedRange", m.selectedRange(), 1, 0)
	checkIME(t, "selection past the end is clamped", m.setMarked("ab", range16{5, 3}, noRange), `preedit "ab" 2,2`)
	checkIME(t, "invalid selection: caret at the end", m.setMarked("𝄞x", noRange, noRange), `preedit "𝄞x" 5,5`)
	checkIME(t, "selection inside a surrogate pair", m.setMarked("𝄞x", range16{1, 1}, noRange), `preedit "𝄞x" 0,4`)
	checkIME(t, "commit", m.insert("d\x00e", range16{0, 9}), `commit "de" preedit "" 0,0`)
}

// TestIMERejectedText: the app rejects what a key and a commit typed (a
// field at its MaxLength reports the same state again, which SetTextInput
// ignores). At the end of the pass the document is the app's text again,
// and the next composition and a reconversion are placed in it.
func TestIMERejectedText(t *testing.T) {
	var m imeInput
	m.setDoc(&platform.SurroundingText{Text: "abc", Cursor: 3, Anchor: 3})

	// A key typing its own text: the KeyEvent carries it.
	m.beginKey("d")
	checkIME(t, "insert d", m.insert("d", noRange))
	if deliver, out := m.endKey(); !deliver || len(out) != 0 {
		t.Errorf("key d: delivered %v, events %d; want the KeyEvent alone", deliver, len(out))
	}
	checkDoc(t, "before the app saw the key", &m, "abcd", 4, 4)
	m.resync() // the pass ends; the app reported nothing new
	checkDoc(t, "after the rejected key", &m, "abc", 3, 3)

	// A commit outside a key, rejected too.
	checkIME(t, "insert e", m.insert("e", noRange), `commit "e" preedit "" 0,0`)
	m.resync()
	checkDoc(t, "after the rejected commit", &m, "abc", 3, 3)
	checkRange(t, "selectedRange", m.selectedRange(), 3, 0)

	checkIME(t, "setMarked か", m.setMarked("か", range16{1, 0}, noRange), `preedit "か" 3,3`)
	checkRange(t, "markedRange", m.markedRange(), 3, 1)
	checkRange(t, "selectedRange in the preedit", m.selectedRange(), 4, 0)
	// Reconversion of "c" and the preedit deletes the "c" the field has.
	checkIME(t, "insert X over c and the preedit", m.insert("X", range16{2, 2}), `delete 1,0 commit "X" preedit "" 0,0`)

	// The app took this one and reported it: the document follows it.
	m.setDoc(&platform.SurroundingText{Text: "abX", Cursor: 3, Anchor: 3})
	m.resync()
	checkDoc(t, "after an accepted commit", &m, "abX", 3, 3)

	// No resync in the middle of a key: its calls build on each other.
	m.beginKey("")
	m.insert("y", noRange)
	m.resync()
	checkDoc(t, "inside a key", &m, "abXy", 4, 4)
	m.endKey()

	// Without surrounding text there is nothing to go back to.
	var n imeInput
	n.setDoc(nil)
	n.resync()
	if n.doc != nil {
		t.Errorf("resync without surrounding text made a document %+v", n.doc)
	}
}

// TestIMEMarkedNUL: the selection setMarkedText: gives is in UTF-16 units
// of the text with its NULs; dropping them must not move it.
func TestIMEMarkedNUL(t *testing.T) {
	var m imeInput
	checkIME(t, "caret after a NUL", m.setMarked("a\x00bc", range16{3, 0}, noRange), `preedit "abc" 2,2`)
	checkRange(t, "selectedRange", m.selectedRange(), 2, 0)
	checkIME(t, "clause after NULs", m.setMarked("\x00日\x00本x", range16{3, 2}, noRange), `preedit "日本x" 3,7`)
	checkRange(t, "selectedRange of the clause", m.selectedRange(), 1, 2)
	checkIME(t, "caret on a NUL", m.setMarked("ab\x00", range16{2, 1}, noRange), `preedit "ab" 2,2`)
	checkIME(t, "default caret at the end", m.setMarked("x\x00y\x00", noRange, noRange), `preedit "xy" 2,2`)
	checkIME(t, "NULs only cancel", m.setMarked("\x00", range16{1, 0}, noRange), `preedit "" 0,0`)
}

// TestIMEKeyDecisions: what a key event handed to interpretKeyEvents: becomes.
func TestIMEKeyDecisions(t *testing.T) {
	type call func(m *imeInput) []platform.IMEEvent
	insert := func(s string) call { return func(m *imeInput) []platform.IMEEvent { return m.insert(s, noRange) } }
	mark := func(s string) call {
		return func(m *imeInput) []platform.IMEEvent { return m.setMarked(s, range16{len([]rune(s)), 0}, noRange) }
	}
	command := func(m *imeInput) []platform.IMEEvent { m.command(); return nil }
	unmark := func(m *imeInput) []platform.IMEEvent { return m.unmark() }
	for _, c := range []struct {
		name    string
		marked  string // composition before the key
		text    string // the KeyEvent's text
		calls   []call
		deliver bool
		want    []string
	}{
		{name: "plain letter", text: "a", calls: []call{insert("a")}, deliver: true},
		{name: "nothing called", text: "a", deliver: true},
		{name: "backspace", calls: []call{command}, deliver: true},
		{name: "control text is a command", calls: []call{insert("\r")}, deliver: true},
		{name: "dead key", calls: []call{mark("´")}, want: []string{`preedit "´" 2,2`}},
		{name: "dead key composed", marked: "´", text: "é", calls: []call{insert("é")}, want: []string{`commit "é" preedit "" 0,0`}},
		{name: "consumed silently during a composition", marked: "か", text: "k"},
		{name: "other text", text: "a", calls: []call{insert("ａ")}, want: []string{`commit "ａ" preedit "" 0,0`}},
		{name: "insert then mark", text: "a", calls: []call{insert("a"), mark("b")},
			want: []string{`commit "a" preedit "" 0,0`, `preedit "b" 1,1`}},
		{name: "Korean space", marked: "하", calls: []call{insert("하"), insert(" ")},
			want: []string{`commit "하" preedit "" 0,0`, `commit " " preedit "" 0,0`}},
		{name: "Korean return", marked: "하", calls: []call{insert("하"), command}, deliver: true,
			want: []string{`commit "하" preedit "" 0,0`}},
		{name: "unmark in a key", marked: "x", text: "y", calls: []call{unmark, insert("y")},
			want: []string{`commit "x" preedit "" 0,0`, `commit "y" preedit "" 0,0`}},
		{name: "insert and command", text: "z", calls: []call{insert("q"), command}, deliver: true,
			want: []string{`commit "q" preedit "" 0,0`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var m imeInput
			m.setDoc(&platform.SurroundingText{})
			if c.marked != "" {
				m.setMarked(c.marked, range16{1, 0}, noRange)
			}
			m.beginKey(c.text)
			var got []platform.IMEEvent
			for _, f := range c.calls {
				got = append(got, f(&m)...)
			}
			deliver, held := m.endKey()
			got = append(got, held...)
			if deliver != c.deliver {
				t.Errorf("KeyEvent delivered %v, want %v", deliver, c.deliver)
			}
			checkIME(t, c.name, got, c.want...)
			if m.key != nil {
				t.Errorf("the key is still being interpreted after endKey")
			}
		})
	}
}

func TestIMECursorRect(t *testing.T) {
	if _, ok := imeCursorRect(nil); ok {
		t.Errorf("a nil state has a cursor rectangle")
	}
	if _, ok := imeCursorRect(&platform.TextInputState{}); ok {
		t.Errorf("the zero cursor area is not unknown")
	}
	r, ok := imeCursorRect(&platform.TextInputState{CursorArea: platform.Rect{X: 3, Y: 4}})
	if !ok || r != (platform.Rect{X: 3, Y: 4, H: 1}) {
		t.Errorf("caret rectangle %v %v, want one point high", r, ok)
	}
}
