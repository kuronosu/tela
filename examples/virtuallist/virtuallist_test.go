package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1280, 800

// mount opens the screen in dev mode, where any dev message fails the test.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h), telatest.Dev()}, opts...)
	return telatest.Mount(t, tela.C(Directory), opts...)
}

var (
	list  = telatest.ByRole("list", "People")
	rows  = telatest.ByRole("listitem", "")
	quiet = 2 * time.Second // the overlay scrollbar fades
)

// row locates the row of index i (0-based) by its number.
func row(i int) telatest.Locator {
	return telatest.ByText("#"+Grouped(i+1), telatest.Exact())
}

// rowOf locates the listitem of index i: its number followed by its role
// (the needle of HasText is trimmed, so "#1 " alone would match "#12").
func rowOf(i int) telatest.Locator {
	return rows.Filter(telatest.HasText("#" + Grouped(i+1) + " " + PersonAt(i).Role))
}

// viewport returns the top and bottom of the list's scrollport (inside
// its 1 px border).
func viewport(a *telatest.App) (top, bottom float32) {
	yBox := a.Bounds(list)
	y, h := yBox.Y, yBox.Height
	return y + 1, y + h - 1
}

// near compares positions within a device pixel: the painted scroll
// offset is snapped to device pixels, and a centered row may sit at half a
// pixel.
func near(a, b float32) bool { return a-b <= 1 && b-a <= 1 }

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {500, 420}, {1600, 900}} {
		mount(t, s[0], s[1])
	}
}

func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale)).Golden(fmt.Sprintf("virtuallist_x%g.png", scale))
		})
	}
}

// TestGoldenAfterJump is the list centered on row 50,000, dark: a scroll
// offset of about 2.6 million logical pixels, also at a fractional scale.
func TestGoldenAfterJump(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			a := mount(t, windowW, windowH, telatest.Dark(), telatest.Scale(scale))
			a.Click(telatest.ByRole("button", "Row 50,000"))
			a.Advance(quiet)
			a.Golden(fmt.Sprintf("virtuallist_jump_dark_x%g.png", scale))
		})
	}
}

func TestOnlyTheRowsInViewAreBuilt(t *testing.T) {
	a := mount(t, windowW, windowH)
	// About ten rows fit in the list; five more are built below them.
	n := a.Count(rows)
	if n < 10 || n > 20 {
		t.Errorf("%d rows built at the top, want 10 to 20", n)
	}
	a.Click(telatest.ByRole("button", "Row 50,000"))
	if n := a.Count(rows); n < 10 || n > 25 {
		t.Errorf("%d rows built around row 50,000", n)
	}
	a.ExpectNoText("Ana Abbott")
}

func TestJumpButtons(t *testing.T) {
	a := mount(t, windowW, windowH)
	top, bottom := viewport(a)

	a.Click(telatest.ByRole("button", "Row 50,000"))
	yBox := a.Bounds(rowOf(49_999))
	y, h := yBox.Y, yBox.Height
	if !near(y+h/2, (top+bottom)/2) {
		t.Errorf("row 50,000 spans %v..%v, want it centered in %v..%v", y, y+h, top, bottom)
	}

	a.Click(telatest.ByRole("button", "Row 1,000"))
	if y := a.Bounds(rowOf(999)).Y; !near(y, top) {
		t.Errorf("row 1,000 starts at %v, want the top of the list (%v)", y, top)
	}

	a.Click(telatest.ByRole("button", "Last"))
	a.ExpectText("#100,000")
	if r := a.Bounds(rowOf(Count - 1)); !near(r.Bottom(), bottom) {
		t.Errorf("the last row ends at %v, want the bottom of the list (%v)", r.Bottom(), bottom)
	}

	a.Click(telatest.ByRole("button", "First"))
	if y := a.Bounds(rowOf(0)).Y; !near(y, top) {
		t.Errorf("the first row starts at %v, want %v", y, top)
	}
}

func TestWheelAndKeys(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Scroll(list, 0, 5000)
	if a.Count(row(0)) != 0 {
		t.Error("row 1 is still built after scrolling 5,000 px")
	}
	y0 := a.Bounds(rowOf(80)).Y
	a.Press("PageDown") // nothing is focused: the list is the page
	y1 := a.Bounds(rowOf(80)).Y
	lh := a.Bounds(list).Height
	if !near(y0-y1, (lh-2)*0.875) {
		t.Errorf("Page Down moved row 81 by %v, want 87.5%% of the viewport (%v)", y0-y1, (lh-2)*0.875)
	}
	a.Press("End")
	a.ExpectText("#100,000")
	_, bottom := viewport(a)
	if r := a.Bounds(rowOf(Count - 1)); !near(r.Bottom(), bottom) {
		t.Errorf("End: the last row ends at %v, want %v", r.Bottom(), bottom)
	}
	a.Press("Home")
	a.ExpectText("Ana Abbott")
}

func TestRowsAreMeasured(t *testing.T) {
	// At 500 px the notes wrap to several lines: the rows are taller than
	// their estimates and still follow one another.
	a := mount(t, 500, 700)
	a.Scroll(list, 0, 1200)
	first := -1
	for i := 0; i < 60; i++ {
		if a.Count(rowOf(i)) == 1 {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("no row built")
	}
	taller := false
	for i := first; a.Count(rowOf(i+1)) == 1; i++ {
		yBox := a.Bounds(rowOf(i))
		y, h := yBox.Y, yBox.Height
		y2 := a.Bounds(rowOf(i + 1)).Y
		if !near(y+h, y2) {
			t.Fatalf("row %d ends at %v, row %d starts at %v", i+1, y+h, i+2, y2)
		}
		if HasNote(i) && h > noteHeight+10 {
			taller = true
		}
	}
	if !taller {
		t.Error("no note row is taller than its estimate at 500 px")
	}
}

func TestFocusAndTab(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Focus(telatest.ByRole("button", "Last"))
	a.Press("Enter")
	a.ExpectText("#100,000")
	a.Press("Shift+Tab")
	a.Press("Shift+Tab")
	a.Press("Shift+Tab")
	a.ExpectFocused(telatest.ByRole("button", "First"))
}

func TestAriaSnapshot(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.AriaGolden("virtuallist.aria.yml")
}

func TestGrouped(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 49_999: "49,999", 100_000: "100,000", 1_234_567: "1,234,567", -4200: "-4,200"} {
		if got := Grouped(n); got != want {
			t.Errorf("Grouped(%d) = %q, want %q", n, got, want)
		}
	}
}
