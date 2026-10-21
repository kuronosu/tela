package core

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// ---------------------------------------------------------------------
// The size index.

func TestSizeIndex(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var x sizeIndex
	x.resize(0, nil, true)
	if x.len() != 0 || x.total() != 0 || x.before(10) != 0 {
		t.Fatalf("empty index: len %d, total %v, before %d", x.len(), x.total(), x.before(10))
	}
	est := func(i int) float32 { return float32(1+i%13) * 1.25 }
	for _, n := range []int{1, 2, 3, 7, 8, 9, 100, 1000} {
		x.resize(n, est, true)
		naive := make([]float64, n+1)
		check := func(when string) {
			t.Helper()
			for i := 0; i < n; i++ {
				naive[i+1] = naive[i] + float64(x.at(i))
			}
			for i := 0; i <= n; i++ {
				if got := x.offset(i); got != naive[i] {
					t.Fatalf("n=%d %s: offset(%d) = %v, want %v", n, when, i, got, naive[i])
				}
			}
			for k := 0; k < 200; k++ {
				y := r.Float64()*naive[n]*1.2 - 5
				if k < 4 {
					y = []float64{0, naive[n], naive[n/2], naive[(n+1)/3]}[k]
				}
				want, wantEq := 0, 0
				for want < n && naive[want+1] < y {
					want++
				}
				for wantEq < n && naive[wantEq+1] <= y {
					wantEq++
				}
				if got := x.before(y); got != want {
					t.Fatalf("n=%d %s: before(%v) = %d, want %d", n, when, y, got, want)
				}
				if got := x.upTo(y); got != wantEq {
					t.Fatalf("n=%d %s: upTo(%v) = %d, want %d", n, when, y, got, wantEq)
				}
			}
		}
		check("built")
		for k := 0; k < 50; k++ {
			i := r.IntN(n)
			v := float32(r.IntN(200)) / 4
			if k%7 == 0 {
				v = 0 // zero-size items
			}
			x.set(i, v)
		}
		check("after set")
	}
	// Growing keeps the sizes of the items that remain; a reset does not.
	x.resize(10, func(int) float32 { return 40 }, true)
	x.set(3, 99)
	x.resize(20, func(int) float32 { return 10 }, false)
	if x.at(3) != 99 || x.at(4) != 40 || x.at(15) != 10 || x.total() != 99+9*40+10*10 {
		t.Errorf("grow: sizes %v, total %v", x.size, x.total())
	}
	x.resize(5, nil, false)
	if x.at(3) != 99 || x.len() != 5 || x.total() != 99+4*40 {
		t.Errorf("shrink: sizes %v, total %v", x.size, x.total())
	}
	x.resize(5, func(int) float32 { return 7 }, true)
	if x.total() != 35 {
		t.Errorf("reset: total %v, want 35", x.total())
	}
}

func TestClampSizes(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for _, c := range []struct{ in, size, est float32 }{
		{40, 40, 40}, {0, 0, 1}, {-3, 0, 1}, {nan, 0, 1}, {inf, maxItemSize, maxItemSize}, {0.25, 0.25, 1},
	} {
		if got := clampSize(c.in); got != c.size {
			t.Errorf("clampSize(%v) = %v, want %v", c.in, got, c.size)
		}
		if got := clampEstimate(c.in); got != c.est {
			t.Errorf("clampEstimate(%v) = %v, want %v", c.in, got, c.est)
		}
	}
}

// ---------------------------------------------------------------------
// Harness.

// vrowBuilds counts the builds of vrow per index.
var vrowBuilds = map[int]int{}

// vrow is a 40 px row component.
func vrow(i int) Node {
	vrowBuilds[i]++
	return div(Class("h-[40px] px-2"), Textf("Row %d", i))
}

func rowNode(i int) Node { return ComponentProps(vrow, i) }

func est40(int) float32 { return 40 }

// sized returns a renderItem of plain divs whose heights are h(i).
func sized(h func(int) float32) func(int) Node {
	return func(i int) Node { return div(Style(style.Height(h(i)))) }
}

// refList mounts a list (Ref attached) in a page; *ref receives its
// ElementRef.
func refList(ref *ElementRef, count int, est func(int) float32, render func(int) Node, props ...Prop) Node {
	return Component(func() Node {
		r := UseElementRef()
		*ref = r
		return div(VirtualList(count, est, render, append([]Prop{AttachRef(r)}, props...)...))
	})
}

func onlyList(t testing.TB, tr *Tree) *vlist {
	t.Helper()
	if len(tr.virt.lists) != 1 {
		t.Fatalf("%d virtual lists, want 1", len(tr.virt.lists))
	}
	return tr.virt.lists[0]
}

// settleTree runs frames until the tree is quiet.
func settleTree(t testing.TB, tr *Tree) {
	t.Helper()
	for i := 0; ; i++ {
		if i == 50 {
			t.Fatal("the tree never settled")
		}
		tr.Frame()
		tr.FlushEffects()
		if !tr.Dirty() {
			return
		}
	}
}

// wrapper returns the wrapper box of built item i, or nil.
func (vl *vlist) wrapper(i int) *Box {
	_, content := vl.boxes()
	if content == nil {
		return nil
	}
	for _, k := range content.kids {
		if v := k.virtMark(); v != nil && v.idx == i {
			return k
		}
	}
	return nil
}

// port returns the top and height of the list's scrollport in window
// coordinates.
func (vl *vlist) port() (top, h float32) {
	outer, _ := vl.boxes()
	return vl.tree.liveOrigin(outer).Y + outer.ld.Layout().Border.Top, outer.scr.client.H
}

// itemTop returns the window y of built item i.
func itemTop(t testing.TB, vl *vlist, i int) float32 {
	t.Helper()
	k := vl.wrapper(i)
	if k == nil {
		t.Fatalf("item %d is not built (built %d..%d)", i, vl.lo, vl.hi)
	}
	return vl.tree.liveOrigin(k).Y
}

func (vl *vlist) scroller() *scroller {
	outer, _ := vl.boxes()
	return outer.scr
}

// checkContiguous checks that the built items of each run are laid out
// one after the other at their offsets.
func checkContiguous(t testing.TB, vl *vlist) {
	t.Helper()
	outer, content := vl.boxes()
	origin := vl.tree.liveOrigin(content).Y
	for _, k := range content.kids {
		i := k.virtMark().idx
		want := float32(vl.sizes.offset(i))
		if got := vl.tree.liveOrigin(k).Y - origin; !near(got, want, 0.01) {
			t.Fatalf("item %d at %v in the content, want its offset %v (built %v)", i, got, want, vl.built)
		}
		if h := k.ld.Layout().Size.H; h != vl.sizes.at(i) {
			t.Fatalf("item %d is %v px, its size is %v", i, h, vl.sizes.at(i))
		}
	}
	if h := content.ld.Layout().Size.H; !near(h, float32(vl.sizes.total()), 0.01) {
		t.Fatalf("content %v px, total %v", h, vl.sizes.total())
	}
	_ = outer
}

// ---------------------------------------------------------------------
// The window.

func TestVirtualBuildsOnlyTheWindow(t *testing.T) {
	clear(vrowBuilds)
	tr := newTestTree(t, 400, 600, div(VirtualList(100_000, est40, rowNode, Class("h-[400px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	// 10 visible rows, 5 more below; none above the first.
	if vl.lo != 0 || vl.hi != 14 || len(vl.built) != 15 {
		t.Fatalf("built %d..%d (%d items), want 0..14", vl.lo, vl.hi, len(vl.built))
	}
	if len(vrowBuilds) != 15 {
		t.Errorf("%d row components built, want 15", len(vrowBuilds))
	}
	s := vl.scroller()
	if s.max.Y != 4_000_000-400 || s.client.H != 400 {
		t.Errorf("max %v, client %v", s.max, s.client)
	}
	if ev := s.event(); ev.ScrollHeight != 4_000_000 {
		t.Errorf("ScrollHeight %v, want 4,000,000", ev.ScrollHeight)
	}
	checkContiguous(t, vl)
	// Fewer than 100 elements in all for 100,000 rows.
	if m := tr.Stats().Mounts; m > 100 {
		t.Errorf("%d elements mounted", m)
	}
}

func TestVirtualScrollInsideTheWindowOnlyPaints(t *testing.T) {
	clear(vrowBuilds)
	tr := newTestTree(t, 400, 600, div(VirtualList(100_000, est40, rowNode, Class("h-[400px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	s := vl.scroller()
	for _, y := range []float32{40, 75, 160} {
		before := tr.Stats()
		tr.setScroll(s, gfx.Point{Y: y})
		settleTree(t, tr)
		d := tr.Stats()
		if d.Builds != before.Builds || d.Layouts != before.Layouts || d.Styles != before.Styles || d.Paints != before.Paints+1 {
			t.Errorf("scroll to %v: %d builds, %d layouts, %d styles, %d paints; want a paint-only frame",
				y, d.Builds-before.Builds, d.Layouts-before.Layouts, d.Styles-before.Styles, d.Paints-before.Paints)
		}
	}
}

func TestVirtualScrollPastTheWindowBuildsOnlyEnteringItems(t *testing.T) {
	clear(vrowBuilds)
	tr := newTestTree(t, 400, 600, div(VirtualList(100_000, est40, rowNode, Class("h-[400px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	s := vl.scroller()
	// Rows 5..14 visible: row 15 must be built.
	before := tr.Stats()
	tr.setScroll(s, gfx.Point{Y: 200})
	settleTree(t, tr)
	d := tr.Stats()
	if vl.lo != 0 || vl.hi != 19 {
		t.Fatalf("built %d..%d, want 0..19", vl.lo, vl.hi)
	}
	// The list and the five entering rows; the rows that stay are not
	// built again.
	if n := d.Builds - before.Builds; n != 6 {
		t.Errorf("%d builds, want 6 (the list and rows 15..19)", n)
	}
	for i := 0; i < 20; i++ {
		if vrowBuilds[i] != 1 {
			t.Errorf("row %d built %d times", i, vrowBuilds[i])
		}
	}
	if d.Unmounts != before.Unmounts {
		t.Errorf("%d unmounts, want 0", d.Unmounts-before.Unmounts)
	}
	// Rows 10..19 visible: 0..4 leave.
	before = tr.Stats()
	tr.setScroll(s, gfx.Point{Y: 400})
	settleTree(t, tr)
	d = tr.Stats()
	if vl.lo != 5 || vl.hi != 24 {
		t.Fatalf("built %d..%d, want 5..24", vl.lo, vl.hi)
	}
	// 5 rows of 3 elements (wrapper, row component, row div; texts too).
	if n := d.Unmounts - before.Unmounts; n != 5*4 {
		t.Errorf("%d unmounts, want 20", n)
	}
	checkContiguous(t, vl)
	// A jump far away replaces the window.
	tr.setScroll(s, gfx.Point{Y: 2_000_000})
	settleTree(t, tr)
	if vl.lo != 49_995 || vl.hi != 50_014 {
		t.Fatalf("built %d..%d after a jump, want 49995..50014", vl.lo, vl.hi)
	}
	checkContiguous(t, vl)
	if got := itemTop(t, vl, 50_000); got != 0 {
		t.Errorf("row 50000 at %v, want 0", got)
	}
}

func TestVirtualOverscan(t *testing.T) {
	for _, c := range []struct{ overscan, lo, hi int }{{0, 500, 509}, {1, 499, 510}, {12, 488, 521}} {
		tr := newTestTree(t, 400, 600, div(VirtualList(1000, est40, sized(est40), Class("h-[400px]"), Overscan(c.overscan))), nil)
		settleTree(t, tr)
		vl := onlyList(t, tr)
		if vl.lo != 0 || vl.hi != 9+c.overscan {
			t.Errorf("Overscan(%d): built %d..%d at the top", c.overscan, vl.lo, vl.hi)
		}
		tr.setScroll(vl.scroller(), gfx.Point{Y: 20_000})
		settleTree(t, tr)
		if vl.lo != c.lo || vl.hi != c.hi {
			t.Errorf("Overscan(%d): built %d..%d, want %d..%d", c.overscan, vl.lo, vl.hi, c.lo, c.hi)
		}
		// With no overscan every new row rebuilds, and the list still
		// settles.
		before := tr.Stats().Builds
		tr.setScroll(vl.scroller(), gfx.Point{Y: 20_040})
		settleTree(t, tr)
		if c.overscan == 0 && (tr.Stats().Builds == before || vl.lo != 501 || vl.hi != 510) {
			t.Errorf("Overscan(0): built %d..%d after one more row", vl.lo, vl.hi)
		}
		checkContiguous(t, vl)
	}
}

// ---------------------------------------------------------------------
// Measuring.

func varHeight(i int) float32 { return float32(20 + (i%5)*10) }

func TestVirtualMeasuresItems(t *testing.T) {
	tr := newTestTree(t, 400, 600, div(VirtualList(10_000, est40, sized(varHeight), Class("h-[400px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	for _, i := range vl.built {
		if vl.sizes.at(i) != varHeight(i) {
			t.Errorf("item %d: size %v, want %v", i, vl.sizes.at(i), varHeight(i))
		}
	}
	// Measured rows average 40 px; the others keep their estimates.
	if got := vl.sizes.at(9000); got != 40 {
		t.Errorf("unbuilt item: size %v, want the estimate", got)
	}
	checkContiguous(t, vl)
	// The window holds every visible item with the measured sizes: 400 px
	// of rows that are 40 px on average is 10 rows plus the overscan.
	top, h := vl.port()
	last := vl.wrapper(vl.hi)
	if y := tr.liveOrigin(last).Y; y < top+h {
		t.Errorf("the last built item starts at %v, inside the viewport (ends at %v)", y, top+h)
	}
	// Scrolling far keeps the measurements of the rows that left.
	s := vl.scroller()
	tr.setScroll(s, gfx.Point{Y: 100_000})
	settleTree(t, tr)
	if vl.sizes.at(2) != varHeight(2) {
		t.Errorf("item 2 lost its measurement: %v", vl.sizes.at(2))
	}
	checkContiguous(t, vl)
}

func TestVirtualStablePositionScrollingUp(t *testing.T) {
	// Items are 80 px, estimated 40: scrolling up from the middle meets
	// unmeasured items above the viewport all the time.
	tall := func(int) float32 { return 80 }
	tr := newTestTree(t, 400, 600, div(VirtualList(5000, est40, sized(tall), Class("h-[400px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	s := vl.scroller()
	tr.setScroll(s, gfx.Point{Y: 100_000}) // item 2500 by the estimates
	settleTree(t, tr)
	top, _ := vl.port()
	checkContiguous(t, vl)
	for step := 0; step < 120; step++ {
		// The first item whose top is in the viewport.
		ref := -1
		for _, i := range vl.built {
			if itemTop(t, vl, i) >= top {
				ref = i
				break
			}
		}
		y0 := itemTop(t, vl, ref)
		tr.setScroll(s, s.pos.Sub(gfx.Point{Y: 30}))
		settleTree(t, tr)
		if y1 := itemTop(t, vl, ref); !near(y1, y0+30, 0.01) {
			t.Fatalf("step %d: item %d moved from %v to %v, want %v (offset %v)", step, ref, y0, y1, y0+30, s.pos.Y)
		}
		checkContiguous(t, vl)
	}
}

// grower is an item whose height is in its state, set from outside.
var growSet = map[int]func(float32){}

func grower(i int) Node {
	h, set := UseStateSet(float32(40))
	growSet[i] = set
	return div(Style(style.Height(h)))
}

func TestVirtualStablePositionWhenItemsAboveGrow(t *testing.T) {
	clear(growSet)
	render := func(i int) Node { return ComponentProps(grower, i) }
	tr := newTestTree(t, 400, 600, div(VirtualList(1000, est40, render, Class("h-[400px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	s := vl.scroller()
	tr.setScroll(s, gfx.Point{Y: 400}) // rows 10..19 visible, 5..9 built above
	settleTree(t, tr)
	y0 := itemTop(t, vl, 12)
	growSet[7](140) // above the viewport: the offset follows
	settleTree(t, tr)
	if y := itemTop(t, vl, 12); y != y0 || s.pos.Y != 500 {
		t.Errorf("row 12 moved from %v to %v (offset %v, want 500)", y0, y, s.pos.Y)
	}
	growSet[15](100) // in the viewport: the rows below move, as in CSS
	settleTree(t, tr)
	if y := itemTop(t, vl, 12); y != y0 {
		t.Errorf("row 12 moved from %v to %v after a row below it grew", y0, y)
	}
	if y := itemTop(t, vl, 16); y != y0+4*40+60 {
		t.Errorf("row 16 at %v, want %v", y, y0+4*40+60)
	}
	checkContiguous(t, vl)
}

func TestVirtualWidthChangeMeasuresAgain(t *testing.T) {
	text := strings.Repeat("word ", 30)
	render := func(i int) Node { return p(Class("py-1"), Textf("%d %s", i, text)) }
	tr := newTestTree(t, 800, 600, div(VirtualList(1000, est40, render, Class("h-[500px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	wide := vl.sizes.at(1)
	tr.SetViewport(gfx.Size{W: 300, H: 600})
	settleTree(t, tr)
	if narrow := vl.sizes.at(1); !(narrow > wide) {
		t.Errorf("row 1 is %v px at 300 px wide, %v at 800", narrow, wide)
	}
	checkContiguous(t, vl)
}

// ---------------------------------------------------------------------
// ScrollToIndex.

func TestVirtualScrollToIndex(t *testing.T) {
	var ref ElementRef
	tr := newTestTree(t, 400, 600, refList(&ref, 100_000, est40, sized(varHeight), Class("h-[400px] p-3")), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	s := vl.scroller()
	for _, c := range []struct {
		i     int
		align ScrollAlign
	}{
		{500, AlignStart}, {50_000, AlignCenter}, {77_777, AlignEnd}, {99_999, AlignEnd}, {0, AlignStart},
		{99_999, AlignStart}, {12, AlignCenter}, {200_000, AlignEnd}, {-5, AlignStart},
	} {
		ref.ScrollToIndex(c.i, c.align)
		settleTree(t, tr)
		i := min(max(c.i, 0), 99_999)
		top, h := vl.port()
		y := itemTop(t, vl, i)
		size := vl.sizes.at(i)
		if size != varHeight(i) {
			t.Fatalf("%d %v: the item is not measured (%v)", c.i, c.align, size)
		}
		var ok bool
		switch {
		case i == 0 && c.align == AlignStart:
			ok = s.pos.Y == 0
		case i == 99_999 && c.align == AlignEnd:
			ok = s.pos.Y == s.max.Y
		case s.pos.Y == s.max.Y && c.align != AlignEnd:
			ok = true // the end of the list stops it
		case c.align == AlignStart:
			ok = near(y, top, 0.01)
		case c.align == AlignCenter:
			ok = near(y+size/2, top+h/2, 0.01)
		case c.align == AlignEnd:
			ok = near(y+size, top+h, 0.01)
		}
		if !ok {
			t.Errorf("ScrollToIndex(%d, %v): item at %v (%v px) in a viewport at %v (%v px), offset %v of %v",
				c.i, c.align, y, size, top, h, s.pos.Y, s.max.Y)
		}
		checkContiguous(t, vl)
	}
}

func TestVirtualScrollToIndexAuto(t *testing.T) {
	var ref ElementRef
	tr := newTestTree(t, 400, 600, refList(&ref, 1000, est40, sized(est40), Class("h-[400px]")), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	s := vl.scroller()
	tr.setScroll(s, gfx.Point{Y: 2000}) // rows 50..59 visible
	settleTree(t, tr)
	ref.ScrollToIndex(55, AlignAuto) // visible: nothing moves
	settleTree(t, tr)
	if s.pos.Y != 2000 {
		t.Errorf("a visible row scrolled the list to %v", s.pos.Y)
	}
	ref.ScrollToIndex(80, AlignAuto) // below: aligned to the end
	settleTree(t, tr)
	if s.pos.Y != 81*40-400 {
		t.Errorf("row 80: offset %v, want %v", s.pos.Y, 81*40-400)
	}
	ref.ScrollToIndex(10, AlignAuto) // above: aligned to the start
	settleTree(t, tr)
	if s.pos.Y != 400 {
		t.Errorf("row 10: offset %v, want 400", s.pos.Y)
	}
	ref.ScrollToIndex(10, ScrollAlign(99)) // unknown: auto
	settleTree(t, tr)
	if s.pos.Y != 400 {
		t.Errorf("row 10 again: offset %v, want 400", s.pos.Y)
	}
}

func TestVirtualScrollToIndexBeforeLayout(t *testing.T) {
	// A ScrollToIndex while the layout is stale waits for the next frame.
	var ref ElementRef
	var setN func(int)
	root := Component(func() Node {
		n, set := UseStateSet(10)
		setN = set
		r := UseElementRef()
		ref = r
		return div(VirtualList(n, est40, sized(varHeight), AttachRef(r), Class("h-[400px]")))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	setN(10_000)
	tr.ApplyUpdates()
	ref.ScrollToIndex(5000, AlignStart) // the list still has 10 items
	settleTree(t, tr)
	vl := onlyList(t, tr)
	top, _ := vl.port()
	if y := itemTop(t, vl, 5000); !near(y, top, 0.01) {
		t.Errorf("row 5000 at %v, want %v", y, top)
	}
}

func TestVirtualScrollToIndexSmoothAndKeys(t *testing.T) {
	var ref ElementRef
	root := refList(&ref, 100_000, est40, sized(varHeight), Class("h-[400px] scroll-smooth"))
	x := newScrollHost(t, root, 400, 600, Config{})
	x.w.SetFocused(true)
	x.settle()
	tr := x.tree()
	vl := onlyList(t, tr)
	s := vl.scroller()
	ref.ScrollToIndex(30_000, AlignCenter)
	x.frame()
	if !(s.pos.Y > 0 && s.pos.Y < 1_000_000) || !s.animating() {
		t.Errorf("a smooth ScrollToIndex jumped: offset %v, animating %v", s.pos.Y, s.animating())
	}
	x.settle()
	top, h := vl.port()
	if y := itemTop(t, vl, 30_000); !near(y+vl.sizes.at(30_000)/2, top+h/2, 0.01) {
		t.Errorf("row 30000 at %v (%v px), want it centered in %v + %v", y, vl.sizes.at(30_000), top, h)
	}
	// End reaches the end, Home the start, though the rows at the end
	// are measured on the way.
	x.press(platform.KeyEnd, platform.CodeEnd, 0)
	x.settle()
	if s.pos.Y != s.max.Y || s.max.Y != float32(vl.sizes.total())-400 {
		t.Errorf("End: offset %v, max %v, total %v", s.pos.Y, s.max.Y, vl.sizes.total())
	}
	if top, h := vl.port(); itemTop(t, vl, 99_999)+vl.sizes.at(99_999) != top+h {
		t.Errorf("End: the last row is not at the bottom")
	}
	x.press(platform.KeyHome, platform.CodeHome, 0)
	x.settle()
	if s.pos.Y != 0 {
		t.Errorf("Home: offset %v", s.pos.Y)
	}
	// Page Down moves by 87.5% of the viewport.
	x.press(platform.KeyPageDown, platform.CodePageDown, 0)
	x.settle()
	if s.pos.Y != 350 {
		t.Errorf("Page Down: offset %v, want 350", s.pos.Y)
	}
	checkContiguous(t, vl)
	x.idle()
}

func TestVirtualScrollToIndexNoOps(t *testing.T) {
	var ref ElementRef
	tr := newTestTree(t, 400, 600, refList(&ref, 0, est40, sized(est40), Class("h-[400px]")), nil)
	settleTree(t, tr)
	ref.ScrollToIndex(3, AlignStart) // no items
	var plain ElementRef
	ElementRef{}.ScrollToIndex(1, AlignStart) // the zero ref
	tr2 := newTestTree(t, 400, 600, Component(func() Node {
		plain = UseElementRef()
		return div(AttachRef(plain), Class("h-[100px] overflow-auto"))
	}), nil)
	settleTree(t, tr2)
	plain.ScrollToIndex(1, AlignStart) // not a list
	if tr.Dirty() || tr2.Dirty() {
		t.Error("a no-op ScrollToIndex made the trees dirty")
	}
}

// ---------------------------------------------------------------------
// Focus and keyboard.

func btnRow(i int) Node {
	return button(Class("block h-[40px] w-full"), Key(i), Textf("Row %d", i))
}

func TestVirtualFocusedItemStaysMounted(t *testing.T) {
	x := newScrollHost(t, div(VirtualList(10_000, est40, btnRow, Class("h-[400px]"))), 400, 600, Config{InstantScroll: true})
	x.w.SetFocused(true)
	x.settle()
	tr := x.tree()
	vl := onlyList(t, tr)
	x.tab()
	x.tab()
	x.settle()
	if el := tr.Focused(); el == nil || keyName(el) != "1" {
		t.Fatalf("focused %s, want row 1", keyName(tr.Focused()))
	}
	focused := tr.Focused()
	tr.setScroll(vl.scroller(), gfx.Point{Y: 200_000})
	x.settle()
	if !focused.mounted || tr.Focused() != focused {
		t.Fatalf("the focused row was unmounted while scrolled away")
	}
	if vl.built[0] != 0 || vl.built[1] != 1 || vl.built[2] != 2 || vl.lo != 4995 {
		t.Errorf("built %v...", vl.built[:4])
	}
	checkContiguous(t, vl)
	// Tab goes on from the focused row and brings it into view.
	x.tab()
	x.settle()
	if keyName(tr.Focused()) != "2" {
		t.Fatalf("Tab focused %s, want row 2", keyName(tr.Focused()))
	}
	top, h := vl.port()
	if y := itemTop(t, vl, 2); y < top || y+40 > top+h {
		t.Errorf("row 2 at %v, outside the viewport %v + %v", y, top, h)
	}
}

func TestVirtualTabWalksTheList(t *testing.T) {
	x := newScrollHost(t, div(VirtualList(10_000, est40, btnRow, Class("h-[400px]"))), 400, 600, Config{InstantScroll: true})
	x.w.SetFocused(true)
	x.settle()
	tr := x.tree()
	vl := onlyList(t, tr)
	// A frame runs between two keys (Tab cannot reach an item that is
	// not built yet: the list moves its window after the focused item is
	// revealed, in the next frame).
	for i := 0; i < 60; i++ {
		x.tab()
		x.frame()
	}
	x.settle()
	if keyName(tr.Focused()) != "59" {
		t.Fatalf("after 60 Tabs focused %s, want row 59", keyName(tr.Focused()))
	}
	top, h := vl.port()
	if y := itemTop(t, vl, 59); y < top || y+40 > top+h {
		t.Errorf("row 59 at %v, outside the viewport %v + %v", y, top, h)
	}
	for i := 0; i < 30; i++ {
		x.shiftTab()
		x.frame()
	}
	x.settle()
	if keyName(tr.Focused()) != "29" {
		t.Fatalf("after 30 Shift+Tabs focused %s, want row 29", keyName(tr.Focused()))
	}
	x.idle()
}

// ---------------------------------------------------------------------
// Props and data changes.

func TestVirtualCountChanges(t *testing.T) {
	var setN func(int)
	root := Component(func() Node {
		n, set := UseStateSet(100)
		setN = set
		return div(VirtualList(n, est40, sized(varHeight), Class("h-[400px]")))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	if vl.sizes.at(3) != varHeight(3) {
		t.Fatal("row 3 not measured")
	}
	setN(200) // appended rows: the measurements stay
	settleTree(t, tr)
	if vl.sizes.len() != 200 || vl.sizes.at(3) != varHeight(3) || vl.sizes.at(150) != 40 {
		t.Errorf("after growing: %d items, row 3 %v, row 150 %v", vl.sizes.len(), vl.sizes.at(3), vl.sizes.at(150))
	}
	tr.setScroll(vl.scroller(), gfx.Point{Y: 5000})
	settleTree(t, tr)
	setN(3) // shrinks below the window
	settleTree(t, tr)
	if vl.lo != 0 || vl.hi != 2 || vl.scroller().pos.Y != 0 {
		t.Errorf("after shrinking: built %d..%d, offset %v", vl.lo, vl.hi, vl.scroller().pos.Y)
	}
	setN(0)
	settleTree(t, tr)
	if len(vl.built) != 0 || vl.sizes.total() != 0 {
		t.Errorf("empty list: built %v, total %v", vl.built, vl.sizes.total())
	}
	checkContiguous(t, vl)
}

func est60(int) float32 { return 60 }

func TestVirtualNewEstimateFunctionResets(t *testing.T) {
	var setEst func(func(int) float32)
	root := Component(func() Node {
		est, set := UseStateSet(est40)
		setEst = set
		return div(VirtualList(1000, est, sized(varHeight), Class("h-[400px]")))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	setEst(est60)
	settleTree(t, tr)
	if vl.sizes.at(500) != 60 || vl.sizes.at(1) != varHeight(1) {
		t.Errorf("after a new estimate: row 500 %v, row 1 %v", vl.sizes.at(500), vl.sizes.at(1))
	}
}

// labelRow is an item with comparable props.
type labelProps struct {
	I     int
	Label string
}

var labelBuilds int

func labelRow(p labelProps) Node {
	labelBuilds++
	return div(Class("h-[40px]"), Textf("%s %d", p.Label, p.I))
}

func TestVirtualParentRebuild(t *testing.T) {
	var setLabel, setOther func(string)
	root := Component(func() Node {
		label, set := UseStateSet("Row")
		other, set2 := UseStateSet("")
		setLabel, setOther = set, set2
		return div(Textf("%s", other), VirtualList(1000, est40, func(i int) Node {
			return ComponentProps(labelRow, labelProps{i, label})
		}, Class("h-[400px]")))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	n := len(vl.built)
	labelBuilds = 0
	setOther("x") // renderItem runs again; the rows' props are equal
	settleTree(t, tr)
	if labelBuilds != 0 {
		t.Errorf("%d rows built after an unrelated change", labelBuilds)
	}
	setLabel("Item")
	settleTree(t, tr)
	if labelBuilds != n {
		t.Errorf("%d rows built after a new label, want %d", labelBuilds, n)
	}
	sem := semantics.Snapshot(tr.Semantics())
	if !strings.Contains(sem, "Item 3") {
		t.Errorf("the rows did not follow the label:\n%s", sem)
	}
}

func TestVirtualValueReadInRenderItem(t *testing.T) {
	var v *Value[string]
	root := Component(func() Node {
		v = UseValue("a")
		return div(VirtualList(100, est40, func(i int) Node {
			return div(Class("h-[40px]"), Textf("%s%d", v.Get(), i))
		}, Class("h-[400px]")))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	v.Set("b")
	settleTree(t, tr)
	if sem := semantics.Snapshot(tr.Semantics()); !strings.Contains(sem, "b4") || strings.Contains(sem, "a4") {
		t.Errorf("the rows did not follow the Value:\n%s", sem)
	}
}

// ---------------------------------------------------------------------
// Placement, semantics and edge cases.

func TestVirtualListWithoutItsOwnScroll(t *testing.T) {
	// No height and no scrolling: the list is 40,000 px tall in a page
	// that scrolls, and builds only what the window shows.
	tr := newTestTree(t, 400, 600, div(Class("pt-[100px]"), VirtualList(1000, est40, sized(est40), Class("overflow-visible"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	if vl.lo != 0 || vl.hi != 17 {
		t.Fatalf("built %d..%d, want 0..17 (rows under the 500 px of window below the padding)", vl.lo, vl.hi)
	}
	tr.setScroll(tr.rootBox.scr, gfx.Point{Y: 20_100}) // the page scrolls to row 500
	settleTree(t, tr)
	if vl.lo != 495 || vl.hi != 519 {
		t.Errorf("built %d..%d after the page scrolled, want 495..519", vl.lo, vl.hi)
	}
	checkContiguous(t, vl)
}

func TestVirtualListInsideAScroller(t *testing.T) {
	// A list with no height of its own inside a scroll container: bounded
	// by the container's viewport.
	root := div(Class("h-[300px] overflow-auto"), Key("outer"), VirtualList(1000, est40, sized(est40), Class("overflow-visible")))
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	if vl.lo != 0 || vl.hi != 12 {
		t.Fatalf("built %d..%d, want 0..12", vl.lo, vl.hi)
	}
	tr.setScroll(keyed(tr, "outer").box.scr, gfx.Point{Y: 4000})
	settleTree(t, tr)
	if vl.lo != 95 || vl.hi != 112 {
		t.Errorf("built %d..%d, want 95..112", vl.lo, vl.hi)
	}
}

func TestVirtualHiddenList(t *testing.T) {
	var setShown func(bool)
	root := Component(func() Node {
		shown, set := UseStateSet(false)
		setShown = set
		c := "h-[400px]"
		if !shown {
			c += " hidden"
		}
		return div(VirtualList(1000, est40, sized(est40), Class(c)))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	if len(vl.built) != 0 {
		t.Errorf("a hidden list built %v", vl.built)
	}
	setShown(true)
	settleTree(t, tr)
	if vl.lo != 0 || vl.hi != 14 {
		t.Errorf("shown: built %d..%d", vl.lo, vl.hi)
	}
	checkContiguous(t, vl)
	// Hidden again, it keeps its items (and their state).
	setShown(false)
	settleTree(t, tr)
	if vl.lo != 0 || vl.hi != 14 {
		t.Errorf("hidden again: built %d..%d", vl.lo, vl.hi)
	}
	// A list under a hidden ancestor builds nothing either.
	tr2 := newTestTree(t, 400, 600, div(Class("hidden"), VirtualList(1000, est40, sized(est40), Class("h-[400px]"))), nil)
	settleTree(t, tr2)
	if vl2 := onlyList(t, tr2); len(vl2.built) != 0 {
		t.Errorf("a list in a hidden div built %v", vl2.built)
	}
}

func TestVirtualFlexColumnList(t *testing.T) {
	// The common page: a header and a list that takes the rest.
	root := div(Class("flex h-screen flex-col"),
		div(Class("h-[100px] shrink-0")),
		VirtualList(1000, est40, sized(est40), Class("flex flex-1 flex-col p-2")))
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	_, content := vl.boxes()
	if h := content.ld.Layout().Size.H; h != 40_000 {
		t.Errorf("the content is %v px in a flex list, want 40,000", h)
	}
	s := vl.scroller()
	if s.client.H != 500 || s.max.Y != 40_000+16-500 {
		t.Errorf("client %v, max %v", s.client.H, s.max.Y)
	}
	if vl.hi != 17 {
		t.Errorf("built up to %d, want 17", vl.hi)
	}
	checkContiguous(t, vl)
}

func TestVirtualEdgeEstimates(t *testing.T) {
	nan := float32(math.NaN())
	for name, est := range map[string]func(int) float32{
		"zero": func(int) float32 { return 0 }, "NaN": func(int) float32 { return nan },
		"negative": func(int) float32 { return -40 }, "huge": func(int) float32 { return float32(math.Inf(1)) },
	} {
		tr := newTestTree(t, 400, 600, div(VirtualList(10_000, est, sized(est40), Class("h-[400px]"))), nil)
		settleTree(t, tr)
		vl := onlyList(t, tr)
		if len(vl.built) == 0 || len(vl.built) > 500 {
			t.Errorf("%s estimates: %d items built", name, len(vl.built))
		}
		tr.setScroll(vl.scroller(), gfx.Point{Y: 1000})
		settleTree(t, tr)
		checkContiguous(t, vl)
	}
}

func TestVirtualTrimsAnOversizedWindow(t *testing.T) {
	// Estimates of 1 px for 40 px rows: the second window holds hundreds
	// of rows, then the measured sizes trim it.
	tiny := func(int) float32 { return 1 }
	tr := newTestTree(t, 400, 600, div(VirtualList(10_000, tiny, sized(est40), Class("h-[400px]"))), nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	if vl.lo != 0 || vl.hi != 14 {
		t.Errorf("built %d..%d, want 0..14", vl.lo, vl.hi)
	}
	// A list that gets shorter keeps its rows until they are more than
	// the overscan too many. The height is a deferred prop, so the list is
	// laid out again but not built by its parent.
	var h *Value[float32]
	tr = newTestTree(t, 400, 600, Component(func() Node {
		h = UseValue(float32(400))
		return div(VirtualList(1000, est40, sized(est40), Style(style.HeightFn(h.Get))))
	}), nil)
	settleTree(t, tr)
	vl = onlyList(t, tr)
	h.Set(280) // 7 rows: 0..11 wanted, 0..14 is within the overscan
	settleTree(t, tr)
	if vl.hi != 14 {
		t.Errorf("built up to %d after a small shrink, want 14", vl.hi)
	}
	h.Set(80) // 2 rows: 0..6 wanted
	settleTree(t, tr)
	if vl.hi != 6 {
		t.Errorf("built up to %d after a large shrink, want 6", vl.hi)
	}
	checkContiguous(t, vl)
}

func TestVirtualClassicScrollbars(t *testing.T) {
	text := strings.Repeat("word ", 12)
	render := func(i int) Node { return p(Textf("%d %s", i, text)) }
	tr := NewTree(Config{Fonts: testFonts, ClassicScrollbars: true})
	tr.SetViewport(gfx.Size{W: 300, H: 600})
	tr.Mount(div(VirtualList(1000, est40, render, Class("h-[400px]"))))
	settleTree(t, tr)
	vl := onlyList(t, tr)
	if s := vl.scroller(); !s.gutterV {
		t.Error("no classic gutter on an overflowing list")
	}
	checkContiguous(t, vl)
}

func TestVirtualAriaSnapshot(t *testing.T) {
	render := func(i int) Node {
		return div(Class("h-[40px]"), Textf("Row %d", i))
	}
	tr := newTestTree(t, 400, 600, div(VirtualList(1000, est40, render, Role("list"), AriaLabel("Rows"), Class("h-[120px]"), Overscan(1))), nil)
	settleTree(t, tr)
	got := semantics.Snapshot(tr.Semantics())
	want := "- list \"Rows\":\n  - listitem: Row 0\n  - listitem: Row 1\n  - listitem: Row 2\n  - listitem: Row 3"
	if got != want {
		t.Errorf("aria snapshot:\n%s\nwant:\n%s", got, want)
	}
}

func TestVirtualKeyAndIdentity(t *testing.T) {
	var setFirst func(bool)
	root := Component(func() Node {
		first, set := UseStateSet(false)
		setFirst = set
		return div(If(first, div(Text("first"))), Map([]int{1}, func(int) Node {
			return VirtualList(100, est40, rowNode, Key("list"), Class("h-[400px]"))
		}))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	vl := onlyList(t, tr)
	if k, ok := vl.el.node.Key(); !ok || k != "list" {
		t.Fatalf("the list's node has key %v", k)
	}
	tr.setScroll(vl.scroller(), gfx.Point{Y: 800})
	settleTree(t, tr)
	setFirst(true)
	settleTree(t, tr)
	if onlyList(t, tr) != vl || vl.scroller().pos.Y != 800 {
		t.Errorf("the keyed list lost its state")
	}
}

func TestVirtualPanics(t *testing.T) {
	for name, tc := range map[string]struct {
		f    func()
		want string
	}{
		"negative count":    {func() { VirtualList(-1, est40, rowNode) }, "0 or more"},
		"nil estimate":      {func() { VirtualList(1, nil, rowNode) }, "estimateSize"},
		"nil render":        {func() { VirtualList(1, est40, nil) }, "renderItem"},
		"Overscan on Div":   {func() { div(Overscan(2)) }, "prop of VirtualList"},
		"negative Overscan": {func() { Overscan(-1) }, "0 or more"},
		"Src":               {func() { VirtualList(1, est40, rowNode, Href("x")) }, "only A"},
		"hook in renderItem": {func() {
			tr := newTestTree(t, 400, 600, div(VirtualList(10, est40, func(i int) Node {
				UseState(i)
				return div()
			}, Class("h-[100px]"))), nil)
			tr.Frame()
		}, "renderItem"},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("%s: no panic", name)
				} else if !strings.Contains(fmt.Sprint(r), tc.want) {
					t.Errorf("%s: panic %q does not mention %q", name, r, tc.want)
				}
			}()
			tc.f()
		}()
	}
}

func TestVirtualUnmount(t *testing.T) {
	var setShown func(bool)
	root := Component(func() Node {
		shown, set := UseStateSet(true)
		setShown = set
		return div(If(shown, VirtualList(100, est40, rowNode, Class("h-[400px]"))))
	})
	tr := newTestTree(t, 400, 600, root, nil)
	settleTree(t, tr)
	setShown(false)
	settleTree(t, tr)
	if len(tr.virt.lists) != 0 {
		t.Errorf("%d lists after unmounting", len(tr.virt.lists))
	}
}

func TestVirtualIdle(t *testing.T) {
	x := newScrollHost(t, div(VirtualList(100_000, est40, sized(varHeight), Class("h-[400px]"))), 400, 600, Config{})
	x.settle()
	vl := onlyList(t, x.tree())
	x.wheel(notch(100, 100, 3))
	x.settle()
	if vl.scroller().pos.Y != 3*wheelNotch {
		t.Errorf("offset %v after three notches", vl.scroller().pos.Y)
	}
	checkContiguous(t, vl)
	x.idle()
}
