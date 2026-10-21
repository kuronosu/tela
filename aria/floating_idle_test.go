package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// TestFloatingFlipAndSize: a long list below a button near the bottom of
// the window flips above it and is limited to the room there, in the first
// frame, and stays there (the two middleware do not undo each other).
func TestFloatingFlipAndSize(t *testing.T) {
	page := func() tela.Node {
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Placement("bottom-start"), aria.Offset(4), aria.Flip(), aria.Shift(8),
			aria.Size(func(a aria.Available) { a.MaxHeight(a.Height) }, aria.SizeOptions{Padding: 8}))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("anchor"), Class("absolute left-4 top-[230px] h-8 w-24"), Text("Open")),
			Portal(Div(fl.Props, TestID("list"), Class("flex w-40 flex-col overflow-y-auto bg-white"),
				Textf("%s", fl.Placement),
				Range(40, func(i int) tela.Node { return Div(Class("h-8 shrink-0"), Textf("Item %d", i)) }))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300), telatest.NoSettle())
	// Above the anchor: from the window's padding down to 4 px over it.
	expectRect(t, "list", app.Bounds(telatest.ByTestID("list")), 16, 8, 160, 218)
	app.ExpectText("top-start")
	app.Advance(time.Second)
	expectRect(t, "list once settled", app.Bounds(telatest.ByTestID("list")), 16, 8, 160, 218)
}

// TestFloatingIdle: nothing changes, nothing runs: a settled popover costs
// no frames (0 % idle CPU).
func TestFloatingIdle(t *testing.T) {
	page := func() tela.Node {
		anchor := tela.UseElementRef()
		fl := aria.UseFloating(anchor, aria.Offset(4), aria.Flip(), aria.Shift(8), aria.Size(func(a aria.Available) { a.MaxHeight(a.Height) }))
		return Div(Class("relative h-full"),
			Button(Ref(anchor), TestID("anchor"), Class("absolute left-10 top-10 h-8 w-10"), Text("M")),
			Portal(Div(fl.Props, TestID("pop"), Class("h-24 w-48 bg-white"), Textf("placement %s", fl.Placement))))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(400, 300))
	before := app.Stats()
	app.Advance(2 * time.Second)
	if after := app.Stats(); after.Frames != before.Frames {
		t.Errorf("an idle popover ran %d frames", after.Frames-before.Frames)
	}
}
