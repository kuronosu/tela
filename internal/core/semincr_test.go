package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
)

// Incremental semantics (semincr.go, ADR-277). Core's tests run with
// TELA_CHECK_SEMANTICS on (TestMain): every tree is also compared with a
// full build. These tests check what only an incremental build can get
// wrong: the dependents of a change, and that what did not change is
// shared with the last tree.

// semNodes returns the nodes of the tree rooted at root by ID.
func semNodes(root *semantics.Node) map[semantics.NodeID]*semantics.Node {
	m := map[semantics.NodeID]*semantics.Node{}
	root.Walk(func(n *semantics.Node) bool {
		m[n.ID] = n
		return true
	})
	return m
}

// semShared returns how many nodes of b are the same pointers as in a.
func semShared(a, b *semantics.Node) (shared, total int) {
	old := semNodes(a)
	b.Walk(func(n *semantics.Node) bool {
		total++
		if old[n.ID] == n {
			shared++
		}
		return true
	})
	return
}

// semSiblings returns div children with buttons, the filler that a change
// elsewhere must leave shared.
func semSiblings(n int) []Child {
	kids := make([]Child, n)
	for i := range kids {
		kids[i] = div(button(Textf("Other %d", i)))
	}
	return kids
}

// TestSemanticsCheckCatchesAMissedMark: the check (TELA_CHECK_SEMANTICS)
// panics when a change reaches no mark, so the tests below would fail
// with a dependent left out.
func TestSemanticsCheckCatchesAMissedMark(t *testing.T) {
	if !checkSemantics {
		t.Skip("TELA_CHECK_SEMANTICS is off")
	}
	tr, _ := snapshotOf(t, div(button(Text("Save"))))
	tr.Semantics()
	var text *Element
	var walk func(el *Element)
	walk = func(el *Element) {
		for _, c := range el.children {
			if c.kind == elemText {
				text = c
			}
			walk(c)
		}
	}
	walk(tr.root)
	text.text = "Changed" // behind the marks' back
	msg := panicMessage(func() { tr.Semantics() })
	if !strings.Contains(msg, "TELA_CHECK_SEMANTICS") {
		t.Fatalf("a change without a mark: panic %q, want the check's", msg)
	}
}

// TestSemanticsIncrementalNameFromContent: a button named by its text
// child takes the new text, and the rest of the tree is shared.
func TestSemanticsIncrementalNameFromContent(t *testing.T) {
	var set func(string)
	tr, _ := snapshotOf(t, div(
		Component(func() Node {
			label, s := UseStateSet("Save")
			set = s
			return button(Text(label))
		}),
		div(semSiblings(50)...)))
	a := tr.Semantics()
	set("Save all")
	tr.Frame()
	if !tr.SemanticsStale() {
		t.Fatal("a new label left the semantics tree current")
	}
	b := tr.Semantics()
	if findNode(b, "button", "Save all") == nil {
		t.Fatalf("the button's name did not follow its text:\n%s", tr.AriaSnapshot())
	}
	shared, total := semShared(a, b)
	if total-shared > 4 { // root, its div, the button and its text
		t.Errorf("%d of %d nodes built again, want 4", total-shared, total)
	}
	if b != tr.Semantics() {
		t.Error("an unchanged tree builds its semantics again")
	}
}

// TestSemanticsIncrementalSetPositions: inserting an item renumbers its
// siblings (PosInSet, SetSize), which were not built again.
func TestSemanticsIncrementalSetPositions(t *testing.T) {
	var setN func(int)
	tr, _ := snapshotOf(t, Component(func() Node {
		n, s := UseStateSet(3)
		setN = s
		items := make([]int, n)
		for i := range items {
			items[i] = n - i // keys move: the new item comes first
		}
		return div(Role("list"), Map(items, func(i int) Node {
			return WithKey(div(Role("listitem"), Textf("Item %d", i)), i)
		}))
	}))
	check := func(n int) {
		t.Helper()
		var items []*semantics.Node
		tr.Semantics().Walk(func(x *semantics.Node) bool {
			if x.Role == "listitem" {
				items = append(items, x)
			}
			return true
		})
		if len(items) != n {
			t.Fatalf("%d items, want %d", len(items), n)
		}
		for i, it := range items {
			if it.PosInSet != i+1 || it.SetSize != n {
				t.Errorf("item %d: position %d of %d, want %d of %d", i, it.PosInSet, it.SetSize, i+1, n)
			}
		}
	}
	check(3)
	setN(4)
	tr.Frame()
	check(4)
	setN(2)
	tr.Frame()
	check(2)
}

// TestSemanticsIncrementalBoundsShift: a box that grows moves the bounds
// of what follows it, which did not change otherwise.
func TestSemanticsIncrementalBoundsShift(t *testing.T) {
	var setTall func(bool)
	tr, _ := snapshotOf(t, div(Class("flex flex-col"),
		Component(func() Node {
			tall, s := UseStateSet(false)
			setTall = s
			cls := "h-10"
			if tall {
				cls = "h-20"
			}
			return div(Class(cls), Text("Header"))
		}),
		div(Class("p-2"), button(Class("h-8"), Text("Below")), div(p(Text("Deep text"))))))
	before := mustNode(t, tr, "button", "Below").Bounds
	setTall(true)
	tr.Frame()
	after := mustNode(t, tr, "button", "Below").Bounds
	if after != before.Translate(0, 40) {
		t.Errorf("bounds %v after the header grew by 40, want %v", after, before.Translate(0, 40))
	}
	para := mustNode(t, tr, "paragraph", "")
	if para.Bounds.Y0 < after.Y1 {
		t.Errorf("the paragraph at %v did not move below the button %v", para.Bounds, after)
	}
}

// TestSemanticsIncrementalLabelFn: a change of a Value an AriaLabelFn
// reads builds that element's node again without a frame; the rest is
// shared.
func TestSemanticsIncrementalLabelFn(t *testing.T) {
	var name *Value[string]
	tr, _ := snapshotOf(t, Component(func() Node {
		name = UseValue("Upload")
		return div(button(AriaLabelFn(name.Get), Text("x")), div(semSiblings(30)...))
	}))
	a := tr.Semantics()
	mustNode(t, tr, "button", "Upload")
	name.Set("Download")
	tr.ApplyUpdates()
	if !tr.SemanticsStale() {
		t.Fatal("the AriaLabelFn's Value left the tree current")
	}
	b := tr.Semantics()
	if findNode(b, "button", "Download") == nil {
		t.Fatalf("the name did not follow the Value:\n%s", tr.AriaSnapshot())
	}
	if shared, total := semShared(a, b); total-shared > 3 { // root, div, button
		t.Errorf("%d of %d nodes built again, want 3", total-shared, total)
	}
}

// TestSemanticsIncrementalLabelledBy: an element named by another one
// (AriaLabelledBy), outside its subtree, follows that one's text.
func TestSemanticsIncrementalLabelledBy(t *testing.T) {
	var set func(string)
	ref := NewElementRef()
	tr, _ := snapshotOf(t, div(
		Component(func() Node {
			title, s := UseStateSet("Delete file?")
			set = s
			return h2(AttachRef(ref), Text(title))
		}),
		div(Role("dialog"), AriaLabelledBy(ref), button(Text("Delete"))),
		div(semSiblings(20)...)))
	mustNode(t, tr, "dialog", "Delete file?")
	set("Delete folder?")
	tr.Frame()
	if findNode(tr.Semantics(), "dialog", "Delete folder?") == nil {
		t.Fatalf("the dialog's name did not follow its label:\n%s", tr.AriaSnapshot())
	}
}

// TestSemanticsIncrementalLabelFor: a Label added for a field's ID names
// the field.
func TestSemanticsIncrementalLabelFor(t *testing.T) {
	var show func(bool)
	tr, _ := snapshotOf(t, Component(func() Node {
		on, s := UseStateSet(false)
		show = s
		return div(If(on, label(For("email"), Text("Email"))), input(ID("email")))
	}))
	show(true)
	tr.Frame()
	if findNode(tr.Semantics(), "textbox", "Email") == nil {
		t.Fatalf("the field did not take its new label:\n%s", tr.AriaSnapshot())
	}
}

// TestSemanticsIncrementalFocusAndScroll: focus moves and scrolling build
// the focused nodes and the moved subtree again, the rest stays shared.
func TestSemanticsIncrementalFocusAndScroll(t *testing.T) {
	rows := make([]Child, 40)
	for i := range rows {
		rows[i] = button(Class("block h-10"), Textf("Row %d", i))
	}
	tr, _ := snapshotOf(t, div(div(Key("list"), Class("h-40 overflow-auto"), Fragment(rows...)), button(Text("Out"))))
	first := mustNode(t, tr, "button", "Row 0")
	if err := tr.FocusNode(first.ID); err != nil {
		t.Fatal(err)
	}
	tr.Frame()
	if n := mustNode(t, tr, "button", "Row 0"); !n.Has(semantics.StateFocused) {
		t.Error("the focused row is not focused")
	}
	out := mustNode(t, tr, "button", "Out")
	list := tr.root.children[0].children[0]
	tr.scriptedRequest(scrollReq{s: list.box.scr, p: gfx.Point{Y: 100}})
	tr.Frame()
	if n := mustNode(t, tr, "button", "Row 0"); n.Bounds.Y0 != first.Bounds.Y0-100 {
		t.Errorf("row 0 at %v after a scroll of 100, was %v", n.Bounds, first.Bounds)
	}
	if mustNode(t, tr, "button", "Out") != out {
		t.Error("a node outside the scrolled container was built again")
	}
}

// TestSemanticsIncrementalInherited: what descendants inherit (hidden,
// disabled, a live region, visibility) follows a change of an ancestor
// alone (each tree is checked against a full build).
func TestSemanticsIncrementalInherited(t *testing.T) {
	var set func(int)
	tr, _ := snapshotOf(t, Component(func() Node {
		step, s := UseStateSet(0)
		set = s
		var props []Child
		switch step {
		case 1:
			props = append(props, AriaHidden(true))
		case 2:
			props = append(props, Disabled(true))
		case 3:
			props = append(props, Role("status"))
		case 4:
			props = append(props, Class("invisible"))
		case 5:
			props = append(props, Class("hidden"))
		}
		return div(append(props, div(div(button(Text("Deep")), Text("text"))))...)
	}))
	for step := 1; step <= 5; step++ {
		set(step)
		tr.Frame()
		n := tr.Semantics()
		set(0)
		tr.Frame()
		if tr.Semantics() == n {
			t.Errorf("step %d: the tree did not change back", step)
		}
	}
}
