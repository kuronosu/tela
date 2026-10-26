package text

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

// Unit tests of text-wrap: balance and pretty (wrapstyle.go). The
// comparison with Chromium is TestChromiumWrapStyle.

func wrapLayout(t *testing.T, s string, ps ParagraphStyle, size, width float32) *Layout {
	t.Helper()
	st := TextStyle{Size: size, LineHeight: LineHeightPixels(size * 1.5)}
	l := NewLayout(layoutCollection(t), &Paragraph{Text: s, Style: ps, Default: st})
	l.BreakLines(width)
	return l
}

func greedyOf(t *testing.T, s string, ps ParagraphStyle, size, width float32) *Layout {
	t.Helper()
	ps.WrapStyle = TextWrapAuto
	return wrapLayout(t, s, ps, size, width)
}

// checkLines checks the invariants of any line breaking: the lines cover
// the clusters in order, and each one fits in width unless greedy breaking
// overflows too.
func checkLines(t *testing.T, l *Layout, width float32) {
	t.Helper()
	next := 0
	for i := range l.NumLines() {
		ln := l.Line(i)
		if ln.ClusterStart != next {
			t.Fatalf("line %d starts at cluster %d, want %d", i, ln.ClusterStart, next)
		}
		next = ln.ClusterEnd
		if ln.Width > width+fitEpsilon {
			t.Errorf("line %d is %g px wide, more than %g", i, ln.Width, width)
		}
	}
	if next != l.NumClusters() && l.ClampOffset() == len(l.Text()) {
		t.Fatalf("the lines end at cluster %d of %d", next, l.NumClusters())
	}
}

var wrapTexts = []string{
	"Balanced headlines read better than ragged ones do",
	"The quick brown fox jumps over the lazy dog while the band plays on and on into the night",
	"Typography is the art and technique of arranging type to make written language legible, readable and appealing when displayed",
	"Extraordinary headline typography",
	"A b c d e f g h i j k l m n o p q r s t u v w x y z",
	"日本語の文章は、漢字、ひらがな、カタカナを組み合わせて書かれます。",
}

func TestBalanceInvariants(t *testing.T) {
	// For every text and width (no word is wider than 110 px): the same
	// number of lines as greedy breaking (Blink keeps it), no line wider
	// than the width, the lines aligned in the full width, and the box's
	// intrinsic widths unchanged.
	for _, s := range wrapTexts {
		for w := float32(110); w <= 520; w += 7 {
			for _, ws := range []TextWrapStyle{TextWrapBalance, TextWrapPretty} {
				ps := ParagraphStyle{WrapStyle: ws, Align: AlignRight}
				l := wrapLayout(t, s, ps, 16, w)
				g := greedyOf(t, s, ps, 16, w)
				name := fmt.Sprintf("%.20q at %g, style %d", s, w, ws)
				if l.NumLines() != g.NumLines() {
					t.Errorf("%s: %d lines, greedy %d", name, l.NumLines(), g.NumLines())
				}
				checkLines(t, l, w)
				if l.AlignWidth() != w {
					t.Errorf("%s: aligned in %g, want the full width", name, l.AlignWidth())
				}
				for i := range l.NumLines() {
					if ln := l.Line(i); ln.Width <= w && math.Abs(float64(ln.X+ln.Width-w)) > 1e-3 {
						t.Errorf("%s: right-aligned line %d ends at %g, want %g", name, i, ln.X+ln.Width, w)
					}
				}
				if l.MinContentWidth() != g.MinContentWidth() || l.MaxContentWidth() != g.MaxContentWidth() {
					t.Errorf("%s: intrinsic widths %g, %g; greedy %g, %g", name,
						l.MinContentWidth(), l.MaxContentWidth(), g.MinContentWidth(), g.MaxContentWidth())
				}
			}
		}
	}
}

func TestBalanceShortensLines(t *testing.T) {
	// "Balanced headlines read better than ragged ones do" at 300 px:
	// greedy breaking leaves "ragged ones do" alone (Chromium's lines are
	// TestChromiumWrapStyle's two-lines case).
	s := wrapTexts[0]
	g := greedyOf(t, s, ParagraphStyle{}, 16, 300)
	l := wrapLayout(t, s, ParagraphStyle{WrapStyle: TextWrapBalance}, 16, 300)
	want := []string{"Balanced headlines read better than ", "ragged ones do"}
	if got := lineTexts(g); !slices.Equal(got, want) {
		t.Fatalf("greedy lines %q, want %q", got, want)
	}
	want = []string{"Balanced headlines read ", "better than ragged ones do"}
	if got := lineTexts(l); !slices.Equal(got, want) {
		t.Fatalf("balanced lines %q, want %q", got, want)
	}
	if l.Width() >= g.Width() || l.Height() != g.Height() {
		t.Errorf("balanced: width %g, height %g; greedy %g, %g", l.Width(), l.Height(), g.Width(), g.Height())
	}
}

func TestBalanceDoesNotApply(t *testing.T) {
	s := wrapTexts[1]
	long := strings.Repeat(s+" ", 3)
	cases := []struct {
		name  string
		text  string
		ps    ParagraphStyle
		width float32
	}{
		{"nowrap", s, ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}, 300},
		{"pre", s, ParagraphStyle{WhiteSpace: WhiteSpacePre}, 300},
		{"one line", s, ParagraphStyle{}, 2000},
		{"no width limit", s, ParagraphStyle{}, float32(math.Inf(1))},
		{"more than 6 lines", long, ParagraphStyle{}, 300},
		{"line-clamp 1", s, ParagraphStyle{LineClamp: 1}, 300},
		{"overflowing word", "A Supercalifragilisticexpialidocious word overflows here", ParagraphStyle{}, 140},
		{"emergency break", "Antidisestablishmentarianism is a long word", ParagraphStyle{OverflowWrap: OverflowWrapBreakWord}, 130},
		{"forced break, few opportunities", "Extraordinary headline typography\nnext", ParagraphStyle{WhiteSpace: WhiteSpacePreLine}, 200},
		{"zero width", s, ParagraphStyle{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps := c.ps
			ps.WrapStyle = TextWrapBalance
			l := wrapLayout(t, c.text, ps, 16, c.width)
			g := greedyOf(t, c.text, ps, 16, c.width)
			if got, want := lineTexts(l), lineTexts(g); !slices.Equal(got, want) {
				t.Errorf("lines %q, want the greedy ones %q", got, want)
			}
		})
	}
}

func TestBalanceFirstPartOnly(t *testing.T) {
	// Blink balances only up to the first forced break.
	s := wrapTexts[0]
	text := s + "\n" + s
	ps := ParagraphStyle{WhiteSpace: WhiteSpacePreLine, WrapStyle: TextWrapBalance}
	got := lineTexts(wrapLayout(t, text, ps, 16, 300))
	want := []string{"Balanced headlines read ", "better than ragged ones do\n", "Balanced headlines read better than ", "ragged ones do"}
	if !slices.Equal(got, want) {
		t.Errorf("lines %q, want %q", got, want)
	}
}

func TestBalanceClampBoundsTheLines(t *testing.T) {
	// With line-clamp, only the visible lines are balanced: the text
	// after the clamp stays where greedy breaking puts it.
	s := wrapTexts[1] + " until the morning"
	for _, k := range []int{2, 3} {
		ps := ParagraphStyle{LineClamp: k, WrapStyle: TextWrapBalance}
		l := wrapLayout(t, s, ps, 16, 300)
		g := greedyOf(t, s, ps, 16, 300)
		if l.ClampOffset() != g.ClampOffset() {
			t.Errorf("clamp %d: the hidden text starts at %d, greedy at %d", k, l.ClampOffset(), g.ClampOffset())
		}
		if l.NumLines() != k {
			t.Errorf("clamp %d: %d lines", k, l.NumLines())
		}
	}
}

func TestBalanceRebreak(t *testing.T) {
	// BreakLines at another width and back gives the same lines: nothing
	// of a balancing is kept.
	s := wrapTexts[2]
	ps := ParagraphStyle{WrapStyle: TextWrapBalance}
	l := wrapLayout(t, s, ps, 16, 280)
	first := lineTexts(l)
	for _, w := range []float32{120, float32(math.Inf(1)), 400, 0, 280} {
		l.BreakLines(w)
	}
	if got := lineTexts(l); !slices.Equal(got, first) {
		t.Errorf("after breaking at other widths: %q, want %q", got, first)
	}
	if fresh := lineTexts(wrapLayout(t, s, ps, 16, 120)); !slices.Equal(fresh, func() []string { l.BreakLines(120); return lineTexts(l) }()) {
		t.Error("breaking again at 120 differs from a fresh layout")
	}
}

func TestBalanceEllipsisAndInlines(t *testing.T) {
	// Balanced lines fit, so text-overflow: ellipsis truncates none of
	// them; inline edges count in the lines' widths.
	s := "Code chips and links with padding wrap like words do in a balanced heading"
	p := &Paragraph{Style: ParagraphStyle{WrapStyle: TextWrapBalance, TextOverflow: TextOverflowEllipsis}, Default: TextStyle{Size: 16}}
	p.AddText("Code ", TextStyle{Size: 16})
	in := p.OpenInline(Inline{Left: 12, Right: 12})
	p.AddText("chips", TextStyle{Size: 16})
	p.CloseInline(in)
	p.AddText(s[len("Code chips"):], TextStyle{Size: 16})
	l := NewLayout(layoutCollection(t), p)
	for w := float32(120); w <= 400; w += 13 {
		l.BreakLines(w)
		checkLines(t, l, w)
		for i := range l.NumLines() {
			if l.Line(i).Ellipsis {
				t.Errorf("at %g: line %d is truncated", w, i)
			}
		}
	}
}

func TestScoreLines(t *testing.T) {
	// Words of 100, 30, 30, 30, 30 and 30 px with no white space, in
	// lines of 160 px. Greedy breaking gives 160 + 90. Balance minimizes
	// the squared free space of every line: 130 + 120 (30² + 40²) beats
	// 160 + 90 (0² + 70²) and 100 + 150 (60² + 10²), and three lines cost
	// a line penalty more. Pretty gives the last line no width score, so
	// the greedy lines are its best.
	l := wrapLayout(t, "x", ParagraphStyle{}, 16, 160)
	var c []wrapCand
	for i, p := range []float32{0, 100, 130, 160, 190, 220, 250} {
		c = append(c, wrapCand{at: int32(i), noBreak: p, ifBreak: p})
	}
	c[len(c)-2].penalty += orphanPenalty
	if ends := l.scoreLines(c, 160, true); !slices.Equal(ends, []int{2, 6}) {
		t.Errorf("balanced line ends %v, want [2 6]", ends)
	}
	if ends := l.scoreLines(c, 160, false); !slices.Equal(ends, []int{3, 6}) {
		t.Errorf("pretty line ends %v, want [3 6]", ends)
	}
	// A line that does not fit is overfull: at 120 px the word of 100 px
	// is alone on its line, and the five others need two lines.
	if ends := l.scoreLines(c, 120, true); len(ends) < 3 {
		t.Errorf("line ends at 120 px %v, want at least 3 lines", ends)
	}
}

func TestBisectWidth(t *testing.T) {
	// "Extraordinary headline typography" has two break opportunities, too
	// few for the score line breaker: the width is bisected to 1 px.
	s := wrapTexts[3]
	g := greedyOf(t, s, ParagraphStyle{}, 16, 200)
	w, ok := g.bisectWidth(200, g.NumLines(), -1, g.advFunc(nil))
	if !ok || !(w < 200) {
		t.Fatalf("bisected width %g, %v", w, ok)
	}
	at := func(w float32) int { return greedyOf(t, s, ParagraphStyle{}, 16, w).NumLines() }
	if at(w) != g.NumLines() || at(w-1) <= g.NumLines() {
		t.Errorf("width %g: %d lines, %d at 1 px less; greedy %d", w, at(w), at(w-1), g.NumLines())
	}
	if w*64 != float32(math.Trunc(float64(w*64))) {
		t.Errorf("width %g is not in LayoutUnits", w)
	}
}

func TestWrapStyleScaleIndependent(t *testing.T) {
	// Line breaks are horizontal: they do not depend on the display
	// scale (Chromium's do not either, on the cases of
	// TestChromiumWrapStyle at 1.5 and 2).
	s := wrapTexts[2]
	for _, ws := range []TextWrapStyle{TextWrapBalance, TextWrapPretty} {
		ps := ParagraphStyle{WrapStyle: ws}
		base := lineTexts(wrapLayout(t, s, ps, 16, 260))
		for _, k := range []float32{1.25, 1.5, 2, 3} {
			p := &Paragraph{Text: s, Style: ps, Default: TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}, Scale: k}
			l := NewLayout(layoutCollection(t), p)
			l.BreakLines(260)
			if got := lineTexts(l); !slices.Equal(got, base) {
				t.Errorf("style %d at scale %g: %q, want %q", ws, k, got, base)
			}
		}
	}
}
