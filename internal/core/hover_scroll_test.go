package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// ---------------------------------------------------------------------
// Hover follows scrolled content (Chromium's fake mouse move).

// hoverLog records the pointer enter and leave events of rows, marked
// "~" when Rehovered.
func hoverLog(r *recorder, k string) []Child {
	f := func(name string) Prop {
		return On("pointer"+name, func(e PointerEvent) {
			m := ""
			if Rehovered(e) {
				m = "~"
			}
			r.add(name + m + ":" + k)
		})
	}
	return []Child{f("enter"), f("leave")}
}

// hoveredLeaf returns the innermost element the mouse hovers.
func hoveredLeaf(tr *Tree) *Element {
	for k, ps := range tr.in.ptrs {
		if k.kind == platform.PointerMouse && len(ps.over) > 0 {
			return ps.over[0]
		}
	}
	return nil
}

// expectHoverAt checks that the mouse hovers what is under (px, py), with
// the hover state on its row.
func expectHoverAt(t *testing.T, x *inputHarness, px, py float32, row string) {
	t.Helper()
	tr := x.tree()
	if got, want := hoveredLeaf(tr), tr.HitTest(px, py); got != want {
		t.Errorf("hovered %s, but %s is under the pointer", keyName(got), keyName(want))
	}
	if el := keyed(tr, row); el == nil || el.istate&style.StateHover == 0 {
		t.Errorf("%s is not hovered", row)
	}
}

func scrollRows(r *recorder, n int, rowClass string) []Child {
	var c []Child
	for i := range n {
		k := fmt.Sprintf("r%d", i)
		c = append(c, div(append([]Child{Key(k), Class(rowClass + " hover:bg-sky-500 transition-colors")}, hoverLog(r, k)...)...))
	}
	return c
}

func TestHoverFollowsScrolling(t *testing.T) {
	inner := func(r *recorder) Node {
		c := append([]Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")}, scrollRows(r, 20, "h-[50px]")...)
		return div(Class("p-0"), div(c...))
	}
	// The page itself scrolls (the root is the viewport's scroller).
	page := func(r *recorder) Node {
		return div(scrollRows(r, 20, "h-[100px]")...)
	}
	cases := []struct {
		name    string
		root    func(*recorder) Node
		cfg     Config
		ev      []platform.ScrollEvent
		want    string // the events after the scroll
		wantRow string
	}{
		// From (50, 20) on r0: 53 px of a notch puts r1 under the pointer.
		{"inner instant notch", inner, Config{InstantScroll: true}, []platform.ScrollEvent{notch(50, 20, 1)}, "leave~:r0 enter~:r1", "r1"},
		{"inner smooth notch", inner, Config{}, []platform.ScrollEvent{notch(50, 20, 1)}, "leave~:r0 enter~:r1", "r1"},
		{"inner touchpad", inner, Config{}, []platform.ScrollEvent{touchpad(50, 20, 40, platform.ScrollPhaseNone)}, "leave~:r0 enter~:r1", "r1"},
		{"inner touchpad gesture", inner, Config{InstantScroll: true}, []platform.ScrollEvent{
			touchpad(50, 20, 20, platform.ScrollPhaseUpdate), touchpad(50, 20, 20, platform.ScrollPhaseUpdate),
			touchpad(50, 20, 0, platform.ScrollPhaseEnd)}, "leave~:r0 enter~:r1", "r1"},
		// Two notches move the page by 106 px: r1 (100..200) is under y 20.
		{"page instant notches", page, Config{InstantScroll: true}, []platform.ScrollEvent{notch(50, 20, 2)}, "leave~:r0 enter~:r1", "r1"},
		{"page smooth notches", page, Config{}, []platform.ScrollEvent{notch(50, 20, 1), notch(50, 20, 1)}, "leave~:r0 enter~:r1", "r1"},
		{"page touchpad", page, Config{}, []platform.ScrollEvent{touchpad(50, 20, 120, platform.ScrollPhaseNone)}, "leave~:r0 enter~:r1", "r1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r recorder
			x := newScrollHost(t, tc.root(&r), 300, 300, tc.cfg)
			x.send(move(50, 20))
			x.settle()
			if got := r.String(); got != "enter:r0" {
				t.Fatalf("before scrolling: %q", got)
			}
			r.reset()
			for _, e := range tc.ev {
				x.wheel(e)
			}
			x.settle()
			if got := r.String(); got != tc.want {
				t.Errorf("after scrolling: %q, want %q", got, tc.want)
			}
			expectHoverAt(t, x, 50, 20, tc.wantRow)
			if el := keyed(x.tree(), "r0"); el.istate&style.StateHover != 0 {
				t.Error("r0 is still hovered")
			}
			// The hover style follows: r1 is restyled with hover:.
			x.runFor(time.Second)
			if bg := x.bg(tc.wantRow); bg.A == 0 {
				t.Errorf("%s has no hover background after the scroll", tc.wantRow)
			}
			// A real move afterwards is not marked.
			r.reset()
			x.send(move(50, 60))
			if g := r.String(); strings.Contains(g, "~") {
				t.Errorf("a pointer move marked as rehovered: %q", g)
			}
		})
	}
}

// A pointer that left the window hovers nothing after a scroll.
func TestHoverAfterScrollPointerOutside(t *testing.T) {
	var r recorder
	c := append([]Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")}, scrollRows(&r, 20, "h-[50px]")...)
	x := newScrollHost(t, div(Class("p-0"), div(c...)), 300, 300, Config{InstantScroll: true})
	x.send(move(50, 20), ptr(platform.PointerLeave, 50, 20, platform.ButtonNone, 0))
	r.reset()
	x.wheel(notch(50, 20, 1))
	x.settle()
	if got := r.String(); got != "" {
		t.Errorf("a pointer outside the window: %q", got)
	}
	if hoveredLeaf(x.tree()) != nil {
		t.Errorf("a pointer outside the window hovers %s", keyName(hoveredLeaf(x.tree())))
	}
}
