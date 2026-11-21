package telatest_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// A primary press at a point with nothing under it (the empty area of an
// h-full page) blurs the focused element, as a mousedown that hits no
// focusable ancestor does in Chromium. It is a regression test: after
// frames had run, the focused element still reported focused=true (found in
// phase 22) until the merge 2e8de0b4 (bisected; it brought ADR-277's
// incremental semantics trees).
func TestPressOnEmptyAreaBlurs(t *testing.T) {
	for _, c := range []struct {
		name  string
		class string
	}{
		{"h-full page", "h-full"},
		{"padded page", "h-full p-4"},
		{"plain page", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			var rec recorder
			app := telatest.MountFunc(t, func() tela.Node {
				n, setN := tela.UseState(0)
				return Div(Class(c.class),
					Button(Class("px-4 py-2"), TestID("b"), OnClick(func() { setN.Set(n + 1) }),
						OnFocus(func(tela.FocusEvent) { rec.add("focus") }),
						OnBlur(func(tela.FocusEvent) { rec.add("blur") }), Textf("Count %d", n)))
			}, telatest.Size(300, 200))
			btn := telatest.ByTestID("b")
			app.Click(btn) // frames have run, the button is focused
			app.ExpectFocused(btn)
			app.ExpectText("Count 1")
			rec.take()
			app.ClickAt(250, 180) // nothing is there
			app.Expect(btn, telatest.Focused(false))
			if got := rec.take(); got != "blur" {
				t.Errorf("events after a press on the empty area: %q, want blur", got)
			}
		})
	}
}
