package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Partial recording (ADR-204): a paint records again only the layers whose
// lists can have changed.
//
// # Dependencies
//
// While a layer is recorded, the painter notes every box whose state the
// list reads, in three ways: what painting the box itself reads (its
// inputs, paintIn: its paint style, geometry, children, paragraph, image,
// field and the like; the layer that paints it notes it with dep), what
// the lists that place it read (paintOut: whether it is displayed, a
// participant of a stacking context and its z-index, whether it is a layer
// of its own, its scroll space, its children, its overflow clip; the lists
// that collect, clip around or cull it note it with odep), and, where the
// list culls, whether its ink met the interest rect (cdep). A box hoisted
// out of tree order is read in different ways by different layers: the
// list of its stacking context places it, its own chunk paints it, so a
// hover color on a card records the card's chunk and not the page's root.
//
// Each box keeps its inputs and placement as they were at its last place
// (rin, rout) and the epochs (layerState.epoch, advanced after every
// paint) at which they last changed. The paint phase compares them when it
// places a box (notePlaced, on frames that build, restyle or lay out) and
// where a frame that only repaints changes what a box records (noteInner:
// a color transition step, a BgFn). A layer is dirty when a box it noted
// changed after it was recorded, when a box it culled-tested would now be
// culled the other way (checked when an ink or an interest rect changed),
// when the interest rect of its scroll space moved, and always for a
// scrollbar layer (recorded on every paint) or a text field's layer that
// asked for it.
//
// # Recording
//
// recordDirty walks the layer tree from the root and records each dirty
// layer whose ancestors are clean again in place: the part of the painting
// code that filled its list runs with the painter state of its opening
// (scroll space, interest rect, culling). A layer opened while another is
// recorded (openLayer) takes the place of the old child layer with the
// same identity (kind, box, lead and hoisted box) and, when that one is
// clean, keeps its list: the marker is written and its content is not
// painted (a kept layer may hold dirty ones: the walk goes on inside it).
// The old children that nothing opened again are freed. A full recording
// (recordAll) remains for what the inputs do not cover: a new scale,
// viewport or canvas, paint-phase text colors (TextColorFn colors the text
// of whole subtrees when it is painted), the stale-layer cases of layer.go,
// and a recipe that cannot run in place (a structure change that the
// parent should have seen; it falls back rather than guess).
//
// TELA_CHECK_LAYERS=1 compares the frame composed after every partial
// recording with one composed from a full recording with the same interest
// rects, command for command, and panics on a difference (tests).

// envCheckLayers turns on the check of partial recordings.
const envCheckLayers = "TELA_CHECK_LAYERS"

// checkLayers reports TELA_CHECK_LAYERS=1 (read once).
var checkLayers = os.Getenv(envCheckLayers) == "1"

// kidsRef identifies a children slice: kids are replaced, never changed in
// place (structure.go), and the pointer to the first slot keeps the old
// array alive, so that a new one cannot have its address.
type kidsRef struct {
	first **Box
	n     int
}

func kidsOf(kids []*Box) kidsRef {
	if len(kids) == 0 {
		return kidsRef{}
	}
	return kidsRef{&kids[0], len(kids)}
}

// paintIn is what painting a box itself reads. It is comparable (==).
type paintIn struct {
	// cs is the paint style; for a box with paint-phase deferred props, its
	// computed style, with the deferred colors in bg and fg (the
	// compositor applies its translation and opacity).
	cs     *style.ComputedStyle
	bg, fg gfx.Color
	u      gfx.Rect
	// The edges of the layout (the size is u's).
	border, padding layout.Insets
	bars            gfx.Size
	kids            kidsRef
	para            *paragraph
	lay             *text.Layout
	pver            uint64
	img             *gfx.Image
	asset           *imageAsset
	tint            tintValue
	fld             *field
	// ink: a mask can clip to it. It grows with what the box's in-flow
	// descendants paint, which the same list paints.
	ink    gfx.Rect
	lscale gfx.Point
	mode   boxMode
	flags  uint8
}

// paintIn flags.
const (
	inChunked  = 1 << iota // a scroll container whose content is a chunk
	inBlend                // isolates blending descendants
	inDeferred             // a deferred layer
	inAnimated             // a layout animation runs
)

// paintOut is what the lists that place a box read. It is comparable.
type paintOut struct {
	space *scroller
	kids  kidsRef
	clip  gfx.RRect // its overflow clip, when it clips
	z     int32
	pos   style.Position
	flags uint16
}

// paintOut flags.
const (
	outNone     = 1 << iota // display: none
	outHidden               // an atomic inline hidden by its paragraph
	outPart                 // a participant of a stacking context
	outStacking             // a stacking context
	outHasPart              // has participants below
	outClips                // clips its content
	outContains             // contains positioned descendants
	outDeferred             // a deferred layer
)

// cullDep is a box that a culling list tested against the interest rect
// of scroll space space, and what it found (cdep).
type cullDep struct {
	box   *Box
	space *scroller
	vis   bool
}

// deferredLayer reports whether bx is painted in a deferred layer.
func deferredLayer(bx *Box) bool {
	return bx.layerSC && bx.deferPaint && bx != bx.tree.rootBox
}

// styleBits are what placing a box reads of its styles, cached on the box
// for its paint and computed styles (Box.sbFor): the box's fields are at
// hand, the styles' are not.
type styleBits struct {
	pcs, cs *style.ComputedStyle
	flags   uint8
	z       int32
	pos     style.Position
}

// styleBits flags.
const (
	sbPositioned = 1 << iota // not static
	sbZ                      // a z-index is set
	sbStacks                 // a stacking context by its paint style alone (stacksByStyle)
	sbClips                  // clips its content
	sbContains               // contains positioned descendants by its computed style (containsByStyle)
)

// bits returns the style bits of placed box bx.
func (bx *Box) bits() *styleBits {
	sb := &bx.sb
	if sb.pcs != bx.pcs || sb.cs != bx.cs {
		cs := bx.pcs
		f := uint8(0)
		if cs.Position != style.PositionStatic {
			f |= sbPositioned
		}
		if cs.ZIndex.Explicit {
			f |= sbZ
		}
		if stacksByStyle(cs) {
			f |= sbStacks
		}
		if clipsOverflow(cs) {
			f |= sbClips
		}
		if containsByStyle(bx.cs) {
			f |= sbContains
		}
		*sb = styleBits{pcs: bx.pcs, cs: bx.cs, flags: f, z: cs.ZIndex.Value, pos: cs.Position}
	}
	return sb
}

// inputsIn returns the inputs of placed box bx.
func (p *painter) inputsIn(bx *Box) paintIn {
	l := bx.ld.Layout()
	in := paintIn{
		cs: bx.pcs, u: bx.u, border: l.Border, padding: l.Padding, bars: l.ScrollbarSize,
		kids: kidsOf(bx.kids), mode: bx.mode, img: bx.img, fld: bx.fld, ink: bx.ink,
	}
	if bx.deferPaint {
		in.cs, in.bg, in.fg = bx.cs, bx.pcs.BackgroundColor, bx.pcs.Text.Color
	}
	if la := bx.lanim; la != nil {
		// A layout animation corrects radii and shadows in the paint
		// style it makes each place.
		in.cs, in.lscale = bx.pcs, la.scale
		if la.active {
			in.flags |= inAnimated
		}
	}
	if pa := bx.para; pa != nil {
		in.para, in.lay, in.pver = pa, pa.lay, pa.ver
	}
	if m := bx.med; m != nil {
		in.asset = m.asset
	}
	if bx.el != nil && bx.mode == modeImage {
		in.tint = bx.el.node.d.media.tintOf()
	}
	if s := bx.scr; s != nil && s.chunked {
		in.flags |= inChunked
	}
	if bx.blendDesc {
		in.flags |= inBlend
	}
	if deferredLayer(bx) {
		in.flags |= inDeferred
	}
	return in
}

// inputsOut returns what the lists that place placed box bx read: its
// participation and z-index (participant, zIndex), stacking (stacking),
// containment (containsPositioned) and overflow clip, from its style bits.
func (p *painter) inputsOut(bx *Box) paintOut {
	sb := bx.bits()
	out := paintOut{space: bx.space, kids: kidsOf(bx.kids), pos: sb.pos}
	f := uint16(0)
	if bx.ls.Display == layout.DisplayNone {
		f |= outNone
	}
	if bx.hiddenAtom {
		f |= outHidden
	}
	zApplies := sb.flags&sbZ != 0 && (sb.flags&sbPositioned != 0 || flexItem(bx))
	st := zApplies || sb.flags&sbStacks != 0 || stacksByState(bx)
	if st {
		f |= outStacking
	}
	if sb.flags&sbPositioned != 0 || st {
		f |= outPart
		if zApplies {
			out.z = sb.z
		}
	}
	if bx.hasPart {
		f |= outHasPart
	}
	if sb.flags&sbClips != 0 {
		f |= outClips
		out.clip = p.overflowClip(bx)
	}
	if bx.anim != nil && containsPositioned(bx) || bx.anim == nil && sb.flags&sbContains != 0 {
		f |= outContains
	}
	if deferredLayer(bx) {
		f |= outDeferred
	}
	out.flags = f
	return out
}

// notePlaced compares the inputs and the placement of bx, just placed, with
// those of its last place, and stamps what changed. Only what some list
// reads is compared (Box.rinN, routN): most boxes of a long list are
// culled with an ancestor and read by none.
func (p *painter) notePlaced(bx *Box) {
	if p.ref {
		return
	}
	L := &p.t.lay
	if bx.rinN > 0 {
		if in := p.inputsIn(bx); in != bx.rin {
			bx.rin, bx.rinAt = in, L.epoch
			L.nchg++
		}
	}
	if bx.routN > 0 {
		if out := p.inputsOut(bx); out != bx.rout {
			bx.rout, bx.routAt = out, L.epoch
			L.nchg++
		}
	}
}

// noneOut is the placement of a box that is not displayed.
var noneOut = paintOut{flags: outNone}

// noteNone notes that bx, a child being placed, is not displayed.
func (p *painter) noteNone(bx *Box) {
	if p.ref || bx.routN == 0 || bx.rout == noneOut {
		return
	}
	L := &p.t.lay
	bx.rout, bx.routAt = noneOut, L.epoch
	L.nchg++
}

// noteInner stamps the inputs of placed box bx as changed on a frame that
// does not place the boxes (an animation step, a deferred color). An
// inline element is painted by its paragraph: its paragraph box changes.
func (t *Tree) noteInner(bx *Box) {
	L := &t.lay
	if pb := bx.inPara; pb != nil {
		if pb.para != nil {
			pb.para.ver++
		}
		bx = pb
	}
	if bx.pcs == nil {
		return
	}
	p := painter{t: t, scale: float64(t.scale)}
	bx.rin, bx.rinAt = p.inputsIn(bx), L.epoch
	L.nchg++
	if bx.routN > 0 {
		// A radius transition changes the overflow clip that the lists of
		// hoisted descendants push.
		if out := p.inputsOut(bx); out != bx.rout {
			bx.rout, bx.routAt = out, L.epoch
		}
	}
}

// dep notes that the list being recorded reads the inputs of bx. A box
// that no list read has its inputs noted now, as they are recorded.
// Reference recordings note nothing.
func (p *painter) dep(bx *Box) {
	l := p.layer
	if l == nil || p.ref {
		return
	}
	l.in = append(l.in, bx)
	if bx.rinN == 0 && bx.pcs != nil {
		bx.rin = p.inputsIn(bx)
	}
	bx.rinN++
}

// odep notes that the list being recorded reads how bx is placed.
func (p *painter) odep(bx *Box) {
	l := p.layer
	if l == nil || p.ref {
		return
	}
	l.out = append(l.out, bx)
	if bx.routN == 0 {
		if bx.pcs == nil || bx.ls.Display == layout.DisplayNone {
			bx.rout = noneOut
		} else {
			bx.rout = p.inputsOut(bx)
		}
	}
	bx.routN++
}

// cdep notes that the list being recorded culls bx when its ink misses the
// interest rect of scroll space s, and reports whether it meets it.
func (p *painter) cdep(bx *Box, s *scroller) bool {
	vis := bx.ink.Intersects(p.interestOf(s))
	if l := p.layer; l != nil && !p.ref {
		l.cul = append(l.cul, cullDep{bx, s, vis})
	}
	return vis
}

// drop forgets what layer l read (it is freed), and what it read before
// its last recording.
func (l *layer) drop() {
	l.dropOld()
	l.in, l.oin = l.oin, l.in
	l.out, l.oout = l.oout, l.out
	l.dropOld()
	clear(l.cul)
	l.cul = l.cul[:0]
}

// dropOld forgets what layer l read before the recording that ends (its
// old lists, kept until then so that the boxes it reads again keep their
// noted inputs).
func (l *layer) dropOld() {
	for _, b := range l.oin {
		b.rinN--
	}
	for _, b := range l.oout {
		b.routN--
	}
	clear(l.oin)
	clear(l.oout)
	l.oin, l.oout = l.oin[:0], l.oout[:0]
}

// stale reports whether layer l must be recorded again.
func (L *layerState) stale(l *layer, p *painter) bool {
	switch l.kind {
	case layerBars:
		return true
	case layerField:
		if f := l.box.fld; f == nil || f.paintDirty {
			return true
		}
	}
	if l.rint != p.interestOf(l.space) {
		return true
	}
	if L.nchg > 0 {
		for _, b := range l.in {
			if b.rinAt > l.rec {
				return true
			}
		}
		for _, b := range l.out {
			if b.routAt > l.rec {
				return true
			}
		}
	}
	if L.recull {
		for i := range l.cul {
			if c := &l.cul[i]; c.box.ink.Intersects(p.interestOf(c.space)) != c.vis {
				return true
			}
		}
	}
	return false
}

// recordDirty records again the layers that changed (see the top of this
// file). It reports false, having changed nothing, when a dirty layer
// cannot be recorded in place: the caller records everything.
func (t *Tree) recordDirty(p *painter) bool {
	L := &t.lay
	if L.root == nil {
		return false
	}
	for _, l := range L.layers {
		if l != nil {
			l.dirty = L.stale(l, p)
		}
	}
	// A dirty layer under clean ancestors is recorded in place; the
	// layers it opens again are recorded with it (the dirty ones) or kept,
	// and a kept one may hold dirty layers: the walk goes on below.
	ok := true
	var walk func(l *layer)
	walk = func(l *layer) {
		if l.dirty {
			if !p.canRecord(l) {
				ok = false
				return
			}
			p.recordLayer(l)
		}
		for i := 0; ok && i < len(l.kids); i++ {
			walk(l.kids[i])
		}
	}
	walk(L.root)
	p.b, p.layer = nil, nil
	return ok
}

// canRecord reports whether layer l can be recorded in place: the boxes
// its recipe paints still open it where it is.
func (p *painter) canRecord(l *layer) bool {
	t := p.t
	placed := func(b *Box) bool { return b != nil && b.pcs != nil && t.inTree(b) }
	switch l.kind {
	case layerRoot:
		return l.box == t.rootBox
	case layerDeferred:
		return placed(l.box) && deferredLayer(l.box) && l.box.space == l.space
	case layerBars:
		return placed(l.box) && l.box.scr != nil
	case layerField:
		return placed(l.box) && l.box.fld != nil && l.box.mode == modeField
	case layerChunk:
		if l.hoist == nil {
			b := l.lead
			return placed(b) && b.scr != nil && b.scr.chunked && b.scr == l.space
		}
		e := l.hoist
		if !placed(e) || !placed(l.sc) || !participant(e) {
			return false
		}
		if l.lead == e {
			return e.space == l.space
		}
		start := len(p.clips)
		p.clipChain(l.sc, e, false)
		ok := false
		for _, c := range p.clips[start:] {
			if c == l.lead {
				ok = c.space == l.space
			}
		}
		p.clips = p.clips[:start]
		return ok
	}
	return false
}

// recordLayer records layer l again in place, with the painter state of
// its opening.
func (p *painter) recordLayer(l *layer) {
	L := &p.t.lay
	p.b, p.layer = L.builder(), l
	p.space, p.interest, p.cull = l.space, p.interestOf(l.space), l.cull
	p.targets = p.targets[:0]
	l.begin(L, p.interest, l.cull)
	switch l.kind {
	case layerRoot:
		t := p.t
		p.b.FillRect(gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}, t.CanvasColor())
		p.paintStacked(t.rootBox)
	case layerDeferred:
		p.dep(l.box)
		p.stackBody(l.box)
	case layerBars:
		p.dep(l.box)
		p.drawBars(l.box)
	case layerField:
		p.dep(l.box)
		l.interest = unbounded
		if l.cull {
			l.interest = p.interest
		}
		p.drawField(l.box, l.interest)
	case layerChunk:
		if l.hoist == nil {
			p.dep(l.lead)
			p.innerBody(l.lead)
			break
		}
		e := l.hoist
		if l.lead == e {
			p.hoistBody(e)
			break
		}
		start := len(p.clips)
		p.clipChain(l.sc, e, false)
		clips := p.clips[start:]
		for i, c := range clips {
			if c == l.lead {
				p.hoistChain(l.sc, e, clips, i, true)
				break
			}
		}
		p.clips = p.clips[:start]
	}
	p.endLayer(l)
}

// begin starts a recording of l: its list, dependencies and children are
// recorded anew, the old children waiting to be opened again (pool).
func (l *layer) begin(L *layerState, interest gfx.Rect, cull bool) {
	l.pool, l.kids, l.pi = l.kids, nil, 0
	l.dropOld() // empty: the last recording ended (endLayer)
	l.in, l.oin = l.oin, l.in
	l.out, l.oout = l.oout, l.out
	clear(l.cul)
	l.cul = l.cul[:0]
	l.rint, l.cull = interest, cull
	l.rec, l.paint, l.dirty = L.epoch, L.paints, false
	L.recorded++
}

// endLayer finishes the recording of l: its list, and the old children
// that were not opened again are freed.
func (p *painter) endLayer(l *layer) {
	L := &p.t.lay
	l.dl = p.b.Finish()
	L.free = append(L.free, p.b)
	l.dropOld()
	for _, k := range l.pool {
		if k != nil {
			L.freeLayer(k)
		}
	}
	clear(l.pool)
	l.pool = nil
}

// take returns the old child of l with the given identity, removing it
// from the pool, or nil.
func (l *layer) take(kind layerKind, box *Box, space *scroller, lead, hoist *Box) *layer {
	n := len(l.pool)
	for i := range n {
		j := l.pi + i
		if j >= n {
			j -= n
		}
		k := l.pool[j]
		if k != nil && k.kind == kind && k.box == box && k.space == space && k.lead == lead && k.hoist == hoist {
			l.pool[j] = nil
			l.pi = j + 1
			return k
		}
	}
	return nil
}

// freeLayer frees layer l and the layers inside it.
func (L *layerState) freeLayer(l *layer) {
	for _, k := range l.kids {
		L.freeLayer(k)
	}
	for _, k := range l.pool {
		if k != nil {
			L.freeLayer(k)
		}
	}
	if int(l.id) < len(L.layers) && L.layers[l.id] == l {
		L.layers[l.id] = nil
		L.freeIDs = append(L.freeIDs, l.id)
	}
	l.drop()
	l.kids, l.pool, l.in, l.out, l.oin, l.oout, l.cul, l.dl = nil, nil, nil, nil, nil, nil, nil, nil
}

// checkPartial compares the frame f composed after a partial recording
// with a full recording of the same boxes and interest rects
// (TELA_CHECK_LAYERS=1).
func (t *Tree) checkPartial(f *frame) {
	type saved struct{ recGen uint64 }
	keep := make([]saved, len(t.scr.list))
	for i, s := range t.scr.list {
		if s != nil {
			keep[i] = saved{s.recGen}
		}
	}
	lay := t.lay
	t.lay = layerState{noCull: lay.noCull, gen: lay.gen + 1}
	p := painter{t: t, scale: float64(t.scale), ref: true}
	t.recordAll(&p)
	g := t.composite()
	t.lay = lay
	for i, s := range t.scr.list {
		if s != nil {
			s.recGen = keep[i].recGen
		}
	}
	a, b := f.dl, g
	n := min(a.Len(), b.Len())
	for i := range n {
		if !gfx.Equal(a.At(i), b.At(i)) || a.BoundsAt(i) != b.BoundsAt(i) {
			panic(fmt.Sprintf("tela: %s: a partial recording differs from a full one at command %d of %d/%d (layer %s):\npartial: %s\nfull:    %s",
				envCheckLayers, i, a.Len(), b.Len(), t.layerOfCommand(f, i), cmdLine(a, i), cmdLine(b, i)))
		}
	}
	if a.Len() != b.Len() {
		panic(fmt.Sprintf("tela: %s: a partial recording has %d commands, a full one %d", envCheckLayers, a.Len(), b.Len()))
	}
}

// layerOfCommand describes the layer of command i of frame f.
func (t *Tree) layerOfCommand(f *frame, i int) string {
	in := f.inst[f.dl.Tag(i)]
	l := in.layer
	name := func(b *Box) string {
		if b == nil || b.el == nil {
			return "-"
		}
		return t.describe(b.el)
	}
	return fmt.Sprintf("%s box %s lead %s hoist %s recorded at paint %d of %d", layerKindNames[l.kind], name(l.box), name(l.lead), name(l.hoist), l.paint, t.lay.paints)
}

// cmdLine returns the line of command i in the dump of dl (one line per
// command).
func cmdLine(dl *gfx.DisplayList, i int) string {
	if lines := strings.Split(dl.String(), "\n"); i < len(lines) {
		return strings.TrimSpace(lines[i])
	}
	return "?"
}
