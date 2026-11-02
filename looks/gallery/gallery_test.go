package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// mount opens the gallery with the tokens of its look, as main does; dev mode
// is on, so any dev message (an unknown class in a recipe, say) fails the
// test.
func mount(t *testing.T, g Gallery, opts ...telatest.Option) *telatest.App {
	t.Helper()
	l := byID(g.Look).Look()
	opts = append([]telatest.Option{telatest.Size(windowW, windowH), telatest.Theme(l.Theme)}, opts...)
	return telatest.Mount(t, g, opts...)
}

var schemes = []tela.ColorScheme{tela.Light, tela.Dark}

func schemeName(s tela.ColorScheme) string {
	if s == tela.Dark {
		return "dark"
	}
	return "light"
}

// eachLook runs f once per look and scheme.
func eachLook(t *testing.T, f func(t *testing.T, id string, s tela.ColorScheme)) {
	for _, c := range catalog {
		for _, s := range schemes {
			t.Run(fmt.Sprintf("%s/%s", c.ID, schemeName(s)), func(t *testing.T) { f(t, c.ID, s) })
		}
	}
}

func same(a, b *image.RGBA) bool {
	return a.Bounds() == b.Bounds() && string(a.Pix) == string(b.Pix)
}

// TestNoDevMessages fails on an unknown class or any other dev warning, at
// several window sizes.
func TestNoDevMessages(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		for _, size := range [][2]float32{{windowW, windowH}, {800, 500}, {1600, 900}} {
			mount(t, Gallery{Look: id, Scheme: s}, telatest.Size(size[0], size[1]))
		}
	})
}

// TestGolden is each look in each scheme at 1100x720.
func TestGolden(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		mount(t, Gallery{Look: id, Scheme: s}).Golden(fmt.Sprintf("gallery_%s_%s.png", id, schemeName(s)))
	})
}

// TestGoldenScale2 is one look on a 2x display, to see the icons and the
// hairlines at their real sharpness.
func TestGoldenScale2(t *testing.T) {
	mount(t, Gallery{Look: "win11", Scheme: tela.Light}, telatest.Scale(2)).Golden("gallery_win11_light_x2.png")
}

// TestLooksDiffer checks the six screens are six different pictures.
func TestLooksDiffer(t *testing.T) {
	var shots []*image.RGBA
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		shots = append(shots, mount(t, Gallery{Look: id, Scheme: s}).Screenshot())
	})
	for i := range shots {
		for j := i + 1; j < len(shots); j++ {
			if same(shots[i], shots[j]) {
				t.Errorf("screens %d and %d are identical", i, j)
			}
		}
	}
}

// TestFollowsTheSystem checks that with no scheme of its own the gallery
// takes the system's: the tokens, not dark: classes, carry the scheme, so the
// empty part of the content is the color the forced scheme paints.
func TestFollowsTheSystem(t *testing.T) {
	for _, c := range catalog {
		t.Run(c.ID, func(t *testing.T) {
			light := mount(t, Gallery{Look: c.ID, Scheme: tela.Light}).PixelAt(420, 690)
			dark := mount(t, Gallery{Look: c.ID, Scheme: tela.Dark}).PixelAt(420, 690)
			if light == dark {
				t.Fatalf("light and dark paint the same %v", light)
			}
			if got := mount(t, Gallery{Look: c.ID}).PixelAt(420, 690); got != light {
				t.Errorf("a light system paints %v, want %v", got, light)
			}
			if got := mount(t, Gallery{Look: c.ID}, telatest.Dark()).PixelAt(420, 690); got != dark {
				t.Errorf("a dark system paints %v, want %v", got, dark)
			}
		})
	}
}

// TestSwitchLook clicks the other looks of the sidebar: the window is then
// the same picture as the gallery that started with that look, so the theme
// and the style are what the switch swaps.
func TestSwitchLook(t *testing.T) {
	for _, from := range catalog {
		for _, to := range catalog {
			if from.ID == to.ID {
				continue
			}
			t.Run(from.ID+"->"+to.ID, func(t *testing.T) {
				a := mount(t, Gallery{Look: from.ID, Scheme: tela.Light})
				a.Click(telatest.ByRole("radio", to.Title))
				want := mount(t, Gallery{Look: to.ID, Scheme: tela.Light}).Screenshot()
				if !same(a.Screenshot(), want) {
					t.Errorf("after choosing %s the window is not the gallery of %s", to.Title, to.ID)
				}
			})
		}
	}
}

// TestSwitchScheme clicks Dark and Light in the sidebar.
func TestSwitchScheme(t *testing.T) {
	a := mount(t, Gallery{Look: "adwaita", Scheme: tela.Light})
	a.Click(telatest.ByRole("radio", "Dark"))
	if !same(a.Screenshot(), mount(t, Gallery{Look: "adwaita", Scheme: tela.Dark}).Screenshot()) {
		t.Error("choosing Dark does not draw the dark gallery")
	}
	a.Click(telatest.ByRole("radio", "Light"))
	if !same(a.Screenshot(), mount(t, Gallery{Look: "adwaita", Scheme: tela.Light}).Screenshot()) {
		t.Error("choosing Light does not draw the light gallery")
	}
}

// TestArrowsStepThroughLooks: the toolbar's arrows go to the previous and the
// next look, around.
func TestArrowsStepThroughLooks(t *testing.T) {
	a := mount(t, Gallery{Look: "win11"})
	a.Click(telatest.ByRole("button", "Previous look"))
	a.Expect(telatest.ByRole("radio", "macOS mixed"), telatest.Checked(true))
	a.Click(telatest.ByRole("button", "Next look"))
	a.Expect(telatest.ByRole("radio", "Windows 11"), telatest.Checked(true))
}

// TestAquaDesignsInTheGallery: the macOS entries draw aqua's designs, and
// their keys parse.
func TestAquaDesignsInTheGallery(t *testing.T) {
	for id, name := range map[string]string{"tahoe": "macOS Tahoe", "sequoia": "macOS Sequoia", "mixed": "macOS Tahoe (mixed: Sequoia controls)"} {
		if got := byID(id).Look().Name; got != name {
			t.Errorf("look %q is %q, want %q", id, got, name)
		}
		if got, err := parseLook(id); err != nil || got != id {
			t.Errorf("parseLook(%q) = %q, %v", id, got, err)
		}
	}
}

// TestOverlays opens the menu, the dialog and the select in every look and
// scheme; a recipe with a class the look's theme does not know fails here.
func TestOverlays(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := mount(t, Gallery{Look: id, Scheme: s})
		a.Click(telatest.ByRole("button", "Actions"))
		a.Expect(telatest.ByRole("menuitem", "Copy"), telatest.Disabled(false))
		a.Press("Escape")
		a.Click(telatest.ByRole("button", "Rename…"))
		a.Expect(telatest.ByRole("dialog", "Rename"), telatest.Disabled(false))
		a.Press("Escape")
		a.Click(telatest.ByRole("button", "Apple Fruit"))
		a.Expect(telatest.ByRole("option", "Banana"), telatest.Disabled(false))
	})
}

// TestGoldenOverlays is the dialog (light) and the menu (dark) of each look.
func TestGoldenOverlays(t *testing.T) {
	for _, c := range catalog {
		t.Run(c.ID, func(t *testing.T) {
			a := mount(t, Gallery{Look: c.ID, Scheme: tela.Light})
			a.Click(telatest.ByRole("button", "Rename…"))
			a.Golden(fmt.Sprintf("gallery_%s_light_dialog.png", c.ID))

			b := mount(t, Gallery{Look: c.ID, Scheme: tela.Dark})
			b.Click(telatest.ByRole("button", "Actions"))
			b.Golden(fmt.Sprintf("gallery_%s_dark_menu.png", c.ID))
		})
	}
}

// TestTableSelection: a click on a row selects it.
func TestTableSelection(t *testing.T) {
	a := mount(t, Gallery{Look: "breeze"})
	row := func(name string) telatest.Locator { return telatest.ByRole("row", name) }
	a.Expect(row("Photo"), telatest.Selected(true))
	a.Click(telatest.ByRole("cell", "Budget"))
	a.Expect(row("Budget"), telatest.Selected(true))
	a.Expect(row("Photo"), telatest.Selected(false))
}

// TestToggleDetails: the details pane is a checkbox of the menu.
func TestToggleDetails(t *testing.T) {
	a := mount(t, Gallery{Look: "breeze"})
	a.ExpectCount(telatest.ByRole("complementary", "Details"), 1)
	a.Click(telatest.ByRole("button", "More options"))
	a.Click(telatest.ByRole("menuitemcheckbox", "Details pane"))
	a.ExpectCount(telatest.ByRole("complementary", "Details"), 0)
}

func TestParse(t *testing.T) {
	for in, want := range map[string]string{"win11": "win11", " KDE ": "", "breeze": "breeze", "Adwaita": "adwaita"} {
		got, err := parseLook(in)
		if want == "" {
			if err == nil {
				t.Errorf("parseLook(%q) = %q, want an error", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("parseLook(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := parseLook(""); err != nil {
		t.Errorf("parseLook(\"\"): %v", err)
	}
	if s, err := parseMode("dark"); err != nil || s != tela.Dark {
		t.Errorf("parseMode(dark) = %v, %v", s, err)
	}
	if _, err := parseMode("maybe"); err == nil {
		t.Error("parseMode(maybe) did not fail")
	}
}

func TestAriaSnapshot(t *testing.T) {
	mount(t, Gallery{Look: "adwaita", Scheme: tela.Light}).AriaGolden("gallery.aria.yml")
}
