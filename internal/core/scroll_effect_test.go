package core

import (
	"fmt"
	"os"
	"strconv"
	"testing"
)

// Scrolls asked for by layout effects (ADR-176): they run after the
// frame's layout but before the scroll phase reads its extents, and must
// land where that layout puts the content, in the frame's own paint.

// rowsTo returns n keyed rows of 50 px for a scroller keyed k, the last
// one attached to last when it is not nil.
func rowsTo(k string, n int, last *ElementRef) []Child {
	out := make([]Child, n)
	for i := range n {
		c := []Child{Key(fmt.Sprintf("%s%d", k, i)), Class("h-[50px] border-b"), Textf("Row %d", i)}
		if i == n-1 && last != nil {
			c = append(c, AttachRef(*last))
		}
		out[i] = div(c...)
	}
	return out
}

// effectScrollers: 200 px scrollers of n rows (n starts at 10) that a
// layout effect scrolls to their end whenever n changes: "a" by
// ScrollIntoView on its last row, "b" by ScrollTo, "c" by ScrollBy, and
// "outer", with 300 px of content above a 100 px scroller "inner", by
// ScrollIntoView on inner's last row (both move).
func effectScrollers(set *Setter[int]) Node {
	comp := func() Node {
		n, s := UseState(10)
		*set = s
		aLast, b, c, innerLast := UseElementRef(), UseElementRef(), UseElementRef(), UseElementRef()
		UseLayoutEffect(func() func() {
			aLast.ScrollIntoView()
			b.ScrollTo(0, 1e6)
			c.ScrollBy(0, 1e6)
			innerLast.ScrollIntoView()
			return nil
		}, n)
		list := func(k string, ref *ElementRef, last *ElementRef) Node {
			ch := []Child{Key(k), Class("h-[200px] w-[120px] overflow-y-auto")}
			if ref != nil {
				ch = append(ch, AttachRef(*ref))
			}
			return div(append(ch, rowsTo(k, n, last)...)...)
		}
		inner := []Child{Key("inner"), Class("h-[100px] overflow-y-auto")}
		inner = append(inner, rowsTo("inner", n, &innerLast)...)
		return div(Class("flex gap-2"),
			list("a", nil, &aLast), list("b", &b, nil), list("c", &c, nil),
			div(Key("outer"), Class("h-[200px] w-[120px] overflow-y-auto"), div(Class("h-[300px]")), div(inner...)),
		)
	}
	return Component(comp)
}

// checkAtEnd checks the offsets and the painted rows of effectScrollers
// with n rows after a frame.
func checkAtEnd(t *testing.T, tr *Tree, n int, what string) {
	t.Helper()
	end := float32(50*n - 200)
	for _, k := range []string{"a", "b", "c"} {
		if got := scr(t, tr, k).pos.Y; got != end {
			t.Errorf("%s: %s at %g, want %g", what, k, got, end)
		}
		last := windowRect(keyed(tr, fmt.Sprintf("%s%d", k, n-1)).box)
		if box := windowRect(keyed(tr, k).box); !approx(last.Y1, box.Y1) {
			t.Errorf("%s: the last row of %s painted at %v, its scroller at %v", what, k, last, box)
		}
	}
	if a, b := scr(t, tr, "inner").pos.Y, scr(t, tr, "outer").pos.Y; a != float32(50*n-100) || b != 200 {
		t.Errorf("%s: inner %g, outer %g; want %d, 200", what, a, b, 50*n-100)
	}
	last := windowRect(keyed(tr, fmt.Sprintf("inner%d", n-1)).box)
	if box := windowRect(keyed(tr, "outer").box); !approx(last.Y1, box.Y1) {
		t.Errorf("%s: inner's last row painted at %v, outer at %v", what, last, box)
	}
}

// TestLayoutEffectScrollsInItsFrame: on mount and when rows are added or
// removed, the frame that lays the rows out is painted scrolled to them,
// with no lag of one row.
func TestLayoutEffectScrollsInItsFrame(t *testing.T) {
	var set Setter[int]
	tr := newTestTree(t, 600, 300, effectScrollers(&set), nil)
	tr.Frame()
	checkAtEnd(t, tr, 10, "first frame")
	for _, n := range []int{11, 15, 12} {
		set.Set(n)
		tr.Frame()
		checkAtEnd(t, tr, n, fmt.Sprintf("frame with %d rows", n))
	}
}

// TestLayoutEffectScrollSmooth: a smooth ScrollTo from a layout effect
// animates toward the end of the frame's extents.
func TestLayoutEffectScrollSmooth(t *testing.T) {
	var set Setter[int]
	comp := func() Node {
		n, s := UseState(10)
		set = s
		ref := UseElementRef()
		UseLayoutEffect(func() func() {
			ref.ScrollTo(0, 1e6)
			return nil
		}, n)
		return div(append([]Child{AttachRef(ref), Key("list"), Class("h-[200px] overflow-y-auto scroll-smooth")}, rowsTo("r", n, nil)...)...)
	}
	tr := newTestTree(t, 300, 300, Component(comp), nil)
	tr.Frame()
	s := scr(t, tr, "list")
	if a := s.anim[1]; !a.active || a.to != 300 || s.pos.Y != 0 {
		t.Errorf("first frame: offset %g, animation %+v; want 0 animating to 300", s.pos.Y, a)
	}
	set.Set(12)
	tr.Frame()
	if a := s.anim[1]; !a.active || a.to != 400 {
		t.Errorf("after two rows: animation %+v; want to 400", a)
	}
}

// TestLayoutEffectFocusRevealsNewElement: focusing an element mounted in
// the frame scrolls it into view before that frame is painted.
func TestLayoutEffectFocusRevealsNewElement(t *testing.T) {
	comp := func() Node {
		ref := UseElementRef()
		UseLayoutEffect(func() func() {
			ref.Focus()
			return nil
		}, Once)
		c := []Child{Key("list"), Class("h-[200px] overflow-y-auto")}
		for i := range 10 {
			b := []Child{Key(i), Class("block h-[50px] w-full")}
			if i == 9 {
				b = append(b, AttachRef(ref))
			}
			c = append(c, button(b...))
		}
		return div(c...)
	}
	tr := newTestTree(t, 300, 300, Component(comp), nil)
	tr.Frame()
	if got := scr(t, tr, "list").pos.Y; got != 300 {
		t.Errorf("first frame: %g, want 300", got)
	}
}

// effectScrollScene: a chat-like column whose layout effects keep its
// scrollers at their end as rows come and go: ScrollIntoView of the last
// row (under a sticky header, rows with AnimateLayout), ScrollTo, and a
// smooth ScrollTo of a scroller nested in another.
func effectScrollScene(st *sceneState) Node {
	comp := func() Node {
		n, set := UseState(6)
		st.setters[0] = set
		st.refs[0], st.refs[1], st.refs[2] = UseElementRef(), UseElementRef(), UseElementRef()
		last := UseElementRef()
		UseLayoutEffect(func() func() {
			last.ScrollIntoView()
			st.refs[1].ScrollTo(0, 1e6)
			st.refs[2].ScrollTo(0, 1e6)
			return nil
		}, n)
		row := func(k string, i int, extra ...Child) Node {
			cls := "flex h-9 items-center gap-2 border-b border-slate-100 px-2 text-sm"
			if i%3 == 1 {
				cls += " bg-linear-to-r from-white to-sky-100"
			}
			c := []Child{Key(fmt.Sprintf("%s%d", k, i)), Class(cls), div(Class("size-5 rounded-full bg-indigo-400")), Textf("Message %d", i)}
			return div(append(c, extra...)...)
		}
		chat := []Child{AttachRef(st.refs[0]), Key("chat"), Class("h-[200px] w-40 overflow-y-auto rounded-lg bg-white"),
			div(Class("sticky top-0 z-10 h-7 bg-slate-800 px-2 text-xs text-white"), Text("Today"))}
		log := []Child{AttachRef(st.refs[1]), Key("log"), Class("h-[160px] w-32 overflow-y-auto bg-slate-50")}
		inner := []Child{AttachRef(st.refs[2]), Key("inner"), Class("h-[90px] overflow-y-auto scroll-smooth bg-amber-50")}
		for i := range n {
			var extra []Child
			if i == n-1 {
				extra = append(extra, AttachRef(last))
			}
			chat = append(chat, row("c", i, append(extra, AnimateLayout())...))
			log = append(log, row("l", i))
			inner = append(inner, row("i", i))
		}
		return div(Class("flex h-screen gap-3 bg-slate-100 p-3"),
			div(chat...), div(log...),
			div(Key("outer"), Class("h-[200px] w-36 overflow-y-auto bg-white"), div(Class("h-[150px] p-2 text-xs"), Text("Above")), div(inner...)),
		)
	}
	return Component(comp)
}

// effectScrollOps adds and removes rows more often than commonOps would.
func effectScrollOps(x *damageHarness, st *sceneState) string {
	switch x.rng.IntN(3) {
	case 0:
		d := 1 + x.rng.IntN(2)
		st.setters[0].Update(func(n int) int { return n + d })
		return fmt.Sprintf("add%d", d)
	case 1:
		st.setters[0].Update(func(n int) int { return max(n-1, 1) })
		return "remove"
	}
	return commonOps(x, st)
}

// TestDamageLayoutEffectScrolls: incremental frames equal full ones while
// layout effects scroll in the frames that add and remove rows.
func TestDamageLayoutEffectScrolls(t *testing.T) {
	steps := 120
	if testing.Short() {
		steps = 30
	}
	if n, err := strconv.Atoi(os.Getenv("TELA_DAMAGE_STEPS")); err == nil {
		steps = n
	}
	runScene(t, "layout_effect_scrolls", effectScrollScene, effectScrollOps, Config{InstantScroll: true}, steps)
	runScene(t, "layout_effect_scrolls_smooth", effectScrollScene, effectScrollOps, Config{}, steps/2)
}
