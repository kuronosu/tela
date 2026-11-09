package aqua_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/telatest"
)

// strip is the controls whose text sits on the accent or on the window
// itself, over the look's window background (no backdrop): a Default
// button, a Link button and a Select.
type strip struct {
	Look   looks.Look
	Scheme tela.ColorScheme
}

func (s strip) Build() tela.Node {
	return Div(Class("relative flex h-full w-full items-center gap-4 p-6 bg-background text-foreground select-none", s.Look.Regions.Window), Scheme(s.Scheme),
		kit.StyleProvider{Style: s.Look.Style, Children: Div(Class("flex items-center gap-4"),
			kit.Button{Children: Text("Continue")},
			kit.Button{Variant: kit.Link, Children: Text("Learn more")},
			kit.Select[string]{DefaultValue: new("apple"), AriaLabel: "Fruit", Class: "w-40", Items: []kit.SelectItem[string]{
				{Value: "apple", Label: "Apple"}, {Value: "banana", Label: "Banana"}}},
		)})
}

// stripContrast is the ratio between the text pixel of a control and its
// fill, measured as in textContrast but over a plain window background.
func stripContrast(t *testing.T, a *telatest.App, l telatest.Locator) float64 {
	t.Helper()
	worst, _ := textContrast(t, a, l)
	return worst
}

// TestAccentTextContrast measures the text on the accent (Default's white
// on the blue and the Link's blue on the window) and the Select trigger's
// text, in each design and scheme, with the sheet's measuring but over the
// plain window. The Default and Link buttons keep macOS's own accent and
// are not made darker: a look reproduces the platform (ADR-264), so their
// ratios are pinned here as floors just under what they measure (Default
// 3.2:1 to 4.2:1, Link 2.8:1 in Tahoe's light to 4.6:1), not the 4.5:1 of
// body text. The Select's text is the foreground over a glass or a gray
// fill: 8.6:1 in Tahoe's dark, the lowest, and it passes AA.
func TestAccentTextContrast(t *testing.T) {
	for _, l := range everyLook() {
		for _, s := range schemes {
			t.Run(l.Name+"/"+s.name, func(t *testing.T) {
				a := telatest.Mount(t, strip{Look: l, Scheme: s.scheme}, telatest.Size(900, 560), telatest.Theme(l.Theme))
				def := stripContrast(t, a, telatest.ByRole("button", "Continue"))
				link := stripContrast(t, a, telatest.ByRole("button", "Learn more"))
				sel := stripContrast(t, a, telatest.ByRole("button", "Apple Fruit"))
				t.Logf("Default %.2f:1, Link %.2f:1, Select %.2f:1", def, link, sel)
				if def < 3.1 {
					t.Errorf("Default's text is %.2f:1 on the accent; the floor is 3.1:1 (macOS's own accent)", def)
				}
				if link < 2.7 {
					t.Errorf("Link's text is %.2f:1 on the window; the floor is 2.7:1 (macOS's own accent)", link)
				}
				if sel < 4.5 {
					t.Errorf("the Select trigger's text is %.2f:1; WCAG AA needs 4.5:1", sel)
				}
			})
		}
	}
}
