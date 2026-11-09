package edit

import "testing"

// TestDropAndDeleteByDrag checks the drag and drop edits: a drop inserts
// over the selection and selects the inserted text (browsers select
// dropped text), and undo and redo restore the selections around it;
// DeleteByDrag deletes the selection as one deleteByDrag step and does
// nothing without one or in a read-only field.
func TestDropAndDeleteByDrag(t *testing.T) {
	d := parse(t, "ab|cd", Options{})
	r := d.do(Command{Kind: Drop, Text: "XY"})
	expectState(t, "drop", d.e, "ab[XY]cd")
	if !r.ValueChanged || !r.SelectionChanged || r.InputType != InputInsertFromDrop || r.Data != "XY" {
		t.Errorf("drop result: %+v", r)
	}
	d.kind(Undo)
	expectState(t, "undo the drop", d.e, "ab|cd")
	d.kind(Redo)
	expectState(t, "redo the drop", d.e, "ab[XY]cd")
	// Over a selection, cleaned like a paste (a single-line field).
	d = parse(t, "a[bc]d", Options{})
	d.do(Command{Kind: Drop, Text: "1\n2"})
	expectState(t, "drop over a selection", d.e, "a[1 2]d")
	// A combining mark dropped after its base selects from the boundary
	// before the base.
	d = parse(t, "e|", Options{})
	d.do(Command{Kind: Drop, Text: "́x"})
	expectState(t, "drop merging with the text before", d.e, "[éx]")

	d = parse(t, "a[bc]d", Options{})
	r = d.kind(DeleteByDrag)
	expectState(t, "delete by drag", d.e, "a|d")
	if !r.ValueChanged || r.InputType != InputDeleteByDrag || r.Clipboard != "" {
		t.Errorf("delete by drag result: %+v", r)
	}
	d.kind(Undo)
	expectState(t, "undo the deletion", d.e, "a[bc]d")
	d = parse(t, "ab|cd", Options{})
	if r := d.kind(DeleteByDrag); r != (Result{}) {
		t.Errorf("delete by drag without a selection: %+v", r)
	}
	d = parse(t, "a[bc]d", Options{ReadOnly: true})
	if r := d.kind(DeleteByDrag); r != (Result{}) {
		t.Errorf("delete by drag in a read-only field: %+v", r)
	}
}

// TestDropJoinUndo checks a move within the field: a Drop with JoinUndo
// after the DeleteByDrag of the dragged text is one undo step (Chromium's
// DragAndDropCommand): undo restores the text and its selection, redo
// moves it again; the steps before stay separate, and a Drop without
// JoinUndo is a step of its own.
func TestDropJoinUndo(t *testing.T) {
	d := parse(t, "one two| three", Options{})
	d.do(Command{Kind: InsertText, Text: "!"})
	d.e.Select(0, 4) // "one " dragged
	d.kind(DeleteByDrag)
	d.e.Select(4, 4) // the drop point after "two!"
	d.do(Command{Kind: Drop, Text: "one ", JoinUndo: true})
	expectState(t, "move", d.e, "two![one ] three")
	d.kind(Undo)
	expectState(t, "undo the move", d.e, "[one ]two! three")
	d.kind(Undo)
	expectState(t, "undo the typing", d.e, "one two| three")
	d.kind(Redo)
	d.kind(Redo)
	expectState(t, "redo the move", d.e, "two![one ] three")
	if d.e.CanRedo() {
		t.Error("a redo step is left after the move")
	}
	d.kind(Undo)
	expectState(t, "undo the redone move", d.e, "[one ]two! three")

	d = parse(t, "[ab]cd", Options{})
	d.kind(DeleteByDrag)
	d.e.Select(2, 2)
	d.do(Command{Kind: Drop, Text: "ab"})
	expectState(t, "drop", d.e, "cd[ab]")
	d.kind(Undo)
	expectState(t, "undo the drop alone", d.e, "cd|")

	// A history limited to one step keeps the drop without its partner.
	d = parse(t, "[ab]cd", Options{UndoLimit: 1})
	d.kind(DeleteByDrag)
	d.e.Select(2, 2)
	d.do(Command{Kind: Drop, Text: "ab", JoinUndo: true})
	d.kind(Undo)
	expectState(t, "undo with a limit of one", d.e, "cd|")
	if d.e.CanUndo() {
		t.Error("an undo step is left past the limit")
	}
}
