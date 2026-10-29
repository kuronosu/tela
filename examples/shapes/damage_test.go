package main

import (
	"testing"

	"github.com/kuronosu/tela/internal/testutil/damagetest"
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
		damagetest.ScrollAround(t, func(opts ...telatest.Option) *telatest.App { return telatest.Mount(t, Shapes(), opts...) }, 640, 420, scale, 8)
	}
}
