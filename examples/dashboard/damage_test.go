package main

import (
	"os"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/damagetest"
	"github.com/kuronosu/tela/telatest"
)

// TestPartialFrames: frames that rasterized the whole window before
// ADR-275 draw only what changed above an unchanged base: the orders
// scrolling over the main pane's glass (from the second frame on, over a
// snapshot of the pane), and the palette's scrim and first frame (over
// the page the framebuffer holds).
func TestPartialFrames(t *testing.T) {
	r := newRig(t, 1920, 1080, 1, false)
	r.settle(600)
	r.tick(nil, r.click("button", "Orders")...)
	r.settle(600)
	tb := r.find("table", "Orders").Bounds
	r.tick(nil, r.move(platform.Point{X: tb.Center().X, Y: tb.Y0 + 200}))
	for i := range 4 {
		if _, d := r.rasterStep(r.wheel(12, platform.ScrollPhaseUpdate)); i > 0 && d.Full {
			t.Errorf("scroll frame %d rasterized the whole window", i)
		}
	}
	r.settle(600)
	if _, d := r.rasterStep(r.click("button", "Search")...); d.Full {
		t.Error("the palette's scrim rasterized the whole window")
	}
	if _, d := r.rasterStep(); d.Full {
		t.Error("the palette's first frame rasterized the whole window")
	}
}

// The damage tests check every frame against a full rendering, and on the
// glass page each one costs a full frame: they play each scale once across
// the package, scale 1 and 1.5 (TestScrollDamage, the interactions at 1.5)
// and 2 (TestDragDamage). TELA_CHECK_DAMAGE=1, which also checks every
// other app of the tests, plays every scale of each. All of them in one
// run took the package past go test's 10-minute timeout on CI's runners.
var checkEveryScale = os.Getenv(telatest.EnvCheckDamage) == "1"

// TestScrollDamage scrolls and hovers the dashboard in a small window at
// scales 1 and 1.5 (and 2 with TELA_CHECK_DAMAGE=1), checking every frame
// against a full rendering. Twelve steps are a PageDown, a PageUp and arrow
// keys both ways, and each step moves the pointer: what 24 steps checked,
// in half the frames.
func TestScrollDamage(t *testing.T) {
	scales := []float64{1, 1.5}
	if checkEveryScale {
		scales = append(scales, 2)
	}
	for _, scale := range scales {
		damagetest.ScrollAround(t, func(opts ...telatest.Option) *telatest.App { return telatest.MountApp(t, App, opts...) }, 900, 560, scale, 12)
	}
}

// TestInteractionDamage plays the interactions of the session (a period
// change, a metric tab, the report dialog, the palette, the tasks, the
// board, the theme) with every frame, animations included, checked against
// a full rendering: the glass panes, springs, layout animations and
// overlays repaint only what changed, and what they repaint is what a full
// frame has.
//
// It plays them at scale 1.5, where the damage of a change is rounded out
// to device pixels and the blur's reach is fractional: the stricter scale.
// A pass checks about 700 frames, so the window is a small laptop's
// (1024x640, 1536x960 device pixels); the pass at scale 1 plays with
// TELA_CHECK_DAMAGE=1.
func TestInteractionDamage(t *testing.T) {
	scales := []float64{1.5}
	if checkEveryScale {
		scales = []float64{1, 1.5}
	}
	for _, scale := range scales {
		a := telatest.MountApp(t, App, telatest.Size(1024, 640), telatest.Scale(scale), telatest.CheckDamage())
		a.Click(telatest.ByRole("radio", "90D"))
		a.Click(telatest.ByRole("tab", "Orders"))
		a.Hover(telatest.ByRole("button", "Orders"))
		a.Scroll(telatest.ByRole("heading", "Subscribers"), 0, 400)
		a.Click(telatest.ByRole("button", "Edit the weekly report"))
		a.Click(telatest.ByRole("radio", "XLSX"))
		a.Click(telatest.ByRole("button", "Save"))
		a.Click(telatest.ByRole("button", "Search"))
		a.Type("ta") // "Go to Tasks" is highlighted first (AutoFocusFirst)
		a.Press("Enter")
		a.Click(telatest.ByRole("checkbox", "Fix failed invoice retries"))
		a.Click(telatest.ByRole("button", "Due"))
		a.Click(telatest.ByRole("radio", "Board"))
		a.Click(telatest.ByRole("button", "Actions for Ship the CSV export"))
		a.Click(telatest.ByRole("menuitem", "In progress"))
		a.Click(telatest.ByRole("button", "Switch theme"))
		a.Click(telatest.ByRole("button", "Notifications, 3 unread"))
		a.Press("Escape")
		a.Click(telatest.ByRole("button", "Switch theme"))
	}
}
