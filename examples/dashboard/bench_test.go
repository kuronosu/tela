package main

import (
	"fmt"
	"image"
	"math"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
)

// This file measures the phase-6 done criterion: the dashboard runs at 60
// fps at 1080p on CPU rendering. It drives the page headlessly through a
// scripted session of about 800 ticks of a 60 Hz clock: hover sweeps (card,
// nav and bar transitions), a period change (springs, a layout-animated
// pill, the sync spinner), a tab switch, a reordered list, scrolling and a
// fling under the sticky glass header, the dialog opening and closing, a
// toast with its countdown, an expanding card and the theme switch (every
// color in the page transitions at once).
//
// A frame is timed the way a Host runs it (core/host.go): the input of the
// tick, Animate, the pipeline (build, style, layout, paint: Tree.Frame),
// the rasterization of the damage (RasterizeFrame), the copy of the damage
// to the buffer a backend presents from (platform.CopyRGBAToBGRA, what the
// Wayland and X11 backends do), and the passive effects. A frame runs only
// when the tree asked for one, as in a window, so idle ticks cost nothing
// and are not counted. The clock is manual: 1/60 s per tick.
//
//	go test -run xxx -bench Session -benchtime 1x -count 3 ./examples/dashboard
//	go test -run TestSession -v ./examples/dashboard     # one session, with the full table
//
// The numbers depend on the machine and on GOMAXPROCS (the rasterizer's
// workers); a table prints both.

// frameInterval is 1/60 s.
const frameInterval = time.Second / 60

// Phase indexes into a frame's phase times.
const (
	phInput = iota
	phAnimate
	phPipeline
	phRaster
	phPresent
	phEffects
	numPhases
)

var phaseNames = [numPhases]string{"input", "animate", "pipeline", "raster", "present", "effects"}

// frameStat is one frame of the session.
type frameStat struct {
	scene  string
	total  time.Duration
	phase  [numPhases]time.Duration
	pixels int // device pixels rasterized (damage area)
	full   bool
	tick   int // the tick of the session
}

// rig is the page in a tree without a window, with the framebuffer a Host
// would keep.
type rig struct {
	tb      testing.TB
	tree    *core.Tree
	w, h    float32
	scale   float64
	fb      *image.RGBA
	out     []byte // the buffer a backend presents from (BGRA)
	outStr  int
	rr      raster.Renderer
	now     time.Duration
	ticks   int  // ticks played
	asked   bool // a frame was requested since the last one ran
	scene   string
	pointer platform.Point
	stop    func()
}

// newRig mounts the dashboard at w x h logical pixels and scale, light or
// dark, and runs its first frames (not part of the session).
func newRig(tb testing.TB, w, h float32, scale float64, dark bool, opts ...func(*core.Config)) *rig {
	tb.Helper()
	r := &rig{tb: tb, w: w, h: h, scale: scale}
	cfg := core.Config{RootClass: rootClass, Dark: dark, Log: func(msg string) { tb.Errorf("dev message: %s", msg) }}
	for _, o := range opts {
		o(&cfg)
	}
	r.tree = core.NewTree(cfg)
	r.tree.SetWake(func() { r.asked = true })
	r.tree.SetRequestFrame(func() { r.asked = true })
	r.tree.SetViewport(gfx.Size{W: w, H: h})
	r.tree.SetScale(float32(scale))
	r.tree.Animate(0) // the frame clock drives the tree from its first frame on (springs wait for it)
	r.stop = r.tree.ReceivePosts()
	tb.Cleanup(func() { r.stop(); r.tree.Unmount() })
	phys := platform.PhysicalSize(platform.Size{W: w, H: h}, scale)
	r.fb = image.NewRGBA(image.Rect(0, 0, phys.X, phys.Y))
	r.outStr = 4 * phys.X
	r.out = make([]byte, r.outStr*phys.Y)
	r.tree.Mount(tela.C(Dashboard))
	r.asked = true
	return r
}

// settle runs frames until the tree asks for no more (bounded), unmeasured.
func (r *rig) settle(limit int) {
	for i := 0; i < limit && r.asked; i++ {
		r.tick(nil)
	}
}

// tick advances the clock by one frame interval and, if the tree asked for
// a frame, runs one like core.Host does. When st is not nil, the frame is
// recorded. It reports whether a frame ran. events runs before the frame
// (the tick's input), inside the measured time when a frame follows.
func (r *rig) tick(st *[]frameStat, events ...func()) bool {
	r.now += frameInterval
	r.ticks++
	t0 := time.Now()
	for _, f := range events {
		f()
	}
	if at, ok := r.tree.NextDeadline(); ok && at <= r.now {
		r.tree.TickGestures(r.now)
	}
	if r.tree.Dirty() {
		r.asked = true
	}
	if !r.asked {
		return false
	}
	var fs frameStat
	fs.scene, fs.tick = r.scene, r.ticks
	mark := time.Now()
	lap := func(ph int) {
		n := time.Now()
		fs.phase[ph] += n.Sub(mark)
		mark = n
	}
	fs.phase[phInput] = mark.Sub(t0) // events and deadlines
	r.asked = false

	r.tree.Animate(r.now)
	lap(phAnimate)
	r.tree.SetScale(float32(r.scale))
	_, changed := r.tree.Frame()
	lap(phPipeline)
	if changed {
		d := r.tree.RasterizeFrame(r.fb, &r.rr, false)
		lap(phRaster)
		fs.full = d.Full
		if d.Full {
			fs.pixels = r.fb.Rect.Dx() * r.fb.Rect.Dy()
			platform.CopyRGBAToBGRA(r.out, r.outStr, r.fb, r.fb.Rect)
		} else {
			for _, s := range d.Shifts {
				fs.pixels += s.Rect.Dx() * s.Rect.Dy()
			}
			for _, rc := range d.Rects {
				fs.pixels += rc.Dx() * rc.Dy()
			}
			for _, rc := range d.Changed() {
				platform.CopyRGBAToBGRA(r.out, r.outStr, r.fb, rc)
			}
		}
		lap(phPresent)
	}
	r.tree.FlushEffects()
	lap(phEffects)
	fs.total = time.Since(t0)
	if st != nil {
		*st = append(*st, fs)
	}
	return true
}

// find returns the semantics node with the role and name, failing the
// test when there is none.
func (r *rig) find(role, name string) *semantics.Node {
	r.tb.Helper()
	var found *semantics.Node
	r.tree.Semantics().Walk(func(n *semantics.Node) bool {
		if found == nil && n.Role == role && n.Name == name {
			found = n
		}
		return found == nil
	})
	if found == nil {
		r.tb.Fatalf("no %s named %q on the page", role, name)
	}
	return found
}

// center is the middle of a node's drawn box.
func center(n *semantics.Node) platform.Point {
	c := n.Bounds.Center()
	return platform.Point{X: c.X, Y: c.Y}
}

var primary = platform.ButtonPrimary.Mask()

func (r *rig) move(p platform.Point) func() {
	return func() {
		r.pointer = p
		r.tree.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: p, Time: r.now})
	}
}

// click moves the pointer to the center of the node and presses and
// releases the primary button, in the tick it is called from.
func (r *rig) click(role, name string) []func() {
	p := center(r.find(role, name))
	return []func(){
		r.move(p),
		func() {
			r.tree.HandlePointer(platform.PointerEvent{Action: platform.PointerDown, Pos: p, Button: platform.ButtonPrimary, Buttons: primary, Time: r.now})
			r.tree.HandlePointer(platform.PointerEvent{Action: platform.PointerUp, Pos: p, Button: platform.ButtonPrimary, Time: r.now})
		},
	}
}

func (r *rig) wheel(dy float32, phase platform.ScrollPhase) func() {
	return func() {
		r.tree.HandleScroll(platform.ScrollEvent{Pos: r.pointer, Delta: platform.Point{Y: dy}, Phase: phase, Time: r.now})
	}
}

// path returns the pointer's position at tick i of a walk through points at
// speed px per tick.
func path(points []platform.Point, speed float32, i int) platform.Point {
	d := float32(i) * speed
	for k := 1; k < len(points); k++ {
		a, b := points[k-1], points[k]
		seg := float32(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)))
		if d <= seg || k == len(points)-1 {
			f := min(d/max(seg, 1), 1)
			return platform.Point{X: a.X + (b.X-a.X)*f, Y: a.Y + (b.Y-a.Y)*f}
		}
		d -= seg
	}
	return points[len(points)-1]
}

// scene is a stretch of the session: ticks ticks of the clock, with on
// called at the start of each one for its input.
type scene struct {
	name  string
	ticks int
	on    func(r *rig, i int) []func()
}

// session is the script. Positions are looked up when a scene starts (a
// scene's on is called with i == 0 first), so they follow the layout.
func session() []scene {
	var sweep []platform.Point
	var content platform.Point
	return []scene{
		{"hover sweep", 100, func(r *rig, i int) []func() {
			if i == 0 {
				nav := center(r.find("button", "Overview"))
				last := center(r.find("button", "Settings"))
				rev := center(r.find("heading", "Revenue"))
				goal := center(r.find("heading", "Monthly goal"))
				perf := center(r.find("heading", "Performance"))
				sweep = []platform.Point{nav, last, {X: rev.X, Y: rev.Y + 40}, {X: goal.X, Y: goal.Y + 40},
					{X: perf.X + 60, Y: perf.Y + 200}, {X: perf.X + 600, Y: perf.Y + 200}, {X: perf.X + 700, Y: perf.Y + 260}}
			}
			return []func(){r.move(path(sweep, 36, i))}
		}},
		{"period 90D", 60, func(r *rig, i int) []func() {
			if i == 0 {
				return r.click("tab", "90D")
			}
			return nil
		}},
		{"tab Orders", 45, func(r *rig, i int) []func() {
			if i == 0 {
				return r.click("tab", "Orders")
			}
			return nil
		}},
		{"reorder tasks", 60, func(r *rig, i int) []func() {
			switch i {
			case 0:
				return r.click("button", "Due")
			case 30:
				return r.click("checkbox", "Fix the failed invoice retries")
			}
			return nil
		}},
		{"scroll and fling", 150, func(r *rig, i int) []func() {
			if i == 0 {
				content = platform.Point{X: r.w * 0.55, Y: r.h * 0.5}
				return []func(){r.move(content)}
			}
			switch {
			case i < 30: // a touchpad drag down
				return []func(){r.wheel(14, platform.ScrollPhaseUpdate)}
			case i == 30: // the fingers lift: a fling
				return []func(){r.wheel(0, platform.ScrollPhaseEnd)}
			case i >= 80 && i < 110: // and back up
				return []func(){r.wheel(-14, platform.ScrollPhaseUpdate)}
			case i == 110:
				return []func(){r.wheel(0, platform.ScrollPhaseEnd)}
			}
			return nil
		}},
		{"dialog", 120, func(r *rig, i int) []func() {
			switch i {
			case 0:
				return r.click("button", "New report")
			case 30:
				return r.click("radio", "CSV")
			case 50:
				return r.click("switch", "Send every week")
			case 70:
				return r.click("button", "Cancel")
			}
			return nil
		}},
		{"toast and expand", 130, func(r *rig, i int) []func() {
			switch i {
			case 0:
				return r.click("button", "Export")
			case 10: // the card is below the fold in a short window
				return []func(){r.wheel(480, platform.ScrollPhaseNone)}
			case 30:
				return r.click("button", "Show details")
			case 80:
				return r.click("button", "Hide details")
			}
			return nil
		}},
		{"theme switch", 100, func(r *rig, i int) []func() {
			switch i {
			case 0, 50:
				return r.click("switch", "Dark mode")
			}
			return nil
		}},
		{"hover sweep 2", 80, func(r *rig, i int) []func() {
			if i == 0 {
				rev := center(r.find("heading", "Revenue"))
				perf := center(r.find("heading", "Performance"))
				sweep = []platform.Point{{X: rev.X, Y: rev.Y + 40}, {X: perf.X + 700, Y: perf.Y + 260}, {X: perf.X + 60, Y: perf.Y + 260}, center(r.find("button", "Overview"))}
			}
			return []func(){r.move(path(sweep, 40, i))}
		}},
		{"settle", 60, nil},
	}
}

// idleTicks is how long the session ends with the page at rest: no frame
// may run.
const idleTicks = 30

// runSession plays the script on r and returns the frames. It fails the
// test if the page still draws frames after the settle scene.
func runSession(r *rig) (frames []frameStat, ticks int) {
	r.tb.Helper()
	r.settle(600) // the first frames and their animations (the numbers count up)
	runtime.GC()
	for _, sc := range session() {
		r.scene = sc.name
		for i := 0; i < sc.ticks; i++ {
			var events []func()
			if sc.on != nil {
				events = sc.on(r, i)
			}
			r.tick(&frames, events...)
			ticks++
		}
	}
	// The idle check: nothing animates forever, so the page asks for no more
	// frames.
	r.scene = "idle"
	extra := 0
	for range idleTicks {
		if r.tick(&frames) {
			extra++
		}
		ticks++
	}
	if extra != 0 {
		r.tb.Errorf("the page drew %d frames in the %d ticks after the session: something still animates or invalidates", extra, idleTicks)
	}
	if r.tree.Animating() || r.tree.Dirty() {
		r.tb.Errorf("the page is not at rest after the session (animating=%v dirty=%v)", r.tree.Animating(), r.tree.Dirty())
	}
	return frames, ticks
}

// ms is a duration in milliseconds.
func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// percentile is the nearest-rank percentile p (0 to 1) of the sorted xs.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	return xs[max(int(math.Ceil(p*float64(len(xs))))-1, 0)]
}

// dist is the p50, p95, p99 and max of xs (not sorted).
type dist struct{ P50, P95, P99, Max, Mean float64 }

func distOf(xs []float64) dist {
	s := slices.Clone(xs)
	slices.Sort(s)
	sum := 0.0
	for _, x := range s {
		sum += x
	}
	d := dist{P50: percentile(s, 0.5), P95: percentile(s, 0.95), P99: percentile(s, 0.99)}
	if len(s) > 0 {
		d.Max, d.Mean = s[len(s)-1], sum/float64(len(s))
	}
	return d
}

func totals(fs []frameStat) []float64 {
	out := make([]float64, len(fs))
	for i, f := range fs {
		out[i] = ms(f.total)
	}
	return out
}

// worstPhase is the phase with the largest p95 and the one with the
// largest single frame.
func worstPhase(fs []frameStat) (byP95 string, p95 float64, byMax string, max float64) {
	for ph := range numPhases {
		xs := make([]float64, len(fs))
		for i, f := range fs {
			xs[i] = ms(f.phase[ph])
		}
		d := distOf(xs)
		if d.P95 > p95 {
			byP95, p95 = phaseNames[ph], d.P95
		}
		if d.Max > max {
			byMax, max = phaseNames[ph], d.Max
		}
	}
	return
}

// report formats the frames as the table of the done criterion.
func report(label string, fs []frameStat, ticks int, w, h float32, scale float64) string {
	var sb strings.Builder
	phys := platform.PhysicalSize(platform.Size{W: w, H: h}, scale)
	fmt.Fprintf(&sb, "\n== %s: %gx%g logical, scale %g (%dx%d device px), GOMAXPROCS=%d raster workers, %d frames in %d ticks\n",
		label, w, h, scale, phys.X, phys.Y, runtime.GOMAXPROCS(0), len(fs), ticks)
	d := distOf(totals(fs))
	fmt.Fprintf(&sb, "frame ms: p50 %.2f  p95 %.2f  p99 %.2f  max %.2f  mean %.2f\n", d.P50, d.P95, d.P99, d.Max, d.Mean)
	over := 0
	for _, f := range fs {
		if f.total > frameInterval {
			over++
		}
	}
	fmt.Fprintf(&sb, "frames over 16.67 ms: %d of %d\n", over, len(fs))
	a, ap, b, bm := worstPhase(fs)
	fmt.Fprintf(&sb, "worst phase: %s by p95 (%.2f ms), %s by max (%.2f ms)\n", a, ap, b, bm)
	fmt.Fprintf(&sb, "%-18s %5s %8s %8s %8s", "phase", "", "p50", "p95", "max")
	sb.WriteByte('\n')
	for ph := range numPhases {
		xs := make([]float64, len(fs))
		for i, f := range fs {
			xs[i] = ms(f.phase[ph])
		}
		d := distOf(xs)
		fmt.Fprintf(&sb, "%-18s %5s %8.2f %8.2f %8.2f\n", phaseNames[ph], "", d.P50, d.P95, d.Max)
	}
	worst := slices.Clone(fs)
	slices.SortFunc(worst, func(a, b frameStat) int { return int(b.total - a.total) })
	fmt.Fprintf(&sb, "slowest frames:")
	for _, f := range worst[:min(8, len(worst))] {
		fmt.Fprintf(&sb, "  [%s t%d %.1fms %.2fMpx full=%v pipe=%.1f raster=%.1f]", f.scene, f.tick, ms(f.total), float64(f.pixels)/1e6, f.full, ms(f.phase[phPipeline]), ms(f.phase[phRaster]))
	}
	sb.WriteByte('\n')
	fmt.Fprintf(&sb, "%-18s %5s %8s %8s %8s %8s\n", "scene", "n", "p50", "p95", "max", "Mpx/f")
	var order []string
	byScene := map[string][]frameStat{}
	for _, f := range fs {
		if _, ok := byScene[f.scene]; !ok {
			order = append(order, f.scene)
		}
		byScene[f.scene] = append(byScene[f.scene], f)
	}
	for _, name := range order {
		g := byScene[name]
		d := distOf(totals(g))
		px := 0
		for _, f := range g {
			px += f.pixels
		}
		fmt.Fprintf(&sb, "%-18s %5d %8.2f %8.2f %8.2f %8.2f\n", name, len(g), d.P50, d.P95, d.Max, float64(px)/float64(len(g))/1e6)
	}
	return sb.String()
}

// configs are the sizes the criterion asks for.
var configs = []struct {
	name  string
	w, h  float32
	scale float64
}{
	{"1080p_x1", 1920, 1080, 1},
	{"1080p_x2", 1920, 1080, 2},
	{"1440x900_x2", 1440, 900, 2},
}

// TestSession plays the session once per configuration and prints the
// table (go test -v). It fails if the page does not come to rest, and, with
// TELA_PERF=strict, if the p95 of 1080p at scale 1 is over a frame
// interval; the number is a property of the machine, so it is not checked
// by default.
func TestSession(t *testing.T) {
	for _, c := range configs {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.w, c.h, c.scale, false)
			frames, ticks := runSession(r)
			t.Log(report(c.name, frames, ticks, c.w, c.h, c.scale))
			if len(frames) < 600 {
				t.Errorf("the session drew %d frames, want at least 600: lengthen the script", len(frames))
			}
			if os.Getenv("TELA_PERF") == "strict" && c.name == "1080p_x1" {
				if d := distOf(totals(frames)); d.P95 > ms(frameInterval) {
					t.Errorf("p95 %.2f ms is over %.2f ms", d.P95, ms(frameInterval))
				}
			}
		})
	}
}

// BenchmarkSession plays the session b.N times (use -benchtime 1x) and
// reports the distribution of the frames' times in ms.
func BenchmarkSession(b *testing.B) {
	for _, c := range configs {
		b.Run(c.name, func(b *testing.B) {
			var all []frameStat
			ticks := 0
			for b.Loop() {
				b.StopTimer()
				r := newRig(b, c.w, c.h, c.scale, false)
				b.StartTimer()
				fs, n := runSession(r)
				all, ticks = append(all, fs...), ticks+n
			}
			d := distOf(totals(all))
			b.ReportMetric(d.P50, "p50-ms")
			b.ReportMetric(d.P95, "p95-ms")
			b.ReportMetric(d.P99, "p99-ms")
			b.ReportMetric(d.Max, "max-ms")
			b.ReportMetric(float64(len(all))/float64(b.N), "frames/op")
			if testing.Verbose() {
				b.Log(report(c.name, all, ticks, c.w, c.h, c.scale))
			}
		})
	}
}

// BenchmarkFullFrame is the rasterization of the whole settled page (what
// a theme switch, a resize or a scrim fading over the window costs per
// frame), at 1080p.
func BenchmarkFullFrame(b *testing.B) {
	for _, c := range configs {
		b.Run(c.name, func(b *testing.B) {
			r := newRig(b, c.w, c.h, c.scale, false)
			r.settle(600)
			b.ResetTimer()
			for b.Loop() {
				r.tree.RasterizeFrame(r.fb, &r.rr, true)
			}
		})
	}
}

// BenchmarkScrollFrame is one frame of scrolling the page by 12 px back and
// forth with the pointer over the content: every card moves over the fixed
// backdrop, so it is close to a full rasterization.
func BenchmarkScrollFrame(b *testing.B) {
	for _, c := range configs {
		b.Run(c.name, func(b *testing.B) {
			r := newRig(b, c.w, c.h, c.scale, false)
			r.settle(600)
			p := platform.Point{X: c.w * 0.55, Y: c.h * 0.5}
			r.tick(nil, r.move(p))
			dy := float32(12)
			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				if i%7 == 0 && i > 0 {
					dy = -dy
				}
				if !r.tick(nil, r.wheel(dy, platform.ScrollPhaseUpdate)) {
					b.Fatalf("iteration %d: the scroll did not draw a frame (asked=%v dirty=%v)", i, r.asked, r.tree.Dirty())
				}
			}
		})
	}
}
