//go:build telagpu && (linux || windows)

package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/gfx/gpu/gputest"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The dashboard drawn by the GPU (ADR-281, ADR-282): the rig of
// bench_test.go with core's GPU frame in the place of the CPU rasterizer
// (Tree.RasterizeFrameGPU: the damage drawn on the device and read back
// into the framebuffer), on every physical device.
//
//	go test -tags telagpu -run TestSessionGPU ./examples/dashboard                           # the frames against the CPU rasterizer's
//	TELA_PERF=1 go test -tags telagpu -run TestSessionPerfGPU -v ./examples/dashboard         # the tables of the measurement

// gpuRenderer is the rig's renderer on a device.
type gpuRenderer struct {
	tb   testing.TB
	g    *core.GPU
	surf *core.GPUSurface
	// noTrim keeps the renderer's memory at rest (a measurement of what
	// the trim costs the frame after it).
	noTrim bool
}

// useGPU makes the GPU of dev draw the rig's frames. The caller closes the
// renderer before the device.
func useGPU(tb testing.TB, r *rig, dev gpu.Device) *gpuRenderer {
	g := core.NewGPU(dev)
	p := &gpuRenderer{tb: tb, g: g, surf: g.NewSurface()}
	r.renderer = p
	return p
}

func (p *gpuRenderer) close() {
	p.surf.Release()
	p.g.Close()
}

func (p *gpuRenderer) rasterize(r *rig) (core.Damage, [numGPUParts]time.Duration, int) {
	d, err := r.tree.RasterizeFrameGPU(r.fb, p.surf, &r.rr, false)
	if err != nil {
		p.tb.Fatalf("the GPU failed (the frame was drawn by the CPU rasterizer): %v", err)
	}
	st := p.surf.Stats
	return d, [numGPUParts]time.Duration{gpuEncode: st.Encode, gpuDraw: st.Draw, gpuShift: st.Shift, gpuRead: st.Read}, st.Renders
}

// trimAt is what core.Host does for its GPU: a trim RasterSettle after a
// frame that left the page at rest.
func (p *gpuRenderer) trimAt(r *rig) time.Duration {
	if !p.noTrim && p.g.TrimDue() && !r.asked && !r.tree.Animating() && !r.tree.Dirty() {
		return r.now + core.RasterSettle
	}
	return 0
}

func (p *gpuRenderer) trim(*rig) { p.g.Trim() }

// savePNG writes img for a failure's report.
func savePNG(t *testing.T, dir, name string, img *image.RGBA) {
	t.Helper()
	golden.WritePNG(t, filepath.Join(dir, name), img)
}

// TestSessionGPU plays the scripted session of bench_test.go with the GPU
// drawing its frames, incrementally, at scales 1 and 2, and compares the
// framebuffer after a frame with
//
//   - a full frame of the GPU, which it must equal bit for bit (the damage
//     and the readback are right);
//   - a full rasterization of the same frame by the CPU rasterizer, within
//     the tolerance of ADR-282 (at most 0.5% of the pixels over 2 levels,
//     none over 24): half of the GPU backend's criterion, "GPU snapshots
//     stay within the agreed tolerance of the CPU goldens".
//
// A CPU frame of the page takes a tenth of a second at scale 2, so the
// frames compared are every full frame and, of the others, one in two at
// scale 1 and one in five at scale 2 (a wrong pixel stays in the
// framebuffer until it is drawn again, so the frames after it show it);
// every frame with TELA_CHECK_DAMAGE=1.
func TestSessionGPU(t *testing.T) {
	gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
				const w, h = 1440, 900
				r := newRig(t, w, h, scale, false)
				p := useGPU(t, r, dev)
				defer p.close()
				ref := p.g.NewSurface()
				defer ref.Release()
				every := 2
				if scale > 1 {
					every = 5
				}
				if checkEveryScale {
					every = 1
				}
				var worst gfxtest.Diff
				var over2 float64
				n, compared, full := 0, 0, 0
				r.checked = func(r *rig, d core.Damage) {
					n++
					if d.Full {
						full++
					}
					if !d.Full && n%every != 0 {
						return
					}
					compared++
					fail := func(name string, want *image.RGBA) string {
						dir := os.Getenv("TELA_DAMAGE_DIR")
						if dir == "" {
							dir = t.TempDir()
						}
						savePNG(t, dir, "gpu.png", r.fb)
						savePNG(t, dir, name, want)
						return dir
					}
					whole := image.NewRGBA(r.fb.Rect)
					if err := ref.DrawFull(r.tree, whole); err != nil {
						t.Fatal(err)
					}
					if m := gfxtest.Compare(r.fb, whole); m.Max != 0 {
						t.Fatalf("scene %q, tick %d: the frame differs from a full frame of the GPU in %d pixels, the first at %v (damage %+v); images in %s",
							r.scene, r.ticks, m.Over0, m.Worst, d, fail("gpu_full.png", whole))
					}
					cpu := r.tree.Rasterize(r.tree.LastFrame(), float32(scale))
					m := gfxtest.Compare(r.fb, cpu)
					if m.Max > worst.Max {
						worst = m
					}
					over2 = max(over2, m.Over2)
					if !m.WithinGPU() {
						t.Errorf("scene %q, tick %d: against the CPU rasterizer the largest difference is %d at %v (limit %d) and %.3f%% of the pixels differ by more than %d levels (limit %.3f%%); images in %s",
							r.scene, r.ticks, m.Max, m.Worst, gfxtest.GPUHard, 100*m.Over2, gfxtest.GPULevels, 100*gfxtest.GPUShare, fail("cpu.png", cpu))
					}
				}
				frames, _ := runSession(r)
				if len(frames) < 450 {
					t.Errorf("the session drew %d frames, want at least 450", len(frames))
				}
				if compared < len(frames)/every/2 || full < 3 {
					t.Errorf("%d frames compared of %d (%d full)", compared, n, full)
				}
				t.Logf("%s, scale %g: %d frames compared of %d (%d full); against the CPU rasterizer at most %d levels (at %v), at most %.3f%% of a frame over 2 levels",
					dev.Name(), scale, compared, n, full, worst.Max, worst.Worst, 100*over2)
			})
		}
	})
}

// typing is a scene after the session for the measurement: the palette
// opens and a query is typed into its field, a letter every few ticks,
// then erased.
func typing() scene {
	const query = "orders"
	key := func(r *rig, k platform.Key, text string) func() {
		return func() {
			r.tree.HandleKey(platform.KeyEvent{Action: platform.KeyPress, Key: k, Text: text, Time: r.now})
			r.tree.HandleKey(platform.KeyEvent{Action: platform.KeyRelease, Key: k, Time: r.now})
		}
	}
	return scene{"typing", 140, func(r *rig, i int) []func() {
		switch {
		case i == 0:
			return r.click("button", "Search")
		case i >= 30 && i < 30+6*len(query) && i%6 == 0:
			c := string(query[(i-30)/6])
			return []func(){key(r, platform.Key(c), c)}
		case i >= 80 && i < 80+4*len(query) && i%4 == 0:
			return []func(){key(r, platform.KeyBackspace, "")}
		case i == 115:
			return []func(){key(r, platform.KeyEscape, "")}
		}
		return nil
	}}
}

// playPerf plays the session and the typing scene on r and returns the
// frames.
func playPerf(r *rig) []frameStat {
	frames, _ := runSession(r)
	sc := typing()
	r.scene = sc.name
	for i := 0; i < sc.ticks; i++ {
		r.tick(&frames, sc.on(r, i)...)
	}
	return frames
}

// perfTable formats the frames of a session by kind: the scenes of the
// script, then the full frames of all of them, then every frame. For each
// kind: the frames, how many took more than a frame interval, the p50,
// p95 and largest frame time, and the mean of each part of a frame, in ms:
// the pipeline (input, animations, build to paint, effects), the damage
// and what else the raster phase does outside the renderer, the encoder,
// the device's draws, its shifts, the readback, and the copy to the buffer
// a backend presents from; then the device pixels a frame draws and its
// renders (one per damaged rectangle). A frame of the CPU rasterizer has
// its time under "raster".
func perfTable(label string, fs []frameStat) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n== %s: %d frames, GOMAXPROCS=%d\n", label, len(fs), runtime.GOMAXPROCS(0))
	fmt.Fprintf(&sb, "%-18s %5s %5s %7s %7s %7s | %8s %7s %7s %7s %7s %7s %7s %7s %7s\n", "kind", "n", ">16.7", "p50", "p95", "max",
		"pipeline", "raster", "encode", "draw", "shift", "read", "present", "Mpx/f", "rects/f")
	row := func(name string, g []frameStat) {
		if len(g) == 0 {
			return
		}
		d := distOf(totals(g))
		over := 0
		var pipe, raster, present time.Duration
		var parts [numGPUParts]time.Duration
		px, renders := 0, 0
		for _, f := range g {
			renders += f.renders
			if f.total > frameInterval {
				over++
			}
			pipe += f.phase[phInput] + f.phase[phAnimate] + f.phase[phPipeline] + f.phase[phEffects]
			rest := f.phase[phRaster]
			for k, v := range f.gpu {
				parts[k] += v
				rest -= v
			}
			raster += rest
			present += f.phase[phPresent]
			px += f.pixels
		}
		n := float64(len(g))
		fmt.Fprintf(&sb, "%-18s %5d %5d %7.2f %7.2f %7.2f | %8.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f\n", name, len(g), over, d.P50, d.P95, d.Max,
			ms(pipe)/n, ms(raster)/n, ms(parts[gpuEncode])/n, ms(parts[gpuDraw])/n, ms(parts[gpuShift])/n, ms(parts[gpuRead])/n, ms(present)/n, float64(px)/n/1e6, float64(renders)/n)
	}
	var order []string
	byScene := map[string][]frameStat{}
	var full []frameStat
	for _, f := range fs {
		if _, ok := byScene[f.scene]; !ok {
			order = append(order, f.scene)
		}
		byScene[f.scene] = append(byScene[f.scene], f)
		if f.full {
			full = append(full, f)
		}
	}
	for _, name := range order {
		row(name, byScene[name])
	}
	row("(full frames)", full)
	row("(all)", fs)
	return sb.String()
}

// perfConfigs are the sizes of the measurement: 2560x1440 at scale 1, and
// 3840x2160 at scale 2.
var perfConfigs = []struct {
	name  string
	w, h  float32
	scale float64
}{
	{"2560x1440_x1", 2560, 1440, 1},
	{"3840x2160_x2", 1920, 1080, 2},
}

// TestSessionPerfGPU measures the session, and a scene of typing after
// it, drawn by the CPU rasterizer and by the GPU of every device, and logs
// a table for each (go test -v). It only measures: set TELA_PERF=1 to play
// it. TELA_PERF_CONFIG selects one configuration by name, and
// TELA_PERF_NOTRIM=1 keeps the GPU renderer's memory at rest, to see what
// the trim (core.RasterSettle) costs the frames after a pause.
func TestSessionPerfGPU(t *testing.T) {
	if os.Getenv("TELA_PERF") == "" {
		t.Skip("a measurement only: set TELA_PERF=1 to play it")
	}
	for _, c := range perfConfigs {
		if only := os.Getenv("TELA_PERF_CONFIG"); only != "" && only != c.name {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			t.Run("cpu", func(t *testing.T) {
				r := newRig(t, c.w, c.h, c.scale, false)
				t.Log(perfTable(c.name+", CPU rasterizer", playPerf(r)))
			})
			gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
				r := newRig(t, c.w, c.h, c.scale, false)
				p := useGPU(t, r, dev)
				defer p.close()
				label := c.name + ", GPU with readback, " + dev.Name()
				if p.noTrim = os.Getenv("TELA_PERF_NOTRIM") != ""; p.noTrim {
					label += ", no trim at rest"
				}
				t.Log(perfTable(label, playPerf(r)))
			})
		})
	}
}
