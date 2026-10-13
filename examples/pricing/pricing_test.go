package main

import (
	"bytes"
	"testing"

	"github.com/kuronosu/tela/examples/internal/exampletest"
)

const windowW, windowH = 1280, 800

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		a := exampletest.Open(t, Pricing, s[0], s[1])
		a.CheckLogs()
	}
}

// TestGolden is the monthly page at 1280x800, at scale 1 and 2.
func TestGolden(t *testing.T) {
	exampletest.Golden(t, "pricing", Pricing, windowW, windowH)
}

// TestThreePlansInARow checks the cards share a row at 1280 and that the
// highlighted one carries the badge.
func TestThreePlansInARow(t *testing.T) {
	a := exampletest.Open(t, Pricing, windowW, windowH)
	y := a.Text("Starter").Rect.Y0
	for _, name := range []string{"Pro", "Enterprise"} {
		if got := a.Text(name).Rect.Y0; got != y {
			t.Errorf("%s at y=%v, Starter at y=%v", name, got, y)
		}
	}
	badge := a.Text("Most popular").Rect
	pro := a.Text("Pro").Rect
	if badge.Y1 > pro.Y0 || badge.Center().X < pro.X0 {
		t.Errorf("badge %v is not above the Pro card heading %v", badge, pro)
	}
}

// TestBillingToggle clicks the switch: prices drop to their yearly
// values, the knob moves to the right, and clicking again restores the
// monthly page bit by bit.
func TestBillingToggle(t *testing.T) {
	a := exampletest.Open(t, Pricing, windowW, windowH)
	knob := func() float32 {
		ks := a.WithClass("size-5", "rounded-full", "shadow-sm")
		if len(ks) != 1 {
			t.Fatalf("%d knobs", len(ks))
		}
		return ks[0].Rect.X0
	}
	for _, s := range []string{"$0", "$29", "$99", "Billed monthly", "Free forever"} {
		if !a.HasText(s) {
			t.Errorf("monthly page lacks %q", s)
		}
	}
	monthly, knob0 := a.Image(), knob()

	track := a.WithClass("h-6", "w-11", "rounded-full")[0].Rect.Center()
	a.Click(track.X, track.Y)
	for _, s := range []string{"$0", "$24", "$79", "Billed $288 per year", "Billed $948 per year", "Free forever"} {
		if !a.HasText(s) {
			t.Errorf("yearly page lacks %q", s)
		}
	}
	for _, s := range []string{"$29", "$99"} {
		if a.HasText(s) {
			t.Errorf("yearly page still shows %q", s)
		}
	}
	if k := knob(); k <= knob0+10 {
		t.Errorf("knob did not move right: x %v -> %v", knob0, k)
	}

	// The pointer is still over the switch: move away to compare pictures.
	a.Move(5, 5)
	a.Click(track.X, track.Y)
	a.Move(5, 5)
	if !a.HasText("$29") || a.HasText("$24") {
		t.Error("second click did not restore the monthly prices")
	}
	if got := a.Image(); !bytes.Equal(got.Pix, monthly.Pix) {
		t.Error("the monthly page is not the same picture after toggling twice")
	}
	a.CheckLogs()
}

// TestBillingLabels checks the two labels are buttons too.
func TestBillingLabels(t *testing.T) {
	a := exampletest.Open(t, Pricing, windowW, windowH)
	a.ClickButton("Monthly")
	if !a.HasText("$29") {
		t.Error("Monthly changed the prices")
	}
	a.ClickText("Yearly")
	if !a.HasText("$24") {
		t.Error("Yearly did not switch to yearly prices")
	}
	a.ClickButton("Monthly")
	if !a.HasText("$29") {
		t.Error("Monthly did not switch back")
	}
}

// TestHoverBuildsNothing checks that pointer movement over the cards and
// buttons restyles without building components.
func TestHoverBuildsNothing(t *testing.T) {
	a := exampletest.Open(t, Pricing, windowW, windowH)
	builds := a.Tree().Stats().Builds
	for _, label := range []string{"Get started", "Start free trial", "Contact sales"} {
		c := a.Button(label).Rect.Center()
		a.Move(c.X, c.Y)
	}
	if got := a.Tree().Stats().Builds - builds; got != 0 {
		t.Errorf("hovering built %d components", got)
	}
}
