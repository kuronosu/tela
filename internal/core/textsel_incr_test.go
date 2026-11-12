package core

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
)

// A change of the static selection costs what changed (textsel.go): the
// highlights are updated only in the paragraphs between the old and the
// new place of each end that moved.

// manyParas is n paragraphs of one line, keyed by their index; every
// seventh holds an inline-block badge with text of its own (an atomic
// inline, whose text comes at its place in the paragraph's).
func manyParas(n int) Node {
	kids := make([]Child, 0, n+1)
	kids = append(kids, Class("p-4 w-96"))
	for i := range n {
		if i%7 == 3 {
			kids = append(kids, p(Key(i), Text(fmt.Sprintf("Paragraph %d has ", i)),
				span(Key(fmt.Sprintf("badge%d", i)), Class("inline-block px-1"), Text("a badge")),
				Text(" inside it")))
			continue
		}
		kids = append(kids, p(Key(i), Text(fmt.Sprintf("Paragraph %d of the long selection", i))))
	}
	return div(kids...)
}

// paraOf returns the paragraph box of the element with key k.
func paraOf(t *testing.T, tr *Tree, k any) *Box {
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
	return bx
}

// highlights returns what every paragraph of the tree paints of the
// selection, by box.
func highlights(tr *Tree) map[*Box]paraSel {
	out := map[*Box]paraSel{}
	var walk func(bx *Box)
	walk = func(bx *Box) {
		if bx.para != nil && bx.para.hl.on {
			out[bx] = bx.para.hl
		}
		for _, k := range bx.kids {
			walk(k)
		}
	}
	walk(tr.rootBox)
	return out
}

// checkHighlights compares the highlights a change left with those a walk
// of the whole range computes, and the list of highlighted paragraphs with
// them.
func checkHighlights(t *testing.T, tr *Tree, step string) {
	t.Helper()
	got := highlights(tr)
	inList := map[*Box]bool{}
	for _, bx := range tr.tsel.hl {
		if bx.para != nil && bx.para.hl.on {
			inList[bx] = true
		}
	}
	tr.refreshHighlights() // the whole range again
	want := highlights(tr)
	if len(got) != len(want) {
		t.Errorf("%s: %d paragraphs highlighted, a full walk highlights %d", step, len(got), len(want))
	}
	for bx, w := range want {
		if g := got[bx]; g != w {
			t.Errorf("%s: paragraph %v paints %+v, a full walk %+v", step, bx.el.key, g, w)
		}
		if !inList[bx] {
			t.Errorf("%s: paragraph %v is highlighted but not listed", step, bx.el.key)
		}
	}
	if len(inList) != len(want) {
		t.Errorf("%s: %d paragraphs listed, %d highlighted", step, len(inList), len(want))
	}
}

// TestTextSelChangeCostsWhatChanged: in a selection over 150 paragraphs, a
// drag that extends the focus by a word inside its paragraph, or to the
// next paragraph, computes the parts of those paragraphs only, and only
// the paragraphs whose part changed are painted again.
func TestTextSelChangeCostsWhatChanged(t *testing.T) {
	x := newKeys(t, manyParas(170), 420, 5000)
	tr := x.tree()
	from, to := textAt(t, tr, 10, 4, 1), textAt(t, tr, 160, 10, 1)
	x.send(move(from.X, from.Y), down(from.X, from.Y))
	x.send(moveHeld(to.X, to.Y))
	start := tr.tsel.focus
	if start.bx != paraOf(t, tr, 160) {
		t.Fatalf("the drag ends in %v, want paragraph 160", start.bx.el.key)
	}
	vers := func() map[*Box]uint64 {
		m := map[*Box]uint64{}
		for i := range 170 {
			bx := paraOf(t, tr, i)
			m[bx] = bx.para.ver
		}
		return m
	}
	step := func(name string, k any, off int, wantTouched int, wantRepainted ...any) {
		t.Helper()
		before := vers()
		pt := textAt(t, tr, k, off, 1)
		x.send(moveHeld(pt.X, pt.Y))
		if f := tr.tsel.focus; f.bx != paraOf(t, tr, k) || f.cur.Offset != off {
			t.Fatalf("%s: the focus is at %v:%d, want %v:%d", name, f.bx.el.key, f.cur.Offset, k, off)
		}
		if n := tr.tsel.touched; n > wantTouched {
			t.Errorf("%s: the change computed the parts of %d paragraphs, want at most %d", name, n, wantTouched)
		}
		after := vers()
		want := map[*Box]bool{}
		for _, r := range wantRepainted {
			want[paraOf(t, tr, r)] = true
		}
		for bx, v := range after {
			if changed := v != before[bx]; changed != want[bx] {
				t.Errorf("%s: paragraph %v painted again: %v, want %v", name, bx.el.key, changed, want[bx])
			}
		}
		checkHighlights(t, tr, name)
	}
	step("a word further", 160, 15, 1, 160)
	step("into the next paragraph", 161, 4, 2, 160, 161)
	step("back", 160, 15, 2, 160, 161)
	// Paragraph 164 holds a badge after offset 8: it is not reached.
	step("four paragraphs further", 164, 8, 5, 160, 161, 162, 163, 164)
	end := textAt(t, tr, 164, 8, 1)
	x.send(up(end.X, end.Y))
	x.b.Advance(time.Second) // no double click with the next press
	// A Shift press extends the same way: paragraph 164 to its end, its
	// badge, and paragraph 165.
	ext := textAt(t, tr, 165, 6, 1)
	shift := down(ext.X, ext.Y)
	shift.Mods = platform.ModShift
	x.send(move(ext.X, ext.Y), shift, up(ext.X, ext.Y))
	if f := tr.tsel.focus; f.bx != paraOf(t, tr, 165) || f.cur.Offset != 6 {
		t.Fatalf("Shift press: the focus is at %v:%d, want 165:6", f.bx.el.key, f.cur.Offset)
	}
	if n := tr.tsel.touched; n > 3 {
		t.Errorf("a Shift press one paragraph further computed the parts of %d paragraphs, want at most 3", n)
	}
	checkHighlights(t, tr, "Shift press")
}

// TestTextSelChangesMatchFullWalks moves the ends of a selection at random
// (in paragraphs and in the badges inside them, forward and backward) and
// checks after every change that the highlights are those of a walk of the
// whole range.
func TestTextSelChangesMatchFullWalks(t *testing.T) {
	x := newKeys(t, manyParas(60), 420, 2500)
	tr := x.tree()
	root := tr.rootBox
	var boxes []*Box
	for i := range 60 {
		boxes = append(boxes, paraOf(t, tr, i))
		if i%7 == 3 {
			boxes = append(boxes, paraOf(t, tr, fmt.Sprintf("badge%d", i)))
		}
	}
	rnd := rand.New(rand.NewPCG(1, 2))
	pos := func() selPos {
		bx := boxes[rnd.IntN(len(boxes))]
		n := len(bx.para.lay.Text())
		return clampPos(selPos{bx: bx, cur: text.Cursor{Offset: rnd.IntN(n + 1)}})
	}
	anchor, focus := pos(), pos()
	tr.setSel(root, anchor, focus)
	for i := range 600 {
		switch rnd.IntN(8) {
		case 0:
			anchor = pos()
		case 7: // a collapsed selection now and then
			focus = anchor
		default:
			focus = pos()
		}
		tr.setSel(root, anchor, focus)
		// Checked every few changes (the check walks the whole range,
		// which starts the next change from a full walk): the changes in
		// between update the highlights of the changes before them.
		if i%5 == 4 {
			checkHighlights(t, tr, fmt.Sprintf("change %d", i))
		}
		if t.Failed() {
			return
		}
	}
}

// BenchmarkTextSelExtend extends the focus of a selection over 900
// paragraphs by a word and back, as a drag does on every move: the cost
// of one change.
func BenchmarkTextSelExtend(b *testing.B) {
	tr := newTestTree(b, 420, 20000, manyParas(1000), nil)
	tr.Frame()
	root := tr.rootBox
	first := keyed(tr, 50).box
	last := keyed(tr, 950).box
	a := selPos{bx: first, cur: text.Cursor{Offset: 4}}
	f1 := selPos{bx: last, cur: text.Cursor{Offset: 10}}
	f2 := selPos{bx: last, cur: text.Cursor{Offset: 15}}
	tr.setSel(root, a, f1)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		if i%2 == 0 {
			tr.setSel(root, a, f2)
		} else {
			tr.setSel(root, a, f1)
		}
	}
	b.ReportMetric(float64(tr.tsel.touched), "paragraphs/change")
}
