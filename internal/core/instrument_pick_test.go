package core

import (
	"bytes"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// pickPage is a scroller with three rows, a translated box and a card
// component; clicks and pointer moves on them are counted.
func pickPage(hits map[string]int) Node {
	count := func(k string) []Child {
		return []Child{Key(k),
			On("click", func(MouseEvent) { hits["click:"+k]++ }),
			On("pointermove", func(PointerEvent) { hits["move:"+k]++ }),
			On("pointerdown", func(PointerEvent) { hits["down:"+k]++ })}
	}
	row := func(k string) Node {
		return Primitive(style.KindDiv, append(count(k), Class("h-[80px] bg-slate-200 border-b"))...)
	}
	return Primitive(style.KindDiv, append(count("page"), Class("relative h-full"),
		Primitive(style.KindDiv, Key("scroller"), Class("h-[100px] w-[200px] overflow-y-auto"), row("a"), row("b"), row("c")),
		Primitive(style.KindDiv, append(count("moved"), Class("absolute left-0 top-[150px] size-[40px] translate-x-[100px] bg-rose-400"))...),
		Primitive(style.KindDiv, Class("absolute left-[250px] top-[20px]"), insCard{Title: "card"}),
	)...)
}

// pickID returns the ID of the primitive with key k.
func pickID(t *testing.T, tr *Tree, k string) ElementID {
	t.Helper()
	for _, e := range tr.Elements(false) {
		if e.Key == k && !e.Component {
			return e.ID
		}
	}
	t.Fatalf("no element %q", k)
	return 0
}

// sameAsApp reports whether the window shows exactly the application: no
// overlay over it.
func sameAsApp(x *inputHarness) bool {
	tr := x.tree()
	want := tr.Rasterize(tr.ReferenceFrame(false), tr.Scale())
	got := x.w.Frame()
	return got != nil && got.Rect == want.Rect && bytes.Equal(got.Pix, want.Pix)
}

// Pick mode highlights the box under the pointer, through scroll offsets
// and transforms, keeps the pointer from the app, and picks with a
// primary press; Escape cancels.
func TestInstrumentPick(t *testing.T) {
	hits := map[string]int{}
	x := newScrollHost(t, pickPage(hits), 400, 300, Config{Dev: true, InstantScroll: true})
	tr := x.tree()
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	x.wheel(touchpad(50, 50, 100, platform.ScrollPhaseNone))
	x.frame()
	if s := scr(t, tr, "scroller"); s.pos.Y != 100 {
		t.Fatalf("scrolled to %v, want 100", s.pos.Y)
	}
	clear(hits)

	var picked []ElementID
	tr.SetPick(true, func(id ElementID) { picked = append(picked, id) })
	if !tr.Picking() || tr.Debug()&debugInspect == 0 {
		t.Fatalf("pick mode: %v, overlay flags %v", tr.Picking(), tr.Debug())
	}
	// (50, 30) is y 130 of the scrolled content: row b.
	x.send(move(50, 30))
	x.frame()
	if h := tr.ins.pick.hover; h == nil || h.key != "b" {
		t.Fatalf("hover %v, want b", h)
	}
	if sameAsApp(x) {
		t.Error("the highlight is not drawn")
	}
	if len(hits) != 0 {
		t.Errorf("the app got pointer events: %v", hits)
	}
	// The translated box is where it is painted, not where it is laid out.
	x.send(move(20, 170))
	if h := tr.ins.pick.hover; h == nil || h.key != "page" {
		t.Errorf("hover at the moved box's layout place: %v, want the page", h)
	}
	x.send(move(120, 170))
	if h := tr.ins.pick.hover; h == nil || h.key != "moved" {
		t.Fatalf("hover %v, want moved", h)
	}
	x.send(down(120, 170), moveHeld(122, 172), up(122, 172))
	x.frame()
	if len(picked) != 1 || picked[0] != pickID(t, tr, "moved") {
		t.Fatalf("picked %v, want the moved box %d", picked, pickID(t, tr, "moved"))
	}
	if tr.Picking() || tr.Debug() != 0 {
		t.Errorf("after the pick: picking %v, overlay flags %v", tr.Picking(), tr.Debug())
	}
	if len(hits) != 0 {
		t.Errorf("the app got pointer events of the pick: %v", hits)
	}
	if !sameAsApp(x) {
		t.Error("the window still shows the highlight")
	}
	// Then the pointer reaches the app again.
	x.click(120, 170)
	if hits["click:moved"] != 1 {
		t.Errorf("after pick mode: %v", hits)
	}

	// Escape cancels: the callback gets 0, and the key's release stays
	// out of the app too.
	clear(hits)
	tr.SetPick(true, func(id ElementID) { picked = append(picked, id) })
	x.send(move(50, 30))
	x.w.SetFocused(true)
	x.w.Inject(platform.KeyEvent{Action: platform.KeyPress, Key: platform.KeyEscape, Code: platform.CodeEscape})
	x.w.Inject(platform.KeyEvent{Action: platform.KeyRelease, Key: platform.KeyEscape, Code: platform.CodeEscape})
	x.b.Drain(x.h)
	if len(picked) != 2 || picked[1] != 0 || tr.Picking() || tr.ins.pick.escUp {
		t.Errorf("after Escape: picked %v, picking %v", picked, tr.Picking())
	}
	// Off, it calls nothing.
	tr.SetPick(true, func(id ElementID) { t.Error("called after SetPick(false)") })
	tr.SetPick(false, nil)
	x.click(50, 30)
	if hits["click:b"] != 1 {
		t.Errorf("after SetPick(false): %v", hits)
	}
}

// Highlight draws an element chosen elsewhere, a component by the boxes
// it renders, until it is cleared.
func TestInstrumentHighlight(t *testing.T) {
	x := newScrollHost(t, pickPage(map[string]int{}), 400, 300, Config{Dev: true})
	tr := x.tree()
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	card := insOne(t, tr, "core.insCard")
	tr.Highlight(card.ID)
	x.frame()
	if tr.Highlighted() != card.ID || sameAsApp(x) {
		t.Fatalf("highlighted %d, drawn %v", tr.Highlighted(), !sameAsApp(x))
	}
	// The overlay covers the card's box, not the rows.
	got, want := x.w.Frame(), tr.Rasterize(tr.ReferenceFrame(false), tr.Scale())
	div := tr.ins.element(card.ID).children[0].box
	r := tr.boxWindowRect(div)
	in := image.Pt(int(r.X0+r.Width()/2), int(r.Y0+r.Height()/2))
	if got.RGBAAt(in.X, in.Y) == want.RGBAAt(in.X, in.Y) {
		t.Errorf("the card's center %v is not highlighted", in)
	}
	if got.RGBAAt(100, 290) != want.RGBAAt(100, 290) {
		t.Error("the highlight covers more than the card")
	}
	// Other overlays keep it, and lose it when it is cleared.
	tr.SetDebug(DebugLayers)
	if tr.Debug() != DebugLayers|debugInspect {
		t.Errorf("flags %v", tr.Debug())
	}
	tr.Highlight(0)
	x.frame()
	if tr.Debug() != DebugLayers {
		t.Errorf("flags %v after Highlight(0)", tr.Debug())
	}
	tr.SetDebug(0)
	x.frame()
	if !sameAsApp(x) {
		t.Error("the window does not show the app alone")
	}
	// Turning the instrumentation off clears it too.
	tr.Highlight(card.ID)
	x.frame()
	tr.SetInstrument(false)
	x.frame()
	if tr.Debug() != 0 || !sameAsApp(x) {
		t.Errorf("after SetInstrument(false): flags %v", tr.Debug())
	}
}
