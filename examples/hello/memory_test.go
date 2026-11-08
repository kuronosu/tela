package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/memprobe"
	"github.com/kuronosu/tela/telatest"
)

// TestMemory is a case of the memory measurement (ADR-240): it mounts the
// screen at its App size and each scale and reports the memory it takes. It
// does nothing unless TELA_MEM is set; run it with
//
//	go run ./internal/testutil/memreport
func TestMemory(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			t.Run("rest", func(t *testing.T) {
				memprobe.Run(t, memprobe.Case{
					Example: "hello", Name: fmt.Sprintf("x%g/rest", scale),
					Width: 800, Height: 450, Scale: scale,
					Run: func(p *memprobe.Probe) any {
						// The window's first frames are drawn and presented
						// when Mount returns.
						a := mount(t, telatest.Scale(scale))
						p.Sample()
						return a
					},
				})
			})
		})
	}
}
