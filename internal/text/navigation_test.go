package text

import (
	"slices"
	"testing"
)

// walk applies m repeatedly from c until the focus stops moving and
// returns the focuses.
func walk(l *Layout, c Cursor, m Movement) []Cursor {
	var out []Cursor
	s := CaretSelection(l.SnapCursor(c))
	for range 4*len(l.Text()) + 8 {
		n := l.Move(s, m, false)
		if n.Focus == s.Focus {
			break
		}
		s = n
		out = append(out, s.Focus)
	}
	return out
}

func eqCursors(t *testing.T, what string, got, want []Cursor) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n got %v\nwant %v", what, got, want)
	}
}

// TestMoveVisual checks arrow keys in visual order: each step crosses one
// cluster on screen, including through bidi runs, soft wraps and boxes.
func TestMoveVisual(t *testing.T) {
	// "ab مرحبا cd": the Arabic word (bytes 3 to 13) is drawn reversed.
	l := simple(t, "ab مرحبا cd", ParagraphStyle{}, inter16, inf32)
	eqCursors(t, "right through a bidi run", walk(l, cur(0), MoveRight),
		[]Cursor{cur(1), cur(2), up(3), cur(11), cur(9), cur(7), cur(5), cur(3), cur(14), cur(15), cur(16)})
	eqCursors(t, "left through a bidi run", walk(l, cur(16), MoveLeft),
		[]Cursor{cur(15), cur(14), cur(13), cur(5), cur(7), cur(9), cur(11), up(13), cur(2), cur(1), cur(0)})
	// Every step moves the caret on screen, in the right direction.
	prev := l.Caret(cur(0)).X
	for _, c := range walk(l, cur(0), MoveRight) {
		if x := l.Caret(c).X; x <= prev {
			t.Errorf("MoveRight to %v did not move right: %g after %g", c, x, prev)
		}
		prev = l.Caret(c).X
	}
	// Right to left paragraph: left moves forward.
	l = simple(t, "مرحبا abc", ParagraphStyle{Direction: DirectionRTL}, inter16, inf32)
	eqCursors(t, "RTL left", walk(l, cur(0), MoveLeft),
		[]Cursor{cur(2), cur(4), cur(6), cur(8), cur(10), up(11), cur(13), cur(12), cur(11)})
	if got := walk(l, cur(0), MoveRight); len(got) != 0 {
		t.Errorf("MoveRight at the start of an RTL paragraph moved: %v", got)
	}
	// A soft wrap after a collapsed space: one step to the next line.
	l = simple(t, "hello world", ParagraphStyle{}, inter16, 60)
	eqCursors(t, "soft wrap", walk(l, cur(3), MoveRight), []Cursor{cur(4), cur(5), cur(6), cur(7), cur(8), cur(9), cur(10), cur(11)})
	eqCursors(t, "soft wrap back", walk(l, cur(8), MoveLeft), []Cursor{cur(7), cur(6), cur(5), cur(4), cur(3), cur(2), cur(1), cur(0)})
	// Hanging spaces: the end of the line and the start of the next one
	// are two stops.
	l = simple(t, "ab  cd", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 20)
	eqCursors(t, "hanging spaces", walk(l, cur(1), MoveRight), []Cursor{cur(2), cur(3), up(4), cur(4), cur(5), cur(6)})
	// Inline boxes are a step of their own.
	p := &Paragraph{Default: inter16}
	p.AddText("ab", inter16)
	p.AddBox(InlineBox{Width: 20, Height: 10})
	p.AddText("cd", inter16)
	l = NewLayout(layoutCollection(t), p)
	eqCursors(t, "box", walk(l, cur(0), MoveRight), []Cursor{cur(1), up(2), cur(2), cur(3), cur(4)})
	// Truncated text: the ellipsis is one step.
	l = simple(t, "Hello wonderful world", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 60)
	eqCursors(t, "ellipsis", walk(l, cur(4), MoveRight), []Cursor{cur(5), up(6), cur(21)})
	eqCursors(t, "ellipsis back", walk(l, cur(21), MoveLeft), []Cursor{up(6), cur(5), cur(4), cur(3), cur(2), cur(1), cur(0)})
}

// TestVisualRoundTrip checks, over the corpus layouts of one direction,
// that MoveRight then MoveLeft (and MoveLeft then MoveRight) returns to
// the starting cursor whenever the first move moved.
func TestVisualRoundTrip(t *testing.T) {
	pure := map[string]bool{
		"latin": true, "latin-justify": true, "latin-center": true, "arabic": true, "arabic-justify": true,
		"cjk": true, "ligature": true, "pre-wrap": true, "pre-wrap-rtl": true,
		"collapse": true, "pre-line": true, "tabs": true, "letter-spacing": true, "narrow": true,
		"trailing-newline": true, "newlines": true, "empty": true, "empty-rtl": true,
	}
	for _, nl := range cursorCorpus(t) {
		if !pure[nl.name] {
			continue
		}
		l := nl.l
		for _, c := range positions(l) {
			if ci := l.ClusterAt(c.Offset - 1); c.Offset > 0 && l.removedSpace(int32(ci)) {
				continue // not a stop: after white space removed at a line end
			}
			for _, m := range [][2]Movement{{MoveRight, MoveLeft}, {MoveLeft, MoveRight}} {
				s := l.Move(CaretSelection(c), m[0], false)
				if s.Focus == c {
					continue
				}
				if back := l.Move(s, m[1], false).Focus; back != c {
					t.Errorf("%s: %v -> %v -> %v", nl.name, c, s.Focus, back)
				}
			}
		}
	}
}

// TestMoveLogical checks MoveForward and MoveBackward.
func TestMoveLogical(t *testing.T) {
	l := simple(t, "ab مرحبا", ParagraphStyle{}, inter16, inf32)
	eqCursors(t, "forward", walk(l, cur(0), MoveForward), []Cursor{cur(1), cur(2), up(3), cur(5), cur(7), cur(9), cur(11), cur(13)})
	eqCursors(t, "backward", walk(l, cur(13), MoveBackward), []Cursor{cur(11), cur(9), cur(7), cur(5), cur(3), cur(2), cur(1), cur(0)})
	// One step per grapheme cluster: emoji (ZWJ sequence, flag, skin tone),
	// the graphemes of a ligature, combining marks.
	l = simple(t, "a👨‍👩‍👧‍👦e\u0301->🇯🇵👍🏽", ParagraphStyle{}, inter16, inf32)
	eqCursors(t, "graphemes", walk(l, cur(0), MoveForward), []Cursor{cur(1), cur(26), cur(29), cur(30), cur(31), cur(39), cur(47)})
	eqCursors(t, "graphemes visually", walk(l, cur(0), MoveRight), walk(l, cur(0), MoveForward))
	// Boxes are steps; positions between two boxes at one offset are not.
	p := &Paragraph{Default: inter16}
	p.AddText("a", inter16)
	p.AddBox(InlineBox{Width: 10, Height: 10})
	p.AddBox(InlineBox{Width: 10, Height: 10})
	p.AddText("b", inter16)
	l = NewLayout(layoutCollection(t), p)
	eqCursors(t, "boxes forward", walk(l, cur(0), MoveForward), []Cursor{up(1), cur(1), cur(2)})
	eqCursors(t, "boxes backward", walk(l, cur(2), MoveBackward), []Cursor{cur(1), up(1), cur(0)})
	// Hanging spaces at a soft wrap: forward stays at the end of the line
	// (Upstream), as in Chromium.
	l = simple(t, "ab  cd", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 20)
	eqCursors(t, "hanging forward", walk(l, cur(2), MoveForward), []Cursor{cur(3), up(4), cur(5), cur(6)})
	// A collapsed space at the end of the text is not a stop.
	l = simple(t, "ab ", ParagraphStyle{}, inter16, inf32)
	eqCursors(t, "final collapsed space", walk(l, cur(0), MoveForward), []Cursor{cur(1), cur(2)})
	eqCursors(t, "final collapsed space back", walk(l, cur(3), MoveBackward), []Cursor{cur(1), cur(0)})
	// Hidden text is skipped.
	l = simple(t, "Hello wonderful world", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 60)
	eqCursors(t, "hidden", walk(l, cur(5), MoveForward), []Cursor{up(6), cur(21)})
	eqCursors(t, "hidden back", walk(l, cur(21), MoveBackward), []Cursor{up(6), cur(5), cur(4), cur(3), cur(2), cur(1), cur(0)})
}

// TestMoveWord checks word movement: UAX #29 words, skipping white space
// and punctuation.
func TestMoveWord(t *testing.T) {
	text := "foo, bar.baz qux  42 x"
	l := simple(t, text, ParagraphStyle{}, inter16, inf32)
	eqCursors(t, "word forward", walk(l, cur(0), MoveWordForward), []Cursor{cur(3), cur(12), cur(16), cur(20), cur(22)})
	eqCursors(t, "word backward", walk(l, cur(22), MoveWordBackward), []Cursor{cur(21), cur(18), cur(13), cur(5), cur(0)})
	eqCursors(t, "next word start", walk(l, cur(0), MoveNextWordStart), []Cursor{cur(5), cur(13), cur(18), cur(21), cur(22)})
	eqCursors(t, "word right (LTR)", walk(l, cur(0), MoveWordRight), walk(l, cur(0), MoveWordForward))
	eqCursors(t, "word left (LTR)", walk(l, cur(22), MoveWordLeft), walk(l, cur(22), MoveWordBackward))
	// Inside a word: to its end or start.
	if got := l.Move(CaretSelection(cur(6)), MoveWordForward, false).Focus; got != cur(12) {
		t.Errorf("forward from inside a word: %v", got)
	}
	if got := l.Move(CaretSelection(cur(7)), MoveWordBackward, false).Focus; got != cur(5) {
		t.Errorf("backward from inside a word: %v", got)
	}
	// Contractions, numbers with separators, CJK (one ideograph each).
	l = simple(t, "don't 3.14 日本語", ParagraphStyle{}, inter16, inf32)
	eqCursors(t, "word forward (UAX #29)", walk(l, cur(0), MoveWordForward), []Cursor{cur(5), cur(10), cur(14), cur(17), cur(20)})
	// Right to left: word left goes forward.
	l = simple(t, "مرحبا بكم في", ParagraphStyle{Direction: DirectionRTL}, inter16, inf32)
	eqCursors(t, "RTL word left", walk(l, cur(0), MoveWordLeft), walk(l, cur(0), MoveWordForward))
	eqCursors(t, "RTL word right", walk(l, cur(len(l.Text())), MoveWordRight), walk(l, cur(len(l.Text())), MoveWordBackward))
	if got := walk(l, cur(0), MoveWordLeft); len(got) != 3 || got[2] != cur(len(l.Text())) {
		t.Errorf("RTL words: %v", got)
	}
}

// TestMoveLineStartEnd checks Home and End.
func TestMoveLineStartEnd(t *testing.T) {
	move := func(l *Layout, c Cursor, m Movement) Cursor { return l.Move(CaretSelection(c), m, false).Focus }
	// Soft wrap after a collapsed space: End is before the space.
	l := simple(t, "hello world foo", ParagraphStyle{}, inter16, 60)
	for _, c := range []struct {
		from Cursor
		m    Movement
		want Cursor
	}{
		{cur(2), MoveLineEnd, cur(5)},
		{cur(2), MoveLineStart, cur(0)},
		{cur(8), MoveLineStart, cur(6)},
		{cur(8), MoveLineEnd, cur(11)},
		{cur(13), MoveLineEnd, cur(15)},
		{cur(6), MoveLineEnd, cur(11)},
	} {
		if got := move(l, c.from, c.m); got != c.want {
			t.Errorf("collapsed: %v from %v = %v, want %v", c.m, c.from, got, c.want)
		}
	}
	// Hanging spaces: End is after them, at the end of the line.
	l = simple(t, "ab  cd", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 20)
	if got := move(l, cur(1), MoveLineEnd); got != up(4) {
		t.Errorf("hanging End: %v", got)
	}
	if got := move(l, up(4), MoveLineStart); got != cur(0) {
		t.Errorf("Home from the end of a line: %v", got)
	}
	// A forced break: End is before it.
	l = simple(t, "ab\ncd", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, inf32)
	if got := move(l, cur(0), MoveLineEnd); got != cur(2) {
		t.Errorf("End before a forced break: %v", got)
	}
	if got := move(l, cur(5), MoveLineStart); got != cur(3) {
		t.Errorf("Home after a forced break: %v", got)
	}
	// Right to left: logical ends of the line.
	l = simple(t, arabicText, ParagraphStyle{Direction: DirectionRTL}, inter16, 200)
	ln := l.Line(0)
	if got := move(l, cur(4), MoveLineStart); got != cur(0) {
		t.Errorf("RTL Home: %v", got)
	}
	if got := move(l, cur(4), MoveLineEnd); l.Caret(got).Line != 0 || !near(l.Caret(got).X, ln.X) {
		t.Errorf("RTL End: %v at %+v, line %+v", got, l.Caret(got), ln)
	}
}

// TestMoveVertical checks MoveUp and MoveDown with a goal column, pages and
// the ends of the text.
func TestMoveVertical(t *testing.T) {
	text := "A long first line here\nshort\nAnother long line here\nend"
	l := simple(t, text, ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, inf32)
	start := cur(15) // in "line" on the first line
	x := l.Caret(start).X
	s := CaretSelection(start)
	s = l.Move(s, MoveDown, false)
	if s.Focus != cur(28) { // the end of "short"
		t.Errorf("down to a short line: %v", s.Focus)
	}
	if g, ok := s.GoalX(); !ok || g != x {
		t.Errorf("goal %g %v, want %g", g, ok, x)
	}
	s = l.Move(s, MoveDown, false)
	if ca := l.Caret(s.Focus); ca.Line != 2 || ca.X > x+8 || ca.X < x-8 {
		t.Errorf("down to a long line: %v at %+v, goal %g", s.Focus, ca, x)
	}
	s = l.Move(s, MoveDown, false)
	if s.Focus != cur(len(text)) {
		t.Errorf("down to a short last line: %v", s.Focus)
	}
	s = l.Move(s, MoveDown, false)
	if s.Focus != cur(len(text)) {
		t.Errorf("down from the last line: %v", s.Focus)
	}
	s = l.Move(s, MoveUp, false)
	if ca := l.Caret(s.Focus); ca.Line != 2 || ca.X > x+8 || ca.X < x-8 {
		t.Errorf("up keeps the goal: %v at %+v", s.Focus, ca)
	}
	s = l.Move(s, MoveLeft, false)
	if _, ok := s.GoalX(); ok {
		t.Error("horizontal movement kept the goal")
	}
	if got := l.Move(CaretSelection(cur(3)), MoveUp, false).Focus; got != cur(0) {
		t.Errorf("up from the first line: %v", got)
	}
	// Pages of two lines.
	s = l.MovePage(CaretSelection(cur(2)), 48, false)
	if ca := l.Caret(s.Focus); ca.Line != 2 {
		t.Errorf("page down: %v line %d", s.Focus, ca.Line)
	}
	// Less than a page left: the last line at the goal column, then the
	// end of the text (and the same upward).
	s = l.MovePage(s, 48, false)
	if ca := l.Caret(s.Focus); ca.Line != 3 || s.Focus == cur(len(text)) {
		t.Errorf("page down to the last line: %v line %d", s.Focus, ca.Line)
	}
	s = l.MovePage(s, 48, false)
	if s.Focus != cur(len(text)) {
		t.Errorf("page down from the last line: %v", s.Focus)
	}
	s = l.MovePage(CaretSelection(cur(40)), -1000, false)
	if ca := l.Caret(s.Focus); ca.Line != 0 || s.Focus == cur(0) {
		t.Errorf("page up to the first line: %v line %d", s.Focus, ca.Line)
	}
	s = l.MovePage(s, -1000, false)
	if s.Focus != cur(0) {
		t.Errorf("page up from the first line: %v", s.Focus)
	}
	// A page shorter than a line still moves one line.
	if s = l.MovePage(CaretSelection(cur(2)), 5, false); l.Caret(s.Focus).Line != 1 {
		t.Errorf("short page: %v", s.Focus)
	}
	if got := l.Move(CaretSelection(cur(10)), MoveDocumentEnd, false).Focus; got != cur(len(text)) {
		t.Errorf("document end: %v", got)
	}
	if got := l.Move(CaretSelection(cur(10)), MoveDocumentStart, false).Focus; got != cur(0) {
		t.Errorf("document start: %v", got)
	}
	// A line that holds only an inline box between two wraps is skipped.
	p := &Paragraph{Default: inter16}
	p.AddText("ab", inter16)
	p.AddBox(InlineBox{Width: 20, Height: 10})
	p.AddText("cd", inter16)
	l = NewLayout(layoutCollection(t), p)
	l.BreakLines(30)
	if got := l.Move(CaretSelection(cur(1)), MoveDown, false).Focus; l.Caret(got).Line != 2 {
		t.Errorf("down over a box line: %v on line %d", got, l.Caret(got).Line)
	}
}

// TestMoveExtend checks shift+movement and collapsing a selection.
func TestMoveExtend(t *testing.T) {
	l := simple(t, "hello world", ParagraphStyle{}, inter16, inf32)
	s := CaretSelection(cur(2))
	for range 3 {
		s = l.Move(s, MoveRight, true)
	}
	if s.Anchor != cur(2) || s.Focus != cur(5) || l.SelectedText(s) != "llo" {
		t.Errorf("shift+right: %+v %q", s, l.SelectedText(s))
	}
	s = l.Move(s, MoveWordForward, true)
	if s.Anchor != cur(2) || s.Focus != cur(11) {
		t.Errorf("shift+word: %+v", s)
	}
	// Left and right without shift collapse to the selection's ends.
	if got := l.Move(s, MoveLeft, false); got != CaretSelection(cur(2)) {
		t.Errorf("collapse left: %+v", got)
	}
	if got := l.Move(s, MoveRight, false); got != CaretSelection(cur(11)) {
		t.Errorf("collapse right: %+v", got)
	}
	if got := l.Move(NewSelection(cur(11), cur(2)), MoveForward, false); got != CaretSelection(cur(11)) {
		t.Errorf("collapse forward: %+v", got)
	}
	// In a right-to-left paragraph, left collapses to the end.
	l = simple(t, "مرحبا بكم", ParagraphStyle{Direction: DirectionRTL}, inter16, inf32)
	if got := l.Move(NewSelection(cur(0), cur(10)), MoveLeft, false); got != CaretSelection(cur(10)) {
		t.Errorf("RTL collapse left: %+v", got)
	}
	// Shift+down keeps the anchor and the goal.
	l = simple(t, "one\ntwo\nthree", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, inf32)
	s = l.Move(CaretSelection(cur(1)), MoveDown, true)
	s = l.Move(s, MoveDown, true)
	if s.Anchor != cur(1) || l.Caret(s.Focus).Line != 2 {
		t.Errorf("shift+down: %+v", s)
	}
	if _, ok := s.GoalX(); !ok {
		t.Error("shift+down dropped the goal")
	}
}

// TestSelectAtPoint checks double and triple clicks and drags by unit.
func TestSelectAtPoint(t *testing.T) {
	l := simple(t, "don't stop, please   now", ParagraphStyle{}, inter16, inf32)
	at := func(off int) float32 { return l.Caret(cur(off)).X + 1 }
	for _, c := range []struct {
		x    float32
		want string
	}{
		{at(1), "don't"},
		{at(4), "don't"},
		{at(6), "stop"},
		{at(10), ","},
		{at(11), " "},
		{at(18), "   "}, // collapsed to one space
		{at(22), "now"},
		{1000, "now"},
		{-5, "don't"},
	} {
		s := l.SelectWordAt(c.x, 12)
		if got := l.SelectedText(s); got != c.want {
			t.Errorf("SelectWordAt(%g) = %q (%+v), want %q", c.x, got, s, c.want)
		}
	}
	// Dragging a word selection extends by words.
	s := l.SelectWordAt(at(7), 12) // "stop"
	s = l.ExtendSelection(s, at(13), 12)
	if got := l.SelectedText(s); got != "stop, please" {
		t.Errorf("word drag forward: %q", got)
	}
	s = l.ExtendSelection(s, at(1), 12)
	if got := l.SelectedText(s); got != "don't stop" || s.Anchor != cur(10) {
		t.Errorf("word drag backward: %q %+v", got, s)
	}
	// A plain drag moves the focus to the caret position.
	s = l.ExtendSelection(CaretSelection(cur(1)), at(3)+2, 12)
	if s.Anchor != cur(1) || s.Focus != cur(4) {
		t.Errorf("drag: %+v", s)
	}
	// Lines and paragraphs.
	l = simple(t, "first paragraph wraps here\nsecond\n", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 100)
	if got := l.SelectedText(l.SelectLineAt(5, 12)); got != "first " {
		t.Errorf("SelectLineAt: %q", got)
	}
	if got := l.SelectedText(l.SelectParagraphAt(5, 30)); got != "first paragraph wraps here\n" {
		t.Errorf("SelectParagraphAt: %q", got)
	}
	last := l.Line(l.NumLines() - 2)
	if got := l.SelectedText(l.SelectParagraphAt(5, last.Top+5)); got != "second\n" {
		t.Errorf("second paragraph: %q", got)
	}
	s = l.SelectParagraphAt(5, 12)
	s = l.ExtendSelection(s, 5, last.Top+5)
	if got := l.SelectedText(s); got != "first paragraph wraps here\nsecond\n" {
		t.Errorf("paragraph drag: %q", got)
	}
	// An inline box: the box.
	p := &Paragraph{Default: inter16}
	p.AddText("ab ", inter16)
	p.AddBox(InlineBox{Width: 20, Height: 10})
	p.AddText(" cd", inter16)
	l = NewLayout(layoutCollection(t), p)
	box, _ := l.BoxRect(0)
	if s := l.SelectWordAt(box.X0+5, 12); s.Anchor != up(3) || s.Focus != cur(3) {
		t.Errorf("box word: %+v", s)
	}
	// The ellipsis: the hidden text.
	l = simple(t, "Hello wonderful world", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, inter16, 60)
	runs := l.LineRuns(0, nil)
	ell := runs[len(runs)-1]
	if got := l.SelectedText(l.SelectWordAt(ell.X+2, 12)); got != "wonderful world" {
		t.Errorf("ellipsis word: %q", got)
	}
	// Empty text.
	l = simple(t, "", ParagraphStyle{}, inter16, 100)
	if s := l.SelectWordAt(10, 10); !s.IsCollapsed() {
		t.Errorf("empty word: %+v", s)
	}
}
