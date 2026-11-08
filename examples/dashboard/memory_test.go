package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/testutil/memprobe"
	"github.com/kuronosu/tela/telatest"
)

// TestMemory has the cases of the memory measurement (ADR-240). It does
// nothing unless TELA_MEM is set; run it with
//
//	go run ./internal/testutil/memreport
//
// For each scale:
//
//   - rest: the page mounted with telatest at the size of App, settled, and
//     left at rest for core.RasterSettle, so that the host's deferred trim
//     of the rasterizer's scratch memory runs (the host's framebuffer and
//     the headless backend's copy of the presented frame).
//   - rig-rest: the page in the rig of bench_test.go at 1920x1080, settled
//     and at rest the same way (rig.rest); the rig has its own framebuffer
//     and a BGRA buffer standing in for the one a backend presents from.
//   - session: the rig at 1920x1080 through the scripted session
//     (runSession, about 1100 ticks), to compare with rig-rest. The heap is
//     sampled after every frame and marked at the end of every scene.
func TestMemory(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			t.Run("rest", func(t *testing.T) {
				memprobe.Run(t, memprobe.Case{
					Example: "dashboard", Name: fmt.Sprintf("x%g/rest", scale),
					Width: App.Width, Height: App.Height, Scale: scale,
					Run: func(p *memprobe.Probe) any {
						a := mount(t, App.Width, App.Height, telatest.Scale(scale))
						// At rest: the host's deferred trim of the
						// rasterizer's memory runs (core.RasterSettle).
						a.Advance(core.RasterSettle + 100*time.Millisecond)
						p.Sample()
						return a
					},
				})
			})
			t.Run("rig-rest", func(t *testing.T) {
				c := configs[0]
				memprobe.Run(t, memprobe.Case{
					Example: "dashboard", Name: fmt.Sprintf("x%g/rig-rest", scale),
					Width: c.w, Height: c.h, Scale: scale,
					Run: func(p *memprobe.Probe) any {
						r := newRig(t, c.w, c.h, scale, false)
						r.settle(600)
						r.rest()
						p.Sample()
						return r
					},
				})
			})
			t.Run("session", func(t *testing.T) {
				c := configs[0]
				memprobe.Run(t, memprobe.Case{
					Example: "dashboard", Name: fmt.Sprintf("x%g/session", scale),
					Width: c.w, Height: c.h, Scale: scale,
					Run: func(p *memprobe.Probe) any {
						r := newRig(t, c.w, c.h, scale, false)
						last := ""
						r.sample = func(r *rig) {
							p.Sample()
							if r.scene != last { // the end of the scene before
								p.Mark(label(last))
								last = r.scene
							}
						}
						runSession(r)
						p.Mark(label(last))
						return r
					},
				})
			})
		})
	}
}

// label names the stretch of the session that ends a scene: the first
// frames, before the script starts, have no scene.
func label(scene string) string {
	if scene == "" {
		return "settled"
	}
	return scene
}
