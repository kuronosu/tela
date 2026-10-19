package core

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// stickySections is a scroll container of sections, each with a sticky
// header (top-0) and rows; the last section's header also sticks to the
// bottom.
func stickySections(ref ElementRef, sections, rows int) Node {
	kids := []Child{AttachRef(ref), Key("list"), Class("h-[200px] w-[240px] overflow-y-auto bg-white")}
	for s := range sections {
		sec := []Child{Key(fmt.Sprintf("sec%d", s)), Class("relative")}
		sec = append(sec, div(Key(fmt.Sprintf("head%d", s)), Class("sticky top-0 z-10 h-8 bg-slate-800 px-2 text-white"), Textf("Section %d", s)))
		for r := range rows {
			sec = append(sec, div(Key(fmt.Sprintf("row%d.%d", s, r)), Class("h-7 border-b border-slate-100 px-2 text-sm"), Textf("Row %d", r)))
		}
		if s == sections-1 {
			sec = append(sec, div(Key("foot"), Class("sticky bottom-0 h-8 bg-sky-600 px-2 text-white"), Text("Footer")))
		}
		kids = append(kids, div(sec...))
	}
	return div(kids...)
}

func TestStickyHeadersFollowScroll(t *testing.T) {
	var ref ElementRef
	comp := func() Node {
		ref = UseElementRef()
		return stickySections(ref, 3, 8)
	}
	tr := newTestTree(t, 300, 300, Component(comp), nil)
	tr.Frame()
	head := func(k string) gfx.Rect { return tr.boxWindowRect(keyed(tr, k).box) }
	// Section height 32 + 8*28 = 256.
	cases := []struct {
		scroll float32
		h0, h1 float32
	}{
		{0, 0, 256},
		{100, 0, 156},  // head0 stuck at the top
		{230, -6, 26},  // head0 pushed out by its section's end (256-32-230 = -6)
		{256, -32, 0},  // head1 stuck
		{400, -176, 0}, // head1 stuck, head0 gone
	}
	for _, c := range cases {
		ref.ScrollTo(0, c.scroll)
		tr.Frame()
		if g := head("head0").Y0; g != c.h0 {
			t.Errorf("scroll %g: head0 at %g, want %g", c.scroll, g, c.h0)
		}
		if g := head("head1").Y0; g != c.h1 {
			t.Errorf("scroll %g: head1 at %g, want %g", c.scroll, g, c.h1)
		}
		// The hit test finds the stuck header at the top.
		if c.scroll == 100 {
			if el := tr.HitTest(20, 10); el == nil || el.key != "head0" {
				t.Errorf("scroll %g: hit %v, want head0", c.scroll, el)
			}
		}
	}
	// The last section's footer sticks to the bottom, but not above its
	// section's top.
	for _, c := range [][2]float32{{0, 512}, {400, 168}, {600, 168}} {
		ref.ScrollTo(0, c[0])
		tr.Frame()
		if g := head("foot").Y0; g != c[1] {
			t.Errorf("scroll %g: footer at %g, want %g", c[0], g, c[1])
		}
	}
}

func TestStickyInsetsAreNotRelativeOffsets(t *testing.T) {
	tr := newTestTree(t, 200, 200, div(Class("h-[100px] overflow-auto"),
		div(Key("s"), Class("sticky top-4 left-3 h-5")), div(Class("h-[400px]"))), nil)
	tr.Frame()
	n := mustFind(t, tr, "s")
	if n.Rect.Y0 != 0 || n.Rect.X0 != 0 {
		t.Errorf("layout rect %v: sticky insets moved the box in layout", n.Rect)
	}
	r := tr.boxWindowRect(keyed(tr, "s").box)
	if r.Y0 != 16 || r.X0 != 0 {
		t.Errorf("painted at %v, want y 16 (stuck by top-4), x 0 (no horizontal scroll)", r)
	}
	_ = style.PositionSticky
}

// TestStickyVirtualListHeaders: a section header of a VirtualList stays
// stuck while its items scroll under it, kept mounted after it leaves the
// built range, and the next header replaces it.
func TestStickyVirtualListHeaders(t *testing.T) {
	var ref ElementRef
	item := func(i int) Node {
		if i%20 == 0 {
			return div(Key("h"), Class("sticky top-0 h-8 bg-slate-800"), Textf("Section %d", i/20))
		}
		return div(Class("h-7"), Textf("Item %d", i))
	}
	comp := func() Node {
		ref = UseElementRef()
		return VirtualList(1000, func(i int) float32 {
			if i%20 == 0 {
				return 32
			}
			return 28
		}, item, AttachRef(ref), Class("h-[200px]"))
	}
	tr := newTestTree(t, 300, 300, Component(comp), nil)
	tr.Frame()
	vl := virtualListOf(ref.element())
	header := func(sec int) *Box {
		_, content := vl.boxes()
		for _, w := range content.kids {
			if v := w.virtMark(); v != nil && v.idx == sec*20 && len(w.kids) > 0 {
				return w.kids[0]
			}
		}
		return nil
	}
	// Section 0: 32 + 19*28 = 564 px. Scrolled to 400, items 0..~13 are
	// before the window; the header is kept and stuck at the top.
	for _, y := range []float32{100, 400} {
		ref.ScrollTo(0, y)
		tr.Frame()
		tr.Frame()
		h := header(0)
		if h == nil {
			t.Fatalf("scroll %g: header 0 not mounted (built %v)", y, vl.built)
		}
		if r := tr.boxWindowRect(h); r.Y0 != 0 {
			t.Errorf("scroll %g: header 0 at %v, want stuck at 0", y, r)
		}
		if el := tr.HitTest(10, 5); el == nil || el.box != h {
			t.Errorf("scroll %g: hit %v, want header 0", y, el)
		}
	}
	// Past section 1's header, it covers header 0 (painted later).
	ref.ScrollTo(0, 700)
	tr.Frame()
	tr.Frame()
	if h := header(1); h == nil || tr.boxWindowRect(h).Y0 != 0 {
		t.Fatalf("scroll 700: header 1 not stuck at the top")
	}
	if el := tr.HitTest(10, 5); el == nil || el.box != header(1) {
		t.Errorf("scroll 700: hit %v, want header 1", el)
	}
}

// stickyScene: sticky section headers (top, pushed out by their section,
// a footer stuck to the bottom), a sticky first column in a horizontal
// strip, a sticky inline tag, a page that scrolls with a sticky page
// header, a fixed button and a fixed box inside a transformed card.
func stickyScene(st *sceneState) Node {
	comp := func() Node {
		st.refs[0], st.refs[1] = UseElementRef(), UseElementRef()
		v, set := UseState(0)
		st.setters[0] = set
		list := []Child{AttachRef(st.refs[0]), Key("list"), Class("mx-3 h-[180px] overflow-y-auto rounded-lg border border-slate-200 bg-white")}
		for s := range 4 {
			hcls := "sticky top-0 z-10 h-8 bg-slate-800 px-2 text-sm text-white"
			if v%2 == 1 && s == 1 {
				hcls = "sticky top-2 z-10 h-10 bg-rose-700 px-2 text-sm text-white"
			}
			sec := []Child{Key(s), div(Class(hcls), Textf("Section %d", s))}
			for r := range 6 {
				cls := "h-7 border-b border-slate-100 px-2 text-sm hover:bg-sky-50"
				if r == 2 {
					cls += " bg-linear-to-r from-white to-amber-100"
				}
				sec = append(sec, div(Key(r), Class(cls), Textf("Row %d.%d", s, r)))
			}
			if s == 3 {
				sec = append(sec, div(Class("sticky bottom-0 h-6 bg-sky-600/90 px-2 text-xs text-white"), Text("Footer")))
			}
			list = append(list, div(sec...))
		}
		strip := []Child{AttachRef(st.refs[1]), Key("strip"), Class("mx-3 mt-3 flex overflow-x-auto rounded-lg bg-slate-100")}
		strip = append(strip, div(Class("sticky left-0 z-10 w-20 shrink-0 bg-indigo-600 p-2 text-xs text-white shadow-lg"), Text("Name")))
		for i := range 10 {
			strip = append(strip, div(Key(i), Class("h-12 w-24 shrink-0 border-r border-slate-200 p-2 text-xs"), Textf("Col %d", i)))
		}
		return div(Class("relative bg-white pb-[400px]"),
			div(Class("sticky top-0 z-20 h-8 bg-white/90 px-3 text-sm shadow"), Text("Page header")),
			div(list...),
			div(strip...),
			div(Class("mx-3 mt-3 h-16 rotate-1 rounded-xl bg-amber-100 p-2 text-xs"), Text("Transformed"),
				div(Class("fixed right-2 bottom-2 size-8 rounded-full bg-emerald-500"))),
			button(Class("fixed right-4 bottom-4 z-30 rounded-full bg-indigo-600 px-3 py-1 text-sm text-white shadow-lg"), Text("Fixed")),
		)
	}
	return Component(comp)
}

// virtualStickyScene: a VirtualList whose every tenth item is a sticky
// section header.
func virtualStickyScene(st *sceneState) Node {
	comp := func() Node {
		st.refs[0] = UseElementRef()
		item := func(i int) Node {
			if i%10 == 0 {
				return div(Class("sticky top-0 z-10 h-8 bg-slate-800 px-3 text-sm text-white"), Textf("Section %d", i/10))
			}
			return div(Class("h-7 border-b border-slate-100 px-3 text-sm hover:bg-sky-50"), Textf("Item %d", i))
		}
		return div(Class("flex h-screen flex-col bg-slate-50"),
			div(Class("h-10 shrink-0 bg-slate-900 px-3 text-white"), Text("Virtual")),
			VirtualList(2000, func(i int) float32 {
				if i%10 == 0 {
					return 32
				}
				return 28
			}, item, AttachRef(st.refs[0]), Class("grow bg-white")),
		)
	}
	return Component(comp)
}

// TestDamageStickyAndFixed: incremental frames are bit-identical to full
// ones while sticky and fixed boxes scroll (ADR-061).
func TestDamageStickyAndFixed(t *testing.T) {
	steps := 160
	if testing.Short() {
		steps = 40
	}
	if n, err := strconv.Atoi(os.Getenv("TELA_DAMAGE_STEPS")); err == nil {
		steps = n
	}
	runScene(t, "sticky", stickyScene, commonOps, Config{InstantScroll: true}, steps)
	runScene(t, "sticky_smooth", stickyScene, commonOps, Config{}, steps)
	runScene(t, "virtual_sticky", virtualStickyScene, commonOps, Config{InstantScroll: true}, steps)
}
