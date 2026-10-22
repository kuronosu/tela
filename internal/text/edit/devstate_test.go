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
