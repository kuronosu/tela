package style

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// testCx is the context of most tests: default theme, a 1280x800 window.
var testCx = &Context{Viewport: gfx.Size{W: 1280, H: 800}}

// mustCompile compiles classes and fails the test on any class error.
func mustCompile(t testing.TB, classes string) *Classes {
	t.Helper()
	c := Compile(classes)
	if c.Err != nil {
		t.Fatalf("Compile(%q): %v", classes, c.Err)
	}
	return c
}

// style computes the style of a Div with the classes under the root style.
func style(t testing.TB, classes string) *ComputedStyle {
	t.Helper()
	return styleIn(t, testCx, KindDiv, nil, classes)
}

func styleIn(t testing.TB, cx *Context, kind Kind, parent *ComputedStyle, classes string, props ...Op) *ComputedStyle {
	t.Helper()
	return Compute(cx, Input{Kind: kind, Parent: parent, Classes: []*Classes{mustCompile(t, classes)}, Props: props})
}

// near reports whether a and b differ by at most eps.
func near(a, b, eps float32) bool {
	d := a - b
	return d <= eps && -d <= eps
}

func colorNear(a, b gfx.Color, eps float32) bool {
	return near(a.R, b.R, eps) && near(a.G, b.G, eps) && near(a.B, b.B, eps) && near(a.A, b.A, eps)
}
