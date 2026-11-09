package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// hasRed reports whether the element has a red pixel: the glyph of a toggle
// whose icon class says text-[#ff0000].
func hasRed(app *telatest.App, l telatest.Locator) bool {
	r := app.Bounds(l)
	img := app.Screenshot()
	for y := int(r.Y); y < int(r.Y+r.Height); y++ {
		for x := int(r.X); x < int(r.X+r.Width); x++ {
			if c := img.RGBAAt(x, y); c.R > 200 && c.G < 60 && c.B < 60 {
				return true
			}
		}
	}
	return false
}

// ToggleStyle.IconVariants adds classes to the glyph of one variant, in a
// Toggle and in the items of a ToggleGroup, and leaves the others alone.
func TestToggleIconVariants(t *testing.T) {
	s := kit.DefaultStyle()
	s.Toggle.IconVariants[kit.Outline] = "text-[#ff0000]"
	app := telatest.Mount(t, kit.StyleProvider{Style: &s, Children: h.Div(h.Class("flex gap-2 p-4"),
		kit.Toggle{Icon: iconBold, AriaLabel: "Bold"},
		kit.Toggle{Variant: kit.Outline, Icon: iconItalic, AriaLabel: "Italic"},
		kit.ToggleGroup[string]{AriaLabel: "Marks", Variant: kit.Outline, Items: []kit.ToggleGroupItem[string]{
			{Value: "u", Icon: iconUnderline, AriaLabel: "Underline"}}},
		kit.ToggleGroup[string]{AriaLabel: "Plain", Items: []kit.ToggleGroupItem[string]{
			{Value: "b", Icon: iconBold, AriaLabel: "Plain bold"}}},
	)})
	for name, want := range map[string]bool{"Bold": false, "Italic": true, "Underline": true, "Plain bold": false} {
		role := "button"
		if name == "Underline" || name == "Plain bold" {
			role = "radio"
		}
		if got := hasRed(app, telatest.ByRole(role, name)); got != want {
			t.Errorf("%s: red glyph = %v, want %v", name, got, want)
		}
	}
}

// A style with no IconVariants (a nil map) still draws the glyph.
func TestToggleIconVariantsNil(t *testing.T) {
	s := kit.DefaultStyle()
	s.Toggle.IconVariants = nil
	s.Toggle.Icon = "pointer-events-none size-4 shrink-0 text-[#ff0000]"
	app := telatest.Mount(t, kit.StyleProvider{Style: &s, Children: kit.Toggle{Icon: tela.Icon{SVG: iconBold.SVG}, AriaLabel: "Bold"}})
	if !hasRed(app, telatest.ByRole("button", "Bold")) {
		t.Error("no glyph drawn with a nil IconVariants")
	}
}
