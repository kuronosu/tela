package core

import (
	"image"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Layers, composition and damage (ARCHITECTURE section 13, "Layers and
// caching" and "Damage tracking").
//
// # Layers
//
// The paint phase records the tree into a tree of layers, each a display
// list of its own (Flutter's repaint boundaries). A list holds a
// gfx.Marker where a child layer goes, so a layer is composited exactly
// where it is in the painting order of paint.go, inside the clips,
// transforms and opacity groups of the boxes around it. The layers are:
//
//   - the root layer, which holds everything the other layers do not;
//   - a chunk: what is painted in the scroll space of a scroll container
//     whose content can move (scroller.chunked: it overflows, or its
//     offset is not zero): its content and in-flow descendants
//     (paintInner), and the hoisted participants that move with it
//     (paintHoisted). Every box is painted at its unscrolled position (u),
//     and the compositor moves a chunk by the scroll offsets between its
//     space and the space of the list that holds its marker, so a scroll
//     re-records nothing. A sticky box has a scroll space of its own too
//     (sticky.go), whose offset is its negated sticky offset: its chunk
//     always carries its translation and nothing in it is culled;
//   - a deferred layer: a stacking context with paint-phase deferred props
//     (TranslateFn, OpacityFn: Box.deferPaint) other than the root. Its
//     list holds everything inside its effects; the compositor pushes its
//     transform, backdrop filter and opacity with the values of this
//     frame, so a change of them only composites again. A box that stops
//     or starts being a stacking context (its transform becomes the
//     identity) records everything again. This is also where an explicit
//     repaint boundary, the animated transforms and opacity of anim.go
//     and the flips of layout animations (layoutanim.go) belong: a box
//     whose paint-phase props change often;
//   - the scrollbars of a scroll container that has a chunk or classic
//     bars, recorded on every frame that paints (they follow the offsets,
//     the fades and the hover states);
//   - the content of a text field (fieldpaint.go), recorded again on a
//     frame that only repaints when the field asked for it (its caret
//     blinked or moved, its selection or its value changed). Its lines
//     are culled by the interest rect of the list that holds its marker,
//     kept for those recordings.
//
// The set is small on purpose: every layer costs a list and a marker.
//
// # Recording
//
// A paint records again only the layers whose lists can have changed
// (record.go, ADR-204): a frame that builds, restyles or lays out places
// every box and compares what each one's painting reads with its last
// place; a frame that only repaints (scrolling, Values read by paint-phase
// deferred props, scrollbar fades and hovers: Tree.markPaint, whose marks
// stop at these layers; the steps of color transitions) places again the
// boxes it marked (their on-screen geometry for hit-testing, the deferred
// props). The scrollbar layers are recorded on every paint. Everything is
// recorded again (a full recording) on the first paint, for a new scale,
// viewport or canvas, while paint-phase text colors are mounted, and when
// a frame that only repaints finds stale layers: a box that starts or stops
// being a layer, a scroll container whose content starts or stops moving.
// A scroll container whose visible content leaves its interest rect gets a
// new one, and the chunks of its space that cull are recorded again.
//
// The interest rect of a chunk (Chromium's) is its scrollport in content
// coordinates grown by one scrollport along each axis that scrolls: a
// chunk records the in-flow boxes (and hoisted participants that are not
// stacking contexts) whose ink (Box.ink) meets it, so a 10,000-row list
// records a few screens, and scrolling records again only when the
// visible part leaves it. Nothing is skipped inside transformed or
// filtered stacking contexts and deferred layers.
//
// # Composition and damage
//
// The compositor appends the layers into one display list, the frame
// (gfx.Builder.AppendList: commands keep their values, their bounds are
// mapped and clipped, hidden ones are dropped), tagging every command with
// its layer instance. Rasterizing the frame fully gives the reference
// pixels; Tree.RasterizeFrame brings a framebuffer that holds the previous
// frame to the new one by rasterizing only the damage (damage.go): the
// device rectangles where the two frames can differ, found by matching
// their commands. A scroll container whose content moved by whole device
// pixels over a uniform backdrop has its pixels shifted instead, and only
// the exposed strips and what moves differently are rasterized.

// layerKind is the kind of a layer.
type layerKind uint8

const (
	layerRoot     layerKind = iota // everything else
	layerChunk                     // a part of a scroll space
	layerDeferred                  // a stacking context with paint-phase deferred props
	layerBars                      // the scrollbars of a scroll container
	layerField                     // the content of a text field
)

var layerKindNames = [...]string{"root", "scroll", "deferred", "scrollbars", "field"}

// layer is one recorded layer.
type layer struct {
	id   uint32
	kind layerKind
	// box is the box the layer belongs to: the root, the scroll container
	// of a chunk or of scrollbars, the deferred stacking context.
	box *Box
	// space is the scroll space the list is painted in; parent the space
	// of the list that holds its marker.
	space, parent *scroller
	dl            *gfx.DisplayList
	// lead is the box whose painting opened a chunk (the scroll container
	// for its content, a hoisted box, a clipping ancestor of one): the
	// damage pairs the chunks of two frames by it.
	lead *Box
	// paint numbers the paint that recorded it (layerState.paints).
	paint uint64
	// interest is, for a text field's layer, the interest rect of the
	// list that holds its marker (unbounded where nothing is culled),
	// kept until it is recorded again.
	interest gfx.Rect

	// Partial recording (record.go). hoist is the hoisted box whose
	// painting opened a chunk (paintHoisted: its own chunk, or that of a
	// clipping ancestor), sc the stacking context that painted it; nil for
	// other layers.
	hoist, sc *Box
	// kids are the layers whose markers its list holds, in order; pool
	// holds the old kids while it is recorded, pi the next one to look at.
	kids, pool []*layer
	pi         int
	// rint and cull are the interest rect and the culling of the list
	// (rint decides nothing for a list that does not cull);
	// in and out the boxes whose inputs and placement it read (dep,
	// odep), cul those it culled-tested (cdep); rec the epoch of its
	// recording; dirty marks it stale.
	rint    gfx.Rect
	cull    bool
	in, out []*Box
	cul     []cullDep
	// oin and oout are in and out of the previous recording while one
	// runs (record.go: dropOld).
	oin, oout []*Box
	rec       uint64
	dirty     bool
	// uses holds the decoded images its list draws (backgrounds and
	// masks: painter.hold), so that the cache keeps them while they are
	// shown (assets.go, "Releasing images"); ouses those of the previous
	// recording while one runs, so that an image drawn again stays alive
	// until it is looked up.
	uses, ouses []*imageAsset
}

// target is a list being recorded: its builder, its scroll space and
// interest rect, whether it skips boxes outside it, and its layer.
type target struct {
	b        *gfx.Builder
	space    *scroller
	interest gfx.Rect
	cull     bool
	layer    *layer
}

// layerState is the layer tree of a Tree and the frames it composed.
type layerState struct {
	layers []*layer // by marker ID
	root   *layer
	// gen numbers the full recordings, paints the paints.
	gen, paints uint64
	// needFull asks the next paint for a full recording, needPlace for
	// placing the boxes first.
	needFull, needPlace bool
	// noCull records every box (tests: the culling must not change the
	// pixels).
	noCull bool
	free   []*gfx.Builder
	// freeIDs are the ids of freed layers (record.go).
	freeIDs []uint32
	// epoch advances after every paint: the boxes stamp their changed
	// inputs with it, and nchg counts the stamps since the last paint;
	// recull reports a changed ink or interest rect since then, for which
	// the culling of the lists is checked again (record.go). recScale,
	// recView and recTrans are the scale, viewport and canvas
	// transparency of the last recording.
	epoch    uint64
	nchg     int
	recull   bool
	recScale float32
	recView  gfx.Size
	recTrans bool

	// The composition in progress: the builder, the frame and the
	// current instance; marker is the method value of composeLayer.
	comp    *gfx.Builder
	compF   *frame
	compCur int32
	marker  func(gfx.Marker)

	// cur is the last composed frame, ras the last one rasterized into a
	// framebuffer of rasRect, and damage how it was rasterized.
	cur, ras *frame
	rasRect  image.Rectangle
	damage   Damage
	// dmg holds the buffers frameDamage reuses.
	dmg damageScratch
	// bdc caches the filtered backdrops of ras in the framebuffer
	// (ADR-200): frameDamage carries the filters whose input did not
	// change to the next frame.
	bdc raster.BackdropCache
	// rasIdle: the last RasterizeFrame left nothing pending (RasterIdle).
	rasIdle bool
	// recorded counts the layers recorded by the last paint.
	recorded int
}

// frame is a composed display list and the layer instances of its
// commands (gfx.DisplayList.Tag).
type frame struct {
	dl    *gfx.DisplayList
	inst  []instance
	scale float32
	// uses holds the decoded images painted while composing it (the masks
	// of deferred layers, whose effects the compositor pushes).
	uses []*imageAsset
}

// hold keeps decoded image a alive with the list being recorded (its
// layer), or with the frame being composed (assets.go, "Releasing
// images").
func (p *painter) hold(a *imageAsset) {
	if a == nil {
		return
	}
	var uses *[]*imageAsset
	switch {
	case p.layer != nil:
		uses = &p.layer.uses
	case p.t.lay.compF != nil:
		uses = &p.t.lay.compF.uses
	default:
		return
	}
	if n := len(*uses); n > 0 && (*uses)[n-1] == a {
		return
	}
	*uses = append(*uses, a)
}

// instance is a layer as composed in a frame.
type instance struct {
	layer *layer
	// dl is the list of the layer composed in this frame.
	dl     *gfx.DisplayList
	parent int32
	// soff is the offset of a chunk's scroll space, and d its translation
	// in the list of its parent.
	soff, d gfx.Point
}

// builder returns an empty builder.
func (L *layerState) builder() *gfx.Builder {
	if n := len(L.free); n > 0 {
		b := L.free[n-1]
		L.free = L.free[:n-1]
		return b
	}
	return gfx.NewBuilder()
}

// newLayer adds a layer to the recording, with a free id when there is
// one.
func (L *layerState) newLayer(kind layerKind, box *Box, space, parent *scroller) *layer {
	l := &layer{kind: kind, box: box, space: space, parent: parent, paint: L.paints}
	if n := len(L.freeIDs); n > 0 {
		l.id = L.freeIDs[n-1]
		L.freeIDs = L.freeIDs[:n-1]
		L.layers[l.id] = l
		return l
	}
	l.id = uint32(len(L.layers))
	L.layers = append(L.layers, l)
	return l
}

// paintPhase paints the tree (ARCHITECTURE section 8, phase 6): places
// the boxes (all of them when full), records the layers that changed
// (record.go; all of them when nothing was recorded yet or something the
// boxes' inputs do not cover changed) and composes the frame.
func (t *Tree) paintPhase(full bool) *gfx.DisplayList {
	t.stats.Paints++
	L := &t.lay
	L.recorded = 0
	L.paints++
	t.la.paints++
	defer t.endLayoutPaint()
	// The boxes whose paint-phase reads changed (markPaint): their
	// deferred props only move their layer, or make it stale. The float
	// pass may mark more (floating.go): every mark ends with the paint.
	marked := t.paintQ
	defer func() {
		for _, b := range t.paintQ {
			b.needsPaint = false
		}
		clear(t.paintQ)
		t.paintQ = t.paintQ[:0]
		// What changed until now is recorded: later changes are newer.
		L.epoch++
		L.nchg, L.recull = 0, false
	}()
	if t.rootBox == nil || t.rootBox.cs == nil {
		L.root, L.layers, L.freeIDs = nil, L.layers[:0], L.freeIDs[:0]
		return t.composite()
	}
	place := full || L.root == nil || L.needPlace
	all := L.root == nil || L.needFull
	L.needFull, L.needPlace = false, false
	p := painter{t: t, scale: float64(t.scale), full: place}
	if !place {
		// The boxes marked for painting (deferred props, animations) are
		// placed again, also when the layers are recorded again anyway.
		p.update(marked)
		place = L.needPlace // stale layers, or an interest rect moves
		all = all || L.needFull
		L.needFull, L.needPlace = false, false
	}
	p.full = place
	if place {
		t.la.places++
		p.place(t.rootBox, nil)
	}
	// The floating elements follow their anchors (floating.go), before
	// anything is recorded.
	t.floatPhase(&p)
	if L.needFull || L.needPlace {
		// A Value published by the float pass changed what a box records.
		all = true
	}
	L.needFull, L.needPlace = false, false
	// What the inputs of the boxes do not cover: the scale (snapping), the
	// viewport and the canvas, and text recolored by the paint-phase text
	// colors of its ancestors (colorfn.go).
	all = all || t.scale != L.recScale || t.viewport != L.recView || t.transCanvas != L.recTrans ||
		t.textColorFns > 0 && L.nchg > 0
	partial := !all && t.recordDirty(&p)
	if !partial {
		t.recordAll(&p)
	}
	L.recScale, L.recView, L.recTrans = t.scale, t.viewport, t.transCanvas
	dl := t.composite()
	if partial && checkLayers {
		t.checkPartial(L.cur)
	}
	return dl
}

// recordAll records every layer.
func (t *Tree) recordAll(p *painter) {
	L := &t.lay
	L.gen++
	for _, l := range L.layers {
		if l != nil {
			l.drop()
			l.kids, l.pool, l.in, l.out, l.oin, l.oout, l.cul = nil, nil, nil, nil, nil, nil, nil
		}
	}
	clear(L.layers)
	L.layers, L.freeIDs, L.recorded = L.layers[:0], L.freeIDs[:0], 0
	root := L.newLayer(layerRoot, t.rootBox, nil, nil)
	L.root = root
	p.b, p.layer = L.builder(), root
	p.space, p.cull = nil, !L.noCull
	p.interest = p.interestOf(nil)
	p.targets = p.targets[:0]
	root.begin(L, p.interest, p.cull)
	p.b.FillRect(gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}, t.CanvasColor())
	p.paintStacked(t.rootBox)
	p.endLayer(root)
	p.b, p.layer = nil, nil
}

// openLayer starts recording a child layer of the list being recorded: a
// marker there, and a new list painted in scroll space space, opened by
// the painting of lead (chunks) for hoisted box hoist (paintHoisted). When
// the list being recorded had a clean child layer with that identity
// (record.go), that layer is kept: its marker is written, nothing else, and
// openLayer reports true (the caller paints none of its content and does
// not close it).
func (p *painter) openLayer(kind layerKind, box *Box, space *scroller, lead, hoist *Box) (*layer, bool) {
	L := &p.t.lay
	up := p.layer
	interest := p.interest
	if space != p.space {
		interest = p.interestOf(space)
	}
	cull := p.cull && kind != layerDeferred // everything in a deferred layer is recorded
	var l *layer
	if up != nil && len(up.pool) > 0 {
		l = up.take(kind, box, space, lead, hoist)
	}
	if l != nil && !l.dirty && kind != layerBars && l.cull == cull && (!cull || l.rint == interest) &&
		(kind != layerField || !box.fld.paintDirty) {
		l.parent = p.space
		up.kids = append(up.kids, l)
		p.b.Marker(l.id)
		return l, true
	}
	if l == nil {
		l = L.newLayer(kind, box, space, p.space)
	}
	l.parent, l.lead, l.hoist = p.space, lead, hoist
	if up != nil {
		up.kids = append(up.kids, l)
	}
	p.b.Marker(l.id)
	p.targets = append(p.targets, target{p.b, p.space, p.interest, p.cull, p.layer})
	p.b, p.layer = L.builder(), l
	if space != p.space {
		p.space, p.interest = space, interest
		if space != nil {
			space.recGen = L.gen
		}
	}
	p.cull = cull
	l.begin(L, interest, cull)
	return l, false
}

// closeLayer ends the layer started by the matching openLayer.
func (p *painter) closeLayer() {
	p.endLayer(p.layer)
	n := len(p.targets) - 1
	tg := p.targets[n]
	p.targets[n] = target{}
	p.targets = p.targets[:n]
	p.b, p.space, p.interest, p.cull, p.layer = tg.b, tg.space, tg.interest, tg.cull, tg.layer
}

// What enter did.
const (
	enterSame = iota // the list being recorded is in that space
	enterNew         // a chunk is being recorded: leave closes it
	enterKept        // a clean chunk was kept: nothing to paint
)

// enter makes what follows, the painting of box lead (for hoisted box
// hoist of stacking context sc, or nil), paint in scroll space s: when it
// is not the space of the list being recorded, it opens a chunk of s.
func (p *painter) enter(s *scroller, lead, hoist, sc *Box) int {
	if s == p.space {
		return enterSame
	}
	var box *Box
	if s != nil {
		box = s.box
	}
	l, kept := p.openLayer(layerChunk, box, s, lead, hoist)
	l.sc = sc
	if kept {
		return enterKept
	}
	return enterNew
}

// leave closes the chunk opened by enter.
func (p *painter) leave() { p.closeLayer() }

// interestOf returns the interest rect of scroll space s: the viewport
// for the window, the scroller's otherwise.
func (p *painter) interestOf(s *scroller) gfx.Rect {
	if s == nil {
		return gfx.Rect{X1: p.t.viewport.W, Y1: p.t.viewport.H}
	}
	return s.interest
}

// placeScroller records the interest rect of scroll container bx when the
// boxes are placed, and on a frame that only repaints finds a chunk that
// must be recorded again.
func (p *painter) placeScroller(bx *Box, s *scroller) {
	vis := paddingRect(bx.u, bx.ld.Layout()).Translate(s.soff.X, s.soff.Y)
	L := &p.t.lay
	if p.full {
		s.chunkedRec = s.chunked
		// An interest rect that still holds the visible part is kept, so
		// that the chunks of the space need not be recorded again
		// (record.go); a reference recording takes the current one.
		if s.chunked && (p.ref || s.recGen != L.gen || !s.interest.ContainsRect(vis)) {
			p.newInterest(s, vis)
		}
		return
	}
	switch {
	case s.chunked != s.chunkedRec:
		L.needFull, L.needPlace = true, true
	case s.chunked && s.recGen == L.gen && !vis.Empty() && !s.interest.ContainsRect(vis):
		// The visible part left it: the chunks of the space are recorded
		// again around the new one (record.go).
		p.newInterest(s, vis)
	}
}

// newInterest sets the interest rect of scroll space s for its visible
// part vis: grown by one scrollport along each axis that scrolls.
func (p *painter) newInterest(s *scroller, vis gfx.Rect) {
	var mx, my float32
	if s.max.X > 0 {
		mx = vis.Width()
	}
	if s.max.Y > 0 {
		my = vis.Height()
	}
	in := vis.InsetEdges(-mx, -my, -mx, -my)
	if in != s.interest && !p.ref {
		p.t.lay.recull = true // what the lists culled against it
	}
	s.interest = in
}

// shiftOf returns how far scroll space s moves its content: the offsets
// of s and of the spaces it is in.
func shiftOf(s *scroller) gfx.Point {
	var d gfx.Point
	for ; s != nil; s = s.outer() {
		d = d.Add(s.soff)
	}
	return d
}

// composite composes the recorded layers into the frame.
func (t *Tree) composite() *gfx.DisplayList {
	L := &t.lay
	if L.comp == nil {
		L.comp = gfx.NewBuilder()
		L.marker = t.composeLayer
	}
	f := &frame{scale: t.scale}
	if L.root != nil && L.root.dl != nil {
		f.inst = append(f.inst, instance{layer: L.root, dl: L.root.dl, parent: -1})
		L.compF, L.compCur = f, 0
		L.comp.SetTag(0)
		L.comp.AppendList(L.root.dl, L.marker)
		L.compF = nil
	}
	f.dl = L.comp.Finish()
	L.cur = f
	return f.dl
}

// composeLayer appends the layer of marker m where it stands.
func (t *Tree) composeLayer(m gfx.Marker) {
	L := &t.lay
	if int(m.ID) >= len(L.layers) {
		return
	}
	l := L.layers[m.ID]
	if l == nil || l.dl == nil {
		return
	}
	f := L.compF
	in := instance{layer: l, dl: l.dl, parent: L.compCur}
	if l.kind == layerChunk {
		if l.space != nil {
			in.soff = l.space.soff
		}
		in.d = shiftOf(l.parent).Sub(shiftOf(l.space))
	}
	idx := int32(len(f.inst))
	f.inst = append(f.inst, in)
	b := L.comp
	prevTag, prevCur := b.SetTag(uint32(idx)), L.compCur
	L.compCur = idx
	switch l.kind {
	case layerChunk:
		// A sticky or floating space's chunk always has its translation,
		// so that the damage sees it move when its offset starts or stops
		// (a pushed block bounds its content, damage.go).
		moved := in.d != (gfx.Point{}) || l.space != nil && l.space.own()
		if moved {
			b.PushTransform(gfx.Translate(in.d.X, in.d.Y))
		}
		b.AppendList(l.dl, L.marker)
		if moved {
			b.PopTransform()
		}
	case layerDeferred:
		p := painter{t: t, b: b, scale: float64(t.scale)}
		pops := p.pushEffects(l.box)
		b.AppendList(l.dl, L.marker)
		p.popEffects(pops)
	default:
		b.AppendList(l.dl, L.marker)
	}
	b.SetTag(prevTag)
	L.compCur = prevCur
}

// ---------------------------------------------------------------------
// Rasterizing frames.

// Damage describes how RasterizeFrame brought a framebuffer to a frame.
type Damage struct {
	// Full reports a full rasterization.
	Full bool
	// Shifts moved pixels, before the rectangles were rasterized: the
	// pixels of each Rect took the values of the pixels DX, DY before
	// them.
	Shifts []Shift
	// Rects are the device rectangles rasterized after the shifts.
	Rects []image.Rectangle
}

// Shift is a block of pixels moved by a scroll: every pixel (x, y) of
// Rect took the value of pixel (x-DX, y-DY), which lies in Rect too.
type Shift struct {
	Rect   image.Rectangle
	DX, DY int
}

// Changed returns the rectangles whose pixels changed (the platform
// damage): the shifted blocks and the rasterized rectangles; nil when
// nothing changed or the frame is full.
func (d *Damage) Changed() []image.Rectangle {
	if d.Full {
		return nil
	}
	out := make([]image.Rectangle, 0, len(d.Shifts)+len(d.Rects))
	for _, s := range d.Shifts {
		out = append(out, s.Rect)
	}
	return append(out, d.Rects...)
}

// Empty reports that the frame changed no pixel.
func (d *Damage) Empty() bool { return !d.Full && len(d.Shifts) == 0 && len(d.Rects) == 0 }

// rasterizeApp brings fb to the last frame of Frame, at the tree's
// scale: fb holds the frame this method rasterized last (its pixels
// outside the damage are kept), unless full is set, the scale or the size
// of fb changed, or nothing was rasterized yet, in which case the frame is
// rasterized fully. The result is bit-identical to a full rasterization of
// the frame (Rasterize). It returns what it did, which LastDamage reports
// too. RasterizeFrame (debug.go) adds the debug overlays.
func (t *Tree) rasterizeApp(fb *image.RGBA, r *raster.Renderer, full bool) Damage {
	L := &t.lay
	f := L.cur
	if f == nil {
		return Damage{}
	}
	var d Damage
	switch {
	case full || L.ras == nil || L.ras.scale != f.scale || L.rasRect != fb.Rect:
		L.bdc.Reset()
		raster.Clear(fb, image.Rectangle{}, t.clearColor())
		r.Render(fb, f.dl, raster.Options{Scale: f.scale, Backdrops: &L.bdc})
		d.Full = true
	case L.ras != f:
		L.dmg.bdc = &L.bdc
		d = frameDamage(L.ras, f, fb.Rect, &L.dmg)
		L.dmg.bdc = nil
		if d.Full {
			raster.Clear(fb, image.Rectangle{}, t.clearColor())
			r.Render(fb, f.dl, raster.Options{Scale: f.scale, Backdrops: &L.bdc})
			break
		}
		for _, s := range d.Shifts {
			shiftPixels(fb, s)
		}
		canvas := t.clearColor()
		for _, rc := range d.Rects {
			raster.Clear(fb, rc, canvas)
			r.Render(fb, f.dl, raster.Options{Scale: f.scale, Region: rc, Backdrops: &L.bdc})
		}
	}
	L.ras, L.rasRect, L.damage = f, fb.Rect, d
	return d
}

// LastDamage returns what the last RasterizeFrame did (debug overlays:
// paint-flash).
func (t *Tree) LastDamage() Damage { return t.lay.damage }

// LastFrame returns the display list of the last paint (nil before the
// first).
func (t *Tree) LastFrame() *gfx.DisplayList {
	if t.lay.cur == nil {
		return nil
	}
	return t.lay.cur.dl
}

// shiftPixels moves the pixels of fb as s says.
func shiftPixels(fb *image.RGBA, s Shift) {
	r := s.Rect
	n := 4 * r.Dx()
	row := func(y int) {
		d := fb.PixOffset(r.Min.X, y)
		src := fb.PixOffset(r.Min.X-s.DX, y-s.DY)
		copy(fb.Pix[d:d+n], fb.Pix[src:src+n])
	}
	if s.DY > 0 {
		for y := r.Max.Y - 1; y >= r.Min.Y; y-- {
			row(y)
		}
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row(y)
	}
}

// ReferenceFrame composes the current state of the tree from scratch, as
// a fresh tree would: every layer recorded again, with interest rects
// around the current offsets (noCull: without skipping anything), into a
// display list that a full rasterization turns into the reference pixels
// of the last frame. It leaves the layers and frames of the tree as they
// were (tests check RasterizeFrame against it).
func (t *Tree) ReferenceFrame(noCull bool) *gfx.DisplayList {
	if t.rootBox == nil || t.rootBox.cs == nil {
		return t.lay.cur.dl
	}
	type saved struct {
		interest gfx.Rect
		recGen   uint64
		geo      [2]barGeom
	}
	keep := make([]saved, len(t.scr.list))
	for i, s := range t.scr.list {
		if s != nil {
			keep[i] = saved{s.interest, s.recGen, s.geo}
		}
	}
	lay, paints, moved := t.lay, t.stats.Paints, t.scr.moved
	t.lay = layerState{noCull: noCull, gen: lay.gen + 1}
	p := painter{t: t, scale: float64(t.scale), full: true, ref: true}
	p.place(t.rootBox, nil)
	t.recordAll(&p)
	dl := t.composite()
	t.lay, t.stats.Paints, t.scr.moved = lay, paints, moved
	for i, s := range t.scr.list {
		if s != nil {
			s.interest, s.recGen, s.geo = keep[i].interest, keep[i].recGen, keep[i].geo
		}
	}
	return dl
}

// LayerInfo describes a recorded layer (debug overlays: layers).
type LayerInfo struct {
	// Kind is "root", "scroll", "deferred" or "scrollbars".
	Kind string
	// Element is the element of the layer's box (nil for anonymous
	// boxes).
	Element *Element
	// Commands is the length of its list, and Recorded reports that the
	// last paint recorded it.
	Commands int
	Recorded bool
	// Bounds is where it was composed in the last frame (logical pixels,
	// window coordinates), the union of its commands.
	Bounds gfx.Rect
}

// Layers returns the layers of the last paint, in painting order (a
// layer before its children).
func (t *Tree) Layers() []LayerInfo {
	L := &t.lay
	var order []*layer
	var walk func(l *layer)
	walk = func(l *layer) {
		order = append(order, l)
		for _, k := range l.kids {
			walk(k)
		}
	}
	if L.root != nil {
		walk(L.root)
	}
	at := make(map[*layer]int, len(order))
	out := make([]LayerInfo, len(order))
	for i, l := range order {
		at[l] = i
		out[i].Kind = layerKindNames[l.kind]
		if l.box != nil {
			out[i].Element = l.box.el
		}
		if l.dl != nil {
			out[i].Commands = l.dl.Len()
		}
		out[i].Recorded = l.paint == L.paints
	}
	if f := L.cur; f != nil {
		for i := range f.dl.Len() {
			// The commands of an instance of layer l, or of a layer
			// inside it.
			for k := int32(f.dl.Tag(i)); k >= 0; k = f.inst[k].parent {
				if j, ok := at[f.inst[k].layer]; ok {
					out[j].Bounds = out[j].Bounds.Union(f.dl.BoundsAt(i))
				}
			}
		}
	}
	return out
}
