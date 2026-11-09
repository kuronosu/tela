package telatest_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Click acts as a user's press (ADR-263): an element with pointer-events-none
// does not receive it, so the click fails like Playwright's "element does not
// receive pointer events", whether the pointer lands on what is behind it or
// on an ancestor, and it names the cause.
func TestClickFailsOnPointerEventsNone(t *testing.T) {
	clicks := 0
	press := h.OnClick(func() { clicks++ })
	for name, root := range map[string]func() tela.Node{
		"alone": func() tela.Node {
			return h.Button(h.Class("pointer-events-none h-10 w-32"), press, h.Text("Go"))
		},
		"in a container": func() tela.Node {
			return h.Div(h.Class("p-4"), h.Button(h.Class("pointer-events-none"), press, h.Text("Go")))
		},
		"inherited": func() tela.Node {
			return h.Div(h.Class("pointer-events-none p-4"), h.Button(press, h.Text("Go")))
		},
	} {
		mustFail(t, func(tb testing.TB) {
			app := telatest.Mount(tb, tela.C(root), telatest.Size(200, 100), telatest.Timeout(100*time.Millisecond))
			app.Click(telatest.ByRole("button", "Go"))
		}, `cannot click getByRole("button", name="Go")`, `button "Go" has pointer-events-none`, "a user cannot click it")
		mustFail(t, func(tb testing.TB) {
			app := telatest.Mount(tb, tela.C(root), telatest.Size(200, 100), telatest.Timeout(100*time.Millisecond))
			app.Tap(telatest.ByRole("button", "Go"))
		}, `cannot tap`, "has pointer-events-none")
		_ = name
	}
	if clicks != 0 {
		t.Errorf("OnClick ran %d times", clicks)
	}
}

// A pointer-events-none element whose child takes the pointer is clicked
// through the child, as in a browser; ClickAt over a pointer-events-none
// element does nothing.
func TestClickReachesAPointerEventsAutoChild(t *testing.T) {
	clicks := 0
	app := telatest.Mount(t, h.Div(h.Class("p-4"), h.Button(h.Class("pointer-events-none"), h.OnClick(func() { clicks++ }),
		h.Span(h.Class("pointer-events-auto"), h.Text("Go")))), telatest.Size(200, 100))
	app.Click(telatest.ByRole("button", "Go"))
	if clicks != 1 {
		t.Errorf("a click through the pointer-events-auto child ran OnClick %d times, want 1", clicks)
	}
	r := app.Bounds(telatest.ByRole("button", "Go"))
	app2 := telatest.Mount(t, h.Div(h.Class("p-4"), h.Button(h.Class("pointer-events-none"), h.OnClick(func() { clicks++ }), h.Text("Go"))), telatest.Size(200, 100))
	app2.ClickAt(r.X+2, r.Y+2)
	if clicks != 1 {
		t.Errorf("ClickAt over a pointer-events-none button ran OnClick (%d clicks)", clicks)
	}
}
