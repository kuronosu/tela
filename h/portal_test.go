package h_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// TestPortalThroughThePublicAPI: a dialog opened through a Portal reads
// the context of its owner, its clicks bubble to the owner, it is above
// the owner's z-index and outside its clip, and it closes.
func TestPortalThroughThePublicAPI(t *testing.T) {
	theme := tela.CreateContext("plain")
	bubbled := 0
	dialog := func() tela.Node {
		return Div(Role("dialog"), AriaLabel(tela.UseContext(theme)),
			Class("fixed top-4 left-4 h-20 w-40 bg-white shadow-xl"),
			Button(Text("Close")))
	}
	page := func() tela.Node {
		open, setOpen := tela.UseState(false)
		return theme.Provider("fancy",
			Div(Class("relative z-50 h-10 w-20 overflow-hidden"),
				OnClick(func(tela.MouseEvent) { bubbled++ }),
				Button(Text("Open"), OnClick(func(tela.MouseEvent) { setOpen.Set(!open) })),
				If(open, Portal(tela.C(dialog))),
			))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(300, 200))
	app.Click(telatest.ByRole("button", "Open"))
	app.ExpectAria(`
		- button "Open"
		- dialog "fancy":
		  - button "Close"
	`)
	before := bubbled
	app.Click(telatest.ByRole("button", "Close"))
	if bubbled != before+1 {
		t.Errorf("the click in the portal did not bubble to its owner (%d, %d)", before, bubbled)
	}
	app.Click(telatest.ByRole("button", "Open"))
	app.ExpectAria(`- button "Open"`)
}
