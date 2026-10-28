package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestDamage types, selects and hovers in a small window at scales 1, 1.5
// and 2, checking every frame against a full rendering (CheckDamage).
func TestDamage(t *testing.T) {
	sandbox(t)
	for _, scale := range []float64{1, 1.5, 2} {
		a := mount(t, telatest.Size(700, 420), telatest.Scale(scale), telatest.CheckDamage())
		typeNote(a, "Damage\nwords on two lines")
		a.MouseMove(60, 120)
		a.MouseMove(200, 60)
		a.Click(telatest.ByRole("button", "Welcome to Notes"))
		a.MouseMove(500, 300)
	}
}
