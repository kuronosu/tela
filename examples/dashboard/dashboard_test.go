package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1440, 900

// mount opens the page in dev mode, where any dev message (an unknown
// class, a hook misuse) fails the test.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h), telatest.Dev()}, opts...)
	return telatest.Mount(t, tela.C(Dashboard), opts...)
}

// TestNoDevMessages opens the page at several window sizes, from a narrow
// window to 4K, in both schemes.
func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {1920, 1080}, {1024, 700}, {700, 600}, {3840, 2160}} {
		mount(t, s[0], s[1])
		mount(t, s[0], s[1], telatest.Dark())
	}
}

// TestGolden is the page at 1440x900 in light and dark, at scale 1 and 2.
func TestGolden(t *testing.T) {
	for _, scheme := range []string{"light", "dark"} {
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("%s_x%g", scheme, scale), func(t *testing.T) {
				opts := []telatest.Option{telatest.Scale(scale)}
				if scheme == "dark" {
					opts = append(opts, telatest.Dark())
				}
				mount(t, windowW, windowH, opts...).Golden(fmt.Sprintf("dashboard_%s_x%g.png", scheme, scale))
			})
		}
	}
}

// TestGoldenScrolled shows the glass header over a page that has scrolled
// under it.
func TestGoldenScrolled(t *testing.T) {
	for _, scheme := range []string{"light", "dark"} {
		t.Run(scheme, func(t *testing.T) {
			opts := []telatest.Option{telatest.InstantScroll()}
			if scheme == "dark" {
				opts = append(opts, telatest.Dark())
			}
			a := mount(t, windowW, windowH, opts...)
			a.Scroll(telatest.ByText("Performance", telatest.Exact()), 0, 340)
			a.Golden(fmt.Sprintf("scrolled_%s.png", scheme))
		})
	}
}

// TestGoldenDialog is the new-report dialog, open, in both schemes.
func TestGoldenDialog(t *testing.T) {
	for _, scheme := range []string{"light", "dark"} {
		t.Run(scheme, func(t *testing.T) {
			var opts []telatest.Option
			if scheme == "dark" {
				opts = append(opts, telatest.Dark())
			}
			a := mount(t, windowW, windowH, opts...)
			a.Click(telatest.ByRole("button", "New report"))
			a.ExpectCount(telatest.ByRole("dialog", "New report"), 1)
			a.Golden(fmt.Sprintf("dialog_%s.png", scheme))
		})
	}
}

// TestRange picks a period: the numbers count to the new period's values,
// and a period change builds the components that read it, once.
func TestRange(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectText("$45,231.89")
	before := a.Stats()
	a.Click(telatest.ByRole("tab", "90D"))
	a.ExpectText("$128,904.12")
	a.ExpectText("6,718")
	a.ExpectCount(telatest.ByRole("tab", "90D").Filter(telatest.HasClass("text-slate-900")), 1)
	// The springs (numbers, 12 bars, 48 sparkline bars) move through layout
	// and paint closures: the frames of the animation build nothing.
	if got := a.Stats().Builds - before.Builds; got > 90 {
		t.Errorf("changing the period built %d components, want the cards, the chart and the bars once each", got)
	}
}

// TestSort reorders the task list: the rows are laid out at their new
// places at once and drawn sliding there (AnimateLayout).
func TestSort(t *testing.T) {
	// Bounds, Count and the Expect functions wait for the animations to end,
	// so the frame in the middle is looked at as pixels.
	a := mount(t, windowW, windowH, telatest.NoSettle(), telatest.InstantScroll())
	rows := telatest.ByText("Priorities", telatest.Exact())
	a.Scroll(rows, 0, 500) // bring the list into view
	row := func(title string) float32 {
		_, y, _, _ := a.Bounds(telatest.ByText(title, telatest.Exact()))
		return y
	}
	first, last := "Fix the failed invoice retries", "Publish the API changelog"
	if row(first) >= row(last) {
		t.Fatalf("by priority %q should come before %q", first, last)
	}
	before := string(a.Screenshot().Pix)
	a.Click(telatest.ByRole("button", "A-Z"))
	a.Advance(40 * time.Millisecond)
	mid := string(a.Screenshot().Pix)
	a.Advance(2 * time.Second)
	end := string(a.Screenshot().Pix)
	if mid == before || mid == end {
		t.Errorf("the rows did not slide: the frame 40 ms in equals the first one (%v) or the last one (%v)", mid == before, mid == end)
	}
	if row(first) >= row("Interview three churned customers") || row("Interview three churned customers") >= row(last) {
		t.Errorf("A-Z should order %q, then Interview..., then %q", first, last)
	}
}

// TestFinishTask checks the first task off: it sinks to the bottom.
func TestFinishTask(t *testing.T) {
	a := mount(t, windowW, windowH)
	title := telatest.ByText("Fix the failed invoice retries", telatest.Exact())
	_, y0, _, _ := a.Bounds(title)
	a.Click(telatest.ByRole("checkbox", "Fix the failed invoice retries"))
	if _, y1, _, _ := a.Bounds(title); y1 <= y0 {
		t.Errorf("the finished task is at %v, it was at %v: it should have sunk", y1, y0)
	}
	a.ExpectCount(telatest.ByRole("checkbox", "Fix the failed invoice retries").Filter(telatest.HasClass("from-indigo-500")), 1)
}

// TestTabs moves the chart's tab: one underline, under the selected tab,
// and the y axis follows the metric.
func TestTabs(t *testing.T) {
	a := mount(t, windowW, windowH)
	underline := telatest.ByClass("h-0.5", "bg-linear-to-r")
	a.ExpectCount(underline, 1)
	a.ExpectText("$9k")
	a.Click(telatest.ByRole("tab", "Orders"))
	a.ExpectCount(underline, 1)
	ux, _, uw, _ := a.Bounds(underline)
	tx, _, tw, _ := a.Bounds(telatest.ByRole("tab", "Orders"))
	if ux < tx || ux+uw > tx+tw {
		t.Errorf("the underline spans %v to %v, outside the Orders tab (%v to %v)", ux, ux+uw, tx, tx+tw)
	}
	a.ExpectNoText("$9k")
	a.ExpectText("2.4k")
}

// TestDialog opens the dialog, closes it with Escape and with the scrim,
// and checks that its form works.
func TestDialog(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(telatest.ByRole("button", "New report"))
	a.ExpectCount(telatest.ByRole("dialog", "New report"), 1)
	a.ExpectCount(telatest.ByRole("radio", "PDF").Filter(telatest.HasClass("text-slate-900")), 1)
	a.Click(telatest.ByRole("radio", "CSV"))
	a.ExpectCount(telatest.ByRole("radio", "CSV").Filter(telatest.HasClass("text-slate-900")), 1)
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("dialog", "New report"), 0)

	a.Click(telatest.ByRole("button", "New report"))
	a.MouseMove(30, 450) // over the scrim, in the sidebar
	a.MouseDown()
	a.MouseUp()
	a.ExpectCount(telatest.ByRole("dialog", "New report"), 0)

	// Create report: the dialog closes (and the toast comes up, see TestToast).
	a.Click(telatest.ByRole("button", "New report"))
	a.Click(telatest.ByRole("button", "Create report"))
	a.ExpectCount(telatest.ByRole("dialog", "New report"), 0)
}

// toastRegion is the pixels the toast covers, as a string to compare. The
// queries of telatest (Count, AriaSnapshot, Bounds) let the app settle,
// which would run the toast's countdown to its end, so a toast in the
// middle of its life is looked at as pixels.
func toastRegion(a *telatest.App) string {
	img := a.Screenshot()
	var sb strings.Builder
	for y := 800; y < 880; y++ {
		sb.Write(img.Pix[img.PixOffset(520, y):img.PixOffset(920, y)])
	}
	return sb.String()
}

// waitToast advances the clock in 50 ms steps until the toast's pixels
// equal (gone) or differ from the page's (shown), at most 8 s, and returns
// when that happened.
func waitToast(t *testing.T, a *telatest.App, page string, shown bool) time.Duration {
	t.Helper()
	for range 160 {
		if (toastRegion(a) != page) == shown {
			return a.Now()
		}
		a.Advance(50 * time.Millisecond)
	}
	t.Fatalf("the toast was never %v", map[bool]string{true: "shown", false: "gone"}[shown])
	return 0
}

// TestToast shows the toast, and its countdown (a tween on the frame clock)
// dismisses it by itself after 4 s, or the button does it by hand.
func TestToast(t *testing.T) {
	a := mount(t, windowW, windowH, telatest.NoSettle())
	a.Advance(3 * time.Second) // the numbers and bars have counted up
	page := toastRegion(a)
	a.Click(telatest.ByRole("button", "Export"))
	start := a.Now() // the click's own frames have run
	shown := waitToast(t, a, page, true)
	gone := waitToast(t, a, page, false)
	if life := gone - shown; life < 3500*time.Millisecond || life > 4500*time.Millisecond {
		t.Errorf("the toast lived %v from %v (the click was at about %v): its countdown is 4 s", life, shown, start)
	}

	// A toast from the dialog's Create button, dismissed by hand.
	a.Click(telatest.ByRole("button", "New report"))
	a.Advance(300 * time.Millisecond)
	a.Click(telatest.ByRole("button", "Create report"))
	waitToast(t, a, page, true)
	a.Advance(400 * time.Millisecond) // it has slid into place
	// The Dismiss button, clicked by position: locating it would let the
	// app settle and the countdown end.
	a.MouseMove(872, 840)
	a.MouseDown()
	a.MouseUp()
	a.MouseMove(720, 20) // off what was under the toast
	a.Advance(400 * time.Millisecond)
	if toastRegion(a) != page {
		t.Errorf("the toast is still there after Dismiss")
	}
}

// TestThemeSwitch toggles the scheme: dark: classes apply and the backdrop
// picks the other image.
func TestThemeSwitch(t *testing.T) {
	a := mount(t, windowW, windowH)
	light := a.Screenshot().RGBAAt(700, 600)
	a.Click(telatest.ByRole("switch", "Dark mode"))
	a.ExpectCount(telatest.ByRole("switch", "Dark mode").Filter(telatest.HasClass("w-[84px]")), 1)
	dark := a.Screenshot().RGBAAt(700, 600)
	if dark.R > light.R/2 || dark.G > light.G/2 {
		t.Errorf("the page did not go dark: %v before, %v after", light, dark)
	}
	a.Click(telatest.ByRole("switch", "Dark mode"))
	if got := a.Screenshot().RGBAAt(700, 600); got != light {
		t.Errorf("switching back gives %v, want %v", got, light)
	}
}

// TestIdle checks that the page at rest asks for no frames, after every
// interaction has ended: no infinite animation is left running.
func TestIdle(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(telatest.ByRole("tab", "7D")) // the sync spinner runs for a moment
	a.Click(telatest.ByRole("button", "Export"))
	a.Advance(6 * time.Second) // the toast counts down and leaves
	frames := a.Stats().Frames
	a.Advance(5 * time.Second)
	if got := a.Stats().Frames; got != frames {
		t.Errorf("the page at rest ran %d frames in 5 s", got-frames)
	}
}

// TestSticky scrolls the page: the header and the sidebar stay where they
// are, and the fixed help button too.
func TestSticky(t *testing.T) {
	a := mount(t, windowW, windowH, telatest.InstantScroll())
	at := func(l telatest.Locator) [2]float32 { x, y, _, _ := a.Bounds(l); return [2]float32{x, y} }
	header, nav, help := telatest.ByRole("tablist", "Period"), telatest.ByRole("button", "Analytics"), telatest.ByRole("button", "Help")
	h0, n0, f0 := at(header), at(nav), at(help)
	a.Scroll(telatest.ByText("Performance", telatest.Exact()), 0, 300)
	if got := at(header); got != h0 {
		t.Errorf("the header moved from %v to %v", h0, got)
	}
	if got := at(nav); got != n0 {
		t.Errorf("the sidebar moved from %v to %v", n0, got)
	}
	if got := at(help); got != f0 {
		t.Errorf("the help button moved from %v to %v", f0, got)
	}
	if _, y, _, _ := a.Bounds(telatest.ByText("Performance", telatest.Exact())); y > 300 {
		t.Errorf("the page did not scroll: Performance is at y=%v", y)
	}
}
