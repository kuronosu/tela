package telatest_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

var iconSquare = tela.MustSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16"/></svg>`))

// ByClass and ByTestID find an h.Icon without a name, which is hidden from
// accessibility (ADR-263); a named icon is found as before, and other hidden
// elements are still not matched.
func TestByClassMatchesAnIcon(t *testing.T) {
	app := telatest.Mount(t, h.Div(h.Class("flex gap-2"),
		h.Icon(iconSquare, h.Class("size-4 text-red-500"), h.TestID("deco")),
		h.Icon(iconSquare, h.Class("size-5 text-blue-500"), h.AriaLabel("Named")),
		h.Span(h.Class("size-4"), h.Text("x")),
		h.Div(h.Class("hidden"), h.Icon(iconSquare, h.Class("size-6 text-green-500"))),
		h.Div(h.AriaHidden(true), h.Icon(iconSquare, h.Class("size-7 text-pink-500")))), telatest.Size(160, 40))
	if n := app.Count(telatest.ByClass("text-red-500")); n != 1 {
		t.Errorf("ByClass(text-red-500) matched %d elements, want the decorative icon", n)
	}
	if b := app.Bounds(telatest.ByClass("size-4", "text-red-500")); b.Width != 16 || b.Height != 16 {
		t.Errorf("the icon's bounds are %v, want 16x16", b)
	}
	if b := app.Bounds(telatest.ByTestID("deco")); b.Width != 16 {
		t.Errorf("ByTestID: the icon's bounds are %v", b)
	}
	if n := app.Count(telatest.ByClass("text-blue-500")); n != 1 {
		t.Errorf("the named icon: %d matches", n)
	}
	if n := app.Count(telatest.ByClass("text-green-500")); n != 0 {
		t.Errorf("an icon in a display: none box matched (%d)", n)
	}
	if n := app.Count(telatest.ByClass("text-pink-500")); n != 0 {
		t.Errorf("an icon in an aria-hidden box matched (%d)", n)
	}
	// The icon is not in the aria snapshot: it is still decorative.
	if s := app.AriaSnapshot(); strings.Contains(s, "red") || strings.Contains(s, "deco") {
		t.Errorf("the decorative icon shows in the aria snapshot:\n%s", s)
	}
}
