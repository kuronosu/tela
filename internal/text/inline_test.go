package text

import (
	"math"
	"testing"
)

// inlinePara builds "Run " + inline("go test") + " now" with a chip-like
// inline of 4 px edges.
func inlinePara(left, right float32) *Paragraph {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st}
	p.AddText("Run ", st)
	i := p.OpenInline(Inline{Style: st, Left: left, Right: right})
	p.AddText("go test", st)
	p.CloseInline(i)
	p.AddText(" now", st)
	return p
}

func TestInlineEdgesShiftText(t *testing.T) {
	plain := NewLayout(layoutCollection(t), inlinePara(0, 0))
	l := NewLayout(layoutCollection(t), inlinePara(6, 10))
	if d := l.MaxContentWidth() - plain.MaxContentWidth(); math.Abs(float64(d-16)) > 0.01 {
		t.Errorf("max-content grows by %g, want 16 (the two edges)", d)
	}
	frags := l.InlineFragments(nil)
	if len(frags) != 1 || !frags[0].Left || !frags[0].Right {
		t.Fatalf("fragments %+v, want one with both edges", frags)
	}
	// The first cluster of the inline starts after the left edge.
	g := l.Cluster(l.ClusterAt(4))
	if d := g.X - frags[0].X0; math.Abs(float64(d-6)) > 0.001 {
		t.Errorf("text starts %g after the fragment, want 6", d)
	}
	// " now" moves by both edges.
	n0, n1 := plain.Cluster(plain.ClusterAt(11)), l.Cluster(l.ClusterAt(11))
	if d := n1.X - n0.X; math.Abs(float64(d-16)) > 0.01 {
		t.Errorf("the text after moves by %g, want 16", d)
	}
	if l.InlineAt(l.ClusterAt(5)) != 0 || l.InlineAt(l.ClusterAt(0)) != -1 {
		t.Errorf("InlineAt: %d, %d", l.InlineAt(l.ClusterAt(5)), l.InlineAt(l.ClusterAt(0)))
	}
}

func TestInlineWrapSlices(t *testing.T) {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st}
	p.AddText("Some ", st)
	i := p.OpenInline(Inline{Style: st, Left: 8, Right: 8})
	p.AddText("decorated text that wraps over lines", st)
	p.CloseInline(i)
	l := NewLayout(layoutCollection(t), p)
	l.BreakLines(120)
	frags := l.InlineFragments(nil)
	if len(frags) < 3 {
		t.Fatalf("%d fragments, want one per line (3 or more)", len(frags))
	}
	for k, f := range frags {
		wantL, wantR := k == 0, k == len(frags)-1
		if f.Left != wantL || f.Right != wantR || f.Line != frags[0].Line+k {
			t.Errorf("fragment %d: %+v, want line %d, left %v, right %v", k, f, frags[0].Line+k, wantL, wantR)
		}
	}
	// RTL: the start edge is on the right of the first line.
	p.Style.Direction = DirectionRTL
	l = NewLayout(layoutCollection(t), p)
	l.BreakLines(120)
	frags = l.InlineFragments(nil)
	if !frags[0].Right || frags[0].Left || !frags[len(frags)-1].Left {
		t.Errorf("RTL fragments %+v: want the right edge first, the left edge last", frags)
	}
}

func TestInlineEmptyPlaceholder(t *testing.T) {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st}
	p.AddText("aaa", st)
	i := p.OpenInline(Inline{Style: st, Left: 5, Right: 5})
	p.CloseInline(i)
	p.AddText("bbb", st)
	l := NewLayout(layoutCollection(t), p)
	// No break opportunity: "aaabbb" stays one word.
	l.BreakLines(10)
	if l.NumLines() != 1 {
		t.Errorf("%d lines, want 1: an empty inline adds no break opportunity", l.NumLines())
	}
	frags := l.InlineFragments(nil)
	if len(frags) != 1 || frags[0].X1-frags[0].X0 != 10 {
		t.Errorf("fragments %+v, want one 10 px wide", frags)
	}
	if got := len(l.LineRuns(0, nil)); got < 2 {
		t.Errorf("%d runs", got)
	}
	for _, r := range l.LineRuns(0, nil) {
		if r.Kind == RunBox && r.Box != -1 {
			t.Errorf("the placeholder reports box %d, want -1", r.Box)
		}
	}
	if _, ok := l.BoxRect(0); ok {
		t.Error("BoxRect(0) reports the placeholder")
	}
}

func TestInlineEllipsisInside(t *testing.T) {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st, Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}}
	p.AddText("aaa ", st)
	i := p.OpenInline(Inline{Style: st, Left: 6, Right: 13})
	p.AddText("bbbbbbbbbbbbbbbbbb", st)
	p.CloseInline(i)
	l := NewLayout(layoutCollection(t), p)
	l.BreakLines(120)
	ln := l.Line(0)
	if !ln.Ellipsis {
		t.Fatal("no ellipsis")
	}
	frags := l.InlineFragments(nil)
	if len(frags) != 1 || !frags[0].Left || frags[0].Right {
		t.Fatalf("fragments %+v, want one without its end edge", frags)
	}
	// The fragment reaches the end of the ellipsis (Chromium paints the
	// inline's background under it).
	if d := frags[0].X1 - (ln.X + ln.Width); math.Abs(float64(d)) > 0.001 {
		t.Errorf("fragment ends at %g, the line at %g", frags[0].X1, ln.X+ln.Width)
	}
	if ln.Width > 120.001 {
		t.Errorf("line width %g > 120", ln.Width)
	}
}

func TestSetBoxIntrinsic(t *testing.T) {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st}
	p.AddText("a ", st)
	p.AddBox(InlineBox{Width: 10, Height: 10})
	p.AddText(" b", st)
	l := NewLayout(layoutCollection(t), p)
	max0 := l.MaxContentWidth()
	l.SetBoxSize(0, 50, 30, 5)
	l.SetBoxIntrinsic(0, 20, 80)
	l.BreakLines(float32(math.Inf(1)))
	if d := l.MaxContentWidth() - max0; math.Abs(float64(d-70)) > 0.01 {
		t.Errorf("max-content grows by %g, want 70 (max width 80)", d)
	}
	if l.MinContentWidth() != 20 {
		t.Errorf("min-content %g, want 20 (the box's min width)", l.MinContentWidth())
	}
	r, ok := l.BoxRect(0)
	if !ok || r.Width() != 50 || r.Height() != 30 {
		t.Errorf("box %v, want 50x30", r)
	}
	// Descent 5: the bottom is 5 px below the baseline.
	if d := r.Y1 - l.Line(0).Baseline; d != 5 {
		t.Errorf("box bottom %g below the baseline, want 5", d)
	}
}

// TestInlineCursorsWork checks that the editing geometry keeps working on
// a paragraph with edges, empty inlines, boxes and bidi text: every
// movement terminates and stays in the text, hit-testing returns valid
// cursors, selections have rectangles.
func TestInlineCursorsWork(t *testing.T) {
	st := TextStyle{Families: []string{"Inter", "Noto Sans Arabic"}, Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st}
	p.AddText("Run ", st)
	i := p.OpenInline(Inline{Style: st, Left: 6, Right: 6})
	p.AddText("go عربي test", st)
	j := p.OpenInline(Inline{Style: st, Left: 3, Right: 3})
	p.CloseInline(j)
	p.AddBox(InlineBox{Width: 12, Height: 12})
	p.CloseInline(i)
	p.AddText(" now and more words to wrap", st)
	l := NewLayout(layoutCollection(t), p)
	l.BreakLines(110)
	n := len(l.Text())
	for _, m := range []Movement{MoveLeft, MoveRight, MoveBackward, MoveForward, MoveWordLeft, MoveWordRight, MoveUp, MoveDown, MoveLineStart, MoveLineEnd} {
		s := CaretSelection(Cursor{})
		for k := 0; k < 4*n; k++ {
			s = l.Move(s, m, false)
			if s.Focus.Offset < 0 || s.Focus.Offset > n {
				t.Fatalf("movement %v left the text: %+v", m, s.Focus)
			}
		}
	}
	for y := float32(-5); y < l.Height()+5; y += 3 {
		for x := float32(-5); x < 120; x += 2.5 {
			c := l.HitTest(x, y)
			if c.Offset < 0 || c.Offset > n {
				t.Fatalf("HitTest(%g, %g) = %+v", x, y, c)
			}
			l.HitTestCluster(x, y)
		}
	}
	all := l.Move(CaretSelection(Cursor{}), MoveDocumentEnd, true)
	if rs := l.SelectionRects(all, nil); len(rs) == 0 {
		t.Error("no selection rectangles")
	}
}

func TestInlineEdgeCases(t *testing.T) {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	nan := float32(math.NaN())
	// Only an empty inline: one line, one fragment as wide as its edges.
	p := &Paragraph{Default: st}
	i := p.OpenInline(Inline{Style: st, Left: 3, Right: 4})
	j := p.OpenInline(Inline{Style: st, Left: nan, Right: float32(math.Inf(1))})
	p.CloseInline(j)
	p.CloseInline(i)
	l := NewLayout(layoutCollection(t), p)
	l.BreakLines(100)
	if l.NumLines() != 1 || l.Height() != 24 {
		t.Errorf("%d lines, height %g", l.NumLines(), l.Height())
	}
	frags := l.InlineFragments(nil)
	if len(frags) != 2 || frags[0].X1-frags[0].X0 != 7 || frags[1].X1 != frags[1].X0 {
		t.Errorf("fragments %+v: want the outer 7 px wide, the inner empty (non-finite edges are 0)", frags)
	}
	// Zero, infinite and invalid widths, an inline at the end.
	p = &Paragraph{Default: st}
	p.AddText("word ", st)
	i = p.OpenInline(Inline{Style: st, Left: 5, Right: 5})
	p.AddText("end", st)
	p.CloseInline(i)
	l = NewLayout(layoutCollection(t), p)
	for _, w := range []float32{0, float32(math.Inf(1)), nan, -5} {
		l.BreakLines(w)
		if f := l.InlineFragments(nil); len(f) != 1 || !f[0].Left || !f[0].Right {
			t.Errorf("width %g: fragments %+v", w, f)
		}
	}
	if got := l.MinContentWidth(); got < 10 {
		t.Errorf("min-content %g, want at least the edges", got)
	}
}

func TestInlineValidation(t *testing.T) {
	st := TextStyle{Size: 16}
	for _, in := range [][]Inline{
		{{Start: 0, End: 9}},                               // past the text
		{{Start: 2, End: 1}},                               // reversed
		{{Start: 0, End: 3}, {Start: 1, End: 4}},           // overlapping siblings
		{{Start: 0, End: 2}, {Start: 1, End: 3, Depth: 1}}, // a child outside its parent
		{{Start: 0, End: 2, Depth: 1}},                     // no parent
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("no panic for %+v", in)
				}
			}()
			NewLayout(layoutCollection(t), &Paragraph{Text: "abcd", Default: st, Inlines: in})
		}()
	}
}
