package text

import (
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// spans returns the x intervals of rects on line li of l.
func spansOn(l *Layout, rects []gfx.Rect, li int) [][2]float32 {
	ln := l.Line(li)
	var out [][2]float32
	for _, r := range rects {
		if r.Y0 == ln.Top {
			out = append(out, [2]float32{r.X0, r.X1})
		}
	}
	return out
}

func nearSpans(a, b [][2]float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !near(a[i][0], b[i][0]) || !near(a[i][1], b[i][1]) {
			return false
		}
	}
	return true
}

// TestSelectionRects checks selection rectangles: per line, split by bidi
// runs, line-break extensions, ligature parts, inline boxes, ellipses.
func TestSelectionRects(t *testing.T) {
	// Three wrapped lines: the first two are extended by a space.
	l := simple(t, "hello world foo", ParagraphStyle{}, inter16, 60)
	space := spaceAdvance(l.sinfo[0].face, 16)
	rects := l.SelectionRects(NewSelection(cur(3), cur(13)), nil)
	x3, x13 := l.Caret(cur(3)).X, l.Caret(cur(13)).X
	l0, l1 := l.Line(0), l.Line(1)
	for li, want := range [][][2]float32{
		{{x3, l0.Width + space}},
		{{0, l1.Width + space}},
		{{0, x13}},
	} {
		if got := spansOn(l, rects, li); !nearSpans(got, want) {
			t.Errorf("wrapped line %d: %v, want %v", li, got, want)
		}
	}
	for _, r := range rects {
		if r.Height() != 24 {
			t.Errorf("rect %v is not as tall as the line", r)
		}
	}
	// Anchor after focus: the same rects.
	if back := l.SelectionRects(NewSelection(cur(13), cur(3)), nil); len(back) != len(rects) || back[0] != rects[0] {
		t.Errorf("reversed selection: %v, want %v", back, rects)
	}
	// Ending at the end of a line (Upstream) does not extend it; ending at
	// the start of the next line (Downstream) does.
	l = simple(t, "hello   world", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 60)
	if got := spansOn(l, l.SelectionRects(NewSelection(cur(0), up(8)), nil), 0); len(got) != 1 || !near(got[0][1], l.Line(0).Width+l.Line(0).Hanging) {
		t.Errorf("ending at the line end: %v", got)
	}
	if got := spansOn(l, l.SelectionRects(NewSelection(cur(0), cur(8)), nil), 0); len(got) != 1 || !near(got[0][1], l.Line(0).Width+l.Line(0).Hanging+space) {
		t.Errorf("ending at the next line's start: %v", got)
	}

	// A logical range across a bidi boundary is two visual rects.
	l = simple(t, "ab مرحبا cd", ParagraphStyle{}, inter16, inf32)
	rects = l.SelectionRects(NewSelection(cur(1), cur(9)), nil) // "b مرح"
	b, sp, meem, hah := l.Cluster(1), l.Cluster(2), l.Cluster(3), l.Cluster(5)
	want := [][2]float32{{b.X, sp.X + sp.Advance}, {hah.X, meem.X + meem.Advance}}
	if got := spansOn(l, rects, 0); !nearSpans(got, want) {
		t.Errorf("bidi selection %v, want %v", got, want)
	}

	// Right to left: the line-break extension is on the left.
	l = simple(t, arabicText, ParagraphStyle{Direction: DirectionRTL}, inter16, 200)
	rects = l.SelectionRects(NewSelection(cur(0), cur(len(arabicText))), nil)
	first := spansOn(l, rects, 0)
	ln := l.Line(0)
	if len(first) != 1 || !near(first[0][0], ln.X-spaceAdvance(l.sinfo[0].face, 16)) || !near(first[0][1], ln.X+ln.Width) {
		t.Errorf("RTL first line %v, line %+v", first, ln)
	}

	// Part of a ligature: by grapheme.
	l = simple(t, "a->b", ParagraphStyle{}, inter16, inf32)
	lig, bb := l.Cluster(1), l.Cluster(2)
	if got := spansOn(l, l.SelectionRects(NewSelection(cur(2), cur(4)), nil), 0); !nearSpans(got, [][2]float32{{lig.X + lig.Advance/2, bb.X + bb.Advance}}) {
		t.Errorf("half a ligature: %v", got)
	}

	// Inline boxes: the box alone, between its two cursors.
	p := &Paragraph{Default: inter16}
	p.AddText("ab", inter16)
	p.AddBox(InlineBox{Width: 20, Height: 10})
	p.AddText("cd", inter16)
	l = NewLayout(layoutCollection(t), p)
	box, _ := l.BoxRect(0)
	if got := spansOn(l, l.SelectionRects(NewSelection(cur(2), up(2)), nil), 0); !nearSpans(got, [][2]float32{{box.X0, box.X1}}) {
		t.Errorf("box selection %v, box %v", got, box)
	}
	if got := l.SelectionRects(NewSelection(cur(2), cur(3)), nil); len(got) != 1 || !near(got[0].X0, box.X1) {
		t.Errorf("selection after the box: %v", got)
	}
	if s, e := NewSelection(cur(2), up(2)).TextRange(); s != 2 || e != 2 {
		t.Errorf("box selection text range [%d, %d)", s, e)
	}

	// Hidden text: the ellipsis is highlighted.
	l = simple(t, "Hello wonderful world", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 60)
	runs := l.LineRuns(0, nil)
	ell := runs[len(runs)-1]
	got := spansOn(l, l.SelectionRects(NewSelection(cur(3), cur(10)), nil), 0)
	if len(got) != 1 || !near(got[0][0], l.Caret(cur(3)).X) || !near(got[0][1], ell.X+ell.Width) {
		t.Errorf("selection into hidden text %v, ellipsis %+v", got, ell)
	}
	if got := spansOn(l, l.SelectionRects(NewSelection(cur(0), cur(3)), nil), 0); len(got) != 1 || got[0][1] > ell.X {
		t.Errorf("visible selection %v highlights the ellipsis", got)
	}

	// A forced break is extended; the last line is not.
	l = simple(t, "ab\ncd", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, inf32)
	rects = l.SelectionRects(NewSelection(cur(1), cur(5)), nil)
	if got := spansOn(l, rects, 0); len(got) != 1 || !near(got[0][1], l.Line(0).Width+space) {
		t.Errorf("forced break: %v", got)
	}
	if got := spansOn(l, rects, 1); len(got) != 1 || !near(got[0][1], l.Line(1).Width) {
		t.Errorf("last line: %v", got)
	}
	// Collapsed selections have no rects.
	if got := l.SelectionRects(CaretSelection(cur(2)), nil); len(got) != 0 {
		t.Errorf("caret selection: %v", got)
	}
}

// TestSelectionRectsProperty checks, over the corpus, that selection
// rectangles are ordered, disjoint, inside their line boxes, and cover
// exactly the selected content: the middle of every selected grapheme
// cluster (of text or an inline box), and of the ellipsis when hidden text
// is selected, is inside a rectangle; the middle of every other one is
// not; and without line-break extensions the widths add up to the
// selected advances.
func TestSelectionRectsProperty(t *testing.T) {
	for _, nl := range cursorCorpus(t) {
		l := nl.l
		ps := positions(l)
		var sels []Selection
		sels = append(sels, l.SelectAll())
		seed := uint32(len(l.Text()) + 7)
		for range 40 {
			seed = seed*1664525 + 1013904223
			a := ps[int(seed>>8)%len(ps)]
			seed = seed*1664525 + 1013904223
			b := ps[int(seed>>8)%len(ps)]
			sels = append(sels, NewSelection(a, b))
		}
		for _, s := range sels {
			checkSelection(t, nl.name, l, s)
		}
	}
}

func checkSelection(t *testing.T, name string, l *Layout, s Selection) {
	t.Helper()
	rects := l.SelectionRects(s, nil)
	plain := l.selectionRects(s, nil, false)
	a, b := l.ordered(s)
	// Order, line boxes and disjointness.
	for i, r := range rects {
		li := l.lineAtY((r.Y0 + r.Y1) / 2)
		ln := l.Line(li)
		if r.Y0 != ln.Top || r.Y1 != ln.Top+ln.Height || !(r.X1 > r.X0) {
			t.Errorf("%s %v: rect %v outside line %d %+v", name, s, r, li, ln)
		}
		brk := l.breakWidth(li)
		if r.X0 < min(ln.X, 0)-brk-0.01 || r.X1 > max(ln.X+ln.Width+ln.Hanging, l.AlignWidth())+brk+0.01 {
			t.Errorf("%s %v: rect %v beyond line %d %+v", name, s, r, li, ln)
		}
		if i > 0 {
			p := rects[i-1]
			if p.Y0 > r.Y0 || (p.Y0 == r.Y0 && p.X1 > r.X0+0.001) {
				t.Errorf("%s %v: rects %v and %v overlap or are out of order", name, s, p, r)
			}
		}
	}
	inside := func(x, y float32) bool {
		for _, r := range plain {
			if x > r.X0 && x < r.X1 && y > r.Y0 && y < r.Y1 {
				return true
			}
		}
		return false
	}
	for li := range l.NumLines() {
		ln := l.Line(li)
		y := ln.Top + ln.Height/2
		var want, got float32
		for _, el := range l.lineElems(li, nil) {
			w := el.x1 - el.x0
			if !(w > 0) {
				continue
			}
			var sel []bool
			switch {
			case el.ci == ciEllipsis:
				sel = []bool{l.selectsHidden(li, a, b)}
			case el.box:
				k := l.clusterText(el.ci)
				sel = []bool{(a.Offset < k || (a.Offset == k && a.Affinity == Upstream)) && (b.Offset > k || (b.Offset == k && b.Affinity == Downstream))}
			default:
				cs, ce := l.clusterRange(el.ci)
				for j := int32(0); j < el.g; j++ {
					lo, hi := cs, ce
					if j > 0 {
						lo = l.graphemeOffset(el.ci, j)
					}
					if j+1 < el.g {
						hi = l.graphemeOffset(el.ci, j+1)
					}
					sel = append(sel, a.Offset <= lo && hi <= b.Offset)
				}
				if el.rtl {
					for i, j := 0, len(sel)-1; i < j; i, j = i+1, j-1 {
						sel[i], sel[j] = sel[j], sel[i]
					}
				}
			}
			for k, on := range sel {
				x0, x1 := el.stopX(int32(k)), el.stopX(int32(k+1))
				if on {
					want += x1 - x0
				}
				if mid := (x0 + x1) / 2; inside(mid, y) != on {
					t.Errorf("%s %v: line %d element %+v part %d selected %v, covered %v", name, s, li, el, k, on, !on)
				}
			}
		}
		for _, r := range plain {
			if r.Y0 == ln.Top {
				got += r.X1 - r.X0
			}
		}
		if math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("%s %v: line %d rect widths %g, selected advances %g", name, s, li, got, want)
		}
	}
}

// TestSelectedText checks TextRange and SelectedText.
func TestSelectedText(t *testing.T) {
	l := simple(t, "  a   b  ", ParagraphStyle{}, inter16, inf32)
	s := NewSelection(cur(9), cur(4)) // snaps to 6
	if got := l.SelectedText(s); got != "b  " {
		t.Errorf("SelectedText %q", got)
	}
	if a, b := s.TextRange(); a != 4 || b != 9 {
		t.Errorf("TextRange [%d, %d)", a, b)
	}
	if got := l.SelectedText(l.SelectAll()); got != "  a   b  " {
		t.Errorf("select all %q", got)
	}
	if !CaretSelection(cur(2)).IsCollapsed() || NewSelection(up(2), cur(2)).IsCollapsed() {
		t.Error("IsCollapsed")
	}
}
