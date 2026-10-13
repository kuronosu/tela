package edit

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// walk applies cmd until the focus stops moving and returns the focuses.
func walk(d *driver, cmd Command) []text.Cursor {
	var out []text.Cursor
	for range 4*len(d.e.Display()) + 8 {
		before := d.e.Selection().Focus
		d.do(cmd)
		f := d.e.Selection().Focus
		if f == before {
			break
		}
		out = append(out, f)
	}
	return out
}

func cur(o int) text.Cursor { return text.Cursor{Offset: o} }
func up(o int) text.Cursor  { return text.Cursor{Offset: o, Affinity: text.Upstream} }

func offsets(cs []text.Cursor) []int {
	out := make([]int, len(cs))
	for i, c := range cs {
		out[i] = c.Offset
	}
	return out
}

// TestMoveCharacter checks arrow keys by grapheme cluster through bidi
// text, emoji sequences, combining marks and CJK, and that pure cursor
// movement never rebuilds the layout.
func TestMoveCharacter(t *testing.T) {
	// "ab مرحبا cd": the Arabic word is drawn reversed (package text's
	// TestMoveVisual has the same walk).
	d := parse(t, "|ab مرحبا cd", Options{})
	got := walk(d, Command{Kind: MoveRight})
	want := []text.Cursor{cur(1), cur(2), up(3), cur(11), cur(9), cur(7), cur(5), cur(3), cur(14), cur(15), cur(16)}
	if !slices.Equal(got, want) {
		t.Errorf("Arabic: got %v, want %v", got, want)
	}
	if d.rebuild != 1 {
		t.Errorf("cursor movement rebuilt the layout %d times", d.rebuild)
	}
	// Hebrew: every step moves the caret right on screen and every
	// position is visited.
	d = parse(t, "|abc שלום def", Options{})
	l := d.layout()
	prev := l.Caret(d.e.Selection().Focus).X
	got = walk(d, Command{Kind: MoveRight})
	for _, c := range got {
		x := l.Caret(c).X
		if x <= prev {
			t.Errorf("Hebrew: MoveRight to %v did not move right (%g after %g)", c, x, prev)
		}
		prev = x
	}
	if seen := offsets(got); len(seen) != 12 {
		t.Errorf("Hebrew: %d stops, want 12: %v", len(seen), seen)
	}
	// Logical movement in Hebrew goes by bytes in order.
	d = parse(t, "|שלום", Options{})
	if got := offsets(walk(d, Command{Kind: MoveForward})); !slices.Equal(got, []int{2, 4, 6, 8}) {
		t.Errorf("Hebrew logical: %v", got)
	}
	// Grapheme clusters are one step.
	d = parse(t, "|a👨\u200d👩\u200d👧e\u0301b日本", Options{})
	if got := offsets(walk(d, Command{Kind: MoveRight})); !slices.Equal(got, []int{1, 19, 22, 23, 26, 29}) {
		t.Errorf("clusters: %v", got)
	}
	// Shift extends, a plain arrow collapses to the edge.
	d = parse(t, "a|bcd", Options{})
	d.do(Command{Kind: MoveRight, Extend: true})
	d.do(Command{Kind: MoveRight, Extend: true})
	expectState(t, "shift+right", d.e, "a[bc]d")
	d.do(Command{Kind: MoveLeft})
	expectState(t, "left collapses", d.e, "a|bcd")
}

// TestMoveWord checks word movement in both conventions and in a
// right-to-left paragraph.
func TestMoveWord(t *testing.T) {
	const s = "hello, world foo"
	tests := []struct {
		name  string
		words WordConvention
		kind  CommandKind
		from  int
		want  []int
	}{
		{"macOS/Linux right", WordsMacLinux, MoveWordRight, 0, []int{5, 12, 16}},
		{"Windows right", WordsWindows, MoveWordRight, 0, []int{7, 13, 16}},
		{"macOS/Linux left", WordsMacLinux, MoveWordLeft, 16, []int{13, 7, 0}},
		{"Windows left", WordsWindows, MoveWordLeft, 16, []int{13, 7, 0}},
	}
	for _, tc := range tests {
		d := newDriver(t, s, Options{Words: tc.words})
		d.e.Select(tc.from, tc.from)
		if got := offsets(walk(d, Command{Kind: tc.kind})); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	// Right to left: left is forward.
	for _, tc := range []struct {
		words WordConvention
		want  int
	}{{WordsMacLinux, 8}, {WordsWindows, 9}} {
		d := newDriver(t, "שלום עולם", Options{Words: tc.words})
		d.dir = text.DirectionRTL
		d.e.Select(0, 0)
		d.kind(MoveWordLeft)
		if f := d.e.Selection().Focus.Offset; f != tc.want {
			t.Errorf("RTL word left (convention %d): %d, want %d", tc.words, f, tc.want)
		}
	}
}

// TestDeleteWordAndLine checks deletion by word and to the line edges.
func TestDeleteWordAndLine(t *testing.T) {
	tests := []struct {
		name       string
		words      WordConvention
		start      string
		kind       CommandKind
		want       string
		inputType  InputType
		multilines bool
	}{
		{"word backward", WordsMacLinux, "hello, world|", DeleteWordBackward, "hello, |", InputDeleteWordBackward, false},
		{"word backward inside a word", WordsMacLinux, "hello, wo|rld", DeleteWordBackward, "hello, |rld", InputDeleteWordBackward, false},
		{"word forward macOS/Linux", WordsMacLinux, "|hello, world", DeleteWordForward, "|, world", InputDeleteWordForward, false},
		{"word forward Windows", WordsWindows, "|hello, world", DeleteWordForward, "|world", InputDeleteWordForward, false},
		{"word with a selection", WordsMacLinux, "he[ll]o", DeleteWordBackward, "he|o", InputDeleteWordBackward, false},
		{"line start", WordsMacLinux, "abc\ndef|", DeleteToLineStart, "abc\n|", InputDeleteSoftLineBack, true},
		{"line start at a line start", WordsMacLinux, "abc\n|", DeleteToLineStart, "abc|", InputDeleteSoftLineBack, true},
		{"line end", WordsMacLinux, "ab|c\ndef", DeleteToLineEnd, "ab|\ndef", InputDeleteSoftLineForward, true},
		{"line end at a line end", WordsMacLinux, "ab|\ndef", DeleteToLineEnd, "ab|def", InputDeleteSoftLineForward, true},
	}
	for _, tc := range tests {
		d := parse(t, tc.start, Options{Words: tc.words, Multiline: tc.multilines})
		r := d.kind(tc.kind)
		expectState(t, tc.name, d.e, tc.want)
		if r.InputType != tc.inputType {
			t.Errorf("%s: input type %q, want %q", tc.name, r.InputType, tc.inputType)
		}
	}
	// Without a layout, a collapsed selection cannot be deleted by word.
	e := New("hello", Options{})
	if p := e.Plan(Command{Kind: DeleteWordBackward}, nil); !p.Empty() {
		t.Errorf("word deletion without a layout: %+v", p)
	}
	// In a wrapped line, the line start is the visual line's.
	d := newDriver(t, "hello world", Options{Multiline: true})
	d.width = 60
	if n := d.layout().NumLines(); n != 2 {
		t.Fatalf("expected 2 lines at width 60, got %d", n)
	}
	d.e.Select(9, 9)
	d.kind(DeleteToLineStart)
	expectState(t, "wrapped line start", d.e, "hello |ld")
}

// TestVertical checks vertical movement with a goal column, pages, Home
// and End, and the ends of the text.
func TestVertical(t *testing.T) {
	d := newDriver(t, "abcdefgh\nab\nabcdefgh", Options{Multiline: true})
	d.e.Select(6, 6)
	var got []int
	for range 2 {
		d.kind(MoveDown)
		got = append(got, d.e.Selection().Focus.Offset)
	}
	for range 2 {
		d.kind(MoveUp)
		got = append(got, d.e.Selection().Focus.Offset)
	}
	if !slices.Equal(got, []int{11, 18, 11, 6}) {
		t.Errorf("goal column through a short line: %v, want [11 18 11 6]", got)
	}
	d.kind(MoveUp)
	expectState(t, "up from the first line", d.e, "|abcdefgh\nab\nabcdefgh")
	// The goal column survives going to the start of the text.
	d.do(Command{Kind: MoveDown, Extend: true})
	if s, e, _ := d.e.SelectionRange(); s != 0 || e != 11 {
		t.Errorf("shift+down: [%d, %d), want [0, 11)", s, e)
	}
	d.kind(MoveDocumentEnd)
	expectState(t, "ctrl+end", d.e, "abcdefgh\nab\nabcdefgh|")
	d.kind(MoveLineStart)
	expectState(t, "home", d.e, "abcdefgh\nab\n|abcdefgh")
	d.kind(MoveDocumentStart)
	d.kind(MoveLineEnd)
	expectState(t, "end", d.e, "abcdefgh|\nab\nabcdefgh")

	// Wrapped lines: down from the first visual line keeps the column.
	d = newDriver(t, "hello world again", Options{Multiline: true})
	d.width = 60
	l := d.layout()
	if l.NumLines() != 3 {
		t.Fatalf("expected 3 lines, got %d", l.NumLines())
	}
	d.e.Select(1, 1)
	x := l.Caret(d.e.Selection().Focus).X
	d.kind(MoveDown)
	c := l.Caret(d.e.Selection().Focus)
	if c.Line != 1 || absf(c.X-x) > 6 {
		t.Errorf("down in wrapped text: line %d x %g, want line 1 near x %g", c.Line, c.X, x)
	}

	// Pages: three lines of 24 px per 72 px page.
	d = newDriver(t, "0\n1\n2\n3\n4\n5\n6\n7\n8\n9", Options{Multiline: true})
	d.e.Select(0, 0)
	d.do(Command{Kind: MovePageDown, Height: 72})
	if line := d.layout().Caret(d.e.Selection().Focus).Line; line != 3 {
		t.Errorf("page down: line %d, want 3", line)
	}
	d.do(Command{Kind: MovePageDown, Height: 72, Extend: true})
	if s, e, _ := d.e.SelectionRange(); s != 6 || e != 12 {
		t.Errorf("shift+page down: [%d, %d), want [6, 12)", s, e)
	}
	d.do(Command{Kind: MovePageUp, Height: 72})
	if line := d.layout().Caret(d.e.Selection().Focus).Line; line != 3 {
		t.Errorf("page up: line %d, want 3", line)
	}
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// TestPointer checks clicks, shift+clicks, double and triple clicks and
// drags by unit.
func TestPointer(t *testing.T) {
	d := newDriver(t, "hello world foo", Options{})
	l := d.layout()
	xAt := func(off int) float32 { return l.Caret(cur(off)).X }
	y := float32(10)
	d.do(Command{Kind: ClickAt, X: xAt(3), Y: y})
	expectState(t, "click", d.e, "hel|lo world foo")
	d.do(Command{Kind: ClickAt, X: xAt(8), Y: y, Extend: true})
	expectState(t, "shift+click", d.e, "hel[lo wo]rld foo")
	d.do(Command{Kind: SelectWordAt, X: (xAt(6) + xAt(11)) / 2, Y: y})
	expectState(t, "double click", d.e, "hello [world] foo")
	d.do(Command{Kind: DragTo, X: xAt(1), Y: y})
	expectState(t, "word drag backward", d.e, "[hello world] foo<")
	d.do(Command{Kind: DragTo, X: xAt(13), Y: y})
	expectState(t, "word drag forward", d.e, "hello [world foo]")
	d.do(Command{Kind: SelectParagraphAt, X: xAt(2), Y: y})
	expectState(t, "triple click in an input", d.e, "[hello world foo]")
	d.kind(SelectAll)
	expectState(t, "select all", d.e, "[hello world foo]")
	if d.rebuild != 1 {
		t.Errorf("pointer commands rebuilt the layout %d times", d.rebuild)
	}

	// Triple click in a textarea selects the paragraph with its break.
	d = newDriver(t, "abc def\nghi", Options{Multiline: true})
	l = d.layout()
	d.do(Command{Kind: SelectParagraphAt, X: l.Caret(cur(2)).X, Y: 10})
	expectState(t, "triple click in a textarea", d.e, "[abc def\n]ghi")
	d.do(Command{Kind: SelectLineAt, X: 0, Y: 30})
	expectState(t, "line at point", d.e, "abc def\n[ghi]")
}

// TestStaleLayout checks that a layout of another text is refused.
func TestStaleLayout(t *testing.T) {
	d := newDriver(t, "abc", Options{})
	l := d.layout()
	d.typ("d")
	defer func() {
		if recover() == nil {
			t.Error("a stale layout was accepted")
		}
	}()
	d.e.Plan(Command{Kind: MoveLeft}, l)
}
