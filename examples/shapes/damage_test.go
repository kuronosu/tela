package main

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// TestScrollDamage scrolls the shapes page (blurred glows, backdrop
// filters, gradients) in a small window, checking every frame against a
// full rendering.
func TestScrollDamage(t *testing.T) {
	if testing.Short() {
		t.Skip("full frames of blurs")
	}
	for _, scale := range []float64{1} {
		scrollAround(t, Shapes(), 640, 420, scale, 8)
	}
}

// scrollAround opens root and checks every frame against a full rendering
// (telatest.CheckDamage) while the pointer moves over the screen and the
// arrow and page keys scroll it down and back up (instantly, so that each
// key is one frame).
func scrollAround(t *testing.T, root tela.Component, w, h float32, scale float64, steps int) {
	t.Helper()
	a := telatest.Mount(t, root, telatest.Size(w, h), telatest.Scale(scale), telatest.InstantScroll(), telatest.CheckDamage())
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
