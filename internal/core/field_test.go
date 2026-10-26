package core

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
	"github.com/kuronosu/tela/internal/text/edit"
)

func input(c ...Child) Node    { return Primitive(style.KindInput, c...) }
func textarea(c ...Child) Node { return Primitive(style.KindTextarea, c...) }
func form(c ...Child) Node     { return Primitive(style.KindForm, c...) }
func label(c ...Child) Node    { return Primitive(style.KindLabel, c...) }

// fieldOf returns the field of the primitive with key k.
func fieldOf(t *testing.T, tr *Tree, k any) *field {
	t.Helper()
	el := keyed(tr, k)
	if el == nil || el.box.fld == nil {
		t.Fatalf("no text field %v", k)
	}
	return el.box.fld
}

func (x *inputHarness) value(k any) string { return fieldOf(x.t, x.tree(), k).ed.RawValue() }

func (x *inputHarness) selection(k any) (int, int) {
	s, e, _ := fieldOf(x.t, x.tree(), k).ed.SelectionRange()
	return s, e
}

// typeText types s one character at a time (key presses with text).
func (x *inputHarness) typeText(s string) {
	for _, r := range s {
		k := platform.Key(string(r))
		x.keys(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeUnidentified, Key: k, Text: string(r)},
			platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeUnidentified, Key: k})
	}
}

// key presses a named key with modifiers.
func (x *inputHarness) key(k platform.Key, mods platform.Modifiers) {
	c, ok := platform.ParseKeyCode(string(k))
	if !ok {
		c = platform.CodeUnidentified
	}
	x.press(k, c, mods)
}

// ctrl presses Ctrl (and more modifiers) with a letter.
func (x *inputHarness) ctrl(letter string, more platform.Modifiers) {
	c, _ := platform.ParseKeyCode("Key" + strings.ToUpper(letter))
	k := platform.Key(letter)
	if more&platform.ModShift != 0 {
		k = platform.Key(strings.ToUpper(letter))
	}
	e := platform.KeyEvent{Action: platform.KeyPress, Code: c, Key: k, Text: string(k), Mods: platform.ModCtrl | more}
	x.keys(e, platform.KeyEvent{Action: platform.KeyRelease, Code: c, Key: k, Mods: platform.ModCtrl | more})
}

func TestFieldTypingUncontrolled(t *testing.T) {
	var changes []string
	root := div(Class("p-4"), input(Key("in"), DefaultValue("ab"), On("change", func(e ChangeEvent) {
		changes = append(changes, e.InputType+":"+e.Value)
	})))
	x := newKeys(t, root, 400, 200)
	x.frame()
	if v := x.value("in"); v != "ab" {
		t.Fatalf("initial value %q", v)
	}
	if s, e := x.selection("in"); s != 0 || e != 0 {
		t.Errorf("initial selection %d %d, want the caret at the start (Chromium)", s, e)
	}
	x.tree().FocusNode(0) // no-op on an unknown node
	x.tab()
	if focusedName(x.tree()) != "in" {
		t.Fatalf("Tab focused %s", focusedName(x.tree()))
	}
	if s, e := x.selection("in"); s != 0 || e != 2 {
		t.Errorf("Tab selected %d..%d, want the whole value", s, e)
	}
	x.typeText("xy")
	x.key(platform.KeyArrowLeft, 0)
	x.typeText("Z")
	if v := x.value("in"); v != "xZy" {
		t.Fatalf("value %q", v)
	}
	x.key(platform.KeyHome, 0)
	x.key(platform.KeyDelete, 0)
	x.key(platform.KeyEnd, 0)
	x.key(platform.KeyBackspace, 0)
	if v := x.value("in"); v != "Z" {
		t.Fatalf("after Delete and Backspace %q", v)
	}
	x.ctrl("z", 0)
	if v := x.value("in"); v != "Zy" {
		t.Errorf("undo: %q", v)
	}
	x.ctrl("y", 0)
	if v := x.value("in"); v != "Z" {
		t.Errorf("redo (Ctrl+Y): %q", v)
	}
	x.ctrl("z", 0)
	x.ctrl("z", platform.ModShift)
	if v := x.value("in"); v != "Z" {
		t.Errorf("redo (Ctrl+Shift+Z): %q", v)
	}
	x.ctrl("a", 0)
	x.typeText("new")
	if v := x.value("in"); v != "new" {
		t.Errorf("Ctrl+A then typing: %q", v)
	}
	want := "insertText:xab insertText:xyab"
	if got := strings.Join(changes[:2], " "); !strings.HasPrefix(got, "insertText:x") {
		t.Errorf("changes %q, want %q...", got, want)
	}
	if last := changes[len(changes)-1]; last != "insertText:new" {
		t.Errorf("last change %q", last)
	}
	// Enter in an input outside a form does nothing; Tab moves on.
	x.enter()
	if v := x.value("in"); v != "new" {
		t.Errorf("Enter changed the value: %q", v)
	}
}

func TestFieldTypingSequence(t *testing.T) {
	var ev []string
	r := func(name string) Prop {
		switch name {
		case "beforeinput":
			return On(name, func(e InputEvent) { ev = append(ev, "before:"+e.InputType+":"+e.Data) })
		case "change":
			return On(name, func(e ChangeEvent) { ev = append(ev, "change:"+e.InputType+":"+e.Value) })
		case "keydown":
			return On(name, func(e KeyboardEvent) { ev = append(ev, "keydown:"+string(e.Key)) })
		}
		return On(name, func(e KeyboardEvent) { ev = append(ev, "keyup:"+string(e.Key)) })
	}
	root := input(Key("in"), r("beforeinput"), r("change"), r("keydown"), r("keyup"))
	x := newKeys(t, root, 300, 100)
	x.tab()
	ev = nil
	x.typeText("a")
	if got, want := strings.Join(ev, " "), "keydown:a before:insertText:a change:insertText:a keyup:a"; got != want {
		t.Errorf("events %q, want %q (the DOM's order)", got, want)
	}
}

func TestFieldBeforeInputCancel(t *testing.T) {
	root := input(Key("in"), On("beforeinput", func(e InputEvent) {
		if strings.Trim(e.Data, "0123456789") != "" {
			e.PreventDefault()
		}
	}))
	x := newKeys(t, root, 300, 100)
	x.tab()
	x.typeText("a1b2")
	if v := x.value("in"); v != "12" {
		t.Errorf("value %q, want only the digits", v)
	}
	// keydown PreventDefault cancels the text too.
	root2 := input(Key("in"), On("keydown", func(e KeyboardEvent) {
		if e.Key == "x" {
			e.PreventDefault()
		}
	}))
	y := newKeys(t, root2, 300, 100)
	y.tab()
	y.typeText("axb")
	if v := y.value("in"); v != "ab" {
		t.Errorf("value %q after a prevented keydown", v)
	}
}

func TestFieldControlled(t *testing.T) {
	// A controlled value without an update is restored after every
	// event (React).
	var setX Setter[string]
	builds := 0
	app := func() Node {
		builds++
		v, set := UseState("fixed")
		setX = set
		return input(Key("in"), FieldValue(v), On("change", func(ChangeEvent) {}))
	}
	x := newKeys(t, Component(app), 300, 100)
	x.tab()
	x.typeText("abc")
	if v := x.value("in"); v != "fixed" {
		t.Errorf("an unchanged controlled value became %q", v)
	}
	// The app's value wins, with the caret at the end.
	setX.Set("from app")
	x.frame()
	if v := x.value("in"); v != "from app" {
		t.Errorf("value %q after a new Value", v)
	}
	if s, e := x.selection("in"); s != 8 || e != 8 {
		t.Errorf("caret %d %d, want the end after a programmatic value", s, e)
	}

	// A transforming controlled field (upper case).
	up := func() Node {
		v, set := UseState("")
		return input(Key("in"), FieldValue(v), On("change", func(e ChangeEvent) { set.Set(strings.ToUpper(e.Value)) }))
	}
	y := newKeys(t, Component(up), 300, 100)
	y.tab()
	y.typeText("hey")
	if v := y.value("in"); v != "HEY" {
		t.Errorf("transformed value %q", v)
	}

	// Bind.
	var got string
	bind := func() Node {
		v, set := UseState("")
		got = v
		return input(Key("in"), Bind(v, set))
	}
	z := newKeys(t, Component(bind), 300, 100)
	z.tab()
	z.typeText("bound")
	z.frame()
	if got != "bound" || z.value("in") != "bound" {
		t.Errorf("Bind: state %q, field %q", got, z.value("in"))
	}
}

func TestFieldBindValue(t *testing.T) {
	var v *Value[string]
	builds := 0
	var readerBuilds int
	reader := func() Node {
		readerBuilds++
		return p(Key("p"), Text("len="+fmt.Sprint(len(v.Get()))))
	}
	app := func() Node {
		builds++
		v = UseValue("hi")
		return div(input(Key("in"), BindValue(v)), Component(reader))
	}
	x := newKeys(t, Component(app), 300, 100)
	x.frame()
	if x.value("in") != "hi" {
		t.Fatalf("value %q", x.value("in"))
	}
	b0, r0 := builds, readerBuilds
	x.tab()
	x.key(platform.KeyEnd, 0)
	x.typeText("!!")
	x.frame()
	if got := v.Get(); got != "hi!!" {
		t.Errorf("v = %q", got)
	}
	if builds != b0 {
		t.Errorf("typing rebuilt the owner %d times", builds-b0)
	}
	if readerBuilds == r0 {
		t.Error("the component that reads v was not rebuilt")
	}
	v.Set("set from code")
	x.frame()
	if x.value("in") != "set from code" {
		t.Errorf("field %q after v.Set", x.value("in"))
	}
	if builds != b0 {
		t.Errorf("v.Set rebuilt the owner")
	}
}

func TestFieldMaxLengthAndInvalid(t *testing.T) {
	root := div(
		input(Key("in"), MaxLength(3), FieldValue("toolong"), On("change", func(ChangeEvent) {})),
		input(Key("mail"), FieldType("email"), DefaultValue("bad")),
		input(Key("url"), FieldType("url"), DefaultValue("https://x.y")),
	)
	x := newKeys(t, root, 400, 200)
	x.frame()
	tr := x.tree()
	in := keyed(tr, "in")
	if in.istate&style.StateInvalid != 0 {
		t.Error("a programmatic value over maxlength is invalid (HTML: only user edits)")
	}
	if keyed(tr, "mail").istate&style.StateInvalid == 0 {
		t.Error(`email "bad" is not invalid`)
	}
	if keyed(tr, "url").istate&style.StateInvalid != 0 {
		t.Error("a valid URL is invalid")
	}
	// Typing is limited.
	u := newKeys(t, input(Key("u"), MaxLength(3)), 300, 100)
	u.tab()
	u.typeText("abcdef")
	if v := u.value("u"); v != "abc" {
		t.Errorf("maxlength: %q", v)
	}
	for _, c := range []struct {
		s  string
		ok bool
	}{{"a@b", true}, {"ana@example.com", true}, {"a@b.", false}, {"@b", false}, {"a@-b", false}, {"a b@c", false}, {"a@b@c", false}} {
		if validEmail(c.s) != c.ok {
			t.Errorf("validEmail(%q) = %v", c.s, !c.ok)
		}
	}
	for _, c := range []struct {
		s  string
		ok bool
	}{{"a:b", true}, {"http://", false}, {"foo", false}, {"https://x.y", true}, {"mailto:a@b", true}} {
		if validURL(c.s) != c.ok {
			t.Errorf("validURL(%q) = %v", c.s, !c.ok)
		}
	}
}

// TestFieldInvalidBarredFromValidation checks that a read-only or
// disabled field never matches invalid: (HTML bars it from constraint
// validation; Chromium's :invalid agrees), whether the field itself is
// ReadOnly or Disabled or an ancestor is Disabled, and that it matches
// again once it is editable. The ancestor case keeps the field's node
// unchanged, so only the ancestor's update can tell it.
func TestFieldInvalidBarredFromValidation(t *testing.T) {
	var set Setter[int]
	inner := input(Key("inner"), AriaLabel("inner"), FieldType("email"), DefaultValue("bad"))
	app := func() Node {
		n, s := UseState(0)
		set = s
		return div(
			input(Key("ro"), AriaLabel("ro"), FieldType("email"), DefaultValue("bad"), ReadOnly(n == 1)),
			input(Key("dis"), AriaLabel("dis"), FieldType("url"), DefaultValue("nope"), Disabled(n == 1)),
			div(Key("set"), Disabled(n == 1), inner),
		)
	}
	x := newKeys(t, Component(app), 400, 300)
	x.frame()
	tr := x.tree()
	invalid := func(k string) bool { return keyed(tr, k).istate&style.StateInvalid != 0 }
	keys := []string{"ro", "dis", "inner"}
	for _, k := range keys {
		if !invalid(k) {
			t.Errorf("%s: an editable field with a bad value is not invalid", k)
		}
	}
	set.Set(1)
	x.frame()
	for _, k := range keys {
		if invalid(k) {
			t.Errorf("%s: a read-only or disabled field matches invalid:", k)
		}
	}
	if keyed(tr, "inner").istate&style.StateReadOnly == 0 {
		t.Error("a field inside a disabled element does not match read-only:")
	}
	for _, k := range keys {
		if mustNode(t, tr, "textbox", k).Has(semantics.StateInvalid) {
			t.Errorf("%s: the semantics node is invalid", k)
		}
	}
	set.Set(0)
	x.frame()
	for _, k := range keys {
		if !invalid(k) {
			t.Errorf("%s: invalid: does not match again once the field is editable", k)
		}
	}
	if keyed(tr, "inner").istate&style.StateReadOnly != 0 {
		t.Error("a field no longer inside a disabled element still matches read-only:")
	}
}

func TestFieldPasswordAndClipboard(t *testing.T) {
	root := div(input(Key("pw"), FieldType("password"), DefaultValue("secret")), input(Key("txt"), DefaultValue("copy me")))
	x := newKeys(t, root, 400, 200)
	tr := x.tree()
	x.frame()
	pw := fieldOf(t, tr, "pw")
	if pw.ed.Display() != strings.Repeat("•", 6) {
		t.Errorf("display %q", pw.ed.Display())
	}
	clip := x.b.Clipboard()
	clip.WriteText("before")
	x.tab()
	x.ctrl("c", 0)
	x.ctrl("x", 0)
	x.b.Drain(x.h)
	if v := pw.ed.Value(); v != "secret" {
		t.Errorf("cut a password: %q", v)
	}
	read := func() string {
		var got string
		clip.ReadText(func(s string, err error) { got = s })
		x.b.Drain(x.h)
		return got
	}
	if got := read(); got != "before" {
		t.Errorf("copying a password wrote %q", got)
	}
	x.tab()
	x.ctrl("c", 0) // Tab selected all of "copy me"
	if got := read(); got != "copy me" {
		t.Errorf("Ctrl+C wrote %q", got)
	}
	x.key(platform.KeyEnd, 0)
	x.key(platform.KeyInsert, platform.ModShift)
	x.b.Drain(x.h)
	if v := x.value("txt"); v != "copy mecopy me" {
		t.Errorf("Shift+Insert pasted into %q", v)
	}
	x.key(platform.KeyHome, platform.ModShift)
	x.key(platform.KeyDelete, platform.ModShift) // cut
	if v := x.value("txt"); v != "" {
		t.Errorf("Shift+Delete left %q", v)
	}
	if got := read(); got != "copy mecopy me" {
		t.Errorf("Shift+Delete wrote %q", got)
	}
	// A paste that arrives after the focus moved is dropped.
	clip.WriteText("late")
	cv, _ := platform.ParseKeyCode("KeyV")
	x.w.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: cv, Key: "v", Text: "v", Mods: platform.ModCtrl})
	x.w.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: cv, Key: "v", Mods: platform.ModCtrl})
	x.w.Inject(keyEv(platform.KeyPress, platform.KeyTab, platform.CodeTab, platform.ModShift))
	x.w.Inject(keyEv(platform.KeyRelease, platform.KeyTab, platform.CodeTab, platform.ModShift))
	x.b.Drain(x.h) // the clipboard answers after the blur
	if v := x.value("txt"); v != "" {
		t.Errorf("a paste after blur reached the field: %q", v)
	}
	// Paste is one insertFromPaste edit with line breaks as spaces.
	var types []string
	y := newKeys(t, input(Key("p"), On("beforeinput", func(e InputEvent) { types = append(types, e.InputType) })), 300, 100)
	y.b.Clipboard().WriteText("a\nb")
	y.tab()
	y.ctrl("v", 0)
	y.b.Drain(y.h)
	if v := y.value("p"); v != "a b" || strings.Join(types, ",") != "insertFromPaste" {
		t.Errorf("paste: %q %v", v, types)
	}
}

func TestFieldTextarea(t *testing.T) {
	root := div(Class("p-2"), textarea(Key("ta"), Class("h-16 w-40")))
	x := newKeys(t, root, 300, 200)
	x.tab()
	x.typeText("one")
	x.enter()
	x.typeText("two")
	if v := x.value("ta"); v != "one\ntwo" {
		t.Fatalf("value %q", v)
	}
	x.key(platform.KeyArrowUp, 0)
	x.typeText("!")
	if v := x.value("ta"); v != "one!\ntwo" {
		t.Errorf("ArrowUp then typing: %q", v)
	}
	for range 8 {
		x.enter()
		x.typeText("line")
	}
	x.key(platform.KeyEnd, platform.ModCtrl) // "two", the last line
	x.frame()
	s := keyed(x.tree(), "ta").box.scr
	if s == nil || s.max.Y <= 0 {
		t.Fatalf("the textarea does not scroll: %+v", s)
	}
	if s.pos.Y != s.max.Y {
		t.Errorf("the caret on the last line is not in view: offset %v of %v", s.pos.Y, s.max.Y)
	}
	x.ctrl("a", 0)
	x.key(platform.KeyBackspace, 0)
	x.frame()
	if s.max.Y != 0 || s.pos.Y != 0 {
		t.Errorf("after clearing: offset %v max %v", s.pos.Y, s.max.Y)
	}
}

func TestFieldBindings(t *testing.T) {
	k := func(key platform.Key, mods platform.Modifiers) KeyboardEvent {
		e := KeyboardEvent{Key: KeyValue(key)}
		e.Shift, e.Ctrl, e.Alt, e.Meta = modifiers(mods)
		if c, ok := platform.ParseKeyCode("Key" + strings.ToUpper(string(key))); ok {
			e.Code = CodeValue(c.String())
		}
		return e
	}
	for _, c := range []struct {
		fl    keyFlavor
		multi bool
		e     KeyboardEvent
		txt   string
		kind  edit.CommandKind
		act   fieldAction
	}{
		{flavorLinux, false, k(platform.KeyArrowLeft, platform.ModCtrl), "", edit.MoveWordLeft, actCommand},
		{flavorWindows, false, k(platform.KeyArrowRight, platform.ModCtrl|platform.ModShift), "", edit.MoveWordRight, actCommand},
		{flavorLinux, false, k(platform.KeyArrowUp, 0), "", edit.MoveDocumentStart, actCommand},
		{flavorWindows, false, k(platform.KeyArrowUp, 0), "", 0, actConsume},
		{flavorLinux, true, k(platform.KeyPageDown, 0), "", edit.MovePageDown, actCommand},
		{flavorLinux, false, k(platform.KeyHome, platform.ModCtrl), "", edit.MoveDocumentStart, actCommand},
		{flavorLinux, false, k(platform.KeyBackspace, platform.ModCtrl), "", edit.DeleteWordBackward, actCommand},
		{flavorLinux, false, k(platform.KeyDelete, platform.ModShift), "", edit.Cut, actCommand},
		{flavorLinux, false, k(platform.KeyInsert, platform.ModCtrl), "", edit.Copy, actCommand},
		{flavorLinux, false, k(platform.KeyInsert, platform.ModShift), "", 0, actPaste},
		{flavorLinux, false, k(platform.KeyEnter, 0), "", 0, actSubmit},
		{flavorLinux, true, k(platform.KeyEnter, platform.ModShift), "", edit.InsertLineBreak, actCommand},
		{flavorLinux, false, k(platform.KeyTab, 0), "", 0, actNone},
		{flavorLinux, false, k("a", platform.ModCtrl), "a", edit.SelectAll, actCommand},
		{flavorLinux, false, k("Z", platform.ModCtrl|platform.ModShift), "Z", edit.Redo, actCommand},
		{flavorLinux, false, k("x", platform.ModCtrl|platform.ModAlt), "x", 0, actNone},
		{flavorLinux, false, k("é", 0), "é", edit.InsertText, actCommand},
		{flavorLinux, false, k("é", platform.ModSuper), "é", 0, actNone},
		// Alt chords type nothing on Linux and Windows (GTK accelerators,
		// WM_SYSCHAR); AltGr is never reported as Alt and types.
		{flavorLinux, false, k("x", platform.ModAlt), "x", 0, actNone},
		{flavorWindows, false, k("x", platform.ModAlt), "x", 0, actNone},
		{flavorWindows, false, k("X", platform.ModAlt|platform.ModShift), "X", 0, actNone},
		{flavorWindows, false, k("@", 0), "@", edit.InsertText, actCommand},
		// Option types characters on macOS.
		{flavorMac, false, k("å", platform.ModAlt), "å", edit.InsertText, actCommand},
		{flavorMac, false, k(platform.KeyArrowLeft, platform.ModAlt), "", edit.MoveWordLeft, actCommand},
		{flavorMac, false, k("c", platform.ModSuper), "c", edit.Copy, actCommand},
		{flavorMac, false, k(platform.KeyBackspace, platform.ModSuper), "", edit.DeleteToLineStart, actCommand},
	} {
		cmd, act := fieldBinding(c.fl, c.multi, c.e, c.txt)
		if act != c.act || act == actCommand && cmd.Kind != c.kind {
			t.Errorf("%v %+v: got %v %v, want %v %v", c.fl, c.e, cmd.Kind, act, c.kind, c.act)
		}
	}
	// A shortcut on a non-Latin layout uses the physical key.
	e := KeyboardEvent{Key: "я", Code: "KeyZ", Ctrl: true}
	if cmd, _ := fieldBinding(flavorLinux, false, e, "я"); cmd.Kind != edit.Undo {
		t.Errorf("Ctrl+я (KeyZ) = %v, want undo", cmd.Kind)
	}
}

func TestFieldPointer(t *testing.T) {
	root := div(Class("p-4"), input(Key("in"), Class("w-64"), DefaultValue("hello brave new world")))
	x := newKeys(t, root, 400, 100)
	x.frame()
	f := fieldOf(t, x.tree(), "in")
	r := windowRect(f.bx)
	lay := f.layout()
	// Click after "hello": the caret goes there and the field is focused.
	cx := r.X0 + lay.Caret(textCursor(5)).X
	cy := r.Center().Y
	x.click(cx+1, cy)
	if focusedName(x.tree()) != "in" {
		t.Fatalf("click focused %s", focusedName(x.tree()))
	}
	if s, e := x.selection("in"); s != 5 || e != 5 {
		t.Errorf("caret %d %d after a click after hello", s, e)
	}
	if !x.tree().foc.visible {
		t.Error("a text field focused by the mouse does not match focus-visible (Chromium: it does)")
	}
	// Shift+click extends.
	x.send(ptr(platform.PointerDown, r.X0+lay.Caret(textCursor(11)).X+1, cy, platform.ButtonPrimary, platform.ButtonPrimary.Mask()))
	x.send(ptr(platform.PointerUp, r.X0+lay.Caret(textCursor(11)).X+1, cy, platform.ButtonPrimary, 0))
	x.b.Advance(time.Second) // not a double click
	pos := r.X0 + lay.Caret(textCursor(11)).X + 1
	x.w.Inject(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: pos, Y: cy}, Button: platform.ButtonPrimary,
		Buttons: platform.ButtonPrimary.Mask(), Mods: platform.ModShift})
	x.w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: pos, Y: cy}, Button: platform.ButtonPrimary, Mods: platform.ModShift})
	x.b.Drain(x.h)
	x.b.Advance(time.Second)
	// Double click selects a word, triple click everything.
	wx := r.X0 + lay.Caret(textCursor(8)).X
	x.click(wx, cy)
	x.click(wx, cy)
	if s, e := x.selection("in"); s != 6 || e != 11 {
		t.Errorf("double click selected %d..%d, want brave (6..11)", s, e)
	}
	x.click(wx, cy)
	if s, e := x.selection("in"); s != 0 || e != 21 {
		t.Errorf("triple click selected %d..%d, want all", s, e)
	}
	// Drag selects.
	x.b.Advance(time.Second)
	x.send(down(r.X0+1, cy), moveHeld(r.X0+lay.Caret(textCursor(5)).X+1, cy), up(r.X0+lay.Caret(textCursor(5)).X+1, cy))
	if s, e := x.selection("in"); s != 0 || e != 5 {
		t.Errorf("drag selected %d..%d", s, e)
	}
}

// countKind counts the layers of kind k.
func countKind(tr *Tree, k layerKind) int {
	n := 0
	for _, l := range tr.lay.layers {
		if l.kind == k {
			n++
		}
	}
	return n
}

// textCursor is a downstream cursor at off.
func textCursor(off int) text.Cursor { return text.Cursor{Offset: off} }

// fieldTree returns a tree with root mounted and laid out.
func fieldTree(t *testing.T, root Node, w, h float32, cfg Config) *Tree {
	t.Helper()
	tr := NewTree(cfg)
	tr.SetViewport(gfx.Size{W: w, H: h})
	tr.Mount(root)
	tr.Frame()
	return tr
}

func TestFieldAutoscroll(t *testing.T) {
	long := strings.Repeat("word ", 40)
	root := div(Class("p-4"), input(Key("in"), Class("w-32"), DefaultValue(long)))
	x := newKeys(t, root, 300, 100)
	x.frame()
	f := fieldOf(t, x.tree(), "in")
	r := windowRect(f.bx)
	cy := r.Center().Y
	x.send(move(r.X0+5, cy), down(r.X0+5, cy), moveHeld(r.X1+40, cy))
	before := f.scrollX
	for range 5 {
		x.frame()
	}
	if f.scrollX <= before {
		t.Fatalf("no autoscroll: %v -> %v", before, f.scrollX)
	}
	_, e := x.selection("in")
	if e < 25 {
		t.Errorf("the selection did not follow the scroll: end %d", e)
	}
	x.send(up(r.X1+40, cy))
	x.settle()
	if x.tree().fieldAutoscrolling() {
		t.Error("still autoscrolling after the release")
	}
}

func TestFieldCaretBlink(t *testing.T) {
	x := newKeys(t, input(Key("in"), DefaultValue("abc")), 300, 100)
	x.tab()
	x.key(platform.KeyEnd, 0)
	x.settle()
	tr := x.tree()
	f := fieldOf(t, tr, "in")
	if !tr.caretShown(f) {
		t.Fatal("no caret after End")
	}
	at, ok := x.h.TimerArmed()
	if !ok {
		t.Fatal("no blink timer")
	}
	if want := tr.caret.since + caretBlink; at != want {
		t.Errorf("blink deadline %v, want %v", at, want)
	}
	paints := tr.Stats().Paints
	layouts := tr.Stats().Layouts
	x.b.Advance(caretBlink)
	x.b.Wake()
	x.b.Drain(x.h)
	x.settle()
	if tr.caretShown(f) {
		t.Error("the caret did not blink off")
	}
	if tr.Stats().Paints == paints || tr.Stats().Layouts != layouts {
		t.Errorf("a blink: paints %d -> %d, layouts %d -> %d (want a paint and no layout)", paints, tr.Stats().Paints, layouts, tr.Stats().Layouts)
	}
	if n := tr.lay.recorded - countKind(tr, layerBars); n != 1 {
		t.Errorf("a blink recorded %d layers besides scrollbars, want only the field's", n)
	}
	// After 10 s without input the caret stays on and no timer is left.
	x.b.Advance(caretIdle)
	x.b.Wake()
	x.b.Drain(x.h)
	x.settle()
	if !tr.caretShown(f) {
		t.Error("the caret is off after it stopped blinking")
	}
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("a timer is still armed after 10 s idle (principle 5)")
	}
	// A selection hides the caret and arms nothing.
	x.key(platform.KeyHome, platform.ModShift)
	x.settle()
	if tr.caretShown(f) {
		t.Error("a caret with a selection")
	}
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("a timer with a selection")
	}
	// An inactive window has no caret.
	x.key(platform.KeyEnd, 0)
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	x.settle()
	if tr.caretShown(f) {
		t.Error("a caret in an inactive window")
	}
}

func TestFieldTypingPaintsOnly(t *testing.T) {
	rows := make([]Child, 0, 200)
	for i := range 200 {
		rows = append(rows, div(Key(i), Class("p-1"), Text(fmt.Sprintf("row %d", i))))
	}
	root := div(input(Key("in"), Class("w-64")), div(rows...))
	x := newKeys(t, root, 400, 300)
	x.tab()
	x.typeText("a")
	x.settle()
	tr := x.tree()
	s0 := tr.Stats()
	x.typeText("b")
	x.settle()
	s1 := tr.Stats()
	if s1.Layouts != s0.Layouts || s1.Builds != s0.Builds || s1.Styles != s0.Styles {
		t.Errorf("a keystroke in an uncontrolled field: layouts +%d builds +%d styles +%d, want none",
			s1.Layouts-s0.Layouts, s1.Builds-s0.Builds, s1.Styles-s0.Styles)
	}
	if n := tr.lay.recorded - countKind(tr, layerBars); n != 1 {
		t.Errorf("a keystroke recorded %d layers besides scrollbars, want the field's only", n)
	}
	if s1.Shapes-s0.Shapes != 1 {
		t.Errorf("a keystroke shaped %d paragraphs, want 1 (the field's)", s1.Shapes-s0.Shapes)
	}
}

func TestFieldDefaultSizes(t *testing.T) {
	root := div(
		div(input(Key("i16"))),
		div(Class("text-sm"), input(Key("i14"))),
		div(Class("text-2xl"), input(Key("i24"))),
		div(textarea(Key("t16"))),
		div(textarea(Key("t4"), Rows(4))),
		div(input(Key("fs"), Class("field-sizing-content"), DefaultValue("Hello world"))),
		div(input(Key("fs0"), Class("field-sizing-content"))),
		div(textarea(Key("tfs"), Class("field-sizing-content"), DefaultValue("One line"))),
		div(input(Key("tall"), Class("h-12"), DefaultValue("Ag"))),
	)
	tr := fieldTree(t, root, 600, 600, Config{Fonts: testFonts})
	size := func(k string) gfx.Size { return keyed(tr, k).box.ld.Layout().Size }
	for _, c := range []struct {
		k    string
		w, h float32
	}{
		{"i16", 248, 24}, {"i14", 218, 20}, {"i24", 372, 32},
		{"t16", 205, 48}, // 220 in Chromium with its 15 px classic scrollbar; Tela's bars overlay
		{"t4", 205, 96},
		{"fs0", 1, 24},
		{"tall", 248, 48},
	} {
		if s := size(c.k); s.W != c.w || s.H != c.h {
			t.Errorf("%s: %vx%v, want %vx%v", c.k, s.W, s.H, c.w, c.h)
		}
	}
	if w := size("fs").W; math.Abs(float64(w-84.5)) > 0.1 {
		t.Errorf("field-sizing-content width %v, want about 84.5 (Chromium)", w)
	}
	if s := size("tfs"); math.Abs(float64(s.W-61.5)) > 0.1 || s.H != 24 {
		t.Errorf("textarea field-sizing-content %v, want about 61.5x24", s)
	}
	// The line of a tall input is centered: with line-height normal, as
	// its height is larger than its line height (Chromium's inner
	// editor, ADR-159): (48 - 20) / 2, the baseline at 14 + 16.
	f := fieldOf(t, tr, "tall")
	if off := f.lineOffset(); off != 14 {
		t.Errorf("line offset %v, want 14", off)
	}
	if b := f.lineOffset() + f.layout().Line(0).Baseline; b != 30 {
		t.Errorf("baseline %v, want 30 (Chromium)", b)
	}
	// Classic scrollbars add Chromium's 15 px gutter to a textarea.
	cl := fieldTree(t, div(textarea(Key("t"))), 600, 200, Config{Fonts: testFonts, ClassicScrollbars: true})
	if s := keyed(cl, "t").box.ld.Layout().Size; s.W != 220 || s.H != 48 {
		t.Errorf("classic textarea %v, want 220x48 (Chromium)", s)
	}
}

func TestFieldIME(t *testing.T) {
	var ev []string
	add := func(s string) { ev = append(ev, s) }
	root := div(input(Key("in"), DefaultValue("ab"),
		On("compositionstart", func(e CompositionEvent) { add("start:" + e.Data) }),
		On("compositionupdate", func(e CompositionEvent) { add("update:" + e.Data) }),
		On("compositionend", func(e CompositionEvent) { add("end:" + e.Data) }),
		On("beforeinput", func(e InputEvent) { add(fmt.Sprintf("before:%s:%s:%v", e.InputType, e.Data, e.IsComposing)) }),
		On("change", func(e ChangeEvent) { add(fmt.Sprintf("change:%s:%s:%v", e.InputType, e.Value, e.IsComposing)) }),
	), input(Key("other")))
	x := newKeys(t, root, 400, 200)
	x.tab()
	x.key(platform.KeyEnd, 0)
	x.settle()
	st := x.w.TextInput()
	if st == nil || st.Purpose != platform.TextPurposeNormal || st.Multiline || st.Surrounding == nil ||
		st.Surrounding.Text != "ab" || st.Surrounding.Cursor != 2 {
		t.Fatalf("text input state %+v", st)
	}
	if st.CursorArea.H != 24 || st.CursorArea.W != 0 {
		t.Errorf("cursor area %+v, want a 24 px tall caret", st.CursorArea)
	}
	if !x.w.IMEActive() {
		t.Fatal("no IME session")
	}
	ime := func(e platform.IMEEvent) {
		x.w.Inject(e)
		x.b.Drain(x.h)
	}
	ev = nil
	ime(platform.IMEEvent{Preedit: "か", CursorStart: 3, CursorEnd: 3})
	ime(platform.IMEEvent{Preedit: "かん", CursorStart: 6, CursorEnd: 6})
	if v := x.value("in"); v != "abかん" {
		t.Errorf("value during composition %q", v)
	}
	// Keys during a composition do nothing.
	x.typeText("z")
	x.key(platform.KeyBackspace, 0)
	ime(platform.IMEEvent{Commit: "漢"})
	want := []string{
		"start:", "before:insertCompositionText:か:true", "update:か", "change:insertCompositionText:abか:true",
		"before:insertCompositionText:かん:true", "update:かん", "change:insertCompositionText:abかん:true",
		"before:insertCompositionText:漢:true", "update:漢", "change:insertCompositionText:ab漢:true", "end:漢",
	}
	if got := strings.Join(ev, " "); got != strings.Join(want, " ") {
		t.Errorf("events\n got %s\nwant %s", got, strings.Join(want, " "))
	}
	if v := x.value("in"); v != "ab漢" {
		t.Errorf("committed value %q", v)
	}
	// Undo removes the whole composition.
	x.ctrl("z", 0)
	if v := x.value("in"); v != "ab" {
		t.Errorf("undo of a composition: %q", v)
	}
	// Cancel.
	ev = nil
	ime(platform.IMEEvent{Preedit: "x", CursorStart: -1, CursorEnd: -1})
	ime(platform.IMEEvent{})
	if v := x.value("in"); v != "ab" || ev[len(ev)-1] != "end:" {
		t.Errorf("cancel: %q %v", v, ev)
	}
	// Delete surrounding text and a commit without a composition.
	ime(platform.IMEEvent{DeleteBefore: 1, Commit: "Z"})
	if v := x.value("in"); v != "aZ" {
		t.Errorf("delete surrounding + commit: %q", v)
	}
	// A click finishes the composition and resets the IME.
	ime(platform.IMEEvent{Preedit: "q", CursorStart: 1, CursorEnd: 1})
	resets := x.w.TextInputResets()
	x.frame()
	cx, cy := x.center("in")
	x.click(cx, cy)
	if v := x.value("in"); v != "aZq" {
		t.Errorf("click during a composition: %q", v)
	}
	if fieldOf(t, x.tree(), "in").ed.Composing() || x.w.TextInputResets() == resets {
		t.Errorf("composing %v, resets %d", fieldOf(t, x.tree(), "in").ed.Composing(), x.w.TextInputResets())
	}
	// The session's end finishes a composition.
	ime(platform.IMEEvent{Preedit: "r", CursorStart: 1, CursorEnd: 1})
	x.w.SetFocused(false)
	x.b.Drain(x.h)
	if fieldOf(t, x.tree(), "in").ed.Composing() || !strings.HasSuffix(x.value("in"), "r") {
		t.Errorf("after the session ended: %q", x.value("in"))
	}
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	// Moving to another field restarts the session: nil, then the state.
	changes := x.w.TextInputChanges()
	x.tab()
	if x.w.TextInputChanges() < changes+2 {
		t.Errorf("moving to another field changed the state %d times, want nil then the new state", x.w.TextInputChanges()-changes)
	}
	x.tab() // out of the fields: nothing focused
	x.settle()
	if focusedName(x.tree()) == "<nil>" && x.w.TextInput() != nil {
		t.Errorf("text input state %+v without a focused field", x.w.TextInput())
	}
}

// TestFieldIMEDeleteSurrounding checks that an IME deletion around the
// caret fires beforeinput before the value changes, as every other edit,
// and that PreventDefault does not cancel it (Chromium ignores the
// result: the IME cannot be told its edit was refused).
func TestFieldIMEDeleteSurrounding(t *testing.T) {
	var ev []string
	var x *inputHarness
	prevent := false
	root := div(input(Key("in"), DefaultValue("abcd"),
		On("beforeinput", func(e InputEvent) {
			ev = append(ev, fmt.Sprintf("before:%s:%v:%s", e.InputType, e.IsComposing, x.value("in")))
			if prevent {
				e.PreventDefault()
			}
		}),
		On("change", func(e ChangeEvent) {
			ev = append(ev, fmt.Sprintf("change:%s:%v:%s", e.InputType, e.IsComposing, e.Value))
		}),
	))
	x = newKeys(t, root, 400, 200)
	x.tab()
	x.key(platform.KeyEnd, 0)
	x.key(platform.KeyArrowLeft, 0)
	x.settle()
	ime := func(e platform.IMEEvent) {
		x.w.Inject(e)
		x.b.Drain(x.h)
	}
	ev = nil
	ime(platform.IMEEvent{DeleteBefore: 1})
	ime(platform.IMEEvent{DeleteAfter: 1})
	prevent = true
	ime(platform.IMEEvent{DeleteBefore: 1})
	prevent = false
	want := []string{
		"before:deleteContentBackward:false:abcd", "change:deleteContentBackward:false:abd",
		"before:deleteContentForward:false:abd", "change:deleteContentForward:false:ab",
		"before:deleteContentBackward:false:ab", "change:deleteContentBackward:false:a",
	}
	if got := strings.Join(ev, " "); got != strings.Join(want, " ") {
		t.Errorf("events\n got %s\nwant %s", got, strings.Join(want, " "))
	}
	// During a composition: around the preedit, IsComposing set.
	ev = nil
	ime(platform.IMEEvent{Preedit: "x", CursorStart: 1, CursorEnd: 1})
	ev = nil
	ime(platform.IMEEvent{DeleteBefore: 1, Preedit: "x", CursorStart: 1, CursorEnd: 1})
	if len(ev) < 2 || ev[0] != "before:deleteContentBackward:true:ax" || ev[1] != "change:deleteContentBackward:true:x" {
		t.Errorf("in a composition: %v", ev)
	}
	// Nothing to delete fires nothing.
	ime(platform.IMEEvent{Commit: "y"})
	x.key(platform.KeyHome, 0)
	ev = nil
	ime(platform.IMEEvent{DeleteBefore: 3})
	if len(ev) != 0 {
		t.Errorf("a deletion at the start fired %v", ev)
	}
}

func TestFieldIMEPurposes(t *testing.T) {
	for _, c := range []struct {
		typ   string
		multi bool
		want  platform.TextPurpose
	}{{"password", false, platform.TextPurposePassword}, {"email", false, platform.TextPurposeEmail},
		{"number", false, platform.TextPurposeNumber}, {"tel", false, platform.TextPurposePhone},
		{"url", false, platform.TextPurposeURL}, {"search", false, platform.TextPurposeSearch}, {"", true, platform.TextPurposeNormal}} {
		var n Node
		if c.multi {
			n = textarea(Key("f"))
		} else {
			n = input(Key("f"), FieldType(c.typ))
		}
		x := newKeys(t, n, 300, 100)
		x.tab()
		x.settle()
		st := x.w.TextInput()
		if st == nil || st.Purpose != c.want || st.Multiline != c.multi {
			t.Errorf("%s: %+v", c.typ, st)
			continue
		}
		if (c.typ == "password") != (st.Surrounding == nil) {
			t.Errorf("%s: surrounding %+v", c.typ, st.Surrounding)
		}
	}
	// A read-only field has no text input state.
	x := newKeys(t, input(Key("f"), ReadOnly(true), DefaultValue("ro")), 300, 100)
	x.tab()
	x.settle()
	if x.w.TextInput() != nil {
		t.Error("a read-only field turned the IME on")
	}
	x.typeText("x")
	if x.value("f") != "ro" {
		t.Error("typed into a read-only field")
	}
}

func TestFormImplicitSubmission(t *testing.T) {
	var log []string
	sub := On("submit", func(SubmitEvent) { log = append(log, "submit") })
	click := On("click", func(MouseEvent) { log = append(log, "click") })
	cases := []struct {
		name string
		root Node
		want string
	}{
		{"one field", form(sub, input(Key("a"))), "submit"},
		{"two fields", form(sub, input(Key("a")), input(Key("b"))), ""},
		{"two fields and a button", form(sub, input(Key("a")), input(Key("b")), button(click, Text("Go"))), "click submit"},
		{"a type=button button only", form(sub, input(Key("a")), button(FieldType("button"), Text("x"))), "submit"},
		{"a disabled submit button", form(sub, input(Key("a")), button(Disabled(true), Text("x"))), ""},
		{"no form", div(input(Key("a"))), ""},
	}
	for _, c := range cases {
		log = nil
		x := newKeys(t, c.root, 300, 200)
		x.tab()
		x.enter()
		if got := strings.Join(log, " "); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// A click on a submit button (no handler) submits; reset restores.
	log = nil
	root := form(sub, input(Key("u"), DefaultValue("d")), button(Key("go"), Text("Go")), button(Key("rs"), FieldType("reset"), Text("Reset")))
	x := newKeys(t, root, 300, 200)
	x.tab()
	x.typeText("xyz")
	x.frame()
	cx, cy := x.center("go")
	x.click(cx, cy)
	if strings.Join(log, " ") != "submit" {
		t.Errorf("button click: %v", log)
	}
	cx, cy = x.center("rs")
	x.click(cx, cy)
	if v := x.value("u"); v != "d" {
		t.Errorf("reset: %q", v)
	}
	if strings.Join(log, " ") != "submit" {
		t.Errorf("reset submitted: %v", log)
	}
	// Enter on a focused submit button submits too (its click).
	log = nil
	x.tab()
	if focusedName(x.tree()) != "go" {
		x.tab()
	}
	x.enter()
	if strings.Join(log, " ") != "submit" {
		t.Errorf("Enter on the button: %v", log)
	}
}

func TestButtonLabelName(t *testing.T) {
	root := div(
		div(button(Key("terms"), ID("terms"), Role("checkbox"), AriaChecked(false)),
			label(Key("l"), For("terms"), Text("I agree."))),
		label(Key("w"), Text("Wrapped "), button(Key("b"), Text("Go"))),
		div(button(Key("own"), ID("own"), Text("Own")), label(For("own"), Text(""))),
		div(button(Key("al"), ID("al"), AriaLabel("Aria wins")), label(For("al"), Text("ignored"))),
	)
	x := newKeys(t, root, 600, 400)
	x.frame()
	// Names checked against Chromium 152's computedName: a wrapping label
	// names the button without the button's own content ("Wrapped").
	want := `- checkbox "I agree."
- text: I agree. Wrapped
- button "Wrapped": Go
- button "Own"
- button "Aria wins"
- text: ignored`
	if got := x.tree().AriaSnapshot(); got != want {
		t.Errorf("snapshot\n%s\nwant\n%s", got, want)
	}
}

func TestLabels(t *testing.T) {
	root := div(
		div(label(Key("l1"), For("name"), Text("Name")),
			input(Key("name"), ID("name"))),
		div(label(Key("l2"), Text("Bio "), textarea(Key("bio")))),
		input(Key("ph"), Placeholder("Search here")),
		input(Key("al"), AriaLabel("Aria wins"), Placeholder("ignored")),
		input(Key("s"), FieldType("search"), DefaultValue("q")),
		input(Key("pw"), FieldType("password"), DefaultValue("abc"), ReadOnly(true)),
		form(AriaLabel("Signup"), input(Key("f"))),
		form(input(Key("g"))),
	)
	x := newKeys(t, root, 600, 400)
	x.frame()
	want := `- text: Name
- textbox "Name"
- text: Bio
- textbox "Bio"
- textbox "Search here"
- textbox "Aria wins"
- searchbox: q
- textbox: •••
- form "Signup":
  - textbox
- textbox`
	if got := x.tree().AriaSnapshot(); got != want {
		t.Errorf("snapshot\n%s\nwant\n%s", got, want)
	}
	// Clicking a label focuses its control.
	// The labels' text, on the line of the fields (a text's bounds cover
	// the atomic inline after it, a limitation of text.SelectionRects).
	_, ny := x.center("name")
	x.click(3, ny)
	if focusedName(x.tree()) != "name" {
		t.Errorf("label For focused %s", focusedName(x.tree()))
	}
	_, by := x.center("bio")
	x.click(5, by+20)
	if focusedName(x.tree()) != "bio" {
		t.Errorf("wrapping label focused %s", focusedName(x.tree()))
	}
	n := x.tree().Semantics()
	var bio *semantics.Node
	n.Walk(func(m *semantics.Node) bool {
		if m.Name == "Bio" && m.Role == "textbox" {
			bio = m
		}
		return true
	})
	if bio == nil || !bio.Has(semantics.StateMultiline) || !bio.Can(semantics.ActionSetValue) {
		t.Errorf("textarea node %+v", bio)
	}
}

// TestDuplicateIDs checks that a duplicate ID resolves to the first
// mounted element with it in tree order (HTML's getElementById), and
// that unmounting one of them leaves the ID to the others.
func TestDuplicateIDs(t *testing.T) {
	var set Setter[int]
	app := func() Node {
		n, s := UseState(0)
		set = s
		var kids []Child
		if n == 2 {
			kids = append(kids, input(Key("c"), ID("x"), Placeholder("C")))
		}
		kids = append(kids, input(Key("a"), ID("x"), Placeholder("A")))
		if n == 0 || n == 2 {
			kids = append(kids, input(Key("b"), ID("x"), Placeholder("B")))
		}
		kids = append(kids, label(Key("l"), For("x"), Text("Name")))
		return div(kids...)
	}
	x := newKeys(t, Component(app), 400, 300)
	x.frame()
	named := func() string {
		var out []string
		x.tree().Semantics().Walk(func(m *semantics.Node) bool {
			if m.Role == "textbox" && m.Name == "Name" {
				out = append(out, m.Name)
			}
			return true
		})
		el := x.tree().byID("x")
		if el == nil {
			return fmt.Sprintf("%d named, byID nil", len(out))
		}
		return fmt.Sprintf("%d named, byID %v", len(out), el.node.d.key)
	}
	if got := named(); got != "1 named, byID a" {
		t.Errorf("two elements with the ID: %s, want the first in tree order", got)
	}
	set.Set(1) // b, mounted last, goes
	x.frame()
	if got := named(); got != "1 named, byID a" {
		t.Errorf("after unmounting the last one mounted: %s", got)
	}
	set.Set(2) // c mounts before a in tree order, b after it
	x.frame()
	if got := named(); got != "1 named, byID c" {
		t.Errorf("after mounting one before: %s", got)
	}
	set.Set(1)
	x.frame()
	if got := named(); got != "1 named, byID a" {
		t.Errorf("after unmounting the first: %s", got)
	}
}

// TestLabelIndex checks the labels found through the index of For: in
// tree order whatever their mount order, and following a changed For.
func TestLabelIndex(t *testing.T) {
	var set Setter[int]
	app := func() Node {
		n, s := UseState(0)
		set = s
		var kids []Child
		if n >= 1 {
			kids = append(kids, label(Key("l1"), For("x"), Text("First")))
		}
		kids = append(kids, input(Key("a"), ID("x")), input(Key("b"), ID("y")))
		target := "x"
		if n == 2 {
			target = "y"
		}
		kids = append(kids, label(Key("l2"), For(target), Text("Second")))
		return div(kids...)
	}
	x := newKeys(t, Component(app), 400, 300)
	x.frame()
	names := func() string {
		var out []string
		x.tree().Semantics().Walk(func(m *semantics.Node) bool {
			if m.Role == "textbox" {
				out = append(out, fmt.Sprintf("%q", m.Name))
			}
			return true
		})
		return strings.Join(out, " ")
	}
	if got := names(); got != `"Second" ""` {
		t.Errorf("one label: %s", got)
	}
	set.Set(1) // l1 mounts after l2 but comes first
	x.frame()
	if got := names(); got != `"First Second" ""` {
		t.Errorf("two labels: %s", got)
	}
	set.Set(2) // l2 now names y
	x.frame()
	if got := names(); got != `"First" "Second"` {
		t.Errorf("after For changed: %s", got)
	}
	set.Set(0) // l1 unmounts, l2 names x again
	x.frame()
	if got := names(); got != `"Second" ""` {
		t.Errorf("after unmounting: %s", got)
	}
	if n := len(x.tree().labels["y"]); n != 0 {
		t.Errorf("%d labels left under y", n)
	}
}

func TestFieldElementRef(t *testing.T) {
	var ref ElementRef
	var sel []string
	app := func() Node {
		ref = UseElementRef()
		return input(Key("in"), AttachRef(ref), DefaultValue("hello world"), On("select", func(e SelectEvent) {
			sel = append(sel, fmt.Sprintf("%d-%d", e.Start, e.End))
		}))
	}
	x := newKeys(t, Component(app), 300, 100)
	x.frame()
	if ref.Value() != "hello world" {
		t.Errorf("Value %q", ref.Value())
	}
	ref.SetSelectionRange(6, 99)
	if s, e := ref.SelectionRange(); s != 6 || e != 11 {
		t.Errorf("SelectionRange %d %d", s, e)
	}
	ref.Select()
	if s, e := ref.SelectionRange(); s != 0 || e != 11 {
		t.Errorf("after Select %d %d", s, e)
	}
	ref.SetSelectionRange(3, 3)
	if strings.Join(sel, " ") != "6-11 0-11 3-3" {
		t.Errorf("select events %v", sel)
	}
	// Non-text elements: no-ops.
	var dref ElementRef
	y := newKeys(t, Component(func() Node {
		dref = UseElementRef()
		return div(AttachRef(dref))
	}), 100, 100)
	y.frame()
	dref.Select()
	dref.SetSelectionRange(1, 2)
	if dref.Value() != "" {
		t.Error("Value of a div")
	}
	if s, e := dref.SelectionRange(); s != 0 || e != 0 {
		t.Error("SelectionRange of a div")
	}
}

func TestFieldPropsChecks(t *testing.T) {
	devAny.Store(true)
	for _, c := range []struct {
		name string
		f    func()
		want string
	}{
		{"children", func() { input(Text("x")) }, "takes no children"},
		{"value on div", func() { div(FieldValue("x")) }, "only Input and Textarea"},
		{"rows on input", func() { input(Rows(3)) }, "only Textarea"},
		{"for on div", func() { div(For("x")) }, "only Label"},
		{"type typo", func() { input(FieldType("emial")) }, `did you mean "email"`},
		{"button type", func() { button(FieldType("sumbit")) }, `did you mean "submit"`},
		{"value and bind", func() { input(FieldValue("x"), BindValue(&Value[string]{})) }, "Value and BindValue"},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), c.want) {
					t.Errorf("%s: panic %v, want %q", c.name, r, c.want)
				}
			}()
			c.f()
		}()
	}
}

func TestFieldDisabled(t *testing.T) {
	root := div(input(Key("a"), Disabled(true), DefaultValue("x")), input(Key("b")))
	x := newKeys(t, root, 300, 100)
	x.tab()
	if focusedName(x.tree()) != "b" {
		t.Errorf("Tab focused %s, want the enabled field", focusedName(x.tree()))
	}
	x.frame()
	cx, cy := x.center("a")
	x.click(cx, cy)
	if focusedName(x.tree()) == "a" {
		t.Error("a disabled field took the focus")
	}
	if keyed(x.tree(), "a").istate&style.StateReadOnly == 0 {
		t.Error("a disabled field does not match read-only:")
	}
}
