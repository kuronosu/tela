package core

import (
	"image"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Damage between two frames (layer.go).
//
// # Matching
//
// The commands of the old frame A and the new frame B are matched in
// order (a common prefix and suffix, then a greedy search for the nearest
// equal pair after each difference): two commands match when they have
// equal values (gfx.Equal) and, for draw commands, equal bounds (the
// bounds of a push or a pop are those of its block, which follow its
// content; the content is matched on its own). A pixel that no unmatched
// command can touch has the same value in both frames: the draw commands
// whose bounds contain it are matched one to one, in the same order, and
// so are the pushes around them (a push's bounds contain its block, so an
// unmatched one would damage the pixel); they nest the same way and paint
// the same thing in the same state, and a block whose content does not
// touch the pixel leaves it alone (a group composites transparent pixels
// there). The damage is therefore the device bounds of
// the unmatched commands of both frames, with three corrections for what
// reads pixels away from where it writes:
//
//   - inside a layer with filters, a command reaches as far as the
//     filters' reach (gfx.DisplayList.LayerReach of every enclosing layer);
//   - a backdrop filter reads the pixels under its whole shape: when they
//     meet the damage, its output is damaged too (until nothing changes);
//   - rasterizing a damage rectangle as a region renders, on a scratch
//     image, the pixels that the backdrop filters reaching it read
//     (raster.BackdropRegion), so that its pixels are those of a full
//     rasterization; the cost of the fallback below counts them.
//
// # Scrolling
//
// When the offset of a scroll container changed by whole device pixels,
// the commands of its chunks (a unit: a run of consecutive commands of
// chunks of its scroll space) moved together by that amount Δ. Δ is the
// difference of the device positions the rasterizer gives the unit in the
// two frames (origin), composing the enclosing translations and the
// chunks' attachments with raster.GridMul: at a fractional scale a
// whole-pixel offset is only approximately a float32 logical value, and
// the rasterizer places such translations on the device pixel grid, so
// the shift is exact there too (ADR-075). If
//
//   - the unit is inside clips and integer translations only (no opacity
//     group, filter or other transform), the same in both frames, and R,
//     the device pixels that these clips cover fully, is not empty;
//   - everything painted before the unit that touches R is a solid fill
//     covering R, the same in both frames (the backdrop is uniform);
//
// then the pixels of R are shifted by Δ instead of rasterized, and the
// commands of the unit are matched by their values and their bounds in
// their own lists (not on screen). The damage then also holds: the pixels
// of R whose source is outside R (the exposed strips); the unit's pixels
// outside R (anti-aliased clip edges, rounded corners); the old commands
// of the unit that are unmatched, moved by Δ; everything painted after the
// unit that touches R (scrollbars, fixed and hoisted boxes), at both its
// old position moved by Δ and its new one; the backdrop filters after the
// unit that read R; and every command of the unit whose pixels are not
// provably the same under a whole-pixel translation: images, gradients,
// backdrop filters, glyphs drawn without the glyph cache, transforms other
// than integer translations (the rasterizer maps their pixels back through
// a float inverse). Shifts of different scroll containers never overlap.
//
// # Fallbacks
//
// A full rasterization when the damage covers most of the frame, or when
// the matching gives up (more than matchLookahead commands differ in a
// row: the rest of the frame is damaged).

// matchLookahead bounds the search for the next matching pair.
const matchLookahead = 48

// fullFraction is the share of the frame above which the damage becomes a
// full rasterization.
const fullFraction = 0.7

// maxDamageRects bounds the number of rasterized rectangles: beyond it,
// rectangles are merged.
const maxDamageRects = 48

// unit is a run of commands of chunks of one scroll space in a frame.
type unit struct {
	space      *scroller
	lead       *Box // the lead box of its first chunk
	start, end int
	// ok reports a valid shift; id numbers it among the valid ones, and
	// group is the id of the first unit of its group (findUnits).
	ok    bool
	id    int
	group int
	dx    int // device shift
	dy    int
	r     image.Rectangle
	enc   []int // the pushes open at start
}

// damager computes the damage between two frames.
type damager struct {
	a, b   *frame
	scale  float64
	bounds image.Rectangle
	// Per command of each frame: the valid unit it is in (-1: none), and
	// the enclosing filters' reach in device pixels.
	unitA, unitB   []int32
	reachA, reachB []int
	// The valid units: old and new runs.
	ua, ub []unit
	rects  []image.Rectangle
}

// frameDamage returns how to bring the pixels of frame a to frame b in a
// framebuffer of the given bounds.
func frameDamage(a, b *frame, bounds image.Rectangle) Damage {
	d := damager{a: a, b: b, scale: float64(b.scale), bounds: bounds}
	return d.run()
}

func (d *damager) run() Damage {
	A, B := d.a.dl, d.b.dl
	d.reachA, d.reachB = d.reaches(A), d.reaches(B)
	d.findUnits()
	// The sequences to match: without the attachments of the units'
	// top-level chunks, which the shift accounts for.
	seqA, seqB := d.sequence(d.a, d.unitA, d.ua), d.sequence(d.b, d.unitB, d.ub)
	matchedA, matchedB := make([]bool, len(seqA)), make([]bool, len(seqB))
	ok := d.match(seqA, seqB, matchedA, matchedB)
	if !ok {
		return Damage{Full: true}
	}
	for k, j := range seqB {
		if !matchedB[k] {
			d.add(d.devB(j))
		}
	}
	for k, i := range seqA {
		if matchedA[k] {
			continue
		}
		if u := d.unitA[i]; u >= 0 {
			un := &d.ua[u]
			d.add(d.devA(i).Add(image.Pt(un.dx, un.dy)).Intersect(un.r))
		} else {
			d.add(d.devA(i))
		}
	}
	var shifts []Shift
	for k := range d.ub {
		if d.ub[k].group == k {
			shifts = append(shifts, d.unitDamage(k)...)
		}
	}
	d.backdropClosure()
	return d.finish(shifts)
}

// devRect returns the device pixels of root-space rectangle r.
func (d *damager) devRect(r gfx.Rect) image.Rectangle {
	if r.Empty() {
		return image.Rectangle{}
	}
	s := d.scale
	x0, y0 := math.Floor(float64(r.X0)*s)-1, math.Floor(float64(r.Y0)*s)-1
	x1, y1 := math.Ceil(float64(r.X1)*s)+1, math.Ceil(float64(r.Y1)*s)+1
	lim := float64(d.bounds.Max.X+d.bounds.Max.Y) + 1
	clampf := func(v float64) int { return int(min(max(v, -lim), lim)) }
	return image.Rect(clampf(x0), clampf(y0), clampf(x1), clampf(y1))
}

func (d *damager) devA(i int) image.Rectangle {
	return grow(d.devRect(d.a.dl.BoundsAt(i)), d.reachA[i])
}

func (d *damager) devB(i int) image.Rectangle {
	return grow(d.devRect(d.b.dl.BoundsAt(i)), d.reachB[i])
}

func grow(r image.Rectangle, n int) image.Rectangle {
	if n == 0 || r.Empty() {
		return r
	}
	return image.Rect(r.Min.X-n, r.Min.Y-n, r.Max.X+n, r.Max.Y+n)
}

// add adds a damaged rectangle.
func (d *damager) add(r image.Rectangle) {
	if r = r.Intersect(d.bounds); !r.Empty() {
		d.rects = append(d.rects, r)
	}
}

// reaches returns, for every command of dl, the device reach of the
// filters of the layers around it (pushes and pops count as inside their
// own layer).
func (d *damager) reaches(dl *gfx.DisplayList) []int {
	out := make([]int, dl.Len())
	var stack []int
	cur := 0
	for i := range dl.Len() {
		c := dl.At(i)
		switch c.(type) {
		case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath, gfx.PushTransform, gfx.PushLayer:
			stack = append(stack, cur)
			if r := dl.LayerReach(i); r > 0 {
				cur += int(math.Ceil(float64(r)*d.scale)) + 1
			}
			out[i] = cur
		case gfx.PopClip, gfx.PopTransform, gfx.PopLayer:
			out[i] = cur
			cur = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		default:
			out[i] = cur
		}
	}
	return out
}

// ---------------------------------------------------------------------
// Scroll units.

// chunkSpaces returns the scroll spaces of the chunks of f with their
// offsets.
func chunkSpaces(f *frame) map[*scroller]gfx.Point {
	m := map[*scroller]gfx.Point{}
	for _, in := range f.inst {
		if in.layer.kind == layerChunk && in.layer.space != nil {
			m[in.layer.space] = in.soff
		}
	}
	return m
}

// findUnits finds the scroll units of both frames and validates them.
func (d *damager) findUnits() {
	A, B := d.a.dl, d.b.dl
	d.unitA, d.unitB = make([]int32, A.Len()), make([]int32, B.Len())
	for i := range d.unitA {
		d.unitA[i] = -1
	}
	for i := range d.unitB {
		d.unitB[i] = -1
	}
	sa, sb := chunkSpaces(d.a), chunkSpaces(d.b)
	// The spaces that moved, in the order of the new frame.
	cand := map[*scroller]bool{}
	var order []*scroller
	for _, in := range d.b.inst {
		s := in.layer.space
		if in.layer.kind != layerChunk || s == nil || cand[s] || s.sticky {
			// A sticky space's offset is not how far its pixels move on
			// screen (its container moves them too): it is matched
			// command by command, never shifted.
			continue
		}
		if oa, ok := sa[s]; ok && oa != sb[s] {
			cand[s] = true
			order = append(order, s)
		}
	}
	if len(cand) == 0 {
		return
	}
	runsA, runsB := d.runs(d.a, cand), d.runs(d.b, cand)
	// Pair the runs of each space in order. A run that follows a shifted
	// run of its space with only pushes and pops between them, under the
	// same pushes, shifts with it (a group: the hoisted boxes painted
	// after a container's content, under its clip pushed again).
	var used []image.Rectangle
	for _, s := range order {
		// Runs pair by the box that leads them (the chunks of hoisted
		// boxes come and go with the interest rects).
		var ra, rb []unit
		for _, u := range runsB {
			if u.space != s {
				continue
			}
			for _, v := range runsA {
				if v.space == s && v.lead == u.lead {
					ra, rb = append(ra, v), append(rb, u)
					break
				}
			}
		}
		prev := -1 // the previous valid unit of s
		for k := range ra {
			ua, ub := ra[k], rb[k]
			id := len(d.ub)
			if prev >= 0 && prev == id-1 && d.continues(&d.ua[prev], &ua, d.a.dl) && d.continues(&d.ub[prev], &ub, d.b.dl) {
				pa, pb := &d.ua[prev], &d.ub[prev]
				ua.dx, ua.dy, ua.r, ua.group, ua.ok = pa.dx, pa.dy, pa.r, pa.group, true
				ub.dx, ub.dy, ub.r, ub.group, ub.ok = pb.dx, pb.dy, pb.r, pb.group, true
			} else {
				if !d.validate(&ua, &ub) {
					prev = -1
					continue
				}
				overlap := false
				for _, r := range used {
					if r.Overlaps(ub.r) {
						overlap = true
					}
				}
				if overlap {
					prev = -1
					continue
				}
				used = append(used, ub.r)
				ua.group, ub.group = id, id
			}
			ua.id, ub.id = id, id
			d.ua, d.ub = append(d.ua, ua), append(d.ub, ub)
			for i := ua.start; i < ua.end; i++ {
				d.unitA[i] = int32(id)
			}
			for i := ub.start; i < ub.end; i++ {
				d.unitB[i] = int32(id)
			}
			prev = id
		}
	}
}

// continues reports whether run u follows run p in dl with only pushes
// and pops between them, under equal pushes.
func (d *damager) continues(p, u *unit, dl *gfx.DisplayList) bool {
	return p.end <= u.start && onlyPushPops(dl, p.end, u.start) && sameEnclosing(dl, p.start, u.start)
}

// runs returns the runs of commands of f in the chunks of candidate
// spaces: for each command, its outermost enclosing chunk instance of a
// candidate space; consecutive commands of the same space form a run.
func (d *damager) runs(f *frame, cand map[*scroller]bool) []unit {
	top := make([]int32, len(f.inst))
	for k := range f.inst {
		in := &f.inst[k]
		top[k] = -1
		if in.parent >= 0 {
			top[k] = top[in.parent]
		}
		if top[k] < 0 && in.layer.kind == layerChunk && cand[in.layer.space] {
			top[k] = int32(k)
		}
	}
	var out []unit
	dl := f.dl
	for i := 0; i < dl.Len(); {
		t := top[dl.Tag(i)]
		if t < 0 {
			i++
			continue
		}
		s := f.inst[t].layer.space
		j := i + 1
		for j < dl.Len() {
			if tj := top[dl.Tag(j)]; tj < 0 || f.inst[tj].layer.space != s {
				break
			}
			j++
		}
		out = append(out, unit{space: s, lead: f.inst[t].layer.lead, start: i, end: j})
		i = j
	}
	return out
}

// onlyPushPops reports whether the commands of dl in [i, j) are pushes
// and pops.
func onlyPushPops(dl *gfx.DisplayList, i, j int) bool {
	for ; i < j; i++ {
		switch dl.At(i).(type) {
		case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath, gfx.PushTransform, gfx.PushLayer,
			gfx.PopClip, gfx.PopTransform, gfx.PopLayer:
		default:
			return false
		}
	}
	return true
}

// sameEnclosing reports whether the pushes open at i and at j are equal.
func sameEnclosing(dl *gfx.DisplayList, i, j int) bool {
	ei, _ := enclosingAll(dl, i)
	ej, _ := enclosingAll(dl, j)
	if len(ei) != len(ej) {
		return false
	}
	for k := range ei {
		if !gfx.Equal(dl.At(ei[k]), dl.At(ej[k])) {
			return false
		}
	}
	return true
}

// validate checks that the pair of runs ua (old) and ub (new) can be
// shifted, and computes the shift and its rectangle.
//
// The shift is the difference of the device positions the rasterizer gives
// the runs' chunks (origin), not the scroll offsets times the scale: at a
// fractional scale a whole-pixel offset is not exact in float32 logical
// pixels, and the rasterizer places it on the grid (raster.GridMul).
func (d *damager) validate(ua, ub *unit) bool {
	encA, okA := enclosing(d.a.dl, ua.start)
	encB, okB := enclosing(d.b.dl, ub.start)
	if !okA || !okB || len(encA) != len(encB) {
		return false
	}
	pa, okA := d.origin(d.a, ua, encA)
	pb, okB := d.origin(d.b, ub, encB)
	if !okA || !okB {
		return false
	}
	dx, dy := float64(pb.X)-float64(pa.X), float64(pb.Y)-float64(pa.Y)
	if math.Abs(dx) > 1<<20 || math.Abs(dy) > 1<<20 {
		return false
	}
	ua.dx, ua.dy = int(dx), int(dy)
	ub.dx, ub.dy = ua.dx, ua.dy
	for k := range encA {
		if !gfx.Equal(d.a.dl.At(encA[k]), d.b.dl.At(encB[k])) {
			return false
		}
	}
	r, ok := d.clipRect(d.b.dl, encB)
	// One pixel less on each side: command bounds are rounded out with a
	// pixel of margin (devRect), so a box that ends at an edge of R would
	// seem to touch it.
	r = r.Inset(1)
	if !ok || r.Empty() {
		return false
	}
	ua.r, ub.r = r, r
	ua.enc, ub.enc = encA, encB
	fa, okA := d.backdrop(d.a.dl, ua.start, encA, r)
	fb, okB := d.backdrop(d.b.dl, ub.start, encB, r)
	if !okA || !okB || len(fa) != len(fb) {
		return false
	}
	for k := range fa {
		if !gfx.Equal(d.a.dl.At(fa[k]), d.b.dl.At(fb[k])) {
			return false
		}
	}
	ua.ok, ub.ok = true, true
	return true
}

// origin returns the device translation at which the rasterizer draws the
// top-level chunks of run u of frame f, whose enclosing pushes are enc
// (rectangular clips and translations): the chunks' attachments (their
// translations, which the compositor pushes) composed with the enclosing
// translations as the rasterizer composes them (raster.GridMul), or the
// enclosing translation alone for chunks drawn without one. ok is false
// unless it is the same whole number of device pixels for every chunk:
// only then do the run's pixels move by exactly the difference of two
// origins.
func (d *damager) origin(f *frame, u *unit, enc []int) (o gfx.Point, ok bool) {
	dl := f.dl
	s := float32(d.scale)
	m := gfx.Scale(s, s)
	for _, i := range enc {
		if c, isT := dl.At(i).(gfx.PushTransform); isT {
			m = raster.GridMul(m, c.Affine)
		}
	}
	o = gfx.Point{X: m[4], Y: m[5]}
	seen := map[uint32]bool{}
	first := true
	for i := u.start; i < u.end; i++ {
		k := dl.Tag(i)
		if seen[k] {
			continue
		}
		seen[k] = true
		if in := &f.inst[k]; in.layer.kind != layerChunk || in.layer.space != u.space || (in.parent >= 0 && d.inUnit(f, in.parent, u.space)) {
			continue // not a top-level chunk of the run's space
		}
		// The first command of a chunk instance is its attachment, if
		// the compositor pushed one (a command of no layer list).
		p := gfx.Point{X: m[4], Y: m[5]}
		if c, isT := dl.At(i).(gfx.PushTransform); isT && dl.Source(i) < 0 {
			a := raster.GridMul(m, c.Affine)
			p = gfx.Point{X: a[4], Y: a[5]}
		}
		if !first && p != o {
			return gfx.Point{}, false
		}
		o, first = p, false
	}
	whole := func(v float32) bool { return float64(v) == math.Round(float64(v)) }
	return o, whole(o.X) && whole(o.Y)
}

// enclosingAll returns the pushes open at command at of dl, outermost
// first.
func enclosingAll(dl *gfx.DisplayList, at int) (enc []int, ok bool) {
	for i := 0; i < at; i++ {
		c := dl.At(i)
		switch c.(type) {
		case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath, gfx.PushTransform, gfx.PushLayer:
			if m := dl.Match(i); m < at {
				i = m // closed before at
				continue
			}
			enc = append(enc, i)
		}
	}
	return enc, true
}

// enclosing returns the pushes open at command at of dl, outermost
// first. ok is false when one of them is not a rectangular clip or a
// translation (a unit inside a group, a filter or a transform is not
// shifted).
func enclosing(dl *gfx.DisplayList, at int) (enc []int, ok bool) {
	enc, _ = enclosingAll(dl, at)
	for _, i := range enc {
		switch c := dl.At(i).(type) {
		case gfx.PushClipRect, gfx.PushClipRRect:
		case gfx.PushTransform:
			if !isTranslation(c.Affine) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return enc, true
}

func isTranslation(m gfx.Affine) bool { return m[0] == 1 && m[1] == 0 && m[2] == 0 && m[3] == 1 }

// intTranslation reports a translation by whole device pixels: one the
// rasterizer places on the device pixel grid when the current transform is
// on it (raster.GridMul), which at a fractional scale includes the float32
// approximations of whole-pixel offsets.
func (d *damager) intTranslation(m gfx.Affine) bool {
	if !isTranslation(m) {
		return false
	}
	s := float32(d.scale)
	_, okX := raster.GridStep(s, m[4])
	_, okY := raster.GridStep(s, m[5])
	return okX && okY
}

// clipRect returns the device pixels fully inside the clips of enc (and
// the frame), and whether every translation among them moves by whole
// device pixels.
func (d *damager) clipRect(dl *gfx.DisplayList, enc []int) (image.Rectangle, bool) {
	r := d.bounds
	var t gfx.Point
	for _, i := range enc {
		switch c := dl.At(i).(type) {
		case gfx.PushTransform:
			if !d.intTranslation(c.Affine) {
				return image.Rectangle{}, false
			}
			t = t.Add(gfx.Point{X: c.Affine[4], Y: c.Affine[5]})
		case gfx.PushClipRect:
			r = r.Intersect(d.inner(gfx.RRect{Rect: c.Rect}, t))
		case gfx.PushClipRRect:
			r = r.Intersect(d.inner(c.RRect, t))
		}
	}
	return r, true
}

// inner returns the device pixels fully inside rr translated by t: its
// rectangle without the corners.
func (d *damager) inner(rr gfx.RRect, t gfx.Point) image.Rectangle {
	rd := rr.Radii
	x0 := float64(rr.Rect.X0+t.X+max(rd[0].X, rd[3].X)) * d.scale
	x1 := float64(rr.Rect.X1+t.X-max(rd[1].X, rd[2].X)) * d.scale
	y0 := float64(rr.Rect.Y0+t.Y+max(rd[0].Y, rd[1].Y)) * d.scale
	y1 := float64(rr.Rect.Y1+t.Y-max(rd[2].Y, rd[3].Y)) * d.scale
	lim := float64(1 << 24)
	cl := func(v float64) int { return int(min(max(v, -lim), lim)) }
	r := image.Rect(cl(math.Ceil(x0)), cl(math.Ceil(y0)), cl(math.Floor(x1)), cl(math.Floor(y1)))
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return image.Rectangle{}
	}
	return r
}

// backdrop returns the commands before index at (outside enc, the pushes
// open there) that touch device rectangle r, and whether they make r
// uniform: solid fills covering r, inside clips covering r and integer
// translations.
func (d *damager) backdrop(dl *gfx.DisplayList, at int, enc []int, r image.Rectangle) ([]int, bool) {
	var fills []int
	type open struct {
		end int
		t   gfx.Point
	}
	var stack []open
	var t gfx.Point
	e := 0
	for i := 0; i < at; i++ {
		for len(stack) > 0 && stack[len(stack)-1].end == i {
			t = stack[len(stack)-1].t
			stack = stack[:len(stack)-1]
		}
		c := dl.At(i)
		if e < len(enc) && enc[e] == i {
			e++
			if tr, ok := c.(gfx.PushTransform); ok {
				t = t.Add(gfx.Point{X: tr.Affine[4], Y: tr.Affine[5]})
			}
			continue
		}
		switch c := c.(type) {
		case gfx.PopClip, gfx.PopTransform, gfx.PopLayer:
			continue
		case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath, gfx.PushTransform, gfx.PushLayer:
			m := dl.Match(i)
			if !d.devRect(dl.BoundsAt(i)).Overlaps(r) {
				i = m
				continue
			}
			switch c := c.(type) {
			case gfx.PushClipRect:
				if !r.In(d.inner(gfx.RRect{Rect: c.Rect}, t)) {
					return nil, false
				}
				stack = append(stack, open{m, t})
			case gfx.PushClipRRect:
				if !r.In(d.inner(c.RRect, t)) {
					return nil, false
				}
				stack = append(stack, open{m, t})
			case gfx.PushTransform:
				if !d.intTranslation(c.Affine) {
					return nil, false
				}
				stack = append(stack, open{m, t})
				t = t.Add(gfx.Point{X: c.Affine[4], Y: c.Affine[5]})
			default:
				return nil, false
			}
		default:
			if !d.paintsIn(c, d.devRect(dl.BoundsAt(i)), r, t) {
				continue
			}
			var rr gfx.RRect
			var paint gfx.Paint
			switch c := c.(type) {
			case gfx.FillRect:
				rr, paint = gfx.RRect{Rect: c.Rect}, c.Paint
			case gfx.FillRRect:
				rr, paint = c.RRect, c.Paint
			default:
				return nil, false
			}
			if _, solid := paint.(gfx.Color); !solid || !r.In(d.inner(rr, t)) {
				return nil, false
			}
			fills = append(fills, i)
		}
	}
	return fills, true
}

// sequence returns the indices of f's commands to match: all but the
// attachments (translations) of the top-level chunks of its valid units.
func (d *damager) sequence(f *frame, unitOf []int32, units []unit) []int {
	out := make([]int, 0, f.dl.Len())
	for i := range f.dl.Len() {
		if unitOf[i] >= 0 && f.dl.Source(i) < 0 {
			switch f.dl.At(i).(type) {
			case gfx.PushTransform, gfx.PopTransform:
				in := &f.inst[f.dl.Tag(i)]
				if in.layer.kind == layerChunk && in.layer.space == units[unitOf[i]].space &&
					(in.parent < 0 || !d.inUnit(f, in.parent, units[unitOf[i]].space)) {
					continue
				}
			}
		}
		out = append(out, i)
	}
	return out
}

// inUnit reports whether instance k of f is inside a chunk of space s.
func (d *damager) inUnit(f *frame, k int32, s *scroller) bool {
	for ; k >= 0; k = f.inst[k].parent {
		if l := f.inst[k].layer; l.kind == layerChunk && l.space == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// Matching.

// eq reports whether old command i and new command j match.
func (d *damager) eq(i, j int) bool {
	ui, uj := d.unitA[i], d.unitB[j]
	if ui != uj {
		return false
	}
	A, B := d.a.dl, d.b.dl
	ca, cb := A.At(i), B.At(j)
	si, sj := A.Source(i), B.Source(j)
	same := si >= 0 && si == sj && d.a.inst[A.Tag(i)].dl == d.b.inst[B.Tag(j)].dl
	if ui < 0 {
		// The bounds of a push or a pop are those of its block, which
		// follow its content: the content's own matching accounts for
		// them.
		return (isPushPop(ca) || A.BoundsAt(i) == B.BoundsAt(j)) && (same || gfx.Equal(ca, cb))
	}
	// Inside a shifted unit: bounds in their own lists.
	if !d.shiftExact(ca) || !d.shiftExact(cb) {
		return false
	}
	if si >= 0 && sj >= 0 {
		la, lb := d.a.inst[A.Tag(i)].dl, d.b.inst[B.Tag(j)].dl
		if la.BoundsAt(si) != lb.BoundsAt(sj) {
			return false
		}
	} else {
		if si >= 0 || sj >= 0 {
			return false
		}
		u := &d.ub[ui]
		dx, dy := float32(float64(u.dx)/d.scale), float32(float64(u.dy)/d.scale)
		if A.BoundsAt(i).Translate(dx, dy) != B.BoundsAt(j) {
			return false
		}
	}
	return same || gfx.Equal(ca, cb)
}

// shiftExact reports whether a command paints the same pixels moved by a
// whole number of device pixels when its coordinates move by them (the
// rasterizer maps device coordinates exactly, but gradients, images and
// transformed content go through a float inverse).
func (d *damager) shiftExact(c gfx.Command) bool {
	solid := func(p gfx.Paint) bool { _, ok := p.(gfx.Color); return ok }
	switch c := c.(type) {
	case gfx.FillRect:
		return solid(c.Paint)
	case gfx.FillRRect:
		return solid(c.Paint)
	case gfx.FillPath:
		return solid(c.Paint)
	case gfx.StrokePath:
		return solid(c.Paint)
	case gfx.DrawGlyphRun:
		// The glyph cache's masks and sprites are a function of their key
		// (raster.GlyphCache); larger glyphs are filled directly.
		return solid(c.Paint) && float64(c.Size)*d.scale <= 256
	case gfx.DrawShadow:
		return true
	case gfx.PushTransform:
		return d.intTranslation(c.Affine)
	case gfx.PushLayer:
		// Blend modes combine the shifted source and backdrop pixel by
		// pixel; a mask's list is not checked, so a masked layer is not
		// shifted.
		return c.Mask.List == nil
	case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath,
		gfx.PopClip, gfx.PopTransform, gfx.PopLayer:
		return true
	}
	return false
}

// match marks the matched elements of the two sequences. It reports
// false when it gives up (damage everything).
func (d *damager) match(sa, sb []int, ma, mb []bool) bool {
	n, m := len(sa), len(sb)
	p := 0
	for p < n && p < m && d.eq(sa[p], sb[p]) {
		ma[p], mb[p] = true, true
		p++
	}
	q := 0
	for q < n-p && q < m-p && d.eq(sa[n-1-q], sb[m-1-q]) {
		ma[n-1-q], mb[m-1-q] = true, true
		q++
	}
	i, j, ei, ej := p, p, n-q, m-q
	for i < ei && j < ej {
		if d.eq(sa[i], sb[j]) {
			ma[i], mb[j] = true, true
			i++
			j++
			continue
		}
		found := false
		for s := 1; s <= matchLookahead && !found; s++ {
			for x := 0; x <= s; x++ {
				y := s - x
				if i+x < ei && j+y < ej && d.eq(sa[i+x], sb[j+y]) {
					i, j = i+x, j+y
					found = true
					break
				}
			}
			if i+s >= ei && j+s >= ej {
				break
			}
		}
		if !found {
			// Everything left in the middle differs.
			return d.unmatchedArea(sa[i:ei], sb[j:ej])
		}
	}
	return true
}

// unmatchedArea decides what to do when the rest of the middle of the two
// sequences differs: a full rasterization when it covers most of the
// frame; the caller then damages the unmatched commands one by one.
func (d *damager) unmatchedArea(ra, rb []int) bool {
	var u image.Rectangle
	for _, i := range ra {
		u = u.Union(d.devA(i).Intersect(d.bounds))
	}
	for _, j := range rb {
		u = u.Union(d.devB(j).Intersect(d.bounds))
	}
	return float64(u.Dx()*u.Dy()) <= fullFraction*float64(d.bounds.Dx()*d.bounds.Dy())
}

// ---------------------------------------------------------------------
// Unit damage.

// unitDamage adds the damage of the group of valid units that starts at
// unit k and returns its shift.
func (d *damager) unitDamage(k int) []Shift {
	last := k
	for last+1 < len(d.ub) && d.ub[last+1].group == k {
		last++
	}
	r := d.ub[k].r
	delta := image.Pt(d.ub[k].dx, d.ub[k].dy)
	// The units' pixels outside R, in both frames.
	for u := k; u <= last; u++ {
		d.outside(d.a.dl, &d.ua[u], d.devA, r)
		d.outside(d.b.dl, &d.ub[u], d.devB, r)
	}
	// The exposed strips.
	moved := r.Add(delta).Intersect(r)
	for _, p := range subtract(r, moved) {
		d.add(p)
	}
	// What is painted after the group and touches R.
	d.after(d.b.dl, d.ub[last].end, d.ub[k].enc, r, d.devB, image.Point{})
	d.after(d.a.dl, d.ua[last].end, d.ua[k].enc, r, d.devA, delta)
	if moved.Empty() {
		return nil
	}
	return []Shift{{Rect: moved, DX: delta.X, DY: delta.Y}}
}

// outside damages the pixels of unit u of dl outside r.
func (d *damager) outside(dl *gfx.DisplayList, u *unit, dev func(int) image.Rectangle, r image.Rectangle) {
	var b image.Rectangle
	for i := u.start; i < u.end; i++ {
		b = b.Union(dev(i))
		if m := dl.Match(i); m > i {
			i = m
		}
	}
	for _, p := range subtract(b, r) {
		d.add(p)
	}
}

// after damages the commands of dl from index from on that can paint in
// r, moved by off: their pixels in r (and, for backdrop filters that read
// r, their whole output). enc are the pushes open at the end of the unit
// (their translations place the commands that follow at their level).
func (d *damager) after(dl *gfx.DisplayList, from int, enc []int, r image.Rectangle, dev func(int) image.Rectangle, off image.Point) {
	// The translations of the open pushes, to test the shapes of the
	// commands at their level.
	ts := make([]gfx.Point, len(enc)+1)
	for k, i := range enc {
		ts[k+1] = ts[k]
		if c, ok := dl.At(i).(gfx.PushTransform); ok {
			ts[k+1] = ts[k].Add(gfx.Point{X: c.Affine[4], Y: c.Affine[5]})
		}
	}
	depth := len(enc)
	rl := r.Sub(off) // r where the frame painted it
	for i := from; i < dl.Len(); i++ {
		c := dl.At(i)
		if isPop(c) {
			if depth > 0 && dl.Match(i) == enc[depth-1] {
				depth--
			}
			continue
		}
		b := dev(i)
		if d.paintsIn(c, b, rl, ts[depth]) {
			d.add(b.Add(off).Intersect(r))
		}
		if m := dl.Match(i); m > i {
			i = m // the push's bounds hold its block
		}
	}
	for k := range dl.NumBackdrops() {
		i, read, _, _ := dl.Backdrop(k)
		if i >= from && d.devRect(read).Overlaps(rl) {
			d.add(dev(i).Add(off))
		}
	}
}

// isPop reports whether c closes a block.
func isPop(c gfx.Command) bool {
	switch c.(type) {
	case gfx.PopClip, gfx.PopTransform, gfx.PopLayer:
		return true
	}
	return false
}

// paintsIn reports whether command c, with device bounds b and drawn
// under translation t, can change pixels of device rectangle r: its bounds
// meet r, unless it is an outer shadow of a box that contains r (CSS
// clips outer shadows to the outside of their box) or a ring (a border,
// an outline: a nonzero path made of two convex contours of opposite
// directions) whose hole contains r.
func (d *damager) paintsIn(c gfx.Command, b, r image.Rectangle, t gfx.Point) bool {
	if !b.Overlaps(r) {
		return false
	}
	switch c := c.(type) {
	case gfx.DrawShadow:
		return c.Inset || !r.In(d.inner(c.RRect, t))
	case gfx.FillPath:
		return c.Rule != gfx.NonZero || !d.inHole(c.Path, r, t)
	}
	return true
}

// inHole reports whether device rectangle r lies in the hole of a ring
// path under translation t: the path is two closed convex contours of
// opposite directions, and the corners of r are inside the polygons of
// the points on both (a convex curve contains its chords, and the
// rasterizer's flattening only adds points on the curve), so the winding
// number in r is 1 - 1 = 0 and the path covers nothing there.
func (d *damager) inHole(path gfx.Path, r image.Rectangle, t gfx.Point) bool {
	var polys [2][]gfx.Point
	n := -1
	pts := path.Points()
	k := 0
	for _, v := range path.Verbs() {
		switch v {
		case gfx.MoveTo:
			n++
			if n > 1 {
				return false
			}
		case gfx.Close:
			continue
		}
		np := v.NumPoints()
		if n < 0 || np == 0 {
			return false
		}
		polys[n] = append(polys[n], pts[k+np-1])
		k += np
	}
	if n != 1 {
		return false
	}
	ax, bx := area2(polys[0]), area2(polys[1])
	if ax == 0 || bx == 0 || (ax > 0) == (bx > 0) {
		return false
	}
	s := d.scale
	corners := [4]gfx.Point{}
	for q, pt := range [4]image.Point{r.Min, {r.Max.X, r.Min.Y}, r.Max, {r.Min.X, r.Max.Y}} {
		corners[q] = gfx.Point{X: float32(float64(pt.X)/s) - t.X, Y: float32(float64(pt.Y)/s) - t.Y}
	}
	for _, poly := range polys {
		if !insideConvex(poly, corners[:]) {
			return false
		}
	}
	return true
}

// area2 returns twice the signed area of a polygon.
func area2(poly []gfx.Point) float64 {
	a := 0.0
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		a += float64(p.X)*float64(q.Y) - float64(q.X)*float64(p.Y)
	}
	return a
}

// insideConvex reports whether every point of pts is inside polygon poly,
// which must be convex (it reports false otherwise), with a margin of
// 1/64 pixel.
func insideConvex(poly, pts []gfx.Point) bool {
	if len(poly) < 3 {
		return false
	}
	sign := 0.0
	for i, a := range poly {
		b := poly[(i+1)%len(poly)]
		ex, ey := float64(b.X)-float64(a.X), float64(b.Y)-float64(a.Y)
		l := math.Hypot(ex, ey)
		if l == 0 {
			continue
		}
		for _, p := range pts {
			cr := (ex*(float64(p.Y)-float64(a.Y)) - ey*(float64(p.X)-float64(a.X))) / l
			if sign == 0 {
				switch {
				case cr > 1.0/64:
					sign = 1
				case cr < -1.0/64:
					sign = -1
				default:
					return false
				}
			}
			if cr*sign < 1.0/64 {
				return false
			}
		}
		// Convexity: the next edge turns the same way.
		c := poly[(i+2)%len(poly)]
		fx, fy := float64(c.X)-float64(b.X), float64(c.Y)-float64(b.Y)
		if turn := ex*fy - ey*fx; turn*sign < 0 {
			return false
		}
	}
	return sign != 0
}

// subtract returns the parts of a outside b (at most four rectangles).
func subtract(a, b image.Rectangle) []image.Rectangle {
	if a.Empty() {
		return nil
	}
	b = b.Intersect(a)
	if b.Empty() {
		return []image.Rectangle{a}
	}
	var out []image.Rectangle
	if b.Min.Y > a.Min.Y {
		out = append(out, image.Rect(a.Min.X, a.Min.Y, a.Max.X, b.Min.Y))
	}
	if b.Max.Y < a.Max.Y {
		out = append(out, image.Rect(a.Min.X, b.Max.Y, a.Max.X, a.Max.Y))
	}
	if b.Min.X > a.Min.X {
		out = append(out, image.Rect(a.Min.X, b.Min.Y, b.Min.X, b.Max.Y))
	}
	if b.Max.X < a.Max.X {
		out = append(out, image.Rect(b.Max.X, b.Min.Y, a.Max.X, b.Max.Y))
	}
	return out
}

// ---------------------------------------------------------------------
// Backdrop filters and the final rectangles.

// backdropClosure damages the output of every backdrop filter of the new
// frame that reads damaged pixels: the part of its output within its reach
// of the damaged pixels it reads (raster.BackdropOutputRegion), or its
// whole output inside a layer with filters, which spreads it further. A
// filter's output is read only by the filters after it, so one pass in
// list order suffices (the damage of other commands counts whatever their
// place in the list).
func (d *damager) backdropClosure() {
	B := d.b.dl
	scale := float32(d.scale)
	for k := range B.NumBackdrops() {
		i, read, _, inFilter := B.Backdrop(k)
		rd := d.devRect(read)
		var o image.Rectangle
		for _, r := range d.rects {
			if !r.Overlaps(rd) {
				continue
			}
			if inFilter {
				o = d.devB(i)
				break
			}
			o = union(o, raster.BackdropOutputRegion(B, scale, k, r))
		}
		d.add(o)
	}
}

// union returns the smallest rectangle containing a and b (either may be
// empty).
func union(a, b image.Rectangle) image.Rectangle {
	switch {
	case a.Empty():
		return b
	case b.Empty():
		return a
	}
	return a.Union(b)
}

// finish merges the damage into a few rectangles, or turns it into a full
// rasterization when rasterizing them (with what their backdrop filters
// read) costs most of a full one.
func (d *damager) finish(shifts []Shift) Damage {
	rects := mergeRects(d.rects)
	area := 0
	for _, r := range rects {
		// Rasterizing r renders the pixels its backdrop filters read too
		// (raster.BackdropRegion). Overlaps count twice: they are
		// rasterized twice.
		g := raster.BackdropRegion(d.b.dl, d.b.scale, r, d.bounds)
		area += g.Dx() * g.Dy()
	}
	for _, s := range shifts {
		area += s.Rect.Dx() * s.Rect.Dy() / 8 // a shift costs a copy
	}
	if float64(area) > fullFraction*float64(d.bounds.Dx()*d.bounds.Dy()) {
		return Damage{Full: true}
	}
	return Damage{Shifts: shifts, Rects: rects}
}

// mergeRects merges rectangles whose bounding box covers little more
// than they do, then the pairs that waste the least until there are at
// most maxDamageRects. Rectangles may overlap: rasterizing a pixel twice
// gives it the same value.
func mergeRects(in []image.Rectangle) []image.Rectangle {
	rs := make([]image.Rectangle, 0, len(in))
	for _, r := range in {
		if !r.Empty() {
			rs = append(rs, r)
		}
	}
	area := func(r image.Rectangle) int { return r.Dx() * r.Dy() }
	// waste is how many pixels the bounding box of a and b adds.
	waste := func(a, b image.Rectangle) int {
		return area(a.Union(b)) - area(a) - area(b) + area(a.Intersect(b))
	}
	for {
		merged := false
		for i := 0; i < len(rs) && !merged; i++ {
			for j := i + 1; j < len(rs); j++ {
				if u := rs[i].Union(rs[j]); waste(rs[i], rs[j]) <= area(u)/8 {
					rs[i] = u
					rs = slices.Delete(rs, j, j+1)
					merged = true
					break
				}
			}
		}
		if merged {
			continue
		}
		if len(rs) <= maxDamageRects {
			return rs
		}
		bi, bj, best := 0, 1, -1
		for i := range rs {
			for j := i + 1; j < len(rs); j++ {
				if w := waste(rs[i], rs[j]); best < 0 || w < best {
					bi, bj, best = i, j, w
				}
			}
		}
		rs[bi] = rs[bi].Union(rs[bj])
		rs = slices.Delete(rs, bj, bj+1)
	}
}

// isPushPop reports whether c opens or closes a block.
func isPushPop(c gfx.Command) bool {
	switch c.(type) {
	case gfx.PushClipRect, gfx.PushClipRRect, gfx.PushClipPath, gfx.PushTransform, gfx.PushLayer,
		gfx.PopClip, gfx.PopTransform, gfx.PopLayer:
		return true
	}
	return false
}
