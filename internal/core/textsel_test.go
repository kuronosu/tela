package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
)

// Tests of the selection of static text (textsel.go, ADR-271).

// textAt returns the window point in the middle of the caret at byte
// offset off of the paragraph box of the element with key k (plus dx).
func textAt(t *testing.T, tr *Tree, k any, off int, dx float32) gfx.Point {
	t.Helper()
	el := keyed(tr, k)
	if el == nil {
		t.Fatalf("no element %v", k)
	}
	bx := el.box
	if bx.inPara != nil {
		bx = bx.inPara
	}
	if !paraBox(bx) {
		t.Fatalf("%v is not a paragraph", k)
	}
	c := contentRect(windowRect(bx), bx.ld.Layout())
	car := bx.para.lay.Caret(text.Cursor{Offset: off})
	return gfx.Point{X: c.X0 + car.X + dx, Y: c.Y0 + car.Top + car.Height/2}
}

// drag presses at a, moves to b in steps and releases there.
func (x *inputHarness) drag(a, b gfx.Point) {
	x.send(move(a.X, a.Y), down(a.X, a.Y))
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		x.send(moveHeld(a.X+(b.X-a.X)*f, a.Y+(b.Y-a.Y)*f))
	}
	x.send(up(b.X, b.Y))
	x.b.Advance(time.Second) // no double click with the next press
}

// clickAt clicks once at p, not as part of a double click.
func (x *inputHarness) clickAt(p gfx.Point) {
	x.click(p.X, p.Y)
	x.b.Advance(time.Second)
}

// readClip returns the text on the harness's clipboard.
func (x *inputHarness) readClip() string {
	var got string
	x.b.Clipboard().ReadText(func(s string, err error) { got = s })
	x.b.Drain(x.h)
	return got
}

func twoParas() Node {
	return div(Class("p-4 w-80"),
		p(Key("a"), Text("Hello brave world")),
		p(Key("b"), Text("Second paragraph here")))
}

func TestTextSelDragAcrossParagraphs(t *testing.T) {
	x := newKeys(t, twoParas(), 400, 200)
	tr := x.tree()
	x.drag(textAt(t, tr, "a", 6, 1), textAt(t, tr, "b", 6, 1))
	if got, want := tr.SelectedText(), "brave world\nSecond"; got != want {
		t.Errorf("drag selected %q, want %q", got, want)
	}
	// Both paragraphs paint their part; the first one's break too.
	a, b := keyed(tr, "a").box.para, keyed(tr, "b").box.para
	if !a.hl.on || !a.hl.more || !b.hl.on || b.hl.more {
		t.Errorf("highlights a %+v b %+v", a.hl, b.hl)
	}
	// Backward: the same text.
	x.drag(textAt(t, tr, "b", 6, 1), textAt(t, tr, "a", 6, 1))
	if got, want := tr.SelectedText(), "brave world\nSecond"; got != want {
		t.Errorf("backward drag selected %q, want %q", got, want)
	}
	// A click elsewhere on the text clears it.
	x.clickAt(textAt(t, tr, "a", 2, 0))
	if got := tr.SelectedText(); got != "" {
		t.Errorf("a click left %q selected", got)
	}
	if a.hl.on || b.hl.on {
		t.Errorf("a click left highlights: %+v %+v", a.hl, b.hl)
	}
}

func TestTextSelWordParagraphShift(t *testing.T) {
	x := newKeys(t, twoParas(), 400, 200)
	tr := x.tree()
	w := textAt(t, tr, "a", 8, 0) // in "brave"
	x.click(w.X, w.Y)
	x.click(w.X, w.Y)
	if got := tr.SelectedText(); got != "brave" {
		t.Errorf("double click selected %q", got)
	}
	x.click(w.X, w.Y)
	if got, want := tr.SelectedText(), "Hello brave world\n"; got != want {
		t.Errorf("triple click selected %q, want %q (the paragraph and its break)", got, want)
	}
	x.b.Advance(time.Second)
	// A double click then a drag extends by words.
	x.click(w.X, w.Y)
	x.send(down(w.X, w.Y))
	to := textAt(t, tr, "b", 2, 0) // in "Second"
	x.send(moveHeld(to.X, to.Y), up(to.X, to.Y))
	if got, want := tr.SelectedText(), "brave world\nSecond"; got != want {
		t.Errorf("word drag selected %q, want %q", got, want)
	}
	x.b.Advance(time.Second)
	// Shift+click extends from the anchor.
	x.clickAt(textAt(t, tr, "a", 0, 0))
	sh := textAt(t, tr, "a", 5, 0)
	x.w.Inject(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: sh.X, Y: sh.Y}, Button: platform.ButtonPrimary,
		Buttons: platform.ButtonPrimary.Mask(), Mods: platform.ModShift})
	x.w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: sh.X, Y: sh.Y}, Button: platform.ButtonPrimary, Mods: platform.ModShift})
	x.b.Drain(x.h)
	if got := tr.SelectedText(); got != "Hello" {
		t.Errorf("shift+click selected %q", got)
	}
}

func TestTextSelKeys(t *testing.T) {
	x := newKeys(t, twoParas(), 400, 200)
	tr := x.tree()
	x.drag(textAt(t, tr, "a", 0, 0), textAt(t, tr, "a", 5, 0))
	x.ctrl("c", 0)
	if got := x.readClip(); got != "Hello" {
		t.Errorf("Ctrl+C copied %q", got)
	}
	x.key(platform.KeyEscape, 0)
	if got := tr.SelectedText(); got != "Hello" {
		t.Errorf("Escape changed the selection to %q (Chromium keeps it)", got)
	}
	x.key(platform.KeyArrowRight, platform.ModShift)
	if got := tr.SelectedText(); got != "Hello" {
		t.Errorf("Shift+ArrowRight changed the selection to %q (no caret browsing)", got)
	}
	x.ctrl("a", 0)
	if got, want := tr.SelectedText(), "Hello brave world\nSecond paragraph here"; got != want {
		t.Errorf("Ctrl+A selected %q, want %q", got, want)
	}
	x.key(platform.KeyInsert, platform.ModCtrl)
	if got, want := x.readClip(), "Hello brave world\nSecond paragraph here"; got != want {
		t.Errorf("Ctrl+Insert copied %q", got)
	}
}

func TestTextSelNotStarted(t *testing.T) {
	clicks := 0
	root := div(Class("p-4 w-96"),
		p(Key("a"), Text("Selectable text here")),
		button(Key("btn"), On("click", func(MouseEvent) { clicks++ }), Text("A button label")),
		div(Key("none"), Class("select-none"), Text("Not selectable at all")),
		div(Key("click"), On("click", func(MouseEvent) { clicks++ }), Text("Clickable card text")),
		input(Key("in"), DefaultValue("field")))
	x := newKeys(t, root, 400, 300)
	tr := x.tree()
	sel := func() { x.drag(textAt(t, tr, "a", 0, 0), textAt(t, tr, "a", 10, 0)) }
	sel()
	if got := tr.SelectedText(); got != "Selectable" {
		t.Fatalf("selected %q", got)
	}
	// A press on a button or on select-none text leaves it; so does a drag
	// from them.
	x.clickAt(textAt(t, tr, "btn", 3, 0))
	if got := tr.SelectedText(); got != "Selectable" || clicks != 1 {
		t.Errorf("a button click: selection %q, clicks %d", got, clicks)
	}
	x.drag(textAt(t, tr, "btn", 0, 0), textAt(t, tr, "a", 15, 0))
	if got := tr.SelectedText(); got != "Selectable" {
		t.Errorf("a drag from a button selected %q", got)
	}
	x.drag(textAt(t, tr, "none", 0, 0), textAt(t, tr, "none", 8, 0))
	if got := tr.SelectedText(); got != "Selectable" {
		t.Errorf("a drag on select-none text: %q", got)
	}
	// A click on the text of a clickable element clicks it and clears the
	// selection; a drag in it selects, and the release still clicks
	// (the DOM's click on the common ancestor).
	x.clickAt(textAt(t, tr, "click", 2, 0))
	if got := tr.SelectedText(); got != "" || clicks != 2 {
		t.Errorf("a click on a clickable: selection %q, clicks %d", got, clicks)
	}
	x.drag(textAt(t, tr, "click", 0, 0), textAt(t, tr, "click", 9, 0))
	if got := tr.SelectedText(); got != "Clickable" || clicks != 3 {
		t.Errorf("a drag in a clickable: selection %q, clicks %d", got, clicks)
	}
	// Focusing a text field clears the static selection.
	in := windowRect(keyed(tr, "in").box).Center()
	x.clickAt(in)
	if got := tr.SelectedText(); got != "" {
		t.Errorf("focusing a field left %q selected", got)
	}
	sel()
	x.tab() // focus moves into the field by the keyboard
	if focusedName(tr) == "in" && tr.SelectedText() != "" {
		t.Errorf("a Tab into the field left %q selected", tr.SelectedText())
	}
}

func TestTextSelCursor(t *testing.T) {
	root := div(Class("p-4 w-96"),
		p(Key("a"), Text("Some text")),
		div(Key("none"), Class("select-none"), Text("Not selectable")),
		button(Key("btn"), Text("Button")),
		div(Key("ptr"), Class("cursor-pointer"), Text("Pointer")))
	x := newKeys(t, root, 400, 300)
	tr := x.tree()
	at := func(p gfx.Point) platform.Cursor {
		x.send(move(p.X, p.Y))
		return tr.Cursor()
	}
	if c := at(textAt(t, tr, "a", 2, 0)); c != platform.CursorText {
		t.Errorf("over text: %v, want text", c)
	}
	a := windowRect(keyed(tr, "a").box)
	if c := at(gfx.Point{X: a.X1 - 2, Y: a.Center().Y}); c != platform.CursorDefault {
		t.Errorf("beside the text: %v, want default", c)
	}
	if c := at(textAt(t, tr, "none", 2, 0)); c != platform.CursorDefault {
		t.Errorf("over select-none text: %v, want default", c)
	}
	if c := at(textAt(t, tr, "btn", 2, 0)); c != platform.CursorPointer {
		t.Errorf("over a button: %v, want pointer", c)
	}
	if c := at(textAt(t, tr, "ptr", 2, 0)); c != platform.CursorPointer {
		t.Errorf("over cursor-pointer text: %v", c)
	}
}

func TestTextSelAtomsAndBlocks(t *testing.T) {
	root := div(Class("p-4 w-96"),
		p(Key("a"), Class("mb-4"), Text("Hi "), span(Key("badge"), Class("inline-block px-1"), Text("NEW")), Text(" there")),
		div(Class("flex gap-2"), span(Key("x"), Text("left")), span(Key("y"), Text("right"))),
		h1(Key("h"), Text("Title")))
	x := newKeys(t, root, 400, 300)
	tr := x.tree()
	x.ctrl("a", 0)
	// The badge's text is inline; a paragraph with a large bottom margin
	// gets a blank line after it (Chromium); flex items are blocks.
	if got, want := tr.SelectedText(), "Hi NEW there\n\nleft\nright\nTitle"; got != want {
		t.Errorf("select all: %q, want %q", got, want)
	}
	x.clickAt(textAt(t, tr, "a", 0, 0)) // a press inside the selection would drag it
	x.drag(textAt(t, tr, "a", 1, 0), textAt(t, tr, "badge", 2, 0))
	if got, want := tr.SelectedText(), "i NE"; got != want {
		t.Errorf("into the badge: %q, want %q", got, want)
	}
}

func TestTextSelPaintAndIdle(t *testing.T) {
	x := newKeys(t, twoParas(), 400, 200)
	tr := x.tree()
	x.frame()
	frames := tr.stats.Frames
	p0 := textAt(t, tr, "a", 7, 0)
	before := x.w.Frame().RGBAAt(int(p0.X), int(p0.Y-6))
	x.drag(textAt(t, tr, "a", 6, 0), textAt(t, tr, "b", 3, 0))
	x.frame()
	got := x.w.Frame().RGBAAt(int(p0.X), int(p0.Y-6))
	if got == before {
		t.Errorf("the selection did not paint: %v", got)
	}
	// No frame without a change.
	n := tr.stats.Frames
	for range 3 {
		x.frame()
	}
	if tr.stats.Frames != n {
		t.Errorf("idle frames after a selection: %d", tr.stats.Frames-n)
	}
	if n == frames {
		t.Error("the drag ran no frame")
	}
}

// Over a link the cursor is the hand, from the browser's stylesheet
// (a:-webkit-any-link { cursor: pointer }), inherited by its content and
// under its classes; with cursor: auto set on it, Chromium's
// ShouldShowIBeamForNode shows the I-beam over its text (IsOverLink). An
// A without a destination is no link: the I-beam over its text.
func TestTextSelCursorLink(t *testing.T) {
	root := div(Class("p-4 w-96"),
		p(Key("p"), Text("Read the "), anchor(Key("ln"), Href("#docs"), Class("px-2"), Text("documentation "), span(Key("in"), Text("inside"))), Text(" first.")),
		p(Key("q"), Text("Or "), anchor(Key("auto"), Href("#x"), Class("cursor-auto"), Text("this link")), Text(" here.")),
		p(Key("r"), Text("An "), anchor(Key("plain"), Text("anchor")), Text(" without one.")),
		p(Key("s"), Text("A "), anchor(Key("txt"), Href("#y"), Class("cursor-text"), Text("text cursor")), Text(" link.")))
	x := newKeys(t, root, 400, 300)
	tr := x.tree()
	at := func(p gfx.Point) platform.Cursor {
		x.send(move(p.X, p.Y))
		return tr.Cursor()
	}
	if c := at(textAt(t, tr, "ln", 12, 0)); c != platform.CursorPointer {
		t.Errorf("over link text: %v, want pointer", c)
	}
	if c := at(textAt(t, tr, "in", 25, 0)); c != platform.CursorPointer {
		t.Errorf("over a span in a link: %v, want pointer", c)
	}
	// Its padding (8 px before its text): no text, still the link's hand.
	first := textAt(t, tr, "ln", 9, 0)
	if c := at(gfx.Point{X: first.X - 4, Y: first.Y}); c != platform.CursorPointer {
		t.Errorf("over a link's padding: %v, want pointer", c)
	}
	if c := at(textAt(t, tr, "auto", 5, 0)); c != platform.CursorText {
		t.Errorf("over cursor-auto link text: %v, want text", c)
	}
	if c := at(textAt(t, tr, "plain", 5, 0)); c != platform.CursorText {
		t.Errorf("over an A without Href: %v, want text", c)
	}
	if c := at(textAt(t, tr, "txt", 4, 0)); c != platform.CursorText {
		t.Errorf("over a cursor-text link: %v, want text", c)
	}
}

// The auto cursor reuses the hit test that routed the pointer event: a
// move hit-tests once. Content that moves under a resting pointer (a
// frame without pointer events) gives the cursor of what is under it now.
func TestTextSelCursorOneHitTest(t *testing.T) {
	var set Setter[bool]
	app := func() Node {
		down, s := UseState(false)
		set = s
		pad := "pt-4"
		if down {
			pad = "pt-40"
		}
		return div(Class("w-96 "+pad), p(Key("a"), Class("px-4"), Text("Some selectable text")))
	}
	x := newKeys(t, Component(app), 400, 200)
	tr := x.tree()
	a := textAt(t, tr, "a", 2, 0)
	x.send(move(a.X, a.Y))
	for i := range 3 {
		n := tr.in.hitTests
		x.send(move(a.X+float32(i+1), a.Y))
		if c := tr.Cursor(); c != platform.CursorText {
			t.Fatalf("over text: %v, want text", c)
		}
		if got := tr.in.hitTests - n; got != 1 {
			t.Errorf("a move and its cursor ran %d hit tests, want 1", got)
		}
	}
	// The text moves down from under the resting pointer: the default
	// arrow, and back.
	set.Set(true)
	x.settle()
	if c := tr.Cursor(); c != platform.CursorDefault {
		t.Errorf("after the text moved away: %v, want default", c)
	}
	set.Set(false)
	x.settle()
	if c := tr.Cursor(); c != platform.CursorText {
		t.Errorf("after the text came back: %v, want text", c)
	}
}

// An A that gets a destination becomes a link, with its cursor, and an A
// that loses it stops being one.
func TestTextSelCursorLinkChanges(t *testing.T) {
	var set Setter[string]
	app := func() Node {
		href, s := UseState("")
		set = s
		return div(Class("p-4 w-96"), p(Text("Go "), anchor(Key("a"), Href(href), Text("there")), Text(" now.")))
	}
	x := newKeys(t, Component(app), 400, 200)
	tr := x.tree()
	pt := textAt(t, tr, "a", 5, 0)
	x.send(move(pt.X, pt.Y))
	if c := tr.Cursor(); c != platform.CursorText {
		t.Errorf("without Href: %v, want text", c)
	}
	set.Set("#there")
	x.settle()
	if c := tr.Cursor(); c != platform.CursorPointer {
		t.Errorf("with Href: %v, want pointer", c)
	}
	set.Set("")
	x.settle()
	if c := tr.Cursor(); c != platform.CursorText {
		t.Errorf("Href removed: %v, want text", c)
	}
}
