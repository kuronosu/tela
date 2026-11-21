package telatest_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Stats.LaidOut counts the boxes laid out, so "no relayout" is assertable:
// a restyle that changes only a color runs a layout phase (Layouts grows)
// that lays nothing out, while a change of size lays boxes out.
func TestStatsLaidOutSeesNoRelayout(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(false)
		wide, setWide := tela.UseState(false)
		class := "bg-slate-200 p-4 w-40"
		if on {
			class = "bg-red-500 p-4 w-40"
		}
		if wide {
			class = "bg-slate-200 p-4 w-60"
		}
		return Div(
			Button(OnClick(func() { setOn.Set(!on) }), Text("Color")),
			Button(OnClick(func() { setWide.Set(!wide) }), Text("Width")),
			Div(Class(class), Text("Box")))
	}, telatest.Size(300, 200))
	before := app.Stats()
	app.Click(telatest.ByRole("button", "Color"))
	after := app.Stats()
	if after.Frames == before.Frames {
		t.Fatal("the color change ran no frame")
	}
	if got := after.LaidOut - before.LaidOut; got != 0 {
		t.Errorf("a color change laid out %d boxes, want 0", got)
	}
	if after.Layouts == before.Layouts {
		t.Errorf("the color change ran no layout phase: Stats.Layouts no longer counts empty ones, update its doc")
	}
	before = app.Stats()
	app.Click(telatest.ByRole("button", "Width"))
	after = app.Stats()
	if got := after.LaidOut - before.LaidOut; got == 0 {
		t.Errorf("a width change laid out no box")
	}
}
