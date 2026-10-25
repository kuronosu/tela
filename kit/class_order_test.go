package kit_test

import (
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The outline variant's border-border is written after the root's
// focus-visible:border-ring; as in Tailwind's stylesheet, the focus border
// must still show when the button has the keyboard focus.
func TestOutlineButtonFocusBorder(t *testing.T) {
	app := telatest.Mount(t, h.Div(h.Class("p-8"),
		kit.Button{Variant: kit.Outline, Children: h.Text("Outline")}), telatest.Size(200, 100))
	b := app.Bounds(telatest.ByRole("button", "Outline"))
	x, y := b.X+0.5, b.Y+b.Height/2
	before := app.PixelAt(x, y)
	app.Press("Tab")
	app.Expect(telatest.ByRole("button", "Outline"), telatest.Focused(true))
	if after := app.PixelAt(x, y); after == before {
		t.Fatalf("the border of a keyboard-focused outline button is %v, as unfocused; want the ring color", after)
	}
}
