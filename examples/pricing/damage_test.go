package main

import (
	"testing"

	"github.com/kuronosu/tela/internal/testutil/damagetest"
	"github.com/kuronosu/tela/telatest"
)

// TestScrollDamage scrolls and hovers the pricing page in a small window
// at scales 1, 1.5 and 2, checking every frame against a full rendering.
func TestScrollDamage(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		damagetest.ScrollAround(t, func(opts ...telatest.Option) *telatest.App { return telatest.Mount(t, Pricing(), opts...) }, 700, 420, scale, 24)
	}
}
