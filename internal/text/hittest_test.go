package text

import (
	"fmt"
	"math"
	"testing"
)

type namedLayout struct {
	name string
	l    *Layout
}

// cursorCorpus returns layouts that cover what cursors must handle: LTR,
// RTL, mixed bidi with numbers, CJK, emoji sequences, ligatures, combining
// marks, soft and forced breaks, hanging and collapsed spaces, tabs,
// inline boxes, justification, letter-spacing, truncation and clamping.
func cursorCorpus(t testing.TB) []namedLayout {
	c := layoutCollection(t)
	var out []namedLayout
	add := func(name, text string, ps ParagraphStyle, st TextStyle, w float32) {
		l := NewLayout(c, &Paragraph{Text: text, Style: ps, Default: st})
		l.BreakLines(w)
		out = append(out, namedLayout{name, l})
	}
	ls := inter16
	ls.LetterSpacing = 2
	rtl := ParagraphStyle{Direction: DirectionRTL}
	preWrap := ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}
	bidi := "The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done."
	bidiRTL := "مرحبا بكم في Tela version 2.0 مع 15 ميزة (جديدة) اليوم."
	add("latin", latinText, ParagraphStyle{}, inter16, 200)
	add("latin-justify", latinText, ParagraphStyle{Align: AlignJustify}, inter16, 230)
	add("latin-center", latinText, ParagraphStyle{Align: AlignCenter}, inter16, 230)
	add("arabic", arabicText, rtl, inter16, 200)
	add("arabic-justify", arabicText, ParagraphStyle{Direction: DirectionRTL, Align: AlignJustify}, inter16, 200)
	add("bidi-ltr", bidi, ParagraphStyle{}, inter16, inf32)
	add("bidi-ltr-wrap", bidi, ParagraphStyle{}, inter16, 170)
	add("bidi-rtl", bidiRTL, rtl, inter16, inf32)
	add("bidi-rtl-wrap", bidiRTL, rtl, inter16, 150)
	add("cjk", cjkText, ParagraphStyle{}, inter16, 200)
	add("emoji", "Hi 👋 world 🌍🎉 family 👨‍👩‍👧‍👦 flags 🇯🇵🇧🇷 thumbs 👍🏽 ok 1️⃣", ParagraphStyle{}, inter16, 150)
	add("ligature", "a->b c=>d x->y office", ParagraphStyle{}, inter16, 90)
	add("combining", "été café ñand̃u Zürich بِسْمِ", ParagraphStyle{}, inter16, 120)
	add("pre-wrap", "Keep   these spaces\n\tand a tab\tinside\nthen a long line that has to wrap somewhere\n", preWrap, inter16, 160)
	add("pre-wrap-rtl", "مرحبا    بكم في\nهذا السطر", ParagraphStyle{Direction: DirectionRTL, WhiteSpace: WhiteSpacePreWrap}, inter16, 70)
	add("collapse", "  Spaces   and\ttabs\n and   newlines   collapse ", ParagraphStyle{}, inter16, 120)
	add("pre-line", "first line\nsecond   line that wraps\n\nlast", ParagraphStyle{WhiteSpace: WhiteSpacePreLine}, inter16, 120)
	add("tabs", "a\tbb\t\tccc\td", ParagraphStyle{WhiteSpace: WhiteSpacePre}, inter16, inf32)
	add("letter-spacing", "Spaced out letters wrap like this", ParagraphStyle{}, ls, 120)
	add("truncate", "Hello wonderful world of truncated text", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 100)
	add("truncate-rtl", "مرحبا بكم في هذا السطر الطويل جدا", ParagraphStyle{Direction: DirectionRTL, WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 100)
	add("truncate-bidi", "abc مرحبا بكم def", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 70)
	add("clamp", latinText, ParagraphStyle{LineClamp: 2}, inter16, 150)
	add("empty", "", ParagraphStyle{}, inter16, 100)
	add("empty-rtl", "", rtl, inter16, 100)
	add("trailing-newline", "ab\n", preWrap, inter16, 100)
	add("newlines", "\n\na\n\n", preWrap, inter16, 100)
	add("narrow", "Supercalifragilistic expialidocious", ParagraphStyle{OverflowWrap: OverflowWrapBreakWord}, inter16, 40)
	// Inline boxes, alone between wraps too.
	for _, w := range []float32{inf32, 45, 30} {
		p := &Paragraph{Default: inter16}
		p.AddBox(InlineBox{Width: 10, Height: 10})
		p.AddText("ab ", inter16)
		p.AddBox(InlineBox{Width: 20, Height: 10, Align: BoxMiddle})
		p.AddText("cd مرحبا ", inter16)
		p.AddBox(InlineBox{Width: 12, Height: 12})
		p.AddText(" بكم", inter16)
		p.AddBox(InlineBox{Width: 10, Height: 10})
		l := NewLayout(c, p)
		l.BreakLines(w)
		out = append(out, namedLayout{fmt.Sprintf("boxes-%g", w), l})
	}
	return out
}

// positions returns the distinct canonical cursors of l.
func positions(l *Layout) []Cursor {
	seen := map[Cursor]bool{}
	var out []Cursor
	for off := 0; off <= len(l.Text()); off++ {
		for _, a := range []Affinity{Downstream, Upstream} {
			c := l.SnapCursor(Cursor{off, a})
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// TestHitTestRoundTrip checks that hit-testing a caret gives its cursor
// back. The invariant: for every canonical cursor whose caret is attached
// to a cluster with an advance, HitTest at the caret's x moved by epsilon
// into that cluster (right for the leading edge of left-to-right text and
// the trailing edge of right-to-left text, left otherwise), on the middle
// of the caret's line, returns that cursor, affinity included. This holds
// at line ends, soft wraps, bidi boundaries, inline boxes and inside
// ligatures. Carets attached to nothing (empty lines), to a cluster
// without advance (removed spaces, forced breaks, controls) or to hidden
// text are excluded; TestHitTestEdges covers them.
func TestHitTestRoundTrip(t *testing.T) {
	for _, nl := range cursorCorpus(t) {
		l := nl.l
		checked := 0
		for _, c := range positions(l) {
			info := l.caretFor(l.snap(c.Offset), c.Affinity)
			if info.ci < 0 {
				continue
			}
			w := l.cadv[info.ci]
			if !(w > 0) {
				continue
			}
			g := float32(l.clusters[info.ci].graph)
			if l.clusters[info.ci].flags&cfBox != 0 {
				g = 1
			}
			eps := min(0.01, w/(4*g))
			dir := float32(1)
			if (info.edge < 0) != info.rtl {
				dir = -1
			}
			ln := l.Line(int(info.line))
			x, y := info.x+dir*eps, ln.Top+ln.Height/2
			if got := l.HitTest(x, y); got != c {
				t.Errorf("%s: caret of %v at (%g, line %d): HitTest(%g, %g) = %v", nl.name, c, info.x, info.line, x, y, got)
			}
			checked++
		}
		if checked == 0 && l.NumClusters() > 0 {
			t.Errorf("%s: nothing checked", nl.name)
		}
	}
}

// TestHitTestEdges checks points outside the text and on lines' ends.
func TestHitTestEdges(t *testing.T) {
	l := simple(t, "hello world foo", ParagraphStyle{}, inter16, 60)
	if l.NumLines() != 3 {
		t.Fatalf("lines:\n%s", dump(l))
	}
	for _, c := range []struct {
		x, y float32
		want Cursor
		what string
	}{
		{-10, -10, cur(0), "above the first line, left"},
		{1000, -10, cur(5), "above the first line, right: before the collapsed space"},
		{1000, 12, cur(5), "past the end of a wrapped line"},
		{-10, 30, cur(6), "before the start of the second line"},
		{1000, 1000, cur(15), "below the last line"},
		{float32(math.NaN()), float32(math.NaN()), cur(0), "NaN"},
		{float32(math.Inf(-1)), float32(math.Inf(1)), cur(12), "infinities"},
	} {
		if got := l.HitTest(c.x, c.y); got != c.want {
			t.Errorf("%s: HitTest(%g, %g) = %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
	// Past the end of a line that ends in a forced break: before it.
	l = simple(t, "ab\ncd", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, inf32)
	if got := l.HitTest(500, 5); got != cur(2) {
		t.Errorf("past a forced break: %v", got)
	}
	// Right to left: left of the line is its end.
	l = simple(t, "مرحبا بكم", ParagraphStyle{Direction: DirectionRTL}, inter16, 200)
	if got := l.HitTest(-5, 5); got.Offset != len(l.Text()) {
		t.Errorf("left of an RTL line: %v", got)
	}
	if got := l.HitTest(500, 5); got != cur(0) {
		t.Errorf("right of an RTL line: %v", got)
	}
	// Clicking on a bidi boundary puts the caret where the pointer is.
	l = simple(t, "ab مرحبا cd", ParagraphStyle{}, inter16, inf32)
	alef := l.Cluster(7)
	if got := l.HitTest(alef.X+0.5, 5); got != up(13) || !near(l.Caret(got).X, alef.X) {
		t.Errorf("left edge of the Arabic word: %v (caret %g, edge %g)", got, l.Caret(got).X, alef.X)
	}
	// Ligature halves.
	l = simple(t, "a->b", ParagraphStyle{}, inter16, inf32)
	lig := l.Cluster(1)
	for _, c := range []struct {
		x    float32
		want Cursor
	}{
		{lig.X + lig.Advance*0.2, cur(1)},
		{lig.X + lig.Advance*0.4, cur(2)},
		{lig.X + lig.Advance*0.6, cur(2)},
		{lig.X + lig.Advance*0.8, cur(3)},
	} {
		if got := l.HitTest(c.x, 5); got != c.want {
			t.Errorf("ligature at %g: %v, want %v", c.x, got, c.want)
		}
	}
	// Ellipsis: the visible text on its start half, the end of the hidden
	// text on its end half.
	l = simple(t, "Hello wonderful world", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 60)
	runs := l.LineRuns(0, nil)
	ell := runs[len(runs)-1]
	if got := l.HitTest(ell.X+1, 5); got != up(6) {
		t.Errorf("ellipsis start half: %v", got)
	}
	if got := l.HitTest(ell.X+ell.Width-1, 5); got != cur(21) {
		t.Errorf("ellipsis end half: %v", got)
	}
	// A line that holds only an inline box between two wraps: before the
	// box on its left half, after it on its right half.
	p := &Paragraph{Default: inter16}
	p.AddText("ab", inter16)
	p.AddBox(InlineBox{Width: 20, Height: 10})
	p.AddText("cd", inter16)
	l = NewLayout(layoutCollection(t), p)
	l.BreakLines(30)
	if l.NumLines() != 3 {
		t.Fatalf("box lines:\n%s", dump(l))
	}
	if got := l.HitTest(3, 36); got != up(2) {
		t.Errorf("box-only line, left half: %v", got)
	}
	if got := l.HitTest(17, 36); got != cur(2) {
		t.Errorf("box-only line, right half: %v", got)
	}
}

// TestHitTestCluster checks what is under a point.
func TestHitTestCluster(t *testing.T) {
	p := &Paragraph{Default: inter16}
	p.AddText("ab ", inter16)
	p.AddBox(InlineBox{ID: 3, Width: 20, Height: 10})
	p.AddText(" مرحبا", inter16)
	l := NewLayout(layoutCollection(t), p)
	a := l.Cluster(0)
	box, _ := l.BoxRect(0)
	meem := l.Cluster(5)
	for _, c := range []struct {
		x, y float32
		ok   bool
		want ClusterHit
	}{
		{a.X + 1, 12, true, ClusterHit{Cluster: 0, Line: 0, Box: -1, Leading: true}},
		{a.X + a.Advance - 1, 12, true, ClusterHit{Cluster: 0, Line: 0, Box: -1}},
		{box.X0 + 2, box.Y0 + 2, true, ClusterHit{Cluster: 3, Line: 0, Box: 0, Leading: true}},
		{box.X0 + 2, box.Y0 - 2, false, ClusterHit{}}, // above the box, inside the line
		{meem.X + meem.Advance - 1, 12, true, ClusterHit{Cluster: 5, Line: 0, Box: -1, Leading: true}},
		{meem.X + 1, 12, true, ClusterHit{Cluster: 5, Line: 0, Box: -1}},
		{-1, 12, false, ClusterHit{}},
		{a.X + 1, -1, false, ClusterHit{}},
		{a.X + 1, 24, false, ClusterHit{}},
		{1000, 12, false, ClusterHit{}},
	} {
		got, ok := l.HitTestCluster(c.x, c.y)
		if ok != c.ok || got != c.want {
			t.Errorf("HitTestCluster(%g, %g) = %+v %v, want %+v %v", c.x, c.y, got, ok, c.want, c.ok)
		}
	}
	l = simple(t, "Hello wonderful world", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 60)
	runs := l.LineRuns(0, nil)
	ell := runs[len(runs)-1]
	if got, ok := l.HitTestCluster(ell.X+1, 5); !ok || !got.Ellipsis || got.Cluster != -1 {
		t.Errorf("ellipsis: %+v %v", got, ok)
	}
}
