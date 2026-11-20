package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// hitPage is a scroller of five 40 px rows, 100 px tall at (0, 0), and a
// button covered by a dialog.
func hitPage() Node {
	row := func(k string) Node { return div(Key(k), Class("h-[40px] shrink-0"), Text(k)) }
	return div(Class("relative h-full"),
		div(Key("scroller"), Class("h-[100px] w-[200px] overflow-y-auto"), row("r0"), row("r1"), row("r2"), row("r3"), row("r4")),
		div(Key("btn"), Role("button"), AriaLabel("Save"), Class("absolute left-[20px] top-[150px] h-[40px] w-[100px]"), Text("Save")),
		div(Key("dlg"), Role("dialog"), AriaLabel("Confirm"), Class("absolute left-[60px] top-[140px] h-[80px] w-[200px] bg-white")),
	)
}

func idOfKey(t *testing.T, tr *Tree, k string) ElementID {
	t.Helper()
	el := keyed(tr, k)
	if el == nil {
		t.Fatalf("no element %q", k)
	}
	return tr.ins.idOf(el)
}

// RevealElement scrolls a row into view and gives its visible part, and
// HitElement tells a point that reaches an element from one covered by
// another.
func TestInstrumentRevealAndHit(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}, InstantScroll: true})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.Mount(hitPage())
	tr.Frame()
	tr.Semantics()

	// r1 is visible from 40 to 80: nothing moves.
	r1 := idOfKey(t, tr, "r1")
	vis, moved, err := tr.RevealElement(r1)
	if err != nil || moved || vis != (gfx.Rect{X0: 0, Y0: 40, X1: 200, Y1: 80}) {
		t.Fatalf("r1: %v moved %v err %v", vis, moved, err)
	}
	// r4 (160 to 200) is below the scroller: it scrolls by 100.
	r4 := idOfKey(t, tr, "r4")
	if _, moved, err = tr.RevealElement(r4); err != nil || !moved {
		t.Fatalf("r4: moved %v err %v", moved, err)
	}
	tr.Frame()
	vis, moved, err = tr.RevealElement(r4)
	if err != nil || moved || vis != (gfx.Rect{X0: 0, Y0: 60, X1: 200, Y1: 100}) {
		t.Fatalf("r4 after the scroll: %v moved %v err %v", vis, moved, err)
	}
	if id, _, within := tr.HitElement(100, 80, r4); !within || id != r4 {
		t.Errorf("hit at r4's center: %d within %v, want %d", id, within, r4)
	}
	// r1 is now scrolled out of the scroller, above it: its visible part
	// after RevealElement is all of it again.
	vis, moved, _ = tr.RevealElement(r1)
	if !moved || vis != (gfx.Rect{X0: 0, Y0: 0, X1: 200, Y1: 40}) {
		t.Errorf("r1 after the scroll: %v moved %v", vis, moved)
	}
	tr.Frame()
	tr.Semantics()

	// The dialog covers the button's right part: its center hits the
	// dialog, its left part the button.
	btn, dlg := idOfKey(t, tr, "btn"), idOfKey(t, tr, "dlg")
	vis, _, _ = tr.RevealElement(btn)
	if vis != (gfx.Rect{X0: 20, Y0: 150, X1: 120, Y1: 190}) {
		t.Fatalf("button: %v", vis)
	}
	c := vis.Center()
	id, node, within := tr.HitElement(c.X, c.Y, btn)
	if within || id != dlg || node == 0 || node != tr.NodeOfElement(dlg) {
		t.Errorf("hit at the button's center: %d node %d within %v, want the dialog %d node %d", id, node, within, dlg, tr.NodeOfElement(dlg))
	}
	if id, node, within = tr.HitElement(30, 170, btn); !within || id == 0 || node != tr.NodeOfElement(btn) {
		t.Errorf("hit at the button's left: %d node %d within %v", id, node, within)
	}
	if id, _, within = tr.HitElement(500, 10, btn); id != 0 || within {
		t.Errorf("outside the window: %d within %v", id, within)
	}
}

// A box clipped by an overflow-clip box, which does not scroll, shows only
// its visible part, and one clipped whole shows none.
func TestInstrumentRevealClipped(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.Mount(div(
		div(Class("h-[50px] w-[100px] overflow-x-clip"),
			div(Key("wide"), Class("h-[30px] w-[300px]"))),
		div(Class("h-[50px] w-[100px] overflow-clip"),
			div(Key("off"), Class("ml-[150px] h-[30px] w-[30px]"))),
	))
	tr.Frame()
	vis, _, err := tr.RevealElement(idOfKey(t, tr, "wide"))
	if err != nil || vis != (gfx.Rect{X0: 0, Y0: 0, X1: 100, Y1: 30}) {
		t.Errorf("wide: %v err %v", vis, err)
	}
	vis, _, err = tr.RevealElement(idOfKey(t, tr, "off"))
	if err != nil || !vis.Empty() {
		t.Errorf("off: %v err %v", vis, err)
	}
	if _, _, err = tr.RevealElement(9999); err == nil {
		t.Error("an unknown element: no error")
	}
}
