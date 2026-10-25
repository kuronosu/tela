package main

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1280, 800

// mount opens the screen in dev mode, where any dev message fails the test.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.Mount(t, Settings(), opts...)
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		mount(t, s[0], s[1])
	}
}

func TestGolden(t *testing.T) {
	// 1.25 and 1.5: fractional scales (Wayland wp_fractional_scale_v1,
	// Windows 125 % and 150 %).
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale)).Golden(fmt.Sprintf("settings_x%g.png", scale))
		})
	}
}

// switchLoc returns the locator of a switch by its name.
func switchLoc(name string) telatest.Locator { return telatest.ByRole("switch", name) }

// trackPixel samples the left end of a switch track, away from the knob in
// both positions of the knob (it is 18 px wide in a 40 px track).
func trackPixel(a *telatest.App, sw telatest.Locator) color.RGBA {
	r := a.Bounds(sw)
	return a.Screenshot().RGBAAt(int(r.X+r.Width/2), int(r.Y)+2)
}

func TestSwitchTogglesAndDirtiesTheForm(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectText("All changes saved")
	a.ExpectCount(telatest.ByRole("switch", ""), 5)
	digest := switchLoc("Weekly digest")
	off := trackPixel(a, digest)
	// The knob is the only box inside a switch, and the third of the five.
	knob := telatest.ByClass("size-[18px]", "rounded-full", "shadow-sm").Nth(2)
	k0 := a.Bounds(knob).X

	a.Expect(digest, telatest.Checked(false))
	a.Click(digest)
	a.MouseMove(5, 5)
	a.Expect(digest, telatest.Checked(true))
	a.ExpectText("You have unsaved changes")
	if on := trackPixel(a, digest); on == off || on.R > 60 {
		t.Errorf("the track is %v after switching on, was %v: want a dark track", on, off)
	}
	if k := a.Bounds(knob).X; k < k0+10 {
		t.Errorf("knob did not move to the right: %v -> %v", k0, k)
	}

	// Toggling back makes the form clean again: prefs == saved.
	a.Click(digest)
	a.ExpectText("All changes saved")
}

// TestSwitchStates checks hover: and active: on a switch, and that the
// pointer cursor shows over it.
func TestSwitchStates(t *testing.T) {
	a := mount(t, windowW, windowH)
	sw := switchLoc("Product updates")
	rest := trackPixel(a, sw)

	a.Hover(sw)
	hover := trackPixel(a, sw)
	if hover == rest {
		t.Error("hover: did not change the track")
	}
	if got := a.Cursor(); got != "pointer" {
		t.Errorf("cursor over a switch is %v, want pointer", got)
	}
	a.MouseDown()
	active := trackPixel(a, sw)
	if active == hover || active == rest {
		t.Errorf("active: did not change the track (rest %v hover %v active %v)", rest, hover, active)
	}
	a.MouseUp() // click: switches on
	a.MouseMove(5, 5)
	a.ExpectText("You have unsaved changes")
}

// TestLockedSwitch checks the disabled switch ignores clicks, and shows the
// not-allowed cursor of its disabled: classes.
func TestLockedSwitch(t *testing.T) {
	a := mount(t, windowW, windowH)
	sw := switchLoc("Security alerts")
	a.Hover(sw)
	if got := a.Cursor(); got != "not-allowed" {
		t.Errorf("cursor over the locked switch is %v, want not-allowed (disabled:cursor-not-allowed)", got)
	}
	// Click ignores disabled elements by failing; a user presses anyway.
	a.MouseDown()
	a.MouseUp()
	a.ExpectText("All changes saved")
}

// TestSaveAndReset drives the footer: Save and Reset are disabled until the
// form changes, Save stores the changes and Reset throws them away.
func TestSaveAndReset(t *testing.T) {
	a := mount(t, windowW, windowH)
	save := telatest.ByRole("button", "Save changes")

	// Disabled: a click does nothing and the not-allowed cursor shows.
	a.Hover(save)
	a.MouseDown()
	a.MouseUp()
	a.ExpectText("All changes saved")
	if got := a.Cursor(); got != "not-allowed" {
		t.Errorf("cursor over a disabled Save is %v", got)
	}

	// Change something, then Save.
	a.Click(telatest.ByRole("radio", "Dark"))
	a.ExpectText("You have unsaved changes")
	a.Click(save)
	a.MouseMove(5, 5)
	a.ExpectText("All changes saved")

	// Change again, then Reset: back to the saved theme (Dark).
	a.Click(telatest.ByRole("radio", "").Filter(telatest.HasText("Spacious")))
	a.Click(telatest.ByRole("button", "Reset"))
	a.MouseMove(5, 5)
	a.ExpectText("All changes saved")
	// The theme and the density are selected (ring-1).
	a.ExpectCount(telatest.ByClass("ring-1", "border"), 2)
}

// TestRadioGroups checks that choosing an option selects it and only it in
// its group, and leaves the other group alone.
func TestRadioGroups(t *testing.T) {
	a := mount(t, windowW, windowH)
	selected := func(label string) bool {
		return a.Count(telatest.ByRole("radio", "").Filter(telatest.HasText(label), telatest.HasClass("ring-1"))) == 1
	}
	if !selected("System") || !selected("Comfortable") {
		t.Fatal("the defaults are not selected")
	}
	a.Click(telatest.ByRole("radio", "Dark"))
	if !selected("Dark") || selected("System") || selected("Light") {
		t.Error("theme group: Dark should be the only selected theme")
	}
	if !selected("Comfortable") {
		t.Error("choosing a theme changed the density")
	}
	a.Click(telatest.ByRole("radio", "").Filter(telatest.HasText("Compact")))
	if !selected("Compact") || selected("Comfortable") || selected("Spacious") {
		t.Error("density group: Compact should be the only selected density")
	}
	if !selected("Dark") {
		t.Error("choosing a density changed the theme")
	}
}

// TestRebuildsAreNarrow checks that toggling a switch does not rebuild the
// whole page: the unchanged rows and the appearance card skip their build.
func TestRebuildsAreNarrow(t *testing.T) {
	a := mount(t, windowW, windowH)
	before := a.Stats()
	a.Click(switchLoc("Weekly digest"))
	after := a.Stats()
	if builds := after.Builds - before.Builds; builds > 12 {
		t.Errorf("one switch rebuilt %d components, want about 9 (page, three cards, one row, one switch...)", builds)
	}
	if skips := after.Skips - before.Skips; skips < 4 {
		t.Errorf("only %d builds skipped: the unchanged rows should skip", skips)
	}
}
