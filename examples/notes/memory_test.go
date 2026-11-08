package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/memprobe"
	"github.com/kuronosu/tela/telatest"
)

// TestMemory is a case of the memory measurement (ADR-240): it mounts the
// main window with the welcome notes, in a sandbox that keeps the user's
// files and settings out of it, at the size of App and each scale, and
// reports the memory it takes. It does nothing unless TELA_MEM is set; run it
// with
//
//	go run ./internal/testutil/memreport
func TestMemory(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			t.Run("rest", func(t *testing.T) {
				memprobe.Run(t, memprobe.Case{
					Example: "notes", Name: fmt.Sprintf("x%g/rest", scale),
					Width: windowW, Height: windowH, Scale: scale,
					Run: func(p *memprobe.Probe) any {
						sandbox(t)
						// The first frames are drawn and presented when mount returns.
						a := mount(t, telatest.Scale(scale))
						p.Sample()
						return a
					},
				})
			})
		})
	}
}
