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
