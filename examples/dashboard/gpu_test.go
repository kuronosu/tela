//go:build telagpu && (linux || windows)

package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	// chain, when not nil, presents the frames without a copy to win, a
	// display server of the test's own (useDirect).
	chain *core.GPUSwapchain
	win   *perfWindow
	// prof, when not nil, is the device measuring where its time goes
	// (TELA_GPU_PROFILE=1), by kind of frame.
	prof         gpu.Profiler
	profiles     map[string]*profileGroup
	profileOrder []string
	// peak is the most the Renderer held on the device after a frame, and
	// rest what it held after each trim at rest (the last one).
	peak, rest gpu.Memory
	rests      int
}

// useGPU makes the GPU of dev draw the rig's frames. The caller closes the
// renderer before the device.
func useGPU(tb testing.TB, r *rig, dev gpu.Device) *gpuRenderer {
	g := core.NewGPU(dev)
	p := &gpuRenderer{tb: tb, g: g, surf: g.NewSurface()}
	if prof, ok := dev.(gpu.Profiler); ok && os.Getenv("TELA_GPU_PROFILE") != "" {
		prof.SetProfiling(true)
		prof.TakeProfile()
		p.prof, p.profiles = prof, map[string]*profileGroup{}
	}
	r.renderer = p
	return p
}

// useDirect is useGPU with the frames presented without a copy, as a Host
// presents them in a window that shows dmabufs: the frame stays on the
// device, and a swapchain copies what changed into a buffer the window is
// given. hold is the number of presented buffers the display server keeps
// busy (perfWindow). ok is false on a device that exports no dmabufs.
func useDirect(tb testing.TB, r *rig, dev gpu.Device, hold int) (p *gpuRenderer, ok bool) {
	exp, isExp := dev.(gpu.Exporter)
	if !isExp || len(exp.ExportFormats()) == 0 {
		return nil, false
	}
	p = useGPU(tb, r, dev)
	p.win = &perfWindow{size: r.fb.Rect.Size(), hold: hold, feedback: platform.DmabufFeedback{Serial: 1}}
	for _, f := range exp.ExportFormats() {
		if f.Fourcc == gpu.FourccXRGB8888 { // what an X server takes, and every Wayland compositor
			p.win.feedback.Formats = append(p.win.feedback.Formats, platform.DmabufFormat{Fourcc: f.Fourcc, Modifier: f.Modifier})
		}
	}
	p.chain = core.NewGPUSwapchain(p.g, p.win)
	p.surf.SetDirect(true)
	r.direct = true
	return p, true
}

func (p *gpuRenderer) close() {
	if p.prof != nil {
		p.prof.SetProfiling(false)
	}
	if p.chain != nil {
		p.chain.Release()
	}
	p.surf.Release()
	p.g.Close()
}

func (p *gpuRenderer) rasterize(r *rig, fs *frameStat) core.Damage {
	d, err := r.tree.RasterizeFrameGPU(r.fb, p.surf, &r.rr, false)
	if err != nil {
		p.tb.Fatalf("the GPU failed (the frame was drawn by the CPU rasterizer): %v", err)
	}
	st := p.surf.Stats
	parts := &fs.gpu
	*parts = [numGPUParts]time.Duration{gpuEncode: st.Encode, gpuDraw: st.Draw, gpuShift: st.Shift, gpuRead: st.Read}
	fs.renders, fs.instances, fs.draws, fs.passes, fs.uploads, fs.uploadBytes = st.Renders, st.Instances, st.Draws, st.Passes, st.Uploads, st.UploadBytes
	fs.culled, fs.glass = st.Culled, [4]int{st.Backdrops.Hits, st.Backdrops.Patched, st.Backdrops.Stored, st.Backdrops.Computed}
	if p.prof != nil {
		p.profile(r.scene, d.Full)
	}
	if m := p.g.Memory(); m.Total() > p.peak.Total() {
		p.peak = m
	}
	if p.chain != nil {
		if ok, why, _ := p.chain.Ready(false); !ok {
			p.tb.Fatalf("the swapchain is not ready: %s", why)
		}
		res, err := p.chain.Present(p.surf, d, d.Full)
		if err != nil || res != core.GPUPresentShown {
			p.tb.Fatalf("presenting without a copy: result %d, %v", res, err)
		}
		parts[gpuCopy] = p.chain.Stats.Copy
	}
	return d
}

// profile adds what the device measured of the frame just drawn to the
// tables of its scene, of the full or partial frames, and of all.
func (p *gpuRenderer) profile(scene string, full bool) {
	rows := p.prof.TakeProfile()
	kind := "(partial frames)"
	if full {
		kind = "(full frames)"
	}
	for _, name := range []string{scene, kind, "(all)"} {
		g := p.profiles[name]
		if g == nil {
			g = &profileGroup{rows: map[string]*gpu.ProfileRow{}}
			p.profiles[name] = g
			p.profileOrder = append(p.profileOrder, name)
		}
		g.frames++
		for _, r := range rows {
			a := g.rows[r.Class]
			if a == nil {
				a = &gpu.ProfileRow{Class: r.Class}
				g.rows[r.Class] = a
			}
			a.Runs += r.Runs
			a.Pixels += r.Pixels
			a.Texels += r.Texels
			a.Time += r.Time
		}
	}
}

// profileGroup is the device's measurement of the frames of one kind.
type profileGroup struct {
	frames int
	rows   map[string]*gpu.ProfileRow
}

// profileTable formats the device's measurement (TELA_GPU_PROFILE=1): for
// each kind of frame, the classes of what the device drew by the time they
// took, per frame: the draws, the megapixels, the milliseconds, and the
// fill rate that makes (gigapixels per second; for a filter, also the
// gigatexels it read per second).
func (p *gpuRenderer) profileTable() string {
	var sb strings.Builder
	for _, name := range p.profileOrder {
		g := p.profiles[name]
		n := float64(g.frames)
		rows := make([]*gpu.ProfileRow, 0, len(g.rows))
		var sum time.Duration
		for _, r := range g.rows {
			rows = append(rows, r)
			if !strings.HasPrefix(r.Class, "(") {
				sum += r.Time
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Time > rows[j].Time })
		fmt.Fprintf(&sb, "\n-- %s: %d frames, classes %.2f ms/frame\n%-44s %8s %9s %9s %8s %8s\n", name, g.frames, ms(sum)/n, "class", "draws/f", "Mpx/f", "ms/f", "Gpx/s", "Gtx/s")
		for _, r := range rows {
			if ms(r.Time)/n < 0.01 && r.Pixels == 0 {
				continue
			}
			rate, trate := 0.0, 0.0
			if r.Time > 0 {
				rate = float64(r.Pixels) / r.Time.Seconds() / 1e9
				trate = float64(r.Texels) / r.Time.Seconds() / 1e9
			}
			fmt.Fprintf(&sb, "%-44s %8.1f %9.3f %9.3f %8.2f %8.2f\n", r.Class, float64(r.Runs)/n, float64(r.Pixels)/n/1e6, ms(r.Time)/n, rate, trate)
		}
	}
	return sb.String()
}

// perfWindow is the display server of the measurement of frames presented
// without a copy: a platform.DmabufPresenter that takes XRGB8888 with the
// device's modifiers, registers what it is given and keeps the hold
// latest buffers presented busy (1: a Wayland compositor releases a
// buffer when the next is committed; 2: an X server that flips). It costs
// nothing, as registering (once per buffer) and presenting (a request on
// a socket) cost next to nothing in a real one: the frame's cost is the
// copy into the buffer and the wait for the device, which are real here.
type perfWindow struct {
	size     image.Point
	hold     int
	feedback platform.DmabufFeedback
	held     []*perfBuffer
	created  int
}

type perfBuffer struct {
	size image.Point
	busy bool
}

func (b *perfBuffer) Size() image.Point { return b.size }
func (b *perfBuffer) Busy() bool        { return b.busy }
func (b *perfBuffer) Destroy()          { b.busy = false }

func (w *perfWindow) DmabufFeedback() (platform.DmabufFeedback, bool) { return w.feedback, true }

func (w *perfWindow) CreateDmabuf(d platform.DmabufDescriptor) (platform.DmabufBuffer, error) {
	platform.CheckDmabuf(d)
	w.created++
	return &perfBuffer{size: d.Size}, nil
}

func (w *perfWindow) PresentDmabuf(pb platform.DmabufBuffer, damage []image.Rectangle) error {
	b := pb.(*perfBuffer)
	if b.busy || b.size != w.size {
		panic("the swapchain presented a busy buffer, or one of another size")
	}
	b.busy = true
	w.held = append(w.held, b)
	for len(w.held) > w.hold {
		w.held[0].busy = false
		w.held = w.held[1:]
	}
	return nil
}

// trimAt is what core.Host does for its GPU: a trim RasterSettle after a
// frame that left the page at rest.
func (p *gpuRenderer) trimAt(r *rig) time.Duration {
	if !p.noTrim && p.g.TrimDue() && !r.asked && !r.tree.Animating() && !r.tree.Dirty() {
		return r.now + core.RasterSettle
	}
	return 0
}

func (p *gpuRenderer) trim(r *rig) {
	p.g.Trim(r.tree.LastFrame())
	if p.chain != nil {
		p.chain.Trim()
	}
	p.rest = p.g.Memory()
	p.rests++
}

// memoryLine describes what the Renderer held on the device: its peak
// after a frame and what it kept after the last trim at rest, in MiB (the
// window's target and the swapchain's buffers apart).
func (p *gpuRenderer) memoryLine() string {
	mib := func(n int64) float64 { return float64(n) / (1 << 20) }
	f := func(m gpu.Memory) string {
		return fmt.Sprintf("%.1f MiB (atlases %.1f, images %.1f, targets %.1f, backdrops %.1f)", mib(m.Total()), mib(m.Atlases), mib(m.Images), mib(m.Targets), mib(m.Backdrops))
	}
	return fmt.Sprintf("renderer memory: peak %s; at rest after %d trims %s", f(p.peak), p.rests, f(p.rest))
}

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
// the device's draws, its shifts, the readback, the copy on the device
// into the buffer of a window presented without a copy ("copy"), and the
// copy in main memory to the buffer a backend presents from ("present");
// then the device pixels a frame draws and its renders (one per damaged
// rectangle). A frame of the CPU rasterizer has its time under "raster".
func perfTable(label string, fs []frameStat) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n== %s: %d frames, GOMAXPROCS=%d\n", label, len(fs), runtime.GOMAXPROCS(0))
	fmt.Fprintf(&sb, "%-18s %5s %5s %7s %7s %7s | %8s %7s %7s %7s %7s %7s %7s %7s %7s %7s\n", "kind", "n", ">16.7", "p50", "p95", "max",
		"pipeline", "raster", "encode", "draw", "shift", "read", "copy", "present", "Mpx/f", "rects/f")
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
		fmt.Fprintf(&sb, "%-18s %5d %5d %7.2f %7.2f %7.2f | %8.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f\n", name, len(g), over, d.P50, d.P95, d.Max,
			ms(pipe)/n, ms(raster)/n, ms(parts[gpuEncode])/n, ms(parts[gpuDraw])/n, ms(parts[gpuShift])/n, ms(parts[gpuRead])/n, ms(parts[gpuCopy])/n, ms(present)/n, float64(px)/n/1e6, float64(renders)/n)
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

// overTable lists the frames of a session that took more than a frame
// interval, with the time of each part in ms, when there are few enough
// to read: where the frames that miss 60 fps spend their time.
func overTable(fs []frameStat) string {
	var sb strings.Builder
	n := 0
	for _, f := range fs {
		if f.total > frameInterval {
			n++
		}
	}
	// TELA_PERF_FRAMES lists every frame of the scenes it names (or of all
	// with "all") instead.
	list := os.Getenv("TELA_PERF_FRAMES")
	listed := func(f *frameStat) bool {
		if list != "" {
			return list == "all" || strings.Contains(list, f.scene)
		}
		return f.total > frameInterval
	}
	if list == "" && (n == 0 || n > 40) {
		return fmt.Sprintf("%d frames over %.1f ms", n, ms(frameInterval))
	}
	fmt.Fprintf(&sb, "\n%d frames over %.1f ms; the frames listed:\n%-18s %5s %5s %7s | %8s %7s %7s %7s %7s %7s %7s %7s %6s %6s %6s %6s %9s %6s %s\n", n, ms(frameInterval),
		"scene", "tick", "full", "total", "pipeline", "raster", "encode", "draw", "shift", "read", "copy", "Mpx", "rects", "inst", "passes", "upl", "upl bytes", "hidden", "glass h/p/s/c")
	for i := range fs {
		f := &fs[i]
		if !listed(f) {
			continue
		}
		pipe := f.phase[phInput] + f.phase[phAnimate] + f.phase[phPipeline] + f.phase[phEffects]
		rest := f.phase[phRaster]
		for _, v := range f.gpu {
			rest -= v
		}
		fmt.Fprintf(&sb, "%-18s %5d %5v %7.2f | %8.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %7.2f %6d %6d %6d %6d %9d %6d %d/%d/%d/%d\n", f.scene, f.tick, f.full, ms(f.total),
			ms(pipe), ms(rest), ms(f.gpu[gpuEncode]), ms(f.gpu[gpuDraw]), ms(f.gpu[gpuShift]), ms(f.gpu[gpuRead]), ms(f.gpu[gpuCopy]), float64(f.pixels)/1e6, f.renders,
			f.instances, f.passes, f.uploads, f.uploadBytes, f.culled, f.glass[0], f.glass[1], f.glass[2], f.glass[3])
	}
	return sb.String()
}

// perfConfigs are the sizes of the measurement: 2560x1440 at scale 1, the
// dashboard's own window at scale 2 (2880x1800, the size of the GPU
// backend's criterion), and 3840x2160 at scale 2.
var perfConfigs = []struct {
	name  string
	w, h  float32
	scale float64
}{
	{"2560x1440_x1", 2560, 1440, 1},
	{"2880x1800_x2", 1440, 900, 2},
	{"3840x2160_x2", 1920, 1080, 2},
}

// TestSessionPerfGPU measures the session, and a scene of typing after
// it, drawn by the CPU rasterizer and by the GPU of every device, read
// back into the framebuffer and presented without a copy (useDirect), and
// logs a table for each (go test -v). It only measures: set TELA_PERF=1
// to play it. TELA_PERF_CONFIG selects one configuration by name,
// TELA_PERF_PATH one way of presenting (cpu, copy or dmabuf),
// TELA_PERF_HOLD=2 makes the display server of the frames presented
// without a copy keep two buffers busy, as an X server that flips (three
// buffers, each two frames behind), and TELA_PERF_NOTRIM=1 keeps the GPU
// renderer's memory at rest, to see what the trim (core.RasterSettle)
// costs the frames after a pause.
func TestSessionPerfGPU(t *testing.T) {
	if os.Getenv("TELA_PERF") == "" {
		t.Skip("a measurement only: set TELA_PERF=1 to play it")
	}
	path := os.Getenv("TELA_PERF_PATH")
	hold := 1
	if os.Getenv("TELA_PERF_HOLD") == "2" {
		hold = 2
	}
	for _, c := range perfConfigs {
		if only := os.Getenv("TELA_PERF_CONFIG"); only != "" && only != c.name {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			if path == "" || path == "cpu" {
				t.Run("cpu", func(t *testing.T) {
					r := newRig(t, c.w, c.h, c.scale, false)
					t.Log(perfTable(c.name+", CPU rasterizer", playPerf(r)))
				})
			}
			gputest.EachDevice(t, func(t *testing.T, dev gpu.Device) {
				noTrim := os.Getenv("TELA_PERF_NOTRIM") != ""
				suffix := ""
				if noTrim {
					suffix = ", no trim at rest"
				}
				if path == "" || path == "copy" {
					r := newRig(t, c.w, c.h, c.scale, false)
					p := useGPU(t, r, dev)
					p.noTrim = noTrim
					t.Log(perfTable(c.name+", GPU with readback, "+dev.Name()+suffix, playPerf(r)))
					p.close()
				}
				if path == "" || path == "dmabuf" {
					r := newRig(t, c.w, c.h, c.scale, false)
					p, ok := useDirect(t, r, dev, hold)
					if !ok {
						t.Logf("%s exports no dmabufs: no frames presented without a copy", dev.Name())
						return
					}
					p.noTrim = noTrim
					frames := playPerf(r)
					t.Log(perfTable(fmt.Sprintf("%s, GPU presented without a copy (the server keeps %d busy; %d buffers registered over the session), %s%s", c.name, hold, p.win.created, dev.Name(), suffix), frames))
					t.Log(overTable(frames))
					t.Log(p.memoryLine())
					if p.prof != nil {
						t.Log(p.profileTable())
					}
					p.close()
				}
			})
		})
	}
}
