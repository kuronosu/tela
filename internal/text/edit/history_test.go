package edit

import (
	"testing"
	"time"
)

// undoAll undoes until nothing is left and returns each state.
func undoAll(d *driver) []string {
	var out []string
	for d.e.CanUndo() {
		d.kind(Undo)
		out = append(out, state(d.e))
	}
	return out
}

func eqStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %q, want %q", what, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: got %q, want %q", what, got, want)
			return
		}
	}
}

// TestUndoCoalescing checks which edits coalesce into one undo step.
func TestUndoCoalescing(t *testing.T) {
	// Words: each word with the space before it is a step.
	d := parse(t, "|", Options{})
	d.typ("hello world")
	eqStrings(t, "words", undoAll(d), []string{"hello|", "|"})
	// Redo replays them, with the selection after each step.
	d.kind(Redo)
	expectState(t, "redo 1", d.e, "hello|")
	r := d.kind(Redo)
	expectState(t, "redo 2", d.e, "hello world|")
	if r.InputType != InputHistoryRedo || !r.ValueChanged {
		t.Errorf("redo result: %+v", r)
	}
	if d.e.CanRedo() {
		t.Error("redo stack not empty")
	}

	// Spaces after spaces stay in the step; a run of spaces after a word
	// starts one.
	d = parse(t, "|", Options{})
	d.typ("a  b")
	eqStrings(t, "spaces", undoAll(d), []string{"a|", "|"})

	// A selection change by navigation closes the step.
	d = parse(t, "|", Options{})
	d.typ("ab")
	d.kind(MoveLeft)
	d.typ("c")
	eqStrings(t, "navigation", undoAll(d), []string{"a|b", "|"})

	// Typing and deleting are different steps; Backspaces coalesce.
	d = parse(t, "|", Options{})
	d.typ("abcd")
	d.kind(DeleteBackward)
	d.kind(DeleteBackward)
	d.typ("x")
	eqStrings(t, "kinds", undoAll(d), []string{"ab|", "abcd|", "|"})

	// Delete runs coalesce at the same offset.
	d = parse(t, "|abcd", Options{})
	d.kind(DeleteForward)
	d.kind(DeleteForward)
	expectState(t, "deletes", d.e, "|cd")
	eqStrings(t, "deletes", undoAll(d), []string{"|abcd"})

	// Typing over a selection starts a step that later typing joins; undo
	// restores the replaced text and its selection.
	d = parse(t, "a[bc]d", Options{})
	d.typ("xy")
	eqStrings(t, "replace", undoAll(d), []string{"a[bc]d"})

	// Paste never coalesces.
	d = parse(t, "|", Options{})
	d.typ("a")
	d.do(Command{Kind: Paste, Text: "b"})
	d.typ("c")
	eqStrings(t, "paste", undoAll(d), []string{"ab|", "a|", "|"})

	// A pause longer than the delay closes the step.
	clk := &clock{}
	d = parse(t, "|", Options{Now: clk.Now})
	d.typ("ab")
	clk.now += 400 * time.Millisecond
	d.typ("c")
	clk.now += 501 * time.Millisecond
	d.typ("d")
	eqStrings(t, "pause", undoAll(d), []string{"abc|", "|"})
	// A negative delay never splits.
	d = parse(t, "|", Options{Now: clk.Now, UndoDelay: -1})
	d.typ("a")
	clk.now += time.Hour
	d.typ("b")
	eqStrings(t, "no pause rule", undoAll(d), []string{"|"})

	// A new edit clears the redo stack.
	d = parse(t, "|", Options{})
	d.typ("a")
	d.kind(Undo)
	d.typ("b")
	if d.e.CanRedo() {
		t.Error("redo survived a new edit")
	}
}

// TestUndoSelection checks that undo restores the selection of deletions.
func TestUndoSelection(t *testing.T) {
	d := parse(t, "ab[cd]e<", Options{})
	d.kind(DeleteBackward)
	expectState(t, "delete selection", d.e, "ab|e")
	r := d.kind(Undo)
	expectState(t, "undo restores the selection", d.e, "ab[cd]e<")
	if r.InputType != InputHistoryUndo || !r.ValueChanged || !r.SelectionChanged {
		t.Errorf("undo result: %+v", r)
	}
	// Backspace runs restore the caret where they started.
	d = parse(t, "abcd|", Options{})
	d.kind(DeleteBackward)
	d.kind(DeleteBackward)
	d.kind(Undo)
	expectState(t, "undo of backspaces", d.e, "abcd|")
	// Nothing to undo: an empty plan, no beforeinput.
	if p := d.e.Plan(Command{Kind: Undo}, nil); !p.Empty() || p.InputType != "" {
		t.Errorf("undo with an empty history: %+v", p)
	}
}

// TestUndoLimit checks the history depth.
func TestUndoLimit(t *testing.T) {
	d := parse(t, "|", Options{UndoLimit: 2})
	for _, s := range []string{"a", "b", "c"} {
		d.do(Command{Kind: Paste, Text: s})
	}
	eqStrings(t, "limit", undoAll(d), []string{"ab|", "a|"})
	d = parse(t, "|", Options{UndoLimit: -1})
	d.typ("abc")
	if d.e.CanUndo() {
		t.Error("history kept with UndoLimit < 0")
	}
}

// TestSetText checks programmatic values and their history modes.
func TestSetText(t *testing.T) {
	d := parse(t, "|", Options{})
	d.typ("hello")
	d.kind(MoveLeft)
	// Echoing the current value back changes nothing.
	if r := d.e.SetText("hello", ResetHistory); r != (Result{}) {
		t.Errorf("echo: %+v", r)
	}
	expectState(t, "echo keeps the selection", d.e, "hell|o")
	if !d.e.CanUndo() {
		t.Error("echo reset the history")
	}
	// A new value: caret at the end, history cleared, no input type.
	r := d.e.SetText("bye", ResetHistory)
	if !r.ValueChanged || !r.DisplayChanged || r.InputType != "" {
		t.Errorf("SetText: %+v", r)
	}
	expectState(t, "SetText", d.e, "bye|")
	if d.e.CanUndo() {
		t.Error("ResetHistory kept the history")
	}
	// RecordHistory: one undoable step.
	d.typ("!")
	d.e.SetText("BYE!", RecordHistory)
	eqStrings(t, "RecordHistory", undoAll(d), []string{"bye!|", "bye|"})
	// A plan made before SetText is stale.
	d = parse(t, "ab|", Options{})
	p := d.e.Plan(Command{Kind: InsertText, Text: "c"}, nil)
	d.e.SetText("xy", ResetHistory)
	if r := d.e.Apply(p); r != (Result{}) {
		t.Errorf("stale plan applied: %+v", r)
	}
	expectState(t, "stale plan", d.e, "xy|")
}

// TestPlan checks that planning changes nothing and that a canceled plan
// is simply not applied.
func TestPlan(t *testing.T) {
	d := parse(t, "a[b]c", Options{})
	ver := d.e.DisplayVersion()
	p := d.e.Plan(Command{Kind: InsertText, Text: "x"}, nil)
	if p.InputType != InputInsertText || p.Data != "x" || p.Empty() {
		t.Errorf("plan: %+v", p)
	}
	expectState(t, "after planning", d.e, "a[b]c")
	if d.e.DisplayVersion() != ver {
		t.Error("planning changed the display")
	}
	// Movements have no input type.
	if p := d.e.Plan(Command{Kind: MoveRight}, d.layout()); p.InputType != "" || p.Empty() {
		t.Errorf("movement plan: %+v", p)
	}
	// A plan of another editor does nothing.
	other := New("zz", Options{})
	if r := other.Apply(p); r != (Result{}) {
		t.Errorf("foreign plan: %+v", r)
	}
	// Applying the plan.
	r := d.e.Apply(d.e.Plan(Command{Kind: InsertText, Text: "x"}, nil))
	if !r.ValueChanged || r.InputType != InputInsertText {
		t.Errorf("apply: %+v", r)
	}
	expectState(t, "applied", d.e, "ax|c")
	if d.e.DisplayVersion() == ver {
		t.Error("an edit kept the display version")
	}
}

// TestSetOptions checks option changes at runtime.
func TestSetOptions(t *testing.T) {
	d := parse(t, "ab|\ncd", Options{Multiline: true})
	d.typ("x")
	r := d.e.SetOptions(Options{})
	if !r.ValueChanged || d.e.Value() != "abxcd" || d.e.CanUndo() {
		t.Errorf("to single-line: %+v %q", r, d.e.Value())
	}
	d = parse(t, "a[bc]", Options{})
	r = d.e.SetOptions(Options{Password: true})
	if !r.DisplayChanged || d.e.Display() != "•••" {
		t.Errorf("to password: %+v %q", r, d.e.Display())
	}
	expectState(t, "selection kept", d.e, "a[bc]")
}

// TestUndoLimitChange checks that lowering the limit drops the oldest
// steps and that ClearHistory empties both stacks.
func TestUndoLimitChange(t *testing.T) {
	d := parse(t, "|", Options{})
	for _, s := range []string{"a", "b", "c", "d"} {
		d.do(Command{Kind: Paste, Text: s})
	}
	d.kind(Undo)
	d.e.SetOptions(Options{UndoLimit: 1})
	eqStrings(t, "trimmed", undoAll(d), []string{"ab|"})
	d.kind(Redo)
	d.e.ClearHistory()
	if d.e.CanUndo() || d.e.CanRedo() {
		t.Error("ClearHistory kept steps")
	}
}
