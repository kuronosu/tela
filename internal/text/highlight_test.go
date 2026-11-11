package text

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestHighlightRects checks the highlight of one paragraph's part of a
// selection across paragraphs (ADR-271): inline boxes are left out, and a
// selection that goes on past the end of the text highlights the break of
// its last line, also when nothing else of the paragraph is selected.
func TestHighlightRects(t *testing.T) {
	l := simple(t, "hello world", ParagraphStyle{}, inter16, 1000)
	space := spaceAdvance(l.sinfo[0].face, 16)
	end := l.Line(0).Width
	x6 := l.Caret(cur(6)).X
	s := NewSelection(cur(6), cur(11))
	if got := spansOn(l, l.HighlightRects(s, false, nil), 0); !nearSpans(got, [][2]float32{{x6, end}}) {
		t.Errorf("within the text: %v", got)
	}
	if got := spansOn(l, l.HighlightRects(s, true, nil), 0); !nearSpans(got, [][2]float32{{x6, end + space}}) {
		t.Errorf("past the end: %v, want the break highlighted", got)
	}
	// A selection that starts at the very end and goes on: only the break.
	at := NewSelection(cur(11), cur(11))
	if got := spansOn(l, l.HighlightRects(at, true, nil), 0); !nearSpans(got, [][2]float32{{end, end + space}}) {
		t.Errorf("from the end: %v", got)
	}
	if got := l.HighlightRects(at, false, nil); len(got) != 0 {
		t.Errorf("a collapsed selection highlights %v", got)
	}
	// Right to left: the break is on the left.
	rtl := simple(t, "שלום עולם", ParagraphStyle{Direction: DirectionRTL}, inter16, 1000)
	all := rtl.SelectAll()
	rs := rtl.HighlightRects(all, true, nil)
	ln := rtl.Line(0)
	if len(rs) != 1 || !near(rs[0].X0, ln.X-rtl.breakWidth(0)) || !near(rs[0].X1, ln.X+ln.Width) {
		t.Errorf("rtl: %v, line %+v", rs, ln)
	}

	// Inline boxes: SelectionRects covers them, HighlightRects does not.
	p := &Paragraph{Default: inter16}
	p.AddText("ab ", inter16)
	p.AddBox(InlineBox{ID: 1, Width: 30, Height: 10})
	p.AddText(" cd", inter16)
	b := NewLayout(layoutCollection(t), p)
	b.BreakLines(1000)
	box, _ := b.BoxRect(0)
	sel := b.SelectAll()
	covers := func(rs []gfx.Rect, x float32) bool {
		for _, r := range rs {
			if x > r.X0 && x < r.X1 {
				return true
			}
		}
		return false
	}
	mid := (box.X0 + box.X1) / 2
	if !covers(b.SelectionRects(sel, nil), mid) {
		t.Error("SelectionRects leaves the box out")
	}
	if covers(b.HighlightRects(sel, false, nil), mid) {
		t.Error("HighlightRects covers the box")
	}
}

// TestRenderedText checks the text copied from static text (ADR-271): the
// laid-out characters, white space processed and transformed, without
// inline boxes, a soft wrap's space kept, removed spaces at a forced break
// and at the end dropped.
func TestRenderedText(t *testing.T) {
	l := simple(t, "  hello    brave\n world  ", ParagraphStyle{}, inter16, 1000)
	if got := l.RenderedText(l.SelectAll()); got != "hello brave world" {
		t.Errorf("collapsed: %q", got)
	}
	// Part of the text: from inside "brave" to inside "world".
	src := l.Text()
	a, b := strings.Index(src, "rave"), strings.Index(src, "rld")
	if got := l.RenderedText(NewSelection(cur(a), cur(b))); got != "rave wo" {
		t.Errorf("part: %q", got)
	}
	// A soft wrap keeps the space between the words.
	w := simple(t, "hello world foo", ParagraphStyle{}, inter16, 60)
	if w.NumLines() < 2 {
		t.Fatalf("did not wrap: %v", lineTexts(w))
	}
	if got := w.RenderedText(w.SelectAll()); got != "hello world foo" {
		t.Errorf("wrapped: %q", got)
	}
	// Preserved newlines and spaces; the space before a forced break of
	// pre-line is removed.
	pre := simple(t, "a  b\nc", ParagraphStyle{WhiteSpace: WhiteSpacePre}, inter16, 1000)
	if got := pre.RenderedText(pre.SelectAll()); got != "a  b\nc" {
		t.Errorf("pre: %q", got)
	}
	pl := simple(t, "a  b \nc", ParagraphStyle{WhiteSpace: WhiteSpacePreLine}, inter16, 1000)
	if got := pl.RenderedText(pl.SelectAll()); got != "a b\nc" {
		t.Errorf("pre-line: %q", got)
	}
	// text-transform.
	up := inter16
	up.Transform = TransformUppercase
	u := simple(t, "Straße", ParagraphStyle{}, up, 1000)
	if got := u.RenderedText(u.SelectAll()); got != "STRASSE" {
		t.Errorf("uppercase: %q", got)
	}
	// Inline boxes are left out.
	p := &Paragraph{Default: inter16}
	p.AddText("ab", inter16)
	p.AddBox(InlineBox{ID: 1, Width: 30, Height: 10})
	p.AddText("cd", inter16)
	bx := NewLayout(layoutCollection(t), p)
	bx.BreakLines(1000)
	if got := bx.RenderedText(bx.SelectAll()); got != "abcd" {
		t.Errorf("boxes: %q", got)
	}
	if got := l.RenderedText(NewSelection(cur(3), cur(3))); got != "" {
		t.Errorf("collapsed selection: %q", got)
	}
}
