package edit

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// TestPreedit checks the preedit lifecycle: display, ranges, selection,
// commit and its undo step.
func TestPreedit(t *testing.T) {
	d := parse(t, "ab|cd", Options{})
	r := d.e.SetPreedit("にほ", 3, 3)
	if r.InputType != InputInsertCompositionText || r.Data != "にほ" || !r.IsComposing || r.ValueChanged || !r.DisplayChanged {
		t.Errorf("preedit result: %+v", r)
	}
	if d.e.Value() != "abcd" || d.e.Display() != "abにほcd" {
		t.Errorf("value %q, display %q", d.e.Value(), d.e.Display())
	}
	c, ok := d.e.Composition()
	if !ok || c.Start != 2 || c.End != 8 || c.CursorStart != 5 || c.CursorEnd != 5 || c.CursorHidden {
		t.Errorf("composition: %+v %v", c, ok)
	}
	if s := d.e.Selection(); s.Anchor.Offset != 5 || s.Focus.Offset != 5 {
		t.Errorf("selection in the preedit: %+v", s)
	}
	// The layout of Display places the caret in the preedit.
	l := d.layout()
	if x := l.Caret(d.e.Selection().Focus).X; !(x > l.Caret(text.Cursor{Offset: 2}).X) {
		t.Errorf("caret not inside the preedit: %g", x)
	}
	// Commands are no-ops while composing.
	for _, k := range []CommandKind{InsertText, DeleteBackward, MoveLeft, Undo, SelectAll} {
		if p := d.e.Plan(Command{Kind: k, Text: "x"}, d.layout()); !p.Empty() {
			t.Errorf("command %d during a composition: %+v", k, p)
		}
	}
	// Update, then commit.
	d.e.SetPreedit("にほん", -1, -1)
	if c, _ := d.e.Composition(); !c.CursorHidden || c.CursorEnd != 11 {
		t.Errorf("hidden cursor: %+v", c)
	}
	r = d.e.CommitComposition("日本")
	if r.InputType != InputInsertFromComposition || r.Data != "日本" || r.IsComposing || !r.ValueChanged {
		t.Errorf("commit result: %+v", r)
	}
	expectState(t, "commit", d.e, "ab日本|cd")
	if d.e.Composing() {
		t.Error("still composing after commit")
	}
	eqStrings(t, "commit undo", undoAll(d), []string{"ab|cd"})
}

// TestPreeditReplacesSelection checks that the first preedit deletes the
// selection and that commit and cancel record it.
func TestPreeditReplacesSelection(t *testing.T) {
	d := parse(t, "a[bc]d", Options{})
	r := d.e.SetPreedit("x", 1, 1)
	if !r.ValueChanged || d.e.Value() != "ad" || d.e.Display() != "axd" {
		t.Errorf("first preedit: %+v %q %q", r, d.e.Value(), d.e.Display())
	}
	d.e.CommitComposition("y")
	expectState(t, "commit", d.e, "ay|d")
	eqStrings(t, "one step", undoAll(d), []string{"a[bc]d"})

	d = parse(t, "a[bc]d", Options{})
	d.e.SetPreedit("x", 1, 1)
	r = d.e.CancelComposition()
	if r.InputType != InputDeleteCompositionText || r.ValueChanged || !r.DisplayChanged {
		t.Errorf("cancel: %+v", r)
	}
	expectState(t, "cancel keeps the deletion", d.e, "a|d")
	eqStrings(t, "cancel undo", undoAll(d), []string{"a[bc]d"})

	// Cancel without a deleted selection leaves no step.
	d = parse(t, "a|d", Options{})
	d.e.SetPreedit("x", 1, 1)
	d.e.CancelComposition()
	if d.e.CanUndo() {
		t.Error("an empty composition left an undo step")
	}
}

// TestFinishComposition checks that finishing keeps the display (and so
// the layout) and commits the preedit.
func TestFinishComposition(t *testing.T) {
	d := parse(t, "ab|", Options{})
	d.e.SetPreedit("cd", 1, 1)
	ver := d.e.DisplayVersion()
	r := d.e.FinishComposition()
	if !r.ValueChanged || r.DisplayChanged || d.e.DisplayVersion() != ver {
		t.Errorf("finish: %+v", r)
	}
	expectState(t, "finish", d.e, "abcd|")
	// A click after finishing uses the same layout.
	l := d.layout()
	d.do(Command{Kind: ClickAt, X: l.Caret(text.Cursor{Offset: 1}).X, Y: 5})
	expectState(t, "click after finish", d.e, "a|bcd")
	if d.rebuild != 1 {
		t.Errorf("finishing rebuilt the layout")
	}
}

// TestCommitWithoutComposition checks direct IME commits, which type.
func TestCommitWithoutComposition(t *testing.T) {
	d := parse(t, "|", Options{})
	d.e.CommitComposition("é")
	r := d.e.CommitComposition("t")
	if r.InputType != InputInsertText {
		t.Errorf("direct commit: %+v", r)
	}
	expectState(t, "direct commits", d.e, "ét|")
	eqStrings(t, "coalesced", undoAll(d), []string{"|"})
}

// TestCompositionMaxLength checks that maxlength applies at commit.
func TestCompositionMaxLength(t *testing.T) {
	d := parse(t, "ab|", Options{MaxLength: 3})
	d.e.SetPreedit("xyz", 3, 3)
	if d.e.Display() != "abxyz" {
		t.Errorf("the preedit was limited: %q", d.e.Display())
	}
	d.e.CommitComposition("xyz")
	expectState(t, "commit truncated", d.e, "abx|")
}

// TestDeleteSurrounding checks text-input-v3's delete_surrounding_text.
func TestDeleteSurrounding(t *testing.T) {
	// DeleteSurroundingType tells the InputType without deleting.
	for _, c := range []struct {
		st            string
		before, after int
		want          InputType
	}{
		{"hello world|!", 5, 1, InputDeleteContent},
		{"ab|c", 1, 0, InputDeleteContentBackward},
		{"ab|c", 0, 1, InputDeleteContentForward},
		{"日本|語", 1, 0, InputDeleteContentBackward},
		{"|abc", 2, 0, InputNone},
		{"abc|", 0, 2, InputNone},
	} {
		d := parse(t, c.st, Options{})
		if got := d.e.DeleteSurroundingType(c.before, c.after); got != c.want {
			t.Errorf("DeleteSurroundingType(%d, %d) on %q = %q, want %q", c.before, c.after, c.st, got, c.want)
		}
		if got := d.e.DeleteSurrounding(c.before, c.after).InputType; got != c.want {
			t.Errorf("DeleteSurrounding(%d, %d) on %q = %q, want %q", c.before, c.after, c.st, got, c.want)
		}
	}
	if d := parse(t, "ab|c", Options{ReadOnly: true}); d.e.DeleteSurroundingType(1, 1) != InputNone {
		t.Error("a read-only editor plans a deletion")
	}

	d := parse(t, "hello world|!", Options{})
	r := d.e.DeleteSurrounding(5, 1)
	if r.InputType != InputDeleteContent || !r.ValueChanged {
		t.Errorf("delete surrounding: %+v", r)
	}
	expectState(t, "both sides", d.e, "hello |")
	eqStrings(t, "undo", undoAll(d), []string{"hello world|!"})

	// The selection's text stays.
	d = parse(t, "ab[cd]ef", Options{})
	d.e.DeleteSurrounding(1, 1)
	expectState(t, "around a selection", d.e, "a[cd]f")

	// Lengths inside a UTF-8 sequence widen to the character.
	d = parse(t, "日本|語", Options{})
	r = d.e.DeleteSurrounding(1, 0)
	if r.InputType != InputDeleteContentBackward {
		t.Errorf("input type: %q", r.InputType)
	}
	expectState(t, "widened", d.e, "日|語")

	// During a composition: around the preedit, which stays; the whole
	// composition is one step.
	d = parse(t, "abc|", Options{})
	d.e.SetPreedit("x", 1, 1)
	r = d.e.DeleteSurrounding(1, 0)
	if !r.IsComposing || d.e.Display() != "abx" || d.e.Value() != "ab" {
		t.Errorf("in a composition: %+v %q %q", r, d.e.Display(), d.e.Value())
	}
	d.e.CommitComposition("Y")
	expectState(t, "composition with deletion", d.e, "abY|")
	eqStrings(t, "composition undo", undoAll(d), []string{"abc|"})
}

// TestPassword checks masking, offset mapping and the disabled commands.
func TestPassword(t *testing.T) {
	const v = "a👨\u200d👩\u200d👧e\u0301b"
	d := newDriver(t, v, Options{Password: true})
	if d.e.Display() != "••••" {
		t.Fatalf("display %q", d.e.Display())
	}
	// Offset mapping per grapheme cluster.
	vb := []int{0, 1, 19, 22, 23}
	for i, off := range vb {
		if got := d.e.ValueToDisplay(off); got != 3*i {
			t.Errorf("ValueToDisplay(%d) = %d, want %d", off, got, 3*i)
		}
		if got := d.e.DisplayToValue(3 * i); got != off {
			t.Errorf("DisplayToValue(%d) = %d, want %d", 3*i, got, off)
		}
	}
	if got := d.e.ValueToDisplay(5); got != 3 {
		t.Errorf("inside a cluster: %d, want 3", got)
	}
	// Arrows move by grapheme cluster of the value.
	d.e.Select(0, 0)
	var got []int
	for range 4 {
		d.kind(MoveRight)
		s, _, _ := d.e.SelectionRange()
		got = append(got, s)
	}
	if !slices.Equal(got, vb[1:]) {
		t.Errorf("arrows: %v, want %v", got, vb[1:])
	}
	// No copy, no cut, no selected text.
	d.kind(SelectAll)
	if p := d.e.Plan(Command{Kind: Copy}, nil); !p.Empty() {
		t.Errorf("copy in a password field: %+v", p)
	}
	if p := d.e.Plan(Command{Kind: Cut}, nil); !p.Empty() {
		t.Errorf("cut in a password field: %+v", p)
	}
	if s := d.e.SelectedText(); s != "" {
		t.Errorf("selected text exposed: %q", s)
	}
	// Words: the masks hold none, so word movement goes to the ends and a
	// double click selects everything.
	d.e.Select(19, 19)
	d.kind(MoveWordRight)
	expectState(t, "word right", d.e, v+"|")
	d.e.Select(19, 19)
	d.kind(DeleteWordBackward)
	expectState(t, "word delete", d.e, "|e\u0301b")
	d = newDriver(t, "secret word", Options{Password: true})
	l := d.layout()
	d.do(Command{Kind: SelectWordAt, X: l.Caret(text.Cursor{Offset: 9}).X, Y: 5})
	expectState(t, "double click", d.e, "[secret word]")
	// Typing in the middle updates the mask and the caret.
	d.e.Select(6, 6)
	d.typ("!")
	if d.e.Display() != "••••••••••••" || d.e.Selection().Focus.Offset != 21 {
		t.Errorf("typing: %q %+v", d.e.Display(), d.e.Selection())
	}
	expectState(t, "typing", d.e, "secret!| word")
	// A preedit is masked too.
	d.e.SetPreedit("xy", 2, 2)
	if c, _ := d.e.Composition(); d.e.Display() != "••••••••••••••" || c.Start != 21 || c.End != 27 {
		t.Errorf("masked preedit: %q %+v", d.e.Display(), c)
	}
}

// TestCompositionMapping checks offset mapping around a preedit.
func TestCompositionMapping(t *testing.T) {
	d := parse(t, "ab|cd", Options{})
	d.e.SetPreedit("xyz", 0, 0)
	for _, tc := range []struct{ display, value int }{{0, 0}, {2, 2}, {3, 2}, {5, 2}, {6, 3}, {7, 4}} {
		if got := d.e.DisplayToValue(tc.display); got != tc.value {
			t.Errorf("DisplayToValue(%d) = %d, want %d", tc.display, got, tc.value)
		}
	}
	for _, tc := range []struct{ value, display int }{{1, 1}, {2, 2}, {3, 6}, {4, 7}} {
		if got := d.e.ValueToDisplay(tc.value); got != tc.display {
			t.Errorf("ValueToDisplay(%d) = %d, want %d", tc.value, got, tc.display)
		}
	}
	if s, e, _ := d.e.SelectionRange(); s != 2 || e != 2 {
		t.Errorf("selection range while composing: [%d, %d)", s, e)
	}
	if d.e.SelectedText() != "" {
		t.Error("selected text while composing")
	}
	if r := d.e.Select(0, 1); r != (Result{}) {
		t.Errorf("Select while composing: %+v", r)
	}
}

// TestSetTextDuringComposition checks that programmatic values and
// option changes end a composition.
func TestSetTextDuringComposition(t *testing.T) {
	// Back to the value from before the composition: the composition is
	// dropped and the selection restored, history untouched.
	d := parse(t, "a[bc]d", Options{})
	d.e.SetPreedit("x", 1, 1)
	d.e.SetText("abcd", ResetHistory)
	expectState(t, "same value", d.e, "a[bc]d")
	if d.e.Composing() {
		t.Error("still composing")
	}
	// A new value with RecordHistory: one step from the value before the
	// composition.
	d = parse(t, "a[bc]d", Options{})
	d.e.SetPreedit("x", 1, 1)
	d.e.SetText("zz", RecordHistory)
	expectState(t, "new value", d.e, "zz|")
	eqStrings(t, "undo", undoAll(d), []string{"a[bc]d"})
	// SetOptions cancels the composition.
	d = parse(t, "ab|", Options{})
	d.e.SetPreedit("x", 1, 1)
	r := d.e.SetOptions(Options{Password: true})
	if d.e.Composing() || !r.DisplayChanged || d.e.Display() != "••" {
		t.Errorf("SetOptions while composing: %+v %q", r, d.e.Display())
	}
	// FinishComposition without a composition does nothing.
	if r := d.e.FinishComposition(); r != (Result{}) {
		t.Errorf("finish without a composition: %+v", r)
	}
}
