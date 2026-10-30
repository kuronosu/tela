package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestGalleryOpensOnTheIntroduction: the first tab of the sidebar is selected
// and the hero is the page.
func TestGalleryOpensOnTheIntroduction(t *testing.T) {
	a := mount(t, Gallery{})
	a.Expect(tabFor("Introduction"), telatest.Selected(true))
	a.ExpectRole("heading", "Hello, kit.")
	a.ExpectText("TELA / KIT")
	a.ExpectText("What it is")
	a.ExpectText("kit.Theme()")
	// A name that is not a page falls back to it.
	b := mount(t, Gallery{Page: "NoSuchPage"})
	b.Expect(tabFor("Introduction"), telatest.Selected(true))
}

// TestIntroductionListsEveryComponent: one button for each component, and
// pressing one opens its page, with the sidebar following.
func TestIntroductionListsEveryComponent(t *testing.T) {
	a := mount(t, Gallery{})
	for _, p := range pages {
		// The tab and the button have the same name.
		a.ExpectCount(telatest.ByRole("button", p.Label), 1)
	}
	a.Click(telatest.ByRole("button", "Calendar"))
	a.Expect(tabFor("Calendar"), telatest.Selected(true))
	a.ExpectCount(heading("Calendar"), 1)
	a.ExpectCount(telatest.ByRole("heading", "Hello, kit."), 0)
}

// TestIntroductionKeyboard reaches a component with the keyboard alone: Tab
// to the page, to a button, Enter.
func TestIntroductionKeyboard(t *testing.T) {
	a := mount(t, Gallery{})
	button := telatest.ByRole("button", "Date Picker")
	a.Focus(button)
	a.Press("Enter")
	a.Expect(tabFor("Date Picker"), telatest.Selected(true))
	a.ExpectCount(heading("Date Picker"), 1)

	b := mount(t, Gallery{})
	b.Focus(telatest.ByRole("button", "Table"))
	b.Press("Space")
	b.Expect(tabFor("Table"), telatest.Selected(true))
}

// TestIntroductionInAPreview: the preview has no sidebar, and a button changes
// the page it shows.
func TestIntroductionInAPreview(t *testing.T) {
	a := mount(t, Previews["Introduction"])
	a.ExpectRole("heading", "Hello, kit.")
	a.Click(telatest.ByRole("button", "Toggle Group"))
	a.ExpectCount(heading("Toggle Group"), 1)
}

// TestIntroductionFollowsTheSchemeAndTheStyle: the page is made of theme
// tokens, so Dark and the Rounded style reach it without a class of its own.
func TestIntroductionFollowsTheSchemeAndTheStyle(t *testing.T) {
	for _, style := range []string{"default", "rounded"} {
		light := mount(t, Gallery{Style: style})
		dark := mount(t, Gallery{Style: style}, telatest.Dark())
		if px := light.PixelAt(1000, 770); px.R < 200 {
			t.Errorf("%s: the light page is dark at its corner: %v", style, px)
		}
		if px := dark.PixelAt(1000, 770); px.R > 40 {
			t.Errorf("%s: the dark page is light at its corner: %v", style, px)
		}
	}
	a := mount(t, Gallery{})
	a.Click(rounded)
	a.Click(darkSwitch)
	if px := a.PixelAt(1000, 770); px.R > 40 {
		t.Errorf("the Introduction is not dark after the switch: %v", px)
	}
}
