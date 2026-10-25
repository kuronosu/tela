package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// TestSelectItemsMirrorInRTL puts the check of the chosen option at the end
// of its row and the text at the start, as the logical padding and inset of
// the recipe (ps-2 pe-8, end-2) do: the check at the right and the text at
// the left in English, the other way round under rtl.
func TestSelectItemsMirrorInRTL(t *testing.T) {
	for _, c := range []struct {
		dir string
		rtl bool
	}{{"ltr", false}, {"rtl", true}} {
		t.Run(c.dir, func(t *testing.T) {
			app := telatest.MountFunc(t, func() tela.Node {
				return h.Div(h.Dir(c.dir), h.Class("p-6"),
					kit.Select[string]{Items: fruitItems, DefaultValue: new("banana"), DefaultOpen: true, AriaLabel: "Fruit"})
			}, telatest.Size(400, 360), telatest.Theme(kit.Theme()))
			row := app.Bounds(telatest.ByRole("option", "Banana"))
			check := app.Bounds(telatest.ByClass("absolute", "end-2").Nth(1)) // the second option, Banana
			rowRight, checkRight := row.X+row.Width, check.X+check.Width
			if c.rtl {
				// The check is within 8 px (end-2) of the left edge of the row.
				if check.X < row.X || check.X > row.X+9 {
					t.Errorf("rtl: the check is at x=%v in the row %+v, want it at the left end", check.X, row)
				}
			} else if checkRight > rowRight || checkRight < rowRight-9 {
				t.Errorf("ltr: the check ends at x=%v in the row %+v, want it at the right end", checkRight, row)
			}
		})
	}
}
