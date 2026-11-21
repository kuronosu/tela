package telatest_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// HasRange reads aria-valuenow, -valuemin and -valuemax, which the aria
// snapshot prints only as the value text.
func TestHasRange(t *testing.T) {
	page := func() tela.Node {
		v, setV := tela.UseState(40.0)
		return Div(
			Div(Role("slider"), AriaLabel("Volume"), TabIndex(0), AriaValueNow(v), AriaValueMin(0), AriaValueMax(100), AriaValueText("40 percent"), Class("h-4 w-40"),
				OnKeyDown(func(e tela.KeyboardEvent) {
					if e.Key == "ArrowRight" {
						setV.Set(v + 10)
					}
				})),
			Div(Role("progressbar"), AriaLabel("Download"), AriaValueNow(3), Class("h-4 w-40")),
			Div(Role("button"), AriaLabel("Plain"), Class("h-4 w-40")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	slider := telatest.ByRole("slider", "Volume")
	app.Expect(slider, telatest.HasRange(40, 0, 100), telatest.HasValue("40 percent"))
	app.Focus(slider)
	app.Press("ArrowRight")
	app.Expect(slider, telatest.HasRange(50, 0, 100)) // waits for the update, as every expectation
	mustFail(t, func(tb testing.TB) {
		a := telatest.MountFunc(tb, page, telatest.Size(300, 200), telatest.Timeout(50*1e6))
		a.Expect(telatest.ByRole("slider", "Volume"), telatest.HasRange(40, 0, 10))
	}, "range now=40 min=0 max=10", "now=40 min=0 max=100")
	// A bar with only a value has no bounds: HasRange wants all three.
	mustFail(t, func(tb testing.TB) {
		a := telatest.MountFunc(tb, page, telatest.Size(300, 200), telatest.Timeout(50*1e6))
		a.Expect(telatest.ByRole("progressbar", "Download"), telatest.HasRange(3, 0, 100))
	}, "now=3 min=unset max=unset")
	mustFail(t, func(tb testing.TB) {
		a := telatest.MountFunc(tb, page, telatest.Size(300, 200), telatest.Timeout(50*1e6))
		a.Expect(telatest.ByRole("button", "Plain"), telatest.HasRange(0, 0, 1))
	}, "not a range widget")
}
