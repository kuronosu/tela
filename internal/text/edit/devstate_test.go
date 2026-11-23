//go:build teladev

package edit

import (
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// TestDevStateRoundTrip moves an editor's value, selection and history to
// a new editor, as a reload does (ADR-113).
func TestDevStateRoundTrip(t *testing.T) {
	a := parse(t, "|", Options{})
	a.typ("hello world")
	a.kind(Undo)  // "hello", " world" on the redo stack
	a.typ(" you") // clears the redo stack
	a.kind(MoveLeft)
	st := a.e.DevState()

	b := parse(t, "|", Options{})
	r := b.e.RestoreDevState(st)
	if !r.ValueChanged || !r.SelectionChanged || !r.DisplayChanged {
		t.Errorf("result %+v", r)
	}
	expectState(t, "restored", b.e, state(a.e))
	// The same steps undo.
	eqStrings(t, "undo", undoAll(b), undoAll(a))
	// The history starts closed: typing right where the last step ended,
	// which would join that step before the reload, is a step of its own.
	src := parse(t, "|", Options{})
	src.typ("hello")
	c := parse(t, "|", Options{})
	c.e.RestoreDevState(src.e.DevState())
	c.typ("x")
	c.kind(Undo)
	expectState(t, "undo after the restore", c.e, "hello|")
}

func text0(off int) text.Cursor { return text.Cursor{Offset: off} }

// TestDevStateDamaged checks that a history that does not fit the value
// is dropped instead of breaking Undo, and that offsets are clamped.
func TestDevStateDamaged(t *testing.T) {
	st := DevState{
		Value:  "abc",
		Anchor: text0(99), Focus: text0(-5),
		Undo: []DevStep{{At: 1, Removed: "", Inserted: "zz"}}, // "zz" is not at 1
	}
	d := parse(t, "|", Options{})
	d.e.RestoreDevState(st)
	if d.e.CanUndo() {
		t.Errorf("a history that does not fit the value was kept")
	}
	if s, e, _ := d.e.SelectionRange(); s != 0 || e != 3 {
		t.Errorf("selection %d..%d, want the offsets clamped to 0..3", s, e)
	}
	// Redo steps are checked from the value too.
	st = DevState{Value: "abc", Redo: []DevStep{{At: 3, Removed: "x", Inserted: "y"}}}
	d.e.RestoreDevState(st)
	if d.e.CanRedo() {
		t.Errorf("a redo step that does not fit was kept")
	}
	// A single-line editor cleans a multi-line value and drops its history.
	st = DevState{Value: "a\nb", Undo: []DevStep{{At: 1, Inserted: "\nb"}}}
	d.e.RestoreDevState(st)
	if d.e.Value() != "ab" || d.e.CanUndo() {
		t.Errorf("value %q (undo %v), want %q without history", d.e.Value(), d.e.CanUndo(), "ab")
	}
}

// TestDevStateCleanedValue checks that a value cleaned for the editor's
// mode (a Textarea's text restored into an Input loses its line breaks)
// keeps neither the saved selection nor the history, whose offsets are
// the saved value's: the caret goes to the end, as SetText puts it.
func TestDevStateCleanedValue(t *testing.T) {
	src := parse(t, "ab\n[cd]\nef", Options{Multiline: true})
	st := src.e.DevState()
	d := parse(t, "|", Options{})
	d.e.RestoreDevState(st)
	expectState(t, "single-line", d.e, "abcdef|")
	// Restored into a multi-line editor, the value is the saved one: the
	// selection is kept.
	m := parse(t, "|", Options{Multiline: true})
	m.e.RestoreDevState(st)
	expectState(t, "multi-line", m.e, "ab\n[cd]\nef")
}

// TestDevStateComposition moves a composition in progress to a new editor
// (ADR-309): the state it started from, then the composition itself, so
// that its commit and its undo work as in the old editor.
func TestDevStateComposition(t *testing.T) {
	a := parse(t, "a[bc]", Options{})
	a.e.SetPreedit("にほ", 6, 6)
	st := a.e.DevState()
	if c := st.Composition; st.Value != "abc" || c == nil || c.Value != "a" || c.At != 1 || c.Preedit != "にほ" {
		t.Fatalf("state %+v (composition %+v): want the state before the composition and the composition", st, st.Composition)
	}
	b := parse(t, "|", Options{})
	b.e.RestoreDevState(st)
	expectState(t, "the state the composition started from", b.e, "a[bc]")
	r := b.e.RestoreDevComposition(*st.Composition)
	if !r.ValueChanged || !r.DisplayChanged || !b.e.Composing() || b.e.Display() != a.e.Display() || b.e.Selection() != a.e.Selection() {
		t.Fatalf("restored composition: result %+v, composing %v, display %q (want %q)", r, b.e.Composing(), b.e.Display(), a.e.Display())
	}
	ca, _ := a.e.Composition()
	cb, _ := b.e.Composition()
	if ca != cb {
		t.Errorf("composition %+v, want %+v", cb, ca)
	}
	b.e.CommitComposition("日本")
	expectState(t, "commit", b.e, "a日本|")
	b.kind(Undo)
	expectState(t, "undo of the composition", b.e, "a[bc]")

	// Without RestoreDevComposition the restore is a clean cancel: the text
	// before the composition, with its selection.
	c := parse(t, "|", Options{})
	c.e.RestoreDevState(st)
	if c.e.Composing() || c.e.Value() != "abc" {
		t.Errorf("cancel: composing %v, %q", c.e.Composing(), c.e.Value())
	}
	// A composition that does not fit is refused, and so is one for a
	// read-only editor.
	for _, bad := range []DevComposition{{Value: "a\nb", At: 1, Preedit: "x"}, {Value: "a", At: 5, Preedit: "x"}, {Value: "é", At: 1, Preedit: "x"}} {
		d := parse(t, "|", Options{})
		if r := d.e.RestoreDevComposition(bad); r != (Result{}) || d.e.Composing() {
			t.Errorf("RestoreDevComposition(%+v) applied", bad)
		}
	}
	ro := parse(t, "|", Options{ReadOnly: true})
	if ro.e.RestoreDevComposition(*st.Composition); ro.e.Composing() {
		t.Errorf("a read-only editor composes")
	}
}
