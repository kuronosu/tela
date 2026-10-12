package text

import (
	"math"
	"testing"
)

var inf32 = float32(math.Inf(1))

// cur is a shorthand for a Downstream cursor.
func cur(off int) Cursor { return Cursor{off, Downstream} }

// up is a shorthand for an Upstream cursor.
func up(off int) Cursor { return Cursor{off, Upstream} }

// TestSnapCursor checks that offsets snap to caret positions: grapheme
// cluster boundaries (back to the start of a grapheme, forward out of
// collapsed white space) and the canonical affinity.
func TestSnapCursor(t *testing.T) {
	pre := ParagraphStyle{WhiteSpace: WhiteSpacePre}
	for _, c := range []struct {
		text string
		ps   ParagraphStyle
		in   Cursor
		want Cursor
	}{
		// Combining marks: inside the grapheme, back to its start.
		{"éa", ParagraphStyle{}, cur(1), cur(0)},
		{"éa", ParagraphStyle{}, cur(2), cur(0)},
		{"éa", ParagraphStyle{}, cur(3), cur(3)},
		// Inside UTF-8 sequences and emoji sequences.
		{"😀x", ParagraphStyle{}, cur(2), cur(0)},
		{"😀x", ParagraphStyle{}, cur(4), cur(4)},
		{"👨‍👩‍👧‍👦x", ParagraphStyle{}, cur(11), cur(0)},
		{"🇯🇵🇧🇷", ParagraphStyle{}, cur(4), cur(0)},
		{"🇯🇵🇧🇷", ParagraphStyle{}, cur(8), cur(8)},
		{"🇯🇵🇧🇷", ParagraphStyle{}, cur(12), cur(8)},
		{"👍🏽x", ParagraphStyle{}, cur(4), cur(0)},
		{"1️⃣2", ParagraphStyle{}, cur(4), cur(0)},
		// Collapsed white space: forward to where the text resumes.
		{"a   b", ParagraphStyle{}, cur(2), cur(4)},
		{"a   b", ParagraphStyle{}, cur(3), cur(4)},
		{"a   b", ParagraphStyle{}, cur(1), cur(1)},
		{"a \n b", ParagraphStyle{}, cur(1), cur(2)},
		{"abc   ", ParagraphStyle{}, cur(4), cur(6)},
		// Removed leading spaces belong to the first cluster: back to 0.
		{"  a", ParagraphStyle{}, cur(1), cur(0)},
		{"  a", ParagraphStyle{}, cur(2), cur(0)},
		// Only removed white space: the end.
		{"   ", ParagraphStyle{}, cur(1), cur(3)},
		// CR LF is one forced break.
		{"a\r\nb", pre, cur(2), cur(3)},
		// Out of range.
		{"abc", ParagraphStyle{}, cur(-5), cur(0)},
		{"abc", ParagraphStyle{}, cur(99), cur(3)},
		{"", ParagraphStyle{}, up(3), cur(0)},
		// Upstream that looks like Downstream is canonicalized.
		{"abc", ParagraphStyle{}, up(1), cur(1)},
		{"abc", ParagraphStyle{}, up(0), cur(0)},
		{"abc", ParagraphStyle{}, up(3), cur(3)},
		{"ab\ncd", pre, up(3), cur(3)}, // after a forced break: the next line
		// The ligature "->" has a caret position between its graphemes.
		{"a->b", ParagraphStyle{}, up(2), cur(2)},
		// Invalid UTF-8: one position per byte.
		{"a\xffb", ParagraphStyle{}, cur(2), cur(2)},
	} {
		l := simple(t, c.text, c.ps, inter16, inf32)
		if got := l.SnapCursor(c.in); got != c.want {
			t.Errorf("%q: SnapCursor(%v) = %v, want %v", c.text, c.in, got, c.want)
		}
	}
	// Upstream stays where it differs: a soft wrap after preserved spaces,
	// a bidi boundary, an inline box.
	l := simple(t, "hello   world", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 60)
	if got := l.SnapCursor(up(8)); got != up(8) {
		t.Errorf("soft wrap: %v", got)
	}
	l = simple(t, "ab مرحبا", ParagraphStyle{}, inter16, inf32)
	if got := l.SnapCursor(up(3)); got != up(3) {
		t.Errorf("bidi boundary: %v", got)
	}
	// White space removed at a soft wrap: Upstream is the next line too.
	l = simple(t, "hello world", ParagraphStyle{}, inter16, 60)
	if got := l.SnapCursor(up(6)); got != cur(6) {
		t.Errorf("collapsed space at a wrap: %v", got)
	}
}

// TestCaretLTR checks carets in left-to-right text: at cluster edges, with
// letter-spacing on the right of each cluster, justification and tabs.
func TestCaretLTR(t *testing.T) {
	ls := inter16
	ls.LetterSpacing = 3
	for _, c := range []struct {
		name string
		text string
		ps   ParagraphStyle
		st   TextStyle
		w    float32
	}{
		{"plain", "Hello, world", ParagraphStyle{}, inter16, inf32},
		{"letter-spacing", "Spaced", ParagraphStyle{}, ls, inf32},
		{"justify", "Justified text stretches its word separators so lines fill", ParagraphStyle{Align: AlignJustify}, inter16, 150},
		{"center", "centered", ParagraphStyle{Align: AlignCenter}, inter16, 200},
		{"tabs", "a\tb\t\tc", ParagraphStyle{WhiteSpace: WhiteSpacePre}, inter16, inf32},
		{"cjk", "日本語の文章です", ParagraphStyle{}, inter16, 70},
		{"emoji", "a😀👨‍👩‍👧‍👦🇯🇵👍🏽b", ParagraphStyle{}, inter16, inf32},
	} {
		l := simple(t, c.text, c.ps, c.st, c.w)
		for ci := range l.NumClusters() {
			cl := l.Cluster(ci)
			if cl.Line < 0 || cl.Graphemes != 1 {
				continue
			}
			ca := l.Caret(cur(cl.TextStart))
			wantLine := cl.Line
			if cl.Flags&ClusterHanging != 0 && cl.Advance == 0 && ci > 0 && l.Cluster(ci-1).Line != cl.Line {
				continue
			}
			if ca.Line != wantLine || !near(ca.X, cl.X) || ca.RTL {
				t.Errorf("%s: caret before %q at line %d x %g, want line %d x %g", c.name, c.text[cl.TextStart:cl.TextEnd], ca.Line, ca.X, wantLine, cl.X)
			}
			// The caret after the cluster (Upstream) is at its right edge,
			// letter-spacing included, unless it is at the start of the
			// next line.
			ca = l.Caret(up(cl.TextEnd))
			if ca.Line == cl.Line && !near(ca.X, cl.X+cl.Advance) {
				t.Errorf("%s: caret after %q at x %g, want %g", c.name, c.text[cl.TextStart:cl.TextEnd], ca.X, cl.X+cl.Advance)
			}
			ln := l.Line(ca.Line)
			if ca.Top != ln.Top || ca.Height != ln.Height || ca.Baseline != ln.Baseline {
				t.Errorf("%s: caret box %+v, line %+v", c.name, ca, ln)
			}
		}
	}
	// Caret metrics: the font of the text, rounded.
	l := simple(t, "aمرحبا", ParagraphStyle{}, inter16, inf32)
	if ca := l.Caret(cur(0)); ca.Ascent != 16 || ca.Descent != 4 {
		t.Errorf("Inter 16px caret metrics %g/%g, want 16/4", ca.Ascent, ca.Descent)
	}
	if ca := l.Caret(cur(1)); ca.Ascent == 16 && ca.Descent == 4 {
		t.Errorf("Arabic caret has Inter's metrics: %+v", ca)
	}
}

// TestCaretRTL checks carets in right-to-left text: the caret of a
// cluster's logical start is at its right edge.
func TestCaretRTL(t *testing.T) {
	l := simple(t, arabicText, ParagraphStyle{Direction: DirectionRTL}, inter16, 200)
	for ci := range l.NumClusters() {
		cl := l.Cluster(ci)
		if cl.Line < 0 || cl.Level%2 == 0 || cl.Advance == 0 {
			continue
		}
		ca := l.Caret(cur(cl.TextStart))
		if ca.Line != cl.Line || !near(ca.X, cl.X+cl.Advance) || !ca.RTL {
			t.Errorf("caret before %q: line %d x %g rtl %v, want line %d x %g", l.Text()[cl.TextStart:cl.TextEnd], ca.Line, ca.X, ca.RTL, cl.Line, cl.X+cl.Advance)
		}
		if ca := l.Caret(up(cl.TextEnd)); ca.Line == cl.Line && !near(ca.X, cl.X) {
			t.Errorf("caret after %q: x %g, want %g", l.Text()[cl.TextStart:cl.TextEnd], ca.X, cl.X)
		}
	}
	// The start of the paragraph is at the right edge of the first line,
	// the end at the left edge of the last one.
	ln := l.Line(0)
	if ca := l.Caret(cur(0)); !near(ca.X, ln.X+ln.Width) {
		t.Errorf("start at %g, want %g", ca.X, ln.X+ln.Width)
	}
	last := l.Line(l.NumLines() - 1)
	if ca := l.Caret(cur(len(arabicText))); ca.Line != l.NumLines()-1 || !near(ca.X, last.X) {
		t.Errorf("end at line %d x %g, want line %d x %g", ca.Line, ca.X, l.NumLines()-1, last.X)
	}
	// An empty right-to-left paragraph has its caret at the right edge.
	l = simple(t, "", ParagraphStyle{Direction: DirectionRTL}, inter16, 100)
	if ca := l.Caret(cur(0)); ca.X != 100 || !ca.RTL {
		t.Errorf("empty RTL caret %+v", ca)
	}
}

// TestCaretBidi checks the two carets of a bidi boundary and numbers
// inside right-to-left text.
func TestCaretBidi(t *testing.T) {
	// "ab مرحبا cd": the Arabic word is at level 1 in an LTR paragraph.
	l := simple(t, "ab مرحبا cd", ParagraphStyle{}, inter16, inf32)
	space, meem := l.Cluster(2), l.Cluster(3)
	alef := l.Cluster(7) // the logical end of the Arabic word, drawn leftmost
	space2 := l.Cluster(8)
	for _, c := range []struct {
		in   Cursor
		x    float32
		rtl  bool
		what string
	}{
		{up(3), space.X + space.Advance, false, "after the space (Upstream)"},
		{cur(3), meem.X + meem.Advance, true, "before the Arabic word (Downstream): its right edge"},
		{up(13), alef.X, true, "after the Arabic word (Upstream): its left edge"},
		{cur(13), space2.X, false, "before the second space (Downstream)"},
	} {
		if ca := l.Caret(c.in); !near(ca.X, c.x) || ca.RTL != c.rtl {
			t.Errorf("%s: %v at x %g rtl %v, want x %g rtl %v", c.what, c.in, ca.X, ca.RTL, c.x, c.rtl)
		}
	}
	// Numbers in right-to-left text run left to right (level 2): the
	// caret moves right through "2024".
	l = simple(t, "مرحبا 2024 بكم", ParagraphStyle{Direction: DirectionRTL}, inter16, inf32)
	prev := float32(-1)
	for off := 11; off <= 15; off++ {
		ca := l.Caret(cur(off))
		if off < 15 && (ca.RTL || ca.X <= prev) {
			t.Errorf("digit caret %d at x %g rtl %v (previous %g)", off, ca.X, ca.RTL, prev)
		}
		prev = ca.X
	}
}

// TestCaretLigature checks carets between the graphemes of a ligature:
// Inter's "->" (calt) is one cluster of two graphemes.
func TestCaretLigature(t *testing.T) {
	l := simple(t, "a->b", ParagraphStyle{}, inter16, inf32)
	lig := l.Cluster(1)
	if lig.Graphemes != 2 {
		t.Fatalf("no ligature: %+v", lig)
	}
	if ca := l.Caret(cur(2)); !near(ca.X, lig.X+lig.Advance/2) {
		t.Errorf("caret inside the ligature at %g, want %g", ca.X, lig.X+lig.Advance/2)
	}
	if ca := l.Caret(up(2)); !near(ca.X, lig.X+lig.Advance/2) {
		t.Errorf("Upstream inside the ligature at %g", ca.X)
	}
}

// TestCaretLineBreaks checks carets at soft wraps (affinity), forced
// breaks and a trailing newline.
func TestCaretLineBreaks(t *testing.T) {
	// Preserved spaces hang: Upstream is after them on the first line,
	// Downstream at the start of the second.
	l := simple(t, "hello   world", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 60)
	if l.NumLines() != 2 {
		t.Fatalf("lines:\n%s", dump(l))
	}
	l0 := l.Line(0)
	if ca := l.Caret(up(8)); ca.Line != 0 || !near(ca.X, l0.X+l0.Width+l0.Hanging) {
		t.Errorf("Upstream at a soft wrap: %+v, line %+v", ca, l0)
	}
	if ca := l.Caret(cur(8)); ca.Line != 1 || ca.X != 0 {
		t.Errorf("Downstream at a soft wrap: %+v", ca)
	}
	// A collapsed space at a wrap: both affinities after it are on the
	// next line; before it is the end of the first line.
	l = simple(t, "hello world", ParagraphStyle{}, inter16, 60)
	for _, c := range []Cursor{up(6), cur(6)} {
		if ca := l.Caret(c); ca.Line != 1 || ca.X != 0 {
			t.Errorf("after the collapsed space %v: %+v", c, ca)
		}
	}
	if ca := l.Caret(cur(5)); ca.Line != 0 || !near(ca.X, l.Line(0).Width) {
		t.Errorf("before the collapsed space: %+v", ca)
	}
	// A forced break: after it is the next line, before it the end of the
	// line.
	l = simple(t, "ab\ncd", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, inf32)
	for _, c := range []Cursor{up(3), cur(3)} {
		if ca := l.Caret(c); ca.Line != 1 || ca.X != 0 {
			t.Errorf("after a forced break %v: %+v", c, ca)
		}
	}
	if ca := l.Caret(cur(2)); ca.Line != 0 || !near(ca.X, l.Line(0).Width) {
		t.Errorf("before a forced break: %+v", ca)
	}
	// A trailing newline: the end is on the empty last line, aligned.
	l = simple(t, "ab\n", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap, Align: AlignCenter}, inter16, 60)
	if ca := l.Caret(cur(3)); ca.Line != 1 || ca.X != 30 {
		t.Errorf("end after a trailing newline: %+v", ca)
	}
	// Empty text: one caret on the strut's line.
	l = simple(t, "", ParagraphStyle{}, inter16, 60)
	if ca := l.Caret(cur(0)); ca.Line != 0 || ca.X != 0 || ca.Height != 24 {
		t.Errorf("empty text: %+v", ca)
	}
}

// TestCaretInlineBoxes checks the carets on both sides of inline boxes.
func TestCaretInlineBoxes(t *testing.T) {
	p := &Paragraph{Default: inter16}
	p.AddBox(InlineBox{Width: 10, Height: 10})
	p.AddText("ab", inter16)
	p.AddBox(InlineBox{Width: 20, Height: 10})
	p.AddText("cd", inter16)
	p.AddBox(InlineBox{Width: 10, Height: 10})
	l := NewLayout(layoutCollection(t), p)
	b0, _ := l.BoxRect(0)
	b1, _ := l.BoxRect(1)
	b2, _ := l.BoxRect(2)
	for _, c := range []struct {
		in   Cursor
		x    float32
		what string
	}{
		{up(0), b0.X0, "before the first box"},
		{cur(0), b0.X1, "after the first box"},
		{up(2), b1.X0, "before the middle box"},
		{cur(2), b1.X1, "after the middle box"},
		{up(4), b2.X0, "before the last box"},
		{cur(4), b2.X1, "after the last box"},
	} {
		if ca := l.Caret(c.in); !near(ca.X, c.x) {
			t.Errorf("%s: %v at %g, want %g", c.what, c.in, ca.X, c.x)
		}
		if got := l.SnapCursor(c.in); got != c.in {
			t.Errorf("%s: SnapCursor(%v) = %v", c.what, c.in, got)
		}
	}
}

// TestCaretHidden checks positions in text hidden by text-overflow and
// line-clamp: at the end-side edge of the ellipsis.
func TestCaretHidden(t *testing.T) {
	ps := ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}
	l := simple(t, "Hello wonderful world", ps, inter16, 60)
	runs := l.LineRuns(0, nil)
	ell := runs[len(runs)-1]
	if ell.Kind != RunEllipsis {
		t.Fatalf("no ellipsis: %+v", runs)
	}
	for _, off := range []int{7, 12, 21} {
		if ca := l.Caret(cur(off)); !ca.Hidden || !near(ca.X, ell.X+ell.Width) {
			t.Errorf("hidden %d: %+v, ellipsis %+v", off, ca, ell)
		}
	}
	if ca := l.Caret(cur(5)); ca.Hidden {
		t.Errorf("visible position hidden: %+v", ca)
	}
	// Right to left: the ellipsis is on the left.
	ps.Direction = DirectionRTL
	l = simple(t, "مرحبا بكم في هذا السطر", ps, inter16, 60)
	runs = l.LineRuns(0, nil)
	if ca := l.Caret(cur(len(l.Text()))); !ca.Hidden || !near(ca.X, runs[0].X) {
		t.Errorf("RTL hidden end: %+v, runs %+v", ca, runs)
	}
	// line-clamp: the clamped text is at the last line's ellipsis.
	l = simple(t, "one two three four five six seven", ParagraphStyle{LineClamp: 2}, inter16, 70)
	if ca := l.Caret(cur(len(l.Text()))); !ca.Hidden || ca.Line != 1 {
		t.Errorf("clamped end: %+v", ca)
	}
	if ca := l.Caret(cur(l.ClampOffset())); !ca.Hidden || ca.Line != 1 {
		t.Errorf("clamp offset: %+v", ca)
	}
}

// TestCursorRect checks the caret rectangle: line box height, on the side
// of the text it belongs to.
func TestCursorRect(t *testing.T) {
	l := simple(t, "ab مرحبا", ParagraphStyle{}, inter16, inf32)
	r := l.CursorRect(cur(1), 2)
	ca := l.Caret(cur(1))
	if r.X0 != ca.X || r.X1 != ca.X+2 || r.Y0 != 0 || r.Y1 != 24 {
		t.Errorf("LTR caret rect %v, caret %+v", r, ca)
	}
	r = l.CursorRect(cur(5), 2)
	ca = l.Caret(cur(5))
	if r.X1 != ca.X || r.X0 != ca.X-2 {
		t.Errorf("RTL caret rect %v, caret %+v", r, ca)
	}
}

// TestRebreakCursors checks that cursor geometry follows BreakLines and
// AlignTo without reshaping.
func TestRebreakCursors(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog: مرحبا بكم 日本語"
	l := simple(t, text, ParagraphStyle{}, inter16, inf32)
	widths := []float32{80, 150, 300, 1000}
	var fresh []*Layout
	for _, w := range widths {
		fresh = append(fresh, simple(t, text, ParagraphStyle{}, inter16, w))
	}
	start := shapeCalls.Load()
	for i, w := range widths {
		l.BreakLines(w)
		fresh := fresh[i]
		for off := 0; off <= len(text); off++ {
			for _, a := range []Affinity{Downstream, Upstream} {
				if g, f := l.Caret(Cursor{off, a}), fresh.Caret(Cursor{off, a}); g != f {
					t.Fatalf("width %g: caret %d/%d %+v, fresh layout %+v", w, off, a, g, f)
				}
			}
		}
		if h, f := l.HitTest(40, 30), fresh.HitTest(40, 30); h != f {
			t.Errorf("width %g: hit %v, fresh %v", w, h, f)
		}
	}
	l.AlignTo(1200)
	if ca := l.Caret(cur(0)); ca.X != 0 {
		t.Errorf("AlignTo: %+v", ca)
	}
	if got := shapeCalls.Load() - start; got != 0 {
		t.Errorf("cursor queries shaped %d times", got)
	}
}
