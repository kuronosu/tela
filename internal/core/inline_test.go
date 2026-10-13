package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// Inline boxes and atomic inlines (inline.go, structure.go): box
// generation, hit-testing on fragments, hover repaints, focus into
// buttons in text, semantics bounds and the inspect JSON.

func TestAtomicInlineStructure(t *testing.T) {
	root := div(
		p(Key("p"), Text("Click "), button(Key("b"), Text("Save")), Text(" now")),
		div(Key("lone"), button(Key("b2"), Text("Alone"))),
		div(Key("flex"), Class("flex"), button(Key("b3"), Text("Item"))),
		div(Key("block"), Text("x"), button(Key("b4"), Class("block"), Text("Block"))),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	// A paragraph with an atomic inline: one paragraph box, the button is
	// its child.
	pb := keyed(tr, "p").box
	b := keyed(tr, "b").box
	if pb.mode != modeParagraph || len(pb.kids) != 1 || pb.kids[0] != b || b.parent != pb {
		t.Fatalf("p: mode %v, kids %v", pb.mode, pb.kids)
	}
	// The button sits on the line, after "Click ".
	lay := pb.para.lay
	if lay.NumLines() != 1 {
		t.Errorf("%d lines, want 1", lay.NumLines())
	}
	bl := b.ld.Layout()
	if bl.Location.X < 30 || bl.Location.Y < 0 || bl.Location.Y > 4 {
		t.Errorf("button at %v", bl.Location)
	}
	// A lone button in a block is on a line too: an anonymous line box,
	// its paragraph is the div's.
	if lone := keyed(tr, "lone").box; lone.mode != modeParagraph || len(lone.kids) != 1 {
		t.Errorf("lone: mode %v kids %d", lone.mode, len(lone.kids))
	}
	// In a flex container a button is a flex item; display: block is
	// block-level.
	if fb := keyed(tr, "flex").box; fb.mode != modeContainer || fb.kids[0] != keyed(tr, "b3").box {
		t.Errorf("flex: the button is not a flex item")
	}
	if bb := keyed(tr, "block").box; bb.mode != modeContainer || len(bb.kids) != 2 {
		t.Errorf("block button: mode %v, %d kids", bb.mode, len(bb.kids))
	}
}

func TestInlineHitTest(t *testing.T) {
	root := div(Class("p-0"),
		p(Key("p"), Class("w-[300px] leading-6"),
			Text("Go "), anchor(Key("a"), Class("px-4 py-1"), Text("docs")), Text(" or "),
			button(Key("b"), Class("px-2"), Text("Save")), Text(" or "),
			code(Key("c"), Class("bg-slate-100 px-1"), Text("x"))),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	pb := keyed(tr, "p").box
	frags := pb.para.fragments()
	if len(frags) != 2 {
		t.Fatalf("%d fragments, want 2", len(frags))
	}
	a := fragBorderBox(keyed(tr, "a").box.cs, &frags[0])
	// The padding of the link: left of its text, and above the line box
	// is outside the paragraph box (py-1 grows 4 px past the content area,
	// which is 2 px inside the 24 px line).
	if got := hitName(tr, a.X0+2, 12); got != "a" {
		t.Errorf("on the link's padding: %s, want a", got)
	}
	if got := hitName(tr, a.X0+a.Width()/2, 12); got != "a" {
		t.Errorf("on the link's text: %s, want a", got)
	}
	if got := hitName(tr, a.X0-2, 12); got != "p" {
		t.Errorf("before the link: %s, want p", got)
	}
	bl := keyed(tr, "b").box.ld.Layout()
	if got := hitName(tr, bl.Location.X+3, bl.Location.Y+bl.Size.H/2); got != "b" {
		t.Errorf("on the button's padding: %s, want b", got)
	}
	cb := fragBorderBox(keyed(tr, "c").box.cs, &frags[1])
	if got := hitName(tr, cb.X0+1, 12); got != "c" {
		t.Errorf("on the code's padding: %s, want c", got)
	}
}

func TestInlineHoverRepaint(t *testing.T) {
	root := div(Class("p-4"),
		p(Class("w-[300px]"), Text("Read the "), anchor(Key("a"), Class("rounded px-1 hover:bg-sky-200 hover:px-2"), Text("documentation")), Text(" first.")),
		p(Text("Second "), span(Class("rounded-md bg-amber-100 px-2 shadow-sm hover:bg-amber-300"), Text("chip")), Text(" end")),
	)
	x := newDamageHarness(t, root, 400, 200, 2, Config{}, 1)
	frag := func() gfx.Rect {
		pb := keyed(x.tree(), "a").box.inPara
		r := pb.para.inlineRects(keyed(x.tree(), "a"), nil)[0]
		c := contentRect(pb.u, pb.ld.Layout())
		return r.Translate(c.X0, c.Y0)
	}
	before := frag()
	x.move(before.X0+10, before.Y0+5)
	x.frame("hover link")
	after := frag()
	// hover:px-2 lays the paragraph out again: the link is 8 px wider.
	if d := after.Width() - before.Width(); d < 7.9 || d > 8.1 {
		t.Errorf("the hovered link is %g px wider, want 8", d)
	}
	x.move(10, 180)
	x.frame("leave")
	x.move(80, 50)
	x.frame("hover chip")
	x.move(10, 180)
	x.frame("leave chip")
}

func TestTruncatedAtomHidden(t *testing.T) {
	root := div(Class("p-0"), p(Key("p"), Class("w-[120px] truncate"),
		Text("A long line of text that is truncated "), button(Key("b"), Class("bg-red-500 px-2"), Text("Hidden"))))
	tr := newTestTree(t, 300, 100, root, nil)
	dl, _ := tr.Frame()
	b := keyed(tr, "b").box
	if !b.hiddenAtom {
		t.Fatal("the truncated button is not hidden")
	}
	if got := hitName(tr, 2, 10); got != "p" {
		t.Errorf("at the paragraph's start: %s, want p", got)
	}
	red := gfx.Paint(b.cs.BackgroundColor)
	for _, c := range dl.All() {
		if f, ok := c.(gfx.FillRect); ok && f.Paint == red {
			t.Errorf("the hidden button's background is painted: %+v", f)
		}
	}
}

func TestInlineFocusOrder(t *testing.T) {
	var r recorder
	root := div(p(Text("Press "), button(Key("one"), r.mouse("click", "one"), Text("One")), Text(" or "),
		button(Key("two"), r.mouse("click", "two"), Text("Two")), Text(".")))
	x := newKeys(t, root, 400, 200)
	x.tab()
	if f := focusedName(x.tree()); f != "one" {
		t.Fatalf("first Tab focuses %s", f)
	}
	x.tab()
	if f := focusedName(x.tree()); f != "two" {
		t.Fatalf("second Tab focuses %s", f)
	}
	x.enter()
	if got := r.String(); got != "click:two" {
		t.Errorf("Enter: %q", got)
	}
	r.reset()
	cx, cy := x.center("one")
	x.click(cx, cy)
	if got := r.String(); got != "click:one" {
		t.Errorf("click: %q", got)
	}
}

func TestInlineSemanticsBounds(t *testing.T) {
	tr, _ := snapshotOf(t, div(Class("p-0"),
		p(Key("p"), Class("w-[200px]"), Text("Go to "), anchor(Href("/x"), Class("px-4"), Text("docs")), Text(" now")),
	))
	link := mustNode(t, tr, "link", "docs")
	// The bounds hold the padding (px-4: 16 px) before the text of "docs".
	lay := keyed(tr, "p").box.para.lay
	c := lay.Cluster(lay.ClusterAt(strings.Index(lay.Text(), "docs")))
	if d := c.X - link.Bounds.X0; d < 15.9 || d > 16.1 {
		t.Errorf("link bounds %v start %g before its text, want 16 (padding)", link.Bounds, d)
	}
}

func TestInspectInlineFragments(t *testing.T) {
	tr := newTestTree(t, 400, 300, div(
		p(Key("p"), Class("w-[120px]"), Text("Some "), span(Class("bg-amber-100 px-2"), Text("decorated text that wraps")), Text(" and "), button(Text("ok"))),
	), nil)
	tr.Frame()
	n := mustFind(t, tr, "p")
	if len(n.Fragments) < 2 || n.Fragments[0].Inline != 0 || n.Fragments[1].Line != n.Fragments[0].Line+1 {
		t.Errorf("fragments %+v", n.Fragments)
	}
	if len(n.Children) != 1 || !n.Children[0].InlineBox || n.Children[0].Kind != "button" {
		t.Fatalf("children %+v", n.Children)
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"inline":["span"]`, `"fragments":[{"inline":0,"line":1,"rect":{"x":0,`, `"inlineBox":true`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON lacks %s:\n%s", want, s)
		}
	}
}
