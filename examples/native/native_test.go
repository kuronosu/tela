package main

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// mount opens the explorer in dev mode, where any dev message (an unknown
// class in a theme, say) fails the test.
func mount(t *testing.T, p Props, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(windowW, windowH), telatest.Dev()}, opts...)
	return telatest.Mount(t, tela.CP(Explorer, p), opts...)
}

// shown is the screen of the previews: Documents with a file selected.
func shown(theme string, m Mode) Props {
	return Props{Theme: theme, Mode: m, Path: "Documents", Sel: "Contract draft.docx"}
}

var modes = []Mode{Light, Dark}

// eachTheme runs f once per theme and scheme.
func eachTheme(t *testing.T, f func(t *testing.T, id string, m Mode)) {
	for _, id := range ThemeIDs {
		for _, m := range modes {
			t.Run(fmt.Sprintf("%s/%s", id, m), func(t *testing.T) { f(t, id, m) })
		}
	}
}

func opt(name string) telatest.Locator { return telatest.ByRole("option", name) }
func btn(name string) telatest.Locator { return telatest.ByRole("button", name) }
func search() telatest.Locator         { return telatest.ByRole("searchbox", "Search") }

// TestNoDevMessages fails on an unknown class or any other dev warning, at
// several window sizes, on the three screens of each theme.
func TestNoDevMessages(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		for _, s := range [][2]float32{{windowW, windowH}, {800, 500}, {1600, 900}} {
			for _, p := range []Props{shown(id, m), {Theme: id, Mode: m}, {Theme: id, Mode: m, Path: "Documents", Query: "zzz"}} {
				telatest.Mount(t, tela.CP(Explorer, p), telatest.Size(s[0], s[1]), telatest.Dev())
			}
		}
	})
}

// TestGolden is each theme in each scheme at 1280x760.
func TestGolden(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		mount(t, shown(id, m)).Golden(fmt.Sprintf("native_%s_%s.png", id, m))
	})
}

// TestGoldenScale2 is one screen per theme on a 2x display, to see the
// icons at their real sharpness.
func TestGoldenScale2(t *testing.T) {
	for _, id := range ThemeIDs {
		t.Run(id, func(t *testing.T) {
			mount(t, shown(id, Light), telatest.Scale(2)).Golden(fmt.Sprintf("native_%s_light_x2.png", id))
		})
	}
}

// same reports whether two screenshots are identical.
func same(a, b *image.RGBA) bool {
	return a.Bounds() == b.Bounds() && string(a.Pix) == string(b.Pix)
}

// TestAutoFollowsTheSystem checks that the Auto mode, which writes the dark
// values as dark: classes, draws the same pixels as the forced modes.
func TestAutoFollowsTheSystem(t *testing.T) {
	for _, id := range ThemeIDs {
		t.Run(id, func(t *testing.T) {
			light := mount(t, shown(id, Light)).Screenshot()
			dark := mount(t, shown(id, Dark)).Screenshot()
			if same(light, dark) {
				t.Fatal("light and dark draw the same pixels")
			}
			if got := mount(t, shown(id, Auto)).Screenshot(); !same(got, light) {
				t.Error("Auto on a light system differs from Light")
			}
			if got := mount(t, shown(id, Auto), telatest.Dark()).Screenshot(); !same(got, dark) {
				t.Error("Auto on a dark system differs from Dark")
			}
		})
	}
}

// TestThemesDiffer checks the six looks are six different pictures.
func TestThemesDiffer(t *testing.T) {
	var shots []*image.RGBA
	for _, id := range ThemeIDs {
		for _, m := range modes {
			shots = append(shots, mount(t, shown(id, m)).Screenshot())
		}
	}
	for i := range shots {
		for j := i + 1; j < len(shots); j++ {
			if same(shots[i], shots[j]) {
				t.Errorf("screens %d and %d are identical", i, j)
			}
		}
	}
}

// isSelected reports whether the option of that name is selected.
func isSelected(a *telatest.App, name string) bool {
	return strings.Contains(a.AriaSnapshot(), fmt.Sprintf("- option %q [selected]", name))
}

func TestAriaSnapshot(t *testing.T) {
	a := mount(t, shown("adwaita", Light))
	a.AriaGolden("native.aria.yml")
}
