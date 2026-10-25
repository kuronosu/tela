package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// mount opens the explorer with the tokens of its look, as main does; dev
// mode is on, so any dev message (an unknown class in a recipe, say) fails
// the test.
func mount(t *testing.T, p Explorer, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(windowW, windowH), telatest.Theme(Looks[p.Look].Theme)}, opts...)
	return telatest.Mount(t, p, opts...)
}

// shown is the screen of the previews: Documents with a file selected.
func shown(look string, s tela.ColorScheme) Explorer {
	return Explorer{Look: look, Scheme: s, Path: "Documents", Sel: "Contract draft.docx"}
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
	for _, id := range LookIDs {
		for _, s := range schemes {
			t.Run(fmt.Sprintf("%s/%s", id, schemeName(s)), func(t *testing.T) { f(t, id, s) })
		}
	}
}

// row finds the row of an entry: a row is named by all its cells, so it is
// found by the text of the name.
func row(name string) telatest.Locator {
	return telatest.ByRole("row", "").Filter(telatest.HasText(name))
}
func cell(name string) telatest.Locator  { return telatest.ByRole("cell", name) }
func btn(name string) telatest.Locator   { return telatest.ByRole("button", name) }
func place(name string) telatest.Locator { return telatest.ByRole("radio", name) }

// search is the one search field; kit.Input is named by its placeholder.
func search() telatest.Locator { return telatest.ByRole("searchbox", "") }

// TestNoDevMessages fails on an unknown class or any other dev warning, at
// several window sizes, on the three screens of each look and with the
// icon view.
func TestNoDevMessages(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		for _, size := range [][2]float32{{windowW, windowH}, {800, 500}, {1600, 900}} {
			for _, p := range []Explorer{
				shown(id, s), {Look: id, Scheme: s},
				{Look: id, Scheme: s, Path: "Documents", Query: "zzz"},
				{Look: id, Scheme: s, Path: "Documents", Sel: "Resume.pdf", View: "icons"},
			} {
				mount(t, p, telatest.Size(size[0], size[1]))
			}
		}
	})
}

// TestGolden is each look in each scheme at 1280x760.
func TestGolden(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		mount(t, shown(id, s)).Golden(fmt.Sprintf("native_%s_%s.png", id, schemeName(s)))
	})
}

// TestGoldenScale2 is each look on a 2x display, to see the icons at their
// real sharpness.
func TestGoldenScale2(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		mount(t, shown(id, s), telatest.Scale(2)).Golden(fmt.Sprintf("native_%s_%s_x2.png", id, schemeName(s)))
	})
}

// same reports whether two screenshots are identical.
func same(a, b *image.RGBA) bool {
	return a.Bounds() == b.Bounds() && string(a.Pix) == string(b.Pix)
}

// TestFollowsTheSystem checks that with no scheme of its own the explorer
// draws the same pixels as the forced ones: the tokens, not dark: classes,
// carry the scheme.
func TestFollowsTheSystem(t *testing.T) {
	for _, id := range LookIDs {
		t.Run(id, func(t *testing.T) {
			light := mount(t, shown(id, tela.Light)).Screenshot()
			dark := mount(t, shown(id, tela.Dark)).Screenshot()
			if same(light, dark) {
				t.Fatal("light and dark draw the same pixels")
			}
			if got := mount(t, shown(id, tela.System)).Screenshot(); !same(got, light) {
				t.Error("a light system differs from Light")
			}
			if got := mount(t, shown(id, tela.System), telatest.Dark()).Screenshot(); !same(got, dark) {
				t.Error("a dark system differs from Dark")
			}
		})
	}
}

// TestLooksDiffer checks the six looks are six different pictures.
func TestLooksDiffer(t *testing.T) {
	var shots []*image.RGBA
	for _, id := range LookIDs {
		for _, s := range schemes {
			shots = append(shots, mount(t, shown(id, s)).Screenshot())
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

func TestAriaSnapshot(t *testing.T) {
	a := mount(t, shown("adwaita", tela.Light))
	a.AriaGolden("native.aria.yml")
}
