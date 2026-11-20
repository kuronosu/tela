package core

import (
	"strings"
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

// newHitTree returns an instrumented tree of 400x300 with root mounted and
// laid out, and its semantics built.
func newHitTree(t *testing.T, root Node) *Tree {
	t.Helper()
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}, InstantScroll: true})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.Mount(root)
	tr.Frame()
	tr.Semantics()
	return tr
}

// The visible part is where the box is painted: through the transforms
// of its ancestors (a toast centered with -translate-x-1/2, a scaled
// panel), cut by clips that are transformed too, so that its center hits
// it. The clips are overflow-clip, which does not scroll: an
// overflow-hidden box would scroll the box into its view.
func TestInstrumentRevealTransformed(t *testing.T) {
	tr := newHitTree(t, div(Class("relative h-full"),
		// Laid out at x 200 to 400, painted at 100 to 300; its clip cuts
		// the inner box (laid out at 350 to 450) to 350 to 400, painted at
		// 250 to 300.
		div(Class("absolute left-1/2 top-[50px] h-[60px] w-[200px] -translate-x-1/2 overflow-clip"),
			div(Key("toast"), Class("ml-[150px] h-[40px] w-[100px]"))),
		// Laid out at 100 to 200 by 150 to 250, scaled by half around its
		// center: its clip is 125 to 175 by 175 to 225, and the wide child
		// (100 to 300 by 150 to 200) shows at 125 to 175 by 175 to 200.
		div(Class("absolute left-[100px] top-[150px] size-[100px] scale-50 overflow-clip"),
			div(Key("wide"), Class("h-[50px] w-[200px]"))),
		// An untransformed clip around a translated box: the clip stays,
		// the box moves under it.
		div(Class("absolute left-0 top-[260px] h-[30px] w-[100px] overflow-clip"),
			div(Key("moved"), Class("h-[30px] w-[100px] translate-x-[50px]"))),
	))
	for _, tt := range []struct {
		key  string
		want gfx.Rect
	}{
		{"toast", gfx.Rect{X0: 250, Y0: 50, X1: 300, Y1: 90}},
		{"wide", gfx.Rect{X0: 125, Y0: 175, X1: 175, Y1: 200}},
		{"moved", gfx.Rect{X0: 50, Y0: 260, X1: 100, Y1: 290}},
	} {
		id := idOfKey(t, tr, tt.key)
		vis, moved, err := tr.RevealElement(id)
		if err != nil || moved || vis != tt.want {
			t.Errorf("%s: visible %v moved %v err %v, want %v", tt.key, vis, moved, err, tt.want)
			continue
		}
		c := vis.Center()
		if hit, _, within := tr.HitElement(c.X, c.Y, id); !within {
			t.Errorf("%s: its visible part's center %v hits %d, not it", tt.key, c, hit)
		}
	}
}

// An inline element's bounds and visible part are its fragments, and a
// point of its paragraph off them does not reach it: a link in a
// paragraph, one that wraps, and a text.
func TestInstrumentInlineTarget(t *testing.T) {
	tr := newHitTree(t, div(
		p(Key("para"), Class("w-[300px] text-[16px]"),
			Text("Read the "), span(Key("link"), Class("px-[2px]"), Text("docs")), Text(" today, then the rest.")),
		p(Key("narrow"), Class("w-[90px] text-[16px]"),
			Text("aa "), span(Key("wrap"), Text("bbbb cccc dddd eeee"))),
	))
	link := keyed(tr, "link")
	id := tr.ins.idOf(link)
	info, ok := tr.Element(id)
	if !ok || info.Bounds == nil || info.Bounds.Empty() {
		t.Fatalf("the link's bounds: %+v", info.Bounds)
	}
	para, _ := tr.Element(idOfKey(t, tr, "para"))
	if *info.Bounds == *para.Bounds || !para.Bounds.ContainsRect(*info.Bounds) {
		t.Errorf("the link's bounds %v, its paragraph's %v", *info.Bounds, *para.Bounds)
	}
	if listed := insByKind(tr, "span")[0]; listed.Bounds == nil || *listed.Bounds != *info.Bounds {
		t.Errorf("Elements lists the link with bounds %v, Element %v", listed.Bounds, *info.Bounds)
	}
	// Its visible part is its fragment's border box, inside its bounds
	// (which hold the line-tall selection rectangle of its text too).
	vis, _, err := tr.RevealElement(id)
	if err != nil || vis.Empty() || !info.Bounds.ContainsRect(vis) || vis.Width() != info.Bounds.Width() {
		t.Fatalf("the link's visible part %v (err %v), its bounds %v", vis, err, *info.Bounds)
	}
	c := vis.Center()
	if hit, _, within := tr.HitElement(c.X, c.Y, id); !within || hit != id {
		t.Errorf("the link's center hits %d within %v", hit, within)
	}
	// The paragraph's start ("Read") hits the paragraph, an ancestor of
	// the link, off its fragments.
	r := para.Bounds
	if hit, _, within := tr.HitElement(r.X0+4, r.Y0+r.Height()/2, id); within || hit != para.ID {
		t.Errorf("the paragraph's start hits %d within %v, want the paragraph %d off the link", hit, within, para.ID)
	}

	// The text after the link: the paragraph's element is hit on it and
	// not on the link or the paragraph's start.
	var after *Element
	for _, c := range keyed(tr, "para").children {
		if c.kind == elemText && strings.HasPrefix(c.text, " today") {
			after = c
		}
	}
	aid := tr.ins.idOf(after)
	avis, _, _ := tr.RevealElement(aid)
	if avis.Empty() || avis.Intersects(*info.Bounds) {
		t.Fatalf("the text's visible part %v (the link is at %v)", avis, *info.Bounds)
	}
	ac := avis.Center()
	if hit, _, within := tr.HitElement(ac.X, ac.Y, aid); !within || hit != para.ID {
		t.Errorf("the text's center hits %d within %v", hit, within)
	}
	if _, _, within := tr.HitElement(r.X0+4, r.Y0+r.Height()/2, aid); within {
		t.Error("the paragraph's start reaches the text after the link")
	}
	if _, _, within := tr.HitElement(c.X, c.Y, aid); within {
		t.Error("the link reaches the text after it")
	}

	// A wrapped link shows the visible part of its first fragment, whose
	// center is on it.
	wrap := keyed(tr, "wrap")
	frags := wrap.box.inPara.para.inlineRects(wrap, nil)
	if len(frags) < 2 {
		t.Fatalf("the wrapped link has %d fragments", len(frags))
	}
	wid := tr.ins.idOf(wrap)
	wvis, _, _ := tr.RevealElement(wid)
	parts := tr.paraParts(wrap, false, nil)
	if len(parts) == 0 || wvis != parts[0] {
		t.Errorf("the wrapped link's visible part %v, want its first fragment of %v", wvis, parts)
	}
	wc := wvis.Center()
	if hit, _, within := tr.HitElement(wc.X, wc.Y, wid); !within || hit != wid {
		t.Errorf("the wrapped link's center %v hits %d within %v", wc, hit, within)
	}
}
