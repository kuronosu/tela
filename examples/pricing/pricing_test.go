package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1280, 800

// mount opens the screen as main does, in dev mode, where any dev message fails the test.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.MountApp(t, App, opts...)
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		mount(t, s[0], s[1])
	}
}

// TestGolden is the monthly page at 1280x800, at scale 1 and 2, and in dark
// mode.
func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale)).Golden(fmt.Sprintf("pricing_x%g.png", scale))
			mount(t, windowW, windowH, telatest.Scale(scale), telatest.Dark()).Golden(fmt.Sprintf("pricing_dark_x%g.png", scale))
		})
	}
}

// exact locates the element whose text is exactly s.
func exact(s string) telatest.Locator { return telatest.ByText(s, telatest.Exact()) }

// hasText reports whether the page has an element whose text is exactly s.
func hasText(a *telatest.App, s string) bool { return a.Count(exact(s)) > 0 }

// TestThreePlansInARow checks the cards share a row at 1280 and that the
// highlighted one carries the badge.
func TestThreePlansInARow(t *testing.T) {
	a := mount(t, windowW, windowH)
	y := a.Bounds(exact("Starter")).Y
	for _, name := range []string{"Pro", "Enterprise"} {
		if got := a.Bounds(exact(name)).Y; got != y {
			t.Errorf("%s at y=%v, Starter at y=%v", name, got, y)
		}
	}
	badge, pro := a.Bounds(exact("Most popular")), a.Bounds(exact("Pro"))
	if badge.Y+badge.Height > pro.Y || badge.X+badge.Width/2 < pro.X {
		t.Errorf("badge at (%v, %v) is not above the Pro card heading at (%v, %v)", badge.X, badge.Y, pro.X, pro.Y)
	}
}

// The two options of the billing segmented control; the yearly one holds a
// badge, which is part of its name.
var (
	monthlyOpt = telatest.ByRole("radio", "Monthly")
	yearlyOpt  = telatest.ByRole("radio", "Yearly Save 20%")
)

// TestBillingToggle presses the yearly option: prices drop to their yearly
// values and the option is on, and pressing Monthly restores the monthly
// page bit by bit.
func TestBillingToggle(t *testing.T) {
	a := mount(t, windowW, windowH)
	for _, s := range []string{"$0", "$29", "$99", "Billed monthly", "Free forever"} {
		if !hasText(a, s) {
			t.Errorf("monthly page lacks %q", s)
		}
	}
	a.Expect(monthlyOpt, telatest.Checked(true))
	a.Expect(yearlyOpt, telatest.Checked(false))
	a.MouseMove(5, 5)
	monthly := a.Screenshot()

	a.Click(yearlyOpt)
	a.Expect(yearlyOpt, telatest.Checked(true))
	a.Expect(monthlyOpt, telatest.Checked(false))
	for _, s := range []string{"$0", "$24", "$79", "Billed $288 per year", "Billed $948 per year", "Free forever"} {
		if !hasText(a, s) {
			t.Errorf("yearly page lacks %q", s)
		}
	}
	for _, s := range []string{"$29", "$99"} {
		if hasText(a, s) {
			t.Errorf("yearly page still shows %q", s)
		}
	}

	// The pointer is still over the control: move away to compare pictures.
	a.Click(monthlyOpt)
	a.MouseMove(5, 5)
	if !hasText(a, "$29") || hasText(a, "$24") {
		t.Error("pressing Monthly did not restore the monthly prices")
	}
	if got := a.Screenshot(); !bytes.Equal(got.Pix, monthly.Pix) {
		t.Error("the monthly page is not the same picture after toggling twice")
	}
}

// TestBillingKeyboard moves the choice with the arrow keys, as a segmented
// control does, and a segmented control keeps one option on.
func TestBillingKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Focus(monthlyOpt)
	a.Press("ArrowRight")
	a.Press("Enter")
	a.ExpectText("$24")
	a.Expect(yearlyOpt, telatest.Checked(true))
	a.Click(yearlyOpt)
	a.Expect(yearlyOpt, telatest.Checked(true)) // not emptied
	a.ExpectText("$24")
}

// TestAriaSnapshot pins what `tela inspect --aria ./examples/pricing` prints
// (testdata/pricing.aria.yml): the segmented control as a radio group, the
// plans' headings and their buttons. The check icons are decorative.
func TestAriaSnapshot(t *testing.T) {
	mount(t, windowW, windowH).AriaGolden("pricing.aria.yml")
}

// TestHoverBuildsNothing checks that pointer movement over the cards and
// buttons restyles without building components.
func TestHoverBuildsNothing(t *testing.T) {
	a := mount(t, windowW, windowH)
	builds := a.Stats().Builds
	for _, label := range []string{"Get started", "Start free trial", "Contact sales"} {
		a.Hover(telatest.ByRole("button", label))
	}
	if got := a.Stats().Builds - builds; got != 0 {
		t.Errorf("hovering built %d components", got)
	}
}
