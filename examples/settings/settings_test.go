package main

import (
	"image/color"
	"testing"

	"github.com/kuronosu/tela/examples/internal/exampletest"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

const windowW, windowH = 1280, 800

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		a := exampletest.Open(t, Settings, s[0], s[1])
		a.CheckLogs()
	}
}

func TestGolden(t *testing.T) {
	exampletest.Golden(t, "settings", Settings, windowW, windowH)
}

// switches returns the five switches of the notifications card in order:
// email, push, digest, marketing and the locked security switch.
func switches(a *exampletest.App) []core.InspectNode {
	t := a.T
	t.Helper()
	sw := a.WithClass("h-[22px]", "w-10", "rounded-full")
	if len(sw) != 5 {
		t.Fatalf("%d switches, want 5", len(sw))
	}
	return sw
}

// trackPixel samples the left end of a switch track, away from the knob in
// both positions of the knob (it is 18 px wide in a 40 px track).
func trackPixel(a *exampletest.App, sw core.InspectNode) color.RGBA {
	return a.Image().RGBAAt(int(sw.Rect.Center().X), int(sw.Rect.Y0)+2)
}

func TestSwitchTogglesAndDirtiesTheForm(t *testing.T) {
	a := exampletest.Open(t, Settings, windowW, windowH)
	if !a.HasText("All changes saved") {
		t.Fatal("a fresh form is not saved")
	}
	digest := switches(a)[2]
	off := trackPixel(a, digest)
	knobX := func() float32 {
		k, ok := a.Find(func(n core.InspectNode) bool {
			return n.Rect.Y0 > digest.Rect.Y0 && n.Rect.Y1 < digest.Rect.Y1 && exampletest.HasClass(n, "rounded-full") && exampletest.HasClass(n, "shadow-sm") && n.Rect.Width() < 20
		})
		if !ok {
			t.Fatal("no knob")
		}
		return k.Rect.X0
	}
	k0 := knobX()

	c := digest.Rect.Center()
	a.Click(c.X, c.Y)
	a.Move(5, 5)
	if !a.HasText("You have unsaved changes") {
		t.Error("toggling a switch did not mark the form as changed")
	}
	if on := trackPixel(a, switches(a)[2]); on == off || on.R > 60 {
		t.Errorf("the track is %v after switching on, was %v: want a dark track", on, off)
	}
	if k := knobX(); k < k0+10 {
		t.Errorf("knob did not move to the right: %v -> %v", k0, k)
	}

	// Toggling back makes the form clean again: prefs == saved.
	a.Click(c.X, c.Y)
	if !a.HasText("All changes saved") {
		t.Error("restoring the value did not clear the unsaved changes")
	}
}

// TestSwitchStates checks hover: and active: on a switch, and that the
// pointer cursor shows over it.
func TestSwitchStates(t *testing.T) {
	a := exampletest.Open(t, Settings, windowW, windowH)
	sw := switches(a)[3]
	rest := trackPixel(a, sw)
	c := sw.Rect.Center()

	a.Move(c.X, c.Y)
	hover := trackPixel(a, sw)
	if hover == rest {
		t.Error("hover: did not change the track")
	}
	if got := a.Window.Cursor(); got != platform.CursorPointer {
		t.Errorf("cursor over a switch is %v, want pointer", got)
	}
	a.Down()
	active := trackPixel(a, sw)
	if active == hover || active == rest {
		t.Errorf("active: did not change the track (rest %v hover %v active %v)", rest, hover, active)
	}
	a.Up() // click: switches on
	a.Move(5, 5)
	if !a.HasText("You have unsaved changes") {
		t.Error("the click did not toggle")
	}
}

// TestLockedSwitch checks the disabled switch ignores clicks, and shows the
// not-allowed cursor of its disabled: classes.
func TestLockedSwitch(t *testing.T) {
	a := exampletest.Open(t, Settings, windowW, windowH)
	sw := switches(a)[4]
	c := sw.Rect.Center()
	a.Move(c.X, c.Y)
	if got := a.Window.Cursor(); got != platform.CursorNotAllowed {
		t.Errorf("cursor over the locked switch is %v, want not-allowed (disabled:cursor-not-allowed)", got)
	}
	a.Click(c.X, c.Y)
	if !a.HasText("All changes saved") {
		t.Error("a click on the locked switch changed the form")
	}
}

// TestSaveAndReset drives the footer: Save and Reset are disabled until the
// form changes, Save stores the changes and Reset throws them away.
func TestSaveAndReset(t *testing.T) {
	a := exampletest.Open(t, Settings, windowW, windowH)
	saveBtn := func() core.InspectNode { return a.Button("Save changes") }

	// Disabled: a click does nothing and no hover look applies.
	c := saveBtn().Rect.Center()
	a.Click(c.X, c.Y)
	if !a.HasText("All changes saved") {
		t.Error("clicking a disabled Save changed the form")
	}
	if a.Window.Cursor() != platform.CursorNotAllowed {
		t.Errorf("cursor over a disabled Save is %v", a.Window.Cursor())
	}

	// Change something, then Save.
	a.ClickText("Dark")
	if !a.HasText("You have unsaved changes") {
		t.Fatal("choosing a theme did not mark the form as changed")
	}
	a.ClickButton("Save changes")
	a.Move(5, 5)
	if !a.HasText("All changes saved") {
		t.Error("Save did not store the changes")
	}

	// Change again, then Reset: back to the saved theme (Dark).
	a.ClickText("Spacious")
	a.ClickButton("Reset")
	a.Move(5, 5)
	if !a.HasText("All changes saved") {
		t.Error("Reset did not restore the saved form")
	}
	if got := selectedOptions(a); len(got) != 2 {
		t.Errorf("%d selected options after Reset, want theme and density", len(got))
	}
	a.CheckLogs()
}

// selectedOptions returns the option cards drawn as selected (ring-1).
func selectedOptions(a *exampletest.App) []core.InspectNode {
	return a.WithClass("ring-1", "border")
}

// TestRadioGroups checks that choosing an option selects it and only it in
// its group, and leaves the other group alone.
func TestRadioGroups(t *testing.T) {
	a := exampletest.Open(t, Settings, windowW, windowH)
	selected := func(label string) bool {
		return exampletest.HasClass(a.Button(label), "ring-1")
	}
	if !selected("System") || !selected("Comfortable") {
		t.Fatal("the defaults are not selected")
	}
	a.ClickText("Dark")
	if !selected("Dark") || selected("System") || selected("Light") {
		t.Error("theme group: Dark should be the only selected theme")
	}
	if !selected("Comfortable") {
		t.Error("choosing a theme changed the density")
	}
	a.ClickText("Compact")
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
	a := exampletest.Open(t, Settings, windowW, windowH)
	before := a.Tree().Stats()
	c := switches(a)[2].Rect.Center()
	a.Click(c.X, c.Y)
	after := a.Tree().Stats()
	if builds := after.Builds - before.Builds; builds > 12 {
		t.Errorf("one switch rebuilt %d components, want about 9 (page, three cards, one row, one switch...)", builds)
	}
	if skips := after.Skips - before.Skips; skips < 4 {
		t.Errorf("only %d builds skipped: the unchanged rows should skip", skips)
	}
}
