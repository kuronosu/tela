package main

import (
	"testing"

	"github.com/kuronosu/tela/examples/internal/exampletest"
)

const windowW, windowH = 1280, 800

// TestNoDevMessages fails on an unknown class or any other dev warning at
// several window sizes.
func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		a := exampletest.Open(t, Dashboard, s[0], s[1])
		a.CheckLogs()
	}
}

// TestGolden is the main screen at 1280x800, at scale 1 and 2.
func TestGolden(t *testing.T) {
	exampletest.Golden(t, "dashboard", Dashboard, windowW, windowH)
}

// TestLayout checks that the four stat cards share a row at 1280 and form
// two rows of two at 800, and that the page stays inside the window.
func TestLayout(t *testing.T) {
	for _, tc := range []struct {
		w, h       float32
		rows, cols int
	}{{1280, 800, 1, 4}, {800, 600, 2, 2}} {
		a := exampletest.Open(t, Dashboard, tc.w, tc.h)
		titles := []string{"Total Revenue", "Subscriptions", "Sales", "Active Now"}
		ys := map[float32]int{}
		for _, title := range titles {
			r := a.Text(title).Rect
			ys[r.Y0]++
			if r.X1 > tc.w || r.Y0 < 0 {
				t.Errorf("%vx%v: %q at %v leaves the window", tc.w, tc.h, title, r)
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

// TestTabs clicks a tab: it becomes the raised one and the others do not.
// Only the Tabs component builds; the rest of the page is untouched.
func TestTabs(t *testing.T) {
	a := exampletest.Open(t, Dashboard, windowW, windowH)
	if tabs := a.WithClass("rounded-md", "px-3", "py-1", "bg-white"); len(tabs) != 1 {
		t.Fatalf("%d active tabs at start, want 1", len(tabs))
	}
	before := a.Tree().Stats()
	a.ClickButton("Analytics")
	raised := a.WithClass("rounded-md", "px-3", "py-1", "bg-white")
	if len(raised) != 1 || raised[0].Rect.Center().X != a.Button("Analytics").Rect.Center().X {
		t.Fatalf("after clicking Analytics, raised tabs: %v", raised)
	}
	if !exampletest.HasClass(a.Button("Analytics"), "shadow-sm") {
		t.Error("the active tab has no shadow")
	}
	after := a.Tree().Stats()
	if got := after.Builds - before.Builds; got > 5 {
		t.Errorf("changing the tab built %d components, want only Tabs and its two tab buttons", got)
	}
	a.CheckLogs()
}

// TestNavLink switches the header navigation.
func TestNavLink(t *testing.T) {
	a := exampletest.Open(t, Dashboard, windowW, windowH)
	a.ClickButton("Customers")
	if !exampletest.HasClass(a.Button("Customers"), "text-zinc-950") {
		t.Error("Customers is not the active link after a click")
	}
	if exampletest.HasClass(a.Button("Settings"), "text-zinc-950") {
		t.Error("Settings looks active")
	}
}

// TestBarHover checks that hovering a column recolors its bar and builds
// nothing: group-hover: restyles the elements that test it.
func TestBarHover(t *testing.T) {
	a := exampletest.Open(t, Dashboard, windowW, windowH)
	cols := a.WithClass("group", "cursor-pointer")
	if len(cols) != len(months) {
		t.Fatalf("%d chart columns, want %d", len(cols), len(months))
	}
	c := cols[2].Rect // March
	x, y := int(c.Center().X), int(c.Y1)-10
	before := a.Image().RGBAAt(x, y)
	builds := a.Tree().Stats().Builds
	a.Move(float32(x), float32(y))
	after := a.Image().RGBAAt(x, y)
	if after == before {
		t.Fatalf("hovering a bar did not change its color (%v)", after)
	}
	if after.B < 200 || after.R > 140 {
		t.Errorf("hovered bar is %v, want indigo", after)
	}
	if got := a.Tree().Stats().Builds; got != builds {
		t.Errorf("hover built %d components", got-builds)
	}
	// Moving to another bar restores the first one.
	d := cols[3].Rect
	a.Move(d.Center().X, d.Y1-10)
	if got := a.Image().RGBAAt(x, y); got != before {
		t.Errorf("bar did not return to %v, is %v", before, got)
	}
}
