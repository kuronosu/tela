package text

import "testing"

// anchorPara builds the texts with an anchor box between them, in a
// paragraph of direction dir.
func anchorPara(dir Direction, before, after string) *Paragraph {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st, Style: ParagraphStyle{Direction: dir}}
	p.AddText(before, st)
	p.AddBox(InlineBox{Anchor: true, Width: 50, Height: 50})
	p.AddText(after, st)
	return p
}

// An anchor resolves its bidi level from the text around it (a neutral,
// as Blink's U+FFFC) and sits where reordering puts it.
func TestAnchorBidi(t *testing.T) {
	for _, tc := range []struct {
		name          string
		dir           Direction
		before, after string
		rtl           bool
		// x is the edge of the cluster at offset at: its left edge, or
		// its right edge when right is set.
		at    int
		right bool
	}{
		// Latin in a right-to-left paragraph: left-to-right, after "XX ".
		{"ltr-text-rtl-para", DirectionRTL, "XX ", "YY", false, 3, false},
		// Between Latin and Arabic in a right-to-left paragraph: the
		// paragraph's direction, at the left edge of "abc".
		{"boundary-rtl-para", DirectionRTL, "abc", "مرحبا", true, 0, false},
		// The same in a left-to-right paragraph: after "abc".
		{"boundary-ltr-para", DirectionLTR, "abc", "مرحبا", false, 2, true},
		// At the start of a right-to-left paragraph before Latin text: the
		// paragraph's direction, at the line's right end.
		{"leading-rtl", DirectionRTL, "", "Leading", true, 6, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := NewLayout(layoutCollection(t), anchorPara(tc.dir, tc.before, tc.after))
			l.BreakLines(400)
			l.AlignTo(400)
			p, ok := l.AnchorPosition(0)
			if !ok {
				t.Fatal("no anchor position")
			}
			c := l.Cluster(l.ClusterAt(tc.at))
			want := c.X
			if tc.right {
				want += c.Advance
			}
			if tc.name == "leading-rtl" {
				want = 400
			}
			if p.RTL != tc.rtl || !near(p.X, want) {
				t.Errorf("anchor at %g rtl %v, want %g rtl %v", p.X, p.RTL, want, tc.rtl)
			}
		})
	}
}

// An anchor takes no space and is transparent to white-space collapsing
// and line breaking; one at the end of the text stays on the last line.
func TestAnchorLines(t *testing.T) {
	coll := layoutCollection(t)
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	plain := NewLayout(coll, &Paragraph{Default: st, Text: "aaaa bbbb"})
	l := NewLayout(coll, anchorPara(DirectionLTR, "aaaa ", " bbbb"))
	if !near(l.MaxContentWidth(), plain.MaxContentWidth()) {
		t.Errorf("max-content %g, want %g (one space, no box)", l.MaxContentWidth(), plain.MaxContentWidth())
	}
	l = NewLayout(coll, anchorPara(DirectionLTR, "aaaa", "bbbb"))
	l.BreakLines(10)
	if l.NumLines() != 1 {
		t.Errorf("%d lines, want 1: no break opportunity at an anchor", l.NumLines())
	}
	l = NewLayout(coll, anchorPara(DirectionRTL, "abc", ""))
	l.BreakLines(400)
	if l.NumLines() != 1 {
		t.Errorf("%d lines, want 1: an anchor at the end stays on its line", l.NumLines())
	}
	// Before a soft wrap, at the end of the line (after its trailing
	// space, which is removed).
	l = NewLayout(coll, anchorPara(DirectionLTR, "aaaa bbbb ", "cccc"))
	w := plain.MaxContentWidth()
	l.BreakLines(w + 1)
	p, _ := l.AnchorPosition(0)
	if l.NumLines() != 2 || p.Line != 0 || !near(p.X, w) {
		t.Errorf("anchor on line %d at %g (%d lines), want line 0 at %g", p.Line, p.X, l.NumLines(), w)
	}
	// At the start of an inline that wraps: on the next line, after the
	// inline's start edge, as Blink breaks before an open tag.
	q := &Paragraph{Default: st}
	q.AddText("aaaa bbbb ", st)
	k := q.OpenInline(Inline{Style: st, Left: 8, Right: 8})
	q.AddBox(InlineBox{Anchor: true})
	q.AddText("cccc", st)
	q.CloseInline(k)
	l = NewLayout(coll, q)
	l.BreakLines(w + 1)
	l.AlignTo(w + 1)
	p, _ = l.AnchorPosition(0)
	if p.Line != 1 || !near(p.X, 8) {
		t.Errorf("anchor on line %d at %g, want line 1 at 8", p.Line, p.X)
	}
}

// Arabic letters join across an anchor (they are shaped in the context of
// the text on its other side), as in Chromium.
func TestAnchorJoining(t *testing.T) {
	coll := layoutCollection(t)
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	plain := NewLayout(coll, &Paragraph{Default: st, Text: "بكم"})
	l := NewLayout(coll, anchorPara(DirectionLTR, "ب", "كم"))
	if !near(l.MaxContentWidth(), plain.MaxContentWidth()) {
		t.Errorf("width %g, want %g: the letters join across the anchor", l.MaxContentWidth(), plain.MaxContentWidth())
	}
}

// An inline offset moves the inline's fragments, the clusters, boxes and
// anchors it holds, carets and selection rectangles, not the lines.
func TestInlineOffset(t *testing.T) {
	st := TextStyle{Size: 16, LineHeight: LineHeightPixels(24)}
	p := &Paragraph{Default: st}
	p.AddText("Run ", st)
	k := p.OpenInline(Inline{Style: st, Left: 4, Right: 4})
	p.AddText("go", st)
	p.AddBox(InlineBox{Width: 10, Height: 10})
	p.AddBox(InlineBox{Anchor: true})
	p.CloseInline(k)
	p.AddText(" now", st)
	l := NewLayout(layoutCollection(t), p)
	l.BreakLines(400)
	frag0 := l.InlineFragments(nil)[0]
	box0, _ := l.BoxRect(0)
	anc0, _ := l.AnchorPosition(1)
	car0 := l.Caret(Cursor{Offset: 5})
	sel0 := l.SelectionRects(NewSelection(Cursor{Offset: 4}, Cursor{Offset: 6}), nil)
	now0 := l.Cluster(l.ClusterAt(7)).X

	l.SetInlineOffset(k, 12, 8)
	l.SetInlinePositioned(k, true)
	frag := l.InlineFragments(nil)[0]
	box, _ := l.BoxRect(0)
	anc, _ := l.AnchorPosition(1)
	car := l.Caret(Cursor{Offset: 5})
	sel := l.SelectionRects(NewSelection(Cursor{Offset: 4}, Cursor{Offset: 6}), nil)
	if !near(frag.X0, frag0.X0+12) || !near(frag.Baseline, frag0.Baseline+8) {
		t.Errorf("fragment %+v, want %+v moved by (12, 8)", frag, frag0)
	}
	if !near(box.X0, box0.X0+12) || !near(box.Y0, box0.Y0+8) {
		t.Errorf("box %v, want %v moved by (12, 8)", box, box0)
	}
	if !near(anc.X, anc0.X+12) || !near(anc.Top, anc0.Top+8) {
		t.Errorf("anchor %+v, want %+v moved by (12, 8)", anc, anc0)
	}
	if !near(car.X, car0.X+12) || !near(car.Top, car0.Top+8) {
		t.Errorf("caret %+v, want %+v moved by (12, 8)", car, car0)
	}
	if len(sel) != 1 || len(sel0) != 1 || !near(sel[0].X0, sel0[0].X0+12) || !near(sel[0].Y0, sel0[0].Y0+8) {
		t.Errorf("selection %v, want %v moved by (12, 8)", sel, sel0)
	}
	if now := l.Cluster(l.ClusterAt(7)).X; now != now0 {
		t.Errorf("the text after moved from %g to %g", now0, now)
	}
	if l.InlineLayer(k) != k || l.BoxLayer(0) != k || l.BoxLayer(1) != k {
		t.Errorf("layers %d %d %d, want %d", l.InlineLayer(k), l.BoxLayer(0), l.BoxLayer(1), k)
	}
}

// An anchor is no caret stop of its own: moving across "ab<anchor>cd" by
// characters takes four steps, as without it.
func TestAnchorCaretStops(t *testing.T) {
	l := NewLayout(layoutCollection(t), anchorPara(DirectionLTR, "ab", "cd"))
	l.BreakLines(400)
	for _, m := range []Movement{MoveForward, MoveRight} {
		s := CaretSelection(Cursor{Offset: 0})
		steps := 0
		for ; steps < 10 && s.Focus.Offset < 4; steps++ {
			s = l.Move(s, m, false)
		}
		if steps != 4 {
			t.Errorf("movement %v: %d steps to the end, want 4", m, steps)
		}
	}
}
