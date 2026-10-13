package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1280, 800

// mount opens the screen in dev mode, where any dev message fails the test.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h), telatest.Dev()}, opts...)
	return telatest.Mount(t, tela.C(Dashboard), opts...)
}

// TestNoDevMessages fails on an unknown class or any other dev warning at
// several window sizes.
func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		mount(t, s[0], s[1])
	}
}

// TestGolden is the main screen at 1280x800, at scale 1 and 2.
func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale)).Golden(fmt.Sprintf("dashboard_x%g.png", scale))
		})
	}
}

// TestLayout checks that the four stat cards share a row at 1280 and form
// two rows of two at 800, and that the page stays inside the window.
func TestLayout(t *testing.T) {
	for _, tc := range []struct {
		w, h       float32
		rows, cols int
	}{{1280, 800, 1, 4}, {800, 600, 2, 2}} {
		a := mount(t, tc.w, tc.h)
		titles := []string{"Total Revenue", "Subscriptions", "Sales", "Active Now"}
		ys := map[float32]int{}
		for _, title := range titles {
			x, y, w, _ := a.Bounds(telatest.ByText(title, telatest.Exact()))
			ys[y]++
			if x+w > tc.w || y < 0 {
				t.Errorf("%vx%v: %q at (%v, %v) leaves the window", tc.w, tc.h, title, x, y)
			}
		}
		if len(ys) != tc.rows {
			t.Errorf("%vx%v: stat cards in %d rows, want %d (%v)", tc.w, tc.h, len(ys), tc.rows, ys)
		}
		for y, n := range ys {
			if n != tc.cols {
				t.Errorf("%vx%v: row at y=%v has %d cards, want %d", tc.w, tc.h, y, n, tc.cols)
			}
		}
	}
}

// raisedTab locates the tabs drawn raised: the active one.
var raisedTab = telatest.ByClass("rounded-md", "px-3", "py-1", "bg-white")

// TestTabs clicks a tab: it becomes the raised one and the others do not.
// Only the Tabs component builds; the rest of the page is untouched.
func TestTabs(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectCount(raisedTab, 1)
	before := a.Stats()
	analytics := telatest.ByRole("button", "Analytics")
	a.Click(analytics)
	a.ExpectCount(raisedTab, 1)
	ax, _, aw, _ := a.Bounds(analytics)
	rx, _, rw, _ := a.Bounds(raisedTab)
	if ax+aw/2 != rx+rw/2 {
		t.Fatalf("after clicking Analytics, the raised tab is centered at %v, not at the Analytics tab (%v)", rx+rw/2, ax+aw/2)
	}
	a.ExpectCount(analytics.Filter(telatest.HasClass("shadow-sm")), 1) // the active tab has a shadow
	after := a.Stats()
	if got := after.Builds - before.Builds; got > 5 {
		t.Errorf("changing the tab built %d components, want only Tabs and its two tab buttons", got)
	}
}

// TestNavLink switches the header navigation.
func TestNavLink(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(telatest.ByRole("button", "Customers"))
	a.ExpectCount(telatest.ByRole("button", "Customers").Filter(telatest.HasClass("text-zinc-950")), 1)
	a.ExpectCount(telatest.ByRole("button", "Settings").Filter(telatest.HasClass("text-zinc-950")), 0)
}

// TestBarHover checks that hovering a column recolors its bar and builds
// nothing: group-hover: restyles the elements that test it.
func TestBarHover(t *testing.T) {
	a := mount(t, windowW, windowH)
	cols := telatest.ByClass("group", "cursor-pointer")
	a.ExpectCount(cols, len(months))
	cx, cy, cw, ch := a.Bounds(cols.Nth(2)) // March
	x, y := int(cx+cw/2), int(cy+ch)-10
	before := a.Screenshot().RGBAAt(x, y)
	builds := a.Stats().Builds
	a.MouseMove(float32(x), float32(y))
	after := a.Screenshot().RGBAAt(x, y)
	if after == before {
		t.Fatalf("hovering a bar did not change its color (%v)", after)
	}
	if after.B < 200 || after.R > 140 {
		t.Errorf("hovered bar is %v, want indigo", after)
	}
	if got := a.Stats().Builds; got != builds {
		t.Errorf("hover built %d components", got-builds)
	}
	// Moving to another bar restores the first one.
	dx, dy, dw, dh := a.Bounds(cols.Nth(3))
	a.MouseMove(dx+dw/2, dy+dh-10)
	if got := a.Screenshot().RGBAAt(x, y); got != before {
		t.Errorf("bar did not return to %v, is %v", before, got)
	}
}
