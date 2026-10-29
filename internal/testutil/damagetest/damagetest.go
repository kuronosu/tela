// Package damagetest is the scroll-and-hover damage check shared by the
// tests of the examples. It imports telatest, so only tests of packages
// above telatest (the examples) may use it.
package damagetest

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// ScrollAround opens an app with mount and checks every frame against a
// full rendering (telatest.CheckDamage) while the pointer moves over the
// screen and the arrow and page keys scroll it down and back up (instantly,
// so that each key is one frame). mount receives the options to pass to
// telatest.Mount or telatest.MountApp: the size w by h, the scale, instant
// scrolling and the damage check.
func ScrollAround(t *testing.T, mount func(opts ...telatest.Option) *telatest.App, w, h float32, scale float64, steps int) {
	t.Helper()
	a := mount(telatest.Size(w, h), telatest.Scale(scale), telatest.InstantScroll(), telatest.CheckDamage())
	first := string(a.Screenshot().Pix)
	for i := range steps {
		if i == steps/2 && string(a.Screenshot().Pix) == first {
			t.Errorf("the page did not scroll")
		}
		f := float32(i) / float32(steps)
		a.MouseMove(w*(0.2+0.6*f), h*(0.3+0.2*float32(i%3)))
		down := i < steps/2
		switch {
		case i%6 == 5 && down:
			a.Press("PageDown")
		case i%6 == 5:
			a.Press("PageUp")
		case down:
			a.Press("ArrowDown")
		default:
			a.Press("ArrowUp")
		}
	}
}
