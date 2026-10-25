package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1280, 800

// mount opens the screen as main does, in dev mode, where any dev message
// fails the test.
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

func TestGolden(t *testing.T) {
	// 1.25 and 1.5: fractional scales (Wayland wp_fractional_scale_v1,
	// Windows 125 % and 150 %).
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale)).Golden(fmt.Sprintf("settings_x%g.png", scale))
		})
	}
}

// TestGoldenStates are the page in dark mode, edited (the footer says so),
// with the Reset confirmation open and with the toast of Save, at scales 1
// and 2.
func TestGoldenStates(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale), telatest.Dark()).Golden(fmt.Sprintf("settings_dark_x%g.png", scale))

			a := mount(t, windowW, windowH, telatest.Scale(scale))
			a.Click(switchLoc("Weekly digest"))
			a.Click(telatest.ByRole("radio", "Dark"))
			a.Click(telatest.ByRole("button", "Reset"))
			a.ExpectText("Discard your changes?")
			a.Golden(fmt.Sprintf("settings_reset_x%g.png", scale))

			// Cancel, then Save: the toast is at the bottom right.
			a.Click(telatest.ByRole("button", "Cancel"))
			a.Click(telatest.ByRole("button", "Save changes"))
			a.MouseMove(5, 5)
			a.ExpectText("Settings saved")
			a.Golden(fmt.Sprintf("settings_saved_x%g.png", scale))
		})
	}
}

// switchLoc returns the locator of a switch by its name.
func switchLoc(name string) telatest.Locator { return telatest.ByRole("switch", name) }

func TestSwitchTogglesAndDirtiesTheForm(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectText("All changes saved")
	a.ExpectCount(telatest.ByRole("switch", ""), 5)
	digest := switchLoc("Weekly digest")
	// The knob is the only box inside a switch, and the third of the five.
	knob := telatest.ByClass("pointer-events-none", "block", "rounded-full").Nth(2)
	k0 := a.Bounds(knob).X
	off := a.Screenshot()

	a.Expect(digest, telatest.Checked(false))
	a.Click(digest)
	a.MouseMove(5, 5)
	a.Expect(digest, telatest.Checked(true))
	a.ExpectText("You have unsaved changes")
	if k := a.Bounds(knob).X; k < k0+8 {
		t.Errorf("knob did not move to the right: %v -> %v", k0, k)
	}
	if string(a.Screenshot().Pix) == string(off.Pix) {
		t.Error("the picture did not change after switching on")
	}

	// Toggling back makes the form clean again: prefs == saved.
	a.Click(digest)
	a.ExpectText("All changes saved")
}

// TestSwitchKeyboard flips a switch with the keyboard, as its label does
// with a click.
func TestSwitchKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	sw := switchLoc("Product updates")
	a.Focus(sw)
	a.Press("Space")
	a.Expect(sw, telatest.Checked(true))
	a.ExpectText("You have unsaved changes")
	// The row's label names the switch and a click on it toggles it.
	a.Click(telatest.ByText("Product updates", telatest.Exact()))
	a.Expect(sw, telatest.Checked(false))
	a.ExpectText("All changes saved")
}

// TestLockedSwitch checks the disabled switch ignores clicks, and shows the
// not-allowed cursor of the recipe's disabled: classes.
func TestLockedSwitch(t *testing.T) {
	a := mount(t, windowW, windowH)
	sw := switchLoc("Security alerts")
	a.Expect(sw, telatest.Checked(true), telatest.Disabled(true))
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
// form changes, Save stores the changes and shows a toast, and Reset asks
// before it throws them away.
func TestSaveAndReset(t *testing.T) {
	a := mount(t, windowW, windowH)
	save := telatest.ByRole("button", "Save changes")
	reset := telatest.ByRole("button", "Reset")

	// Disabled (pointer-events-none): pressing where they are does nothing.
	a.Expect(save, telatest.Disabled(true))
	a.Expect(reset, telatest.Disabled(true))
	for _, b := range []telatest.Locator{save, reset} {
		r := a.Bounds(b)
		a.ClickAt(r.X+r.Width/2, r.Y+r.Height/2)
	}
	a.ExpectText("All changes saved")
	a.ExpectNoText("Discard your changes?")

	// Change something, then Save: the toast says so.
	a.ExpectNoText("Settings saved")
	a.Click(telatest.ByRole("radio", "Dark"))
	a.ExpectText("You have unsaved changes")
	a.Click(save)
	a.MouseMove(5, 5)
	a.ExpectText("All changes saved")
	a.ExpectText("Settings saved")
	a.ExpectText("Your preferences were updated.")

	// Change again, then Reset: the dialog asks first, Cancel keeps the
	// change and the action throws it away.
	a.Click(telatest.ByRole("radio", "Spacious"))
	a.Click(reset)
	a.ExpectText("Discard your changes?")
	a.Click(telatest.ByRole("button", "Cancel"))
	a.ExpectNoText("Discard your changes?")
	a.ExpectText("You have unsaved changes")
	a.Expect(telatest.ByRole("radio", "Spacious"), telatest.Checked(true))

	a.Click(reset)
	a.Click(telatest.ByRole("button", "Discard changes"))
	a.MouseMove(5, 5)
	a.ExpectNoText("Discard your changes?")
	a.ExpectText("All changes saved")
	// Back to the saved settings: Dark theme, Comfortable density.
	a.Expect(telatest.ByRole("radio", "Dark"), telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "Comfortable"), telatest.Checked(true))
}

// TestAppearanceGroups checks that choosing an option selects it and only it
// in its group, and leaves the other group alone: the theme is a radio group
// of cards, the density a segmented control.
func TestAppearanceGroups(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Expect(telatest.ByRole("radio", "System"), telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "Comfortable"), telatest.Checked(true))

	a.Click(telatest.ByRole("radio", "Dark"))
	a.Expect(telatest.ByRole("radio", "Dark"), telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "System"), telatest.Checked(false))
	a.Expect(telatest.ByRole("radio", "Light"), telatest.Checked(false))
	a.Expect(telatest.ByRole("radio", "Comfortable"), telatest.Checked(true))

	a.Click(telatest.ByRole("radio", "Compact"))
	a.Expect(telatest.ByRole("radio", "Compact"), telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "Comfortable"), telatest.Checked(false))
	a.Expect(telatest.ByRole("radio", "Spacious"), telatest.Checked(false))
	a.Expect(telatest.ByRole("radio", "Dark"), telatest.Checked(true))
	a.ExpectText("Fit more on screen.")

	// A segmented control keeps one item on: pressing it again does nothing.
	a.Click(telatest.ByRole("radio", "Compact"))
	a.Expect(telatest.ByRole("radio", "Compact"), telatest.Checked(true))
}

// TestRadioKeyboard moves the theme with the arrow keys, which check as
// they move (a radio group).
func TestRadioKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Focus(telatest.ByRole("radio", "System"))
	a.Press("ArrowLeft")
	a.Expect(telatest.ByRole("radio", "Dark"), telatest.Checked(true))
	a.ExpectText("You have unsaved changes")
}

// TestRebuildsAreNarrow checks that toggling a switch does not rebuild the
// whole page: the unchanged rows and the appearance card skip their build.
// Every kit component is a component of its own, so a row costs a few builds
// more than the divs it replaced (about 17 against 9), but the unchanged
// rows still skip.
func TestRebuildsAreNarrow(t *testing.T) {
	a := mount(t, windowW, windowH)
	before := a.Stats()
	a.Click(switchLoc("Weekly digest"))
	after := a.Stats()
	if builds := after.Builds - before.Builds; builds > 20 {
		t.Errorf("one switch rebuilt %d components, want about 17: the page, the row with its kit Field, Label and Switch, and the footer with its kit Card and Buttons", builds)
	}
	if skips := after.Skips - before.Skips; skips < 4 {
		t.Errorf("only %d builds skipped: the unchanged rows should skip", skips)
	}
}
