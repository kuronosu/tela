package core

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Bases of damaged rectangles (ADR-275).
//
// A damaged rectangle X of the new frame B is rasterized by rendering the
// list over X. When the pixels of X differ only from some list position p
// on (a dialog or a palette that animates above an unchanged page, the
// rows of a table that scroll over a fixed glass pane, a scrim laid over
// the page), the commands before p paint the same pixels there in both
// frames, and rendering them again is most of the cost: the wallpaper, the
// glass panes and their blur. A base is the pixels of the region the
// render reads, g (X grown by the reach of the backdrop filters from p on,
// raster.BackdropCache.RegionFrom), as the commands before p leave them;
// the rectangle is then rendered from p on over its base
// (raster.Options.From and Base), bit for bit what a full render gives.
//
// Two pixel sources can be a base:
//
//   - the framebuffer, which holds the old frame A: when the commands of A
//     that touch g are all matched with commands of B before p, and the
//     commands of B before p that touch g are all matched (a scrim or a
//     dialog appended above the page);
//   - a snapshot taken by the previous frame at position q of A
//     (raster.Snapshot): when the commands of A before q and those of B
//     before p that touch g are matched with each other, one to one.
//
// Matched commands are equal with equal bounds, and a command whose
// bounds miss g does not write g, so in both cases the commands that
// write g before the base's position are the same, in the same order (the
// pop of a clip or a transform paints nothing: the end of a block around
// p need not be matched before it). What they read beyond g is the
// backdrop filters' input: the backdrop closure damages, from the
// filter's position, the output of a filter whose input changed, so a
// base is refused when such damage before p meets g, or when a shift
// meets g (its pixels moved). The framebuffer is a base only when g meets
// no other rectangle or shift of the frame; those rectangles are
// rasterized after the ones it is the base of. A rectangle whose
// framebuffer base is spoiled in a few places (the toasts' region above a
// scrim) is split: the parts away from them keep the base.
//
// p is the first position at which a damaged rectangle meeting X starts to
// differ (damager.rpos). A frame takes a snapshot at p of each rectangle
// it rasterizes without one, when what lies under it is costly to draw
// again (worthSnap), over the rectangle and a margin for an element that
// grows (Options.Snap, or a copy of the framebuffer's base); it keeps the
// snapshot it rendered from, valid for the next frame too, so that the
// frames of one interaction find them; a full frame takes one of the
// whole frame at the first difference. The snapshots of one tree hold at
// most one framebuffer's worth of pixels, are dropped when a frame does
// not keep them, and TrimRaster drops them at rest.

// Bases of a rectangle (basePlan.base): none, the framebuffer, or the index
// of a snapshot of layerState.snaps.
const (
	baseNone = -1
	baseFB   = -2
)

// basePlan is how rasterizeApp renders a damaged rectangle.
type basePlan struct {
	from int             // raster.Options.From (0: the whole list)
	base int             // baseNone, baseFB or a snapshot's index
	snap bool            // take a snapshot at from for the next frame
	read image.Rectangle // the pixels the render reads (RegionFrom)
	// region, when not empty, is the region rendered: the rectangle and a
	// margin for its snapshot.
	region image.Rectangle
	// keep is, for a snapshot base, the part of the snapshot whose pixels
	// are those before from in the new frame too (at least read), and for
	// the framebuffer, the pixels the snapshot copies from it.
	keep image.Rectangle
}

// snapEntry is a snapshot of the last rasterized frame: the pixels of rect
// before its command at.
type snapEntry struct {
	at   int
	rect image.Rectangle // the valid part of s.Rect
	s    raster.Snapshot
}

// image returns the valid pixels of e.
func (e *snapEntry) image() *image.RGBA {
	return e.s.Image().SubImage(e.rect).(*image.RGBA)
}

// rectStart returns the list position from which the pixels of damaged
// rectangle m differ: the new command its rpos names, 0 for what may
// differ from the start.
func (d *damager) rectStart(m int) int {
	if p := d.rpos[m]; p >= 0 {
		return min((p+1)/2, d.b.dl.Len())
	}
	return 0
}

// startOf returns the first list position from which the pixels of x may
// differ (the smallest rectStart of the damaged rectangles meeting it),
// moved back to the outermost layer open there (a render cannot start
// inside a layer), or 0.
func (d *damager) startOf(x image.Rectangle) int {
	p := d.b.dl.Len()
	for m, r := range d.rects {
		if r.Overlaps(x) {
			p = min(p, d.rectStart(m))
		}
	}
	return layerStart(d.b.dl, p)
}

// layerStart returns at, or the index of the outermost layer open before
// command at.
func layerStart(dl *gfx.DisplayList, at int) int {
	enc, _ := enclosingAll(dl, at)
	for _, i := range enc {
		if _, ok := dl.At(i).(gfx.PushLayer); ok {
			return i
		}
	}
	return at
}

// devs returns the device bounds of every command of a frame (devA or
// devB), computed once per frame in buf.
func devs(n int, dev func(int) image.Rectangle, buf *[]image.Rectangle) []image.Rectangle {
	if len(*buf) == n {
		return *buf
	}
	*buf = resized(*buf, n)
	for i := range n {
		(*buf)[i] = dev(i)
	}
	return *buf
}

// baseBad returns the parts of g whose pixels, as the commands of the new
// frame before p leave them, may differ from those the commands of the old
// frame before q left there (see the file's documentation): the parts of g
// that a shift or a shifted unit meets, that the grown output of a
// backdrop filter before p meets, and that a command meets which breaks
// the one-to-one matching (a new command before p that is not matched
// with an old one before q, an old one before q not matched with a new
// one before p). With first, it returns at the first one; it appends to
// out.
func (d *damager) baseBad(g image.Rectangle, p, q int, shifts []Shift, first bool, out []image.Rectangle) []image.Rectangle {
	s := d.scratch
	da := devs(d.a.dl.Len(), d.devA, &s.devA)
	db := devs(d.b.dl.Len(), d.devB, &s.devB)
	found := func(r image.Rectangle) bool {
		if r = r.Intersect(g); !r.Empty() {
			out = append(out, r)
			return first
		}
		return false
	}
	for _, sh := range shifts {
		if found(sh.Rect) || found(sh.Rect.Sub(image.Pt(sh.DX, sh.DY))) {
			return out
		}
	}
	for k := range d.ub {
		if found(d.ub[k].r) {
			return out
		}
	}
	for m, r := range d.rects {
		if d.rkind[m] == rectOther && d.rpos[m] < 2*p && found(r) {
			return out
		}
	}
	A, B := d.a.dl, d.b.dl
	for j := range p {
		if i := s.pairB[j]; (i < 0 || i >= q) && !paintsNothing(B, j) && found(db[j]) {
			return out
		}
	}
	for i := range q {
		if j := s.pairA[i]; (j < 0 || j >= p) && !paintsNothing(A, i) && found(da[i]) {
			return out
		}
	}
	return out
}

// paintsNothing reports whether command i of dl is the pop of a clip or a
// transform: it paints nothing (its push sets the state of what is drawn
// inside, and is checked with them), so the end of a block that encloses
// a base's position need not be matched before it.
func paintsNothing(dl *gfx.DisplayList, i int) bool {
	switch dl.At(i).(type) {
	case gfx.PopClip, gfx.PopTransform:
		return true
	}
	return false
}

// baseOK reports whether the pixels of g before p in the new frame are
// those before q in the old one (baseBad finds no part of g).
func (d *damager) baseOK(g image.Rectangle, p, q int, shifts []Shift) bool {
	return len(d.baseBad(g, p, q, shifts, true, nil)) == 0
}

// snapMargin is how far, in logical pixels, a snapshot reaches beyond the
// rectangle it is taken for, so that the next frame of an element that
// grows (a dialog or a palette that scales in) still finds it.
const snapMargin = 16

// plan chooses the base of each rectangle (see the file's documentation).
// It returns the rectangles to rasterize, their plans and the cost of
// rasterizing them, in pixels: the pixels each one reads, a quarter of
// them for a rectangle rendered from a base (it draws only what lies above
// it). A rectangle whose framebuffer base is spoiled by a few small parts
// (the toasts' region above a scrim that covers the window) is split: the
// parts that keep the base are rendered from it, the others from the
// start. A rectangle rendered from the start that takes a snapshot is
// rendered with a margin of snapMargin around it.
func (d *damager) plan(rects []image.Rectangle, shifts []Shift) ([]image.Rectangle, []basePlan, int) {
	s := d.scratch
	B := d.b.dl
	nA := d.a.dl.Len()
	margin := int(math.Ceil(snapMargin * d.scale))
	var outR []image.Rectangle
	var outP []basePlan
	cost := 0
	none := func(x image.Rectangle) {
		g := s.bdc.Region(B, d.b.scale, x, d.bounds)
		outR, outP = append(outR, x), append(outP, basePlan{base: baseNone, read: g})
		cost += g.Dx() * g.Dy()
	}
	based := func(x image.Rectangle, pl basePlan) {
		outR, outP = append(outR, x), append(outP, pl)
		cost += pl.read.Dx() * pl.read.Dy() / 4
	}
	for k, x := range rects {
		p := d.startOf(x)
		if p == 0 {
			none(x)
			continue
		}
		read := s.bdc.RegionFrom(B, d.b.scale, x, d.bounds, p)
		pl := basePlan{from: p, base: baseNone, read: read}
		for e := range s.snaps {
			en := &s.snaps[e]
			if !read.In(en.rect) {
				continue
			}
			// The whole snapshot stays valid when nothing before from
			// changed in it, so that the next frame finds the pixels its
			// rectangle grows into (a scroll's damage moves).
			if d.baseOK(en.rect, p, en.at, shifts) {
				pl.base, pl.keep = e, en.rect
				break
			}
			if d.baseOK(read, p, en.at, shifts) {
				pl.base, pl.keep = e, read
				break
			}
		}
		if pl.base >= 0 {
			based(x, pl)
			continue
		}
		if d.alone(k, read, rects) && d.baseOK(read, p, nA, shifts) {
			pl.base = baseFB
			if d.worthSnap(read, p) {
				pl.snap, pl.keep = true, read
				if g := s.bdc.RegionFrom(B, d.b.scale, x.Inset(-margin).Intersect(d.bounds), d.bounds, p); d.alone(k, g, rects) && d.baseOK(g, p, nA, shifts) {
					pl.keep = g
				}
			}
			based(x, pl)
			continue
		}
		if d.split(k, x, p, rects, shifts, none, based) {
			continue
		}
		if !d.worthSnap(read, p) {
			none(x)
			continue
		}
		// The margin is rendered (its pixels did not change: they are
		// written with their values) but not presented.
		rg := x.Inset(-margin).Intersect(d.bounds)
		g := s.bdc.Region(B, d.b.scale, rg, d.bounds)
		outR = append(outR, x)
		outP = append(outP, basePlan{from: p, base: baseNone, snap: true, read: g, region: rg})
		cost += g.Dx() * g.Dy()
	}
	return outR, outP, cost
}

// split renders rectangle x, the k-th of rects, from the framebuffer where
// its pixels before p keep their values (see plan), and reports whether it
// did: the parts of x away from what spoils the base (baseBad and the
// other rectangles), if they keep at least half of it. It calls based for
// them and none for the rest of x.
func (d *damager) split(k int, x image.Rectangle, p int, rects []image.Rectangle, shifts []Shift, none func(image.Rectangle), based func(image.Rectangle, basePlan)) bool {
	s := d.scratch
	B := d.b.dl
	nA := d.a.dl.Len()
	read := s.bdc.RegionFrom(B, d.b.scale, x, d.bounds, p)
	bad := d.baseBad(read, p, nA, shifts, false, nil)
	for m, r := range rects {
		if m != k && r.Overlaps(read) {
			bad = append(bad, r.Intersect(read))
		}
	}
	if len(bad) > maxSplitBad {
		return false
	}
	pieces := []image.Rectangle{x}
	for _, b := range bad {
		var next []image.Rectangle
		for _, pc := range pieces {
			next = append(next, subtract(pc, b)...)
		}
		if pieces = next; len(pieces) > maxSplitBad {
			return false
		}
	}
	var good []image.Rectangle
	var reads []image.Rectangle
	area := 0
	for _, pc := range pieces {
		g := s.bdc.RegionFrom(B, d.b.scale, pc, d.bounds, p)
		if d.alone(k, g, rects) && d.baseOK(g, p, nA, shifts) {
			good, reads = append(good, pc), append(reads, g)
			area += pc.Dx() * pc.Dy()
		}
	}
	if 2*area < x.Dx()*x.Dy() {
		return false
	}
	rest := []image.Rectangle{x}
	for _, gd := range good {
		var next []image.Rectangle
		for _, r := range rest {
			next = append(next, subtract(r, gd)...)
		}
		rest = next
	}
	for i, gd := range good {
		based(gd, basePlan{from: p, base: baseFB, read: reads[i]})
	}
	for _, r := range rest {
		none(r)
	}
	return true
}

// maxSplitBad bounds the parts split works with.
const maxSplitBad = 16

// worthSnap reports whether a snapshot of g at p is worth its two copies:
// whether what the commands before p draw in g, weighted by their cost
// per pixel (a backdrop filter blurs, an image samples, a shadow and a
// gradient evaluate a function), comes to at least snapWorth pixels of
// solid fill per pixel of g. The snapshots of a page of solid boxes are
// not taken: drawing them again costs about as much as copying.
func (d *damager) worthSnap(g image.Rectangle, p int) bool {
	db := devs(d.b.dl.Len(), d.devB, &d.scratch.devB)
	B := d.b.dl
	need := snapWorth * g.Dx() * g.Dy()
	cost := 0
	for j := range p {
		o := db[j].Intersect(g)
		if o.Empty() || B.Match(j) >= 0 {
			continue
		}
		w := 1
		switch c := B.At(j).(type) {
		case gfx.BackdropFilter:
			w = 8
		case gfx.DrawImage:
			w = 3
		case gfx.DrawShadow:
			w = 2
		case gfx.FillRect:
			w = paintCost(c.Paint)
		case gfx.FillRRect:
			w = paintCost(c.Paint)
		case gfx.FillPath:
			w = paintCost(c.Paint)
		}
		if cost += w * o.Dx() * o.Dy(); cost >= need {
			return true
		}
	}
	return false
}

// snapWorth is the cost per pixel above which worthSnap takes a snapshot.
const snapWorth = 4

// paintCost is the weight of a fill in worthSnap.
func paintCost(p gfx.Paint) int {
	if _, ok := p.(gfx.Color); ok {
		return 1
	}
	return 2
}

// alone reports whether g meets none of the rectangles but the k-th.
func (d *damager) alone(k int, g image.Rectangle, rects []image.Rectangle) bool {
	for m, r := range rects {
		if m != k && r.Overlaps(g) {
			return false
		}
	}
	return true
}

// finish merges the damage into a few rectangles and chooses their bases,
// or turns it into a full rasterization when rasterizing them (with what
// they read) costs most of a full one. A full frame takes a snapshot at
// the first position where the frames differ (Damage.snapAt).
func (d *damager) finish(shifts []Shift) Damage {
	full := Damage{Full: true}
	if p := d.startOf(d.bounds); p > 0 && d.worthSnap(d.bounds, p) {
		full.snapAt = p
	}
	rects, plans, area := d.plan(mergeRects(d.rects), shifts)
	for _, s := range shifts {
		area += s.Rect.Dx() * s.Rect.Dy() / 8 // a shift costs a copy
	}
	if float64(area) > fullFraction*float64(d.bounds.Dx()*d.bounds.Dy()) {
		return full
	}
	return Damage{Shifts: shifts, Rects: rects, plans: plans}
}

// ---------------------------------------------------------------------
// Rasterizing with bases.

// snapBudget is the most pixels the snapshots of a framebuffer of n pixels
// hold.
func snapBudget(n int) int { return n }

// renderRects rasterizes the rectangles of d into fb, from their bases,
// and keeps the snapshots the next frame may start from.
func (t *Tree) renderRects(fb *image.RGBA, r *raster.Renderer, f *frame, d Damage) {
	L := &t.lay
	old := L.snaps
	keep := L.snapBuf[:0]
	budget := snapBudget(fb.Rect.Dx() * fb.Rect.Dy())
	// The snapshots kept count first, so that new ones fit beside them.
	used := 0
	for k := range d.plans {
		if b := d.plans[k].base; b >= 0 && d.firstUse(k) {
			used += old[b].s.Rect.Dx() * old[b].s.Rect.Dy()
		}
	}
	canvas := t.clearColor()
	opts := raster.Options{Scale: f.scale, Backdrops: &L.bdc}
	// The rectangles whose base is the framebuffer first: their base is
	// what the old frame left there, which the others overwrite.
	order := make([]int, 0, len(d.Rects))
	for pass := range 2 {
		for k := range d.Rects {
			if fbBase := k < len(d.plans) && d.plans[k].base == baseFB; fbBase == (pass == 0) {
				order = append(order, k)
			}
		}
	}
	for _, k := range order {
		rc := d.Rects[k]
		var pl basePlan
		if k < len(d.plans) {
			pl = d.plans[k]
		} else {
			pl.base = baseNone
		}
		o := opts
		o.Region = rc
		var sn *raster.Snapshot
		if pl.snap && used+pl.keep.Dx()*pl.keep.Dy() <= budget {
			sn = &raster.Snapshot{At: pl.from, Pix: L.spareSnap()}
		}
		switch {
		case pl.base >= 0:
			e := &old[pl.base]
			o.From, o.Base = pl.from, e.image()
			r.Render(fb, f.dl, o)
			if !L.kept(keep, e) {
				// Its pixels are those before from in this frame too.
				keep = append(keep, snapEntry{at: pl.from, rect: pl.keep, s: e.s})
			}
		case pl.base == baseFB:
			if sn != nil {
				// The base is the framebuffer: take it before rendering.
				n := 4 * pl.keep.Dx() * pl.keep.Dy()
				if cap(sn.Pix) < n {
					sn.Pix = make([]uint8, n)
				}
				sn.Pix, sn.Rect = sn.Pix[:n], pl.keep
				copyPixels(sn.Image(), fb, pl.keep)
			}
			o.From = pl.from
			r.Render(fb, f.dl, o)
		default:
			if !pl.region.Empty() {
				rc = pl.region
				o.Region = rc
			}
			raster.Clear(fb, rc, canvas)
			o.Snap = sn
			r.Render(fb, f.dl, o)
		}
		if sn != nil && !sn.Rect.Empty() {
			used += sn.Rect.Dx() * sn.Rect.Dy()
			keep = append(keep, snapEntry{at: pl.from, rect: sn.Rect, s: *sn})
		} else if sn != nil {
			L.giveSnap(sn.Pix)
		}
	}
	L.replaceSnaps(keep)
}

// renderFull rasterizes f fully into fb, taking a snapshot at snapAt for
// the next frame when it is positive.
func (t *Tree) renderFull(fb *image.RGBA, r *raster.Renderer, f *frame, snapAt int) {
	L := &t.lay
	raster.Clear(fb, image.Rectangle{}, t.clearColor())
	o := raster.Options{Scale: f.scale, Backdrops: &L.bdc}
	keep := L.snapBuf[:0]
	if snapAt > 0 {
		sn := &raster.Snapshot{At: snapAt, Pix: L.spareSnap()}
		o.Snap = sn
		r.Render(fb, f.dl, o)
		if !sn.Rect.Empty() {
			keep = append(keep, snapEntry{at: snapAt, rect: sn.Rect, s: *sn})
		} else {
			L.giveSnap(sn.Pix)
		}
	} else {
		r.Render(fb, f.dl, o)
	}
	L.replaceSnaps(keep)
}

// firstUse reports whether the k-th rectangle of d is the first whose
// base is its snapshot.
func (d *Damage) firstUse(k int) bool {
	for m := range k {
		if d.plans[m].base == d.plans[k].base {
			return false
		}
	}
	return true
}

// kept reports whether keep holds a snapshot sharing e's pixels.
func (L *layerState) kept(keep []snapEntry, e *snapEntry) bool {
	for k := range keep {
		if len(keep[k].s.Pix) > 0 && len(e.s.Pix) > 0 && &keep[k].s.Pix[0] == &e.s.Pix[0] {
			return true
		}
	}
	return false
}

// replaceSnaps makes keep the snapshots of the frame just rasterized: the
// buffers of the others become the spare.
func (L *layerState) replaceSnaps(keep []snapEntry) {
	for k := range L.snaps {
		if !L.kept(keep, &L.snaps[k]) {
			L.giveSnap(L.snaps[k].s.Pix)
		}
	}
	clear(L.snaps)
	L.snaps, L.snapBuf = keep, L.snaps[:0]
}

// spareSnap returns the spare snapshot buffer (nil: none) and forgets it.
func (L *layerState) spareSnap() []uint8 {
	b := L.snapSpare
	L.snapSpare = nil
	return b
}

// giveSnap keeps the larger of b and the spare buffer as the spare.
func (L *layerState) giveSnap(b []uint8) {
	if cap(b) > cap(L.snapSpare) {
		L.snapSpare = b[:0]
	}
}

// dropSnaps releases the snapshots and the spare buffer.
func (L *layerState) dropSnaps() {
	clear(L.snaps)
	L.snaps = L.snaps[:0]
	L.snapSpare = nil
}

// snapBytes returns the bytes the snapshots and the spare buffer hold.
func (L *layerState) snapBytes() int {
	n := cap(L.snapSpare)
	for k := range L.snaps {
		n += cap(L.snaps[k].s.Pix)
	}
	return n
}

// copyPixels copies the pixels of rc from src to dst, whose bounds hold it.
func copyPixels(dst, src *image.RGBA, rc image.Rectangle) {
	n := 4 * rc.Dx()
	for y := rc.Min.Y; y < rc.Max.Y; y++ {
		copy(dst.Pix[dst.PixOffset(rc.Min.X, y):][:n], src.Pix[src.PixOffset(rc.Min.X, y):][:n])
	}
}
