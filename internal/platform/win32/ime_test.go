package win32

import (
	"encoding/binary"
	"image"
	"strings"
	"testing"
	utf16pkg "unicode/utf16"

	"github.com/kuronosu/tela/internal/platform"
)

func u16(s string) []uint16 { return utf16pkg.Encode([]rune(s)) }

// attrs builds a GCS_COMPATTR array: one byte per UTF-16 code unit of each
// clause.
func attrs(clauses ...any) []byte {
	var out []byte
	for i := 0; i < len(clauses); i += 2 {
		n := len(u16(clauses[i].(string)))
		for range n {
			out = append(out, byte(clauses[i+1].(int)))
		}
	}
	return out
}

// compMsg is a WM_IME_COMPOSITION carrying a composition string.
func compMsg(comp string, at []byte, cursor int) compUpdate {
	return compUpdate{flags: gcsCompStr | gcsCompAttr | gcsCompClause | gcsCursorPos | gcsCompReadStr, comp: u16(comp), attrs: at, cursor: cursor}
}

func TestIMEComposerJapanese(t *testing.T) {
	// The messages of Microsoft IME for "nihongo", Space, Enter (the live
	// test logs the real ones), then a second word committed by typing on.
	type step struct {
		name string
		do   func(c *imeComposer) (platform.IMEEvent, bool)
		want *platform.IMEEvent // nil: no event
	}
	ev := func(commit, preedit string, cs, ce int) *platform.IMEEvent {
		return &platform.IMEEvent{Commit: commit, Preedit: preedit, CursorStart: cs, CursorEnd: ce}
	}
	update := func(u compUpdate) func(c *imeComposer) (platform.IMEEvent, bool) {
		return func(c *imeComposer) (platform.IMEEvent, bool) { return c.update(u) }
	}
	const in, tc, cv, tn = attrInput, attrTargetConverted, attrConverted, attrTargetNotConverted
	steps := []step{
		{"start", func(c *imeComposer) (platform.IMEEvent, bool) { c.start(); return platform.IMEEvent{}, false }, nil},
		{"n", update(compMsg("ｎ", attrs("ｎ", in), 1)), ev("", "ｎ", 3, 3)},
		{"ni", update(compMsg("に", attrs("に", in), 1)), ev("", "に", 3, 3)},
		{"nihongo", update(compMsg("にほんご", attrs("にほんご", in), 4)), ev("", "にほんご", 12, 12)},
		{"same again", update(compMsg("にほんご", attrs("にほんご", in), 4)), nil},
		{"cursor moved", update(compUpdate{flags: gcsCursorPos, comp: u16("にほんご"), attrs: attrs("にほんご", in), cursor: 2}), ev("", "にほんご", 6, 6)},
		{"reading only", update(compUpdate{flags: gcsCompReadStr}), nil},
		{"space: one clause", update(compMsg("日本語", attrs("日本語", tc), 3)), ev("", "日本語", 0, 9)},
		{"two clauses, first", update(compMsg("日本語を", attrs("日本語", tc, "を", cv), 3)), ev("", "日本語を", 0, 9)},
		{"two clauses, second", update(compMsg("日本語を", attrs("日本語", cv, "を", tc), 4)), ev("", "日本語を", 9, 12)},
		{"selected unconverted", update(compMsg("にほんご", attrs("に", in, "ほん", tn, "ご", in), 3)), ev("", "にほんご", 3, 9)},
		{"enter", update(compUpdate{flags: gcsResultStr | 0x1000 /* GCS_RESULTCLAUSE */, result: u16("日本語を")}), ev("日本語を", "", 0, 0)},
		{"end", func(c *imeComposer) (platform.IMEEvent, bool) { return c.end() }, nil},
		{"start 2", func(c *imeComposer) (platform.IMEEvent, bool) { c.start(); return platform.IMEEvent{}, false }, nil},
		{"word", update(compMsg("です", attrs("です", in), 2)), ev("", "です", 6, 6)},
		{"result and next", update(compUpdate{flags: gcsResultStr | gcsCompStr | gcsCompAttr | gcsCursorPos, result: u16("です"), comp: u16("ね"), attrs: attrs("ね", in), cursor: 1}), ev("です", "ね", 3, 3)},
		{"escape", update(compUpdate{flags: 0}), ev("", "", 0, 0)},
		{"end after cancel", func(c *imeComposer) (platform.IMEEvent, bool) { return c.end() }, nil},
	}
	var c imeComposer
	for _, s := range steps {
		got, ok := s.do(&c)
		switch {
		case s.want == nil && ok:
			t.Errorf("%s: got %+v, want no event", s.name, got)
		case s.want != nil && !ok:
			t.Errorf("%s: no event, want %+v", s.name, *s.want)
		case s.want != nil && got != *s.want:
			t.Errorf("%s: got %+v, want %+v", s.name, got, *s.want)
		}
		if ok {
			if p := platform.CheckIMEEvent(got); p != "" {
				t.Errorf("%s: invalid event %+v: %s", s.name, got, p)
			}
		}
	}
	if c.open() {
		t.Errorf("composition still open: %+v", c)
	}
}

func TestIMEComposerEnd(t *testing.T) {
	// An IME that ends with a preedit still shown cancelled it.
	var c imeComposer
	c.start()
	c.update(compMsg("あ", attrs("あ", attrInput), 1))
	if got, ok := c.end(); !ok || got != (platform.IMEEvent{}) {
		t.Errorf("end with a preedit = %+v, %v; want an empty preedit", got, ok)
	}
	if c.open() {
		t.Error("open after end")
	}
	// The Korean IME: the composition ends, then the result arrives.
	c.start()
	c.update(compUpdate{flags: gcsCompStr, comp: u16("한"), cursor: -1})
	if c.preedit != "한" || c.cursor != [2]int{-1, -1} {
		t.Errorf("Korean preedit %q cursor %v; want 한 with a hidden cursor", c.preedit, c.cursor)
	}
	c.end()
	got, ok := c.update(compUpdate{flags: gcsResultStr, result: u16("한")})
	if !ok || got != (platform.IMEEvent{Commit: "한"}) {
		t.Errorf("late result = %+v, %v; want a commit", got, ok)
	}
	// A result that commits nothing new and no preedit: nothing.
	if got, ok := c.update(compUpdate{flags: gcsResultStr}); ok {
		t.Errorf("empty result = %+v, want no event", got)
	}
	// clear forgets a composition without an event (the session ended).
	c.start()
	c.update(compMsg("か", attrs("か", attrInput), 1))
	c.clear()
	if c.open() {
		t.Error("open after clear")
	}
	if got, ok := c.update(compUpdate{flags: 0}); ok {
		t.Errorf("cancel after clear = %+v, want no event", got)
	}
}

func TestIMEText(t *testing.T) {
	for _, tc := range []struct {
		name   string
		comp   []uint16
		attrs  []byte
		cursor int
		want   platform.IMEEvent
	}{
		{"astral, cursor after it", u16("𠮷a"), attrs("𠮷a", attrInput), 2,
			platform.IMEEvent{Preedit: "𠮷a", CursorStart: 4, CursorEnd: 4}},
		{"astral target clause", u16("𠮷a"), attrs("𠮷", attrTargetConverted, "a", attrConverted), 0,
			platform.IMEEvent{Preedit: "𠮷a", CursorStart: 0, CursorEnd: 4}},
		{"cursor inside a surrogate pair", u16("𠮷a"), nil, 1,
			platform.IMEEvent{Preedit: "𠮷a", CursorStart: 0, CursorEnd: 0}},
		{"cursor past the end", u16("ab"), nil, 9,
			platform.IMEEvent{Preedit: "ab", CursorStart: 2, CursorEnd: 2}},
		{"no cursor", u16("ab"), nil, -1,
			platform.IMEEvent{Preedit: "ab", CursorStart: -1, CursorEnd: -1}},
		{"attributes of another length", u16("ab"), []byte{attrTargetConverted}, 1,
			platform.IMEEvent{Preedit: "ab", CursorStart: 1, CursorEnd: 1}},
		{"target to the end", u16("abc"), attrs("a", attrConverted, "bc", attrTargetConverted), 0,
			platform.IMEEvent{Preedit: "abc", CursorStart: 1, CursorEnd: 3}},
		{"lone surrogate", []uint16{'a', 0xD800, 'b'}, attrs("a", attrInput, "x", attrInput, "b", attrInput), 2,
			platform.IMEEvent{Preedit: "a�b", CursorStart: 4, CursorEnd: 4}},
		{"NUL", []uint16{'a', 0, 'b'}, nil, 3,
			platform.IMEEvent{Preedit: "ab", CursorStart: 2, CursorEnd: 2}},
	} {
		var c imeComposer
		got, ok := c.update(compUpdate{flags: gcsCompStr | gcsCompAttr | gcsCursorPos, comp: tc.comp, attrs: tc.attrs, cursor: tc.cursor})
		if !ok || got != tc.want {
			t.Errorf("%s: %+v, %v; want %+v", tc.name, got, ok, tc.want)
		}
		if p := platform.CheckIMEEvent(got); p != "" {
			t.Errorf("%s: invalid event: %s", tc.name, p)
		}
	}
	var c imeComposer
	if got, _ := c.update(compUpdate{flags: gcsResultStr, result: []uint16{'x', 0, 0xDC00}}); got.Commit != "x�" {
		t.Errorf("result %q, want sanitized", got.Commit)
	}
}

func TestIMECaretRect(t *testing.T) {
	for _, tc := range []struct {
		area  platform.Rect
		scale float64
		want  image.Rectangle
	}{
		{platform.Rect{X: 10, Y: 20, H: 18}, 1, image.Rect(10, 20, 11, 38)},
		{platform.Rect{X: 10.2, Y: 20, W: 0, H: 18}, 1.5, image.Rect(15, 30, 16, 57)},
		{platform.Rect{X: 10.5, Y: 20.25, W: 30, H: 17.5}, 2, image.Rect(21, 40, 81, 76)},
		{platform.Rect{X: -5, Y: -3, W: 2, H: 0}, 1.25, image.Rect(-7, -4, -3, -3)},
	} {
		if got := imeCaretRect(tc.area, tc.scale); got != tc.want {
			t.Errorf("imeCaretRect(%v, %g) = %v, want %v", tc.area, tc.scale, got, tc.want)
		}
	}
}

func TestPlaceIME(t *testing.T) {
	caret := image.Rect(30, 40, 31, 64)
	for _, tc := range []struct {
		lang  uint16
		want  imePlacement
		caret bool
	}{
		{langJapanese, imePlacement{comp: image.Pt(30, 40), caret: image.Pt(30, 64), cands: []candForm{
			{cfsCandidatePos, image.Pt(30, 40), image.Rectangle{}},
			{cfsExclude, image.Pt(30, 40), image.Rect(30, 40, 31, 64)},
		}}, true},
		{langKorean, imePlacement{comp: image.Pt(30, 40), caret: image.Pt(30, 40), cands: []candForm{
			{cfsCandidatePos, image.Pt(30, 40), image.Rectangle{}},
			{cfsExclude, image.Pt(30, 41), image.Rect(30, 41, 31, 65)},
		}}, false},
		{langChinese, imePlacement{comp: image.Pt(30, 40), caret: image.Pt(30, 40), cands: []candForm{
			{cfsCandidatePos, image.Pt(30, 40), image.Rectangle{}},
		}}, true},
		{0x09, imePlacement{comp: image.Pt(30, 40), caret: image.Pt(30, 40), cands: []candForm{
			{cfsCandidatePos, image.Pt(30, 40), image.Rectangle{}},
			{cfsExclude, image.Pt(30, 40), image.Rect(30, 40, 31, 64)},
		}}, false},
	} {
		got := placeIME(caret, tc.lang)
		if got.comp != tc.want.comp || got.caret != tc.want.caret || len(got.cands) != len(tc.want.cands) {
			t.Errorf("lang %#x: %+v, want %+v", tc.lang, got, tc.want)
			continue
		}
		for i := range got.cands {
			if got.cands[i] != tc.want.cands[i] {
				t.Errorf("lang %#x: candidate form %d = %+v, want %+v", tc.lang, i, got.cands[i], tc.want.cands[i])
			}
		}
		if needsSystemCaret(tc.lang) != tc.caret {
			t.Errorf("lang %#x: needsSystemCaret %v", tc.lang, !tc.caret)
		}
	}
}

// parseReconvert reads a RECONVERTSTRING back: the string and the
// composition and target ranges in code units.
func parseReconvert(t *testing.T, b []byte) (text string, comp, target [2]int) {
	t.Helper()
	le := binary.LittleEndian
	if int(le.Uint32(b)) != len(b) || le.Uint32(b[4:]) != 0 || le.Uint32(b[12:]) != reconvertHeader {
		t.Fatalf("bad header % x", b[:reconvertHeader])
	}
	n := int(le.Uint32(b[8:]))
	if len(b) != reconvertHeader+2*n {
		t.Fatalf("size %d for %d code units", len(b), n)
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = le.Uint16(b[reconvertHeader+2*i:])
	}
	comp = [2]int{int(le.Uint32(b[20:])) / 2, int(le.Uint32(b[16:]))}
	target = [2]int{int(le.Uint32(b[28:])) / 2, int(le.Uint32(b[24:]))}
	return string(utf16pkg.Decode(u)), comp, target
}

func TestDocumentFeed(t *testing.T) {
	if documentFeed(nil, "") != nil {
		t.Error("feed without surrounding text")
	}
	st := &platform.SurroundingText{Text: "私は𠮷を", Cursor: len("私は𠮷"), Anchor: len("私は𠮷")}
	text, comp, target := parseReconvert(t, documentFeed(st, "にほんご"))
	if text != "私は𠮷にほんごを" || comp != [2]int{4, 4} || target != comp {
		t.Errorf("feed during a composition: %q comp %v target %v", text, comp, target)
	}
	text, comp, _ = parseReconvert(t, documentFeed(st, ""))
	if text != "私は𠮷を" || comp != [2]int{4, 0} {
		t.Errorf("feed at the caret: %q comp %v", text, comp)
	}
	sel := &platform.SurroundingText{Text: "abcdef", Cursor: 4, Anchor: 2}
	text, comp, _ = parseReconvert(t, documentFeed(sel, ""))
	if text != "abcdef" || comp != [2]int{2, 0} {
		t.Errorf("feed with a selection: %q comp %v", text, comp)
	}
	long := strings.Repeat("あ", 1000) + strings.Repeat("い", 1000)
	text, comp, _ = parseReconvert(t, documentFeed(&platform.SurroundingText{Text: long, Cursor: len(long) / 2, Anchor: len(long) / 2}, "x"))
	if text != strings.Repeat("あ", feedContext)+"x"+strings.Repeat("い", feedContext) || comp != [2]int{feedContext, 1} {
		t.Errorf("long feed: %d characters, comp %v", len([]rune(text)), comp)
	}
}

func TestReconvertSelection(t *testing.T) {
	st := &platform.SurroundingText{Text: "私は日本語を", Cursor: len("私は日本語"), Anchor: len("私は")}
	text, comp, target := parseReconvert(t, reconvertSelection(st, false))
	if text != "日本語" || comp != [2]int{0, 3} || target != comp {
		t.Errorf("reconvert %q comp %v target %v", text, comp, target)
	}
	if reconvertSelection(st, true) != nil {
		t.Error("reconversion during a composition")
	}
	if reconvertSelection(&platform.SurroundingText{Text: "abc", Cursor: 1, Anchor: 1}, false) != nil {
		t.Error("reconversion without a selection")
	}
	if reconvertSelection(nil, false) != nil {
		t.Error("reconversion without surrounding text")
	}
}
