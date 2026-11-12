package kit_test

import (
	"cmp"
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/kit"
)

// This file measures what a big kit.Table costs per frame, the way the
// dashboard bench drives a page (examples/dashboard/bench_test.go): a Tree
// without a window, a manual clock, and a frame that is Animate, the
// pipeline (Tree.Frame: build, style, layout, paint) and the rasterization
// of its damage. The pipeline time is reported apart from the raster time,
// and the Tree's work counters say how much was built and laid out.
//
//	go test -run xxx -bench TablePerf -benchtime 20x ./kit
//
// To split the pipeline by phase use -cpuprofile and look at the
// cumulative time of core.(*Tree).buildPhase, stylePhase, boxPhase,
// layoutPhase and paintPhase.

// perfRig is a page mounted in a Tree at 1000 x 700 and scale 1.
type perfRig struct {
	tb   testing.TB
	tree *core.Tree
	fb   *image.RGBA
	rr   raster.Renderer
	now  time.Duration
}

func newPerfRig(tb testing.TB, root tela.Component) (*perfRig, time.Duration) {
	tb.Helper()
	th := kit.Theme()
	r := &perfRig{tb: tb}
	r.tree = core.NewTree(core.Config{Dev: core.ResolveDev(nil), RootClass: "bg-background text-foreground", Theme: (*core.Theme)(&th),
		Log: func(msg string) { tb.Errorf("dev message: %s", msg) }})
	r.tree.SetWake(func() {})
	r.tree.SetRequestFrame(func() {})
	r.tree.SetViewport(gfx.Size{W: 1000, H: 700})
	r.tree.SetScale(1)
	r.tree.Animate(0)
	stop := r.tree.ReceivePosts()
	tb.Cleanup(func() { stop(); r.tree.Unmount() })
	r.fb = image.NewRGBA(image.Rect(0, 0, 1000, 700))
	start := time.Now()
	r.tree.Mount(core.NodeOf(root))
	r.frame()
	mount := time.Since(start)
	for range 30 {
		r.frame() // settle: the scrollbars fade, the transitions end
	}
	return r, mount
}

// frame runs one frame and returns the time of the pipeline and of the
// rasterization of its damage.
func (r *perfRig) frame() (pipeline, rasterize time.Duration) {
	r.now += time.Second / 60
	r.tree.Animate(r.now)
	t0 := time.Now()
	_, changed := r.tree.Frame()
	pipeline = time.Since(t0)
	if changed {
		t1 := time.Now()
		r.tree.RasterizeFrame(r.fb, &r.rr, false)
		rasterize = time.Since(t1)
	}
	r.tree.FlushEffects()
	return
}

// perfBump is the setter of the counter of the page being measured.
var perfBump func(int)

// perfHeader is an unrelated sibling of the table with a state of its own.
type perfHeader struct{}

func (perfHeader) Build() tela.Node {
	n, set := tela.UseState(0)
	perfBump = set.Set
	return h.Div(h.Class("p-2"), h.Text(fmt.Sprintf("updates: %d", n)))
}

// perfCellSet are the setters of the texts of the perfCells of the
// fourth row: its second column (auto) and its last one (140px).
var perfCellSet [2]func(string)

// perfCell is a body cell's content with a state of its own, so that its
// text changes without building the table again (a live value in a cell).
type perfCell struct {
	v   string
	set int
}

func (c perfCell) Build() tela.Node {
	v, set := tela.UseState(c.v)
	perfCellSet[c.set] = set.Set
	return h.Text(v)
}

// perfPage is a header and a table in a scroll container. With owner true
// the counter is the page's own state, so a bump rebuilds the page (and
// the table, whose func props never compare equal); otherwise it is the
// header's, and the table is not rebuilt. The table is made once, so the
// frames do not pay for the data.
type perfPage struct {
	table kit.Table[trow]
	owner bool
}

// perfTable is a table of rows of six columns of text, as the dashboard's
// (no per-cell props): four px columns and two that share the rest. The
// last column sorts.
func perfTable(rows int, virtual bool) kit.Table[trow] {
	data := make([]trow, rows)
	for i := range data {
		data[i] = trow{i, []string{fmt.Sprintf("INV%03d", i+1), []string{"Paid", "Pending", "Unpaid"}[i%3], "Credit Card", "PayPal", "Bank Transfer", fmt.Sprintf("$%d.00", 100+i*50)}}
	}
	widths := []string{"", "", "140px", "140px", "140px", "140px"}
	cols := make([]kit.Column[trow], len(widths))
	for c := range cols {
		cols[c] = kit.Column[trow]{Header: fmt.Sprintf("Column %d", c), Width: widths[c],
			Cell: func(r trow) tela.Node {
				if r.i == 3 && (c == 1 || c == 5) {
					return perfCell{r.v[c], c / 5}
				}
				return h.Text(r.v[c])
			}}
	}
	cols[5].Align = kit.AlignRight
	cols[5].Sort = func(a, b trow) int { return cmp.Compare(a.v[5], b.v[5]) }
	t := kit.Table[trow]{Rows: data, RowKey: func(r trow) any { return r.i }, Columns: cols,
		Class: "min-h-0 flex-1 rounded-md border border-border"}
	if virtual {
		t.RowHeight = 37
	}
	return t
}

func (p perfPage) Build() tela.Node {
	var head tela.Child = perfHeader{}
	if p.owner {
		n, set := tela.UseState(0)
		perfBump = set.Set
		head = h.Div(h.Class("p-2"), h.Text(fmt.Sprintf("updates: %d", n)))
	}
	return h.Div(h.Class("flex h-[700px] w-[1000px] flex-col p-4"), head, p.table)
}

// perfCase runs op b.N times on a settled page and reports the pipeline
// and raster time per op and the work counters of the last one.
func perfCase(b *testing.B, p perfPage, op func(r *perfRig, i int)) {
	r, mount := newPerfRig(b, p)
	b.ReportMetric(float64(mount.Microseconds())/1000, "mount-ms")
	var pipe, ras time.Duration
	var last core.Stats
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		before := r.tree.Stats()
		op(r, i)
		pl, rs := r.frame()
		pipe, ras = pipe+pl, ras+rs
		last = statsOf(before, r.tree.Stats())
	}
	b.StopTimer()
	n := float64(b.N)
	b.ReportMetric(float64(pipe.Microseconds())/n/1000, "pipeline-ms")
	b.ReportMetric(float64(ras.Microseconds())/n/1000, "raster-ms")
	b.ReportMetric(float64(last.Builds), "builds")
	b.ReportMetric(float64(last.Styles), "styles")
	b.ReportMetric(float64(last.Layouts), "layouts")
	b.ReportMetric(float64(last.Paints), "paints")
	b.ReportMetric(float64(last.LaidOut), "laidout")
	b.ReportMetric(float64(last.Remapped), "remapped")
	b.ReportMetric(float64(last.Placed), "placed")
}

func statsOf(a, b core.Stats) core.Stats {
	return core.Stats{Builds: b.Builds - a.Builds, Styles: b.Styles - a.Styles, Layouts: b.Layouts - a.Layouts,
		Paints: b.Paints - a.Paints, Mounts: b.Mounts - a.Mounts, Unmounts: b.Unmounts - a.Unmounts,
		LaidOut: b.LaidOut - a.LaidOut, Remapped: b.Remapped - a.Remapped, Placed: b.Placed - a.Placed}
}

func perfOps() map[string]func(r *perfRig, i int) {
	return map[string]func(r *perfRig, i int){
		// A state outside the table changes (a spring value, a counter).
		"bump": func(r *perfRig, i int) { perfBump(i + 1) },
		// The text of a visible cell of an auto column changes (its own
		// state): "Paid" and "Sent" in turn, narrower than the column's
		// widest cell ("Pending"), so the column keeps its width.
		"cell": func(r *perfRig, i int) { perfCellSet[0]([]string{"Paid", "Sent"}[i%2]) },
		// The amount of the same row changes, in its 140px column: the
		// cell's new text measures as the old one.
		"cellpx": func(r *perfRig, i int) { perfCellSet[1](fmt.Sprintf("$%d.00", 250+i%2)) },
		// The pointer crosses from one row to the next (hover styles).
		"hover": func(r *perfRig, i int) {
			r.tree.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 300, Y: 120 + float32(i%8)*41}, Time: r.now})
		},
		// The wheel scrolls the table by 12 px, back and forth.
		"scroll": func(r *perfRig, i int) {
			dy := float32(12)
			if i/7%2 == 1 {
				dy = -dy
			}
			r.tree.HandleScroll(platform.ScrollEvent{Pos: platform.Point{X: 300, Y: 300}, Delta: platform.Point{Y: dy}, Phase: platform.ScrollPhaseUpdate, Time: r.now})
		},
		// A fling: 100 px per frame, more than two rows, so a virtual table
		// builds the entering rows and drops the leaving ones every frame.
		"fling": func(r *perfRig, i int) {
			dy := float32(100)
			if i/50%2 == 1 {
				dy = -dy
			}
			r.tree.HandleScroll(platform.ScrollEvent{Pos: platform.Point{X: 300, Y: 300}, Delta: platform.Point{Y: dy}, Phase: platform.ScrollPhaseUpdate, Time: r.now})
		},
	}
}

func BenchmarkTablePerf(b *testing.B) {
	for _, rows := range []int{240, 2000} {
		for _, owner := range []bool{false, true} {
			for _, op := range []string{"bump", "hover", "cell", "cellpx", "scroll", "fling"} {
				if owner && op != "bump" {
					continue
				}
				name := fmt.Sprintf("rows=%d/%s", rows, op)
				if owner {
					name = fmt.Sprintf("rows=%d/bump-parent", rows)
				}
				b.Run(name, func(b *testing.B) { perfCase(b, perfPage{table: perfTable(rows, false), owner: owner}, perfOps()[op]) })
			}
		}
	}
}

// BenchmarkTablePerfVirtual is BenchmarkTablePerf's cases with RowHeight
// set: the same table, virtual.
func BenchmarkTablePerfVirtual(b *testing.B) {
	for _, rows := range []int{240, 2000, 100000} {
		for _, owner := range []bool{false, true} {
			for _, op := range []string{"bump", "hover", "scroll", "fling"} {
				if owner && op != "bump" {
					continue
				}
				name := fmt.Sprintf("rows=%d/%s", rows, op)
				if owner {
					name = fmt.Sprintf("rows=%d/bump-parent", rows)
				}
				b.Run(name, func(b *testing.B) {
					perfCase(b, perfPage{table: perfTable(rows, true), owner: owner}, perfOps()[op])
				})
			}
		}
	}
}
