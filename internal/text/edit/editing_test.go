package edit

import (
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// TestInsert checks typing and pasting over carets and selections, with
// the reported changes.
func TestInsert(t *testing.T) {
	d := parse(t, "ab|cd", Options{})
	r := d.do(Command{Kind: InsertText, Text: "X"})
	expectState(t, "type", d.e, "abX|cd")
	if !r.ValueChanged || !r.SelectionChanged || !r.DisplayChanged || r.InputType != InputInsertText || r.Data != "X" || r.IsComposing {
		t.Errorf("typing result: %+v", r)
	}
	d = parse(t, "a[bc]d", Options{})
	r = d.do(Command{Kind: Paste, Text: "日本語"})
	expectState(t, "paste over a selection", d.e, "a日本語|d")
	if r.InputType != InputInsertFromPaste || r.Data != "日本語" {
		t.Errorf("paste result: %+v", r)
	}
	// The caret after an insertion is attached to the inserted text.
	if f := d.e.Selection().Focus; f.Offset != len("a日本語") || f.Affinity != text.Upstream {
		t.Errorf("caret after insertion: %+v", f)
	}
	// Empty and fully stripped text inserts nothing.
	d = parse(t, "a[b]c", Options{})
	if r := d.do(Command{Kind: InsertText, Text: "\x00\x01"}); r != (Result{}) {
		t.Errorf("control-only insertion changed something: %+v", r)
	}
	expectState(t, "control-only insertion", d.e, "a[b]c")
	// Arabic, combining marks and emoji are inserted as they are.
	d = parse(t, "|", Options{})
	d.typ("مرحبا e\u0301 👨\u200d👩\u200d👧")
	expectState(t, "mixed scripts", d.e, "مرحبا e\u0301 👨\u200d👩\u200d👧|")
}

// TestSanitize checks the newline and control character rules.
func TestSanitize(t *testing.T) {
	tests := []struct {
		name      string
		multiline bool
		in, want  string
	}{
		{"single-line line breaks become spaces", false, "a\r\nb\nc\rd", "a b c d|"},
		{"multi-line line breaks become LF", true, "a\r\nb\nc\rd", "a\nb\nc\nd|"},
		{"controls are removed, tab kept", false, "a\x00b\x7fc\u0085d\te\x1b", "abcd\te|"},
		{"invalid UTF-8", false, "a\xffb", "a�b|"},
	}
	for _, tc := range tests {
		d := parse(t, "|", Options{Multiline: tc.multiline})
		d.do(Command{Kind: Paste, Text: tc.in})
		expectState(t, tc.name, d.e, tc.want)
	}
	// Enter: a line feed in a textarea, nothing in an input.
	d := parse(t, "ab|", Options{})
	if r := d.kind(InsertLineBreak); r != (Result{}) {
		t.Errorf("Enter in a single-line field: %+v", r)
	}
	d = parse(t, "ab|", Options{Multiline: true})
	if r := d.kind(InsertLineBreak); r.InputType != InputInsertLineBreak || r.Data != "\n" {
		t.Errorf("Enter in a textarea: %+v", r)
	}
	expectState(t, "Enter in a textarea", d.e, "ab\n|")
	// SetText: HTML value sanitization.
	e := New("a\r\nb\nc", Options{})
	if e.Value() != "abc" {
		t.Errorf("single-line value: %q", e.Value())
	}
	e = New("a\r\nb\rc\x00", Options{Multiline: true})
	if e.Value() != "a\nb\nc\x00" {
		t.Errorf("multi-line value: %q", e.Value())
	}
}

// TestMaxLength checks maxlength in UTF-16 code units, truncated on
// grapheme cluster boundaries.
func TestMaxLength(t *testing.T) {
	tests := []struct {
		name, start, insert, want string
		max                       int
	}{
		{"ASCII truncated", "abc|", "defgh", "abcde|", 5},
		{"astral characters count two", "ab|", "😀😀", "ab😀|", 4},
		{"no room for half a surrogate pair", "abc|", "😀", "abc|", 4},
		{"a ZWJ sequence is not split", "|", "a👨\u200d👩\u200d👧", "a|", 6},
		{"a combining sequence is not split", "|", "abe\u0301", "ab|", 3},
		{"the replaced selection counts as room", "a[bcd]e", "XYZW", "aXYZ|e", 5},
		{"full: nothing inserted", "abcde|", "f", "abcde|", 5},
	}
	for _, tc := range tests {
		d := parse(t, tc.start, Options{MaxLength: tc.max})
		d.do(Command{Kind: Paste, Text: tc.insert})
		expectState(t, tc.name, d.e, tc.want)
	}
	// A longer value set by the program is kept; the user can delete but
	// not insert.
	d := newDriver(t, "abcdefg", Options{MaxLength: 5})
	if d.e.Value() != "abcdefg" {
		t.Errorf("SetText was truncated: %q", d.e.Value())
	}
	d.typ("x")
	expectState(t, "over the limit", d.e, "abcdefg|")
	d.kind(DeleteBackward)
	expectState(t, "deleting over the limit", d.e, "abcdef|")
	// The plan's data is what will be inserted.
	d = parse(t, "abc|", Options{MaxLength: 4})
	if p := d.e.Plan(Command{Kind: Paste, Text: "xyz"}, nil); p.Data != "x" || p.InputType != InputInsertFromPaste {
		t.Errorf("plan of a truncated paste: %+v", p)
	}
}

// TestBackspace checks Blink's Backspace rules and Delete by grapheme
// cluster.
func TestBackspace(t *testing.T) {
	tests := []struct {
		name, start, want string
		password          bool
	}{
		{"Latin", "ab|", "a|", false},
		{"combining accent goes first", "xe\u0301|", "xe|", false},
		{"Devanagari vowel sign", "कि|", "क|", false},
		{"Thai vowel", "กิ|", "ก|", false},
		{"Indic ZWJ is a code point", "क्\u200d|", "क्|", false},
		{"ZWJ family", "a👨\u200d👩\u200d👧|", "a|", false},
		{"flag", "a🇫🇷|", "a|", false},
		{"keycap", "a1\ufe0f\u20e3|", "a|", false},
		{"skin tone", "a👍🏽|", "a|", false},
		{"tag sequence", "a🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F|", "a|", false},
		{"variation selector with its base", "a葛\U000E0100|", "a|", false},
		{"emoji presentation selector", "a❤\ufe0f|", "a|", false},
		{"CJK", "日本|", "日|", false},
		{"line feed", "a\n|", "a|", false},
		{"password: whole grapheme", "xe\u0301|", "x|", true},
		{"start of text", "|ab", "|ab", false},
		{"selection", "a[bc]d", "a|d", false},
	}
	for _, tc := range tests {
		d := parse(t, tc.start, Options{Multiline: true, Password: tc.password})
		d.kind(DeleteBackward)
		expectState(t, tc.name, d.e, tc.want)
	}
	forward := []struct{ name, start, want string }{
		{"Latin", "|ab", "|b"},
		{"combining sequence", "|e\u0301x", "|x"},
		{"ZWJ family", "|👨\u200d👩\u200d👧a", "|a"},
		{"flag pair", "|🇫🇷🇩🇪", "|🇩🇪"},
		{"end of text", "ab|", "ab|"},
		{"selection", "[ab]c<", "|c"},
	}
	for _, tc := range forward {
		d := parse(t, tc.start, Options{})
		r := d.kind(DeleteForward)
		expectState(t, "Delete: "+tc.name, d.e, tc.want)
		if r.ValueChanged && r.InputType != InputDeleteContentForward {
			t.Errorf("Delete: %s: input type %q", tc.name, r.InputType)
		}
	}
}

// TestReadOnly checks that read-only fields refuse edits but keep
// selection and copy.
func TestReadOnly(t *testing.T) {
	d := parse(t, "a[bc]d", Options{ReadOnly: true})
	for _, k := range []CommandKind{InsertText, Paste, DeleteBackward, DeleteForward, DeleteWordBackward, Cut, Undo} {
		if r := d.do(Command{Kind: k, Text: "x"}); r != (Result{}) {
			t.Errorf("command %d in a read-only field: %+v", k, r)
		}
	}
	if r := d.kind(Copy); r.Clipboard != "bc" {
		t.Errorf("copy in a read-only field: %+v", r)
	}
	if r := d.kind(MoveRight); !r.SelectionChanged {
		t.Errorf("navigation in a read-only field: %+v", r)
	}
	if r := d.e.SetPreedit("x", 1, 1); r != (Result{}) {
		t.Errorf("preedit in a read-only field: %+v", r)
	}
	expectState(t, "read-only", d.e, "abc|d")
}

// TestCopyCut checks the clipboard commands.
func TestCopyCut(t *testing.T) {
	d := parse(t, "a[bc]d", Options{})
	p := d.e.Plan(Command{Kind: Copy}, nil)
	if p.Clipboard != "bc" || p.InputType != InputNone || p.Empty() {
		t.Errorf("copy plan: %+v", p)
	}
	if r := d.e.Apply(p); r.Clipboard != "bc" || r.ValueChanged || r.SelectionChanged {
		t.Errorf("copy: %+v", r)
	}
	r := d.kind(Cut)
	if r.Clipboard != "bc" || r.InputType != InputDeleteByCut || !r.ValueChanged {
		t.Errorf("cut: %+v", r)
	}
	expectState(t, "cut", d.e, "a|d")
	// Nothing to copy from a caret.
	if p := d.e.Plan(Command{Kind: Copy}, nil); !p.Empty() {
		t.Errorf("copy of a caret: %+v", p)
	}
}
