package aria_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// screenPreviews are the states of the screen with an overlay open or a
// panel selected, each seeded through screenPage's fields.
var screenPreviews = []struct {
	name string
	page screenPage
}{
	{"overview", screenPage{}},
	{"actions menu", screenPage{Menu: "actions"}},
	{"view menu", screenPage{Menu: "view"}},
	{"combobox", screenPage{Search: "se", SearchOpen: true}},
	{"tooltip", screenPage{Tooltip: true}},
	{"dialog", screenPage{Dialog: true, Version: "v2.14.1"}},
	{"dialog select", screenPage{Dialog: true, Version: "v2.14.1", EnvOpen: true}},
	{"activity tab", screenPage{Tab: "activity"}},
	{"alerts tab", screenPage{Tab: "alerts"}},
}

// TestScreenNoDevMessages mounts every state at several window sizes: a class
// typo, a missing key or a hook misuse is reported in dev mode, which fails
// the test.
func TestScreenNoDevMessages(t *testing.T) {
	for _, p := range screenPreviews {
		for _, s := range [][2]float32{{1120, 760}, {720, 600}, {1600, 900}} {
			t.Run(fmt.Sprintf("%s/%gx%g", p.name, s[0], s[1]), func(t *testing.T) {
				screenMount(t, p.page, telatest.Size(s[0], s[1]))
			})
		}
	}
}

// TestScreenDamage plays every overlay in and out, with the keyboard and the
// pointer, checking each frame against a full rendering
// (telatest.CheckDamage): the entrances and exits (Presence, starting:, scale
// and opacity transitions), the floating elements that are placed in the
// frame that shows them, the focus rings and the scrolled list of the
// combobox must paint the same pixels incrementally as from scratch.
func TestScreenDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			a := screenMount(t, screenPage{}, telatest.Scale(scale), telatest.CheckDamage())

			// The menus.
			screenPressTab(a, 4)
			a.Press("Enter")
			a.Press("ArrowDown")
			a.Press("End")
			a.Press("Escape")
			a.Press("Shift+Tab")
			a.Press("Space") // opens the View menu on its first item
			a.Press("Space") // unchecks Branch: the column leaves the list
			a.Press("Escape")

			// The combobox, with a list that scrolls.
			a.Click(screenProjectToggle)
			a.Hover(telatest.ByRole("option", "edge-router"))
			a.Scroll(telatest.ByRole("listbox", "Project"), 0, 120)
			a.Press("End")
			a.Press("Escape")
			a.Click(screenProjectBox)
			a.Press("Control+a")
			a.Type("an")
			a.Press("ArrowDown")
			a.Press("Escape")

			// The dialog, its select, and the tooltip with its arrow.
			a.Click(screenNewButton)
			a.Type("v1")
			a.Press("Tab")
			a.Press("Enter")
			a.Press("ArrowDown")
			a.Press("Enter")
			a.Press("Escape")
			a.Hover(screenBellButton)
			a.Press("Tab")
		})
	}
}

// TestScreenGoesIdle: once the entrance transitions end, a window with an
// overlay open asks for no frames: no timer, no endless animation (the hooks'
// delays run on the frame clock and the floating elements follow their
// anchors without a frame of their own). The clock is advanced by hand
// (NoSettle), so the test does not depend on the machine's speed.
//
// The dialog's text field has the focus, so its caret blinks: those frames
// only paint it, and nothing is built, styled or laid out again.
func TestScreenGoesIdle(t *testing.T) {
	for _, p := range screenPreviews {
		t.Run(p.name, func(t *testing.T) {
			a := screenMount(t, p.page, telatest.NoSettle())
			a.Advance(3 * time.Second)
			before := a.Stats()
			a.Advance(2 * time.Second)
			after := a.Stats()
			if p.name == "dialog" {
				if after.Builds != before.Builds || after.Styles != before.Styles || after.Layouts != before.Layouts {
					t.Errorf("a blinking caret rebuilt, styled or laid out the window: %+v then %+v", before, after)
				}
				return
			}
			if after.Frames != before.Frames {
				t.Errorf("still requesting frames 3 s after the mount: %d frames in the next 2 s (%+v)", after.Frames-before.Frames, after)
			}
		})
	}
}

// TestScreenAriaGolden pins the whole aria snapshot of the screen, closed and
// with the combobox's list open: the landmarks, the tab list with its panel,
// the list box and the open list, as the hooks put them together.
func TestScreenAriaGolden(t *testing.T) {
	screenMount(t, screenPage{}).AriaGolden("screen.aria.yml")
	a := screenMount(t, screenPage{})
	a.Press("Tab")
	a.Press("Control+a")
	a.Type("se")
	a.Press("ArrowDown")
	a.AriaGolden("screen_combobox_open.aria.yml")
}
