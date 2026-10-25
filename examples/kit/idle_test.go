package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// pulses are the pages that animate forever on purpose: a skeleton and an
// indeterminate bar pulse.
var pulses = map[string]bool{"Skeleton": true, "Progress": true}

// TestGoesIdle: once a page has been drawn, a window that nobody touches
// asks for no frames: no timer, no endless animation. The clock is advanced
// by hand (NoSettle), so the test does not depend on the machine's speed.
func TestGoesIdle(t *testing.T) {
	for _, name := range Previews.Names() {
		if pulses[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			a := mount(t, Previews[name], telatest.NoSettle())
			a.Advance(3 * time.Second)
			before := a.Stats()
			a.Advance(2 * time.Second)
			after := a.Stats()
			if after.Frames != before.Frames {
				t.Errorf("still requesting frames 3 s after the mount: %d frames in the next 2 s (%+v)", after.Frames-before.Frames, after)
			}
		})
	}
}

// TestOpenOverlaysGoIdle: with a dialog, a menu and a date picker open, and
// their entrances over, nothing asks for frames either, apart from the
// caret of a focused field (frames that only paint it).
func TestOpenOverlaysGoIdle(t *testing.T) {
	for _, tc := range []struct{ page, open string }{
		{"DropdownMenu", "View"},
		{"DatePicker", "Date"},
		{"Popover", "Open popover"},
		{"Select", "Select a fruit Fruit"},
	} {
		t.Run(tc.page, func(t *testing.T) {
			a := mount(t, Previews[tc.page], telatest.NoSettle())
			a.Click(telatest.ByRole("button", tc.open))
			a.Advance(3 * time.Second)
			before := a.Stats()
			a.Advance(2 * time.Second)
			after := a.Stats()
			if after.Builds != before.Builds || after.Styles != before.Styles || after.Layouts != before.Layouts {
				t.Errorf("an open %s rebuilt, styled or laid out the window while idle: %+v then %+v", tc.page, before, after)
			}
		})
	}
}

// TestDamage plays the gallery through the places most likely to paint
// wrong incrementally, checking every frame against a full rendering
// (telatest.CheckDamage): the page changes of the sidebar, the style and
// scheme switches (every class of the window changes), a modal dialog, a
// menu and a toast entering and leaving.
func TestDamage(t *testing.T) {
	a := mount(t, Gallery{}, telatest.CheckDamage())
	a.Click(tabFor("Dialog"))
	a.Click(telatest.ByRole("button", "Edit profile"))
	a.Press("Escape")
	a.Click(tabFor("Dropdown Menu"))
	a.Click(telatest.ByRole("button", "View"))
	a.Press("ArrowDown")
	a.Press("Escape")
	a.Click(tabFor("Toast"))
	a.Click(telatest.ByRole("button", "Success"))
	a.Advance(5 * time.Second)
	a.Click(rounded)
	a.Click(darkSwitch)
	a.Click(tabFor("Calendar"))
	a.Press("Tab")
	a.Click(telatest.ByRole("radio", "Default"))
	a.Click(darkSwitch)
}
